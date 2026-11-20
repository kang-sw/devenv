package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsdoc/wsdoctest"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// ticketMapFake serves the ticket map from the synthetic board written under
// the session's project root, through the same encoder the real source uses.
type ticketMapFake struct {
	*pageFake
	mu    sync.Mutex
	reads int
}

func (s *ticketMapFake) ReadTicketGraph(reg Registration) ([]byte, bool, error) {
	s.mu.Lock()
	s.reads++
	s.mu.Unlock()
	payload, err := wsdoc.TicketMapJSON(reg.Project)
	return payload, err == nil, err
}

// The ticket map overlay and View B in Chromium (the ticket's Phase 2
// Playwright items 1-14): the #thandle placement, the first open's one
// request, the matrix, the 3-tier click-focus with measured connectors, the
// semantic grid, Full View and the file-card hand-off, Esc layering, the
// framed shell's dim, and the 900px layout, in both themes.
// WS_TICKETMAP_SHOTS (absolute) keeps the overlay's screenshots beside the
// approved mockup's same states for the side-by-side capture check.
func TestTicketMapBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to use cached Chromium")
	}
	shots := os.Getenv("WS_TICKETMAP_SHOTS")
	if shots != "" && !filepath.IsAbs(shots) {
		t.Fatal("WS_TICKETMAP_SHOTS must be absolute")
	}
	mockup, err := filepath.Abs(filepath.Join("..", "..", "..", "ai-docs", "assets", "mockups", "261119", "ticket-map-view", "ticket-map-v5-bc.html"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mockup); err != nil {
		mockup = ""
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	SetDevAssets(nil)
	layout, err := wsdoctest.TicketMapFixtureLayout(wsdoctest.TicketMapDefaultOpen)
	if err != nil {
		t.Fatal(err)
	}
	epicPath := "ai-docs/tickets/" + layout.Dirs[wsdoctest.TicketMapEpicStem] + "/" + wsdoctest.TicketMapEpicStem + ".md"
	// #thandle shows only while the page has a tickets panel.
	view := wswhiteboard.BuildView(&wswhiteboard.State{Seq: 1, Topics: []wswhiteboard.TopicDoc{{Name: "Plan", Body: "ticket map board", Changed: 1}}},
		wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, Panels: []wswhiteboard.Panel{
			{ID: "tickets", Title: "Tickets", Lanes: []wswhiteboard.PanelLane{{Name: "todo", Rows: []wswhiteboard.PanelRow{{Text: "Orchard platform", Path: epicPath}}}}},
		}})
	regions, err := wswhiteboard.RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	var src *ticketMapFake
	f := startPageFixture(t, func(o *DaemonOptions) {
		o.Port = 0
		o.LAN = false
		o.bindHost = "127.0.0.1"
		pf := o.Sessions.(*pageFake)
		p := pf.page("livesession")
		p.Board = regions
		p.Seq++
		pf.setPage("livesession", p)
		src = &ticketMapFake{pageFake: pf}
		o.Sessions = src
	})
	// The board sits under the session's project root, so the route's
	// payload and the file-card hand-off's preview both resolve it.
	wsdoctest.WriteTicketMapFixture(t, f.project, wsdoctest.TicketMapDefaultOpen)
	payload, err := wsdoc.TicketMapJSON(f.project)
	if err != nil {
		t.Fatal(err)
	}
	graph := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(graph, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "node", "testdata/ticketmap.browser.cjs", f.base, graph, wsdoctest.TicketMapEpicStem, shots, mockup).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium ticket map: %v", err)
	}
	src.mu.Lock()
	reads := src.reads
	src.mu.Unlock()
	if reads == 0 {
		t.Fatal("the browser never read the ticket graph")
	}
}
