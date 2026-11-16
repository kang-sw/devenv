package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// An actionable ticket carries the plumbing check: an epic is stamped required
// whatever sage_review_design says (TestServeStdioTicketsCreateEpicIgnoresDesignKnob).
func TestServeStdioTicketsCreateUsesResolvedSageReviewConfig(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)

	resolver := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
	if err := resolver.Set(wsconfig.ItemSageReviewDesign, "ask", wsconfig.SetOptions{}); err != nil {
		t.Fatalf("set sage_review_design: %v", err)
	}

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 902600, root, nil))

	resp := callToolWithKey(t, server, 1, key, "tickets.create_empty", map[string]any{
		"stem":          "feat-sage-create",
		"initial_state": "ready",
	})
	if !strings.Contains(resp, "Created ai-docs/tickets/ready/") || !strings.Contains(resp, "recommended") {
		t.Fatalf("tickets.create_empty response missing created path or posture: %s", resp)
	}

	matches, err := filepath.Glob(filepath.Join(root, "ai-docs", "tickets", "ready", "*-feat-sage-create.md"))
	if err != nil {
		t.Fatalf("glob created ticket: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("created ticket matches = %v, want exactly one", matches)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read created ticket: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "sage-review-design: recommended") {
		t.Fatalf("created ticket missing recommended posture:\n%s", body)
	}
}

// An epic's design review is exempt from sage_review_design: at the builtin
// default (off) the todo stub is still stamped required.
func TestServeStdioTicketsCreateEpicIgnoresDesignKnob(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 902601, root, nil))

	resp := callToolWithKey(t, server, 1, key, "tickets.create_empty", map[string]any{
		"stem":          "epic-sage-create-default",
		"initial_state": "todo",
	})
	if !strings.Contains(resp, "Created ai-docs/tickets/todo/") || !strings.Contains(resp, "design required") {
		t.Fatalf("tickets.create_empty response missing created path or posture: %s", resp)
	}

	matches, err := filepath.Glob(filepath.Join(root, "ai-docs", "tickets", "todo", "*-epic-sage-create-default.md"))
	if err != nil {
		t.Fatalf("glob created ticket: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("created ticket matches = %v, want exactly one", matches)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read created ticket: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "sage-review-design: required") {
		t.Fatalf("created epic missing required posture (an epic ignores sage_review_design, builtin off):\n%s", body)
	}
}

func TestServeStdioGlobalSageReviewDesignAutoRequiresTicketBoundaryReview(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 902602, root, nil))

	tuneResp := callToolWithKey(t, server, 1, key, "config.tune", map[string]any{
		"key":   wsconfig.ItemSageReviewDesign,
		"value": "auto",
		"scope": "global",
	})
	if !strings.Contains(tuneResp, "sage_review_design: auto [scope:global]") {
		t.Fatalf("global sage_review_design tune response = %s", tuneResp)
	}

	resp := callToolWithKey(t, server, 2, key, "tickets.create_empty", map[string]any{
		"stem":          "epic-sage-create-global-auto",
		"initial_state": "todo",
	})
	if !strings.Contains(resp, "required") {
		t.Fatalf("global sage_review_design=auto must resolve to required ticket posture: %s", resp)
	}
}

// TestServeStdioTicketsCreateEmptyStatesSkeletonCaveat is the 260723 Phase 2
// review-fix regression test: formatTicketCreate's next_instruction caveat
// ("valid empty skeleton + initial posture") must actually reach the
// dispatched tools/call response, not just live in the source comment.
func TestServeStdioTicketsCreateEmptyStatesSkeletonCaveat(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 902605, root, nil))

	resp := callToolWithKey(t, server, 1, key, "tickets.create_empty", map[string]any{
		"stem":          "feat-skeleton-caveat",
		"initial_state": "todo",
	})
	if !strings.Contains(resp, "valid empty skeleton") {
		t.Fatalf("tickets.create_empty response missing skeleton caveat: %s", resp)
	}
}

// TestServeStdioTicketsMoveToReadyStatesSageGateNextInstruction covers the
// "moved" branch of ticketMutateNextInstruction (260723 Phase 2): promoting a
// ticket to ready/ must surface the sage-review-gate follow-on instruction in
// the dispatched tools/call response.
func TestServeStdioTicketsMoveToReadyStatesSageGateNextInstruction(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	stem := "260101-feat-ready-next-instruction"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
		"---\ntitle: Ready move\nsage-review-design: completed\nsage-review-completeness: completed\n---\n\nBody.\n")

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 902606, root, nil))

	moveResp := callToolWithKey(t, server, 1, key, "tickets.move", map[string]any{
		"stem": stem,
		"to":   "ready",
	})
	if !strings.Contains(moveResp, "next_instruction: Call tickets.sage_gate(stem, landing) next") {
		t.Fatalf("tickets.move to ready response missing sage_gate next_instruction: %s", moveResp)
	}
}

// TestServeStdioTicketsCloseUnresolvedPhaseStatesSoftWarnNextInstruction
// covers the "closed" branch of ticketMutateNextInstruction (260723 Phase 2):
// closing a ticket with an unresolved phase heading must surface the
// soft-warning follow-on instruction in the dispatched tools/call response.
func TestServeStdioTicketsCloseUnresolvedPhaseStatesSoftWarnNextInstruction(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	stem := "260101-feat-close-next-instruction"
	body := "---\ntitle: Open phase\n---\n\n" +
		"## Phases\n\n" +
		"### Phase 1: First\n\n" +
		"Never resolved.\n"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"), body)

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 902607, root, nil))

	closeResp := callToolWithKey(t, server, 1, key, "tickets.close", map[string]any{
		"stem":   stem,
		"status": "done",
	})
	if !strings.Contains(closeResp, "next_instruction: This is a soft warning only (no block)") {
		t.Fatalf("tickets.close response missing unresolved-phase next_instruction: %s", closeResp)
	}
}

// TestServeStdioTicketsCloseMergeReviewNudgeOnUnmergedImplBranch covers the
// Phase 2 merge-review trigger: closing a ticket while the current branch is
// an impl/<root>/<stem> branch carrying commits ahead of its merge root must
// surface a next_instruction nudging review-and-merge of that branch into
// the merge root. tickets.close itself performs no merge (real git branches
// are built here — the first integration coverage of the real
// aheadOfMergeRootCount rev-list path; Phase 1 covered it only via
// hand-built implementBranchObservation unit tests).
func TestServeStdioTicketsCloseMergeReviewNudgeOnUnmergedImplBranch(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	runGit(t, root, "checkout", "-b", "root-branch")
	mustWrite(t, root, "README.md", "root\n")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "root commit")

	runGit(t, root, "checkout", "-b", "impl/root-branch/close-nudge")
	mustWrite(t, root, "impl-file.txt", "impl\n")
	runGit(t, root, "add", "impl-file.txt")
	runGit(t, root, "commit", "-m", "impl commit")

	stem := "260101-feat-close-merge-review-nudge"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"), "---\ntitle: Merge review nudge\n---\n\nBody.\n")

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	closeResp := callToolWithKey(t, server, 2, key, "tickets.close", map[string]any{
		"stem":   stem,
		"status": "done",
	})
	if !strings.Contains(closeResp, "next_instruction:") {
		t.Fatalf("tickets.close response missing merge-review next_instruction: %s", closeResp)
	}
	for _, want := range []string{"Warning:", "impl/root-branch/close-nudge", "root-branch", "the lead integrates it with git.merge", "Do not merge from the worker"} {
		if !strings.Contains(closeResp, want) {
			t.Fatalf("tickets.close merge-review next_instruction missing %q: %s", want, closeResp)
		}
	}
}

func TestServeStdioTicketsCloseWarnsWhenImplMergeStateUnverifiable(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	runGit(t, root, "checkout", "-b", "impl/missing-root/close-nudge")
	mustWrite(t, root, "README.md", "root\n")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "root commit")
	stem := "260101-feat-close-unverifiable-merge"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"), "---\ntitle: Merge warning\n---\n\nBody.\n")
	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
	resp := callToolWithKey(t, server, 2, key, "tickets.close", map[string]any{"stem": stem, "status": "done"})
	if !strings.Contains(resp, "Warning: could not verify implementation merge state") {
		t.Fatalf("missing observable merge-state warning: %s", resp)
	}
	if _, err := os.Stat(filepath.Join(root, "ai-docs", "tickets", ".done", stem+".md")); err != nil {
		t.Fatalf("advisory blocked closure: %v; response: %s", err, resp)
	}
}

// TestServeStdioTicketsCloseNoMergeReviewNudgeWhenImplBranchClean is the
// merged/clean counterpart: closing on an impl/<root>/<stem> branch with no
// commits ahead of its merge root (identical to root) must not surface a
// merge-review next_instruction.
func TestServeStdioTicketsCloseNoMergeReviewNudgeWhenImplBranchClean(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	runGit(t, root, "checkout", "-b", "root-branch")
	mustWrite(t, root, "README.md", "root\n")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "root commit")

	runGit(t, root, "checkout", "-b", "impl/root-branch/clean-nudge")

	stem := "260101-feat-close-no-merge-review-nudge-clean"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"), "---\ntitle: No merge review nudge\n---\n\nBody.\n")

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	closeResp := callToolWithKey(t, server, 2, key, "tickets.close", map[string]any{
		"stem":   stem,
		"status": "done",
	})
	if strings.Contains(closeResp, "next_instruction:") {
		t.Fatalf("tickets.close on a clean impl branch should carry no next_instruction: %s", closeResp)
	}
}

// TestServeStdioTicketsCloseNoMergeReviewNudgeOnPlainBranch covers the
// non-impl case: closing on a plain (never impl/-prefixed) branch must not
// surface a merge-review next_instruction, even with prior commits.
func TestServeStdioTicketsCloseNoMergeReviewNudgeOnPlainBranch(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	runGit(t, root, "checkout", "-b", "feature/plain-branch")
	mustWrite(t, root, "README.md", "root\n")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "plain commit")

	stem := "260101-feat-close-no-merge-review-nudge-plain"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"), "---\ntitle: No merge review nudge\n---\n\nBody.\n")

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	closeResp := callToolWithKey(t, server, 2, key, "tickets.close", map[string]any{
		"stem":   stem,
		"status": "done",
	})
	if strings.Contains(closeResp, "next_instruction:") {
		t.Fatalf("tickets.close on a plain non-impl branch should carry no next_instruction: %s", closeResp)
	}
}

// TestServeStdioTicketsMoveToReadyUnresolvedPostureWarnsInResponse is a C5
// dispatch-level test: a todo/ -> ready/ move at the shipped defaults
// (sage_review: auto -> completeness required; sage_review_design: off ->
// design skipped, terminal) with an unresolved completeness posture
// must succeed and carry the ready-sage-posture warning text in the actual
// tools/call response text, not just in wsdoc.TicketMutateResult.Tip.
func TestServeStdioTicketsMoveToReadyUnresolvedPostureWarnsInResponse(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	stem := "260101-feat-ready-unresolved-warn"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
		"---\ntitle: Unresolved\n---\n\nBody.\n")

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 902608, root, nil))

	moveResp := callToolWithKey(t, server, 1, key, "tickets.move", map[string]any{
		"stem": stem,
		"to":   "ready",
	})
	if !strings.Contains(moveResp, "sage-review-completeness is unreviewed") {
		t.Fatalf("tickets.move to ready response missing unresolved-posture warning: %s", moveResp)
	}
	if !strings.Contains(moveResp, "ws/git.commit will fail on guardrail ready-sage-posture") {
		t.Fatalf("tickets.move to ready response missing consequence statement: %s", moveResp)
	}
	if !strings.Contains(moveResp, "ws/tickets.sage_gate") {
		t.Fatalf("tickets.move to ready response missing resolving-call pointer: %s", moveResp)
	}
	matches, err := filepath.Glob(filepath.Join(root, "ai-docs", "tickets", "ready", stem+".md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("moved ticket should exist at ready/: matches=%v err=%v", matches, err)
	}
}

// TestServeStdioTicketsMoveNonReadyBlockedRejectsInResponse is the
// dispatch-level counterpart of the wsdoc-level
// TestTicketsMoveUpwardNonReadyBlockedRejectsMove. C5 gave the ready-landing
// *warning* paths dispatch-level coverage but left the non-ready *hard
// rejection* (blockedUpwardMoveError) asserted only inside wsdoc, so a
// dispatch-case edit that downgraded it to a soft Tip, swallowed it, or ran it
// after atomicGitMove would pass every test. This asserts the rejection
// actually reaches the tools/call response text and that the ticket does not
// move.
func TestServeStdioTicketsMoveNonReadyBlockedRejectsInResponse(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	stem := "260101-epic-nonready-blocked-dispatch"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "idea", stem+".md"),
		"---\ntitle: Blocked\nsage-review-design: blocked\n---\n\nBody.\n")

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 902610, root, nil))

	moveResp := callToolWithKey(t, server, 1, key, "tickets.move", map[string]any{
		"stem": stem,
		"to":   "todo",
	})
	if !strings.Contains(moveResp, "sage-review-design: blocked") {
		t.Fatalf("tickets.move idea->todo blocked response missing the rejection text: %s", moveResp)
	}
	if !strings.Contains(moveResp, "address blocked review before promoting") {
		t.Fatalf("tickets.move idea->todo blocked response missing the action-oriented rejection: %s", moveResp)
	}
	// It must be a hard error, not a soft tip on a successful move.
	if strings.Contains(moveResp, "moved") || strings.Contains(moveResp, "tip: ") {
		t.Fatalf("blocked non-ready move must not report success: %s", moveResp)
	}
	if _, err := os.Stat(filepath.Join(root, "ai-docs", "tickets", "todo", stem+".md")); err == nil {
		t.Fatalf("ticket moved to todo/ despite blocked rejection")
	}
	if _, err := os.Stat(filepath.Join(root, "ai-docs", "tickets", "idea", stem+".md")); err != nil {
		t.Fatalf("ticket should remain at idea/: %v", err)
	}
}

// TestServeStdioTicketsMoveNonReadyBlockedSurfacesPartialMutationNotice
// reproduces the 2026-07-13 scenario from
// 260713-bug-tickets-move-error-mutates-frontmatter at the MCP tool-response
// layer, scoped to the one path that still hard-blocks after 16c77241
// relocated ready-landing sage-posture enforcement to ws/git.commit: a
// legacy-schema ticket (single sage-review: field) blocked on a non-ready
// upward move (idea -> todo). TicketsMove self-heals (migrates) the
// frontmatter before the block, so the tool response text must carry an
// explicit partial-mutation notice alongside the error, not the bare error
// alone.
func TestServeStdioTicketsMoveNonReadyBlockedSurfacesPartialMutationNotice(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	stem := "260101-epic-sage-move-nonready-blocked-partial"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "idea", stem+".md"),
		"---\ntitle: Sage\nsage-review: blocked\n---\n\nBody.\n")

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 902611, root, nil))

	moveResp := callToolWithKey(t, server, 1, key, "tickets.move", map[string]any{
		"stem": stem,
		"to":   "todo",
	})
	if !strings.Contains(moveResp, "sage-review-design: blocked") {
		t.Fatalf("tickets.move blocked response missing the underlying error: %s", moveResp)
	}
	if !strings.Contains(moveResp, "partial-mutation:") {
		t.Fatalf("tickets.move blocked response missing partial-mutation notice: %s", moveResp)
	}

	raw, err := os.ReadFile(filepath.Join(root, "ai-docs", "tickets", "idea", stem+".md"))
	if err != nil {
		t.Fatalf("read ticket after blocked move: %v", err)
	}
	if !strings.Contains(string(raw), "sage-review-design: blocked") {
		t.Fatalf("ticket missing self-healed sage-review-design field after blocked move:\n%s", string(raw))
	}
	if _, err := os.Stat(filepath.Join(root, "ai-docs", "tickets", "todo", stem+".md")); err == nil {
		t.Fatalf("ticket moved to todo/ despite blocked rejection")
	}
}

// TestServeStdioTicketsCreateEmptyReadyUnresolvedPostureWarnsInResponse is
// the tickets.create_empty counterpart of the above (C5): creating directly
// at ready/ under the shipped default posture must succeed and surface the
// same warning shape in the dispatch response.
func TestServeStdioTicketsCreateEmptyReadyUnresolvedPostureWarnsInResponse(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 902609, root, nil))

	createResp := callToolWithKey(t, server, 1, key, "tickets.create_empty", map[string]any{
		"stem":          "feat-ready-create-unresolved-warn",
		"initial_state": "ready",
	})
	if !strings.Contains(createResp, "Created ai-docs/tickets/ready/") {
		t.Fatalf("tickets.create_empty response missing created path: %s", createResp)
	}
	if !strings.Contains(createResp, "sage-review-completeness is unreviewed") {
		t.Fatalf("tickets.create_empty ready response missing unresolved-posture warning: %s", createResp)
	}
	if !strings.Contains(createResp, "ws/git.commit will fail on guardrail ready-sage-posture") {
		t.Fatalf("tickets.create_empty ready response missing consequence statement: %s", createResp)
	}
	if !strings.Contains(createResp, "ws/tickets.sage_gate") {
		t.Fatalf("tickets.create_empty ready response missing resolving-call pointer: %s", createResp)
	}
	matches, err := filepath.Glob(filepath.Join(root, "ai-docs", "tickets", "ready", "*-feat-ready-create-unresolved-warn.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("created ticket should exist at ready/: matches=%v err=%v", matches, err)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read created ticket: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "sage-review-design: skipped") || !strings.Contains(body, "sage-review-completeness: required") {
		t.Fatalf("created ticket missing both per-stage stamped fields (C4 parity with tickets.move):\n%s", body)
	}
}

func TestServeStdioTicketsMoveDefaultsToSkippedDesignReview(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 902602, root, nil))

	createResp := callToolWithKey(t, server, 1, key, "tickets.create_empty", map[string]any{
		"stem":          "feat-sage-move-default",
		"initial_state": "todo",
	})
	if !strings.Contains(createResp, "Created ai-docs/tickets/todo/") {
		t.Fatalf("tickets.create_empty response missing created path: %s", createResp)
	}
	createdMatches, err := filepath.Glob(filepath.Join(root, "ai-docs", "tickets", "todo", "*-feat-sage-move-default.md"))
	if err != nil || len(createdMatches) != 1 {
		t.Fatalf("glob created ticket: matches=%v err=%v", createdMatches, err)
	}
	datedStem := strings.TrimSuffix(filepath.Base(createdMatches[0]), ".md")

	moveResp := callToolWithKey(t, server, 2, key, "tickets.move", map[string]any{
		"stem": datedStem,
		"to":   "ready",
	})
	if !strings.Contains(moveResp, "skipped") {
		t.Fatalf("tickets.move response missing skipped posture tip (sage_review_design builtin default is off): %s", moveResp)
	}

	matches, err := filepath.Glob(filepath.Join(root, "ai-docs", "tickets", "ready", "*-feat-sage-move-default.md"))
	if err != nil {
		t.Fatalf("glob moved ticket: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("moved ticket matches = %v, want exactly one", matches)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read moved ticket: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "sage-review-design: skipped") {
		t.Fatalf("moved ticket missing skipped posture (sage_review_design builtin default is off):\n%s", body)
	}
}

func TestServeStdioTicketsMoveExplicitOverrideWinsOverBuiltinDefault(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)

	resolver := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
	if err := resolver.Set(wsconfig.ItemSageReviewDesign, "ask", wsconfig.SetOptions{}); err != nil {
		t.Fatalf("set sage_review_design: %v", err)
	}

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 902603, root, nil))

	createResp := callToolWithKey(t, server, 1, key, "tickets.create_empty", map[string]any{
		"stem":          "feat-sage-move-override",
		"initial_state": "todo",
	})
	if !strings.Contains(createResp, "Created ai-docs/tickets/todo/") {
		t.Fatalf("tickets.create_empty response missing created path: %s", createResp)
	}
	createdMatches, err := filepath.Glob(filepath.Join(root, "ai-docs", "tickets", "todo", "*-feat-sage-move-override.md"))
	if err != nil || len(createdMatches) != 1 {
		t.Fatalf("glob created ticket: matches=%v err=%v", createdMatches, err)
	}
	datedStem := strings.TrimSuffix(filepath.Base(createdMatches[0]), ".md")

	moveResp := callToolWithKey(t, server, 2, key, "tickets.move", map[string]any{
		"stem": datedStem,
		"to":   "ready",
	})
	if !strings.Contains(moveResp, "recommended") {
		t.Fatalf("tickets.move response missing recommended posture tip (explicit override should win): %s", moveResp)
	}

	matches, err := filepath.Glob(filepath.Join(root, "ai-docs", "tickets", "ready", "*-feat-sage-move-override.md"))
	if err != nil {
		t.Fatalf("glob moved ticket: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("moved ticket matches = %v, want exactly one", matches)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read moved ticket: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "sage-review-design: recommended") {
		t.Fatalf("moved ticket missing recommended posture (explicit project-scope override should win over builtin default):\n%s", body)
	}
}
