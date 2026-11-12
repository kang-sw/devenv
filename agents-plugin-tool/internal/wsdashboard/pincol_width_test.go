package wsdashboard

import (
	"strings"
	"testing"
)

// TestPinnedColumnWidensThePage pins the pinned column's width budget: the
// page wrapper grows by the column (--pinw, user-resizable) so the board
// keeps its no-pin width, instead of the column and the board splitting the
// wrapper's fixed cap. Below 2200px the column replaces the rail, so the
// growth is the column less the rail and its gap. Both drawers (the
// session drawer's .dr-in, the tracker's through --tkmaxw) grow with the
// page, so their handles stay at its right edge.
func TestPinnedColumnWidensThePage(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		":root[data-pincol] .dwrap:not(.narrow) { max-width: calc(1680px - 340px + var(--pinw, 480px)); }",
		":root[data-pincol] .dwrap:not(.narrow) { max-width: calc(1680px + 16px + var(--pinw, 480px)); }",
		":root[data-pincol] .dr-in { max-width: calc(1680px - 340px + var(--pinw, 480px)); }",
		":root[data-pincol] .tk-drawer { --tkmaxw: calc(1680px - 340px + var(--pinw, 480px)); }",
		":root[data-pincol] .dr-in { max-width: calc(1680px + 16px + var(--pinw, 480px)); }",
		":root[data-pincol] .tk-drawer { --tkmaxw: calc(1680px + 16px + var(--pinw, 480px)); }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
}

// TestEdgePinZoneFitsTheStripRightOfThePage pins the first-pin edge zone's
// width: it fills the strip right of the 1680px page, at least 48px and at
// most 380px, so it leaves the board column to its own reorder drops; below
// 1880px, where that strip is narrow, its label runs vertically.
func TestEdgePinZoneFitsTheStripRightOfThePage(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	zone := ""
	for _, line := range strings.Split(css, "\n") {
		if strings.HasPrefix(line, ".bpzone {") {
			zone = line
		}
	}
	if want := "width: clamp(48px, calc((100vw - 1680px) / 2 + 4px), 380px);"; !strings.Contains(zone, want) {
		t.Errorf("the .bpzone rule lacks %q: %q", want, zone)
	}
	if want := "@media (max-width: 1879.98px) { .bpzone { writing-mode: vertical-rl; padding: 12px 4px; } }"; !strings.Contains(css, want) {
		t.Errorf("dashboard.css lacks %q", want)
	}
}
