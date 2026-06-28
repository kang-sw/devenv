package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

const implementPrepGuardrails = `Before edits or dispatch, run mental-model lookup, read returned docs ancestors first, read the 260605 migration anchor when target touches plugin architecture, host-neutral migration, spawn-removal, or adapter boundaries, and read infra.read("impl-playbook"). `

// keysOf extracts the ordered key sequence of a todo list for assertions.
func keysOf(list []todoItem) []string {
	out := make([]string, len(list))
	for i, item := range list {
		out[i] = item.Key
	}
	return out
}

func eqKeys(a []string, b ...string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stringPtr(s string) *string {
	return &s
}

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

func TestDeriveImplementTodos(t *testing.T) {
	cases := []struct {
		needReview, needDoc bool
		want                []string
	}{
		{false, false, []string{"route", "prep", "edit", "final-action-gate", "merge"}},
		{true, false, []string{"route", "prep", "edit", "review", "final-action-gate", "merge"}},
		{false, true, []string{"route", "prep", "edit", "doc-pre-pass", "doc-commit-gate", "doc-closeout", "final-action-gate", "merge"}},
		{true, true, []string{"route", "prep", "edit", "review", "doc-pre-pass", "doc-commit-gate", "doc-closeout", "final-action-gate", "merge"}},
	}
	for _, tc := range cases {
		got := deriveImplementTodos(tc.needReview, tc.needDoc)
		if !eqKeys(keysOf(got), tc.want...) {
			t.Fatalf("derive(review=%v doc=%v) = %v, want %v", tc.needReview, tc.needDoc, keysOf(got), tc.want)
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
		Delegation:  "direct-edit",
		PlanDepth:   "none",
		ReviewAlloc: "single",
		NeedReview:  true,
	})
	wantTitles := map[string]string{
		"prep":   "Prep",
		"edit":   "Edit (direct)",
		"review": "Review (single)",
	}
	for _, item := range got {
		if want, ok := wantTitles[item.Key]; ok && item.Title != want {
			t.Fatalf("%s title = %q, want %q", item.Key, item.Title, want)
		}
	}
}

func TestDeriveImplementTodoInstructionsDirectEditLeadOnly(t *testing.T) {
	got := deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  "direct-edit",
		BranchPlan:  implementBranchPlan{Action: "continue", CurrentBranch: "implement/tiny-edit"},
		PlanDepth:   "none",
		ReviewAlloc: "lead-only",
		NeedReview:  false,
		DocMode:     "skipped",
		DocReason:   "docs not touched",
		NeedDoc:     false,
	})
	if !eqKeys(keysOf(got), "route", "prep", "edit", "review", "final-action-gate", "merge") {
		t.Fatalf("lead-only review todo shape = %v", keysOf(got))
	}
	edit := requireInstruction(t, todoByKey(t, got, "edit"))
	if !strings.Contains(edit, "Apply the source edits directly in this lead context") {
		t.Fatalf("edit instruction missing direct-edit guidance: %q", edit)
	}
	if strings.Contains(edit, "delegated implementer") {
		t.Fatalf("direct-edit instruction mentioned delegated dispatch: %q", edit)
	}
	review := requireInstruction(t, todoByKey(t, got, "review"))
	if review != "Perform lead-owned review only; record why external reviewers are unnecessary for this verdict, then preserve the rationale for the final report." {
		t.Fatalf("lead-only review instruction = %q", review)
	}
}

func TestDeriveImplementTodoInstructionsDelegatedSurvey(t *testing.T) {
	got := deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  "delegated",
		BranchPlan:  implementBranchPlan{Action: "create", CurrentBranch: "feature/base", TargetBranch: "implement/demo", MergeTarget: "feature/base"},
		PlanDepth:   "survey",
		ReviewAlloc: "partitioned: correctness, fit, test",
		NeedReview:  true,
		DocMode:     "standard",
		NeedDoc:     true,
	})
	prep := requireInstruction(t, todoByKey(t, got, "prep"))
	if prep != implementPrepGuardrails+"Call ws.path.generate(kind: \"plan\", stems: [target stem or scope]) to create the plan path, render plan-populator-survey with ticket_path, selected_phase, and plan_path, and dispatch it to write the light implementation plan. If survey returns [escalate-to-research] for low confidence or strategic uncertainty, render plan-populator-research with the same plan path before implementer dispatch. Do not create a separate brief." {
		t.Fatalf("prep instruction = %q", prep)
	}
	edit := requireInstruction(t, todoByKey(t, got, "edit"))
	if edit != "After the survey plan is ready and any [escalate-to-research] signal is resolved on the same plan path, render implementer with PlanPath and dispatch the delegated implementer; capture the implemented commit range for review and relays." {
		t.Fatalf("edit instruction = %q", edit)
	}
}

func TestDeriveImplementTodoInstructionsPrepGuardrails(t *testing.T) {
	for _, tc := range []struct {
		name     string
		depth    string
		wantTail string
	}{
		{
			name:     "none",
			depth:    "none",
			wantTail: "Confirm the direct-edit facts are still accurate",
		},
		{
			name:     "survey",
			depth:    "survey",
			wantTail: "render plan-populator-survey with ticket_path, selected_phase, and plan_path",
		},
		{
			name:     "research",
			depth:    "research",
			wantTail: "Render plan-populator-research with ticket_path, selected_phase, and an existing plan_path",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveImplementTodosFromVerdict(implementTodoVerdict{
				Delegation:  "delegated",
				BranchPlan:  implementBranchPlan{Action: "continue", CurrentBranch: "implement/demo"},
				PlanDepth:   tc.depth,
				ReviewAlloc: "lead-only",
				DocMode:     "skipped",
			})
			prep := requireInstruction(t, todoByKey(t, got, "prep"))
			for _, want := range []string{"mental-model lookup", "260605 migration anchor", `infra.read("impl-playbook")`, tc.wantTail} {
				if !strings.Contains(prep, want) {
					t.Fatalf("prep instruction for %s missing %q: %q", tc.name, want, prep)
				}
			}
		})
	}
}

func TestDeriveImplementTodoInstructionsPartitionedReview(t *testing.T) {
	got := deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  "delegated",
		BranchPlan:  implementBranchPlan{Action: "continue", CurrentBranch: "implement/demo"},
		PlanDepth:   "survey",
		ReviewAlloc: "partitioned: correctness, test",
		NeedReview:  true,
		DocMode:     "standard",
		NeedDoc:     false,
	})
	review := requireInstruction(t, todoByKey(t, got, "review"))
	if !strings.Contains(review, "Dispatch correctness and test reviewers") {
		t.Fatalf("review instruction missing selected partitions: %q", review)
	}
	if !strings.Contains(review, "Reviewer prompt frame") || !strings.Contains(review, "Review relay and Re-review prompts") {
		t.Fatalf("review instruction missing named template guidance: %q", review)
	}
	if strings.Contains(review, "fit") {
		t.Fatalf("review instruction mentioned unselected fit partition: %q", review)
	}
}

func TestDeriveImplementTodoInstructionsDocs(t *testing.T) {
	standard := deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  "delegated",
		BranchPlan:  implementBranchPlan{Action: "continue", CurrentBranch: "implement/demo"},
		PlanDepth:   "survey",
		ReviewAlloc: "single",
		NeedReview:  true,
		DocMode:     "standard",
		NeedDoc:     true,
	})
	for key, want := range map[string]string{
		"doc-pre-pass":    "mental-model-updater",
		"doc-commit-gate": "executor-wrapup",
		"doc-closeout":    "documentation-only branch-tip suffix",
	} {
		instruction := requireInstruction(t, todoByKey(t, standard, key))
		if !strings.Contains(instruction, want) {
			t.Fatalf("%s instruction = %q, want containing %q", key, instruction, want)
		}
	}

	skipped := deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  "delegated",
		BranchPlan:  implementBranchPlan{Action: "continue", CurrentBranch: "implement/demo"},
		PlanDepth:   "survey",
		ReviewAlloc: "single",
		NeedReview:  true,
		DocMode:     "skipped",
		DocReason:   "documentation tracked in follow-up",
		NeedDoc:     false,
	})
	for _, key := range []string{"doc-pre-pass", "doc-commit-gate", "doc-closeout"} {
		if idx := indexOfTodo(skipped, key); idx >= 0 {
			t.Fatalf("skipped doc mode should omit %s: %v", key, keysOf(skipped))
		}
	}
	final := requireInstruction(t, todoByKey(t, skipped, "final-action-gate"))
	if !strings.Contains(final, "documentation tracked in follow-up") {
		t.Fatalf("skipped doc reason not carried to final gate: %q", final)
	}
}

func TestDeriveImplementTodoInstructionsBranchStop(t *testing.T) {
	got := deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  "delegated",
		BranchPlan:  implementBranchPlan{Action: "stop", Reason: "merge target required while already on an implementation branch"},
		PlanDepth:   "survey",
		ReviewAlloc: "partitioned: correctness, fit, test",
		NeedReview:  true,
		DocMode:     "standard",
		NeedDoc:     true,
	})
	for _, key := range []string{"route", "prep", "edit", "review", "doc-pre-pass", "doc-commit-gate", "doc-closeout", "final-action-gate", "merge"} {
		instruction := requireInstruction(t, todoByKey(t, got, key))
		if !strings.Contains(instruction, "merge target required") {
			t.Fatalf("%s stop instruction did not include blocker: %q", key, instruction)
		}
		for _, forbidden := range []string{"Dispatch the delegated implementer", "Apply the source edits", "ws.path.generate", "plan-populator-survey", "Verify source, tests"} {
			if strings.Contains(instruction, forbidden) {
				t.Fatalf("%s stop instruction implies unreachable work via %q: %q", key, forbidden, instruction)
			}
		}
	}
}

func TestDeriveOtherEnterTodos(t *testing.T) {
	if !eqKeys(keysOf(deriveProceedTodos()), "route-context", "resolve-verdict") {
		t.Fatalf("proceed derivation mismatch: %v", keysOf(deriveProceedTodos()))
	}
	if !eqKeys(keysOf(deriveSprintTodos()), "edit", "verify", "commit", "post-edit", "wrap") {
		t.Fatalf("sprint derivation mismatch: %v", keysOf(deriveSprintTodos()))
	}
	if !eqKeys(keysOf(deriveSalvageTodos()), "containment", "survey-fanout", "premise-interview", "classification", "capture") {
		t.Fatalf("salvage derivation mismatch: %v", keysOf(deriveSalvageTodos()))
	}
}

func TestTodoKeyUniquenessAndReuse(t *testing.T) {
	list, err := todoAppend(nil, "A", "A", todoPending, nil)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Key != "a" {
		t.Fatalf("key was not normalized: %q", list[0].Key)
	}
	if _, err := todoAppend(list, "a", "dup", todoPending, nil); err == nil {
		t.Fatal("expected duplicate key error")
	}
	// erase then re-append the same key must succeed (keys are reusable).
	list, err = todoErase(list, "a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := todoAppend(list, "a", "A again", todoPending, nil); err != nil {
		t.Fatalf("re-append after erase failed: %v", err)
	}
}

func TestTodoKeyValidation(t *testing.T) {
	valid := []string{"abc", "abc-123", "abc.def", "abc_def", "ABC"}
	for _, key := range valid {
		if _, err := normalizeTodoKey(key); err != nil {
			t.Fatalf("normalizeTodoKey(%q) errored: %v", key, err)
		}
	}
	invalid := []string{"", " ", " a", "a ", "a b", "{abc}", "abc/def", "abc:def"}
	for _, key := range invalid {
		if _, err := normalizeTodoKey(key); err == nil {
			t.Fatalf("normalizeTodoKey(%q) succeeded, want error", key)
		}
	}
}

func TestTodoInsertAndCheck(t *testing.T) {
	list, _ := todoAppend(nil, "a", "A", todoPending, nil)
	list, _ = todoAppend(list, "c", "C", todoPending, nil)
	list, err := todoInsert(list, "c", "b", "B", todoPending, nil, false) // before c
	if err != nil {
		t.Fatal(err)
	}
	if !eqKeys(keysOf(list), "a", "b", "c") {
		t.Fatalf("insert_before mismatch: %v", keysOf(list))
	}
	list, err = todoInsert(list, "a", "a2", "A2", todoPending, nil, true) // after a
	if err != nil {
		t.Fatal(err)
	}
	if !eqKeys(keysOf(list), "a", "a2", "b", "c") {
		t.Fatalf("insert_after mismatch: %v", keysOf(list))
	}
	if _, err := todoInsert(list, "missing", "x", "X", todoPending, nil, true); err == nil {
		t.Fatal("expected ref_key-not-found error")
	}
	list, err = todoCheck(list, "b", todoDone)
	if err != nil {
		t.Fatal(err)
	}
	if list[indexOfTodo(list, "b")].Status != todoDone {
		t.Fatal("check did not update status")
	}
	if _, err := todoCheck(list, "missing", todoWip); err == nil {
		t.Fatal("expected check-not-found error")
	}
}

func TestTodoInstructionPreservedThroughStatusAndOrderMutations(t *testing.T) {
	list, _ := todoAppend(nil, "a", "A", todoPending, stringPtr("Alpha instruction"))
	list, _ = todoAppend(list, "c", "C", todoPending, stringPtr("Charlie instruction"))
	list, err := todoInsert(list, "c", "b", "B", todoPending, stringPtr("Bravo instruction"), false)
	if err != nil {
		t.Fatal(err)
	}

	list, err = todoCheck(list, "b", todoDone)
	if err != nil {
		t.Fatal(err)
	}
	if got := list[indexOfTodo(list, "b")].Instruction; got == nil || *got != "Bravo instruction" {
		t.Fatalf("check did not preserve instruction: %#v", got)
	}

	list, err = todoReorder(list, "b", "c", "a", false)
	if err != nil {
		t.Fatal(err)
	}
	if !eqKeys(keysOf(list), "b", "c", "a") {
		t.Fatalf("reorder keys = %v", keysOf(list))
	}
	for _, item := range list {
		if item.Instruction == nil || *item.Instruction == "" {
			t.Fatalf("reorder lost instruction for %s: %#v", item.Key, item.Instruction)
		}
	}

	list = todoClear(list, true)
	if !eqKeys(keysOf(list), "c", "a") {
		t.Fatalf("clear(done_only) keys = %v", keysOf(list))
	}
	if got := list[indexOfTodo(list, "a")].Instruction; got == nil || *got != "Alpha instruction" {
		t.Fatalf("clear(done_only) did not preserve untouched instruction: %#v", got)
	}
}

func TestTodoReadOldRecordWithoutInstruction(t *testing.T) {
	item, err := todoRead([]todoItem{{Key: "old", Title: "Old", Status: todoPending}}, "old")
	if err != nil {
		t.Fatal(err)
	}
	if item.Key != "old" || item.Title != "Old" || item.Status != todoPending {
		t.Fatalf("unexpected old-record payload: %+v", item)
	}
	if item.Instruction != nil {
		t.Fatalf("old record instruction = %#v, want nil", item.Instruction)
	}
	if _, err := todoRead([]todoItem{{Key: "old", Title: "Old", Status: todoPending}}, "missing"); err == nil {
		t.Fatal("expected missing-key read error")
	}
}

func TestTodoClear(t *testing.T) {
	list, _ := todoAppend(nil, "a", "A", todoDone, nil)
	list, _ = todoAppend(list, "b", "B", todoPending, nil)
	list, _ = todoAppend(list, "c", "C", todoDone, nil)
	list, _ = todoAppend(list, "d", "D", todoWip, nil)
	if doneCleared := todoClear(list, true); !eqKeys(keysOf(doneCleared), "b", "d") {
		t.Fatalf("clear(done_only) mismatch: %v", keysOf(doneCleared))
	}
	if all := todoClear(list, false); len(all) != 0 {
		t.Fatalf("clear(all) left %d items", len(all))
	}
}

func TestTodoReorder(t *testing.T) {
	build := func() []todoItem {
		var l []todoItem
		for _, k := range []string{"a", "b", "c", "d", "e"} {
			l, _ = todoAppend(l, k, strings.ToUpper(k), todoPending, nil)
		}
		return l
	}
	// move span [b,c] after e -> a,d,e,b,c
	got, err := todoReorder(build(), "b", "c", "e", true)
	if err != nil {
		t.Fatal(err)
	}
	if !eqKeys(keysOf(got), "a", "d", "e", "b", "c") {
		t.Fatalf("reorder after mismatch: %v", keysOf(got))
	}
	// move single span [d] before a -> d,a,b,c,e
	got, err = todoReorder(build(), "d", "d", "a", false)
	if err != nil {
		t.Fatal(err)
	}
	if !eqKeys(keysOf(got), "d", "a", "b", "c", "e") {
		t.Fatalf("reorder before mismatch: %v", keysOf(got))
	}
	// ref inside span -> error
	if _, err := todoReorder(build(), "b", "d", "c", true); err == nil {
		t.Fatal("expected ref-inside-span error")
	}
	// from after to -> error
	if _, err := todoReorder(build(), "c", "b", "e", true); err == nil {
		t.Fatal("expected from>to error")
	}
	// unknown ref -> error
	if _, err := todoReorder(build(), "a", "b", "zz", true); err == nil {
		t.Fatal("expected unknown-ref error")
	}
}

func TestRenderTodosSummaryAndFull(t *testing.T) {
	longInstruction := "012345678901234567890123456789012345678901234567890123456789EXTRA"
	wantPreview := "012345678901234567890123456789012345678901234567890123456789"
	list := []todoItem{
		{Key: "a", Title: "A", Status: todoDone},
		{Key: "b", Title: "B", Status: todoDone},
		{Key: "c", Title: "C", Status: todoPending, Instruction: &longInstruction},
		{Key: "d", Title: "D", Status: todoWip},
		{Key: "e", Title: "E", Status: todoDone},
		{Key: "f", Title: "F", Status: todoDefer},
		{Key: "g", Title: "G", Status: todoDone},
	}
	// Summary: active block c,d. one context each side: b and e shown. a, f, g
	// not shown; f+g collapse to a single trailing "..." (defer collapses like done).
	wantSummary := strings.Join([]string{
		"...",
		"- [x] {b} B",
		"- [ ] {c} C",
		"      " + wantPreview,
		"- [~] {d} D",
		"- [x] {e} E",
		"...",
	}, "\n")
	if got := renderTodos(list, false); got != wantSummary {
		t.Fatalf("summary render mismatch:\n got:\n%s\nwant:\n%s", got, wantSummary)
	}
	if got := renderTodos(list, false); strings.Contains(got, "EXTRA") {
		t.Fatalf("summary render included instruction tail:\n%s", got)
	}
	wantFull := strings.Join([]string{
		"- [x] {a} A", "- [x] {b} B", "- [ ] {c} C", "      " + longInstruction, "- [~] {d} D", "- [x] {e} E", "- [>] {f} F", "- [x] {g} G",
	}, "\n")
	if got := renderTodos(list, true); got != wantFull {
		t.Fatalf("full render mismatch:\n got:\n%s\nwant:\n%s", got, wantFull)
	}
	if got := renderTodos(nil, false); got != "(no todos)" {
		t.Fatalf("empty render = %q", got)
	}
}

func TestRenderTodosEmptyInstructionsOmitted(t *testing.T) {
	empty := ""
	list := []todoItem{
		{Key: "nil", Title: "Nil", Status: todoPending},
		{Key: "empty", Title: "Empty", Status: todoPending, Instruction: &empty},
	}
	for _, full := range []bool{false, true} {
		got := renderTodos(list, full)
		if strings.Contains(got, "\n      ") {
			t.Fatalf("renderTodos(full=%v) rendered an instruction line for nil/empty instructions:\n%s", full, got)
		}
	}
}

func TestRenderTodosCheckpointAdjacentActionableInstructions(t *testing.T) {
	list := []todoItem{
		{Key: "a", Title: "A", Status: todoDone, Instruction: stringPtr("Alpha full instruction")},
		{Key: "b", Title: "B", Status: todoPending, Instruction: stringPtr("Bravo full instruction")},
		{Key: "c", Title: "C", Status: todoDone, Instruction: stringPtr("Charlie checked instruction")},
		{Key: "d", Title: "D", Status: todoWip, Instruction: stringPtr("Delta full instruction")},
		{Key: "e", Title: "E", Status: todoPending, Instruction: stringPtr("Echo non-adjacent instruction")},
	}
	want := strings.Join([]string{
		"- [x] {a} A",
		"      ...+",
		"- [ ] {b} B",
		"      Bravo full instruction",
		"- [x] {c} C",
		"      ...+",
		"- [~] {d} D",
		"      Delta full instruction",
		"- [ ] {e} E",
		"      ...+",
	}, "\n")
	if got := renderTodosCheckpoint(list, "c"); got != want {
		t.Fatalf("checkpoint render mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
	if got := renderTodosCheckpoint(list, "c"); strings.Contains(got, "\n...") {
		t.Fatalf("checkpoint render collapsed ordered context:\n%s", got)
	}
	if got := renderTodosCheckpoint(list, "c"); strings.Contains(got, "Charlie checked instruction") || strings.Contains(got, "Echo non-adjacent instruction") {
		t.Fatalf("checkpoint render expanded checked or non-adjacent instructions:\n%s", got)
	}
}

func TestRenderTodosCheckpointFirstAndLastAdjacency(t *testing.T) {
	list := []todoItem{
		{Key: "a", Title: "A", Status: todoDone, Instruction: stringPtr("Alpha checked instruction")},
		{Key: "b", Title: "B", Status: todoPending, Instruction: stringPtr("Bravo next instruction")},
		{Key: "c", Title: "C", Status: todoPending, Instruction: stringPtr("Charlie non-adjacent instruction")},
	}
	wantFirst := strings.Join([]string{
		"- [x] {a} A",
		"      ...+",
		"- [ ] {b} B",
		"      Bravo next instruction",
		"- [ ] {c} C",
		"      ...+",
	}, "\n")
	if got := renderTodosCheckpoint(list, "a"); got != wantFirst {
		t.Fatalf("first checkpoint render mismatch:\n got:\n%s\nwant:\n%s", got, wantFirst)
	}

	wantLast := strings.Join([]string{
		"- [x] {a} A",
		"      ...+",
		"- [ ] {b} B",
		"      Bravo next instruction",
		"- [ ] {c} C",
		"      ...+",
	}, "\n")
	if got := renderTodosCheckpoint(list, "c"); got != wantLast {
		t.Fatalf("last checkpoint render mismatch:\n got:\n%s\nwant:\n%s", got, wantLast)
	}
}

func TestRenderTodosCheckpointKeepsDoneDeferAndInstructionlessAdjacentCompact(t *testing.T) {
	empty := ""
	list := []todoItem{
		{Key: "done", Title: "Done", Status: todoDone, Instruction: stringPtr("Done instruction")},
		{Key: "target", Title: "Target", Status: todoDone},
		{Key: "defer", Title: "Defer", Status: todoDefer, Instruction: stringPtr("Defer instruction")},
	}
	want := strings.Join([]string{
		"- [x] {done} Done",
		"      ...+",
		"- [x] {target} Target",
		"- [>] {defer} Defer",
		"      ...+",
	}, "\n")
	if got := renderTodosCheckpoint(list, "target"); got != want {
		t.Fatalf("done/defer checkpoint render mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}

	instructionless := []todoItem{
		{Key: "nil", Title: "Nil", Status: todoPending},
		{Key: "target", Title: "Target", Status: todoDone},
		{Key: "empty", Title: "Empty", Status: todoWip, Instruction: &empty},
	}
	if got := renderTodosCheckpoint(instructionless, "target"); strings.Contains(got, "\n      ") {
		t.Fatalf("checkpoint rendered nil/empty adjacent instruction:\n%s", got)
	}
}

func TestParseTodoStatus(t *testing.T) {
	for _, ok := range []string{"pending", "wip", "done", "defer", ""} {
		if _, err := parseTodoStatus(ok); err != nil {
			t.Fatalf("parseTodoStatus(%q) errored: %v", ok, err)
		}
	}
	if _, err := parseTodoStatus("bogus"); err == nil {
		t.Fatal("expected invalid-status error")
	}
}

// --- store-level tests (sandboxed cache) -------------------------------------

func newSandboxStore(t *testing.T) (*sessionStore, string) {
	t.Helper()
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	store := newSessionStore()
	key, err := store.mint(t.TempDir(), roleLead, "")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	return store, key
}

func TestStoreConcurrentTodoWrites(t *testing.T) {
	store, key := newSandboxStore(t)
	const n = 25
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			itemKey := fmt.Sprintf("k%02d", i)
			if err := store.mutateTodos(key, func(list []todoItem) ([]todoItem, error) {
				return todoAppend(list, itemKey, itemKey, todoPending, nil)
			}); err != nil {
				t.Errorf("append %s: %v", itemKey, err)
			}
		}(i)
	}
	wg.Wait()

	record, ok := store.readState(key)
	if !ok {
		t.Fatal("readState after concurrent writes failed")
	}
	if len(record.Todos) != n {
		t.Fatalf("expected %d todos after concurrent writes, got %d", n, len(record.Todos))
	}
	// The on-disk file must remain a complete, parseable record.
	dir, _ := store.keysDir()
	if _, parseOK := store.readRecord(dir, key); !parseOK {
		t.Fatal("on-disk record not parseable after concurrent writes")
	}
}

func TestStoreAgendaRoundTrip(t *testing.T) {
	store, key := newSandboxStore(t)
	blob := json.RawMessage(`{"mode":"implement","delegation":"delegated","need_review":true}`)
	if err := store.setAgenda(key, "implement", blob); err != nil {
		t.Fatal(err)
	}
	record, ok := store.readState(key)
	if !ok || record.Agenda == nil {
		t.Fatal("agenda not stored")
	}
	var got map[string]any
	if err := json.Unmarshal(record.Agenda["implement"], &got); err != nil {
		t.Fatalf("agenda blob not round-tripped: %v", err)
	}
	if got["delegation"] != "delegated" {
		t.Fatalf("agenda payload mismatch: %v", got)
	}
	// clear removes the blob and nils the map when empty.
	if err := store.clearAgenda(key, "implement"); err != nil {
		t.Fatal(err)
	}
	record, _ = store.readState(key)
	if len(record.Agenda) != 0 {
		t.Fatalf("agenda not cleared: %v", record.Agenda)
	}
}

func TestEnterModeReplacesTodos(t *testing.T) {
	store, key := newSandboxStore(t)
	// seed a prior list
	_ = store.mutateTodos(key, func(list []todoItem) ([]todoItem, error) {
		return todoAppend(list, "stale", "stale", todoPending, nil)
	})
	if err := store.enterMode(key, "implement", json.RawMessage(`{"x":1}`), deriveImplementTodos(true, false)); err != nil {
		t.Fatal(err)
	}
	record, _ := store.readState(key)
	if !eqKeys(keysOf(record.Todos), "route", "prep", "edit", "review", "final-action-gate", "merge") {
		t.Fatalf("enter did not replace todo list: %v", keysOf(record.Todos))
	}
	if record.Agenda["implement"] == nil {
		t.Fatal("enter did not store agenda blob")
	}
}

func TestResolveProceedRoutes(t *testing.T) {
	cases := []struct {
		name       string
		args       map[string]any
		wantRoute  string
		wantNext   string
		wantReason string
		wantCond   string
	}{
		{
			name:       "ready ticket routes to implement",
			args:       proceedReadyArgs("text"),
			wantRoute:  "implementation-dispatch.ready-actionable",
			wantNext:   "lead-implement",
			wantReason: "status=ready",
			wantCond:   "scope-blocked=none",
		},
		{
			name: "inline direct implementation",
			args: proceedArgs("inline", "inline cleanup", nil, map[string]any{
				"ticket": map[string]any{"actionable": "yes"},
				"gates":  map[string]any{"needs_ticket": "no", "scope_blocked": "none", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "whole target"},
			}),
			wantRoute:  "implementation-dispatch.inline-direct",
			wantNext:   "lead-implement",
			wantReason: "needs-ticket=no",
			wantCond:   "status=n/a",
		},
		{
			name: "idea ticket routes to ticket writing",
			args: proceedArgs("ticket-path", "idea ticket", nil, map[string]any{
				"ticket": map[string]any{"status": "idea", "category": "other", "freshness": "current"},
				"gates":  map[string]any{"scope_blocked": "none", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "Phase 1: Demo"},
			}),
			wantRoute:  "ticket-readiness.status-refresh",
			wantNext:   "lead-write-ticket",
			wantReason: "status=idea",
			wantCond:   "status=idea",
		},
		{
			name: "todo ticket routes to ticket writing",
			args: proceedArgs("ticket-path", "todo ticket", map[string]any{"ticket_path": "ai-docs/tickets/todo/260101-feat-demo.md"}, map[string]any{
				"ticket": map[string]any{"status": "todo", "category": "other", "freshness": "current"},
				"gates":  map[string]any{"scope_blocked": "none", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "Phase 1: Demo"},
			}),
			wantRoute:  "ticket-readiness.status-refresh",
			wantNext:   "lead-write-ticket",
			wantReason: "status=todo",
			wantCond:   "status=todo",
		},
		{
			name: "done ticket stops",
			args: proceedArgs("ticket-path", "done ticket", nil, map[string]any{
				"ticket": map[string]any{"status": "done", "category": "other", "freshness": "current"},
				"gates":  map[string]any{"scope_blocked": "none", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "whole target"},
			}),
			wantRoute:  "terminal-artifact.done",
			wantNext:   "stop",
			wantReason: "status=done",
			wantCond:   "status=done",
		},
		{
			name: "dropped ticket stops",
			args: proceedArgs("ticket-path", "dropped ticket", nil, map[string]any{
				"ticket": map[string]any{"status": "dropped", "category": "other", "freshness": "current"},
				"gates":  map[string]any{"scope_blocked": "none", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "whole target"},
			}),
			wantRoute:  "terminal-artifact.dropped",
			wantNext:   "stop",
			wantReason: "status=dropped",
			wantCond:   "status=dropped",
		},
		{
			name: "missing ticket stops",
			args: proceedArgs("ticket-path", "missing ticket", nil, map[string]any{
				"ticket": map[string]any{"ticket_missing": "yes", "status": "ready", "category": "other"},
				"gates":  map[string]any{"scope_blocked": "none", "discussion_needed": "no"},
			}),
			wantRoute:  "terminal-artifact.missing-ticket",
			wantNext:   "stop",
			wantReason: "ticket-missing=yes",
			wantCond:   "ticket-missing=yes",
		},
		{
			name: "unknown status stops",
			args: proceedArgs("ticket-path", "unknown status", nil, map[string]any{
				"ticket": map[string]any{"status": "unknown", "category": "other"},
				"gates":  map[string]any{"scope_blocked": "none", "discussion_needed": "no"},
			}),
			wantRoute:  "terminal-artifact.unknown-status",
			wantNext:   "stop",
			wantReason: "status=unknown",
			wantCond:   "status=unknown",
		},
		{
			name: "container blocker preserved",
			args: proceedArgs("ticket-path", "epic", nil, map[string]any{
				"ticket": map[string]any{"status": "ready", "category": "epic", "freshness": "current"},
				"gates":  map[string]any{"scope_blocked": "none", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "Phase 1: Board"},
			}),
			wantRoute:  "container-ticket.epic",
			wantNext:   "stop",
			wantReason: "category=epic",
			wantCond:   "scope-blocked=container-ticket",
		},
		{
			name: "workset container blocker preserved",
			args: proceedArgs("ticket-path", "workset", nil, map[string]any{
				"ticket": map[string]any{"status": "ready", "category": "workset", "freshness": "current"},
				"gates":  map[string]any{"scope_blocked": "none", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "whole target"},
			}),
			wantRoute:  "container-ticket.workset",
			wantNext:   "stop",
			wantReason: "category=workset",
			wantCond:   "scope-blocked=container-ticket",
		},
		{
			name: "migration anchor missing stops",
			args: proceedArgs("ticket-path", "anchor missing", nil, map[string]any{
				"ticket": map[string]any{"status": "ready", "category": "other", "freshness": "current"},
				"gates":  map[string]any{"migration_anchor": "missing", "scope_blocked": "none", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "Phase 1: Demo"},
			}),
			wantRoute:  "anchor-discussion.migration-anchor-missing",
			wantNext:   "stop",
			wantReason: "migration-anchor=missing",
			wantCond:   "migration-anchor=missing",
		},
		{
			name: "migration anchor conflict routes to discussion",
			args: proceedArgs("ticket-path", "anchor conflict", nil, map[string]any{
				"ticket": map[string]any{"status": "ready", "category": "other", "freshness": "current"},
				"gates":  map[string]any{"migration_anchor": "conflict", "scope_blocked": "none", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "Phase 1: Demo"},
			}),
			wantRoute:  "anchor-discussion.migration-anchor-conflict",
			wantNext:   "lead-discuss",
			wantReason: "migration-anchor=conflict",
			wantCond:   "discussion-needed=yes",
		},
		{
			name: "discussion needed routes to discussion",
			args: proceedArgs("ticket-path", "needs discussion", nil, map[string]any{
				"ticket": map[string]any{"status": "ready", "category": "other", "freshness": "current"},
				"gates":  map[string]any{"migration_anchor": "n/a", "scope_blocked": "none", "discussion_needed": "yes"},
				"work":   map[string]any{"slice": "Phase 1: Demo"},
			}),
			wantRoute:  "anchor-discussion.discussion-needed",
			wantNext:   "lead-discuss",
			wantReason: "discussion-needed=yes",
			wantCond:   "discussion-needed=yes",
		},
		{
			name: "freshness refresh routes to ticket writing",
			args: proceedArgs("ticket-path", "stale ticket", nil, map[string]any{
				"ticket": map[string]any{"status": "ready", "category": "other", "freshness": "missing-settled-decisions"},
				"gates":  map[string]any{"scope_blocked": "none", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "Phase 1: Demo"},
			}),
			wantRoute:  "ticket-readiness.freshness-refresh",
			wantNext:   "lead-write-ticket",
			wantReason: "freshness=missing-settled-decisions",
			wantCond:   "freshness=missing-settled-decisions",
		},
		{
			name: "inline target needing ticket routes to ticket writing",
			args: proceedArgs("inline", "public behavior change", nil, map[string]any{
				"ticket": map[string]any{"actionable": "yes"},
				"gates":  map[string]any{"needs_ticket": "yes", "scope_blocked": "none", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "whole target"},
			}),
			wantRoute:  "ticket-readiness.inline-needs-ticket",
			wantNext:   "lead-write-ticket",
			wantReason: "needs-ticket=yes",
			wantCond:   "needs-ticket=yes",
		},
		{
			name: "multiple explicit phases blocker preserved",
			args: proceedArgs("ticket-path", "multi phase", nil, map[string]any{
				"ticket": map[string]any{"status": "ready", "category": "other", "freshness": "current"},
				"gates":  map[string]any{"scope_blocked": "multiple-explicit-phases", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "blocked"},
			}),
			wantRoute:  "scope-gate.multiple-explicit-phases",
			wantNext:   "stop",
			wantReason: "scope-blocked=multiple-explicit-phases",
			wantCond:   "scope-blocked=multiple-explicit-phases",
		},
		{
			name: "too broad blocker preserved",
			args: proceedArgs("ticket-path", "too broad", nil, map[string]any{
				"ticket": map[string]any{"status": "ready", "category": "other", "freshness": "current"},
				"gates":  map[string]any{"scope_blocked": "too-broad", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "blocked"},
			}),
			wantRoute:  "scope-gate.too-broad",
			wantNext:   "stop",
			wantReason: "scope-blocked=too-broad",
			wantCond:   "scope-blocked=too-broad",
		},
		{
			name: "no unfinished phase blocker preserved",
			args: proceedArgs("ticket-path", "complete ticket", nil, map[string]any{
				"ticket": map[string]any{"status": "ready", "category": "other", "freshness": "current"},
				"gates":  map[string]any{"scope_blocked": "no-unfinished-phase", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "blocked"},
			}),
			wantRoute:  "scope-gate.no-unfinished-phase",
			wantNext:   "stop",
			wantReason: "scope-blocked=no-unfinished-phase",
			wantCond:   "scope-blocked=no-unfinished-phase",
		},
		{
			name: "phase already complete blocker preserved",
			args: proceedArgs("ticket-path", "completed phase", nil, map[string]any{
				"ticket": map[string]any{"status": "ready", "category": "other", "freshness": "current"},
				"gates":  map[string]any{"scope_blocked": "phase-already-complete", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "blocked"},
			}),
			wantRoute:  "scope-gate.phase-already-complete",
			wantNext:   "stop",
			wantReason: "scope-blocked=phase-already-complete",
			wantCond:   "scope-blocked=phase-already-complete",
		},
		{
			name: "partial route stops conservatively",
			args: proceedArgs("inline", "partial", nil, map[string]any{
				"ticket": map[string]any{"actionable": "yes"},
			}),
			wantRoute:  "fallback.insufficient-route-facts",
			wantNext:   "stop",
			wantReason: "insufficient",
			wantCond:   "needs-ticket=unknown",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input, err := parseProceedInput(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			got := resolveProceed(input)
			if got.Route != tc.wantRoute || got.Next != tc.wantNext {
				t.Fatalf("route/next = %s/%s, want %s/%s\nraw:\n%s", got.Route, got.Next, tc.wantRoute, tc.wantNext, got.Raw)
			}
			if !strings.Contains(got.Reason, tc.wantReason) {
				t.Fatalf("reason = %q, want containing %q", got.Reason, tc.wantReason)
			}
			if !containsString(got.Conditions, tc.wantCond) {
				t.Fatalf("conditions %v do not contain %q", got.Conditions, tc.wantCond)
			}
			if got.NextInstruction == "" {
				t.Fatal("next instruction is empty")
			}
			if !strings.Contains(got.Raw, "\nNext: "+got.NextInstruction+"\n\n") {
				t.Fatalf("raw verdict missing matching Next line:\n%s", got.Raw)
			}
		})
	}
}

func TestProceedNextInstructions(t *testing.T) {
	t.Setenv("WS_MCP_NAMESPACE", "wsflow")
	cases := []struct {
		name       string
		args       map[string]any
		wantNext   string
		wantText   string
		wantNoText string
	}{
		{
			name:     "implement instruction names playbook and pre-source boundary",
			args:     proceedReadyArgs("text"),
			wantNext: "lead-implement",
			wantText: `Routing to next action: lead-implement. Call wsflow/playbook.print(name: "lead-implement"), then execute the returned playbook inline for this target and phase before inspecting source`,
		},
		{
			name: "write ticket instruction names playbook and reroute",
			args: proceedArgs("ticket-path", "todo ticket", map[string]any{"ticket_path": "ai-docs/tickets/todo/260101-feat-demo.md"}, map[string]any{
				"ticket": map[string]any{"status": "todo", "category": "other", "freshness": "current"},
				"gates":  map[string]any{"scope_blocked": "none", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "Phase 1: Demo"},
			}),
			wantNext: "lead-write-ticket",
			wantText: `Routing to next action: lead-write-ticket. Call wsflow/playbook.print(name: "lead-write-ticket"), then execute the returned playbook inline. After it returns, capture the Ticket path; if it is under ai-docs/tickets/ready/, rebuild route context and rerun wsflow/enter.proceed`,
		},
		{
			name: "discussion instruction names skill namespace",
			args: proceedArgs("ticket-path", "needs discussion", nil, map[string]any{
				"ticket": map[string]any{"status": "ready", "category": "other", "freshness": "current"},
				"gates":  map[string]any{"scope_blocked": "none", "discussion_needed": "yes"},
				"work":   map[string]any{"slice": "Phase 1: Demo"},
			}),
			wantNext: "lead-discuss",
			wantText: `Routing to next action: lead-discuss. Continue through wsflow:lead-discuss with the blocker in Reason.`,
		},
		{
			name: "stop instruction does not invoke playbooks",
			args: proceedArgs("ticket-path", "done ticket", nil, map[string]any{
				"ticket": map[string]any{"status": "done", "category": "other", "freshness": "current"},
				"gates":  map[string]any{"scope_blocked": "none", "discussion_needed": "no"},
				"work":   map[string]any{"slice": "whole target"},
			}),
			wantNext:   "stop",
			wantText:   "Routing to next action: stop. Stop. Report the blocker in Reason",
			wantNoText: "playbook.print",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input, err := parseProceedInput(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			got := resolveProceed(input)
			if got.Next != tc.wantNext {
				t.Fatalf("next = %q, want %q", got.Next, tc.wantNext)
			}
			if !strings.Contains(got.NextInstruction, tc.wantText) {
				t.Fatalf("instruction = %q, want containing %q", got.NextInstruction, tc.wantText)
			}
			if tc.wantNoText != "" && strings.Contains(got.NextInstruction, tc.wantNoText) {
				t.Fatalf("instruction = %q, should not contain %q", got.NextInstruction, tc.wantNoText)
			}
			if !strings.Contains(got.Raw, "Next: "+got.NextInstruction) {
				t.Fatalf("raw missing Next instruction:\n%s", got.Raw)
			}
		})
	}

	if got := proceedNextInstruction("status-report"); !strings.Contains(got, "Routing to next action: status-report. Stop. Report the status in Reason") {
		t.Fatalf("status-report instruction = %q", got)
	}
}

func TestProceedInputRejectsNonStringFactTypes(t *testing.T) {
	_, err := parseProceedInput(proceedArgs("ticket-path", "bad type", nil, map[string]any{
		"ticket": map[string]any{"status": 123},
	}))
	if err == nil || !strings.Contains(err.Error(), "status must be a string or null") {
		t.Fatalf("non-string fact error = %v, want status type error", err)
	}
}

func TestEnterProceedSchemaAdvertisesNullableFacts(t *testing.T) {
	useLeadProfile(t)
	server := NewServer(t.TempDir(), "test")
	properties := toolPropertiesByName(t, callToolsList(t, server), "ws.enter.proceed")
	target := objectProperties(t, properties["target"])
	assertNullableSchema(t, target["ticket_path"])
	assertNullableSchema(t, target["kind"])

	facts := objectProperties(t, properties["facts"])
	ticketFacts := objectProperties(t, facts["ticket"])
	gateFacts := objectProperties(t, facts["gates"])
	workFacts := objectProperties(t, facts["work"])
	for name, schema := range map[string]any{
		"ticket.status":          ticketFacts["status"],
		"ticket.phase":           ticketFacts["phase"],
		"gates.scope_blocked":    gateFacts["scope_blocked"],
		"gates.migration_anchor": gateFacts["migration_anchor"],
		"work.slice":             workFacts["slice"],
	} {
		t.Run(name, func(t *testing.T) {
			assertNullableSchema(t, schema)
		})
	}
}

func TestEnterImplementSchemaRequiresTargetAndAdvertisesNullableFacts(t *testing.T) {
	useLeadProfile(t)
	server := NewServer(t.TempDir(), "test")
	var listResp map[string]any
	if err := json.Unmarshal([]byte(callToolsList(t, server)), &listResp); err != nil {
		t.Fatal(err)
	}
	result, _ := listResp["result"].(map[string]any)
	listedTools, _ := result["tools"].([]any)
	var schema map[string]any
	for _, rawTool := range listedTools {
		tool, _ := rawTool.(map[string]any)
		if tool["name"] == "ws.enter.implement" {
			schema, _ = tool["inputSchema"].(map[string]any)
			break
		}
	}
	if schema == nil {
		t.Fatal("ws.enter.implement schema not found")
	}
	required, _ := schema["required"].([]any)
	if !containsAnyString(required, "target") {
		t.Fatalf("required = %#v, want target", required)
	}

	properties, _ := schema["properties"].(map[string]any)
	target := objectProperties(t, properties["target"])
	assertNullableSchema(t, target["ticket_path"])
	assertNullableSchema(t, target["kind"])

	facts := objectProperties(t, properties["facts"])
	scope := objectProperties(t, facts["scope"])
	assertNullableSchema(t, scope["span"])
	assertNullableSchema(t, scope["surface"])
	policy := objectProperties(t, properties["policy"])
	docs := objectProperties(t, policy["docs"])
	assertNullableSchema(t, docs["mode"])
}

func objectProperties(t *testing.T, raw any) map[string]any {
	t.Helper()
	obj, _ := raw.(map[string]any)
	props, _ := obj["properties"].(map[string]any)
	if props == nil {
		t.Fatalf("schema missing object properties: %#v", raw)
	}
	return props
}

func assertNullableSchema(t *testing.T, raw any) {
	t.Helper()
	schema, _ := raw.(map[string]any)
	types, _ := schema["type"].([]any)
	hasString, hasNull := false, false
	for _, typ := range types {
		switch typ {
		case "string":
			hasString = true
		case "null":
			hasNull = true
		}
	}
	if !hasString || !hasNull {
		t.Fatalf("schema type = %#v, want string+null in %#v", schema["type"], schema)
	}
	if enumValues, ok := schema["enum"].([]any); ok {
		hasNullEnum := false
		for _, value := range enumValues {
			if value == nil {
				hasNullEnum = true
				break
			}
		}
		if !hasNullEnum {
			t.Fatalf("nullable enum missing null value: %#v", enumValues)
		}
	}
}

func TestEnterProceedStoresVerdictAgendaAndTodos(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 903500, root, nil))

	_ = callToolWithKey(t, server, 1, key, "ws.todo.append", map[string]any{"key": "stale", "title": "stale"})
	text := callToolWithKey(t, server, 2, key, "ws.enter.proceed", proceedReadyArgs("text"))
	nonEmpty := nonEmptyLines(text)
	if len(nonEmpty) < 3 {
		t.Fatalf("raw verdict too short:\n%s", text)
	}
	if nonEmpty[0] != "Proceed Verdict" || nonEmpty[1] != "Route: implementation-dispatch.ready-actionable" || nonEmpty[2] != "NEXT: lead-implement" {
		t.Fatalf("unexpected first verdict lines: %v\nfull:\n%s", nonEmpty[:3], text)
	}
	if !strings.Contains(text, "Agenda:") || !strings.Contains(text, "- next_skill: lead-implement") {
		t.Fatalf("raw verdict missing clear agenda/next direction:\n%s", text)
	}

	record, ok := server.sessions.readState(key)
	if !ok {
		t.Fatal("session record not found")
	}
	if !eqKeys(keysOf(record.Todos), "route-context", "resolve-verdict") {
		t.Fatalf("enter.proceed did not replace todo list: %v", keysOf(record.Todos))
	}
	var agenda proceedAgenda
	if err := json.Unmarshal(record.Agenda["proceed"], &agenda); err != nil {
		t.Fatalf("agenda did not store proceed verdict subset: %v", err)
	}
	if agenda.NextSkill != "lead-implement" || agenda.Route != "implementation-dispatch.ready-actionable" || !containsString(agenda.Conditions, "freshness=current") {
		t.Fatalf("unexpected agenda: %+v", agenda)
	}
}

func TestEnterProceedJSONIncludesRawVerdict(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 903600, root, nil))

	raw := callToolWithKey(t, server, 1, key, "ws.enter.proceed", proceedReadyArgs("text"))
	jsonText := callToolWithKey(t, server, 2, key, "ws.enter.proceed", proceedReadyArgs("json"))
	var result proceedResult
	if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
		t.Fatalf("json verdict did not parse: %v\n%s", err, jsonText)
	}
	if result.Raw != raw {
		t.Fatalf("json raw field mismatch\njson raw:\n%s\ntext raw:\n%s", result.Raw, raw)
	}
	if !result.TodoReplaced {
		t.Fatal("json result did not report todo replacement")
	}
	if result.NextInstruction == "" || !strings.Contains(result.Raw, "Next: "+result.NextInstruction) {
		t.Fatalf("json result missing next instruction/raw line: %+v", result)
	}
}

func TestEnterProceedWarningsAndErrors(t *testing.T) {
	input, err := parseProceedInput(proceedArgs("inline", "inline", map[string]any{
		"ticket_path": "ai-docs/tickets/ready/260101-feat-demo.md",
	}, map[string]any{
		"ticket": map[string]any{"status": "ready", "actionable": "yes"},
		"gates":  map[string]any{"needs_ticket": "no", "scope_blocked": "none", "discussion_needed": "no"},
		"work":   map[string]any{"slice": "whole target"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	result := resolveProceed(input)
	if result.Next != "lead-implement" {
		t.Fatalf("contradictory inline facts should still route conservatively, got %s", result.Next)
	}
	if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, "\n"), "ignored for inline target") {
		t.Fatalf("expected inline contradiction warnings, got %v", result.Warnings)
	}

	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 903700, root, nil))
	respLine := callToolLineWithKey(t, server, 1, key, "ws.enter.proceed", proceedArgs("ticket-path", "bad enum", nil, map[string]any{
		"ticket": map[string]any{"status": "blocked"},
	}))
	if !toolIsError(t, respLine) || !strings.Contains(toolText(t, respLine), "invalid status") {
		t.Fatalf("invalid enum did not return tool error: %s", respLine)
	}
}

func proceedReadyArgs(format string) map[string]any {
	args := proceedArgs("ticket-path", "260101-feat-demo", map[string]any{
		"ticket_stem": "260101-feat-demo",
		"ticket_path": "ai-docs/tickets/ready/260101-feat-demo.md",
	}, map[string]any{
		"ticket": map[string]any{
			"ticket_missing": "no",
			"has_ticket":     "yes",
			"status":         "ready",
			"category":       "other",
			"actionable":     "yes",
			"freshness":      "current",
			"phase":          "Phase 1: Demo",
		},
		"gates": map[string]any{
			"discussion_needed": "no",
			"needs_ticket":      "n/a",
			"scope_blocked":     "none",
			"migration_anchor":  "n/a",
		},
		"work": map[string]any{
			"category": "implementation",
			"slice":    "Phase 1: Demo",
		},
	})
	args["format"] = format
	return args
}

func implementReadyArgs(format string) map[string]any {
	args := map[string]any{
		"target": map[string]any{
			"kind":        "ticket",
			"label":       "260627-feat-enter-implement-deterministic-verdict-engine",
			"ticket_stem": "260627-feat-enter-implement-deterministic-verdict-engine",
			"ticket_path": "ai-docs/tickets/ready/260627-feat-enter-implement-deterministic-verdict-engine.md",
			"scope_label": "Phase 1: MCP-owned implement strategy verdict",
			"scope_slug":  "enter-implement-deterministic-verdict-engine",
		},
		"facts": map[string]any{
			"scope": map[string]any{
				"span":                        "multi-file",
				"surface":                     "public-interface",
				"new_public_symbol":           "no",
				"new_type_contract":           "yes",
				"test_surface":                "existing",
				"explicit_delegation_request": "no",
			},
			"complexity": map[string]any{
				"change_points":    "partially-known",
				"reuse_points":     "unconfirmed",
				"strategy_shape":   "single-obvious",
				"side_effect_risk": "moderate",
				"cold_context":     "no",
			},
			"risk": map[string]any{
				"correctness":          "high",
				"fit":                  "moderate",
				"test":                 "moderate",
				"security_or_contract": "moderate",
			},
		},
		"policy": map[string]any{
			"branch": map[string]any{
				"merge_target": "feature/ferrule",
				"allow_rename": "no",
			},
			"review": map[string]any{"override": "auto"},
			"docs":   map[string]any{"mode": "standard"},
		},
	}
	args["format"] = format
	return args
}

func implementDirectSkipDocsArgs(format string) map[string]any {
	args := map[string]any{
		"target": map[string]any{
			"kind":        "inline",
			"label":       "Tiny direct edit",
			"scope_label": "whole target",
			"scope_slug":  "tiny-direct-edit",
		},
		"facts": map[string]any{
			"scope": map[string]any{
				"span":                        "single-file",
				"surface":                     "internal",
				"new_public_symbol":           "no",
				"new_type_contract":           "no",
				"test_surface":                "none",
				"explicit_delegation_request": "no",
			},
			"complexity": map[string]any{
				"change_points":    "clear",
				"reuse_points":     "not-applicable",
				"strategy_shape":   "single-obvious",
				"side_effect_risk": "low",
				"cold_context":     "no",
			},
			"risk": map[string]any{
				"correctness":          "low",
				"fit":                  "low",
				"test":                 "low",
				"security_or_contract": "low",
			},
		},
		"policy": map[string]any{
			"branch": map[string]any{
				"allow_rename": "no",
			},
			"review": map[string]any{"override": "auto"},
			"docs": map[string]any{
				"mode":   "skip-with-reason",
				"reason": "documentation not touched",
			},
		},
	}
	args["format"] = format
	return args
}

func proceedArgs(kind, label string, targetExtra, facts map[string]any) map[string]any {
	target := map[string]any{"kind": kind, "label": label}
	for k, v := range targetExtra {
		target[k] = v
	}
	args := map[string]any{"target": target}
	if facts != nil {
		args["facts"] = facts
	}
	return args
}

func callToolLineWithKey(t *testing.T, server *Server, id int, key, name string, args map[string]any) string {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	args["session_key"] = key
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params":  map[string]any{"name": name, "arguments": args},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(string(raw)+"\n"), &out); err != nil {
		t.Fatalf("ServeStdio(%s) error: %v", name, err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	return byID[fmt.Sprint(id)]
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsAnyString(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func nonEmptyLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out
}

// --- MCP integration ---------------------------------------------------------

// callToolWithKey runs a single tools/call line through its own ServeStdio,
// injecting the session key. Ordered, dependent steps must each be a separate
// ServeStdio call because ServeStdio processes stream lines concurrently.
func callToolWithKey(t *testing.T, server *Server, id int, key, name string, args map[string]any) string {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	args["session_key"] = key
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params":  map[string]any{"name": name, "arguments": args},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(string(raw)+"\n"), &out); err != nil {
		t.Fatalf("ServeStdio(%s) error: %v", name, err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	return toolText(t, byID[fmt.Sprint(id)])
}

func readTodoInstruction(t *testing.T, server *Server, id int, key, todoKey string) string {
	t.Helper()
	raw := callToolWithKey(t, server, id, key, "ws.todo.read", map[string]any{"key": todoKey})
	var payload todoReadPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("todo.read(%s) did not parse: %v\n%s", todoKey, err, raw)
	}
	if payload.Instruction == nil || strings.TrimSpace(*payload.Instruction) == "" {
		t.Fatalf("todo.read(%s) missing instruction: %+v", todoKey, payload)
	}
	return *payload.Instruction
}

func TestServeStdioSessionStateFlow(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 900100, root, nil))

	enter := callToolWithKey(t, server, 1, key, "ws.enter.implement", map[string]any{
		"delegation": "delegated", "need_review": true, "need_doc": false,
	})
	if !strings.Contains(enter, "entered implement mode") {
		t.Fatalf("enter.implement response unexpected: %s", enter)
	}

	full := callToolWithKey(t, server, 2, key, "ws.todo.list", map[string]any{"mode": "full"})
	for _, want := range []string{"Route", "Prep", "Edit", "Review", "Final action gate", "Merge"} {
		if !strings.Contains(full, want) {
			t.Fatalf("full todo list missing %q: %s", want, full)
		}
	}
	if strings.Contains(full, "Doc pre-pass") {
		t.Fatalf("need_doc=false but Doc steps present: %s", full)
	}

	if got := callToolWithKey(t, server, 3, key, "ws.todo.check", map[string]any{"key": "route", "status": "done"}); !strings.Contains(got, "todo done: route") {
		t.Fatalf("check response unexpected: %s", got)
	}

	// After checking route done, the summary renders route with the done marker as
	// adjacent context for the still-active prep step.
	summary := callToolWithKey(t, server, 4, key, "ws.todo.list", nil)
	if !strings.Contains(summary, "- [x] {route} Route") {
		t.Fatalf("summary missing checked Route context: %s", summary)
	}
}

func TestServeStdioEnterImplementVerdictLabels(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 902500, root, nil))

	enter := callToolWithKey(t, server, 1, key, "ws.enter.implement", map[string]any{
		"delegation":   "direct-edit",
		"plan_depth":   "none",
		"review_alloc": "single",
		"need_review":  true,
	})
	for _, want := range []string{
		"- [ ] {prep} Prep",
		"- [ ] {edit} Edit (direct)",
		"- [ ] {review} Review (single)",
	} {
		if !strings.Contains(enter, want) {
			t.Fatalf("enter.implement output missing %q:\n%s", want, enter)
		}
	}

	if got := callToolWithKey(t, server, 2, key, "ws.enter.implement", map[string]any{
		"plan_depth":   "brief",
		"review_alloc": "single",
		"need_review":  true,
	}); !strings.Contains(got, `invalid plan_depth "brief"`) {
		t.Fatalf("invalid plan_depth error expected, got: %s", got)
	}

	if got := callToolWithKey(t, server, 3, key, "ws.enter.implement", map[string]any{
		"delegation":   "direct-edit",
		"review_alloc": "single",
		"need_review":  true,
	}); !strings.Contains(got, "- [ ] {prep} Prep") || !strings.Contains(got, "- [ ] {edit} Edit (direct)") {
		t.Fatalf("direct-edit omitted plan_depth should default to no planner, got: %s", got)
	} else if strings.Contains(got, "Prep (survey plan)") || strings.Contains(got, "plan-populator-survey") {
		t.Fatalf("direct-edit omitted plan_depth exposed planner path: %s", got)
	}

	if got := callToolWithKey(t, server, 4, key, "ws.enter.implement", map[string]any{
		"delegation":   "delegated",
		"plan_depth":   "research",
		"review_alloc": "single",
		"need_review":  true,
	}); !strings.Contains(got, `invalid plan_depth "research" for delegated legacy enter`) || !strings.Contains(got, `[escalate-to-research]`) {
		t.Fatalf("legacy research plan_depth should be rejected with survey escalation guidance, got: %s", got)
	}

	if got := callToolWithKey(t, server, 5, key, "ws.enter.implement", map[string]any{
		"delegation":   "direct-edit",
		"plan_depth":   "research",
		"review_alloc": "single",
		"need_review":  true,
	}); !strings.Contains(got, `invalid plan_depth "research" for direct-edit`) {
		t.Fatalf("direct-edit research plan_depth should be rejected, got: %s", got)
	}

	if got := callToolWithKey(t, server, 6, key, "ws.enter.implement", map[string]any{
		"review_alloc": "single",
		"need_review":  true,
	}); !strings.Contains(got, "- [ ] {prep} Prep (survey plan)") || !strings.Contains(got, "- [ ] {edit} Edit (delegated)") {
		t.Fatalf("legacy omitted delegation/plan_depth should default to delegated survey, got: %s", got)
	}

	if got := callToolWithKey(t, server, 7, key, "ws.enter.implement", map[string]any{
		"review_alloc": "singel",
		"need_review":  true,
	}); !strings.Contains(got, `invalid review_alloc "singel"`) {
		t.Fatalf("invalid review_alloc error expected, got: %s", got)
	}
}

func TestEnterImplementNewSchemaReturnsVerdictAndStoresAgenda(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	text := callToolWithKey(t, server, 2, key, "ws.enter.implement", implementReadyArgs("text"))
	for _, want := range []string{
		"Implementation Verdict",
		"Mode: delegated",
		"Branch Action: create implement/enter-implement-deterministic-verdict-engine",
		"Plan Depth: survey",
		"Review Allocation: partitioned: correctness, fit, test",
		"Next: Create implement/enter-implement-deterministic-verdict-engine",
		"ws.path.generate(kind: \"plan\")",
		"plan-populator-survey",
		"[escalate-to-research]",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("enter.implement verdict missing %q:\n%s", want, text)
		}
	}

	jsonText := callToolWithKey(t, server, 3, key, "ws.enter.implement", implementReadyArgs("json"))
	var result implementResult
	if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
		t.Fatalf("json verdict did not parse: %v\n%s", err, jsonText)
	}
	if result.Raw == "" || !strings.Contains(result.Raw, "Implementation Verdict") {
		t.Fatalf("json result missing raw verdict: %+v", result)
	}
	if result.Verdict.BranchPlan.Action != "create" || result.Verdict.Delegation != "delegated" {
		t.Fatalf("unexpected verdict: %+v", result.Verdict)
	}

	record, ok := server.sessions.readState(key)
	if !ok {
		t.Fatal("session record not found")
	}
	var agenda implementAgenda
	if err := json.Unmarshal(record.Agenda["implement"], &agenda); err != nil {
		t.Fatalf("agenda did not store implement verdict subset: %v", err)
	}
	if agenda.BranchPlan.Action != "create" || agenda.ReviewAlloc != "partitioned: correctness, fit, test" {
		t.Fatalf("unexpected agenda: %+v", agenda)
	}
	if !eqKeys(keysOf(record.Todos), "route", "prep", "edit", "review", "doc-pre-pass", "doc-commit-gate", "doc-closeout", "final-action-gate", "merge") {
		t.Fatalf("enter.implement did not replace todo list: %v", keysOf(record.Todos))
	}
	readPrep := callToolWithKey(t, server, 4, key, "ws.todo.read", map[string]any{"key": "prep"})
	var prepPayload todoReadPayload
	if err := json.Unmarshal([]byte(readPrep), &prepPayload); err != nil {
		t.Fatalf("prep todo read did not parse: %v\n%s", err, readPrep)
	}
	if prepPayload.Instruction == nil || *prepPayload.Instruction != implementPrepGuardrails+"Call ws.path.generate(kind: \"plan\", stems: [target stem or scope]) to create the plan path, render plan-populator-survey with ticket_path, selected_phase, and plan_path, and dispatch it to write the light implementation plan. If survey returns [escalate-to-research] for low confidence or strategic uncertainty, render plan-populator-research with the same plan path before implementer dispatch. Do not create a separate brief." {
		t.Fatalf("prep instruction = %#v", prepPayload.Instruction)
	}
	full := callToolWithKey(t, server, 5, key, "ws.todo.list", map[string]any{"mode": "full"})
	for _, want := range []string{
		"- [ ] {prep} Prep (survey plan)\n      " + implementPrepGuardrails + "Call ws.path.generate(kind: \"plan\"",
		"render plan-populator-survey with ticket_path, selected_phase, and plan_path",
		"render plan-populator-research with the same plan path",
		"- [ ] {edit} Edit (delegated)\n      After the survey plan is ready",
		"render implementer with PlanPath",
	} {
		if !strings.Contains(full, want) {
			t.Fatalf("full todo list missing enter-derived instruction %q:\n%s", want, full)
		}
	}
	for _, forbidden := range []string{"Prep (brief", "implementation brief"} {
		if strings.Contains(full, forbidden) {
			t.Fatalf("full todo list retained old brief wording %q:\n%s", forbidden, full)
		}
	}
}

func TestEnterImplementFocusedTodosDirectLeadOnlySkippedDocs(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	jsonText := callToolWithKey(t, server, 2, key, "ws.enter.implement", implementDirectSkipDocsArgs("json"))
	var result implementResult
	if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
		t.Fatalf("json verdict did not parse: %v\n%s", err, jsonText)
	}
	if result.Verdict.Delegation != "direct-edit" || result.Verdict.ReviewAlloc != "lead-only" || result.Verdict.DocMode != "skipped" {
		t.Fatalf("unexpected focused verdict: %+v", result.Verdict)
	}

	edit := readTodoInstruction(t, server, 3, key, "edit")
	if !strings.Contains(edit, "Apply the source edits directly") || strings.Contains(edit, "delegated implementer") {
		t.Fatalf("direct-edit todo instruction not focused: %q", edit)
	}
	review := readTodoInstruction(t, server, 4, key, "review")
	if !strings.Contains(review, "Perform lead-owned review only") || strings.Contains(review, "Reviewer prompt frame") {
		t.Fatalf("lead-only review todo instruction not focused: %q", review)
	}
	final := readTodoInstruction(t, server, 5, key, "final-action-gate")
	if !strings.Contains(final, "documentation not touched") {
		t.Fatalf("skipped-doc reason missing from final action todo: %q", final)
	}
	full := callToolWithKey(t, server, 6, key, "ws.todo.list", map[string]any{"mode": "full"})
	for _, forbidden := range []string{"{doc-pre-pass}", "{doc-commit-gate}", "{doc-closeout}", "Dispatch the delegated implementer"} {
		if strings.Contains(full, forbidden) {
			t.Fatalf("focused direct/skipped-doc todo list contains forbidden %q:\n%s", forbidden, full)
		}
	}
}

func TestEnterImplementStopsOnImplementBranchWithoutMergeTarget(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	runGit(t, root, "switch", "-c", "implement/old-scope")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	args := implementReadyArgs("json")
	delete(args["policy"].(map[string]any)["branch"].(map[string]any), "merge_target")
	text := callToolWithKey(t, server, 2, key, "ws.enter.implement", args)
	var result implementResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("json verdict did not parse: %v\n%s", err, text)
	}
	if result.Verdict.BranchPlan.Action != "stop" {
		t.Fatalf("expected stop branch action, got %+v", result.Verdict.BranchPlan)
	}
	if !strings.Contains(result.Raw, "Branch Action: stop - merge target required") || !strings.Contains(result.NextInstruction, "Stop before source edits") {
		t.Fatalf("stop verdict missing blocker guidance:\n%s", result.Raw)
	}
	for _, forbidden := range []string{"ws.path.generate", "plan-populator-survey", "render implementer"} {
		if strings.Contains(result.NextInstruction, forbidden) {
			t.Fatalf("branch-stop next instruction includes unreachable %q: %q", forbidden, result.NextInstruction)
		}
	}
	edit := readTodoInstruction(t, server, 3, key, "edit")
	if !strings.Contains(edit, "merge target required") {
		t.Fatalf("branch-stop edit todo missing blocker: %q", edit)
	}
	for _, forbidden := range []string{"Dispatch the delegated implementer", "Apply the source edits directly", "Run the standard documentation pre-pass"} {
		if strings.Contains(edit, forbidden) {
			t.Fatalf("branch-stop edit todo implies unreachable work via %q: %q", forbidden, edit)
		}
	}
	full := callToolWithKey(t, server, 4, key, "ws.todo.list", map[string]any{"mode": "full"})
	if !strings.Contains(full, "merge target required while already on an implementation branch") || strings.Contains(full, "Dispatch the delegated implementer with Delegate dispatch") {
		t.Fatalf("branch-stop full todo list not focused:\n%s", full)
	}
}

func TestServeStdioTicketsCreateUsesResolvedSageReviewConfig(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)

	resolver := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
	if err := resolver.Set(wsconfig.ItemSageReview, "ask", wsconfig.SetOptions{}); err != nil {
		t.Fatalf("set sage_review: %v", err)
	}

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 902600, root, nil))

	resp := callToolWithKey(t, server, 1, key, "tickets.create", map[string]any{
		"stem":          "feat-sage-create",
		"initial_state": "todo",
	})
	if !strings.Contains(resp, "Created ai-docs/tickets/todo/") || !strings.Contains(resp, "recommended") {
		t.Fatalf("tickets.create response missing created path or posture: %s", resp)
	}

	matches, err := filepath.Glob(filepath.Join(root, "ai-docs", "tickets", "todo", "*-feat-sage-create.md"))
	if err != nil {
		t.Fatalf("glob created ticket: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("created ticket matches = %v, want exactly one", matches)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read created ticket: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "sage-review: recommended") {
		t.Fatalf("created ticket missing recommended posture:\n%s", body)
	}
}

func TestServeStdioTodoKeyNormalization(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 903000, root, nil))

	if got := callToolWithKey(t, server, 1, key, "ws.todo.append", map[string]any{
		"key": "Review.Step_1", "title": "Review",
	}); !strings.Contains(got, "todo appended: review.step_1") {
		t.Fatalf("append did not report normalized key: %s", got)
	}
	if got := callToolWithKey(t, server, 2, key, "ws.todo.append", map[string]any{
		"key": "review.step_1", "title": "dup",
	}); !strings.Contains(got, "already exists") {
		t.Fatalf("duplicate-after-normalization error expected, got: %s", got)
	}
	if got := callToolWithKey(t, server, 3, key, "ws.todo.check", map[string]any{
		"key": "REVIEW.STEP_1", "status": "done",
	}); !strings.Contains(got, "todo done: review.step_1") {
		t.Fatalf("check did not normalize lookup key: %s", got)
	}
	if full := callToolWithKey(t, server, 4, key, "ws.todo.list", map[string]any{"mode": "full"}); !strings.Contains(full, "- [x] {review.step_1} Review") {
		t.Fatalf("full list missing normalized rendered key: %s", full)
	}
	if got := callToolWithKey(t, server, 5, key, "ws.todo.append", map[string]any{
		"key": "{bad}", "title": "bad",
	}); !strings.Contains(got, "invalid character") {
		t.Fatalf("invalid key error expected, got: %s", got)
	}
	if got := callToolWithKey(t, server, 6, key, "ws.todo.append", map[string]any{
		"key": " Review ", "title": "bad",
	}); !strings.Contains(got, "leading or trailing whitespace") {
		t.Fatalf("whitespace key error expected, got: %s", got)
	}
}

func TestServeStdioTodoInstructionReadSurface(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 903100, root, nil))

	if got := callToolWithKey(t, server, 1, key, "ws.todo.append", map[string]any{
		"key":         "a",
		"title":       "A",
		"instruction": "Alpha full instruction",
	}); !strings.Contains(got, "todo appended: a") {
		t.Fatalf("append with instruction unexpected: %s", got)
	}
	if got := callToolWithKey(t, server, 2, key, "ws.todo.insert_before", map[string]any{
		"ref_key":     "a",
		"key":         "b",
		"title":       "B",
		"instruction": nil,
	}); !strings.Contains(got, "todo inserted: b") {
		t.Fatalf("insert_before null instruction unexpected: %s", got)
	}
	if got := callToolWithKey(t, server, 3, key, "ws.todo.insert_after", map[string]any{
		"ref_key":     "a",
		"key":         "c",
		"title":       "C",
		"instruction": "Charlie full instruction",
	}); !strings.Contains(got, "todo inserted: c") {
		t.Fatalf("insert_after with instruction unexpected: %s", got)
	}
	if got := callToolWithKey(t, server, 4, key, "ws.todo.check", map[string]any{
		"key": "a", "status": "done",
	}); !strings.Contains(got, "todo done: a") {
		t.Fatalf("check unexpected: %s", got)
	}
	if got := callToolWithKey(t, server, 5, key, "ws.todo.reorder", map[string]any{
		"span":     map[string]any{"from_key": "a", "to_key": "c"},
		"position": map[string]any{"before": "b"},
	}); !strings.Contains(got, "todo span reordered") {
		t.Fatalf("reorder unexpected: %s", got)
	}

	readA := callToolWithKey(t, server, 6, key, "ws.todo.read", map[string]any{"key": "A"})
	var payload todoReadPayload
	if err := json.Unmarshal([]byte(readA), &payload); err != nil {
		t.Fatalf("read payload did not parse: %v\n%s", err, readA)
	}
	if payload.Key != "a" || payload.Title != "A" || payload.Status != todoDone {
		t.Fatalf("unexpected read payload: %+v", payload)
	}
	if payload.Instruction == nil || *payload.Instruction != "Alpha full instruction" {
		t.Fatalf("read did not return full instruction: %+v", payload)
	}

	readB := callToolWithKey(t, server, 7, key, "ws.todo.read", map[string]any{"key": "b"})
	var nullPayload todoReadPayload
	if err := json.Unmarshal([]byte(readB), &nullPayload); err != nil {
		t.Fatalf("null read payload did not parse: %v\n%s", err, readB)
	}
	if nullPayload.Instruction != nil {
		t.Fatalf("null instruction = %#v, want nil", nullPayload.Instruction)
	}

	if got := callToolWithKey(t, server, 8, key, "ws.todo.append", map[string]any{
		"key": "bad", "title": "Bad", "instruction": []any{"not", "string"},
	}); !strings.Contains(got, "instruction must be a string or null") {
		t.Fatalf("invalid instruction error expected, got: %s", got)
	}
	if got := callToolWithKey(t, server, 9, key, "ws.todo.read", map[string]any{"key": "missing"}); !strings.Contains(got, `todo key "missing" not found`) {
		t.Fatalf("missing read error expected, got: %s", got)
	}
}

func TestServeStdioTodoListInstructionRendering(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 903150, root, nil))
	longInstruction := "Render this instruction preview through summary mode while preserving full mode details beyond sixty characters."
	wantPreview := "Render this instruction preview through summary mode while p"
	wantTail := "reserving full mode details beyond sixty characters."

	if got := callToolWithKey(t, server, 1, key, "ws.todo.append", map[string]any{
		"key":         "render",
		"title":       "Render instruction",
		"instruction": longInstruction,
	}); !strings.Contains(got, "todo appended: render") {
		t.Fatalf("append unexpected: %s", got)
	}

	summary := callToolWithKey(t, server, 2, key, "ws.todo.list", nil)
	if !strings.Contains(summary, "- [ ] {render} Render instruction\n      "+wantPreview) {
		t.Fatalf("summary list missing instruction preview:\n%s", summary)
	}
	if strings.Contains(summary, wantTail) {
		t.Fatalf("summary list rendered instruction tail, want preview only:\n%s", summary)
	}

	full := callToolWithKey(t, server, 3, key, "ws.todo.list", map[string]any{"mode": "full"})
	if !strings.Contains(full, "- [ ] {render} Render instruction\n      "+longInstruction) {
		t.Fatalf("full list missing full instruction:\n%s", full)
	}
}

func TestServeStdioTodoCheckCheckpointRendering(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 903175, root, nil))

	for i, item := range []struct {
		key, title, instruction string
	}{
		{"a", "A", "Alpha adjacent instruction"},
		{"b", "B", "Bravo checked instruction"},
		{"c", "C", "Charlie adjacent instruction"},
		{"d", "D", "Delta non-adjacent instruction"},
	} {
		if got := callToolWithKey(t, server, i+1, key, "ws.todo.append", map[string]any{
			"key": item.key, "title": item.title, "instruction": item.instruction,
		}); !strings.Contains(got, "todo appended: "+item.key) {
			t.Fatalf("append %s unexpected: %s", item.key, got)
		}
	}

	markers := map[string]string{
		string(todoPending): "- [ ]",
		string(todoWip):     "- [~]",
		string(todoDone):    "- [x]",
		string(todoDefer):   "- [>]",
	}
	for i, status := range []todoStatus{todoPending, todoWip, todoDone, todoDefer} {
		got := callToolWithKey(t, server, 10+i, key, "ws.todo.check", map[string]any{
			"key": "b", "status": string(status),
		})
		want := strings.Join([]string{
			fmt.Sprintf("todo %s: b", status),
			"- [ ] {a} A",
			"      Alpha adjacent instruction",
			fmt.Sprintf("%s {b} B", markers[string(status)]),
			"      ...+",
			"- [ ] {c} C",
			"      Charlie adjacent instruction",
			"- [ ] {d} D",
			"      ...+",
			"",
		}, "\n")
		if got != want {
			t.Fatalf("todo.check(%s) checkpoint mismatch:\n got:\n%s\nwant:\n%s", status, got, want)
		}
		if strings.Contains(got, "Bravo checked instruction") || strings.Contains(got, "Delta non-adjacent instruction") {
			t.Fatalf("checkpoint expanded checked or non-adjacent instruction:\n%s", got)
		}
	}
}

func TestTodoCheckToolSchemaHasNoFormat(t *testing.T) {
	var checkTool map[string]any
	for _, tool := range tools() {
		if tool["name"] == "ws.todo.check" {
			checkTool = tool
			break
		}
	}
	if checkTool == nil {
		t.Fatal("ws.todo.check missing from tools()")
	}
	if desc, _ := checkTool["description"].(string); !strings.Contains(desc, "checkpoint todo rendering") {
		t.Fatalf("ws.todo.check description missing checkpoint contract: %q", desc)
	}
	schema, ok := checkTool["inputSchema"].(map[string]any)
	if !ok {
		t.Fatalf("ws.todo.check schema has unexpected type: %#v", checkTool["inputSchema"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("ws.todo.check properties have unexpected type: %#v", schema["properties"])
	}
	if _, ok := properties["format"]; ok {
		t.Fatalf("ws.todo.check schema advertised forbidden format property: %#v", properties["format"])
	}
}

func TestServeStdioTodoReorderHandler(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 901000, root, nil))

	// Build a known list a,b,c,d via the append handler so the reorder handler
	// operates on real on-disk state.
	for i, k := range []string{"a", "b", "c", "d"} {
		if got := callToolWithKey(t, server, 1000+i, key, "ws.todo.append", map[string]any{
			"key": k, "title": strings.ToUpper(k),
		}); !strings.Contains(got, "todo appended: "+k) {
			t.Fatalf("append %s unexpected: %s", k, got)
		}
	}

	// position {after: ...}: move span [a,b] after d -> c,d,a,b
	if got := callToolWithKey(t, server, 1100, key, "ws.todo.reorder", map[string]any{
		"span":     map[string]any{"from_key": "a", "to_key": "b"},
		"position": map[string]any{"after": "d"},
	}); !strings.Contains(got, "todo span reordered") {
		t.Fatalf("reorder(after) unexpected: %s", got)
	}
	if rec, ok := server.sessions.readState(key); !ok || !eqKeys(keysOf(rec.Todos), "c", "d", "a", "b") {
		t.Fatalf("reorder(after) on-disk order wrong: %v", keysOf(rec.Todos))
	}

	// position {before: ...}: move span [a,b] before c -> a,b,c,d
	if got := callToolWithKey(t, server, 1101, key, "ws.todo.reorder", map[string]any{
		"span":     map[string]any{"from_key": "a", "to_key": "b"},
		"position": map[string]any{"before": "c"},
	}); !strings.Contains(got, "todo span reordered") {
		t.Fatalf("reorder(before) unexpected: %s", got)
	}
	if rec, ok := server.sessions.readState(key); !ok || !eqKeys(keysOf(rec.Todos), "a", "b", "c", "d") {
		t.Fatalf("reorder(before) on-disk order wrong: %v", keysOf(rec.Todos))
	}

	// Malformed: missing span -> compact error, list unchanged.
	if got := callToolWithKey(t, server, 1102, key, "ws.todo.reorder", map[string]any{
		"position": map[string]any{"before": "c"},
	}); !strings.Contains(got, "span {from_key, to_key} is required") {
		t.Fatalf("missing-span error expected, got: %s", got)
	}
	// Malformed: missing position -> compact error.
	if got := callToolWithKey(t, server, 1103, key, "ws.todo.reorder", map[string]any{
		"span": map[string]any{"from_key": "a", "to_key": "b"},
	}); !strings.Contains(got, "position {before|after: ref_key} is required") {
		t.Fatalf("missing-position error expected, got: %s", got)
	}
	// Malformed: position with neither before nor after -> compact error.
	if got := callToolWithKey(t, server, 1104, key, "ws.todo.reorder", map[string]any{
		"span":     map[string]any{"from_key": "a", "to_key": "b"},
		"position": map[string]any{"sideways": "c"},
	}); !strings.Contains(got, "position must set either before or after") {
		t.Fatalf("empty-position error expected, got: %s", got)
	}
	// List must still be intact after the error paths.
	if rec, ok := server.sessions.readState(key); !ok || !eqKeys(keysOf(rec.Todos), "a", "b", "c", "d") {
		t.Fatalf("list mutated by error paths: %v", keysOf(rec.Todos))
	}
}

func TestServeStdioAgendaHandler(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 902000, root, nil))

	// set: an arbitrary nested object value must JSON-marshal and round-trip.
	if got := callToolWithKey(t, server, 2000, key, "ws.agenda.set", map[string]any{
		"key":   "notes",
		"value": map[string]any{"mode": "implement", "tags": []any{"x", "y"}},
	}); !strings.Contains(got, "agenda set: notes") {
		t.Fatalf("agenda.set unexpected: %s", got)
	}
	rec, ok := server.sessions.readState(key)
	if !ok || rec.Agenda["notes"] == nil {
		t.Fatalf("agenda blob not persisted via handler")
	}
	var blob map[string]any
	if err := json.Unmarshal(rec.Agenda["notes"], &blob); err != nil {
		t.Fatalf("agenda blob not valid JSON: %v", err)
	}
	if blob["mode"] != "implement" {
		t.Fatalf("agenda value did not round-trip via handler: %v", blob)
	}

	// missing value -> compact error.
	if got := callToolWithKey(t, server, 2001, key, "ws.agenda.set", map[string]any{
		"key": "notes",
	}); !strings.Contains(got, "value is required") {
		t.Fatalf("missing-value error expected, got: %s", got)
	}

	// clear: removes the blob through the handler.
	if got := callToolWithKey(t, server, 2002, key, "ws.agenda.clear", map[string]any{
		"key": "notes",
	}); !strings.Contains(got, "agenda cleared: notes") {
		t.Fatalf("agenda.clear unexpected: %s", got)
	}
	rec, _ = server.sessions.readState(key)
	if len(rec.Agenda) != 0 {
		t.Fatalf("agenda not cleared via handler: %v", rec.Agenda)
	}
}

func TestServeStdioTodoListRequiresSessionKey(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	// Call ws.todo.list with no session_key at all: must be a compact error, not a panic.
	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ws.todo.list","arguments":{}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !strings.Contains(byID["1"], "session_key is required") {
		t.Fatalf("expected session_key-required error, got: %s", byID["1"])
	}
}

// --- Phase 3a: stripModeGatedRegion pure-logic tests (TDD) -------------------

func TestStripModeGatedRegion_NoMarker(t *testing.T) {
	body := "line one\nline two\nline three"
	// Without any marker, the body should be unchanged regardless of keepContent.
	if got := stripModeGatedRegion(body, true); got != body {
		t.Fatalf("keepContent=true mutated marker-free body:\n got: %q\nwant: %q", got, body)
	}
	if got := stripModeGatedRegion(body, false); got != body {
		t.Fatalf("keepContent=false mutated marker-free body:\n got: %q\nwant: %q", got, body)
	}
}

func TestStripModeGatedRegion_KeepContent(t *testing.T) {
	// keepContent=true: marker lines removed, inner content kept.
	body := "before\n<!-- ws:fresh-only:start -->\nbootstrap line\n<!-- ws:fresh-only:end -->\nafter"
	want := "before\nbootstrap line\nafter"
	if got := stripModeGatedRegion(body, true); got != want {
		t.Fatalf("keepContent=true:\n got: %q\nwant: %q", got, want)
	}
}

func TestStripModeGatedRegion_StripContent(t *testing.T) {
	// keepContent=false: marker lines AND inner content removed.
	body := "before\n<!-- ws:fresh-only:start -->\nbootstrap line\n<!-- ws:fresh-only:end -->\nafter"
	want := "before\nafter"
	if got := stripModeGatedRegion(body, false); got != want {
		t.Fatalf("keepContent=false:\n got: %q\nwant: %q", got, want)
	}
}

func TestStripModeGatedRegion_MultipleLines(t *testing.T) {
	// Multiple inner lines in the gated region.
	body := "before\n<!-- ws:fresh-only:start -->\nline A\nline B\nline C\n<!-- ws:fresh-only:end -->\nafter"
	wantKeep := "before\nline A\nline B\nline C\nafter"
	wantStrip := "before\nafter"
	if got := stripModeGatedRegion(body, true); got != wantKeep {
		t.Fatalf("multi-line keep:\n got: %q\nwant: %q", got, wantKeep)
	}
	if got := stripModeGatedRegion(body, false); got != wantStrip {
		t.Fatalf("multi-line strip:\n got: %q\nwant: %q", got, wantStrip)
	}
}

func TestStripModeGatedRegion_MultipleRegions(t *testing.T) {
	// Multiple gated regions in one body.
	body := "a\n<!-- ws:fresh-only:start -->\nR1\n<!-- ws:fresh-only:end -->\nb\n<!-- ws:fresh-only:start -->\nR2\n<!-- ws:fresh-only:end -->\nc"
	wantKeep := "a\nR1\nb\nR2\nc"
	wantStrip := "a\nb\nc"
	if got := stripModeGatedRegion(body, true); got != wantKeep {
		t.Fatalf("multi-region keep:\n got: %q\nwant: %q", got, wantKeep)
	}
	if got := stripModeGatedRegion(body, false); got != wantStrip {
		t.Fatalf("multi-region strip:\n got: %q\nwant: %q", got, wantStrip)
	}
}

func TestStripModeGatedRegion_UnclosedMarker(t *testing.T) {
	// Unclosed start marker: trailing lines treated per keepContent.
	body := "before\n<!-- ws:fresh-only:start -->\norphaned line\nno end marker"
	wantKeep := "before\norphaned line\nno end marker"
	wantStrip := "before"
	if got := stripModeGatedRegion(body, true); got != wantKeep {
		t.Fatalf("unclosed keep:\n got: %q\nwant: %q", got, wantKeep)
	}
	if got := stripModeGatedRegion(body, false); got != wantStrip {
		t.Fatalf("unclosed strip:\n got: %q\nwant: %q", got, wantStrip)
	}
}

// callToolNoKey issues a single tools/call MCP request WITHOUT injecting a
// session_key — needed to test the fresh-mode path of ws.workflow_manual.
func callToolNoKey(t *testing.T, server *Server, id int, name string, args map[string]any) string {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	// Do NOT inject session_key (contrast with callToolWithKey).
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params":  map[string]any{"name": name, "arguments": args},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(string(raw)+"\n"), &out); err != nil {
		t.Fatalf("ServeStdio(%s) error: %v", name, err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	return toolText(t, byID[fmt.Sprint(id)])
}

// --- Phase 3a: ws.workflow_manual integration tests --------------------------

func TestWorkflowManualFreshMode(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")

	// Fresh mode: call with the reserved sentinel key (not keyless). The sentinel
	// is never minted, so the keyed lead-only gate sees a lookup-miss and skips;
	// the call reaches the handler, which maps the sentinel to fresh mode.
	resp := callToolWithKey(t, server, 5001, freshBootstrapKey, "ws.workflow_manual", nil)

	// Self-bootstrap fragment must be present (gated region is KEPT).
	if !strings.Contains(resp, "mint your lead key") {
		t.Errorf("fresh mode: self-bootstrap fragment absent from response:\n%s", resp)
	}
	// Per-root rule fragment must be present (always shown).
	if !strings.Contains(resp, "once per working root") {
		t.Errorf("fresh mode: per-root rule fragment absent from response:\n%s", resp)
	}
	// No "Session State" section in fresh mode.
	if strings.Contains(resp, "Session State") {
		t.Errorf("fresh mode: unexpected Session State section in response:\n%s", resp)
	}
}

func TestWorkflowManualKeylessRejected(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")

	// Keyless call must be rejected with a required session_key error.
	resp := callToolNoKey(t, server, 5002, "ws.workflow_manual", nil)

	if !strings.Contains(resp, "session_key") {
		t.Errorf("keyless: response must mention session_key, got:\n%s", resp)
	}
	// The bootstrap fragment must NOT be present — no leak of ferrule guidance.
	if strings.Contains(resp, "mint your lead key") {
		t.Errorf("keyless: self-bootstrap fragment must be absent from error response:\n%s", resp)
	}
}

func TestWorkflowManualDelegateKeyBlocked(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")

	// Mint a delegate-scoped key directly.
	delegateKey, err := server.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatalf("mint delegate key: %v", err)
	}

	// ws.workflow_manual must be rejected for delegate keys at the keyed gate.
	// The gate returns an RPC-level error (-32601), not a toolText response, so
	// read the raw JSON line (same pattern as session_auth_test.go assertGateError).
	rawResp := callToolOnce(t, server, 5003, "ws.workflow_manual", map[string]any{
		"session_key": delegateKey,
	})

	// Must receive the lead-only profile rejection (JSON-RPC error -32601).
	if !strings.Contains(rawResp, "tool not available in current") {
		t.Errorf("delegate key: expected lead-only rejection, got:\n%s", rawResp)
	}
	// The rejection must carry the keyed-gate RPC error code (-32601), not a soft
	// text response — confirm the code is present in the raw JSON-RPC error.
	if !strings.Contains(rawResp, "-32601") {
		t.Errorf("delegate key: expected JSON-RPC error code -32601, got:\n%s", rawResp)
	}
	// Must NOT contain a manual body.
	if strings.Contains(rawResp, "mint your lead key") || strings.Contains(rawResp, "Session State") {
		t.Errorf("delegate key: manual body must be absent from rejection response:\n%s", rawResp)
	}
}

func TestWorkflowManualContinueMode(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5100, root, nil))

	// Enter implement mode to populate agenda+todos.
	enter := callToolWithKey(t, server, 5101, key, "ws.enter.implement", map[string]any{
		"delegation": "delegated", "need_review": true, "need_doc": false,
	})
	if !strings.Contains(enter, "entered implement mode") {
		t.Fatalf("enter.implement unexpected: %s", enter)
	}

	// Continue mode: key present and resolves.
	resp := callToolWithKey(t, server, 5102, key, "ws.workflow_manual", nil)

	// Self-bootstrap fragment must be ABSENT (stripped in continue mode).
	if strings.Contains(resp, "mint your lead key") {
		t.Errorf("continue mode: self-bootstrap fragment should be absent:\n%s", resp)
	}
	// Per-root rule fragment must be present (always shown).
	if !strings.Contains(resp, "once per working root") {
		t.Errorf("continue mode: per-root rule fragment absent:\n%s", resp)
	}
	// Session State section must be present.
	if !strings.Contains(resp, "Session State") {
		t.Errorf("continue mode: Session State section absent:\n%s", resp)
	}
	// Agenda content present: renderSessionState emits "### agenda: <key>" headings.
	if !strings.Contains(resp, "### agenda: implement") {
		t.Errorf("continue mode: agenda heading '### agenda: implement' absent:\n%s", resp)
	}
	// Todo summary content present (Route and Prep are active).
	if !strings.Contains(resp, "Route") {
		t.Errorf("continue mode: todo 'Route' absent from summary:\n%s", resp)
	}
	if !strings.Contains(resp, "Prep") {
		t.Errorf("continue mode: todo 'Prep' absent from summary:\n%s", resp)
	}
}

func TestWorkflowManualTodoInstructionPreview(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5110, root, nil))
	longInstruction := "Restore this todo instruction through the workflow manual summary path without showing the extra full detail."
	wantPreview := "Restore this todo instruction through the workflow manual su"
	wantTail := "mmary path without showing the extra full detail."

	if got := callToolWithKey(t, server, 5111, key, "ws.todo.append", map[string]any{
		"key":         "restore",
		"title":       "Restore instruction",
		"instruction": longInstruction,
	}); !strings.Contains(got, "todo appended: restore") {
		t.Fatalf("append unexpected: %s", got)
	}

	resp := callToolWithKey(t, server, 5112, key, "ws.workflow_manual", nil)
	if !strings.Contains(resp, "### Todos") {
		t.Fatalf("workflow manual response missing todo summary:\n%s", resp)
	}
	if !strings.Contains(resp, "- [ ] {restore} Restore instruction\n      "+wantPreview) {
		t.Fatalf("workflow manual missing instruction preview:\n%s", resp)
	}
	if strings.Contains(resp, wantTail) {
		t.Fatalf("workflow manual rendered instruction tail, want preview only:\n%s", resp)
	}
}

func TestWorkflowManualUnknownKey(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheDir)

	server := NewServer(root, "test")

	// Use a syntactically valid but never-minted key.
	badKey := "no-such-key-here"
	resp := callToolWithKey(t, server, 5200, badKey, "ws.workflow_manual", nil)

	// Must contain the no-restorable-state notice.
	if !strings.Contains(resp, "no restorable state for session key") {
		t.Errorf("unknown key: no-restorable-state notice absent:\n%s", resp)
	}
	// Bootstrap line must be ABSENT in fail-loud mode (Phase 3a: stripped, not kept).
	if strings.Contains(resp, "mint your lead key") {
		t.Errorf("unknown key: self-bootstrap fragment must be absent (fail-loud strips it):\n%s", resp)
	}
	// Fail-loud renders NO manual body, so the always-shown per-root rule and the
	// ws.ferrule mention it carries must be absent — a non-lead caller reaching
	// fail-loud (any unregistered key) must not learn the lead self-bootstrap call.
	if strings.Contains(resp, "once per working root") || strings.Contains(resp, "ferrule") {
		t.Errorf("unknown key: manual body / ferrule mention must be absent in fail-loud:\n%s", resp)
	}
	// The recovery pointer names only the lead-revive skill (no ferrule/sentinel).
	if !strings.Contains(resp, "lead-revive") {
		t.Errorf("unknown key: fail-loud notice should point to lead-revive recovery:\n%s", resp)
	}
	// Must NOT have minted a key file. Use os.Stat on the specific record path so
	// the check is meaningful even when the keys/ directory was never created.
	keysDir := filepath.Join(cacheDir, "keys")
	_, statErr := os.Stat(filepath.Join(keysDir, badKey+".json"))
	if !os.IsNotExist(statErr) {
		t.Errorf("unknown key: key file was minted (stat: %v)", statErr)
	}
}

func TestWorkflowManualGitCommitReinjection(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5300, root, nil))

	// Enter implement mode to populate todos.
	callToolWithKey(t, server, 5301, key, "ws.enter.implement", map[string]any{
		"delegation": "delegated", "need_review": true, "need_doc": false,
	})

	// Stage a file and commit.
	testFile := filepath.Join(root, "test-p3a.txt")
	if err := os.WriteFile(testFile, []byte("p3a test\n"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	// git add
	{
		var out bytes.Buffer
		payload := map[string]any{
			"jsonrpc": "2.0",
			"id":      5302,
			"method":  "tools/call",
			"params": map[string]any{"name": "git.commit", "arguments": map[string]any{
				"session_key": key,
				"paths":       []any{"test-p3a.txt"},
				"title":       "test(p3a): re-injection test",
				"ai_context":  []any{"Phase 3a git.commit re-injection test"},
			}},
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := server.ServeStdio(context.Background(), strings.NewReader(string(raw)+"\n"), &out); err != nil {
			t.Fatalf("git.commit error: %v", err)
		}
		byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
		commitResp := toolText(t, byID["5302"])

		// Assert that the commit response contains a todo summary fragment.
		if !strings.Contains(commitResp, "## TODO(ws reminder: update this if stale)") {
			t.Errorf("git.commit re-injection: reminder heading absent from response:\n%s", commitResp)
		}
		if !strings.Contains(commitResp, "Route") && !strings.Contains(commitResp, "- [") {
			t.Errorf("git.commit re-injection: todo summary absent from response:\n%s", commitResp)
		}
	}

	// Also assert: a commit with no todos appends nothing extra.
	// Create a new session with no todos, commit, and check the output shape.
	root2 := t.TempDir()
	initGit(t, root2)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server2 := NewServer(root2, "test")
	key2, _ := parseLoginResponse(t, callLogin(t, server2, 5400, root2, nil))

	testFile2 := filepath.Join(root2, "test-p3a-notodo.txt")
	if err := os.WriteFile(testFile2, []byte("no todo\n"), 0o644); err != nil {
		t.Fatalf("write test file2: %v", err)
	}
	{
		var out bytes.Buffer
		payload := map[string]any{
			"jsonrpc": "2.0",
			"id":      5401,
			"method":  "tools/call",
			"params": map[string]any{"name": "git.commit", "arguments": map[string]any{
				"session_key": key2,
				"paths":       []any{"test-p3a-notodo.txt"},
				"title":       "test(p3a): no-todo commit",
				"ai_context":  []any{"no-todo test"},
			}},
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := server2.ServeStdio(context.Background(), strings.NewReader(string(raw)+"\n"), &out); err != nil {
			t.Fatalf("git.commit(no-todo) error: %v", err)
		}
		byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
		commitResp := toolText(t, byID["5401"])
		// No todo summary section appended.
		if strings.Contains(commitResp, "Todo (post-commit)") || strings.Contains(commitResp, "TODO(ws reminder: update this if stale)") {
			t.Errorf("git.commit(no-todo): unexpected Todo section:\n%s", commitResp)
		}
	}
}
