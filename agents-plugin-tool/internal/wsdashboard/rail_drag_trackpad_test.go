package wsdashboard

import "testing"

func TestRailDragTrackpadRelease(t *testing.T) {
	data, err := dashboardTestAsset("assets/dashboard-rail.js")
	if err != nil {
		t.Fatal(err)
	}
	js := whiteboardBridgeFunctions(t, string(data), "rdMove", "rdEnd")
	runNode(t, "rail trackpad release", `
var assert = require('assert'), timers = new Map(), next = 0, rdrag, drops = 0, releases = 0, overs = 0, rdragged = false, RD_SLOP = 5;
function setTimeout(f) { timers.set(++next, f); return next; }
function clearTimeout(id) { timers.delete(id); }
function flush() { var a = [...timers.values()]; timers.clear(); a.forEach(f => f()); }
var root = {classList:{remove(){}}};
function rdOver() { overs++; } function rdBegin() {} function rdLine() {} function rdHint() {} function rdKid() {} function railMarks() {} function railRelease() { releases++; } function rdDrop() { drops++; } function rdSave() { drops++; }
`+js+`
function start(on) { rdrag = {id:1,on:on,item:{isConnected:true,classList:{remove(){}}},z:{ref:'target'},noop:false}; return rdrag; }
function move(buttons) { rdMove({pointerId:1,pointerType:'mouse',buttons:buttons,clientX:20,clientY:20,preventDefault(){}}); }
var d = start(true); move(0); assert.strictEqual(rdrag,d); assert.deepStrictEqual(d.z,{ref:'target'}); rdEnd({pointerId:1,type:'pointerup'}); flush(); assert.strictEqual(drops,1); assert.strictEqual(releases,1);
d = start(true); move(0); flush(); flush(); assert.strictEqual(rdrag,null); assert.strictEqual(drops,1); assert.strictEqual(releases,2);
d = start(true); move(0); move(1); assert.strictEqual(overs,1); flush(); assert.strictEqual(rdrag,d); rdEnd({pointerId:1,type:'pointercancel'}); flush(); assert.strictEqual(drops,1);
start(false); move(0); assert.strictEqual(rdrag,null); flush();
`)
}
