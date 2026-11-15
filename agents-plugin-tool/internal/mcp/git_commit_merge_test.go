package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const mergeCommitValidTicket = "---\ntitle: Incoming ticket\nsage-review-design: completed\nsage-review-completeness: completed\n---\n\n# Incoming ticket\n\nA sufficiently long unchanged body for Git to detect the status move as a rename.\n"

type mcpCommitMergeFixture struct {
	root, ours, incoming string
}

func newMCPCommitMergeFixture(t *testing.T, baseExtra, oursExtra, incomingExtra func(string)) mcpCommitMergeFixture {
	t.Helper()
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	t.Setenv("WS_MCP_NO_DASHBOARD", "1")
	root := t.TempDir()
	initGit(t, root)
	runGit(t, root, "switch", "-c", "ours")
	runGit(t, root, "config", "commit.gpgsign", "false")
	mustWrite(t, root, "conflict.txt", "base\n")
	mustWrite(t, root, "incoming.txt", "base incoming\n")
	mustWrite(t, root, "unrelated.txt", "untouched\n")
	mustWrite(t, root, "composite.txt", "first\n"+strings.Repeat("middle\n", 20)+"last\n")
	if baseExtra != nil {
		baseExtra(root)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	runGit(t, root, "switch", "-c", "incoming")
	mustWrite(t, root, "conflict.txt", "theirs\n")
	mustWrite(t, root, "incoming.txt", "incoming version\n")
	if incomingExtra != nil {
		incomingExtra(root)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "incoming")
	incoming := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	runGit(t, root, "switch", "ours")
	mustWrite(t, root, "conflict.txt", "ours\n")
	if oursExtra != nil {
		oursExtra(root)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "ours")
	ours := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	cmd := exec.Command("git", "-C", root, "-c", "merge.directoryRenames=false", "merge", "--no-ff", "--no-commit", "incoming")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "CONFLICT") {
		t.Fatalf("expected conflicted merge: %v\n%s", err, out)
	}
	mustWrite(t, root, "conflict.txt", "resolved\n")
	return mcpCommitMergeFixture{root: root, ours: ours, incoming: incoming}
}

func (f mcpCommitMergeFixture) call(t *testing.T, paths []string, branch, format string) (string, bool) {
	t.Helper()
	server := NewServer(f.root, "test")
	t.Cleanup(server.Close)
	request := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "git.commit", "arguments": map[string]any{
			"paths": paths, "expected_branch": branch, "format": format,
			"title": "fix: conclude resolved merge", "ai_context": []string{"Retain merge content and structured context."},
		}},
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := serveStdioWithSession(t, server, f.root, string(raw)+"\n", &out); err != nil {
		t.Fatal(err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	return toolText(t, byID["1"]), toolIsError(t, byID["1"])
}

func (f mcpCommitMergeFixture) assertUncommitted(t *testing.T) {
	t.Helper()
	if got := strings.TrimSpace(string(runGitOutput(t, f.root, "rev-parse", "HEAD"))); got != f.ours {
		t.Fatalf("refusal moved HEAD: %s", got)
	}
	if got := strings.TrimSpace(string(runGitOutput(t, f.root, "rev-parse", "MERGE_HEAD"))); got != f.incoming {
		t.Fatalf("refusal lost merge state: %s", got)
	}
}

func (f mcpCommitMergeFixture) assertCompleted(t *testing.T) {
	t.Helper()
	parents := strings.Fields(string(runGitOutput(t, f.root, "show", "-s", "--format=%P", "HEAD")))
	if !reflect.DeepEqual(parents, []string{f.ours, f.incoming}) {
		t.Fatalf("parents = %q", parents)
	}
	body := string(runGitOutput(t, f.root, "show", "-s", "--format=%B", "HEAD"))
	if !strings.Contains(body, "## AI Context\n- Retain merge content and structured context.") {
		t.Fatalf("lost structured context: %s", body)
	}
	mergeFile := strings.TrimSpace(string(runGitOutput(t, f.root, "rev-parse", "--git-path", "MERGE_HEAD")))
	if !filepath.IsAbs(mergeFile) {
		mergeFile = filepath.Join(f.root, mergeFile)
	}
	if _, err := os.Stat(mergeFile); !os.IsNotExist(err) {
		t.Fatalf("merge state remains: %v", err)
	}
	if got := string(runGitOutput(t, f.root, "show", "HEAD:conflict.txt")); got != "resolved\n" {
		t.Fatalf("resolution = %q", got)
	}
	if got := string(runGitOutput(t, f.root, "show", "HEAD:incoming.txt")); got != "incoming version\n" {
		t.Fatalf("incoming tree = %q", got)
	}
}

func TestGitCommitMergeIncomingTicketVerification(t *testing.T) {
	const oldPath = "ai-docs/tickets/todo/261115-feat-incoming-move.md"
	const newPath = "ai-docs/tickets/ready/261115-feat-incoming-move.md"
	for _, kind := range []string{"addition", "rename", "deletion", "invalid addition", "invalid rename"} {
		t.Run(kind, func(t *testing.T) {
			f := newMCPCommitMergeFixture(t, func(root string) {
				if kind == "rename" || kind == "invalid rename" {
					mustWrite(t, root, oldPath, mergeCommitValidTicket)
				}
				// A removed invalid ticket must not be read by the verifier.
				if kind == "deletion" {
					mustWrite(t, root, "ai-docs/tickets/todo/not-a-valid-stem.md", "bad ticket\n")
				}
			}, nil, func(root string) {
				switch kind {
				case "addition":
					mustWrite(t, root, newPath, mergeCommitValidTicket)
				case "rename", "invalid rename":
					if err := os.MkdirAll(filepath.Join(root, "ai-docs/tickets/ready"), 0o755); err != nil {
						t.Fatal(err)
					}
					runGit(t, root, "mv", oldPath, newPath)
					if kind == "invalid rename" {
						mustWrite(t, root, newPath, strings.ReplaceAll(mergeCommitValidTicket, "sage-review-design: completed", "sage-review-design: pending"))
					}
				case "deletion":
					runGit(t, root, "rm", "ai-docs/tickets/todo/not-a-valid-stem.md")
				case "invalid addition":
					mustWrite(t, root, "ai-docs/tickets/todo/not-a-valid-stem.md", "---\ntitle: Invalid\n---\n\nBad.\n")
				}
			})
			runGit(t, f.root, "config", "commit.cleanup", "strip")
			mustWrite(t, f.root, "unrelated.txt", "dirty work remains\n")
			text, failed := f.call(t, []string{"conflict.txt"}, "ours", "text")
			if strings.HasPrefix(kind, "invalid") {
				if !failed || !strings.Contains(text, "ticket verify failed") {
					t.Fatalf("incoming invalid ticket bypassed verifier: %s", text)
				}
				want := "[stem]"
				if kind == "invalid rename" {
					want = "[ready-sage-posture]"
				}
				if !strings.Contains(text, want) {
					t.Fatalf("wrong verifier refusal, want %s: %s", want, text)
				}
				f.assertUncommitted(t)
				return
			}
			if failed {
				t.Fatalf("merge completion failed: %s", text)
			}
			f.assertCompleted(t)
			if kind == "rename" && !strings.Contains(text, newPath) {
				t.Fatalf("missing admitted ticket move: %s", text)
			}
			if got := string(runGitOutput(t, f.root, "status", "--porcelain")); got != " M unrelated.txt\n" {
				t.Fatalf("dirty work not retained: %q", got)
			}
		})
	}
}

func TestGitCommitMergeRefusalPreservesContent(t *testing.T) {
	for _, kind := range []string{"unrelated addition", "unrelated edit", "incoming bytes", "incoming mode", "unresolved", "branch mismatch", "missing merge state"} {
		t.Run(kind, func(t *testing.T) {
			f := newMCPCommitMergeFixture(t, nil, nil, nil)
			paths, branch, want := []string{"conflict.txt"}, "ours", "unrelated staged path"
			switch kind {
			case "unrelated addition":
				mustWrite(t, f.root, "unrelated-added.txt", "unapproved\n")
				runGit(t, f.root, "add", "unrelated-added.txt")
			case "unrelated edit":
				mustWrite(t, f.root, "unrelated.txt", "unapproved\n")
				runGit(t, f.root, "add", "unrelated.txt")
			case "incoming bytes":
				mustWrite(t, f.root, "incoming.txt", "contamination\n")
				runGit(t, f.root, "add", "incoming.txt")
			case "incoming mode":
				runGit(t, f.root, "update-index", "--chmod=+x", "incoming.txt")
			case "unresolved":
				paths, want = []string{"incoming.txt"}, "unmerged"
			case "branch mismatch":
				branch, want = "not-ours", "believed branch"
			case "missing merge state":
				path := strings.TrimSpace(string(runGitOutput(t, f.root, "rev-parse", "--git-path", "MERGE_HEAD")))
				if !filepath.IsAbs(path) {
					path = filepath.Join(f.root, path)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			// Stage the authorized resolution in advance except when testing
			// unresolved stages or the before-staging branch guard.
			if kind != "unresolved" && kind != "branch mismatch" {
				runGit(t, f.root, "add", "conflict.txt")
			}
			indexBefore := string(runGitOutput(t, f.root, "ls-files", "--stage", "-z"))
			text, failed := f.call(t, paths, branch, "text")
			if !failed || !strings.Contains(text, want) {
				t.Fatalf("want %s refusal, got %s", want, text)
			}
			if kind != "unresolved" && kind != "branch mismatch" && !strings.Contains(text, "existing paths grant") {
				t.Fatalf("missing explicit-grant recovery: %s", text)
			}
			if kind == "missing merge state" {
				if head := strings.TrimSpace(string(runGitOutput(t, f.root, "rev-parse", "HEAD"))); head != f.ours {
					t.Fatal("HEAD moved")
				}
			} else {
				f.assertUncommitted(t)
			}
			if after := string(runGitOutput(t, f.root, "ls-files", "--stage", "-z")); after != indexBefore {
				t.Fatal("refusal changed staged bytes/modes")
			}
		})
	}
}

func TestGitCommitMergeCompositeContentRequiresExplicitGrant(t *testing.T) {
	middle := strings.Repeat("middle\n", 20)
	f := newMCPCommitMergeFixture(t, nil, func(root string) {
		mustWrite(t, root, "composite.txt", "our first\n"+middle+"last\n")
	}, func(root string) {
		mustWrite(t, root, "composite.txt", "first\n"+middle+"incoming last\n")
	})
	// Git merged non-overlapping changes cleanly, but its composite blob is
	// not the incoming tree's blob and has no automatic admission authority.
	composite := "our first\n" + middle + "incoming last\n"
	if got := string(runGitOutput(t, f.root, "show", ":composite.txt")); got != composite {
		t.Fatalf("fixture is not composite: %q", got)
	}
	text, failed := f.call(t, []string{"conflict.txt"}, "ours", "text")
	if !failed || !strings.Contains(text, "composite.txt") || !strings.Contains(text, "existing paths grant") {
		t.Fatalf("composite content admitted: %s", text)
	}
	f.assertUncommitted(t)
	text, failed = f.call(t, []string{"conflict.txt", "composite.txt"}, "ours", "json")
	if failed {
		t.Fatalf("explicit grant recovery failed: %s", text)
	}
	var result struct {
		Hash  string   `json:"hash"`
		Paths []string `json:"paths"`
	}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatal(err)
	}
	if result.Hash == "" || !containsMCPCommitPath(result.Paths, "incoming.txt") {
		t.Fatalf("missing admitted path in JSON: %s", text)
	}
	f.assertCompleted(t)
	if got := string(runGitOutput(t, f.root, "show", "HEAD:composite.txt")); got != composite {
		t.Fatalf("composite content lost: %q", got)
	}
}

func containsMCPCommitPath(paths []string, path string) bool {
	for _, candidate := range paths {
		if candidate == path {
			return true
		}
	}
	return false
}
