package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

// tagTitlePrelude is a minimal page for form.js under node: no controls, a
// document that records its listeners, and a wsT that counts its calls.
const tagTitlePrelude = `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function E(cls) { var me = this; this.attrs = {}; this.cls = cls || []; this.classList = { contains: function (c) { return me.cls.indexOf(c) >= 0; } }; }
E.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
E.prototype.setAttribute = function (k, v) { this.attrs[k] = String(v); };
E.prototype.hasAttribute = function (k) { return k in this.attrs; };
var calls = 0, listeners = {};
function wsT(k) { calls++; return "T(" + k + ")"; }
var document = { getElementById: function () { return null; }, querySelectorAll: function () { return []; },
  addEventListener: function (type, fn) { (listeners[type] = listeners[type] || []).push(fn); }, documentElement: new E() };
var window = { wsT: wsT };
function hover(el) { (listeners.mouseover || []).forEach(function (fn) { fn({ target: el }); }); }
`

// TestClassTagTitleOnHover pins the class tag tooltip: the first hover of a
// tag sets its title from the client table, a later hover leaves it alone,
// a non-class element gets nothing, and a second run of the script (a
// dashboard board swap) adds no second handler. The cases execute under
// node when it is installed.
func TestClassTagTitleOnHover(t *testing.T) {
	data, err := assetFS.ReadFile("assets/form.js")
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; class tag title cases not executed")
	}
	form := "(function () {\n" + string(data) + "})();\n"
	script := tagTitlePrelude + form + form + `
check("one handler across runs", (listeners.mouseover || []).length, 1);
var pol = new E(["tag", "policy"]), tech = new E(["tag", "technical"]), ann = new E(["tag", "announced"]);
hover(pol); hover(tech); hover(ann);
check("policy title", pol.getAttribute("title"), "T(js.wb.tag.policy)");
check("technical title", tech.getAttribute("title"), "T(js.wb.tag.technical)");
check("announced title", ann.getAttribute("title"), "T(js.wb.tag.announced)");
var n = calls;
hover(pol);
check("a later hover looks nothing up", calls, n);
var other = new E(["tag"]), plain = new E(["policy"]);
hover(other); hover(plain);
check("no title off a class tag", [other.hasAttribute("title"), plain.hasAttribute("title")], [false, false]);
process.stdout.write(bad.join("\n"));`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("class tag title cases failed:\n%s", s)
	}
}

// TestClassTagRendersWithoutTitle pins that the explanation is not in the
// rendered markup: the tag carries its class alone, and the script adds the
// title on hover.
func TestClassTagRendersWithoutTitle(t *testing.T) {
	for name, layout := range layouts() {
		out := render(t, sampleView(layout))
		for _, want := range []string{`<span class="tag technical">technical</span>`, `<span class="tag announced">announced</span>`} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: missing %s", name, want)
			}
		}
	}
}
