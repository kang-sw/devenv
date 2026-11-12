package wsdashboard

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestTopicBodyImageRelayStaysNarrow pins the page's relay of a topic-body
// frame's image request (dashboard.js relayImage), the security boundary
// between a body's opaque-origin scripts and the daemon's origin: the request
// is handled only inside the listener that has already matched its source to
// one of the page's own body frames, the fetch takes only a URL relayHref
// allows, and relayHref allows only a URL on that frame's own data-relay list
// that is this session's image route for an image file name. The relayHref
// cases execute under node when it is installed.
func TestTopicBodyImageRelayStaysNarrow(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)

	// Own frames only: the one call site sits in the listener that returns
	// unless docFrame matched the message's source.
	if n := strings.Count(js, "relayImage(f, m);"); n != 1 {
		t.Fatalf("relayImage call sites = %d, want 1", n)
	}
	listener := regexp.MustCompile(`window\.addEventListener\("message", function \(e\) \{\n    var f = docFrame\(e\.source\), m = e\.data;\n    if \(!f \|\| !m \|\| typeof m !== "object"\) return;\n[\s\S]*?\n  \}\);\n`).FindString(js)
	if !strings.Contains(listener, `m.wbDoc === "img"`) || !strings.Contains(listener, "relayImage(f, m)") {
		t.Fatalf("the image relay is not inside the docFrame-gated listener:\n%s", listener)
	}
	relay := regexp.MustCompile(`function relayImage\(f, m\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	for _, want := range []string{
		`relayHref(f.getAttribute("data-relay") || "", m.href)`,
		`if (!href || typeof id !== "number"`,
		`fetch(href, { credentials: "same-origin"`,
		`/^image\/[a-z0-9.+-]+$/.test(type)`,
		`f.contentWindow.postMessage({ wbDoc: "img", id: id, blob: b }, "*")`,
	} {
		if !strings.Contains(relay, want) {
			t.Errorf("relayImage lacks %q:\n%s", want, relay)
		}
	}
	if strings.Count(relay, "fetch(") != 1 {
		t.Errorf("relayImage fetches other than the one allowed URL:\n%s", relay)
	}

	fn := regexp.MustCompile(`function relayHref\(list, href\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	if fn == "" {
		t.Fatal("dashboard.js holds no relayHref")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; relayHref cases not executed")
	}
	const ok = "/s/rid1/image?path=%2Ftmp%2Fshot.png"
	cases := []struct {
		List string `json:"list"`
		Href string `json:"href"`
		Want bool   `json:"want"`
	}{
		{ok, ok, true},
		{"/s/rid1/image?path=%2Fa.png " + ok, ok, true},
		{"/s/rid1/image?path=%2Ftmp%2Fshot+1.svg", "/s/rid1/image?path=%2Ftmp%2Fshot+1.svg", true},
		{"", ok, false},       // not on the frame's list
		{ok + "x", ok, false}, // a list entry it only prefixes
		{"x" + ok, ok, false}, // a list entry it only suffixes
		{ok, ok + " ", false}, // spaces never match an entry
		{"/s/other/image?path=%2Fa.png", "/s/other/image?path=%2Fa.png", false}, // another session
		{"/s/rid1/file?path=%2Fa.png", "/s/rid1/file?path=%2Fa.png", false},     // another route
		{"/s/rid1/image?path=%2Fa.txt", "/s/rid1/image?path=%2Fa.txt", false},   // not an image name
		{"/s/rid1/image?path=%2Fa.png&x=1", "/s/rid1/image?path=%2Fa.png&x=1", false},
		{"/s/rid1/image?path=%2Fa.png#f", "/s/rid1/image?path=%2Fa.png#f", false},
		{"/s/rid1/image?path=%E0%A4%A.png", "/s/rid1/image?path=%E0%A4%A.png", false}, // malformed escape
		{"http://evil.example/s/rid1/image?path=a.png", "http://evil.example/s/rid1/image?path=a.png", false},
		{"//evil.example/s/rid1/image?path=a.png", "//evil.example/s/rid1/image?path=a.png", false},
	}
	in, _ := json.Marshal(cases)
	script := "var rid = \"rid1\";\n" + fn + `
var cases = ` + string(in) + `, bad = [];
cases.forEach(function (c) { var got = relayHref(c.list, c.href); if ((got === c.href) !== c.want || (!c.want && got !== "")) bad.push(JSON.stringify(c) + " -> " + JSON.stringify(got)); });
[123, null, undefined, {}].forEach(function (v) { if (relayHref("` + ok + `", v) !== "") bad.push("non-string href allowed: " + String(v)); });
if (relayHref("` + ok + `", "") !== "") bad.push("empty href allowed");
rid = "";
if (relayHref("/s//image?path=a.png", "/s//image?path=a.png") !== "") bad.push("allowed with no reply-id");
process.stdout.write(bad.join("\n"));
`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("relayHref cases failed:\n%s", s)
	}
}
