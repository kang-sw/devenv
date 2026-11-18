package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	dir := filepath.Join(f.root, filepath.FromSlash(whiteboardDirRel), wsmailbox.ReplyID(other))
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
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
