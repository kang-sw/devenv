package wsdashboard

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
	"golang.org/x/net/html"
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
		"O.searchState.load()", "O.searchAsk", "js.tk.qs.tracker",
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
	if strings.Index(string(tracker), `id="tkqanswer"`) > strings.Index(string(tracker), `class="tk-qresults"`) {
		t.Error("layout A must put the answer before the five result cells")
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
	for _, want := range []string{".tk .tk-qmain.with-answer { grid-template-columns: minmax(0, 1.12fr) minmax(0, 2fr); }", `grid-template-areas: "board content tracker" "files content tnote"`, "[data-step=wide]", "prefers-reduced-motion: reduce", ".palette-flash { animation: none; }"} {
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
		{"dashboard.js", "dashboard-tracker.js", []string{"function searchAnswerClient(io)", "function paletteSearch(o)"}},
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
	for _, removed := range []string{"tkqscope", "tkqjump", "function qStrip(", "function qEnterJump("} {
		if strings.Contains(string(tracker), removed) {
			t.Errorf("deleted search chrome/routing remains: %s", removed)
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

func TestSearchAnswerDrawerBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run testdata/search-answer.test.cjs locally")
	}
	out, err := exec.Command(node, "testdata/search-answer.test.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("answer drawer cases failed: %v\n%s", err, out)
	}
}

// This adapter fixture parses production templates without starting a browser or
// daemon. Native disclosure/scroll geometry remains a separate acceptance check.
func TestSearchPaletteRenderedEntries(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	state := &wswhiteboard.State{}
	apply := func(commands string) {
		t.Helper()
		_, problems := wswhiteboard.Apply(state, commands, wswhiteboard.Env{Now: time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)})
		if len(problems) != 0 {
			t.Fatal(problems)
		}
	}
	apply(`<wb:topic name="stream"><wb:log>Older-page needle</wb:log><wb:state value="waiting"/><wb:state value="discussing"/></wb:topic>`)
	for i := 0; i < wswhiteboard.HistoryWindow+8; i++ {
		apply(`<wb:topic name="stream"><wb:log>Padding log line</wb:log></wb:topic>`)
	}
	apply(`<wb:topic name="stream"><wb:panel recap="Earlier panel recap"><p>Historical payload needle</p></wb:panel><wb:panel id="p1" recap="Latest panel recap"><p>Latest payload needle</p><script>forbiddenScriptNeedle()</script></wb:panel><wb:log>Live log needle</wb:log></wb:topic>`)
	apply(`<wb:topic name="body"><wb:body><p>Topic-owned body needle</p></wb:body></wb:topic>`)
	regions, err := wswhiteboard.RenderRegions(wswhiteboard.BuildView(state, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, Location: time.UTC}))
	if err != nil {
		t.Fatal(err)
	}
	type domNode struct {
		Tag      string            `json:"tag,omitempty"`
		Text     string            `json:"text,omitempty"`
		Attrs    map[string]string `json:"attrs,omitempty"`
		Children []*domNode        `json:"children,omitempty"`
		Content  []*domNode        `json:"content,omitempty"`
	}
	frames := map[string]*domNode{}
	var convert func(*html.Node) *domNode
	var parse func(string) *domNode
	parse = func(source string) *domNode {
		n, err := html.Parse(strings.NewReader(source))
		if err != nil {
			t.Fatal(err)
		}
		return convert(n)
	}
	convert = func(n *html.Node) *domNode {
		out := &domNode{}
		if n.Type == html.TextNode {
			out.Text = n.Data
		} else if n.Type == html.ElementNode {
			out.Tag, out.Attrs = n.Data, map[string]string{}
			for _, a := range n.Attr {
				out.Attrs[a.Key] = a.Val
				if (a.Key == "data-srcdoc" || a.Key == "srcdoc") && frames[a.Val] == nil {
					frames[a.Val] = &domNode{}
					frames[a.Val] = parse(a.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.CommentNode || c.Type == html.DoctypeNode {
				continue
			}
			if n.Data == "template" {
				out.Content = append(out.Content, convert(c))
			} else {
				out.Children = append(out.Children, convert(c))
			}
		}
		return out
	}
	fixture := struct {
		Root   *domNode            `json:"root"`
		Frames map[string]*domNode `json:"frames"`
	}{parse(string(regions.Board)), frames}
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "board-dom.json")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, "testdata/search-palette.test.cjs", file).CombinedOutput()
	if err != nil {
		t.Fatalf("rendered entry cases failed: %v\n%s", err, out)
	}
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
