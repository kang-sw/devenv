package wsagent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
		t.Run(backend, func(t *testing.T) {
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
			c.do(func() {
				c.handle(Event{Seq: 100, Live: &wslive.Event{Kind: wslive.KindReply, Text: "consumed prefix"}})
				c.handle(Event{Seq: 100, LineEnd: true})
			})
			h.runner.Close() // same Runner.Close used by SIGTERM, /shutdown, --replace
			rec := h.record(id)
			if rec.Sends != 2 || rec.HolderSeq != 100 || rec.TurnMark != 1 || len(rec.Queued) != 0 {
				t.Fatalf("shutdown lost send coverage/cursor or deferred kick: %+v", rec)
			}
			dir := wsdashboard.SessionDir(h.dir, id)
			snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(dir))
			if err != nil || snap.Lead.Reply == nil || snap.Lead.Reply.Text != "consumed prefix" {
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
