package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsreview"
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
				r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{})
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
				merged, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, target, mergeMessage(), releaseAcknowledgement(r))
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
			snapshot, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "untracked"), []byte("keep"), 0644); err != nil {
				t.Fatal(err)
			}
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
			r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "wrong", mergeMessage(), ack)
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
			snapshot, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{})
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
			r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), ack)
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
			snapshot, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{})
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
			r, err := mergeImplBranch(context.Background(), root, runner, branch, "", mergeMessage(), releaseAcknowledgement(snapshot))
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
			r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{})
			if err != nil || r.Status != "policy_blocked" {
				t.Fatalf("result=%+v err=%v", r, err)
			}
			requireMergeDiagnostic(t, r, "release_target", "overrideable")
			code := map[string]string{"contained": "already_contained", "missing-source": "ref_inspection", "invalid-source": "invalid_ref", "missing-target": "ref_inspection"}[fault]
			requireMergeDiagnostic(t, r, code, "must_resolve")
			if fault == "contained" {
				r, err = mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), releaseAcknowledgement(r))
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
	r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{})
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
	_, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), implMergeAcknowledgement{ReleaseTargetOverride: true})
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
				r, err := mergeImplBranch(context.Background(), root, runner, branch, "", mergeMessage(), ack)
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
			r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, input, "goal/topic", mergeMessage(), implMergeAcknowledgement{})
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
			_, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, tc.assertion, mergeMessage(), implMergeAcknowledgement{})
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
	for _, target := range []string{"develop", "main"} {
		t.Run(target, func(t *testing.T) {
			root, branch := mergeFixture(t, target)
			runGit(t, root, "switch", target)
			if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("target\n"), 0644); err != nil {
				t.Fatal(err)
			}
			runGit(t, root, "commit", "-am", "target change")
			ack := implMergeAcknowledgement{}
			if target == "main" {
				r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), ack)
				if err != nil || r.Status != "policy_blocked" {
					t.Fatalf("result=%+v err=%v", r, err)
				}
				ack = releaseAcknowledgement(r)
			}
			r, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, branch, "", mergeMessage(), ack)
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
	_, err := mergeImplBranch(context.Background(), root, wsgit.ExecRunner{}, "impl/-/unit", "", mergeMessage(), implMergeAcknowledgement{})
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
