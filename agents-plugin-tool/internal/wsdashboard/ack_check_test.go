package wsdashboard

import (
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// decisionNudgeFormat is the nudge text the answer-ack ticket's Decision 9
// fixes, written out here rather than taken from the implementation's
// constant, so a drift in the shipped text fails a test.
const decisionNudgeFormat = "Dashboard answer check: the owner's answers to %s reached you, but those board items are still open. Record each answer you acted on with wb:answer (the owner's words as its body); if an item should stay open, say why. This check runs once per answer batch."

func TestAckBatchTakesValueChipsSentInTheSettledRange(t *testing.T) {
	const through, settled = 1000, 2000
	rec := func(chips ...wsinbox.Chip) wsinbox.Record { return wsinbox.Record{Entry: wsinbox.Entry{Chips: chips}} }
	value := func(id string, at int64) wsinbox.Chip { return wsinbox.Chip{ID: id, Answer: "a", SentAt: at} }
	drained := []wsinbox.Record{
		// Out of ID order, so the result's order is the function's.
		rec(value("11", 1500)),
		rec(value("01", through)),
		rec(wsinbox.Chip{ID: "02", Note: "only a note", SentAt: 1500}),
		rec(value("03", settled)),
		rec(value("04", 0)),
		rec(wsinbox.Chip{ID: "05", Answer: "ok", Note: "and a note", SentAt: 1500}),
		rec(value("06", 1200)),
		rec(value("07", 1400)),
		rec(value("08", through-1)),
		rec(value("09", 1500)),
		rec(value("09", 1600)),
		rec(value("10", 1500)),
		{Entry: wsinbox.Entry{Topics: []wsinbox.TopicNote{{Topic: "plan", Note: "a topic note"}}, SentAt: 1500}},
	}
	// 06 was re-asked after its answer, 07 at its answer's millisecond.
	reasked := map[int]int64{6: 1300, 7: 1400}

	if got, want := ackBatch(drained, reasked, through, settled), []int{1, 5, 7, 9, 10, 11}; !reflect.DeepEqual(got, want) {
		t.Errorf("ackBatch = %v, want %v", got, want)
	}
	if got, want := ackBatch(drained, nil, through, settled), []int{1, 5, 6, 7, 9, 10, 11}; !reflect.DeepEqual(got, want) {
		t.Errorf("ackBatch without re-asks = %v, want %v", got, want)
	}
	if got := ackBatch(drained, reasked, settled+1, settled+1000); len(got) != 0 {
		t.Errorf("ackBatch past every chip = %v, want none", got)
	}

	// 10 is answered on the board, 11 is gone from it: neither is named.
	open := map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true, 9: true}
	items := BoardItems{Open: open, Reasked: reasked}
	if got, want := ackNudgeIDs(drained, items, through, settled), []int{1, 5, 7, 9}; !reflect.DeepEqual(got, want) {
		t.Errorf("ackNudgeIDs = %v, want %v", got, want)
	}
	if got := ackNudgeIDs(drained, BoardItems{Reasked: reasked}, through, settled); len(got) != 0 {
		t.Errorf("ackNudgeIDs with no open item = %v, want none", got)
	}
}

func TestAckNudgeTextNamesTheIDsAscendingAndCapped(t *testing.T) {
	if got, want := ackNudgeText([]int{558, 561}), fmt.Sprintf(decisionNudgeFormat, "(558), (561)"); got != want {
		t.Errorf("ackNudgeText(558, 561) =\n%q\nwant\n%q", got, want)
	}
	if got, want := ackNudgeText([]int{7}), fmt.Sprintf(decisionNudgeFormat, "(07)"); got != want {
		t.Errorf("ackNudgeText(7) = %q, want %q", got, want)
	}
	var ids, names []string
	var many []int
	for id := 1; id <= 25; id++ {
		many = append(many, id)
		ids = append(ids, fmt.Sprintf("(%02d)", id))
	}
	names = ids[:20]
	if got, want := ackNudgeText(many[:20]), fmt.Sprintf(decisionNudgeFormat, strings.Join(names, ", ")); got != want {
		t.Errorf("ackNudgeText of 20 IDs = %q, want %q", got, want)
	}
	if got, want := ackNudgeText(many), fmt.Sprintf(decisionNudgeFormat, strings.Join(names, ", ")+", ..."); got != want {
		t.Errorf("ackNudgeText of 25 IDs = %q, want %q", got, want)
	}
}

// ackSource is the page fixture's source with a board item source
// (BoardItemsSource): each session's open items and re-ask times are set by
// the test, and the board reads are counted.
type ackSource struct {
	*pageFake
	boardMu    sync.Mutex
	open       map[string]map[int]bool
	reasked    map[string]map[int]int64
	boardReads map[string]int
}

func newAckSource() *ackSource {
	return &ackSource{pageFake: newPageFake(), open: map[string]map[int]bool{}, reasked: map[string]map[int]int64{}, boardReads: map[string]int{}}
}

func (s *ackSource) BoardItems(reg Registration) (BoardItems, bool) {
	s.boardMu.Lock()
	defer s.boardMu.Unlock()
	s.boardReads[reg.ReplyID]++
	return BoardItems{Open: maps.Clone(s.open[reg.ReplyID]), Reasked: maps.Clone(s.reasked[reg.ReplyID])}, true
}

// setOpen makes ids session id's open items, replacing the earlier set.
func (s *ackSource) setOpen(id string, ids ...int) {
	s.boardMu.Lock()
	defer s.boardMu.Unlock()
	open := map[int]bool{}
	for _, n := range ids {
		open[n] = true
	}
	s.open[id] = open
}

func (s *ackSource) boardReadCount(id string) int {
	s.boardMu.Lock()
	defer s.boardMu.Unlock()
	return s.boardReads[id]
}

// ackFixture is a daemon over ackSource with one board session, ackID,
// whose lead is idle under a live adapter.
type ackFixture struct {
	t       *testing.T
	dir     string
	project string
	src     *ackSource
	d       *Daemon
	base    string
}

const ackID = "acksession"

// newAckFixture registers ackID, idle with a board, without starting the
// daemon (start).
func newAckFixture(t *testing.T) *ackFixture {
	t.Helper()
	f := &ackFixture{t: t, dir: t.TempDir(), project: t.TempDir(), src: newAckSource()}
	f.addSession(ackID)
	return f
}

// startAckFixture is newAckFixture started with an answer-check delay long
// enough that no timer check runs during the test: the test calls
// checkAnswers itself.
func startAckFixture(t *testing.T) *ackFixture {
	t.Helper()
	f := newAckFixture(t)
	f.start(time.Hour)
	return f
}

// start starts the daemon with the given answer-check delay and waits for
// ackID to be indexed.
func (f *ackFixture) start(delay time.Duration) {
	f.t.Helper()
	f.d = startIndexedDaemon(f.t, f.dir, f.src, time.Hour, func(o *DaemonOptions) {
		o.noGitBadge = true
		o.ackDelay = delay
	})
	f.base = endpointURL(f.d.Endpoint(), "")
	f.waitIndexed(ackID)
}

func (f *ackFixture) waitIndexed(id string) {
	f.t.Helper()
	waitFor(f.t, id+" to be indexed", func() bool { _, ok := f.d.Index().Session(id); return ok })
}

// addSession registers a board session with an idle lead; once the daemon
// runs, it is indexed before this returns.
func (f *ackFixture) addSession(id string) {
	f.t.Helper()
	register(f.t, f.dir, id, f.project, time.Now())
	f.src.setPage(id, PageContent{HasBoard: true, Seq: 1})
	f.idle(id)
	if f.d != nil {
		f.d.Index().Refresh()
		f.waitIndexed(id)
	}
}

// lead writes session id's snapshot: adapter (empty for none), the given
// lead, and updatedAt as its heartbeat.
func (f *ackFixture) lead(id, adapter string, updatedAt time.Time, lead wslive.Lead) {
	f.t.Helper()
	writeSnapshot(f.t, f.dir, id, &wslive.Snapshot{SchemaVersion: 1, Adapter: adapter, UpdatedAt: updatedAt.UnixMilli(), HeartbeatMs: 30000, Lead: lead})
}

// idle writes an idle lead under a live adapter.
func (f *ackFixture) idle(id string) {
	f.t.Helper()
	f.lead(id, "claude", time.Now(), wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}})
}

func (f *ackFixture) sessionDir(id string) string { return SessionDir(f.dir, id) }
func (f *ackFixture) inbox(id string) string      { return wsinbox.Dir(f.sessionDir(id)) }

// seed writes the answer check's record with through.
func (f *ackFixture) seed(id string, through int64) {
	f.t.Helper()
	if err := writeAckCheck(f.sessionDir(id), ackCheckRecord{Through: through}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *ackFixture) record(id string) ackCheckRecord { return readAckCheck(f.sessionDir(id)) }

// deliver sends chips at sentAt and drains them, as an adapter does when a
// turn starts.
func (f *ackFixture) deliver(id string, sentAt time.Time, chips ...wsinbox.Chip) {
	f.t.Helper()
	f.sendHeld(id, sentAt, wsinbox.Entry{Parts: []string{"answers"}, Chips: chips})
	if drained, err := wsinbox.Drain(f.inbox(id)); err != nil || len(drained) != 1 {
		f.t.Fatalf("drain = %+v, %v", drained, err)
	}
}

// sendHeld holds entry in session id's inbox, sent at sentAt.
func (f *ackFixture) sendHeld(id string, sentAt time.Time, entry wsinbox.Entry) {
	f.t.Helper()
	if _, _, err := wsinbox.Send(f.inbox(id), entry, sentAt); err != nil {
		f.t.Fatal(err)
	}
}

// drain drains session id's inbox (a turn starting).
func (f *ackFixture) drain(id string) []wsinbox.Record {
	f.t.Helper()
	drained, err := wsinbox.Drain(f.inbox(id))
	if err != nil {
		f.t.Fatal(err)
	}
	return drained
}

// turn appends a settled turn of durMs with reason to session id's ring and
// returns its start as the ring reads it back (its time less its duration).
func (f *ackFixture) turn(id string, durMs int64, reason string) int64 {
	f.t.Helper()
	dir := wslive.RingDir(f.sessionDir(id))
	if err := wslive.AppendRing(dir, []wslive.RingEntry{{Kind: wslive.RingTurn, DurationMs: durMs, Reason: reason}}); err != nil {
		f.t.Fatal(err)
	}
	ring, err := wslive.ReadRing(dir)
	if err != nil || len(ring) == 0 {
		f.t.Fatalf("ring = %+v, %v", ring, err)
	}
	last := ring[len(ring)-1]
	if last.Kind != wslive.RingTurn {
		f.t.Fatalf("newest ring entry = %+v", last)
	}
	return last.At.UnixMilli() - last.DurationMs
}

func (f *ackFixture) held(id string) []wsinbox.Record {
	f.t.Helper()
	held, err := wsinbox.Held(f.inbox(id))
	if err != nil {
		f.t.Fatal(err)
	}
	return held
}

// batch builds a settled answer batch on session id: the record's through a
// minute back, a value chip for item sent half a minute back and drained,
// and a 10 s turn settling now (so its start lies after the chip). It
// returns through and the turn's start.
func (f *ackFixture) batch(id string, item int, reason string) (through, start int64) {
	f.t.Helper()
	base := time.Now()
	through = base.Add(-time.Minute).UnixMilli()
	f.seed(id, through)
	f.deliver(id, base.Add(-30*time.Second), wsinbox.Chip{ID: fmt.Sprintf("%02d", item), Answer: "a"})
	start = f.turn(id, 10_000, reason)
	if start <= base.Add(-30*time.Second).UnixMilli() {
		f.t.Fatalf("turn start %d does not follow the chip", start)
	}
	return through, start
}

// wantNudge asserts session id holds exactly one message, the nudge naming
// names: one part, no chips, topics, or images.
func (f *ackFixture) wantNudge(id string, names string) {
	f.t.Helper()
	held := f.held(id)
	if len(held) != 1 {
		f.t.Fatalf("held = %+v, want one nudge", held)
	}
	e := held[0].Entry
	want := fmt.Sprintf(decisionNudgeFormat, names)
	if !reflect.DeepEqual(e.Parts, []string{want}) || len(e.Chips) != 0 || len(e.Topics) != 0 || len(e.Images) != 0 || e.Steer || e.Command != nil {
		f.t.Fatalf("held entry = %+v, want the nudge %q alone", e, want)
	}
}

// wantNone asserts session id holds no message.
func (f *ackFixture) wantNone(id string) {
	f.t.Helper()
	if held := f.held(id); len(held) != 0 {
		f.t.Fatalf("held = %+v, want no nudge", held)
	}
}

// wantThrough asserts the answer check's record reads through.
func (f *ackFixture) wantThrough(id string, through int64) {
	f.t.Helper()
	if got := f.record(id).Through; got != through {
		f.t.Fatalf("through = %d, want %d", got, through)
	}
}

// TestAnswerCheckNudgesOnceForAnOpenAnsweredItem: a value chip delivered
// before the settled turn, its item still open, holds one nudge naming the
// item and moves through to the turn's start; a second check, while the
// nudge is held and after it is delivered, sends nothing more.
func TestAnswerCheckNudgesOnceForAnOpenAnsweredItem(t *testing.T) {
	f := startAckFixture(t)
	f.src.setOpen(ackID, 7)
	_, start := f.batch(ackID, 7, "")
	f.d.checkAnswers(ackID)
	f.wantNudge(ackID, "(07)")
	f.wantThrough(ackID, start)

	f.d.checkAnswers(ackID)
	f.wantNudge(ackID, "(07)")
	f.drain(ackID)
	f.d.checkAnswers(ackID)
	f.wantNone(ackID)
	f.wantThrough(ackID, start)
}

// TestAnswerCheckDoesNotLoop: the nudge's own turn settles with the item
// still open; the nudge carries no chip, so it is no batch and no second
// nudge follows, and through stays at the answer's turn.
func TestAnswerCheckDoesNotLoop(t *testing.T) {
	f := startAckFixture(t)
	f.src.setOpen(ackID, 7)
	_, start := f.batch(ackID, 7, "")
	f.d.checkAnswers(ackID)
	f.wantNudge(ackID, "(07)")

	f.drain(ackID)
	if next := f.turn(ackID, 1, ""); next <= start {
		t.Fatalf("the nudge's turn start %d does not follow %d", next, start)
	}
	f.d.checkAnswers(ackID)
	f.wantNone(ackID)
	f.wantThrough(ackID, start)
}

// TestAnswerCheckSpendsARecordedBatchWithoutANudge: the item was answered on
// the board, so no nudge goes out, and the batch is still spent.
func TestAnswerCheckSpendsARecordedBatchWithoutANudge(t *testing.T) {
	f := startAckFixture(t)
	f.src.setOpen(ackID, 3)
	_, start := f.batch(ackID, 7, "")
	f.d.checkAnswers(ackID)
	f.wantNone(ackID)
	f.wantThrough(ackID, start)
	if f.src.boardReadCount(ackID) != 1 {
		t.Fatalf("board reads = %d, want 1", f.src.boardReadCount(ackID))
	}
}

// TestAnswerCheckIgnoresNoteOnlyChips: a note-only chip and a topic note
// alone are no batch: no nudge, nothing written, and the board is not read.
func TestAnswerCheckIgnoresNoteOnlyChips(t *testing.T) {
	f := startAckFixture(t)
	f.src.setOpen(ackID, 7)
	base := time.Now()
	through := base.Add(-time.Minute).UnixMilli()
	f.seed(ackID, through)
	f.sendHeld(ackID, base.Add(-30*time.Second), wsinbox.Entry{
		Chips:  []wsinbox.Chip{{ID: "07", Note: "only a note"}},
		Topics: []wsinbox.TopicNote{{Topic: "plan", Note: "a topic note"}},
	})
	f.drain(ackID)
	f.turn(ackID, 10_000, "")
	f.d.checkAnswers(ackID)
	f.wantNone(ackID)
	f.wantThrough(ackID, through)
	if n := f.src.boardReadCount(ackID); n != 0 {
		t.Fatalf("board reads = %d, want none for an empty batch", n)
	}
}

// TestAnswerCheckWaitsOutHeldInput: a held message means another turn is
// coming, so the check skips; once that turn settles, the earlier batch is
// nudged.
func TestAnswerCheckWaitsOutHeldInput(t *testing.T) {
	f := startAckFixture(t)
	f.src.setOpen(ackID, 7)
	through, _ := f.batch(ackID, 7, "")
	f.sendHeld(ackID, time.Now(), wsinbox.Entry{Parts: []string{"one more thing"}})
	f.d.checkAnswers(ackID)
	if held := f.held(ackID); len(held) != 1 || held[0].Entry.Text() != "one more thing" {
		t.Fatalf("held = %+v, want the user's message alone", held)
	}
	f.wantThrough(ackID, through)

	f.drain(ackID)
	next := f.turn(ackID, 1, "")
	f.d.checkAnswers(ackID)
	f.wantNudge(ackID, "(07)")
	f.wantThrough(ackID, next)
}

// TestAnswerCheckSkipsABusyLead: mid-turn, or waiting on a permission, the
// check skips without moving through; the same batch is nudged once the
// lead is idle.
func TestAnswerCheckSkipsABusyLead(t *testing.T) {
	for _, c := range []struct {
		name string
		lead wslive.Lead
	}{
		{"mid-turn", wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, TurnStartedAt: time.Now().UnixMilli()}},
		{"thinking without a turn start", wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseThinking}}},
		{"permission wait", wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhasePermission, Permission: &wslive.Permission{ToolName: "Bash"}}, TurnStartedAt: time.Now().UnixMilli()}},
		{"permission held while idle", wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle, Permission: &wslive.Permission{ToolName: "Bash"}}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := startAckFixture(t)
			f.src.setOpen(ackID, 7)
			through, start := f.batch(ackID, 7, "")
			f.lead(ackID, "claude", time.Now(), c.lead)
			f.d.checkAnswers(ackID)
			f.wantNone(ackID)
			f.wantThrough(ackID, through)

			f.idle(ackID)
			f.d.checkAnswers(ackID)
			f.wantNudge(ackID, "(07)")
			f.wantThrough(ackID, start)
		})
	}
}

// TestAnswerCheckSkipsATurnTheDashboardInterrupted: the interrupt route
// records interruptedAt; a settled turn that started before it is no
// settle (no nudge, through unchanged), and the batch is nudged at the next
// turn that started after it.
func TestAnswerCheckSkipsATurnTheDashboardInterrupted(t *testing.T) {
	f := startAckFixture(t)
	f.src.setOpen(ackID, 7)
	base := time.Now()
	through := base.Add(-time.Minute).UnixMilli()
	f.seed(ackID, through)
	f.deliver(ackID, base.Add(-30*time.Second), wsinbox.Chip{ID: "07", Answer: "a"})

	before := time.Now().UnixMilli()
	if code, _, raw := postJSON(t, f.base+"/s/"+ackID+"/interrupt", f.base, ``); code != http.StatusOK {
		t.Fatalf("interrupt = %d: %s", code, raw)
	}
	after := time.Now().UnixMilli()
	rec := f.record(ackID)
	if rec.InterruptedAt < before || rec.InterruptedAt > after || rec.Through != through {
		t.Fatalf("record after the interrupt = %+v, want interruptedAt in [%d, %d] and through %d", rec, before, after, through)
	}
	// The adapter takes the interrupt entry and stops the turn.
	if _, err := wsinbox.DrainInterrupt(f.inbox(ackID)); err != nil {
		t.Fatal(err)
	}
	if stopped := f.turn(ackID, 10_000, ""); stopped > rec.InterruptedAt {
		t.Fatalf("stopped turn start %d does not precede the interrupt %d", stopped, rec.InterruptedAt)
	}
	f.d.checkAnswers(ackID)
	f.wantNone(ackID)
	f.wantThrough(ackID, through)

	waitFor(t, "the clock to pass the interrupt", func() bool { return time.Now().UnixMilli() > rec.InterruptedAt+2 })
	next := f.turn(ackID, 1, "")
	if next <= rec.InterruptedAt {
		t.Fatalf("next turn start %d does not follow the interrupt %d", next, rec.InterruptedAt)
	}
	f.d.checkAnswers(ackID)
	f.wantNudge(ackID, "(07)")
	if got := f.record(ackID); got != (ackCheckRecord{Through: next, InterruptedAt: rec.InterruptedAt}) {
		t.Fatalf("record = %+v, want through %d and the interrupt kept", got, next)
	}
}

// TestAnswerCheckSkipsATerminalStop: a settled turn whose end reason is
// aborted is no settle (no nudge, through unchanged); answer, error, and no
// reason are normal settles.
func TestAnswerCheckSkipsATerminalStop(t *testing.T) {
	for _, c := range []struct {
		reason string
		nudge  bool
	}{
		{"aborted", false},
		{"answer", true},
		{"error", true},
		{"", true},
	} {
		t.Run("reason="+c.reason, func(t *testing.T) {
			f := startAckFixture(t)
			f.src.setOpen(ackID, 7)
			through, start := f.batch(ackID, 7, c.reason)
			f.d.checkAnswers(ackID)
			if !c.nudge {
				f.wantNone(ackID)
				f.wantThrough(ackID, through)
				return
			}
			f.wantNudge(ackID, "(07)")
			f.wantThrough(ackID, start)
		})
	}
}

// TestAnswerCheckSeedsItsFirstEvaluation: with no record, or a record that
// holds only interruptedAt, the first check writes through as the settled
// turn's start and sends nothing, even over a non-empty batch.
func TestAnswerCheckSeedsItsFirstEvaluation(t *testing.T) {
	for _, c := range []struct {
		name        string
		interrupted int64
	}{
		{"no record", 0},
		{"interrupt only", time.Now().Add(-2 * time.Minute).UnixMilli()},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := startAckFixture(t)
			f.src.setOpen(ackID, 7)
			if c.interrupted != 0 {
				if err := writeAckCheck(f.sessionDir(ackID), ackCheckRecord{InterruptedAt: c.interrupted}); err != nil {
					t.Fatal(err)
				}
			}
			base := time.Now()
			f.deliver(ackID, base.Add(-30*time.Second), wsinbox.Chip{ID: "07", Answer: "a"})
			start := f.turn(ackID, 10_000, "")
			f.d.checkAnswers(ackID)
			f.wantNone(ackID)
			if got := f.record(ackID); got != (ackCheckRecord{Through: start, InterruptedAt: c.interrupted}) {
				t.Fatalf("record = %+v, want through %d, interruptedAt %d", got, start, c.interrupted)
			}
		})
	}
}

// TestAnswerCheckSkipsATurnWithoutDuration: a settled turn with no duration
// has no usable start: no nudge, and through unchanged.
func TestAnswerCheckSkipsATurnWithoutDuration(t *testing.T) {
	f := startAckFixture(t)
	f.src.setOpen(ackID, 7)
	base := time.Now()
	through := base.Add(-time.Minute).UnixMilli()
	f.seed(ackID, through)
	f.deliver(ackID, base.Add(-30*time.Second), wsinbox.Chip{ID: "07", Answer: "a"})
	f.turn(ackID, 0, "")
	f.d.checkAnswers(ackID)
	f.wantNone(ackID)
	f.wantThrough(ackID, through)
}

// TestAnswerCheckNeedsAnAdapterABoardAndALeadSession: without a live
// adapter, without a board, or on an agent conversation, the check sends
// nothing and writes nothing.
func TestAnswerCheckNeedsAnAdapterABoardAndALeadSession(t *testing.T) {
	setups := []struct {
		name  string
		setup func(f *ackFixture) string
	}{
		{"no adapter", func(f *ackFixture) string {
			f.lead(ackID, "", time.Now(), wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}})
			return ackID
		}},
		{"stale adapter", func(f *ackFixture) string {
			f.lead(ackID, "claude", time.Now().Add(-time.Hour), wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}})
			return ackID
		}},
		{"no snapshot", func(f *ackFixture) string {
			const id = "nosnapshot"
			register(t, f.dir, id, f.project, time.Now())
			f.src.setPage(id, PageContent{HasBoard: true, Seq: 1})
			f.d.Index().Refresh()
			f.waitIndexed(id)
			return id
		}},
		{"no board", func(f *ackFixture) string {
			const id = "noboard"
			register(t, f.dir, id, f.project, time.Now())
			f.src.setPage(id, PageContent{HasBoard: false, Seq: 1})
			f.idle(id)
			f.d.Index().Refresh()
			f.waitIndexed(id)
			return id
		}},
		{"agent conversation", func(f *ackFixture) string {
			const id = "agentconvo"
			now := time.Now()
			reg := Registration{ReplyID: id, Project: f.project, Harness: "claude", RegisteredAt: stampFor(now), HeartbeatAt: stampFor(now), Agent: true}
			if err := WriteRegistration(f.dir, reg); err != nil {
				t.Fatal(err)
			}
			f.idle(id)
			f.d.Index().Refresh()
			waitFor(t, "the agent conversation to be indexed", func() bool { s, ok := f.d.Index().Session(id); return ok && s.Agent })
			return id
		}},
	}
	for _, c := range setups {
		t.Run(c.name, func(t *testing.T) {
			f := startAckFixture(t)
			id := c.setup(f)
			f.src.setOpen(id, 7)
			through, _ := f.batch(id, 7, "")
			f.d.checkAnswers(id)
			f.wantNone(id)
			f.wantThrough(id, through)
			if n := f.src.boardReadCount(id); n != 0 {
				t.Fatalf("board reads = %d, want none", n)
			}
		})
	}
}

// TestAnswerCheckSendsNothingMoreAfterARestart: a daemon started over the
// same directory after a nudge finds the batch spent.
func TestAnswerCheckSendsNothingMoreAfterARestart(t *testing.T) {
	f := startAckFixture(t)
	f.src.setOpen(ackID, 7)
	_, start := f.batch(ackID, 7, "")
	f.d.checkAnswers(ackID)
	f.wantNudge(ackID, "(07)")
	f.drain(ackID)
	f.d.Close()
	waitDone(t, f.d)

	// The restarted daemon's first refresh re-checks every session on its
	// own timer; the direct check below makes the outcome certain.
	f.start(10 * time.Millisecond)
	f.d.checkAnswers(ackID)
	f.wantNone(ackID)
	f.wantThrough(ackID, start)
}

// TestAnswerCheckConcurrentEvaluationsSendOneNudge: two checks of the same
// settled batch running at once hold exactly one nudge (run under -race).
func TestAnswerCheckConcurrentEvaluationsSendOneNudge(t *testing.T) {
	f := startAckFixture(t)
	ids := []string{ackID}
	for i := 1; i < 8; i++ {
		id := fmt.Sprintf("ackconcurrent%d", i)
		f.addSession(id)
		ids = append(ids, id)
	}
	starts := map[string]int64{}
	for _, id := range ids {
		f.src.setOpen(id, 7)
		_, starts[id] = f.batch(id, 7, "")
	}
	gate := make(chan struct{})
	var wg sync.WaitGroup
	for _, id := range ids {
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-gate
				f.d.checkAnswers(id)
			}()
		}
	}
	close(gate)
	wg.Wait()
	for _, id := range ids {
		f.wantNudge(id, "(07)")
		f.wantThrough(id, starts[id])
	}
}

// TestAnswerCheckRunsOnTheDaemonsTriggers drives the check through its
// timer: the daemon's first index refresh re-checks a session already
// settled on disk, and a session change re-checks the next batch.
func TestAnswerCheckRunsOnTheDaemonsTriggers(t *testing.T) {
	f := newAckFixture(t)
	f.src.setOpen(ackID, 7, 8)
	_, start := f.batch(ackID, 7, "")
	f.start(20 * time.Millisecond)
	waitFor(t, "the restart check's nudge", func() bool { return len(f.held(ackID)) > 0 })
	f.wantNudge(ackID, "(07)")
	f.wantThrough(ackID, start)

	// The nudge turn runs and settles; a new answer is delivered, and the
	// next turn settles. A session change (the board's fingerprint) pokes the
	// check.
	f.drain(ackID)
	f.turn(ackID, 1, "")
	sent := time.Now()
	f.deliver(ackID, sent, wsinbox.Chip{ID: "08", Answer: "ok"})
	waitFor(t, "the clock to pass the answer", func() bool { return time.Now().UnixMilli() > sent.UnixMilli()+2 })
	next := f.turn(ackID, 1, "")
	f.src.setPage(ackID, PageContent{HasBoard: true, Seq: 2})
	if !f.d.Index().Nudge(ackID, f.project) {
		t.Fatal("the index ignored the session's nudge")
	}
	waitFor(t, "the session change's nudge", func() bool { return len(f.held(ackID)) > 0 })
	f.wantNudge(ackID, "(08)")
	f.wantThrough(ackID, next)
}
