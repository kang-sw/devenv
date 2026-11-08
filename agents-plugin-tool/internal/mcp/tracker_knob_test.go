package mcp

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

const (
	trackerKnobKey    = "workflow.tracker_enabled"
	whiteboardKnobKey = "workflow.whiteboard_enabled"
)

func (f *whiteboardFixture) tuneTracker(t *testing.T, args map[string]any) {
	t.Helper()
	args["key"] = trackerKnobKey
	if resp := f.call(t, "config.tune", args); isToolError(resp) {
		t.Fatalf("config.tune(%v): %s", args, resp)
	}
}

// trackerOn reads the knob the way the tracker manual section will: through
// the session's resolver with the server's current harness.
func (f *whiteboardFixture) trackerOn(t *testing.T, key string) bool {
	t.Helper()
	resolver := f.s.sessionResolver(key, builtinConfigDefaults())
	on, err := f.s.trackerEnabled(&resolver, key)
	if err != nil {
		t.Fatal(err)
	}
	return on
}

func TestTrackerKnobWriteDefaultsAndCatalog(t *testing.T) {
	f := newWhiteboardFixture(t)
	if wsconfig.GlobalOnly(wsconfig.ItemWorkflowTrackerEnabled) {
		t.Fatalf("workflow.tracker_enabled must resolve through every scope")
	}
	entry := registryEntryByKey(wsconfig.ItemWorkflowTrackerEnabled)
	if !entry.HarnessBucketed() || !entry.RequiresLeadAuthority || !entry.RepoScoped() || !entry.NoAgentVisible || !entry.ResolverBacked {
		t.Fatalf("tracker entry must be harness-bucketed, lead-only, repo-scoped, agentless-visible, resolver-backed: %+v", entry)
	}
	if got, want := entry.RepoKey(), "workflow.tracker_enabled.<claude|codex|pi|all>"; got != want {
		t.Fatalf("RepoKey = %q, want %q", got, want)
	}

	catalog := parseTuningCatalogResponse(t, f.call(t, "config.list", map[string]any{"format": "json"}))
	knob := requireTuningKnob(t, catalog, trackerKnobKey)
	assertFieldEnum(t, knob.ValueFields, "value", []string{"on", "off"})
	assertFieldEnum(t, knob.SelectorFields, "harness", []string{"claude", "codex", "pi", "*"})
	assertFieldEnum(t, knob.SelectorFields, "scope", wsconfig.ScopeSchemaEnum())
	if got := mustMarshalJSON(t, knob.Current); !strings.Contains(got, `"value":"off"`) || !strings.Contains(got, `"scope":"builtin"`) {
		t.Fatalf("catalog current value must be the builtin off: %s", got)
	}
	if !knob.RepoScope || knob.RepoKey != "workflow.tracker_enabled.<claude|codex|pi|all>" {
		t.Fatalf("catalog must offer the repo scope under the bucket keys: %+v", knob)
	}

	// No scope and no harness: global scope, the session's harness bucket.
	resp := toolText(t, f.call(t, "config.tune", map[string]any{"key": trackerKnobKey, "value": "on"}))
	if !strings.Contains(resp, "workflow.tracker_enabled.claude: on [scope:global]") || strings.Contains(resp, "shadowed") {
		t.Fatalf("a scope-less, harness-less write must land in the global claude bucket: %s", resp)
	}
	if v := readStoredOverride(t, wsconfig.ScopeGlobal, f.root, "workflow.tracker_enabled.claude"); v != "on" {
		t.Fatalf("stored global claude bucket = %q", v)
	}
	catalog = parseTuningCatalogResponse(t, f.call(t, "config.list", map[string]any{"format": "json"}))
	if got := mustMarshalJSON(t, requireTuningKnob(t, catalog, trackerKnobKey).Current); !strings.Contains(got, `"value":"on"`) || !strings.Contains(got, `"scope":"global"`) {
		t.Fatalf("catalog current value must reflect the write: %s", got)
	}
	if got := toolText(t, f.call(t, "config.get", map[string]any{"key": trackerKnobKey})); !strings.Contains(got, "workflow.tracker_enabled: on [scope:global]") {
		t.Fatalf("config.get must read the session's harness bucket: %s", got)
	}
	if v := readStoredOverride(t, wsconfig.ScopeGlobal, f.root, whiteboardKnobKey+".claude"); v != "" {
		t.Fatalf("a tracker write must not touch the whiteboard knob, got %q", v)
	}
	resp = toolText(t, f.call(t, "config.tune", map[string]any{"key": trackerKnobKey, "reset": true}))
	if !strings.Contains(resp, "workflow.tracker_enabled.claude: reset [scope:global]") || !strings.Contains(resp, "effective workflow.tracker_enabled: off [scope:builtin]") {
		t.Fatalf("a scope-less reset must clear the global claude bucket and echo global: %s", resp)
	}
	if v := readStoredOverride(t, wsconfig.ScopeGlobal, f.root, "workflow.tracker_enabled.claude"); v != "" {
		t.Fatalf("reset must remove the global claude bucket, got %q", v)
	}

	resp = toolText(t, f.call(t, "config.tune", map[string]any{"key": trackerKnobKey, "value": "on", "harness": "codex"}))
	if !strings.Contains(resp, "workflow.tracker_enabled.codex: on [scope:global]") || !strings.Contains(resp, "shadowed: effective workflow.tracker_enabled: off [scope:builtin]") {
		t.Fatalf("a codex-bucket write from a claude session: %s", resp)
	}
	resp = toolText(t, f.call(t, "config.tune", map[string]any{"key": trackerKnobKey, "value": "on", "harness": "*", "scope": "project"}))
	if !strings.Contains(resp, "workflow.tracker_enabled.all: on [scope:project]") {
		t.Fatalf("harness * must store the all bucket: %s", resp)
	}
}

func TestTrackerKnobWriteRequiresLeadKeyAtEveryScope(t *testing.T) {
	f := newWhiteboardFixture(t)
	delegate, err := f.s.sessions.mint(f.root, roleDelegate, f.key)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range wsconfig.ScopeSchemaEnum() {
		for _, key := range []string{"", delegate} {
			resp := f.call(t, "config.tune", map[string]any{"key": trackerKnobKey, "value": "on", "scope": scope, "session_key": key})
			if !isToolError(resp) {
				t.Fatalf("scope %s, key %q: a write without a lead session_key must be refused: %s", scope, key, resp)
			}
		}
	}
}

func TestTrackerKnobResolvesPerHarness(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.tuneTracker(t, map[string]any{"value": "on", "harness": "claude"})
	if !f.trackerOn(t, f.key) {
		t.Fatalf("a global claude on must resolve on in a claude session")
	}
	f.s.sessionHarness = "codex"
	if f.trackerOn(t, f.key) {
		t.Fatalf("a global claude on must not resolve on in a codex session")
	}
	f.tuneTracker(t, map[string]any{"value": "on", "harness": "*"})
	if !f.trackerOn(t, f.key) {
		t.Fatalf("the all bucket must apply to a codex session with no codex value")
	}
	f.tuneTracker(t, map[string]any{"value": "off", "harness": "codex"})
	if f.trackerOn(t, f.key) {
		t.Fatalf("the codex bucket must beat the all bucket")
	}
}

func TestTrackerKnobSessionScope(t *testing.T) {
	f := newWhiteboardFixture(t)
	other, _ := parseLoginResponse(t, callLogin(t, f.s, 920000, f.root, nil))
	f.tuneTracker(t, map[string]any{"value": "on", "scope": "session"})
	if !f.trackerOn(t, f.key) {
		t.Fatalf("a session on must resolve on for that session")
	}
	if f.trackerOn(t, other) {
		t.Fatalf("a session on must not reach another lead session")
	}
	f.tuneTracker(t, map[string]any{"value": "on", "harness": "*"})
	f.tuneTracker(t, map[string]any{"value": "off", "scope": "session"})
	if f.trackerOn(t, f.key) {
		t.Fatalf("a session off must beat a global all on")
	}
	f.tuneTracker(t, map[string]any{"reset": true, "scope": "session"})
	if !f.trackerOn(t, f.key) {
		t.Fatalf("after the session reset, the global all on must apply")
	}
}

func TestEnableTrackerKnobForSessionWritesSessionBucket(t *testing.T) {
	f := newWhiteboardFixture(t)
	other, _ := parseLoginResponse(t, callLogin(t, f.s, 920000, f.root, nil))
	if f.trackerOn(t, f.key) {
		t.Fatalf("precondition: knob resolves off")
	}
	if err := f.s.enableTrackerKnobForSession(f.key); err != nil {
		t.Fatal(err)
	}
	if v, ok := f.s.sessions.getOverride(f.key, "workflow.tracker_enabled.claude"); !ok || v != "on" {
		t.Fatalf("session claude bucket = %q, %v; want on", v, ok)
	}
	if !f.trackerOn(t, f.key) {
		t.Fatalf("trackerEnabled must read on after the helper")
	}
	if f.trackerOn(t, other) {
		t.Fatalf("another session must stay off")
	}
	if _, ok := f.s.sessions.getOverride(f.key, whiteboardKnobKey+".claude"); ok {
		t.Fatalf("the tracker helper must not write the whiteboard knob")
	}
	resolver := f.s.sessionResolver(f.key, builtinConfigDefaults())
	if on, err := f.s.whiteboardEnabled(&resolver, f.key); err != nil || on {
		t.Fatalf("whiteboard knob must stay off: %v, %v", on, err)
	}
}

func TestEnableTrackerKnobForSessionLeavesExplicitSessionOff(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.tuneTracker(t, map[string]any{"value": "off", "scope": "session"})
	if err := f.s.enableTrackerKnobForSession(f.key); err != nil {
		t.Fatal(err)
	}
	if f.trackerOn(t, f.key) {
		t.Fatalf("an explicit session off must stand")
	}
	// A session all-bucket off stands even when a global harness bucket says off.
	g := newWhiteboardFixture(t)
	g.tuneTracker(t, map[string]any{"value": "off", "harness": "claude"})
	g.tuneTracker(t, map[string]any{"value": "off", "harness": "*", "scope": "session"})
	if err := g.s.enableTrackerKnobForSession(g.key); err != nil {
		t.Fatal(err)
	}
	if g.trackerOn(t, g.key) {
		t.Fatalf("a session all off must stand")
	}
}

func TestEnableTrackerKnobForSessionOverridesNonSessionOff(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.tuneTracker(t, map[string]any{"value": "off", "harness": "claude"})
	if err := f.s.enableTrackerKnobForSession(f.key); err != nil {
		t.Fatal(err)
	}
	if !f.trackerOn(t, f.key) {
		t.Fatalf("a global off must be overridden at session scope")
	}
}

func TestEnableTrackerKnobForSessionNoopWhenAlreadyOn(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.tuneTracker(t, map[string]any{"value": "on"})
	if err := f.s.enableTrackerKnobForSession(f.key); err != nil {
		t.Fatal(err)
	}
	for _, storeKey := range []string{trackerKnobKey, trackerKnobKey + ".claude", trackerKnobKey + ".all"} {
		if _, ok := f.s.sessions.getOverride(f.key, storeKey); ok {
			t.Fatalf("an already-on knob must write no session override, found %s", storeKey)
		}
	}
}
