package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestDocCoverageWarningFiresOnFerrule verifies the warning fires at
// ws.ferrule time when a doc area has no frontmatter-bearing .md file, and
// points to the config.doc_coverage_alarm setter.
func TestDocCoverageWarningFiresOnFerrule(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	resp := callLogin(t, s, 1, root, nil)
	text := toolText(t, resp)

	if !strings.Contains(text, "Doc coverage is missing") {
		t.Fatalf("ferrule response must carry the doc-coverage warning: %s", text)
	}
	if !strings.Contains(text, "config.doc_coverage_alarm") {
		t.Fatalf("warning must point to the config.doc_coverage_alarm setter: %s", text)
	}
}

// TestDocCoverageWarningFiresOnWorkflowManual verifies the same doc-coverage
// warning fires from ws.workflow_manual, in both the FRESH-with-root branch
// and the CONTINUE branch.
func TestDocCoverageWarningFiresOnWorkflowManual(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))

	s := NewServer(root, "test")

	freshResp := callToolWithKey(t, s, 1, freshBootstrapKey, "workflow_manual", map[string]any{
		"root": root,
	})
	if !strings.Contains(freshResp, "Doc coverage is missing") {
		t.Fatalf("workflow_manual FRESH-with-root must carry the doc-coverage warning: %s", freshResp)
	}

	key, _ := parseLoginResponse(t, callLogin(t, s, 2, root, nil))
	continueResp := callToolWithKey(t, s, 3, key, "workflow_manual", nil)
	if !strings.Contains(continueResp, "Doc coverage is missing") {
		t.Fatalf("workflow_manual CONTINUE must carry the doc-coverage warning: %s", continueResp)
	}
}

// TestDocCoverageWarningSilentWhenBothAreasCovered verifies the warning stays
// silent once both ai-docs/spec/ and ai-docs/mental-model/ have at least one
// frontmatter-bearing .md file.
func TestDocCoverageWarningSilentWhenBothAreasCovered(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", "---\ntitle: Demo\n---\n# Demo\n")
	mustWrite(t, root, "ai-docs/mental-model/demo.md", "---\ndomain: demo\n---\n# Demo\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	resp := callLogin(t, s, 1, root, nil)
	if strings.Contains(toolText(t, resp), "Doc coverage is missing") {
		t.Fatalf("ferrule must stay silent when both doc areas are covered: %s", toolText(t, resp))
	}
}

// TestDocCoverageWarningFiresForSpecOnlyMissing verifies that when only
// ai-docs/mental-model/ is covered (spec missing), the warning names only
// ai-docs/spec/ as missing, exercising the `case !hasSpec:` branch of
// docCoverageWarning's three-way switch.
func TestDocCoverageWarningFiresForSpecOnlyMissing(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/mental-model/demo.md", "---\ndomain: demo\n---\n# Demo\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	resp := callLogin(t, s, 1, root, nil)
	text := toolText(t, resp)

	if !strings.Contains(text, "ai-docs/spec/ has no .md file carrying a frontmatter block") {
		t.Fatalf("warning must name only ai-docs/spec/ as missing: %s", text)
	}
	if strings.Contains(text, "ai-docs/mental-model/ has no") || strings.Contains(text, "and ai-docs/mental-model/") {
		t.Fatalf("warning must not also blame ai-docs/mental-model/ when it is covered: %s", text)
	}
}

// TestDocCoverageWarningFiresForMentalModelOnlyMissing verifies that when
// only ai-docs/spec/ is covered (mental-model missing), the warning names
// only ai-docs/mental-model/ as missing, exercising the `default:` branch of
// docCoverageWarning's three-way switch.
func TestDocCoverageWarningFiresForMentalModelOnlyMissing(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", "---\ntitle: Demo\n---\n# Demo\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	resp := callLogin(t, s, 1, root, nil)
	text := toolText(t, resp)

	if !strings.Contains(text, "ai-docs/mental-model/ has no .md file carrying a frontmatter block") {
		t.Fatalf("warning must name only ai-docs/mental-model/ as missing: %s", text)
	}
	if strings.Contains(text, "ai-docs/spec/ has no") || strings.Contains(text, "and ai-docs/mental-model/") {
		t.Fatalf("warning must not also blame ai-docs/spec/ when it is covered: %s", text)
	}
}

// TestDocCoverageWarningSuppressedWhenOff verifies config.doc_coverage_alarm
// off (global scope, the item is global-only) silences the warning.
func TestDocCoverageWarningSuppressedWhenOff(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))

	offResp := callToolOnce(t, s, 2, "config.doc_coverage_alarm", map[string]any{
		"session_key": key,
		"value":       "off",
	})
	if !strings.Contains(toolText(t, offResp), "doc_coverage_alarm: off [scope:global]") {
		t.Fatalf("config.doc_coverage_alarm off call must succeed: %s", offResp)
	}

	suppressedResp := callLogin(t, s, 3, root, nil)
	if strings.Contains(toolText(t, suppressedResp), "Doc coverage is missing") {
		t.Fatalf("ferrule must not warn once doc_coverage_alarm is off: %s", suppressedResp)
	}
}

// TestDocCoverageAlarmTuningKnob verifies config.tuning lists the
// doc_coverage_alarm knob with the resolved current value/scope, and that
// config.doc_coverage_alarm can set and reset it, mirroring
// TestBootstrapAlarmTuningKnob.
func TestDocCoverageAlarmTuningKnob(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))

	offResp := callToolOnce(t, s, 2, "config.doc_coverage_alarm", map[string]any{
		"session_key": key,
		"value":       "off",
	})
	if !strings.Contains(toolText(t, offResp), "doc_coverage_alarm: off [scope:global]") {
		t.Fatalf("doc_coverage_alarm off call must succeed: %s", offResp)
	}

	tuningText := toolText(t, callToolOnce(t, s, 3, "config.tuning", map[string]any{
		"session_key": key,
	}))
	if !strings.Contains(tuningText, "doc_coverage_alarm") {
		t.Fatalf("config.tuning must list the doc_coverage_alarm knob: %s", tuningText)
	}
	// Scope the "off" assertion to the doc_coverage_alarm knob's own block: the
	// catalog also lists workflow.prefer_subagent, whose builtin default is
	// "off", so a bare substring check on "off" would pass even if
	// doc_coverage_alarm's own current value were wired wrong.
	knobBlocks := strings.Split(tuningText, "\n\n")
	var docCoverageAlarmBlock string
	for _, block := range knobBlocks {
		if strings.HasPrefix(block, "doc_coverage_alarm (") {
			docCoverageAlarmBlock = block
			break
		}
	}
	if docCoverageAlarmBlock == "" {
		t.Fatalf("config.tuning must contain a doc_coverage_alarm knob block: %s", tuningText)
	}
	if !strings.Contains(docCoverageAlarmBlock, `current: {"value":"off"`) {
		t.Fatalf("config.tuning must report the resolved off value scoped to doc_coverage_alarm's own block: %s", docCoverageAlarmBlock)
	}

	resetResp := callToolOnce(t, s, 4, "config.doc_coverage_alarm", map[string]any{
		"session_key": key,
		"reset":       true,
	})
	if !strings.Contains(toolText(t, resetResp), "doc_coverage_alarm: on [scope:builtin]") {
		t.Fatalf("reset must report the builtin-sourced value: %s", resetResp)
	}

	conflictResp := callToolOnce(t, s, 5, "config.doc_coverage_alarm", map[string]any{
		"session_key": key,
		"value":       "on",
		"reset":       true,
	})
	if !strings.Contains(conflictResp, `"isError":true`) {
		t.Fatalf("value+reset together must error: %s", conflictResp)
	}
}
