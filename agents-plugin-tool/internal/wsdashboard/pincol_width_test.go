package wsdashboard

import (
	"strings"
	"testing"
)

// TestPinnedColumnFillsTheWindowRightOfTheBoard pins the wide layout: a
// session page stands at the window's left edge (the index page stays
// centred), and while the pinned column shows, the board keeps its no-pin
// width (--pbw, narrowed to leave the column 480px; --boardw once the user
// drags it, clamped to what the window leaves so a width saved on a wider
// window cannot push the column below its floor) and the column fills the
// remaining width to the window's right edge, 280px at least. From 1600px both drawers (the session drawer's
// .dr-in, the tracker's through --tkmaxw and --tkml) span the window, so
// their handles stand at its right edge whether or not anything is pinned,
// while the session drawer's sheet stays 960px at most, anchored right
// under them, clear of the session rail.
func TestPinnedColumnFillsTheWindowRightOfTheBoard(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		".dwrap { margin: 0; padding: 12px 16px; max-width: 1680px; }",
		".dwrap.narrow { margin: 0 auto; max-width: 1080px; padding-top: 16px; }",
		".dr-in { position: relative; max-width: 1680px; margin: 0; }",
		`@media (min-width: 1600px) {
  .dr-in { max-width: none; }
  .dash:not(.index) { --tkmaxw: none; }
  .dr-clip { left: auto; width: min(960px, calc(100% - 2 * var(--dpad))); }
}`,
		"@media (min-width: 1600px) { .dgrid.board { margin: 0; } }",
		".dash { --tkml: 0px; }",
		".dash.index { --tkmaxw: 1080px; --tkml: auto; }",
		":root { --pbw: calc(1680px - 32px - 356px); }",
		".dash.board { --pbw: 900px; }",
		":root[data-pincol] .dbody { grid-template-columns: minmax(0, min(var(--boardw, min(var(--pbw), 100% - 496px)), 100% - 296px)) minmax(280px, 1fr); }",
		":root[data-pincol] .dbody { grid-template-columns: 340px minmax(0, min(var(--boardw, min(var(--pbw), 100% - 852px)), 100% - 652px)) minmax(280px, 1fr); }",
		":root[data-pincol] .dwrap:not(.narrow), :root[data-pincol] .dgrid.board { max-width: none; }",
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
// 48px, so it leaves the board column to its own reorder drops; below
// 1780px, where that strip is narrow, its label runs vertically. From
// 1780px it stands there idle as the empty column's placeholder while
// nothing is pinned, the layout is not "tabs", and no store page shows.
func TestEdgePinZoneFitsTheStripRightOfThePage(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	zone := ""
	for _, line := range strings.Split(css, "\n") {
		if strings.HasPrefix(line, ".bpzone {") {
			zone = line
		}
	}
	if want := "width: max(48px, calc(100vw - 1696px));"; !strings.Contains(zone, want) {
		t.Errorf("the .bpzone rule lacks %q: %q", want, zone)
	}
	for _, want := range []string{
		"@media (max-width: 1779.98px) { .bpzone { writing-mode: vertical-rl; padding: 12px 4px; } }",
		`@media (min-width: 1780px) {
  :root:not([data-pincol]):not([data-bdrag]) body:not(.tk-paged):not(:has(#grid[data-layout="tabs"])) .bpzone { display: grid;`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
}
