package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

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

// TestTodoAndAgendaListTextTheClaudeBandParses pins the todo.list full-mode
// and agenda.list text, one item of each todo status, to the literals the
// Claude mod's status band parses (TODO_TEXT and AGENDA_TEXT in
// agents-plugin/hooks/claude-mod/band-engine.test.tsx): neither tool has a
// structured format, so a change here must change the band's parse with it.
func TestTodoAndAgendaListTextTheClaudeBandParses(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 931000, root, nil))

	id := 931001
	call := func(name string, args map[string]any) string {
		id++
		return callToolWithKey(t, server, id, key, name, args)
	}
	if got := call("todo.list", map[string]any{"mode": "full"}); got != "(no todos)\n" {
		t.Fatalf("empty todo.list full = %q", got)
	}
	if got := call("agenda.list", nil); got != "no agenda blobs\n" {
		t.Fatalf("empty agenda.list = %q", got)
	}
	call("todo.add", map[string]any{"key": "alpha", "title": "Alpha done"})
	call("todo.add", map[string]any{"key": "beta", "title": "Beta in progress", "instruction": "Beta instruction"})
	call("todo.add", map[string]any{"key": "gamma", "title": "Gamma pending"})
	call("todo.add", map[string]any{"key": "delta", "title": "Delta deferred"})
	call("todo.check", map[string]any{"key": "alpha", "status": "done"})
	call("todo.check", map[string]any{"key": "beta", "status": "wip"})
	call("todo.check", map[string]any{"key": "delta", "status": "defer"})
	call("agenda.set", map[string]any{"key": "plan", "value": "x"})
	call("agenda.set", map[string]any{"key": "implement", "value": map[string]any{"mode": "delegated"}})

	const todoText = "- [x] {alpha} Alpha done\n- [~] {beta} Beta in progress\n      Beta instruction\n- [ ] {gamma} Gamma pending\n- [>] {delta} Delta deferred\n"
	if got := call("todo.list", map[string]any{"mode": "full"}); got != todoText {
		t.Fatalf("todo.list full drifted from the band's literal:\n got: %q\nwant: %q", got, todoText)
	}
	const agendaText = "- implement: {mode}\n- plan: \"x\"\n"
	if got := call("agenda.list", nil); got != agendaText {
		t.Fatalf("agenda.list drifted from the band's literal:\n got: %q\nwant: %q", got, agendaText)
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

	record, ok := store.readState(key, partAll)
	if !ok {
		t.Fatal("readState after concurrent writes failed")
	}
	if len(record.Todos) != n {
		t.Fatalf("expected %d todos after concurrent writes, got %d", n, len(record.Todos))
	}
	// The on-disk files must remain a complete, parseable record.
	d, _ := store.dirs()
	if _, parseOK := store.readRecord(d, key, partAll); !parseOK {
		t.Fatal("on-disk record not parseable after concurrent writes")
	}
}

func TestStoreAgendaRoundTrip(t *testing.T) {
	store, key := newSandboxStore(t)
	blob := json.RawMessage(`{"mode":"implement","delegation":"delegated","need_review":true}`)
	if err := store.setAgenda(key, "implement", blob); err != nil {
		t.Fatal(err)
	}
	record, ok := store.readState(key, partAll)
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
	record, _ = store.readState(key, partAll)
	if len(record.Agenda) != 0 {
		t.Fatalf("agenda not cleared: %v", record.Agenda)
	}
}

func TestStoreMergeAgendaKeepsUntouchedNumbers(t *testing.T) {
	store, key := newSandboxStore(t)
	if err := store.setAgenda(key, "w", json.RawMessage(`{"big":12345678901234567,"f":1.10}`)); err != nil {
		t.Fatalf("setAgenda: %v", err)
	}
	if _, err := store.mergeAgendaNote(key, "w", json.RawMessage(`{"x":1}`)); err != nil {
		t.Fatalf("mergeAgendaNote: %v", err)
	}
	record, _ := store.readState(key, partAll)
	if got := string(record.Agenda["w"]); got != `{"big":12345678901234567,"f":1.10,"x":1}` {
		t.Fatalf("merge rewrote untouched numbers: %s", got)
	}
}

func TestStoreClearAllAgenda(t *testing.T) {
	store, key := newSandboxStore(t)
	if err := store.setAgenda(key, "implement", json.RawMessage(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := store.setAgenda(key, "sprint", json.RawMessage(`{"b":2}`)); err != nil {
		t.Fatal(err)
	}
	record, ok := store.readState(key, partAll)
	if !ok || len(record.Agenda) != 2 {
		t.Fatalf("expected 2 agenda blobs before clear-all: %v", record.Agenda)
	}
	if err := store.clearAllAgenda(key); err != nil {
		t.Fatal(err)
	}
	record, _ = store.readState(key, partAll)
	if len(record.Agenda) != 0 {
		t.Fatalf("clearAllAgenda did not remove every blob: %v", record.Agenda)
	}
	// clearing an already-empty agenda map is a no-op, not an error.
	if err := store.clearAllAgenda(key); err != nil {
		t.Fatalf("clearAllAgenda on empty agenda returned error: %v", err)
	}
}

func TestEnterModeReplacesTodos(t *testing.T) {
	store, key := newSandboxStore(t)
	// seed a prior list
	_ = store.mutateTodos(key, func(list []todoItem) ([]todoItem, error) {
		return todoAppend(list, "stale", "stale", todoPending, nil)
	})
	if err := store.enterMode(key, "implement", json.RawMessage(`{"x":1}`), deriveImplementTodosFromVerdict(implementTodoVerdict{NeedReview: true})); err != nil {
		t.Fatal(err)
	}
	record, _ := store.readState(key, partAll)
	if !eqKeys(keysOf(record.Todos), "route", "prep", "edit", "review", "final-action-gate", "merge") {
		t.Fatalf("enter did not replace todo list: %v", keysOf(record.Todos))
	}
	if record.Agenda["implement"] == nil {
		t.Fatal("enter did not store agenda blob")
	}
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

// implementSkipDocsArgs is the ad-hoc (inline) target: no ticket, so no route
// facts, which is the ad-hoc contract rather than a missing-facts condition.
func implementSkipDocsArgs(format string) map[string]any {
	args := map[string]any{
		"target": map[string]any{
			"kind":        "inline",
			"label":       "Tiny direct edit",
			"scope_label": "whole target",
			"scope_slug":  "tiny-direct-edit",
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

// implementReadyFacts is the fact set implementReadyArgs' ticket target
// carries. A test that varies one fact copies this map, edits the copy, and
// passes it to writeImplementReadyTicket.
func implementReadyFacts() map[string]string {
	return map[string]string{
		"scope.span":                  "multi-file",
		"scope.surface":               "public-interface",
		"scope.new_public_symbol":     "no",
		"scope.new_type_contract":     "yes",
		"scope.test_surface":          "existing",
		"complexity.reuse_points":     "unconfirmed",
		"complexity.side_effect_risk": "moderate",
		"risk.correctness":            "high",
		"risk.fit":                    "moderate",
		"risk.test":                   "moderate",
		"risk.security_or_contract":   "moderate",
	}
}

// writeImplementReadyTicket writes the ticket file implementReadyArgs' target
// points at, with facts rendered as its ## Route Facts table. This is the only
// way a run gets route facts now: they are the ticket's, not the caller's.
func writeImplementReadyTicket(t *testing.T, root string, facts map[string]string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("---\ntitle: Demo\n---\n\n# Demo\n\n## Route Facts\n\n| fact | value | evidence |\n|---|---|---|\n")
	keys := make([]string, 0, len(facts))
	for key := range facts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		b.WriteString("| " + key + " | " + facts[key] + " | fixture |\n")
	}
	b.WriteString("\n## Phases\n\n### Phase 1: Demo\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260627-feat-enter-implement-deterministic-verdict-engine.md", b.String())
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

func TestServeStdioSessionStateFlow(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 900100, root, nil))
	enableReviewPhase(t, server, key)

	args := implementSkipDocsArgs("text")
	enter := callToolWithKey(t, server, 1, key, "route.resolve_implement", args)
	if !strings.Contains(enter, "Implementation Verdict") {
		t.Fatalf("route.resolve_implement response unexpected: %s", enter)
	}

	full := callToolWithKey(t, server, 2, key, "todo.list", map[string]any{"mode": "full"})
	for _, want := range []string{"Route", "Prep", "Edit", "Review", "Final action gate", "Merge"} {
		if !strings.Contains(full, want) {
			t.Fatalf("full todo list missing %q: %s", want, full)
		}
	}
	if strings.Contains(full, "Doc pre-pass") {
		t.Fatalf("docs skipped but Doc steps present: %s", full)
	}

	if got := callToolWithKey(t, server, 3, key, "todo.check", map[string]any{"key": "route", "status": "done"}); !strings.Contains(got, "todo done: route") {
		t.Fatalf("check response unexpected: %s", got)
	}

	// After checking route done, the summary renders route with the done marker as
	// adjacent context for the still-active prep step.
	summary := callToolWithKey(t, server, 4, key, "todo.list", nil)
	if !strings.Contains(summary, "- [x] {route} Route") {
		t.Fatalf("summary missing checked Route context: %s", summary)
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
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")

	// Fresh mode: call with the reserved sentinel key (not keyless). The sentinel
	// is never minted, so the keyed lead-only gate sees a lookup-miss and skips;
	// the call reaches the handler, which maps the sentinel to fresh mode.
	resp := callToolWithKey(t, server, 5001, freshBootstrapKey, "workflow_manual", nil)
	if !strings.HasPrefix(resp, workflowManualReadFullNotice+"\n\n") {
		t.Fatalf("fresh mode must start with the full-result notice: %s", resp)
	}

	// No "Session State" section in fresh mode.
	if strings.Contains(resp, "Session State") {
		t.Errorf("fresh mode: unexpected Session State section in response:\n%s", resp)
	}
}

func TestWorkflowManualFreshModeWithRoot(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")

	// Fresh-with-root mode: sentinel key + root supplied. The handler must mint a
	// lead session key inline, strip the fresh-only block, and return Session Key
	// and Session State sections.
	resp := callToolWithKey(t, server, 5010, freshBootstrapKey, "workflow_manual", map[string]any{
		"root": root,
	})

	// (a) Session Key section must be present.
	if !strings.Contains(resp, "## Session Key") {
		t.Errorf("fresh-with-root: ## Session Key section absent from response:\n%s", resp)
	}
	// (c) Session State section must be present.
	if !strings.Contains(resp, "## Session State") {
		t.Errorf("fresh-with-root: ## Session State section absent from response:\n%s", resp)
	}
	// (d) Extract the minted key from the response and verify it resolves to a lead
	//     session entry. The key appears on the first non-empty line after the
	//     "## Session Key" heading.
	var mintedKey string
	inSessionKeySection := false
	for _, line := range strings.Split(resp, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Session Key" {
			inSessionKeySection = true
			continue
		}
		if inSessionKeySection {
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "#") {
				// Next section — key not found
				break
			}
			mintedKey = trimmed
			break
		}
	}
	if mintedKey == "" {
		t.Fatalf("fresh-with-root: could not extract minted key from response:\n%s", resp)
	}
	entry, ok := server.sessions.lookup(mintedKey)
	if !ok {
		t.Fatalf("fresh-with-root: minted key %q does not resolve to a session entry", mintedKey)
	}
	if entry.scope != roleLead {
		t.Errorf("fresh-with-root: minted key scope = %q; want %q", entry.scope, roleLead)
	}
}

func TestWorkflowManualKeylessRejected(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")

	// Keyless call must be rejected with a required session_key error.
	resp := callToolNoKey(t, server, 5002, "workflow_manual", nil)

	if !strings.Contains(resp, "session_key") {
		t.Errorf("keyless: response must mention session_key, got:\n%s", resp)
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
	rawResp := callToolOnce(t, server, 5003, "workflow_manual", map[string]any{
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
	if strings.Contains(rawResp, "Session State") {
		t.Errorf("delegate key: manual body must be absent from rejection response:\n%s", rawResp)
	}
}

func TestWorkflowManualContinueMode(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5100, root, nil))

	// Enter implement mode to populate agenda+todos.
	enter := callToolWithKey(t, server, 5101, key, "route.resolve_implement", implementSkipDocsArgs("text"))
	if !strings.Contains(enter, "Implementation Verdict") {
		t.Fatalf("route.resolve_implement unexpected: %s", enter)
	}

	// Continue mode: key present and resolves.
	resp := callToolWithKey(t, server, 5102, key, "workflow_manual", nil)

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
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5110, root, nil))
	longInstruction := "Restore this todo instruction through the workflow manual summary path without showing the extra full detail."
	wantPreview := "Restore this todo instruction through the workflow manual su"
	wantTail := "mmary path without showing the extra full detail."

	if got := callToolWithKey(t, server, 5111, key, "todo.add", map[string]any{
		"key":         "restore",
		"title":       "Restore instruction",
		"instruction": longInstruction,
	}); !strings.Contains(got, "todo added: restore") {
		t.Fatalf("append unexpected: %s", got)
	}

	resp := callToolWithKey(t, server, 5112, key, "workflow_manual", nil)
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
	resp := callToolWithKey(t, server, 5200, badKey, "workflow_manual", nil)

	// Must contain the no-restorable-state notice.
	if !strings.Contains(resp, "no restorable state for session key") {
		t.Errorf("unknown key: no-restorable-state notice absent:\n%s", resp)
	}
	// Fail-loud renders NO manual body, so the always-shown per-root rule and the
	// ws.ferrule mention it carries must be absent — a non-lead caller reaching
	// fail-loud (any unregistered key) must not learn the lead self-bootstrap call.
	if strings.Contains(resp, "ferrule") {
		t.Errorf("unknown key: manual body / ferrule mention must be absent in fail-loud:\n%s", resp)
	}
	// The recovery pointer names only the lead-revive skill (no ferrule/sentinel).
	if !strings.Contains(resp, "lead-revive") {
		t.Errorf("unknown key: fail-loud notice should point to lead-revive recovery:\n%s", resp)
	}
	// Must NOT have minted a key record. Use os.Stat on the specific record path so
	// the check is meaningful even when the sessions/ directory was never created.
	_, statErr := os.Stat(filepath.Join(cacheDir, sessionsDirName, badKey))
	if !os.IsNotExist(statErr) {
		t.Errorf("unknown key: key record was minted (stat: %v)", statErr)
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
	callToolWithKey(t, server, 5301, key, "route.resolve_implement", implementSkipDocsArgs("text"))

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
				"session_key":     key,
				"paths":           []any{"test-p3a.txt"},
				"title":           "test(p3a): re-injection test",
				"ai_context":      []any{"Phase 3a git.commit re-injection test"},
				"expected_branch": headBranch(t, root),
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

		// The session-key tip trailer must land after the TODO summary (key
		// line last, for maximal recency/salience).
		wantTip := fmt.Sprintf("tip: preserve this session key: %s during compaction", key)
		if !strings.Contains(commitResp, wantTip) {
			t.Errorf("git.commit re-injection: session-key tip absent from response:\n%s", commitResp)
		}
		if idx := strings.Index(commitResp, "## TODO("); idx == -1 || strings.Index(commitResp, wantTip) < idx {
			t.Errorf("git.commit re-injection: session-key tip must follow the TODO summary trailer:\n%s", commitResp)
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
				"session_key":     key2,
				"paths":           []any{"test-p3a-notodo.txt"},
				"title":           "test(p3a): no-todo commit",
				"ai_context":      []any{"no-todo test"},
				"expected_branch": headBranch(t, root2),
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
		// The session-key tip still appends even with no todos.
		wantTip := fmt.Sprintf("tip: preserve this session key: %s during compaction", key2)
		if !strings.Contains(commitResp, wantTip) {
			t.Errorf("git.commit(no-todo): session-key tip absent from response:\n%s", commitResp)
		}
	}
}

func TestAppendSessionKeyTip(t *testing.T) {
	const key = "scarily-imminent-ploy-plated-84"
	withKey := appendSessionKeyTip("commit ok", key)
	wantTip := fmt.Sprintf("tip: preserve this session key: %s during compaction", key)
	if !strings.Contains(withKey, wantTip) {
		t.Errorf("appendSessionKeyTip: tip absent for non-empty key:\n%s", withKey)
	}
	if !strings.HasPrefix(withKey, "commit ok") {
		t.Errorf("appendSessionKeyTip: original text not preserved:\n%s", withKey)
	}

	noKey := appendSessionKeyTip("commit ok", "")
	if noKey != "commit ok" {
		t.Errorf("appendSessionKeyTip: expected no-op for empty key, got:\n%s", noKey)
	}
}

// --- 260702: ws.workflow_state integration tests -----------------------------

func TestWorkflowStateReturnsSessionStateOnly(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5500, root, nil))

	// Enter implement mode to populate agenda+todos.
	enter := callToolWithKey(t, server, 5501, key, "route.resolve_implement", implementSkipDocsArgs("text"))
	if !strings.Contains(enter, "Implementation Verdict") {
		t.Fatalf("route.resolve_implement unexpected: %s", enter)
	}

	manualResp := callToolWithKey(t, server, 5502, key, "workflow_manual", nil)
	stateResp := callToolWithKey(t, server, 5503, key, "workflow_state", nil)

	// The workflow_state output must exactly equal the "## Session State" section
	// workflow_manual renders for the same session at the same point in time.
	manualIdx := strings.Index(manualResp, "## Session State")
	if manualIdx < 0 {
		t.Fatalf("workflow_manual response missing Session State section:\n%s", manualResp)
	}
	wantState := manualResp[manualIdx:]
	if stateResp != wantState {
		t.Fatalf("workflow_state mismatch.\ngot:\n%s\nwant (workflow_manual's Session State suffix):\n%s", stateResp, wantState)
	}

	// No manual reference/primitives text: workflow_state must be substantially
	// shorter than the full workflow_manual render, and must not carry manual-only
	// fragments (e.g. the always-shown per-root rule or ferrule mention).
	if len(stateResp) >= len(manualResp) {
		t.Fatalf("workflow_state (%d bytes) not shorter than workflow_manual (%d bytes)", len(stateResp), len(manualResp))
	}
	if strings.Contains(stateResp, "ferrule") {
		t.Errorf("workflow_state leaked manual body content:\n%s", stateResp)
	}

	// Session State content itself is present (agenda + todos).
	if !strings.Contains(stateResp, "### agenda: implement") {
		t.Errorf("workflow_state missing agenda heading:\n%s", stateResp)
	}
	if !strings.Contains(stateResp, "### Todos") {
		t.Errorf("workflow_state missing Todos heading:\n%s", stateResp)
	}
}

// TestWorkflowStateByteIdenticalToWorkflowManualWhenNotesPresent is the
// notes-present sibling of TestWorkflowStateReturnsSessionStateOnly: once
// facet B (260823) makes wsnote.Compute always render a non-empty "# Notes"
// block, workflow_state's resolved-record branch must append the identical
// computeNotes(rec.Root) text workflow_manual's CONTINUE branch does, or the
// two responses silently diverge only when a note actually exists.
func TestWorkflowStateByteIdenticalToWorkflowManualWhenNotesPresent(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5505, root, nil))

	callToolWithKey(t, server, 5506, key, "note.write", map[string]any{
		"layer": "worktree",
		"notes": []any{map[string]any{"key": "state.parity", "value": "seen by both tools", "priority": 1}},
	})

	manualResp := callToolWithKey(t, server, 5507, key, "workflow_manual", nil)
	stateResp := callToolWithKey(t, server, 5508, key, "workflow_state", nil)

	manualIdx := strings.Index(manualResp, "## Session State")
	if manualIdx < 0 {
		t.Fatalf("workflow_manual response missing Session State section:\n%s", manualResp)
	}
	wantState := manualResp[manualIdx:]
	if stateResp != wantState {
		t.Fatalf("workflow_state mismatch with notes present.\ngot:\n%s\nwant (workflow_manual's Session State suffix):\n%s", stateResp, wantState)
	}
	if !strings.Contains(stateResp, "state.parity") || !strings.Contains(stateResp, "seen by both tools") {
		t.Fatalf("workflow_state missing the written note:\n%s", stateResp)
	}
}

func TestWorkflowStateEmptySessionReturnsEmptyPayloadNotError(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5510, root, nil))

	// No enter.* call, no todos: session exists but has empty agenda/todos.
	resp := callToolWithKey(t, server, 5511, key, "workflow_state", nil)

	if !strings.Contains(resp, "## Session State") {
		t.Fatalf("workflow_state empty session missing Session State heading:\n%s", resp)
	}
	if !strings.Contains(resp, "(no todos)") {
		t.Errorf("workflow_state empty session missing empty-todos marker:\n%s", resp)
	}
	// Must not be the fail-loud no-restorable-state notice — this is a resolved,
	// merely-empty session, not an unknown key.
	if strings.Contains(resp, "no restorable state for session key") {
		t.Errorf("workflow_state empty session incorrectly returned fail-loud notice:\n%s", resp)
	}
}

func TestWorkflowStateKeylessRejected(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")

	resp := callToolNoKey(t, server, 5520, "workflow_state", nil)

	if !strings.Contains(resp, "session_key") {
		t.Errorf("keyless: response must mention session_key, got:\n%s", resp)
	}
	if strings.Contains(resp, "mint your lead key") {
		t.Errorf("keyless: self-bootstrap fragment must be absent from error response:\n%s", resp)
	}
}

func TestWorkflowStateDelegateKeyBlockedSameAsWorkflowManual(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")

	delegateKey, err := server.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatalf("mint delegate key: %v", err)
	}

	manualRawResp := callToolOnce(t, server, 5530, "workflow_manual", map[string]any{
		"session_key": delegateKey,
	})
	stateRawResp := callToolOnce(t, server, 5531, "workflow_state", map[string]any{
		"session_key": delegateKey,
	})

	// Same lead-only profile rejection shape (RPC-level -32601 error) as
	// workflow_manual for a non-lead scoped key.
	for _, rawResp := range []string{manualRawResp, stateRawResp} {
		if !strings.Contains(rawResp, "tool not available in current") {
			t.Errorf("delegate key: expected lead-only rejection, got:\n%s", rawResp)
		}
		if !strings.Contains(rawResp, "-32601") {
			t.Errorf("delegate key: expected JSON-RPC error code -32601, got:\n%s", rawResp)
		}
	}
	if strings.Contains(stateRawResp, "Session State") {
		t.Errorf("delegate key: workflow_state body must be absent from rejection response:\n%s", stateRawResp)
	}
}

func TestWorkflowStateUnknownKeySameFailLoudAsWorkflowManual(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheDir)

	server := NewServer(root, "test")

	badKey := "no-such-key-here"
	manualResp := callToolWithKey(t, server, 5540, badKey, "workflow_manual", nil)
	stateResp := callToolWithKey(t, server, 5541, badKey, "workflow_state", nil)

	// workflow_state must produce the identical fail-loud notice text that
	// workflow_manual renders for the same unresolvable key (both end with the
	// same "## Session State ... no restorable state ..." notice).
	manualIdx := strings.Index(manualResp, "## Session State")
	if manualIdx < 0 {
		t.Fatalf("workflow_manual fail-loud response missing Session State heading:\n%s", manualResp)
	}
	wantNotice := manualResp[manualIdx:]
	if stateResp != wantNotice {
		t.Fatalf("workflow_state fail-loud mismatch.\ngot:\n%s\nwant (workflow_manual's fail-loud notice):\n%s", stateResp, wantNotice)
	}
	if !strings.Contains(stateResp, "lead-revive") {
		t.Errorf("unknown key: workflow_state fail-loud notice should point to lead-revive recovery:\n%s", stateResp)
	}
	if strings.Contains(stateResp, "ferrule") || strings.Contains(stateResp, "one Git worktree") {
		t.Errorf("unknown key: workflow_state must not leak manual body / ferrule mention:\n%s", stateResp)
	}

	// Must NOT have minted a key record.
	_, statErr := os.Stat(filepath.Join(cacheDir, sessionsDirName, badKey))
	if !os.IsNotExist(statErr) {
		t.Errorf("unknown key: key record was minted (stat: %v)", statErr)
	}
}

func TestWorkflowStateFreshSentinelIsFailLoudNotFresh(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")

	// The fresh-bootstrap sentinel is never a stored record; workflow_state has
	// no FRESH mode, so it must fall through to the same fail-loud path as any
	// other unresolvable key rather than minting or rendering manual content.
	resp := callToolWithKey(t, server, 5550, freshBootstrapKey, "workflow_state", nil)

	if !strings.Contains(resp, "no restorable state for session key") {
		t.Errorf("sentinel key: expected fail-loud notice, got:\n%s", resp)
	}
	if strings.Contains(resp, "mint your lead key") {
		t.Errorf("sentinel key: workflow_state must not render fresh-mode manual content:\n%s", resp)
	}
}

func TestWorkflowStateToolSchema(t *testing.T) {
	useLeadProfile(t)
	server := NewServer(t.TempDir(), "test")
	properties := toolPropertiesByName(t, callToolsList(t, server), "workflow_state")
	if _, ok := properties["session_key"]; !ok {
		t.Fatalf("workflow_state schema missing session_key property: %#v", properties)
	}
}

// enableReviewPhase opts key into the full per-phase review (review_phase=full
// at session scope) for tests whose subject is the risk-keyed review
// allocation or its installed two-round review todo, which review_phase's
// builtin lite would otherwise replace with the single lite reviewer.
func enableReviewPhase(t *testing.T, server *Server, key string) {
	t.Helper()
	if err := server.sessions.setOverride(key, wsconfig.ItemReviewPhase, "full"); err != nil {
		t.Fatalf("enable review_phase: %v", err)
	}
}
