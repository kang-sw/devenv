package wsdashboard

import (
	"regexp"
	"testing"
)

// TestBoardMotionStartsAtFirstFrame pins when a board swap's motion starts
// (dashboard.js startMotion). The timeline's time holds still at the last
// frame's through a task, and a large board's swap is a long task:
// animations started as created were already over at the swap's first
// frame, which drew the old heights (a closing topic's empty open height,
// the board under it blank) and jumped to the end on the next. The swap
// hands every growth and slide to startMotion (TestBoardTopicMotion pins
// the wiring), which holds them at their first keyframe and sets their
// start to the clock at the next frame's callbacks; an animation no longer
// running by then (cancelled) is not restarted, and a swap with nothing to
// move asks for no frame. The cases execute under node when it is
// installed.
func TestBoardMotionStartsAtFirstFrame(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	f := regexp.MustCompile(`(?s)\n  function startMotion\(.*?\n  \}\n`).FindString(js)
	if f == "" {
		t.Fatal("dashboard.js holds no startMotion")
	}
	runNode(t, "board motion start", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var frames = [], clock = 100;
function requestAnimationFrame(f) { frames.push(f); }
var performance = { now: function () { return clock; } };
function Anim(state) { var me = this; this.playState = state; this.startTime = 100; this.fill = "none"; this.effect = { updateTiming: function (t) { if (t.fill) me.fill = t.fill; } }; }
`+f+`
var grow = new Anim("running"), slide = new Anim("running"), gone = new Anim("running");
startMotion([grow, null, slide, gone]);
check("held at the first keyframe until they start", [grow.fill, slide.fill], ["backwards", "backwards"]);
check("one frame asked for", frames.length, 1);
clock = 400; gone.playState = "idle";
frames.shift()();
check("started at the first frame's clock", [grow.startTime, slide.startTime], [400, 400]);
check("a cancelled animation stays cancelled", gone.startTime, 100);
startMotion([null, null]);
check("nothing to move asks for no frame", frames.length, 0);
process.stdout.write(bad.join("\n"));`)
}
