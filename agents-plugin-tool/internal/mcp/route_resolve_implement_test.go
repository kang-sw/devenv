package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// assertRouteResolveSchemaIsOpaque asserts the published tools/list schema for
// toolName is fully opaque: properties are exactly session_key + params,
// params is a bare object with no nested properties, required is exactly
// [session_key], and both the tool description and params description point
// at the owning skill by name (pointer-text/naming-consistency proof for the
// companion redirect-guard ticket).
func assertRouteResolveSchemaIsOpaque(t *testing.T, listLine, toolName, skillPointer string) {
	t.Helper()
	var listResp map[string]any
	if err := json.Unmarshal([]byte(listLine), &listResp); err != nil {
		t.Fatal(err)
	}
	result, _ := listResp["result"].(map[string]any)
	listedTools, _ := result["tools"].([]any)
	var tool map[string]any
	for _, rawTool := range listedTools {
		candidate, _ := rawTool.(map[string]any)
		if candidate["name"] == toolName {
			tool = candidate
			break
		}
	}
	if tool == nil {
		t.Fatalf("tools/list missing %s", toolName)
	}
	description, _ := tool["description"].(string)
	if !strings.Contains(description, skillPointer) {
		t.Fatalf("%s description = %q, want it to contain %q", toolName, description, skillPointer)
	}

	schema, _ := tool["inputSchema"].(map[string]any)
	if schema == nil {
		t.Fatalf("%s missing inputSchema", toolName)
	}

	properties, _ := schema["properties"].(map[string]any)
	if len(properties) != 2 {
		t.Fatalf("%s properties = %#v, want exactly session_key and params", toolName, properties)
	}
	for _, forbidden := range []string{"target", "facts", "policy"} {
		if _, present := properties[forbidden]; present {
			t.Fatalf("%s properties still advertise %q; want opaque params only", toolName, forbidden)
		}
	}
	if _, ok := properties["session_key"]; !ok {
		t.Fatalf("%s properties missing session_key: %#v", toolName, properties)
	}

	params, _ := properties["params"].(map[string]any)
	if params == nil {
		t.Fatalf("%s properties missing params object: %#v", toolName, properties)
	}
	if params["type"] != "object" {
		t.Fatalf("%s params type = %#v, want \"object\"", toolName, params["type"])
	}
	if _, hasNestedProperties := params["properties"]; hasNestedProperties {
		t.Fatalf("%s params has residual nested properties: %#v", toolName, params)
	}
	paramsDescription, _ := params["description"].(string)
	if !strings.Contains(paramsDescription, skillPointer) {
		t.Fatalf("%s params description = %q, want it to contain %q", toolName, paramsDescription, skillPointer)
	}

	required, _ := schema["required"].([]any)
	if len(required) != 1 || required[0] != "session_key" {
		t.Fatalf("%s required = %#v, want exactly [\"session_key\"]", toolName, required)
	}
}

func TestRouteResolveImplementSchemaIsOpaque(t *testing.T) {
	useLeadProfile(t)
	server := NewServer(t.TempDir(), "test")
	assertRouteResolveSchemaIsOpaque(t, callToolsList(t, server), "route.resolve_implement", "ticket-worker")
}

func routeStateSnapshot(t *testing.T, server *Server, key string) string {
	t.Helper()
	record, ok := server.sessions.readState(key, partAll)
	if !ok {
		t.Fatalf("session record not found for %q", key)
	}
	payload := struct {
		Agenda map[string]json.RawMessage `json:"agenda"`
		Todos  []todoItem                 `json:"todos"`
	}{Agenda: record.Agenda, Todos: record.Todos}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestRouteResolveWrappedCallsMatchTypedVerdictAndState(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")

	for _, tc := range []struct {
		name string
		tool string
		args func(string) map[string]any
	}{
		{name: "implement", tool: "route.resolve_implement", args: implementReadyArgs},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, format := range []string{"text", "json"} {
				t.Run(format, func(t *testing.T) {
					typedKey, _ := parseLoginResponse(t, callLogin(t, server, 100, root, nil))
					wrappedKey, _ := parseLoginResponse(t, callLogin(t, server, 101, root, nil))
					typed := callToolWithKey(t, server, 102, typedKey, tc.tool, tc.args(format))
					wrapped := callToolWithKey(t, server, 103, wrappedKey, tc.tool, map[string]any{"params": tc.args(format)})
					if wrapped != typed {
						t.Fatalf("wrapped verdict differed from typed call\ntyped:\n%s\nwrapped:\n%s", typed, wrapped)
					}
					if got, want := routeStateSnapshot(t, server, wrappedKey), routeStateSnapshot(t, server, typedKey); got != want {
						t.Fatalf("wrapped state differed from typed call\ntyped: %s\nwrapped: %s", want, got)
					}
				})
			}
		})
	}
}

func TestRouteResolveWrappedMalformedEnvelopeDoesNotMutateState(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	for _, tool := range []string{"route.resolve_implement"} {
		t.Run(tool, func(t *testing.T) {
			server := NewServer(root, "test")
			key, _ := parseLoginResponse(t, callLogin(t, server, 200, root, nil))
			cases := []struct {
				name string
				args map[string]any
				want string
			}{
				{name: "null params", args: map[string]any{"params": nil}, want: "params must be an object"},
				{name: "array params", args: map[string]any{"params": []any{}}, want: "params must be an object"},
				{name: "string params", args: map[string]any{"params": "invalid"}, want: "params must be an object"},
				{name: "empty params", args: map[string]any{"params": map[string]any{}}, want: "target is required"},
				{name: "mixed typed field", args: map[string]any{"params": map[string]any{}, "target": map[string]any{}}, want: "wrapped calls accept only session_key and params"},
				{name: "mixed legacy field", args: map[string]any{"params": map[string]any{}, "delegation": "delegated"}, want: "wrapped calls accept only session_key and params"},
				{name: "nested session key", args: map[string]any{"params": map[string]any{"session_key": "inner"}}, want: "params.session_key is not allowed"},
			}
			for index, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					before := routeStateSnapshot(t, server, key)
					got := callToolWithKey(t, server, 201+index, key, tool, tc.args)
					if !strings.Contains(got, tc.want) {
						t.Fatalf("error = %q, want %q", got, tc.want)
					}
					if after := routeStateSnapshot(t, server, key); after != before {
						t.Fatalf("malformed wrapper mutated state\nbefore: %s\nafter: %s", before, after)
					}
				})
			}
		})
	}
}

func TestRouteResolveWrappedUsesOuterSessionKey(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 300, root, nil))

	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{tool: "route.resolve_implement", args: implementReadyArgs("text")},
	} {
		before := routeStateSnapshot(t, server, key)
		got := callToolWithKey(t, server, 301, "unknown-outer-key", tc.tool, map[string]any{"params": tc.args})
		if !strings.Contains(got, "session key not found: unknown-outer-key") {
			t.Fatalf("%s did not authenticate outer key: %s", tc.tool, got)
		}
		if after := routeStateSnapshot(t, server, key); after != before {
			t.Fatalf("unknown outer key mutated known session\nbefore: %s\nafter: %s", before, after)
		}
	}
}

func readTodoInstruction(t *testing.T, server *Server, id int, key, todoKey string) string {
	t.Helper()
	raw := callToolWithKey(t, server, id, key, "todo.read", map[string]any{"key": todoKey})
	var payload todoReadPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("todo.read(%s) did not parse: %v\n%s", todoKey, err, raw)
	}
	if payload.Instruction == nil || strings.TrimSpace(*payload.Instruction) == "" {
		t.Fatalf("todo.read(%s) missing instruction: %+v", todoKey, payload)
	}
	return *payload.Instruction
}

func TestEnterImplementNewSchemaReturnsVerdictAndStoresAgenda(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	runGit(t, root, "switch", "-c", "feature/base")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
	enableReviewPhase(t, server, key)
	writeImplementReadyTicket(t, root, implementReadyFacts())

	text := callToolWithKey(t, server, 2, key, "route.resolve_implement", implementReadyArgs("text"))
	for _, want := range []string{
		"Implementation Verdict",
		"Mode: delegated",
		"Branch Action: create impl/feature/base/jot-pug-mossy",
		"Review Allocation: partitioned: correctness, fit, test",
		"Route Facts: read from the ticket",
		"Next: Create impl/feature/base/jot-pug-mossy",
		"installed Prep and Edit todos",
		"partitioned: correctness, fit, test review",
		"and the final action gate in order",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("route.resolve_implement verdict missing %q:\n%s", want, text)
		}
	}

	jsonText := callToolWithKey(t, server, 3, key, "route.resolve_implement", implementReadyArgs("json"))
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

	record, ok := server.sessions.readState(key, partAll)
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
	if !eqKeys(keysOf(record.Todos), "route", "prep", "edit", "review", "final-action-gate", "merge") {
		t.Fatalf("route.resolve_implement did not replace todo list: %v", keysOf(record.Todos))
	}
	readPrep := callToolWithKey(t, server, 4, key, "todo.read", map[string]any{"key": "prep"})
	var prepPayload todoReadPayload
	if err := json.Unmarshal([]byte(readPrep), &prepPayload); err != nil {
		t.Fatalf("prep todo read did not parse: %v\n%s", err, readPrep)
	}
	if prepPayload.Instruction == nil || !strings.HasPrefix(*prepPayload.Instruction, implementPrepGuardrails("")) {
		t.Fatalf("prep instruction = %#v", prepPayload.Instruction)
	}
	full := callToolWithKey(t, server, 5, key, "todo.list", map[string]any{"mode": "full"})
	for _, want := range []string{
		"- [ ] {prep} Prep\n      " + implementPrepGuardrails(""),
		"- [ ] {edit} Edit\n      Apply the source edits",
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

// TestEnterImplementTicketTargetTodosCarryNoPlanningStage pins the collapsed
// route shape for a ticket target: the installed list opens route -> prep ->
// edit -> review with no stage in between, and no todo mints or names a plan.
// The target is the plan.
func TestEnterImplementTicketTargetTodosCarryNoPlanningStage(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	runGit(t, root, "switch", "-c", "feature/base")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
	enableReviewPhase(t, server, key)
	writeImplementReadyTicket(t, root, implementReadyFacts())
	callToolWithKey(t, server, 2, key, "route.resolve_implement", implementReadyArgs("text"))

	record, ok := server.sessions.readState(key, partAll)
	if !ok {
		t.Fatal("session record not found")
	}
	keys := keysOf(record.Todos)
	for i, want := range []string{"route", "prep", "edit", "review"} {
		if i >= len(keys) || keys[i] != want {
			t.Fatalf("todo list = %v, want it to open with route, prep, edit, review", keys)
		}
	}
	full := callToolWithKey(t, server, 3, key, "todo.list", map[string]any{"mode": "full"})
	for _, forbidden := range []string{"plan-populator", "path.generate", "Plan Depth", "plan_depth", "survey plan", "research plan", "{plan}"} {
		if strings.Contains(full, forbidden) {
			t.Fatalf("todo list retained planning-stage text %q:\n%s", forbidden, full)
		}
	}
}

func TestEnterImplementAllocatesSingleReviewForBoundedPublicExistingTestChange(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
	enableReviewPhase(t, server, key)
	facts := implementReadyFacts()
	facts["scope.new_type_contract"] = "no"
	facts["complexity.reuse_points"] = "confirmed"
	facts["risk.correctness"] = "low"
	facts["risk.fit"] = "low"
	facts["risk.test"] = "low"
	facts["risk.security_or_contract"] = "low"
	writeImplementReadyTicket(t, root, facts)

	jsonText := callToolWithKey(t, server, 2, key, "route.resolve_implement", implementReadyArgs("json"))
	var result implementResult
	if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
		t.Fatalf("json verdict did not parse: %v\n%s", err, jsonText)
	}
	if result.Verdict.ReviewAlloc != "single" || !result.Verdict.NeedReview {
		t.Fatalf("bounded public/existing-test review = %q need=%v, want single true", result.Verdict.ReviewAlloc, result.Verdict.NeedReview)
	}
	review := readTodoInstruction(t, server, 3, key, "review")
	for _, want := range []string{
		"Render `reviewer`", "one full-scope review", "rendered path", "generated findings path",
	} {
		if !strings.Contains(review, want) {
			t.Fatalf("single-review todo instruction missing %q: %q", want, review)
		}
	}
	for _, want := range implementReviewRoundWants() {
		if !strings.Contains(review, want) {
			t.Fatalf("single-review todo instruction missing %q: %q", want, review)
		}
	}
	if strings.Contains(review, "reviewers") {
		t.Fatalf("single-review todo instruction = %q", review)
	}
	// The two-round worker-owned model retires the relay delegate and the
	// ceiling elevation on every allocation shape, single included: neither
	// implementer-relay nor implementer-elevated is reachable here.
	for _, forbidden := range implementReviewRoundForbidden() {
		if strings.Contains(review, forbidden) {
			t.Fatalf("single-review todo instruction retained superseded relay/elevation wording %q: %q", forbidden, review)
		}
	}
}

// TestEnterImplementSkippedDocsOmitsDocTodos covers the fact set that used to
// select the caller-edits/caller-reviews fast path plus a documentation skip.
// The documentation skip still drops the three doc todos; the fast path no
// longer exists, so the same call now yields one execution mode, an independent
// reviewer, and the standard branch and merge tail.
func TestEnterImplementSkippedDocsOmitsDocTodos(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "README.md", "initialized\n")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "initial")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
	enableReviewPhase(t, server, key)

	jsonText := callToolWithKey(t, server, 2, key, "route.resolve_implement", implementSkipDocsArgs("json"))
	var result implementResult
	if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
		t.Fatalf("json verdict did not parse: %v\n%s", err, jsonText)
	}
	if result.Verdict.Delegation != "delegated" || result.Verdict.ReviewAlloc != "single" || !result.Verdict.NeedReview {
		t.Fatalf("smallest safe change lost its independent reviewer: %+v", result.Verdict)
	}
	if result.Verdict.DocMode != "skipped" {
		t.Fatalf("doc mode = %q, want skipped", result.Verdict.DocMode)
	}
	if result.Verdict.BranchPlan.Action != "create" || result.Verdict.BranchPlan.MergeTarget == "" {
		t.Fatalf("verdict skipped the standard branch path: %+v", result.Verdict.BranchPlan)
	}

	review := readTodoInstruction(t, server, 4, key, "review")
	if !strings.Contains(review, "Render `reviewer`") {
		t.Fatalf("review todo did not dispatch an independent reviewer: %q", review)
	}
	for _, forbidden := range []string{"Perform lead-owned review only", "external reviewers are unnecessary"} {
		if strings.Contains(review, forbidden) {
			t.Fatalf("review todo retained the caller-reviews-itself path %q: %q", forbidden, review)
		}
	}
	full := callToolWithKey(t, server, 6, key, "todo.list", map[string]any{"mode": "full"})
	for _, forbidden := range []string{"{doc-pre-pass}", "{doc-commit-gate}", "{doc-closeout}", "{complete}"} {
		if strings.Contains(full, forbidden) {
			t.Fatalf("skipped-doc todo list contains forbidden %q:\n%s", forbidden, full)
		}
	}
	for _, want := range []string{"{final-action-gate}", "{merge}"} {
		if !strings.Contains(full, want) {
			t.Fatalf("skipped-doc todo list missing standard tail %q:\n%s", want, full)
		}
	}
}

func TestEnterImplementUnbornRepositoryUsesStandardCreatePath(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
	enableReviewPhase(t, server, key)
	jsonText := callToolWithKey(t, server, 2, key, "route.resolve_implement", implementSkipDocsArgs("json"))
	var result implementResult
	if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
		t.Fatalf("json verdict did not parse: %v\n%s", err, jsonText)
	}
	if result.Verdict.BranchPlan.Action != "create" || result.Verdict.BranchPlan.StartCommit != "(initial)" {
		t.Fatalf("unborn repository branch plan = %+v, want standard create with observed marker", result.Verdict.BranchPlan)
	}
	record, ok := server.sessions.readState(key, partAll)
	if !ok {
		t.Fatal("session record not found")
	}
	if !eqKeys(keysOf(record.Todos), "route", "prep", "edit", "review", "final-action-gate", "merge") {
		t.Fatalf("unborn fallback todo shape = %v", keysOf(record.Todos))
	}
}

func TestEnterImplementNearMissesPreserveStandardBranchAndMergeTodos(t *testing.T) {
	cases := []struct {
		name       string
		branch     string
		wantAction string
	}{
		{
			name:       "fresh branch creates",
			wantAction: "create",
		},
		{
			name:       "matching impl branch continues",
			branch:     implementTargetBranchName("", "tiny-direct-edit"),
			wantAction: "continue",
		},
		{
			name:       "legacy implement branch renames",
			branch:     "implement/tiny-direct-edit",
			wantAction: "rename",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useLeadProfile(t)
			root := t.TempDir()
			initGit(t, root)
			if tc.branch != "" {
				runGit(t, root, "switch", "-c", tc.branch)
			}
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			server := NewServer(root, "test")
			key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
			enableReviewPhase(t, server, key)

			args := implementSkipDocsArgs("json")
			if tc.branch != "" {
				args["policy"].(map[string]any)["branch"].(map[string]any)["merge_target"] = "master"
				args["policy"].(map[string]any)["branch"].(map[string]any)["allow_rename"] = "yes"
			}
			jsonText := callToolWithKey(t, server, 2, key, "route.resolve_implement", args)
			var result implementResult
			if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
				t.Fatalf("json verdict did not parse: %v\n%s", err, jsonText)
			}
			if result.Verdict.BranchPlan.Action != tc.wantAction {
				t.Fatalf("branch action = %q, want %q; verdict=%+v", result.Verdict.BranchPlan.Action, tc.wantAction, result.Verdict)
			}
			record, ok := server.sessions.readState(key, partAll)
			if !ok {
				t.Fatal("session record not found")
			}
			if !eqKeys(keysOf(record.Todos), "route", "prep", "edit", "review", "final-action-gate", "merge") {
				t.Fatalf("standard near-miss todo shape = %v", keysOf(record.Todos))
			}
			finalAction := requireInstruction(t, todoByKey(t, record.Todos, "final-action-gate"))
			merge := requireInstruction(t, todoByKey(t, record.Todos, "merge"))
			if !strings.Contains(finalAction, "Do not merge; the worker ends at the report") || !strings.Contains(merge, "Do not merge from the worker") {
				t.Fatalf("standard merge instructions changed: final=%q merge=%q", finalAction, merge)
			}
		})
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
	text := callToolWithKey(t, server, 2, key, "route.resolve_implement", args)
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
	for _, forbidden := range []string{"path.generate", "plan-populator-survey", "render implementer"} {
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
	full := callToolWithKey(t, server, 4, key, "todo.list", map[string]any{"mode": "full"})
	if !strings.Contains(full, "merge target required while already on an implementation branch") || strings.Contains(full, "Dispatch the delegated implementer with Delegate dispatch") {
		t.Fatalf("branch-stop full todo list not focused:\n%s", full)
	}
}

func TestEnterImplementNewImplPrefixBranchTargetExists(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "file.txt", "one\n")
	runGit(t, root, "add", "file.txt")
	runGit(t, root, "commit", "-m", "initial")
	// The target branch this scenario resolves to is "impl/jot-pug-mossy":
	// implementReadyArgs's ticket_stem now deterministically derives the
	// branch stem via wskey.Derive (authoritative over the ready-args
	// scope_slug), overriding the caller-supplied scope_slug. Pre-create it so
	// observeImplementBranch's TargetExists check and
	// deriveImplementBranchPlan's target-branch naming both key off the same
	// shared helper (implementTargetBranchName) instead of drifting.
	runGit(t, root, "switch", "-c", "impl/jot-pug-mossy")
	runGit(t, root, "switch", "-c", "impl/old-scope")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	args := implementReadyArgs("json")
	args["policy"].(map[string]any)["branch"].(map[string]any)["allow_rename"] = "yes"
	text := callToolWithKey(t, server, 2, key, "route.resolve_implement", args)
	var result implementResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("json verdict did not parse: %v\n%s", err, text)
	}
	if result.Verdict.BranchPlan.Action != "stop" {
		t.Fatalf("expected stop branch action when target already exists, got %+v", result.Verdict.BranchPlan)
	}
	if result.Verdict.BranchPlan.TargetBranch != "impl/jot-pug-mossy" {
		t.Fatalf("target branch = %q, want impl/jot-pug-mossy", result.Verdict.BranchPlan.TargetBranch)
	}
	if !strings.Contains(result.Verdict.BranchPlan.Reason, "already exists") {
		t.Fatalf("expected already-exists reason, got %q", result.Verdict.BranchPlan.Reason)
	}
}

// TestEnterImplementCreatePathMergeRootRefConflictDetectedByRealGit exercises
// the real git-backed D/F ref-conflict detector in observeImplementBranch
// (the loop over targetBranch's "/"-separated ancestor segments, each
// rev-parse --verify'd as refs/heads/<ancestor>) end to end. Every other test
// that touches MergeRootRefConflict fabricates the observation directly and
// only proves deriveImplementBranchPlan's response to a conflict, not that
// the ancestor-path scan actually finds one against a real repo. Here a
// legacy single-segment ref "impl/ws-dashboard-dev" is pre-created as a real
// branch, then route.resolve_implement is driven for a fresh create on current
// branch "ws-dashboard-dev" — whose derived target is
// "impl/ws-dashboard-dev/<stem>" — so the scan's second ancestor probe
// (segments[:2] == "impl/ws-dashboard-dev") must rev-parse-verify a ref that
// really exists on disk for the conflict to be detected.
func TestEnterImplementCreatePathMergeRootRefConflictDetectedByRealGit(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "file.txt", "one\n")
	runGit(t, root, "add", "file.txt")
	runGit(t, root, "commit", "-m", "initial")
	// Pre-create the legacy single-segment ref that will collide with the new
	// nested target's ancestor path. Left un-checked-out on purpose: the D/F
	// conflict is a ref that already exists, not a branch that is currently
	// active.
	runGit(t, root, "branch", "impl/ws-dashboard-dev")
	// The current branch becomes the merge root the create path derives
	// automatically; it must not itself be impl/- or implement/-prefixed.
	runGit(t, root, "switch", "-c", "ws-dashboard-dev")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	args := implementReadyArgs("json")
	text := callToolWithKey(t, server, 2, key, "route.resolve_implement", args)
	var result implementResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("json verdict did not parse: %v\n%s", err, text)
	}
	if result.Verdict.BranchPlan.Action != "stop" {
		t.Fatalf("expected stop branch action on real D/F ref conflict, got %+v", result.Verdict.BranchPlan)
	}
	wantTargetBranch := "impl/ws-dashboard-dev/jot-pug-mossy"
	if result.Verdict.BranchPlan.TargetBranch != wantTargetBranch {
		t.Fatalf("target branch = %q, want %q", result.Verdict.BranchPlan.TargetBranch, wantTargetBranch)
	}
	if result.Verdict.BranchPlan.MergeTarget != "ws-dashboard-dev" {
		t.Fatalf("merge target = %q, want ws-dashboard-dev (derived merge root)", result.Verdict.BranchPlan.MergeTarget)
	}
	if !strings.Contains(result.Verdict.BranchPlan.Reason, "impl/ws-dashboard-dev") {
		t.Fatalf("reason = %q, want it to name the real conflicting ref impl/ws-dashboard-dev", result.Verdict.BranchPlan.Reason)
	}
	if !strings.Contains(result.NextInstruction, "Stop before source edits") {
		t.Fatalf("next instruction = %q, want the branch-safety-blocker stop instruction", result.NextInstruction)
	}
}

// --- binding-anchor production-wiring integration tests -----------------------
//
// These drive the real handlers (NewServer + callToolWithKey) against a
// t.TempDir() root whose AGENTS.md either declares a `### Binding Anchor` or
// omits it, so the AGENTS.md-read wiring added at the three route.resolve_*
// call sites is exercised end-to-end — not the pure parser and not a
// hand-constructed verdict/input struct with the field pre-set.

const bindingAnchorTestPath = "ai-docs/tickets/idea/000000-demo-anchor.md"
const bindingAnchorTestTopics = "alpha topic, beta topic"

// bindingAnchorTestClause is the Prep-guardrail clause a declaring root must
// render (matching wsreview.BindingAnchor.PrepClause's format).
const bindingAnchorTestClause = "read " + bindingAnchorTestPath + " when target touches " + bindingAnchorTestTopics + ", "

func writeBindingAnchorAGENTS(t *testing.T, root string) {
	t.Helper()
	content := "# AGENTS.md\n\n## Workflow\n\n### Binding Anchor\nanchor: " + bindingAnchorTestPath + "\ntopics: " + bindingAnchorTestTopics + "\n\n### Commit Rules\nother content\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func writeSectionlessAGENTS(t *testing.T, root string) {
	t.Helper()
	content := "# AGENTS.md\n\n## Workflow\n\n### Review Policy\nreview-track: develop\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func readPrepInstruction(t *testing.T, server *Server, id int, key string) string {
	t.Helper()
	readPrep := callToolWithKey(t, server, id, key, "todo.read", map[string]any{"key": "prep"})
	var prepPayload todoReadPayload
	if err := json.Unmarshal([]byte(readPrep), &prepPayload); err != nil {
		t.Fatalf("prep todo read did not parse: %v\n%s", err, readPrep)
	}
	if prepPayload.Instruction == nil {
		t.Fatalf("prep todo has no instruction:\n%s", readPrep)
	}
	return *prepPayload.Instruction
}

func assertPrepAnchorClause(t *testing.T, prep string, declared bool) {
	t.Helper()
	if declared {
		if !strings.Contains(prep, bindingAnchorTestClause) {
			t.Fatalf("declaring root: prep instruction missing rendered clause %q:\n%s", bindingAnchorTestClause, prep)
		}
		return
	}
	for _, forbidden := range []string{bindingAnchorTestPath, "when target touches", "alpha topic"} {
		if strings.Contains(prep, forbidden) {
			t.Fatalf("sectionless root: prep instruction unexpectedly contains %q:\n%s", forbidden, prep)
		}
	}
}

// TestEnterImplementTypedPathRendersDeclaredBindingAnchor exercises the typed
// route.resolve_implement path in handleEnterImplement: the clause is read from
// record.Root's AGENTS.md.
func TestEnterImplementTypedPathRendersDeclaredBindingAnchor(t *testing.T) {
	for _, tc := range []struct {
		name     string
		declared bool
	}{
		{name: "declared", declared: true},
		{name: "sectionless", declared: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useLeadProfile(t)
			root := t.TempDir()
			initGit(t, root)
			runGit(t, root, "switch", "-c", "feature/base")
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			if tc.declared {
				writeBindingAnchorAGENTS(t, root)
			} else {
				writeSectionlessAGENTS(t, root)
			}
			server := NewServer(root, "test")
			key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
			callToolWithKey(t, server, 2, key, "route.resolve_implement", implementReadyArgs("text"))
			assertPrepAnchorClause(t, readPrepInstruction(t, server, 3, key), tc.declared)
		})
	}
}
