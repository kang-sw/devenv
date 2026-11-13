package wsdashboard

import (
	"regexp"
	"testing"
)

// Exercise the actual panel helpers and capture toggle listener, including the
// storage round trip with fresh page state. frameDOM queues browser-like toggles.
func TestPanelShutPersistence(t *testing.T) {
	raw, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	constants := regexp.MustCompile(`var SHUT_KEY = [^\n]+;`).FindString(string(raw))
	listener := regexp.MustCompile(`doc\.addEventListener\("toggle", function \(e\) \{\n[\s\S]*?\n  \}, true\);`).FindString(string(raw))
	if constants == "" || listener == "" {
		t.Fatal("dashboard.js holds no panel shut constants or toggle listener")
	}
	src := dashboardFuncs(t, "isPanel", "shutId", "shutList", "shutHas", "shutPut", "isPeer", "openKey", "restoreOpenOne", "restoreOpen")
	runNode(t, "panel shut persistence", frameDOM+constants+src+`
var rid = "session-a", openState = {}, PEER_KEY = "p.", railSets = null;
var storage = {};
function load(k) { return Object.prototype.hasOwnProperty.call(storage, k) ? storage[k] : null; }
function save(k, v) { if (v === null) delete storage[k]; else storage[k] = v; }
function panel(key, open) {
  var attrs = { "class": "entry-panel", "data-key": key };
  if (open) attrs.open = "";
  return new El("details", attrs);
}
`+listener+`
check("the production cap is 200", SHUT_CAP === 200);
var p = panel("entry:e1", true);
doc.appendChild(p);
check("data-key entry panel is recognized", isPanel(p));
check("a panel without data-key is not persisted", !isPanel(new El("details", { "class": "entry-panel" })));
check("a non-panel keyed fold is not persisted", !isPanel(new El("details", { "data-key": "entry:e1" })));
p.open = false; tick();
check("a hand fold reaches storage through the toggle listener", load(SHUT_KEY) === JSON.stringify(["session-a/entry:e1"]));
check("the entry id is scoped by session", shutId(p) === "session-a/entry:e1");

// Reload: discard both in-memory caches, retaining only serialized storage.
doc.removeChild(p);
openState = {}; shutSet = null;
p = panel("entry:e1", true); doc.appendChild(p);
restoreOpen(doc);
check("a folded data-key panel restores closed with a fresh shutSet", !p.open && shutHas(shutId(p)));
tick();
var next = panel("entry:e2", true); doc.appendChild(next);
restoreOpenOne(next);
check("an overwrite's different entry key keeps its server default", next.open && !shutHas(shutId(next)));
rid = "session-b"; openState = {};
var other = panel("entry:e1", true);
restoreOpenOne(other);
check("another session does not inherit the collapse", other.open && !shutHas(shutId(other)));
rid = "session-a";

p.open = true; tick();
check("a hand reopen removes the stored collapse", load(SHUT_KEY) === null && !shutHas(shutId(p)));
openState = {}; shutSet = null;
var reopened = panel("entry:e1", true);
restoreOpenOne(reopened);
check("reopening survives another reload", reopened.open);

// Draft/in-memory open state takes precedence over an old stored collapse.
shutPut(shutId(p), true);
openState[openKey(p)] = true;
var preferred = panel("entry:e1", false); doc.appendChild(preferred);
check("the conflicting collapse is present before restore", shutHas(shutId(preferred)));
restoreOpenOne(preferred);
check("stored openState entry wins over the shut list", preferred.open);
tick();

// LRU: touching an existing id moves it to the newest end without duplicates.
save(SHUT_KEY, null); shutSet = null;
for (var i = 0; i < SHUT_CAP; i++) shutPut("session-a/entry:lru" + i, true);
shutPut("session-a/entry:lru0", true);
shutPut("session-a/entry:lru200", true);
var expected = [];
for (var i = 2; i < SHUT_CAP; i++) expected.push("session-a/entry:lru" + i);
expected.push("session-a/entry:lru0", "session-a/entry:lru200");
check("LRU retains exactly the 200 newest ids in order", shutList().length === SHUT_CAP && JSON.stringify(shutList()) === JSON.stringify(expected));
shutSet = null;
check("the evicted id stays absent after reload", !shutHas("session-a/entry:lru1"));
check("the re-folded oldest id and newest id survive reload", shutHas("session-a/entry:lru0") && shutHas("session-a/entry:lru200"));
if (bad.length) console.log(bad.join("\n"));
`)
}
