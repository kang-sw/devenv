package wsagent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// TestClaudeLeadArgsStreamPartialMessages: a hosted lead asks for partial
// messages, so its reply can stream; a worker's command line does not.
func TestClaudeLeadArgsStreamPartialMessages(t *testing.T) {
	lead, err := claudeLeadArgs(claudePluginLeadSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(lead, "--include-partial-messages") {
		t.Fatalf("lead args lack --include-partial-messages: %v", lead)
	}
	worker, err := claudeArgs(claudeSpec(t, `{"backend":"claude","tools":["Read"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(worker, "--include-partial-messages") {
		t.Fatalf("worker args carry --include-partial-messages: %v", worker)
	}
}

// replayPartial maps the partial-messages recording line by line into a live
// state, recording the streaming reply after every line and the ring entries.
func replayPartial(t *testing.T, step int64) (st *wslive.State, drafts []string, ring []wslive.RingEntry, events []Event) {
	t.Helper()
	f, err := os.Open("testdata/claude-lead-partial-messages.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	m := newClaudeMapper(StartSpec{Lead: &LeadSpec{}})
	clock := int64(1_000_000)
	m.now = func() time.Time { clock += step; return time.UnixMilli(clock) }
	st = wslive.NewState("test-key")
	turn := false
	apply := func(ev wslive.Event) {
		res, err := st.Apply(ev, clock)
		if err != nil {
			t.Fatalf("apply %s: %v", ev.Kind, err)
		}
		ring = append(ring, res.Ring...)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		for _, ev := range m.mapLine(sc.Bytes(), nopReplies{}) {
			events = append(events, ev)
			switch {
			case ev.Live != nil:
				apply(*ev.Live)
			case ev.TurnStart && !turn: // the runner opens a turn once
				turn = true
				apply(wslive.Event{Kind: wslive.KindTurnStart})
			case ev.TurnEnd:
				turn = false
				apply(wslive.Event{Kind: wslive.KindTurnEnd})
			}
		}
		if r := st.Snapshot(clock).Lead.Reply; r != nil {
			drafts = append(drafts, r.Text)
		}
	}
	return st, drafts, ring, events
}

// TestClaudeMapperStreamsPartialReply: the lead's text deltas show as the
// reply so far while the block is written, a subagent's never do, and the
// block's whole assistant line finishes it as one ring reply: no duplicate,
// no streaming reply left behind.
func TestClaudeMapperStreamsPartialReply(t *testing.T) {
	st, drafts, ring, _ := replayPartial(t, 200)
	var replies []string
	for _, e := range ring {
		if e.Kind == wslive.RingReply {
			replies = append(replies, e.Text)
		}
	}
	if len(replies) != 1 || !strings.HasPrefix(replies[0], "Rivers carve their paths") {
		t.Fatalf("want exactly one finished reply, got %q", replies)
	}
	if len(drafts) < 3 {
		t.Fatalf("the reply should stream over several lines, drafts %q", drafts)
	}
	for i, d := range drafts {
		if strings.Contains(d, "SUBAGENT") {
			t.Fatalf("a subagent's delta reached the lead's reply: %q", d)
		}
		if !strings.HasPrefix(replies[0], d) {
			t.Fatalf("draft %d is not a prefix of the final reply: %q", i, d)
		}
		if i > 0 && len(d) < len(drafts[i-1]) {
			t.Fatalf("drafts should only grow: %q then %q", drafts[i-1], d)
		}
	}
	if st.Snapshot(2_000_000).Lead.Reply != nil {
		t.Fatalf("no streaming reply may outlive the final one")
	}
}

// TestClaudeMapperThrottlesPartialReply: deltas closer than
// claudePartialEvery send the reply so far once; the final line still
// carries the whole text.
func TestClaudeMapperThrottlesPartialReply(t *testing.T) {
	_, _, ring, events := replayPartial(t, 0)
	partials := 0
	for _, ev := range events {
		if ev.Live != nil && ev.Live.Kind == wslive.KindReply && !ev.Live.Final {
			partials++
		}
	}
	if partials != 1 {
		t.Fatalf("with no time passing the reply so far should be sent once, sent %d", partials)
	}
	replies := 0
	for _, e := range ring {
		if e.Kind == wslive.RingReply {
			replies++
		}
	}
	if replies != 1 {
		t.Fatalf("want one finished reply, got %d", replies)
	}
}

// TestClaudeMapperInterruptedStreamKeepsItsText: a stream cut short (no
// whole assistant line) is finished by the turn's end with the text so far.
func TestClaudeMapperInterruptedStreamKeepsItsText(t *testing.T) {
	m := newClaudeMapper(StartSpec{Lead: &LeadSpec{}})
	st := wslive.NewState("test-key")
	var ring []wslive.RingEntry
	apply := func(ev wslive.Event) {
		res, err := st.Apply(ev, 1)
		if err != nil {
			t.Fatal(err)
		}
		ring = append(ring, res.Ring...)
	}
	apply(wslive.Event{Kind: wslive.KindTurnStart})
	for _, line := range []string{
		`{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}},"parent_tool_use_id":null}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"half a "}},"parent_tool_use_id":null}`,
	} {
		for _, ev := range m.mapLine([]byte(line), nopReplies{}) {
			if ev.Live != nil {
				apply(*ev.Live)
			}
		}
	}
	apply(wslive.Event{Kind: wslive.KindTurnEnd})
	if len(ring) == 0 || ring[0].Kind != wslive.RingReply || ring[0].Text != "half a " {
		t.Fatalf("the cut stream should finish as the reply, ring %+v", ring)
	}
}

// claudeFixtureLines reads a recording's lines.
func claudeFixtureLines(t *testing.T, name string) [][]byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var lines [][]byte
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if len(line) > 0 {
			lines = append(lines, line)
		}
	}
	return lines
}

// TestClaudeRingSkipMatchesTheDroppedDeltas: the holder's skip pattern
// matches exactly the recorded tool input, thinking, and signature deltas,
// never a text delta, a block or message boundary, or any other line.
func TestClaudeRingSkipMatchesTheDroppedDeltas(t *testing.T) {
	skip := regexp.MustCompile(claudeRingSkip)
	matched := map[string]int{}
	for _, name := range []string{"claude-lead-partial-messages.jsonl", "claude-lead-partial-tool-input.jsonl"} {
		for _, line := range claudeFixtureLines(t, name) {
			var l struct {
				Type  string `json:"type"`
				Event struct {
					Type  string `json:"type"`
					Delta struct {
						Type string `json:"type"`
					} `json:"delta"`
				} `json:"event"`
			}
			if err := json.Unmarshal(line, &l); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			delta := l.Event.Delta.Type
			want := l.Type == "stream_event" && l.Event.Type == "content_block_delta" &&
				(delta == "input_json_delta" || delta == "thinking_delta" || delta == "signature_delta")
			if got := skip.Match(line); got != want {
				t.Fatalf("%s: skip = %v, want %v for %.160s", name, got, want, line)
			}
			if want {
				matched[delta]++
			}
		}
	}
	for _, delta := range []string{"input_json_delta", "thinking_delta", "signature_delta"} {
		if matched[delta] == 0 {
			t.Errorf("the recordings hold no %s line the pattern matched", delta)
		}
	}
}

// TestClaudeMapperIgnoresRingSkippedLines: the lines a lead's holder keeps
// out of its ring change nothing the mapper makes of a recording.
func TestClaudeMapperIgnoresRingSkippedLines(t *testing.T) {
	skip := regexp.MustCompile(claudeRingSkip)
	mapAll := func(keep func([]byte) bool) []Event {
		m := newClaudeMapper(StartSpec{Lead: &LeadSpec{}})
		m.now = func() time.Time { return time.UnixMilli(1_000_000) }
		var events []Event
		for _, name := range []string{"claude-lead-partial-tool-input.jsonl", "claude-lead-partial-messages.jsonl"} {
			for _, line := range claudeFixtureLines(t, name) {
				if keep(line) {
					events = append(events, m.mapLine(line, nopReplies{})...)
				}
			}
		}
		return events
	}
	all := mapAll(func([]byte) bool { return true })
	kept := mapAll(func(line []byte) bool { return !skip.Match(line) })
	if len(all) == 0 || !reflect.DeepEqual(all, kept) {
		t.Fatalf("mapping without the skipped lines differs:\nall  %+v\nkept %+v", all, kept)
	}
}

// TestClaudeLeadHolderSkipsDroppedDeltas: a hosted Claude lead's holder is
// launched with the skip pattern.
func TestClaudeLeadHolderSkipsDroppedDeltas(t *testing.T) {
	h := newLeadHarness(t).start()
	_, holder := h.runLead("one")
	if got := h.holders.launchSkip(holder); got != claudeRingSkip {
		t.Fatalf("a Claude lead's holder ring skip = %q, want claudeRingSkip", got)
	}
}
