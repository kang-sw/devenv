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
		".dash { --tkml: 0px; }",
		".dash.index { --tkmaxw: 1080px; --tkml: auto; }",
		":root { --pbw: calc(1680px - 32px - 356px); }",
		".dash { --stagew: var(--pbw); }",
		".dash.board { --pbw: 900px; }",
		`:root:is([data-gal="col"], [data-gal="empty"], [data-gal="drag"]) .dbody { grid-template-columns: 340px minmax(0, min(var(--boardw, min(var(--pbw), 100% - 852px)), 100% - 652px)) minmax(280px, 1fr); }`,
		`:root:is([data-gal="col"], [data-gal="empty"], [data-gal="drag"]) .dwrap:not(.narrow),`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
	if want := ".tk .tk-dr-in { position: relative; max-width: var(--tkmaxw, 1680px); margin: 0 auto 0 var(--tkml, auto); }"; !strings.Contains(mustAsset(t, "tracker.css"), want) {
		t.Errorf("tracker.css lacks %q", want)
	}
}

// TestGalleryNoReservedColumnWithoutAPage pins the 4-band docking gallery CSS:
// Bands 1-3 (< 1600px, tabs, or >= 1600px without enough spare width for the
// 4th column) keep the gallery fused over #bcol (data-gal="narrow") while the
// rail stays permanently docked from 1600px; Band 4 activates the 3rd grid track
// (rail + stage + gallery column) with data-gal="empty", "drag", or "col",
// gating .bp-edge on [data-pincol].
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
		`  :root[data-pincol] .bgal > .bp-edge { display: block; }`,
		`  :root:is([data-gal="col"], [data-gal="empty"], [data-gal="drag"]) .bgal:not([hidden]) { position: relative; z-index: auto; top: auto; bottom: auto; right: auto; width: auto; grid-column: 3; grid-row: 1; }`,
		`  :root:is([data-gal="col"], [data-gal="empty"], [data-gal="drag"]) .dash:not(.live):not(.history) .bgal:not([hidden]) { position: sticky; top: 12px; height: calc(100vh - 24px); height: calc(100dvh - 24px); }`,
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

// TestGalleryLayoutModes runs the dynamic 4-band layout mapping (cards.js
// canFitFourCol + dashboard.js galMode + layout hook over WsGallery.layout)
// under node: Bands 1-3 keep narrow/fused gallery (no overlay rail from
// 1600px); Band 4 opens the 4th column iff !tabs && vw >= 1600 &&
// spareRight >= boardW (e.g. >= 2441px on default 5:7 live pages, >= 2204px
// on 900px board-first pages).
func TestGalleryLayoutModes(t *testing.T) {
	js := mustAsset(t, "dashboard.js")
	cardsJS := mustAsset(t, "cards.js")
	mode := regexp.MustCompile(`(?s)\n  function galMode\(\) \{.*?\n  \}\n`).FindString(js)
	hook := regexp.MustCompile(`(?s)\n      layout: function \(s\) \{.*?\n      \},\n`).FindString(js)
	canFit := regexp.MustCompile(`(?s)\n  function canFitFourCol\(vw, tabs\) \{.*?\n  \}\n`).FindString(cardsJS)
	if mode == "" || hook == "" || canFit == "" {
		t.Fatal("dashboard.js/cards.js holds no galMode / layout / canFitFourCol")
	}
	for _, want := range []string{
		`if (PIN_WIDE && PIN_WIDE.addEventListener) PIN_WIDE.addEventListener("change", function () { window.WsCards.relayout(); });`,
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
var CONVW_MIN = 320, SPLITB_MIN = 360, boardw = 0, convow = 0, board = null, opt = { split: {} };
function load() { return layoutKey; }
var galNarrow = false;
var window = { get innerWidth() { return width; } };
function railOpen() { rails++; }
function placeGallery() { placed++; }
` + canFit + mode + `
var optLayout = { ` + hook + ` };
function at(px, pages, dragging, tabs) {
  width = px; layoutKey = tabs ? "tabs" : "auto";
  var m = galMode(), s = G.layout({ width: m.width, tabs: m.tabs, fourCol: canFitFourCol(m.width, m.tabs), pages: pages, dragging: dragging });
  s.pages = pages; s.dragging = dragging;
  optLayout.layout(s);
  return [attrs["data-gal"] || null, "data-pincol" in attrs, s.overlay];
}
check("Band 1 1599px, no page: no gallery", at(1599, 0, false), [null, false, false]);
check("Band 1 1599px, a page: the narrow view", at(1599, 1, false), ["narrow", false, false]);
check("Band 1 1599px, a drag with no page: the narrow rail", at(1599, 0, true), ["narrow", false, false]);
check("Band 2 1640px, a page: fused narrow, no overlay rail", at(1640, 1, false), ["narrow", false, false]);
check("Band 3 1920px, a page: fused narrow, no overlay rail", at(1920, 1, false), ["narrow", false, false]);
check("Band 3 2440px live default, a page: still Band 3 (744 < 744.33)", at(2440, 1, false), ["narrow", false, false]);
check("Band 4 2441px live default, no page: idle empty slot in Col 3", at(2441, 0, false), ["empty", false, false]);
check("Band 4 2560px live default, a page: 4-col gallery with rail docked", at(2560, 1, false), ["col", true, false]);
check("Band 4 2560px, a drag with no page: drag slot in Col 3, no pincol", at(2560, 0, true), ["drag", false, false]);
opt.split = null;
check("Band 3 2203px board-first, no page: still fused narrow", at(2203, 0, false), [null, false, false]);
check("Band 4 2204px board-first, no page: idle empty slot in Col 3", at(2204, 0, false), ["empty", false, false]);
board = { closest: function (selector) { return selector === ".dgrid.worktree" ? {} : null; } };
check("worktree 2204px keeps gallery fused while its 744px board would not fit", at(2204, 1, false), ["narrow", false, false]);
check("worktree 2441px fits the board beside its gallery", at(2441, 1, false), ["col", true, false]);
board = null;
opt.split = {};
check("tabs at 2560px, a page: the narrow view", at(2560, 1, false, true), ["narrow", false, false]);
rails = 0; at(1920, 1, false); at(2560, 1, false); at(1920, 1, false);
check("each pincol transition closes the session rail overlay once", rails, 2);
placed = 0; at(1920, 1, false);
check("Band 3 places the gallery over the board column", [placed, galNarrow], [1, true]);
at(2560, 1, false);
check("Band 4 stops placing the narrow overlay", galNarrow, false);
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
		".dash:has(> .dwrap .dgrid:is(.live, .history)) { --cvw: var(--convow, calc(var(--boardw) * 5 / 7)); }",
		`  :root[data-pincol][data-boardw]:is([data-gal="col"], [data-gal="empty"], [data-gal="drag"]) .dbody:has(> .dgrid:is(.live, .history)) { grid-template-columns: 340px minmax(0, min(var(--cvw) + 16px + var(--boardw), 100% - 652px)) minmax(280px, 1fr); }`,
		":root[data-boardw] .dgrid:is(.live, .history) { --dgrid-cols: minmax(320px, min(var(--cvw), 100% - 360px - var(--dgrid-gap))) minmax(0, 1fr); }",
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
