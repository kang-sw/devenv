package mcp

import (
	"strings"
	"testing"
)

func todoByKey(t *testing.T, list []todoItem, key string) todoItem {
	t.Helper()
	for _, item := range list {
		if item.Key == key {
			return item
		}
	}
	t.Fatalf("todo key %q not found in %v", key, keysOf(list))
	return todoItem{}
}

func requireInstruction(t *testing.T, item todoItem) string {
	t.Helper()
	if item.Instruction == nil || *item.Instruction == "" {
		t.Fatalf("todo %q has no instruction: %+v", item.Key, item)
	}
	return *item.Instruction
}

// TestDeriveImplementTodos pins the whole installed skeleton, including the
// absence of any documentation step: the write-time doc pass is retired, so
// review is the only optional block left.
func TestDeriveImplementTodos(t *testing.T) {
	cases := []struct {
		needReview bool
		want       []string
	}{
		{false, []string{"route", "prep", "edit", "final-action-gate", "merge"}},
		{true, []string{"route", "prep", "edit", "review", "final-action-gate", "merge"}},
	}
	for _, tc := range cases {
		got := deriveImplementTodosFromVerdict(implementTodoVerdict{
			Delegation:  implementDelegationMode,
			ReviewAlloc: "partitioned",
			NeedReview:  tc.needReview,
		})
		if !eqKeys(keysOf(got), tc.want...) {
			t.Fatalf("derive(review=%v) = %v, want %v", tc.needReview, keysOf(got), tc.want)
		}
		for _, item := range got {
			if item.Status != todoPending {
				t.Fatalf("derived item %q status = %q, want pending", item.Key, item.Status)
			}
		}
	}
}

func TestDeriveImplementTodosFromVerdictTitles(t *testing.T) {
	got := deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  "delegated",
		ReviewAlloc: "single",
		NeedReview:  true,
	})
	wantTitles := map[string]string{
		"prep":   "Prep",
		"edit":   "Edit",
		"review": "Review (single)",
	}
	for _, item := range got {
		if want, ok := wantTitles[item.Key]; ok && item.Title != want {
			t.Fatalf("%s title = %q, want %q", item.Key, item.Title, want)
		}
	}
}

// TestDeriveImplementTodoInstructionsPrepGuardrails pins the reduced Prep
// preamble: the project-declared binding anchor (a generic hook, omitted
// entirely when the project declares none) plus the implementation playbook,
// and nothing else. The mental-model lookup clause is gone with the layer that
// backed it.
func TestDeriveImplementTodoInstructionsPrepGuardrails(t *testing.T) {
	// declaredClause is the pre-rendered anchor clause a declaring project's
	// route.resolve_implement handler would splice in; the no-declaration case
	// leaves BindingAnchorClause empty so the clause is omitted entirely.
	const declaredClause = "read ai-docs/tickets/idea/demo-anchor.md when target touches plugin architecture, adapter boundaries, "
	for _, tc := range []struct {
		name         string
		anchorClause string
	}{
		{name: "no declaration"},
		{name: "declared anchor", anchorClause: declaredClause},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveImplementTodosFromVerdict(implementTodoVerdict{
				Delegation:          "delegated",
				BranchPlan:          implementBranchPlan{Action: "continue", CurrentBranch: "impl/demo/demo"},
				ReviewAlloc:         "single",
				NeedReview:          true,
				BindingAnchorClause: tc.anchorClause,
			})
			prep := requireInstruction(t, todoByKey(t, got, "prep"))
			if !strings.HasPrefix(prep, "Before edits or dispatch, ") {
				t.Fatalf("prep instruction for %s lost its preamble: %q", tc.name, prep)
			}
			if !strings.Contains(prep, `infra.read("impl-playbook")`) {
				t.Fatalf("prep instruction for %s dropped the implementation playbook read: %q", tc.name, prep)
			}
			for _, forbidden := range []string{"mental-model lookup", "docs ancestors", "plan-populator", "path.generate"} {
				if strings.Contains(prep, forbidden) {
					t.Fatalf("prep instruction for %s retained removed clause %q: %q", tc.name, forbidden, prep)
				}
			}
			if tc.anchorClause == "" {
				if strings.Contains(prep, "when target touches") {
					t.Fatalf("prep instruction for %s unexpectedly rendered an anchor clause: %q", tc.name, prep)
				}
			} else if !strings.Contains(prep, tc.anchorClause) {
				t.Fatalf("prep instruction for %s missing declared anchor clause %q: %q", tc.name, tc.anchorClause, prep)
			}
		})
	}
}

func TestImplementReviewStructuredWrappers(t *testing.T) {
	for _, partition := range []string{"correctness", "fit", "test"} {
		t.Run(partition, func(t *testing.T) {
			review := implementReviewInstruction(implementTodoVerdict{ReviewAlloc: "partitioned: " + partition})
			if !strings.Contains(review, partition+" (`code-review-"+partition+"`)") {
				t.Fatalf("partition lost its structured wrapper: %q", review)
			}
			if strings.Contains(review, "`code-reviewer`") {
				t.Fatalf("flat include became a delegated wrapper: %q", review)
			}
		})
	}
	review := implementReviewInstruction(implementTodoVerdict{ReviewAlloc: "single"})
	if !strings.Contains(review, "Render `reviewer`") || strings.Contains(review, "code-review-") {
		t.Fatalf("single review lost its full-scope wrapper: %q", review)
	}
}

func TestDeriveImplementTodoInstructionsPartitionedReview(t *testing.T) {
	got := deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  "delegated",
		BranchPlan:  implementBranchPlan{Action: "continue", CurrentBranch: "implement/demo"},
		ReviewAlloc: "partitioned: correctness, test",
		NeedReview:  true,
	})
	review := requireInstruction(t, todoByKey(t, got, "review"))
	if !strings.Contains(review, "Dispatch correctness (`code-review-correctness`) and test (`code-review-test`) reviewers") {
		t.Fatalf("review instruction missing selected partitions: %q", review)
	}
	if !strings.Contains(review, "render each named structured wrapper") || !strings.Contains(review, "fresh reviewer render") {
		t.Fatalf("review instruction missing named template guidance: %q", review)
	}
	if strings.Contains(review, "fit") {
		t.Fatalf("review instruction mentioned unselected fit partition: %q", review)
	}
	for _, want := range implementReviewRoundWants() {
		if !strings.Contains(review, want) {
			t.Fatalf("partitioned review instruction missing %q: %q", want, review)
		}
	}
	for _, forbidden := range implementReviewRoundForbidden() {
		if strings.Contains(review, forbidden) {
			t.Fatalf("partitioned review instruction retained superseded relay/elevation wording %q: %q", forbidden, review)
		}
	}
}

// implementReviewRoundWants pins the two-round clause set every review-dispatching
// allocation shape (single, partitioned, bare partitioned) shares, matching the
// shipped ticket-worker review protocol. Each entry is load-bearing:
//   - the worker fixes findings itself, by severity, with no relay delegate;
//   - Minor findings are recorded in the review summary only;
//   - review is two rounds, never more, and round two uses a fresh reviewer
//     render that verifies round one's fixes only and raises nothing new;
//   - a new observation from round two is reported as unresolved, not a third
//     round;
//   - a Critical finding still open after round two is a stop reported to the
//     lead, never a silent elevation or re-review.
func implementReviewRoundWants() []string {
	return []string{
		"Fix the findings yourself by severity; there is no relay delegate",
		"Record Minor findings in the review summary only",
		"Review is two rounds, never more",
		"round two uses a fresh reviewer render that checks only whether round one's findings were fixed",
		"raises nothing new, and reports any new observation as unresolved rather than opening a third round",
		"A Critical finding still open after round two is a stop",
		"report it to the lead rather than elevating or re-reviewing it",
	}
}

// implementReviewRoundForbidden pins the superseded relay/elevation vocabulary
// out of existence on every review-dispatching path. The two-round worker-owned
// model retires the relay delegate, the disposition-marker vocabulary, the
// bounded multi-round Critical branch, and the ceiling elevation, plus the
// still-older review-adjudicator arbitration terms.
func implementReviewRoundForbidden() []string {
	return []string{
		"implementer-relay",
		"implementer-elevated",
		"Critical exception",
		"review #2",
		"review #3",
		"relay #1",
		"unconditionally elevate",
		"disposition marker",
		"[won't fix: <reason>]",
		"review-adjudicator",
		"[maintained]",
		"Adjudicate at most once per relay slot",
		"Capacity:",
		"Root-cause:",
		"Elevated inputs:",
		"Precedence:",
	}
}

func TestDeriveImplementTodoInstructionsBarePartitionedReviewFallback(t *testing.T) {
	got := deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  "delegated",
		BranchPlan:  implementBranchPlan{Action: "continue", CurrentBranch: "implement/demo"},
		ReviewAlloc: "partitioned",
		NeedReview:  true,
	})
	review := requireInstruction(t, todoByKey(t, got, "review"))
	if !strings.Contains(review, "Dispatch the selected reviewers") {
		t.Fatalf("bare partitioned alloc did not reach the fallback review instruction: %q", review)
	}
	for _, want := range implementReviewRoundWants() {
		if !strings.Contains(review, want) {
			t.Fatalf("fallback review instruction missing %q: %q", want, review)
		}
	}
	for _, forbidden := range implementReviewRoundForbidden() {
		if strings.Contains(review, forbidden) {
			t.Fatalf("fallback review instruction retained superseded relay/elevation wording %q: %q", forbidden, review)
		}
	}
}

// TestDeriveImplementTodoInstructionsFinalGate pins what is left of the
// documentation surface after the write-time doc pass was retired: no doc todo
// is installed, and the final gate carries the verification guidance with no
// clause promising a documentation closeout the skeleton can no longer run.
func TestDeriveImplementTodoInstructionsFinalGate(t *testing.T) {
	got := deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  "delegated",
		BranchPlan:  implementBranchPlan{Action: "continue", CurrentBranch: "implement/demo"},
		ReviewAlloc: "single",
		NeedReview:  true,
	})
	for _, key := range []string{"doc-pre-pass", "doc-commit-gate", "doc-closeout"} {
		if idx := indexOfTodo(got, key); idx >= 0 {
			t.Fatalf("no doc todo may be installed, found %s: %v", key, keysOf(got))
		}
	}
	finalAction := requireInstruction(t, todoByKey(t, got, "final-action-gate"))
	for _, want := range []string{"impl-playbook unchanged-input verification rule", "documentation-only commits run affected checks", "Verify the review is resolved"} {
		if !strings.Contains(finalAction, want) {
			t.Fatalf("final-action-gate instruction missing reusable verification guidance %q: %q", want, finalAction)
		}
	}
	for _, forbidden := range []string{"documentation closeout", "documentation policy"} {
		if strings.Contains(finalAction, forbidden) {
			t.Fatalf("final-action-gate instruction still promises %q, but no documentation todo is installed: %q", forbidden, finalAction)
		}
	}
}

// Both todo paths prohibit worker merging and hand the confirmation signal
// to the lead, including when skip is explicitly selected.
func TestDeriveImplementTodoInstructionsMergeConfirmSkip(t *testing.T) {
	for _, action := range []string{"continue", "create"} {
		skip := deriveImplementTodosFromVerdict(implementTodoVerdict{
			Delegation:  "delegated",
			BranchPlan:  implementBranchPlan{Action: action, CurrentBranch: "goal/drain-example", MergeConfirm: "skip"},
			ReviewAlloc: "single",
			NeedReview:  true,
		})
		finalAction := requireInstruction(t, todoByKey(t, skip, "final-action-gate"))
		if !strings.Contains(finalAction, "Do not merge; the worker ends at the report") {
			t.Fatalf("[action=%s] final-action-gate instruction should default to no-merge: %q", action, finalAction)
		}
		if !strings.Contains(finalAction, "merge_confirm: skip") {
			t.Fatalf("[action=%s] final-action-gate instruction with merge_confirm=skip should describe the explicit-merge no-approval path: %q", action, finalAction)
		}
		merge := requireInstruction(t, todoByKey(t, skip, "merge"))
		if strings.Contains(merge, "After user approval") {
			t.Fatalf("[action=%s] merge instruction with merge_confirm=skip should not require approval when a merge is chosen: %q", action, merge)
		}
		if !strings.Contains(merge, "the lead auto-calls git.merge") {
			t.Fatalf("[action=%s] merge instruction with merge_confirm=skip missing explicit-merge no-approval guidance: %q", action, merge)
		}
		if !strings.Contains(merge, "Do not merge from the worker") {
			t.Fatalf("[action=%s] merge instruction should be opt-in with no-merge default stated: %q", action, merge)
		}
	}

	for _, tc := range []struct {
		name         string
		mergeConfirm string
	}{
		{"ask", "ask"},
		{"absent", ""},
	} {
		for _, action := range []string{"continue", "create"} {
			verdict := deriveImplementTodosFromVerdict(implementTodoVerdict{
				Delegation:  "delegated",
				BranchPlan:  implementBranchPlan{Action: action, CurrentBranch: "implement/demo", MergeConfirm: tc.mergeConfirm},
				ReviewAlloc: "single",
				NeedReview:  true,
			})
			finalAction := requireInstruction(t, todoByKey(t, verdict, "final-action-gate"))
			if !strings.Contains(finalAction, "Do not merge; the worker ends at the report") {
				t.Fatalf("[mergeConfirm=%s action=%s] final-action-gate instruction should default to no-merge: %q", tc.name, action, finalAction)
			}
			if !strings.Contains(finalAction, "merge_confirm: ask") {
				t.Fatalf("[mergeConfirm=%s action=%s] final-action-gate instruction should still require approval for an explicitly chosen merge: %q", tc.name, action, finalAction)
			}
			merge := requireInstruction(t, todoByKey(t, verdict, "merge"))
			if !strings.Contains(merge, "the lead surfaces the report for user approval before calling git.merge") {
				t.Fatalf("[mergeConfirm=%s action=%s] merge instruction with merge_confirm=%s should still require approval when a merge is chosen: %q", tc.name, action, tc.mergeConfirm, merge)
			}
			if !strings.Contains(merge, "Do not merge from the worker") {
				t.Fatalf("[mergeConfirm=%s action=%s] merge instruction should state no-merge as the default outcome: %q", tc.name, action, merge)
			}
		}
	}
}

// The recreate branch action installs a worker-facing route instruction that
// deletes the fully-landed leftover before creating, and never promises a plain
// create-over that a raw `git switch -c` would fail on.
func TestDeriveImplementTodoInstructionsRecreate(t *testing.T) {
	verdict := deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  "delegated",
		BranchPlan:  implementBranchPlan{Action: "recreate", CurrentBranch: "develop", TargetBranch: "impl/develop/leftover", MergeTarget: "develop", MergeConfirm: "ask"},
		ReviewAlloc: "single",
		NeedReview:  true,
	})
	route := requireInstruction(t, todoByKey(t, verdict, "route"))
	for _, want := range []string{"Delete the leftover", "git branch -d", "impl/develop/leftover", "do not call route.resolve_implement again"} {
		if !strings.Contains(route, want) {
			t.Fatalf("recreate route instruction missing %q: %q", want, route)
		}
	}
	// recreate is a non-stop action: the worker proceeds to edits.
	edit := requireInstruction(t, todoByKey(t, verdict, "edit"))
	if strings.Contains(edit, "Do not start source edits") {
		t.Fatalf("recreate must proceed to edits, not block them: %q", edit)
	}
}

func TestDeriveImplementTodoInstructionsBranchStop(t *testing.T) {
	got := deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  "delegated",
		BranchPlan:  implementBranchPlan{Action: "stop", Reason: "merge target required while already on an implementation branch"},
		ReviewAlloc: "partitioned: correctness, fit, test",
		NeedReview:  true,
	})
	for _, key := range []string{"route", "prep", "edit", "review", "final-action-gate", "merge"} {
		instruction := requireInstruction(t, todoByKey(t, got, key))
		if !strings.Contains(instruction, "merge target required") {
			t.Fatalf("%s stop instruction did not include blocker: %q", key, instruction)
		}
		for _, forbidden := range []string{"Dispatch the delegated implementer", "Apply the source edits", "path.generate", "plan-populator-survey", "Verify source, tests"} {
			if strings.Contains(instruction, forbidden) {
				t.Fatalf("%s stop instruction implies unreachable work via %q: %q", key, forbidden, instruction)
			}
		}
	}
}

// TestDeriveImplementTodoInstructionsCriticalStop pins the Critical terminal at
// the generation source (independent of allocation shape, since the clause is
// shared): an unresolved Critical after round two is a stop reported to the
// lead, matching the shipped ticket-worker stop (e). The superseded bounded
// multi-round Critical branch and its ceiling elevation must be gone.
func TestDeriveImplementTodoInstructionsCriticalStop(t *testing.T) {
	got := deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  "delegated",
		BranchPlan:  implementBranchPlan{Action: "continue", CurrentBranch: "implement/demo"},
		ReviewAlloc: "single",
		NeedReview:  true,
	})
	review := requireInstruction(t, todoByKey(t, got, "review"))
	for _, want := range []string{
		"Review is two rounds, never more",
		"round two uses a fresh reviewer render that checks only whether round one's findings were fixed",
		"A Critical finding still open after round two is a stop",
		"report it to the lead rather than elevating or re-reviewing it",
	} {
		if !strings.Contains(review, want) {
			t.Fatalf("Critical-stop review instruction missing %q: %q", want, review)
		}
	}
	// The two-round stop replaces the bounded multi-round Critical branch and
	// its ceiling elevation outright: no relay delegate, no third round, and no
	// elevation to a heavier worker.
	for _, forbidden := range implementReviewRoundForbidden() {
		if strings.Contains(review, forbidden) {
			t.Fatalf("Critical-stop review instruction retained superseded relay/elevation wording %q: %q", forbidden, review)
		}
	}
}

// TestDeriveImplementTodoInstructionsLiteReview pins the lite allocation's
// installed review todo: its own title and wrapper, the shared fix-ownership
// clause, the one-pass rule with no re-review, the unfixable-Critical stop, and
// the unfixed-Important unresolved line — and none of the two-round protocol,
// which an unknown allocation would otherwise fall into.
func TestDeriveImplementTodoInstructionsLiteReview(t *testing.T) {
	got := deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  "delegated",
		BranchPlan:  implementBranchPlan{Action: "continue", CurrentBranch: "implement/demo"},
		ReviewAlloc: "lite",
		NeedReview:  true,
	})
	item := todoByKey(t, got, "review")
	if item.Title != "Review (lite)" {
		t.Fatalf("lite review title = %q, want %q", item.Title, "Review (lite)")
	}
	review := requireInstruction(t, item)
	for _, want := range []string{
		"Render `code-review-lite` and dispatch one fresh reviewer covering correctness and test integrity",
		"Fix the findings yourself by severity; there is no relay delegate",
		"Lite review is one pass with no re-review",
		"fix the Critical and Important findings in that pass and dispatch no second reviewer",
		"A Critical finding you cannot fix is a stop — report it to the lead",
		"an Important finding you leave unfixed goes under unresolved with the reason",
	} {
		if !strings.Contains(review, want) {
			t.Fatalf("lite review instruction missing %q: %q", want, review)
		}
	}
	for _, forbidden := range append([]string{"two rounds", "round two", "Render `reviewer`", "code-review-correctness", "code-review-fit", "code-review-test"}, implementReviewRoundForbidden()...) {
		if strings.Contains(review, forbidden) {
			t.Fatalf("lite review instruction carries %q: %q", forbidden, review)
		}
	}
}
