package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// Include Git admin files: checking only HEAD and porcelain would miss an index
// refresh, reflog write, ref deletion, or damage to staged file contents.
func occupiedMergeSnapshot(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err == nil {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			files[rel] = data
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func requireOccupiedMergeUnchanged(t *testing.T, root string, before map[string][]byte) {
	t.Helper()
	after := occupiedMergeSnapshot(t, root)
	if reflect.DeepEqual(before, after) {
		return
	}
	for path, old := range before {
		if now, ok := after[path]; !ok || !bytes.Equal(old, now) {
			t.Errorf("refused merge changed or removed %s", path)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("refused merge created %s", path)
		}
	}
	t.FailNow()
}

func prepareOccupiedMerge(t *testing.T, target, worker string, dirty bool) (string, string) {
	t.Helper()
	root, source := mergeFixture(t, target)
	runGit(t, root, "switch", "-c", worker, target)
	if dirty {
		writeTestFile(t, root, "shared.txt", "worker unstaged\n")
		writeTestFile(t, root, "staged.txt", "worker staged\n")
		runGit(t, root, "add", "staged.txt")
		writeTestFile(t, root, "staged.txt", "worker unstaged on staged\n")
		writeTestFile(t, root, "scratch/untracked.txt", "worker scratch\n")
	}
	return root, source
}

func requireOccupiedMergeGuidance(t *testing.T, out, worker string) {
	t.Helper()
	for _, want := range []string{worker, "occupied", `profile: "housekeeping"`, "worktree.acquire", "returned session key", "worktree.release"} {
		if !strings.Contains(out, want) {
			t.Fatalf("refusal lacks %q: %s", want, out)
		}
	}
	if strings.Contains(out, "stale-worker-key") || strings.Contains(out, "stale-parent-key") {
		t.Fatalf("lease records cannot identify the branch occupant: %s", out)
	}
}

func TestImplMergeOccupiedRootPreservesBytes(t *testing.T) {
	for _, worker := range []string{"impl/develop/worker-b", "impl/rootless-worker"} {
		for _, dirty := range []bool{false, true} {
			for _, lease := range []string{"missing", "stale"} {
				t.Run(fmt.Sprintf("%s/dirty=%t/%s", worker, dirty, lease), func(t *testing.T) {
					root, source := prepareOccupiedMerge(t, "develop", worker, dirty)
					if lease == "stale" {
						if err := writeWorktreeLease(context.Background(), wsgit.ExecRunner{}, root, worktreeLease{
							WorkerKey: "stale-worker-key", ParentKey: "stale-parent-key", AcquiredAt: "2000-01-01T00:00:00Z",
						}); err != nil {
							t.Fatal(err)
						}
					}
					// Packed refs ensure the guard protects both ref representations.
					runGit(t, root, "pack-refs", "--all")
					before := occupiedMergeSnapshot(t, root)
					runner := mergeInterceptRunner{intercept: func(ctx context.Context, root string, args []string) ([]byte, error, bool) {
						if args[0] == "status" || args[0] == "switch" || args[0] == "branch" || isGitMergeArgs(args) {
							t.Fatalf("refused merge attempted status or mutation: %v", args)
						}
						return nil, nil, false
					}}
					r, err := mergeImplBranch(context.Background(), root, runner, source, "develop", mergeMessage(), implMergeAcknowledgement{}, nil, mergeSizeGuard{})
					if err == nil || r.Status != "policy_blocked" || r.BranchDeleted || r.Commit != "" {
						t.Fatalf("occupied checkout admitted: %+v err=%v", r, err)
					}
					requireMergeDiagnostic(t, r, "occupied_root", "must_resolve")
					requireOccupiedMergeGuidance(t, err.Error(), worker)
					requireOccupiedMergeUnchanged(t, root, before)
				})
			}
		}
	}
}

func TestImplMergeOccupiedRootRechecksBeforeSwitch(t *testing.T) {
	root, source := mergeFixture(t, "develop")
	worker := "impl/develop/intervening-worker"
	runGit(t, root, "branch", worker, "develop")
	targetReads, statuses := 0, 0
	var before map[string][]byte
	runner := mergeInterceptRunner{intercept: func(ctx context.Context, root string, args []string) ([]byte, error, bool) {
		if args[0] == "switch" || args[0] == "branch" || isGitMergeArgs(args) {
			t.Fatalf("merge attempted mutation after another worker arrived: %v", args)
		}
		if args[0] == "status" {
			statuses++
			if before != nil {
				t.Fatal("status ran after occupancy changed")
			}
		}
		if args[0] == "show-ref" && args[len(args)-1] == "refs/heads/develop" {
			targetReads++
			if targetReads == 2 {
				// Simulate an external checkout during the final tip inspection.
				runGit(t, root, "switch", worker)
				writeTestFile(t, root, "shared.txt", "intervening worker edits\n")
				before = occupiedMergeSnapshot(t, root)
			}
		}
		return nil, nil, false
	}}
	r, err := mergeImplBranch(context.Background(), root, runner, source, "develop", mergeMessage(), implMergeAcknowledgement{}, nil, mergeSizeGuard{})
	if err == nil || r.Status != "policy_blocked" || before == nil || statuses != 1 {
		t.Fatalf("missing final occupancy check: %+v err=%v statuses=%d", r, err, statuses)
	}
	requireMergeDiagnostic(t, r, "occupied_root", "must_resolve")
	requireOccupiedMergeGuidance(t, err.Error(), worker)
	requireOccupiedMergeUnchanged(t, root, before)
}

func TestImplMergeOccupiedRootInspectionFailsClosed(t *testing.T) {
	for _, failAt := range []int{1, 2} {
		t.Run(fmt.Sprint(failAt), func(t *testing.T) {
			root, source := mergeFixture(t, "develop")
			reads, statuses := 0, 0
			runner := mergeInterceptRunner{intercept: func(ctx context.Context, root string, args []string) ([]byte, error, bool) {
				if args[0] == "status" {
					statuses++
				}
				if args[0] == "switch" || args[0] == "branch" || isGitMergeArgs(args) {
					t.Fatalf("failed occupancy inspection authorized mutation: %v", args)
				}
				if reflect.DeepEqual(args, []string{"symbolic-ref", "--quiet", "HEAD"}) {
					reads++
					if reads == failAt {
						return nil, fmt.Errorf("synthetic HEAD inspection failure"), true
					}
				}
				return nil, nil, false
			}}
			r, err := mergeImplBranch(context.Background(), root, runner, source, "develop", mergeMessage(), implMergeAcknowledgement{}, nil, mergeSizeGuard{})
			if err == nil || r.Status != "policy_blocked" || statuses != failAt-1 {
				t.Fatalf("result=%+v err=%v statuses=%d", r, err, statuses)
			}
			d := requireMergeDiagnostic(t, r, "occupied_root_inspection", "must_resolve")
			if !strings.Contains(d.RawOutput, "synthetic HEAD inspection failure") {
				t.Fatalf("lost inspection evidence: %+v", d)
			}
		})
	}
}

func TestImplMergeOccupiedRootAllowedCheckouts(t *testing.T) {
	for _, checkout := range []string{"source", "target", "impl-target", "non-impl"} {
		t.Run(checkout, func(t *testing.T) {
			target := "develop"
			if checkout == "impl-target" {
				target = "impl/develop/target"
			}
			root, source := mergeFixture(t, target)
			input := source
			switch checkout {
			case "source":
				input = "" // Legacy inference from the current source remains valid.
			case "target", "impl-target":
				runGit(t, root, "switch", target)
			case "non-impl":
				runGit(t, root, "switch", "-c", "feature/unoccupied", target)
			}
			r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, input, target, mergeMessage(), implMergeAcknowledgement{}, nil, mergeSizeGuard{})
			if err != nil || r.Status != "merged" || !r.BranchDeleted {
				t.Fatalf("allowed checkout refused: %+v err=%v", r, err)
			}
			if got := strings.TrimSpace(string(runGitOutput(t, root, "symbolic-ref", "--short", "HEAD"))); got != target {
				t.Fatalf("HEAD=%q want %q", got, target)
			}
		})
	}
}

func TestImplMergeOccupiedRootMCP(t *testing.T) {
	for _, tc := range []struct {
		name, target, format, parent, noAgent string
		dirty, override                       bool
	}{
		{"lead-text-clean", "develop", "text", "", "0", false, false},
		{"worker-json-dirty", "develop", "json", "stale-parent-key", "1", true, false},
		{"release-policy", "main", "json", "", "1", false, false},
		{"release-acknowledged", "main", "json", "stale-parent-key", "0", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worker := "impl/" + tc.target + "/worker-b"
			root, source := prepareOccupiedMerge(t, tc.target, worker, tc.dirty)
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
			t.Setenv("WS_MCP_NO_AGENT", tc.noAgent)
			s := NewServer(root, "test")
			defer s.Close()
			key, err := s.sessions.mint(canonicalRootForTest(t, root), roleLead, tc.parent)
			if err != nil {
				t.Fatal(err)
			}
			args := map[string]any{"session_key": key, "branch": source, "target": tc.target,
				"title": "merge(test): protect worker checkout", "ai_context": []string{"Test occupied-root MCP refusal."}, "format": tc.format}
			if tc.override {
				args["release_target_override"] = true
				args["expected_source_oid"] = strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", source)))
				args["expected_target_oid"] = strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", tc.target)))
			}
			before := occupiedMergeSnapshot(t, root)
			out := toolText(t, callToolOnce(t, s, 1, "git.merge", args))
			if tc.target == "main" {
				var r implMergeResult
				if err := json.Unmarshal([]byte(out), &r); err != nil || r.Status != "policy_blocked" {
					t.Fatalf("missing structured release refusal: %s err=%v", out, err)
				}
				d := requireMergeDiagnostic(t, r, "occupied_root", "must_resolve")
				requireOccupiedMergeGuidance(t, d.Reason, worker)
				if !tc.override {
					requireMergeDiagnostic(t, r, "release_target", "overrideable")
				}
			} else {
				requireOccupiedMergeGuidance(t, out, worker)
			}
			requireOccupiedMergeUnchanged(t, root, before)
		})
	}
}
