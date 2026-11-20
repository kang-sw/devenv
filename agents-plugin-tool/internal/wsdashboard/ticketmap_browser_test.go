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

// ticketMapEnv is a framed live page with a tickets panel over the synthetic
// board written under the session's project root, so the route's payload
// and the file-card hand-off's preview both resolve it.
type ticketMapEnv struct {
	f      *pageFixture
	src    *ticketMapFake
	graph  string // the payload the route serves, for the browser to read
	shots  string // WS_TICKETMAP_SHOTS, "" when unset
	mockup string // the approved mockup, "" when absent
}

// startTicketMapBrowser skips without WS_PLAYWRIGHT_MODULE and otherwise
// starts the shared ticket map browser fixture.
func startTicketMapBrowser(t *testing.T) *ticketMapEnv {
	t.Helper()
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to use cached Chromium")
	}
	env := &ticketMapEnv{shots: os.Getenv("WS_TICKETMAP_SHOTS")}
	if env.shots != "" && !filepath.IsAbs(env.shots) {
		t.Fatal("WS_TICKETMAP_SHOTS must be absolute")
	}
	mockup, err := filepath.Abs(filepath.Join("..", "..", "..", "ai-docs", "assets", "mockups", "261119", "ticket-map-view", "ticket-map-v5-bc.html"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mockup); err == nil {
		env.mockup = mockup
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
	env.f = startPageFixture(t, func(o *DaemonOptions) {
		o.Port = 0
		o.LAN = false
		o.bindHost = "127.0.0.1"
		pf := o.Sessions.(*pageFake)
		p := pf.page("livesession")
		p.Board = regions
		p.Seq++
		pf.setPage("livesession", p)
		env.src = &ticketMapFake{pageFake: pf}
		o.Sessions = env.src
	})
	wsdoctest.WriteTicketMapFixture(t, env.f.project, wsdoctest.TicketMapDefaultOpen)
	env.graph = ticketMapPayload(t, env.f.project)
	return env
}

// ticketMapPayload writes the board's encoded payload to a temp file.
func ticketMapPayload(t *testing.T, project string) string {
	t.Helper()
	payload, err := wsdoc.TicketMapJSON(project)
	if err != nil {
		t.Fatal(err)
	}
	graph := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(graph, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	return graph
}

// run runs a browser script under ctx's deadline and requires the page to
// have read the ticket graph through the route.
func (env *ticketMapEnv) run(t *testing.T, label, script string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "node", append([]string{script}, args...)...).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium %s: %v", label, err)
	}
	env.src.mu.Lock()
	reads := env.src.reads
	env.src.mu.Unlock()
	if reads == 0 {
		t.Fatal("the browser never read the ticket graph")
	}
}

// The ticket map overlay and View B in Chromium (the ticket's Phase 2
// Playwright items 1-14): the #thandle placement, the first open's one
// request, the matrix, the 3-tier click-focus with measured connectors, the
// semantic grid, Full View and the file-card hand-off, Esc layering, the
// framed shell's dim, and the 900px layout, in both themes.
// WS_TICKETMAP_SHOTS (absolute) keeps the overlay's screenshots beside the
// approved mockup's same states for the side-by-side capture check.
func TestTicketMapBrowser(t *testing.T) {
	env := startTicketMapBrowser(t)
	env.run(t, "ticket map", "testdata/ticketmap.browser.cjs", env.f.base, env.graph, wsdoctest.TicketMapEpicStem, env.shots, env.mockup)
}
