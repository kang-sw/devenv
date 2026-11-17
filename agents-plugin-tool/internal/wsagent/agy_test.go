package wsagent

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

// agyRun is a recorded agy session fed through a fresh agyMapper the way a
// live process sees it: each client request is re-keyed to the id ws
// writes for its method (agyID kinds), a prompt is registered as sent
// before its line, and the client's own answers and notifications are
// skipped (ws writes them; agy's output is what the mapper reads).
type agyRun struct {
	m       *agyMapper
	events  []Event
	answers map[string]controlAnswer
	sent    []any
}

func (r *agyRun) answer(id string, a controlAnswer) { r.answers[id] = a }
func (r *agyRun) send(v any)                        { r.sent = append(r.sent, v) }

var agyFixtureKinds = map[string]string{
	"initialize":                agyKindInit,
	"session/new":               agyKindNew,
	"session/resume":            agyKindResume,
	"session/set_config_option": agyKindConfig,
	"session/set_mode":          agyKindMode,
	"session/prompt":            agyKindPrompt,
}

func feedAgyFixture(t *testing.T, name string) *agyRun {
	t.Helper()
	r := &agyRun{m: newAgyMapper(""), answers: map[string]controlAnswer{}}
	ids := map[string]string{}
	n := 0
	for _, line := range readLeadFixture(t, name) {
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(line.raw, &msg); err != nil {
			t.Fatal(err)
		}
		_, hasMethod := msg["method"]
		id, hasID := msg["id"]
		if line.in {
			if !hasMethod || !hasID {
				continue
			}
			var method string
			_ = json.Unmarshal(msg["method"], &method)
			kind := agyFixtureKinds[method]
			if kind == "" {
				kind = "other"
			}
			n++
			wsID := agyRequestMark + "-" + kind + "-fixture-" + strconv.Itoa(n)
			ids[string(id)] = wsID
			if kind == agyKindPrompt {
				r.m.promptSent(wsID)
			}
			continue
		}
		raw := line.raw
		if !hasMethod && hasID {
			if wsID, ok := ids[string(id)]; ok {
				msg["id"], _ = json.Marshal(wsID)
				raw, _ = json.Marshal(msg)
			}
		}
		r.events = append(r.events, r.m.mapLine(raw, r)...)
	}
	return r
}

func (r *agyRun) count(match func(Event) bool) int {
	n := 0
	for _, ev := range r.events {
		if match(ev) {
			n++
		}
	}
	return n
}

func (r *agyRun) harness() []HarnessReport {
	var out []HarnessReport
	for _, ev := range r.events {
		if ev.Harness != nil {
			out = append(out, *ev.Harness)
		}
	}
	return out
}

// turns splits the events at each TurnEnd, the TurnEnd last in its part.
func (r *agyRun) turns() [][]Event {
	var out [][]Event
	var cur []Event
	for _, ev := range r.events {
		cur = append(cur, ev)
		if ev.TurnEnd {
			out = append(out, cur)
			cur = nil
		}
	}
	return out
}

// checkAgyCallEnds: a call ends by its own final update, which carries a
// content list, not by the turn end cutting it short. The ended batches
// hold every call agy finished and fail exactly the ones it reported
// failed (denied or cancelled).
func checkAgyCallEnds(t *testing.T, msgs []codexMsg, r *agyRun) {
	t.Helper()
	finals := map[string]string{}
	for _, m := range msgs {
		if m.Method == "session/update" {
			id, _ := jsonAt(m.Params, "update", "toolCallId").(string)
			if st, _ := jsonAt(m.Params, "update", "status").(string); st == "completed" || st == "failed" {
				finals[id] = st
			}
		}
	}
	wantFailed := 0
	for _, st := range finals {
		if st == "failed" {
			wantFailed++
		}
	}
	lastBatch := map[string]wireBatch{}
	for _, ev := range r.events {
		if ev.Live == nil || ev.Live.Kind != wslive.KindAgent {
			continue
		}
		for _, raw := range ev.Live.Batches {
			var b wireBatch
			if err := json.Unmarshal(raw, &b); err != nil {
				t.Fatal(err)
			}
			lastBatch[b.Epoch+"/"+strconv.FormatInt(b.Seq, 10)] = b
		}
	}
	calls, failed := 0, 0
	for _, b := range lastBatch {
		calls += b.CallCount
		failed += b.Failed
	}
	if wantFailed == 0 || calls != len(finals) || failed != wantFailed {
		t.Fatalf("calls %d failed %d; agy finished %d calls, %d failed", calls, failed, len(finals), wantFailed)
	}
}

func finalReplies(evs []Event) []string {
	var out []string
	for _, ev := range evs {
		if ev.Live != nil && ev.Live.Kind == wslive.KindReply && ev.Live.Final {
			out = append(out, ev.Live.Text)
		}
	}
	return out
}

// TestAgyMapperLeadSession: the recorded lead session maps to its turns
// (the steered pair is one), each approval keyed by its tool call and
// settled, the cancelled turn aborted, the session id, mode changes, the
// harness version with nothing demoted, and usage.
func TestAgyMapperLeadSession(t *testing.T) {
	msgs := readCodexFixture(t, "agy-1.5.9-lead-session.jsonl")
	r := feedAgyFixture(t, "agy-1.5.9-lead-session.jsonl")

	reports := r.harness()
	// The Phase 1 recordings keep the version the probe started on in
	// their names; agy updated itself during it, and this one reports
	// 1.5.10 (TestAgyFixtureVersions).
	if len(reports) == 0 || reports[0].Version != "1.5.10" {
		t.Fatalf("initialize reports the version: %+v", reports)
	}
	for _, rep := range reports {
		if len(rep.Demoted) != 0 {
			t.Fatalf("the recorded session demotes nothing: %+v", rep)
		}
	}
	var sessions, modes []string
	for _, ev := range r.events {
		if ev.SessionID != "" {
			sessions = append(sessions, ev.SessionID)
		}
		if ev.PermissionMode != "" {
			modes = append(modes, ev.PermissionMode)
		}
	}
	if len(sessions) != 1 || sessions[0] != jsonAt(msgs[3].Result, "sessionId") {
		t.Fatalf("the session id: %v", sessions)
	}
	if !slices.Equal(modes, []string{"default", "accept-edits", "default"}) {
		t.Fatalf("the modes agy reported: %v", modes)
	}

	// Prompts 3, 4, 5, 6 (cancelled), 7+8 (8 steers 7) and 14.
	turns := r.turns()
	if len(turns) != 6 || r.count(func(ev Event) bool { return ev.TurnStart }) != 6 {
		t.Fatalf("six turns, the steered pair one: %d ends, %d starts", len(turns), r.count(func(ev Event) bool { return ev.TurnStart }))
	}
	for i, turn := range turns {
		end := turn[len(turn)-1]
		if end.TurnError != "" || (i == 3) != (end.TurnReason == wslive.TurnAborted) {
			t.Fatalf("turn %d ends %+v", i, end)
		}
	}
	if got := finalReplies(turns[0]); len(got) != 1 || got[0] != acpText(msgs, 6, 19) {
		t.Fatalf("the first turn's reply: %q", got)
	}
	if got := finalReplies(turns[4]); len(got) != 1 || got[0] != "QUOKKA" {
		t.Fatalf("the steered turn's reply: %q", got)
	}

	var asked, settled []string
	for _, ev := range r.events {
		if ev.Approval != nil {
			asked = append(asked, ev.Approval.ID)
			if ev.Approval.ToolUseID != ev.Approval.ID || ev.Approval.Tool == "" || len(ev.Approval.Input) == 0 {
				t.Fatalf("an approval carries its call: %+v", ev.Approval)
			}
		}
		if ev.ApprovalDone != "" {
			settled = append(settled, ev.ApprovalDone)
		}
	}
	want := []string{"call_6370694005842387702", "call_17200880840140070632", "call_14700971740588177344", "call_4142686772815747814"}
	if !slices.Equal(asked, want) || !slices.Equal(settled, want) {
		t.Fatalf("approvals asked %v settled %v", asked, settled)
	}
	checkAgyCallEnds(t, msgs, r)
	if len(r.m.approvals) != 0 || len(r.m.outstanding) != 0 || r.m.inTurn {
		t.Fatalf("nothing is left pending: %+v %+v %v", r.m.approvals, r.m.outstanding, r.m.inTurn)
	}

	var usage *wslive.UsageContext
	for _, ev := range turns[0] {
		if ev.Live != nil && ev.Live.Kind == wslive.KindStatus {
			usage = ev.Live.Context
		}
	}
	if usage == nil || usage.Tokens == nil || *usage.Tokens != 11538 || usage.Window != 1048576 || usage.Percent == nil {
		t.Fatalf("usage after the first turn: %+v", usage)
	}
	if !r.m.offers("thought_level") || !r.m.offers("model") || r.m.mode() != "default" {
		t.Fatalf("the session's options and mode: %v %q", r.m.options, r.m.mode())
	}
	if r.count(func(ev Event) bool { return ev.Live != nil && ev.Live.Kind == wslive.KindCommands }) == 0 {
		t.Fatal("the command list is shown")
	}
	for id, a := range r.answers {
		if !a.ok {
			t.Fatalf("control %s answered %+v", id, a)
		}
	}
}

// TestAgyMapperHostedLead: the Phase 2 smoke's stream on 1.5.10, recorded
// at the harness's stdio under a hosted lead (sanitized: session id, token,
// session key). It resumes the session, then: an approval allowed, one
// denied (agy ends the turn with no reply), an interrupt with one pending,
// a follow-up held to the turn end, a steer during a running command, a
// model switch to Claude, gpt-oss and back with an effort, accept-edits and
// back, /compact, an agy skill and ws tool calls.
func TestAgyMapperHostedLead(t *testing.T) {
	msgs := readCodexFixture(t, "agy-1.5.10-hosted-lead.jsonl")
	r := feedAgyFixture(t, "agy-1.5.10-hosted-lead.jsonl")
	for _, rep := range r.harness() {
		if rep.Version != "1.5.10" || len(rep.Demoted) != 0 {
			t.Fatalf("the harness report: %+v", rep)
		}
	}
	// Sixteen prompts; the steered pair is one turn.
	turns := r.turns()
	if len(turns) != 15 {
		t.Fatalf("fifteen turns: %d", len(turns))
	}
	for i, turn := range turns {
		end := turn[len(turn)-1]
		if end.TurnError != "" || (i == 2) != (end.TurnReason == wslive.TurnAborted) {
			t.Fatalf("turn %d ends %+v", i, end)
		}
	}
	if got := finalReplies(turns[5]); len(got) == 0 || !strings.Contains(got[len(got)-1], "QUOKKA") {
		t.Fatalf("the steered turn's reply: %q", got)
	}
	// agy sends a Claude model's refusal of ws's dotted tool names as reply
	// text and ends the turn normally.
	if got := finalReplies(turns[6]); len(got) != 1 || !strings.Contains(got[0], "INVALID_ARGUMENT") {
		t.Fatalf("the Claude model's turn: %q", got)
	}
	asked, settled := 0, 0
	for _, ev := range r.events {
		if ev.Approval != nil {
			asked++
		}
		if ev.ApprovalDone != "" {
			settled++
		}
	}
	if asked != 10 || settled != asked || len(r.m.approvals) != 0 || len(r.m.outstanding) != 0 || r.m.inTurn {
		t.Fatalf("approvals asked %d settled %d; pending %v %v %v", asked, settled, r.m.approvals, r.m.outstanding, r.m.inTurn)
	}
	checkAgyCallEnds(t, msgs, r)
	var modes []string
	for _, ev := range r.events {
		if ev.PermissionMode != "" {
			modes = append(modes, ev.PermissionMode)
		}
	}
	if !slices.Equal(modes, []string{"default", "accept-edits", "default"}) {
		t.Fatalf("the modes agy reported: %v", modes)
	}
	// /compact (turn 10) leaves no event; the next turn's usage drops.
	usage := func(turn []Event) int64 {
		var n int64
		for _, ev := range turn {
			if ev.Live != nil && ev.Live.Kind == wslive.KindStatus && ev.Live.Context != nil && ev.Live.Context.Tokens != nil {
				n = *ev.Live.Context.Tokens
			}
		}
		return n
	}
	if before, after := usage(turns[10]), usage(turns[11]); after == 0 || after >= before {
		t.Fatalf("usage around /compact: %d then %d", before, after)
	}
	if !r.m.offers("thought_level") || !r.m.offers("model") {
		t.Fatalf("back on Gemini the effort option returns: %v", r.m.options)
	}
	var wsCall bool
	for _, ev := range turns[14] {
		if ev.Live != nil && ev.Live.Kind == wslive.KindAgent {
			for _, raw := range ev.Live.Batches {
				wsCall = wsCall || strings.Contains(string(raw), `"mcp_ws_git.status"`)
			}
		}
	}
	if !wsCall {
		t.Fatal("ws tool calls keep their dotted names")
	}
}

// TestAgyMapperStaleApproval: a cancel leaves the permission request
// unanswered and nothing withdraws it; the prompt's cancelled answer ends
// the turn aborted, the request settled with it.
func TestAgyMapperStaleApproval(t *testing.T) {
	r := feedAgyFixture(t, "agy-1.5.9-stale-approval.jsonl")
	turns := r.turns()
	if len(turns) != 3 {
		t.Fatalf("three turns: %d", len(turns))
	}
	if end := turns[1][len(turns[1])-1]; end.TurnReason != wslive.TurnAborted || end.TurnError != "" {
		t.Fatalf("the cancelled turn ends aborted: %+v", end)
	}
	if !slices.ContainsFunc(turns[1], func(ev Event) bool { return ev.ApprovalDone == "call_200098001102663881" }) {
		t.Fatal("the cancelled turn settles its request")
	}

	// Without the call's failed status, the prompt's answer settles it.
	m := newAgyMapper("s")
	rr := &agyRun{m: m, answers: map[string]controlAnswer{}}
	m.promptSent("ws-p-e-1")
	var evs []Event
	for _, line := range []string{
		`{"jsonrpc":"2.0","method":"session/request_permission","params":{"sessionId":"s","toolCall":{"toolCallId":"c1","name":"run_command","args":{"CommandLine":"ls"},"title":"ls"},"options":[{"optionId":"allow","kind":"allow_once"},{"optionId":"deny","kind":"reject_once"}]},"id":"7"}`,
		`{"jsonrpc":"2.0","result":{"stopReason":"cancelled"},"id":"ws-p-e-1"}`,
	} {
		evs = append(evs, m.mapLine([]byte(line), rr)...)
	}
	if !evs[0].TurnStart || evs[1].Approval == nil || evs[1].Approval.ID != "c1" || evs[2].ApprovalDone != "c1" || !evs[len(evs)-1].TurnEnd || evs[len(evs)-1].TurnReason != wslive.TurnAborted {
		t.Fatalf("a pending request settles at the turn's end: %+v", evs)
	}
}

// TestAgyMapperResume: a failed resume answers its waiter with the error;
// a resumed session reports its id; a control ws does not read is
// answered all the same.
func TestAgyMapperResume(t *testing.T) {
	r := feedAgyFixture(t, "agy-1.5.9-resume.jsonl")
	var failed, ok int
	for id, a := range r.answers {
		kind, _ := agyIDKind(id)
		switch {
		case kind == agyKindResume && !a.ok && strings.Contains(a.err, "Failed to read session"):
			failed++
		case a.ok:
			ok++
		}
	}
	if failed != 1 || ok < 4 {
		t.Fatalf("answers: %d failed resumes, %d ok (%+v)", failed, ok, r.answers)
	}
	var sessions []string
	for _, ev := range r.events {
		if ev.SessionID != "" {
			sessions = append(sessions, ev.SessionID)
		}
	}
	if !slices.Equal(sessions, []string{"bafd4e08-ee30-4936-babd-d37d834c5e29", "b1a8d6a4-a4d8-47ae-b90d-748773141c34"}) {
		t.Fatalf("the resumed and the new session: %v", sessions)
	}
	if len(r.turns()) != 4 {
		t.Fatalf("four turns: %d", len(r.turns()))
	}
}

// TestAgyAnswerApprovalByKind: the answer echoes the request's raw id and
// picks the option of the asked kind, never a fixed option id.
func TestAgyAnswerApprovalByKind(t *testing.T) {
	m := newAgyMapper("s")
	rr := &agyRun{m: m, answers: map[string]controlAnswer{}}
	m.mapLine([]byte(`{"jsonrpc":"2.0","method":"session/request_permission","params":{"toolCall":{"toolCallId":"c1","name":"x"},"options":[{"optionId":"no","kind":"reject_once"},{"optionId":"yes","kind":"allow_once"}]},"id":"1"}`), rr)
	m.mapLine([]byte(`{"jsonrpc":"2.0","method":"session/request_permission","params":{"toolCall":{"name":"y"},"options":[{"optionId":"ok","kind":"allow_once"}]},"id":2}`), rr)
	raw, option, err := m.answerFor("c1", true)
	if err != nil || string(raw) != `"1"` || option != "yes" {
		t.Fatalf("allow: %s %q %v", raw, option, err)
	}
	if raw, option, err = m.answerFor("c1", false); err != nil || string(raw) != `"1"` || option != "no" {
		t.Fatalf("deny: %s %q %v", raw, option, err)
	}
	// A request without a tool call id is keyed by its raw id.
	if raw, option, err = m.answerFor("rpc:2", true); err != nil || string(raw) != `2` || option != "ok" {
		t.Fatalf("by request id: %s %q %v", raw, option, err)
	}
	if _, _, err = m.answerFor("rpc:2", false); err == nil {
		t.Fatal("a request offering no deny option refuses a denial")
	}
	if _, _, err = m.answerFor("gone", true); err == nil {
		t.Fatal("an unknown request is refused")
	}
}

// TestAgyMapperAnswersUnknownRequests: an agent request ws does not serve
// gets an error answer at once, so agy never waits on it.
func TestAgyMapperAnswersUnknownRequests(t *testing.T) {
	m := newAgyMapper("s")
	rr := &agyRun{m: m, answers: map[string]controlAnswer{}}
	evs := m.mapLine([]byte(`{"jsonrpc":"2.0","method":"fs/read_text_file","params":{},"id":"9"}`), rr)
	if len(rr.sent) != 1 || len(evs) != 1 || evs[0].Notice == "" {
		t.Fatalf("answered %v, events %+v", rr.sent, evs)
	}
	raw, _ := json.Marshal(rr.sent[0])
	if !strings.Contains(string(raw), `"id":"9"`) || !strings.Contains(string(raw), `-32601`) {
		t.Fatalf("the error answer: %s", raw)
	}
}

// TestAgyMapperShapeDemotes: a session without a model option or modes,
// an init without resume, and a command list without compact demote
// their items; a model without a thought level demotes nothing.
func TestAgyMapperShapeDemotes(t *testing.T) {
	m := newAgyMapper("")
	rr := &agyRun{m: m, answers: map[string]controlAnswer{}}
	last := func(evs []Event) HarnessReport {
		t.Helper()
		for i := len(evs) - 1; i >= 0; i-- {
			if evs[i].Harness != nil {
				return *evs[i].Harness
			}
		}
		t.Fatalf("no harness report in %+v", evs)
		return HarnessReport{}
	}
	rep := last(m.mapLine([]byte(`{"jsonrpc":"2.0","result":{"protocolVersion":1,"agentInfo":{"version":"1.5.9"},"agentCapabilities":{"sessionCapabilities":{}}},"id":"ws-init-e-1"}`), rr))
	if rep.Version != "1.5.9" || !slices.Equal(rep.Demoted, []CapabilityItem{ItemResume}) {
		t.Fatalf("no resume capability: %+v", rep)
	}
	rep = last(m.mapLine([]byte(`{"jsonrpc":"2.0","result":{"sessionId":"s","configOptions":[{"id":"thought_level","currentValue":"high","options":[{"value":"high"}]}]},"id":"ws-new-e-2"}`), rr))
	if !slices.Equal(rep.Demoted, []CapabilityItem{ItemResume, ItemModelEffort, ItemApproval}) {
		t.Fatalf("no model option, no modes: %+v", rep)
	}
	// An effort applies only where the option takes it (SetEffort keeps
	// the rest for a later model).
	if !m.offersValue("thought_level", "high") || m.offersValue("thought_level", "low") {
		t.Fatalf("offered thought levels: %v", m.options)
	}
	rep = last(m.mapLine([]byte(`{"jsonrpc":"2.0","result":{"sessionId":"s","modes":{"currentModeId":"default","availableModes":[{"id":"default"}]},"configOptions":[{"id":"model","currentValue":"m","options":[{"value":"m"}]}]},"id":"ws-resume-e-3"}`), rr))
	if !slices.Equal(rep.Demoted, []CapabilityItem{ItemResume}) {
		t.Fatalf("a model without a thought level: %+v", rep)
	}
	if m.offers("thought_level") || !m.offers("model") {
		t.Fatalf("offered options: %v", m.options)
	}
	rep = last(m.mapLine([]byte(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"available_commands_update","availableCommands":[{"name":"goal"}]}}}`), rr))
	if !slices.Contains(rep.Demoted, ItemCompaction) {
		t.Fatalf("a list without compact: %+v", rep)
	}
	evs := m.mapLine([]byte(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"available_commands_update","availableCommands":[{"name":"compact","description":"Compact"}]}}}`), rr)
	if rep = last(evs); slices.Contains(rep.Demoted, ItemCompaction) {
		t.Fatalf("compact listed again: %+v", rep)
	}
	evs = m.mapLine([]byte(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"available_commands_update","availableCommands":[{"name":"compact"}]}}}`), rr)
	if slices.ContainsFunc(evs, func(ev Event) bool { return ev.Harness != nil }) {
		t.Fatalf("an unchanged list reports nothing new: %+v", evs)
	}
}

// TestAgyMapperTrackerWrite: a completed ws tracker.update call raises
// the tracker write; a failed one does not.
func TestAgyMapperTrackerWrite(t *testing.T) {
	m := newAgyMapper("s")
	rr := &agyRun{m: m, answers: map[string]controlAnswer{}}
	var evs []Event
	for _, line := range []string{
		`{"jsonrpc":"2.0","method":"session/update","params":{"update":{"sessionUpdate":"tool_call","toolCallId":"a","name":"mcp_ws_tracker.update","status":"in_progress","args":{}}}}`,
		`{"jsonrpc":"2.0","method":"session/update","params":{"update":{"sessionUpdate":"tool_call_update","toolCallId":"a","name":"mcp_ws_tracker.update","status":"completed"}}}`,
		`{"jsonrpc":"2.0","method":"session/update","params":{"update":{"sessionUpdate":"tool_call","toolCallId":"b","name":"mcp_ws_tracker.update","status":"in_progress","args":{}}}}`,
		`{"jsonrpc":"2.0","method":"session/update","params":{"update":{"sessionUpdate":"tool_call_update","toolCallId":"b","name":"mcp_ws_tracker.update","status":"failed"}}}`,
	} {
		evs = append(evs, m.mapLine([]byte(line), rr)...)
	}
	if n := len(slices.DeleteFunc(slices.Clone(evs), func(ev Event) bool { return !ev.TrackerWrite })); n != 1 {
		t.Fatalf("%d tracker writes: %+v", n, evs)
	}
	if agyMCPToolName(ReservedMCPServer, TrackerWriteTool) != "mcp_ws_tracker.update" {
		t.Fatal(agyMCPToolName(ReservedMCPServer, TrackerWriteTool))
	}
}

// TestAgyRingSkipMatchesThoughts: the holder's ring skip drops agy's
// thought chunks and keeps every other line.
func TestAgyRingSkipMatchesThoughts(t *testing.T) {
	skip := regexp.MustCompile(agyRingSkip)
	thoughts, kept := 0, 0
	for _, line := range readLeadFixture(t, "agy-1.5.9-lead-session.jsonl") {
		if line.in {
			continue
		}
		var m codexMsg
		_ = json.Unmarshal(line.raw, &m)
		if skip.Match(line.raw) != (acpUpdate(m) == "agent_thought_chunk") {
			t.Fatalf("ring skip on %.120s", line.raw)
		}
		if acpUpdate(m) == "agent_thought_chunk" {
			thoughts++
		} else {
			kept++
		}
	}
	if thoughts == 0 || kept == 0 {
		t.Fatalf("%d thoughts, %d kept", thoughts, kept)
	}
}

// TestAgyFixtureVersions: the catalog's guaranteed range is the range
// of versions the recorded sessions report (agentInfo.version), whatever
// a fixture's name says.
func TestAgyFixtureVersions(t *testing.T) {
	names, err := filepath.Glob("testdata/agy-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, path := range names {
		for _, m := range readCodexFixture(t, filepath.Base(path)) {
			if v, ok := jsonAt(m.Result, "agentInfo", "version").(string); ok && !m.in {
				seen[v] = true
			}
		}
	}
	p := AgyCatalog()
	if !seen[p.Oldest] || !seen[p.Newest] {
		t.Fatalf("recorded versions %v; the catalog guarantees %s..%s", seen, p.Oldest, p.Newest)
	}
	lead := feedAgyFixture(t, "agy-1.5.9-lead-session.jsonl").harness()
	if caps := p.Resolve(RoleLead, lead[0].Version); caps.Unverified {
		t.Fatalf("the recorded lead session resolves unverified at %s", lead[0].Version)
	}
}

// TestAgyCatalogValues pins the agy lead profile: what the adapter
// delivers for a hosted agy lead at each guaranteed version.
func TestAgyCatalogValues(t *testing.T) {
	p := AgyCatalog()
	if err := validateCatalog(p); err != nil {
		t.Fatal(err)
	}
	want := map[CapabilityItem]CapabilityGrade{
		ItemSkillList: GradeUnsupported, ItemSkillInjection: GradeNative, ItemCompaction: GradeNative,
		ItemRewind: GradeUnsupported, ItemToolRemoval: GradeUnsupported, ItemSubagentRemoval: GradeUnsupported,
		ItemModelEffort: GradeNative, ItemApproval: GradeNative, ItemSteer: GradeNative,
		ItemInterrupt: GradeNative, ItemFollowUp: GradeSubstitute, ItemResume: GradeNative,
		ItemReasoningStream: GradeUnsupported, ItemUsageQuery: GradeNative,
	}
	for _, version := range []string{p.Oldest, p.Newest} {
		caps := p.Resolve(RoleLead, version)
		if caps.Unverified {
			t.Errorf("lead at %s resolved unverified", version)
		}
		for _, item := range CatalogItems {
			if got := caps.Grade(item); got != want[item] {
				t.Errorf("lead %s at %s = %s, want %s", item, version, got, want[item])
			}
		}
	}
	for _, e := range p.Entries {
		if e.Role != RoleLead {
			t.Errorf("entry %s declares role %q: agy hosts leads only", e.Item, e.Role)
		}
	}
}
