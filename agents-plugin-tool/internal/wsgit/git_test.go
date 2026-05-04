package wsgit

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDiffArgsAppendsPathFiltersAfterDoubleDash(t *testing.T) {
	args, mode, err := DiffArgs(DiffOptions{Range: "HEAD~1..HEAD", Mode: DiffModeNameOnly, Paths: []string{"a.txt", "dir/b.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"diff", "--name-only", "HEAD~1..HEAD", "--", "a.txt", "dir/b.txt"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("DiffArgs = %#v, want %#v", args, want)
	}
	if mode != DiffModeNameOnly {
		t.Fatalf("mode = %q", mode)
	}
}

func TestLogArgsBoundsLimitAndKeepsRange(t *testing.T) {
	args, limit, err := LogArgs(LogOptions{Range: "main..HEAD", Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	if limit != 100 {
		t.Fatalf("limit = %d, want 100", limit)
	}
	if got, want := args[len(args)-1], "main..HEAD"; got != want {
		t.Fatalf("last arg = %q, want %q; args=%#v", got, want, args)
	}
}

func TestRevisionFieldsRejectGitOptions(t *testing.T) {
	if _, _, err := DiffArgs(DiffOptions{Range: "--output=/tmp/ws-mcp-diff"}); err == nil {
		t.Fatal("DiffArgs accepted option-like range")
	}
	if _, _, err := LogArgs(LogOptions{Range: "--output=/tmp/ws-mcp-log"}); err == nil {
		t.Fatal("LogArgs accepted option-like range")
	}
	if _, err := (Client{}).MergeBase(context.Background(), "/repo", "--is-ancestor", "HEAD"); err == nil {
		t.Fatal("MergeBase accepted option-like base")
	}
}

func TestDiffArgsRejectsUnsupportedMode(t *testing.T) {
	if _, _, err := DiffArgs(DiffOptions{Mode: "patch-with-stat"}); err == nil {
		t.Fatal("DiffArgs accepted unsupported mode")
	}
}

func TestMergeBaseArgsUsesSeparateArgvEntries(t *testing.T) {
	want := []string{"merge-base", "main", "HEAD"}
	if got := MergeBaseArgs("main", "HEAD"); !reflect.DeepEqual(got, want) {
		t.Fatalf("MergeBaseArgs = %#v, want %#v", got, want)
	}
}

func TestParseStatusPorcelainV2(t *testing.T) {
	status := ParseStatus([]byte("# branch.oid abc123\n# branch.head main\n# branch.upstream origin/main\n# branch.ab +2 -1\n1 M. N... 100644 100644 100644 aaa bbb file.txt\n? new file.txt\n"))
	if status.Clean {
		t.Fatal("status unexpectedly clean")
	}
	if status.Branch.Head != "main" || status.Branch.OID != "abc123" || status.Branch.Upstream != "origin/main" || status.Branch.Ahead != 2 || status.Branch.Behind != 1 {
		t.Fatalf("branch parsed incorrectly: %#v", status.Branch)
	}
	want := []FileStatus{
		{Path: "file.txt", Status: "M.", IndexStatus: "M", WorktreeStatus: "."},
		{Path: "new file.txt", Status: "?"},
	}
	if !reflect.DeepEqual(status.ChangedFiles, want) {
		t.Fatalf("files = %#v, want %#v", status.ChangedFiles, want)
	}
}

func TestParseLogOmitsBodyUnlessRequested(t *testing.T) {
	raw := []byte("abc\x1fAda\x1f2026-05-03T00:00:00Z\x1fSubject\x1fBody text\x1e")
	withoutBody := ParseLog(raw, false)
	if len(withoutBody) != 1 || withoutBody[0].Body != "" {
		t.Fatalf("without body = %#v", withoutBody)
	}
	withBody := ParseLog(raw, true)
	if len(withBody) != 1 || withBody[0].Body != "Body text" {
		t.Fatalf("with body = %#v", withBody)
	}
}

type recordingRunner struct {
	root string
	args []string
	out  []byte
	err  error
}

func (r *recordingRunner) RunGit(_ context.Context, root string, args ...string) ([]byte, error) {
	r.root = root
	r.args = append([]string(nil), args...)
	return r.out, r.err
}

func TestClientStatusUsesPorcelainBranchArgs(t *testing.T) {
	runner := &recordingRunner{out: []byte("# branch.head main\n")}
	_, err := (Client{Runner: runner}).Status(context.Background(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"status", "--porcelain=v2", "--branch"}
	if runner.root != "/repo" || !reflect.DeepEqual(runner.args, want) {
		t.Fatalf("root,args = %q,%#v; want /repo,%#v", runner.root, runner.args, want)
	}
}

func TestClientPropagatesRunnerError(t *testing.T) {
	runnerErr := errors.New("not a git repository")
	runner := &recordingRunner{err: runnerErr}
	if _, err := (Client{Runner: runner}).Status(context.Background(), "/repo"); !errors.Is(err, runnerErr) {
		t.Fatalf("Status error = %v, want %v", err, runnerErr)
	}
}

func TestMergeBaseRequiresRevisions(t *testing.T) {
	if _, err := (Client{}).MergeBase(context.Background(), "/repo", "", "HEAD"); err == nil {
		t.Fatal("MergeBase accepted missing base")
	}
	if _, err := (Client{}).MergeBase(context.Background(), "/repo", "main", ""); err == nil {
		t.Fatal("MergeBase accepted missing head")
	}
}

func TestCommitStagesExplicitPathsAndBuildsMessage(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		{},
		{},
		[]byte("1 A. N... 100644 100644 100644 aaa bbb src/file.go\n"),
		[]byte("M\tai-docs/tickets/todo/260503-feat-demo.md\n"),
		[]byte("diff --git a/ai-docs/tickets/todo/260503-feat-demo.md b/ai-docs/tickets/todo/260503-feat-demo.md\n+++ b/ai-docs/tickets/todo/260503-feat-demo.md\n+### Result (abc123) - 2026-05-04\n"),
		{},
		[]byte("abc123\n"),
	}}
	result, err := (Client{Runner: runner}).Commit(context.Background(), "/repo", CommitOptions{
		Paths:       []string{"src"},
		Title:       "feat(ws-mcp): add commit tool",
		Description: "Builds a structured workflow commit.",
		AIContext:   []string{"User intent: make commit creation portable.", "Verification: unit test."},
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
	if !reflect.DeepEqual(runner.calls[1].args, wantFirst) {
		t.Fatalf("add args = %#v, want %#v", runner.calls[1].args, wantFirst)
	}
	commitArgs := runner.calls[5].args
	if len(commitArgs) != 3 || commitArgs[0] != "commit" || commitArgs[1] != "-m" {
		t.Fatalf("commit args = %#v", commitArgs)
	}
	message := commitArgs[2]
	for _, want := range []string{"feat(ws-mcp): add commit tool", "Builds a structured workflow commit.", "## AI Context", "- User intent: make commit creation portable."} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %q:\n%s", want, message)
		}
	}
	if !strings.Contains(message, "## Updated Tickets") || !strings.Contains(message, "260503-feat-demo: added ### Result") {
		t.Fatalf("message missing auto ticket summary:\n%s", message)
	}
}

func TestCommitRefusesUnrelatedStagedPaths(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		{},
		{},
		[]byte("1 M. N... 100644 100644 100644 aaa bbb src/file.go\n1 M. N... 100644 100644 100644 aaa bbb docs/note.md\n"),
	}}
	_, err := (Client{Runner: runner}).Commit(context.Background(), "/repo", CommitOptions{
		Paths:     []string{"src/file.go"},
		Title:     "feat: scoped",
		AIContext: []string{"User intent: scoped commit."},
	})
	if err == nil || !strings.Contains(err.Error(), "unrelated staged path") {
		t.Fatalf("Commit error = %v, want unrelated staged path", err)
	}
}

func TestCommitExpandsTicketMovePathsByStem(t *testing.T) {
	preStatus := ParseStatus([]byte("1 .D N... 100644 100644 100644 aaa bbb ai-docs/tickets/todo/260503-feat-demo.md\n? ai-docs/tickets/.done/260503-feat-demo.md\n"))
	paths := expandCommitPathsForTicketMoves(preStatus, []string{"ai-docs/tickets/.done/260503-feat-demo.md"})
	want := []string{"ai-docs/tickets/.done/260503-feat-demo.md", "ai-docs/tickets/todo/260503-feat-demo.md"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestCommitStagesDeletedTicketMoveByParentDirectory(t *testing.T) {
	preStatus := ParseStatus([]byte("1 .D N... 100644 100644 100644 aaa bbb ai-docs/tickets/todo/260503-feat-demo.md\n? ai-docs/tickets/.done/260503-feat-demo.md\n"))
	got := stagingPathsForCommit("/repo", []string{"ai-docs/tickets/.done/260503-feat-demo.md", "ai-docs/tickets/todo/260503-feat-demo.md"}, preStatus)
	want := []string{"ai-docs/tickets/.done/260503-feat-demo.md", "ai-docs/tickets/todo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("staging paths = %#v, want %#v", got, want)
	}
}

func TestCommitRequiresAIContextAndRelativePaths(t *testing.T) {
	_, err := normalizeCommitOptions(CommitOptions{Paths: []string{"src"}, Title: "feat: x"})
	if err == nil || !strings.Contains(err.Error(), "ai_context") {
		t.Fatalf("normalize error = %v, want ai_context", err)
	}
	_, err = normalizeCommitOptions(CommitOptions{Paths: []string{"../outside"}, Title: "feat: x", AIContext: []string{"context"}})
	if err == nil || !strings.Contains(err.Error(), "inside the repository") {
		t.Fatalf("normalize error = %v, want repository boundary", err)
	}
}

func TestParseTicketNameStatusDetectsMoves(t *testing.T) {
	changes := parseTicketNameStatus([]byte("R100\tai-docs/tickets/todo/260503-feat-demo.md\tai-docs/tickets/.done/260503-feat-demo.md\n"))
	if len(changes) != 1 {
		t.Fatalf("changes = %#v", changes)
	}
	change := changes[0]
	if change.Stem != "260503-feat-demo" || change.FromStatus != "todo" || change.ToStatus != ".done" || change.OldPath == "" {
		t.Fatalf("change = %#v", change)
	}
}

func TestParseTicketResultAdditions(t *testing.T) {
	changes := parseTicketResultAdditions([]byte("diff --git a/ai-docs/tickets/todo/260503-feat-demo.md b/ai-docs/tickets/todo/260503-feat-demo.md\n+++ b/ai-docs/tickets/todo/260503-feat-demo.md\n+### Result (abc123) - 2026-05-04\n+body\n"))
	if len(changes) != 1 || changes[0].Stem != "260503-feat-demo" || changes[0].ResultHeading != "### Result (abc123) - 2026-05-04" {
		t.Fatalf("changes = %#v", changes)
	}
}

type sequenceRunner struct {
	calls []gitCall
	outs  [][]byte
	errs  []error
}

type gitCall struct {
	root string
	args []string
}

func (r *sequenceRunner) RunGit(_ context.Context, root string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, gitCall{root: root, args: append([]string(nil), args...)})
	index := len(r.calls) - 1
	var out []byte
	if index < len(r.outs) {
		out = r.outs[index]
	}
	var err error
	if index < len(r.errs) {
		err = r.errs[index]
	}
	return out, err
}
