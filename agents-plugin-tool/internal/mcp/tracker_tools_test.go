package mcp

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wstrackerhost"
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
		if strings.TrimSpace(section) == "" || strings.Contains(section, "{{") {
			t.Fatalf("the tracker section must be non-empty and rendered:\n%s", section)
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
	if strings.Count(text, "## Command syntax") != 1 {
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
