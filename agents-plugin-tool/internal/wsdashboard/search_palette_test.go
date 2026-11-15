package wsdashboard

import (
	"os/exec"
	"strings"
	"testing"
)

func TestSearchPaletteAssets(t *testing.T) {
	tracker, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	page, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	css, err := dashboardFS.ReadFile("assets/tracker.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"class: 'tk tk-palette dr-search-palette'", "O.searchHost.setOpen(true)", "function qScore(", "function qBoard(",
		"function qShortcut(", "O.searchRows", "O.searchPick", "e.isComposing || e.keyCode === 229",
		"O.searchState.load()", "js.tk.qs.tracker",
		"O.fileFind", "O.fileList", "excerpts: true",
	} {
		if !strings.Contains(string(tracker), want) {
			t.Errorf("tracker lacks palette hook %q", want)
		}
	}
	hostCSS, err := dashboardTestAsset("assets/dashboard.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".dr-search-handle { right: calc(var(--dpad) + var(--shr, 0px))", ".dr-search .tk.dr-search-palette { position: static; width: 100%; height: 100%"} {
		if !strings.Contains(string(hostCSS), want) {
			t.Errorf("host search sheet CSS lacks %q", want)
		}
	}
	// The renderer stays host-agnostic: the file hooks come from the host.
	for _, page := range []string{"WsPreview", "/fs/"} {
		if strings.Contains(string(tracker), page) {
			t.Errorf("tracker reaches the page's %q itself", page)
		}
	}
	if strings.Contains(string(tracker), `class="tk-qpop"`) || strings.Contains(string(css), ".tk .tk-qpop") {
		t.Error("old quick-switcher popover remains")
	}
	for _, want := range []string{"searchRows: paletteRows", "searchPick: palettePick", "paletteFiles({", `"&excerpts=1"`, "parsed.content.querySelector", `p.open = true`, `target.scrollIntoView({ block: "center" })`} {
		if !strings.Contains(string(page), want) {
			t.Errorf("host lacks palette hook %q", want)
		}
	}
	if strings.Contains(string(tracker), "d.showModal()") || strings.Contains(string(css), ".tk.tk-palette::backdrop") {
		t.Error("search sheet must not create a modal or backdrop")
	}
	for _, want := range []string{`grid-template-areas: "board content tracker" "files content tnote"`, "[data-step=wide]", "prefers-reduced-motion: reduce", ".palette-flash { animation: none; }"} {
		if !strings.Contains(string(css), want) {
			t.Errorf("palette CSS lacks %q", want)
		}
	}
}

func TestSearchDrawerSplitOwnership(t *testing.T) {
	for _, tc := range []struct {
		logical string
		owner   string
		markers []string
	}{
		{"dashboard.js", "dashboard-drawer.js", []string{"function drApply(anim)", "var sbody = null, shandle = null;"}},
		{"dashboard.js", "dashboard-tracker.js", []string{"function paletteSearch(o)"}},
		{"dashboard.css", "dashboard-drawer.css", []string{".dr-search-handle {", ".dr-fold.search {"}},
	} {
		whole, err := dashboardTestAsset("assets/" + tc.logical)
		if err != nil {
			t.Fatal(err)
		}
		owner, err := dashboardFS.ReadFile("assets/" + tc.owner)
		if err != nil {
			t.Fatal(err)
		}
		opening, err := dashboardFS.ReadFile("assets/" + tc.logical)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range tc.markers {
			if strings.Count(string(whole), marker) != 1 || !strings.Contains(string(owner), marker) || strings.Contains(string(opening), marker) {
				t.Errorf("%q must occur once, in %s rather than the opening fragment", marker, tc.owner)
			}
		}
	}
}

func TestSearchPaletteChromeDeletions(t *testing.T) {
	tracker, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	css, err := dashboardFS.ReadFile("assets/tracker.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{"tkqscope", "tkqjump", "function qStrip(", "function qEnterJump(", "tkqanswer", "searchAsk", "searchAnswerMount", "js.search.askhint"} {
		if strings.Contains(string(tracker), removed) {
			t.Errorf("deleted search chrome/routing remains: %s", removed)
		}
	}
	for _, removed := range []string{".tk-qanswer", ".with-answer", ".tk-qcite", ".tk-qsources"} {
		if strings.Contains(string(css), removed) {
			t.Errorf("deleted answer CSS remains: %s", removed)
		}
	}
	page, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{"function searchAnswerClient(", "function searchCitation(", "/search-answers/", `key + ".history"`, `key + ".context"`} {
		if strings.Contains(string(page), removed) {
			t.Errorf("deleted search answer integration remains: %s", removed)
		}
	}
	if !strings.Contains(string(css), "font-size: 13px; height: 26px; padding: 3px 8px;") {
		t.Error("search input must retain the approved short row")
	}
}

func TestSearchPaletteHandleGeometry(t *testing.T) {
	room := dashboardJSBlock(t, "\n  function drRoom() {", "\n  // drFoldShow")
	runTrackerNode(t, "search handle geometry", `
var bad = [], props = {}, drHas = true, drRoomW = -1, drGhOff = -1, drFoldW = -1, drFoldGw = -1;
function check(label, got, want) { if (got !== want) bad.push(label + ': ' + got + ' != ' + want); }
var width = 178, git = 90, search = 35;
var drhandle = { getBoundingClientRect: function () { return { width: width }; } };
var gitRegion = { getBoundingClientRect: function () { return { width: git }; } };
var shandle = { getBoundingClientRect: function () { return { width: search }; } };
function ghandle() { return git ? {} : null; }
var drheader = { classList: { toggle: function () {} } };
var doc = { documentElement: { style: { setProperty: function (k, v) { props[k] = v; } } } };
`+room+`
[178, 80].forEach(function (w) {
  width = w; drRoom();
  check('search immediately left of panels', props['--shr'], (w + 6) + 'px');
  check('git left of search', props['--ghr'], (w + 6 + search + 6) + 'px');
  check('header reserves all three handles', props['--drhw'], (w + 6 + search + 6 + git) + 'px');
});
drHas = false; git = 0; drRoom();
check('search alone starts at the right edge', props['--shr'], '0px');
check('search alone reserves its width', props['--drhw'], '35px');
process.stdout.write(bad.join("\n"));
`)
}

func TestSearchPaletteBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run testdata/search-palette.test.cjs locally")
	}
	out, err := exec.Command(node, "testdata/search-palette.test.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("palette cases failed: %v\n%s", err, out)
	}
}
