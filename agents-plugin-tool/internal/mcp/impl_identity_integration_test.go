package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func writeImplIdentityReadyTicket(t *testing.T, root, stem string) string {
	t.Helper()
	path := filepath.Join("ai-docs", "tickets", "ready", stem+".md")
	mustWrite(t, root, path, `---
title: Impl identity fixture
---

# Impl identity fixture

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | fixture |
| scope.surface | public-interface | fixture |
| scope.new_public_symbol | no | fixture |
| scope.new_type_contract | no | fixture |
| scope.test_surface | existing | fixture |
| complexity.reuse_points | confirmed | fixture |
| complexity.side_effect_risk | low | fixture |
| risk.correctness | low | fixture |
| risk.fit | low | fixture |
| risk.test | low | fixture |
| risk.security_or_contract | low | fixture |

## Phases

### Phase 1: Fixture
`)
	return filepath.ToSlash(path)
}

func implIdentityRouteArgs(stem, path, allowRename string) map[string]any {
	return map[string]any{
		"target": map[string]any{
			"kind":        "ticket",
			"label":       stem,
			"ticket_stem": stem,
			"ticket_path": path,
			"scope_label": "Phase 1: Fixture",
		},
		"policy": map[string]any{
			"branch": map[string]any{"allow_rename": allowRename},
		},
		"format": "json",
	}
}

func TestImplIdentityResolveProvisionRouteIntegration(t *testing.T) {
	useLeadProfile(t)
	for _, tc := range []struct {
		name, base string
	}{
		{name: "develop", base: "develop"},
		{name: "nested goal", base: "goal/develop/topic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			initGit(t, root)
			root = canonicalRootForTest(t, root)
			stem := "260921-feat-resolver-" + strings.ReplaceAll(tc.name, " ", "-")
			path := writeImplIdentityReadyTicket(t, root, stem)
			runGit(t, root, "checkout", "-b", "develop")
			runGit(t, root, "add", ".")
			runGit(t, root, "commit", "-m", "fixture")
			if tc.base != "develop" {
				runGit(t, root, "checkout", "-b", tc.base)
			}

			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			server := NewServer(root, "test")
			key, err := server.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
			if err != nil {
				t.Fatal(err)
			}

			list := callToolsList(t, server)
			entry := toolEntryTextByName(t, list, "git.resolve_impl_branch")
			var schema struct {
				Description string `json:"description"`
				InputSchema struct {
					Properties map[string]any `json:"properties"`
					Required   []string       `json:"required"`
				} `json:"inputSchema"`
			}
			if err := json.Unmarshal([]byte(entry), &schema); err != nil {
				t.Fatalf("parse git.resolve_impl_branch schema: %v", err)
			}
			for _, want := range []string{"does not inspect whether the branch exists or has landed", "worktree.acquire", "route.resolve_implement"} {
				if !strings.Contains(schema.Description, want) {
					t.Fatalf("git.resolve_impl_branch description missing %q: %s", want, schema.Description)
				}
			}
			for _, want := range []string{"session_key", "ticket_stem", "base", "format"} {
				if _, ok := schema.InputSchema.Properties[want]; !ok {
					t.Fatalf("git.resolve_impl_branch schema missing property %q: %s", want, entry)
				}
			}
			required := make(map[string]bool, len(schema.InputSchema.Required))
			for _, name := range schema.InputSchema.Required {
				required[name] = true
			}
			for _, want := range []string{"session_key", "ticket_stem", "base"} {
				if !required[want] {
					t.Fatalf("git.resolve_impl_branch schema missing required field %q: %s", want, entry)
				}
			}

			var resolved struct {
				Branch    string `json:"branch"`
				MergeRoot string `json:"merge_root"`
			}
			text := callToolWithKey(t, server, 1, key, "git.resolve_impl_branch", map[string]any{
				"ticket_stem": stem,
				"base":        tc.base,
				"format":      "json",
			})
			if err := json.Unmarshal([]byte(text), &resolved); err != nil {
				t.Fatalf("resolve response: %v\n%s", err, text)
			}
			if resolved.Branch != implTicketBranch(tc.base, stem) || resolved.MergeRoot != implementMergeRootFor(tc.base) {
				t.Fatalf("resolved branch=%q merge_root=%q", resolved.Branch, resolved.MergeRoot)
			}

			var acquired worktreeAcquireResult
			acquire := callToolOnce(t, server, 2, "worktree.acquire", map[string]any{
				"session_key": key, "base": tc.base, "profile": "full", "target_branch": resolved.Branch, "format": "json",
			})
			if err := json.Unmarshal([]byte(toolText(t, acquire)), &acquired); err != nil {
				t.Fatalf("acquire response: %v\n%s", err, acquire)
			}
			t.Cleanup(func() {
				_ = callToolOnce(t, server, 4, "worktree.release", map[string]any{"session_key": key, "key": acquired.WorkerKey})
			})

			var routed implementResult
			text = callToolWithKey(t, server, 3, acquired.WorkerKey, "route.resolve_implement", implIdentityRouteArgs(stem, path, "yes"))
			if err := json.Unmarshal([]byte(text), &routed); err != nil {
				t.Fatalf("route response: %v\n%s", err, text)
			}
			if routed.Verdict.BranchPlan.Action != "continue" || routed.Verdict.BranchPlan.CurrentBranch != resolved.Branch {
				t.Fatalf("provisioned canonical branch did not continue: %+v", routed.Verdict.BranchPlan)
			}
		})
	}
}

func TestResolveImplBranchRequestValidationAndDefaultFormat(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	root = canonicalRootForTest(t, root)
	runGit(t, root, "checkout", "-b", "develop")
	runGit(t, root, "commit", "--allow-empty", "-m", "fixture")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	key, err := server.sessions.mint(root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "missing ticket stem", args: map[string]any{"base": "develop"}, want: "missing required: ticket_stem"},
		{name: "missing base", args: map[string]any{"ticket_stem": "260921-feat-example"}, want: "missing required: base"},
		{name: "invalid base", args: map[string]any{"ticket_stem": "260921-feat-example", "base": "invalid..branch"}, want: "invalid branch name"},
		{name: "detached HEAD", args: map[string]any{"ticket_stem": "260921-feat-example", "base": "HEAD"}, want: "must name a branch"},
		{name: "trimmed detached HEAD", args: map[string]any{"ticket_stem": "260921-feat-example", "base": " HEAD "}, want: "must name a branch"},
		{name: "detached marker", args: map[string]any{"ticket_stem": "260921-feat-example", "base": "(detached)"}, want: "must name a branch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"session_key": key}
			for name, value := range tc.args {
				args[name] = value
			}
			response := callToolOnce(t, server, 1, "git.resolve_impl_branch", args)
			if !toolIsError(t, response) || !strings.Contains(toolText(t, response), tc.want) {
				t.Fatalf("request was not rejected as expected: %s", response)
			}
		})
	}

	stem := "260921-feat-default-format"
	text := callToolWithKey(t, server, 2, key, "git.resolve_impl_branch", map[string]any{"ticket_stem": stem, "base": "develop"})
	want := "branch: " + implTicketBranch("develop", stem) + "\nmerge_root: develop\n"
	if text != want {
		t.Fatalf("default response = %q, want compact response %q", text, want)
	}
}

func TestImplIdentityVouchedLandedTargetRoute(t *testing.T) {
	useLeadProfile(t)
	for _, base := range []string{"develop", "goal/develop/topic"} {
		for _, uniqueTarget := range []bool{false, true} {
			name := base + "/landed"
			if uniqueTarget {
				name = base + "/unique"
			}
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				initGit(t, root)
				root = canonicalRootForTest(t, root)
				stem := "260921-feat-vouched-reentry"
				path := writeImplIdentityReadyTicket(t, root, stem)
				runGit(t, root, "switch", "-c", base)
				runGit(t, root, "add", ".")
				runGit(t, root, "commit", "-m", "fixture")
				target := implTicketBranch(base, stem)
				runGit(t, root, "switch", "-c", target)
				runGit(t, root, "commit", "--allow-empty", "-m", "earlier phase")
				runGit(t, root, "switch", base)
				runGit(t, root, "merge", "--no-ff", target, "-m", "land earlier phase")
				if uniqueTarget {
					runGit(t, root, "switch", target)
					runGit(t, root, "commit", "--allow-empty", "-m", "unique target work")
					runGit(t, root, "switch", base)
				}
				branch := "impl/" + base + "/manual"
				runGit(t, root, "switch", "-c", branch)
				runGit(t, root, "commit", "--allow-empty", "-m", "vouched current work")
				runGit(t, root, "branch", "--set-upstream-to", base)
				refsBefore := string(runGitOutput(t, root, "show-ref", "--heads"))
				currentTip := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", branch)))
				baseTip := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", base)))
				upstream := string(runGitOutput(t, root, "config", "--get-regexp", "^branch\\."))

				t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
				server := NewServer(root, "test")
				key, err := server.sessions.mint(root, roleLead, "")
				if err != nil {
					t.Fatal(err)
				}
				args := implIdentityRouteArgs(stem, path, "no")
				policy := args["policy"].(map[string]any)["branch"].(map[string]any)
				policy["identity_vouch"] = map[string]any{"branch": branch, "ticket_stem": stem}
				policy["merge_target"] = "ignored-root"
				text := callToolWithKey(t, server, 1, key, "route.resolve_implement", args)
				var routed implementResult
				if err := json.Unmarshal([]byte(text), &routed); err != nil {
					t.Fatalf("route response: %v\n%s", err, text)
				}
				if got := string(runGitOutput(t, root, "show-ref", "--heads")); got != refsBefore {
					t.Fatalf("routing mutated refs: before %s, after %s", refsBefore, got)
				}
				if got := strings.TrimSpace(string(runGitOutput(t, root, "symbolic-ref", "--short", "HEAD"))); got != branch {
					t.Fatalf("routing switched current branch to %s", got)
				}
				if got := string(runGitOutput(t, root, "config", "--get-regexp", "^branch\\.")); got != upstream {
					t.Fatalf("routing changed tracking: %s", got)
				}
				plan := routed.Verdict.BranchPlan
				if uniqueTarget {
					if plan.Action != "stop" || strings.Contains(routed.NextInstruction, "git branch -d") {
						t.Fatalf("unique target became cleanup-eligible: %+v\n%s", plan, routed.NextInstruction)
					}
					return
				}
				if plan.Action != "recreate" || plan.CurrentBranch != branch || plan.TargetBranch != target || plan.MergeTarget != base || plan.StartCommit != currentTip {
					t.Fatalf("wrong recreate plan: %+v", plan)
				}
				for _, want := range []string{"git branch -d", target, "recreate it from " + base} {
					if !strings.Contains(routed.NextInstruction, want) {
						t.Fatalf("missing %q in %s", want, routed.NextInstruction)
					}
				}
				for _, warning := range routed.Warnings {
					if strings.Contains(warning, "not on an implementation branch") {
						t.Fatalf("name-rooted recreate was misreported as create-path: %s", warning)
					}
				}

				// Even an eligible plan does not authorize forced deletion of a
				// branch held by another worktree. Git must refuse and keep refs.
				held := filepath.Join(t.TempDir(), "held-target")
				runGit(t, root, "worktree", "add", held, target)
				if _, err := server.gitRunner().RunGit(context.Background(), root, "branch", "-d", target); err == nil {
					t.Fatal("non-forced deletion removed a checked-out target")
				}
				if got := string(runGitOutput(t, root, "show-ref", "--heads")); got != refsBefore {
					t.Fatalf("refused deletion changed refs: %s", got)
				}
				runGit(t, root, "worktree", "remove", held)
				runGit(t, root, "branch", "-d", target)
				runGit(t, root, "switch", "-c", target, base)
				if got := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD"))); got != baseTip {
					t.Fatalf("recreated target starts at %s, want merge root %s", got, baseTip)
				}
				if got := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", branch))); got != currentTip {
					t.Fatalf("prior current work lost: %s, want %s", got, currentTip)
				}
				if got := string(runGitOutput(t, root, "config", "--get-regexp", "^branch\\.")); got != upstream {
					t.Fatalf("recreation changed prior branch tracking: %s", got)
				}
			})
		}
	}
}

func TestParseImplementPolicyRejectsMalformedIdentityVouch(t *testing.T) {
	for _, tc := range []struct {
		name  string
		vouch any
	}{
		{name: "not an object", vouch: "manual"},
		{name: "nil", vouch: nil},
		{name: "missing branch", vouch: map[string]any{"ticket_stem": "260921-feat-example"}},
		{name: "branch is not string", vouch: map[string]any{"branch": 1, "ticket_stem": "260921-feat-example"}},
		{name: "blank branch", vouch: map[string]any{"branch": " \t", "ticket_stem": "260921-feat-example"}},
		{name: "missing ticket", vouch: map[string]any{"branch": "impl/develop/manual"}},
		{name: "ticket is not string", vouch: map[string]any{"branch": "impl/develop/manual", "ticket_stem": false}},
		{name: "blank ticket", vouch: map[string]any{"branch": "impl/develop/manual", "ticket_stem": "\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseImplementPolicy(map[string]any{
				"branch": map[string]any{"identity_vouch": tc.vouch},
			})
			if err == nil || !strings.Contains(err.Error(), "policy.branch.identity_vouch") {
				t.Fatalf("malformed vouch error = %v", err)
			}
		})
	}
}

func TestImplIdentityVouchRouteConsumption(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	root = canonicalRootForTest(t, root)
	stem := "260921-feat-vouched-manual-branch"
	path := writeImplIdentityReadyTicket(t, root, stem)
	runGit(t, root, "checkout", "-b", "develop")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "fixture")
	branch := "impl/develop/manual"
	runGit(t, root, "checkout", "-b", branch)
	runGit(t, root, "commit", "--allow-empty", "-m", "unmerged fixture work")

	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	key, err := server.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
	if err != nil {
		t.Fatal(err)
	}

	args := implIdentityRouteArgs(stem, path, "yes")
	var noVouch implementResult
	text := callToolWithKey(t, server, 1, key, "route.resolve_implement", args)
	if err := json.Unmarshal([]byte(text), &noVouch); err != nil {
		t.Fatalf("no-vouch route response: %v\n%s", err, text)
	}
	if noVouch.Verdict.BranchPlan.Action != "stop" || !strings.Contains(noVouch.NextInstruction, "worker must not self-authorize") {
		t.Fatalf("no vouch did not preserve the safety stop: %+v\n%s", noVouch.Verdict.BranchPlan, noVouch.NextInstruction)
	}

	args = implIdentityRouteArgs(stem, path, "yes")
	args["policy"].(map[string]any)["branch"].(map[string]any)["identity_vouch"] = map[string]any{"branch": branch, "ticket_stem": stem}
	var renamed implementResult
	text = callToolWithKey(t, server, 2, key, "route.resolve_implement", args)
	if err := json.Unmarshal([]byte(text), &renamed); err != nil {
		t.Fatalf("vouched route response: %v\n%s", err, text)
	}
	if renamed.Verdict.BranchPlan.Action != "rename" || renamed.Verdict.BranchPlan.TargetBranch != implTicketBranch("develop", stem) {
		t.Fatalf("vouch did not consume the identity assertion into canonical rename: %+v", renamed.Verdict.BranchPlan)
	}

	args = implIdentityRouteArgs(stem, path, "no")
	args["policy"].(map[string]any)["branch"].(map[string]any)["identity_vouch"] = map[string]any{"branch": branch, "ticket_stem": stem}
	var shared implementResult
	text = callToolWithKey(t, server, 3, key, "route.resolve_implement", args)
	if err := json.Unmarshal([]byte(text), &shared); err != nil {
		t.Fatalf("vouched shared route response: %v\n%s", err, text)
	}
	if shared.Verdict.BranchPlan.Action != "continue" || shared.Verdict.BranchPlan.TargetBranch != branch {
		t.Fatalf("vouched rename-disabled branch did not continue in place: %+v", shared.Verdict.BranchPlan)
	}
}

// TestImplIdentityMergedLeftoverResolveAcquireRoute pins Decision 3's end-to-end
// flow: git.resolve_impl_branch keeps returning the deterministic canonical
// branch name (unchanged contract) even after that branch was merged into its
// merge root; worktree.acquire recreates the fully landed leftover at develop's
// tip so route.resolve_implement continues cleanly, while an unlanded leftover
// with unique commits is preserved rather than discarded.
func TestImplIdentityMergedLeftoverResolveAcquireRoute(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	root = canonicalRootForTest(t, root)
	stem := "261121-feat-merged-leftover-reentry"
	path := writeImplIdentityReadyTicket(t, root, stem)
	runGit(t, root, "checkout", "-b", "develop")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "fixture")

	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	key, err := server.sessions.mint(root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}

	resolveBranch := func(id int) (branch, mergeRoot, ticketStem string) {
		t.Helper()
		var got struct {
			Branch     string `json:"branch"`
			MergeRoot  string `json:"merge_root"`
			TicketStem string `json:"ticket_stem"`
		}
		text := callToolWithKey(t, server, id, key, "git.resolve_impl_branch", map[string]any{
			"ticket_stem": stem, "base": "develop", "format": "json",
		})
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Fatalf("resolve response: %v\n%s", err, text)
		}
		return got.Branch, got.MergeRoot, got.TicketStem
	}

	canonical, mergeRoot, gotStem := resolveBranch(1)
	if canonical != implTicketBranch("develop", stem) || mergeRoot != "develop" || gotStem != stem {
		t.Fatalf("first resolve = (%q, %q, %q)", canonical, mergeRoot, gotStem)
	}

	// Simulate an earlier run whose canonical branch landed on develop via
	// --no-ff merge, leaving the local canonical ref behind develop.
	runGit(t, root, "switch", "-c", canonical)
	runGit(t, root, "commit", "--allow-empty", "-m", "earlier phase work")
	runGit(t, root, "switch", "develop")
	runGit(t, root, "merge", "--no-ff", canonical, "-m", "merge earlier phase")
	developTip := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "develop")))

	// Pure name resolution returns the exact same canonical name.
	second, _, _ := resolveBranch(2)
	if second != canonical {
		t.Fatalf("second resolve = %q, want canonical %q", second, canonical)
	}

	// worktree.acquire recreates the merged canonical branch at develop's tip.
	var acquired worktreeAcquireResult
	acqResp := callToolOnce(t, server, 3, "worktree.acquire", map[string]any{
		"session_key": key, "base": "develop", "profile": "full", "target_branch": second, "format": "json",
	})
	if toolIsError(t, acqResp) {
		t.Fatalf("acquire merged canonical branch failed: %s", acqResp)
	}
	if err := json.Unmarshal([]byte(toolText(t, acqResp)), &acquired); err != nil {
		t.Fatalf("parse acquire: %v\n%s", err, acqResp)
	}
	t.Cleanup(func() {
		_ = callToolOnce(t, server, 99, "worktree.release", map[string]any{"session_key": key, "key": acquired.WorkerKey})
	})
	if gotHead := strings.TrimSpace(string(runGitOutput(t, acquired.Path, "rev-parse", "HEAD"))); gotHead != developTip {
		t.Fatalf("recreated canonical head = %s, want develop tip %s", gotHead, developTip)
	}

	// route.resolve_implement continues cleanly on the recreated canonical branch.
	var routed implementResult
	routeText := callToolWithKey(t, server, 4, acquired.WorkerKey, "route.resolve_implement", implIdentityRouteArgs(stem, path, "yes"))
	if err := json.Unmarshal([]byte(routeText), &routed); err != nil {
		t.Fatalf("route response: %v\n%s", err, routeText)
	}
	if routed.Verdict.BranchPlan.Action != "continue" || routed.Verdict.BranchPlan.CurrentBranch != canonical {
		t.Fatalf("recreated canonical branch did not continue: %+v", routed.Verdict.BranchPlan)
	}

	// An unlanded commit on the canonical branch must be preserved on re-acquire.
	runGit(t, acquired.Path, "commit", "--allow-empty", "-m", "unlanded work")
	unlandedTip := strings.TrimSpace(string(runGitOutput(t, acquired.Path, "rev-parse", "HEAD")))
	if rel := callToolOnce(t, server, 5, "worktree.release", map[string]any{"session_key": key, "key": acquired.WorkerKey}); toolIsError(t, rel) {
		t.Fatalf("release failed: %s", rel)
	}
	runGit(t, root, "commit", "--allow-empty", "-m", "develop moved ahead")
	acqAgain := callToolOnce(t, server, 6, "worktree.acquire", map[string]any{
		"session_key": key, "base": "develop", "profile": "full", "target_branch": canonical, "format": "json",
	})
	if toolIsError(t, acqAgain) {
		t.Fatalf("re-acquire unlanded canonical branch failed: %s", acqAgain)
	}
	var acquired2 worktreeAcquireResult
	if err := json.Unmarshal([]byte(toolText(t, acqAgain)), &acquired2); err != nil {
		t.Fatalf("parse second acquire: %v", err)
	}
	t.Cleanup(func() {
		_ = callToolOnce(t, server, 100, "worktree.release", map[string]any{"session_key": key, "key": acquired2.WorkerKey})
	})
	if gotHead := strings.TrimSpace(string(runGitOutput(t, acquired2.Path, "rev-parse", "HEAD"))); gotHead != unlandedTip {
		t.Fatalf("unlanded canonical branch was reset: got %s, want %s", gotHead, unlandedTip)
	}
}
