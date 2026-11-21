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

func TestDiffArgsDefaultsToStatMode(t *testing.T) {
	args, mode, err := DiffArgs(DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"diff", "--stat"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("DiffArgs = %#v, want %#v", args, want)
	}
	if mode != DiffModeStat {
		t.Fatalf("mode = %q, want %q", mode, DiffModeStat)
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

func TestClientDiffStatIncludesUntrackedFiles(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		[]byte(" tracked.go | 2 +-\n 1 file changed, 1 insertion(+), 1 deletion(-)\n"),
		[]byte("notes/new.md\x00"),
	}}
	result, err := (Client{Runner: runner}).Diff(context.Background(), "/repo", DiffOptions{Mode: DiffModeStat})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "tracked.go | 2 +-") || !strings.Contains(result.Output, "Untracked files:\n  notes/new.md\n") {
		t.Fatalf("diff output missing tracked or untracked content:\n%s", result.Output)
	}
	wantCalls := [][]string{
		{"diff", "--stat"},
		{"ls-files", "--others", "--exclude-standard", "-z"},
	}
	for i, want := range wantCalls {
		if !reflect.DeepEqual(runner.calls[i].args, want) {
			t.Fatalf("call %d args = %#v, want %#v", i, runner.calls[i].args, want)
		}
	}
}

func TestClientDiffNameOnlyIncludesPathFilteredUntrackedFiles(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		[]byte("src/tracked.go\n"),
		[]byte("src/new.go\x00"),
	}}
	result, err := (Client{Runner: runner}).Diff(context.Background(), "/repo", DiffOptions{Mode: DiffModeNameOnly, Paths: []string{"src"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "src/tracked.go\nsrc/new.go\n" {
		t.Fatalf("diff output = %q", result.Output)
	}
	want := []string{"ls-files", "--others", "--exclude-standard", "-z", "--", "src"}
	if !reflect.DeepEqual(runner.calls[1].args, want) {
		t.Fatalf("ls-files args = %#v, want %#v", runner.calls[1].args, want)
	}
}

func TestClientDiffNameOnlyIncludesSpecificFileInsideUntrackedDirectory(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		{},
		[]byte("ai-docs/.old/spec/260505/agent-system.md\x00"),
	}}
	path := "ai-docs/.old/spec/260505/agent-system.md"
	result, err := (Client{Runner: runner}).Diff(context.Background(), "/repo", DiffOptions{Mode: DiffModeNameOnly, Paths: []string{path}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != path+"\n" {
		t.Fatalf("diff output = %q", result.Output)
	}
	want := []string{"ls-files", "--others", "--exclude-standard", "-z", "--", path}
	if !reflect.DeepEqual(runner.calls[1].args, want) {
		t.Fatalf("ls-files args = %#v, want %#v", runner.calls[1].args, want)
	}
}

func TestClientDiffRangeDoesNotIncludeWorktreeUntrackedFiles(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{[]byte("src/tracked.go\n")}}
	result, err := (Client{Runner: runner}).Diff(context.Background(), "/repo", DiffOptions{Range: "HEAD~1..HEAD", Mode: DiffModeNameOnly})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "src/tracked.go\n" {
		t.Fatalf("diff output = %q", result.Output)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %#v, want only git diff for revision range", runner.calls)
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

func TestCurrentUserEmailTrimsOutput(t *testing.T) {
	runner := &recordingRunner{out: []byte("  Dev@Example.com \n")}
	got := CurrentUserEmail(context.Background(), runner, "/repo")
	if got != "Dev@Example.com" {
		t.Fatalf("CurrentUserEmail = %q, want trimmed original-case email", got)
	}
	want := []string{"config", "user.email"}
	if runner.root != "/repo" || !reflect.DeepEqual(runner.args, want) {
		t.Fatalf("root,args = %q,%#v; want /repo,%#v", runner.root, runner.args, want)
	}
}

func TestCurrentUserEmailFoldsErrorToEmpty(t *testing.T) {
	// user.email unset (or a bot with no identity) makes git exit non-zero;
	// the ownership gate must degrade to assign-any, so the error folds to "".
	runner := &recordingRunner{err: errors.New("exit status 1")}
	if got := CurrentUserEmail(context.Background(), runner, "/repo"); got != "" {
		t.Fatalf("CurrentUserEmail on git error = %q, want empty", got)
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
