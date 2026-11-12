package wsdashboard

import (
	"strings"
	"testing"
)

// TestPinnedColumnWidensThePage pins the pinned column's width budget: the
// page wrapper grows by the column (--pinw, user-resizable) so the board
// keeps its no-pin width, instead of the column and the board splitting the
// wrapper's fixed cap. Below 2200px the column replaces the rail, so the
// growth is the column less the rail and its gap.
func TestPinnedColumnWidensThePage(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		":root[data-pincol] .dwrap:not(.narrow) { max-width: calc(1680px - 340px + var(--pinw, 480px)); }",
		":root[data-pincol] .dwrap:not(.narrow) { max-width: calc(1680px + 16px + var(--pinw, 480px)); }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
}
