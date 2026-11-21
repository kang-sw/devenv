package mcp

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func TestDashboardDrainingBoardRetainsIndexAndCaches(t *testing.T) {
	t.Setenv("WS_MCP_NO_DASHBOARD", "1")
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="Plan"><wb:body>intact committed content</wb:body></wb:topic>`)
	src := dashboardPageSource(t, nil, f.s.sessions.boards)
	now := time.Now().UTC()
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root, HeartbeatAt: now.Format(time.RFC3339)}
	dir := t.TempDir()
	if err := wsdashboard.WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	page, ok, err := src.ReadPage(reg)
	if err != nil || !ok || !page.HasBoard {
		t.Fatalf("initial page: %v %v", ok, err)
	}
	// Keep the registration live while testing owner recovery.
	idx := wsdashboard.NewIndex(wsdashboard.IndexOptions{Dir: dir, Source: src, Now: func() time.Time { return now }})
	idx.Refresh()
	good, ok := idx.Session(reg.ReplyID)
	if !ok || !good.Live || good.Lifecycle != wsdashboard.SessionActiveOn {
		t.Fatalf("initial live session missing: %+v %v", good, ok)
	}
	before := src.prints[reg.ReplyID]
	boards := len(src.boards.entries)
	// A draining owner is a transient read failure. It must not evict
	// the last valid index entry or cached render.
	if err := f.s.sessions.boards.Close(); err != nil {
		t.Fatal(err)
	}
	idx.Refresh()
	if got, ok := idx.Session(reg.ReplyID); !ok || got != good {
		t.Fatalf("draining owner removed/changed index: %+v %v", got, ok)
	}
	if _, ok, err := src.ReadPage(reg); ok || !errors.Is(err, errWhiteboardDraining) {
		t.Fatalf("draining owner not transient: found=%v err=%v", ok, err)
	}
	if src.prints[reg.ReplyID] != before || len(src.boards.entries) != boards {
		t.Fatal("transient cleared valid source caches")
	}
	f.s = newTestBoardOwner(t, f.root, "test")
	src.store.boards = f.s.sessions.boards
	idx.Refresh()
	got, ok := idx.Session(reg.ReplyID)
	if !ok || !got.Live {
		t.Fatalf("recovery lost session: %+v %v", got, ok)
	}
	if recovered, ok, err := src.ReadPage(reg); !ok || err != nil || recovered.Seq != page.Seq || recovered.Board.Board != page.Board.Board {
		t.Fatalf("recovery lost page content: found=%v err=%v", ok, err)
	}
	// A readable identity mismatch removes the session.
	d, err := src.store.dirs()
	if err != nil {
		t.Fatal(err)
	}
	path := d.partPath(f.key, partKey)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var key sessionKeyRecord
	if err := json.Unmarshal(raw, &key); err != nil {
		t.Fatal(err)
	}
	key.Root = t.TempDir()
	raw, err = json.Marshal(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	idx.Refresh()
	if _, ok := idx.Session(reg.ReplyID); ok {
		t.Fatal("identity mismatch retained stale session")
	}
}

func TestDashboardTransientRecordClassification(t *testing.T) {
	for _, kind := range []string{"malformed-key", "malformed-part", "io", "permission", "absent", "scan-error"} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("WS_MCP_NO_DASHBOARD", "1")
			f := newWhiteboardFixture(t)
			src := dashboardPageSource(t, nil, f.s.sessions.boards)
			reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
			d, err := src.store.dirs()
			if err != nil {
				t.Fatal(err)
			}
			path := d.partPath(f.key, partKey)
			switch kind {
			case "malformed-key":
				err = os.WriteFile(path, []byte("{"), 0600)
			case "malformed-part":
				err = os.WriteFile(d.partPath(f.key, partTodos), []byte("{"), 0600)
			case "io":
				if err = os.Remove(path); err == nil {
					err = os.Mkdir(path, 0700)
				}
			case "permission":
				if err = os.Chmod(path, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(path, 0600) })
				if _, err := os.ReadFile(path); err == nil {
					t.Skip("platform permits owner read despite mode 000")
				}
			case "absent":
				err = os.Remove(path)
			case "scan-error":
				src.listKeys = func(sessionDirs) ([]string, error) { return nil, os.ErrPermission }
			}
			if err != nil {
				t.Fatal(err)
			}
			_, found, readErr := src.ReadSession(reg)
			_, pageFound, pageErr := src.ReadPage(reg)
			if kind == "absent" {
				if found || readErr != nil || pageFound || pageErr != nil {
					t.Fatalf("absence: %v %v / %v %v", found, readErr, pageFound, pageErr)
				}
			} else if found || readErr == nil || pageFound || pageErr == nil {
				t.Fatalf("transient collapsed into absence: %v %v / %v %v", found, readErr, pageFound, pageErr)
			}
			if kind == "permission" || kind == "scan-error" {
				if !errors.Is(readErr, os.ErrPermission) || !errors.Is(pageErr, os.ErrPermission) {
					t.Fatal("permission cause was erased")
				}
			}
			if kind == "malformed-key" || kind == "malformed-part" {
				var syntax *json.SyntaxError
				if !errors.As(readErr, &syntax) || !errors.As(pageErr, &syntax) {
					t.Fatal("decode cause was erased")
				}
			}
		})
	}
}
