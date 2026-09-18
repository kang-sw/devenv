package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsreview"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGitAllow runs git and returns its combined output without failing the test
// on a non-zero exit; used to reach expected-failure states like a conflicting
// cherry-pick that leaves unmerged index entries.
func runGitAllow(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, _ := cmd.CombinedOutput()
	return out
}

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

func requireMergeDiagnostic(t *testing.T, r implMergeResult, code, classification string) implMergeDiagnostic {
	t.Helper()
	for _, d := range r.Diagnostics {
		if d.Code == code {
			if d.Classification != classification || d.Reason == "" || d.Resolution == "" {
				t.Fatalf("incomplete diagnostic: %+v", d)
			}
			return d
		}
	}
	t.Fatalf("missing %s in %+v", code, r)
	return implMergeDiagnostic{}
}

func releaseAcknowledgement(r implMergeResult) implMergeAcknowledgement {
	return implMergeAcknowledgement{ReleaseTargetOverride: true, ExpectedSourceOID: r.SourceOID, ExpectedTargetOID: r.TargetOID}
}

func TestImplMergeReleaseAcknowledgement(t *testing.T) {
	for _, target := range []string{"main", "master"} {
		for _, topology := range []string{"trunk", "release-boundary"} {
			t.Run(target+"/"+topology, func(t *testing.T) {
				root, branch := mergeFixture(t, target)
				if topology == "release-boundary" {
					runGit(t, root, "branch", "develop", target)
				}
				r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{}, "")
				if err != nil || r.Status != "policy_blocked" || r.BranchDeleted {
					t.Fatalf("result=%+v err=%v", r, err)
				}
				d := requireMergeDiagnostic(t, r, "release_target", "overrideable")
				if len(r.Diagnostics) != 1 || !strings.Contains(d.Resolution, "release_target_override: true") || !strings.Contains(d.Resolution, "expected_target_oid") {
					t.Fatalf("bad retry: %+v", r)
				}
				if !validMergeOID(r.SourceOID) || !validMergeOID(r.TargetOID) || r.SourceOID == r.TargetOID {
					t.Fatalf("bad snapshot: %+v", r)
				}
				if r.Review == nil || r.Review.UncoveredCount == nil || *r.Review.UncoveredCount != 1 {
					t.Fatalf("missing review evidence: %+v", r.Review)
				}
				if current := strings.TrimSpace(string(runGitOutput(t, root, "symbolic-ref", "--short", "HEAD"))); current != branch {
					t.Fatalf("refusal switched to %s", current)
				}
				merged, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, target, mergeMessage(), releaseAcknowledgement(r), "")
				if err != nil || merged.Status != "merged" || !merged.BranchDeleted {
					t.Fatalf("result=%+v err=%v", merged, err)
				}
				parents := strings.Fields(string(runGitOutput(t, root, "rev-list", "--parents", "-n", "1", "HEAD")))
				if len(parents) != 3 || parents[1] != r.TargetOID || parents[2] != r.SourceOID {
					t.Fatalf("wrong merge parents: %v", parents)
				}
			})
		}
	}
}

func TestImplMergeReleaseCollectsSafetyFindings(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(fmt.Sprint(override), func(t *testing.T) {
			root, branch := mergeFixture(t, "main")
			snapshot, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{}, "")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "staged.txt"), []byte("keep"), 0644); err != nil {
				t.Fatal(err)
			}
			runGit(t, root, "add", "staged.txt")
			mergeHead := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "--git-path", "MERGE_HEAD")))
			if !filepath.IsAbs(mergeHead) {
				mergeHead = filepath.Join(root, mergeHead)
			}
			if err := os.WriteFile(mergeHead, []byte(snapshot.TargetOID+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
			ack := implMergeAcknowledgement{}
			if override {
				ack = releaseAcknowledgement(snapshot)
			}
			r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "wrong", mergeMessage(), ack, "")
			if err != nil || r.Status != "policy_blocked" {
				t.Fatalf("result=%+v err=%v", r, err)
			}
			for _, code := range []string{"target_mismatch", "dirty_worktree", "merge_in_progress"} {
				requireMergeDiagnostic(t, r, code, "must_resolve")
			}
			if !override {
				requireMergeDiagnostic(t, r, "release_target", "overrideable")
			}
			if head := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD"))); head != snapshot.SourceOID {
				t.Fatal("refusal moved HEAD")
			}
		})
	}
}

func TestImplMergeReleaseRejectsChangedTipsAndMissingOIDs(t *testing.T) {
	for _, change := range []string{"source", "target", "both", "missing", "abbreviated"} {
		t.Run(change, func(t *testing.T) {
			root, branch := mergeFixture(t, "master")
			snapshot, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{}, "")
			if err != nil {
				t.Fatal(err)
			}
			ack := releaseAcknowledgement(snapshot)
			if change == "source" || change == "both" {
				runGit(t, root, "commit", "--allow-empty", "-m", "new source")
			}
			if change == "target" || change == "both" {
				runGit(t, root, "switch", "master")
				runGit(t, root, "commit", "--allow-empty", "-m", "new target")
			}
			if change == "missing" {
				ack.ExpectedSourceOID = ""
			}
			if change == "abbreviated" {
				ack.ExpectedTargetOID = ack.ExpectedTargetOID[:7]
			}
			before := string(runGitOutput(t, root, "rev-parse", "HEAD"))
			r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), ack, "")
			if err != nil || r.Status != "policy_blocked" {
				t.Fatalf("result=%+v err=%v", r, err)
			}
			if change == "source" || change == "both" {
				requireMergeDiagnostic(t, r, "source_changed", "must_resolve")
			}
			if change == "target" || change == "both" {
				requireMergeDiagnostic(t, r, "target_changed", "must_resolve")
			}
			if change == "missing" || change == "abbreviated" {
				requireMergeDiagnostic(t, r, "expected_oids_required", "must_resolve")
			}
			if after := string(runGitOutput(t, root, "rev-parse", "HEAD")); before != after {
				t.Fatal("refusal moved HEAD")
			}
			if r.SourceOID != strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", branch))) || r.TargetOID != strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "master"))) {
				t.Fatal("refusal did not return fresh OIDs")
			}
		})
	}
}

type mergeInterceptRunner struct {
	intercept func(context.Context, string, []string) ([]byte, error, bool)
}

func (r mergeInterceptRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	if out, err, handled := r.intercept(ctx, root, args); handled {
		return out, err
	}
	return (wsgit.ExecRunner{}).RunGit(ctx, root, args...)
}

func TestImplMergeReleaseRawDiagnosticAndFinalRecheck(t *testing.T) {
	for _, fault := range []string{"count", "oid", "merge-state", "during-inspection", "checkout-target", "checkout-source"} {
		t.Run(fault, func(t *testing.T) {
			root, branch := mergeFixture(t, "main")
			snapshot, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{}, "")
			if err != nil {
				t.Fatal(err)
			}
			mutated, merged := false, false
			runner := mergeInterceptRunner{intercept: func(ctx context.Context, root string, args []string) ([]byte, error, bool) {
				if args[0] == "merge" {
					merged = true
				}
				if fault == "count" && args[0] == "rev-list" {
					return []byte(strings.Repeat("not-a-number", 400)), nil, true
				}
				if fault == "oid" && args[0] == "show-ref" && strings.Contains(args[len(args)-1], "refs/heads/impl/") {
					return []byte("ambiguous\nresult"), nil, true
				}
				if fault == "merge-state" && args[len(args)-1] == "MERGE_HEAD" {
					return []byte("fatal inspection"), fmt.Errorf("inspection failed"), true
				}
				if !mutated && fault == "during-inspection" && args[0] == "rev-list" {
					runGit(t, root, "commit", "--allow-empty", "-m", "new source")
					mutated = true
				}
				if strings.HasPrefix(fault, "checkout-") && args[0] == "switch" {
					out, err := (wsgit.ExecRunner{}).RunGit(ctx, root, args...)
					if err != nil {
						return out, err, true
					}
					if fault == "checkout-target" {
						runGit(t, root, "commit", "--allow-empty", "-m", "new target")
					} else {
						runGit(t, root, "update-ref", "refs/heads/"+branch, snapshot.TargetOID)
					}
					return out, nil, true
				}
				return nil, nil, false
			}}
			r, err := mergeImplBranch(context.Background(), root, runner, branch, "", mergeMessage(), releaseAcknowledgement(snapshot), "")
			if err != nil || r.Status != "policy_blocked" || merged {
				t.Fatalf("result=%+v err=%v merged=%t", r, err, merged)
			}
			code := map[string]string{"count": "containment_inspection", "oid": "ref_inspection", "merge-state": "merge_inspection", "during-inspection": "source_changed", "checkout-target": "target_changed", "checkout-source": "source_changed"}[fault]
			d := requireMergeDiagnostic(t, r, code, "must_resolve")
			if fault == "count" || fault == "oid" || fault == "merge-state" {
				if len(d.Command) == 0 || d.RawOutput == "" || len(d.RawOutput) > 2060 {
					t.Fatalf("missing bounded raw evidence: %+v", d)
				}
				if !strings.Contains(r.text(), "raw_output:") {
					t.Fatal("text lost raw evidence")
				}
			}
		})
	}
}

func TestImplMergeReleaseContainmentAndRefs(t *testing.T) {
	for _, fault := range []string{"contained", "missing-source", "invalid-source", "missing-target"} {
		t.Run(fault, func(t *testing.T) {
			root, branch := mergeFixture(t, "main")
			if fault == "contained" {
				runGit(t, root, "update-ref", "refs/heads/main", "HEAD")
			}
			if fault == "missing-source" {
				branch = "impl/main/absent"
			}
			if fault == "invalid-source" {
				branch = "impl/main/invalid..ref"
			}
			if fault == "missing-target" {
				runGit(t, root, "branch", "-D", "main")
			}
			r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{}, "")
			if err != nil || r.Status != "policy_blocked" {
				t.Fatalf("result=%+v err=%v", r, err)
			}
			requireMergeDiagnostic(t, r, "release_target", "overrideable")
			code := map[string]string{"contained": "already_contained", "missing-source": "ref_inspection", "invalid-source": "invalid_ref", "missing-target": "ref_inspection"}[fault]
			requireMergeDiagnostic(t, r, code, "must_resolve")
			if fault == "contained" {
				r, err = mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), releaseAcknowledgement(r), "")
				if err != nil || r.Status != "policy_blocked" {
					t.Fatalf("containment waived: %+v %v", r, err)
				}
				requireMergeDiagnostic(t, r, code, "must_resolve")
			}
		})
	}
}

func TestImplMergeReleaseReviewEvidence(t *testing.T) {
	root, branch := mergeFixture(t, "main")
	base := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "main")))
	covered := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", branch)))
	if err := wsreview.Append(root, wsreview.Entry{Base: base, Head: covered, Verdict: "pass"}); err != nil {
		t.Fatal(err)
	}
	if err := wsreview.Append(root, wsreview.Entry{Base: base, Head: base, Verdict: "block", Ref: "unresolved-ticket"}); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "ai-docs/.review-ledger.md")
	runGit(t, root, "commit", "-m", "record review frontier")
	r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{}, "")
	if err != nil || r.Status != "policy_blocked" || len(r.Diagnostics) != 1 {
		t.Fatalf("result=%+v err=%v", r, err)
	}
	if r.Review == nil || r.Review.Frontier == nil || r.Review.Frontier.Head != covered || r.Review.UncoveredCount == nil || *r.Review.UncoveredCount != 1 || !strings.Contains(r.Review.UncoveredRange, "^"+covered) {
		t.Fatalf("bad review evidence: %+v", r.Review)
	}
	if !strings.Contains(r.text(), "does not establish review quality or absence of unresolved findings") {
		t.Fatal("missing evidence limitation")
	}
}

func TestImplMergeOverrideScope(t *testing.T) {
	root, branch := mergeFixture(t, "develop")
	_, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{ReleaseTargetOverride: true}, "")
	if err == nil || !strings.Contains(err.Error(), "only to main or master") {
		t.Fatalf("override scope lost: %v", err)
	}
}

func TestImplMergeRequiresExactLocalRefsDespiteTagShadows(t *testing.T) {
	for _, target := range []string{"main", "develop"} {
		for _, missing := range []string{"source", "target", "source-at-checkout"} {
			t.Run(target+"/"+missing, func(t *testing.T) {
				root, branch := mergeFixture(t, target)
				sourceOID := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", branch)))
				targetOID := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", target)))
				ack := implMergeAcknowledgement{}
				if target == "main" {
					ack = implMergeAcknowledgement{ReleaseTargetOverride: true, ExpectedSourceOID: sourceOID, ExpectedTargetOID: targetOID}
				}
				if missing == "source" {
					branch = "impl/" + target + "/absent"
					runGit(t, root, "tag", "refs/heads/"+branch, sourceOID)
				} else if missing == "target" {
					runGit(t, root, "tag", "refs/heads/"+target, targetOID)
					runGit(t, root, "branch", "-D", target)
				}
				merged := false
				runner := mergeInterceptRunner{intercept: func(ctx context.Context, root string, args []string) ([]byte, error, bool) {
					if args[0] == "merge" {
						merged = true
					}
					if missing == "source-at-checkout" && args[0] == "switch" {
						out, err := (wsgit.ExecRunner{}).RunGit(ctx, root, args...)
						if err != nil {
							return out, err, true
						}
						runGit(t, root, "tag", "refs/heads/"+branch, sourceOID)
						runGit(t, root, "branch", "-D", branch)
						return out, nil, true
					}
					return nil, nil, false
				}}
				r, err := mergeImplBranch(context.Background(), root, runner, branch, "", mergeMessage(), ack, "")
				if merged || r.Status != "policy_blocked" || (target == "main" && err != nil) || (target != "main" && err == nil) {
					t.Fatalf("tag shadow accepted: %+v err=%v merged=%t", r, err, merged)
				}
				requireMergeDiagnostic(t, r, "ref_inspection", "must_resolve")
			})
		}
	}
}

func TestImplMergeReleaseMCPRoundTrip(t *testing.T) {
	for _, noAgent := range []string{"0", "1"} {
		t.Run(noAgent, func(t *testing.T) {
			t.Setenv("WS_MCP_NO_AGENT", noAgent)
			root, branch := mergeFixture(t, "main")
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			s := NewServer(root, "test")
			props := toolPropertiesByName(t, callToolsList(t, s), "git.merge")
			for _, field := range []string{"release_target_override", "expected_source_oid", "expected_target_oid"} {
				if props[field] == nil {
					t.Fatalf("missing %s", field)
				}
			}
			key, err := s.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
			if err != nil {
				t.Fatal(err)
			}
			args := map[string]any{"session_key": key, "branch": branch, "title": "merge(test): release", "ai_context": []string{"Test explicit acknowledgement."}, "format": "json"}
			resp := callToolOnce(t, s, 1, "git.merge", args)
			var r implMergeResult
			if err := json.Unmarshal([]byte(toolText(t, resp)), &r); err != nil {
				t.Fatalf("%v: %s", err, resp)
			}
			if r.Status != "policy_blocked" {
				t.Fatalf("not a structured refusal: %s", resp)
			}
			args["release_target_override"], args["expected_source_oid"], args["expected_target_oid"] = true, r.SourceOID, r.TargetOID
			args["format"] = "text"
			if out := toolText(t, callToolOnce(t, s, 2, "git.merge", args)); !strings.Contains(out, "status: merged") {
				t.Fatalf("retry failed: %s", out)
			}
		})
	}
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
			r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, input, "goal/topic", mergeMessage(), implMergeAcknowledgement{}, "")
			if err != nil || r.Status != "merged" || !r.BranchDeleted {
				t.Fatalf("result=%+v err=%v", r, err)
			}
			// A clean post-merge worktree must not raise the dirty_after_merge nudge.
			if len(r.Diagnostics) != 0 {
				t.Fatalf("clean merge raised diagnostics: %+v", r.Diagnostics)
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

func TestImplMergeCleanupFailureSurfacesDiagnostic(t *testing.T) {
	root, branch := mergeFixture(t, "develop")
	runner := mergeInterceptRunner{intercept: func(ctx context.Context, root string, args []string) ([]byte, error, bool) {
		if len(args) >= 2 && args[0] == "branch" && args[1] == "-d" {
			return []byte("simulated cleanup failure"), fmt.Errorf("branch -d refused"), true
		}
		return nil, nil, false
	}}
	r, err := mergeImplBranch(context.Background(), root, runner, branch, "develop", mergeMessage(), implMergeAcknowledgement{}, "")
	if err != nil {
		t.Fatalf("cleanup failure must stay non-fatal: err=%v result=%+v", err, r)
	}
	if r.Status != "merged" || r.BranchDeleted {
		t.Fatalf("want merged with BranchDeleted=false, got status=%q deleted=%t", r.Status, r.BranchDeleted)
	}
	if r.Advisory != "" {
		t.Fatalf("cleanup failure must surface as a diagnostic, not a quiet advisory string: %q", r.Advisory)
	}
	d := requireMergeDiagnostic(t, r, "cleanup_failed", "advisory")
	if !strings.Contains(d.Reason, branch) || !strings.Contains(strings.ToLower(d.Reason), "orphan") {
		t.Fatalf("diagnostic must name the orphan branch: %+v", d)
	}
	if !strings.Contains(r.text(), "cleanup_failed [advisory]") {
		t.Fatalf("diagnostic must render loudly in text(): %s", r.text())
	}
	// The orphan really remains: it is the leftover the create path recovers.
	if refs := strings.TrimSpace(string(runGitOutput(t, root, "branch", "--list", branch))); refs == "" {
		t.Fatalf("expected orphan branch %q to remain after failed cleanup", branch)
	}
}

func TestImplMergeRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, target, assertion, want string
		dirty                         bool
	}{
		{"mismatch", "develop", "elsewhere", "does not match", false},
		{"dirty", "develop", "", "clean index", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, branch := mergeFixture(t, tc.target)
			before := string(runGitOutput(t, root, "rev-parse", "HEAD"))
			if tc.dirty {
				if err := os.WriteFile(filepath.Join(root, "staged.txt"), []byte("keep"), 0644); err != nil {
					t.Fatal(err)
				}
				runGit(t, root, "add", "staged.txt")
			}
			_, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, tc.assertion, mergeMessage(), implMergeAcknowledgement{}, "")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v", err)
			}
			if after := string(runGitOutput(t, root, "rev-parse", "HEAD")); after != before {
				t.Fatal("refusal moved HEAD")
			}
		})
	}
}

func TestImplMergeTolerantWorktree(t *testing.T) {
	t.Run("untracked-proceeds-and-nudges", func(t *testing.T) {
		root, branch := mergeFixture(t, "goal/topic")
		if err := os.WriteFile(filepath.Join(root, "scratch.txt"), []byte("keep\n"), 0644); err != nil {
			t.Fatal(err)
		}
		r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "goal/topic", mergeMessage(), implMergeAcknowledgement{}, "")
		if err != nil || r.Status != "merged" || !r.BranchDeleted {
			t.Fatalf("result=%+v err=%v", r, err)
		}
		if _, statErr := os.Stat(filepath.Join(root, "scratch.txt")); statErr != nil {
			t.Fatalf("untracked file lost: %v", statErr)
		}
		d := requireMergeDiagnostic(t, r, "dirty_after_merge", "advisory")
		if !strings.Contains(d.Reason, "goal/topic") {
			t.Fatalf("nudge must name the target branch: %+v", d)
		}
		if !strings.Contains(r.text(), "dirty_after_merge [advisory]") {
			t.Fatalf("nudge must render loudly in text(): %s", r.text())
		}
	})

	t.Run("multiple-untracked-proceeds-and-nudges-plural", func(t *testing.T) {
		// Both other subtests here that reach dirty_after_merge leave exactly one
		// dirty file, so the n>1 "entries" plural branch of its advisory message
		// is otherwise untested. Leave two untracked files behind to exercise it.
		root, branch := mergeFixture(t, "goal/topic")
		if err := os.WriteFile(filepath.Join(root, "scratch1.txt"), []byte("keep\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "scratch2.txt"), []byte("keep\n"), 0644); err != nil {
			t.Fatal(err)
		}
		r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "goal/topic", mergeMessage(), implMergeAcknowledgement{}, "")
		if err != nil || r.Status != "merged" || !r.BranchDeleted {
			t.Fatalf("result=%+v err=%v", r, err)
		}
		d := requireMergeDiagnostic(t, r, "dirty_after_merge", "advisory")
		if !strings.Contains(d.Reason, "2 dirty working-tree entries remain") {
			t.Fatalf("plural rendering missing: %+v", d)
		}
	})

	t.Run("unstaged-proceeds-excluded-from-commit", func(t *testing.T) {
		root := initGitRepo(t)
		runGit(t, root, "switch", "-C", "goal/topic")
		if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("base\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "other.txt"), []byte("base\n"), 0644); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "add", ".")
		runGit(t, root, "commit", "-m", "base")
		branch := "impl/goal/topic/unit"
		runGit(t, root, "switch", "-c", branch)
		if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("impl\n"), 0644); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "commit", "-am", "impl change")
		// Non-overlapping unstaged modification to a file the merge does not touch.
		if err := os.WriteFile(filepath.Join(root, "other.txt"), []byte("working\n"), 0644); err != nil {
			t.Fatal(err)
		}
		r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "goal/topic", mergeMessage(), implMergeAcknowledgement{}, "")
		if err != nil || r.Status != "merged" {
			t.Fatalf("result=%+v err=%v", r, err)
		}
		// The merge commit must record other.txt unchanged (base), not the working copy.
		if committed := strings.TrimSpace(string(runGitOutput(t, root, "show", "HEAD:other.txt"))); committed != "base" {
			t.Fatalf("unstaged change folded into merge commit: %q", committed)
		}
		// The working modification is preserved on the target checkout.
		if wt, readErr := os.ReadFile(filepath.Join(root, "other.txt")); readErr != nil || strings.TrimSpace(string(wt)) != "working" {
			t.Fatalf("working change lost: %q err=%v", wt, readErr)
		}
		requireMergeDiagnostic(t, r, "dirty_after_merge", "advisory")
	})

	t.Run("staged-blocks", func(t *testing.T) {
		root, branch := mergeFixture(t, "develop")
		if err := os.WriteFile(filepath.Join(root, "staged.txt"), []byte("keep\n"), 0644); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "add", "staged.txt")
		r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "develop", mergeMessage(), implMergeAcknowledgement{}, "")
		if err == nil || r.Status != "policy_blocked" {
			t.Fatalf("staged change admitted: %+v err=%v", r, err)
		}
		d := requireMergeDiagnostic(t, r, "dirty_worktree", "must_resolve")
		if !strings.Contains(d.Reason, "clean index") {
			t.Fatalf("staged change wrong reason: %+v", d)
		}
	})

	t.Run("unmerged-without-merge-head-blocks", func(t *testing.T) {
		root, branch := mergeFixture(t, "develop")
		// Diverge develop so cherry-picking it onto impl conflicts, leaving unmerged
		// index entries under a CHERRY_PICK_HEAD (not a MERGE_HEAD).
		runGit(t, root, "switch", "develop")
		if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("develop\n"), 0644); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "commit", "-am", "develop diverge")
		dev := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
		runGit(t, root, "switch", branch)
		runGitAllow(t, root, "cherry-pick", dev) // conflicts, leaves unmerged entries
		if out := strings.TrimSpace(string(runGitOutput(t, root, "diff", "--name-only", "--diff-filter=U"))); out != "shared.txt" {
			t.Fatalf("expected unmerged shared.txt, got %q", out)
		}
		if _, statErr := os.Stat(filepath.Join(root, ".git", "MERGE_HEAD")); statErr == nil {
			t.Fatal("precondition: MERGE_HEAD must be absent so merge_in_progress does not mask the unmerged case")
		}
		r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "develop", mergeMessage(), implMergeAcknowledgement{}, "")
		if err == nil || r.Status != "policy_blocked" {
			t.Fatalf("unmerged paths admitted: %+v err=%v", r, err)
		}
		d := requireMergeDiagnostic(t, r, "dirty_worktree", "must_resolve")
		if !strings.Contains(d.Reason, "unmerged") {
			t.Fatalf("unmerged wrong reason: %+v", d)
		}
	})

	t.Run("both-added-unmerged-blocks", func(t *testing.T) {
		root, branch := mergeFixture(t, "develop")
		// Add a file on develop and a conflicting file at the same path on impl,
		// then cherry-pick to produce an add/add (AA) unmerged entry without a
		// MERGE_HEAD — exercising only the A&&A arm of indexBlocksMerge's unmerged
		// predicate. The sibling D&&D arm is exercised by
		// "both-deleted-unmerged-blocks" below.
		runGit(t, root, "switch", "develop")
		if err := os.WriteFile(filepath.Join(root, "added.txt"), []byte("develop\n"), 0644); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "add", "added.txt")
		runGit(t, root, "commit", "-m", "develop adds file")
		dev := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
		runGit(t, root, "switch", branch)
		if err := os.WriteFile(filepath.Join(root, "added.txt"), []byte("impl\n"), 0644); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "add", "added.txt")
		runGit(t, root, "commit", "-m", "impl adds file")
		runGitAllow(t, root, "cherry-pick", dev) // add/add conflict -> AA
		if porcelain := string(runGitOutput(t, root, "status", "--porcelain=v1")); !strings.Contains(porcelain, "AA added.txt") {
			t.Fatalf("expected AA unmerged entry, got: %q", porcelain)
		}
		r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "develop", mergeMessage(), implMergeAcknowledgement{}, "")
		if err == nil || r.Status != "policy_blocked" {
			t.Fatalf("both-added unmerged admitted: %+v err=%v", r, err)
		}
		d := requireMergeDiagnostic(t, r, "dirty_worktree", "must_resolve")
		if !strings.Contains(d.Reason, "unmerged") {
			t.Fatalf("both-added wrong reason: %+v", d)
		}
	})

	t.Run("both-deleted-unmerged-blocks", func(t *testing.T) {
		root, branch := mergeFixture(t, "develop")
		// A genuine DD (both-deleted) index entry cannot be reached through an
		// ordinary conflicting merge/cherry-pick: git resolves an agreeing delete
		// on both sides without conflict. Construct the real on-disk index state
		// directly the way git status itself defines it — a stage-1 (base) entry
		// with no stage-2/stage-3 entry — via update-index --index-info, then let
		// git status classify it; this exercises the D&&D arm of
		// indexBlocksMerge's unmerged predicate.
		if err := os.WriteFile(filepath.Join(root, "deleted.txt"), []byte("shared\n"), 0644); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "add", "deleted.txt")
		runGit(t, root, "commit", "-m", "impl adds deleted.txt")
		blob := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD:deleted.txt")))
		runGit(t, root, "update-index", "--force-remove", "--", "deleted.txt")
		if err := os.Remove(filepath.Join(root, "deleted.txt")); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "update-index", "--index-info")
		cmd.Dir = root
		cmd.Stdin = strings.NewReader(fmt.Sprintf("100644 %s 1\tdeleted.txt\n", blob))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("update-index --index-info: %v\n%s", err, out)
		}
		if porcelain := string(runGitOutput(t, root, "status", "--porcelain=v1")); !strings.Contains(porcelain, "DD deleted.txt") {
			t.Fatalf("expected DD unmerged entry, got: %q", porcelain)
		}
		r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "develop", mergeMessage(), implMergeAcknowledgement{}, "")
		if err == nil || r.Status != "policy_blocked" {
			t.Fatalf("both-deleted unmerged admitted: %+v err=%v", r, err)
		}
		d := requireMergeDiagnostic(t, r, "dirty_worktree", "must_resolve")
		if !strings.Contains(d.Reason, "unmerged") {
			t.Fatalf("both-deleted wrong reason: %+v", d)
		}
	})

	t.Run("overlapping-change-blocked-actionably", func(t *testing.T) {
		root, branch := mergeFixture(t, "develop")
		before := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
		// Unstaged change overlapping the merged path (shared.txt): now allowed past
		// the narrowed gate, but git's own switch refuses it. It must surface as an
		// actionable diagnostic, not a bare error string.
		if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("overlap-local\n"), 0644); err != nil {
			t.Fatal(err)
		}
		r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "develop", mergeMessage(), implMergeAcknowledgement{}, "")
		if err == nil || r.Status != "policy_blocked" {
			t.Fatalf("overlapping change admitted: %+v err=%v", r, err)
		}
		d := requireMergeDiagnostic(t, r, "switch_failed", "must_resolve")
		if len(d.Command) == 0 || d.RawOutput == "" || !strings.Contains(d.Resolution, "overlapping") {
			t.Fatalf("switch failure not actionable: %+v", d)
		}
		// The refusal did not switch away or move HEAD, and the local change survives.
		if current := strings.TrimSpace(string(runGitOutput(t, root, "symbolic-ref", "--short", "HEAD"))); current != branch {
			t.Fatalf("refusal switched to %s", current)
		}
		if after := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD"))); after != before {
			t.Fatal("refusal moved HEAD")
		}
		if wt, _ := os.ReadFile(filepath.Join(root, "shared.txt")); strings.TrimSpace(string(wt)) != "overlap-local" {
			t.Fatalf("overlapping change lost: %q", wt)
		}
	})
}

func TestImplMergeConflictAdvisory(t *testing.T) {
	for _, tc := range []struct{ target, source string }{
		{"develop", ""}, {"main", ""}, {"develop", "goal/develop/topic"},
		{"develop", "epic/topic"}, {"main", "feature/topic"},
	} {
		t.Run(tc.target+"/"+tc.source, func(t *testing.T) {
			target := tc.target
			root, branch := mergeFixture(t, target)
			if tc.source != "" {
				runGit(t, root, "branch", "-m", branch, tc.source)
				branch = tc.source
			}
			runGit(t, root, "switch", target)
			if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("target\n"), 0644); err != nil {
				t.Fatal(err)
			}
			runGit(t, root, "commit", "-am", "target change")
			ack := implMergeAcknowledgement{}
			if target == "main" {
				r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, target, mergeMessage(), ack, "")
				if err != nil || r.Status != "policy_blocked" {
					t.Fatalf("result=%+v err=%v", r, err)
				}
				ack = releaseAcknowledgement(r)
			}
			r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, target, mergeMessage(), ack, "")
			if err != nil || r.Status != "conflict" || r.BranchDeleted || !strings.Contains(r.Advisory, "lead-delegate") {
				t.Fatalf("result=%+v err=%v", r, err)
			}
			if out := strings.TrimSpace(string(runGitOutput(t, root, "diff", "--name-only", "--diff-filter=U"))); out != "shared.txt" {
				t.Fatalf("conflict state lost: %s", out)
			}
			runGit(t, root, "rev-parse", "--verify", "MERGE_HEAD")
			runGit(t, root, "show-ref", "--verify", "refs/heads/"+branch)
		})
	}
}

func TestImplMergeRejectsCheckoutShorthand(t *testing.T) {
	root, branch := mergeFixture(t, "main")
	mainBefore := string(runGitOutput(t, root, "rev-parse", "refs/heads/main"))
	runGit(t, root, "update-ref", "refs/heads/-", "refs/heads/main")
	runGit(t, root, "branch", "-m", branch, "impl/-/unit")
	_, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, "impl/-/unit", "", mergeMessage(), implMergeAcknowledgement{}, "")
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

func TestGenericMergePromotion(t *testing.T) {
	for _, branch := range []string{"goal/develop/topic", "epic/topic", "feature/topic", "develop"} {
		for _, explicit := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", branch, explicit), func(t *testing.T) {
				root, old := mergeFixture(t, "integration")
				runGit(t, root, "branch", "-m", old, branch)
				source := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
				target := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "integration")))
				input := ""
				if explicit {
					runGit(t, root, "switch", "integration")
					input = branch
				}
				r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, input, "integration", mergeMessage(), implMergeAcknowledgement{}, "")
				deleted := strings.HasPrefix(branch, "goal/")
				if err != nil || r.Status != "merged" || r.BranchDeleted != deleted || r.Branch != branch || r.Target != "integration" {
					t.Fatalf("result=%+v err=%v", r, err)
				}
				parents := strings.Fields(string(runGitOutput(t, root, "rev-list", "--parents", "-n", "1", "HEAD")))
				if len(parents) != 3 || parents[1] != target || parents[2] != source {
					t.Fatalf("wrong merge parents: %v", parents)
				}
				present := strings.TrimSpace(string(runGitOutput(t, root, "branch", "--list", branch))) != ""
				if present == deleted {
					t.Fatalf("wrong source lifecycle: present=%t deleted=%t", present, deleted)
				}
				body := string(runGitOutput(t, root, "log", "-1", "--format=%B"))
				if !strings.Contains(body, "## AI Context") || !strings.Contains(body, "## Ticket Updates") {
					t.Fatalf("merge record lost: %s", body)
				}
			})
		}
	}
}

func TestGenericMergeRefusals(t *testing.T) {
	for _, scenario := range []string{"missing-target", "dirty", "same-branch", "contained", "target-shorthand", "source-shorthand", "missing-source", "missing-local-target", "changed-at-checkout"} {
		t.Run(scenario, func(t *testing.T) {
			root, old := mergeFixture(t, "develop")
			branch, target := "epic/topic", "develop"
			runGit(t, root, "branch", "-m", old, branch)
			want := ""
			switch scenario {
			case "missing-target":
				target = ""
			case "dirty":
				if err := os.WriteFile(filepath.Join(root, "staged.txt"), []byte("keep"), 0644); err != nil {
					t.Fatal(err)
				}
				runGit(t, root, "add", "staged.txt")
				want = "dirty_worktree"
			case "same-branch":
				target, want = branch, "already_contained"
			case "contained":
				runGit(t, root, "update-ref", "refs/heads/develop", "HEAD")
				want = "already_contained"
			case "target-shorthand":
				target = "-"
			case "source-shorthand":
				branch, want = "@{-1}", "invalid_ref"
			case "missing-source":
				branch, want = "feature/absent", "ref_inspection"
				runGit(t, root, "tag", "refs/heads/"+branch, "HEAD")
			case "missing-local-target":
				target, want = "absent", "ref_inspection"
				runGit(t, root, "tag", "refs/heads/"+target, "develop")
			case "changed-at-checkout":
				want = "source_changed"
			}
			merged := false
			runner := mergeInterceptRunner{intercept: func(ctx context.Context, root string, args []string) ([]byte, error, bool) {
				if args[0] == "merge" {
					merged = true
				}
				if scenario == "changed-at-checkout" && args[0] == "switch" {
					runGit(t, root, "commit", "--allow-empty", "-m", "source moved")
				}
				return nil, nil, false
			}}
			r, err := mergeImplBranch(context.Background(), root, runner, branch, target, mergeMessage(), implMergeAcknowledgement{}, "")
			if err == nil || merged || r.BranchDeleted {
				t.Fatalf("unsafe merge: %+v err=%v merged=%t", r, err, merged)
			}
			if want != "" {
				requireMergeDiagnostic(t, r, want, "must_resolve")
			}
		})
	}
}

func TestGenericMergeReleaseMCP(t *testing.T) {
	for _, noAgent := range []string{"0", "1"} {
		for _, branch := range []string{"goal/main/topic", "epic/topic", "develop"} {
			t.Run(noAgent+"/"+branch, func(t *testing.T) {
				t.Setenv("WS_MCP_NO_AGENT", noAgent)
				root, old := mergeFixture(t, "main")
				runGit(t, root, "branch", "-m", old, branch)
				t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
				s := NewServer(root, "test")
				key, err := s.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
				if err != nil {
					t.Fatal(err)
				}
				args := map[string]any{"session_key": key, "branch": branch, "target": "main", "title": "merge(test): promote", "ai_context": []string{"Test generic release acknowledgement."}, "format": "json"}
				var r implMergeResult
				if err := json.Unmarshal([]byte(toolText(t, callToolOnce(t, s, 1, "git.merge", args))), &r); err != nil {
					t.Fatal(err)
				}
				if r.Status != "policy_blocked" {
					t.Fatalf("missing refusal: %+v", r)
				}
				requireMergeDiagnostic(t, r, "release_target", "overrideable")
				args["release_target_override"], args["expected_source_oid"], args["expected_target_oid"] = true, r.SourceOID, r.TargetOID
				if err := os.WriteFile(filepath.Join(root, "staged.txt"), []byte("keep"), 0644); err != nil {
					t.Fatal(err)
				}
				runGit(t, root, "add", "staged.txt")
				if err := json.Unmarshal([]byte(toolText(t, callToolOnce(t, s, 2, "git.merge", args))), &r); err != nil {
					t.Fatal(err)
				}
				requireMergeDiagnostic(t, r, "dirty_worktree", "must_resolve")
				runGit(t, root, "rm", "-f", "staged.txt")
				args["format"] = "text"
				out := toolText(t, callToolOnce(t, s, 3, "git.merge", args))
				if !strings.Contains(out, "status: merged") || !strings.Contains(out, fmt.Sprintf("branch_deleted: %t", strings.HasPrefix(branch, "goal/"))) {
					t.Fatalf("retry: %s", out)
				}
			})
		}
	}
}

// TestImplMergeRefusesTargetHeldElsewhere pins Decision 14: a target branch
// checked out in another worktree is a merge stop, diagnosed as
// target_held_elsewhere with the holder's kind (pool vs. plain vs. prunable)
// and path, with nothing switched or merged.
func TestImplMergeRefusesTargetHeldElsewhere(t *testing.T) {
	mkHeld := func(t *testing.T, root string) (poolRoot, held string) {
		t.Helper()
		poolRoot = filepath.Join(t.TempDir(), "pool")
		if err := os.MkdirAll(poolRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		held = filepath.Join(poolRoot, "held")
		runGit(t, root, "worktree", "add", held, "develop")
		// Canonicalize the pool through the checked-out worktree so it compares
		// equal to the git-canonical paths listWorktrees returns (macOS /var ->
		// /private/var), exactly as server.go derives it from a canonical mainRoot.
		return filepath.Dir(canonicalRootForTest(t, held)), held
	}
	assertUnmoved := func(t *testing.T, root, branch, developBefore, headBefore string) {
		t.Helper()
		if head := strings.TrimSpace(string(runGitOutput(t, root, "symbolic-ref", "--short", "HEAD"))); head != branch {
			t.Fatalf("HEAD moved to %q, want %q", head, branch)
		}
		if after := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "refs/heads/develop"))); after != developBefore {
			t.Fatal("develop tip moved on a refused merge")
		}
		if after := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD"))); after != headBefore {
			t.Fatal("HEAD commit moved on a refused merge")
		}
	}

	t.Run("pool-holder", func(t *testing.T) {
		root, branch := mergeFixture(t, "develop")
		poolRoot, held := mkHeld(t, root)
		// d.Reason embeds the path exactly as listWorktrees reports it (git's own
		// resolved form, e.g. macOS /var -> /private/var). held is the raw
		// filepath.Join value passed to `git worktree add`, which on Windows can
		// diverge from git's reported form in separator and/or short-path/symlink
		// resolution. Canonicalize held through the same `git rev-parse
		// --show-toplevel` path mkHeld already uses for poolRoot, so the
		// containment check compares two instances of git's own canonical form
		// instead of git's form against a filepath.Join-built one.
		heldCanonical := canonicalRootForTest(t, held)
		developBefore := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "refs/heads/develop")))
		headBefore := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
		r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{}, poolRoot)
		if err == nil || r.Status != "policy_blocked" {
			t.Fatalf("held target not refused: %+v err=%v", r, err)
		}
		if !strings.Contains(err.Error(), "checked out at") {
			t.Fatalf("error must name the held checkout: %v", err)
		}
		d := requireMergeDiagnostic(t, r, "target_held_elsewhere", "must_resolve")
		if !strings.Contains(d.Reason, "ws pool worktree") || !strings.Contains(d.Reason, heldCanonical) {
			t.Fatalf("pool-holder reason must name the pool kind and path: %+v", d)
		}
		assertUnmoved(t, root, branch, developBefore, headBefore)
	})

	t.Run("plain-holder", func(t *testing.T) {
		root, branch := mergeFixture(t, "develop")
		_, held := mkHeld(t, root)
		// See the pool-holder case above for why held must be canonicalized
		// through git's own resolution before the containment check.
		heldCanonical := canonicalRootForTest(t, held)
		developBefore := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "refs/heads/develop")))
		headBefore := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
		// poolRoot "" (unknown) -> the holder is named as a plain worktree.
		r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{}, "")
		if err == nil || r.Status != "policy_blocked" {
			t.Fatalf("held target not refused: %+v err=%v", r, err)
		}
		d := requireMergeDiagnostic(t, r, "target_held_elsewhere", "must_resolve")
		if strings.Contains(d.Reason, "ws pool worktree") {
			t.Fatalf("unknown-pool holder must not be named a ws pool worktree: %+v", d)
		}
		if !strings.Contains(d.Reason, "another worktree") || !strings.Contains(d.Reason, heldCanonical) {
			t.Fatalf("plain-holder reason must name another worktree and the path: %+v", d)
		}
		assertUnmoved(t, root, branch, developBefore, headBefore)
	})

	t.Run("no-other-worktree-merges", func(t *testing.T) {
		root, branch := mergeFixture(t, "develop")
		r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{}, filepath.Join(t.TempDir(), "pool"))
		if err != nil || r.Status != "merged" {
			t.Fatalf("inspection must be inert with no other worktree: %+v err=%v", r, err)
		}
	})

	t.Run("prunable-holder", func(t *testing.T) {
		root, branch := mergeFixture(t, "develop")
		poolRoot, held := mkHeld(t, root)
		// Remove the held worktree's directory WITHOUT pruning: the record still
		// holds develop (blocking the merge) but is prunable.
		if err := os.RemoveAll(held); err != nil {
			t.Fatal(err)
		}
		r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{}, poolRoot)
		if err == nil || r.Status != "policy_blocked" {
			t.Fatalf("prunable held target not refused: %+v err=%v", r, err)
		}
		d := requireMergeDiagnostic(t, r, "target_held_elsewhere", "must_resolve")
		if !strings.Contains(d.Reason, "stale worktree record") {
			t.Fatalf("prunable reason must name a stale worktree record: %+v", d)
		}
		if !strings.Contains(d.Resolution, "git worktree prune") {
			t.Fatalf("prunable resolution must direct to git worktree prune: %+v", d)
		}
		// After pruning, the branch is free and the merge lands.
		runGit(t, root, "worktree", "prune")
		r2, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{}, poolRoot)
		if err != nil || r2.Status != "merged" {
			t.Fatalf("merge must land after prune: %+v err=%v", r2, err)
		}
	})
}

// TestImplMergeDispatchClassifiesHeldTargetByPool drives git.merge through the
// real dispatch path (callToolOnce, the same harness the other dispatch tests
// use) rather than calling mergeImplBranch directly, so the pool-root
// resolution glue in server.go's `case "git.merge"` handler (session_key ->
// wsconfig ItemWorktreePool -> listWorktrees(root) -> resolvePoolRoot, fed
// into mergeImplBranch as its poolRoot argument) is itself exercised.
// TestImplMergeRefusesTargetHeldElsewhere above pins the pool-vs-plain
// classification logic in mergeImplBranch with a hand-supplied poolRoot; that
// leaves the dispatch-level resolution untested, so a regression there (e.g.
// the poolRoot argument silently dropped, or session_key/config wiring
// broken) would mislabel a pool holder as "another worktree" without any test
// catching it, even though the label routes lead-run's "Handle the report"
// response. See mkHeld's canonicalization comment above for why the pool
// path is derived through canonicalRootForTest.
func TestImplMergeDispatchClassifiesHeldTargetByPool(t *testing.T) {
	t.Run("pool-holder", func(t *testing.T) {
		root, branch := mergeFixture(t, "develop")
		root = canonicalRootForTest(t, root)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		s := NewServer(root, "test")
		key, err := s.sessions.mint(root, roleLead, "")
		if err != nil {
			t.Fatal(err)
		}
		// The default worktree_pool config resolves to
		// $(GitRoot)/../.ws-worktrees/$(GitRootDirName) — exactly what the
		// dispatch handler computes with no explicit config override, so no
		// config.tune is needed to make the holder fall inside the pool.
		poolRoot := resolvePoolRoot("", root)
		if err := os.MkdirAll(poolRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		held := filepath.Join(poolRoot, "held")
		runGit(t, root, "worktree", "add", held, "develop")
		// The dispatch response embeds the path as listWorktrees reports it
		// (git's own resolved form), which can diverge from this filepath.Join
		// value on Windows (separator and/or short-path/symlink resolution). See
		// TestImplMergeRefusesTargetHeldElsewhere/pool-holder for why held must
		// be canonicalized through git's own `rev-parse --show-toplevel` before
		// the containment check.
		heldCanonical := canonicalRootForTest(t, held)

		// mergeImplBranch returns a non-nil error for target_held_elsewhere, which
		// toolJSONResponse collapses to a plain error-text response regardless of
		// format:"json" (see toolJSONResponse's err != nil branch in server.go), so
		// this asserts on the rendered text rather than unmarshalling a result
		// struct.
		resp := callToolOnce(t, s, 1, "git.merge", map[string]any{
			"session_key": key, "branch": branch, "title": "merge(test): land implementation",
			"ai_context": []string{"Test pool-holder classification reaches dispatch."},
		})
		out := toolText(t, resp)
		if !strings.Contains(out, "checked out at") {
			t.Fatalf("held target not refused via dispatch: %s", out)
		}
		if !strings.Contains(out, "ws pool worktree") || !strings.Contains(out, heldCanonical) {
			t.Fatalf("dispatch must classify the pool holder and name its path: %s", out)
		}
	})

	t.Run("plain-holder", func(t *testing.T) {
		root, branch := mergeFixture(t, "develop")
		root = canonicalRootForTest(t, root)
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		s := NewServer(root, "test")
		key, err := s.sessions.mint(root, roleLead, "")
		if err != nil {
			t.Fatal(err)
		}
		// Held outside the resolved pool (an unrelated temp dir): the dispatch
		// handler's resolved poolRoot must not cover it, so the classification
		// falls back to a plain "another worktree".
		held := filepath.Join(t.TempDir(), "elsewhere")
		runGit(t, root, "worktree", "add", held, "develop")
		// See the pool-holder case above for why held must be canonicalized
		// through git's own resolution before the containment check.
		heldCanonical := canonicalRootForTest(t, held)

		resp := callToolOnce(t, s, 1, "git.merge", map[string]any{
			"session_key": key, "branch": branch, "title": "merge(test): land implementation",
			"ai_context": []string{"Test plain-holder classification reaches dispatch."},
		})
		out := toolText(t, resp)
		if !strings.Contains(out, "checked out at") {
			t.Fatalf("held target not refused via dispatch: %s", out)
		}
		if strings.Contains(out, "ws pool worktree") {
			t.Fatalf("holder outside the resolved pool must not be named a ws pool worktree: %s", out)
		}
		if !strings.Contains(out, "another worktree") || !strings.Contains(out, heldCanonical) {
			t.Fatalf("dispatch must classify the plain holder and name its path: %s", out)
		}
	})
}

// TestListWorktreesPrunable pins the prunable-record parse: a worktree whose
// directory was removed without prune reports Prunable, and a live one does not.
func TestListWorktreesPrunable(t *testing.T) {
	root, branch := mergeFixture(t, "develop")
	_ = branch
	held := filepath.Join(t.TempDir(), "held")
	runGit(t, root, "worktree", "add", held, "develop")
	entries, err := listWorktrees(context.Background(), wsgit.ExecRunner{}, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Prunable {
			t.Fatalf("live worktree wrongly marked prunable: %+v", e)
		}
	}
	if err := os.RemoveAll(held); err != nil {
		t.Fatal(err)
	}
	entries, err = listWorktrees(context.Background(), wsgit.ExecRunner{}, root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Branch == "refs/heads/develop" && e.Prunable {
			found = true
		}
	}
	if !found {
		t.Fatalf("removed worktree record not marked prunable: %+v", entries)
	}
}
