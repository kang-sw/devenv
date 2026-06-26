package mcp

import (
	"encoding/json"
	"fmt"
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
)

// todoItem is one ordered checklist entry. Identity is the caller-provided key,
// unique within the active list; the title is human-facing text.
type todoItem struct {
	Key    string     `json:"key"`
	Title  string     `json:"title"`
	Status todoStatus `json:"status"`
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
func todoAppend(list []todoItem, key, title string, status todoStatus) ([]todoItem, error) {
	normalizedKey, err := normalizeTodoKey(key)
	if err != nil {
		return nil, err
	}
	if indexOfTodo(list, normalizedKey) >= 0 {
		return nil, fmt.Errorf("todo key %q already exists", normalizedKey)
	}
	return append(list, todoItem{Key: normalizedKey, Title: title, Status: status}), nil
}

// todoInsert inserts a new item before or after refKey. after=false inserts
// before refKey; after=true inserts after it.
func todoInsert(list []todoItem, refKey, key, title string, status todoStatus, after bool) ([]todoItem, error) {
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
	out = append(out, todoItem{Key: normalizedKey, Title: title, Status: status})
	out = append(out, list[pos:]...)
	return out, nil
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
		lines := make([]string, 0, len(list))
		for _, item := range list {
			lines = append(lines, renderTodoLine(item))
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
			lines = append(lines, renderTodoLine(item))
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

func renderTodoLine(item todoItem) string {
	return fmt.Sprintf("%s {%s} %s", todoMarker(item.Status), item.Key, item.Title)
}

// --- enter-mode todo derivation ----------------------------------------------

type implementTodoVerdict struct {
	Delegation  string
	PlanDepth   string
	ReviewAlloc string
	NeedReview  bool
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
		{Key: "route", Title: "Route"},
		{Key: "prep", Title: implementPrepTitle(verdict.PlanDepth)},
		{Key: "edit", Title: implementEditTitle(verdict.Delegation)},
	}
	if verdict.NeedReview {
		items = append(items, todoItem{Key: "review", Title: implementReviewTitle(verdict.ReviewAlloc)})
	}
	if verdict.NeedDoc {
		items = append(items,
			todoItem{Key: "doc-pre-pass", Title: "Doc pre-pass"},
			todoItem{Key: "doc-commit-gate", Title: "Doc commit gate"},
			todoItem{Key: "doc-closeout", Title: "Doc closeout"},
		)
	}
	items = append(items,
		todoItem{Key: "final-action-gate", Title: "Final action gate"},
		todoItem{Key: "merge", Title: "Merge"},
	)
	return withPendingStatus(items)
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

func parseImplementPlanDepth(raw string) (string, error) {
	switch strings.ToLower(raw) {
	case "":
		return "survey", nil
	case "none", "brief", "survey", "research":
		return strings.ToLower(raw), nil
	default:
		return "", fmt.Errorf("invalid plan_depth %q: want one of none, brief, survey, research", raw)
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
	case "brief":
		return "Prep (brief)"
	case "survey":
		return "Prep (brief + survey plan)"
	case "research":
		return "Prep (brief + research plan)"
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
		return "Review"
	}
}

// deriveProceedTodos mirrors lead-proceed "On: invoke": build route context,
// select route, emit routing verdict, execute verdict.
func deriveProceedTodos() []todoItem {
	return withPendingStatus([]todoItem{
		{Key: "route-context", Title: "Build route context"},
		{Key: "select-route", Title: "Select route"},
		{Key: "routing-verdict", Title: "Emit routing verdict"},
		{Key: "execute-verdict", Title: "Execute verdict"},
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

func (s *Server) handleAgendaSet(id json.RawMessage, args map[string]any) response {
	const tool = "ws.agenda.set"
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
	const tool = "ws.agenda.clear"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
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
	needReview, _ := args["need_review"].(bool)
	needDoc, _ := args["need_doc"].(bool)
	delegation, err := parseImplementDelegation(stringValue(args["delegation"]))
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("ws.enter.implement: %w", err))
	}
	planDepth, err := parseImplementPlanDepth(stringValue(args["plan_depth"]))
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("ws.enter.implement: %w", err))
	}
	reviewAlloc, err := parseImplementReviewAlloc(stringValue(args["review_alloc"]))
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("ws.enter.implement: %w", err))
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
	return s.handleEnter(id, "ws.enter.implement", "implement", args, todos)
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func (s *Server) handleEnterProceed(id json.RawMessage, args map[string]any) response {
	return s.handleEnter(id, "ws.enter.proceed", "proceed", args, deriveProceedTodos())
}

func (s *Server) handleEnterSprint(id json.RawMessage, args map[string]any) response {
	return s.handleEnter(id, "ws.enter.sprint", "sprint", args, deriveSprintTodos())
}

func (s *Server) handleEnterSalvage(id json.RawMessage, args map[string]any) response {
	return s.handleEnter(id, "ws.enter.salvage", "salvage", args, deriveSalvageTodos())
}

func (s *Server) handleTodoAppend(id json.RawMessage, args map[string]any) response {
	const tool = "ws.todo.append"
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
	if err := s.sessions.mutateTodos(sessionKey, func(list []todoItem) ([]todoItem, error) {
		return todoAppend(list, normalizedKey, title, todoPending)
	}); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	return toolTextResponse(id, fmt.Sprintf("todo appended: %s\n", normalizedKey), nil)
}

func (s *Server) handleTodoInsert(id json.RawMessage, args map[string]any, after bool) response {
	tool := "ws.todo.insert_before"
	if after {
		tool = "ws.todo.insert_after"
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
	if err := s.sessions.mutateTodos(sessionKey, func(list []todoItem) ([]todoItem, error) {
		return todoInsert(list, refKey, normalizedKey, title, todoPending, after)
	}); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	return toolTextResponse(id, fmt.Sprintf("todo inserted: %s\n", normalizedKey), nil)
}

func (s *Server) handleTodoCheck(id json.RawMessage, args map[string]any) response {
	const tool = "ws.todo.check"
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
	if err := s.sessions.mutateTodos(sessionKey, func(list []todoItem) ([]todoItem, error) {
		return todoCheck(list, normalizedKey, status)
	}); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	return toolTextResponse(id, fmt.Sprintf("todo %s: %s\n", status, normalizedKey), nil)
}

func (s *Server) handleTodoErase(id json.RawMessage, args map[string]any) response {
	const tool = "ws.todo.erase"
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
	const tool = "ws.todo.clear"
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
	const tool = "ws.todo.list"
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

func (s *Server) handleTodoReorder(id json.RawMessage, args map[string]any) response {
	const tool = "ws.todo.reorder"
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
