package main

import (
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
)

func TestMoveMessage(t *testing.T) {
	env := newTestEnv(t)
	env.createFolder(t, "Archive")
	uid := env.seed(t, "INBOX", "to be moved")
	env.requireCount(t, "INBOX", 1)

	if err := env.svc.MoveMessage(uid, "INBOX", "Archive"); err != nil {
		t.Fatalf("MoveMessage: %v", err)
	}

	env.requireCount(t, "INBOX", 0)
	env.requireCount(t, "Archive", 1)
	sums := env.listSummaries(t, "Archive")
	if len(sums) != 1 || sums[0].Subject != "to be moved" {
		t.Fatalf("moved message not intact in Archive: %+v", sums)
	}
}

// A move to a folder that doesn't exist must fail loudly and leave the
// message where it was — not silently drop it.
func TestMoveMessageToMissingDestinationFails(t *testing.T) {
	env := newTestEnv(t)
	uid := env.seed(t, "INBOX", "stays put")

	if err := env.svc.MoveMessage(uid, "INBOX", "NoSuchFolder"); err == nil {
		t.Fatal("expected error moving to a nonexistent folder")
	}
	env.requireCount(t, "INBOX", 1)
}

func TestDeleteMessageMovesToTrash(t *testing.T) {
	env := newTestEnv(t)
	uid := env.seed(t, "INBOX", "delete me softly")

	permanent, err := env.svc.DeleteMessage(uid, "INBOX")
	if err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if permanent {
		t.Fatal("deleting from INBOX should move to Trash, not permanently delete")
	}

	env.requireCount(t, "INBOX", 0)
	env.requireCount(t, "Trash", 1)
	sums := env.listSummaries(t, "Trash")
	if len(sums) != 1 || sums[0].Subject != "delete me softly" {
		t.Fatalf("deleted message not intact in Trash: %+v", sums)
	}
}

func TestDeleteMessageFromTrashIsPermanent(t *testing.T) {
	env := newTestEnv(t)
	uid := env.seed(t, "Trash", "gone forever")

	permanent, err := env.svc.DeleteMessage(uid, "Trash")
	if err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if !permanent {
		t.Fatal("deleting from Trash should be permanent")
	}

	env.requireCount(t, "Trash", 0)
	if sums := env.listSummaries(t, "Trash"); len(sums) != 0 {
		t.Fatalf("permanently deleted message still listed: %+v", sums)
	}
}

// The Trash check is case-insensitive: a folder named "trash" takes the
// permanent-deletion branch rather than moving to "Trash". Since the server
// folder is named "Trash" (IMAP names are case-sensitive), the operation
// fails loudly and leaves the message untouched — never a silent misdirected
// delete.
func TestDeleteMessageTrashFolderCaseInsensitive(t *testing.T) {
	env := newTestEnv(t)
	uid := env.seed(t, "INBOX", "case check")

	permanent, err := env.svc.DeleteMessage(uid, "trash")
	if err == nil {
		t.Fatal(`expected error deleting from nonexistent folder "trash"`)
	}
	if !permanent {
		t.Fatal(`expected the permanent branch to be selected for "trash"`)
	}
	env.requireCount(t, "INBOX", 1)
	sums := env.listSummaries(t, "INBOX")
	if len(sums) != 1 || sums[0].UID != uid {
		t.Fatalf("message affected by failed delete: %+v", sums)
	}
}

// A permanent delete of a UID that doesn't exist is a safe no-op: STORE and
// EXPUNGE match nothing, so no other message may disappear.
func TestDeleteMessageMissingUIDNoOpInPermanentBranch(t *testing.T) {
	env := newTestEnv(t)
	uid := env.seed(t, "Trash", "innocent bystander")

	permanent, err := env.svc.DeleteMessage(424242, "Trash")
	if err != nil {
		t.Fatalf("DeleteMessage with missing UID: %v", err)
	}
	if !permanent {
		t.Fatal("expected permanent=true for the Trash branch")
	}
	env.requireCount(t, "Trash", 1)
	sums := env.listSummaries(t, "Trash")
	if len(sums) != 1 || sums[0].UID != uid {
		t.Fatalf("bystander message affected by deleting a missing UID: %+v", sums)
	}
}

func TestMarkReadAndUnread(t *testing.T) {
	env := newTestEnv(t)
	uid := env.seed(t, "INBOX", "flag me")

	sums := env.listSummaries(t, "INBOX")
	if hasFlag(findSummary(t, sums, "flag me").Flags, "\\Seen") {
		t.Fatal("seeded message should start unread")
	}

	if err := env.svc.MarkRead(uid, "INBOX"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	sums = env.listSummaries(t, "INBOX")
	if !hasFlag(findSummary(t, sums, "flag me").Flags, "\\Seen") {
		t.Fatal("\\Seen flag not set by MarkRead")
	}
	unread, err := env.svc.ListMessages("INBOX", 200, true)
	if err != nil {
		t.Fatalf("ListMessages(unread): %v", err)
	}
	if len(unread) != 0 {
		t.Fatalf("read message still listed as unread: %+v", unread)
	}

	if err := env.svc.MarkUnread(uid, "INBOX"); err != nil {
		t.Fatalf("MarkUnread: %v", err)
	}
	sums = env.listSummaries(t, "INBOX")
	if hasFlag(findSummary(t, sums, "flag me").Flags, "\\Seen") {
		t.Fatal("\\Seen flag not removed by MarkUnread")
	}
	unread, err = env.svc.ListMessages("INBOX", 200, true)
	if err != nil {
		t.Fatalf("ListMessages(unread): %v", err)
	}
	if len(unread) != 1 {
		t.Fatalf("unread listing after MarkUnread: %+v", unread)
	}
}

func TestCreateFolder(t *testing.T) {
	env := newTestEnv(t)

	if err := env.svc.CreateFolder("Projects"); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	folders := env.listFolders(t)
	fi := folderByName(t, folders, "Projects")
	if !fi.Selectable {
		t.Fatal("created folder should be selectable")
	}
	env.requireCount(t, "Projects", 0)

	if err := env.svc.CreateFolder("Projects"); err == nil {
		t.Fatal("expected duplicate CreateFolder to fail")
	}
}

func TestRenameFolder(t *testing.T) {
	env := newTestEnv(t)
	env.createFolder(t, "Old")
	uid := env.seed(t, "Old", "keep me")

	if err := env.svc.RenameFolder("Old", "New"); err != nil {
		t.Fatalf("RenameFolder: %v", err)
	}
	if folders := env.listFolders(t); folderExists(folders, "Old") {
		t.Fatal("old folder name still listed after rename")
	}
	env.requireCount(t, "New", 1)
	sums := env.listSummaries(t, "New")
	if len(sums) != 1 || sums[0].UID != uid || sums[0].Subject != "keep me" {
		t.Fatalf("messages not preserved across rename: %+v", sums)
	}

	if err := env.svc.RenameFolder("Ghost", "Anywhere"); err == nil {
		t.Fatal("expected renaming a nonexistent folder to fail")
	}
}

func TestDeleteFolder(t *testing.T) {
	env := newTestEnv(t)
	env.createFolder(t, "Doomed")
	env.seed(t, "Doomed", "inside doomed")

	if err := env.svc.DeleteFolder("Doomed"); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	if folders := env.listFolders(t); folderExists(folders, "Doomed") {
		t.Fatal("deleted folder still listed")
	}

	if err := env.svc.DeleteFolder("Doomed"); err == nil {
		t.Fatal("expected deleting a nonexistent folder to fail")
	}
}

// Without the MOVE capability the client falls back to COPY + STORE \Deleted
// + EXPUNGE. That path must produce the same end state.
func TestMoveAndDeleteFallbackWithoutMoveCapability(t *testing.T) {
	env := newTestEnvWithCaps(t, imap.CapUIDPlus)
	env.createFolder(t, "Archive")

	uid := env.seed(t, "INBOX", "fallback move")
	if err := env.svc.MoveMessage(uid, "INBOX", "Archive"); err != nil {
		t.Fatalf("MoveMessage (fallback): %v", err)
	}
	env.requireCount(t, "INBOX", 0)
	env.requireCount(t, "Archive", 1)

	uid2 := env.seed(t, "INBOX", "fallback delete")
	permanent, err := env.svc.DeleteMessage(uid2, "INBOX")
	if err != nil {
		t.Fatalf("DeleteMessage (fallback): %v", err)
	}
	if permanent {
		t.Fatal("deleting from INBOX should move to Trash, not permanently delete")
	}
	env.requireCount(t, "INBOX", 0)
	env.requireCount(t, "Trash", 1)
}

// Smoke test for the read path — also grounds the listing helpers used to
// verify the destructive operations above.
func TestReadOperations(t *testing.T) {
	env := newTestEnv(t)
	uidA := env.seed(t, "INBOX", "alpha")
	env.seed(t, "INBOX", "beta")

	folders := env.listFolders(t)
	inbox := folderByName(t, folders, "INBOX")
	if inbox.TotalCount != 2 || inbox.UnreadCount != 2 {
		t.Fatalf("unexpected INBOX counts: total=%d unread=%d", inbox.TotalCount, inbox.UnreadCount)
	}
	if inbox.Delimiter != "/" || !inbox.Selectable {
		t.Fatalf("unexpected INBOX metadata: %+v", inbox)
	}

	sums := env.listSummaries(t, "INBOX")
	if len(sums) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(sums))
	}
	sumA := findSummary(t, sums, "alpha")
	if sumA.UID != uidA || sumA.From != "sender@example.com" {
		t.Fatalf("unexpected summary for alpha: %+v", sumA)
	}

	hits, err := env.svc.SearchMessages("alpha", "INBOX", 20)
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(hits) != 1 || hits[0].Subject != "alpha" {
		t.Fatalf("unexpected search results: %+v", hits)
	}

	msg, err := env.svc.GetMessage(uidA, "INBOX")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if msg.Subject != "alpha" || msg.BodyText != "Body of alpha\r\n" {
		t.Fatalf("unexpected message content: subject=%q body=%q", msg.Subject, msg.BodyText)
	}
	if msg.From != "sender@example.com" || len(msg.To) != 1 || msg.To[0] != testUsername {
		t.Fatalf("unexpected message addresses: %+v", msg)
	}
	if msg.MessageID == "" {
		t.Fatal("expected Message-ID to be populated")
	}

	if _, err := env.svc.GetMessage(424242, "INBOX"); err == nil {
		t.Fatal("expected GetMessage for a missing UID to fail")
	}
}

func TestDialWithBadCredentials(t *testing.T) {
	env := newTestEnv(t)
	cfg := *env.cfg
	cfg.Password = "wrong-password"
	svc := NewMailService(&cfg, nil)

	if _, err := svc.ListFolders(); err == nil {
		t.Fatal("expected login with wrong password to fail")
	} else if !strings.Contains(err.Error(), "IMAP login") {
		t.Fatalf("expected login error, got: %v", err)
	}
}
