package mcp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/gitbatch"
	"github.com/kang-sw/devenv/internal/wsgit"
)

const occupiedMoveStem = "260101-feat-occupied-move"

func occupiedMoveFixture(t *testing.T, from string, tracked bool) (string, *Server, string) {
	t.Helper()
	useLeadProfile(t)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_MCP_NO_DASHBOARD", "1")
	root := t.TempDir()
	initGit(t, root)
	runGit(t, root, "checkout", "-b", "develop")
	mustWrite(t, root, "baseline.txt", "baseline\n")
	path := "ai-docs/tickets/" + from + "/" + occupiedMoveStem + ".md"
	if tracked {
		mustWrite(t, root, path, ixTicket("Move"))
	}
	runGit(t, root, "add", "-A")
	runGit(t, root, "-c", "commit.gpgsign=false", "commit", "-m", "board")
	if !tracked {
		mustWrite(t, root, path, ixTicket("Move"))
	}
	s := NewServer(root, "test")
	t.Cleanup(func() { s.Close() })
	// Bind while develop is current, before the simulated external switch.
	key, err := s.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	return root, s, key
}

func occupiedMoveCall(t *testing.T, s *Server, key, to string) string {
	t.Helper()
	return callToolOnce(t, s, 1, "tickets.move", map[string]any{
		"session_key": key, "stem": occupiedMoveStem, "to": to,
	})
}

func requireOccupiedMoveAbsent(t *testing.T, root, to string) {
	t.Helper()
	// The destination directory, not just its ticket, must remain absent.
	if _, err := os.Stat(filepath.Join(root, "ai-docs", "tickets", to)); !os.IsNotExist(err) {
		t.Fatalf("destination directory exists or stat failed: %v", err)
	}
}

func TestTicketsMoveOccupiedRootPreservesBytes(t *testing.T) {
	for _, branch := range []string{"impl/develop/worker", "impl/worker"} {
		for _, tracked := range []bool{false, true} {
			for _, from := range []string{"todo", "ready"} {
				for _, lease := range []string{"missing", "stale"} {
					t.Run(fmt.Sprintf("%s/tracked=%t/%s/%s", branch, tracked, from, lease), func(t *testing.T) {
						root, s, key := occupiedMoveFixture(t, from, tracked)
						to := "ready"
						if from == "ready" {
							to = "todo"
						}
						runGit(t, root, "switch", "-c", branch)
						mustWrite(t, root, "baseline.txt", "unstaged baseline\n")
						mustWrite(t, root, "staged.txt", "staged worker data\n")
						runGit(t, root, "add", "staged.txt")
						mustWrite(t, root, "staged.txt", "unstaged over staged worker data\n")
						mustWrite(t, root, "scratch/untracked.txt", "untracked worker data\n")
						if lease == "stale" {
							if err := writeWorktreeLease(context.Background(), wsgit.ExecRunner{}, root, worktreeLease{
								WorkerKey: "stale-worker-key", ParentKey: "stale-parent-key", AcquiredAt: "2000-01-01T00:00:00Z",
							}); err != nil {
								t.Fatal(err)
							}
						}
						before := occupiedMergeSnapshot(t, root)
						resp := occupiedMoveCall(t, s, key, to)
						// Compare every file and Git admin byte before any status read
						// can refresh the index and conceal a handler-side refresh.
						requireOccupiedMergeUnchanged(t, root, before)
						requireOccupiedMoveAbsent(t, root, to)
						if !isToolError(resp) {
							t.Fatalf("occupied move succeeded: %s", resp)
						}
						requireOccupiedMergeGuidance(t, toolText(t, resp), branch)
					})
				}
			}
		}
	}
}

func TestTicketsMoveOccupiedRootAllowedSessions(t *testing.T) {
	for _, tc := range []struct {
		name, branch, parent string
		role                 toolRole
	}{
		{"develop", "develop", "", roleLead},
		{"non-impl", "feature/board", "", roleLead},
		{"detached", "", "", roleLead},
		{"parented-lead", "impl/develop/worker", "live", roleLead},
		{"parented-delegate", "impl/worker", "live", roleDelegate},
		{"agent-delegate", "impl/develop/worker", "", roleDelegate},
		{"rootless-agent-delegate", "impl/worker", "", roleDelegate},
		{"missing-parent-lead", "impl/worker", "missing-parent-record", roleLead},
		{"missing-parent-delegate", "impl/develop/worker", "missing-parent-record", roleDelegate},
	} {
		for _, from := range []string{"todo", "ready"} {
			t.Run(tc.name+"/"+from, func(t *testing.T) {
				root, s, key := occupiedMoveFixture(t, from, true)
				if tc.parent != "" || tc.role == roleDelegate {
					parent := tc.parent
					if parent == "live" {
						parent = key
					}
					var err error
					key, err = s.sessions.mint(canonicalRootForTest(t, root), tc.role, parent)
					if err != nil {
						t.Fatal(err)
					}
				}
				switch tc.branch {
				case "":
					runGit(t, root, "checkout", "--detach")
				case "develop":
				default:
					runGit(t, root, "switch", "-c", tc.branch)
				}
				to := "ready"
				if from == "ready" {
					to = "todo"
				}
				head := append([]byte(nil), runGitOutput(t, root, "rev-parse", "HEAD")...)
				resp := occupiedMoveCall(t, s, key, to)
				if isToolError(resp) {
					t.Fatalf("allowed move refused: %s", resp)
				}
				oldPath := "ai-docs/tickets/" + from + "/" + occupiedMoveStem + ".md"
				newPath := "ai-docs/tickets/" + to + "/" + occupiedMoveStem + ".md"
				if _, err := os.Stat(filepath.Join(root, oldPath)); !os.IsNotExist(err) {
					t.Fatalf("old ticket remains: %v", err)
				}
				body, err := os.ReadFile(filepath.Join(root, newPath))
				if err != nil {
					t.Fatal(err)
				}
				if to == "ready" {
					for _, want := range []string{"sage-review-design: skipped", "sage-review-completeness: required"} {
						if !bytes.Contains(body, []byte(want)) {
							t.Fatalf("promotion did not normalize %q: %s", want, body)
						}
					}
				} else if string(body) != ixTicket("Move") {
					t.Fatalf("downward move rewrote frontmatter: %s", body)
				}
				if staged := string(runGitOutput(t, root, "show", ":"+newPath)); staged != string(body) {
					t.Fatalf("destination not staged byte-for-byte: %s", staged)
				}
				changes := string(runGitOutput(t, root, "diff", "--cached", "--name-status", "--no-renames"))
				if changes != "D\t"+oldPath+"\nA\t"+newPath+"\n" && changes != "A\t"+newPath+"\nD\t"+oldPath+"\n" {
					t.Fatalf("not a staged rename: %s", changes)
				}
				if !bytes.Equal(head, runGitOutput(t, root, "rev-parse", "HEAD")) {
					t.Fatal("move changed HEAD")
				}
			})
		}
	}
}

// Agent workers carry parentless delegate keys. Even a server rooted elsewhere
// must move/close only in the key's checkout, never the other worker's index.
func TestTicketMutationsAgentWorkerBoundRoot(t *testing.T) {
	for _, tool := range []string{"tickets.move", "tickets.close"} {
		t.Run(tool, func(t *testing.T) {
			root, _, _ := occupiedMoveFixture(t, "todo", true)
			runGit(t, root, "switch", "-c", "impl/develop/worker")
			key, err := NewAgentKeyStore().Mint(canonicalRootForTest(t, root))
			if err != nil {
				t.Fatal(err)
			}
			foreign := implRootRepo(t, "impl/develop/other")
			oldPath := "ai-docs/tickets/todo/" + occupiedMoveStem + ".md"
			mustWrite(t, foreign, oldPath, ixTicket("Foreign"))
			runGit(t, foreign, "add", oldPath)
			s := NewServer(foreign, "test")
			defer s.Close()
			before := occupiedMergeSnapshot(t, foreign)
			args := map[string]any{"session_key": key, "stem": occupiedMoveStem}
			to := "ready"
			if tool == "tickets.move" {
				args["to"] = to
			} else {
				to = ".done"
				args["status"] = "done"
			}
			resp := callToolOnce(t, s, 1, tool, args)
			requireOccupiedMergeUnchanged(t, foreign, before)
			if isToolError(resp) {
				t.Fatalf("agent worker refused: %s", resp)
			}
			newPath := "ai-docs/tickets/" + to + "/" + occupiedMoveStem + ".md"
			body, err := os.ReadFile(filepath.Join(root, newPath))
			if err != nil {
				t.Fatal(err)
			}
			if staged := runGitOutput(t, root, "show", ":"+newPath); !bytes.Equal(staged, body) {
				t.Fatalf("destination not staged byte-for-byte: %s", staged)
			}
			changes := string(runGitOutput(t, root, "diff", "--cached", "--name-only", "--no-renames"))
			if changes != oldPath+"\n"+newPath+"\n" && changes != newPath+"\n"+oldPath+"\n" {
				t.Fatalf("staged paths outside the ticket move: %s", changes)
			}
		})
	}
}

func TestTicketsMoveOccupiedRootPrecedesConfigAndOwnershipRefresh(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(fmt.Sprintf("malformed-config=%t", malformed), func(t *testing.T) {
			e := newIxEnv(t)
			c := e.clone("lead", "lead@example.com")
			defer c.server.Close()
			c.init()
			e.clock.Advance(time.Hour) // Make the ownership view require a refresh.
			c.runner.reset()
			if malformed {
				mustWrite(t, c.root, ".ws-workflow/config.json", "{broken repo sage config\n")
				if _, err := c.server.resolveSageReviewConfig(c.key); err == nil {
					t.Fatal("fixture did not make sage config resolution fail")
				}
			}
			branch := "impl/develop/worker"
			runGit(t, c.root, "switch", "-c", branch)
			before := occupiedMergeSnapshot(t, c.root)
			originBefore := occupiedMergeSnapshot(t, e.origin)
			out, isErr := c.call("tickets.move", map[string]any{"stem": stemAlpha, "to": "todo"})
			requireOccupiedMergeUnchanged(t, c.root, before)
			requireOccupiedMergeUnchanged(t, e.origin, originBefore)
			if !isErr {
				t.Fatalf("occupied move succeeded: %s", out)
			}
			requireOccupiedMergeGuidance(t, out, branch)
			if c.runner.total() != 0 {
				t.Fatalf("occupancy refusal refreshed ownership: %v", c.runner.counts)
			}
		})
	}
}

func TestTicketsMoveOccupiedRootInspectionFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("local POSIX Git shim")
	}
	for _, failAt := range []int{1, 2} {
		t.Run(fmt.Sprintf("inspection=%d", failAt), func(t *testing.T) {
			testOccupiedMoveInspectionFailure(t, failAt)
		})
	}
}

func testOccupiedMoveInspectionFailure(t *testing.T, failAt int) {
	t.Helper()
	root, _, _ := occupiedMoveFixture(t, "todo", true)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	shim := t.TempDir()
	counter := filepath.Join(shim, "head-reads")
	quotedCounter := gitbatch.ShellQuote(filepath.ToSlash(counter))
	// Fail either the initial observation or the writer-boundary recheck.
	mustWrite(t, shim, "git", fmt.Sprintf("#!/bin/sh\nif [ \"$3\" = symbolic-ref ] && [ \"$4\" = --quiet ] && [ \"$5\" = HEAD ]; then\n count=0\n if [ -f %s ]; then read count < %s; fi\n count=$((count + 1))\n printf '%%s\\n' \"$count\" > %s\n if [ \"$count\" = %d ]; then\n  echo 'synthetic HEAD inspection failure' >&2\n  exit 2\n fi\nfi\nexec %s \"$@\"\n", quotedCounter, quotedCounter, quotedCounter, failAt, gitbatch.ShellQuote(filepath.ToSlash(realGit))))
	if err := os.Chmod(filepath.Join(shim, "git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A hosted environment confines the shim to this server's subprocesses.
	env := append([]string(nil), os.Environ()...)
	for i, entry := range env {
		if strings.HasPrefix(entry, "PATH=") {
			env[i] = "PATH=" + shim + string(os.PathListSeparator) + os.Getenv("PATH")
		}
	}
	s := NewServerWithOptions(ServerOptions{Root: root, Version: "test", Env: env})
	defer s.Close()
	key, err := s.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	before := occupiedMergeSnapshot(t, root)
	resp := occupiedMoveCall(t, s, key, "ready")
	requireOccupiedMergeUnchanged(t, root, before)
	requireOccupiedMoveAbsent(t, root, "ready")
	if !isToolError(resp) || !strings.Contains(toolText(t, resp), "synthetic HEAD inspection failure") {
		t.Fatalf("missing HEAD inspection refusal: %s", resp)
	}
	reads, err := os.ReadFile(counter)
	if err != nil || strings.TrimSpace(string(reads)) != fmt.Sprint(failAt) {
		t.Fatalf("HEAD inspections=%q err=%v, want %d", reads, err, failAt)
	}
}
