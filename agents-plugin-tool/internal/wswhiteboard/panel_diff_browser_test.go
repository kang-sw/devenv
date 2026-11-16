package wswhiteboard

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func panelDiffEdgeFixture() View {
	st := &State{Topics: []TopicDoc{{Name: "edges"}}}
	for i, pair := range [][2]string{
		{"all removed", ""},
		{"- keep\n- gone a\n- gone b", "- keep"},
		{"```go\nx := 1\nkeep := true\n```", "```go\nx := 2\nkeep := true\n```"},
		{"- **parent** old\n  - child", "- **parent** changed\n  - child"},
		{"first\n\nsecond", "first"},
		{"<div><ul><li>old</li></ul></div>", "<div><ul><li>new</li></ul></div>"},
	} {
		for _, body := range pair {
			entries := &st.Topics[0].Entries
			*entries = append(*entries, Entry{ID: fmt.Sprintf("e%d", len(*entries)+1), Kind: EntryPanel, PanelID: fmt.Sprintf("p%d", i+1), Recap: fmt.Sprintf("edge %d", i+1), Text: body})
		}
	}
	return BuildView(st, ViewInput{Layout: LayoutFull})
}

// Uses real rendered three/four-version boards; only the streaming transport
// and the dashboard's frame mounting are supplied by the Chromium harness.
func TestPanelDiffBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	a, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{}
	for _, name := range []string{"/", "/live", "/framed", "/edge"} {
		v := panelDiffFixture(t, name == "/live")
		if name == "/edge" {
			v = panelDiffEdgeFixture()
		}
		var buf bytes.Buffer
		if err := a.templateFor(v.Lang).ExecuteTemplate(&buf, "page", newPageData(v, a, name == "/framed")); err != nil {
			t.Fatal(err)
		}
		pages[name] = buf.String()
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if page, ok := pages[r.URL.Path]; ok {
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
	cmd := exec.CommandContext(ctx, "node", "testdata/panel-diff.browser.cjs", srv.URL, os.Getenv("WS_PANEL_DIFF_SHOTS"))
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("panel diff Chromium: %v", err)
	}
}
