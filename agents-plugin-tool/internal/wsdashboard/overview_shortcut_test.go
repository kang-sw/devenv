package wsdashboard

import (
	"strings"
	"testing"
)

func TestBoardOverviewShortcutTargets(t *testing.T) {
	js := mustAsset(t, "dashboard.js")
	_, js, found := strings.Cut(js, "  function overviewButton(d) {")
	if !found {
		t.Fatal("missing Overview shortcut")
	}
	js, _, found = strings.Cut(js, "\n  if (shell && window.WsShell) {")
	if !found {
		t.Fatal("missing shell receiver boundary")
	}
	runNode(t, "Overview shortcut targets", `
var bad = [], handler, clicked = 0, sent = 0, visible = true, present = true;
var shell = false, shHidden = false, shOn = null;
var button = { getClientRects: function () { return visible ? [{}] : []; }, click: function () { clicked++; } };
var doc = { querySelector: function () { return present ? button : null; }, addEventListener: function (kind, fn) { handler = fn; } };
function shSend(ent, kind) { if (ent !== shOn || kind !== "overview") throw Error("wrong recipient"); sent++; }
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function fire(attrs, selector) {
  var e = Object.assign({ key: "q", ctrlKey: true, target: { closest: function (s) { return selector && s.indexOf(selector) >= 0; } },
    preventDefault: function () { this.defaultPrevented = true; } }, attrs || {});
  clicked = sent = 0; handler(e); return [!!e.defaultPrevented, clicked, sent];
}
`+"  function overviewButton(d) {"+js+`
check("session activates its toolbar button", fire(), [true, 1, 0]);
check("uppercase Q also works without Shift", fire({ key: "Q" }), [true, 1, 0]);
[ {ctrlKey:false}, {metaKey:true}, {altKey:true}, {shiftKey:true}, {isComposing:true}, {keyCode:229}, {key:"k"} ].forEach(function (attrs) {
  check("unclaimed key " + JSON.stringify(attrs), fire(attrs), [false, 0, 0]);
});
check("already claimed event", fire({ defaultPrevented: true }), [true, 0, 0]);
["input", "textarea", "select", "[contenteditable]", ".xterm", ".tm-host"].forEach(function (s) {
  check("native target " + s, fire({}, s), [false, 0, 0]);
});
check("inherited editable", fire({ target: { isContentEditable: true } }), [false, 0, 0]);
shHidden = true;
check("hidden session", fire(), [false, 0, 0]);
shHidden = false; present = false;
check("no board", fire(), [false, 0, 0]);
present = true; visible = false;
check("board not visible", fire(), [false, 0, 0]);
visible = true; shell = true;
check("no shown frame", fire(), [false, 0, 0]);
shOn = { el: { contentDocument: doc } };
check("shell forwards instead of toggling its own state", fire(), [true, 0, 1]);
present = false;
check("shown frame has no board", fire(), [false, 0, 0]);
present = true; visible = false;
check("shown frame hides its board", fire(), [false, 0, 0]);
visible = true;
Object.defineProperty(shOn.el, "contentDocument", { get: function () { throw Error("unloading"); } });
check("unavailable frame", fire(), [false, 0, 0]);
process.stdout.write(bad.join("\n"));
`)
}
