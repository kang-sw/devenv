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
// while both sheets stay 1280px at most, anchored right under them.
func TestPinnedColumnFillsTheWindowRightOfTheBoard(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		// No top padding: the header's handles hang from the window's top edge.
		".dwrap { margin: 0; padding: 0 16px 12px; max-width: 1680px; }",
		".dwrap.narrow { margin: 0 auto; max-width: 1080px; padding-top: 16px; }",
		".dr-in { position: relative; max-width: 1680px; margin: 0; }",
		`@media (min-width: 1600px) {
  .dr-in { max-width: none; }
  .dash:not(.index) { --tkmaxw: none; }
  .dr-clip { left: auto; width: min(1280px, calc(100% - 2 * var(--dpad))); }
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
// nothing is pinned, the layout is not "tabs", and no store page shows
// (from 2200px the column itself shows instead: data-pincol hides it).
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
  :root:not([data-pincol]):not([data-bdrag]) body:not(.tk-paged):not([data-dashboard-layout="tabs"]) .bpzone { display: grid;`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
}

// TestPinnedColumnSlotFromTheRailDockedWidth pins the four-panel width:
// from 2200px, where the rail stays docked beside the pinned column, the
// page shows the column with nothing pinned too (dashboard.js
// pinColumnFits, asked again when the window crosses 2200px), so the
// column's board-width handle works before the first pin, and the column's
// empty pin list is the idle pin zone's quiet "drop here" placeholder, lit
// while a drag runs; below 2200px the column still waits for a pin.
func TestPinnedColumnSlotFromTheRailDockedWidth(t *testing.T) {
	js := mustAsset(t, "dashboard.js")
	for _, want := range []string{
		`var PIN_SLOT = window.matchMedia ? window.matchMedia("(min-width: 2200px)") : null;`,
		`return (n > 0 || !!(PIN_SLOT && PIN_SLOT.matches)) && !!(PIN_WIDE && PIN_WIDE.matches) && !(live && (grid.getAttribute("data-layout") || load(LAYOUT_KEY)) === "tabs");`,
		`[PIN_WIDE, PIN_SLOT].forEach(function (m) { if (m && m.addEventListener) m.addEventListener("change", function () { window.WsCards.relayout(); }); });`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("dashboard.js lacks %q", want)
		}
	}
	css := mustAsset(t, "dashboard.css")
	head := "@media (min-width: 2200px) {\n  /* rail and column both show */"
	at := strings.Index(css, head)
	if at < 0 {
		t.Fatalf("dashboard.css lacks %q", head)
	}
	block := css[at:]
	block = block[:strings.Index(block, "\n}")]
	for _, want := range []string{
		`  :root[data-pincol] body:not(.tk-paged) .bpcol > .bpins:empty { display: grid; place-items: center; padding: 12px; border: 2px dashed color-mix(in srgb, var(--wb-muted-fg) 40%, transparent);`,
		`  :root[data-pincol] .bpcol > .bpins:empty::before { content: attr(data-drop); }`,
		`  :root[data-pincol][data-bdrag] body:not(.tk-paged) .bpcol > .bpins:empty { border-color: var(--wb-primary);`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("the 2200px block lacks %q", want)
		}
	}
}

// TestConversationSplitHandle pins the live grid's conversation/board
// split: the handle (.dcv-edge, cards.js splitHandle) is mounted on the
// live grid only, shows only where the conversation and the board stand
// side by side (an explicit "split" from 760px, anything but "tabs" from
// 1100px), stands in the gap left of the board styled as the pinned
// column's handle, and a dragged width (--convow with data-convow) keeps
// the conversation 320px and the board 360px at least, while the default
// split stays the stylesheet's 5:7.
func TestConversationSplitHandle(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		".dgrid { --dgrid-cols: minmax(320px, 5fr) minmax(0, 7fr); --dgrid-gap: 16px; }",
		":root[data-convow] .dgrid.live { --dgrid-cols: minmax(320px, min(var(--convow), 100% - 360px - var(--dgrid-gap))) minmax(0, 1fr); }",
		".dgrid.live > .dcv-edge { display: none; grid-area: board; justify-self: start; position: relative; z-index: 2; width: 9px; margin-left: -12px; cursor: col-resize; touch-action: none; }",
		`  .dgrid.live[data-layout="split"] > .dcv-edge { display: block; }`,
		`  .dgrid.live:not([data-layout="tabs"]) > .dcv-edge { display: block; }`,
		`.bp-edge::after, .dcv-edge::after { content: "";`,
		".bp-edge:hover::after, .bp-edge.drag::after, .dcv-edge:hover::after, .dcv-edge.drag::after { background: var(--wb-primary); }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
	if want := `split: grid && grid.classList.contains("live") ? grid : null,`; !strings.Contains(mustAsset(t, "dashboard.js"), want) {
		t.Errorf("dashboard.js lacks %q", want)
	}
	if want := "CONVW_MIN = 320, SPLITB_MIN = 360;"; !strings.Contains(mustAsset(t, "cards.js"), want) {
		t.Errorf("cards.js lacks %q: its clamps follow the stylesheet's", want)
	}
}

// TestLiveCellIsTheConversationAndTheBoard pins the push layout beside the
// pinned column on the live page: once a board width is set (data-boardw,
// the board's own column there, cards.js TestColumnGrabsPush), the board's
// cell is the conversation's width (--cvw: --convow, else the 5:7 share of
// the board), the grid's 16px gap and the board's, clamped as the cell was,
// so each handle moves only the panel on its left and the column takes the
// difference; the grid gives the conversation its width and the board the
// rest. Other pages keep --boardw as the whole cell.
func TestLiveCellIsTheConversationAndTheBoard(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{
		".dbody:has(> .dgrid.live) { --cvw: var(--convow, calc(var(--boardw) * 5 / 7)); }",
		`  :root[data-pincol][data-boardw] .dbody:has(> .dgrid.live) { grid-template-columns: minmax(0, min(var(--cvw) + 16px + var(--boardw), 100% - 296px)) minmax(280px, 1fr); }`,
		`  :root[data-pincol][data-boardw] .dbody:has(> .dgrid.live) { grid-template-columns: 340px minmax(0, min(var(--cvw) + 16px + var(--boardw), 100% - 652px)) minmax(280px, 1fr); }`,
		":root[data-pincol][data-boardw] .dgrid.live { --dgrid-cols: minmax(320px, min(var(--cvw), 100% - 360px - var(--dgrid-gap))) minmax(0, 1fr); }",
		".dgrid { --dgrid-cols: minmax(320px, 5fr) minmax(0, 7fr); --dgrid-gap: 16px; }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
}

// TestTrackerStopsAtItsWidestLayout pins the tracker's wide cap: from 1600px,
// where it lays out layout D (Items at two shares, Notifications, the tab
// pane: four panels' worth), its drawer sheet is 1600px at most, anchored
// right under the handle row like the session drawer's, and the store page is
// 1600px at most, left in its cell.
func TestTrackerStopsAtItsWidestLayout(t *testing.T) {
	if want := `@media (min-width: 1600px) {
  .tk .tk-dr-clip { left: auto; width: min(1600px, calc(100% - 2 * var(--dpad, 16px))); }
  .tk.tk-spage { max-width: 1600px; }
}`; !strings.Contains(mustAsset(t, "tracker.css"), want) {
		t.Errorf("tracker.css lacks %q", want)
	}
	if want := "function layoutOf() { var w = window.innerWidth; return w >= 1600 ? 'd'"; !strings.Contains(mustAsset(t, "tracker.js"), want) {
		t.Errorf("tracker.js lacks %q: the cap's 1600px threshold follows layout D's", want)
	}
}
