package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// TestSelectionCaptureBrowser drags a real pointer selection through a rich
// topic body in Chromium and requires the captured mention to be exactly the
// selected text. Opt in with WS_PLAYWRIGHT_MODULE, like TestSelectionNoteBrowser.
func TestSelectionCaptureBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	f := startPageFixture(t)
	writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000})
	body := "First paragraph with **bold words** and `inline code` here.\n\nSecond paragraph: alpha beta gamma delta.\n\n- list one\n- list two\n"
	f.src.setPage("livesession", PageContent{HasBoard: true, Seq: 10, Board: boardRegions(t, body)})
	f.d.Index().Refresh()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, node, "testdata/selection-capture.browser.cjs", f.base).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium selection capture: %v", err)
	}
}
