package wsdashboard

import (
	"regexp"
	"testing"
)

// Stream connection changes must not overwrite the process indicator.
func TestConnectedLeavesProcessIndicatorAlone(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	src := regexp.MustCompile(`(?s)\n  function connected\(on\) \{.*?\n  \}\n`).FindString(string(data))
	if src == "" {
		t.Fatal("connected function not found")
	}
	runNode(t, "connection and process indicators", `
var sseOn, selector, stream = { classList: { toggle: function (_, off) { stream.off = off; } } };
var host = { title: "harness process stopped" };
function q(s) { selector = s; return s === ".dwrap > .top #sse" ? [stream] : [stream, host]; }
function wsT(s) { return s; }
`+src+`
connected(false);
if (selector !== ".dwrap > .top #sse" || !stream.off || host.title !== "harness process stopped") throw Error("connection changed process indicator");
connected(true);
if (stream.off || stream.title !== "js.sse.title.on") throw Error("stream did not reconnect");
`)
}
