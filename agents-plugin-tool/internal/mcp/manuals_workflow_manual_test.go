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

// TestWorkflowManualManualsBlockIsAlwaysOnWhenNoManualsExist verifies the
// block is an always-on authoring anchor: even with no ai-docs/manuals/
// directory, both the FRESH-with-root and CONTINUE branches still carry the
// "# Manuals" header, the authoring-guidance paragraph, and the "(none yet)"
// placeholder. (The FRESH-without-root branch, which has no root to resolve
// manuals from, still never renders the block — unchanged.)
func TestWorkflowManualManualsBlockIsAlwaysOnWhenNoManualsExist(t *testing.T) {
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
	if !strings.Contains(freshResp, "# Manuals") || !strings.Contains(freshResp, "- (none yet)") {
		t.Fatalf("workflow_manual FRESH-with-root must carry the always-on Manuals anchor with a (none yet) placeholder: %s", freshResp)
	}
	if !strings.Contains(freshResp, manualsAuthoringGuidance) {
		t.Fatalf("workflow_manual FRESH-with-root must carry the manuals authoring-guidance paragraph: %s", freshResp)
	}

	key, _ := parseLoginResponse(t, callLogin(t, s, 2, root, nil))
	continueResp := callToolWithKey(t, s, 3, key, "workflow_manual", nil)
	if !strings.Contains(continueResp, "# Manuals") || !strings.Contains(continueResp, "- (none yet)") {
		t.Fatalf("workflow_manual CONTINUE must carry the always-on Manuals anchor with a (none yet) placeholder: %s", continueResp)
	}
	if !strings.Contains(continueResp, manualsAuthoringGuidance) {
		t.Fatalf("workflow_manual CONTINUE must carry the manuals authoring-guidance paragraph: %s", continueResp)
	}
}
