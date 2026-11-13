package wsagent

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// leverTimers is the runner's injectable lever hold timer (Options.Timer):
// each start is recorded with a channel the test fires.
type leverTimers struct {
	mu    sync.Mutex
	durs  []time.Duration
	chans []chan time.Time
	stops int
}

func (f *leverTimers) start(d time.Duration) (<-chan time.Time, func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := make(chan time.Time, 1)
	f.durs = append(f.durs, d)
	f.chans = append(f.chans, c)
	return c, func() {
		f.mu.Lock()
		f.stops++
		f.mu.Unlock()
	}
}

func (f *leverTimers) counts() (started, stopped int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.chans), f.stops
}

func (f *leverTimers) fire(t *testing.T, i int) {
	t.Helper()
	f.mu.Lock()
	c := f.chans[i]
	f.mu.Unlock()
	c <- time.Now()
}

// leverLines are the compaction fixture's lines the hold tests replay: the
// request_compaction call and its result (paired by tool_use_id), the
// compacting status that starts the compaction's own turn, its boundary,
// and the result that ends that turn.
type leverLines struct {
	call, result, compacting, boundary, compactEnd string
}

func readLeverLines(t *testing.T) leverLines {
	t.Helper()
	lines := readLeadFixture(t, "claude-lead-compaction.jsonl")
	out := func(l leadLine) bool { return !l.in }
	call := findLine(t, lines, 0, "request_compaction tool_use", func(l leadLine) bool {
		return out(l) && l.field("type") == "assistant" && bytes.Contains(l.raw, []byte(`"name":"mcp__ws__request_compaction"`))
	})
	res := findLine(t, lines, call, "request_compaction tool_result", func(l leadLine) bool {
		return out(l) && l.field("type") == "user" && bytes.Contains(l.raw, []byte(`"tool_use_id":"toolu_01P23Tfe64xau7hs1gsv1hu5"`))
	})
	compacting := findLine(t, lines, res, "compacting status", func(l leadLine) bool {
		return out(l) && l.field("subtype") == "status" && l.field("status") == "compacting"
	})
	boundary := findLine(t, lines, compacting, "compact boundary", func(l leadLine) bool { return out(l) && l.field("subtype") == "compact_boundary" })
	end := findLine(t, lines, boundary, "compaction result", func(l leadLine) bool { return out(l) && l.field("type") == "result" })
	return leverLines{string(lines[call].raw), string(lines[res].raw), string(lines[compacting].raw), string(lines[boundary].raw), string(lines[end].raw)}
}

// leverHarness is a lead harness whose lever hold timer the test drives.
func leverHarness(t *testing.T) (*leadHarness, *leverTimers) {
	timers := &leverTimers{}
	h := newLeadHarness(t)
	h.opts.Timer = timers.start
	return h.start(), timers
}

// leverTurn runs the first turn up to the lever call's result, with "two"
// sent meanwhile, and returns the conversation, its holder, and its key.
func leverTurn(t *testing.T, h *leadHarness, ll leverLines) (id, holder, key string) {
	t.Helper()
	id, holder = h.runLead("one")
	key = h.record(id).WSKey
	h.holders.out(holder, initLine("sess-1", "auto"), ll.call, ll.result)
	h.sync(id, holder)
	h.send(id, "two")
	if st := h.state(id); st.Held != 1 {
		t.Fatalf("a message sent during the turn is held: %+v", st)
	}
	return id, holder, key
}

// heldThrough checks nothing past the first turn was written.
func heldThrough(t *testing.T, h *leadHarness, id, holder, what string) {
	t.Helper()
	h.quiesce(id)
	if got := h.userInputs(holder); !slices.Equal(got, []string{"one"}) {
		t.Fatalf("%s: written %q; the follow-up must wait for the compaction", what, got)
	}
}

// TestLeverHoldWaitsForTheCompaction: a follow-up, mail, and a child report
// queued during the turn that requested a compaction are held at its end
// until that compaction starts, and go as one turn when the compaction's own
// turn ends: the held follow-up comes after the compaction (Decision 7).
func TestLeverHoldWaitsForTheCompaction(t *testing.T) {
	ll := readLeverLines(t)
	h, timers := leverHarness(t)
	id, holder, key := leverTurn(t, h, ll)
	h.holders.out(holder, replyLine("u1", "DONE"), resultLine())
	h.idle(id)
	heldThrough(t, h, id, holder, "at the lever turn's end")
	if started, _ := timers.counts(); started != 1 || timers.durs[0] != leverHoldCap {
		t.Fatalf("the cap timer should start once for %v: %d started, %v", leverHoldCap, started, timers.durs)
	}
	// Mail and a child report are held with it.
	h.mail.deliver(key, "From b@x: more")
	eventually(t, "the mail to be claimed", func() bool { return len(h.record(id).Mail) == 1 })
	c := h.runner.conv(id)
	c.do(func() { c.takeSteer("child report", 0) })
	heldThrough(t, h, id, holder, "mail and a report during the hold")

	h.holders.out(holder, ll.compacting)
	eventually(t, "the compaction's turn", func() bool { return h.state(id).Status == StatusRunning })
	heldThrough(t, h, id, holder, "while the compaction runs")
	if _, stopped := timers.counts(); stopped != 1 {
		t.Fatalf("the compaction start should stop the cap timer: %d stops", stopped)
	}
	h.holders.out(holder, ll.boundary, ll.compactEnd)
	eventually(t, "the held turn", func() bool { return len(h.userInputs(holder)) == 2 })
	got := h.userInputs(holder)[1]
	for _, part := range []string{"two", "From b@x: more", "child report"} {
		if !strings.Contains(got, part) {
			t.Fatalf("the turn after the compaction = %q; want it to carry %q", got, part)
		}
	}
}

// TestLeverHoldCapReleases: with no compaction start, the cap releases what
// waited.
func TestLeverHoldCapReleases(t *testing.T) {
	ll := readLeverLines(t)
	h, timers := leverHarness(t)
	id, holder, _ := leverTurn(t, h, ll)
	h.holders.out(holder, replyLine("u1", "DONE"), resultLine())
	h.idle(id)
	heldThrough(t, h, id, holder, "before the cap")
	timers.fire(t, 0)
	eventually(t, "the follow-up at the cap", func() bool { return slices.Equal(h.userInputs(holder), []string{"one", "two"}) })
}

// TestLeverHoldNotArmed: a turn with no lever call, an interrupted lever
// turn (its aborted_* result), and a lever turn the runner interrupted
// itself deliver queued input at their end as before.
func TestLeverHoldNotArmed(t *testing.T) {
	ll := readLeverLines(t)
	aborted := func() string {
		var m map[string]any
		_ = json.Unmarshal([]byte(resultLine()), &m)
		m["subtype"], m["is_error"], m["terminal_reason"] = "error_during_execution", true, "aborted_tools"
		return jsonLine(m)
	}
	t.Run("no lever call", func(t *testing.T) {
		h, timers := leverHarness(t)
		id, holder := h.runLead("one")
		h.holders.out(holder, initLine("sess-1", "auto"), replyLine("u1", "first"))
		h.sync(id, holder)
		h.send(id, "two")
		h.holders.out(holder, resultLine())
		eventually(t, "the follow-up at the turn end", func() bool { return slices.Equal(h.userInputs(holder), []string{"one", "two"}) })
		if started, _ := timers.counts(); started != 0 {
			t.Fatalf("no cap timer should start: %d", started)
		}
	})
	t.Run("interrupted lever turn", func(t *testing.T) {
		h, timers := leverHarness(t)
		id, holder, _ := leverTurn(t, h, ll)
		h.holders.out(holder, aborted())
		eventually(t, "the follow-up at the turn end", func() bool { return slices.Equal(h.userInputs(holder), []string{"one", "two"}) })
		if started, _ := timers.counts(); started != 0 {
			t.Fatalf("no cap timer should start: %d", started)
		}
		h.quiesce(id)
	})
	t.Run("lever turn the runner interrupted", func(t *testing.T) {
		h, timers := leverHarness(t)
		id, holder, key := leverTurn(t, h, ll)
		// The interrupt takes the held message back; mail claimed after it
		// is what waits for the turn's end.
		if _, _, err := h.runner.Interrupt(id); err != nil {
			t.Fatal(err)
		}
		h.mail.deliver(key, "From b@x: more")
		eventually(t, "the mail to be claimed", func() bool { return len(h.record(id).Mail) == 1 })
		// A harness that reports no aborted reason for it.
		h.holders.out(holder, resultLine())
		eventually(t, "the mail at the turn end", func() bool { return slices.Equal(h.userInputs(holder), []string{"one", "From b@x: more"}) })
		if started, _ := timers.counts(); started != 0 {
			t.Fatalf("no cap timer should start: %d", started)
		}
	})
}
