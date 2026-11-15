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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdevassets"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
	"golang.org/x/net/websocket"
)

// This deliberately does not instantiate a Daemon: page fixtures may start
// providers. Only pure production renderers and a private websocket are needed.
func TestBoardViewedLiveBrowser(t *testing.T) {
	if os.Getenv("WS_VIEWED_PERF_PLAYWRIGHT") == "" || os.Getenv("WS_VIEWED_PERF_CHROMIUM") == "" {
		t.Skip("requires cached WS_VIEWED_PERF_PLAYWRIGHT/CHROMIUM")
	}
	if root := os.Getenv("WS_VIEWED_LIVE_ASSETS_ROOT"); root != "" {
		r, err := wsdevassets.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		SetDevAssets(r)
		defer SetDevAssets(nil)
	}
	st := &wswhiteboard.State{Seq: 1200, EntrySeq: 1200, PanelCounter: 300}
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("Topic %03d", i)
		if i == 0 {
			name = "A"
		}
		if i == 1 {
			name = "B"
		}
		topic := wswhiteboard.TopicDoc{Name: name, Changed: 1200 - i}
		for j := 0; j < 12; j++ {
			id := i*12 + j + 1
			e := wswhiteboard.Entry{ID: fmt.Sprintf("e%d", id), Kind: wswhiteboard.EntryAgent, Text: fmt.Sprintf("Live reasoning summary %d: inspected the render transaction, compared stable identities and checked geometry before admitting the next event.\n\nLonger review notes retain realistic wrapping and chronological history.", id), At: "2026-11-15T00:00:00Z"}
			if j%4 == 0 {
				p := i*3 + j/4 + 1
				e.Kind = wswhiteboard.EntryPanel
				e.PanelID = fmt.Sprintf("p%d", p)
				e.Recap = fmt.Sprintf("Image-heavy review panel %d", p)
				e.Text = viewedLivePayload(p)
			}
			topic.Entries = append(topic.Entries, e)
		}
		st.Topics = append(st.Topics, topic)
	}
	var mu sync.Mutex
	var conn *websocket.Conn
	var topics boardTopicState
	render := func() (regionSet, error) {
		wb, err := wswhiteboard.RenderRegions(wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: "viewedlive"}))
		if err != nil {
			return nil, err
		}
		return renderSessionRegions(SessionView{ReplyID: "viewedlive", Adapter: true, Live: true, HasBoard: true, Seq: st.Seq, Board: wb.Board, Panels: wb.Panels}, nil)
	}
	regions, err := render()
	if err != nil {
		t.Fatal(err)
	}
	page, err := renderSessionPage(SessionView{ReplyID: "viewedlive", Adapter: true, Live: true, HasBoard: true, Seq: st.Seq}, regions, false, "")
	if err != nil {
		t.Fatal(err)
	}
	// Instrument whole served bytes, not extracted functions or a replacement DOM.
	for _, name := range []string{"swap", "morphBoard", "boardRenderBlockers", "reading", "viewport"} {
		needle := "function " + name + "("
		if strings.Count(page, needle) != 1 {
			if name == "swap" {
				t.Fatal("missing unique swap instrumentation site")
			}
			continue
		}
		wrapper := "function " + name + "(){var t=performance.now();try{return __measured_" + name + ".apply(this,arguments);}finally{var m=window.__viewed;m.calls['" + name + "']=(m.calls['" + name + "']||0)+1;m.ms['" + name + "']=(m.ms['" + name + "']||0)+performance.now()-t;}} function __measured_" + name + "("
		page = strings.Replace(page, needle, wrapper, 1)
	}
	// Record the production admission measurements: late frame shrink/growth
	// must not reinterpret a window after stream.js has already settled it.
	for name, wrapper := range map[string]string{
		"readyHeight": `function readyHeight(row){var height=__admission_readyHeight(row),m=window.__viewed;if(m.historyActive)m.historyActive.rows.push({id:row.getAttribute('data-row'),height:height});return height;} function __admission_readyHeight(`,
		"progress":    `function progress(h,op){var m=window.__viewed;if(!m.historyAdmission)return __admission_progress(h,op);var record={topic:h.getAttribute('data-history'),base:op.height,rows:[]};m.historyActive=record;try{return __admission_progress(h,op);}finally{m.historyActive=null;record.height=op.height;record.done=!!op.done;record.waiting=op.row&&op.row.getAttribute('data-row');record.remaining=op.gap.querySelector('template[data-history-gap]').content.children.length;m.historyAdmission.push(record);}} function __admission_progress(`,
	} {
		needle := "function " + name + "("
		if strings.Count(page, needle) != 1 {
			t.Fatalf("missing unique history admission instrumentation site: %s", name)
		}
		page = strings.Replace(page, needle, wrapper, 1)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/s/viewedlive", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, page)
	})
	mux.Handle("/s/viewedlive/events", websocket.Handler(func(c *websocket.Conn) {
		mu.Lock()
		conn = c
		region := regions[regionBoard]
		next := splitBoardTopics(string(region.HTML))
		event, data := topics.event(region, next)
		err := websocket.JSON.Send(c, map[string]any{"event": event, "data": data})
		topics.previous = next
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
		command := fmt.Sprintf(`<wb:topic name="B"><wb:log>Completed live reasoning event %d: checked layout, preserved the reading window and reported the next bounded result.</wb:log></wb:topic>`, st.Seq)
		if r.URL.Query().Get("panels") == "1" {
			command = `<wb:topic name="A"><wb:panel recap="Late image panel one">` + viewedLivePayload(301) + `</wb:panel><wb:panel recap="Late image panel two">` + viewedLivePayload(302) + `</wb:panel></wb:topic>`
		}
		_, problems := wswhiteboard.Apply(st, command, wswhiteboard.Env{Now: time.Date(2026, 11, 15, 0, 0, 1, 0, time.UTC)})
		if len(problems) > 0 {
			http.Error(w, strings.Join(problems, ";"), 500)
			return
		}
		nextRegions, e := render()
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		region := nextRegions[regionBoard]
		next := splitBoardTopics(string(region.HTML))
		event, data := topics.event(region, next)
		if e = websocket.JSON.Send(conn, map[string]any{"event": event, "data": data}); e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		topics.previous = next
		regions = nextRegions
		json.NewEncoder(w).Encode(map[string]any{"hash": region.Hash, "event": event, "seq": st.Seq})
	})
	// Unrelated dashboard probes have no backing store and cannot escape here.
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
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/viewed-live.browser.cjs", server.URL)
	out, e := cmd.CombinedOutput()
	t.Logf("%s", out)
	if e != nil {
		t.Fatalf("integrated viewed browser: %v", e)
	}
}

func viewedLivePayload(id int) string {
	// The opaque frame receives only a write-only stimulus. Actual IMG intrinsic
	// dimensions change, and the unmodified production frame bridge reports them.
	return fmt.Sprintf(`<h2>Panel %d</h2><p>Stable review context and image-heavy evidence.</p><img width="280" src="data:image/svg+xml,%%3Csvg xmlns='http://www.w3.org/2000/svg' width='280' height='180'%%3E%%3Crect width='280' height='180' fill='tan'/%%3E%%3C/svg%%3E"><script>addEventListener('message',function(e){if(e.source!==parent||!e.data||e.data.viewedLate!==true)return;document.querySelector('img').src="data:image/svg+xml,%%3Csvg xmlns='http://www.w3.org/2000/svg' width='280' height='460'%%3E%%3Crect width='280' height='460' fill='teal'/%%3E%%3C/svg%%3E";});</script>`, id)
}
