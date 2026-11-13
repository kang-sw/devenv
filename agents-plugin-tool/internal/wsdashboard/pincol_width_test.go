package wsdashboard

import (
	"strings"
	"testing"
)

// TestPinnedColumnFillsTheWindowRightOfTheBoard pins the wide layout: a
// session page stands at the window's left edge (the index page stays
// centred), and while the pinned column shows, the board keeps its no-pin
// width (--pbw) and the column fills the remaining width to the window's
// right edge, never narrower than --pinw (user-resizable). The page then
// fills the window, and both drawers (the session drawer's .dr-in, the
// tracker's through --tkmaxw and --tkml) follow it, so their handles stay
// at its right edge.
func TestPinnedColumnFillsTheWindowRightOfTheBoard(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		".dwrap { margin: 0; padding: 12px 16px; max-width: 1680px; }",
		".dwrap.narrow { margin: 0 auto; max-width: 1080px; padding-top: 16px; }",
		".dr-in { position: relative; max-width: 1680px; margin: 0; }",
		"@media (min-width: 1600px) { .dgrid.board { margin: 0; } }",
		".dash { --tkml: 0px; }",
		".dash.index { --tkmaxw: 1080px; --tkml: auto; }",
		":root { --pbw: calc(1680px - 32px - 356px); }",
		".dash.board { --pbw: 900px; }",
		":root[data-pincol] .dbody { grid-template-columns: minmax(0, var(--pbw)) minmax(var(--pinw, 480px), 1fr); }",
		":root[data-pincol] .dbody { grid-template-columns: 340px minmax(0, var(--pbw)) minmax(var(--pinw, 480px), 1fr); }",
		":root[data-pincol] .dwrap:not(.narrow), :root[data-pincol] .dr-in { max-width: none; }",
		":root[data-pincol] .tk-drawer { --tkmaxw: none; }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
	if want := ".tk .tk-dr-in { position: relative; max-width: var(--tkmaxw, 1680px); margin: 0 auto 0 var(--tkml, auto); }"; !strings.Contains(mustAsset(t, "tracker.css"), want) {
		t.Errorf("tracker.css lacks %q", want)
	}
}

// TestEdgePinZoneFitsTheStripRightOfThePage pins the first-pin edge zone's
// width: it fills the strip right of the left-aligned 1680px page, at least
// 48px and at most 380px, so it leaves the board column to its own reorder
// drops; below 1780px, where that strip is narrow, its label runs
// vertically.
func TestEdgePinZoneFitsTheStripRightOfThePage(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	zone := ""
	for _, line := range strings.Split(css, "\n") {
		if strings.HasPrefix(line, ".bpzone {") {
			zone = line
		}
	}
	if want := "width: clamp(48px, calc(100vw - 1676px), 380px);"; !strings.Contains(zone, want) {
		t.Errorf("the .bpzone rule lacks %q: %q", want, zone)
	}
	if want := "@media (max-width: 1779.98px) { .bpzone { writing-mode: vertical-rl; padding: 12px 4px; } }"; !strings.Contains(css, want) {
		t.Errorf("dashboard.css lacks %q", want)
	}
}
