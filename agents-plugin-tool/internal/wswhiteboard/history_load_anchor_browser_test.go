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

// Loading above a fold keeps the row under it at the same viewport position,
// in the page scroller and in a nested one, for a top fold and an inner fold,
// across repeated clicks, also while late images in the loaded rows size.
// A wheel during that settle window ends the hold, and the hold ends by
// itself. Needs WS_PLAYWRIGHT_MODULE.
func TestHistoryLoadAnchorBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	dir := t.TempDir()
	write := func(name string, question bool) string {
		st := &State{}
		seq := 0
		logs := func(n int) {
			for i := 0; i < n; i++ {
				seq++
				mustApply(t, st, fmt.Sprintf(`<wb:topic name="t"><wb:log>log %d: a reasonably long line of text for the history window</wb:log></wb:topic>`, seq))
			}
		}
		logs(150)
		if question {
			mustApply(t, st, `<wb:topic name="t"><wb:add class="technical"><wb:title>still open</wb:title></wb:add></wb:topic>`)
			logs(150)
		}
		view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "anc", Location: time.UTC})
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
	top, inner := write("top.html", false), write("inner.html", true)
	shots := os.Getenv("WS_ANCHOR_SHOTS")
	if shots == "" {
		shots = dir
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/history-load-anchor.cjs", top, inner, shots)
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("load anchor: %v", err)
	}
}
