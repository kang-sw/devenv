package wsdashboard

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestTrackerTkReadAllRendersEachStoreAsItArrives pins the tracker page's
// multi-store read (dashboard.js tkReadAll): with two stores whose view
// reads answer at different times, the mounted renderer is told as soon as
// the fast store's view lands, while the slow store keeps its previous view,
// and done fires only once both have answered. The last view is not
// announced by tkReadAll itself, since done's caller (tkReload) does. The
// cases execute under node when it is installed.
func TestTrackerTkReadAllRendersEachStoreAsItArrives(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)\n  function tkErrText\(status, v\) \{.*?\n  // tkReload re-reads`).FindString(string(data))
	if block == "" {
		t.Fatal("dashboard.js holds no tkErrText ... tkReload block")
	}
	block = strings.TrimSuffix(block, "// tkReload re-reads")
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; tkReadAll cases not executed")
	}
	script := `
var tkApi = {}, tkOrder = [], tkViews = {}, tkErrs = {}, tkSeen = {}, tkHostErr = null, tkSupport = false, tkLastRev = null;
var pending = {}, log = [], bad = [];
function wsT(k) { return k; }
function tkUrl(id, op) { return "/tracker/stores/" + id + "/" + op; }
function tkFetch(path, body, done) {
  if (path === "/tracker/stores") { done(200, { rev: 1, stores: [{ id: "fast" }, { id: "slow" }] }); return; }
  pending[path] = done;
}
var tkSubs = [function () { log.push("notify:" + (tkViews.fast ? tkViews.fast.v : "-") + "," + (tkViews.slow ? tkViews.slow.v : "-")); }];
` + block + `
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
tkViews.slow = { v: "old" };
tkReadAll(function (status) { log.push("listed:" + status); }, function () { log.push("done"); });
check("before any view", log, ["listed:200"]);
pending["/tracker/stores/fast/view"](200, { view: { v: "new" } });
check("fast view landed, slow still out", log, ["listed:200", "notify:new,old"]);
pending["/tracker/stores/slow/view"](200, { view: { v: "new" } });
check("both landed", log, ["listed:200", "notify:new,old", "done"]);
check("slow view replaced", tkViews.slow, { v: "new" });

// Before the renderer mounts, an early view tells nobody.
tkApi = null; log = []; pending = {}; tkViews = {};
tkReadAll(function (status) { log.push("listed:" + status); }, function () { log.push("done"); });
pending["/tracker/stores/slow/view"](500, {});
check("unmounted, first view landed", log, ["listed:200"]);
pending["/tracker/stores/fast/view"](200, { view: { v: "new" } });
check("unmounted, both landed", log, ["listed:200", "done"]);
process.stdout.write(bad.join("\n"));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("tkReadAll cases failed:\n%s", s)
	}
}
