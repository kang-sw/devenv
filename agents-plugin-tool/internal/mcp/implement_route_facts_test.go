package mcp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnterImplementTicketTargetRoutesFromTicketFacts pins the relocation: the
// caller supplies the target and the runtime policy only, and every fact the
// verdict is derived from comes off the ticket's ## Route Facts section.
func TestEnterImplementTicketTargetRoutesFromTicketFacts(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	runGit(t, root, "switch", "-c", "feature/base")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
	writeImplementReadyTicket(t, root, implementReadyFacts())

	var result implementResult
	if err := json.Unmarshal([]byte(callToolWithKey(t, server, 2, key, "route.resolve_implement", implementReadyArgs("json"))), &result); err != nil {
		t.Fatalf("json verdict did not parse: %v", err)
	}
	if result.RouteFacts != "read from the ticket" {
		t.Fatalf("route facts = %q, want the ticket source", result.RouteFacts)
	}
	if result.Verdict.ReviewAlloc != "partitioned: correctness, fit, test" {
		t.Fatalf("review alloc = %q, want the ticket facts' partition", result.Verdict.ReviewAlloc)
	}
	for _, want := range []string{"span=multi-file", "surface=public-interface", "correctness-risk=high", "route-facts=ticket"} {
		if !containsString(result.Conditions, want) {
			t.Fatalf("conditions missing %q: %v", want, result.Conditions)
		}
	}

	// The same ticket, one fact edited, reroutes without the caller changing.
	facts := implementReadyFacts()
	facts["risk.correctness"] = "low"
	facts["risk.fit"] = "low"
	facts["risk.test"] = "low"
	facts["risk.security_or_contract"] = "low"
	facts["scope.new_type_contract"] = "no"
	facts["complexity.reuse_points"] = "confirmed"
	writeImplementReadyTicket(t, root, facts)
	if err := json.Unmarshal([]byte(callToolWithKey(t, server, 3, key, "route.resolve_implement", implementReadyArgs("json"))), &result); err != nil {
		t.Fatalf("json verdict did not parse: %v", err)
	}
	if result.Verdict.ReviewAlloc != "single" || !result.Verdict.NeedReview {
		t.Fatalf("edited ticket facts did not reroute: %q need=%v", result.Verdict.ReviewAlloc, result.Verdict.NeedReview)
	}
}

// TestEnterImplementRejectsCallerSuppliedFacts pins the single source of
// truth: facts defaulted from the ticket but overridable by the caller would
// leave the same judgment in two places, which is the cost the relocation
// removes.
func TestEnterImplementRejectsCallerSuppliedFacts(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
	writeImplementReadyTicket(t, root, implementReadyFacts())

	args := implementReadyArgs("text")
	args["facts"] = map[string]any{"risk": map[string]any{"correctness": "low"}}
	got := callToolWithKey(t, server, 2, key, "route.resolve_implement", args)
	if !strings.Contains(got, "facts are not a caller argument") || !strings.Contains(got, "## Route Facts") {
		t.Fatalf("caller-supplied facts should be refused with the new location: %s", got)
	}
}

// TestEnterImplementMissingRouteFactsIsNamedNotDefaulted pins the named
// outcome. A conservative fallback verdict is exactly what a capable caller
// obeys without noticing the facts behind it do not exist, so the absence is
// stated in the verdict text, the conditions, and the warnings.
func TestEnterImplementMissingRouteFactsIsNamedNotDefaulted(t *testing.T) {
	for _, tc := range []struct {
		name       string
		body       string
		wantStatus string
		wantDetail string
	}{
		{
			name:       "no ticket file",
			wantStatus: "absent",
			wantDetail: "does not exist",
		},
		{
			name:       "no section",
			body:       "---\ntitle: Demo\n---\n\n# Demo\n\n## Phases\n\n### Phase 1: Demo\n",
			wantStatus: "absent",
			wantDetail: "has no ## Route Facts section",
		},
		{
			name:       "section with no rows",
			body:       "---\ntitle: Demo\n---\n\n# Demo\n\n## Route Facts\n\nTBD.\n\n## Phases\n",
			wantStatus: "unreadable",
			wantDetail: "has no fact rows",
		},
		{
			name:       "unrecognized fact",
			body:       "---\ntitle: Demo\n---\n\n# Demo\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| complexity.cold_context | no |\n",
			wantStatus: "unreadable",
			wantDetail: `unrecognized route fact "complexity.cold_context"`,
		},
		{
			name:       "value outside the enum",
			body:       "---\ntitle: Demo\n---\n\n# Demo\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| risk.correctness | medium |\n",
			wantStatus: "unreadable",
			wantDetail: `invalid correctness "medium"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useLeadProfile(t)
			root := t.TempDir()
			initGit(t, root)
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			server := NewServer(root, "test")
			key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
			if tc.body != "" {
				mustWrite(t, root, "ai-docs/tickets/ready/260627-feat-enter-implement-deterministic-verdict-engine.md", tc.body)
			}

			var result implementResult
			if err := json.Unmarshal([]byte(callToolWithKey(t, server, 2, key, "route.resolve_implement", implementReadyArgs("json"))), &result); err != nil {
				t.Fatalf("json verdict did not parse: %v", err)
			}
			if !strings.HasPrefix(result.RouteFacts, "missing ("+tc.wantStatus+")") || !strings.Contains(result.RouteFacts, tc.wantDetail) {
				t.Fatalf("route facts line = %q, want missing (%s) naming %q", result.RouteFacts, tc.wantStatus, tc.wantDetail)
			}
			if !strings.HasPrefix(result.NextInstruction, "Stop and report missing route facts:") {
				t.Fatalf("next instruction did not lead with the missing-facts stop: %q", result.NextInstruction)
			}
			if !containsString(result.Conditions, "route-facts="+tc.wantStatus) {
				t.Fatalf("conditions missing the named outcome: %v", result.Conditions)
			}
			var named bool
			for _, warning := range result.Warnings {
				if strings.Contains(warning, "route facts missing") {
					named = true
				}
			}
			if !named {
				t.Fatalf("warnings did not name the missing facts: %v", result.Warnings)
			}
			// The branch plan is still resolved: the caller needs it to act on
			// the stop, and withholding it would make the outcome unreadable.
			if result.Verdict.BranchPlan.Action != "create" {
				t.Fatalf("branch plan dropped on the missing-facts path: %+v", result.Verdict.BranchPlan)
			}
		})
	}
}

// TestEnterImplementAdHocTargetReadsNoTicket pins the ad-hoc path: an inline
// or unknown target has no ticket, its description is the contract, and the
// absence of facts is stated as such rather than as a missing-facts stop.
func TestEnterImplementAdHocTargetReadsNoTicket(t *testing.T) {
	for _, kind := range []string{"inline", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			useLeadProfile(t)
			root := t.TempDir()
			initGit(t, root)
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			server := NewServer(root, "test")
			key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

			args := implementSkipDocsArgs("json")
			args["target"].(map[string]any)["kind"] = kind
			var result implementResult
			if err := json.Unmarshal([]byte(callToolWithKey(t, server, 2, key, "route.resolve_implement", args)), &result); err != nil {
				t.Fatalf("json verdict did not parse: %v", err)
			}
			if !strings.HasPrefix(result.RouteFacts, "n/a (ad-hoc target") {
				t.Fatalf("route facts = %q, want the ad-hoc statement", result.RouteFacts)
			}
			if !strings.HasPrefix(result.NextInstruction, "Ad-hoc target:") || !strings.Contains(result.NextInstruction, "closed list") {
				t.Fatalf("ad-hoc next instruction did not point at the stop protocol: %q", result.NextInstruction)
			}
			if strings.Contains(result.NextInstruction, "missing route facts") {
				t.Fatalf("ad-hoc run reported missing facts: %q", result.NextInstruction)
			}
			if !result.Verdict.NeedReview {
				t.Fatalf("ad-hoc run dropped independent review: %+v", result.Verdict)
			}
			record, ok := server.sessions.readState(key)
			if !ok {
				t.Fatal("session record not found")
			}
			if !eqKeys(keysOf(record.Todos), "route", "prep", "edit", "review", "final-action-gate", "merge") {
				t.Fatalf("ad-hoc todo shape = %v", keysOf(record.Todos))
			}
			full := callToolWithKey(t, server, 3, key, "todo.list", map[string]any{"mode": "full"})
			for _, forbidden := range []string{"plan-populator", "Plan Depth", "plan_depth", "survey plan"} {
				if strings.Contains(full, forbidden) {
					t.Fatalf("ad-hoc todo list retained planning-stage text %q:\n%s", forbidden, full)
				}
			}
		})
	}
}
