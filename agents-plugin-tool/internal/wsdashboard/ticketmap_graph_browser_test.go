package wsdashboard

import (
	"testing"

	"github.com/kang-sw/devenv/internal/wsdoc/wsdoctest"
)

// View C in Chromium (the ticket's Phase 3 Playwright items 1-17 and its
// Performance evidence): the stage-1 Keyword Soup on its centroids, the calm
// backdrop, hover and the keyword filter, pan, wheel and button zoom, the
// semantic zoom's label budget, the Ego Ring, the B/C hand-over, touch pinch,
// layout time at 170 and 300 open tickets, wheel frame time and the single
// fetch, in both themes at 1600x960 and 1280x800. The 300-ticket board is
// encoded here and served to one session in place of the route's payload.
// WS_TICKETMAP_SHOTS (absolute) keeps the c-*, ego-* and kw-filter-* states
// beside the approved mockup's same states (mock-c-*).
func TestTicketMapGraphBrowser(t *testing.T) {
	env := startTicketMapBrowser(t)
	big := t.TempDir()
	wsdoctest.WriteTicketMapFixture(t, big, 300)
	graph300 := ticketMapPayload(t, big)
	env.run(t, "ticket map View C", "testdata/ticketmap-graph.browser.cjs", env.f.base, env.graph, graph300, wsdoctest.TicketMapEpicStem, env.shots, env.mockup)
}
