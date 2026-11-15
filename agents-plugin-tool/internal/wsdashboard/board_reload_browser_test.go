package wsdashboard

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Pure renderers and a disposable HTTP server only: no daemon or providers.
func TestBoardViewedReloadBrowser(t *testing.T) {
	if os.Getenv("WS_VIEWED_PERF_PLAYWRIGHT") == "" || os.Getenv("WS_VIEWED_PERF_CHROMIUM") == "" {
		t.Skip("requires cached WS_VIEWED_PERF_PLAYWRIGHT/CHROMIUM")
	}
	state := &wswhiteboard.State{Seq: 100}
	for i := 0; i < 18; i++ {
		topic := wswhiteboard.TopicDoc{Name: fmt.Sprintf("Topic%02d", i), Changed: 100 - i}
		topic.Entries = []wswhiteboard.Entry{{ID: fmt.Sprintf("e%d", i+1), Kind: wswhiteboard.EntryPanel, PanelID: fmt.Sprintf("p%d", i+1), Recap: "Stable panel", Text: `<div style="height:240px">Reading material</div>`, At: "2026-11-15T00:00:00Z"}}
		if i == 9 {
			// The reading target starts outside the newest three-panel window.
			for j := 0; j < 3; j++ {
				topic.Entries = append(topic.Entries, wswhiteboard.Entry{ID: fmt.Sprintf("e%d", 100+j), Kind: wswhiteboard.EntryPanel, PanelID: fmt.Sprintf("p%d", 100+j), Recap: "Newer panel", Text: `<div style="height:240px">Newer content</div>`, At: "2026-11-15T00:00:00Z"})
			}
		}
		state.Topics = append(state.Topics, topic)
	}
	render := func() regionSet {
		wb, err := wswhiteboard.RenderRegions(wswhiteboard.BuildView(state, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: "reloadtest"}))
		if err != nil {
			t.Fatal(err)
		}
		regions, err := renderSessionRegions(SessionView{ReplyID: "reloadtest", Adapter: true, Live: true, HasBoard: true, Seq: state.Seq, Board: wb.Board, Panels: wb.Panels}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return regions
	}
	regions := render()
	page, err := renderSessionPage(SessionView{ReplyID: "reloadtest", Adapter: true, Live: true, HasBoard: true, Seq: state.Seq}, regions, false, "")
	if err != nil {
		t.Fatal(err)
	}
	// A content-only update above the reader: no recency/order change.
	state.Topics[0].Entries = append(state.Topics[0].Entries, wswhiteboard.Entry{ID: "e99", Kind: wswhiteboard.EntryAgent, Text: "One new chronological log line above the reader.", At: "2026-11-15T00:00:01Z"})
	changed := render()[regionBoard]
	mux := http.NewServeMux()
	mux.HandleFunc("/s/reloadtest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, page)
	})
	mux.HandleFunc("/board-update", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, changed.HTML)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if data, ct, ok := wswhiteboard.FrameAsset(path.Base(r.URL.Path)); ok {
			w.Header().Set("Content-Type", ct)
			w.Write(data)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/viewed-reload.browser.cjs", server.URL)
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("viewed reload browser: %v", err)
	}
}
