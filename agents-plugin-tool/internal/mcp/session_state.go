package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// session_state.go implements the session state machine layered onto the
// per-session record store (session_auth.go): the agenda namespace (freeform
// session-level blobs) and the todos namespace (ordered step-level checklist).
//
// All disk persistence reuses the existing sessionRecord file
// (<cache-root>/keys/<session-key>.json), its atomic temp+rename writer
// (writeRecordAtomic), and reader (readRecord). The pure list logic here is kept
// free of disk I/O so it can be table-tested without the server or filesystem.

// todoStatus is the lifecycle state of a single todo item. Unknown values are
// rejected at the parsing boundary (parseTodoStatus) so the stored list never
// holds an unrenderable status.
type todoStatus string

const (
	todoPending todoStatus = "pending"
	todoWip     todoStatus = "wip"
	todoDone    todoStatus = "done"
	todoDefer   todoStatus = "defer"

	todoInstructionPreviewRunes = 60
)

// todoItem is one ordered checklist entry. Identity is the caller-provided key,
// unique within the active list; the title is human-facing text. Instruction is
// optional focused runbook prose for the item; old records without it unmarshal
// with nil and remain compatible.
type todoItem struct {
	Key         string     `json:"key"`
	Title       string     `json:"title"`
	Status      todoStatus `json:"status"`
	Instruction *string    `json:"instruction,omitempty"`
}

type todoReadPayload struct {
	Key         string     `json:"key"`
	Title       string     `json:"title"`
	Status      todoStatus `json:"status"`
	Instruction *string    `json:"instruction"`
}

// parseTodoStatus validates a caller-supplied status string. An empty string
// defaults to pending so append/insert callers may omit it.
func parseTodoStatus(raw string) (todoStatus, error) {
	switch todoStatus(raw) {
	case todoPending, todoWip, todoDone, todoDefer:
		return todoStatus(raw), nil
	case "":
		return todoPending, nil
	default:
		return "", fmt.Errorf("invalid status %q: want one of pending, wip, done, defer", raw)
	}
}

// todoMarker maps a status to its rendering marker prefix.
func todoMarker(status todoStatus) string {
	switch status {
	case todoWip:
		return "- [~]"
	case todoDone:
		return "- [x]"
	case todoDefer:
		return "- [>]"
	default: // pending and any unexpected value render as pending
		return "- [ ]"
	}
}

// todoActive reports whether a status counts as an active (non-collapsing) item
// for summary rendering. defer collapses the same as done.
func todoActive(status todoStatus) bool {
	return status == todoPending || status == todoWip
}

// indexOfTodo returns the position of key in list, or -1 when absent.
func indexOfTodo(list []todoItem, key string) int {
	key = strings.ToLower(key)
	for i, item := range list {
		if strings.ToLower(item.Key) == key {
			return i
		}
	}
	return -1
}

// --- pure list mutations (no disk I/O) ---------------------------------------

func normalizeTodoKey(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("todo key must be non-empty")
	}
	if raw != strings.TrimSpace(raw) {
		return "", fmt.Errorf("todo key %q must not contain leading or trailing whitespace", raw)
	}
	key := strings.ToLower(raw)
	for len(key) > 0 {
		r, size := utf8.DecodeRuneInString(key)
		if size == 0 || r == utf8.RuneError && size == 1 {
			return "", fmt.Errorf("todo key %q contains invalid UTF-8", raw)
		}
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return "", fmt.Errorf("todo key %q contains invalid character %q: want lowercase letters, digits, '.', '_', or '-'", raw, r)
		}
		key = key[size:]
	}
	return strings.ToLower(raw), nil
}

// todoAppend adds a new item at the end. A duplicate key (still present in the
// active list) is an error; erased keys are reusable because they are gone from
// the slice.
func todoAppend(list []todoItem, key, title string, status todoStatus, instruction *string) ([]todoItem, error) {
	normalizedKey, err := normalizeTodoKey(key)
	if err != nil {
		return nil, err
	}
	if indexOfTodo(list, normalizedKey) >= 0 {
		return nil, fmt.Errorf("todo key %q already exists", normalizedKey)
	}
	return append(list, todoItem{Key: normalizedKey, Title: title, Status: status, Instruction: instruction}), nil
}

// todoInsert inserts a new item before or after refKey. after=false inserts
// before refKey; after=true inserts after it.
func todoInsert(list []todoItem, refKey, key, title string, status todoStatus, instruction *string, after bool) ([]todoItem, error) {
	normalizedRef, err := normalizeTodoKey(refKey)
	if err != nil {
		return nil, fmt.Errorf("ref_key: %w", err)
	}
	normalizedKey, err := normalizeTodoKey(key)
	if err != nil {
		return nil, err
	}
	if indexOfTodo(list, normalizedKey) >= 0 {
		return nil, fmt.Errorf("todo key %q already exists", normalizedKey)
	}
	ref := indexOfTodo(list, normalizedRef)
	if ref < 0 {
		return nil, fmt.Errorf("ref_key %q not found", normalizedRef)
	}
	pos := ref
	if after {
		pos = ref + 1
	}
	out := make([]todoItem, 0, len(list)+1)
	out = append(out, list[:pos]...)
	out = append(out, todoItem{Key: normalizedKey, Title: title, Status: status, Instruction: instruction})
	out = append(out, list[pos:]...)
	return out, nil
}

func todoRead(list []todoItem, key string) (todoReadPayload, error) {
	normalizedKey, err := normalizeTodoKey(key)
	if err != nil {
		return todoReadPayload{}, err
	}
	idx := indexOfTodo(list, normalizedKey)
	if idx < 0 {
		return todoReadPayload{}, fmt.Errorf("todo key %q not found", normalizedKey)
	}
	item := list[idx]
	return todoReadPayload{
		Key:         item.Key,
		Title:       item.Title,
		Status:      item.Status,
		Instruction: item.Instruction,
	}, nil
}

// todoCheck sets the status of an existing item.
func todoCheck(list []todoItem, key string, status todoStatus) ([]todoItem, error) {
	normalizedKey, err := normalizeTodoKey(key)
	if err != nil {
		return nil, err
	}
	idx := indexOfTodo(list, normalizedKey)
	if idx < 0 {
		return nil, fmt.Errorf("todo key %q not found", normalizedKey)
	}
	list[idx].Status = status
	return list, nil
}

// todoErase removes an item by key. The key becomes reusable.
func todoErase(list []todoItem, key string) ([]todoItem, error) {
	normalizedKey, err := normalizeTodoKey(key)
	if err != nil {
		return nil, err
	}
	idx := indexOfTodo(list, normalizedKey)
	if idx < 0 {
		return nil, fmt.Errorf("todo key %q not found", normalizedKey)
	}
	out := make([]todoItem, 0, len(list)-1)
	out = append(out, list[:idx]...)
	out = append(out, list[idx+1:]...)
	return out, nil
}

// todoClear removes all items, or only done items when doneOnly is true
// (leaving pending, wip, and defer).
func todoClear(list []todoItem, doneOnly bool) []todoItem {
	if !doneOnly {
		return nil
	}
	out := make([]todoItem, 0, len(list))
	for _, item := range list {
		if item.Status != todoDone {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// todoReorder moves the contiguous span [fromKey … toKey] as a block to before
// or after refKey. The span must be a valid forward range, and refKey must lie
// outside the span.
func todoReorder(list []todoItem, fromKey, toKey, refKey string, after bool) ([]todoItem, error) {
	normalizedFrom, err := normalizeTodoKey(fromKey)
	if err != nil {
		return nil, fmt.Errorf("from_key: %w", err)
	}
	normalizedTo, err := normalizeTodoKey(toKey)
	if err != nil {
		return nil, fmt.Errorf("to_key: %w", err)
	}
	normalizedRef, err := normalizeTodoKey(refKey)
	if err != nil {
		return nil, fmt.Errorf("ref_key: %w", err)
	}
	from := indexOfTodo(list, normalizedFrom)
	if from < 0 {
		return nil, fmt.Errorf("from_key %q not found", normalizedFrom)
	}
	to := indexOfTodo(list, normalizedTo)
	if to < 0 {
		return nil, fmt.Errorf("to_key %q not found", normalizedTo)
	}
	if from > to {
		return nil, fmt.Errorf("from_key %q must not come after to_key %q", normalizedFrom, normalizedTo)
	}
	ref := indexOfTodo(list, normalizedRef)
	if ref < 0 {
		return nil, fmt.Errorf("ref_key %q not found", normalizedRef)
	}
	if ref >= from && ref <= to {
		return nil, fmt.Errorf("ref_key %q is inside the moved span", normalizedRef)
	}

	span := append([]todoItem(nil), list[from:to+1]...)
	rest := make([]todoItem, 0, len(list)-len(span))
	rest = append(rest, list[:from]...)
	rest = append(rest, list[to+1:]...)

	// Locate refKey within rest (its index shifts once the span is removed) and
	// splice the span back in relative to it.
	refIdx := indexOfTodo(rest, normalizedRef)
	pos := refIdx
	if after {
		pos = refIdx + 1
	}
	out := make([]todoItem, 0, len(list))
	out = append(out, rest[:pos]...)
	out = append(out, span...)
	out = append(out, rest[pos:]...)
	return out, nil
}

// renderTodos formats the list. full=true shows every item in order; full=false
// (summary) shows all active (pending/wip) items plus one adjacent context item
// (done/defer) on each side of every contiguous active block, collapsing each
// remaining run to a single "..." line.
func renderTodos(list []todoItem, full bool) string {
	if len(list) == 0 {
		return "(no todos)"
	}
	if full {
		lines := make([]string, 0, len(list)*2)
		for _, item := range list {
			lines = append(lines, renderTodoLines(item, full)...)
		}
		return strings.Join(lines, "\n")
	}

	// shown[i] is true when item i is active, or directly adjacent to an active
	// item (the one-context-item-each-side rule). Runs of non-shown items
	// collapse to a single "..." line.
	shown := make([]bool, len(list))
	for i, item := range list {
		if todoActive(item.Status) {
			shown[i] = true
			if i > 0 {
				shown[i-1] = true
			}
			if i+1 < len(list) {
				shown[i+1] = true
			}
		}
	}

	var lines []string
	collapsed := false
	for i, item := range list {
		if shown[i] {
			lines = append(lines, renderTodoLines(item, full)...)
			collapsed = false
			continue
		}
		if !collapsed {
			lines = append(lines, "...")
			collapsed = true
		}
	}
	return strings.Join(lines, "\n")
}

func renderTodosCheckpoint(list []todoItem, checkedKey string) string {
	if len(list) == 0 {
		return "(no todos)"
	}
	checkedIdx := indexOfTodo(list, checkedKey)
	lines := make([]string, 0, len(list)*2)
	for i, item := range list {
		adjacent := checkedIdx >= 0 && (i == checkedIdx-1 || i == checkedIdx+1)
		fullInstruction := adjacent && todoActive(item.Status) && item.Instruction != nil && *item.Instruction != ""
		if fullInstruction {
			lines = append(lines, renderTodoLines(item, true)...)
			continue
		}
		lines = append(lines, renderTodoLine(item))
		if item.Instruction != nil && *item.Instruction != "" {
			lines = append(lines, "      ...+")
		}
	}
	return strings.Join(lines, "\n")
}

func renderTodoLine(item todoItem) string {
	return fmt.Sprintf("%s {%s} %s", todoMarker(item.Status), item.Key, item.Title)
}

func renderTodoLines(item todoItem, full bool) []string {
	lines := []string{renderTodoLine(item)}
	if item.Instruction == nil || *item.Instruction == "" {
		return lines
	}
	instruction := *item.Instruction
	if !full {
		instruction = todoInstructionPreview(instruction)
	}
	lines = append(lines, "      "+instruction)
	return lines
}

func todoInstructionPreview(instruction string) string {
	runes := []rune(instruction)
	if len(runes) <= todoInstructionPreviewRunes {
		return instruction
	}
	return string(runes[:todoInstructionPreviewRunes])
}

// --- enter-mode todo derivation ----------------------------------------------

type implementTodoVerdict struct {
	Delegation  string
	BranchPlan  implementBranchPlan
	PlanDepth   string
	ReviewAlloc string
	NeedReview  bool
	DocMode     string
	DocReason   string
	NeedDoc     bool
}

// deriveImplementTodos builds the lead-implement checklist. Route, Prep, Edit,
// Final action gate, and Merge are always present; Review is inserted after Edit
// when needReview; the Doc steps are inserted after Review when needDoc. Order
// mirrors the lead-implement pipeline (Route -> Prep -> Edit -> Review -> Doc ->
// Final action gate -> Merge).
func deriveImplementTodos(needReview, needDoc bool) []todoItem {
	return deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  "delegated",
		PlanDepth:   "survey",
		ReviewAlloc: "partitioned",
		NeedReview:  needReview,
		NeedDoc:     needDoc,
	})
}

func deriveImplementTodosFromVerdict(verdict implementTodoVerdict) []todoItem {
	items := []todoItem{
		{Key: "route", Title: "Route", Instruction: implementInstructionPtr(implementRouteInstruction(verdict))},
		{Key: "prep", Title: implementPrepTitle(verdict.PlanDepth), Instruction: implementInstructionPtr(implementPrepInstruction(verdict))},
		{Key: "edit", Title: implementEditTitle(verdict.Delegation), Instruction: implementInstructionPtr(implementEditInstruction(verdict))},
	}
	if verdict.NeedReview || isLeadOnlyReview(verdict.ReviewAlloc) {
		items = append(items, todoItem{Key: "review", Title: implementReviewTitle(verdict.ReviewAlloc), Instruction: implementInstructionPtr(implementReviewInstruction(verdict))})
	}
	if verdict.NeedDoc {
		items = append(items,
			todoItem{Key: "doc-pre-pass", Title: "Doc pre-pass", Instruction: implementInstructionPtr(implementDocPrePassInstruction(verdict))},
			todoItem{Key: "doc-commit-gate", Title: "Doc commit gate", Instruction: implementInstructionPtr(implementDocCommitGateInstruction(verdict))},
			todoItem{Key: "doc-closeout", Title: "Doc closeout", Instruction: implementInstructionPtr(implementDocCloseoutInstruction(verdict))},
		)
	}
	items = append(items,
		todoItem{Key: "final-action-gate", Title: "Final action gate", Instruction: implementInstructionPtr(implementFinalActionInstruction(verdict))},
		todoItem{Key: "merge", Title: "Merge", Instruction: implementInstructionPtr(implementMergeInstruction(verdict))},
	)
	return withPendingStatus(items)
}

func implementInstructionPtr(instruction string) *string {
	return &instruction
}

func parseImplementDelegation(raw string) (string, error) {
	switch strings.ToLower(raw) {
	case "", "delegated":
		return "delegated", nil
	case "direct-edit":
		return "direct-edit", nil
	default:
		return "", fmt.Errorf("invalid delegation %q: want one of delegated, direct-edit", raw)
	}
}

func parseImplementReviewAlloc(raw string) (string, error) {
	switch strings.ToLower(raw) {
	case "":
		return "partitioned", nil
	case "lead-only", "single", "partitioned":
		return strings.ToLower(raw), nil
	case "partitioned: correctness", "partitioned: fit", "partitioned: test",
		"partitioned: correctness, fit", "partitioned: correctness, test", "partitioned: fit, test",
		"partitioned: correctness, fit, test":
		return "partitioned", nil
	default:
		return "", fmt.Errorf("invalid review_alloc %q: want one of lead-only, single, partitioned", raw)
	}
}

func implementPrepTitle(planDepth string) string {
	switch strings.ToLower(strings.TrimSpace(planDepth)) {
	case "none", "":
		return "Prep"
	case "survey":
		return "Prep (survey plan)"
	case "research":
		return "Prep (research plan)"
	default:
		return "Prep"
	}
}

func implementEditTitle(delegation string) string {
	switch strings.ToLower(strings.TrimSpace(delegation)) {
	case "delegated":
		return "Edit (delegated)"
	case "direct", "direct-edit", "inline", "lead-owned":
		return "Edit (direct)"
	case "":
		return "Edit"
	default:
		return "Edit"
	}
}

func implementReviewTitle(reviewAlloc string) string {
	switch strings.ToLower(strings.TrimSpace(reviewAlloc)) {
	case "single", "single reviewer":
		return "Review (single)"
	case "partitioned", "partitioned: correctness, fit, test", "partitioned: correctness,fit,test":
		return "Review (partitioned)"
	case "lead-only", "lead only":
		return "Review (lead-only)"
	case "":
		return "Review"
	default:
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(reviewAlloc)), "partitioned:") {
			return "Review (partitioned)"
		}
		return "Review"
	}
}

func implementRouteInstruction(verdict implementTodoVerdict) string {
	plan := verdict.BranchPlan
	switch plan.Action {
	case "stop":
		return fmt.Sprintf("Stop before source edits: %s. Resolve the branch policy or branch state before continuing.", firstNonEmpty(plan.Reason, "branch action is blocked"))
	case "create":
		return fmt.Sprintf("Create %s from %s before source edits, then keep %s as the merge target. Mark route complete only after the branch action succeeds; do not call enter.implement again.", firstNonEmpty(plan.TargetBranch, "the implementation branch"), firstNonEmpty(plan.MergeTarget, plan.CurrentBranch, "the current branch"), firstNonEmpty(plan.MergeTarget, "the selected base branch"))
	case "rename":
		return fmt.Sprintf("Rename the current implementation branch to %s before source edits, preserving %s as the merge target. Mark route complete only after the branch action succeeds; do not call enter.implement again.", firstNonEmpty(plan.TargetBranch, "the target implementation branch"), firstNonEmpty(plan.MergeTarget, "the selected base branch"))
	case "continue":
		return fmt.Sprintf("Continue on %s for this implementation path before starting prep or edits. Keep the existing implementation branch context and do not call enter.implement again.", firstNonEmpty(plan.CurrentBranch, plan.TargetBranch, "the current implementation branch"))
	default:
		return "Confirm the implementation branch setup before source edits, then follow the selected implementation path."
	}
}

func implementPrepInstruction(verdict implementTodoVerdict) string {
	if isBranchStop(verdict) {
		return fmt.Sprintf("Do not prepare further implementation work until the branch blocker is resolved: %s.", firstNonEmpty(verdict.BranchPlan.Reason, "branch action is blocked"))
	}
	const guardrails = `Before edits or dispatch, run mental-model lookup, read returned docs ancestors first, read the 260605 migration anchor when target touches plugin architecture, host-neutral migration, spawn-removal, or adapter boundaries, and read infra.read("impl-playbook"). `
	switch strings.ToLower(strings.TrimSpace(verdict.PlanDepth)) {
	case "none", "":
		return guardrails + "Confirm the direct-edit facts are still accurate, identify the focused verification command, and proceed without a separate brief, survey, or research plan."
	case "survey":
		return guardrails + "Call path.generate(kind: \"plan\", stems: [target stem or scope]) to create the plan path, render plan-populator-survey with ticket_path, selected_phase, and plan_path, and dispatch it to write the light implementation plan. If survey returns [escalate-to-research] for low confidence or strategic uncertainty, render plan-populator-research with the same plan path before implementer dispatch. Do not create a separate brief."
	case "research":
		return guardrails + "Render plan-populator-research with ticket_path, selected_phase, and an existing plan_path, then dispatch it to refine or replace the same implementation plan before implementer dispatch. Do not create a separate brief."
	default:
		return guardrails + "Prepare the implementation context required by the selected verdict before edits."
	}
}

func implementEditInstruction(verdict implementTodoVerdict) string {
	if isBranchStop(verdict) {
		return fmt.Sprintf("Do not start source edits while branch action is stop: %s.", firstNonEmpty(verdict.BranchPlan.Reason, "branch action is blocked"))
	}
	switch strings.ToLower(strings.TrimSpace(verdict.Delegation)) {
	case "direct-edit":
		return "Apply the source edits directly in this lead context, run focused verification, commit the logical checkpoint, and capture the resulting commit range."
	case "delegated":
		switch strings.ToLower(strings.TrimSpace(verdict.PlanDepth)) {
		case "survey":
			return "After the survey plan is ready and any [escalate-to-research] signal is resolved on the same plan path, render implementer with PlanPath and dispatch the delegated implementer; capture the implemented commit range for review and relays."
		case "research":
			return "After the research plan is ready on the same plan path, render implementer with PlanPath and dispatch the delegated implementer; capture the implemented commit range for review and relays."
		default:
			return "Dispatch the delegated implementer with Delegate dispatch and the Implementer spawn prompt, using the resolved implementation context; capture the implemented commit range for review and relays."
		}
	default:
		return "Execute the selected implementation path and verify the changed behavior before review or documentation closeout."
	}
}

func implementReviewInstruction(verdict implementTodoVerdict) string {
	if isBranchStop(verdict) {
		return fmt.Sprintf("Do not start review before implementation can run; resolve the branch blocker first: %s.", firstNonEmpty(verdict.BranchPlan.Reason, "branch action is blocked"))
	}
	if isLeadOnlyReview(verdict.ReviewAlloc) {
		return "Perform lead-owned review only; record why external reviewers are unnecessary for this verdict, then preserve the rationale for the final report."
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(verdict.ReviewAlloc)), "partitioned:") {
		return fmt.Sprintf("Dispatch %s reviewers with the Reviewer prompt frame and generated review paths. Use Review relay and Re-review prompts only for genuinely new non-clean Critical/Important findings.", formatReviewPartitions(verdict.ReviewAlloc))
	}
	if strings.EqualFold(strings.TrimSpace(verdict.ReviewAlloc), "single") {
		return "Dispatch one reviewer with the Reviewer prompt frame and a generated review path. Use Review relay and Re-review prompts only for genuinely new non-clean Critical/Important findings."
	}
	return "Dispatch the selected reviewers with the Reviewer prompt frame and generated review paths. Use Review relay and Re-review prompts only for genuinely new non-clean Critical/Important findings."
}

func implementDocPrePassInstruction(verdict implementTodoVerdict) string {
	if isBranchStop(verdict) {
		return fmt.Sprintf("Do not start documentation work before implementation can run; resolve the branch blocker first: %s.", firstNonEmpty(verdict.BranchPlan.Reason, "branch action is blocked"))
	}
	return "Run the standard documentation pre-pass: update specs first, then dispatch mental-model-updater with the implemented commit range."
}

func implementDocCommitGateInstruction(verdict implementTodoVerdict) string {
	if isBranchStop(verdict) {
		return fmt.Sprintf("Do not open the documentation commit gate before source edits can run; resolve the branch blocker first: %s.", firstNonEmpty(verdict.BranchPlan.Reason, "branch action is blocked"))
	}
	return "Run the documentation commit gate: read executor-wrapup, update ticket result or project memory when reachable, and commit documentation changes before the final action gate."
}

func implementDocCloseoutInstruction(verdict implementTodoVerdict) string {
	if isBranchStop(verdict) {
		return fmt.Sprintf("Do not close documentation before implementation can run; resolve the branch blocker first: %s.", firstNonEmpty(verdict.BranchPlan.Reason, "branch action is blocked"))
	}
	return "Run documentation closeout compaction only for a safe documentation-only branch-tip suffix; otherwise record the skipped compaction status."
}

func implementFinalActionInstruction(verdict implementTodoVerdict) string {
	if isBranchStop(verdict) {
		return fmt.Sprintf("Do not ask for final action approval while branch action is stop: %s.", firstNonEmpty(verdict.BranchPlan.Reason, "branch action is blocked"))
	}
	if strings.EqualFold(strings.TrimSpace(verdict.DocMode), "skipped") {
		return fmt.Sprintf("Verify source, tests, review disposition, and skipped documentation policy before asking for final action approval: %s.", firstNonEmpty(verdict.DocReason, "no documentation updates are reachable in this verdict"))
	}
	return "Verify source, tests, review disposition, and standard documentation closeout before asking for final action approval."
}

func implementMergeInstruction(verdict implementTodoVerdict) string {
	if isBranchStop(verdict) {
		return fmt.Sprintf("Do not merge while branch action is stop: %s.", firstNonEmpty(verdict.BranchPlan.Reason, "branch action is blocked"))
	}
	return "After user approval, perform the selected final action against the verdict merge target and preserve the workflow-owned merge record."
}

func isBranchStop(verdict implementTodoVerdict) bool {
	return strings.EqualFold(strings.TrimSpace(verdict.BranchPlan.Action), "stop")
}

func isLeadOnlyReview(reviewAlloc string) bool {
	return strings.EqualFold(strings.TrimSpace(reviewAlloc), "lead-only") || strings.EqualFold(strings.TrimSpace(reviewAlloc), "lead only")
}

func formatReviewPartitions(reviewAlloc string) string {
	raw := strings.TrimSpace(reviewAlloc)
	_, partsRaw, ok := strings.Cut(raw, ":")
	if !ok {
		return "partitioned"
	}
	parts := []string{}
	for _, part := range strings.Split(partsRaw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "partitioned"
	}
	return joinHumanList(parts)
}

func joinHumanList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
}

// deriveProceedTodos mirrors lead-proceed "On: invoke": build route context,
// then resolve the MCP verdict with an executable Next instruction.
func deriveProceedTodos() []todoItem {
	return withPendingStatus([]todoItem{
		{Key: "route-context", Title: "Build route context"},
		{Key: "resolve-verdict", Title: "Resolve MCP verdict"},
	})
}

// deriveSprintTodos mirrors the lead-sprint episode lifecycle (On: sprint-edit
// plus On: wrap episode).
func deriveSprintTodos() []todoItem {
	return withPendingStatus([]todoItem{
		{Key: "edit", Title: "Edit (lead-owned, in-context)"},
		{Key: "verify", Title: "Verify (focused)"},
		{Key: "commit", Title: "Commit (Sprint-Edit markers)"},
		{Key: "post-edit", Title: "Post-edit decision (keep / wrap / shift)"},
		{Key: "wrap", Title: "Wrap episode (spec + mental-model + doc closure)"},
	})
}

// deriveSalvageTodos mirrors the lead-salvage states: containment, survey
// fanout, premise interview, classification, capture.
func deriveSalvageTodos() []todoItem {
	return withPendingStatus([]todoItem{
		{Key: "containment", Title: "Containment (freeze evidence, confirm failure claim)"},
		{Key: "survey-fanout", Title: "Survey fanout"},
		{Key: "premise-interview", Title: "Premise interview"},
		{Key: "classification", Title: "Classification (salvage report + recovery plan)"},
		{Key: "capture", Title: "Capture (recovery tickets after approval)"},
	})
}

// withPendingStatus stamps every item with pending status. Derivation builders
// leave Status empty for brevity.
func withPendingStatus(items []todoItem) []todoItem {
	for i := range items {
		items[i].Status = todoPending
	}
	return items
}

// --- store-bound read-modify-write wrappers ----------------------------------

// mutateRecord runs fn against the session record for sessionKey under s.mu and
// persists the result atomically. It is the shared read-modify-write primitive
// for every agenda/todo mutation, matching the discipline of setOverride.
func (s *sessionStore) mutateRecord(sessionKey string, fn func(*sessionRecord) error) error {
	dir, err := s.keysDir()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.readRecord(dir, sessionKey)
	if !ok {
		return fmt.Errorf("session key not found: %s", sessionKey)
	}
	if err := fn(&record); err != nil {
		return err
	}
	return s.writeRecordAtomic(dir, sessionKey, record)
}

// readState returns a snapshot of the session record for read-only callers
// (e.g. ws.todo.list). It holds s.mu to avoid racing a concurrent write.
func (s *sessionStore) readState(sessionKey string) (sessionRecord, bool) {
	dir, err := s.keysDir()
	if err != nil {
		return sessionRecord{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readRecord(dir, sessionKey)
}

// setAgenda upserts an agenda blob under key.
func (s *sessionStore) setAgenda(sessionKey, key string, value json.RawMessage) error {
	return s.mutateRecord(sessionKey, func(r *sessionRecord) error {
		if r.Agenda == nil {
			r.Agenda = map[string]json.RawMessage{}
		}
		r.Agenda[key] = value
		return nil
	})
}

// clearAgenda removes the agenda blob for key. A missing key is a no-op.
func (s *sessionStore) clearAgenda(sessionKey, key string) error {
	return s.mutateRecord(sessionKey, func(r *sessionRecord) error {
		delete(r.Agenda, key)
		if len(r.Agenda) == 0 {
			r.Agenda = nil
		}
		return nil
	})
}

// clearAllAgenda removes every agenda blob for the session. An already-empty
// agenda map is a no-op.
func (s *sessionStore) clearAllAgenda(sessionKey string) error {
	return s.mutateRecord(sessionKey, func(r *sessionRecord) error {
		r.Agenda = nil
		return nil
	})
}

// enterMode atomically stores the typed payload as an agenda blob under
// agendaKey and replaces the entire todo list with todos. This is the single
// write behind every ws.enter.* tool: agenda update and todo replacement land
// together so a reader never observes a half-applied mode switch.
func (s *sessionStore) enterMode(sessionKey, agendaKey string, payload json.RawMessage, todos []todoItem) error {
	return s.mutateRecord(sessionKey, func(r *sessionRecord) error {
		if r.Agenda == nil {
			r.Agenda = map[string]json.RawMessage{}
		}
		r.Agenda[agendaKey] = payload
		r.Todos = todos
		return nil
	})
}

// mutateTodos applies a pure list mutation to the stored todo list.
func (s *sessionStore) mutateTodos(sessionKey string, fn func([]todoItem) ([]todoItem, error)) error {
	return s.mutateRecord(sessionKey, func(r *sessionRecord) error {
		next, err := fn(r.Todos)
		if err != nil {
			return err
		}
		r.Todos = next
		return nil
	})
}

func (s *sessionStore) mutateTodosResult(sessionKey string, fn func([]todoItem) ([]todoItem, error)) ([]todoItem, error) {
	var updated []todoItem
	if err := s.mutateRecord(sessionKey, func(r *sessionRecord) error {
		next, err := fn(r.Todos)
		if err != nil {
			return err
		}
		r.Todos = next
		updated = append([]todoItem(nil), next...)
		return nil
	}); err != nil {
		return nil, err
	}
	return updated, nil
}

// --- MCP handlers ------------------------------------------------------------
//
// These parse arguments, drive the store, and format the compact text response.
// They are the dispatch targets for the ws.agenda.*, ws.enter.*, and ws.todo.*
// cases in callTool. All session-state tools require a session_key and are
// reachable by any role that holds one (roleAllowsTool does not gate these
// prefixes), per the ticket's D3 scoping decision.

// sessionStateKey extracts and validates the required session_key argument.
func sessionStateKey(toolName string, args map[string]any) (string, error) {
	key, _ := args["session_key"].(string)
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("%s: session_key is required", toolName)
	}
	return key, nil
}

// stringArg returns a trimmed required string argument or an error.
func stringArg(toolName, name string, args map[string]any) (string, error) {
	v, _ := args[name].(string)
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("%s: %s is required", toolName, name)
	}
	return v, nil
}

func rawStringArg(toolName, name string, args map[string]any) (string, error) {
	v, _ := args[name].(string)
	if v == "" {
		return "", fmt.Errorf("%s: %s is required", toolName, name)
	}
	return v, nil
}

func todoInstructionArg(toolName string, args map[string]any) (*string, error) {
	raw, ok := args["instruction"]
	if !ok || raw == nil {
		return nil, nil
	}
	instruction, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("%s: instruction must be a string or null", toolName)
	}
	return &instruction, nil
}

func (s *Server) handleAgendaSet(id json.RawMessage, args map[string]any) response {
	const tool = "agenda.set"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	key, err := stringArg(tool, "key", args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	value, ok := args["value"]
	if !ok {
		return toolTextResponse(id, "", fmt.Errorf("%s: value is required", tool))
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: value is not JSON-encodable: %w", tool, err))
	}
	if err := s.sessions.setAgenda(sessionKey, key, raw); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	return toolTextResponse(id, fmt.Sprintf("agenda set: %s\n", key), nil)
}

func (s *Server) handleAgendaClear(id json.RawMessage, args map[string]any) response {
	const tool = "agenda.clear"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	all, _ := args["all"].(bool)
	if all {
		if err := s.sessions.clearAllAgenda(sessionKey); err != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
		}
		return toolTextResponse(id, "agenda cleared: all\n", nil)
	}
	key, err := stringArg(tool, "key", args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if err := s.sessions.clearAgenda(sessionKey, key); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	return toolTextResponse(id, fmt.Sprintf("agenda cleared: %s\n", key), nil)
}

// agendaSummary renders a compact single-line preview of an agenda blob's
// JSON value for ws.agenda.list. Object blobs show their top-level keys;
// other JSON shapes fall back to a truncated raw rendering.
func agendaSummary(raw json.RawMessage) string {
	const maxLen = 80
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		summary := fmt.Sprintf("{%s}", strings.Join(keys, ", "))
		if len(summary) > maxLen {
			summary = summary[:maxLen-1] + "…"
		}
		return summary
	}
	flat := strings.Join(strings.Fields(string(raw)), " ")
	if len(flat) > maxLen {
		flat = flat[:maxLen-1] + "…"
	}
	return flat
}

func (s *Server) handleAgendaList(id json.RawMessage, args map[string]any) response {
	const tool = "agenda.list"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	record, ok := s.sessions.readState(sessionKey)
	if !ok {
		return toolTextResponse(id, "", fmt.Errorf("%s: session key not found: %s", tool, sessionKey))
	}
	if len(record.Agenda) == 0 {
		return toolTextResponse(id, "no agenda blobs\n", nil)
	}
	keys := make([]string, 0, len(record.Agenda))
	for k := range record.Agenda {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", k, agendaSummary(record.Agenda[k])))
	}
	return toolTextResponse(id, sb.String(), nil)
}

// handleEnter stores the typed payload (all args except session_key) under the
// mode's agenda key and replaces the todo list with the derived items.
func (s *Server) handleEnter(id json.RawMessage, tool, mode string, args map[string]any, todos []todoItem) response {
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	payload := map[string]any{}
	for k, v := range args {
		if k == "session_key" {
			continue
		}
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: payload is not JSON-encodable: %w", tool, err))
	}
	if err := s.sessions.enterMode(sessionKey, mode, raw, todos); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	text := fmt.Sprintf("entered %s mode; todo list replaced (%d items)\n%s\n", mode, len(todos), renderTodos(todos, false))
	return toolTextResponse(id, text, nil)
}

func (s *Server) handleEnterImplement(id json.RawMessage, args map[string]any) response {
	if _, hasNewTarget := args["target"]; hasNewTarget {
		const tool = "enter.implement"
		sessionKey, err := sessionStateKey(tool, args)
		if err != nil {
			return toolTextResponse(id, "", err)
		}
		record, ok := s.sessions.readState(sessionKey)
		if !ok {
			return toolTextResponse(id, "", fmt.Errorf("%s: session key not found: %s", tool, sessionKey))
		}
		input, err := parseImplementInput(args)
		if err != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
		}
		normalized, _ := normalizeImplementFacts(input)
		targetBranch := "implement/" + normalized.ScopeSlug
		obs, err := observeImplementBranch(record.Root, targetBranch)
		if err != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: branch preflight failed: %w", tool, err))
		}
		result := resolveImplement(input, obs)
		rawAgenda, err := json.Marshal(result.Agenda)
		if err != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: agenda is not JSON-encodable: %w", tool, err))
		}
		todos := deriveImplementTodosFromVerdict(implementTodoVerdict{
			Delegation:  result.Verdict.Delegation,
			BranchPlan:  result.Verdict.BranchPlan,
			PlanDepth:   result.Verdict.PlanDepth,
			ReviewAlloc: result.Verdict.ReviewAlloc,
			NeedReview:  result.Verdict.NeedReview,
			DocMode:     result.Verdict.DocMode,
			DocReason:   result.Agenda.DocReason,
			NeedDoc:     result.Verdict.DocMode == "standard",
		})
		if err := s.sessions.enterMode(sessionKey, "implement", rawAgenda, todos); err != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
		}
		if input.Format == "json" {
			text, err := implementResultJSON(result)
			return toolTextResponse(id, text, err)
		}
		return toolTextResponse(id, result.Raw, nil)
	}

	needReview, _ := args["need_review"].(bool)
	needDoc, _ := args["need_doc"].(bool)
	delegation, err := parseImplementDelegation(stringValue(args["delegation"]))
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("enter.implement: %w", err))
	}
	planDepth, err := parseLegacyImplementPlanDepth(delegation, stringValue(args["plan_depth"]))
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("enter.implement: %w", err))
	}
	reviewAlloc, err := parseImplementReviewAlloc(stringValue(args["review_alloc"]))
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("enter.implement: %w", err))
	}
	args["delegation"] = delegation
	args["plan_depth"] = planDepth
	args["review_alloc"] = reviewAlloc
	todos := deriveImplementTodosFromVerdict(implementTodoVerdict{
		Delegation:  delegation,
		PlanDepth:   planDepth,
		ReviewAlloc: reviewAlloc,
		NeedReview:  needReview,
		NeedDoc:     needDoc,
	})
	return s.handleEnter(id, "enter.implement", "implement", args, todos)
}

func parseLegacyImplementPlanDepth(delegation string, raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch delegation {
	case "direct-edit":
		switch normalized {
		case "", "none":
			return "none", nil
		case "survey", "research":
			return "", fmt.Errorf("invalid plan_depth %q for direct-edit: want none", raw)
		default:
			return "", fmt.Errorf("invalid plan_depth %q: want one of none, survey", raw)
		}
	case "delegated":
		switch normalized {
		case "", "survey":
			return "survey", nil
		case "research":
			return "", fmt.Errorf("invalid plan_depth %q for delegated legacy enter: start with survey and escalate to research only after survey returns [escalate-to-research]", raw)
		case "none":
			return "", fmt.Errorf("invalid plan_depth %q for delegated: want survey", raw)
		default:
			return "", fmt.Errorf("invalid plan_depth %q: want one of none, survey", raw)
		}
	default:
		return "", fmt.Errorf("invalid delegation %q: want one of delegated, direct-edit", delegation)
	}
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func (s *Server) handleEnterProceed(id json.RawMessage, args map[string]any) response {
	const tool = "enter.proceed"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	input, err := parseProceedInput(args)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	result := resolveProceed(input)
	rawAgenda, err := json.Marshal(result.Agenda)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: agenda is not JSON-encodable: %w", tool, err))
	}
	todos := deriveProceedTodos()
	if err := s.sessions.enterMode(sessionKey, "proceed", rawAgenda, todos); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	if input.Format == "json" {
		text, err := proceedResultJSON(result)
		return toolTextResponse(id, text, err)
	}
	return toolTextResponse(id, result.Raw, nil)
}

func (s *Server) handleEnterSprint(id json.RawMessage, args map[string]any) response {
	return s.handleEnter(id, "enter.sprint", "sprint", args, deriveSprintTodos())
}

func (s *Server) handleEnterSalvage(id json.RawMessage, args map[string]any) response {
	return s.handleEnter(id, "enter.salvage", "salvage", args, deriveSalvageTodos())
}

func (s *Server) handleTodoAppend(id json.RawMessage, args map[string]any) response {
	const tool = "todo.append"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	key, err := rawStringArg(tool, "key", args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	normalizedKey, err := normalizeTodoKey(key)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	title, _ := args["title"].(string)
	instruction, err := todoInstructionArg(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if err := s.sessions.mutateTodos(sessionKey, func(list []todoItem) ([]todoItem, error) {
		return todoAppend(list, normalizedKey, title, todoPending, instruction)
	}); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	return toolTextResponse(id, fmt.Sprintf("todo appended: %s\n", normalizedKey), nil)
}

func (s *Server) handleTodoInsert(id json.RawMessage, args map[string]any, after bool) response {
	tool := "todo.insert_before"
	if after {
		tool = "todo.insert_after"
	}
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	refKey, err := rawStringArg(tool, "ref_key", args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	key, err := rawStringArg(tool, "key", args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	normalizedKey, err := normalizeTodoKey(key)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	title, _ := args["title"].(string)
	instruction, err := todoInstructionArg(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if err := s.sessions.mutateTodos(sessionKey, func(list []todoItem) ([]todoItem, error) {
		return todoInsert(list, refKey, normalizedKey, title, todoPending, instruction, after)
	}); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	return toolTextResponse(id, fmt.Sprintf("todo inserted: %s\n", normalizedKey), nil)
}

func (s *Server) handleTodoCheck(id json.RawMessage, args map[string]any) response {
	const tool = "todo.check"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	key, err := rawStringArg(tool, "key", args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	normalizedKey, err := normalizeTodoKey(key)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	statusRaw, _ := args["status"].(string)
	status, err := parseTodoStatus(strings.TrimSpace(statusRaw))
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	if strings.TrimSpace(statusRaw) == "" {
		return toolTextResponse(id, "", fmt.Errorf("%s: status is required", tool))
	}
	updated, err := s.sessions.mutateTodosResult(sessionKey, func(list []todoItem) ([]todoItem, error) {
		return todoCheck(list, normalizedKey, status)
	})
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	return toolTextResponse(id, fmt.Sprintf("todo %s: %s\n%s\n", status, normalizedKey, renderTodosCheckpoint(updated, normalizedKey)), nil)
}

func (s *Server) handleTodoErase(id json.RawMessage, args map[string]any) response {
	const tool = "todo.erase"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	key, err := rawStringArg(tool, "key", args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	normalizedKey, err := normalizeTodoKey(key)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	if err := s.sessions.mutateTodos(sessionKey, func(list []todoItem) ([]todoItem, error) {
		return todoErase(list, normalizedKey)
	}); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	return toolTextResponse(id, fmt.Sprintf("todo erased: %s\n", normalizedKey), nil)
}

func (s *Server) handleTodoClear(id json.RawMessage, args map[string]any) response {
	const tool = "todo.clear"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	doneOnly, _ := args["done_only"].(bool)
	if err := s.sessions.mutateTodos(sessionKey, func(list []todoItem) ([]todoItem, error) {
		return todoClear(list, doneOnly), nil
	}); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	if doneOnly {
		return toolTextResponse(id, "done todos cleared\n", nil)
	}
	return toolTextResponse(id, "todos cleared\n", nil)
}

func (s *Server) handleTodoList(id json.RawMessage, args map[string]any) response {
	const tool = "todo.list"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	record, ok := s.sessions.readState(sessionKey)
	if !ok {
		return toolTextResponse(id, "", fmt.Errorf("%s: session key not found: %s", tool, sessionKey))
	}
	full := strings.TrimSpace(strings.ToLower(fmt.Sprint(args["mode"]))) == "full"
	return toolTextResponse(id, renderTodos(record.Todos, full)+"\n", nil)
}

func (s *Server) handleTodoRead(id json.RawMessage, args map[string]any) response {
	const tool = "todo.read"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	key, err := rawStringArg(tool, "key", args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	record, ok := s.sessions.readState(sessionKey)
	if !ok {
		return toolTextResponse(id, "", fmt.Errorf("%s: session key not found: %s", tool, sessionKey))
	}
	item, err := todoRead(record.Todos, key)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	return toolJSONResponse(id, item, nil)
}

func (s *Server) handleTodoReorder(id json.RawMessage, args map[string]any) response {
	const tool = "todo.reorder"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	span, _ := args["span"].(map[string]any)
	if span == nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: span {from_key, to_key} is required", tool))
	}
	fromKey, _ := span["from_key"].(string)
	toKey, _ := span["to_key"].(string)
	if fromKey == "" || toKey == "" {
		return toolTextResponse(id, "", fmt.Errorf("%s: span.from_key and span.to_key are required", tool))
	}
	position, _ := args["position"].(map[string]any)
	if position == nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: position {before|after: ref_key} is required", tool))
	}
	var refKey string
	var after bool
	if v, ok := position["before"].(string); ok && v != "" {
		refKey, after = v, false
	} else if v, ok := position["after"].(string); ok && v != "" {
		refKey, after = v, true
	} else {
		return toolTextResponse(id, "", fmt.Errorf("%s: position must set either before or after to a ref_key", tool))
	}
	if err := s.sessions.mutateTodos(sessionKey, func(list []todoItem) ([]todoItem, error) {
		return todoReorder(list, fromKey, toKey, refKey, after)
	}); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	return toolTextResponse(id, "todo span reordered\n", nil)
}
