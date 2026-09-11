package main

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/jhillyerd/enmime/v2"
)

// FolderInfo represents a mailbox folder with counts and hierarchy metadata.
type FolderInfo struct {
	Name        string   `json:"name"`
	TotalCount  int      `json:"total_count"`
	UnreadCount int      `json:"unread_count"`
	Delimiter   string   `json:"delimiter,omitempty"`
	Attrs       []string `json:"attrs,omitempty"`
	Selectable  bool     `json:"selectable"`
}

// MessageSummary represents a message without body content (for listings).
type MessageSummary struct {
	UID     uint32   `json:"uid"`
	Subject string   `json:"subject"`
	From    string   `json:"from"`
	To      []string `json:"to"`
	Date    string   `json:"date"`
	Flags   []string `json:"flags"`
	Size    int64    `json:"size"`
}

// AttachmentInfo represents an attachment's metadata.
type AttachmentInfo struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}

// FullMessage represents a complete message with body and attachments.
type FullMessage struct {
	UID         uint32           `json:"uid"`
	Subject     string           `json:"subject"`
	From        string           `json:"from"`
	To          []string         `json:"to"`
	CC          []string         `json:"cc"`
	BCC         []string         `json:"bcc"`
	Date        string           `json:"date"`
	MessageID   string           `json:"message_id"`
	Flags       []string         `json:"flags"`
	Headers     map[string]string `json:"headers"`
	BodyText    string           `json:"body_text"`
	BodyHTML    string           `json:"body_html"`
	Truncated   bool             `json:"truncated"`
	Attachments []AttachmentInfo  `json:"attachments"`
}

// maxBodyLen limits body text returned to the LLM to protect context windows.
const maxBodyLen = 50 * 1024

// ListFolders returns all mailbox folders with message counts, listed recursively.
// Container folders (\Noselect) are included but marked as not selectable.
func (s *MailService) ListFolders() ([]FolderInfo, error) {
	var folders []FolderInfo
	err := s.withIMAP(func(c *imapclient.Client) error {
		// Use "*" for recursive listing — shows all subfolders like "Folders/ben@b7r.dev"
		listData, err := c.List("", "*", nil).Collect()
		if err != nil {
			return fmt.Errorf("listing folders: %w", err)
		}

		for _, mbox := range listData {
			selectable := true
			for _, attr := range mbox.Attrs {
				if attr == imap.MailboxAttrNoSelect {
					selectable = false
					break
				}
			}

			fi := FolderInfo{
				Name:       mbox.Mailbox,
				Delimiter:  string(mbox.Delim),
				Selectable: selectable,
			}
			for _, attr := range mbox.Attrs {
				fi.Attrs = append(fi.Attrs, string(attr))
			}

			// Only fetch counts for selectable folders
			if selectable {
				status, err := c.Status(mbox.Mailbox, &imap.StatusOptions{
					NumMessages: true,
					NumUnseen:   true,
				}).Wait()
				if err != nil {
					s.log.Warn("status failed for folder", "folder", mbox.Mailbox, "error", err)
				} else {
					if status.NumMessages != nil {
						fi.TotalCount = int(*status.NumMessages)
					}
					if status.NumUnseen != nil {
						fi.UnreadCount = int(*status.NumUnseen)
					}
				}
			}

			folders = append(folders, fi)
		}

		// Append virtual address folders (only if configured programmatically)
		if s.hasVirtualFolders() {
			if _, err := c.Select(allMailFolder, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
				s.log.Warn("could not select All Mail for virtual folder counts", "error", err)
			} else {
				for _, addr := range s.cfg.FolderAddresses {
					fi := FolderInfo{Name: addr, Selectable: true, Delimiter: "/"}

					totalData, err := c.UIDSearch(&imap.SearchCriteria{
						Header: []imap.SearchCriteriaHeaderField{{Key: "To", Value: addr}},
					}, nil).Wait()
					if err == nil {
						fi.TotalCount = len(totalData.AllUIDs())
					}

					unreadData, err := c.UIDSearch(&imap.SearchCriteria{
						Header:  []imap.SearchCriteriaHeaderField{{Key: "To", Value: addr}},
						NotFlag: []imap.Flag{imap.FlagSeen},
					}, nil).Wait()
					if err == nil {
						fi.UnreadCount = len(unreadData.AllUIDs())
					}

					folders = append(folders, fi)
				}
			}
		}

		return nil
	})
	return folders, err
}

// ListMessages returns recent messages from a folder.
// If the folder is a virtual address folder, searches All Mail filtered by To: header.
func (s *MailService) ListMessages(folder string, limit int, unreadOnly bool) ([]MessageSummary, error) {
	if folder == "" {
		folder = "INBOX"
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	// Resolve virtual folder to real IMAP folder + address filter
	virtAddr, isVirtual := s.resolveVirtualFolder(folder)
	imapFolder := folder
	if isVirtual {
		imapFolder = allMailFolder
	}

	var messages []MessageSummary
	err := s.withIMAP(func(c *imapclient.Client) error {
		// Use EXAMINE (read-only) so we don't affect \Recent flags
		selectData, err := c.Select(imapFolder, &imap.SelectOptions{ReadOnly: true}).Wait()
		if err != nil {
			return fmt.Errorf("selecting folder %q: %w", imapFolder, err)
		}
		total := selectData.NumMessages

		if isVirtual {
			// Virtual folder: search All Mail by To: header
			criteria := &imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{{Key: "To", Value: virtAddr}},
			}
			if unreadOnly {
				criteria.NotFlag = []imap.Flag{imap.FlagSeen}
			}
			searchData, err := c.UIDSearch(criteria, nil).Wait()
			if err != nil {
				return fmt.Errorf("searching virtual folder: %w", err)
			}
			uids := searchData.AllUIDs()
			if len(uids) == 0 {
				return nil
			}
			if len(uids) > limit {
				uids = uids[len(uids)-limit:]
			}
			uidSet := imap.UIDSetNum(uids...)

			fetchOpts := &imap.FetchOptions{
				Envelope:   true,
				Flags:      true,
				RFC822Size: true,
				UID:        true,
			}
			bufs, err := c.Fetch(uidSet, fetchOpts).Collect()
			if err != nil {
				return fmt.Errorf("fetching messages: %w", err)
			}
			for _, buf := range bufs {
				messages = append(messages, bufToSummary(buf))
			}
			return nil
		}

		// Real folder: original logic
		if total == 0 && !unreadOnly {
			return nil
		}

		var uidSet imap.UIDSet
		if unreadOnly {
			searchData, err := c.UIDSearch(&imap.SearchCriteria{
				NotFlag: []imap.Flag{imap.FlagSeen},
			}, nil).Wait()
			if err != nil {
				return fmt.Errorf("searching unread: %w", err)
			}
			uids := searchData.AllUIDs()
			// Take the last `limit` UIDs (most recent unread)
			if len(uids) > limit {
				uids = uids[len(uids)-limit:]
			}
			if len(uids) == 0 {
				return nil
			}
			uidSet = imap.UIDSetNum(uids...)
		} else {
			// Fetch by sequence number: last N messages
			start := uint32(1)
			if total > uint32(limit) {
				start = total - uint32(limit) + 1
			}
			seqSet := imap.SeqSetNum()
			seqSet.AddRange(start, total)

			fetchOpts := &imap.FetchOptions{
				Envelope:   true,
				Flags:      true,
				RFC822Size: true,
				UID:        true,
			}
			bufs, err := c.Fetch(seqSet, fetchOpts).Collect()
			if err != nil {
				return fmt.Errorf("fetching messages: %w", err)
			}
			for _, buf := range bufs {
				messages = append(messages, bufToSummary(buf))
			}
			return nil
		}

		fetchOpts := &imap.FetchOptions{
			Envelope:   true,
			Flags:      true,
			RFC822Size: true,
		}
		bufs, err := c.Fetch(uidSet, fetchOpts).Collect()
		if err != nil {
			return fmt.Errorf("fetching messages: %w", err)
		}
		for _, buf := range bufs {
			messages = append(messages, bufToSummary(buf))
		}
		return nil
	})
	return messages, err
}

// SearchMessages searches for messages matching a text query.
// If the folder is a virtual address folder, searches within All Mail filtered by To: header.
func (s *MailService) SearchMessages(query, folder string, limit int) ([]MessageSummary, error) {
	if folder == "" {
		folder = "INBOX"
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Resolve virtual folder
	virtAddr, isVirtual := s.resolveVirtualFolder(folder)
	imapFolder := folder
	if isVirtual {
		imapFolder = allMailFolder
	}

	var messages []MessageSummary
	err := s.withIMAP(func(c *imapclient.Client) error {
		_, err := c.Select(imapFolder, &imap.SelectOptions{ReadOnly: true}).Wait()
		if err != nil {
			return fmt.Errorf("selecting folder %q: %w", imapFolder, err)
		}

		criteria := &imap.SearchCriteria{
			Text: []string{query},
		}
		if isVirtual {
			criteria.Header = []imap.SearchCriteriaHeaderField{{Key: "To", Value: virtAddr}}
		}

		searchData, err := c.UIDSearch(criteria, nil).Wait()
		if err != nil {
			return fmt.Errorf("searching: %w", err)
		}

		uids := searchData.AllUIDs()
		if len(uids) == 0 {
			return nil
		}
		// Take the last `limit` UIDs (most recent matches)
		if len(uids) > limit {
			uids = uids[len(uids)-limit:]
		}
		uidSet := imap.UIDSetNum(uids...)

		fetchOpts := &imap.FetchOptions{
			Envelope: true,
			Flags:    true,
		}
		bufs, err := c.Fetch(uidSet, fetchOpts).Collect()
		if err != nil {
			return fmt.Errorf("fetching search results: %w", err)
		}
		for _, buf := range bufs {
			messages = append(messages, bufToSummary(buf))
		}
		return nil
	})
	return messages, err
}

// GetMessage returns a full message including body and attachments.
func (s *MailService) GetMessage(uid uint32, folder string) (*FullMessage, error) {
	if folder == "" {
		folder = "INBOX"
	}

	// Resolve virtual folder — UIDs are valid in All Mail
	imapFolder := folder
	if _, isVirtual := s.resolveVirtualFolder(folder); isVirtual {
		imapFolder = allMailFolder
	}

	var msg *FullMessage
	err := s.withIMAP(func(c *imapclient.Client) error {
		_, err := c.Select(imapFolder, &imap.SelectOptions{ReadOnly: true}).Wait()
		if err != nil {
			return fmt.Errorf("selecting folder %q: %w", imapFolder, err)
		}

		uidSet := imap.UIDSetNum(imap.UID(uid))
		bodySection := &imap.FetchItemBodySection{Peek: true}
		fetchOpts := &imap.FetchOptions{
			BodySection: []*imap.FetchItemBodySection{bodySection},
			Flags:       true,
			Envelope:    true,
			UID:         true,
		}
		bufs, err := c.Fetch(uidSet, fetchOpts).Collect()
		if err != nil {
			return fmt.Errorf("fetching message: %w", err)
		}
		if len(bufs) == 0 {
			return fmt.Errorf("message with UID %d not found in %q", uid, folder)
		}

		buf := bufs[0]
		rawBody := buf.FindBodySection(bodySection)
		if rawBody == nil {
			return fmt.Errorf("message body not found for UID %d", uid)
		}

		// Parse MIME with enmime
		env, err := enmime.ReadEnvelope(bytes.NewReader(rawBody))
		if err != nil {
			return fmt.Errorf("parsing message: %w", err)
		}

		fm := &FullMessage{
			UID:       uint32(buf.UID),
			Subject:   env.GetHeader("Subject"),
			From:      env.GetHeader("From"),
			MessageID: env.GetHeader("Message-ID"),
			Flags:     flagsToStrings(buf.Flags),
		}

		// Recipients
		fm.To = addressListFromHeader(env, "To")
		fm.CC = addressListFromHeader(env, "Cc")
		fm.BCC = addressListFromHeader(env, "Bcc")

		if date := env.GetHeader("Date"); date != "" {
			fm.Date = date
		}

		// Headers map (selected useful headers)
		fm.Headers = make(map[string]string)
		for _, key := range []string{"Date", "From", "To", "Cc", "Bcc", "Subject",
			"Message-ID", "In-Reply-To", "References", "Reply-To"} {
			if v := env.GetHeader(key); v != "" {
				fm.Headers[key] = v
			}
		}

		// Body
		fm.BodyText = env.Text
		fm.BodyHTML = env.HTML
		if len(fm.BodyText) > maxBodyLen {
			fm.BodyText = fm.BodyText[:maxBodyLen]
			fm.Truncated = true
		}

		// Attachments
		for _, att := range env.Attachments {
			fm.Attachments = append(fm.Attachments, AttachmentInfo{
				Filename:    att.FileName,
				ContentType: att.ContentType,
				Size:        len(att.Content),
			})
		}
		for _, att := range env.Inlines {
			if att.FileName != "" {
				fm.Attachments = append(fm.Attachments, AttachmentInfo{
					Filename:    att.FileName,
					ContentType: att.ContentType,
					Size:        len(att.Content),
				})
			}
		}

		msg = fm
		return nil
	})
	return msg, err
}

// MoveMessage moves a message from one folder to another.
func (s *MailService) MoveMessage(uid uint32, folder, destFolder string) error {
	// Resolve virtual source folder — UIDs are valid in All Mail
	imapFolder := folder
	if _, isVirtual := s.resolveVirtualFolder(folder); isVirtual {
		imapFolder = allMailFolder
	}
	return s.withIMAP(func(c *imapclient.Client) error {
		_, err := c.Select(imapFolder, nil).Wait()
		if err != nil {
			return fmt.Errorf("selecting folder %q: %w", imapFolder, err)
		}
		uidSet := imap.UIDSetNum(imap.UID(uid))
		if _, err := c.Move(uidSet, destFolder).Wait(); err != nil {
			return fmt.Errorf("moving message: %w", err)
		}
		return nil
	})
}

// MarkRead sets the \Seen flag on a message.
func (s *MailService) MarkRead(uid uint32, folder string) error {
	imapFolder := folder
	if _, isVirtual := s.resolveVirtualFolder(folder); isVirtual {
		imapFolder = allMailFolder
	}
	return s.withIMAP(func(c *imapclient.Client) error {
		_, err := c.Select(imapFolder, nil).Wait()
		if err != nil {
			return fmt.Errorf("selecting folder %q: %w", imapFolder, err)
		}
		uidSet := imap.UIDSetNum(imap.UID(uid))
		if err := c.Store(uidSet, &imap.StoreFlags{
			Op:    imap.StoreFlagsAdd,
			Flags: []imap.Flag{imap.FlagSeen},
		}, nil).Close(); err != nil {
			return fmt.Errorf("marking message as read: %w", err)
		}
		return nil
	})
}

// MarkUnread removes the \Seen flag from a message.
func (s *MailService) MarkUnread(uid uint32, folder string) error {
	imapFolder := folder
	if _, isVirtual := s.resolveVirtualFolder(folder); isVirtual {
		imapFolder = allMailFolder
	}
	return s.withIMAP(func(c *imapclient.Client) error {
		_, err := c.Select(imapFolder, nil).Wait()
		if err != nil {
			return fmt.Errorf("selecting folder %q: %w", imapFolder, err)
		}
		uidSet := imap.UIDSetNum(imap.UID(uid))
		if err := c.Store(uidSet, &imap.StoreFlags{
			Op:    imap.StoreFlagsDel,
			Flags: []imap.Flag{imap.FlagSeen},
		}, nil).Close(); err != nil {
			return fmt.Errorf("marking message as unread: %w", err)
		}
		return nil
	})
}

// DeleteMessage deletes a message. If not in Trash, moves to Trash.
// If already in Trash, permanently deletes it.
func (s *MailService) DeleteMessage(uid uint32, folder string) (permanent bool, err error) {
	// Resolve virtual folder — UIDs are valid in All Mail
	imapFolder := folder
	if _, isVirtual := s.resolveVirtualFolder(folder); isVirtual {
		imapFolder = allMailFolder
	}

	if strings.EqualFold(imapFolder, "Trash") {
		// Permanent deletion
		return true, s.withIMAP(func(c *imapclient.Client) error {
			_, err := c.Select(imapFolder, nil).Wait()
			if err != nil {
				return fmt.Errorf("selecting folder %q: %w", imapFolder, err)
			}
			uidSet := imap.UIDSetNum(imap.UID(uid))
			if err := c.Store(uidSet, &imap.StoreFlags{
				Op:    imap.StoreFlagsAdd,
				Flags: []imap.Flag{imap.FlagDeleted},
			}, nil).Close(); err != nil {
				return fmt.Errorf("marking message for deletion: %w", err)
			}
			if err := c.Expunge().Close(); err != nil {
				return fmt.Errorf("expunging: %w", err)
			}
			return nil
		})
	}

	// Move to Trash
	return false, s.withIMAP(func(c *imapclient.Client) error {
		_, err := c.Select(imapFolder, nil).Wait()
		if err != nil {
			return fmt.Errorf("selecting folder %q: %w", imapFolder, err)
		}
		uidSet := imap.UIDSetNum(imap.UID(uid))
		if _, err := c.Move(uidSet, "Trash").Wait(); err != nil {
			return fmt.Errorf("moving message to Trash: %w", err)
		}
		return nil
	})
}

// CreateFolder creates a new mailbox folder.
func (s *MailService) CreateFolder(name string) error {
	return s.withIMAP(func(c *imapclient.Client) error {
		if err := c.Create(name, nil).Wait(); err != nil {
			return fmt.Errorf("creating folder %q: %w", name, err)
		}
		return nil
	})
}

// RenameFolder renames a mailbox folder.
func (s *MailService) RenameFolder(oldName, newName string) error {
	return s.withIMAP(func(c *imapclient.Client) error {
		if err := c.Rename(oldName, newName, nil).Wait(); err != nil {
			return fmt.Errorf("renaming folder %q to %q: %w", oldName, newName, err)
		}
		return nil
	})
}

// DeleteFolder deletes a mailbox folder.
func (s *MailService) DeleteFolder(name string) error {
	return s.withIMAP(func(c *imapclient.Client) error {
		if err := c.Delete(name).Wait(); err != nil {
			return fmt.Errorf("deleting folder %q: %w", name, err)
		}
		return nil
	})
}

// bufToSummary converts a FetchMessageBuffer to a MessageSummary.
func bufToSummary(buf *imapclient.FetchMessageBuffer) MessageSummary {
	ms := MessageSummary{
		UID:   uint32(buf.UID),
		Size:  buf.RFC822Size,
		Flags: flagsToStrings(buf.Flags),
	}
	if buf.Envelope != nil {
		ms.Subject = buf.Envelope.Subject
		ms.From = envelopeFromAddr(buf.Envelope.From)
		ms.To = envelopeAddrList(buf.Envelope.To)
		if !buf.Envelope.Date.IsZero() {
			ms.Date = buf.Envelope.Date.Format(time.RFC3339)
		}
	}
	return ms
}

// envelopeFromAddr formats an imap.Address slice as a single "Name <email>" string.
func envelopeFromAddr(addrs []imap.Address) string {
	if len(addrs) == 0 {
		return ""
	}
	var parts []string
	for _, a := range addrs {
		email := a.Mailbox + "@" + a.Host
		if a.Name != "" {
			parts = append(parts, fmt.Sprintf("%s <%s>", a.Name, email))
		} else {
			parts = append(parts, email)
		}
	}
	return strings.Join(parts, ", ")
}

// envelopeAddrList formats an imap.Address slice as a list of email strings.
func envelopeAddrList(addrs []imap.Address) []string {
	var list []string
	for _, a := range addrs {
		list = append(list, a.Mailbox+"@"+a.Host)
	}
	return list
}

// addressListFromHeader extracts email addresses from an enmime envelope header.
func addressListFromHeader(env *enmime.Envelope, header string) []string {
	addrs, err := env.AddressList(header)
	if err != nil {
		return nil
	}
	var list []string
	for _, a := range addrs {
		if a.Name != "" {
			list = append(list, fmt.Sprintf("%s <%s>", a.Name, a.Address))
		} else {
			list = append(list, a.Address)
		}
	}
	return list
}

// flagsToStrings converts imap.Flags to string slice.
func flagsToStrings(flags []imap.Flag) []string {
	var s []string
	for _, f := range flags {
		s = append(s, string(f))
	}
	return s
}
