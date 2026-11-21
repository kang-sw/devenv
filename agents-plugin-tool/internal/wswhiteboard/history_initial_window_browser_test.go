package wswhiteboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// The initial history view holds at least the bottom 1000px (whole rows,
// crossing row included), each "load above" adds another >= 1000px, and a
// history under 1000px never folds. Needs WS_PLAYWRIGHT_MODULE.
func TestHistoryInitialWindowBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	dir := t.TempDir()
	write := func(name string, n int) string {
		st := &State{}
		for i := 0; i < n; i++ {
			mustApply(t, st, fmt.Sprintf(`<wb:topic name="t"><wb:log>log %d: a reasonably long line of text for the history window</wb:log></wb:topic>`, i))
		}
		view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "win", Location: time.UTC})
		regions, err := RenderRegions(view)
		if err != nil {
			t.Fatal(err)
		}
		assets, err := PageAssets()
		if err != nil {
			t.Fatal(err)
		}
		page := `<!DOCTYPE html><html><head><style>` + string(assets.CSS) + `</style></head><body>` + string(assets.Sprite) + `<main>` + string(regions.Board) + `</main><aside>` + string(regions.Panels) + `</aside><script>` + string(wsi18n.Script(view.Lang)) + `</script><script>` + string(assets.AnswersScript) + `</script><script>(function(){` + string(assets.FormScript) + `})();</script></body></html>`
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	long, short := write("long.html", 100), write("short.html", 12)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/history-initial-window.cjs", long, short)
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("initial window: %v", err)
	}
}
