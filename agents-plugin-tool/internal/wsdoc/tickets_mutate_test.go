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

func TestTicketsMoveUpwardToTodoStampsResolvedSageReviewPosture(t *testing.T) {
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
			wantLine := "sage-review: " + tc.wantReview
			if !strings.Contains(body, wantLine) {
				t.Fatalf("moved ticket missing %s:\n%s", wantLine, body)
			}
			if !strings.Contains(result.Tip, tc.wantReview) {
				t.Fatalf("Tip = %q, want resolved posture %q", result.Tip, tc.wantReview)
			}
		})
	}
}

func TestTicketsMoveUpwardToReadyBlocksUnresolvedSageReviewPosture(t *testing.T) {
	for _, tc := range []struct {
		name       string
		bodyReview string
		config     string
		wantReview string
		wantErr    string
	}{
		{"legacy-pending-ask", "pending", "ask", "recommended", "skip recommended review"},
		{"legacy-pending-auto", "pending", "auto", "required", "run sage review"},
		{"recommended", "recommended", "off", "recommended", "skip recommended review"},
		{"required", "required", "off", "required", "run sage review"},
		{"blocked", "blocked", "auto", "blocked", "address blocked review"},
		{"absent-auto", "", "auto", "required", "run sage review"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			stem := "260101-feat-sage-" + tc.name
			body := "---\ntitle: Sage\n"
			if tc.bodyReview != "" {
				body += "sage-review: " + tc.bodyReview + "\n"
			}
			body += "---\n\nBody.\n"
			oldRel := filepath.Join("ai-docs", "tickets", "todo", stem+".md")
			oldAbs := filepath.Join(root, oldRel)
			mustWrite(t, root, oldRel, body)
			runner := &mockGitRunner{}

			if _, err := TicketsMove(root, runner, TicketMoveOptions{
				TicketStem: stem,
				To:         "ready",
				SageReview: tc.config,
			}); err == nil {
				t.Fatalf("TicketsMove promoted unresolved sage-review %q", tc.bodyReview)
			} else if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			after := readFileString(t, oldAbs)
			wantLine := "sage-review: " + tc.wantReview
			if !strings.Contains(after, wantLine) {
				t.Fatalf("ticket missing %s after validation:\n%s", wantLine, after)
			}
			if len(runner.calls) != 0 {
				t.Fatalf("git called on guard failure: %#v", runner.calls)
			}
		})
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
