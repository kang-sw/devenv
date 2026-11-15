package wsdashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// TestBoardViewedBrowser is deliberately opt-in: only a cached Playwright and
// disposable production-rendered fixture are used, never an installed daemon.
func TestBoardViewedBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to a cached playwright module")
	}
	capture := os.Getenv("WS_VIEWED_CAPTURE_DIR")
	if capture == "" {
		t.Fatal("WS_VIEWED_CAPTURE_DIR is required for geometry and cleanup evidence")
	}
	if err := os.MkdirAll(capture, 0o755); err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	f := startPageFixture(t)
	writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000})
	states := map[string]string{}
	for _, variant := range []string{"base", "queue1", "queue2", "changeA", "mixed", "transfer"} {
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
	// Read baseline bytes without switching refs or importing historical code.
	assets := []map[string]string{}
	for _, path := range []string{"internal/wsdashboard/assets/dashboard.js", "internal/wsdashboard/assets/dashboard.css", "internal/wswhiteboard/assets/stream.js", "internal/wswhiteboard/assets/page.css"} {
		current, err := os.ReadFile(pathFromDashboard(path))
		if err != nil {
			t.Fatal(err)
		}
		baseline, err := exec.Command("git", "show", "72df121fcadb5e4e027b84c15474e402f9adfc34:agents-plugin-tool/"+path).Output()
		if err != nil {
			t.Fatal(err)
		}
		assets = append(assets, map[string]string{"path": path, "current": string(current), "baseline": string(baseline)})
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/assets":
			json.NewEncoder(w).Encode(assets)
		case "/state":
			html, ok := states[r.URL.Query().Get("v")]
			if !ok {
				http.Error(w, "unknown fixture state", http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"html": html})
		default:
			http.NotFound(w, r)
		}
	}))
	defer fixture.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 125*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/viewed-panels.browser.cjs", f.base, fixture.URL)
	// Give Node's signal handler a chance to close its owned browser server.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("viewed-panel Chromium acceptance: %v (evidence: %s)", err, capture)
	}
}

func pathFromDashboard(path string) string {
	return filepath.Join("..", "..", path)
}

func viewedBrowserState(variant string) *wswhiteboard.State {
	st := &wswhiteboard.State{Seq: 100}
	add := func(name string, changed int, panels ...int) {
		topic := wswhiteboard.TopicDoc{Name: name, Changed: changed}
		for _, p := range panels {
			h := 240
			if p == 3 || p == 6 || p == 7 {
				h = 300
			}
			if p == 1 {
				h = 1000
			}
			text := fmt.Sprintf(`<div style="height:%dpx;padding:12px;box-sizing:border-box"><h2>%s panel p%d</h2><p>Stable reading material, table notes and review context.</p><a href="#p2">Return to reading panel</a></div>`, h, name, p)
			if (variant == "changeA" || variant == "mixed") && p == 1 {
				text += `<p>External rewrite above the unchanged sibling.</p>`
			}
			if (variant == "queue1" || variant == "queue2") && p == 3 {
				text += `<p>Queued external revision ` + variant + `</p>`
			}
			topic.Entries = append(topic.Entries, wswhiteboard.Entry{ID: "e" + strconv.Itoa(p), Kind: wswhiteboard.EntryPanel, PanelID: "p" + strconv.Itoa(p), Text: text, Recap: fmt.Sprintf("%s reading panel %d", name, p), At: "2026-11-15T00:00:00Z"})
		}
		if name == "A" && variant == "mixed" {
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
	add("Q", 40, 7)
	add("PinBelow", 30, 9)
	// The 31st closed topic is fetched, not present in the initial board.
	add("Past", 20, 10, 11)
	st.ClosedTopics = append(st.ClosedTopics, "Past")
	for i := 0; i < 30; i++ {
		name := fmt.Sprintf("Archive %02d", i)
		add(name, 1)
		st.ClosedTopics = append(st.ClosedTopics, name)
	}
	if variant == "transfer" {
		st.ClosedTopics = st.ClosedTopics[1:]
		for i := range st.Topics {
			if st.Topics[i].Name == "Past" {
				st.Topics[i].Changed = 300
			}
		}
	}
	return st
}
