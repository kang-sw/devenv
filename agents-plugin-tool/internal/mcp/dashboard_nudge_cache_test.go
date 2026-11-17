package mcp

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

func TestDashboardSingleNudgeRefreshesCachedScopeContact(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	hf := newHistoryFixture(t, now)
	mustSend(t, hf.serverB, 50, hf.keyB, "alice@worktree", "hello")
	record, ok := hf.serverB.sessions.readState(hf.keyB, partAll)
	if !ok {
		t.Fatal("missing record")
	}
	dir := t.TempDir()
	reg := wsdashboard.Registration{ReplyID: wsmailbox.ReplyID(hf.keyB), Project: record.Root,
		RegisteredAt: now.Format(time.RFC3339Nano), HeartbeatAt: now.Format(time.RFC3339Nano)}
	if err := wsdashboard.WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	src := &DashboardSessionSource{store: hf.serverB.sessions, stateDir: func() (string, error) { return dir, nil }}
	idx := wsdashboard.NewIndex(wsdashboard.IndexOptions{Dir: dir, Source: src})
	idx.Refresh()
	contacts, _ := src.ReadContacts(reg)
	if len(contacts) != 1 || contacts[0].ReplyID != wsmailbox.ReplyID(hf.keyA) {
		t.Fatalf("warm contacts: %+v", contacts)
	}
	before, ok := idx.Session(reg.ReplyID)
	if !ok {
		t.Fatal("missing warm index row")
	}
	path, err := wsmailbox.PathForScope(wsmailbox.ScopeWorktree, reg.Project)
	if err != nil {
		t.Fatal(err)
	}
	if err := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		p := store.Presence["alice"]
		p.Owner, p.Cwd = "rebound-owner", "rebound-cwd"
		store.Presence["alice"] = p
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !idx.Nudge(reg.ReplyID, reg.Project) {
		t.Fatal("normal nudge refused")
	}
	idx.Settle()
	after, _ := idx.Session(reg.ReplyID)
	contacts, _ = src.ReadContacts(reg)
	if after.Rev != before.Rev+1 || len(contacts) != 1 || contacts[0].ReplyID != wsmailbox.ReplyID("rebound-owner") || contacts[0].Cwd != "rebound-cwd" {
		t.Fatalf("single scope nudge: revision %d -> %d, contacts %+v", before.Rev, after.Rev, contacts)
	}
	_, misses := src.scopes.Counts()
	for range 16 {
		src.ReadContacts(reg)
	}
	if _, after := src.scopes.Counts(); after != misses {
		t.Fatalf("presentation contacts reopened scope %d times", after-misses)
	}
}

func TestDashboardSingleNudgeRefreshesCachedSnapshotAndRecordPart(t *testing.T) {
	f := newWhiteboardFixture(t)
	dir := t.TempDir()
	now := time.Now().UTC()
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root, Harness: "pi",
		RegisteredAt: now.Format(time.RFC3339Nano), HeartbeatAt: now.Format(time.RFC3339Nano)}
	if err := wsdashboard.WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	if err := f.s.sessions.mutateTodos(f.key, func([]todoItem) ([]todoItem, error) {
		return []todoItem{{Key: "cached", Title: "before"}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	write := func(text string) {
		t.Helper()
		raw, err := json.Marshal(wslive.Snapshot{
			SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000,
			Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, Reply: &wslive.Reply{Text: text}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(wslive.SnapshotPath(wsdashboard.SessionDir(dir, reg.ReplyID)), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("before")
	src := &DashboardSessionSource{store: f.s.sessions, stateDir: func() (string, error) { return dir, nil }}
	idx := wsdashboard.NewIndex(wsdashboard.IndexOptions{Dir: dir, Source: src})
	idx.Refresh() // actual source, record FileCache and second SnapshotCache owner
	before, ok := idx.Session(reg.ReplyID)
	if !ok {
		t.Fatal("missing warm index row")
	}
	// Warm presentation reads immediately before each external change. No
	// manual invalidation, second nudge, clock advancement or Refresh follows.
	for _, change := range []string{"snapshot", "record part"} {
		t.Run(change, func(t *testing.T) {
			if snap := src.snapshots.Snapshot(dir, reg.ReplyID); snap == nil {
				t.Fatal("missing warm snapshot")
			}
			if _, _, record, ok, err := src.lookupRecord(reg); err != nil || !ok || len(record.Todos) != 1 || record.Todos[0].Title != "before" {
				t.Fatalf("warm record: %+v, %v", record, err)
			}
			switch change {
			case "snapshot":
				write("snapshot changed inside TTL")
			case "record part":
				if err := f.s.sessions.mutateTodos(f.key, func(items []todoItem) ([]todoItem, error) {
					items[0].Title = "record part changed inside TTL"
					return items, nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			if !idx.Nudge(reg.ReplyID, reg.Project) {
				t.Fatal("normal nudge refused")
			}
			idx.Settle()
			after, ok := idx.Session(reg.ReplyID)
			if !ok || after.Rev != before.Rev+1 {
				t.Fatalf("single %s nudge kept revision: %d -> %d", change, before.Rev, after.Rev)
			}
			before = after
		})
	}
	if snap := src.snapshots.Snapshot(dir, reg.ReplyID); snap == nil || snap.Lead.Reply == nil || snap.Lead.Reply.Text != "snapshot changed inside TTL" {
		t.Fatalf("source snapshot remained stale: %+v", snap)
	}
	_, _, record, ok, err := src.lookupRecord(reg)
	if !ok || err != nil || len(record.Todos) != 1 || record.Todos[0].Title != "record part changed inside TTL" {
		t.Fatalf("source record part remained stale: %+v, %v", record, err)
	}
	// Invalidation is confined to ReadSession, never repeated per page read.
	// Presentation lookups still share the just-validated parsed parts.
	_, misses := src.records.Counts()
	for range 16 {
		if _, _, _, ok, err := src.lookupRecord(reg); !ok || err != nil {
			t.Fatalf("shared lookup: %v", err)
		}
	}
	if _, after := src.records.Counts(); after != misses {
		t.Fatalf("presentation lookups reopened record parts %d times", after-misses)
	}
}
