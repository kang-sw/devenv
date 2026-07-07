package mcp

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// writeTemplateVersionFixture writes a skills-root fixture at
// <skillsRoot>/lead-bootstrap/AGENTS.template.md carrying the given tag, and
// points WS_SKILLS_ROOT/WS_RSRC_ROOT at it for the duration of the test (per
// wsrsrc.ResolveRoot's env-override resolution order).
func writeTemplateVersionFixture(t *testing.T, latest int) string {
	t.Helper()
	skillsRoot := t.TempDir()
	mustWrite(t, skillsRoot, filepath.Join("lead-bootstrap", "AGENTS.template.md"),
		fmt.Sprintf("# Template\n\n<!-- Template Version: v%04d -->\n", latest))
	t.Setenv("WS_SKILLS_ROOT", skillsRoot)
	return skillsRoot
}

// TestBootstrapStalenessWarningFiresOnFerrule verifies the warning fires at
// ws.ferrule time when the downstream AGENTS.md tag is behind the shipped
// lead-bootstrap template tag, and names both version numbers plus the
// permanent-silence setter.
func TestBootstrapStalenessWarningFiresOnFerrule(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "AGENTS.md", "# Root\n\n<!-- Template Version: v0001 -->\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	writeTemplateVersionFixture(t, 2)

	s := NewServer(root, "test")
	resp := callLogin(t, s, 1, root, nil)
	text := toolText(t, resp)

	if !strings.Contains(text, "Bootstrap template is stale") {
		t.Fatalf("ferrule response must carry the staleness warning: %s", text)
	}
	if !strings.Contains(text, "v0001") || !strings.Contains(text, "v0002") {
		t.Fatalf("warning must name both installed and latest versions: %s", text)
	}
	if !strings.Contains(text, "config.bootstrap_alarm") {
		t.Fatalf("warning must point to the config.bootstrap_alarm setter: %s", text)
	}
}

// TestBootstrapStalenessWarningFiresOnWorkflowManual verifies the same
// staleness warning fires from ws.workflow_manual, in both the
// FRESH-with-root branch and the CONTINUE branch.
func TestBootstrapStalenessWarningFiresOnWorkflowManual(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "AGENTS.md", "# Root\n\n<!-- Template Version: v0001 -->\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	writeTemplateVersionFixture(t, 2)
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))

	s := NewServer(root, "test")

	freshResp := callToolWithKey(t, s, 1, freshBootstrapKey, "workflow_manual", map[string]any{
		"root": root,
	})
	if !strings.Contains(freshResp, "Bootstrap template is stale") {
		t.Fatalf("workflow_manual FRESH-with-root must carry the staleness warning: %s", freshResp)
	}

	key, _ := parseLoginResponse(t, callLogin(t, s, 2, root, nil))
	continueResp := callToolWithKey(t, s, 3, key, "workflow_manual", nil)
	if !strings.Contains(continueResp, "Bootstrap template is stale") {
		t.Fatalf("workflow_manual CONTINUE must carry the staleness warning: %s", continueResp)
	}
}

// TestBootstrapStalenessWarningSuppressedWhenOff verifies config.bootstrap_alarm
// off (global scope, the item is global-only) silences the warning.
func TestBootstrapStalenessWarningSuppressedWhenOff(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "AGENTS.md", "# Root\n\n<!-- Template Version: v0001 -->\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	writeTemplateVersionFixture(t, 2)

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))

	offResp := callToolOnce(t, s, 2, "config.bootstrap_alarm", map[string]any{
		"session_key": key,
		"value":       "off",
	})
	if !strings.Contains(toolText(t, offResp), "bootstrap_alarm: off [scope:global]") {
		t.Fatalf("config.bootstrap_alarm off call must succeed: %s", offResp)
	}

	suppressedResp := callLogin(t, s, 3, root, nil)
	if strings.Contains(toolText(t, suppressedResp), "Bootstrap template is stale") {
		t.Fatalf("ferrule must not warn once bootstrap_alarm is off: %s", suppressedResp)
	}
}

// TestBootstrapStalenessWarningSilentWithoutTag verifies the no-tag-silent
// rule: a downstream AGENTS.md with no Template Version tag never warns, even
// though "latest" is presumably higher than "0" (an untagged project never
// opted into the ws bootstrap contract).
func TestBootstrapStalenessWarningSilentWithoutTag(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "AGENTS.md", "# Root\n\nno template marker here\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	writeTemplateVersionFixture(t, 2)

	s := NewServer(root, "test")
	resp := callLogin(t, s, 1, root, nil)
	if strings.Contains(toolText(t, resp), "Bootstrap template is stale") {
		t.Fatalf("ferrule must stay silent when the downstream AGENTS.md has no Template Version tag: %s", resp)
	}
}

// TestBootstrapAlarmTuningKnob verifies config.tuning lists the bootstrap_alarm
// knob with the resolved current value/scope, and that config.bootstrap_alarm
// can set and reset it, mirroring
// TestWorkflowPreferSubagentWriterProductionPath /
// TestWorkflowPreferSubagentResetRestoresBuiltin.
func TestBootstrapAlarmTuningKnob(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))

	offResp := callToolOnce(t, s, 2, "config.bootstrap_alarm", map[string]any{
		"session_key": key,
		"value":       "off",
	})
	if !strings.Contains(toolText(t, offResp), "bootstrap_alarm: off [scope:global]") {
		t.Fatalf("bootstrap_alarm off call must succeed: %s", offResp)
	}

	tuningText := toolText(t, callToolOnce(t, s, 3, "config.tuning", map[string]any{
		"session_key": key,
	}))
	if !strings.Contains(tuningText, "bootstrap_alarm") {
		t.Fatalf("config.tuning must list the bootstrap_alarm knob: %s", tuningText)
	}
	if !strings.Contains(tuningText, "off") {
		t.Fatalf("config.tuning must report the resolved off value for bootstrap_alarm: %s", tuningText)
	}

	resetResp := callToolOnce(t, s, 4, "config.bootstrap_alarm", map[string]any{
		"session_key": key,
		"reset":       true,
	})
	if !strings.Contains(toolText(t, resetResp), "bootstrap_alarm: on [scope:builtin]") {
		t.Fatalf("reset must report the builtin-sourced value: %s", resetResp)
	}

	conflictResp := callToolOnce(t, s, 5, "config.bootstrap_alarm", map[string]any{
		"session_key": key,
		"value":       "on",
		"reset":       true,
	})
	if !strings.Contains(conflictResp, `"isError":true`) {
		t.Fatalf("value+reset together must error: %s", conflictResp)
	}
}
