package wsgit

import (
	"context"
	"reflect"
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
	args, limit := LogArgs(LogOptions{Range: "main..HEAD", Limit: 500})
	if limit != 100 {
		t.Fatalf("limit = %d, want 100", limit)
	}
	if got, want := args[len(args)-1], "main..HEAD"; got != want {
		t.Fatalf("last arg = %q, want %q; args=%#v", got, want, args)
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
}

func (r *recordingRunner) RunGit(_ context.Context, root string, args ...string) ([]byte, error) {
	r.root = root
	r.args = append([]string(nil), args...)
	return r.out, nil
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
