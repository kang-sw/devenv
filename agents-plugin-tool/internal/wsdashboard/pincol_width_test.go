package wsdashboard

import (
	"encoding/json"
	"regexp"
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
		`:root:is([data-pincol], [data-gal="empty"], [data-gal="drag"]) .dbody { grid-template-columns: 340px minmax(0, min(var(--boardw, min(var(--pbw), 100% - 852px)), 100% - 652px)) minmax(280px, 1fr); }`,
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

// TestGalleryNoReservedColumnWithoutAPage pins the docking gallery's wide
// layout CSS: from 1600–2199px the gallery is the page body's last column
// only while the page marks one (data-pincol, with a docked page); a drag
// alone shows it over the right edge (data-gal="drag", >= 360px wide).
// From 2200px, when 0 pages are docked at rest, the idle dashed drop slot
// (data-gal="empty") occupies Col 3 beside the docked rail and stage, with
// .bp-edge still gated to [data-pincol] and zero layout jump when dragging
// begins (data-gal="drag" stays in Col 3). Below 1600px or in tabs,
// data-gal="narrow" lays it fixed over the board's column.
func TestGalleryNoReservedColumnWithoutAPage(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, gone := range []string{".bpzone", ".bpins", ".bpcol", "data-drop", "attr(data-label)"} {
		if strings.Contains(css, gone) {
			t.Errorf("dashboard.css still carries the pin stack's %q", gone)
		}
	}
	for _, want := range []string{
		".bgal { display: none; }",
		":root[data-gal] .bgal:not([hidden]) { display: flex; gap: 6px; min-width: 0; min-height: 0; }",
		".bgal > .bp-edge { display: none; }",
		`:root[data-gal="narrow"] .bgal:not([hidden]) { position: fixed; z-index: 35; left: var(--gal-left, 0px); top: var(--gal-top, 0px); width: var(--gal-width, 100vw); height: var(--gal-height, 100vh); pointer-events: none; }`,
		`:root[data-gal="drag"] .bgal:not([hidden]) { position: fixed; z-index: 40; top: 50px; bottom: 12px; right: 16px; width: clamp(360px, calc(100vw - 1696px), 520px); }`,
		"  :root[data-pincol] .bgal:not([hidden]) { position: relative; grid-row: 1; grid-column: 2; }",
		"  :root[data-pincol] .bgal > .bp-edge { display: block; }",
		`  :root:is([data-pincol], [data-gal="empty"], [data-gal="drag"]) .bgal:not([hidden]) { position: relative; z-index: auto; top: auto; bottom: auto; right: auto; width: auto; grid-column: 3; grid-row: 1; }`,
		`  :root:is([data-pincol], [data-gal="empty"], [data-gal="drag"]) .dash:not(.live):not(.history) .bgal:not([hidden]) { position: sticky; top: 12px; height: calc(100vh - 24px); height: calc(100dvh - 24px); }`,
		// Inactive pages leave the clipped viewport by transform alone, a gap
		// past its edge (an edge-adjacent box still counts as intersecting).
		".gal-viewport { flex: 1 1 0; min-width: 0; min-height: 0; position: relative; overflow: hidden; overflow: clip; }",
		".gal-page { position: absolute; inset: 0; display: flex; min-width: 0; min-height: 0; transform: translateY(calc(var(--gal-off, 0) * (100% + 24px))); }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
	at := strings.Index(css, "/* ===== paged docking gallery")
	end := strings.Index(css, ".rail .rhd {")
	if at < 0 || end < at {
		t.Fatal("dashboard.css lacks the gallery block")
	}
	// No rule whose subject is the gallery, a page, a split or a pane hides
	// it by display, visibility, opacity or content-visibility (the empty
	// gallery's own display: none applies only with nothing docked, where
	// the page sets no data-gal).
	block := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(css[at:end], "")
	for _, m := range regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`).FindAllStringSubmatch(block, -1) {
		sel, decls := strings.TrimSpace(m[1]), m[2]
		if sel == ".bgal" {
			continue
		}
		for _, one := range strings.Split(sel, ",") {
			parts := strings.Fields(one)
			subject := parts[len(parts)-1]
			if !regexp.MustCompile(`^(\.bgal|\.gal-(page|viewport|split|pane))\b`).MatchString(subject) {
				continue
			}
			for _, banned := range []string{"content-visibility", "visibility: hidden", "opacity: 0", "display: none"} {
				if strings.Contains(decls, banned) {
					t.Errorf("a gallery rule hides with %q: %s", banned, sel)
				}
			}
		}
	}
}

// TestGalleryLayoutModes runs the page's layout mapping (dashboard.js
// galMode and the gallery's layout hook) over WsGallery.layout under node:
// below 1600px, or with the "tabs" layout at any width, the gallery is the
// narrow view over the board's column (shown only with a docked page or a
// drag); from 1600px a docked page makes it a column (data-pincol, which
// below 2200px also turns the session rail into its overlay); with no
// docked page at 1600–2199px no column is reserved and a drag alone shows
// it over the right edge (data-gal="drag"); from 2200px with no docked
// page at rest, the idle empty slot shows in Col 3 (data-gal="empty",
// without data-pincol). Crossing 1600px changes the layout only.
func TestGalleryLayoutModes(t *testing.T) {
	js := mustAsset(t, "dashboard.js")
	mode := regexp.MustCompile(`(?s)\n  function galMode\(\) \{.*?\n  \}\n`).FindString(js)
	hook := regexp.MustCompile(`(?s)\n      layout: function \(s\) \{.*?\n      \},\n`).FindString(js)
	if mode == "" || hook == "" {
		t.Fatal("dashboard.js holds no galMode / gallery layout hook")
	}
	for _, want := range []string{
		`var PIN_SLOT = window.matchMedia ? window.matchMedia("(min-width: 2200px)") : null;`,
		`[PIN_WIDE, PIN_SLOT].forEach(function (m) { if (m && m.addEventListener) m.addEventListener("change", function () { window.WsCards.relayout(); }); });`,
		"host: dbody || doc.body, mode: galMode,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("dashboard.js lacks %q", want)
		}
	}
	gallery, _ := json.Marshal(mustAsset(t, "gallery.js"))
	hook = strings.TrimSuffix(strings.TrimSpace(hook), ",")
	runNode(t, "gallery layout modes", "var GALLERY = "+string(gallery)+";\n"+`
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var w = {}; new Function("window", "module", GALLERY)(w, undefined); var G = w.WsGallery;
var attrs = {}, rails = 0, placed = 0, width = 0, layoutKey = "auto";
var root = { hasAttribute: function (k) { return k in attrs; }, setAttribute: function (k, v) { attrs[k] = v; }, removeAttribute: function (k) { delete attrs[k]; } };
var grid = { getAttribute: function () { return layoutKey; } }, live = true, LAYOUT_KEY = "layout";
function load() { return layoutKey; }
function media(min) { return { get matches() { return width >= min; } }; }
var PIN_WIDE = media(1600), PIN_SLOT = media(2200), galNarrow = false;
var window = { get innerWidth() { return width; } };
function railOpen() { rails++; }
function placeGallery() { placed++; }
` + mode + `
var opt = { ` + hook + ` };
function at(px, pages, dragging, tabs) {
  width = px; layoutKey = tabs ? "tabs" : "auto";
  var m = galMode(), s = G.layout({ width: m.width, tabs: m.tabs, pages: pages, dragging: dragging });
  s.pages = pages; s.dragging = dragging;
  opt.layout(s);
  return [attrs["data-gal"] || null, "data-pincol" in attrs, s.overlay];
}
check("1599px, no page: no gallery", at(1599, 0, false), [null, false, false]);
check("1599px, a page: the narrow view", at(1599, 1, false), ["narrow", false, false]);
check("1599px, a drag with no page: the narrow rail", at(1599, 0, true), ["narrow", false, false]);
check("1600px, a page: a column, the session rail overlaying", at(1600, 1, false), ["col", true, true]);
check("2199px, a page: still the overlay", at(2199, 1, false), ["col", true, true]);
check("2200px, a page: a column beside the docked rail", at(2200, 1, false), ["col", true, false]);
check("2200px, no page: idle empty slot in Col 3", at(2200, 0, false), ["empty", false, false]);
check("2600px, no page: idle empty slot in Col 3", at(2600, 0, false), ["empty", false, false]);
check("1800px, no page: no reserved column", at(1800, 0, false), [null, false, false]);
check("2200px, a drag with no page: drag slot in Col 3, no pincol", at(2200, 0, true), ["drag", false, false]);
check("1800px, a drag with no page: over the right edge, the rail docked", at(1800, 0, true), ["drag", false, false]);
check("tabs at 2600px, a page: the narrow view", at(2600, 1, false, true), ["narrow", false, false]);
check("galMode holds the width to each media query's side", [(width = 1599.5, galMode().width < 1600), (width = 2199.9, galMode().width)], [true, 2199]);
rails = 0; at(1599, 1, false); at(1600, 1, false); at(1599, 1, false);
check("each column change closes the session rail overlay once", rails, 2);
placed = 0; at(1599, 1, false);
check("the narrow view places the gallery over the board's column", [placed, galNarrow], [1, true]);
at(1600, 1, false);
check("the wide view stops placing it", galNarrow, false);
process.stdout.write(bad.join("\n"));
`)
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
