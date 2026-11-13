package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

// TestRailMotionFollowsScrollOnly pins that rail.js turns a capsule into a
// bar (and back) only on a scroll. A run after a size change in the board
// (the ResizeObserver: a dashboard board swap growing and shrinking its
// topics, a body opened) redraws the stacks in place: during a swap's
// motion, item after item crossed the bottom stack's line, and each
// crossing hid that item's bar behind a copy aimed at where the bar stood
// that frame, so closed topics pouring in showed as blank rows one after
// another. The same membership change on a scroll still morphs. The cases
// execute under node when it is installed.
func TestRailMotionFollowsScrollOnly(t *testing.T) {
	data, err := assetFS.ReadFile("assets/rail.js")
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; rail motion cases not executed")
	}
	// The prelude's elements gain what the motion touches (animate, clones,
	// attribute removal), the body can animate, and the board keeps its
	// listeners so a case can fire a scroll.
	script := railPrelude + `
var anims = [], listeners = {};
E.prototype.animate = function (k) { anims.push(this); return {}; };
E.prototype.cloneNode = function () { var c = new E(this.tag); c.cls = Object.assign({}, this.cls); return c; };
E.prototype.removeAttribute = function (k) { delete this.attrs[k]; };
E.prototype.removeChild = function (c) { this.children.splice(this.children.indexOf(c), 1); };
Object.defineProperty(E.prototype, "parentNode", { get: function () { return this.parent; } });
Object.defineProperty(E.prototype, "lastChild", { get: function () { return this.children[this.children.length - 1] || null; } });
document.body.animate = function () {};
board.addEventListener = function (t, f) { listeners[t] = f; };
function topics(off) { board.children.forEach(function (t) { if (t.attrs["data-topic"]) { var i = "abcdef".indexOf(t.attrs["data-topic"]); t.y = i * 600 - off; t.children[0].y = t.children[0].children[0].y = t.y; } }); }
function ghosts() { return anims.filter(function (e) { return e.cls["wb-rail-ghost"]; }).length; }
` + string(data) + `
frame();
// Topic c sits below the bottom line (745) in the bottom stack. The board
// moves it above the line by a size change (topics shift up 500): the
// stack loses it with no motion.
anims = []; topics(500); ro(); frame();
check("a size change morphs nothing", anims.length, 0);
// Back, settled by another size change, then the same crossing by a
// scroll: c's capsule grows into its bar, and a's bar, scrolled past,
// turns into the top stack's capsule.
topics(0); ro(); frame(); anims = [];
board.scrollTop = 500; listeners.scroll(); frame();
check("a scroll morphs at both edges", ghosts(), 2);
process.stdout.write(bad.join("\n"));`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("rail motion cases failed:\n%s", s)
	}
}
