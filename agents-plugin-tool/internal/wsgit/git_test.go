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

func TestCommitMessageRendersMentalModelNotesUnderAIContext(t *testing.T) {
	message := CommitMessage(CommitOptions{
		Title:               "docs(workflow): capture model note",
		AIContext:           []string{"User intent: record commit-message context."},
		MentalModelNotes:    []string{"git.commit now emits structured Mental Model Notes."},
		UpdatedTickets:      []string{"260519-bug-git-commit-mental-model-notes"},
		UpdatedSpecs:        []string{"260519-git-commit-mental-model-notes"},
		UpdatedMentalModels: []string{"git-workflow-tools"},
	})

	want := strings.Join([]string{
		"docs(workflow): capture model note",
		"",
		"## AI Context",
		"- User intent: record commit-message context.",
		"",
		"### Mental Model Notes",
		"- git.commit now emits structured Mental Model Notes.",
		"",
		"",
		"## Updated Tickets",
		"- 260519-bug-git-commit-mental-model-notes",
		"",
		"",
		"## Updated Specs",
		"- 260519-git-commit-mental-model-notes",
		"",
		"",
		"## Updated Mental Models",
		"- git-workflow-tools",
	}, "\n")
	if message != want {
		t.Fatalf("CommitMessage =\n%s\nwant:\n%s", message, want)
	}
}

func TestCommitMessageOmitsEmptyMentalModelNotes(t *testing.T) {
	opts, err := normalizeCommitOptions(CommitOptions{
		Paths:            []string{"src"},
		Title:            "docs(workflow): keep model notes optional",
		AIContext:        []string{"User intent: preserve existing commits."},
		MentalModelNotes: []string{"", " \t\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	message := CommitMessage(opts)
	if strings.Contains(message, "### Mental Model Notes") {
		t.Fatalf("message emitted empty Mental Model Notes subsection:\n%s", message)
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

func TestCommitExpandsTodoToReadyTicketMovePathsByStem(t *testing.T) {
	preStatus := ParseStatus([]byte("1 .D N... 100644 100644 100644 aaa bbb ai-docs/tickets/todo/260503-feat-demo.md\n? ai-docs/tickets/ready/260503-feat-demo.md\n"))
	paths := expandCommitPathsForTicketMoves(preStatus, []string{"ai-docs/tickets/ready/260503-feat-demo.md"})
	want := []string{"ai-docs/tickets/ready/260503-feat-demo.md", "ai-docs/tickets/todo/260503-feat-demo.md"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestCommitStagesDeletedTicketMoveByParentDirectory(t *testing.T) {
	preStatus := ParseStatus([]byte("1 .D N... 100644 100644 100644 aaa bbb ai-docs/tickets/todo/260503-feat-demo.md\n? ai-docs/tickets/.done/260503-feat-demo.md\n"))
	got := stagingCommandsForCommit([]string{"ai-docs/tickets/.done/260503-feat-demo.md", "ai-docs/tickets/todo/260503-feat-demo.md"}, preStatus)
	want := [][]string{
		{"add", "-A", "--", "ai-docs/tickets/.done/260503-feat-demo.md"},
		{"rm", "--cached", "--ignore-unmatch", "--", "ai-docs/tickets/todo/260503-feat-demo.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("staging commands = %#v, want %#v", got, want)
	}
}

func TestCommitStagesDeletedTodoToReadyTicketMove(t *testing.T) {
	preStatus := ParseStatus([]byte("1 .D N... 100644 100644 100644 aaa bbb ai-docs/tickets/todo/260503-feat-demo.md\n? ai-docs/tickets/ready/260503-feat-demo.md\n"))
	got := stagingCommandsForCommit([]string{"ai-docs/tickets/ready/260503-feat-demo.md", "ai-docs/tickets/todo/260503-feat-demo.md"}, preStatus)
	want := [][]string{
		{"add", "-A", "--", "ai-docs/tickets/ready/260503-feat-demo.md"},
		{"rm", "--cached", "--ignore-unmatch", "--", "ai-docs/tickets/todo/260503-feat-demo.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("staging commands = %#v, want %#v", got, want)
	}
}

func TestCommitStagesDeletedTicketMoveWhenOldStatusDirectoryIsGone(t *testing.T) {
	preStatus := ParseStatus([]byte("1 .D N... 100644 100644 100644 aaa bbb ai-docs/tickets/wip/260503-feat-demo.md\n? ai-docs/tickets/todo/260503-feat-demo.md\n"))
	got := stagingCommandsForCommit([]string{"ai-docs/tickets/todo/260503-feat-demo.md", "ai-docs/tickets/wip/260503-feat-demo.md"}, preStatus)
	want := [][]string{
		{"add", "-A", "--", "ai-docs/tickets/todo/260503-feat-demo.md"},
		{"rm", "--cached", "--ignore-unmatch", "--", "ai-docs/tickets/wip/260503-feat-demo.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("staging commands = %#v, want %#v", got, want)
	}
}

func TestCommitStagesRenamedDirectoryWithoutAddingMissingOldRoot(t *testing.T) {
	preStatus := ParseStatus([]byte("2 RM N... 100644 100644 100644 aaa bbb R100 agents-plugin/skills/lead-check-blockers/SKILL.md\tagents-plugin/skills/lead-can-we-proceed/SKILL.md\n"))
	got := stagingCommandsForCommit([]string{"agents-plugin/skills/lead-can-we-proceed", "agents-plugin/skills/lead-check-blockers"}, preStatus)
	want := [][]string{
		{"add", "-A", "--", "agents-plugin/skills/lead-check-blockers"},
		{"rm", "--cached", "--ignore-unmatch", "--", "agents-plugin/skills/lead-can-we-proceed/SKILL.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("staging commands = %#v, want %#v", got, want)
	}
}

func TestCommitStagesDeletedDirectoryRootByConcreteChildren(t *testing.T) {
	preStatus := ParseStatus([]byte("1 D. N... 100644 000000 000000 aaa 0000000000000000000000000000000000000000 old/file.txt\n"))
	got := stagingCommandsForCommit([]string{"old"}, preStatus)
	want := [][]string{
		{"rm", "--cached", "--ignore-unmatch", "--", "old/file.txt"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("staging commands = %#v, want %#v", got, want)
	}
}

func TestCommitStillAddsRootWithLiveChangesAndDeletedChildren(t *testing.T) {
	preStatus := ParseStatus([]byte("1 .D N... 100644 100644 100644 aaa bbb src/old.go\n1 .M N... 100644 100644 100644 aaa bbb src/live.go\n"))
	got := stagingCommandsForCommit([]string{"src"}, preStatus)
	want := [][]string{
		{"add", "-A", "--", "src"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("staging commands = %#v, want %#v", got, want)
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

func TestParseTicketNameStatusDetectsReadyMoves(t *testing.T) {
	changes := parseTicketNameStatus([]byte("R100\tai-docs/tickets/todo/260503-feat-demo.md\tai-docs/tickets/ready/260503-feat-demo.md\n"))
	if len(changes) != 1 {
		t.Fatalf("changes = %#v", changes)
	}
	change := changes[0]
	if change.Stem != "260503-feat-demo" || change.FromStatus != "todo" || change.ToStatus != "ready" || change.OldPath == "" {
		t.Fatalf("change = %#v", change)
	}
}

func TestParseTicketNameStatusReconstructsAddDeleteMove(t *testing.T) {
	changes := parseTicketNameStatus([]byte("D\tai-docs/tickets/todo/260503-feat-demo.md\nA\tai-docs/tickets/.done/260503-feat-demo.md\n"))
	if len(changes) != 1 {
		t.Fatalf("changes = %#v", changes)
	}
	change := changes[0]
	if change.Stem != "260503-feat-demo" || change.FromStatus != "todo" || change.ToStatus != ".done" {
		t.Fatalf("change statuses = %#v", change)
	}
	if change.Path != "ai-docs/tickets/.done/260503-feat-demo.md" || change.OldPath != "ai-docs/tickets/todo/260503-feat-demo.md" {
		t.Fatalf("change paths = %#v", change)
	}
}

func TestParseTicketNameStatusDoesNotReconstructAmbiguousAddDeleteMove(t *testing.T) {
	changes := parseTicketNameStatus([]byte(strings.Join([]string{
		"D\tai-docs/tickets/todo/260503-feat-demo.md",
		"D\tai-docs/tickets/ready/260503-feat-demo.md",
		"A\tai-docs/tickets/.done/260503-feat-demo.md",
		"",
	}, "\n")))
	if len(changes) != 3 {
		t.Fatalf("changes = %#v", changes)
	}
	for _, change := range changes {
		if change.FromStatus != "" || change.OldPath != "" {
			t.Fatalf("ambiguous change reconstructed a move: %#v", change)
		}
	}
}

func TestCommitMergesResultHeadingIntoReconstructedAddDeleteMove(t *testing.T) {
	runner := &sequenceRunner{outs: [][]byte{
		{},
		{},
		[]byte("1 A. N... 100644 100644 100644 aaa bbb ai-docs/tickets/.done/260503-feat-demo.md\n1 D. N... 100644 000000 000000 aaa 0000000000000000000000000000000000000000 ai-docs/tickets/todo/260503-feat-demo.md\n"),
		[]byte("D\tai-docs/tickets/todo/260503-feat-demo.md\nA\tai-docs/tickets/.done/260503-feat-demo.md\n"),
		[]byte("diff --git a/ai-docs/tickets/todo/260503-feat-demo.md b/ai-docs/tickets/.done/260503-feat-demo.md\n+++ b/ai-docs/tickets/.done/260503-feat-demo.md\n+### Result (abc123) - 2026-05-04\n"),
		{},
		[]byte("abc123\n"),
	}}
	result, err := (Client{Runner: runner}).Commit(context.Background(), "/repo", CommitOptions{
		Paths:     []string{"ai-docs/tickets"},
		Title:     "docs(ticket): close demo ticket",
		AIContext: []string{"User intent: close the workflow ticket."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TicketChanges) != 1 {
		t.Fatalf("ticket changes = %#v", result.TicketChanges)
	}
	change := result.TicketChanges[0]
	if change.FromStatus != "todo" || change.ToStatus != ".done" || !change.ResultAdded || change.ResultHeading != "### Result (abc123) - 2026-05-04" {
		t.Fatalf("ticket change = %#v", change)
	}
	message := runner.calls[5].args[2]
	if !strings.Contains(message, "260503-feat-demo: moved todo -> .done and added ### Result") {
		t.Fatalf("message missing reconstructed move summary:\n%s", message)
	}
}

func TestParseTicketResultAdditions(t *testing.T) {
	changes := parseTicketResultAdditions([]byte("diff --git a/ai-docs/tickets/todo/260503-feat-demo.md b/ai-docs/tickets/todo/260503-feat-demo.md\n+++ b/ai-docs/tickets/todo/260503-feat-demo.md\n+### Result (abc123) - 2026-05-04\n+body\n"))
	if len(changes) != 1 || changes[0].Stem != "260503-feat-demo" || changes[0].ResultHeading != "### Result (abc123) - 2026-05-04" {
		t.Fatalf("changes = %#v", changes)
	}
}

func TestParseTicketEditionAdditions(t *testing.T) {
	changes := parseTicketResultAdditions([]byte("diff --git a/ai-docs/tickets/ready/260503-feat-demo.md b/ai-docs/tickets/ready/260503-feat-demo.md\n+++ b/ai-docs/tickets/ready/260503-feat-demo.md\n+#### Edition (def456) - 2026-05-05\n+Follow-up tweak.\n"))
	if len(changes) != 1 || changes[0].Stem != "260503-feat-demo" || changes[0].ResultHeading != "#### Edition (def456) - 2026-05-05" {
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
