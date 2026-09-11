package mcp

import (
	"context"
	"github.com/kang-sw/devenv/internal/wsgit"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mergeFixture(t *testing.T, target string) (string, string) {
	t.Helper()
	root := initGitRepo(t)
	runGit(t, root, "switch", "-C", target)
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("base\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	branch := "impl/" + target + "/test-merge-unit"
	runGit(t, root, "switch", "-c", branch)
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("impl\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-am", "impl change")
	return root, branch
}

func mergeMessage() wsgit.CommitOptions {
	return wsgit.CommitOptions{Title: "merge(test): land implementation", AIContext: []string{"Preserve the implementation boundary."}, UpdatedTickets: []string{"test-ticket"}}
}

func TestImplMergeNoFFAndCleanup(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "explicit"}[explicit], func(t *testing.T) {
			root, branch := mergeFixture(t, "goal/topic")
			source := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
			input := ""
			if explicit {
				runGit(t, root, "switch", "goal/topic")
				input = branch
			}
			runGit(t, root, "config", "merge.ff", "only")
			r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, input, "goal/topic", mergeMessage())
			if err != nil || r.Status != "merged" || !r.BranchDeleted {
				t.Fatalf("result=%+v err=%v", r, err)
			}
			parents := strings.Fields(string(runGitOutput(t, root, "rev-list", "--parents", "-n", "1", "HEAD")))
			if len(parents) != 3 || parents[2] != source {
				t.Fatalf("not a boundary merge: %v", parents)
			}
			if refs := string(runGitOutput(t, root, "branch", "--list", branch)); strings.TrimSpace(refs) != "" {
				t.Fatalf("branch retained: %s", refs)
			}
			body := string(runGitOutput(t, root, "log", "-1", "--format=%B"))
			if !strings.Contains(body, "## AI Context\n- Preserve") || !strings.Contains(body, "## Ticket Updates") {
				t.Fatalf("record lost: %s", body)
			}
		})
	}
}

func TestImplMergeRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, target, assertion, want string
		dirty                         bool
	}{
		{"mismatch", "develop", "elsewhere", "does not match", false},
		{"main", "main", "", "forbidden", false},
		{"master", "master", "", "forbidden", false},
		{"dirty", "develop", "", "clean worktree", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, branch := mergeFixture(t, tc.target)
			before := string(runGitOutput(t, root, "rev-parse", "HEAD"))
			if tc.dirty {
				if err := os.WriteFile(filepath.Join(root, "untracked"), []byte("keep"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			_, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, tc.assertion, mergeMessage())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v", err)
			}
			if after := string(runGitOutput(t, root, "rev-parse", "HEAD")); after != before {
				t.Fatal("refusal moved HEAD")
			}
		})
	}
}

func TestImplMergeConflictAdvisory(t *testing.T) {
	root, branch := mergeFixture(t, "develop")
	runGit(t, root, "switch", "develop")
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("target\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-am", "target change")
	r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage())
	if err != nil || r.Status != "conflict" || r.BranchDeleted || !strings.Contains(r.Advisory, "lead-delegate") {
		t.Fatalf("result=%+v err=%v", r, err)
	}
	if out := strings.TrimSpace(string(runGitOutput(t, root, "diff", "--name-only", "--diff-filter=U"))); out != "shared.txt" {
		t.Fatalf("conflict state lost: %s", out)
	}
	runGit(t, root, "rev-parse", "--verify", "MERGE_HEAD")
	runGit(t, root, "show-ref", "--verify", "refs/heads/"+branch)
}

func TestImplMergeRejectsCheckoutShorthand(t *testing.T) {
	root, branch := mergeFixture(t, "main")
	mainBefore := string(runGitOutput(t, root, "rev-parse", "refs/heads/main"))
	runGit(t, root, "update-ref", "refs/heads/-", "refs/heads/main")
	runGit(t, root, "branch", "-m", branch, "impl/-/unit")
	_, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, "impl/-/unit", "", mergeMessage())
	if err == nil || !strings.Contains(err.Error(), "invalid merge target") {
		t.Fatalf("shorthand target accepted: %v", err)
	}
	if after := string(runGitOutput(t, root, "rev-parse", "refs/heads/main")); after != mainBefore {
		t.Fatal("forbidden main branch changed")
	}
	if current := strings.TrimSpace(string(runGitOutput(t, root, "symbolic-ref", "--short", "HEAD"))); current != "impl/-/unit" {
		t.Fatalf("refusal changed checkout: %s", current)
	}
}

func TestImplMergeMCPAuthorityAndSchema(t *testing.T) {
	root, branch := mergeFixture(t, "develop")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := NewServer(root, "test")
	props := toolPropertiesByName(t, callToolsList(t, s), "git.merge")
	if props["session_key"] == nil || props["branch"] == nil {
		t.Fatalf("missing schema: %v", props)
	}
	for _, role := range []toolRole{roleDelegate, roleLead} {
		key, err := s.sessions.mint(canonicalRootForTest(t, root), role, "")
		if err != nil {
			t.Fatal(err)
		}
		resp := callToolOnce(t, s, 1, "git.merge", map[string]any{"session_key": key, "branch": branch, "title": "merge(test): land impl", "ai_context": []string{"Test lead ownership."}})
		if role == roleDelegate {
			if !strings.Contains(string(resp), "tool not available") {
				t.Fatalf("delegate not refused: %s", resp)
			}
		} else if out := toolText(t, resp); !strings.Contains(out, "status: merged") {
			t.Fatalf("lead failed: %s", out)
		}
	}
}
