package wsdashboard

import (
	"regexp"
	"testing"
)

// TestComposerCarriesWebPicks runs the composer's web pick paths from
// dashboard.js under node: a pick or a console block alone is a message, a
// send carries both and drops the sent ones, and the composer refuses one
// past the inbox's caps with a warning rather than a send the inbox would
// refuse whole.
func TestComposerCarriesWebPicks(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	notes := regexp.MustCompile(`(?s)\n  function noteWarn\(text\) \{[^\n]*\n  function noteOk\(text\) \{[^\n]*\n`).FindString(src)
	caps := regexp.MustCompile(`\n  var WEB_PICKS = 50, WEB_CONSOLES = 10;\n`).FindString(src)
	if notes == "" || caps == "" {
		t.Fatal("dashboard.js holds no noteWarn / noteOk pair, or no web pick caps at the inbox's 50 picks and 10 console blocks")
	}
	fns := composerFunc(t, src, "sendMessage") + composerFunc(t, src, "addWebPick") + composerFunc(t, src, "addWebConsole")
	runNode(t, "web picks", composerToastPrelude+notes+caps+"var sending = false, drafts = 0;\nfunction queueDraft() { drafts++; }\n"+fns+`
var bodies = [];
post = function (route, body, done) { bodies.push([route, JSON.parse(JSON.stringify(body))]); done(answer[0], answer[1]); };
var p1 = { url: "http://localhost:5173/", selector: "#a", text: "A" }, c1 = { url: "http://localhost:5173/", lines: ["e1"] };
check("a pick is taken", [addWebPick(p1), wpicks, drafts], [true, [p1], 1]);
sendMessage();
check("a pick alone is a message", bodies, [["message", { text: "", chips: [], topics: [], mentions: [], picks: [p1] }]]);
check("the sent pick is dropped", wpicks, []);
bodies = [];
addWebConsole(c1); ctext.value = "see";
sendMessage();
check("a console block rides with the text", bodies, [["message", { text: "see", chips: [], topics: [], mentions: [], consoles: [c1] }]]);
check("the sent console block is dropped", wconsoles, []);
bodies = []; toasts = [];
answer = [false, "busy"]; addWebPick(p1); sendMessage();
check("a refused send keeps the pick", wpicks, [p1]);
answer = [true, {}]; wpicks = []; toasts = [];
for (var i = 0; i < 50; i++) addWebPick({ url: "u", selector: "#p" + i, text: "" });
for (var j = 0; j < 10; j++) addWebConsole({ url: "u", lines: ["e"] });
check("one pick past the cap is refused", [addWebPick(p1), wpicks.length], [false, 50]);
check("one console block past the cap is refused", [addWebConsole(c1), wconsoles.length], [false, 10]);
check("each refusal warns", toasts, [["js.web.cap.picks:50", "warn"], ["js.web.cap.consoles:10", "warn"]]);
process.stdout.write(bad.join("\n"));
`)
}
