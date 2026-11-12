package wswhiteboard

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// codeBody is a topic body with a three-line block, a one-line block, and a
// diagram, whose code carries digits a copy must not mix with line numbers.
const codeBody = "intro\n\n```go\nx := 1\ny := 2\nz := 3\n```\n\n```\none line\n```\n\n```mermaid\ngraph TD; A-->B\n```\n"

// TestCodeBlocksScriptReachesEveryPageAndFrame pins where code.js runs: the
// full page and the embed (after form.js, which defines wbBindBoard), the
// dashboard's shared form script, and a served topic-body frame, whose
// document cannot reach the host page's copy.
func TestCodeBlocksScriptReachesEveryPageAndFrame(t *testing.T) {
	code, err := embeddedAsset("code.js")
	if err != nil {
		t.Fatal(err)
	}
	form, err := embeddedAsset("form.js")
	if err != nil {
		t.Fatal(err)
	}
	assets, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	// The scripts are inlined in the page, where html/template escapes them
	// for the script context, so the check is on the unescaped asset text.
	if !strings.Contains(assets.docFrame, string(code)) {
		t.Errorf("a topic-body frame does not run code.js")
	}
	shared, err := PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(shared.FormScript), string(form)+"\n"+string(code)) {
		t.Errorf("the dashboard's shared form script does not run code.js right after form.js")
	}
	for name, l := range layouts() {
		newPage := newPageData(BuildView(&State{}, ViewInput{Layout: l, ReplyID: "r"}), assets, false)
		if !strings.Contains(string(newPage.Script), string(form)+string(code)) {
			t.Errorf("%s: the page script does not run code.js right after form.js", name)
		}
	}
}

// TestCodeBlocksGetNumbersAndCopyUnderNode runs code.js under node over a
// rendered topic body: a multi-line block gets a gutter with one number per
// line, a one-line block gets none, both get a copy button, a diagram is left
// alone, a second run wires nothing twice, the copy action takes the code
// text without numbers and shows the done label before returning, both in the
// page's language, and a board bound later (wbBindBoard) is wired too, with
// the English fallback where the page has no wsT.
func TestCodeBlocksGetNumbersAndCopyUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; code block cases not executed")
	}
	inner, _ := docBody(t, codeBody, ViewInput{Layout: LayoutFull})
	board, _ := json.Marshal(`<div class="doc-body md">` + inner + `</div>`)
	code, err := embeddedAsset("code.js")
	if err != nil {
		t.Fatal(err)
	}
	script := "var BOARD = " + string(board) + ";\n" + formPrelude + string(wsi18n.SubsetScript("ko", codeKeyPrefix)) + `
// A page's wsT is a global as well as window.wsT.
var wsT = window.wsT;
// What code.js needs beyond the form's mini DOM: building and moving nodes,
// a body to hang the scratch textarea on, and a copy that records what the
// textarea held.
Object.defineProperty(E.prototype, "className", { get: function () { return this.attrs["class"] || ""; }, set: function (v) { this.attrs["class"] = v; } });
Object.defineProperty(E.prototype, "parentNode", { get: function () { return this.parent; } });
Object.defineProperty(E.prototype, "innerHTML", { get: function () { return this.html || ""; }, set: function (v) { this.html = v; this.children = []; this.textContent = ""; } });
E.prototype.appendChild = function (c) { if (c.parent) c.parent.removeChild(c); c.parent = this; this.children.push(c); return c; };
E.prototype.removeChild = function (c) { this.children.splice(this.children.indexOf(c), 1); c.parent = null; return c; };
E.prototype.insertBefore = function (c, ref) { if (c.parent) c.parent.removeChild(c); c.parent = this; this.children.splice(this.children.indexOf(ref), 0, c); return c; };
document.createElement = function (tag) { return new E(tag); };
document.body = new E("body");
var copied = [], copyOK = true;
document.execCommand = function (cmd) { if (cmd !== "copy") return false; var ta = document.body.children[document.body.children.length - 1]; copied.push(ta && ta.value); return copyOK; };
var timers = [];
function setTimeout(fn) { timers.push(fn); return timers.length; }
function clearTimeout() {}
(function () {
` + string(code) + `
var pres = document.querySelectorAll("pre");
var codes = document.querySelectorAll(".md pre > code");
check("the diagram is not a code block", [pres.length, codes.length], [3, 2]);
var wraps = document.querySelectorAll(".cb");
check("one wrapper per code block", wraps.length, 2);
var multi = wraps[0], single = wraps[1];
check("a multi-line block has a gutter", [multi.hasAttribute("class"), multi.classList.contains("cb-ln"), multi.querySelectorAll("pre > span.ln").length], [true, true, 1]);
check("one number per line", multi.querySelector("span.ln").textContent, "1\n2\n3");
check("the gutter is hidden from readers", multi.querySelector("span.ln").getAttribute("aria-hidden"), "true");
check("a one-line block has no gutter", [single.classList.contains("cb-ln"), single.querySelectorAll("span.ln").length], [false, 0]);
check("the pre sits in its wrapper before the button", [multi.children[0].tag, multi.children[1].tag], ["pre", "button"]);
var btn = multi.querySelector("button.cb-copy");
check("the button copies, in the page's language", [btn.getAttribute("title"), btn.getAttribute("aria-label")], ["복사", "복사"]);
check("a one-line block has its button", single.querySelectorAll("button.cb-copy").length, 1);

// A second pass over the same nodes wires nothing again.
wbCodeBlocks(document);
check("wired once", [document.querySelectorAll(".cb").length, multi.querySelectorAll("button").length, multi.querySelectorAll("span.ln").length], [2, 1, 1]);

// Copy: the code text, never the numbers.
click(btn);
check("copy takes the code alone", copied, ["x := 1\ny := 2\nz := 3\n"]);
check("the button reads done", [btn.className, btn.textContent], ["cb-copy done", "복사됨"]);
timers.forEach(function (fn) { fn(); });
check("the button returns", [btn.className, btn.innerHTML.indexOf("<svg") === 0], ["cb-copy", true]);
click(single.querySelector("button"));
check("a one-line block copies its line", copied[1], "one line\n");
timers.forEach(function (fn) { fn(); });
copyOK = false;
var cur = single.querySelector("button"); timers = [];
click(cur);
check("a failed copy does not claim success", [cur.className, timers.length], ["cb-copy", 0]);
check("no scratch textarea is left behind", document.body.children.length, 0);

// A board bound later: a later wbBindBoard wires it and still binds the form.
// This run has no wsT (a page without the client table), so the button takes
// the English fallback.
delete window.wsT;
var later = new E("div", { "class": "md" }), lp = new E("pre"), lc = new E("code");
lc.textContent = "a\nb\n"; lp.appendChild(lc); later.appendChild(lp); root.appendChild(later);
var bound = [];
window.wbBindBoard = function (r) { bound.push(r); };
` + string(code) + `
window.wbBindBoard(later);
check("the form binding still runs", [bound.length, bound[0] === later], [1, true]);
check("the later board is wired", [later.querySelectorAll(".cb-ln").length, later.querySelector("span.ln").textContent], [1, "1\n2"]);
check("without wsT the button reads the fallback", later.querySelector("button.cb-copy").getAttribute("title"), "Copy");
})();
if (bad.length) { console.log(bad.join("\n")); process.exit(1); }
console.log("ok");
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "ok" {
		t.Fatalf("code block cases: %v\n%s", err, out)
	}
}
