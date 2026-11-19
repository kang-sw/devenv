package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestSessionOverlayLayerScale(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, rule := range []string{
		"--z-cards: 1; --z-panels: 40; --z-popovers: 60; --z-modal: 70;",
		".doc-body.framed { position: relative; z-index: var(--z-cards); }",
		":root[data-shell][data-fpopover] .frames { z-index: var(--z-popovers); }",
		":root[data-shell][data-fdim] .frames { z-index: var(--z-modal); }",
		// A popover-lifted frame stays see-through so the shell header and rail
		// beneath it never vanish (the hover card and harness menu regression).
		":root[data-shell][data-fpopover]:not([data-fdim]) .frames > iframe.on { background: transparent; }",
		":root[data-frame][data-lifted], :root[data-frame][data-lifted] body, :root[data-frame][data-lifted] .dash { background: transparent; }",
	} {
		if !strings.Contains(css, rule) {
			t.Errorf("missing overlay layer rule: %s", rule)
		}
	}
}

// The mirrored header button must send search to the shown frame even when
// the click lands on its SVG. The frame-side receiver is covered separately.
func TestShellSearchButtonClick(t *testing.T) {
	js := mustAsset(t, "dashboard.js")
	search := regexp.MustCompile(`(?s)\n  function shSearch\(\) \{.*?\n  \}\n`).FindString(js)
	click := regexp.MustCompile(`(?s)    if \(shTop\) shTop.addEventListener\("click", function \(e\) \{.*?\n    \}\);`).FindString(js)
	if search == "" || click == "" {
		t.Fatal("dashboard.js holds no shell search click path")
	}
	if !strings.Contains(mustAsset(t, "tracker.js"), "openSearch: qOpen,") {
		t.Fatal("tracker API must open the palette directly")
	}
	runNode(t, "shell search click", `
var bad = [], handler, sent = [], focused = 0;
var shown = { el: { focus: function () { focused++; } } }, shOn = shown;
var shTop = { addEventListener: function (kind, fn) { handler = fn; } };
function shSend(ent, kind) { sent.push([ent === shown, kind]); }
`+search+click+`
function fire(selector) {
  var prevented = false;
  handler({ target: { closest: function (sel) { return sel === selector ? {} : null; } },
    preventDefault: function () { prevented = true; } });
  return prevented;
}
if (!fire("#tkqs") || focused !== 1 || JSON.stringify(sent) !== '[[true,"search"]]') bad.push("search icon click did not reach the shown frame");
if (fire(".unrelated") || sent.length !== 1) bad.push("unrelated header click opened search");
shOn = null;
fire("#tkqs");
if (sent.length !== 1) bad.push("search sent without a shown frame");
process.stdout.write(bad.join("\n"));
`)
}

// Session pages have #drawer/#shandle (sbody) and must omit #tkqs from #qsslot
// while still mounting WsTracker; index pages lack sbody and must mount #tkqs.
func TestSessionOmitsDuplicateSearchButtonAndIndexKeepsIt(t *testing.T) {
	paletteSearch := dashboardJSBlock(t, "\n  function paletteSearch(o) {", "\n  // tkSearch opens")
	runTrackerNode(t, "tkMount qsSlot routing", `
var bad = [];
function testMount(hasDrawer) {
  var mountedOpts = null, headerCls = [], opened = 0;
  var header = { classList: { add: function (c) { headerCls.push(c); } } };
  var slot = { id: 'qsslot' }, wrap = { querySelector: function () { return { id: 'dbody' }; } };
  var sbody = hasDrawer ? { id: 'sbody' } : null;
  var rid = '', tkSupport = false, tkAdapter = {}, tkApi = null, tkWantSearch = false, DR_SEARCH = 'dr.search';
  var root = { hasAttribute: function () { return false; } };
  var doc = { querySelector: function (s) { return s === '.dwrap > .top' ? header : s === '.dwrap' ? wrap : null; } };
  function $(id) { return id === 'qsslot' ? slot : null; }
  function load() { return null; }
  function save() {}
  function drMax() { return 500; }
  function tlTracker() {}
  function paletteRows() { return []; }
  function palettePick() {}
  function paletteFiles(o) { return o; }
  var window = { WsTracker: { mount: function (ad, opts) { mountedOpts = opts; return { openSearch: function () { opened++; } }; } } };
  `+paletteSearch+`
  tkMount();
  return { mountedOpts: mountedOpts, headerCls: headerCls };
}
var sess = testMount(true);
if (!sess.mountedOpts) bad.push("session tkMount skipped WsTracker.mount");
else {
  if (sess.mountedOpts.qsSlot !== null) bad.push("session tkMount passed qsSlot=" + sess.mountedOpts.qsSlot);
  if (!sess.mountedOpts.searchHost) bad.push("session tkMount omitted searchHost");
}
var idx = testMount(false);
if (!idx.mountedOpts) bad.push("index tkMount skipped WsTracker.mount");
else {
  if (!idx.mountedOpts.qsSlot || idx.mountedOpts.qsSlot.id !== 'qsslot') bad.push("index tkMount omitted qsSlot");
  if (idx.mountedOpts.searchHost) bad.push("index tkMount unexpectedly set searchHost");
}
process.stdout.write(bad.join("\n"));
`)

	trackerMount := mustAsset(t, "tracker.js")
	if !strings.Contains(trackerMount, `id="tki-search"`) {
		t.Fatal("tracker.js must keep #tki-search symbol definition")
	}
	if !strings.Contains(trackerMount, `if (O.qsSlot && !O.searchHost)`) {
		t.Fatal("tracker.js must guard #tkqs creation with O.qsSlot && !O.searchHost")
	}
}

func TestSearchTriggerBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	f := startPageFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "node", "testdata/search-trigger.browser.cjs", f.base).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("search trigger Chromium acceptance: %v", err)
	}
}
