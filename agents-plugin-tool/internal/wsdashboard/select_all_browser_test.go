package wsdashboard

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// The shared handler has its own slot, leaving the content-addressed main
// bundle identical to the assembled dashboard sources on frames and the index.
func TestPagesCarryScopedSelectAllScript(t *testing.T) {
	f := startPageFixture(t)
	wb, err := wswhiteboard.PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	script := "<script>" + string(wb.SelectAllScript) + "</script>"
	for _, path := range []string{"/s/livesession", "/s/livesession?frame=1", "/s/boardsession?frame=1", "/", "/?frame=1"} {
		code, page := get(t, f.base+path)
		page = frameDependencies(t, page)
		if code != http.StatusOK || strings.Count(page, script) != 1 {
			t.Errorf("%s: status %d, scoped select-all script count %d, want 200 and one", path, code, strings.Count(page, script))
		}
	}
}

// Exercises the actual dashboard (shell and session), standalone whiteboard,
// and file-preview sheet, without connecting to the shared daemon.
func TestScopedSelectAllBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	state := &wswhiteboard.State{Seq: 1, Topics: []wswhiteboard.TopicDoc{{
		Name: "Plan", Body: "First **card body** paragraph.\n\nSecond paragraph with `copyable code`.", Changed: 1,
		Entries: []wswhiteboard.Entry{{ID: "e1", Kind: "agent", Text: "A single log entry to select."}},
	}}, Items: []wswhiteboard.Item{{ID: 1, Class: "policy", Topic: "Plan", Title: "A copyable item title", Rationale: "Only this card body.", Options: []string{"First choice", "Second choice"}, Status: "open"}}}
	view := wswhiteboard.BuildView(state, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: "selection-fixture",
		Panels: []wswhiteboard.Panel{{ID: "notes", Title: "Notes panel", Rows: []wswhiteboard.PanelRow{{Text: "Only this panel body."}}}},
	})
	regions, err := wswhiteboard.RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	f := startPageFixture(t)
	writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000})
	f.src.setPage("livesession", PageContent{HasBoard: true, Seq: 1, Board: regions})
	writeFile(t, filepath.Join(f.project, "selection-scope.txt"), "Only the file preview content.\nNot the rail, header, or background card.\n")
	f.d.Index().Refresh()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "node", "testdata/select-all.browser.cjs", f.base, os.Getenv("WS_SELECT_ALL_SHOTS")).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("scoped select-all: %v", err)
	}
}
