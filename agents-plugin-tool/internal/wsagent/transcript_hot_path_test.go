package wsagent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// The atomic writer's one CreateTemp per replacement, with snapshot I/O stubbed
// out below, counts agent.json replacements without timing/mtime assumptions.
func recordAtomicWrites() int64 {
	var n int64
	for site, v := range wsstate.FileIOCounters() {
		if strings.Contains(site, "wsagent/record.go:") && strings.HasSuffix(site, ":create_temp") {
			n += v
		}
	}
	return n
}

func TestTranscriptStreamingOnlyPersistsRingLineCursor(t *testing.T) {
	now := time.Unix(1000, 0)
	dir := filepath.Join(t.TempDir(), "stream")
	r := NewRunner(Options{Dir: dir, Now: func() time.Time { return now }, Log: io.Discard})
	c := &conv{r: r, dir: dir, rec: Record{ReplyID: "stream", WSKey: "key", Root: dir}, turn: true}
	c.live = newLiveWriter(dir, "key", "pi", r.opts.Now, io.Discard)
	slot := &idleSnapshotSlot{}
	c.live.slot = slot
	c.live.wake = slot.RingAppended
	c.live.writeSnapshotFile = func(string, []byte, os.FileMode) error { return nil }
	c.update(func(*Record) {})
	c.live.apply(wslive.Event{Kind: wslive.KindTurnStart})
	before := recordAtomicWrites()
	for i := uint64(1); i <= 200; i++ {
		c.handle(Event{Seq: i, Live: &wslive.Event{Kind: wslive.KindReply, Text: fmt.Sprintf("delta %d", i)}})
		c.handle(Event{Seq: i, LineEnd: true})
	}
	if n := recordAtomicWrites() - before; n != 0 {
		t.Fatalf("200 snapshot-only lines made %d record replacements, want 0", n)
	}
	if rec, err := readRecord(dir); err != nil || rec.HolderSeq != 0 || !c.seqDirty {
		t.Fatalf("snapshot-only cursor should wait: %+v, %v", rec, err)
	}
	c.handle(Event{Seq: 201, Live: &wslive.Event{Kind: wslive.KindReply, Text: "finished", Final: true}})
	c.handle(Event{Seq: 201, LineEnd: true})
	if n := recordAtomicWrites() - before; n != 1 {
		t.Fatalf("a ring line made %d record replacements, want 1", n)
	}
	if rec, err := readRecord(dir); err != nil || rec.HolderSeq != 201 || slot.rings != 1 {
		t.Fatalf("ring append must fence replay immediately: %+v, %v, wakes=%d", rec, err, slot.rings)
	}
	c.handle(Event{Seq: 202, Live: &wslive.Event{Kind: wslive.KindReply, Text: "next progress"}})
	c.handle(Event{Seq: 202, LineEnd: true})
	c.persistSeq() // heartbeat boundary
	if rec, err := readRecord(dir); err != nil || rec.HolderSeq != 202 {
		t.Fatalf("heartbeat did not save snapshot-only cursor: %+v, %v", rec, err)
	}
}

func TestDeferredChildSendFlushesInLetGoWithoutAProcess(t *testing.T) {
	now := time.Unix(1000, 0)
	dir := filepath.Join(t.TempDir(), "child")
	r := NewRunner(Options{Dir: dir, Now: func() time.Time { return now }, Log: io.Discard})
	c := &conv{r: r, dir: dir, inbox: wsinbox.Dir(dir), inboxC: make(chan struct{}, 1),
		rec: Record{ReplyID: "child", WSKey: "key", Root: dir, Backend: BackendClaude,
			Parent: &ParentRef{Kind: ParentLead, ReplyID: "parent"}, LastActiveAt: "old"}}
	r.convs["child"] = c
	c.live = newLiveWriter(dir, "key", "claude", r.opts.Now, io.Discard)
	c.update(func(*Record) {})
	before := recordAtomicWrites()
	if _, err := r.Send("child", "durable inbox text"); err != nil {
		t.Fatal(err)
	}
	if n := recordAtomicWrites() - before; n != 0 {
		t.Fatalf("send made %d record replacements, want 0", n)
	}
	if rec, err := readRecord(dir); err != nil || rec.Sends != 0 || rec.LastActiveAt != "old" {
		t.Fatalf("send was not deferred: %+v, %v", rec, err)
	}
	c.letGo()
	if rec, err := readRecord(dir); err != nil || rec.Sends != 1 || rec.LastActiveAt != now.UTC().Format(time.RFC3339) {
		t.Fatalf("letGo lost deferred child state with no process: %+v, %v", rec, err)
	}
	if n := recordAtomicWrites() - before; n != 1 {
		t.Fatalf("shutdown made %d record replacements, want 1", n)
	}
}

type transcriptSendProbe struct {
	Process
	send func(string) error
}

func (p transcriptSendProbe) Send(text string) error { return p.send(text) }

func TestTranscriptKickPersistsBothSidesOfSendImmediately(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kick")
	r := NewRunner(Options{Dir: dir, Log: io.Discard})
	c := &conv{r: r, dir: dir, inbox: wsinbox.Dir(dir), sends: 1,
		rec: Record{ReplyID: "kick", WSKey: "key", Root: dir, Sends: 1}}
	c.live = newLiveWriter(dir, "key", "pi", r.opts.Now, io.Discard)
	if _, _, err := wsinbox.Send(c.inbox, wsinbox.Entry{Parts: []string{"queued input"}}, r.opts.Now()); err != nil {
		t.Fatal(err)
	}
	c.proc = transcriptSendProbe{send: func(text string) error {
		rec, err := readRecord(dir)
		if err != nil || len(rec.Queued) != 1 || rec.Queued[0].Text != text {
			t.Fatalf("consumed input was not durable before Send: %+v, %v", rec, err)
		}
		return nil
	}}
	c.kick()
	if rec, err := readRecord(dir); err != nil || len(rec.Queued) != 0 || rec.Drained != 1 || rec.TurnMark != 1 {
		t.Fatalf("successful Send did not immediately clear replayable input: %+v, %v", rec, err)
	}
}

func TestExpiredChildSnapshotCloseDoesNotRecreateSessionDirectory(t *testing.T) {
	h, _ := agentHarness(t, func(o *Options) { o.heartbeat = time.Hour })
	id, p := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "one", Tier: "small", Authority: "read-only"})
	p.reply("done")
	eventually(t, "settled child", func() bool { return h.state(id).Status == StatusIdle })
	h.asleep(id)
	c := h.runner.conv(id)
	if !h.runner.expire(id) {
		t.Fatal("waiting child did not expire")
	}
	wait(t, "expired child's loop exit", c.done)
	// Exercise the worst ordering deterministically: the deferred slot close
	// runs after forget removed the directory, not just before it.
	c.live.closeSlot()
	if _, err := os.Stat(c.dir); !os.IsNotExist(err) {
		t.Fatalf("expired child's directory was recreated: %v", err)
	}
}

func TestRefusedWakePersistsSendCoverageBeforeCrashRecovery(t *testing.T) {
	clk := newTestClock()
	h, _ := agentHarness(t, func(o *Options) {
		o.Now, o.heartbeat, o.ChildWaitingDays = clk.Now, time.Hour, 2
	})
	parent, child := settledPair(t, h)
	if err := writePermissionSelection(wsdashboard.SessionDir(h.dir, parent), PermissionReadOnly); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Deliver(child, Delivery{Text: "must survive a refused wake"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "refused wake", func() bool {
		return strings.Contains(h.state(child).Error, "parent's current level read-only")
	})
	c := h.runner.conv(child)
	if !c.do(func() {}) {
		t.Fatal("child loop ended")
	}
	// Capture the actual recovery record BEFORE any graceful Close can flush
	// it. A future startup must retain the input even past its activity expiry.
	rec := h.record(child)
	if rec.LastTurn == nil || rec.Sends <= rec.LastTurn.Mark || len(rec.Queued) != 0 {
		t.Fatalf("refused wake did not persist undrained send coverage: %+v", rec)
	}
	clk.advance(3 * day)
	checker := NewRunner(Options{Now: clk.Now, ChildWaitingDays: 2})
	if expired := checker.expiredAtStart([]Record{rec}); len(expired) != 0 {
		t.Fatalf("crash recovery would delete unread input with child %v", expired)
	}
	held, err := wsinbox.Held(c.inbox)
	if err != nil || len(held) != 1 || !strings.Contains(held[0].Entry.Text(), "must survive a refused wake") {
		t.Fatalf("refused wake lost unread inbox text: %+v, %v", held, err)
	}
}

func TestHostedTranscriptUsesSlotNotFullNotify(t *testing.T) {
	h := newLeadHarness(t)
	h.opts.LiveHost = &summarySnapshotHost{}
	h.opts.heartbeat = time.Hour
	h.start()
	id := h.lead(BackendPi, LeadOptions{}).ID
	h.send(id, "one")
	c := h.runner.conv(id)
	h.quiesce(id)
	before := h.notes.Load()
	c.do(func() {
		c.apply(wslive.Event{Kind: wslive.KindReply, Text: "stream"})
		c.apply(wslive.Event{Kind: wslive.KindReply, Text: "finished", Final: true})
		// Summary commits use this same callback, including off-loop retries.
		c.live.wake()
	})
	if n := h.notes.Load() - before; n != 0 {
		t.Fatalf("slotted transcript issued %d full directory-refresh notifications", n)
	}
}

func TestTranscriptShutdownFlushesDeferredSendsAndConsumedSnapshot(t *testing.T) {
	for _, backend := range []string{BackendClaude, BackendPi} {
		for _, consumed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/consumed=%v", backend, consumed), func(t *testing.T) {
				h := newLeadHarness(t)
				host := &summarySnapshotHost{}
				h.opts.LiveHost = host
				h.opts.heartbeat = time.Hour
				h.start()
				id := h.lead(backend, LeadOptions{}).ID
				h.send(id, "one")
				h.quiesce(id)
				c := h.runner.conv(id)
				diskBefore := h.record(id)
				if _, err := h.runner.Send(id, "held through shutdown"); err != nil {
					t.Fatal(err)
				}
				h.quiesce(id)
				// No turn boundary has occurred, so only RAM holds Sends=2.
				if rec := h.record(id); rec.Sends != diskBefore.Sends {
					t.Fatalf("held send was not deferred: %d -> %d", diskBefore.Sends, rec.Sends)
				}
				wantSeq := diskBefore.HolderSeq
				dirtySeq, dirtyRecord := false, false
				c.do(func() {
					if consumed {
						c.handle(Event{Seq: 100, Live: &wslive.Event{Kind: wslive.KindReply, Text: "consumed prefix"}})
						c.handle(Event{Seq: 100, LineEnd: true})
						wantSeq = 100
					} else {
						// With no dirty cursor, persistSeq cannot incidentally save
						// Sends: only letGo's explicit flushRecord can do so.
						c.wmu.Lock()
						dirtyRecord = c.recordDirty
						c.wmu.Unlock()
						dirtySeq = c.seqDirty
					}
				})
				if !consumed && (dirtySeq || !dirtyRecord) {
					t.Fatal("test requires dirty send metadata without a dirty cursor")
				}
				h.runner.Close() // same Runner.Close used by SIGTERM, /shutdown, --replace
				rec := h.record(id)
				if rec.Sends != 2 || rec.HolderSeq != wantSeq || rec.TurnMark != 1 || len(rec.Queued) != 0 {
					t.Fatalf("shutdown lost send coverage/cursor or deferred kick: %+v", rec)
				}
				dir := wsdashboard.SessionDir(h.dir, id)
				snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(dir))
				if err != nil || (consumed && (snap.Lead.Reply == nil || snap.Lead.Reply.Text != "consumed prefix")) {
					t.Fatalf("shutdown lost consumed snapshot: %+v, %v", snap, err)
				}
				if _, ok := host.snapshot(dir); ok {
					t.Fatal("shutdown retained the RAM slot after flushing")
				}
				held, err := wsinbox.Held(wsinbox.Dir(dir))
				if err != nil || len(held) != 1 || !strings.Contains(held[0].Entry.Text(), "held through shutdown") {
					t.Fatalf("shutdown lost the held input: %+v, %v", held, err)
				}
			})
		}
	}
}

func ringSiteOpCount(op string) int64 {
	var n int64
	for site, count := range wsstate.FileIOCounters() {
		if strings.HasPrefix(site, "file_io_site:wslive/ring") && strings.HasSuffix(site, ":"+op) {
			n += count
		}
	}
	return n
}

func TestNonLeadChildUsesHostedSlotAndRAMHeartbeat(t *testing.T) {
	host := &summarySnapshotHost{}
	clk := newTestClock()
	h, _ := agentHarness(t, func(o *Options) {
		o.LiveHost = host
		o.Now = clk.Now
		o.heartbeat = time.Hour
	})
	id, p := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "work", Tier: "small", Authority: "read-only"})
	p.reply("child done")
	eventually(t, "settled child", func() bool { return h.state(id).Status == StatusIdle })
	c := h.runner.conv(id)
	dir := wsdashboard.SessionDir(h.dir, id)
	if _, ok := host.snapshot(dir); !ok {
		t.Fatal("non-lead child did not open a hosted RAM slot")
	}

	var diskWrites int
	c.do(func() {
		c.live.writeSnapshotFile = func(path string, raw []byte, mode os.FileMode) error {
			diskWrites++
			return writeFileAtomic(path, raw, mode)
		}
	})
	var notes atomic.Int64
	origNotify := h.runner.notify
	h.runner.notify = func(replyID, project string) {
		notes.Add(1)
		origNotify(replyID, project)
	}
	for range wslive.StaleAfterBeats + 1 {
		clk.advance(wslive.HeartbeatInterval)
		c.do(func() { c.live.heartbeatSnapshot() })
	}
	if diskWrites != 0 {
		t.Fatalf("idle child heartbeats made %d disk snapshot writes, want 0", diskWrites)
	}
	if snap, ok := host.snapshot(dir); !ok || snap.Stale(clk.Now()) {
		t.Fatalf("child RAM slot went stale: %+v", snap)
	}
	// Streaming and ring appends route through the slot, not full notify.
	c.do(func() {
		c.apply(wslive.Event{Kind: wslive.KindReply, Text: "streaming"})
		c.apply(wslive.Event{Kind: wslive.KindReply, Text: "final child reply", Final: true})
	})
	if n := notes.Load(); n != 0 {
		t.Fatalf("slotted child made %d full notify calls, want 0", n)
	}
	// Graceful shutdown flushes final snapshot and closes the slot.
	h.runner.Close()
	if _, ok := host.snapshot(dir); ok {
		t.Fatal("closed child retained its RAM slot")
	}
	if ring, err := wslive.ReadRing(wslive.RingDir(dir)); err != nil || len(ring) == 0 || ring[len(ring)-1].Text != "final child reply" {
		t.Fatalf("child ring after graceful shutdown = %+v, %v", ring, err)
	}
}

func TestHostedLeadPhase2HotPathZeroFenceReadsAndZeroRingReadDir(t *testing.T) {
	h := newLeadHarness(t)
	host := &summarySnapshotHost{}
	h.opts.LiveHost = host
	h.opts.heartbeat = time.Hour
	closeRings := wslive.EnableRingCache(filepath.Join(h.dir, "sessions"))
	defer closeRings()
	h.start()

	id := h.lead(BackendPi, LeadOptions{}).ID
	h.send(id, "first")
	h.quiesce(id)
	c := h.runner.conv(id)
	// Warm the ring writer with one entry before measuring steady-state hot path.
	c.do(func() {
		c.apply(wslive.Event{Kind: wslive.KindReply, Text: "warm", Final: true})
	})

	regReadsBefore := registrationReads()
	ringReadDirBefore := ringSiteOpCount("read_dir")
	ringTempBefore := ringSiteOpCount("create_temp")
	ringRenameBefore := ringSiteOpCount("atomic_replace")
	ringOpenBefore := ringSiteOpCount("open_file")

	c.do(func() {
		for i := range 25 {
			c.apply(wslive.Event{Kind: wslive.KindReply, Text: fmt.Sprintf("delta %d", i)})
		}
		c.apply(wslive.Event{Kind: wslive.KindReply, Text: "steady reply", Final: true})
		c.live.heartbeatSnapshot()
	})

	if got := registrationReads() - regReadsBefore; got != 0 {
		t.Fatalf("steady-state lead hot path made %d registration reads, want 0", got)
	}
	if got := ringSiteOpCount("read_dir") - ringReadDirBefore; got != 0 {
		t.Fatalf("steady-state ring append made %d read_dir calls, want 0", got)
	}
	if got := ringSiteOpCount("create_temp") - ringTempBefore; got != 0 {
		t.Fatalf("steady-state ring append made %d create_temp calls, want 0", got)
	}
	if got := ringSiteOpCount("atomic_replace") - ringRenameBefore; got != 0 {
		t.Fatalf("steady-state ring append made %d atomic_replace calls, want 0", got)
	}
	if got := ringSiteOpCount("open_file") - ringOpenBefore; got != 0 {
		t.Fatalf("steady-state ring append made %d open_file calls, want 0", got)
	}

	// Graceful shutdown closes handles cleanly and preserves every ring entry.
	h.runner.Close()
	if err := closeRings(); err != nil {
		t.Fatal(err)
	}
	dir := wsdashboard.SessionDir(h.dir, id)
	ring, err := wslive.ReadRing(wslive.RingDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, rec := range ring {
		if rec.Text == "steady reply" {
			found = true
		}
	}
	if !found {
		t.Fatalf("graceful shutdown lost steady reply from ring: %+v", ring)
	}
}
