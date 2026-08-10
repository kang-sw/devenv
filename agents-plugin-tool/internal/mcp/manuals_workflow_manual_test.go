package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestWorkflowManualCarriesManualsBlockOnFreshAndContinue verifies the
// ambient "# Manuals" block (260807 Phase 1) is injected into workflow_manual
// output on both the FRESH-with-root branch and the CONTINUE branch when a
// fixture manual exists under ai-docs/manuals/, alongside the existing scope
// announcement injection (see scope_announcement_test.go for the sibling
// pattern this test mirrors).
func TestWorkflowManualCarriesManualsBlockOnFreshAndContinue(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "ai-docs/manuals/deploy.md", "---\nsummary: How to deploy the service.\n---\n# Deploy\n")
	mustWrite(t, root, "ai-docs/manuals/no-summary.md", "# No Summary\n\nBody with no frontmatter.\n")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))

	s := NewServer(root, "test")

	freshResp := callToolWithKey(t, s, 1, freshBootstrapKey, "workflow_manual", map[string]any{
		"root": root,
	})
	if !strings.Contains(freshResp, "# Manuals") {
		t.Fatalf("workflow_manual FRESH-with-root must carry the Manuals block: %s", freshResp)
	}
	if !strings.Contains(freshResp, "ai-docs/manuals/deploy.md") || !strings.Contains(freshResp, "How to deploy the service.") {
		t.Fatalf("workflow_manual FRESH-with-root must list the fixture manual with its summary: %s", freshResp)
	}
	if !strings.Contains(freshResp, "ai-docs/manuals/no-summary.md") || !strings.Contains(freshResp, "no summary") {
		t.Fatalf("workflow_manual FRESH-with-root must report the summary-less manual, not drop it: %s", freshResp)
	}

	key, _ := parseLoginResponse(t, callLogin(t, s, 2, root, nil))
	continueResp := callToolWithKey(t, s, 3, key, "workflow_manual", nil)
	if !strings.Contains(continueResp, "# Manuals") {
		t.Fatalf("workflow_manual CONTINUE must carry the Manuals block: %s", continueResp)
	}
	if !strings.Contains(continueResp, "ai-docs/manuals/deploy.md") || !strings.Contains(continueResp, "How to deploy the service.") {
		t.Fatalf("workflow_manual CONTINUE must list the fixture manual with its summary: %s", continueResp)
	}
	if !strings.Contains(continueResp, "ai-docs/manuals/no-summary.md") || !strings.Contains(continueResp, "no summary") {
		t.Fatalf("workflow_manual CONTINUE must report the summary-less manual, not drop it: %s", continueResp)
	}
}

// TestManualsListAndFindMCPToolsReturnFixtureManual verifies manuals.list and
// manuals.find are reachable as ordinary MCP tools (discovery parity with
// specs.*/mental_models.*), not only through workflow_manual's ambient
// injection — the ticket's verification requirement (c).
func TestManualsListAndFindMCPToolsReturnFixtureManual(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "ai-docs/manuals/deploy.md", "---\nsummary: How to deploy the service.\n---\n# Deploy\n")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))

	listResp := callToolWithKey(t, s, 2, key, "manuals.list", nil)
	if !strings.Contains(listResp, "ai-docs/manuals/deploy.md") || !strings.Contains(listResp, "How to deploy the service.") {
		t.Fatalf("manuals.list must return the fixture manual: %s", listResp)
	}

	findResp := callToolWithKey(t, s, 3, key, "manuals.find", map[string]any{"query": "deploy"})
	if !strings.Contains(findResp, "ai-docs/manuals/deploy.md") {
		t.Fatalf("manuals.find must return the fixture manual for a matching query: %s", findResp)
	}

	emptyFindResp := callToolWithKey(t, s, 4, key, "manuals.find", map[string]any{"query": "no-such-term-anywhere"})
	if strings.Contains(emptyFindResp, "ai-docs/manuals/deploy.md") {
		t.Fatalf("manuals.find must not return the fixture manual for a non-matching query: %s", emptyFindResp)
	}
}

// TestWorkflowManualManualsBlockAbsentWhenNoManualsExist verifies the
// injection is a true no-op (scopeAnnouncement-style silent case) when
// ai-docs/manuals/ does not exist — the common Phase 1 state before any
// manual is authored.
func TestWorkflowManualManualsBlockAbsentWhenNoManualsExist(t *testing.T) {
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
	if strings.Contains(freshResp, "# Manuals") {
		t.Fatalf("workflow_manual FRESH-with-root must stay silent with no manuals: %s", freshResp)
	}

	key, _ := parseLoginResponse(t, callLogin(t, s, 2, root, nil))
	continueResp := callToolWithKey(t, s, 3, key, "workflow_manual", nil)
	if strings.Contains(continueResp, "# Manuals") {
		t.Fatalf("workflow_manual CONTINUE must stay silent with no manuals: %s", continueResp)
	}
}
