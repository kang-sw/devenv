package mcp

import (
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
			for _, want := range []string{"ticket_stem", "base", "format", "Pure name resolution"} {
				if !strings.Contains(entry, want) {
					t.Fatalf("git.resolve_impl_branch schema missing %q: %s", want, entry)
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
				"session_key": key, "base": tc.base, "target_branch": resolved.Branch, "format": "json",
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
