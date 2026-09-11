package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- list_folders ---

type ListFoldersInput struct{}

type ListFoldersOutput struct {
	Folders []FolderInfo `json:"folders"`
}

func (s *MailService) handleListFolders(ctx context.Context, req *mcp.CallToolRequest, _ ListFoldersInput) (*mcp.CallToolResult, ListFoldersOutput, error) {
	folders, err := s.ListFolders()
	if err != nil {
		return nil, ListFoldersOutput{}, err
	}
	if folders == nil {
		folders = []FolderInfo{}
	}
	return nil, ListFoldersOutput{Folders: folders}, nil
}

// --- list_messages ---

type ListMessagesInput struct {
	Folder     string `json:"folder" jsonschema:"the mailbox folder to list messages from (default: INBOX)"`
	Limit      int    `json:"limit" jsonschema:"maximum number of messages to return (default: 50, max: 200)"`
	UnreadOnly bool   `json:"unread_only" jsonschema:"if true, return only unread messages"`
}

type ListMessagesOutput struct {
	Messages []MessageSummary `json:"messages"`
}

func (s *MailService) handleListMessages(ctx context.Context, req *mcp.CallToolRequest, in ListMessagesInput) (*mcp.CallToolResult, ListMessagesOutput, error) {
	messages, err := s.ListMessages(in.Folder, in.Limit, in.UnreadOnly)
	if err != nil {
		return nil, ListMessagesOutput{}, err
	}
	if messages == nil {
		messages = []MessageSummary{}
	}
	return nil, ListMessagesOutput{Messages: messages}, nil
}

// --- search_messages ---

type SearchMessagesInput struct {
	Query  string `json:"query" jsonschema:"the search query text"`
	Folder string `json:"folder" jsonschema:"the mailbox folder to search in (default: INBOX)"`
	Limit  int    `json:"limit" jsonschema:"maximum number of results to return (default: 20, max: 100)"`
}

type SearchMessagesOutput struct {
	Messages []MessageSummary `json:"messages"`
}

func (s *MailService) handleSearchMessages(ctx context.Context, req *mcp.CallToolRequest, in SearchMessagesInput) (*mcp.CallToolResult, SearchMessagesOutput, error) {
	if in.Query == "" {
		return nil, SearchMessagesOutput{}, fmt.Errorf("query is required")
	}
	messages, err := s.SearchMessages(in.Query, in.Folder, in.Limit)
	if err != nil {
		return nil, SearchMessagesOutput{}, err
	}
	if messages == nil {
		messages = []MessageSummary{}
	}
	return nil, SearchMessagesOutput{Messages: messages}, nil
}

// --- get_message ---

type GetMessageInput struct {
	UID    uint32 `json:"uid" jsonschema:"the UID of the message to retrieve"`
	Folder string `json:"folder" jsonschema:"the mailbox folder containing the message (default: INBOX)"`
}

type GetMessageOutput struct {
	Message *FullMessage `json:"message"`
}

func (s *MailService) handleGetMessage(ctx context.Context, req *mcp.CallToolRequest, in GetMessageInput) (*mcp.CallToolResult, GetMessageOutput, error) {
	if in.UID == 0 {
		return nil, GetMessageOutput{}, fmt.Errorf("uid is required")
	}
	msg, err := s.GetMessage(in.UID, in.Folder)
	if err != nil {
		return nil, GetMessageOutput{}, err
	}
	return nil, GetMessageOutput{Message: msg}, nil
}

// --- send_message ---

type SendMessageInput struct {
	To      []string `json:"to" jsonschema:"list of recipient email addresses"`
	Subject string   `json:"subject" jsonschema:"the email subject line"`
	Body    string   `json:"body" jsonschema:"the email body text (plain text)"`
	CC      []string `json:"cc,omitempty" jsonschema:"list of CC email addresses (optional)"`
	BCC     []string `json:"bcc,omitempty" jsonschema:"list of BCC email addresses (optional)"`
}

type SendMessageOutput struct {
	Sent       bool     `json:"sent"`
	Recipients []string `json:"recipients"`
}

func (s *MailService) handleSendMessage(ctx context.Context, req *mcp.CallToolRequest, in SendMessageInput) (*mcp.CallToolResult, SendMessageOutput, error) {
	if len(in.To) == 0 {
		return nil, SendMessageOutput{}, fmt.Errorf("at least one 'to' recipient is required")
	}
	if in.Subject == "" {
		return nil, SendMessageOutput{}, fmt.Errorf("subject is required")
	}
	if in.Body == "" {
		return nil, SendMessageOutput{}, fmt.Errorf("body is required")
	}
	result, err := s.SendMessage(in.To, in.Subject, in.Body, in.CC, in.BCC)
	if err != nil {
		return nil, SendMessageOutput{}, err
	}
	return nil, SendMessageOutput{
		Sent:       result.Sent,
		Recipients: result.Recipients,
	}, nil
}

// --- move_message ---

type MoveMessageInput struct {
	UID        uint32 `json:"uid" jsonschema:"the UID of the message to move"`
	Folder     string `json:"folder" jsonschema:"the source folder containing the message"`
	DestFolder string `json:"dest_folder" jsonschema:"the destination folder to move the message to"`
}

type MoveMessageOutput struct {
	Moved bool   `json:"moved"`
	UID   uint32 `json:"uid"`
	From  string `json:"from"`
	To    string `json:"to"`
}

func (s *MailService) handleMoveMessage(ctx context.Context, req *mcp.CallToolRequest, in MoveMessageInput) (*mcp.CallToolResult, MoveMessageOutput, error) {
	if in.UID == 0 {
		return nil, MoveMessageOutput{}, fmt.Errorf("uid is required")
	}
	if in.Folder == "" {
		return nil, MoveMessageOutput{}, fmt.Errorf("folder is required")
	}
	if in.DestFolder == "" {
		return nil, MoveMessageOutput{}, fmt.Errorf("dest_folder is required")
	}
	if err := s.MoveMessage(in.UID, in.Folder, in.DestFolder); err != nil {
		return nil, MoveMessageOutput{}, err
	}
	return nil, MoveMessageOutput{
		Moved: true,
		UID:   in.UID,
		From:  in.Folder,
		To:    in.DestFolder,
	}, nil
}

// --- mark_read ---

type MarkReadInput struct {
	UID    uint32 `json:"uid" jsonschema:"the UID of the message to mark as read"`
	Folder string `json:"folder" jsonschema:"the mailbox folder containing the message"`
}

type MarkReadOutput struct {
	Success bool   `json:"success"`
	UID     uint32 `json:"uid"`
	Folder  string `json:"folder"`
}

func (s *MailService) handleMarkRead(ctx context.Context, req *mcp.CallToolRequest, in MarkReadInput) (*mcp.CallToolResult, MarkReadOutput, error) {
	if in.UID == 0 {
		return nil, MarkReadOutput{}, fmt.Errorf("uid is required")
	}
	if in.Folder == "" {
		return nil, MarkReadOutput{}, fmt.Errorf("folder is required")
	}
	if err := s.MarkRead(in.UID, in.Folder); err != nil {
		return nil, MarkReadOutput{}, err
	}
	return nil, MarkReadOutput{Success: true, UID: in.UID, Folder: in.Folder}, nil
}

// --- mark_unread ---

type MarkUnreadInput struct {
	UID    uint32 `json:"uid" jsonschema:"the UID of the message to mark as unread"`
	Folder string `json:"folder" jsonschema:"the mailbox folder containing the message"`
}

type MarkUnreadOutput struct {
	Success bool   `json:"success"`
	UID     uint32 `json:"uid"`
	Folder  string `json:"folder"`
}

func (s *MailService) handleMarkUnread(ctx context.Context, req *mcp.CallToolRequest, in MarkUnreadInput) (*mcp.CallToolResult, MarkUnreadOutput, error) {
	if in.UID == 0 {
		return nil, MarkUnreadOutput{}, fmt.Errorf("uid is required")
	}
	if in.Folder == "" {
		return nil, MarkUnreadOutput{}, fmt.Errorf("folder is required")
	}
	if err := s.MarkUnread(in.UID, in.Folder); err != nil {
		return nil, MarkUnreadOutput{}, err
	}
	return nil, MarkUnreadOutput{Success: true, UID: in.UID, Folder: in.Folder}, nil
}

// --- delete_message ---

type DeleteMessageInput struct {
	UID    uint32 `json:"uid" jsonschema:"the UID of the message to delete"`
	Folder string `json:"folder" jsonschema:"the mailbox folder containing the message"`
}

type DeleteMessageOutput struct {
	Deleted   bool   `json:"deleted"`
	Permanent bool   `json:"permanent"`
	UID       uint32 `json:"uid"`
	Folder    string `json:"folder"`
}

func (s *MailService) handleDeleteMessage(ctx context.Context, req *mcp.CallToolRequest, in DeleteMessageInput) (*mcp.CallToolResult, DeleteMessageOutput, error) {
	if in.UID == 0 {
		return nil, DeleteMessageOutput{}, fmt.Errorf("uid is required")
	}
	if in.Folder == "" {
		return nil, DeleteMessageOutput{}, fmt.Errorf("folder is required")
	}
	permanent, err := s.DeleteMessage(in.UID, in.Folder)
	if err != nil {
		return nil, DeleteMessageOutput{}, err
	}
	return nil, DeleteMessageOutput{
		Deleted:   true,
		Permanent: permanent,
		UID:       in.UID,
		Folder:    in.Folder,
	}, nil
}

// --- create_folder ---

type CreateFolderInput struct {
	Name string `json:"name" jsonschema:"the name of the folder to create"`
}

type CreateFolderOutput struct {
	Created bool   `json:"created"`
	Name    string `json:"name"`
}

func (s *MailService) handleCreateFolder(ctx context.Context, req *mcp.CallToolRequest, in CreateFolderInput) (*mcp.CallToolResult, CreateFolderOutput, error) {
	if in.Name == "" {
		return nil, CreateFolderOutput{}, fmt.Errorf("name is required")
	}
	if err := s.CreateFolder(in.Name); err != nil {
		return nil, CreateFolderOutput{}, err
	}
	return nil, CreateFolderOutput{Created: true, Name: in.Name}, nil
}

// --- rename_folder ---

type RenameFolderInput struct {
	OldName string `json:"old_name" jsonschema:"the current name of the folder"`
	NewName string `json:"new_name" jsonschema:"the new name for the folder"`
}

type RenameFolderOutput struct {
	Renamed bool   `json:"renamed"`
	OldName string `json:"old_name"`
	NewName string `json:"new_name"`
}

func (s *MailService) handleRenameFolder(ctx context.Context, req *mcp.CallToolRequest, in RenameFolderInput) (*mcp.CallToolResult, RenameFolderOutput, error) {
	if in.OldName == "" {
		return nil, RenameFolderOutput{}, fmt.Errorf("old_name is required")
	}
	if in.NewName == "" {
		return nil, RenameFolderOutput{}, fmt.Errorf("new_name is required")
	}
	if err := s.RenameFolder(in.OldName, in.NewName); err != nil {
		return nil, RenameFolderOutput{}, err
	}
	return nil, RenameFolderOutput{Renamed: true, OldName: in.OldName, NewName: in.NewName}, nil
}

// --- delete_folder ---

type DeleteFolderInput struct {
	Name string `json:"name" jsonschema:"the name of the folder to delete"`
}

type DeleteFolderOutput struct {
	Deleted bool   `json:"deleted"`
	Name    string `json:"name"`
}

func (s *MailService) handleDeleteFolder(ctx context.Context, req *mcp.CallToolRequest, in DeleteFolderInput) (*mcp.CallToolResult, DeleteFolderOutput, error) {
	if in.Name == "" {
		return nil, DeleteFolderOutput{}, fmt.Errorf("name is required")
	}
	if err := s.DeleteFolder(in.Name); err != nil {
		return nil, DeleteFolderOutput{}, err
	}
	return nil, DeleteFolderOutput{Deleted: true, Name: in.Name}, nil
}

// registerTools registers all 12 MCP tools on the server.
func registerTools(server *mcp.Server, svc *MailService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_folders",
		Description: "List all mailbox folders recursively with total and unread message counts. Folders are hierarchical (e.g. 'Folders/ben@b7r.dev', 'Labels/personal'). Container folders (selectable=false) cannot hold messages directly but may have subfolders.",
	}, svc.handleListFolders)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_messages",
		Description: "List recent messages in a folder (envelopes only, no body). Returns subject, from, to, date, flags, and size for each message.",
	}, svc.handleListMessages)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_messages",
		Description: "Search for messages by keyword across headers and body. Returns matching messages with envelope info.",
	}, svc.handleSearchMessages)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_message",
		Description: "Get a full message including body text, HTML, headers, and attachment list by UID. Body text is truncated at 50KB.",
	}, svc.handleGetMessage)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "send_message",
		Description: "Compose and send an email via SMTP. Requires at least one 'to' recipient, a subject, and a body.",
	}, svc.handleSendMessage)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "move_message",
		Description: "Move a message from one folder to another by UID",
	}, svc.handleMoveMessage)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mark_read",
		Description: "Mark a message as read by setting the \\Seen flag",
	}, svc.handleMarkRead)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mark_unread",
		Description: "Mark a message as unread by removing the \\Seen flag",
	}, svc.handleMarkUnread)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_message",
		Description: "Delete a message. Moves to Trash if not already there. If already in Trash, permanently deletes it.",
	}, svc.handleDeleteMessage)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_folder",
		Description: "Create a new mailbox folder",
	}, svc.handleCreateFolder)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rename_folder",
		Description: "Rename a mailbox folder",
	}, svc.handleRenameFolder)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_folder",
		Description: "Delete a mailbox folder (must be empty on some servers)",
	}, svc.handleDeleteFolder)
}
