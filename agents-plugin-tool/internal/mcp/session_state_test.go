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

func TestDeriveOtherEnterTodos(t *testing.T) {
	if !eqKeys(keysOf(deriveProceedTodos()), "route-context", "select-route", "routing-verdict", "execute-verdict") {
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
	list, err := todoAppend(nil, "a", "A", todoPending)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := todoAppend(list, "a", "dup", todoPending); err == nil {
		t.Fatal("expected duplicate key error")
	}
	// erase then re-append the same key must succeed (keys are reusable).
	list, err = todoErase(list, "a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := todoAppend(list, "a", "A again", todoPending); err != nil {
		t.Fatalf("re-append after erase failed: %v", err)
	}
}

func TestTodoInsertAndCheck(t *testing.T) {
	list, _ := todoAppend(nil, "a", "A", todoPending)
	list, _ = todoAppend(list, "c", "C", todoPending)
	list, err := todoInsert(list, "c", "b", "B", todoPending, false) // before c
	if err != nil {
		t.Fatal(err)
	}
	if !eqKeys(keysOf(list), "a", "b", "c") {
		t.Fatalf("insert_before mismatch: %v", keysOf(list))
	}
	list, err = todoInsert(list, "a", "a2", "A2", todoPending, true) // after a
	if err != nil {
		t.Fatal(err)
	}
	if !eqKeys(keysOf(list), "a", "a2", "b", "c") {
		t.Fatalf("insert_after mismatch: %v", keysOf(list))
	}
	if _, err := todoInsert(list, "missing", "x", "X", todoPending, true); err == nil {
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

func TestTodoClear(t *testing.T) {
	list, _ := todoAppend(nil, "a", "A", todoDone)
	list, _ = todoAppend(list, "b", "B", todoPending)
	list, _ = todoAppend(list, "c", "C", todoDone)
	list, _ = todoAppend(list, "d", "D", todoWip)
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
			l, _ = todoAppend(l, k, strings.ToUpper(k), todoPending)
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
	list := []todoItem{
		{Key: "a", Title: "A", Status: todoDone},
		{Key: "b", Title: "B", Status: todoDone},
		{Key: "c", Title: "C", Status: todoPending},
		{Key: "d", Title: "D", Status: todoWip},
		{Key: "e", Title: "E", Status: todoDone},
		{Key: "f", Title: "F", Status: todoDefer},
		{Key: "g", Title: "G", Status: todoDone},
	}
	// Summary: active block c,d. one context each side: b and e shown. a, f, g
	// not shown; f+g collapse to a single trailing "..." (defer collapses like done).
	wantSummary := strings.Join([]string{
		"...",
		"- [x] B",
		"- [ ] C",
		"- [~] D",
		"- [x] E",
		"...",
	}, "\n")
	if got := renderTodos(list, false); got != wantSummary {
		t.Fatalf("summary render mismatch:\n got:\n%s\nwant:\n%s", got, wantSummary)
	}
	wantFull := strings.Join([]string{
		"- [x] A", "- [x] B", "- [ ] C", "- [~] D", "- [x] E", "- [>] F", "- [x] G",
	}, "\n")
	if got := renderTodos(list, true); got != wantFull {
		t.Fatalf("full render mismatch:\n got:\n%s\nwant:\n%s", got, wantFull)
	}
	if got := renderTodos(nil, false); got != "(no todos)" {
		t.Fatalf("empty render = %q", got)
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
				return todoAppend(list, itemKey, itemKey, todoPending)
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
		return todoAppend(list, "stale", "stale", todoPending)
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
	if !strings.Contains(summary, "- [x] Route") {
		t.Fatalf("summary missing checked Route context: %s", summary)
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

	// Fresh mode: no session_key supplied.
	resp := callToolNoKey(t, server, 5001, "ws.workflow_manual", nil)

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
	// Agenda content present (the implement blob).
	if !strings.Contains(resp, "implement") {
		t.Errorf("continue mode: agenda key 'implement' absent:\n%s", resp)
	}
	// Todo summary content present (Route and Prep are active).
	if !strings.Contains(resp, "Route") {
		t.Errorf("continue mode: todo 'Route' absent from summary:\n%s", resp)
	}
	if !strings.Contains(resp, "Prep") {
		t.Errorf("continue mode: todo 'Prep' absent from summary:\n%s", resp)
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
	// Must NOT have minted a key file (check the keys dir).
	keysDir := filepath.Join(cacheDir, "keys")
	entries, err := filepath.Glob(filepath.Join(keysDir, "*.json"))
	if err != nil {
		t.Fatalf("glob keys dir: %v", err)
	}
	// No file should exist for the unknown key.
	for _, entry := range entries {
		if strings.Contains(entry, badKey) {
			t.Errorf("unknown key: key file was minted: %s", entry)
		}
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
		if strings.Contains(commitResp, "Todo (post-commit)") {
			t.Errorf("git.commit(no-todo): unexpected Todo section:\n%s", commitResp)
		}
	}
}
