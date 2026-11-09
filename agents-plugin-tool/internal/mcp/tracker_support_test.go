package mcp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// supportAllowlist is the allowlist the runner gives a support
// conversation's ws-mcp (its profile's ws_tools).
var supportAllowlist = strings.Join(wsagent.SupportWSTools, ",")

// trackerSupportFixture is a lead fixture with two registered stores, "me"
// and "other", each holding one item, and a support conversation of store
// "me": an agent key at the store's directory and the daemon record that
// names it.
type trackerSupportFixture struct {
	*whiteboardFixture
	meDir    string
	meItem   string
	other    string
	support  string
	dashDir  string
	recorded wsagent.Record
}

func newTrackerSupportFixture(t *testing.T) *trackerSupportFixture {
	t.Helper()
	f := &trackerSupportFixture{whiteboardFixture: newTrackerLeadFixture(t)}
	f.trackerUpdate(t, trackerPersonal)
	f.trackerUpdate(t, `<tk:register store="other" member="ada" given="Ada"/>`)
	f.meItem = trackerWrote(t, f.trackerUpdate(t, `<tk:store name="me"><tk:item.create kind="task" title="Fix the parser" due="2026-11-20"/>
<tk:item.assign item="$1" people="ada"/></tk:store>`))["1"]
	f.other = trackerWrote(t, f.trackerUpdate(t, `<tk:store name="other"><tk:item.create kind="todo" title="Other store todo"/></tk:store>`))["1"]
	h, err := f.s.trackerHost()
	if err != nil {
		t.Fatal(err)
	}
	e, err := h.Entry("me")
	if err != nil {
		t.Fatal(err)
	}
	f.meDir = e.Dir
	if f.dashDir, err = wsdashboard.Dir(""); err != nil {
		t.Fatal(err)
	}
	f.support, f.recorded = f.conversation(t, f.meDir, wsagent.SupportProfileClaude)
	return f
}

// conversation mints an agent key at root, as the runner's key store does,
// and writes the daemon's record of a conversation of profile under it.
func (f *trackerSupportFixture) conversation(t *testing.T, root, profile string) (string, wsagent.Record) {
	t.Helper()
	key, err := NewAgentKeyStore().Mint(root)
	if err != nil {
		t.Fatal(err)
	}
	rec := wsagent.Record{ReplyID: wsmailbox.ReplyID(key), Profile: profile, Backend: wsagent.BackendClaude, Root: root, WSKey: key, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := wsagent.WriteRecordForTest(f.dashDir, rec); err != nil {
		t.Fatal(err)
	}
	return key, rec
}

func (f *trackerSupportFixture) as(key string, args map[string]any) map[string]any {
	args["session_key"] = key
	return args
}

// TestTrackerSupportConversationPassesGateWithPinnedWrites: the support
// conversation of store "me" reads every store, writes to "me", and is
// refused a write to another store and the register and sync commands.
func TestTrackerSupportConversationPassesGateWithPinnedWrites(t *testing.T) {
	f := newTrackerSupportFixture(t)
	if out := toolText(t, f.call(t, "tracker.query", f.as(f.support, map[string]any{}))); !strings.Contains(out, "me") || !strings.Contains(out, "other") {
		t.Fatalf("the support key should read the store list: %s", out)
	}
	if out := toolText(t, f.call(t, "tracker.query", f.as(f.support, map[string]any{"payload": map[string]any{"store": "other", "scope": "all"}}))); !strings.Contains(out, "Other store todo") {
		t.Fatalf("the support key should read another store: %s", out)
	}
	resp := f.call(t, "tracker.update", f.as(f.support, map[string]any{"commands": `<tk:store name="me"><tk:comment.add item="` + f.meItem + `">asked by the user</tk:comment.add></tk:store>`}))
	if isToolError(resp) || !strings.Contains(toolText(t, resp), "me: wrote 1 event") {
		t.Fatalf("the support key should write to its own store: %s", resp)
	}
	for name, commands := range map[string]string{
		"another store": `<tk:store name="other"><tk:item.create kind="task" title="smuggled"/></tk:store>`,
		// A second group is refused by the parse; the pin still names the
		// first group's store.
		"two groups": `<tk:store name="other"><tk:item.create kind="task" title="smuggled"/></tk:store><tk:store name="me"><tk:comment.add item="` + f.meItem + `">ok</tk:comment.add></tk:store>`,
	} {
		text := f.trackerRefused(t, "tracker.update", f.as(f.support, map[string]any{"commands": commands}))
		if !strings.Contains(text, `its own store "me" only`) {
			t.Errorf("%s: a write outside the pinned store must be refused naming it: %s", name, text)
		}
	}
	for _, commands := range []string{`<tk:sync store="me"/>`, `<tk:sync/>`, `<tk:register store="third" member="ada" given="Ada"/>`} {
		text := f.trackerRefused(t, "tracker.update", f.as(f.support, map[string]any{"commands": commands}))
		if !strings.Contains(text, "cannot register or sync") {
			t.Errorf("%s must be refused for the support key: %s", commands, text)
		}
	}
	if out := f.trackerQuery(t, map[string]any{"store": "other", "scope": "all"}); strings.Contains(out, "smuggled") {
		t.Fatalf("a refused write must write nothing: %s", out)
	}
	if out := f.trackerQuery(t, nil); strings.Contains(out, "third") {
		t.Fatalf("a refused register must register nothing: %s", out)
	}
	// The tracker grammar reaches the admitted key.
	if out := toolText(t, f.call(t, "playbook.read", f.as(f.support, map[string]any{"name": "tracker"}))); !strings.Contains(out, "tk:item.create") {
		t.Fatalf("the support key should read the tracker playbook: %s", out)
	}
}

// TestTrackerSupportGateRefusesEveryOtherSpawnedKey: only a key the
// daemon's open record names, at a store's directory, with a profile naming
// the tool, passes; a ferrule-style parentless delegate key, a worker key,
// and every near miss stay refused.
func TestTrackerSupportGateRefusesEveryOtherSpawnedKey(t *testing.T) {
	f := newTrackerSupportFixture(t)
	const spawned = "not available to a spawned session"
	refused := func(name, key string) {
		t.Helper()
		for tool, args := range map[string]map[string]any{
			"tracker.update": {"commands": `<tk:store name="me"><tk:comment.add item="` + f.meItem + `">smuggled</tk:comment.add></tk:store>`},
			"tracker.query":  {},
			wikiQueryTool:    {"store": "me", "op": "list"},
			wikiUpdateTool:   {"store": "me", "wiki": "smuggled", "op": "write", "new_wiki": true, "path": "home", "content": "smuggled\n"},
		} {
			resp := f.call(t, tool, f.as(key, args))
			if !isGateRefusal(resp) || !strings.Contains(resp, spawned) {
				t.Errorf("%s: %s must be refused by the call gate: %s", name, tool, resp)
			}
		}
	}
	// A parentless delegate key with no daemon record (as a ferrule login
	// mints), even at the store's directory.
	bare, err := f.s.sessions.mint(f.meDir, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	refused("parentless delegate without a record", bare)
	worker, err := f.s.sessions.mint(f.root, roleLead, f.key)
	if err != nil {
		t.Fatal(err)
	}
	refused("worker key", worker)
	child, err := f.s.sessions.mint(f.meDir, roleDelegate, f.key)
	if err != nil {
		t.Fatal(err)
	}
	refused("delegate with a parent", child)

	// A record naming another key under this key's reply-id.
	forged := f.recorded
	forged.ReplyID = wsmailbox.ReplyID(bare)
	if err := wsagent.WriteRecordForTest(f.dashDir, forged); err != nil {
		t.Fatal(err)
	}
	refused("record naming another key", bare)

	// A conversation at a directory that is no store's.
	notStore, _ := f.conversation(t, f.root, wsagent.SupportProfileClaude)
	refused("conversation outside every store", notStore)

	// A closed conversation.
	closed, rec := f.conversation(t, f.meDir, wsagent.SupportProfileClaude)
	rec.ClosedAt = time.Now().UTC().Format(time.RFC3339)
	if err := wsagent.WriteRecordForTest(f.dashDir, rec); err != nil {
		t.Fatal(err)
	}
	refused("closed conversation", closed)

	// A conversation whose profile names no tracker tool, and one whose
	// profile names only the read.
	writeGlobalProfiles(t, map[string]string{
		"chat":   `{"backend":"claude","ws_tools":["playbook.read"]}`,
		"reader": `{"backend":"claude","ws_tools":["tracker.query"]}`,
	})
	chat, _ := f.conversation(t, f.meDir, "chat")
	refused("profile without tracker tools", chat)
	reader, _ := f.conversation(t, f.meDir, "reader")
	if resp := f.call(t, "tracker.query", f.as(reader, map[string]any{})); isToolError(resp) {
		t.Fatalf("a profile naming tracker.query should read: %s", resp)
	}
	if resp := f.call(t, "tracker.update", f.as(reader, map[string]any{"commands": `<tk:store name="me"><tk:comment.add item="` + f.meItem + `">x</tk:comment.add></tk:store>`})); !isToolError(resp) || !strings.Contains(resp, spawned) {
		t.Fatalf("a profile without tracker.update must not write: %s", resp)
	}
	for tool, args := range map[string]map[string]any{
		wikiQueryTool:  {"store": "me", "op": "list"},
		wikiUpdateTool: {"store": "me", "wiki": "smuggled", "op": "write", "new_wiki": true, "path": "home", "content": "smuggled\n"},
	} {
		if resp := f.call(t, tool, f.as(reader, args)); !isGateRefusal(resp) || !strings.Contains(resp, spawned) {
			t.Fatalf("a profile naming only tracker.query must not reach %s past the call gate: %s", tool, resp)
		}
	}
	if out := f.wikiCall(t, wikiQueryTool, map[string]any{"store": "me", "op": "list"}); strings.Contains(out, "smuggled") {
		t.Fatalf("a refused key must save no wiki: %s", out)
	}
	if out := f.trackerQuery(t, map[string]any{"store": "me", "item": f.meItem}); strings.Contains(out, "smuggled") {
		t.Fatalf("a refused key must write nothing: %s", out)
	}
}

// writeGlobalProfiles writes agent profiles into the fixture's global
// config file.
func writeGlobalProfiles(t *testing.T, profiles map[string]string) {
	t.Helper()
	var b strings.Builder
	b.WriteString(`{"schema_version":1,"structured_overrides":{`)
	first := true
	for name, raw := range profiles {
		if !first {
			b.WriteString(",")
		}
		first = false
		b.WriteString(`"` + wsagent.ProfilePrefix + name + `":` + raw)
	}
	b.WriteString("}}")
	dir := os.Getenv("WS_CONFIG_HOME")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestTrackerShowCardsOwnStoreOnly: an item of the conversation's store
// comes back as a card with its mini-card fields; another store's item as
// text with no card; an unknown or malformed ID is refused.
func TestTrackerShowCardsOwnStoreOnly(t *testing.T) {
	f := newTrackerSupportFixture(t)
	t.Setenv("WS_MCP_ALLOWED_TOOLS", supportAllowlist)
	out := toolText(t, f.call(t, trackerShowTool, f.as(f.support, map[string]any{"item": f.meItem})))
	first, _, _ := strings.Cut(out, "\n")
	if first != "card me "+f.meItem {
		t.Fatalf("an own-store item should start with its card line, got %q in:\n%s", first, out)
	}
	for _, want := range []string{`"Fix the parser"`, "status todo", "priority", "due 2026-11-20", "assignees: ada"} {
		if !strings.Contains(out, want) {
			t.Errorf("the card result lacks %q:\n%s", want, out)
		}
	}
	out = toolText(t, f.call(t, trackerShowTool, f.as(f.support, map[string]any{"item": f.other})))
	if strings.HasPrefix(out, trackerCardPrefix) || !strings.Contains(out, "no card") || !strings.Contains(out, "store other") || !strings.Contains(out, "Other store todo") {
		t.Fatalf("another store's item should be text with no card:\n%s", out)
	}
	for _, item := range []string{"ada.nowhere-99", "not an id"} {
		resp := f.call(t, trackerShowTool, f.as(f.support, map[string]any{"item": item}))
		if !isToolError(resp) {
			t.Errorf("tracker.show(%q) must be refused: %s", item, resp)
		}
	}
	// A lead that allowlists the tool still has no chat to draw in.
	resp := f.call(t, trackerShowTool, map[string]any{"item": f.meItem})
	if !isToolError(resp) || !strings.Contains(resp, "no chat") {
		t.Fatalf("a lead's tracker.show should be refused: %s", resp)
	}
	// A spawned key that is no support conversation is refused at the gate.
	bare, err := f.s.sessions.mint(f.meDir, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp := f.call(t, trackerShowTool, f.as(bare, map[string]any{"item": f.meItem})); !isToolError(resp) || !strings.Contains(resp, "not available to a spawned session: only a tracker store's support conversation draws item cards") {
		t.Fatalf("a non-support spawned key must be refused tracker.show, with its own reason: %s", resp)
	}
}

// TestHiddenToolsListedOnlyWhenAllowlisted: tracker.show and git.search
// never show in an ordinary session's tool list or capabilities, nor answer
// a call there; an allowlist naming them lists them.
func TestHiddenToolsListedOnlyWhenAllowlisted(t *testing.T) {
	f := newTrackerSupportFixture(t)
	hidden := []string{trackerShowTool, gitSearchTool}
	list := callToolsList(t, f.s)
	for _, name := range hidden {
		if strings.Contains(list, `"`+name+`"`) {
			t.Errorf("a lead's tools/list shows %s", name)
		}
		if slices.Contains(LeadToolNames(), name) {
			t.Errorf("LeadToolNames shows %s", name)
		}
	}
	if !strings.Contains(list, `"tracker.query"`) {
		t.Fatalf("tools/list should still list tracker.query")
	}
	resp := f.call(t, trackerShowTool, f.as(f.support, map[string]any{"item": f.meItem}))
	if !strings.Contains(resp, "tool not available") {
		t.Fatalf("a hidden tool must not answer without the allowlist: %s", resp)
	}
	t.Setenv("WS_MCP_ALLOWED_TOOLS", supportAllowlist)
	list = callToolsList(t, f.s)
	if !strings.Contains(list, `"`+trackerShowTool+`"`) || !slices.Contains(LeadToolNames(), trackerShowTool) {
		t.Fatalf("an allowlist naming tracker.show should list it: %s", list)
	}
}
