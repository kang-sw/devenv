package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeStdioTodoKeyNormalization(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 903000, root, nil))

	if got := callToolWithKey(t, server, 1, key, "todo.add", map[string]any{
		"key": "Review.Step_1", "title": "Review",
	}); !strings.Contains(got, "todo added: review.step_1") {
		t.Fatalf("append did not report normalized key: %s", got)
	}
	if got := callToolWithKey(t, server, 2, key, "todo.add", map[string]any{
		"key": "review.step_1", "title": "dup",
	}); !strings.Contains(got, "already exists") {
		t.Fatalf("duplicate-after-normalization error expected, got: %s", got)
	}
	if got := callToolWithKey(t, server, 3, key, "todo.check", map[string]any{
		"key": "REVIEW.STEP_1", "status": "done",
	}); !strings.Contains(got, "todo done: review.step_1") {
		t.Fatalf("check did not normalize lookup key: %s", got)
	}
	if full := callToolWithKey(t, server, 4, key, "todo.list", map[string]any{"mode": "full"}); !strings.Contains(full, "- [x] {review.step_1} Review") {
		t.Fatalf("full list missing normalized rendered key: %s", full)
	}
	if got := callToolWithKey(t, server, 5, key, "todo.add", map[string]any{
		"key": "{bad}", "title": "bad",
	}); !strings.Contains(got, "invalid character") {
		t.Fatalf("invalid key error expected, got: %s", got)
	}
	if got := callToolWithKey(t, server, 6, key, "todo.add", map[string]any{
		"key": " Review ", "title": "bad",
	}); !strings.Contains(got, "leading or trailing whitespace") {
		t.Fatalf("whitespace key error expected, got: %s", got)
	}
}

func TestServeStdioTodoAddErrorBranches(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 903200, root, nil))

	// title is advertised as required. Omitted and empty values must fail before
	// either append or insert can mutate the list.
	if got := callToolWithKey(t, server, 0, key, "todo.add", map[string]any{
		"key": "missing-title",
	}); !strings.Contains(got, "missing required: title") {
		t.Fatalf("missing title error expected, got: %s", got)
	}
	if got := callToolWithKey(t, server, 0, key, "todo.add", map[string]any{
		"key": "empty-title", "title": "",
	}); !strings.Contains(got, "todo.add: title is required") {
		t.Fatalf("empty title error expected, got: %s", got)
	}
	if got := callToolWithKey(t, server, 0, key, "todo.add", map[string]any{
		"key": "valid-title", "title": "Valid title",
	}); !strings.Contains(got, "todo added: valid-title") {
		t.Fatalf("valid title should be accepted, got: %s", got)
	}

	// position not in the end|before|after enum.
	if got := callToolWithKey(t, server, 1, key, "todo.add", map[string]any{
		"key": "x", "title": "X", "position": "middle",
	}); !strings.Contains(got, "todo.add: position must be one of end, before, after") {
		t.Fatalf("bad position error expected, got: %s", got)
	}

	// ref_key missing entirely for position before/after.
	if got := callToolWithKey(t, server, 2, key, "todo.add", map[string]any{
		"key": "x", "title": "X", "position": "before",
	}); !strings.Contains(got, "todo.add: ref_key is required when position is before") {
		t.Fatalf("missing ref_key(before) error expected, got: %s", got)
	}
	if got := callToolWithKey(t, server, 3, key, "todo.add", map[string]any{
		"key": "x", "title": "X", "position": "after",
	}); !strings.Contains(got, "todo.add: ref_key is required when position is after") {
		t.Fatalf("missing ref_key(after) error expected, got: %s", got)
	}

	// ref_key present but empty for before/after -> same "required" message
	// (missing and empty are equivalent per the ticket's fail-loud wording).
	if got := callToolWithKey(t, server, 4, key, "todo.add", map[string]any{
		"key": "x", "title": "X", "position": "before", "ref_key": "",
	}); !strings.Contains(got, "todo.add: ref_key is required when position is before") {
		t.Fatalf("empty ref_key(before) error expected, got: %s", got)
	}

	// ref_key supplied for position: end -> rejected, both with an explicit
	// "end" and with position omitted entirely (implicit default).
	if got := callToolWithKey(t, server, 5, key, "todo.add", map[string]any{
		"key": "x", "title": "X", "position": "end", "ref_key": "whatever",
	}); !strings.Contains(got, "todo.add: ref_key must be omitted when position is end") {
		t.Fatalf("ref_key-for-end(explicit) error expected, got: %s", got)
	}
	if got := callToolWithKey(t, server, 6, key, "todo.add", map[string]any{
		"key": "x", "title": "X", "ref_key": "whatever",
	}); !strings.Contains(got, "todo.add: ref_key must be omitted when position is end") {
		t.Fatalf("ref_key-for-end(implicit) error expected, got: %s", got)
	}

	// ref_key supplied but EMPTY for position: end -> still rejected (both
	// explicit "end" and implicit/omitted position). This is the load-bearing
	// case for the comma-ok `args["ref_key"]` presence check in handleTodoAdd:
	// a regression back to rawStringArg's empty-string-means-missing collapse
	// would silently accept this and must be caught here.
	if got := callToolWithKey(t, server, 601, key, "todo.add", map[string]any{
		"key": "x", "title": "X", "position": "end", "ref_key": "",
	}); !strings.Contains(got, "todo.add: ref_key must be omitted when position is end") {
		t.Fatalf("empty ref_key-for-end(explicit) error expected, got: %s", got)
	}
	if got := callToolWithKey(t, server, 602, key, "todo.add", map[string]any{
		"key": "x", "title": "X", "ref_key": "",
	}); !strings.Contains(got, "todo.add: ref_key must be omitted when position is end") {
		t.Fatalf("empty ref_key-for-end(implicit) error expected, got: %s", got)
	}

	// None of the error paths above should have mutated the list beyond the
	// one valid-title control item.
	if rec, ok := server.sessions.readState(key, partAll); !ok || len(rec.Todos) != 1 || rec.Todos[0].Key != "valid-title" || rec.Todos[0].Title != "Valid title" {
		t.Fatalf("error paths must not mutate the todo list: %+v", rec.Todos)
	}

	// Pin the other side of the comma-ok distinction: an ABSENT ref_key for
	// position: end must still be accepted, both with an explicit "end" and
	// with position omitted entirely (implicit default).
	if got := callToolWithKey(t, server, 603, key, "todo.add", map[string]any{
		"key": "end-explicit-no-ref", "title": "T", "position": "end",
	}); !strings.Contains(got, "todo added: end-explicit-no-ref") {
		t.Fatalf("absent ref_key with explicit position=end should be accepted, got: %s", got)
	}
	if got := callToolWithKey(t, server, 604, key, "todo.add", map[string]any{
		"key": "end-implicit-no-ref", "title": "T",
	}); !strings.Contains(got, "todo added: end-implicit-no-ref") {
		t.Fatalf("absent ref_key with implicit (omitted) position should be accepted, got: %s", got)
	}

	// Reused verbatim error messages (from todoAppend/todoInsert/normalizeTodoKey)
	// still fire through the new todo.add dispatch, wrapped under the new tool name.
	if got := callToolWithKey(t, server, 7, key, "todo.add", map[string]any{
		"key": "seed", "title": "Seed",
	}); !strings.Contains(got, "todo added: seed") {
		t.Fatalf("seed add unexpected: %s", got)
	}
	if got := callToolWithKey(t, server, 8, key, "todo.add", map[string]any{
		"key": "seed", "title": "dup",
	}); !strings.Contains(got, `todo key "seed" already exists`) {
		t.Fatalf("duplicate key error expected, got: %s", got)
	}
	if got := callToolWithKey(t, server, 9, key, "todo.add", map[string]any{
		"key": "{bad}", "title": "bad",
	}); !strings.Contains(got, "invalid character") {
		t.Fatalf("invalid key format error expected, got: %s", got)
	}
	if got := callToolWithKey(t, server, 10, key, "todo.add", map[string]any{
		"key": " seed ", "title": "bad",
	}); !strings.Contains(got, "leading or trailing whitespace") {
		t.Fatalf("whitespace key error expected, got: %s", got)
	}
	if got := callToolWithKey(t, server, 11, key, "todo.add", map[string]any{
		"key": "missing-ref", "title": "T", "position": "before", "ref_key": "does-not-exist",
	}); !strings.Contains(got, `ref_key "does-not-exist" not found`) {
		t.Fatalf("unknown ref_key error expected, got: %s", got)
	}
	if got := callToolWithKey(t, server, 12, key, "todo.add", map[string]any{
		"key": "bad-instruction", "title": "T", "instruction": 42,
	}); !strings.Contains(got, "instruction must be a string or null") {
		t.Fatalf("bad instruction type error expected, got: %s", got)
	}
}

// TestServeStdioTodoAddReproducesOldPlacements is the byte-identical mutation
// backstop for the merge: todo.add(position: end) and the pre-merge append
// tool must produce the same list state, and todo.add(position: before|after,
// ref_key) must match the pre-merge insert-before/after core-function
// behavior already proven by TestTodoInsertAndCheck. This verifies the
// dispatch-level wiring reuses todoAppend/todoInsert unchanged.
func TestServeStdioTodoAddReproducesOldPlacements(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 903300, root, nil))

	// end (equivalent to the old plain-append tool): a, then c.
	if got := callToolWithKey(t, server, 1, key, "todo.add", map[string]any{
		"key": "a", "title": "A",
	}); !strings.Contains(got, "todo added: a") {
		t.Fatalf("add(end) a unexpected: %s", got)
	}
	if got := callToolWithKey(t, server, 2, key, "todo.add", map[string]any{
		"key": "c", "title": "C",
	}); !strings.Contains(got, "todo added: c") {
		t.Fatalf("add(end) c unexpected: %s", got)
	}
	if rec, ok := server.sessions.readState(key, partAll); !ok || !eqKeys(keysOf(rec.Todos), "a", "c") {
		t.Fatalf("post-append order wrong: %v", keysOf(rec.Todos))
	}

	// before (equivalent to the old insert-before tool): insert b before c -> a,b,c.
	if got := callToolWithKey(t, server, 3, key, "todo.add", map[string]any{
		"key": "b", "title": "B", "position": "before", "ref_key": "c",
	}); !strings.Contains(got, "todo added: b") {
		t.Fatalf("add(before) b unexpected: %s", got)
	}
	if rec, ok := server.sessions.readState(key, partAll); !ok || !eqKeys(keysOf(rec.Todos), "a", "b", "c") {
		t.Fatalf("post-insert_before order wrong: %v", keysOf(rec.Todos))
	}

	// after (equivalent to the old insert-after tool): insert a2 after a -> a,a2,b,c.
	if got := callToolWithKey(t, server, 4, key, "todo.add", map[string]any{
		"key": "a2", "title": "A2", "position": "after", "ref_key": "a",
	}); !strings.Contains(got, "todo added: a2") {
		t.Fatalf("add(after) a2 unexpected: %s", got)
	}
	rec, ok := server.sessions.readState(key, partAll)
	if !ok || !eqKeys(keysOf(rec.Todos), "a", "a2", "b", "c") {
		t.Fatalf("post-insert_after order wrong: %v", keysOf(rec.Todos))
	}
	// This is the exact list shape TestTodoInsertAndCheck proves for the
	// unchanged todoAppend/todoInsert cores called directly — confirming the
	// dispatch-level todo.add reproduces the pre-merge tools' mutations
	// byte-for-byte, not just "no error".
	for _, item := range rec.Todos {
		if item.Status != todoPending {
			t.Fatalf("newly added item %q must be pending, got %v", item.Key, item.Status)
		}
	}
}

func TestServeStdioTodoInstructionReadSurface(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 903100, root, nil))

	if got := callToolWithKey(t, server, 1, key, "todo.add", map[string]any{
		"key":         "a",
		"title":       "A",
		"instruction": "Alpha full instruction",
	}); !strings.Contains(got, "todo added: a") {
		t.Fatalf("append with instruction unexpected: %s", got)
	}
	if got := callToolWithKey(t, server, 2, key, "todo.add", map[string]any{
		"position":    "before",
		"ref_key":     "a",
		"key":         "b",
		"title":       "B",
		"instruction": nil,
	}); !strings.Contains(got, "todo added: b") {
		t.Fatalf("insert_before null instruction unexpected: %s", got)
	}
	if got := callToolWithKey(t, server, 3, key, "todo.add", map[string]any{
		"position":    "after",
		"ref_key":     "a",
		"key":         "c",
		"title":       "C",
		"instruction": "Charlie full instruction",
	}); !strings.Contains(got, "todo added: c") {
		t.Fatalf("insert_after with instruction unexpected: %s", got)
	}
	if got := callToolWithKey(t, server, 4, key, "todo.check", map[string]any{
		"key": "a", "status": "done",
	}); !strings.Contains(got, "todo done: a") {
		t.Fatalf("check unexpected: %s", got)
	}
	if got := callToolWithKey(t, server, 5, key, "todo.reorder", map[string]any{
		"span":     map[string]any{"from_key": "a", "to_key": "c"},
		"position": map[string]any{"before": "b"},
	}); !strings.Contains(got, "todo span reordered") {
		t.Fatalf("reorder unexpected: %s", got)
	}

	readA := callToolWithKey(t, server, 6, key, "todo.read", map[string]any{"key": "A"})
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

	readB := callToolWithKey(t, server, 7, key, "todo.read", map[string]any{"key": "b"})
	var nullPayload todoReadPayload
	if err := json.Unmarshal([]byte(readB), &nullPayload); err != nil {
		t.Fatalf("null read payload did not parse: %v\n%s", err, readB)
	}
	if nullPayload.Instruction != nil {
		t.Fatalf("null instruction = %#v, want nil", nullPayload.Instruction)
	}

	if got := callToolWithKey(t, server, 8, key, "todo.add", map[string]any{
		"key": "bad", "title": "Bad", "instruction": []any{"not", "string"},
	}); !strings.Contains(got, "instruction must be a string or null") {
		t.Fatalf("invalid instruction error expected, got: %s", got)
	}
	if got := callToolWithKey(t, server, 9, key, "todo.read", map[string]any{"key": "missing"}); !strings.Contains(got, `todo key "missing" not found`) {
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

	if got := callToolWithKey(t, server, 1, key, "todo.add", map[string]any{
		"key":         "render",
		"title":       "Render instruction",
		"instruction": longInstruction,
	}); !strings.Contains(got, "todo added: render") {
		t.Fatalf("append unexpected: %s", got)
	}

	summary := callToolWithKey(t, server, 2, key, "todo.list", nil)
	if !strings.Contains(summary, "- [ ] {render} Render instruction\n      "+wantPreview) {
		t.Fatalf("summary list missing instruction preview:\n%s", summary)
	}
	if strings.Contains(summary, wantTail) {
		t.Fatalf("summary list rendered instruction tail, want preview only:\n%s", summary)
	}

	full := callToolWithKey(t, server, 3, key, "todo.list", map[string]any{"mode": "full"})
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
		if got := callToolWithKey(t, server, i+1, key, "todo.add", map[string]any{
			"key": item.key, "title": item.title, "instruction": item.instruction,
		}); !strings.Contains(got, "todo added: "+item.key) {
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
		got := callToolWithKey(t, server, 10+i, key, "todo.check", map[string]any{
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
		if tool["name"] == "todo.check" {
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
		if got := callToolWithKey(t, server, 1000+i, key, "todo.add", map[string]any{
			"key": k, "title": strings.ToUpper(k),
		}); !strings.Contains(got, "todo added: "+k) {
			t.Fatalf("append %s unexpected: %s", k, got)
		}
	}

	// position {after: ...}: move span [a,b] after d -> c,d,a,b
	if got := callToolWithKey(t, server, 1100, key, "todo.reorder", map[string]any{
		"span":     map[string]any{"from_key": "a", "to_key": "b"},
		"position": map[string]any{"after": "d"},
	}); !strings.Contains(got, "todo span reordered") {
		t.Fatalf("reorder(after) unexpected: %s", got)
	}
	if rec, ok := server.sessions.readState(key, partAll); !ok || !eqKeys(keysOf(rec.Todos), "c", "d", "a", "b") {
		t.Fatalf("reorder(after) on-disk order wrong: %v", keysOf(rec.Todos))
	}

	// position {before: ...}: move span [a,b] before c -> a,b,c,d
	if got := callToolWithKey(t, server, 1101, key, "todo.reorder", map[string]any{
		"span":     map[string]any{"from_key": "a", "to_key": "b"},
		"position": map[string]any{"before": "c"},
	}); !strings.Contains(got, "todo span reordered") {
		t.Fatalf("reorder(before) unexpected: %s", got)
	}
	if rec, ok := server.sessions.readState(key, partAll); !ok || !eqKeys(keysOf(rec.Todos), "a", "b", "c", "d") {
		t.Fatalf("reorder(before) on-disk order wrong: %v", keysOf(rec.Todos))
	}

	// Malformed: missing span -> compact error, list unchanged.
	if got := callToolWithKey(t, server, 1102, key, "todo.reorder", map[string]any{
		"position": map[string]any{"before": "c"},
	}); !strings.Contains(got, "missing required: span") {
		t.Fatalf("missing-span error expected, got: %s", got)
	}
	// Malformed: missing position -> compact error.
	if got := callToolWithKey(t, server, 1103, key, "todo.reorder", map[string]any{
		"span": map[string]any{"from_key": "a", "to_key": "b"},
	}); !strings.Contains(got, "missing required: position") {
		t.Fatalf("missing-position error expected, got: %s", got)
	}
	// Malformed: position with neither before nor after -> compact error.
	if got := callToolWithKey(t, server, 1104, key, "todo.reorder", map[string]any{
		"span":     map[string]any{"from_key": "a", "to_key": "b"},
		"position": map[string]any{"sideways": "c"},
	}); !strings.Contains(got, "position must set either before or after") {
		t.Fatalf("empty-position error expected, got: %s", got)
	}
	// List must still be intact after the error paths.
	if rec, ok := server.sessions.readState(key, partAll); !ok || !eqKeys(keysOf(rec.Todos), "a", "b", "c", "d") {
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
	if got := callToolWithKey(t, server, 2000, key, "agenda.set", map[string]any{
		"key":   "notes",
		"value": map[string]any{"mode": "implement", "tags": []any{"x", "y"}},
	}); !strings.Contains(got, "agenda set: notes") {
		t.Fatalf("agenda.set unexpected: %s", got)
	}
	rec, ok := server.sessions.readState(key, partAll)
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
	if got := callToolWithKey(t, server, 2001, key, "agenda.set", map[string]any{
		"key": "notes",
	}); !strings.Contains(got, "missing required: value") {
		t.Fatalf("missing-value error expected, got: %s", got)
	}

	// clear: removes the blob through the handler.
	if got := callToolWithKey(t, server, 2002, key, "agenda.clear", map[string]any{
		"key": "notes",
	}); !strings.Contains(got, "agenda cleared: notes") {
		t.Fatalf("agenda.clear unexpected: %s", got)
	}
	rec, _ = server.sessions.readState(key, partAll)
	if len(rec.Agenda) != 0 {
		t.Fatalf("agenda not cleared via handler: %v", rec.Agenda)
	}
}

func TestServeStdioAgendaListHandler(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 903000, root, nil))

	// empty agenda -> explicit no-blobs message, not an error.
	if got := callToolWithKey(t, server, 3000, key, "agenda.list", map[string]any{}); !strings.Contains(got, "no agenda blobs") {
		t.Fatalf("agenda.list on empty agenda unexpected: %s", got)
	}

	if got := callToolWithKey(t, server, 3001, key, "agenda.set", map[string]any{
		"key":   "implement",
		"value": map[string]any{"delegation": "direct-edit"},
	}); !strings.Contains(got, "agenda set: implement") {
		t.Fatalf("agenda.set unexpected: %s", got)
	}
	if got := callToolWithKey(t, server, 3002, key, "agenda.set", map[string]any{
		"key":   "sprint",
		"value": map[string]any{"episode": "edit"},
	}); !strings.Contains(got, "agenda set: sprint") {
		t.Fatalf("agenda.set unexpected: %s", got)
	}

	// list must enumerate both keys, sorted alphabetically, each with a summary.
	got := callToolWithKey(t, server, 3003, key, "agenda.list", map[string]any{})
	implementIdx := strings.Index(got, "implement")
	sprintIdx := strings.Index(got, "sprint")
	if implementIdx == -1 || sprintIdx == -1 {
		t.Fatalf("agenda.list missing expected keys: %s", got)
	}
	if implementIdx > sprintIdx {
		t.Fatalf("agenda.list keys not sorted alphabetically: %s", got)
	}
	if !strings.Contains(got, "delegation") || !strings.Contains(got, "episode") {
		t.Fatalf("agenda.list missing per-blob summaries: %s", got)
	}

	// merge: the merged blob's top-level keys show through agenda.list.
	if got := callToolWithKey(t, server, 3007, key, "agenda.set", map[string]any{
		"key":   "sprint",
		"merge": true,
		"value": map[string]any{"episode": nil, "phase": "review"},
	}); !strings.Contains(got, "agenda set: sprint") {
		t.Fatalf("agenda.set merge unexpected: %s", got)
	}
	got = callToolWithKey(t, server, 3008, key, "agenda.list", map[string]any{})
	if !strings.Contains(got, "phase") || strings.Contains(got, "episode") {
		t.Fatalf("agenda.list does not show the merged blob: %s", got)
	}

	// unknown session key -> compact error, not a silent empty list.
	if got := callToolWithKey(t, server, 3004, "not-a-real-key", "agenda.list", map[string]any{}); !strings.Contains(got, "session key not found") {
		t.Fatalf("agenda.list with bad key expected session-not-found error, got: %s", got)
	}

	// clear(all: true) removes every blob in one call.
	if got := callToolWithKey(t, server, 3005, key, "agenda.clear", map[string]any{
		"all": true,
	}); !strings.Contains(got, "agenda cleared: all") {
		t.Fatalf("agenda.clear(all) unexpected: %s", got)
	}
	rec, ok := server.sessions.readState(key, partAll)
	if !ok || len(rec.Agenda) != 0 {
		t.Fatalf("agenda.clear(all) did not clear every blob: %v", rec.Agenda)
	}
	if got := callToolWithKey(t, server, 3006, key, "agenda.list", map[string]any{}); !strings.Contains(got, "no agenda blobs") {
		t.Fatalf("agenda.list after clear(all) unexpected: %s", got)
	}
}

func TestServeStdioAgendaSetMerge(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 904000, root, nil))

	id := 4000
	set := func(blobKey string, args map[string]any) string {
		t.Helper()
		id++
		args["key"] = blobKey
		return callToolWithKey(t, server, id, key, "agenda.set", args)
	}
	stored := func(blobKey string) string {
		t.Helper()
		rec, ok := server.sessions.readState(key, partAll)
		if !ok {
			t.Fatalf("session record missing")
		}
		raw, ok := rec.Agenda[blobKey]
		if !ok {
			return "<absent>"
		}
		return string(raw)
	}

	base := map[string]any{
		"keep":  "k",
		"drop":  "d",
		"nest":  map[string]any{"a": 1, "b": 2},
		"list":  []any{1, 2, 3},
		"count": 7,
		"flag":  true,
	}
	if got := set("w", map[string]any{"value": base}); !strings.Contains(got, "agenda set: w") {
		t.Fatalf("seed agenda.set unexpected: %s", got)
	}

	// field set, null removal, recursive merge keeping siblings, and whole
	// replacement of array and scalar fields.
	if got := set("w", map[string]any{"merge": true, "value": map[string]any{
		"added": "new",
		"drop":  nil,
		"nest":  map[string]any{"b": nil, "c": 3},
		"list":  []any{9},
		"flag":  "now-a-string",
	}}); !strings.Contains(got, "agenda set: w") {
		t.Fatalf("merge agenda.set unexpected: %s", got)
	}
	want := `{"added":"new","count":7,"flag":"now-a-string","keep":"k","list":[9],"nest":{"a":1,"c":3}}`
	if got := stored("w"); got != want {
		t.Fatalf("merged blob mismatch\n got: %s\nwant: %s", got, want)
	}

	// an object patch onto a stored scalar field replaces it with the patch
	// applied to {}.
	set("w", map[string]any{"merge": true, "value": map[string]any{"keep": map[string]any{"x": 1, "y": nil}}})
	if got := stored("w"); !strings.Contains(got, `"keep":{"x":1}`) {
		t.Fatalf("object patch over scalar field not applied to {}: %s", got)
	}

	// missing key: created from the patch with null fields dropped.
	set("fresh", map[string]any{"merge": true, "value": map[string]any{"a": 1, "gone": nil, "o": map[string]any{"z": nil}}})
	if got := stored("fresh"); got != `{"a":1,"o":{}}` {
		t.Fatalf("missing key not created from patch: %s", got)
	}

	// a stored non-object blob is treated as {} and replaced by the result.
	set("scalar", map[string]any{"value": []any{"x"}})
	set("scalar", map[string]any{"merge": true, "value": map[string]any{"a": "b"}})
	if got := stored("scalar"); got != `{"a":"b"}` {
		t.Fatalf("non-object stored blob not replaced: %s", got)
	}

	// merge absent or false keeps replace behavior.
	set("w", map[string]any{"value": map[string]any{"only": 1}})
	if got := stored("w"); got != `{"only":1}` {
		t.Fatalf("plain set did not replace: %s", got)
	}
	set("w", map[string]any{"merge": false, "value": map[string]any{"other": 2}})
	if got := stored("w"); got != `{"other":2}` {
		t.Fatalf("merge:false did not replace: %s", got)
	}

	// merge with a non-object or null value is rejected and stores nothing.
	for _, bad := range []any{nil, []any{1}, "s", 3, true} {
		got := set("w", map[string]any{"merge": true, "value": bad})
		if !strings.Contains(got, "value must be a JSON object when merge is true") {
			t.Fatalf("merge with %#v not rejected: %s", bad, got)
		}
		if got := stored("w"); got != `{"other":2}` {
			t.Fatalf("rejected merge changed the stored blob: %s", got)
		}
	}
	if got := set("w", map[string]any{"merge": "true", "value": map[string]any{"x": 1}}); !strings.Contains(got, "merge must be a boolean") {
		t.Fatalf("non-bool merge not rejected: %s", got)
	}
	if got := stored("w"); got != `{"other":2}` {
		t.Fatalf("non-bool merge changed the stored blob: %s", got)
	}
	if got := set("never", map[string]any{"merge": true, "value": nil}); !strings.Contains(got, "must be a JSON object") {
		t.Fatalf("null merge on missing key not rejected: %s", got)
	}
	if got := stored("never"); got != "<absent>" {
		t.Fatalf("rejected merge created a blob: %s", got)
	}
}

// sizedBlob returns a stored JSON string blob of exactly n bytes (n >= 2).
func sizedBlob(n int) json.RawMessage {
	return json.RawMessage(`"` + strings.Repeat("x", n-2) + `"`)
}

const agendaNotePrefix = "note: agenda totals "

func TestRenderSessionStateAgendaSizeNoteThreshold(t *testing.T) {
	below := renderSessionState(sessionRecord{Agenda: map[string]json.RawMessage{
		"a": sizedBlob(4000),
		"b": sizedBlob(4191),
	}})
	if strings.Contains(below, agendaNotePrefix) {
		t.Fatalf("agenda total 8191 must not carry the size note:\n%s", below)
	}

	at := sessionRecord{Agenda: map[string]json.RawMessage{
		"a": sizedBlob(4000),
		"b": sizedBlob(4192),
	}}
	want := "note: agenda totals 8192 bytes (threshold 8192); clear stale blobs with agenda.clear (key, or all: true)."
	first := renderSessionState(at)
	if !strings.Contains(first, "\n"+want+"\n") {
		t.Fatalf("agenda total 8192 must carry the exact size note:\n%s", first)
	}
	// Placement: after the last agenda blob, before the todo summary.
	noteIdx := strings.Index(first, want)
	if lastBlob := strings.Index(first, "### agenda: b"); lastBlob < 0 || lastBlob > noteIdx {
		t.Fatalf("size note must follow the agenda blobs:\n%s", first)
	}
	if todos := strings.Index(first, "### Todos"); todos < noteIdx {
		t.Fatalf("size note must precede ### Todos:\n%s", first)
	}
	// Stateless: every render over the threshold repeats the note.
	if second := renderSessionState(at); second != first {
		t.Fatalf("consecutive renders over the threshold must be identical, note included:\n%s\n---\n%s", first, second)
	}

	// Key names are excluded from the measure: long keys do not push a
	// sub-threshold total over.
	longKeys := renderSessionState(sessionRecord{Agenda: map[string]json.RawMessage{
		strings.Repeat("k", 500): sizedBlob(8191),
	}})
	if strings.Contains(longKeys, agendaNotePrefix) {
		t.Fatalf("key names must not count toward the agenda total:\n%s", longKeys)
	}
}

func TestRenderSessionStateAgendaSizeNoteCountsResolverBlobs(t *testing.T) {
	implement := sizedBlob(1582)
	proceed := sizedBlob(976)
	other := sizedBlob(8192 - 1582 - 976)
	without := renderSessionState(sessionRecord{Agenda: map[string]json.RawMessage{"scratch": other}})
	if strings.Contains(without, agendaNotePrefix) {
		t.Fatalf("the scratch blob alone is under the threshold:\n%s", without)
	}
	with := renderSessionState(sessionRecord{Agenda: map[string]json.RawMessage{
		"implement": implement,
		"proceed":   proceed,
		"scratch":   other,
	}})
	if !strings.Contains(with, "note: agenda totals 8192 bytes (threshold 8192)") {
		t.Fatalf("typed resolver blobs must count toward the agenda total:\n%s", with)
	}
}

func TestServeStdioAgendaSetSizeNote(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 904000, root, nil))

	// A string value of m runes is stored as m+2 bytes (its JSON quotes).
	if got := callToolWithKey(t, server, 4000, key, "agenda.set", map[string]any{
		"key":   "a",
		"value": strings.Repeat("x", 4000-2),
	}); got != "agenda set: a\n" {
		t.Fatalf("agenda.set below the threshold must carry no note, got: %q", got)
	}
	if got := callToolWithKey(t, server, 4001, key, "agenda.set", map[string]any{
		"key":   "b",
		"value": strings.Repeat("x", 4191-2),
	}); got != "agenda set: b\n" {
		t.Fatalf("agenda.set at total 8191 must carry no note, got: %q", got)
	}
	// Overwriting b grows the total to exactly 8192: the total is computed
	// after the new blob is stored, not from the prior value.
	want := "agenda set: b\nnote: agenda totals 8192 bytes (threshold 8192); clear stale blobs with agenda.clear (key, or all: true).\n"
	if got := callToolWithKey(t, server, 4002, key, "agenda.set", map[string]any{
		"key":   "b",
		"value": strings.Repeat("x", 4192-2),
	}); got != want {
		t.Fatalf("agenda.set at total 8192 unexpected:\n got: %q\nwant: %q", got, want)
	}
	// Repeats on every set while over the threshold.
	if got := callToolWithKey(t, server, 4003, key, "agenda.set", map[string]any{
		"key":   "c",
		"value": "y",
	}); !strings.Contains(got, "note: agenda totals 8195 bytes (threshold 8192)") {
		t.Fatalf("agenda.set over the threshold must repeat the note, got: %q", got)
	}
	// Session-state renders carry it on every call while over the threshold.
	for i := 0; i < 2; i++ {
		if got := callToolWithKey(t, server, 4004+i, key, "workflow_state", nil); !strings.Contains(got, "note: agenda totals 8195 bytes (threshold 8192)") {
			t.Fatalf("workflow_state render %d over the threshold must carry the note: %s", i, got)
		}
	}
	if got := callToolWithKey(t, server, 4006, key, "workflow_manual", nil); !strings.Contains(got, "note: agenda totals 8195 bytes (threshold 8192)") {
		t.Fatalf("workflow_manual over the threshold must carry the note: %s", got)
	}
	// Clearing drops the note.
	callToolWithKey(t, server, 4007, key, "agenda.clear", map[string]any{"key": "a"})
	if got := callToolWithKey(t, server, 4008, key, "workflow_state", nil); strings.Contains(got, agendaNotePrefix) {
		t.Fatalf("workflow_state under the threshold must not carry the note: %s", got)
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
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"todo.list","arguments":{}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !strings.Contains(byID["1"], "session_key is required") {
		t.Fatalf("expected session_key-required error, got: %s", byID["1"])
	}
}
