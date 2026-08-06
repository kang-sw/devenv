package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

// enableSparseCheckout turns on core.sparseCheckout with a scope that hides
// one ready/ and one todo/ ticket while leaving another visible, mirroring
// the exact command shape ws:lead-scope-worktree derives.
func enableSparseCheckout(t *testing.T, root string) {
	t.Helper()
	runGit(t, root, "sparse-checkout", "set", "--no-cone",
		"/*",
		"!/ai-docs/tickets/ready/*",
		"!/ai-docs/tickets/todo/*",
		"/ai-docs/tickets/ready/kept-*",
		"/ai-docs/tickets/todo/kept-*",
	)
}

// mustWriteAndCommitTicket writes a ticket file and commits it so it lands in
// the index; TicketScope only detects hiding for indexed paths.
func mustWriteAndCommitTicket(t *testing.T, root, status, stem string) {
	t.Helper()
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", status, stem+".md"), "# "+stem+"\n")
	runGit(t, root, "add", "ai-docs/tickets")
	runGit(t, root, "commit", "-m", "add "+stem)
}

// TestScopeAnnouncementFiresOnWorkflowManual verifies the scope announcement
// fires from ws.workflow_manual, in both the FRESH-with-root branch and the
// CONTINUE branch, once core.sparseCheckout hides at least one ticket.
func TestScopeAnnouncementFiresOnWorkflowManual(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWriteAndCommitTicket(t, root, "ready", "kept-one")
	mustWriteAndCommitTicket(t, root, "ready", "hidden-one")
	mustWriteAndCommitTicket(t, root, "todo", "hidden-two")
	enableSparseCheckout(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))

	s := NewServer(root, "test")

	freshResp := callToolWithKey(t, s, 1, freshBootstrapKey, "workflow_manual", map[string]any{
		"root": root,
	})
	if !strings.Contains(freshResp, "Sparse-checkout scope is active") {
		t.Fatalf("workflow_manual FRESH-with-root must carry the scope announcement: %s", freshResp)
	}
	if !strings.Contains(freshResp, "hidden-one") || !strings.Contains(freshResp, "hidden-two") {
		t.Fatalf("workflow_manual FRESH-with-root must name the hidden stems: %s", freshResp)
	}
	if !strings.Contains(freshResp, "worktree-ticket-scope.md") {
		t.Fatalf("workflow_manual FRESH-with-root scope announcement must point to the reference manual: %s", freshResp)
	}

	key, _ := parseLoginResponse(t, callLogin(t, s, 2, root, nil))
	continueResp := callToolWithKey(t, s, 3, key, "workflow_manual", nil)
	if !strings.Contains(continueResp, "Sparse-checkout scope is active") {
		t.Fatalf("workflow_manual CONTINUE must carry the scope announcement: %s", continueResp)
	}
	if !strings.Contains(continueResp, "hidden-one") || !strings.Contains(continueResp, "hidden-two") {
		t.Fatalf("workflow_manual CONTINUE must name the hidden stems: %s", continueResp)
	}
}

// TestWorkflowManualScopeAnnouncementByteUnchangedWhenUnscoped verifies the
// scope block's marker text is absent in a plain initGit-only repo with no
// sparse-checkout, so the render-layer injection is a true no-op when
// core.sparseCheckout is unset. Phase 1's wsdoc package already separately
// proves the zero-subprocess property via requireGateSpawnsNoGit; this test
// only proves the render-layer no-op.
func TestWorkflowManualScopeAnnouncementByteUnchangedWhenUnscoped(t *testing.T) {
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
	if strings.Contains(freshResp, "Sparse-checkout scope is active") {
		t.Fatalf("workflow_manual FRESH-with-root must stay silent when unscoped: %s", freshResp)
	}

	key, _ := parseLoginResponse(t, callLogin(t, s, 2, root, nil))
	continueResp := callToolWithKey(t, s, 3, key, "workflow_manual", nil)
	if strings.Contains(continueResp, "Sparse-checkout scope is active") {
		t.Fatalf("workflow_manual CONTINUE must stay silent when unscoped: %s", continueResp)
	}
}

// TestScopeAnnouncementFiresWithNoTicketHidden verifies the scope block
// still renders when core.sparseCheckout is active but no ticket is
// currently hidden (Hidden == 0 branch of scopeAnnouncement), so the caller
// still learns a scope is in effect even before anything is actually hidden.
func TestScopeAnnouncementFiresWithNoTicketHidden(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWriteAndCommitTicket(t, root, "ready", "kept-one")
	enableSparseCheckout(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))

	s := NewServer(root, "test")
	freshResp := callToolWithKey(t, s, 1, freshBootstrapKey, "workflow_manual", map[string]any{
		"root": root,
	})
	if !strings.Contains(freshResp, "Sparse-checkout scope is active") {
		t.Fatalf("workflow_manual FRESH-with-root must announce an active scope even with nothing hidden yet: %s", freshResp)
	}
	if !strings.Contains(freshResp, "no ticket is currently hidden") {
		t.Fatalf("workflow_manual FRESH-with-root must state that no ticket is currently hidden: %s", freshResp)
	}
}
