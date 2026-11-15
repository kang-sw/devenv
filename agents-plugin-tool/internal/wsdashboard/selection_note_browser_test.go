package wsdashboard

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// Opt in with WS_PLAYWRIGHT_MODULE pointing at an installed playwright module.
// This uses only disposable fixture sessions, never an installed/shared daemon.
func TestSelectionNoteBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	f := startPageFixture(t)
	for _, id := range []string{"livesession", "boardsession"} {
		writeSnapshot(t, f.dir, id, &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000})
	}
	board := boardRegions(t, "Sandbox selection text")
	original := string(board.Board)
	host := func(updated bool) string {
		first := `<p id="quote-first">First immutable quote</p>`
		if updated {
			first = `<p id="quote-updated">Replacement source must not retarget</p>`
		}
		return fmt.Sprintf(`<section class="topic" data-topic="review" data-changed="%d"><div class="tophead"><div class="tbar"><b>Review</b></div></div><div class="topic-stream"><div class="entry" data-entry="e7" data-entry-item="3">%s</div><div class="entry" data-entry="e8"><p id="quote-next">Second immutable quote</p></div></div></section>`, 1+map[bool]int{true: 1}[updated], first)
	}
	setBoard := func(updated bool) {
		b := board
		b.Board = template.HTML(host(updated) + original)
		f.src.setPage("livesession", PageContent{HasBoard: true, Seq: 10 + map[bool]int{true: 1}[updated], Board: b})
		f.d.Index().Refresh()
	}
	setBoard(false)
	updates := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setBoard(r.URL.Query().Get("reset") != "1")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer updates.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/selection-note.browser.cjs", f.base, updates.URL)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium selection note acceptance: %v", err)
	}
}
