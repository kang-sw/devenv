package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// whiteboard.load installs another session's downloaded board into the
// caller's board: the first load into a board-less session makes no backup,
// a later one keeps the previous board beside it, and the page is rendered
// as after an update.
func TestWhiteboardLoadReplacesTheCallersBoard(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="handoff" label="Handoff"><wb:panel recap="Design">payload</wb:panel><wb:log>ready</wb:log><wb:add class="check"><wb:title>Verify</wb:title></wb:add></wb:topic>`)
	data, err := whiteboardFolder(f.root, f.key).ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(t.TempDir(), "board.zip")
	if err := os.WriteFile(zipPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	other, err := f.s.sessions.mint(f.root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}

	resp := f.call(t, "whiteboard.load", map[string]any{"session_key": other, "path": zipPath})
	text := toolText(t, resp)
	if isToolError(resp) || !strings.Contains(text, "loaded: 1 topics from "+zipPath) || !strings.Contains(text, "backup: none") {
		t.Fatalf("first load = %s", text)
	}
	if q := toolText(t, f.call(t, "whiteboard.query", map[string]any{"session_key": other})); !strings.Contains(q, "handoff") {
		t.Fatalf("loaded board query = %s", q)
	}
	if v, ok := f.s.sessions.getOverride(other, "workflow.whiteboard_enabled.claude"); !ok || v != "on" {
		t.Fatalf("the load did not turn the session's board knob on: %q, %v", v, ok)
	}
	dir := filepath.Join(f.root, filepath.FromSlash(whiteboardDirRel), wsmailbox.ReplyID(other))
	if _, err := os.Stat(filepath.Join(dir, "index.html")); !os.IsNotExist(err) {
		t.Fatalf("the load did not render the page: %v", err)
	}

	text = toolText(t, f.call(t, "whiteboard.load", map[string]any{"session_key": other, "path": zipPath}))
	if !strings.Contains(text, "backup: the previous board is in "+dir+".bak-") {
		t.Fatalf("second load = %s", text)
	}
	// The source board is untouched, and the backup is no session's board.
	if q := toolText(t, f.call(t, "whiteboard.query", map[string]any{})); !strings.Contains(q, "handoff") {
		t.Fatalf("source board query = %s", q)
	}

	missing := filepath.Join(t.TempDir(), "missing.zip")
	if resp := f.call(t, "whiteboard.load", map[string]any{"session_key": other, "path": missing}); !isToolError(resp) || !strings.Contains(toolText(t, resp), missing) {
		t.Fatalf("missing zip = %s", resp)
	}
}

// whiteboard.load is the top-level lead's alone: a sub-lead is refused, not
// rewritten onto its lead's board as the other whiteboard tools are.
func TestWhiteboardLoadRefusesASublead(t *testing.T) {
	f := newWhiteboardFixture(t)
	sub := mintSublead(t, f.s, f.root, f.key, "ticket-runner")
	resp := f.call(t, "whiteboard.load", map[string]any{"session_key": sub, "path": filepath.Join(t.TempDir(), "b.zip")})
	if !strings.Contains(resp, whiteboardLeadOnlyDenial("whiteboard.load")) {
		t.Fatalf("sub-lead load = %s", resp)
	}
}

// A loaded board reuses the replaced board's item IDs, so a dashboard answer
// sent to the replaced board's item 1 (held or drained in the inbox) must not
// attach to the loaded, unrelated item 1. The page's sent marks and the
// daemon's answer check drop a chip sent before its item's re-ask time
// (wsdashboard sentMarks, ackBatch); the load floors every item's re-ask time
// at the load, so both see the old answer as answering an earlier ask.
func TestWhiteboardLoadDetachesAnswersToTheReplacedBoard(t *testing.T) {
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	f.update(t, `<wb:topic name="old"><wb:add class="policy"><wb:title>Approve the old plan?</wb:title><wb:option>yes</wb:option><wb:option>no</wb:option></wb:add></wb:topic>`)
	if page := readPageOK(t, src, reg); page.Reasked[1] != 0 {
		t.Fatalf("fixture: item 1 already re-asked: %v", page.Reasked)
	}
	// The owner answers the old item 1 from the dashboard.
	sentAt := time.Now().UnixMilli()
	time.Sleep(5 * time.Millisecond)

	other, err := f.s.sessions.mint(f.root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp := f.call(t, "whiteboard.update", map[string]any{"session_key": other, "commands": `<wb:topic name="imported"><wb:add class="policy"><wb:title>Ship the imported change?</wb:title><wb:option>ship</wb:option><wb:option>hold</wb:option></wb:add></wb:topic>`}); isToolError(resp) {
		t.Fatalf("source board: %s", resp)
	}
	data, err := whiteboardFolder(f.root, other).ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(t.TempDir(), "board.zip")
	if err := os.WriteFile(zipPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if resp := f.call(t, "whiteboard.load", map[string]any{"session_key": f.key, "path": zipPath}); isToolError(resp) {
		t.Fatalf("load: %s", resp)
	}

	if q := toolText(t, f.call(t, "whiteboard.query", map[string]any{})); !strings.Contains(q, "Ship the imported change?") {
		t.Fatalf("loaded board query = %s", q)
	}
	if page := readPageOK(t, src, reg); page.Reasked[1] <= sentAt {
		t.Fatalf("page re-ask floor of the loaded item 1 = %d, want after the old answer at %d (Reasked %v)", page.Reasked[1], sentAt, page.Reasked)
	}
	items, ok := src.BoardItems(reg)
	if !ok || !items.Open[1] {
		t.Fatalf("BoardItems = %+v, %v; want the loaded item 1 open", items, ok)
	}
	if items.Reasked[1] <= sentAt {
		t.Fatalf("answer-check re-ask floor of the loaded item 1 = %d, want after the old answer at %d", items.Reasked[1], sentAt)
	}
}
