package wswhiteboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

func TestDiffFenceBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	body := "Review the proposed edit:\n\n```diff\ndiff --git a/notes.txt b/notes.txt\nindex 1234567..89abcde 100644\n--- a/notes.txt\n+++ b/notes.txt\n@@ -1,3 +1,4 @@\n Context line\n-Old line\n+New line\n+Another line\n```"
	st := &State{Seq: 1, EntrySeq: 1, PanelCounter: 1, Topics: []TopicDoc{{Name: "diff preview", Entries: []Entry{{ID: "e1", Kind: EntryPanel, PanelID: "p1", PanelName: "review", Recap: "Proposed edit", Text: body}}}}}
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "markdown-diff"})
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	assets, err := PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	page := `<!DOCTYPE html><html><head><style>` + string(assets.CSS) + `</style></head><body>` + string(assets.Sprite) + `<main>` + string(regions.Board) + `</main><aside>` + string(regions.Panels) + `</aside><script>` + string(wsi18n.Script(view.Lang)) + `</script><script>` + string(assets.AnswersScript) + `</script><script>(function(){` + string(assets.FormScript) + `})();</script></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(page))
			return
		}
		if data, kind, ok := FrameAsset(strings.TrimPrefix(r.URL.Path, "/")); ok {
			w.Header().Set("Content-Type", kind)
			_, _ = w.Write(data)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/markdown-diff.browser.cjs", srv.URL, os.Getenv("WS_MARKDOWN_DIFF_SHOTS"))
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("diff fence Chromium: %v", err)
	}
}
