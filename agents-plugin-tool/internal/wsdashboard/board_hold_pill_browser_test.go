package wsdashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
	"golang.org/x/net/websocket"
)

// Only renderers and an owned Port 0 websocket: no daemon, credentials, or
// provider startup. Canonical placement is deliberately unchanged by updates;
// this acceptance test covers the indicator, not the later reorder-hold phases.
func TestBoardHoldPillBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for isolated Chromium acceptance")
	}
	artifacts := os.Getenv("WS_BOARD_HOLD_ARTIFACTS")
	if artifacts == "" {
		artifacts = t.TempDir()
	}
	if err := os.MkdirAll(artifacts, 0o700); err != nil {
		t.Fatal(err)
	}
	artifacts, err := filepath.Abs(artifacts)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conn *websocket.Conn
	var st *wswhiteboard.State
	var topics boardTopicState
	apply := func(command string) error {
		_, problems := wswhiteboard.Apply(st, command, wswhiteboard.Env{Now: time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)})
		if len(problems) != 0 {
			return fmt.Errorf("fixture Apply: %v", problems)
		}
		return nil
	}
	reset := func() error {
		st = &wswhiteboard.State{}
		topics = boardTopicState{}
		for i := 25; i > 0; i-- {
			command := fmt.Sprintf(`<wb:topic name="T%02d"><wb:log>Stable reading context for topic %02d.</wb:log><wb:log>Keep the visible review notes in place until the reader applies the update.</wb:log></wb:topic>`, i, i)
			if i == 4 {
				command += `<wb:topic name="T04"><wb:panel recap="Reading panel"><p>Stable panel document.</p></wb:panel></wb:topic>`
			}
			if i <= 2 {
				command += fmt.Sprintf(`<wb:topic name="T%02d"><wb:add class="policy"><wb:title>Review choice %d</wb:title><wb:option>A</wb:option><wb:option>B</wb:option></wb:add></wb:topic>`, i, i)
			}
			if err := apply(command); err != nil {
				return err
			}
		}
		for i := range st.Topics {
			var number int
			fmt.Sscanf(st.Topics[i].Name, "T%d", &number)
			st.Topics[i].Changed = 1000 - number
		}
		return nil
	}
	if err := reset(); err != nil {
		t.Fatal(err)
	}
	render := func(adapter bool) (SessionView, regionSet, error) {
		view := wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: "holdpill"})
		view.Lang = "ko"
		wb, err := wswhiteboard.RenderRegions(view)
		if err != nil {
			return SessionView{}, nil, err
		}
		v := SessionView{ReplyID: "holdpill", Adapter: adapter, Live: true, HasBoard: true, Seq: st.Seq, Board: wb.Board, Panels: wb.Panels, Lang: "ko"}
		r, err := renderSessionRegions(v, nil)
		return v, r, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if conn != nil {
			conn.Close()
			conn = nil
		}
		if err := reset(); err != nil {
			http.Error(w, err.Error(), 500)
		}
	})
	mux.HandleFunc("/s/holdpill", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		v, regions, err := render(r.URL.Query().Get("document") != "1")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		page, err := renderSessionPage(v, regions, false, "")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		// Observe production morphs; do not replace its DOM or staging logic.
		page = strings.Replace(page, "function morphBoard(", "function morphBoard(){window.__holdMorphs=(window.__holdMorphs||0)+1;return __holdMorphBoard.apply(this,arguments);} function __holdMorphBoard(", 1)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, page)
	})
	mux.Handle("/s/holdpill/events", websocket.Handler(func(c *websocket.Conn) {
		mu.Lock()
		conn = c
		_, regions, err := render(true)
		if err == nil {
			region := regions[regionBoard]
			next := splitBoardTopics(string(region.HTML))
			event, data := topics.event(region, next)
			err = websocket.JSON.Send(c, map[string]any{"event": event, "data": data})
			topics.previous = next
		}
		mu.Unlock()
		if err != nil {
			return
		}
		for {
			var incoming string
			if websocket.Message.Receive(c, &incoming) != nil {
				return
			}
		}
	}))
	mux.HandleFunc("/update", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method != "POST" || conn == nil {
			http.Error(w, "no private stream", 400)
			return
		}
		name := r.URL.Query().Get("topic")
		var changed int
		for _, topic := range st.Topics {
			if topic.Name == name {
				changed = topic.Changed
			}
		}
		command := fmt.Sprintf(`<wb:topic name="%s"><wb:log>Pending review update %d.</wb:log></wb:topic>`, name, st.Seq)
		if r.URL.Query().Get("close") == "1" {
			command = fmt.Sprintf(`<wb:topic name="%s"><wb:close result="Reviewed"/></wb:topic>`, name)
		}
		if err := apply(command); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		for i := range st.Topics {
			if st.Topics[i].Name == name {
				st.Topics[i].Changed = changed
				if r.URL.Query().Get("rise") == "1" {
					st.Topics[i].Changed = 2000 + st.Seq
				}
			}
		}
		_, regions, err := render(true)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		region := regions[regionBoard]
		next := splitBoardTopics(string(region.HTML))
		event, data := topics.event(region, next)
		if err := websocket.JSON.Send(conn, map[string]any{"event": event, "data": data}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		topics.previous = next
		json.NewEncoder(w).Encode(map[string]any{"hash": region.Hash, "event": event})
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
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		if conn != nil {
			conn.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/board-hold-pill.browser.cjs", server.URL, artifacts)
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("board hold pill browser: %v", err)
	}
}
