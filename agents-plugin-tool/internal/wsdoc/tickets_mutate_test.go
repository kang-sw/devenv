package wsdoc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recordedGitCall struct {
	Args []string
}

type mockGitRunner struct {
	calls []recordedGitCall
	root  string
}

func (m *mockGitRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	m.root = root
	m.calls = append(m.calls, recordedGitCall{Args: append([]string(nil), args...)})
	// Emulate `git mv` so the on-disk move is observable by the test.
	if len(args) >= 3 && args[0] == "mv" {
		src := args[len(args)-2]
		dst := args[len(args)-1]
		oldPath := filepath.Join(root, filepath.FromSlash(src))
		newPath := filepath.Join(root, filepath.FromSlash(dst))
		// Real `git mv` does not create intermediate directories; the production
		// code is responsible for that. Mirror that here so the test does not
		// mask a missing-mkdir bug.
		if err := os.Rename(oldPath, newPath); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

const sampleTicket = "---\ntitle: Sample\nstatus: open\n---\n\n# Sample\n\nBody text.\n"

func TestTicketsCloseMovesDoneWithCompletedDate(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-sample"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"), sampleTicket)
	runner := &mockGitRunner{}

	result, err := TicketsClose(root, runner, TicketCloseOptions{
		TicketStem: stem,
		Status:     "done",
		Today:      "2026-01-15",
	})
	if err != nil {
		t.Fatalf("TicketsClose: %v", err)
	}
	if result.OldPath != "ai-docs/tickets/todo/"+stem+".md" {
		t.Fatalf("OldPath = %q", result.OldPath)
	}
	if result.NewPath != "ai-docs/tickets/.done/"+stem+".md" {
		t.Fatalf("NewPath = %q", result.NewPath)
	}
	newAbs := filepath.Join(root, "ai-docs", "tickets", ".done", stem+".md")
	body := readFileString(t, newAbs)
	if !strings.Contains(body, "completed: 2026-01-15") {
		t.Fatalf("completed field missing:\n%s", body)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("git calls = %d, want 2: %#v", len(runner.calls), runner.calls)
	}
	if runner.calls[0].Args[0] != "add" || runner.calls[0].Args[1] != result.OldPath {
		t.Fatalf("first git call = %#v, want add %s", runner.calls[0].Args, result.OldPath)
	}
	if runner.calls[1].Args[0] != "mv" {
		t.Fatalf("second git call = %#v, want mv", runner.calls[1].Args)
	}
}

func TestTicketsCloseMovesDroppedWithDroppedDate(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-drop"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "ready", stem+".md"), sampleTicket)
	runner := &mockGitRunner{}

	result, err := TicketsClose(root, runner, TicketCloseOptions{
		TicketStem: stem,
		Status:     "dropped",
		Today:      "2026-02-02",
	})
	if err != nil {
		t.Fatalf("TicketsClose: %v", err)
	}
	if result.NewPath != "ai-docs/tickets/.dropped/"+stem+".md" {
		t.Fatalf("NewPath = %q", result.NewPath)
	}
	body := readFileString(t, filepath.Join(root, "ai-docs", "tickets", ".dropped", stem+".md"))
	if !strings.Contains(body, "dropped: 2026-02-02") {
		t.Fatalf("dropped field missing:\n%s", body)
	}
}

func TestTicketsCloseAppendsResolutionSection(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-resolve"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"), sampleTicket)
	runner := &mockGitRunner{}

	_, err := TicketsClose(root, runner, TicketCloseOptions{
		TicketStem: stem,
		Status:     "done",
		Resolution: "Implemented in PR #42.",
		Today:      "2026-03-03",
	})
	if err != nil {
		t.Fatalf("TicketsClose: %v", err)
	}
	body := readFileString(t, filepath.Join(root, "ai-docs", "tickets", ".done", stem+".md"))
	if !strings.Contains(body, "## Resolution (2026-03-03)") {
		t.Fatalf("resolution heading missing:\n%s", body)
	}
	if !strings.Contains(body, "Implemented in PR #42.") {
		t.Fatalf("resolution text missing:\n%s", body)
	}
}

func TestTicketsCloseUnresolvedPhaseSoftWarnTip(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-openphase"
	body := "---\ntitle: Open phase\n---\n\n" +
		"## Phases\n\n" +
		"### Phase 1: First\n\n" +
		"Never resolved.\n"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"), body)
	runner := &mockGitRunner{}

	result, err := TicketsClose(root, runner, TicketCloseOptions{
		TicketStem: stem,
		Status:     "done",
		Today:      "2026-01-15",
	})
	if err != nil {
		t.Fatalf("TicketsClose: %v", err)
	}
	if !strings.Contains(result.Tip, "unresolved phase") {
		t.Fatalf("Tip = %q, want an unresolved-phase soft-warn tip", result.Tip)
	}
}

func TestTicketsCloseResolvedPhasesHaveNoTip(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-resolvedphase"
	body := "---\ntitle: Resolved phase\n---\n\n" +
		"## Phases\n\n" +
		"### Phase 1: First\n\n" +
		"### Result (abc123) - 2026-01-15\n\n" +
		"Done.\n"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"), body)
	runner := &mockGitRunner{}

	result, err := TicketsClose(root, runner, TicketCloseOptions{
		TicketStem: stem,
		Status:     "done",
		Today:      "2026-01-15",
	})
	if err != nil {
		t.Fatalf("TicketsClose: %v", err)
	}
	if result.Tip != "" {
		t.Fatalf("Tip = %q, want empty when every phase has a Result", result.Tip)
	}
}

func TestTicketsCloseRejectsAlreadyClosedTicket(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-closed"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", ".done", stem+".md"), sampleTicket)
	runner := &mockGitRunner{}

	if _, err := TicketsClose(root, runner, TicketCloseOptions{
		TicketStem: stem,
		Status:     "done",
		Today:      "2026-01-15",
	}); err == nil {
		t.Fatal("TicketsClose accepted an already-closed ticket")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("git called on guard failure: %#v", runner.calls)
	}
}

func TestTicketsCloseRejectsUnknownStem(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", ".keep"), "")
	runner := &mockGitRunner{}

	if _, err := TicketsClose(root, runner, TicketCloseOptions{
		TicketStem: "260101-feat-missing",
		Status:     "done",
		Today:      "2026-01-15",
	}); err == nil {
		t.Fatal("TicketsClose accepted an unknown stem")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("git called on guard failure: %#v", runner.calls)
	}
}

func TestTicketsCloseRejectsInvalidStatus(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-wip"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"), sampleTicket)
	runner := &mockGitRunner{}

	if _, err := TicketsClose(root, runner, TicketCloseOptions{
		TicketStem: stem,
		Status:     "wip",
		Today:      "2026-01-15",
	}); err == nil {
		t.Fatal("TicketsClose accepted status=wip")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("git called on guard failure: %#v", runner.calls)
	}
}

func TestTicketsMoveUpwardIdeaToTodo(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-up"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "idea", stem+".md"), sampleTicket)
	runner := &mockGitRunner{}

	result, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "todo",
	})
	if err != nil {
		t.Fatalf("TicketsMove: %v", err)
	}
	if result.OldPath != "ai-docs/tickets/idea/"+stem+".md" {
		t.Fatalf("OldPath = %q", result.OldPath)
	}
	if result.NewPath != "ai-docs/tickets/todo/"+stem+".md" {
		t.Fatalf("NewPath = %q", result.NewPath)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("git calls = %d, want 2: %#v", len(runner.calls), runner.calls)
	}
	if runner.calls[0].Args[0] != "add" {
		t.Fatalf("first git call = %#v, want add", runner.calls[0].Args)
	}
	if runner.calls[1].Args[0] != "mv" {
		t.Fatalf("second git call = %#v, want mv", runner.calls[1].Args)
	}
}

// TestTicketsMoveUpwardNonReadyBlockedRejectsMove is the C3 regression test:
// de-blocking is scoped to the ready/ landing only (per the ticket's decision
// text, "tickets.move and create_empty stop rejecting on ready sage
// posture"). A non-ready upward move (e.g. idea/ -> todo/) re-entering a
// `sage-review-design: blocked` posture — reachable via a demote/re-promote
// round trip after sage_stamp records `blocked` on a todo-landing design
// review — must still hard-reject, because tickets_verify.go's
// ready-sage-posture guardrail only runs for status == "ready"; outside a
// ready landing there is no chokepoint downstream to relocate enforcement
// to, so removing this rejection would move enforcement to nowhere.
func TestTicketsMoveUpwardNonReadyBlockedRejectsMove(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-nonready-blocked"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "idea", stem+".md"),
		"---\ntitle: Blocked\nsage-review-design: blocked\n---\n\nBody.\n")
	runner := &mockGitRunner{}

	_, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "todo",
		SageReview: "auto",
	})
	if err == nil {
		t.Fatal("TicketsMove idea->todo with blocked design posture: expected rejection, got nil error")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("error = %v, want blocked-posture rejection", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("git called despite blocked rejection: %#v", runner.calls)
	}
	// The ticket must remain at idea/, not moved.
	if _, statErr := os.Stat(filepath.Join(root, "ai-docs", "tickets", "todo", stem+".md")); statErr == nil {
		t.Fatalf("ticket moved to todo/ despite blocked rejection")
	}
}

func TestTicketsMoveDownwardReadyToTodoReturnsTip(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-down"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "ready", stem+".md"), sampleTicket)
	runner := &mockGitRunner{}

	result, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "todo",
	})
	if err != nil {
		t.Fatalf("TicketsMove: %v", err)
	}
	if !strings.Contains(result.Tip, "spec") {
		t.Fatalf("Tip = %q, want spec mention", result.Tip)
	}
}

func TestTicketsMoveRejectsSameStatus(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-same"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"), sampleTicket)
	runner := &mockGitRunner{}

	if _, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "todo",
	}); err == nil {
		t.Fatal("TicketsMove accepted same-status move")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("git called on guard failure: %#v", runner.calls)
	}
}

func TestTicketsMoveUpwardToTodoStampsResolvedSageReviewPostures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		config     string
		wantReview string
	}{
		{"empty", "", "skipped"},
		{"off", "off", "skipped"},
		{"ask", "ask", "recommended"},
		{"auto", "auto", "required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			stem := "260101-feat-sage-" + tc.name
			mustWrite(t, root, filepath.Join("ai-docs", "tickets", "idea", stem+".md"),
				"---\ntitle: Sage\n---\n\nBody.\n")
			runner := &mockGitRunner{}

			result, err := TicketsMove(root, runner, TicketMoveOptions{
				TicketStem: stem,
				To:         "todo",
				SageReview: tc.config,
			})
			if err != nil {
				t.Fatalf("TicketsMove idea->todo: %v", err)
			}
			body := readFileString(t, filepath.Join(root, filepath.FromSlash(result.NewPath)))
			for _, field := range []string{"sage-review-design", "sage-review-completeness"} {
				wantLine := field + ": " + tc.wantReview
				if !strings.Contains(body, wantLine) {
					t.Fatalf("moved ticket missing %s:\n%s", wantLine, body)
				}
			}
			if !strings.Contains(result.Tip, "design "+tc.wantReview) || !strings.Contains(result.Tip, "completeness "+tc.wantReview) {
				t.Fatalf("Tip = %q, want both stages at resolved posture %q", result.Tip, tc.wantReview)
			}
		})
	}
}

func TestTicketsMoveUpwardToTodoEpicStampsDesignOnly(t *testing.T) {
	root := t.TempDir()
	stem := "260101-epic-sage"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "idea", stem+".md"),
		"---\ntitle: Sage\n---\n\nBody.\n")
	runner := &mockGitRunner{}

	result, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "todo",
		SageReview: "auto",
	})
	if err != nil {
		t.Fatalf("TicketsMove idea->todo: %v", err)
	}
	body := readFileString(t, filepath.Join(root, filepath.FromSlash(result.NewPath)))
	if !strings.Contains(body, "sage-review-design: required") {
		t.Fatalf("epic ticket missing sage-review-design: %s", body)
	}
	if strings.Contains(body, "sage-review-completeness") {
		t.Fatalf("epic ticket must not stamp sage-review-completeness: %s", body)
	}
	if !strings.Contains(result.Tip, "design required") || strings.Contains(result.Tip, "completeness") {
		t.Fatalf("Tip = %q, want design-only mention", result.Tip)
	}
}

func TestTicketsMoveUpwardToTodoExemptCategoriesStampNoSageReviewField(t *testing.T) {
	for _, category := range []string{"research", "workset"} {
		t.Run(category, func(t *testing.T) {
			root := t.TempDir()
			stem := "260101-" + category + "-sage"
			mustWrite(t, root, filepath.Join("ai-docs", "tickets", "idea", stem+".md"),
				"---\ntitle: Sage\n---\n\nBody.\n")
			runner := &mockGitRunner{}

			result, err := TicketsMove(root, runner, TicketMoveOptions{
				TicketStem: stem,
				To:         "todo",
				SageReview: "auto",
			})
			if err != nil {
				t.Fatalf("TicketsMove idea->todo: %v", err)
			}
			body := readFileString(t, filepath.Join(root, filepath.FromSlash(result.NewPath)))
			if strings.Contains(body, "sage-review") {
				t.Fatalf("exempt category ticket must not contain sage-review: %s", body)
			}
			if result.Tip != "" {
				t.Fatalf("Tip = %q, want empty for exempt category", result.Tip)
			}
		})
	}
}

// TestTicketsMoveUpwardToReadyWarnsOnUnresolvedSageReviewPosture asserts the
// de-blocked mutation-time path: a ready/ landing with a non-terminal
// required sage-review stage now succeeds (ws/git.commit's
// ready-sage-posture guardrail is the sole HARD enforcement point) and
// carries the "unreviewed" warning variant naming the first non-terminal
// field, mirroring readyPostureProblems' design-before-completeness order.
func TestTicketsMoveUpwardToReadyWarnsOnUnresolvedSageReviewPosture(t *testing.T) {
	for _, tc := range []struct {
		name     string
		body     string
		config   string
		want     map[string]string
		wantWarn string
	}{
		{
			name:     "legacy-pending-ask",
			body:     "sage-review: pending\n",
			config:   "ask",
			want:     map[string]string{"sage-review-design": "recommended", "sage-review-completeness": "recommended"},
			wantWarn: "sage-review-design is unreviewed (posture recommended; review has not run yet)",
		},
		{
			name:     "legacy-pending-auto",
			body:     "sage-review: pending\n",
			config:   "auto",
			want:     map[string]string{"sage-review-design": "required", "sage-review-completeness": "required"},
			wantWarn: "sage-review-design is unreviewed (posture required; review has not run yet)",
		},
		{
			name:     "design-recommended",
			body:     "sage-review-design: recommended\n",
			config:   "off",
			want:     map[string]string{"sage-review-design": "recommended", "sage-review-completeness": "skipped"},
			wantWarn: "sage-review-design is unreviewed (posture recommended; review has not run yet)",
		},
		{
			name:     "design-required",
			body:     "sage-review-design: required\n",
			config:   "off",
			want:     map[string]string{"sage-review-design": "required", "sage-review-completeness": "skipped"},
			wantWarn: "sage-review-design is unreviewed (posture required; review has not run yet)",
		},
		{
			name:     "design-blocked",
			body:     "sage-review-design: blocked\n",
			config:   "auto",
			want:     map[string]string{"sage-review-design": "blocked"},
			wantWarn: "sage-review-design is blocked (a prior review found unresolved issues)",
		},
		{
			name:     "design-terminal-completeness-recommended",
			body:     "sage-review-design: completed\nsage-review-completeness: recommended\n",
			config:   "off",
			want:     map[string]string{"sage-review-design": "completed", "sage-review-completeness": "recommended"},
			wantWarn: "sage-review-completeness is unreviewed (posture recommended; review has not run yet)",
		},
		{
			name:     "design-terminal-completeness-blocked",
			body:     "sage-review-design: skipped\nsage-review-completeness: blocked\n",
			config:   "off",
			want:     map[string]string{"sage-review-completeness": "blocked"},
			wantWarn: "sage-review-completeness is blocked (a prior review found unresolved issues)",
		},
		{
			// Hard invariant preserved even as a warning: design not-terminal
			// is still reported first even when completeness is already
			// terminal.
			name:     "design-recommended-completeness-completed",
			body:     "sage-review-design: recommended\nsage-review-completeness: completed\n",
			config:   "off",
			want:     map[string]string{"sage-review-design": "recommended", "sage-review-completeness": "completed"},
			wantWarn: "sage-review-design is unreviewed (posture recommended; review has not run yet)",
		},
		{
			name:     "absent-auto",
			body:     "",
			config:   "auto",
			want:     map[string]string{"sage-review-design": "required", "sage-review-completeness": "required"},
			wantWarn: "sage-review-design is unreviewed (posture required; review has not run yet)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			stem := "260101-feat-sage-" + tc.name
			body := "---\ntitle: Sage\n" + tc.body + "---\n\nBody.\n"
			oldRel := filepath.Join("ai-docs", "tickets", "todo", stem+".md")
			mustWrite(t, root, oldRel, body)
			runner := &mockGitRunner{}

			result, err := TicketsMove(root, runner, TicketMoveOptions{
				TicketStem: stem,
				To:         "ready",
				SageReview: tc.config,
			})
			if err != nil {
				t.Fatalf("TicketsMove: %v", err)
			}
			if !strings.Contains(result.Tip, tc.wantWarn) {
				t.Fatalf("Tip = %q, want it to contain %q", result.Tip, tc.wantWarn)
			}
			after := readFileString(t, filepath.Join(root, filepath.FromSlash(result.NewPath)))
			for field, want := range tc.want {
				wantLine := field + ": " + want
				if !strings.Contains(after, wantLine) {
					t.Fatalf("ticket missing %s after validation:\n%s", wantLine, after)
				}
			}
			if len(runner.calls) == 0 {
				t.Fatalf("git was not called; the move must still succeed (soft warning only, not a block)")
			}
		})
	}
}

// TestTicketsMoveUpwardToReadyFromIdeaWarnsOnUnresolvedSageReviewPosture is a
// variant of the "absent-auto" case in
// TestTicketsMoveUpwardToReadyWarnsOnUnresolvedSageReviewPosture, but places
// the ticket fixture under idea/ instead of todo/ before promoting straight
// to ready. The ticket's stated hard invariant names "idea->ready" as an
// entry path that must never skip design review; this makes that coverage
// self-evident rather than relying on an implicit "curStatus is irrelevant"
// argument about prepareSageReviewForUpwardMove. The move still succeeds
// (single chokepoint moved to ws/git.commit); the design-unreviewed warning
// still fires.
func TestTicketsMoveUpwardToReadyFromIdeaWarnsOnUnresolvedSageReviewPosture(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-sage-from-idea"
	oldRel := filepath.Join("ai-docs", "tickets", "idea", stem+".md")
	mustWrite(t, root, oldRel, "---\ntitle: Sage\n---\n\nBody.\n")
	runner := &mockGitRunner{}

	result, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "ready",
		SageReview: "auto",
	})
	if err != nil {
		t.Fatalf("TicketsMove idea->ready: %v", err)
	}
	if !strings.Contains(result.Tip, "sage-review-design is unreviewed (posture required; review has not run yet)") {
		t.Fatalf("Tip = %q, want design-required warning", result.Tip)
	}
	after := readFileString(t, filepath.Join(root, filepath.FromSlash(result.NewPath)))
	for _, wantLine := range []string{"sage-review-design: required", "sage-review-completeness: required"} {
		if !strings.Contains(after, wantLine) {
			t.Fatalf("ticket missing %s after validation:\n%s", wantLine, after)
		}
	}
	if len(runner.calls) == 0 {
		t.Fatalf("git was not called; the move must still succeed (soft warning only, not a block)")
	}
}

func TestTicketsMoveUpwardToReadyEpicOnlyChecksDesign(t *testing.T) {
	root := t.TempDir()
	stem := "260101-epic-checked"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
		"---\ntitle: Epic\nsage-review-design: completed\nsage-review-completeness: blocked\n---\n\nBody.\n")
	runner := &mockGitRunner{}

	result, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "ready",
		SageReview: "auto",
	})
	if err != nil {
		t.Fatalf("TicketsMove epic ready promotion should ignore completeness: %v", err)
	}
	if strings.Contains(result.Tip, "completeness") {
		t.Fatalf("Tip = %q, want no completeness mention for epic", result.Tip)
	}
}

// TestTicketsMoveUpwardToReadyEpicWarnsOnUnresolvedDesign complements
// TestTicketsMoveUpwardToReadyEpicOnlyChecksDesign: that test only covers the
// terminal/ignore-completeness case (design completed, completeness
// blocked, promotion succeeds). This asserts the epic-specific gate still
// surfaces the warning when sage-review-design itself is non-terminal (the
// move itself now always succeeds; ws/git.commit is the sole hard gate).
func TestTicketsMoveUpwardToReadyEpicWarnsOnUnresolvedDesign(t *testing.T) {
	root := t.TempDir()
	stem := "260101-epic-unresolved"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
		"---\ntitle: Epic\nsage-review-design: recommended\n---\n\nBody.\n")
	runner := &mockGitRunner{}

	result, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "ready",
		SageReview: "auto",
	})
	if err != nil {
		t.Fatalf("TicketsMove epic ready promotion with unresolved design: %v", err)
	}
	if !strings.Contains(result.Tip, "sage-review-design is unreviewed (posture recommended; review has not run yet)") {
		t.Fatalf("Tip = %q, want design-recommended warning", result.Tip)
	}
	if len(runner.calls) == 0 {
		t.Fatalf("git was not called; the move must still succeed (soft warning only, not a block)")
	}
}

func TestTicketsMoveUpwardToReadyExemptCategoriesNoFieldTouchedNoError(t *testing.T) {
	for _, category := range []string{"research", "workset"} {
		t.Run(category, func(t *testing.T) {
			root := t.TempDir()
			stem := "260101-" + category + "-untouched"
			mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
				"---\ntitle: Exempt\n---\n\nBody.\n")
			runner := &mockGitRunner{}

			result, err := TicketsMove(root, runner, TicketMoveOptions{
				TicketStem: stem,
				To:         "ready",
				SageReview: "auto",
			})
			if err != nil {
				t.Fatalf("TicketsMove: %v", err)
			}
			body := readFileString(t, filepath.Join(root, filepath.FromSlash(result.NewPath)))
			if strings.Contains(body, "sage-review") {
				t.Fatalf("exempt category ticket must not contain sage-review: %s", body)
			}
		})
	}
}

func TestTicketsMoveUpwardToReadyLegacyCompletedMigratesToBothFieldsTerminal(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-legacy-completed"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
		"---\ntitle: Legacy\nsage-review: completed\n---\n\nBody.\n")
	runner := &mockGitRunner{}

	result, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "ready",
		SageReview: "auto",
	})
	if err != nil {
		t.Fatalf("TicketsMove legacy completed: %v", err)
	}
	body := readFileString(t, filepath.Join(root, filepath.FromSlash(result.NewPath)))
	for _, field := range []string{"sage-review-design", "sage-review-completeness"} {
		wantLine := field + ": completed"
		if !strings.Contains(body, wantLine) {
			t.Fatalf("migrated ticket missing %s:\n%s", wantLine, body)
		}
	}
}

// TestTicketsMoveUpwardToReadyLegacyBlockedWarnsDistinctly asserts the
// `blocked` variant of the mutation-time warning: it de-blocks the move too
// (per ticket decision) but must read as a genuinely distinct message from
// the "unreviewed" variant ("a prior review found unresolved issues" vs.
// "review has not run"), since the two imply different next actions.
func TestTicketsMoveUpwardToReadyLegacyBlockedWarnsDistinctly(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-legacy-blocked"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
		"---\ntitle: Legacy\nsage-review: blocked\n---\n\nBody.\n")
	runner := &mockGitRunner{}

	result, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "ready",
		SageReview: "auto",
	})
	if err != nil {
		t.Fatalf("TicketsMove legacy-blocked promotion: %v", err)
	}
	if !strings.Contains(result.Tip, "sage-review-design is blocked (a prior review found unresolved issues)") {
		t.Fatalf("Tip = %q, want the blocked-variant warning", result.Tip)
	}
	if strings.Contains(result.Tip, "review has not run yet") {
		t.Fatalf("Tip = %q, blocked warning must not reuse the unreviewed variant's wording", result.Tip)
	}
	if len(runner.calls) == 0 {
		t.Fatalf("git was not called; the move must still succeed (soft warning only, not a block)")
	}
}

func TestTicketsMoveToReadyWarnsWhenNoSpecAddressing(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-nospec"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
		"---\ntitle: NoSpec\nsage-review: skipped\n---\n\nBody.\n")
	runner := &mockGitRunner{}

	result, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "ready",
		SageReview: "off",
	})
	if err != nil {
		t.Fatalf("TicketsMove: %v", err)
	}
	if !strings.Contains(result.Tip, "ready gate is normally enforced by lead-write-ticket") {
		t.Fatalf("Tip = %q, want ready-gate warning", result.Tip)
	}
}

func TestTicketsMoveToReadySucceedsDespiteMissingSpecAddressing(t *testing.T) {
	// The warning is advisory only; the move must still succeed and the file
	// must land at the requested destination.
	root := t.TempDir()
	stem := "260101-feat-nospec-succeeds"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
		"---\ntitle: NoSpec\nsage-review: skipped\n---\n\nBody.\n")
	runner := &mockGitRunner{}

	result, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "ready",
		SageReview: "off",
	})
	if err != nil {
		t.Fatalf("TicketsMove: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(result.NewPath))); statErr != nil {
		t.Fatalf("moved ticket missing at %s: %v", result.NewPath, statErr)
	}
}

func TestTicketsMoveToReadyNoWarningWithSpecFrontmatter(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-withspec"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
		"---\ntitle: WithSpec\nsage-review: skipped\nspec: 260101-spec-example\n---\n\nBody.\n")
	runner := &mockGitRunner{}

	result, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "ready",
		SageReview: "off",
	})
	if err != nil {
		t.Fatalf("TicketsMove: %v", err)
	}
	if strings.Contains(result.Tip, "ready gate") {
		t.Fatalf("Tip = %q, want no ready-gate warning when spec: is set", result.Tip)
	}
}

func TestTicketsMoveToReadyNoWarningWithSpecRemoveFrontmatter(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-withspecremove"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
		"---\ntitle: WithSpecRemove\nsage-review: skipped\nspec-remove: 260101-spec-old\n---\n\nBody.\n")
	runner := &mockGitRunner{}

	result, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "ready",
		SageReview: "off",
	})
	if err != nil {
		t.Fatalf("TicketsMove: %v", err)
	}
	if strings.Contains(result.Tip, "ready gate") {
		t.Fatalf("Tip = %q, want no ready-gate warning when spec-remove: is set", result.Tip)
	}
}

func TestTicketsMoveToReadyNoWarningWithSpecImpactSection(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-withimpact"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
		"---\ntitle: WithImpact\nsage-review: skipped\n---\n\nBody.\n\n## Spec Impact\n\nDetails.\n")
	runner := &mockGitRunner{}

	result, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "ready",
		SageReview: "off",
	})
	if err != nil {
		t.Fatalf("TicketsMove: %v", err)
	}
	if strings.Contains(result.Tip, "ready gate") {
		t.Fatalf("Tip = %q, want no ready-gate warning when ## Spec Impact is present", result.Tip)
	}
}

func TestTicketsMoveToReadyNoWarningForExemptCategories(t *testing.T) {
	for _, category := range []string{"epic", "research", "workset"} {
		t.Run(category, func(t *testing.T) {
			root := t.TempDir()
			stem := "260101-" + category + "-sample"
			mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
				"---\ntitle: Exempt\nsage-review: skipped\n---\n\nBody.\n")
			runner := &mockGitRunner{}

			result, err := TicketsMove(root, runner, TicketMoveOptions{
				TicketStem: stem,
				To:         "ready",
				SageReview: "off",
			})
			if err != nil {
				t.Fatalf("TicketsMove: %v", err)
			}
			if strings.Contains(result.Tip, "ready gate") {
				t.Fatalf("Tip = %q, want no ready-gate warning for exempt category %s", result.Tip, category)
			}
		})
	}
}

func TestTicketsMoveToReadyCombinesSageTipAndReadyGateWarning(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-combo"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "idea", stem+".md"),
		"---\ntitle: Combo\nsage-review: skipped\n---\n\nBody.\n")
	runner := &mockGitRunner{}

	result, err := TicketsMove(root, runner, TicketMoveOptions{
		TicketStem: stem,
		To:         "ready",
		SageReview: "off",
	})
	if err != nil {
		t.Fatalf("TicketsMove: %v", err)
	}
	if !strings.Contains(result.Tip, "design skipped") || !strings.Contains(result.Tip, "completeness skipped") {
		t.Fatalf("Tip = %q, want sage review posture tip for both stages", result.Tip)
	}
	if !strings.Contains(result.Tip, "ready gate is normally enforced by lead-write-ticket") {
		t.Fatalf("Tip = %q, want ready-gate warning", result.Tip)
	}
}

func TestTicketsMoveUpwardToReadyAllowsResolvedSageReviewPosture(t *testing.T) {
	for _, posture := range []string{"completed", "skipped"} {
		t.Run(posture, func(t *testing.T) {
			root := t.TempDir()
			stem := "260101-feat-sage-" + posture
			mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
				"---\ntitle: Sage\nsage-review: "+posture+"\n---\n\nBody.\n")
			runner := &mockGitRunner{}

			result, err := TicketsMove(root, runner, TicketMoveOptions{
				TicketStem: stem,
				To:         "ready",
				SageReview: "auto",
			})
			if err != nil {
				t.Fatalf("TicketsMove blocked %s sage-review: %v", posture, err)
			}
			if !strings.Contains(result.Tip, posture) {
				t.Fatalf("Tip = %q, want posture %q", result.Tip, posture)
			}
		})
	}
}
