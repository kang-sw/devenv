package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

func TestSessionOverlayLayerScale(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, rule := range []string{
		"--z-cards: 1; --z-panels: 40; --z-popovers: 60; --z-modal: 70;",
		".doc-body.framed { position: relative; z-index: var(--z-cards); }",
		":root[data-shell][data-fpopover] .frames { z-index: var(--z-popovers); }",
		":root[data-shell][data-fdim] .frames { z-index: var(--z-modal); }",
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
