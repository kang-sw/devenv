package wswhiteboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

func TestPanelMentionTemplateAndBrowser(t *testing.T) {
	view := BuildView(panelVersionsState(t), ViewInput{Layout: LayoutFull, ReplyID: "panel-mention", Location: time.UTC})
	view.Lang = "en"
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	board := string(regions.Board)
	if got := strings.Count(board, `class="ibtn pfmention"`); got != 2 {
		t.Fatalf("panel mention buttons = %d, want historical and latest", got)
	}
	if !strings.Contains(board, `aria-label="Mention this panel"`) {
		t.Fatal("localized panel mention aria-label missing")
	}
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for panel mention browser test")
	}
	assets, err := PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	page := `<!doctype html><html><head><style>` + string(assets.CSS) + `</style></head><body>` + string(assets.Sprite) + `<main>` + board + `</main><div class="composer"><textarea id="ctext"></textarea></div><script>` + string(wsi18n.Script(view.Lang)) + `</script><script>` + string(assets.AnswersScript) + `</script><script>(function(){` + string(assets.FormScript) + `})();</script></body></html>`
	file := filepath.Join(t.TempDir(), "panel-mention.html")
	if err := os.WriteFile(file, []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/panel-mention.browser.cjs", file, os.Getenv("WS_INLINE_MENTION_SHOTS"))
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("panel mention browser: %v", err)
	}
}
