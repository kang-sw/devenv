package wsdashboard

import (
	"strings"
	"testing"
)

// TestComposerGrowFitsThePlaceholder runs grow() and placeholderHeight()
// from dashboard.js under node with a stubbed layout: an empty message box
// is as tall as its wrapped placeholder (its own scrollHeight does not count
// it), never past the 160px cap, and a box with text sizes to the text
// alone. Real layout is checked in a browser (the ticket's verification);
// this pins the decision logic and the wiring that calls it.
func TestComposerGrowFitsThePlaceholder(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	fns := composerFunc(t, src, "placeholderHeight") + composerFunc(t, src, "grow")
	runNode(t, "composer grow", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var measureH = 55, made = 0, chl = null;
var doc = { createElement: function () { made++; var el = { style: {}, className: "", offsetHeight: 0, setAttribute: function () {}, set textContent(v) { this._t = v; this.offsetHeight = measureH; } }; return el; } };
var host = { appended: [], appendChild: function (e) { this.appended.push(e); } };
var ctext = { value: "", scrollHeight: 38, style: {}, parentNode: host, getAttribute: function () { return ctext.ph; }, ph: "a long wrapped placeholder" };
var cmeasure = null;
`+fns+`
grow();
check("an empty box takes its placeholder's height", ctext.style.height, "55px");
check("the measuring layer is a .chl copy, made once", [made, host.appended[0].className], [1, "chl chm"]);
measureH = 500; grow();
check("the placeholder height is capped at 160px", ctext.style.height, "160px");
check("the measuring layer is reused", made, 1);
measureH = 55; ctext.value = "x"; ctext.scrollHeight = 94; grow();
check("a box with text sizes to its text", ctext.style.height, "94px");
ctext.value = ""; ctext.scrollHeight = 38; ctext.ph = ""; grow();
check("an empty box without a placeholder keeps its own height", ctext.style.height, "38px");
process.stdout.write(bad.join("\n"));
`)
}

// TestComposerRegrowsOnPlaceholderAndWidth pins the sites that keep an empty
// box fitted: a placeholder swap (setInput, renderCmd), and a width-only
// change through a ResizeObserver that ignores height-only changes, so the
// height grow() sets cannot loop.
func TestComposerRegrowsOnPlaceholderAndWidth(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		"t.placeholder = HINT[inputMode]; growCtext();",
		`wsT("js.cmd.args", cmdSel.name) : ctextHint);` + "\n    grow();",
		"if (w === ctextW) return;",
		"doc.fonts.ready.then(growCtext)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("dashboard.js lacks %q", want)
		}
	}
	css, err := dashboardFS.ReadFile("assets/dashboard.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), ".chl.chm {") {
		t.Error("dashboard.css lacks the .chl.chm measuring rule")
	}
}
