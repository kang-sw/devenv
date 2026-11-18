package mcp

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

const profileKnobKey = "workflow.profile"

// withOrchestrateWhitelist swaps orchestrateHarnessWhitelist for one test.
func withOrchestrateWhitelist(t *testing.T, harnesses ...string) {
	t.Helper()
	prev := orchestrateHarnessWhitelist
	orchestrateHarnessWhitelist = map[string]bool{}
	for _, h := range harnesses {
		orchestrateHarnessWhitelist[h] = true
	}
	t.Cleanup(func() { orchestrateHarnessWhitelist = prev })
}

func countWarningLines(text string) int {
	n := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "warning: ") {
			n++
		}
	}
	return n
}

func TestWorkflowProfileKnobRegistration(t *testing.T) {
	f := newWhiteboardFixture(t)
	if wsconfig.GlobalOnly(wsconfig.ItemWorkflowProfile) {
		t.Fatalf("workflow.profile must resolve through every scope")
	}
	entry := registryEntryByKey(wsconfig.ItemWorkflowProfile)
	if entry.Key != profileKnobKey {
		t.Fatalf("registry row missing for %s", profileKnobKey)
	}
	if !entry.HarnessBucketed() || !entry.RequiresLeadAuthority || !entry.NoAgentVisible || !entry.ResolverBacked {
		t.Fatalf("profile entry must be harness-bucketed, lead-only, agentless-visible, resolver-backed: %+v", entry)
	}
	assertFieldEnum(t, entry.ValueFields, "value", []string{"direct", "orchestrate"})
	for _, k := range []string{profileKnobKey, profileKnobKey + ".claude", profileKnobKey + ".codex", profileKnobKey + ".pi", profileKnobKey + ".all"} {
		if got := wsconfig.DefaultScope(k); got != wsconfig.ScopeGlobal {
			t.Fatalf("default scope of %s = %q, want global", k, got)
		}
	}
	if got := builtinConfigDefaults()[wsconfig.ItemWorkflowProfile]; got != "direct" {
		t.Fatalf("builtin default = %q, want direct", got)
	}

	catalog := parseTuningCatalogResponse(t, f.call(t, "config.list", map[string]any{"format": "json"}))
	knob := requireTuningKnob(t, catalog, profileKnobKey)
	assertFieldEnum(t, knob.ValueFields, "value", []string{"direct", "orchestrate"})
	assertFieldEnum(t, knob.SelectorFields, "harness", []string{"claude", "codex", "pi", "*"})
	if got := mustMarshalJSON(t, knob.Current); !strings.Contains(got, `"value":"direct"`) || !strings.Contains(got, `"scope":"builtin"`) {
		t.Fatalf("catalog current value must be the builtin direct: %s", got)
	}
}

func TestWorkflowProfileKnobWriteEachValue(t *testing.T) {
	withOrchestrateWhitelist(t, "claude", "codex", "pi")
	f := newWhiteboardFixture(t)
	for _, value := range []string{"orchestrate", "direct"} {
		resp := toolText(t, f.call(t, "config.tune", map[string]any{"key": profileKnobKey, "value": value, "harness": "codex"}))
		if !strings.Contains(resp, "workflow.profile.codex: "+value+" [scope:global]") {
			t.Fatalf("write %s: %s", value, resp)
		}
		if v := readStoredOverride(t, wsconfig.ScopeGlobal, f.root, profileKnobKey+".codex"); v != value {
			t.Fatalf("stored codex bucket = %q, want %s", v, value)
		}
	}
	if resp := f.call(t, "config.tune", map[string]any{"key": profileKnobKey, "value": "bogus"}); !isToolError(resp) {
		t.Fatalf("an unknown profile value must be rejected: %s", resp)
	}
}

func TestOrchestrateWhitelistWarningHelper(t *testing.T) {
	withOrchestrateWhitelist(t)
	got := orchestrateWhitelistWarning("claude")
	want := "warning: workflow.profile orchestrate is set for claude, which is not verified to support nested subagent spawn (lead -> sub-lead -> worker -> delegate -> explore); sessions on it may fail to run orchestrated flows.\n"
	if got != want {
		t.Fatalf("single-harness warning:\n got %q\nwant %q", got, want)
	}
	for _, bucket := range []string{"*", "all"} {
		got = orchestrateWhitelistWarning(bucket)
		if !strings.Contains(got, "for claude, codex, pi, which are not verified") || !strings.Contains(got, "sessions on them may fail") {
			t.Fatalf("%s warning must name every harness off the whitelist: %q", bucket, got)
		}
	}

	withOrchestrateWhitelist(t, "claude")
	if got = orchestrateWhitelistWarning("claude"); got != "" {
		t.Fatalf("whitelisted harness must not warn: %q", got)
	}
	if got = orchestrateWhitelistWarning("*"); !strings.Contains(got, "for codex, pi, which are") || strings.Contains(got, "claude") {
		t.Fatalf("* must name only the harnesses off the whitelist: %q", got)
	}

	withOrchestrateWhitelist(t, "claude", "codex", "pi")
	if got = orchestrateWhitelistWarning("*"); got != "" {
		t.Fatalf("fully whitelisted * must not warn: %q", got)
	}
}

func TestWorkflowProfileTuneWarning(t *testing.T) {
	f := newWhiteboardFixture(t)
	tune := func(args map[string]any) string {
		args["key"] = profileKnobKey
		return toolText(t, f.call(t, "config.tune", args))
	}

	withOrchestrateWhitelist(t)
	resp := tune(map[string]any{"value": "orchestrate", "harness": "codex"})
	if !strings.Contains(resp, "warning: workflow.profile orchestrate is set for codex, which is not verified") || countWarningLines(resp) != 1 {
		t.Fatalf("off-whitelist single harness must warn exactly once: %s", resp)
	}
	resp = tune(map[string]any{"value": "orchestrate", "harness": "*"})
	if !strings.Contains(resp, "for claude, codex, pi, which are not verified") || countWarningLines(resp) != 1 {
		t.Fatalf("* with harnesses off the whitelist must warn exactly once: %s", resp)
	}
	for _, args := range []map[string]any{
		{"value": "direct", "harness": "codex"},
		{"value": "direct", "harness": "*"},
		{"reset": true, "harness": "codex"},
	} {
		if resp = tune(args); countWarningLines(resp) != 0 {
			t.Fatalf("%v must not warn: %s", args, resp)
		}
	}

	withOrchestrateWhitelist(t, "codex")
	if resp = tune(map[string]any{"value": "orchestrate", "harness": "codex"}); countWarningLines(resp) != 0 {
		t.Fatalf("whitelisted harness must not warn: %s", resp)
	}
	resp = tune(map[string]any{"value": "orchestrate", "harness": "*"})
	if !strings.Contains(resp, "for claude, pi, which are") || countWarningLines(resp) != 1 {
		t.Fatalf("* must warn for the remaining harnesses only: %s", resp)
	}

	withOrchestrateWhitelist(t, "claude", "codex", "pi")
	if resp = tune(map[string]any{"value": "orchestrate", "harness": "*"}); countWarningLines(resp) != 0 {
		t.Fatalf("fully whitelisted * must not warn: %s", resp)
	}
}
