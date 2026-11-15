package wsdashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// TestBoardViewedFixture is a server-only opt-in. The approved outer runner
// copies these identical fixture bytes into two complete source snapshots,
// compiles each independently, and owns the single browser driver and cleanup.
func TestBoardViewedFixture(t *testing.T) {
	readyPath := os.Getenv("WS_VIEWED_READY")
	if readyPath == "" {
		t.Skip("server fixture requires WS_VIEWED_READY and the approved outer runner")
	}
	stopPath, pin := os.Getenv("WS_VIEWED_STOP"), os.Getenv("WS_VIEWED_SOURCE_PIN")
	if stopPath == "" || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(pin) {
		t.Fatal("stop file and full source pin are required")
	}
	// Never opt into a developer tree or an installed daemon. Port 0 and both
	// temporary directories are local to this test process.
	SetDevAssets(nil)
	f := startPageFixture(t, func(o *DaemonOptions) {
		o.Port = 0
		o.LAN = false
		o.bindHost = "127.0.0.1"
	})
	writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000})
	states := map[string]string{}
	for _, variant := range []string{"base", "queue1", "queue2", "changeA", "changeB", "pin1", "pin2", "mixed", "receipt", "transfer", "transferEdit"} {
		st := viewedBrowserState(variant)
		view := wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: "livesession"})
		regions, err := wswhiteboard.RenderRegions(view)
		if err != nil {
			t.Fatal(err)
		}
		states[variant] = string(regions.Board)
		if variant == "base" {
			f.src.setPage("livesession", PageContent{HasBoard: true, Seq: 100, Board: regions})
			f.src.mu.Lock()
			f.src.boards["livesession"] = st
			f.src.mu.Unlock()
		}
	}
	f.d.Index().Refresh()
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/state" {
			http.NotFound(w, r)
			return
		}
		html, ok := states[r.URL.Query().Get("v")]
		if !ok {
			http.Error(w, "unknown immutable fixture state", http.StatusBadRequest)
			return
		}
		// This endpoint is a second exact Port 0 origin. Permit only its
		// paired dashboard document to read immutable fixture snapshots.
		w.Header().Set("Access-Control-Allow-Origin", f.base)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]string{"html": html, "source_pin": pin})
	}))
	defer fixture.Close()
	assets, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	ready := map[string]any{
		"pid": os.Getpid(), "dashboard_origin": f.base, "fixture_origin": fixture.URL,
		"asset_id": assets.data.ID, "source_pin": pin, "cache_dir": f.dir, "project_dir": f.project,
	}
	raw, err := json.MarshalIndent(ready, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(readyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readyPath+".tmp", append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(readyPath+".tmp", readyPath); err != nil {
		t.Fatal(err)
	}
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupt)
	lifetime := time.NewTimer(240 * time.Second)
	defer lifetime.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-interrupt:
			return
		case <-lifetime.C:
			t.Fatal("fixture lifetime cap reached before owner stop")
		case <-tick.C:
			if _, err := os.Stat(stopPath); err == nil {
				return
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
	}
}

func viewedBrowserState(variant string) *wswhiteboard.State {
	st := &wswhiteboard.State{Seq: 100}
	add := func(name string, changed int, panels ...int) {
		topic := wswhiteboard.TopicDoc{Name: name, Changed: changed}
		for _, p := range panels {
			h := 210
			if p == 1 || p == 12 {
				h = 1100
			}
			if p == 3 || p == 6 || p == 7 {
				h = 290
			}
			text := fmt.Sprintf(`<div style="height:%dpx;padding:12px;box-sizing:border-box"><h2>%s panel p%d</h2><p>Stable reading material, table notes and review context.</p><a href="#p2">Return to reading panel</a></div>`, h, name, p)
			if (variant == "changeA" || variant == "mixed") && p == 1 {
				text += `<p>External rewrite above the unchanged sibling.</p>`
			}
			if (variant == "queue1" || variant == "queue2" || variant == "changeB") && p == 3 {
				text += `<p>Queued external revision ` + variant + `</p>`
			}
			if (variant == "pin1" || variant == "pin2") && p == 7 {
				text += `<p>Pinned external revision ` + variant + `</p>`
			}
			if variant == "transferEdit" && p == 10 {
				text += `<p>Fetched external revision.</p>`
			}
			// A write-only fixture stimulus makes the production frame bridge
			// report real content growth. The driver never reads opaque DOM.
			text += fmt.Sprintf(`<script>addEventListener('message',function(e){if(e.source!==parent||!e.data||e.data.viewedFixturePanel!=='p%d')return;var h=e.data.pixels;if(!Number.isInteger(h)||h<1||h>100)return;var n=document.createElement('div');n.style.cssText='height:'+h+'px;margin:0;padding:0';(document.querySelector('.doc-body')||document.body).appendChild(n);});</script>`, p)
			topic.Entries = append(topic.Entries, wswhiteboard.Entry{ID: "e" + strconv.Itoa(p), Kind: wswhiteboard.EntryPanel, PanelID: "p" + strconv.Itoa(p), Text: text, Recap: fmt.Sprintf("%s reading panel %d", name, p), At: "2026-11-15T00:00:00Z"})
		}
		if name == "A" && (variant == "mixed" || variant == "receipt") {
			topic.Entries = append(topic.Entries, wswhiteboard.Entry{ID: "e99", Kind: wswhiteboard.EntrySystem, ItemID: 1, Text: "Item {{1}} answered [yes]: fixture receipt", At: "2026-11-15T00:00:01Z"})
		}
		st.Topics = append(st.Topics, topic)
	}
	add("Above", 100, 4)
	add("A", 90, 1, 2)
	b := 80
	if variant == "queue1" || variant == "queue2" {
		b = 200
	}
	add("B", b, 3)
	add("Below", 70, 5)
	add("PinAbove", 60, 8)
	add("P", 50, 6)
	q := 40
	if variant == "pin1" || variant == "pin2" {
		q = 210
	}
	add("Q", q, 7)
	add("PinBelow", 30, 9)
	// Past and its older tail are outside the initial closed-topic cap.
	// The real tail panel supplies scroll range below the fetched reading pair.
	add("PastTail", 10, 12)
	st.ClosedTopics = append(st.ClosedTopics, "PastTail")
	add("Past", 20, 10, 11)
	st.ClosedTopics = append(st.ClosedTopics, "Past")
	for i := 0; i < 30; i++ {
		name := fmt.Sprintf("Archive %02d", i)
		add(name, 1)
		st.ClosedTopics = append(st.ClosedTopics, name)
	}
	if variant == "transfer" || variant == "transferEdit" {
		closed := st.ClosedTopics[:0]
		for _, name := range st.ClosedTopics {
			if name != "Past" {
				closed = append(closed, name)
			}
		}
		st.ClosedTopics = closed
		for i := range st.Topics {
			if st.Topics[i].Name == "Past" {
				st.Topics[i].Changed = 300
			}
		}
	}
	return st
}
