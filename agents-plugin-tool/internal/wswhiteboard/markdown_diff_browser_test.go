package wswhiteboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestDiffFenceBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	body := "Review the proposed edit:\n\n```diff\ndiff --git a/notes.txt b/notes.txt\nindex 1234567..89abcde 100644\n--- a/notes.txt\n+++ b/notes.txt\n@@ -1,3 +1,4 @@\n Context line\n-Old line\n+New line\n+Another line\n```"
	st := &State{Seq: 1, EntrySeq: 1, PanelCounter: 1, Topics: []TopicDoc{{Name: "diff preview", Entries: []Entry{{ID: "e1", Kind: EntryPanel, PanelID: "p1", PanelName: "review", Recap: "Proposed edit", Text: body}}}}}
	page := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "markdown-diff"}))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
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
