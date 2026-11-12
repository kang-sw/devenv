package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

// pillPrelude is a minimal page for pill.js under node: a board host, three
// open-question items (the third marked sent), and a title already carrying
// a stale prefix.
const pillPrelude = `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function E() { this.children = []; this.style = {}; this.attrs = {}; this.cls = {}; var me = this; this.classList = { contains: function (c) { return !!me.cls[c]; } }; }
E.prototype.querySelector = function () { return null; };
E.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
E.prototype.setAttribute = function (k, v) { this.attrs[k] = String(v); };
E.prototype.hasAttribute = function (k) { return k in this.attrs; };
E.prototype.appendChild = function (c) { this.children.push(c); return c; };
E.prototype.insertBefore = function (c) { this.children.unshift(c); return c; };
Object.defineProperty(E.prototype, "firstChild", { get: function () { return this.children[0] || null; } });
E.prototype.addEventListener = function () {};
E.prototype.removeEventListener = function () {};
E.prototype.getBoundingClientRect = function () { return { top: 0, bottom: 0, width: 0, height: 0 }; };
function IntersectionObserver() { this.observe = function () {}; this.disconnect = function () {}; }
function getComputedStyle() { return { overflowY: "auto" }; }
function wsT(k) { return k; }
var host = new E(), items = [new E(), new E(), new E()];
items[2].cls["sent-sent"] = true;
var title = "(9) Board - x", html = new E();
var document = { body: new E(), documentElement: html, getElementById: function (id) { return id === "bcol" ? host : null; },
  querySelector: function () { return null; }, querySelectorAll: function () { return items; }, createElement: function () { return new E(); } };
Object.defineProperty(document, "title", { get: function () { return title; }, set: function (v) { title = v; } });
var window = { IntersectionObserver: IntersectionObserver, innerHeight: 800, addEventListener: function () {}, removeEventListener: function () {} };
`

// TestPillTitleDefersToTheDashboard pins the open-question pill's title
// write: a standalone page gets its own "(N) " prefix over the recorded base,
// and a page whose html element carries data-wb-titlehost (the dashboard)
// keeps its title, even on a run before the page's hook exists, while the
// count goes to window.wbTitleCount. The cases execute under node when it is
// installed.
func TestPillTitleDefersToTheDashboard(t *testing.T) {
	data, err := assetFS.ReadFile("assets/pill.js")
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; pill title cases not executed")
	}
	pill := string(data)
	for name, script := range map[string]string{
		"standalone": pillPrelude + pill + `
check("standalone: own prefix", title, "(2) Board - x");
check("standalone: base kept", html.getAttribute("data-wb-title"), "Board - x");
process.stdout.write(bad.join("\n"));`,
		"hosted": pillPrelude + `
html.attrs["data-wb-titlehost"] = "";
var got = [];
` + pill + `
check("hosted: no title write before the hook exists", [title, html.getAttribute("data-wb-title")], ["(9) Board - x", null]);
window.wbTitleCount = function (n) { got.push(n); };
` + pill + `
check("hosted: the count goes to the hook", got.slice(-1), [2]);
check("hosted: title untouched", title, "(9) Board - x");
process.stdout.write(bad.join("\n"));`,
	} {
		out, err := nodeScript(node, script).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: node: %v\n%s", name, err, out)
		}
		if s := strings.TrimSpace(string(out)); s != "" {
			t.Errorf("%s pill title cases failed:\n%s", name, s)
		}
	}
}

// TestPillCountsAnsweredCheckRows pins the pill's responded() for check
// rows: a staged "no" (the row's data-no) or a ticked check box answers the
// row, so the still-open count the pill hands the dashboard leaves it out.
// The cases execute under node when it is installed.
func TestPillCountsAnsweredCheckRows(t *testing.T) {
	data, err := assetFS.ReadFile("assets/pill.js")
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; pill check-row cases not executed")
	}
	script := pillPrelude + `
html.attrs["data-wb-titlehost"] = "";
var got = [];
window.wbTitleCount = function (n) { got.push(n); };
function row(state) {
  var e = new E(), box = { checked: state === "ticked" };
  e.querySelector = function (s) { return s === "label.ack > input, label.ck > input" ? box : null; };
  if (state === "no") e.attrs["data-no"] = "";
  return e;
}
items = [row("no"), row("ticked"), row(""), row("")];
` + string(data) + `
check("a staged no and a ticked box answer their rows", got.slice(-1), [2]);
process.stdout.write(bad.join("\n"));`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("pill check-row cases failed:\n%s", s)
	}
}
