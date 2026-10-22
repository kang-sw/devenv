package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// lookupPeersResult is lookup_peers' JSON shape as a caller parses it.
type lookupPeersResult struct {
	Self           map[string]any            `json:"self"`
	Peers          []mailboxPeerOut          `json:"peers"`
	Correspondents []mailboxCorrespondentOut `json:"correspondents"`
}

func lookupPeersJSON(t *testing.T, s *Server, id int, key, scope string) lookupPeersResult {
	t.Helper()
	raw := callToolWithKey(t, s, id, key, "mailbox.lookup_peers", map[string]any{"scope": scope, "format": "json"})
	var out lookupPeersResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("lookup_peers JSON: %v: %s", err, raw)
	}
	if out.Correspondents == nil {
		t.Fatalf("lookup_peers JSON correspondents is null, want an array: %s", raw)
	}
	return out
}

// setMailboxClock pins mailboxNow to at for the rest of the test.
func setMailboxClock(t *testing.T, at time.Time) {
	t.Helper()
	original := mailboxNow
	t.Cleanup(func() { mailboxNow = original })
	mailboxNow = func() time.Time { return at }
}

func replyEntryFor(t *testing.T, key string) (wsmailbox.ReplyEntry, bool) {
	t.Helper()
	path, err := wsmailbox.ReplyRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	store, err := wsmailbox.LoadReplyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := store.Entries[wsmailbox.ReplyID(key)]
	return entry, ok
}

// TestMailboxLookupPeersRecencyOrderCapAndAutoExclusion pins the named-peer
// listing: records are listed whether or not their process is live, newest
// last_seen first, at most mailboxPeerListCap per scope with the caller's
// own inbox excluded before the cap; a non-live auto-identity record is
// omitted (its address is re-minted on restart) while a live one is listed;
// and a non-live record never carries the CONFLICT marker.
func TestMailboxLookupPeersRecencyOrderCapAndAutoExclusion(t *testing.T) {
	setupMailboxTestEnv(t)
	t.Setenv(envMailboxAuto, "")
	root := t.TempDir()
	initGit(t, root)
	base := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	setMailboxClock(t, base)

	t.Setenv(envMailbox, "selfy@worktree")
	looker := NewServer(root, "test")
	keyLooker := mailboxLogin(t, looker, 1, root)

	path, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatal(err)
	}
	deadPID := exitedChildPID(t)
	if err := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		store.Presence["auto-live"] = wsmailbox.Presence{Name: "auto-live", Scope: wsmailbox.ScopeWorktree, PID: os.Getppid(), Auto: true, LastSeen: base.Add(-30 * time.Second).Format(time.RFC3339)}
		store.Presence["auto-dead"] = wsmailbox.Presence{Name: "auto-dead", Scope: wsmailbox.ScopeWorktree, PID: deadPID, Auto: true, LastSeen: base.Add(-time.Minute).Format(time.RFC3339)}
		for i := 0; i < 22; i++ {
			name := fmt.Sprintf("p%02d", i)
			store.Presence[name] = wsmailbox.Presence{
				Name: name, Scope: wsmailbox.ScopeWorktree, PID: deadPID,
				LastSeen: base.Add(-time.Hour + time.Duration(i)*time.Minute).Format(time.RFC3339),
				// A stale conflict flag on a dead record must not surface.
				Conflict: i == 21,
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	got := lookupPeersJSON(t, looker, 2, keyLooker, "worktree")
	if got.Self["address"] != "selfy@worktree" {
		t.Fatalf("self changed shape or value: %#v", got.Self)
	}
	want := []string{"auto-live@worktree"}
	for i := 21; i >= 3; i-- {
		want = append(want, fmt.Sprintf("p%02d@worktree", i))
	}
	if len(got.Peers) != len(want) {
		t.Fatalf("peers = %d entries, want %d (cap after self-exclusion): %#v", len(got.Peers), len(want), got.Peers)
	}
	for i, p := range got.Peers {
		if p.Address != want[i] {
			t.Fatalf("peers[%d] = %s, want %s (last_seen desc): %#v", i, p.Address, want[i], got.Peers)
		}
		if p.LastSeen == "" {
			t.Fatalf("peers[%d] carries no last_seen: %#v", i, p)
		}
		if p.Conflict {
			t.Fatalf("non-live record %s carries a conflict marker", p.Address)
		}
	}
	if got.Peers[1].LastSeen != base.Add(-39*time.Minute).Format(time.RFC3339) {
		t.Fatalf("peer last_seen not the stored record's: %#v", got.Peers[1])
	}

	text := callToolWithKey(t, looker, 3, keyLooker, "mailbox.lookup_peers", map[string]any{"scope": "worktree"})
	if strings.Contains(text, "auto-dead@worktree") || strings.Contains(text, "CONFLICT") || strings.Contains(text, "selfy@worktree harness") {
		t.Fatalf("text listing leaked an excluded record or a dead conflict: %s", text)
	}
	if !strings.Contains(text, "p21@worktree") || !strings.Contains(text, " last_seen="+base.Add(-39*time.Minute).Format(time.RFC3339)) {
		t.Fatalf("text peer line missing last_seen: %s", text)
	}
}

// TestMailboxCorrespondentsFromSendAndRecv pins the caller-scoped
// correspondence history: send and recv each record the counterparty
// (named and reply-id forms) with the caller's own exchange time and the
// counterparty's harness/cwd; the list is newest first, returned for every
// scope, invisible to an unrelated session, and survives a server restart
// under the same session key.
func TestMailboxCorrespondentsFromSendAndRecv(t *testing.T) {
	setupMailboxTestEnv(t)
	t.Setenv(envMailboxAuto, "")
	root := t.TempDir()
	initGit(t, root)
	t0 := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	setMailboxClock(t, t0)

	t.Setenv(envMailbox, "alice@worktree")
	serverA := NewServer(root, "test")
	serverA.sessionHarness = "codex"
	keyA := mailboxLogin(t, serverA, 1, root)

	t.Setenv(envMailbox, "")
	serverB := NewServer(root, "test")
	serverB.sessionHarness = "pi"
	keyB := mailboxLogin(t, serverB, 1, root)
	serverC := NewServer(root, "test")
	keyC := mailboxLogin(t, serverC, 1, root)

	if got := lookupPeersJSON(t, serverB, 2, keyB, "worktree"); len(got.Correspondents) != 0 {
		t.Fatalf("a session that exchanged nothing has correspondents: %#v", got.Correspondents)
	}
	if text := callToolWithKey(t, serverB, 3, keyB, "mailbox.lookup_peers", map[string]any{"scope": "worktree"}); !strings.Contains(text, "correspondents:\nno correspondents\n") {
		t.Fatalf("text form missing the empty correspondents block: %s", text)
	}

	// B -> alice@worktree (named counterparty, metadata from its presence).
	callToolWithKey(t, serverB, 4, keyB, "mailbox.send", map[string]any{"to": "alice@worktree", "content": "hello alice"})

	// A drains it at t1: the counterparty is B's reply-id, metadata from
	// B's own reply-id entry; last_exchange is A's recv time, not sent_at.
	t1 := t0.Add(5 * time.Minute)
	setMailboxClock(t, t1)
	if recv := callToolWithKey(t, serverA, 5, keyA, "mailbox.recv", nil); !strings.Contains(recv, "hello alice") {
		t.Fatalf("alice did not receive: %s", recv)
	}
	replyB := wsmailbox.ReplyIDPrefix + wsmailbox.ReplyID(keyB)
	gotA := lookupPeersJSON(t, serverA, 6, keyA, "worktree")
	if len(gotA.Correspondents) != 1 || gotA.Correspondents[0] != (mailboxCorrespondentOut{Address: replyB, LastExchange: t1.Format(time.RFC3339), Harness: "pi", Cwd: root}) {
		t.Fatalf("recv did not record the reply-id sender with its metadata: %#v", gotA.Correspondents)
	}

	// A replies to B's reply-id at t2; B drains at t3 (sender is A's
	// reply-id: a worktree slug does not reach a reply-id recipient).
	t2 := t1.Add(time.Minute)
	setMailboxClock(t, t2)
	callToolWithKey(t, serverA, 7, keyA, "mailbox.send", map[string]any{"to": replyB, "content": "hi back"})
	t3 := t2.Add(time.Minute)
	setMailboxClock(t, t3)
	if recv := callToolWithKey(t, serverB, 8, keyB, "mailbox.recv", nil); !strings.Contains(recv, "hi back") {
		t.Fatalf("B did not receive the reply: %s", recv)
	}

	replyA := wsmailbox.ReplyIDPrefix + wsmailbox.ReplyID(keyA)
	wantB := []mailboxCorrespondentOut{
		{Address: replyA, LastExchange: t3.Format(time.RFC3339), Harness: "codex", Cwd: root},
		{Address: "alice@worktree", LastExchange: t0.Format(time.RFC3339), Harness: "codex", Cwd: root},
	}
	for i, scope := range []string{"worktree", "machine", "clone"} {
		got := lookupPeersJSON(t, serverB, 10+i, keyB, scope)
		if len(got.Correspondents) != len(wantB) || got.Correspondents[0] != wantB[0] || got.Correspondents[1] != wantB[1] {
			t.Fatalf("scope %s: correspondents = %#v, want %#v", scope, got.Correspondents, wantB)
		}
	}
	if gotA := lookupPeersJSON(t, serverA, 13, keyA, "worktree"); len(gotA.Correspondents) != 1 || gotA.Correspondents[0].LastExchange != t2.Format(time.RFC3339) {
		t.Fatalf("A's send did not refresh its contact to the send time: %#v", gotA.Correspondents)
	}

	text := callToolWithKey(t, serverB, 14, keyB, "mailbox.lookup_peers", map[string]any{"scope": "worktree"})
	wantBlock := "correspondents:\n" +
		replyA + " last_exchange=" + t3.Format(time.RFC3339) + " harness=codex cwd=" + root + "\n" +
		"alice@worktree last_exchange=" + t0.Format(time.RFC3339) + " harness=codex cwd=" + root + "\n"
	if !strings.Contains(text, wantBlock) {
		t.Fatalf("text correspondents block = %s, want it to contain %s", text, wantBlock)
	}

	// An unrelated session sees none of it.
	gotC := lookupPeersJSON(t, serverC, 15, keyC, "worktree")
	if len(gotC.Correspondents) != 0 {
		t.Fatalf("an unrelated session sees another session's correspondents: %#v", gotC.Correspondents)
	}
	rawC := callToolWithKey(t, serverC, 16, keyC, "mailbox.lookup_peers", map[string]any{"scope": "machine"})
	if strings.Contains(rawC, replyA) || strings.Contains(rawC, replyB) {
		t.Fatalf("an unrelated session can discover other sessions' reply-ids: %s", rawC)
	}

	// Restart: a new server process, same session key.
	serverB2 := NewServer(root, "test")
	if got := lookupPeersJSON(t, serverB2, 17, keyB, "worktree"); len(got.Correspondents) != 2 || got.Correspondents[0].Address != replyA {
		t.Fatalf("correspondents did not survive a server restart: %#v", got.Correspondents)
	}
}

// TestMailboxCorrespondentsRecvRecordsNamedSender pins recv's counterparty
// preference: an envelope stamped with the sender's named identity (From)
// records that slug@scope address, with the harness/cwd of its named
// presence, rather than the sender's reply-id.
func TestMailboxCorrespondentsRecvRecordsNamedSender(t *testing.T) {
	setupMailboxTestEnv(t)
	t.Setenv(envMailboxAuto, "")
	root := t.TempDir()
	initGit(t, root)
	t0 := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	setMailboxClock(t, t0)

	t.Setenv(envMailbox, "alice@worktree")
	serverA := NewServer(root, "test")
	keyA := mailboxLogin(t, serverA, 1, root)

	t.Setenv(envMailbox, "bob@worktree")
	serverB := NewServer(root, "test")
	serverB.sessionHarness = "pi"
	keyB := mailboxLogin(t, serverB, 1, root)

	// bob@worktree -> alice@worktree: a named sender mailing a named
	// recipient stamps From with its slug.
	callToolWithKey(t, serverB, 2, keyB, "mailbox.send", map[string]any{"to": "alice@worktree", "content": "from bob"})

	t1 := t0.Add(5 * time.Minute)
	setMailboxClock(t, t1)
	if recv := callToolWithKey(t, serverA, 3, keyA, "mailbox.recv", nil); !strings.Contains(recv, "bob@worktree: from bob") {
		t.Fatalf("alice did not receive bob's named mail: %s", recv)
	}

	got := lookupPeersJSON(t, serverA, 4, keyA, "worktree")
	want := mailboxCorrespondentOut{Address: "bob@worktree", LastExchange: t1.Format(time.RFC3339), Harness: "pi", Cwd: root}
	if len(got.Correspondents) != 1 || got.Correspondents[0] != want {
		t.Fatalf("recv did not record the named sender from its presence: %#v, want [%#v]", got.Correspondents, want)
	}
}

// TestMailboxCorrespondentsSelfExclusionMergeAndRecvOnlyRetention pins the
// remaining history rules: mailing one's own reply-id or own named inbox
// records no contact; a later publish (lookup or send) keeps existing
// contacts; a recv that drains nothing creates no entry; and a recv-only
// named-inbox owner gets an entry whose contact writes count as activity for
// reply-id retention.
func TestMailboxCorrespondentsSelfExclusionMergeAndRecvOnlyRetention(t *testing.T) {
	setupMailboxTestEnv(t)
	t.Setenv(envMailboxAuto, "")
	root := t.TempDir()
	initGit(t, root)
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	setMailboxClock(t, t0)

	t.Setenv(envMailbox, "owner@worktree")
	serverA := NewServer(root, "test")
	keyA := mailboxLogin(t, serverA, 1, root)
	t.Setenv(envMailbox, "")
	serverB := NewServer(root, "test")
	keyB := mailboxLogin(t, serverB, 1, root)
	serverD := NewServer(root, "test")
	keyD := mailboxLogin(t, serverD, 1, root)

	// A recv that drains nothing records nothing and creates no entry.
	callToolWithKey(t, serverD, 2, keyD, "mailbox.recv", nil)
	if _, ok := replyEntryFor(t, keyD); ok {
		t.Fatalf("an empty recv created a reply-id entry")
	}

	// Recv-only owner: A never sends; B mails A's named inbox; A drains.
	callToolWithKey(t, serverB, 3, keyB, "mailbox.send", map[string]any{"to": "owner@worktree", "content": "one"})
	callToolWithKey(t, serverA, 4, keyA, "mailbox.recv", nil)
	entryA, ok := replyEntryFor(t, keyA)
	replyB := wsmailbox.ReplyIDPrefix + wsmailbox.ReplyID(keyB)
	if !ok || entryA.LastSeen != t0.Format(time.RFC3339) || len(entryA.Contacts) != 1 || entryA.Contacts[replyB].LastExchange == "" {
		t.Fatalf("recv-only owner's history not created: ok=%v %#v", ok, entryA)
	}

	// Self-exclusion: B mails its own reply-id; A mails its own named inbox
	// and drains it. Neither records a self contact.
	ownB := wsmailbox.ReplyIDPrefix + wsmailbox.ReplyID(keyB)
	callToolWithKey(t, serverB, 5, keyB, "mailbox.send", map[string]any{"to": ownB, "content": "note to self"})
	callToolWithKey(t, serverB, 6, keyB, "mailbox.recv", nil)
	entryB, _ := replyEntryFor(t, keyB)
	if _, self := entryB.Contacts[ownB]; self || len(entryB.Contacts) != 1 {
		t.Fatalf("self-send recorded a contact for B: %#v", entryB.Contacts)
	}
	callToolWithKey(t, serverA, 7, keyA, "mailbox.send", map[string]any{"to": "owner@worktree", "content": "self memo"})
	callToolWithKey(t, serverA, 8, keyA, "mailbox.recv", nil)
	entryA, _ = replyEntryFor(t, keyA)
	if _, self := entryA.Contacts["owner@worktree"]; self || len(entryA.Contacts) != 1 {
		t.Fatalf("own-named-inbox exchange recorded a contact for A: %#v", entryA.Contacts)
	}

	// Repeated publishes keep the history.
	lookupPeersJSON(t, serverB, 9, keyB, "worktree")
	lookupPeersJSON(t, serverB, 10, keyB, "machine")
	if entryB, _ = replyEntryFor(t, keyB); len(entryB.Contacts) != 1 || entryB.Contacts["owner@worktree"].LastExchange == "" || entryB.Cwd != root {
		t.Fatalf("repeated publishReplyID lost contacts or metadata: %#v", entryB)
	}

	// Retention: a later contact write refreshes A's LastSeen, so the
	// entry outlives 30 days from its creation.
	t20 := t0.Add(20 * 24 * time.Hour)
	setMailboxClock(t, t20)
	callToolWithKey(t, serverB, 11, keyB, "mailbox.send", map[string]any{"to": "owner@worktree", "content": "two"})
	callToolWithKey(t, serverA, 12, keyA, "mailbox.recv", nil)
	if entryA, _ = replyEntryFor(t, keyA); entryA.LastSeen != t20.Format(time.RFC3339) {
		t.Fatalf("contact write did not refresh last_seen: %#v", entryA)
	}
	setMailboxClock(t, t0.Add(35*24*time.Hour))
	lookupPeersJSON(t, serverD, 13, keyD, "worktree") // publish -> reap pass
	if entryA, ok = replyEntryFor(t, keyA); !ok || len(entryA.Contacts) != 1 {
		t.Fatalf("recv-only owner's history was reaped despite recent activity: ok=%v %#v", ok, entryA)
	}
}
