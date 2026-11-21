// Tests for guarded Git commits and commit messages.
package wsgit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestCommitStagesExplicitPathsAndBuildsMessage(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		[]byte("main\n"), // symbolic-ref --short HEAD (branch guard)
		{},
		{},
		[]byte("1 A. N... 100644 100644 100644 aaa bbb src/file.go\n"),
		[]byte("M\tai-docs/tickets/todo/260503-feat-demo.md\n"),
		[]byte("diff --git a/ai-docs/tickets/todo/260503-feat-demo.md b/ai-docs/tickets/todo/260503-feat-demo.md\n+++ b/ai-docs/tickets/todo/260503-feat-demo.md\n+### Result (abc123) - 2026-05-04\n"),
		{},
		[]byte("abc123\n"),
	}}
	result, err := (Client{Runner: runner}).Commit(context.Background(), "/repo", CommitOptions{
		Paths:          []string{"src"},
		Title:          "feat(ws-mcp): add commit tool",
		Description:    "Builds a structured workflow commit.",
		AIContext:      []string{"User intent: make commit creation portable.", "Verification: unit test."},
		ExpectedBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Hash != "abc123" || result.Title != "feat(ws-mcp): add commit tool" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.TicketChanges) != 1 || !result.TicketChanges[0].ResultAdded {
		t.Fatalf("ticket changes = %#v", result.TicketChanges)
	}
	wantFirst := []string{"add", "-A", "--", "src"}
	if !reflect.DeepEqual(runner.calls[2].args, wantFirst) {
		t.Fatalf("add args = %#v, want %#v", runner.calls[2].args, wantFirst)
	}
	commitArgs := runner.calls[6].args
	if len(commitArgs) != 4 || commitArgs[0] != "commit" || commitArgs[1] != "-m" || commitArgs[3] != "--cleanup=whitespace" {
		t.Fatalf("commit args = %#v", commitArgs)
	}
	message := commitArgs[2]
	for _, want := range []string{"feat(ws-mcp): add commit tool", "Builds a structured workflow commit.", "## AI Context", "- User intent: make commit creation portable."} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %q:\n%s", want, message)
		}
	}
	if !strings.Contains(message, "## Ticket Updates") || !strings.Contains(message, "260503-feat-demo: added ### Result") {
		t.Fatalf("message missing auto ticket summary:\n%s", message)
	}
}

func TestCommitTicketUpdateInferenceControl(t *testing.T) {
	for _, tc := range []struct {
		name     string
		updates  []string
		suppress bool
		want     string
	}{
		{name: "direct nil retains inference", want: "260503-feat-demo: added ### Result (abc123) - 2026-05-04"},
		{name: "direct allocated empty retains inference", updates: []string{}, want: "260503-feat-demo: added ### Result (abc123) - 2026-05-04"},
		{name: "suppression survives normalization", updates: []string{}, suppress: true},
		{name: "explicit summaries remain authoritative", updates: []string{"  supplied summary  "}, suppress: true, want: "supplied summary"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &sequenceRunner{outs: [][]byte{
				[]byte("main\n"), {}, {},
				[]byte("1 M. N... 100644 100644 100644 aaa bbb ai-docs/tickets/todo/260503-feat-demo.md\n"),
				[]byte("M\tai-docs/tickets/todo/260503-feat-demo.md\n"),
				[]byte("diff --git a/ai-docs/tickets/todo/260503-feat-demo.md b/ai-docs/tickets/todo/260503-feat-demo.md\n+++ b/ai-docs/tickets/todo/260503-feat-demo.md\n+### Result (abc123) - 2026-05-04\n"),
				{}, []byte("abc123\n"),
			}}
			verified := false
			client := Client{Runner: runner, Verifier: func(root string, paths []string) ([]string, error) {
				verified = true
				if root != "/repo" || !reflect.DeepEqual(paths, []string{"ai-docs/tickets/todo/260503-feat-demo.md"}) {
					t.Fatalf("verification root/paths = %q/%v", root, paths)
				}
				return []string{"retained advisory"}, nil
			}}
			result, err := client.Commit(context.Background(), "/repo", CommitOptions{
				Paths: []string{"ai-docs/tickets/todo/260503-feat-demo.md"}, Title: "test: summaries",
				AIContext: []string{"retained context"}, ExpectedBranch: "main",
				UpdatedTickets: tc.updates, SuppressInferredTicketUpdates: tc.suppress,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !verified || !reflect.DeepEqual(result.Advisories, []string{"retained advisory"}) || len(result.TicketChanges) != 1 || !result.TicketChanges[0].ResultAdded {
				t.Fatalf("verification or metadata lost: verified=%v result=%#v", verified, result)
			}
			message := runner.calls[6].args[2]
			if !strings.Contains(message, "## AI Context\n- retained context") {
				t.Fatalf("context lost: %s", message)
			}
			if tc.want == "" {
				if strings.Contains(message, "## Ticket Updates") {
					t.Fatalf("suppressed summaries present: %s", message)
				}
			} else if !strings.HasSuffix(message, "## Ticket Updates\n- "+tc.want) {
				t.Fatalf("unexpected summaries: %s", message)
			}
		})
	}
}

func TestNormalizeCommitOptionsTicketUpdateSuppressionIsInternal(t *testing.T) {
	opts, err := normalizeCommitOptions(CommitOptions{
		Paths: []string{"src"}, Title: "test: internal flag", AIContext: []string{"context"},
		ExpectedBranch: "main", SuppressInferredTicketUpdates: true,
	})
	if err != nil || !opts.SuppressInferredTicketUpdates {
		t.Fatalf("normalize = %#v, %v", opts, err)
	}
	data, err := json.Marshal(opts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Suppress") || strings.Contains(string(data), "suppress") {
		t.Fatalf("internal flag serialized: %s", data)
	}
	var decoded CommitOptions
	if err := json.Unmarshal([]byte(`{"SuppressInferredTicketUpdates":true,"suppress_inferred_ticket_updates":true}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SuppressInferredTicketUpdates {
		t.Fatal("JSON set internal suppression flag")
	}
}

func TestCommitAcceptsLargeAIContextArray(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		[]byte("main\n"), // symbolic-ref --short HEAD (branch guard)
		{},
		{},
		[]byte("1 A. N... 100644 100644 100644 aaa bbb src/file.go\n"),
		{},
		{},
		{},
		[]byte("abc123\n"),
	}}
	aiContext := make([]string, 0, 60)
	for i := 0; i < 60; i++ {
		aiContext = append(aiContext, fmt.Sprintf("Entry %d: %s", i, strings.TrimSpace(strings.Repeat("context detail ", 20))))
	}
	result, err := (Client{Runner: runner}).Commit(context.Background(), "/repo", CommitOptions{
		Paths:          []string{"src"},
		Title:          "feat(ws-mcp): accept a large ai_context array",
		AIContext:      aiContext,
		ExpectedBranch: "main",
	})
	if err != nil {
		t.Fatalf("Commit error = %v, want nil", err)
	}
	if result.Hash != "abc123" {
		t.Fatalf("result = %#v", result)
	}
	commitArgs := runner.calls[6].args
	if len(commitArgs) != 4 || commitArgs[0] != "commit" || commitArgs[1] != "-m" || commitArgs[3] != "--cleanup=whitespace" {
		t.Fatalf("commit args = %#v", commitArgs)
	}
	message := commitArgs[2]
	if len(message) < 5000 {
		t.Fatalf("commit message too short for large ai_context, len=%d", len(message))
	}
	for _, want := range []string{aiContext[0], aiContext[len(aiContext)-1]} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing entry %q", want)
		}
	}
}

// The commit message renders exactly two structured sections: `## AI Context`
// and `## Ticket Updates`. Pinning the whole string is deliberate — the spec
// and mental-model trailer options this tool once carried were removed with
// their layer, and a full-message assertion fails if any of them is
// reintroduced, where a contains-check would not.
func TestCommitMessageRendersOnlyAIContextAndTicketSections(t *testing.T) {
	message := CommitMessage(CommitOptions{
		Title:          "docs(workflow): capture commit context",
		AIContext:      []string{"User intent: record commit-message context."},
		UpdatedTickets: []string{"260519-bug-git-commit-message-sections"},
	})

	want := strings.Join([]string{
		"docs(workflow): capture commit context",
		"",
		"## AI Context",
		"- User intent: record commit-message context.",
		"",
		"",
		"## Ticket Updates",
		"- 260519-bug-git-commit-message-sections",
	}, "\n")
	if message != want {
		t.Fatalf("CommitMessage =\n%s\nwant:\n%s", message, want)
	}
}

func TestCommitRefusesUnrelatedStagedPaths(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		[]byte("main\n"), // symbolic-ref --short HEAD (branch guard)
		{},
		{},
		[]byte("1 M. N... 100644 100644 100644 aaa bbb src/file.go\n1 M. N... 100644 100644 100644 aaa bbb docs/note.md\n"),
	}}
	_, err := (Client{Runner: runner}).Commit(context.Background(), "/repo", CommitOptions{
		Paths:          []string{"src/file.go"},
		Title:          "feat: scoped",
		AIContext:      []string{"User intent: scoped commit."},
		ExpectedBranch: "main",
	})
	if err == nil || !strings.Contains(err.Error(), "unrelated staged path") {
		t.Fatalf("Commit error = %v, want unrelated staged path", err)
	}
}

// TestCommitRefusesBranchMismatch pins the expected_branch guard: when the
// actual checkout differs from the believed branch, Commit refuses before any
// staging mutation and reports both branch names.
func TestCommitRefusesBranchMismatch(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		[]byte("impl/develop/other\n"), // symbolic-ref: actual branch differs from believed
	}}
	_, err := (Client{Runner: runner}).Commit(context.Background(), "/repo", CommitOptions{
		Paths:          []string{"src/file.go"},
		Title:          "feat: guarded",
		AIContext:      []string{"User intent: guarded commit."},
		ExpectedBranch: "develop",
	})
	if err == nil {
		t.Fatal("Commit did not refuse a believed/actual branch mismatch")
	}
	for _, want := range []string{"develop", "impl/develop/other", "believed", "actual"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Commit error = %v, want it to mention %q", err, want)
		}
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner.calls = %#v, want exactly 1 (branch guard) — a mismatch must refuse before any staging mutation", runner.calls)
	}
}

// TestCommitRefusesDetachedHead pins the detached-HEAD arm of the guard:
// `git symbolic-ref --quiet` fails with empty output on a detached HEAD, which
// Commit treats as no current branch and refuses before any staging mutation.
func TestCommitRefusesDetachedHead(t *testing.T) {
	runner := &sequenceRunner{
		outs: [][]byte{nil},
		errs: []error{errors.New("exit status 1")}, // symbolic-ref --quiet exits non-zero on detached HEAD
	}
	_, err := (Client{Runner: runner}).Commit(context.Background(), "/repo", CommitOptions{
		Paths:          []string{"src/file.go"},
		Title:          "feat: guarded",
		AIContext:      []string{"User intent: guarded commit."},
		ExpectedBranch: "develop",
	})
	if err == nil {
		t.Fatal("Commit did not refuse a detached HEAD")
	}
	if !strings.Contains(err.Error(), "detached") {
		t.Fatalf("Commit error = %v, want it to mention a detached HEAD", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner.calls = %#v, want exactly 1 (branch guard) — a detached HEAD must refuse before any staging mutation", runner.calls)
	}
}

// TestCommitRefusesImplBranchNamesEscapeHatch pins Decision 9: when the actual
// checkout is a worker's impl/ branch, the mismatch refusal appends the sparse
// housekeeping worktree escape hatch naming the parent as the base; a non-impl
// mismatch keeps the plain refusal. The detached-HEAD arm is unchanged
// (TestCommitRefusesDetachedHead).
func TestCommitRefusesImplBranchNamesEscapeHatch(t *testing.T) {
	implRunner := &sequenceRunner{outs: [][]byte{[]byte("impl/develop/foo\n")}}
	_, err := (Client{Runner: implRunner}).Commit(context.Background(), "/repo", CommitOptions{
		Paths: []string{"src/file.go"}, Title: "feat: x", AIContext: []string{"intent"}, ExpectedBranch: "develop",
	})
	if err == nil {
		t.Fatal("Commit did not refuse the mismatch")
	}
	if !strings.Contains(err.Error(), `worktree.acquire(base: "develop"`) {
		t.Fatalf("impl-branch refusal must name worktree.acquire(base: \"develop\"): %v", err)
	}

	featRunner := &sequenceRunner{outs: [][]byte{[]byte("feature/x\n")}}
	_, err = (Client{Runner: featRunner}).Commit(context.Background(), "/repo", CommitOptions{
		Paths: []string{"src/file.go"}, Title: "feat: x", AIContext: []string{"intent"}, ExpectedBranch: "develop",
	})
	if err == nil {
		t.Fatal("Commit did not refuse the mismatch")
	}
	if strings.Contains(err.Error(), "worktree.acquire") {
		t.Fatalf("non-impl refusal must not name the escape hatch: %v", err)
	}

	// Rootless impl/<stem>: no parent segment, so implBranchParent returns "" and
	// the refusal falls back to the "branch this worker was spawned from" phrasing
	// instead of naming a quoted base — the mcp twin already pins this case in
	// occupied_root_announcement_test.go's TestOccupiedRootAnnouncementTextVariants.
	rootlessRunner := &sequenceRunner{outs: [][]byte{[]byte("impl/foo\n")}}
	_, err = (Client{Runner: rootlessRunner}).Commit(context.Background(), "/repo", CommitOptions{
		Paths: []string{"src/file.go"}, Title: "feat: x", AIContext: []string{"intent"}, ExpectedBranch: "develop",
	})
	if err == nil {
		t.Fatal("Commit did not refuse the mismatch")
	}
	if !strings.Contains(err.Error(), "worktree.acquire(base: <the branch this worker was spawned from>") {
		t.Fatalf("rootless impl-branch refusal must fall back to the spawned-from phrasing: %v", err)
	}
	if strings.Contains(err.Error(), `base: "`) {
		t.Fatalf("rootless impl-branch refusal must not name a quoted base: %v", err)
	}
}

func TestCommitRequiresAIContextAndRelativePaths(t *testing.T) {
	t.Run("absent field", func(t *testing.T) {
		_, err := normalizeCommitOptions(CommitOptions{Paths: []string{"src"}, Title: "feat: x"})
		if err == nil || !strings.Contains(err.Error(), "no ai_context field was received") {
			t.Fatalf("normalize error = %v, want absent-field condition", err)
		}
	})
	t.Run("empty array", func(t *testing.T) {
		_, err := normalizeCommitOptions(CommitOptions{Paths: []string{"src"}, Title: "feat: x", AIContext: []string{}})
		if err == nil || !strings.Contains(err.Error(), "received an empty array") {
			t.Fatalf("normalize error = %v, want empty-array condition", err)
		}
	})
	t.Run("all blank entries", func(t *testing.T) {
		_, err := normalizeCommitOptions(CommitOptions{Paths: []string{"src"}, Title: "feat: x", AIContext: []string{"   ", "\n"}})
		if err == nil || !strings.Contains(err.Error(), "received 2 entries, all blank") {
			t.Fatalf("normalize error = %v, want all-blank condition", err)
		}
	})
	// A present array whose sole entry is the exact empty string must report
	// the same "all blank" condition as a whitespace-only entry, not get
	// mistaken for a present-but-empty array — wsgit itself already gets
	// this right off the raw, untrimmed AIContext slice; this pins it so a
	// caller upstream (e.g. the MCP server layer) cannot silently regress it
	// by pre-filtering "" entries before wsgit ever sees them.
	t.Run("single empty-string entry classifies as all blank, not empty array", func(t *testing.T) {
		_, err := normalizeCommitOptions(CommitOptions{Paths: []string{"src"}, Title: "feat: x", AIContext: []string{""}})
		if err == nil || !strings.Contains(err.Error(), "received 1 entry, all blank") {
			t.Fatalf("normalize error = %v, want all-blank condition for a single empty-string entry", err)
		}
	})
	t.Run("path refusals name the commit root", func(t *testing.T) {
		for _, tc := range []struct {
			path string
			want []string
		}{
			{"../outside", []string{"must stay inside the commit root /work/tree-a"}},
			{"/work/tree-a/src/file.go", []string{"must be relative to the commit root /work/tree-a", `pass "src/file.go"`}},
			{"/elsewhere/file.go", []string{"must be relative to the commit root /work/tree-a"}},
		} {
			runner := &sequenceRunner{}
			_, err := (Client{Runner: runner}).Commit(context.Background(), "/work/tree-a", CommitOptions{Paths: []string{tc.path}, Title: "feat: x", AIContext: []string{"context"}, ExpectedBranch: "main"})
			if err == nil {
				t.Fatalf("Commit(%q) did not refuse", tc.path)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Commit(%q) error = %v, want it to contain %q", tc.path, err, want)
				}
			}
			if strings.HasPrefix(tc.path, "/elsewhere") && strings.Contains(err.Error(), "pass ") {
				t.Errorf("Commit(%q) error = %v suggests a relative spelling for a path outside the root", tc.path, err)
			}
			if len(runner.calls) != 0 {
				t.Errorf("Commit(%q) ran git %#v before refusing the path", tc.path, runner.calls)
			}
		}
	})
	t.Run("missing expected_branch", func(t *testing.T) {
		_, err := normalizeCommitOptions(CommitOptions{Paths: []string{"src"}, Title: "feat: x", AIContext: []string{"context"}})
		if err == nil || !strings.Contains(err.Error(), "expected_branch is required") {
			t.Fatalf("normalize error = %v, want expected_branch required condition", err)
		}
	})
}
