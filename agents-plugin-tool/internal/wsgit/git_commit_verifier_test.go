// Tests for Git commit verifier handling.
package wsgit

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestCommitBlockedByVerifierNeverReachesCommit(t *testing.T) {
	root := t.TempDir()
	ticketPath := "ai-docs/tickets/todo/not-a-valid-stem.md"
	mustWriteGitTestFixture(t, root, ticketPath, "---\ntitle: Bad\n---\n\nBody.\n")

	runner := &sequenceRunner{outs: [][]byte{
		[]byte("main\n"), // symbolic-ref --short HEAD (branch guard)
		{},               // pre-status
		{},               // add
		[]byte("1 A. N... 100644 100644 100644 aaa bbb " + ticketPath + "\n"), // post-status
	}}
	verifyErr := errors.New("ticket verify failed: stem does not match the ticket stem pattern")
	client := Client{Runner: runner, Verifier: func(gotRoot string, gotPaths []string) ([]string, error) {
		if gotRoot != root {
			t.Fatalf("verifier root = %q, want %q", gotRoot, root)
		}
		if len(gotPaths) != 1 || gotPaths[0] != ticketPath {
			t.Fatalf("verifier paths = %#v, want [%q]", gotPaths, ticketPath)
		}
		return nil, verifyErr
	}}

	_, err := client.Commit(context.Background(), root, CommitOptions{
		Paths:          []string{ticketPath},
		Title:          "test: blocked by verifier",
		AIContext:      []string{"User intent: prove the commit gate blocks an invalid ticket."},
		ExpectedBranch: "main",
	})
	if !errors.Is(err, verifyErr) {
		t.Fatalf("Commit error = %v, want %v", err, verifyErr)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("runner.calls = %#v, want exactly 4 (branch guard, pre-status, add, post-status) — verifier must block before ticket-change detection or `git commit`", runner.calls)
	}
	for _, call := range runner.calls {
		if len(call.args) > 0 && call.args[0] == "commit" {
			t.Fatalf("Commit issued `git commit` despite a blocking Verifier: %#v", runner.calls)
		}
	}
}

// TestCommitVetoDiscardsAdvisoriesAlongsideError pins the intended-but-until-
// now-implicit behavior at Commit's verifier call site: a Verifier that
// returns both advisories and a non-nil error must still veto the commit,
// and the advisories it returned must not leak into the (zero-value)
// CommitResult — a vetoed commit has nothing to advise on.
func TestCommitVetoDiscardsAdvisoriesAlongsideError(t *testing.T) {
	root := t.TempDir()
	ticketPath := "ai-docs/tickets/todo/not-a-valid-stem.md"
	mustWriteGitTestFixture(t, root, ticketPath, "---\ntitle: Bad\n---\n\nBody.\n")

	runner := &sequenceRunner{outs: [][]byte{
		[]byte("main\n"), // symbolic-ref --short HEAD (branch guard)
		{},               // pre-status
		{},               // add
		[]byte("1 A. N... 100644 100644 100644 aaa bbb " + ticketPath + "\n"), // post-status
	}}
	verifyErr := errors.New("ticket verify failed: stem does not match the ticket stem pattern")
	client := Client{Runner: runner, Verifier: func(string, []string) ([]string, error) {
		return []string{"WARN [unresolved-phases] should never surface: commit is vetoed"}, verifyErr
	}}

	result, err := client.Commit(context.Background(), root, CommitOptions{
		Paths:          []string{ticketPath},
		Title:          "test: veto discards advisories",
		AIContext:      []string{"User intent: prove a veto's advisories never leak into CommitResult."},
		ExpectedBranch: "main",
	})
	if !errors.Is(err, verifyErr) {
		t.Fatalf("Commit error = %v, want %v", err, verifyErr)
	}
	if len(result.Advisories) != 0 {
		t.Fatalf("result.Advisories = %#v, want empty — a vetoed commit must discard advisories returned alongside its error", result.Advisories)
	}
}

// TestCommitProceedsWhenVerifierNilDefaultsToNoOp asserts the DI seam's
// backward-compat default: a Client built without a Verifier (the zero
// value, as every pre-existing Client{Runner: ...} test literal in this file
// still constructs) must commit exactly as before this gate existed.
func TestCommitProceedsWhenVerifierNilDefaultsToNoOp(t *testing.T) {
	root := t.TempDir()
	ticketPath := "ai-docs/tickets/todo/260503-feat-demo.md"
	mustWriteGitTestFixture(t, root, ticketPath, "---\ntitle: Demo\n---\n\nBody.\n")

	runner := &sequenceRunner{outs: [][]byte{
		[]byte("main\n"), // symbolic-ref --short HEAD (branch guard)
		{},               // pre-status
		{},               // add
		[]byte("1 A. N... 100644 100644 100644 aaa bbb " + ticketPath + "\n"), // post-status
		{},                 // detectTicketChanges name-status
		{},                 // detectTicketChanges unified diff
		{},                 // commit -m
		[]byte("abc123\n"), // rev-parse HEAD
	}}
	client := Client{Runner: runner}

	result, err := client.Commit(context.Background(), root, CommitOptions{
		Paths:          []string{ticketPath},
		Title:          "test: nil verifier is a no-op",
		AIContext:      []string{"User intent: prove nil Verifier does not change existing Commit behavior."},
		ExpectedBranch: "main",
	})
	if err != nil {
		t.Fatalf("Commit returned error with nil Verifier: %v", err)
	}
	if result.Hash != "abc123" {
		t.Fatalf("result.Hash = %q, want abc123", result.Hash)
	}
	if len(runner.calls) != 8 || runner.calls[6].args[0] != "commit" {
		t.Fatalf("runner.calls = %#v, want `git commit` to run at call index 6", runner.calls)
	}
}

// TestCommitPromotionVerifierSeesOnlyDestinationPath exercises the ticket's
// named "promotion" verification boundary: a staged todo/ -> ready/ rename
// must commit, and the Verifier must see only the destination path (the
// source path is unreadable post-rename, so handing it to
// wsdoc.TicketVerify's file-exists guardrail would wrongly fail the commit).
func TestCommitPromotionVerifierSeesOnlyDestinationPath(t *testing.T) {
	root := "/repo"
	preStatus := "1 .D N... 100644 100644 100644 aaa bbb ai-docs/tickets/todo/260503-feat-demo.md\n? ai-docs/tickets/ready/260503-feat-demo.md\n"
	postStatus := "2 R. N... 100644 100644 100644 aaa bbb R100 ai-docs/tickets/ready/260503-feat-demo.md\tai-docs/tickets/todo/260503-feat-demo.md\n"
	runner := &sequenceRunner{outs: [][]byte{
		[]byte("main\n"),   // symbolic-ref --short HEAD (branch guard)
		[]byte(preStatus),  // pre-status
		{},                 // add ready/...
		{},                 // rm --cached todo/...
		[]byte(postStatus), // post-status
		{},                 // detectTicketChanges name-status
		{},                 // detectTicketChanges unified diff
		{},                 // commit -m
		[]byte("abc123\n"), // rev-parse HEAD
	}}
	var gotPaths []string
	client := Client{Runner: runner, Verifier: func(gotRoot string, paths []string) ([]string, error) {
		gotPaths = paths
		if gotRoot != root {
			t.Fatalf("verifier root = %q, want %q", gotRoot, root)
		}
		return nil, nil
	}}

	result, err := client.Commit(context.Background(), root, CommitOptions{
		Paths:          []string{"ai-docs/tickets/ready/260503-feat-demo.md"},
		Title:          "docs(ticket): promote demo ticket",
		AIContext:      []string{"User intent: promote a ticket from todo to ready."},
		ExpectedBranch: "main",
	})
	if err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}
	if result.Hash != "abc123" {
		t.Fatalf("result.Hash = %q, want abc123", result.Hash)
	}
	want := []string{"ai-docs/tickets/ready/260503-feat-demo.md"}
	if !reflect.DeepEqual(gotPaths, want) {
		t.Fatalf("verifier paths = %#v, want %#v", gotPaths, want)
	}
}

// TestCommitCloseVerifierSeesOnlyDestinationPath mirrors the promotion case
// for tickets.close's todo/ -> .dropped/ transition, per the ticket's
// explicit "close path" regression requirement.
func TestCommitCloseVerifierSeesOnlyDestinationPath(t *testing.T) {
	root := "/repo"
	preStatus := "1 .D N... 100644 100644 100644 aaa bbb ai-docs/tickets/todo/260503-feat-demo.md\n? ai-docs/tickets/.dropped/260503-feat-demo.md\n"
	postStatus := "2 R. N... 100644 100644 100644 aaa bbb R100 ai-docs/tickets/.dropped/260503-feat-demo.md\tai-docs/tickets/todo/260503-feat-demo.md\n"
	runner := &sequenceRunner{outs: [][]byte{
		[]byte("main\n"), // symbolic-ref --short HEAD (branch guard)
		[]byte(preStatus),
		{},
		{},
		[]byte(postStatus),
		{},
		{},
		{},
		[]byte("abc123\n"),
	}}
	var gotPaths []string
	client := Client{Runner: runner, Verifier: func(_ string, paths []string) ([]string, error) {
		gotPaths = paths
		return nil, nil
	}}

	result, err := client.Commit(context.Background(), root, CommitOptions{
		Paths:          []string{"ai-docs/tickets/.dropped/260503-feat-demo.md"},
		Title:          "docs(ticket): drop demo ticket",
		AIContext:      []string{"User intent: close a ticket by dropping it."},
		ExpectedBranch: "main",
	})
	if err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}
	if result.Hash != "abc123" {
		t.Fatalf("result.Hash = %q, want abc123", result.Hash)
	}
	want := []string{"ai-docs/tickets/.dropped/260503-feat-demo.md"}
	if !reflect.DeepEqual(gotPaths, want) {
		t.Fatalf("verifier paths = %#v, want %#v", gotPaths, want)
	}
}

// TestCommitOutrightDeletionSkipsVerifierEntirely covers the risk flagged in
// the plan: filtering opts.Paths down to empty (the sole staged path was an
// outright ticket deletion) must skip the Verifier call altogether rather
// than invoking it with an empty slice, since wsdoc.TicketVerify treats an
// empty paths argument as a caller-input error.
func TestCommitOutrightDeletionSkipsVerifierEntirely(t *testing.T) {
	root := "/repo"
	preStatus := "1 .D N... 100644 100644 100644 aaa bbb ai-docs/tickets/idea/260503-feat-demo.md\n"
	postStatus := "1 D. N... 100644 000000 000000 aaa 0000000000000000000000000000000000000000 ai-docs/tickets/idea/260503-feat-demo.md\n"
	runner := &sequenceRunner{outs: [][]byte{
		[]byte("main\n"), // symbolic-ref --short HEAD (branch guard)
		[]byte(preStatus),
		{},
		[]byte(postStatus),
		{},
		{},
		{},
		[]byte("abc123\n"),
	}}
	verifierCalled := false
	client := Client{Runner: runner, Verifier: func(string, []string) ([]string, error) {
		verifierCalled = true
		return nil, nil
	}}

	result, err := client.Commit(context.Background(), root, CommitOptions{
		Paths:          []string{"ai-docs/tickets/idea/260503-feat-demo.md"},
		Title:          "docs(ticket): delete demo ticket",
		AIContext:      []string{"User intent: delete a ticket outright."},
		ExpectedBranch: "main",
	})
	if err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}
	if result.Hash != "abc123" {
		t.Fatalf("result.Hash = %q, want abc123", result.Hash)
	}
	if verifierCalled {
		t.Fatalf("Verifier was invoked for an outright deletion with no remaining paths to verify")
	}
	if len(result.Advisories) != 0 {
		t.Fatalf("result.Advisories = %#v, want empty when the Verifier is skipped entirely", result.Advisories)
	}
}

// TestCommitDivergentCaseExcludesRenameOldPathEvenWhenCallerPassedIt pins the
// strategy choice documented in the plan: when the caller explicitly passes
// both sides of a staged rename alongside an unrelated ticket's own path,
// the Verifier must still see only the rename's destination path plus the
// unrelated path — not the rename's stale old path.
func TestCommitContentOnlyEditLeavesVerifierPathsUnmodified(t *testing.T) {
	root := "/repo"
	ticketPath := "ai-docs/tickets/todo/260503-feat-demo.md"
	postStatus := "1 A. N... 100644 100644 100644 aaa bbb " + ticketPath + "\n"
	runner := &sequenceRunner{outs: [][]byte{
		[]byte("main\n"), // symbolic-ref --short HEAD (branch guard)
		{},
		{},
		[]byte(postStatus),
		{},
		{},
		{},
		[]byte("abc123\n"),
	}}
	var gotPaths []string
	client := Client{Runner: runner, Verifier: func(_ string, paths []string) ([]string, error) {
		gotPaths = paths
		return nil, nil
	}}

	result, err := client.Commit(context.Background(), root, CommitOptions{
		Paths:          []string{ticketPath},
		Title:          "docs(ticket): edit demo ticket body",
		AIContext:      []string{"User intent: a plain content-only ticket edit."},
		ExpectedBranch: "main",
	})
	if err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}
	if result.Hash != "abc123" {
		t.Fatalf("result.Hash = %q, want abc123", result.Hash)
	}
	want := []string{ticketPath}
	if !reflect.DeepEqual(gotPaths, want) {
		t.Fatalf("verifier paths = %#v, want %#v (filter must be a no-op for content-only edits)", gotPaths, want)
	}
}

// TestCommitStagesCapturedIdeaTicketUnderSparseScopeThenSelfHides is the
// integration-style regression pin for the #260810 lifecycle: under a real
// `--no-cone` sparse-checkout scope that excludes `ai-docs/tickets/idea/*`
// (re-including only the tracked keep-file), a newly captured idea ticket
// stages and commits via `Client.Commit(SparseScopeActive: true)` with no
// manual widen, and then self-hides (returns to skip-worktree) only on the
// next `git sparse-checkout reapply` — while staying readable from the index
// (`git show HEAD:<path>`) throughout. Exercises real `git` subprocesses
// (ExecRunner), not the fake sequenceRunner used elsewhere in this file,
// since the self-hiding behavior depends on git's own skip-worktree
// re-evaluation.
