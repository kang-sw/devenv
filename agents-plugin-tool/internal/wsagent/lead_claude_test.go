package wsagent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// The claude-lead-*.jsonl fixtures are cut from recordings of a headless
// claude started as a lead conversation starts it (-p, stream-json both
// ways, --permission-prompt-tool stdio). A line the recording's driver
// wrote to the harness's input is kept in place behind a ">> " mark; every
// other line is harness output. Long lists and texts are shortened and
// paths made neutral; nothing a mapper reads was changed.

// leadLine is one fixture line: harness output, or (in) a line written to
// the harness's input.
type leadLine struct {
	in  bool
	raw []byte
}

// field reads a top-level string field of the line.
func (l leadLine) field(name string) string {
	var m map[string]json.RawMessage
	_ = json.Unmarshal(l.raw, &m)
	var s string
	_ = json.Unmarshal(m[name], &s)
	return s
}

func readLeadFixture(t *testing.T, name string) []leadLine {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []leadLine
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		raw := bytes.TrimSpace(sc.Bytes())
		if len(raw) == 0 {
			continue
		}
		line := leadLine{raw: append([]byte(nil), raw...)}
		if rest, ok := bytes.CutPrefix(raw, []byte(">> ")); ok {
			line = leadLine{in: true, raw: append([]byte(nil), rest...)}
		}
		if !json.Valid(line.raw) {
			t.Fatalf("%s: a line is not JSON: %.80s", name, line.raw)
		}
		out = append(out, line)
	}
	return out
}

// findLine is the index of the first fixture line at or after from that match holds for.
func findLine(t *testing.T, lines []leadLine, from int, what string, match func(leadLine) bool) int {
	t.Helper()
	for i := from; i < len(lines); i++ {
		if match(lines[i]) {
			return i
		}
	}
	t.Fatalf("the fixture has no %s line", what)
	return -1
}

func leadSpec() StartSpec {
	return StartSpec{Root: "/work/repo", Profile: Profile{Backend: BackendClaude, Lead: true}, Lead: &LeadSpec{}}
}

// leadReplay runs lines through a lead's claude mapper and keeps the
// conversation the runner would: a turn opens at a send or at the first
// TurnStart with none running, closes at a TurnEnd, and waiting prompts are
// listed until an ApprovalDone names them.
type leadReplay struct {
	t       *testing.T
	m       *claudeMapper
	st      *wslive.State
	clock   int64
	replies *recordedReplies
	// asks are the get_context_usage requests the mapper sent, kept out of
	// replies so those hold its answers alone.
	asks   []map[string]any
	ring   []wslive.RingEntry
	events []Event

	turn bool
	// opened names what opened each turn: "send", or the type (a system
	// line's subtype) of the output line whose TurnStart did.
	opened    []string
	ends      int
	turnErrs  []string
	approvals []ApprovalRequest
	modes     []string
	tasks     []int
	sessions  []string
	states    []string
	notices   []string
	// held counts messages sent during a turn: the runner submits them at
	// the turn's end, which opens the next turn ("flush").
	held int
}

func newLeadReplay(t *testing.T) *leadReplay {
	r := &leadReplay{t: t, m: newClaudeMapper(leadSpec()), st: wslive.NewState("test-key"), clock: 1_000_000, replies: &recordedReplies{}}
	r.m.now = func() time.Time { r.clock += 100; return time.UnixMilli(r.clock) }
	return r
}

func (r *leadReplay) apply(ev wslive.Event) {
	r.t.Helper()
	res, err := r.st.Apply(ev, r.clock)
	if err != nil {
		r.t.Fatalf("apply %s: %v", ev.Kind, err)
	}
	r.ring = append(r.ring, res.Ring...)
}

// line feeds one fixture line and returns the events an output line
// mapped to. A user line on the input opens a turn, as the runner's send
// does.
func (r *leadReplay) line(l leadLine) []Event {
	r.t.Helper()
	if l.in {
		if l.field("type") == "user" && !r.turn {
			r.turn = true
			r.opened = append(r.opened, "send")
			r.apply(wslive.Event{Kind: wslive.KindTurnStart, Origin: wslive.OriginWhiteboard})
		}
		return nil
	}
	kind := l.field("type")
	if kind == "system" {
		kind = l.field("subtype")
	}
	evs := r.m.mapLine(l.raw, askSplitter{r.replies, &r.asks})
	for _, ev := range evs {
		r.events = append(r.events, ev)
		switch {
		case ev.Fatal != nil:
			r.t.Fatalf("a lead's line should never refuse the launch: %v", ev.Fatal)
		case ev.Live != nil:
			r.apply(*ev.Live)
		case ev.SessionID != "":
			r.sessions = append(r.sessions, ev.SessionID)
		case ev.TurnStart:
			if !r.turn {
				r.turn = true
				r.opened = append(r.opened, kind)
				r.apply(wslive.Event{Kind: wslive.KindTurnStart})
			}
		case ev.TurnEnd:
			if r.turn {
				r.turn = false
				r.ends++
				r.apply(wslive.Event{Kind: wslive.KindTurnEnd})
			}
			if ev.TurnError != "" {
				r.turnErrs = append(r.turnErrs, ev.TurnError)
			}
			if r.held > 0 {
				r.held, r.turn = 0, true
				r.opened = append(r.opened, "flush")
				r.apply(wslive.Event{Kind: wslive.KindTurnStart, Origin: wslive.OriginWhiteboard})
			}
		case ev.HarnessState != "":
			r.states = append(r.states, ev.HarnessState)
		case ev.Notice != "":
			r.notices = append(r.notices, ev.Notice)
		case ev.Approval != nil:
			r.approvals = append(r.approvals, *ev.Approval)
		case ev.ApprovalDone != "":
			r.approvals = slices.DeleteFunc(r.approvals, func(a ApprovalRequest) bool { return a.ID == ev.ApprovalDone })
		case ev.PermissionMode != "":
			r.modes = append(r.modes, ev.PermissionMode)
		case ev.BackgroundTasks != nil:
			r.tasks = append(r.tasks, *ev.BackgroundTasks)
		}
	}
	return evs
}

// askSplitter records a mapper's get_context_usage requests apart from
// everything else it sends.
type askSplitter struct {
	*recordedReplies
	asks *[]map[string]any
}

func (s askSplitter) send(v any) {
	s.recordedReplies.send(v)
	last := s.recordedReplies.sent[len(s.recordedReplies.sent)-1]
	if id, _ := last["request_id"].(string); strings.HasPrefix(id, claudeContextAsk) {
		s.recordedReplies.sent = s.recordedReplies.sent[:len(s.recordedReplies.sent)-1]
		*s.asks = append(*s.asks, last)
	}
}

func (r *leadReplay) all(lines []leadLine) {
	r.t.Helper()
	for _, l := range lines {
		r.line(l)
	}
}

// ringTexts lists the texts of the ring's entries of kind, oldest first.
func ringTexts(ring []wslive.RingEntry, kind string) []string {
	var out []string
	for _, e := range ring {
		if e.Kind == kind {
			out = append(out, e.Text)
		}
	}
	return out
}

func countEvents(evs []Event, match func(Event) bool) int {
	n := 0
	for _, ev := range evs {
		if match(ev) {
			n++
		}
	}
	return n
}

func isTurnEnd(ev Event) bool { return ev.TurnEnd }

// sameJSON reports whether a encodes the same JSON value as the line b.
func sameJSON(t *testing.T, a any, b []byte) bool {
	t.Helper()
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var x, y any
	if err := json.Unmarshal(raw, &x); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(x, y)
}

func isCanUseTool(l leadLine) bool {
	return !l.in && l.field("type") == "control_request" && bytes.Contains(l.raw, []byte(`"can_use_tool"`))
}

func isAnswer(l leadLine) bool { return l.in && l.field("type") == "control_response" }

func isResult(l leadLine) bool { return !l.in && l.field("type") == "result" }

func isState(state string) func(leadLine) bool {
	return func(l leadLine) bool {
		return !l.in && l.field("subtype") == "session_state_changed" && l.field("state") == state
	}
}

// TestClaudeLeadApprovalAllow: a can_use_tool control request becomes one
// waiting prompt, an allow is the control_response the recording's driver
// sent (the tool call's input handed back as updatedInput), and the tool
// call's result settles the prompt. Recorded with Claude 2.1.322.
func TestClaudeLeadApprovalAllow(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-approval-allow.jsonl")
	r := newLeadReplay(t)
	ask := findLine(t, lines, 0, "can_use_tool", isCanUseTool)
	r.all(lines[:ask])
	if len(r.approvals) != 0 || !r.turn {
		t.Fatalf("before the request a turn runs with no prompt: turn %v, %+v", r.turn, r.approvals)
	}
	evs := r.line(lines[ask])
	if len(evs) != 1 || evs[0].Approval == nil {
		t.Fatalf("a can_use_tool request is one Approval event, got %+v", evs)
	}
	a := *evs[0].Approval
	if a.ID != "84855f98-7ecb-4ead-97c4-9c43864c898a" || a.Tool != "Bash" || a.ToolUseID != "toolu_01WUaebN1z2LLW13S1FFrkUu" || a.AgentID != "" || a.StartedAt == 0 ||
		a.Description == "" || !sameJSON(t, a.Input, []byte(`{"command":"echo hi > probe-a.txt","description":"Write \"hi\" to probe-a.txt in the working directory"}`)) {
		t.Fatalf("approval = %+v (input %s)", a, a.Input)
	}
	if len(r.replies.sent) != 0 {
		t.Fatalf("a permission prompt waits for the user; the mapper sent %v", r.replies.sent)
	}
	answer := findLine(t, lines, ask, "sent control_response", isAnswer)
	if !sameJSON(t, claudeApprovalResponse(a, true, ""), lines[answer].raw) {
		raw, _ := json.Marshal(claudeApprovalResponse(a, true, ""))
		t.Fatalf("allow line =\n%s\nwant the recorded\n%s", raw, lines[answer].raw)
	}
	// The prompt stays listed until the tool call's result arrives.
	result := findLine(t, lines, answer, "tool result", func(l leadLine) bool { return !l.in && l.field("type") == "user" })
	r.all(lines[answer:result])
	if len(r.approvals) != 1 {
		t.Fatalf("the prompt waits until its tool call returns: %+v", r.approvals)
	}
	evs = r.line(lines[result])
	if len(evs) == 0 || evs[0].ApprovalDone != a.ID {
		t.Fatalf("the tool result settles the prompt first, got %+v", evs)
	}
	r.all(lines[result+1:])
	if len(r.approvals) != 0 || r.turn {
		t.Fatalf("after the fixture no prompt waits and no turn runs: %+v, turn %v", r.approvals, r.turn)
	}
	if !slices.Equal(r.opened, []string{"send", "send"}) || r.ends != 2 {
		t.Fatalf("turns opened by %v, ended %d; want two sent turns", r.opened, r.ends)
	}
	if got := ringTexts(r.ring, wslive.RingReply); len(got) != 2 || !strings.HasPrefix(got[0], "DONE") || !strings.HasPrefix(got[1], "DONE") {
		t.Fatalf("ring replies = %q", got)
	}
	if len(r.sessions) == 0 || r.sessions[0] != "07125467-ab3c-4594-95ab-5358610cf91d" || !slices.Contains(r.modes, "default") {
		t.Fatalf("init should report the session %v and the permission mode %v", r.sessions, r.modes)
	}
}

// TestClaudeLeadApprovalDenyAndSessionState runs a recording made with
// session-state lines on (Claude 2.1.322): a running state opens the turn
// before any other line, the requires_action state beside a can_use_tool
// request adds no second prompt, a deny is the control_response the driver
// sent, the running state after the answer opens no second turn, and the
// turn ends at its idle state, not at its result.
func TestClaudeLeadApprovalDenyAndSessionState(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-approval-deny.jsonl")
	r := newLeadReplay(t)
	// The first output line is the running state: it alone opens a turn.
	first := findLine(t, lines, 0, "output", func(l leadLine) bool { return !l.in })
	if evs := r.m.mapLine(lines[first].raw, r.replies); len(evs) != 2 || evs[0].HarnessState != HarnessRunning || !evs[1].TurnStart {
		t.Fatalf("a running state is the state and a TurnStart, got %+v", evs)
	}
	r = newLeadReplay(t)

	ask := findLine(t, lines, 0, "can_use_tool", isCanUseTool)
	state := findLine(t, lines, 0, "requires_action", isState(HarnessWaiting))
	if state > ask {
		t.Fatalf("the fixture should hold requires_action (line %d) before can_use_tool (line %d)", state, ask)
	}
	r.all(lines[:ask+1])
	if len(r.approvals) != 1 || countEvents(r.events, func(ev Event) bool { return ev.Approval != nil }) != 1 {
		t.Fatalf("requires_action beside can_use_tool leaves one prompt: %+v", r.approvals)
	}
	a := r.approvals[0]
	answer := findLine(t, lines, ask, "sent control_response", isAnswer)
	if a.ID != "dedd1842-7da7-40fc-91f4-a30bf7f23279" || !sameJSON(t, claudeApprovalResponse(a, false, "probe"), lines[answer].raw) {
		raw, _ := json.Marshal(claudeApprovalResponse(a, false, "probe"))
		t.Fatalf("deny line =\n%s\nwant the recorded\n%s", raw, lines[answer].raw)
	}

	// requires_action, can_use_tool, deny, running: still the one turn.
	again := findLine(t, lines, answer, "running after the answer", isState(HarnessRunning))
	r.all(lines[ask+1 : again])
	if evs := r.line(lines[again]); len(evs) != 2 || !evs[1].TurnStart {
		t.Fatalf("the running state after an answer = %+v", evs)
	}
	if !slices.Equal(r.opened, []string{"send"}) || !r.turn || r.ends != 0 {
		t.Fatalf("a running state after requires_action opens no second turn: opened %v, ended %d", r.opened, r.ends)
	}

	// The result settles the turn's batch and usage, and the idle state
	// that follows it ends the turn.
	result := findLine(t, lines, again, "result", isResult)
	idle := findLine(t, lines, result, "idle", isState(HarnessIdle))
	r.all(lines[again+1 : result])
	if evs := r.line(lines[result]); countEvents(evs, isTurnEnd) != 0 || !r.turn {
		t.Fatalf("with session states on a result ends no turn: %+v", evs)
	}
	r.all(lines[result+1 : idle])
	if evs := r.line(lines[idle]); len(evs) == 0 || !evs[len(evs)-1].TurnEnd || evs[len(evs)-1].TurnError != "" || r.turn {
		t.Fatalf("the idle state ends the turn: %+v", evs)
	}

	r.all(lines[idle+1:])
	if len(r.approvals) != 0 || r.turn {
		t.Fatalf("after the fixture no prompt waits and no turn runs: %+v, turn %v", r.approvals, r.turn)
	}
	if !slices.Equal(r.opened, []string{"send", "send"}) || r.ends != 2 || countEvents(r.events, isTurnEnd) != 2 {
		t.Fatalf("turns opened by %v, ended %d (%d TurnEnd events); want two sent turns, each ended once", r.opened, r.ends, countEvents(r.events, isTurnEnd))
	}
	if want := []string{HarnessRunning, HarnessWaiting, HarnessRunning, HarnessIdle, HarnessRunning, HarnessIdle}; !slices.Equal(r.states, want) {
		t.Fatalf("harness states = %v, want %v", r.states, want)
	}
	if got := ringTexts(r.ring, wslive.RingReply); len(got) != 3 || got[2] != "ok" {
		t.Fatalf("ring replies = %q", got)
	}
	if len(r.replies.sent) != 0 || len(r.turnErrs) != 0 {
		t.Fatalf("the mapper sent %v, turn errors %v", r.replies.sent, r.turnErrs)
	}
	// The recording answered no get_context_usage, so each turn's init
	// asked for the compaction window again.
	if inits := countLines(lines, func(l leadLine) bool { return !l.in && l.field("subtype") == "init" }); inits == 0 || len(r.asks) != inits {
		t.Fatalf("context asks = %v, want one per init (%d)", r.asks, inits)
	}
}

// countLines counts the fixture lines match holds for.
func countLines(lines []leadLine, match func(leadLine) bool) int {
	n := 0
	for _, l := range lines {
		if match(l) {
			n++
		}
	}
	return n
}

// TestClaudeLeadHeldSendFlushesAtIdle: a message sent during a turn is
// submitted when the idle state ends that turn, not at the result before
// it, so the idle state cannot end the turn the submitted message began.
// The recording's second message was typed after the idle state; here it
// is held from the middle of the first turn, as the runner holds one.
func TestClaudeLeadHeldSendFlushesAtIdle(t *testing.T) {
	var lines []leadLine
	sends := 0
	for _, l := range readLeadFixture(t, "claude-lead-approval-deny.jsonl") {
		if l.in && l.field("type") == "user" {
			if sends++; sends == 2 {
				continue
			}
		}
		lines = append(lines, l)
	}
	result := findLine(t, lines, 0, "result", isResult)
	idle := findLine(t, lines, result, "idle", isState(HarnessIdle))
	lastIdle := findLine(t, lines, idle+1, "second idle", isState(HarnessIdle))

	r := newLeadReplay(t)
	r.all(lines[:result])
	r.held = 1
	r.line(lines[result])
	if r.held != 1 || !r.turn || r.ends != 0 {
		t.Fatalf("the result must not submit the held message: held %d, turn %v, ended %d", r.held, r.turn, r.ends)
	}
	r.all(lines[result+1 : idle+1])
	if r.held != 0 || !r.turn || r.ends != 1 || !slices.Equal(r.opened, []string{"send", "flush"}) {
		t.Fatalf("the idle state ends the turn and submits the held message: held %d, turn %v, ended %d, opened %v", r.held, r.turn, r.ends, r.opened)
	}
	// Nothing of the first turn is left to end the second one.
	r.all(lines[idle+1 : lastIdle])
	if !r.turn || r.ends != 1 {
		t.Fatalf("the submitted message's turn stays open until its own idle state: turn %v, ended %d", r.turn, r.ends)
	}
	r.line(lines[lastIdle])
	if r.turn || r.ends != 2 || !slices.Equal(r.opened, []string{"send", "flush"}) {
		t.Fatalf("after the second idle state: turn %v, ended %d, opened %v", r.turn, r.ends, r.opened)
	}
}

// TestClaudeLeadSessionStateOpensAndCloses: a running state opens a turn no
// send began, an idle state closes one with no result (settling a prompt
// left open), and a result's error reaches the turn's end at the idle.
func TestClaudeLeadSessionStateOpensAndCloses(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-session-state.jsonl")
	running := lines[findLine(t, lines, 0, "running", isState(HarnessRunning))]
	idle := lines[findLine(t, lines, 0, "idle", isState(HarnessIdle))]

	// The recording: running, the turn, its result, idle. No send line is
	// in it, so the running state is what opens the turn.
	r := newLeadReplay(t)
	r.all(lines)
	if !slices.Equal(r.opened, []string{"session_state_changed"}) || r.ends != 1 || r.turn || countEvents(r.events, isTurnEnd) != 1 {
		t.Fatalf("opened by %v, ended %d (TurnEnd events %d)", r.opened, r.ends, countEvents(r.events, isTurnEnd))
	}
	if !slices.Equal(r.states, []string{HarnessRunning, HarnessIdle}) {
		t.Fatalf("harness states = %v", r.states)
	}

	// A turn the harness began and ended with no result line.
	r = newLeadReplay(t)
	r.line(running)
	if !r.turn {
		t.Fatal("a running state opens a turn")
	}
	deny := readLeadFixture(t, "claude-lead-approval-deny.jsonl")
	r.line(deny[findLine(t, deny, 0, "can_use_tool", isCanUseTool)])
	if len(r.approvals) != 1 {
		t.Fatalf("approvals = %+v", r.approvals)
	}
	evs := r.line(idle)
	if r.turn || r.ends != 1 || len(r.approvals) != 0 || len(evs) == 0 || !evs[len(evs)-1].TurnEnd || evs[len(evs)-1].TurnError != "" {
		t.Fatalf("an idle state with no result closes the turn and its prompt: turn %v, ends %d, %+v, events %+v", r.turn, r.ends, r.approvals, evs)
	}

	// A failed turn: the result's error is the turn's, told at the idle
	// state, once. An interrupt is no failure.
	r = newLeadReplay(t)
	r.line(running)
	failed := leadLine{raw: []byte(`{"type":"result","subtype":"error_during_execution","is_error":true,"result":"boom"}`)}
	if evs := r.line(failed); countEvents(evs, isTurnEnd) != 0 {
		t.Fatalf("a failed result ends no turn while states arrive: %+v", evs)
	}
	if evs := r.line(idle); !evs[len(evs)-1].TurnEnd || evs[len(evs)-1].TurnError != "boom" {
		t.Fatalf("the idle state carries the result's error: %+v", evs)
	}
	r.line(running)
	r.line(leadLine{raw: []byte(`{"type":"result","subtype":"error_during_execution","is_error":true,"terminal_reason":"aborted_streaming"}`)})
	if evs := r.line(idle); !evs[len(evs)-1].TurnEnd || evs[len(evs)-1].TurnError != "" {
		t.Fatalf("an earlier error must not reach a later turn, and an interrupt is none: %+v", evs)
	}
	if !slices.Equal(r.turnErrs, []string{"boom"}) || r.ends != 2 {
		t.Fatalf("turn errors %v, ended %d", r.turnErrs, r.ends)
	}

	// Not a lead: the state lines are not read.
	m := newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`))
	if evs := m.mapLine(running.raw, nopReplies{}); len(evs) != 0 {
		t.Fatalf("a profile conversation ignores session states, got %+v", evs)
	}
}

// TestClaudeLeadLocalCommandsWithSessionStates: with session states on, a
// slash command the harness answers itself (a num_turns 0 result) is a
// turn like any other, from its running state to its idle state. Recorded
// with Claude 2.1.322.
func TestClaudeLeadLocalCommandsWithSessionStates(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-local-commands.jsonl")
	r := newLeadReplay(t)
	for _, l := range lines {
		evs := r.line(l)
		if isResult(l) && (!bytes.Contains(l.raw, []byte(`"num_turns":0`)) || countEvents(evs, isTurnEnd) != 0 || !r.turn) {
			t.Fatalf("a local command's result should be num_turns 0 and end no turn here: %+v", evs)
		}
	}
	if !slices.Equal(r.opened, []string{"send", "send"}) || r.ends != 2 || r.turn {
		t.Fatalf("turns opened by %v, ended %d", r.opened, r.ends)
	}
	if got := ringTexts(r.ring, wslive.RingReply); len(got) != 2 || !strings.HasPrefix(got[0], "## Context Usage") {
		t.Fatalf("ring replies = %q", got)
	}
}

// TestClaudeLeadResultEndsTheTurnWithoutSessionStates: a harness that
// emits no session states (the three recordings below have none) ends a
// turn at its result: a sent turn's, a slash command's num_turns 0 one,
// and the unsolicited one of a compaction or a background task's turn.
func TestClaudeLeadResultEndsTheTurnWithoutSessionStates(t *testing.T) {
	for _, name := range []string{"claude-lead-approval-allow.jsonl", "claude-lead-compaction.jsonl", "claude-lead-background-task.jsonl"} {
		r := newLeadReplay(t)
		results := 0
		for _, l := range readLeadFixture(t, name) {
			if !l.in && l.field("subtype") == "session_state_changed" {
				t.Fatalf("%s should hold no session state line", name)
			}
			evs := r.line(l)
			if isResult(l) {
				results++
				if len(evs) == 0 || !evs[len(evs)-1].TurnEnd || r.turn {
					t.Fatalf("%s: result %d should end the turn: %+v", name, results, evs)
				}
			}
		}
		if r.ends != results || len(r.states) != 0 {
			t.Fatalf("%s: %d turns ended for %d results (states %v)", name, r.ends, results, r.states)
		}
	}
}

// TestClaudeLeadSteeredMessageJoinsTheTurn: a user message written while a
// turn runs is taken up inside that turn (recorded on 2.1.322 with user
// messages echoed: the second message's echo follows the running tool's
// result). Its queue lines, its echo, and the one result for both
// messages open no second turn, and the turn ends once, at the idle state;
// on a harness that reports no states, at the one result.
func TestClaudeLeadSteeredMessageJoinsTheTurn(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-steer.jsonl")
	sends := 0
	for _, l := range lines {
		if l.in && l.field("type") == "user" {
			sends++
		}
	}
	idle := findLine(t, lines, 0, "idle", isState(HarnessIdle))
	if sends != 2 || idle != len(lines)-1 {
		t.Fatalf("the recording should hold two messages and end idle: %d sends, idle at %d of %d", sends, idle, len(lines))
	}
	r := newLeadReplay(t)
	r.all(lines[:idle])
	if !r.turn || r.ends != 0 || !slices.Equal(r.opened, []string{"send"}) {
		t.Fatalf("one turn runs up to the idle state: turn %v, ended %d, opened %v", r.turn, r.ends, r.opened)
	}
	r.line(lines[idle])
	if r.turn || r.ends != 1 || len(r.turnErrs) != 0 || !slices.Equal(r.opened, []string{"send"}) {
		t.Fatalf("the idle state ends the one turn: turn %v, ended %d, errors %v, opened %v", r.turn, r.ends, r.turnErrs, r.opened)
	}
	if got := ringTexts(r.ring, wslive.RingReply); !slices.Equal(got, []string{"PINEAPPLE"}) {
		t.Fatalf("ring replies = %q", got)
	}
	// The echo of a message is no entry: the runner wrote one at the send.
	if got := ringTexts(r.ring, wslive.RingUser); len(got) != 0 {
		t.Fatalf("the mapper writes no user entry: %q", got)
	}

	plain := newLeadReplay(t)
	for _, l := range lines {
		if !l.in && l.field("subtype") == "session_state_changed" {
			continue
		}
		evs := plain.line(l)
		if isResult(l) && (len(evs) == 0 || !evs[len(evs)-1].TurnEnd) {
			t.Fatalf("with no state lines the result ends the turn: %+v", evs)
		}
	}
	if plain.turn || plain.ends != 1 || !slices.Equal(plain.opened, []string{"send"}) {
		t.Fatalf("with no state lines: turn %v, ended %d, opened %v", plain.turn, plain.ends, plain.opened)
	}
}

// TestClaudeLeadActionWithNoPrompt: the harness says it waits on the user
// (requires_action) and what arrives is no can_use_tool request. The mapper
// reports the state, so the runner can show a wait it has no prompt for,
// and refuses a control request it has no handler for with an error
// answer. The fixture is the denied-prompt recording's first turn with its
// can_use_tool request replaced by an MCP elicitation request, the answer
// by the error this mapper sends, and the tool's lines left out.
func TestClaudeLeadActionWithNoPrompt(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-action-mismatch.jsonl")
	r := newLeadReplay(t)
	request := findLine(t, lines, 0, "control request", func(l leadLine) bool { return !l.in && l.field("type") == "control_request" })
	r.all(lines[:request])
	if len(r.states) == 0 || r.states[len(r.states)-1] != HarnessWaiting {
		t.Fatalf("the harness should say it waits before the request: %v", r.states)
	}
	evs := r.line(lines[request])
	if len(evs) != 1 || evs[0].Notice == "" || !strings.Contains(evs[0].Notice, "elicitation") {
		t.Fatalf("an unhandled control request is a note for the log, got %+v", evs)
	}
	answer := findLine(t, lines, request, "sent control_response", isAnswer)
	if len(r.replies.sent) != 1 || !sameJSON(t, r.replies.sent[0], lines[answer].raw) {
		t.Fatalf("the request should be refused with\n%s\ngot %v", lines[answer].raw, r.replies.sent)
	}
	r.all(lines[request+1:])
	if len(r.approvals) != 0 || countEvents(r.events, func(ev Event) bool { return ev.Approval != nil }) != 0 {
		t.Fatalf("no prompt may wait: %+v", r.approvals)
	}
	if want := []string{HarnessRunning, HarnessWaiting, HarnessRunning, HarnessIdle}; !slices.Equal(r.states, want) {
		t.Fatalf("harness states = %v, want %v", r.states, want)
	}
	if !slices.Equal(r.opened, []string{"send"}) || r.ends != 1 || r.turn {
		t.Fatalf("turns opened by %v, ended %d", r.opened, r.ends)
	}

	// A wait with no request at all (a prompt the process inherited): the
	// state alone.
	m := newClaudeMapper(leadSpec())
	waiting := lines[findLine(t, lines, 0, "requires_action", isState(HarnessWaiting))]
	if evs := m.mapLine(waiting.raw, nopReplies{}); len(evs) != 1 || evs[0].HarnessState != HarnessWaiting {
		t.Fatalf("a requires_action state = %+v", evs)
	}
	// An unreadable request is refused too; one with no id cannot be.
	replies := &recordedReplies{}
	if evs := m.mapLine([]byte(`{"type":"control_request","request_id":"r-9","request":"?"}`), replies); len(evs) != 1 || evs[0].Notice == "" || len(replies.sent) != 1 {
		t.Fatalf("an unreadable control request: events %+v, sent %v", evs, replies.sent)
	}
	if evs := m.mapLine([]byte(`{"type":"control_request","request":{"subtype":"elicitation"}}`), replies); len(evs) != 0 || len(replies.sent) != 1 {
		t.Fatalf("a control request with no id: events %+v, sent %v", evs, replies.sent)
	}
}

// TestClaudeLeadCompaction: a slash command's num_turns 0 result ends its
// turn, a typed /compact is a turn whose re-emitted assistant line adds no
// second reply, and a compaction the harness starts by itself is a turn
// with no send, opened by its compacting status and closed by an
// unsolicited result.
func TestClaudeLeadCompaction(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-compaction.jsonl")
	r := newLeadReplay(t)
	r.all(lines)
	// Turns: the first reply, /context, /compact, the request_compaction
	// call, the compaction that call asked for, and a last question.
	want := []string{"send", "send", "send", "send", "status", "send"}
	if !slices.Equal(r.opened, want) || r.ends != len(want) || r.turn {
		t.Fatalf("turns opened by %v, ended %d (running %v); want %v", r.opened, r.ends, r.turn, want)
	}
	zero := 0
	for _, l := range lines {
		if !l.in && l.field("type") == "result" && bytes.Contains(l.raw, []byte(`"num_turns":0`)) {
			zero++
		}
	}
	if zero != 3 || countEvents(r.events, isTurnEnd) != len(want) {
		t.Fatalf("the fixture should hold three num_turns 0 results (has %d), each a TurnEnd (%d in all)", zero, countEvents(r.events, isTurnEnd))
	}
	replies := ringTexts(r.ring, wslive.RingReply)
	if len(replies) != 4 || replies[0] != "OK" || !strings.HasPrefix(replies[1], "## Context Usage") || replies[2] != "DONE" || replies[3] != "PLUM" {
		t.Fatalf("ring replies = %q; the compaction's re-emitted message must not repeat", replies)
	}
	// The fixture does hold the /context reply twice, under one line uuid.
	seen := map[string]int{}
	for _, l := range lines {
		if !l.in && l.field("type") == "assistant" {
			seen[l.field("uuid")]++
		}
	}
	if seen["f0ac3f01-9dfd-45fa-be27-aec69fb587a1"] != 2 {
		t.Fatalf("the fixture should re-emit the /context reply: %v", seen)
	}
	var compacts []wslive.RingEntry
	for _, e := range r.ring {
		if e.Kind == wslive.RingCompact {
			compacts = append(compacts, e)
		}
	}
	if len(compacts) != 2 {
		t.Fatalf("two compactions should reach the ring, got %+v", compacts)
	}
	if len(r.turnErrs) != 0 || len(r.replies.sent) != 0 {
		t.Fatalf("turn errors %v, sent %v", r.turnErrs, r.replies.sent)
	}
}

// leverTurnEnds lists, by TurnEnd, whether each ended a turn that requested
// a compaction (Event.CompactRequested).
func leverTurnEnds(evs []Event) []bool {
	var out []bool
	for _, ev := range evs {
		if ev.TurnEnd {
			out = append(out, ev.CompactRequested)
		}
	}
	return out
}

// leverFixture is the compaction fixture with edit applied to the
// request_compaction call's tool_result line (lines 29-30 pair it by
// tool_use_id) and to the result line that ends that call's turn.
func leverFixture(t *testing.T, editResult, editTurnResult func(map[string]any)) []leadLine {
	t.Helper()
	lines := readLeadFixture(t, "claude-lead-compaction.jsonl")
	const id = "toolu_01P23Tfe64xau7hs1gsv1hu5"
	call := findLine(t, lines, 0, "request_compaction tool_use", func(l leadLine) bool {
		return !l.in && l.field("type") == "assistant" && bytes.Contains(l.raw, []byte(`"name":"mcp__ws__request_compaction"`))
	})
	res := findLine(t, lines, call, "request_compaction tool_result", func(l leadLine) bool {
		return !l.in && l.field("type") == "user" && bytes.Contains(l.raw, []byte(`"tool_use_id":"`+id+`"`))
	})
	end := findLine(t, lines, res, "lever turn result", func(l leadLine) bool { return !l.in && l.field("type") == "result" })
	rewrite := func(i int, edit func(map[string]any)) {
		if edit == nil {
			return
		}
		var m map[string]any
		if err := json.Unmarshal(lines[i].raw, &m); err != nil {
			t.Fatal(err)
		}
		edit(m)
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		lines[i].raw = raw
	}
	rewrite(res, editResult)
	rewrite(end, editTurnResult)
	return lines
}

// TestClaudeLeadCompactionLeverDetected: the turn carrying a successful
// request_compaction call (the fixture's fourth turn) ends with
// CompactRequested, exactly once; every other turn, the compaction's own
// among them, ends without it.
func TestClaudeLeadCompactionLeverDetected(t *testing.T) {
	r := newLeadReplay(t)
	r.all(leverFixture(t, nil, nil))
	got := leverTurnEnds(r.events)
	want := []bool{false, false, false, true, false, false}
	if !slices.Equal(got, want) {
		t.Fatalf("CompactRequested by turn end = %v; want %v", got, want)
	}
}

// TestClaudeLeadCompactionLeverNotDetected: an error tool_result, or an
// interrupted lever turn (an aborted_* terminal reason, which ends the turn
// with no error), requests nothing.
func TestClaudeLeadCompactionLeverNotDetected(t *testing.T) {
	cases := map[string][2]func(map[string]any){
		"error result": {func(m map[string]any) {
			blocks := m["message"].(map[string]any)["content"].([]any)
			blocks[0].(map[string]any)["is_error"] = true
		}, nil},
		"interrupted turn": {nil, func(m map[string]any) {
			m["subtype"], m["is_error"], m["terminal_reason"] = "error_during_execution", true, "aborted_tools"
		}},
	}
	for name, edits := range cases {
		t.Run(name, func(t *testing.T) {
			r := newLeadReplay(t)
			r.all(leverFixture(t, edits[0], edits[1]))
			if got := leverTurnEnds(r.events); slices.Contains(got, true) {
				t.Fatalf("CompactRequested by turn end = %v; want none", got)
			}
			if len(r.turnErrs) != 0 {
				t.Fatalf("turn errors %v", r.turnErrs)
			}
		})
	}
}

// TestClaudeLeadCompactionLeverAtIdle: on a harness that ends a turn at its
// idle state, the lever turn's result hands the request to that idle's
// TurnEnd; an interrupted result does not.
func TestClaudeLeadCompactionLeverAtIdle(t *testing.T) {
	line := func(v map[string]any) []byte {
		raw, _ := json.Marshal(v)
		return raw
	}
	for _, aborted := range []bool{false, true} {
		m := newClaudeMapper(leadSpec())
		var evs []Event
		feed := func(v map[string]any) { evs = append(evs, m.mapLine(line(v), nopReplies{})...) }
		feed(map[string]any{"type": "system", "subtype": "session_state_changed", "state": HarnessRunning})
		feed(map[string]any{"type": "assistant", "message": map[string]any{"id": "m1", "content": []map[string]any{{"type": "tool_use", "id": "lv", "name": "mcp__plugin_ws_ws__request_compaction", "input": map[string]any{}}}}})
		feed(map[string]any{"type": "user", "message": map[string]any{"content": []map[string]any{{"type": "tool_result", "tool_use_id": "lv", "content": "Compaction requested."}}}})
		result := map[string]any{"type": "result", "subtype": "success", "terminal_reason": "completed"}
		if aborted {
			result = map[string]any{"type": "result", "subtype": "error_during_execution", "is_error": true, "terminal_reason": "aborted_streaming"}
		}
		feed(result)
		if n := countEvents(evs, isTurnEnd); n != 0 {
			t.Fatalf("aborted=%v: the result ended the turn (%d TurnEnds); the idle should", aborted, n)
		}
		feed(map[string]any{"type": "system", "subtype": "session_state_changed", "state": HarnessIdle})
		if got := leverTurnEnds(evs); !slices.Equal(got, []bool{!aborted}) {
			t.Fatalf("aborted=%v: CompactRequested by turn end = %v", aborted, got)
		}
		// The next turn, with no call, requests nothing.
		feed(map[string]any{"type": "system", "subtype": "session_state_changed", "state": HarnessRunning})
		feed(map[string]any{"type": "result", "subtype": "success", "terminal_reason": "completed"})
		feed(map[string]any{"type": "system", "subtype": "session_state_changed", "state": HarnessIdle})
		if got := leverTurnEnds(evs); !slices.Equal(got, []bool{!aborted, false}) {
			t.Fatalf("aborted=%v: CompactRequested by turn end = %v", aborted, got)
		}
	}
}

// TestClaudeLeadKeepsARealMessageAfterCompaction: only a line uuid seen
// before the boundary is dropped after it; the drop ends with the turn.
func TestClaudeLeadKeepsARealMessageAfterCompaction(t *testing.T) {
	assistant := func(uuid, text string) []byte {
		raw, _ := json.Marshal(map[string]any{"type": "assistant", "uuid": uuid, "message": map[string]any{"id": "msg-" + uuid, "content": []map[string]any{{"type": "text", "text": text}}}})
		return raw
	}
	replies := func(evs []Event) []string {
		var out []string
		for _, ev := range evs {
			if ev.Live != nil && ev.Live.Kind == wslive.KindReply {
				out = append(out, ev.Live.Text)
			}
		}
		return out
	}
	m := newClaudeMapper(leadSpec())
	if got := replies(m.mapLine(assistant("u1", "before"), nopReplies{})); !slices.Equal(got, []string{"before"}) {
		t.Fatalf("replies = %q", got)
	}
	// With no compaction a repeated uuid is still a message.
	if got := replies(m.mapLine(assistant("u1", "before"), nopReplies{})); len(got) != 1 {
		t.Fatalf("a repeated line outside a compaction is kept, got %q", got)
	}
	m.mapLine([]byte(`{"type":"system","subtype":"status","status":"compacting"}`), nopReplies{})
	m.mapLine([]byte(`{"type":"system","subtype":"compact_boundary","compact_metadata":{"trigger":"auto"}}`), nopReplies{})
	if got := m.mapLine(assistant("u1", "before"), nopReplies{}); len(got) != 0 {
		t.Fatalf("the re-emitted line is dropped whole, got %+v", got)
	}
	if got := replies(m.mapLine(assistant("u2", "after"), nopReplies{})); !slices.Equal(got, []string{"after"}) {
		t.Fatalf("a new message after the boundary is kept, got %q", got)
	}
	m.mapLine([]byte(`{"type":"result","subtype":"success","num_turns":0}`), nopReplies{})
	if got := replies(m.mapLine(assistant("u2", "after"), nopReplies{})); len(got) != 1 {
		t.Fatalf("the drop ends with the compaction's turn, got %q", got)
	}
}

// TestClaudeLeadBackgroundTasks: background_tasks_changed carries the whole
// current list, and the turn a finished task brings starts with no send.
func TestClaudeLeadBackgroundTasks(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-background-task.jsonl")
	r := newLeadReplay(t)
	r.all(lines)
	if !slices.Equal(r.tasks, []int{1, 0}) {
		t.Fatalf("background task counts = %v, want [1 0]", r.tasks)
	}
	if !slices.Equal(r.opened, []string{"send", "init"}) || r.ends != 2 || r.turn {
		t.Fatalf("turns opened by %v, ended %d; the task's completion starts a turn nobody sent", r.opened, r.ends)
	}
	if got := ringTexts(r.ring, wslive.RingReply); len(got) != 2 || got[0] != "STARTED" {
		t.Fatalf("ring replies = %q", got)
	}
	// A background shell command is no subagent.
	if agents := r.st.Snapshot(r.clock).Agents; len(agents) != 0 {
		t.Fatalf("a background bash task should add no subagent: %+v", agents)
	}
	// Not a lead: the count is not read.
	m := newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`))
	for _, l := range lines {
		if l.field("subtype") == "background_tasks_changed" {
			if evs := m.mapLine(l.raw, nopReplies{}); len(evs) != 0 {
				t.Fatalf("a profile conversation ignores the task list, got %+v", evs)
			}
		}
	}
}

// TestClaudeLeadResultEndsTheTurnWhileBackgroundTasksRun replays a
// recording with session states on (Claude 2.1.323): a turn that leaves a
// background Agent task running gets its result but no idle state, and a
// prompt sent while the task runs gets a result with no running or idle
// state around it; the one idle comes after the task's completion turn.
// Each of those results ends its turn, so the lead shows idle between
// them and every turn writes its own turn entry.
func TestClaudeLeadResultEndsTheTurnWhileBackgroundTasksRun(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-background-agent-states.jsonl")
	first := findLine(t, lines, 0, "result", isResult)
	second := findLine(t, lines, first+1, "second result", isResult)
	third := findLine(t, lines, second+1, "third result", isResult)
	idle := findLine(t, lines, 0, "idle", isState(HarnessIdle))
	if idle != third+1 || countLines(lines, isState(HarnessRunning)) != 1 {
		t.Fatalf("the recording should hold one running state and one idle state, right after the third result (idle at %d, third result at %d)", idle, third)
	}

	r := newLeadReplay(t)
	r.all(lines[:first+1])
	lead := r.st.Snapshot(r.clock).Lead
	if r.turn || r.ends != 1 || lead.TurnStartedAt != 0 || lead.Phase != wslive.PhaseIdle {
		t.Fatalf("a result while a background task runs ends the turn: turn %v, ended %d, lead %+v", r.turn, r.ends, lead)
	}
	r.all(lines[first+1 : second+1])
	if r.turn || r.ends != 2 || !slices.Equal(r.opened, []string{"send", "send"}) {
		t.Fatalf("the second message's result ends its turn: turn %v, ended %d, opened %v", r.turn, r.ends, r.opened)
	}
	r.all(lines[second+1 : idle])
	if !r.turn || r.ends != 2 || !slices.Equal(r.opened, []string{"send", "send", "init"}) {
		t.Fatalf("the completion turn's result, with no task left, defers to the idle state: turn %v, ended %d, opened %v", r.turn, r.ends, r.opened)
	}
	evs := r.line(lines[idle])
	if r.turn || r.ends != 3 || countEvents(r.events, isTurnEnd) != 3 || len(evs) == 0 || !evs[len(evs)-1].TurnEnd {
		t.Fatalf("the idle state ends the completion turn, whose result came with no task left: ended %d, TurnEnd events %d, %+v", r.ends, countEvents(r.events, isTurnEnd), evs)
	}
	if got := ringKinds(r.ring, wslive.RingTurn); got != 3 {
		t.Fatalf("one turn entry per turn, got %d", got)
	}
	if got := ringTexts(r.ring, wslive.RingReply); len(got) != 3 || got[0] != "STARTED" || got[1] != "ok2" {
		t.Fatalf("ring replies = %q", got)
	}
	if !slices.Equal(r.tasks, []int{1, 0}) || !slices.Equal(r.states, []string{HarnessRunning, HarnessIdle}) {
		t.Fatalf("background task counts %v, harness states %v", r.tasks, r.states)
	}

	// The second message held from the first turn instead: the first
	// result submits it, and its turn ends at its own result.
	var held []leadLine
	sends := 0
	for _, l := range lines {
		if l.in && l.field("type") == "user" {
			if sends++; sends == 2 {
				continue
			}
		}
		held = append(held, l)
	}
	r = newLeadReplay(t)
	r.all(held[:first])
	r.held = 1
	r.line(held[first])
	if r.held != 0 || !r.turn || r.ends != 1 || !slices.Equal(r.opened, []string{"send", "flush"}) {
		t.Fatalf("the result submits the held message: held %d, turn %v, ended %d, opened %v", r.held, r.turn, r.ends, r.opened)
	}
	r.all(held[first+1:])
	if r.turn || r.ends != 3 || !slices.Equal(r.opened, []string{"send", "flush", "init"}) {
		t.Fatalf("turn %v, ended %d, opened %v", r.turn, r.ends, r.opened)
	}
}

// TestClaudeLeadIdleAfterAnEndedTurnEndsNothing: an idle state that
// follows a result which ended the turn (a background task ran) belongs to
// that turn: it ends nothing, not the turn a held message began at the
// result. A failed result carries its error itself, and output after the
// result with no running state still opens a turn.
func TestClaudeLeadIdleAfterAnEndedTurnEndsNothing(t *testing.T) {
	line := func(s string) leadLine { return leadLine{raw: []byte(s)} }
	running, idle, result := line(stateLine(HarnessRunning)), line(stateLine(HarnessIdle)), line(resultLine())

	r := newLeadReplay(t)
	r.line(leadLine{in: true, raw: []byte(`{"type":"user","message":{"role":"user","content":"one"}}`)})
	r.all([]leadLine{running, line(tasksLine(1))})
	r.held = 1
	r.line(result)
	if !r.turn || r.ends != 1 || !slices.Equal(r.opened, []string{"send", "flush"}) {
		t.Fatalf("the result ends the turn and submits the held message: turn %v, ended %d, opened %v", r.turn, r.ends, r.opened)
	}
	if evs := r.line(idle); len(evs) != 1 || evs[0].HarnessState != HarnessIdle || !r.turn || r.ends != 1 {
		t.Fatalf("the idle after the ended turn ends nothing: %+v, turn %v, ended %d", evs, r.turn, r.ends)
	}
	r.all([]leadLine{running, line(replyLine("u2", "two"))})
	if evs := r.line(line(`{"type":"result","subtype":"error_during_execution","is_error":true,"result":"boom"}`)); len(evs) == 0 || !evs[len(evs)-1].TurnEnd || evs[len(evs)-1].TurnError != "boom" || r.turn {
		t.Fatalf("a failed result while the task runs ends the turn with its error: %+v", evs)
	}
	r.line(line(replyLine("u3", "more")))
	if !r.turn || !slices.Equal(r.opened, []string{"send", "flush", "assistant"}) {
		t.Fatalf("output after the result opens a turn: turn %v, opened %v", r.turn, r.opened)
	}
	r.line(result)
	r.line(idle)
	if r.turn || r.ends != 3 || !slices.Equal(r.turnErrs, []string{"boom"}) {
		t.Fatalf("turn %v, ended %d, errors %v", r.turn, r.ends, r.turnErrs)
	}

	// With the task list empty again, the result defers to the idle state.
	r.all([]leadLine{line(tasksLine(0)), running, line(replyLine("u4", "four"))})
	if evs := r.line(result); countEvents(evs, isTurnEnd) != 0 || !r.turn {
		t.Fatalf("with no task running the result ends no turn: %+v", evs)
	}
	if evs := r.line(idle); len(evs) == 0 || !evs[len(evs)-1].TurnEnd || r.turn || r.ends != 4 {
		t.Fatalf("the idle state ends it: %+v, ended %d", evs, r.ends)
	}
}

// TestClaudeLeadSettingsStream: the harness's answers to apply_flag_settings
// and set_permission_mode release their requests, the status line after a
// mode change reports the mode, and neither opens a turn.
func TestClaudeLeadSettingsStream(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-settings.jsonl")
	r := newLeadReplay(t)
	r.all(lines)
	if !slices.Equal(r.replies.answers, []string{"probe-0", "probe-1", "probe-2"}) {
		t.Fatalf("answered requests = %v", r.replies.answers)
	}
	if !slices.Equal(r.modes, []string{"auto", "acceptEdits", "acceptEdits"}) {
		t.Fatalf("reported modes = %v, want the init's, the status line's, and the next init's", r.modes)
	}
	if !slices.Equal(r.opened, []string{"send", "send"}) || r.ends != 2 || r.turn {
		t.Fatalf("turns opened by %v, ended %d; a settings answer opens none", r.opened, r.ends)
	}
	status := lines[findLine(t, lines, 0, "status", func(l leadLine) bool { return l.field("subtype") == "status" })]
	evs := newClaudeMapper(leadSpec()).mapLine(status.raw, nopReplies{})
	if len(evs) != 1 || evs[0].PermissionMode != "acceptEdits" {
		t.Fatalf("a status line with a permission mode is that mode alone, got %+v", evs)
	}
}

// canUseTool is the recorded can_use_tool line with its request changed by
// edit.
func canUseTool(t *testing.T, edit func(request map[string]any)) []byte {
	t.Helper()
	lines := readLeadFixture(t, "claude-lead-approval-allow.jsonl")
	var line map[string]any
	if err := json.Unmarshal(lines[findLine(t, lines, 0, "can_use_tool", isCanUseTool)].raw, &line); err != nil {
		t.Fatal(err)
	}
	edit(line["request"].(map[string]any))
	raw, err := json.Marshal(line)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestClaudeLeadDeniesWhatItCannotAsk: a question tool's prompt and a
// malformed request are answered at once with a deny and its message, and
// leave no prompt waiting.
func TestClaudeLeadDeniesWhatItCannotAsk(t *testing.T) {
	const id = "84855f98-7ecb-4ead-97c4-9c43864c898a"
	const malformed = claudeMalformedDenial
	cases := []struct {
		name    string
		edit    func(map[string]any)
		message string
	}{
		{"AskUserQuestion", func(r map[string]any) { r["tool_name"] = "AskUserQuestion" }, claudeChatOnlyDenial},
		{"ExitPlanMode", func(r map[string]any) { r["tool_name"] = "ExitPlanMode" }, claudePlanDenial},
		{"requiresUserInteraction", func(r map[string]any) { r["requiresUserInteraction"] = true }, claudeChatOnlyDenial},
		{"requires_user_interaction", func(r map[string]any) { r["requires_user_interaction"] = true }, claudeChatOnlyDenial},
		{"no tool name", func(r map[string]any) { delete(r, "tool_name") }, malformed},
		{"no tool call id", func(r map[string]any) { delete(r, "tool_use_id") }, malformed},
		{"no input", func(r map[string]any) { delete(r, "input") }, malformed},
		{"input not an object", func(r map[string]any) { r["input"] = "echo hi" }, malformed},
		{"null input", func(r map[string]any) { r["input"] = nil }, malformed},
	}
	for _, c := range cases {
		m := newClaudeMapper(leadSpec())
		replies := &recordedReplies{}
		if evs := m.mapLine(canUseTool(t, c.edit), replies); len(evs) != 0 {
			t.Errorf("%s: no prompt may wait, got %+v", c.name, evs)
		}
		want := map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": id, "response": map[string]any{"behavior": "deny", "message": c.message}}}
		if len(replies.sent) != 1 || !reflect.DeepEqual(replies.sent[0], want) {
			t.Errorf("%s: sent %v, want one deny %v", c.name, replies.sent, want)
		}
		if len(m.approvals) != 0 {
			t.Errorf("%s: the mapper should track no prompt: %v", c.name, m.approvals)
		}
	}
	if !strings.Contains(claudeChatOnlyDenial, "chat") || !strings.Contains(claudePlanDenial, "plan mode") || claudePlanDenial == claudeChatOnlyDenial {
		t.Fatalf("a question is to be asked in the chat, and a plan presented there with the way out of plan mode named:\n%q\n%q", claudeChatOnlyDenial, claudePlanDenial)
	}

	// A profile conversation has no permission prompts and answers none.
	replies := &recordedReplies{}
	m := newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`))
	if evs := m.mapLine(canUseTool(t, func(map[string]any) {}), replies); len(evs) != 0 || len(replies.sent) != 0 {
		t.Fatalf("a profile conversation has no permission prompts: events %+v, sent %v", evs, replies.sent)
	}
}

// TestClaudeLeadApprovalWithdrawn: a control_cancel_request settles its
// prompt with no answer written, and the prompt is settled once.
func TestClaudeLeadApprovalWithdrawn(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-approval-allow.jsonl")
	ask := findLine(t, lines, 0, "can_use_tool", isCanUseTool)
	result := findLine(t, lines, ask, "tool result", func(l leadLine) bool { return !l.in && l.field("type") == "user" })
	end := findLine(t, lines, result, "result", func(l leadLine) bool { return !l.in && l.field("type") == "result" })
	r := newLeadReplay(t)
	r.all(lines[:ask+1])
	id := r.approvals[0].ID
	cancel := leadLine{raw: []byte(`{"type":"control_cancel_request","request_id":"` + id + `"}`)}
	evs := r.line(cancel)
	if len(evs) != 1 || evs[0].ApprovalDone != id || len(r.approvals) != 0 || len(r.replies.sent) != 0 {
		t.Fatalf("a withdrawn prompt is one ApprovalDone and no answer: %+v, sent %v", evs, r.replies.sent)
	}
	before := countEvents(r.events, func(ev Event) bool { return ev.ApprovalDone != "" })
	r.all(lines[result : end+1])
	if got := countEvents(r.events, func(ev Event) bool { return ev.ApprovalDone != "" }); got != before {
		t.Fatalf("a withdrawn prompt is settled once; %d more ApprovalDone followed", got-before)
	}
	// A cancel of a prompt nobody waits on still names it, so the runner
	// drops whatever it holds under that id.
	if evs := r.line(cancel); len(evs) != 1 || evs[0].ApprovalDone != id || len(r.replies.sent) != 0 {
		t.Fatalf("a repeated cancel: %+v, sent %v", evs, r.replies.sent)
	}
}

// TestClaudeLeadResultSettlesOpenPrompts: a turn that ends with a prompt
// still open (an interrupt) settles it, since no answer can follow.
func TestClaudeLeadResultSettlesOpenPrompts(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-approval-allow.jsonl")
	ask := findLine(t, lines, 0, "can_use_tool", isCanUseTool)
	end := findLine(t, lines, ask, "result", func(l leadLine) bool { return !l.in && l.field("type") == "result" })
	r := newLeadReplay(t)
	r.all(lines[:ask+1])
	id := r.approvals[0].ID
	evs := r.line(lines[end])
	if len(r.approvals) != 0 || len(evs) == 0 || evs[0].ApprovalDone != id || !evs[len(evs)-1].TurnEnd {
		t.Fatalf("a result settles the open prompt before it ends the turn: %+v", evs)
	}
}

func TestClaudeLeadArgs(t *testing.T) {
	bundle := leadBundledFixture(t, "wsflow")
	spec := leadSpec()
	spec.Root = t.TempDir()
	spec.Env = []string{"CLAUDE_CONFIG_DIR=" + t.TempDir()}
	spec.Lead.PermissionMode = "plan"
	spec.Model, spec.Effort, spec.SessionID = "opus", "high", "sess-1"
	// What a profile conversation holds back must not reach a lead's
	// command line even when the spec carries it.
	spec.Instructions = "/sess/instructions.md"
	spec.WSMCP = &MCPCommand{Command: "/bin/ws-mcp"}
	args, err := claudeArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"--setting-sources", "--strict-mcp-config", "--mcp-config", "--tools", "--allowedTools", "--disable-slash-commands",
		"--append-system-prompt-file", "--bare", "--system-prompt", "--dangerously-skip-permissions", "--no-session-persistence"} {
		if slices.Contains(args, banned) {
			t.Fatalf("a lead's args must not carry %s: %v", banned, args)
		}
	}
	for _, want := range []string{"-p", "--verbose"} {
		if !slices.Contains(args, want) {
			t.Fatalf("args lack %s: %v", want, args)
		}
	}
	for flag, want := range map[string]string{
		"--input-format": "stream-json", "--output-format": "stream-json",
		"--permission-prompt-tool": "stdio", "--permission-mode": "plan",
		"--model": "opus", "--effort": "high", "--resume": "sess-1",
		"--plugin-dir": bundle, "--settings": `{"enabledPlugins":{}}`,
	} {
		if got, ok := flagValue(args, flag); !ok || got != want {
			t.Fatalf("%s = %q (present %v), want %q", flag, got, ok, want)
		}
	}

	plainSpec := leadSpec()
	plainSpec.Root, plainSpec.Env = spec.Root, spec.Env
	plain, err := claudeArgs(plainSpec)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := flagValue(plain, "--permission-mode"); got != DefaultLeadPermissionMode {
		t.Fatalf("--permission-mode = %q, want the default %q", got, DefaultLeadPermissionMode)
	}
	if got, _ := flagValue(plain, "--permission-prompt-tool"); got != "stdio" {
		t.Fatalf("--permission-prompt-tool = %q", got)
	}
	for _, absent := range []string{"--resume", "--model", "--effort"} {
		if slices.Contains(plain, absent) {
			t.Fatalf("a new lead session with no choices carries no %s: %v", absent, plain)
		}
	}
}

func TestClaudeLeadEnv(t *testing.T) {
	spec := leadSpec()
	spec.Env = []string{
		"PATH=/bin", "HOME=/h", "WS_CACHE_HOME=/c", "WS_CONFIG_HOME=/cfg",
		// The daemon's own session: its harness variables, mailbox
		// identity, ws-mcp launcher state, and hosted-session variables.
		"CLAUDECODE=1", "CLAUDE_CODE_ENTRYPOINT=cli", "CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=0", "CLAUDE_CONFIG_DIR=/claude",
		"WS_MAILBOX=lead@m", "WS_MAILBOX_AUTO=1",
		"WS_MCP_ROOT=/elsewhere", "WS_MCP_ALLOWED_TOOLS=tracker.query", "WS_MCP_NO_DASHBOARD=1", "WS_RSRC=/rsrc", "WS_RSRC_DIR=/rsrc",
		"WS_HOSTED_SESSION=1", "WS_HOSTED_SESSION_KEY=the-daemons-own", "WS_HOSTED_SESSION_ROW=/old/row", "WS_HOSTED_OTHER=x",
	}
	spec.Lead.Env = map[string]string{EnvHostedSession: "1", EnvHostedSessionKey: "lead-key", EnvHostedSessionRow: "/state/row.json"}
	got := claudeLeadEnv(spec)
	slices.Sort(got)
	want := []string{
		"CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=1", "CLAUDE_CONFIG_DIR=/claude", "HOME=/h", "PATH=/bin", "WS_CACHE_HOME=/c", "WS_CONFIG_HOME=/cfg",
		"WS_HOSTED_SESSION=1", "WS_HOSTED_SESSION_KEY=lead-key", "WS_HOSTED_SESSION_ROW=/state/row.json",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("claudeLeadEnv =\n%v\nwant\n%v", got, want)
	}
}

// loopChild is a streamProcess child that records each input line and
// answers every control request with a bare success, as Claude 2.1.322
// answers apply_flag_settings.
type loopChild struct {
	p     *streamProcess
	mu    sync.Mutex
	lines [][]byte
}

func (c *loopChild) name() string { return "claude" }

func (c *loopChild) write(line []byte) error {
	c.mu.Lock()
	c.lines = append(c.lines, bytes.TrimSpace(line))
	c.mu.Unlock()
	var d struct {
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
	}
	if json.Unmarshal(line, &d) == nil && d.Type == "control_request" {
		answer, _ := json.Marshal(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": d.RequestID}})
		go c.p.feed(answer, 0, false)
	}
	return nil
}

func (c *loopChild) kill()                {}
func (c *loopChild) stop(<-chan struct{}) {}

func (c *loopChild) written() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]byte(nil), c.lines...)
}

// controlRequest splits a control_request line into its request id and its
// request body.
func controlRequest(t *testing.T, line []byte) (id string, request any) {
	t.Helper()
	var d struct {
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
		Request   any    `json:"request"`
	}
	if err := json.Unmarshal(line, &d); err != nil || d.Type != "control_request" {
		t.Fatalf("not a control request: %s (%v)", line, err)
	}
	return d.RequestID, d.Request
}

// TestClaudeLeadProcessControls pins a lead's in-stream controls against
// the lines the recording's driver sent: apply_flag_settings for the
// effort, set_permission_mode for the mode, and the two approval answers.
// A return to the default effort is a null level and to the default model
// a set_model that names none (both seen to clear the override on Claude
// 2.1.322). A profile conversation takes the effort and model controls
// too, and has no mode control.
func TestClaudeLeadProcessControls(t *testing.T) {
	child := &loopChild{}
	sp := newStreamProcess(newClaudeMapper(leadSpec()), claudeProtocol{})
	sp.child, child.p = child, sp
	p := claudeProcess{streamProcess: sp, lead: true}

	if err := p.SetEffort("low"); err != nil {
		t.Fatalf("SetEffort: %v", err)
	}
	if err := p.SetPermissionMode("acceptEdits"); err != nil {
		t.Fatalf("SetPermissionMode: %v", err)
	}
	if err := p.SetEffort(""); err != nil {
		t.Fatalf("SetEffort to the default: %v", err)
	}
	if err := p.SetModel("opus"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}
	if err := p.SetModel(""); err != nil {
		t.Fatalf("SetModel to the default: %v", err)
	}
	if err := p.SetPermissionMode(""); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("an empty mode has no in-stream form, got %v", err)
	}
	allow := readLeadFixture(t, "claude-lead-approval-allow.jsonl")
	deny := readLeadFixture(t, "claude-lead-approval-deny.jsonl")
	approval := func(lines []leadLine) ApprovalRequest {
		evs := newClaudeMapper(leadSpec()).mapLine(lines[findLine(t, lines, 0, "can_use_tool", isCanUseTool)].raw, nopReplies{})
		if len(evs) != 1 || evs[0].Approval == nil {
			t.Fatalf("no approval in the fixture: %+v", evs)
		}
		return *evs[0].Approval
	}
	if err := p.AnswerApproval(approval(allow), true, "ignored for an allow"); err != nil {
		t.Fatal(err)
	}
	if err := p.AnswerApproval(approval(deny), false, "probe"); err != nil {
		t.Fatal(err)
	}
	// A deny with no message still tells the model who refused.
	bare := approval(deny)
	if err := p.AnswerApproval(bare, false, "  "); err != nil {
		t.Fatal(err)
	}
	// An allow of a request recorded with no input hands back an object.
	bare.Input = nil
	if err := p.AnswerApproval(bare, true, ""); err != nil {
		t.Fatal(err)
	}

	got := child.written()
	if len(got) != 9 {
		t.Fatalf("input lines = %d, want 9:\n%s", len(got), bytes.Join(got, []byte("\n")))
	}
	settings := readLeadFixture(t, "claude-lead-settings.jsonl")
	sent := func(subtype string) []byte {
		return settings[findLine(t, settings, 0, subtype, func(l leadLine) bool { return l.in && bytes.Contains(l.raw, []byte(`"`+subtype+`"`)) })].raw
	}
	for i, want := range [][]byte{
		sent("apply_flag_settings"), sent("set_permission_mode"),
		[]byte(`{"type":"control_request","request_id":"-","request":{"subtype":"apply_flag_settings","settings":{"effortLevel":null}}}`),
		[]byte(`{"type":"control_request","request_id":"-","request":{"subtype":"set_model","model":"opus"}}`),
		[]byte(`{"type":"control_request","request_id":"-","request":{"subtype":"set_model"}}`),
	} {
		id, request := controlRequest(t, got[i])
		_, recorded := controlRequest(t, want)
		if id == "" || !reflect.DeepEqual(request, recorded) {
			t.Fatalf("control request %d =\n%s\nwant the request of\n%s", i, got[i], want)
		}
	}
	for i, want := range [][]byte{
		allow[findLine(t, allow, 0, "sent control_response", isAnswer)].raw,
		deny[findLine(t, deny, 0, "sent control_response", isAnswer)].raw,
		[]byte(`{"type":"control_response","response":{"subtype":"success","request_id":"dedd1842-7da7-40fc-91f4-a30bf7f23279","response":{"behavior":"deny","message":"The user denied this tool call."}}}`),
		[]byte(`{"type":"control_response","response":{"subtype":"success","request_id":"dedd1842-7da7-40fc-91f4-a30bf7f23279","response":{"behavior":"allow","updatedInput":{}}}}`),
	} {
		if !sameJSON(t, json.RawMessage(got[5+i]), want) {
			t.Fatalf("approval answer %d =\n%s\nwant\n%s", i, got[5+i], want)
		}
	}

	// A profile conversation's process: the effort control too (its
	// launch settings stay, since the control merges into them), and no
	// mode control.
	plain := &loopChild{}
	psp := newStreamProcess(newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`)), claudeProtocol{})
	psp.child, plain.p = plain, psp
	pp := claudeProcess{streamProcess: psp}
	if err := pp.SetEffort("high"); err != nil {
		t.Fatalf("SetEffort on a profile conversation: %v", err)
	}
	if err := pp.SetPermissionMode("plan"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("a profile conversation's mode is not the user's to change, got %v", err)
	}
	lines := plain.written()
	if len(lines) != 1 {
		t.Fatalf("input lines = %s", bytes.Join(lines, []byte("\n")))
	}
	if _, request := controlRequest(t, lines[0]); !reflect.DeepEqual(request, map[string]any{"subtype": "apply_flag_settings", "settings": map[string]any{"effortLevel": "high"}}) {
		t.Fatalf("effort request = %s", lines[0])
	}
}
