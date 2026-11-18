package wsagent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

// codexRun replays recorded server output. Only client request ids are
// translated to ws's kind-bearing ids; client notifications and answers are
// not server output and never enter the mapper. Synthetic cases below are
// explicitly labelled and do not extend the recorded version guarantee.
type codexRun struct {
	m       *codexMapper
	events  []Event
	answers map[string]controlAnswer
	sent    []any
}

func newCodexRun(session string) *codexRun {
	return &codexRun{m: newCodexMapper(session), answers: map[string]controlAnswer{}}
}

func (r *codexRun) answer(id string, answer controlAnswer) { r.answers[id] = answer }
func (r *codexRun) send(v any)                             { r.sent = append(r.sent, v) }
func (r *codexRun) feed(raw string) []Event {
	events := r.m.mapLine([]byte(raw), r)
	r.events = append(r.events, events...)
	return events
}

func feedCodexFixture(t *testing.T, name string) *codexRun {
	t.Helper()
	r := newCodexRun("")
	ids := map[string]string{}
	for _, line := range readLeadFixture(t, name) {
		var msg codexWire
		if err := json.Unmarshal(line.raw, &msg); err != nil {
			t.Fatal(err)
		}
		if line.in {
			if msg.Method != "" && len(msg.ID) > 0 {
				kind := map[string]string{"initialize": "init", "thread/start": "open", "thread/resume": "open", "model/list": "models", "turn/start": "turn"}[msg.Method]
				if kind == "" {
					kind = "control"
				}
				ids[string(msg.ID)] = fmt.Sprintf("wsc/%s/fixture/%d", kind, len(ids)+1)
			}
			continue
		}
		raw := line.raw
		if msg.Method == "" && len(msg.ID) > 0 {
			if id, ok := ids[string(msg.ID)]; ok {
				var fields map[string]json.RawMessage
				_ = json.Unmarshal(raw, &fields)
				fields["id"], _ = json.Marshal(id)
				raw, _ = json.Marshal(fields)
			}
		}
		r.feed(string(raw))
	}
	return r
}

func codexTurns(events []Event) [][]Event {
	var turns [][]Event
	var turn []Event
	for _, event := range events {
		turn = append(turn, event)
		if event.TurnEnd {
			turns = append(turns, turn)
			turn = nil
		}
	}
	return turns
}

func codexBatches(t *testing.T, events []Event) []wireBatch {
	t.Helper()
	var batches []wireBatch
	for _, event := range events {
		if event.Live != nil {
			for _, raw := range event.Live.Batches {
				var batch wireBatch
				if err := json.Unmarshal(raw, &batch); err != nil {
					t.Fatal(err)
				}
				batches = append(batches, batch)
			}
		}
	}
	return batches
}

func TestCodexMapperRecordedLeadSession(t *testing.T) {
	r := feedCodexFixture(t, "codex-0.160.0-lead-session.jsonl")
	turns := codexTurns(r.events)
	if len(turns) != 5 {
		t.Fatalf("five recorded turns, steer is not a separate turn: %d", len(turns))
	}
	starts, usage, limits := 0, 0, 0
	var approvals, settled []string
	for _, event := range r.events {
		if event.TurnStart {
			starts++
		}
		if event.Approval != nil {
			approvals = append(approvals, event.Approval.ID)
			if event.Approval.Tool == "" || len(event.Approval.Input) == 0 || !strings.Contains(event.Approval.Description, "allow/deny only") {
				t.Fatalf("approval loses its context or binary limitation: %+v", event.Approval)
			}
		}
		if event.ApprovalDone != "" {
			settled = append(settled, event.ApprovalDone)
		}
		if event.Harness != nil && (event.Harness.Version != "0.160.0" || len(event.Harness.Demoted) != 0) {
			t.Fatalf("recorded init shape: %+v", event.Harness)
		}
		if event.Live != nil && event.Live.Context != nil {
			usage++
			if usage == 1 {
				c := event.Live.Context
				if c.Tokens == nil || *c.Tokens != 15967 || c.Window != 475000 || c.Percent == nil || *c.Percent != float64(15967)*100/475000 || c.CacheHit == nil || *c.CacheHit != float64(12288)*100/15869 {
					t.Fatalf("first recorded usage, not cumulative usage: %+v", c)
				}
			}
		}
		if event.Live != nil && len(event.Live.Limits) > 0 {
			limits++
			for _, limit := range event.Live.Limits {
				if limit.ResetsAt < 1_000_000_000_000 {
					t.Fatalf("reset must be milliseconds: %+v", limit)
				}
			}
		}
	}
	if starts != 5 || usage != 10 || limits != 10 || !slices.Equal(approvals, []string{"0", "1"}) || !slices.Equal(settled, approvals) {
		t.Fatalf("starts %d usage %d limits %d approvals %v settled %v", starts, usage, limits, approvals, settled)
	}
	if got := finalReplies(turns[0]); !slices.Equal(got, []string{"I’ll call `probe_echo` with `hi`.\n", "user rejected MCP tool call\nPROBE-MARKER-7"}) {
		t.Fatalf("first reply exactly once, before TurnEnd: %q", got)
	}
	if got := finalReplies(turns[3]); len(got) == 0 || got[len(got)-1] != "A\nB\nZEBRA" {
		t.Fatalf("steered reply: %q", got)
	}
	for i, turn := range turns {
		end := turn[len(turn)-1]
		if end.TurnError != "" || (end.TurnReason == wslive.TurnAborted) != (i == 2) {
			t.Fatalf("turn %d: %+v", i, end)
		}
	}
	// The interrupted command never completed in its turn. Its batch must
	// close as failed before TurnEnd, not linger until the recorded late
	// completion during a subsequent turn.
	batches := codexBatches(t, turns[2])
	if len(batches) == 0 || batches[len(batches)-1].EndedAt == nil || batches[len(batches)-1].Failed != 1 {
		t.Fatalf("interrupted open command: %+v", batches)
	}
	if len(r.m.approvals) != 0 || r.m.turnID != "" || r.m.batches.batch != nil {
		t.Fatal("the recorded stream left a turn, approval or tool open")
	}
	failedControls := 0
	for _, answer := range r.answers {
		if !answer.ok {
			failedControls++
		}
	}
	if failedControls != 2 { // stale-turn steer and idle steer
		t.Fatalf("recorded steer errors must answer their waiters: %d", failedControls)
	}
}

// Replay the recorded lead stream line by line as well: a late old-turn
// command completion must emit nothing, and each live tool update must
// precede the recorded turn boundary that follows it.
func TestCodexMapperRecordedToolOrdering(t *testing.T) {
	msgs := readCodexFixture(t, "codex-0.160.0-lead-session.jsonl")
	r := newCodexRun("")
	for _, msg := range msgs {
		if !msg.in && msg.Method == "" && jsonAt(msg.Result, "thread", "id") != nil {
			r.m.sessionID, _ = jsonAt(msg.Result, "thread", "id").(string)
			break
		}
	}
	if r.m.sessionID == "" {
		t.Fatal("recording has no thread/start result")
	}
	active := ""
	starts, completions, late := 0, 0, 0
	for _, msg := range msgs {
		if msg.in || msg.Method == "" {
			continue
		}
		events := r.feed(jsonLine(msg))
		if msg.str("threadId") != r.m.sessionID {
			continue
		}
		switch msg.Method {
		case "turn/started":
			active = msg.str("turn", "id")
		case "turn/completed":
			if len(events) == 0 || !events[len(events)-1].TurnEnd {
				t.Fatalf("turn boundary last: %+v", events)
			}
			active = ""
		case "item/started", "item/completed":
			kind := msg.str("item", "type")
			if kind != "commandExecution" && kind != "mcpToolCall" {
				continue
			}
			if msg.str("turnId") != active {
				late++
				if len(events) != 0 {
					t.Fatalf("late old-turn tool event polluted current turn: %+v", events)
				}
				continue
			}
			batches := codexBatches(t, events)
			if len(batches) != 1 {
				t.Fatalf("recorded %s must produce ordered tool state: %+v", msg.Method, events)
			}
			batch := batches[0]
			if msg.Method == "item/started" {
				starts++
				if batch.Running == nil || batch.EndedAt != nil || batch.CallCount < 1 {
					t.Fatalf("tool starts running: %+v", batch)
				}
				if kind == "mcpToolCall" && batch.Running.ToolName != msg.str("item", "server")+"/"+msg.str("item", "tool") {
					t.Fatalf("MCP server/tool identity: %+v", batch)
				}
			} else {
				completions++
				if batch.EndedAt == nil || batch.Running != nil {
					t.Fatalf("tool completion before turn end: %+v", batch)
				}
				if msg.str("item", "status") == "declined" && batch.Failed == 0 {
					t.Fatalf("declined tool must be failed: %+v", batch)
				}
			}
		}
	}
	if starts != 5 || completions != 4 || late != 1 {
		t.Fatalf("recorded live tools: %d starts, %d completions, %d late completions", starts, completions, late)
	}
}

func TestCodexMapperRecordedResumeAndStaleApproval(t *testing.T) {
	r := feedCodexFixture(t, "codex-0.160.0-resume.jsonl")
	if turns := codexTurns(r.events); len(turns) != 3 {
		t.Fatalf("foreign subagent turn must not end the lead turn: %d", len(turns))
	}
	failures := 0
	for _, answer := range r.answers {
		if !answer.ok {
			failures++
			if !strings.Contains(answer.err, "no rollout found") && !strings.Contains(answer.err, "required MCP servers failed") {
				t.Fatalf("unexpected recorded error: %+v", answer)
			}
		}
	}
	if failures != 2 {
		t.Fatalf("missing resume and refused MCP errors: %d", failures)
	}
	r = feedCodexFixture(t, "codex-0.160.0-stale-approval.jsonl")
	turns := codexTurns(r.events)
	if len(turns) != 2 || turns[1][len(turns[1])-1].TurnReason != wslive.TurnAborted {
		t.Fatalf("recorded approval interrupt: %+v", turns)
	}
	settled := 0
	for _, event := range r.events {
		if event.ApprovalDone == "0" {
			settled++
		}
	}
	if settled != 1 {
		t.Fatalf("turn end settles once; later serverRequest/resolved is stale: %d", settled)
	}
	if _, _, err := r.m.approvalAnswer("0", true); err == nil {
		t.Fatal("late answer to the recorded interrupted approval was accepted")
	}
}

// Synthetic notifications supplement the recorded streams with negative
// cases and provider failures not present in those recordings.
func TestCodexMapperSyntheticFilteringAndErrors(t *testing.T) {
	r := newCodexRun("s")
	r.feed(`{"method":"turn/started","params":{"threadId":"s","turn":{"id":"t"}}}`)
	for _, line := range []string{
		`not json`,
		`{"method":"future/notification","params":{"threadId":"s","turnId":"t"}}`,
		`{"method":"turn/started","params":{"threadId":"foreign","turn":{"id":"x"}}}`,
		`{"method":"turn/completed","params":{"threadId":"foreign","turn":{"id":"t","status":"completed"}}}`,
		`{"method":"thread/status/changed","params":{"threadId":"foreign","status":{"type":"idle"}}}`,
		`{"method":"item/agentMessage/delta","params":{"threadId":"foreign","turnId":"t","itemId":"a","delta":"wrong"}}`,
		`{"method":"item/completed","params":{"threadId":"s","turnId":"old","item":{"id":"a","type":"agentMessage","text":"stale"}}}`,
		`{"method":"thread/tokenUsage/updated","params":{"threadId":"s","turnId":"old","tokenUsage":{"last":{"totalTokens":123}}}}`,
		`{"method":"error","params":{"threadId":"s","turnId":"old","error":{"message":"stale"},"willRetry":true}}`,
		`{"method":"turn/completed","params":{"threadId":"s","turn":{"id":"old","status":"failed"}}}`,
		`{"id":"wsc/turn/previous/1","result":{"turn":{"id":"old","status":"inProgress"}}}`,
	} {
		if got := r.feed(line); len(got) != 0 {
			t.Fatalf("ignored line %s emitted %+v", line, got)
		}
	}
	for i := 1; i <= 2; i++ {
		events := r.feed(`{"method":"error","params":{"threadId":"s","turnId":"t","error":{"message":"retry me"},"willRetry":true}}`)
		if len(events) != 1 || events[0].Retry == nil || events[0].Retry.Attempt != i || events[0].Retry.Error != "retry me" {
			t.Fatalf("retry %d: %+v", i, events)
		}
	}
	events := r.feed(`{"method":"item/agentMessage/delta","params":{"threadId":"s","turnId":"t","itemId":"a","delta":"hello"}}`)
	if len(events) != 2 || !events[0].RetryEnd || events[1].Live.Text != "hello" || events[1].Live.Final {
		t.Fatalf("streamed text clears retry before reply: %+v", events)
	}
	events = r.feed(`{"method":"item/agentMessage/delta","params":{"threadId":"s","turnId":"t","itemId":"a","delta":" world"}}`)
	if events[1].Live.Text != "hello world" {
		t.Fatalf("ordered accumulated deltas: %+v", events)
	}
	events = r.feed(`{"method":"error","params":{"threadId":"s","turnId":"t","error":{"message":"login needed","codexErrorInfo":"unauthorized"},"willRetry":false}}`)
	if len(events) != 2 || events[0].Alert == nil || events[0].Alert.Kind != wslive.NoticeLogin || events[1].Notice != "codex: login needed" {
		t.Fatalf("typed login alert precedes notice: %+v", events)
	}
	events = r.feed(`{"method":"turn/completed","params":{"threadId":"s","turn":{"id":"t","status":"failed","error":{"message":"quota","codexErrorInfo":"usageLimitExceeded"},"items":[{"id":"a","type":"agentMessage","text":"final"}]}}}`)
	if len(events) != 5 || events[0].Live == nil || !events[0].Live.Final || events[0].Live.Text != "final" || events[2].Alert == nil || events[2].Alert.Kind != wslive.NoticeUsageLimit || !events[4].TurnEnd || events[4].TurnError != "quota" {
		t.Fatalf("final reply then typed error then failed turn end: %+v", events)
	}
	if got := r.feed(`{"method":"turn/completed","params":{"threadId":"s","turn":{"id":"t","status":"failed"}}}`); len(got) != 0 {
		t.Fatalf("duplicate end: %+v", got)
	}
}

func TestCodexMapperSyntheticBinaryApprovals(t *testing.T) {
	for _, method := range []string{"item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval", "mcpServer/elicitation/request"} {
		for _, id := range []string{`7`, `"7"`} {
			for _, allow := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/%s/%t", method, id, allow), func(t *testing.T) {
					r := newCodexRun("s")
					r.feed(`{"method":"turn/started","params":{"threadId":"s","turn":{"id":"t"}}}`)
					line := fmt.Sprintf(`{"id":%s,"method":%q,"params":{"threadId":"s","turnId":"t","itemId":"tool","command":"ls","permissions":{"network":{"enabled":true}},"_meta":{"codex_approval_kind":"mcp_tool_call"}}}`, id, method)
					events := r.feed(line)
					if len(events) != 1 || events[0].Approval == nil || events[0].Approval.ID != id || events[0].Approval.ToolUseID != "tool" {
						t.Fatalf("stable raw-id correlation: %+v", events)
					}
					if got := r.feed(line); len(got) != 0 {
						t.Fatalf("duplicate request: %+v", got)
					}
					raw, result, err := r.m.approvalAnswer(id, allow)
					if err != nil || string(raw) != id {
						t.Fatalf("raw id echoed: %s %v", raw, err)
					}
					decision := "decline"
					if allow {
						decision = "accept"
					}
					want := `{"decision":"` + decision + `"}`
					switch method {
					case "mcpServer/elicitation/request":
						want = `{"action":"` + decision + `"}`
					case "item/permissions/requestApproval":
						want = `{"permissions":{},"scope":"turn"}`
						if allow {
							want = `{"permissions":{"network":{"enabled":true}},"scope":"turn"}`
						}
					}
					if got := jsonLine(result); got != want {
						t.Fatalf("binary response %s, want %s; no session grants or persist", got, want)
					}
					if _, _, err := r.m.approvalAnswer(id, !allow); err == nil {
						t.Fatal("duplicate answer accepted")
					}
					events = r.feed(fmt.Sprintf(`{"method":"serverRequest/resolved","params":{"threadId":"s","requestId":%s}}`, id))
					if len(events) != 1 || events[0].ApprovalDone != id {
						t.Fatalf("resolution: %+v", events)
					}
					if _, _, err := r.m.approvalAnswer(id, allow); err == nil {
						t.Fatal("stale answer accepted")
					}
				})
			}
		}
	}
}

func TestCodexMapperSyntheticUnsupportedRequests(t *testing.T) {
	for _, tc := range []struct{ method, thread, turn, message string }{
		{"item/tool/requestUserInput", "s", "t", claudeChatOnlyDenial},
		{"mcpServer/elicitation/request", "s", "t", claudeChatOnlyDenial},
		{"item/tool/call", "s", "t", "does not support"},
		{"account/chatgptAuthTokens/refresh", "s", "t", "does not support"},
		{"attestation/generate", "s", "t", "does not support"},
		{"future/request", "s", "t", "does not support"},
		{"item/commandExecution/requestApproval", "foreign", "t", "another or malformed"},
		{"item/commandExecution/requestApproval", "s", "old", "stale Codex turn"},
	} {
		t.Run(tc.method+"/"+tc.thread+"/"+tc.turn, func(t *testing.T) {
			r := newCodexRun("s")
			r.feed(`{"method":"turn/started","params":{"threadId":"s","turn":{"id":"t"}}}`)
			events := r.feed(fmt.Sprintf(`{"id":"server-id","method":%q,"params":{"threadId":%q,"turnId":%q}}`, tc.method, tc.thread, tc.turn))
			if len(events) != 1 || !strings.Contains(events[0].Notice, tc.message) || len(r.sent) != 1 || len(r.m.approvals) != 0 {
				t.Fatalf("must refuse rather than silently stall or approve: events %+v replies %v", events, r.sent)
			}
			var reply struct {
				ID    string `json:"id"`
				Error struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal([]byte(jsonLine(r.sent[0])), &reply)
			if reply.ID != "server-id" || reply.Error.Code != -32601 || !strings.Contains(reply.Error.Message, tc.message) {
				t.Fatalf("correlated JSON-RPC error: %+v", reply)
			}
		})
	}
}

func TestCodexMapperSyntheticInitShapeAndRecovery(t *testing.T) {
	r := newCodexRun("")
	bad := r.feed(`{"id":"wsc/open/e/1","result":{"thread":{}}}`)
	if len(bad) != 0 || r.answers["wsc/open/e/1"].ok {
		t.Fatalf("missing identity must fail its waiter: %+v %+v", bad, r.answers)
	}
	events := r.feed(`{"id":"wsc/open/e/2","result":{"thread":{"id":"s","cliVersion":"0.161.0"}}}`)
	if len(events) != 4 || events[0].SessionID != "s" || events[1].Harness == nil || events[1].Harness.Version != "0.161.0" || !slices.Equal(events[1].Harness.Demoted, []CapabilityItem{ItemModelEffort, ItemApproval}) || events[2].BackgroundTasks == nil || *events[2].BackgroundTasks != -1 {
		t.Fatalf("identity/version/demotion and unknown background count: %+v", events)
	}
	events = r.feed(`{"id":"wsc/recover/e/3","result":{"thread":{"id":"foreign","status":{"type":"idle"},"turns":[]}}}`)
	if len(events) != 1 || events[0].Notice == "" || events[0].TurnEnd {
		t.Fatalf("foreign recovery cannot release input: %+v", events)
	}
	// Compaction is synthetic: Phase 1 did not record its behavior.
	r.feed(`{"method":"turn/started","params":{"threadId":"s","turn":{"id":"compact"}}}`)
	events = r.feed(`{"method":"item/started","params":{"threadId":"s","turnId":"compact","item":{"id":"c","type":"contextCompaction"}}}`)
	if len(events) != 1 || events[0].Live == nil || events[0].Live.Phase != wslive.CompactStart {
		t.Fatalf("compact start: %+v", events)
	}
	events = r.feed(`{"method":"turn/completed","params":{"threadId":"s","turn":{"id":"compact","status":"interrupted"}}}`)
	if len(events) != 3 || events[0].Live == nil || events[0].Live.Phase != wslive.CompactFailed || !events[2].TurnEnd {
		t.Fatalf("interrupted compaction closes before TurnEnd: %+v", events)
	}
}

func TestCodexModelsSyntheticAdvertisedEfforts(t *testing.T) {
	codexChoices.Lock()
	models, efforts := codexChoices.models, codexChoices.efforts
	codexChoices.Unlock()
	t.Cleanup(func() {
		codexChoices.Lock()
		codexChoices.models, codexChoices.efforts = models, efforts
		codexChoices.Unlock()
	})
	codexModels(json.RawMessage(`{"data":[{"model":"visible-a","supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"high"}]},{"model":"hidden","hidden":true,"supportedReasoningEfforts":[{"reasoningEffort":"hidden-only"}]},{"model":"visible-b","supportedReasoningEfforts":[{"reasoningEffort":"high"},{"reasoningEffort":"xhigh"},{"reasoningEffort":""}]},{"model":"plain"},{"model":"","supportedReasoningEfforts":[{"reasoningEffort":"invalid"}]}]}`))
	got, levels, err := (CodexBackend{}).Choices(context.Background())
	if err != nil || len(got) != 3 || !slices.Equal(levels, []string{"low", "high", "xhigh"}) {
		t.Fatalf("advertised visible choices only: %+v %v %v", got, levels, err)
	}
	for i, id := range []string{"visible-a", "visible-b", "plain"} {
		if got[i].ID != id || got[i].Thinking == nil || *got[i].Thinking != (i != 2) {
			t.Fatalf("choice %d: %+v", i, got[i])
		}
	}
	got[0].ID, levels[0] = "mutated", "mutated"
	again, againLevels, _ := (CodexBackend{}).Choices(context.Background())
	if again[0].ID != "visible-a" || againLevels[0] != "low" {
		t.Fatal("Choices exposed cache slices")
	}
	codexModels(json.RawMessage(`{"data":null}`))
	codexModels(json.RawMessage(`broken`))
	after, afterLevels, _ := (CodexBackend{}).Choices(context.Background())
	if !reflect.DeepEqual(after, again) || !slices.Equal(afterLevels, againLevels) {
		t.Fatal("malformed listing replaced advertised choices")
	}
}
