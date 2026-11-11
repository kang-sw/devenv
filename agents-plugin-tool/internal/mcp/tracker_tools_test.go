package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

func (f *whiteboardFixture) trackerUpdate(t *testing.T, commands string) string {
	t.Helper()
	resp := f.call(t, "tracker.update", map[string]any{"commands": commands})
	if isToolError(resp) {
		t.Fatalf("tracker.update(%s) failed: %s", commands, toolText(t, resp))
	}
	return toolText(t, resp)
}

func (f *whiteboardFixture) trackerQuery(t *testing.T, payload map[string]any) string {
	t.Helper()
	args := map[string]any{}
	if payload != nil {
		args["payload"] = payload
	}
	resp := f.call(t, "tracker.query", args)
	if isToolError(resp) {
		t.Fatalf("tracker.query(%v) failed: %s", payload, toolText(t, resp))
	}
	return toolText(t, resp)
}

// trackerRefused calls tool and returns its refusal text, failing the test
// when the call succeeds.
func (f *whiteboardFixture) trackerRefused(t *testing.T, tool string, args map[string]any) string {
	t.Helper()
	resp := f.call(t, tool, args)
	if !isToolError(resp) {
		t.Fatalf("%s(%v) must be refused: %s", tool, args, toolText(t, resp))
	}
	return toolText(t, resp)
}

func (f *whiteboardFixture) manualText(t *testing.T, key string) string {
	t.Helper()
	return toolText(t, f.call(t, "workflow_manual", map[string]any{"session_key": key}))
}

func initTrackerRemote(t *testing.T) string {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "--quiet", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("init remote: %v: %s", err, out)
	}
	return remote
}

// trackerOtherMachine is a second machine's host, with its own root and so
// its own signing key.
func trackerOtherMachine(t *testing.T) *wstrackerhost.Host {
	t.Helper()
	h, err := wstrackerhost.New(wstrackerhost.Options{Root: filepath.Join(t.TempDir(), "other")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

var trackerWroteLine = regexp.MustCompile(`(?m)^(\d+) (tk:\S+) (\S+)$`)

// trackerWrote maps each command position of an update's output to the
// event id (display form) it wrote.
func trackerWrote(t *testing.T, out string) map[string]string {
	t.Helper()
	ids := map[string]string{}
	for _, m := range trackerWroteLine.FindAllStringSubmatch(out, -1) {
		ids[m[1]] = m[3]
	}
	if len(ids) == 0 {
		t.Fatalf("no written ids in %q", out)
	}
	return ids
}

const trackerPersonal = `<tk:register store="me" member="ada" org="Acme" family="Kim" given="Ada"/>`

func TestTrackerUpdateRegistersAndJoinsWithSavedProfile(t *testing.T) {
	f := newTrackerLeadFixture(t)
	ctx := context.Background()
	out := f.trackerUpdate(t, trackerPersonal)
	for _, want := range []string{"registered me: created, member ada, no remote", `profile: org="Acme" family="Kim" given="Ada"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("personal register output missing %q:\n%s", want, out)
		}
	}

	// Another machine creates the team store on the remote; this one joins
	// it with no profile attributes, so with the profile saved above.
	remote := initTrackerRemote(t)
	bo := trackerOtherMachine(t)
	if _, err := bo.Register(ctx, wstrackerhost.RegisterRequest{ID: "team", Remote: remote, Member: "bo", Profile: &wstracker.Profile{Given: "Bo"}}); err != nil {
		t.Fatalf("bo creates: %v", err)
	}
	out = f.trackerUpdate(t, `<tk:register store="team" member="ada" remote="`+remote+`" label="Team"/>`)
	for _, want := range []string{"registered team: joined, member ada, remote " + remote, `profile: org="Acme" family="Kim" given="Ada"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("join output missing %q:\n%s", want, out)
		}
	}
	if _, err := bo.Sync(ctx, "team"); err != nil {
		t.Fatalf("bo syncs: %v", err)
	}
	v, err := bo.View(ctx, "team", 0)
	if err != nil {
		t.Fatal(err)
	}
	if m, ok := v.Members["ada"]; !ok || m.Profile != (wstracker.Profile{Org: "Acme", Family: "Kim", Given: "Ada"}) {
		t.Fatalf("bo must see ada joined with the saved profile: %+v", v.Members["ada"])
	}

	list := f.trackerQuery(t, nil)
	for _, want := range []string{"- me, member ada, no remote", "- team (Team), member ada, remote " + remote, `- org="Acme" family="Kim" given="Ada" (used last)`} {
		if !strings.Contains(list, want) {
			t.Fatalf("store list missing %q:\n%s", want, list)
		}
	}

	text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:register store="team" member="ada" remote="` + remote + `"/>`})
	if !strings.Contains(text, `"team" is already registered on this machine`) {
		t.Fatalf("a second register of one store id must be refused before the network: %s", text)
	}
	text = f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:register store="team2" member="bo" remote="` + remote + `"/>`})
	if !strings.Contains(text, "Nothing was registered") {
		t.Fatalf("a join under a taken member id must fail and register nothing: %s", text)
	}
	if strings.Contains(f.trackerQuery(t, nil), "team2") {
		t.Fatalf("a failed register must leave no registry entry")
	}
}

// trackerCorruptAfterPush breaks the tracker registry once, after the
// first push succeeds, so a registration fails after it reached the remote.
type trackerCorruptAfterPush struct {
	registry string
	done     bool
}

func (r *trackerCorruptAfterPush) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	out, err := wsgit.ExecRunner{}.RunGit(ctx, root, args...)
	if err == nil && !r.done && slices.Contains(args, "push") {
		r.done = true
		if werr := os.WriteFile(r.registry, []byte("not json"), 0o600); werr != nil {
			return nil, werr
		}
	}
	return out, err
}

// A register that fails after its push says what the remote already holds
// and how to finish, instead of claiming nothing was registered; sending
// the same command again finishes it.
func TestTrackerRegisterFailureAfterPushNamesWhatTheRemoteHolds(t *testing.T) {
	f := newTrackerLeadFixture(t)
	root, err := wstrackerhost.Root(wsconfig.Options{})
	if err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(root, "stores.json")
	f.s.tracker.opts.Runner = &trackerCorruptAfterPush{registry: registry}
	register := `<tk:register store="team" member="ada" given="Ada" remote="` + initTrackerRemote(t) + `"/>`
	text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": register})
	if !strings.Contains(text, "Nothing was registered on this machine, but the remote already holds the new store, with member ada as its sole admin; send the same tk:register again to finish") {
		t.Fatalf("a failure after the push must name what the remote holds: %s", text)
	}
	if err := os.Remove(registry); err != nil {
		t.Fatal(err)
	}
	if out := f.trackerUpdate(t, register); !strings.Contains(out, "registered team: created, member ada") {
		t.Fatalf("sending the register again must finish it: %s", out)
	}
}

func TestTrackerCallsTurnTheKnobOnOnlyWhenTheySucceed(t *testing.T) {
	f := newTrackerLeadFixture(t)
	text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:store name="me"><tk:item.create kind="task" title="x"/></tk:store>`})
	if !strings.Contains(text, "no store is registered") {
		t.Fatalf("an event command with no store registered must say so: %s", text)
	}
	f.trackerRefused(t, "tracker.query", map[string]any{"payload": map[string]any{"scope": "mine"}})
	if f.trackerOn(t, f.key) || strings.Contains(f.manualText(t, f.key), "### Tracker") {
		t.Fatalf("a refused call must leave the knob off and the manual without the tracker section")
	}
	f.trackerUpdate(t, trackerPersonal)
	if !f.trackerOn(t, f.key) {
		t.Fatalf("a successful tracker.update must turn the knob on at session scope")
	}
	manual := f.manualText(t, f.key)
	if !strings.Contains(manual, "### Tracker") || !strings.Contains(manual, "`tk:item.create`") {
		t.Fatalf("knob on: the manual must carry the tracker section:\n%s", manual)
	}

	// A successful query turns it on as well.
	g := newTrackerLeadFixture(t)
	if out := g.trackerQuery(t, nil); !strings.Contains(out, "stores: none registered") || !strings.Contains(out, "profiles: none saved") {
		t.Fatalf("an empty store list: %s", out)
	}
	if !g.trackerOn(t, g.key) {
		t.Fatalf("a successful tracker.query must turn the knob on at session scope")
	}
	// So does the JSON store list, whose output has no room for a warning.
	j := newTrackerLeadFixture(t)
	resp := j.call(t, "tracker.query", map[string]any{"format": "json"})
	var list struct {
		Stores []json.RawMessage `json:"stores"`
	}
	if isToolError(resp) || json.Unmarshal([]byte(toolText(t, resp)), &list) != nil || list.Stores == nil {
		t.Fatalf("the JSON store list: %s", toolText(t, resp))
	}
	if !j.trackerOn(t, j.key) {
		t.Fatalf("a successful JSON tracker.query must turn the knob on at session scope")
	}
}

// isGateRefusal reports whether resp is the call gate's JSON-RPC refusal
// (-32601) rather than a handler's tool error, which can carry the same
// text and would mask a gate that let the call through.
func isGateRefusal(resp string) bool {
	return strings.Contains(resp, `"error":{"code":-32601,`)
}

func TestTrackerToolsRefuseSpawnedKeys(t *testing.T) {
	f := newTrackerLeadFixture(t)
	worker, err := f.s.sessions.mint(f.root, roleLead, f.key)
	if err != nil {
		t.Fatal(err)
	}
	delegate, err := f.s.sessions.mint(f.root, roleDelegate, f.key)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{worker, delegate} {
		for tool, args := range map[string]map[string]any{
			"tracker.update": {"session_key": key, "commands": trackerPersonal},
			"tracker.query":  {"session_key": key},
			wikiQueryTool:    {"session_key": key, "store": "me", "op": "list"},
			wikiUpdateTool:   {"session_key": key, "store": "me", "wiki": "docs", "op": "write", "new_wiki": true, "path": "home", "content": "smuggled\n"},
		} {
			resp := f.call(t, tool, args)
			if !isGateRefusal(resp) || !strings.Contains(resp, "not available to a spawned session") {
				t.Fatalf("%s with a spawned key must be refused by the call gate: %s", tool, resp)
			}
		}
	}
	if out := f.trackerQuery(t, nil); !strings.Contains(out, "stores: none registered") {
		t.Fatalf("a refused spawned register must register nothing: %s", out)
	}
}

// A key the session store cannot find (made up, stale, or expired) passes
// the call gate, which only refuses keys it finds; the tracker tools refuse
// it themselves before any registry read or write.
func TestTrackerToolsRefuseUnknownKeys(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	for tool, args := range map[string]map[string]any{
		"tracker.update": {"session_key": "not-a-real-key", "commands": `<tk:register store="mine" member="eve" given="Eve"/>`},
		"tracker.query":  {"session_key": "not-a-real-key"},
		wikiQueryTool:    {"session_key": "not-a-real-key", "store": "me", "op": "list"},
		wikiUpdateTool:   {"session_key": "not-a-real-key", "store": "me", "wiki": "docs", "op": "write", "new_wiki": true, "path": "home", "content": "smuggled\n"},
	} {
		text := f.trackerRefused(t, tool, args)
		if !strings.Contains(text, "unknown_session") {
			t.Fatalf("%s with an unknown key must be refused as an unknown session: %s", tool, text)
		}
	}
	text := f.trackerRefused(t, "tracker.update", map[string]any{"session_key": "bogus", "commands": `<tk:store name="me"><tk:item.create kind="task" title="smuggled"/></tk:store>`})
	if !strings.Contains(text, "unknown_session") {
		t.Fatalf("an event write with an unknown key must be refused: %s", text)
	}
	if out := f.trackerQuery(t, nil); strings.Contains(out, "mine") {
		t.Fatalf("a refused unknown-key register must register nothing: %s", out)
	}
	if out := f.trackerQuery(t, map[string]any{"store": "me", "scope": "all"}); strings.Contains(out, "smuggled") {
		t.Fatalf("a refused unknown-key write must write nothing: %s", out)
	}
	if out := f.wikiCall(t, wikiQueryTool, map[string]any{"store": "me", "op": "list"}); !strings.Contains(out, "has no wikis") {
		t.Fatalf("a refused unknown-key wiki save must write nothing: %s", out)
	}
}

// A raw multi-line Korean markdown body goes in unescaped and comes back
// byte for byte; $N names what earlier commands of the batch create.
func TestTrackerUpdateWritesRawBodiesAndBatchReferences(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	body := "## 배경\n\n파서가 `<a href=\"x\">` 태그와 \"따옴표\", 'quote', & 기호를 그대로 둔다.\n- 첫째\n- 둘째"
	out := f.trackerUpdate(t, `<tk:store name="me">
<tk:item.create kind="task" title="파서 정리" due="2026-11-20" project="ws">
`+body+`
</tk:item.create>
<tk:item.assign item="$1" people="ada"/>
<tk:comment.add item="$1">첫 댓글
두 줄</tk:comment.add>
<tk:comment.add item="$1" reply_to="$3">답글</tk:comment.add>
<tk:item.status item="$1" status="doing" origin="agent-auto">시작</tk:item.status>
</tk:store>`)
	if !strings.Contains(out, "me: wrote 5 events") {
		t.Fatalf("update output: %s", out)
	}
	ids := trackerWrote(t, out)
	item, first, second := ids["1"], ids["3"], ids["4"]

	h, err := f.s.trackerHost()
	if err != nil {
		t.Fatal(err)
	}
	v, err := h.View(context.Background(), "me", 0)
	if err != nil {
		t.Fatal(err)
	}
	it := v.Items[strings.Replace(item, "-", "#", 1)]
	if it.Body != body || it.Title != "파서 정리" {
		t.Fatalf("body must round-trip byte for byte:\n got %q\nwant %q", it.Body, body)
	}
	reply := v.Comments[strings.Replace(second, "-", "#", 1)]
	if reply.ReplyTo == nil || *reply.ReplyTo != strings.Replace(first, "-", "#", 1) {
		t.Fatalf("$3 must name the first comment: %+v", reply)
	}
	for _, ev := range v.Events {
		if ev.Type == wstracker.TypeItemStatus && ev.Origin != wstracker.OriginAgentAuto {
			t.Fatalf("origin agent-auto must reach the event: %+v", ev)
		}
		if ev.Type == wstracker.TypeItemCreate && ev.Origin != wstracker.OriginAgent {
			t.Fatalf("an event without origin must be the agent's: %+v", ev)
		}
	}

	full := f.trackerQuery(t, map[string]any{"store": "me", "item": item})
	for _, want := range []string{
		item + ` task "파서 정리"`,
		"status doing", "due 2026-11-20", "project ws",
		"body:\n" + body + "\n",
		`ada doing (by ada), note "시작"`,
		first + " ada at", "  첫 댓글\n  두 줄",
		second + " ada at", "reply to " + first,
	} {
		if !strings.Contains(full, want) {
			t.Fatalf("item view missing %q:\n%s", want, full)
		}
	}
	mine := f.trackerQuery(t, map[string]any{"store": "me"})
	for _, want := range []string{"store me, viewer ada (Ada Kim), admin", "today: my work 1", "my assignments:\n- " + item + " P", "파서 정리: ada doing (by ada)"} {
		if !strings.Contains(mine, want) {
			t.Fatalf("mine scope missing %q:\n%s", want, mine)
		}
	}
	// The item ID's # form is accepted too.
	if alt := f.trackerQuery(t, map[string]any{"store": "me", "item": strings.Replace(item, "-", "#", 1)}); !strings.Contains(alt, "파서 정리") {
		t.Fatalf("the stored id form must find the item: %s", alt)
	}
	if none := f.trackerQuery(t, map[string]any{"store": "me", "scope": "all", "kind": "topic"}); !strings.Contains(none, "no rows pass the filters") {
		t.Fatalf("a filter that matches nothing must say so: %s", none)
	}
	// todo is no filter value: the store folds a todo create as a task.
	if bad := f.trackerRefused(t, "tracker.query", map[string]any{"payload": map[string]any{"store": "me", "kind": "todo"}}); !strings.Contains(bad, "must be one of task, topic") {
		t.Fatalf("a todo kind filter must be refused with the two kinds: %s", bad)
	}
	if hit := f.trackerQuery(t, map[string]any{"store": "me", "scope": "all", "text": "따옴표"}); !strings.Contains(hit, item+" [task doing]") {
		t.Fatalf("a text filter must match the body: %s", hit)
	}

	resp := f.call(t, "tracker.query", map[string]any{"payload": map[string]any{"store": "me"}, "format": "json"})
	var state struct {
		Store struct {
			Name string `json:"name"`
		} `json:"store"`
		Items map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(toolText(t, resp)), &state); err != nil || state.Store.Name != "me" || len(state.Items) != 1 {
		t.Fatalf("format json must return the store's view state: %v %s", err, toolText(t, resp))
	}
}

// The priority line names where the levels are changed, so an agent asked
// "where do I change the priorities" can answer from the query alone.
func TestTrackerQueryPriorityLineNamesWhereLevelsChange(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	out := f.trackerQuery(t, map[string]any{"store": "me"})
	want := "\npriority: 0 Notice, 1 Task (default), 2 Urgent; an admin changes the levels in the store settings of the dashboard's Team tab, or with tk:roles.set_priority_levels\n"
	if !strings.Contains(out, want) {
		t.Fatalf("priority line must carry the hint %q:\n%s", want, out)
	}
}

// A store with labels prints them after the priority line, with how an item
// takes one; rows and the item detail name an item's labels, and q's
// label: filters by name.
func TestTrackerQueryShowsLabels(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	out := f.trackerUpdate(t, `<tk:store name="me">
<tk:roles.add_label name="docs" color="#3366cc"/>
<tk:roles.add_label name="bug" color="#d14343"/>
<tk:item.create kind="task" title="Write guide" labels="$1 $2"/>
<tk:item.create kind="task" title="Plain"/>
</tk:store>`)
	ids := trackerWrote(t, out)
	docs, bug, guide, plain := trackerAlias(ids["1"]), trackerAlias(ids["2"]), ids["3"], ids["4"]
	q := f.trackerQuery(t, map[string]any{"store": "me", "scope": "all"})
	line := "\nlabels: " + docs + " docs #3366cc, " + bug + " bug #d14343; " + trackerLabelsHint + "\n"
	if !strings.Contains(q, line) {
		t.Fatalf("query must print the labels line %q:\n%s", line, q)
	}
	if !strings.Contains(q, "Write guide (labels docs, bug)") {
		t.Fatalf("the row must name its labels:\n%s", q)
	}
	if one := f.trackerQuery(t, map[string]any{"store": "me", "item": guide}); !strings.Contains(one, ", labels docs, bug\n") {
		t.Fatalf("the item detail must name its labels:\n%s", one)
	}
	got := f.trackerQuery(t, map[string]any{"store": "me", "scope": "all", "q": "label:BUG"})
	if !strings.Contains(got, guide+" [") || strings.Contains(got, plain+" [") {
		t.Fatalf("label:BUG must list only the labelled item:\n%s", got)
	}
}

// payload.q takes the dashboard search grammar and AND-s with kind,
// status, project, and text; text keeps its substring meaning; a malformed
// known key is refused naming the token.
func TestTrackerQuerySearchField(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	out := f.trackerUpdate(t, `<tk:store name="me">
<tk:item.create kind="task" title="Ship release" due="2026-11-20" priority="2" project="web">body</tk:item.create>
<tk:item.assign item="$1" people="ada"/>
<tk:item.status item="$1" status="doing"/>
<tk:comment.add item="$1">needs a changelog</tk:comment.add>
<tk:item.create kind="task" title="Write docs" due="2026-12-01" project="web">a:b in the body</tk:item.create>
<tk:item.create kind="task" title="Gone"/>
<tk:item.hide item="$6"/>
</tk:store>`)
	ids := trackerWrote(t, out)
	ship, docs, gone := ids["1"], ids["5"], ids["6"]
	all := func(payload map[string]any) string {
		payload["store"], payload["scope"] = "me", "all"
		return f.trackerQuery(t, payload)
	}
	for _, c := range []struct {
		payload   map[string]any
		hit, miss []string
	}{
		{map[string]any{"q": "status:doing"}, []string{ship}, []string{docs}},
		{map[string]any{"q": "changelog"}, []string{ship}, []string{docs}},
		{map[string]any{"q": `due:<2026-12-01 "ship release"`}, []string{ship}, []string{docs}},
		{map[string]any{"q": "priority:>=2 assignee:ada"}, []string{ship}, []string{docs}},
		{map[string]any{"q": "project:web", "status": "todo"}, []string{docs}, []string{ship}},
		{map[string]any{"q": "project:web", "text": "release"}, []string{ship}, []string{docs}},
		// text is a plain substring: the colon does not start a filter
		{map[string]any{"text": "a:b"}, []string{docs}, []string{ship}},
		// an unknown key is free text
		{map[string]any{"q": "a:b"}, []string{docs}, []string{ship}},
		// a hidden item shows only for status hidden, in q too
		{map[string]any{"q": "gone"}, nil, []string{gone}},
		{map[string]any{"q": "status:hidden"}, []string{gone}, []string{ship, docs}},
	} {
		got := all(c.payload)
		for _, id := range c.hit {
			if !strings.Contains(got, id+" [") {
				t.Errorf("%v must list %s:\n%s", c.payload, id, got)
			}
		}
		for _, id := range c.miss {
			if strings.Contains(got, id+" [") {
				t.Errorf("%v must not list %s:\n%s", c.payload, id, got)
			}
		}
	}
	if none := all(map[string]any{"q": "status:done"}); !strings.Contains(none, "no rows pass the filters") {
		t.Fatalf("a q that matches nothing must say so: %s", none)
	}
	// the mine scope filters its assignment rows by the item's status
	if mine := f.trackerQuery(t, map[string]any{"store": "me", "q": "status:doing"}); !strings.Contains(mine, ship+" P") {
		t.Fatalf("mine with status:doing must list the doing assignment: %s", mine)
	}
	bad := f.trackerRefused(t, "tracker.query", map[string]any{"payload": map[string]any{"store": "me", "q": "ship due:<tomorrow"}})
	if !strings.Contains(bad, `payload.q: search "due:<tomorrow": due takes a date YYYY-MM-DD`) {
		t.Fatalf("a malformed due token must be refused by name: %s", bad)
	}
	if bad := f.trackerRefused(t, "tracker.query", map[string]any{"payload": map[string]any{"q": "ship"}}); !strings.Contains(bad, "need store") {
		t.Fatalf("q without store must be refused: %s", bad)
	}
}

// One flagged command refuses the whole batch with its command position and
// line; the commands before it write nothing.
func TestTrackerUpdateIsAllOrNothing(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	h, err := f.s.trackerHost()
	if err != nil {
		t.Fatal(err)
	}
	before, err := h.View(context.Background(), "me", 0)
	if err != nil {
		t.Fatal(err)
	}
	beforeEvents := len(before.Events)
	text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:store name="me"><tk:item.create kind="task" title="kept out"/>

<tk:item.hide item="$1"/>
<tk:item.hide item="$1"/>
</tk:store>`})
	if !strings.Contains(text, "command 3 <tk:item.hide> (line 4): the store would flag this write:") || !strings.Contains(text, "nothing was applied") {
		t.Fatalf("the refusal must cite the flagged command and its line:\n%s", text)
	}
	// Under any status: a partial write of command 1 alone leaves a visible
	// item, of commands 1 and 2 a hidden one.
	for _, status := range []string{"", "hidden"} {
		payload := map[string]any{"store": "me", "scope": "all"}
		if status != "" {
			payload["status"] = status
		}
		if out := f.trackerQuery(t, payload); strings.Contains(out, "kept out") {
			t.Fatalf("a refused batch must write nothing (status %q):\n%s", status, out)
		}
	}
	after, err := h.View(context.Background(), "me", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Items) != 0 || len(after.Events) != beforeEvents {
		t.Fatalf("a refused batch must write nothing: %d items, %d events (was %d)", len(after.Items), len(after.Events), beforeEvents)
	}
}

// Every problem the batch has is listed at once, before any write, with the
// syntax and the command table so one retry can fix the call.
func TestTrackerUpdateListsEveryProblem(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:register store="x" member="a"/>
<tk:store name="me">
<tk:item.create kind="epic" title="t" origin="human"/>
<tk:item.assign item="$1" people="a" levels="2"/>
<tk:comment.add item="$9"></tk:comment.add>
<tk:item.edit item="$3"/>
<tk:item.close item="maya"/>
</tk:store>
<tk:store name="other"><tk:item.hide store="me" item="$1"/></tk:store>
<tk:item.hide item="$1"/>`})
	for _, want := range []string{
		"command 1 <tk:register> (line 1): reaches the network and cannot be rolled back, so it stands alone in its batch",
		"command 2 <tk:item.create> (line 3) kind: must be one of task, todo, topic",
		"command 2 <tk:item.create> (line 3) origin: must be one of agent, agent-auto",
		"command 3 <tk:item.assign> (line 4): levels stands alone; beside people or rules",
		"command 4 <tk:comment.add> (line 5) item: $N names an earlier command of this batch by position",
		"command 4 <tk:comment.add> (line 5) body: must be non-empty",
		"command 5 <tk:item.edit> (line 6) item: $3 names command 3, which creates no item; $N must name a <tk:item.create>",
		"command 5 <tk:item.edit> (line 6): changes nothing",
		"command 6 <tk:item.close> (line 7) item: must be an item ID such as maya.pc-5, or $N",
		"<tk:store> (line 9): a second <tk:store> group; one batch writes to one store, so it holds one group",
		"command 7 <tk:item.hide> (line 9): unknown attribute store (known: origin, item)",
		`command 8 <tk:item.hide> (line 10): goes inside a <tk:store name="..."> group`,
		"## Commands", "`tk:item.create`",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("refusal missing %q:\n%s", want, text)
		}
	}
	if strings.Count(text, "## Command syntax") != 1 || strings.Index(text, "## Command syntax") > strings.Index(text, "## Commands") {
		t.Fatalf("the refusal must carry the syntax once, ahead of the command table:\n%s", text)
	}
	// The group names a registered store, once for all its commands; the
	// network commands stay at the root.
	text = f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:store name="other"><tk:item.hide item="ada.pc-1"/><tk:item.hide item="ada.pc-2"/></tk:store>`})
	if want := `- <tk:store> (line 1) name: "other" is not a registered store (registered: me)` + "\n"; strings.Count(text, want) != 1 {
		t.Fatalf("refusal must name the unregistered store once:\n%s", text)
	}
	text = f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:store name="me"><tk:sync/></tk:store>`})
	if !strings.Contains(text, "command 1 <tk:sync> (line 1): goes at the root, outside <tk:store> (line 1)") {
		t.Fatalf("refusal must keep tk:sync at the root:\n%s", text)
	}
	text = f.trackerRefused(t, "tracker.update", map[string]any{"commands": map[string]any{"x": 1}})
	if !strings.Contains(text, "commands: must be a string") {
		t.Fatalf("a JSON commands value must be refused: %s", text)
	}
	text = f.trackerRefused(t, "tracker.query", map[string]any{"payload": map[string]any{"store": "me", "scope": "everything", "item": "x-1"}, "format": "json"})
	for _, want := range []string{"payload.scope: must be one of", "payload.item: shows one item", "## Query", "tracker.query payload:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("query refusal missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "## Commands") {
		t.Fatalf("the query refusal must carry the Query section only:\n%s", text)
	}
}

// A write reaches the other clone through tk:sync and the other clone's
// write reaches this one through the fetch before a query.
func TestTrackerSyncCommandPushesAndQueryFetches(t *testing.T) {
	f := newTrackerLeadFixture(t)
	ctx := context.Background()
	remote := initTrackerRemote(t)
	out := f.trackerUpdate(t, `<tk:register store="team" member="ada" remote="`+remote+`" given="Ada"/>`)
	if !strings.Contains(out, "registered team: created") {
		t.Fatalf("register on an empty remote must create: %s", out)
	}
	bo := trackerOtherMachine(t)
	if _, err := bo.Register(ctx, wstrackerhost.RegisterRequest{ID: "team", Remote: remote, Member: "bo", Profile: &wstracker.Profile{Given: "Bo"}}); err != nil {
		t.Fatalf("bo joins: %v", err)
	}
	ids := trackerWrote(t, f.trackerUpdate(t, `<tk:store name="team"><tk:item.create kind="task" title="for bo"/></tk:store>`))
	if out := f.trackerUpdate(t, `<tk:sync store="team"/>`); !strings.Contains(out, "team: pushed") {
		t.Fatalf("tk:sync must push the write: %s", out)
	}
	if _, err := bo.Sync(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	v, err := bo.View(ctx, "team", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v.Items[strings.Replace(ids["1"], "-", "#", 1)]; !ok {
		t.Fatalf("bo must see ada's synced write")
	}

	payload, _ := json.Marshal(map[string]any{"kind": "task", "title": "from bo"})
	if _, err := bo.Dispatch(ctx, "team", []wsledger.NewEvent{{Type: wstracker.TypeItemCreate, Payload: payload}}); err != nil {
		t.Fatal(err)
	}
	if _, err := bo.Sync(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if all := f.trackerQuery(t, map[string]any{"store": "team", "scope": "all"}); !strings.Contains(all, "from bo") {
		t.Fatalf("a query must fetch the other clone's write first:\n%s", all)
	}
	if out := f.trackerUpdate(t, `<tk:sync/>`); !strings.Contains(out, "team: ") {
		t.Fatalf("tk:sync without store must sync every store: %s", out)
	}
}

func TestTrackerManualSectionsRenderOnceAndNeverForSpawnedKeys(t *testing.T) {
	syntaxHeadings := func(text string) int {
		n := 0
		for _, line := range strings.Split(text, "\n") {
			if strings.HasSuffix(line, "# Command syntax") {
				n++
			}
		}
		return n
	}
	t.Run("whiteboard only", func(t *testing.T) {
		f := newWhiteboardFixture(t)
		f.tune(t, map[string]any{"value": "on"})
		m := f.manualText(t, f.key)
		if syntaxHeadings(m) != 1 || !strings.Contains(m, "### Whiteboard") || strings.Contains(m, "### Tracker") {
			t.Fatalf("whiteboard only: syntax once, whiteboard, no tracker:\n%s", m)
		}
	})
	t.Run("tracker only", func(t *testing.T) {
		f := newWhiteboardFixture(t)
		f.tuneTracker(t, map[string]any{"value": "on"})
		m := f.manualText(t, f.key)
		if syntaxHeadings(m) != 1 || strings.Contains(m, "### Whiteboard") || !strings.Contains(m, "### Tracker") ||
			strings.Index(m, "### Command syntax") > strings.Index(m, "### Tracker") {
			t.Fatalf("tracker only: syntax once ahead of the tracker section, no whiteboard:\n%s", m)
		}
		section := m[strings.Index(m, "### Tracker"):]
		if end := strings.Index(section, "\n## "); end >= 0 {
			section = section[:end]
		}
		for _, want := range []string{"#### Recording", "#### Commands", "#### Sync", "`origin=\"agent-auto\"`", "`tk:register`"} {
			if !strings.Contains(section, want) {
				t.Fatalf("tracker section missing %q:\n%s", want, section)
			}
		}
		if strings.Contains(section, "{{") || strings.Contains(section, "\n# Tracker") || !strings.Contains(section, "\n#### Query") {
			t.Fatalf("the tracker section must be rendered, title dropped, headings demoted:\n%s", section)
		}
	})
	t.Run("both", func(t *testing.T) {
		f := newWhiteboardFixture(t)
		f.tune(t, map[string]any{"value": "on"})
		f.tuneTracker(t, map[string]any{"value": "on"})
		for name, m := range map[string]string{
			"workflow_manual": f.manualText(t, f.key),
			"playbook.read":   toolText(t, f.call(t, "playbook.read", map[string]any{"name": workflowManualPlaybookName})),
		} {
			syntax, wb, tk := strings.Index(m, "### Command syntax"), strings.Index(m, "### Whiteboard"), strings.Index(m, "### Tracker")
			if syntaxHeadings(m) != 1 || syntax < 0 || wb < syntax || tk < wb {
				t.Fatalf("both, %s: syntax once, then whiteboard, then tracker:\n%s", name, m)
			}
		}
	})
	t.Run("spawned key", func(t *testing.T) {
		f := newWhiteboardFixture(t)
		f.tuneTracker(t, map[string]any{"value": "on"})
		worker, err := f.s.sessions.mint(f.root, roleLead, f.key)
		if err != nil {
			t.Fatal(err)
		}
		for name, m := range map[string]string{
			"workflow_manual": f.manualText(t, worker),
			"playbook.read":   toolText(t, f.call(t, "playbook.read", map[string]any{"session_key": worker, "name": workflowManualPlaybookName})),
		} {
			if syntaxHeadings(m) != 0 || strings.Contains(m, "### Tracker") || strings.Contains(m, "### Whiteboard") {
				t.Fatalf("knob on, spawned key, %s: neither section renders:\n%s", name, m)
			}
		}
		if lead := f.manualText(t, f.key); !strings.Contains(lead, "### Tracker") {
			t.Fatalf("knob on: the lead's manual must carry the tracker section")
		}
	})
}

// The tracker playbook read alone states the whole input format.
func TestTrackerPlaybookReadAppendsCommandSyntax(t *testing.T) {
	f := newWhiteboardFixture(t)
	text := toolText(t, f.call(t, "playbook.read", map[string]any{"name": trackerPlaybookName}))
	if strings.Count(text, "## Command syntax") != 1 || strings.Index(text, "## Command syntax") < strings.Index(text, "## Sync") ||
		!strings.Contains(text, "`commands` is one string of commands.") {
		t.Fatalf("the tracker playbook must end with the command syntax section once:\n%s", text)
	}
	if strings.Contains(text, "{{") {
		t.Fatalf("the tracker playbook must render its placeholders:\n%s", text)
	}
	lower := strings.ToLower(text)
	for _, host := range []string{"claude", "codex", "devenv"} {
		if strings.Contains(lower, host) {
			t.Fatalf("the tracker playbook must name no host or repository, found %q", host)
		}
	}
}

func TestParseTrackerID(t *testing.T) {
	for in, want := range map[string]string{
		"maya.pc-5":       "maya.pc#5",
		"maya.pc#5":       "maya.pc#5",
		"kim-ho.my-pc-12": "kim-ho.my-pc#12",
	} {
		if got, err := parseTrackerID(in); err != nil || got != want {
			t.Fatalf("parseTrackerID(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"maya", "maya-5", "maya.pc-0", "maya.pc#x", "maya.pc#5!c:1"} {
		if _, err := parseTrackerID(bad); err == nil {
			t.Fatalf("parseTrackerID(%q) must fail", bad)
		}
	}
}

// Every tk: attribute and body is plain text, so an escaped one is decoded:
// a title and project, an item body, a note, and a comment; CDATA stays
// literal.
func TestTrackerUpdateDecodesEscapedText(t *testing.T) {
	var problems wsxmlcmd.Problems
	b := planTrackerBatch(&problems, `<tk:store name="team"><tk:item.create kind="task" title="Parse &lt;key&gt; &amp; value" project="a&#38;b">body &lt;b&gt;</tk:item.create>`+
		`<tk:item.edit item="$1" title="&quot;t&quot;">new &lt;b&gt;</tk:item.edit>`+
		`<tk:item.status item="$1" status="doing">waiting on &lt;key&gt;</tk:item.status>`+
		`<tk:item.close item="$1">done &amp; dusted <![CDATA[&lt;kept&gt;]]></tk:item.close>`+
		`<tk:comment.add item="$1">see &lt;b&gt;</tk:comment.add>`+
		`<tk:comment.edit comment="$5">edit &lt;b&gt;</tk:comment.edit></tk:store>`,
		[]wstrackerhost.StoreEntry{{ID: "team"}})
	if len(problems.List) > 0 {
		t.Fatalf("problems: %q", problems.List)
	}
	want := []map[string]any{
		{"title": "Parse <key> & value", "project": "a&b", "body": "body <b>"},
		{"title": `"t"`, "body": "new <b>"},
		{"note": "waiting on <key>"},
		{"note": "done & dusted &lt;kept&gt;"},
		{"body": "see <b>"},
		{"body": "edit <b>"},
	}
	for i, fields := range want {
		for k, v := range fields {
			if got := b.events[i].payload[k]; got != v {
				t.Errorf("event %d (%s) %s = %q, want %q", i+1, b.events[i].typ, k, got, v)
			}
		}
	}
}

// trackerPlan plans commands for the store team with ada as its member and
// fails the test on any problem.
func trackerPlan(t *testing.T, commands string) *trackerBatch {
	t.Helper()
	var problems wsxmlcmd.Problems
	b := planTrackerBatch(&problems, `<tk:store name="team">`+commands+`</tk:store>`, []wstrackerhost.StoreEntry{{ID: "team", Member: "ada"}})
	if len(problems.List) > 0 {
		t.Fatalf("problems: %q", problems.List)
	}
	return b
}

// stage is retired: the writer refuses it on a create or an edit and as a
// clear field, and item.status takes idea.
func TestTrackerUpdateRefusesStage(t *testing.T) {
	b := trackerPlan(t, `<tk:item.create kind="task" title="t"/><tk:item.status item="$1" status="idea"/>`)
	if got := b.events[1].payload["status"]; got != "idea" {
		t.Fatalf("item.status idea = %v", got)
	}
	var problems wsxmlcmd.Problems
	planTrackerBatch(&problems, `<tk:store name="team"><tk:item.create kind="task" title="t" stage="review"/>`+
		`<tk:item.edit item="$1" stage="review"/>`+
		`<tk:item.edit item="$1" clear="stage"/></tk:store>`, []wstrackerhost.StoreEntry{{ID: "team", Member: "ada"}})
	for _, want := range []string{
		`command 1 <tk:item.create> (line 1): unknown attribute stage (known: `,
		`command 2 <tk:item.edit> (line 1): unknown attribute stage (known: `,
		`command 3 <tk:item.edit> (line 1) clear: must list body or due; got "stage"`,
	} {
		if !slices.ContainsFunc(problems.List, func(p string) bool { return strings.HasPrefix(p, want) }) {
			t.Errorf("problems missing %q:\n%q", want, problems.List)
		}
	}
}

// An item.assign without completion gets it stored: assignee when the
// writer is the only target, assigner otherwise; an explicit one is kept.
func TestTrackerUpdateCompletionDefault(t *testing.T) {
	for attrs, want := range map[string]string{
		`people="ada"`:                       "assignee",
		`people="ada ada"`:                   "assignee",
		`people="bo"`:                        "assigner",
		`people="ada bo"`:                    "assigner",
		`rules="ada:1"`:                      "assigner",
		`people="ada" rules="bo:*"`:          "assigner",
		`levels="1"`:                         "assigner",
		`people="bo" completion="assignee"`:  "assignee",
		`people="ada" completion="assigner"`: "assigner",
	} {
		b := trackerPlan(t, `<tk:item.assign item="ada.pc-1" `+attrs+`/>`)
		if got := b.events[0].payload["completion"]; got != want {
			t.Errorf("%s: completion = %v, want %s", attrs, got, want)
		}
	}
}

// A topic the batch creates and does not assign is self-assigned right
// after its create (the writer holds it); a task, or a create still written
// as todo, gets none (the store holds it for its creator), nor does a topic
// the batch assigns by $N, and every $N still names its command's event.
func TestTrackerUpdateSelfAssignsCreates(t *testing.T) {
	b := trackerPlan(t, `<tk:item.create kind="task" title="a"/>`+
		`<tk:item.create kind="todo" title="b" parent="$1" origin="agent-auto"/>`+
		`<tk:item.create kind="topic" title="c"/>`+
		`<tk:item.create kind="topic" title="d"/>`+
		`<tk:item.assign item="$4" people="bo"/>`+
		`<tk:comment.add item="$2" refs="$1 $4">hi</tk:comment.add>`)
	next := wsledger.EventID{Member: "ada", Clone: "pc", Seq: 10}
	evs, err := b.newEvents(next)
	if err != nil {
		t.Fatal(err)
	}
	id := func(seq int) string { e := next; e.Seq = seq; return e.String() }
	want := []struct {
		typ, origin string
		payload     map[string]any
	}{
		{wstracker.TypeItemCreate, wstracker.OriginAgent, map[string]any{"kind": "task", "title": "a"}},
		{wstracker.TypeItemCreate, wstracker.OriginAgentAuto, map[string]any{"kind": "todo", "title": "b", "parent": id(10)}},
		{wstracker.TypeItemCreate, wstracker.OriginAgent, map[string]any{"kind": "topic", "title": "c"}},
		{wstracker.TypeItemAssign, wstracker.OriginAgent, map[string]any{"item": id(12), "mode": "chosen", "people": []any{"ada"}, "completion": "assignee"}},
		{wstracker.TypeItemCreate, wstracker.OriginAgent, map[string]any{"kind": "topic", "title": "d"}},
		{wstracker.TypeItemAssign, wstracker.OriginAgent, map[string]any{"item": id(14), "mode": "chosen", "people": []any{"bo"}, "completion": "assigner"}},
		{wstracker.TypeCommentAdd, wstracker.OriginAgent, map[string]any{"item": id(11), "refs": []any{id(10), id(14)}, "body": "hi"}},
	}
	if len(evs) != len(want) {
		t.Fatalf("got %d events, want %d", len(evs), len(want))
	}
	for i, w := range want {
		var got map[string]any
		if err := json.Unmarshal(evs[i].Payload, &got); err != nil {
			t.Fatal(err)
		}
		if evs[i].Type != w.typ || evs[i].Origin != w.origin || !reflect.DeepEqual(got, w.payload) {
			t.Errorf("event %d = %s %s %s, want %s %s %v", i, evs[i].Type, evs[i].Origin, evs[i].Payload, w.typ, w.origin, w.payload)
		}
	}
	if !b.events[3].selfAssign || b.events[3].cmd.Pos != 3 || b.events[5].selfAssign {
		t.Fatalf("the topic's self-assignment is marked and carries its create's command")
	}
}

// Through the tool: a created task is written alone and held by its
// creator at todo, item.status idea in the same batch makes it an idea, and
// that holding cannot be unassigned; assigning someone else replaces it.
func TestTrackerUpdateCreatorHoldsAnUnassignedTask(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	out := f.trackerUpdate(t, `<tk:store name="me">
<tk:item.create kind="task" title="mine"/>
<tk:item.create kind="task" title="later"/>
<tk:item.status item="$2" status="idea"/>
</tk:store>`)
	ids := trackerWrote(t, out)
	mine, later := ids["1"], ids["2"]
	if !strings.Contains(out, "me: wrote 3 events") || strings.Contains(out, "self-assignment") {
		t.Fatalf("a created task gets no self-assignment:\n%s", out)
	}
	if got := f.trackerQuery(t, map[string]any{"store": "me", "item": mine}); !strings.Contains(got, "status todo") || !strings.Contains(got, "ada todo (by ada)") {
		t.Fatalf("a created task is its creator's at todo:\n%s", got)
	}
	if got := f.trackerQuery(t, map[string]any{"store": "me", "scope": "all", "status": "idea"}); !strings.Contains(got, later+" [task idea]") || strings.Contains(got, mine) {
		t.Fatalf("the idea item alone shows status idea:\n%s", got)
	}
	text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:store name="me"><tk:item.unassign item="` + mine + `" assignee="ada"/></tk:store>`})
	if !strings.Contains(text, "the store would flag this write:") || !strings.Contains(text, "ada has no live or pending assignment on ") {
		t.Fatalf("the creator's holding cannot be unassigned:\n%s", text)
	}

	// Assigning someone else replaces the creator's holding.
	remote := initTrackerRemote(t)
	f.trackerUpdate(t, `<tk:register store="team" member="ada" remote="`+remote+`"/>`)
	bo := trackerOtherMachine(t)
	if _, err := bo.Register(context.Background(), wstrackerhost.RegisterRequest{ID: "team", Remote: remote, Member: "bo", Profile: &wstracker.Profile{Given: "Bo"}}); err != nil {
		t.Fatalf("bo joins: %v", err)
	}
	f.trackerUpdate(t, `<tk:sync store="team"/>`)
	task := trackerWrote(t, f.trackerUpdate(t, `<tk:store name="team">
<tk:item.create kind="task" title="for bo"/>
<tk:item.assign item="$1" people="bo"/>
</tk:store>`))["1"]
	if got := f.trackerQuery(t, map[string]any{"store": "team", "item": task}); !strings.Contains(got, "- bo pending (by ada") || strings.Contains(got, "ada todo (by ada)") {
		t.Fatalf("assigning bo replaces the creator's holding:\n%s", got)
	}
}

// A wiki sync warning carries git's error text, which can name the remote
// with its credentials; the sync report must not echo them.
func TestTrackerSyncReportRedactsWarningCredentials(t *testing.T) {
	rep := wsledger.SyncReport{Warnings: []wsledger.Warning{{
		Ref:    "refs/ws/wiki/docs",
		Kind:   wsledger.WarnWikiSyncFailed,
		Detail: "ledger: wiki push: fatal: unable to access 'https://user:s3cr3t-token@example.com/store.git/': 403",
	}}}
	got := formatTrackerSyncReport(rep)
	if strings.Contains(got, "s3cr3t-token") || strings.Contains(got, "user:") {
		t.Fatalf("sync report leaks the remote's credentials: %q", got)
	}
	if !strings.Contains(got, "warning: refs/ws/wiki/docs "+wsledger.WarnWikiSyncFailed) || !strings.Contains(got, "https://***@example.com/store.git/") {
		t.Fatalf("sync report = %q, want the warning with its redacted remote", got)
	}
}

// trackerEntry returns store id's registry entry through the fixture's
// host, or ok false when it is not registered.
func (f *whiteboardFixture) trackerEntry(t *testing.T, id string) (wstrackerhost.StoreEntry, bool) {
	t.Helper()
	h, err := f.s.trackerHost()
	if err != nil {
		t.Fatal(err)
	}
	e, err := h.Entry(id)
	if errors.Is(err, wstrackerhost.ErrNotRegistered) {
		return wstrackerhost.StoreEntry{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return e, true
}

// tk:unregister of a store with a remote is refused while it holds an
// unpushed write, confirm or not; once synced it removes the entry and the
// clone, confirm ignored, leaves the remote, and tk:register restores it.
func TestTrackerUnregisterRemoteStore(t *testing.T) {
	f := newTrackerLeadFixture(t)
	remote := initTrackerRemote(t)
	f.trackerUpdate(t, `<tk:register store="team" member="ada" remote="`+remote+`" given="Ada"/>`)
	f.trackerUpdate(t, `<tk:store name="team"><tk:item.create kind="task" title="kept"/></tk:store>`)
	for _, commands := range []string{`<tk:unregister store="team"/>`, `<tk:unregister store="team" confirm="team"/>`} {
		text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": commands})
		for _, want := range []string{`"team" was not unregistered`, "1 own event that is not pushed yet", "sync the store first"} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s with an unpushed write must be refused, missing %q: %s", commands, want, text)
			}
		}
		if _, ok := f.trackerEntry(t, "team"); !ok {
			t.Fatalf("%s: a refused unregister must keep the store", commands)
		}
	}
	e, _ := f.trackerEntry(t, "team")
	f.trackerUpdate(t, `<tk:sync store="team"/>`)
	out := f.trackerUpdate(t, `<tk:unregister store="team" confirm="not-the-id"/>`)
	for _, want := range []string{"unregistered team: removed from this machine", "the remote " + remote + " was left as it is", "<tk:register> can register the store again"} {
		if !strings.Contains(out, want) {
			t.Fatalf("unregister answer missing %q: %s", want, out)
		}
	}
	if _, ok := f.trackerEntry(t, "team"); ok {
		t.Fatalf("an unregistered store must leave the registry")
	}
	if _, err := os.Stat(e.Dir); !os.IsNotExist(err) {
		t.Fatalf("an unregistered store's clone must be deleted: %v", err)
	}
	if list := f.trackerQuery(t, nil); strings.Contains(list, "team") {
		t.Fatalf("the store list must drop an unregistered store:\n%s", list)
	}
	if out := f.trackerUpdate(t, `<tk:register store="team" member="ada" remote="`+remote+`"/>`); !strings.Contains(out, "registered team: created, member ada, remote "+remote) {
		// "created": the store's genesis lists ada, its creator.
		t.Fatalf("the remote must be left so the store registers again: %s", out)
	}
	if all := f.trackerQuery(t, map[string]any{"store": "team", "scope": "all"}); !strings.Contains(all, "kept") {
		t.Fatalf("the re-registered store must hold the pushed write:\n%s", all)
	}
}

// A store with no remote holds its only copy here: tk:unregister removes it
// only when confirm types its id, and the refusal says so.
func TestTrackerUnregisterNoRemoteStoreNeedsConfirm(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	e, _ := f.trackerEntry(t, "me")
	for _, commands := range []string{`<tk:unregister store="me"/>`, `<tk:unregister store="me" confirm="you"/>`} {
		text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": commands})
		for _, want := range []string{`"me" was not unregistered`, "no remote", "lost permanently", `<tk:unregister store="me" confirm="me"/>`} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s must be refused, missing %q: %s", commands, want, text)
			}
		}
		if _, ok := f.trackerEntry(t, "me"); !ok {
			t.Fatalf("%s: a refused unregister must keep the store", commands)
		}
	}
	out := f.trackerUpdate(t, `<tk:unregister store="me" confirm="me"/>`)
	if !strings.Contains(out, "unregistered me: removed from this machine, and its data deleted") {
		t.Fatalf("a confirmed unregister answer: %s", out)
	}
	if _, ok := f.trackerEntry(t, "me"); ok {
		t.Fatalf("a confirmed unregister must leave the registry")
	}
	if _, err := os.Stat(e.Dir); !os.IsNotExist(err) {
		t.Fatalf("a confirmed unregister must delete the clone: %v", err)
	}
}

// tk:relabel sets and clears the machine-local label and refuses a bad one.
func TestTrackerRelabel(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	if out := f.trackerUpdate(t, `<tk:relabel store="me" label=" Mine "/>`); !strings.Contains(out, `relabeled me: label "Mine"`) {
		t.Fatalf("relabel answer: %s", out)
	}
	if list := f.trackerQuery(t, nil); !strings.Contains(list, "- me (Mine), member ada") {
		t.Fatalf("the store list must show the new label:\n%s", list)
	}
	if out := f.trackerUpdate(t, `<tk:relabel store="me" label=""/>`); !strings.Contains(out, "relabeled me: label cleared") {
		t.Fatalf("an empty label must clear it: %s", out)
	}
	if list := f.trackerQuery(t, nil); !strings.Contains(list, "- me, member ada") {
		t.Fatalf("a cleared label must show the id alone:\n%s", list)
	}
	text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:relabel store="me" label="` + strings.Repeat("x", wstrackerhost.MaxLabelLen+1) + `"/>`})
	if !strings.Contains(text, "at most 80 characters") || !strings.Contains(text, "Nothing changed") {
		t.Fatalf("a too-long label must be refused: %s", text)
	}
	if e, _ := f.trackerEntry(t, "me"); e.Label != "" {
		t.Fatalf("a refused relabel must change nothing: %q", e.Label)
	}
	text = f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:relabel store="me"/>`})
	if !strings.Contains(text, `label: required (label="" clears the label back to the store id)`) {
		t.Fatalf("relabel without label must be refused: %s", text)
	}
}

// tk:hide and tk:unhide flip the machine-local hidden flag; a hidden store
// keeps answering every tracker read and write.
func TestTrackerHideUnhide(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	if out := f.trackerUpdate(t, `<tk:hide store="me"/>`); !strings.Contains(out, "hid me:") {
		t.Fatalf("hide answer: %s", out)
	}
	if e, _ := f.trackerEntry(t, "me"); !e.Hidden {
		t.Fatalf("hide must set the registry flag")
	}
	if list := f.trackerQuery(t, nil); !strings.Contains(list, "- me, member ada, hidden on the dashboard, no remote") {
		t.Fatalf("the store list must keep a hidden store, marked:\n%s", list)
	}
	resp := f.call(t, "tracker.query", map[string]any{"format": "json"})
	var list struct {
		Stores []struct {
			ID     string `json:"id"`
			Hidden bool   `json:"hidden"`
		} `json:"stores"`
	}
	if err := json.Unmarshal([]byte(toolText(t, resp)), &list); err != nil || len(list.Stores) != 1 || !list.Stores[0].Hidden {
		t.Fatalf("the JSON store list must mark a hidden store: %v %s", err, toolText(t, resp))
	}
	f.trackerUpdate(t, `<tk:store name="me"><tk:item.create kind="task" title="still written"/></tk:store>`)
	if all := f.trackerQuery(t, map[string]any{"store": "me", "scope": "all"}); !strings.Contains(all, "still written") {
		t.Fatalf("a hidden store must keep answering reads and writes:\n%s", all)
	}
	if out := f.trackerUpdate(t, `<tk:unhide store="me"/>`); !strings.Contains(out, "unhid me:") {
		t.Fatalf("unhide answer: %s", out)
	}
	if e, _ := f.trackerEntry(t, "me"); e.Hidden {
		t.Fatalf("unhide must clear the registry flag")
	}
	if list := f.trackerQuery(t, nil); strings.Contains(list, "hidden") {
		t.Fatalf("an unhidden store must not be marked:\n%s", list)
	}
}

// The machine-local store commands name a registered store and stand alone
// in their batch.
func TestTrackerLocalStoreCommandsRefuseUnknownStoresAndCompanions(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	for _, commands := range []string{`<tk:unregister store="nope"/>`, `<tk:relabel store="nope" label="x"/>`, `<tk:hide store="nope"/>`, `<tk:unhide store="nope"/>`} {
		text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": commands})
		if !strings.Contains(text, `store: "nope" is not a registered store (registered: me)`) {
			t.Errorf("%s must be refused naming the registered stores: %s", commands, text)
		}
	}
	text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:hide store="me"/><tk:relabel store="me" label="x"/>`})
	if !strings.Contains(text, "command 1 <tk:hide> (line 1): changes this machine's store registry, not the store, and cannot be rolled back with a batch, so it stands alone in its batch") {
		t.Fatalf("a store command must stand alone: %s", text)
	}
	if e, _ := f.trackerEntry(t, "me"); e.Hidden || e.Label != "" {
		t.Fatalf("a refused batch must change nothing: %+v", e)
	}
}
