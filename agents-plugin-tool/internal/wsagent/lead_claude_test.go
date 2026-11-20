package wsagent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"github.com/kang-sw/devenv/internal/wslive"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
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
