package wsmailbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// memFixture is one cache root (and config home) for the memory store.
type memFixture struct {
	t     *testing.T
	paths Paths
	cache string // canonical cache root
	ring  string
	named string // a worktree-scope store path under the cache root
	reply string
}

func newMemFixture(t *testing.T) *memFixture {
	t.Helper()
	p := Paths{Config: wsconfig.Options{ConfigHome: t.TempDir()}, State: wsstate.Options{CacheHome: t.TempDir()}}
	s := p.Store()
	if s.err != nil {
		t.Fatal(s.err)
	}
	cache, err := wsstate.CacheRoot(p.State)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := p.ReplyRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ClosePersistent() })
	return &memFixture{t: t, paths: p, cache: cache, ring: filepath.Join(cache, RingDirName),
		named: filepath.Join(cache, "proj", "fixture", storeFileName), reply: reply}
}

// open opens the persistent instance (a daemon start).
func (f *memFixture) open() *Persistent {
	f.t.Helper()
	p, err := OpenPersistent(f.paths)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

// kill drops a persistent instance the way a killed daemon does: the lock
// and descriptor go, nothing is exported.
func (f *memFixture) kill(p *Persistent) {
	f.t.Helper()
	in := p.in
	registryMu.Lock()
	delete(registry, in.ringDir)
	registryMu.Unlock()
	in.mu.Lock()
	in.closed = true
	_ = in.release()
	in.mu.Unlock()
}

func (f *memFixture) send(name, content string) {
	f.t.Helper()
	if err := f.paths.Store().WithLock(f.named, func(s *StoreFile) error {
		s.Queues = AppendQueue(s.Queues, name, Envelope{From: "peer@worktree", Content: content, SentAt: "2026-11-17T00:00:00Z"})
		return nil
	}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *memFixture) drain(name string) []Envelope {
	f.t.Helper()
	var got []Envelope
	if err := f.paths.Store().WithLock(f.named, func(s *StoreFile) error {
		got = s.Queues[name]
		delete(s.Queues, name)
		return nil
	}); err != nil {
		f.t.Fatal(err)
	}
	return got
}

func (f *memFixture) queue(name string) []Envelope {
	f.t.Helper()
	_, _, q, err := f.paths.Store().NamedQueue(f.named, name)
	if err != nil {
		f.t.Fatal(err)
	}
	return q
}

func contents(q []Envelope) string {
	out := make([]string, len(q))
	for i, e := range q {
		out[i] = e.Content
	}
	return strings.Join(out, ",")
}

func (f *memFixture) segments() []string {
	f.t.Helper()
	entries, err := os.ReadDir(f.ring)
	if err != nil {
		f.t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if segmentNamePattern.MatchString(e.Name()) {
			out = append(out, filepath.Join(f.ring, e.Name()))
		}
	}
	return out
}

func captureRingWarnings(t *testing.T) func() []string {
	t.Helper()
	var (
		mu  sync.Mutex
		got []string
	)
	prev := ringWarn
	ringWarn = func(format string, args ...any) {
		mu.Lock()
		got = append(got, fmt.Sprintf(format, args...))
		mu.Unlock()
	}
	t.Cleanup(func() { ringWarn = prev })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), got...)
	}
}

func TestMemStoreSurvivesAKilledDaemonWithoutLegacyFiles(t *testing.T) {
	f := newMemFixture(t)
	d := f.open()
	f.send("lead", "one")
	f.send("lead", "two")
	if err := f.paths.WriteListeningMarker(ListeningMarker{SessionKey: "amber-tide-fox", PID: 42}); err != nil {
		t.Fatal(err)
	}
	f.kill(d)
	if _, err := os.Stat(f.named); !os.IsNotExist(err) {
		t.Fatalf("a steady-state send wrote the legacy store file: %v", err)
	}

	f.open()
	if got := contents(f.queue("lead")); got != "one,two" {
		t.Fatalf("queue after the restart = %q, want one,two", got)
	}
	if m, ok, err := f.paths.ReadListeningMarker("amber-tide-fox"); err != nil || !ok || m.PID != 42 {
		t.Fatalf("listening marker after the restart = %+v %v %v", m, ok, err)
	}
}

func TestMemStoreNeverRedeliversAcknowledgedMail(t *testing.T) {
	f := newMemFixture(t)
	// An upgrade backlog: the previous version left mail in the file.
	if err := WithFileLock(f.named, func(s *StoreFile) error {
		s.Queues = AppendQueue(s.Queues, "lead", Envelope{Content: "backlog"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	d := f.open()
	f.send("lead", "fresh")
	if got := contents(f.drain("lead")); got != "backlog,fresh" {
		t.Fatalf("drain = %q, want backlog,fresh", got)
	}
	f.kill(d)

	f.open()
	if got := f.queue("lead"); len(got) != 0 {
		t.Fatalf("acknowledged mail redelivered after a crash: %+v", got)
	}
	file, err := LoadFile(f.named)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Queues["lead"]) != 0 {
		t.Fatalf("the imported backlog stayed in the legacy file: %+v", file.Queues)
	}
}

func TestMemStoreCutsATornRecord(t *testing.T) {
	f := newMemFixture(t)
	warnings := captureRingWarnings(t)
	d := f.open()
	f.send("lead", "kept")
	f.kill(d)
	segs := f.segments()
	newest := segs[len(segs)-1]
	before, err := os.ReadFile(newest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newest, append(append([]byte(nil), before...), `{"kind":"tx","op":"send","named":{"x`...), 0o644); err != nil {
		t.Fatal(err)
	}

	f.open()
	if got := contents(f.queue("lead")); got != "kept" {
		t.Fatalf("queue after a torn record = %q, want kept", got)
	}
	after, _ := os.ReadFile(newest)
	if string(after) != string(before) {
		t.Fatalf("the torn record was not cut: %d bytes, want %d", len(after), len(before))
	}
	if w := strings.Join(warnings(), "\n"); !strings.Contains(w, "torn") {
		t.Fatalf("warnings = %q, want the torn record reported", w)
	}
	f.send("lead", "next")
	if got := contents(f.queue("lead")); got != "kept,next" {
		t.Fatalf("queue after appending past the cut = %q", got)
	}
}

func TestMemStoreRotationCarriesLiveStateAndReapsIdleState(t *testing.T) {
	f := newMemFixture(t)
	warnings := captureRingWarnings(t)
	prev := segmentBytes
	segmentBytes = 4096
	t.Cleanup(func() { segmentBytes = prev })

	f.open()
	// A quiet session: one mail nobody drains, written before every
	// rotation.
	f.send("quiet", "still here")
	now := storeNow()
	old := now.Add(-HistoryRetention - time.Hour)
	store := f.paths.Store()
	if err := store.Update("seed", func(tx *Tx) error {
		if _, err := tx.AppendHistory("idle-session", old, HistoryEntry{Direction: HistoryRecv, Counterparty: "x", Body: "old"}); err != nil {
			return err
		}
		reply := tx.Reply()
		reply.Entries["expired"] = ReplyEntry{LastSeen: old.Format(time.RFC3339)}
		reply.Entries["recent"] = ReplyEntry{LastSeen: now.Format(time.RFC3339)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 400; i++ {
		f.send("busy", fmt.Sprintf("m%03d", i))
		f.drain("busy")
	}
	if segs := f.segments(); len(segs) != SegmentCount {
		t.Fatalf("segments = %d, want the cap %d", len(segs), SegmentCount)
	}
	if got := contents(f.queue("quiet")); got != "still here" {
		t.Fatalf("quiet session's mail after rotations = %q", got)
	}
	if h, _ := store.History("idle-session"); len(h) != 0 {
		t.Fatalf("an idle history window survived rotation: %+v", h)
	}
	reply, _ := store.LoadReply()
	if _, ok := reply.Entries["expired"]; ok {
		t.Fatal("an expired reply entry survived rotation")
	}
	if _, ok := reply.Entries["recent"]; !ok {
		t.Fatal("a recent reply entry was reaped")
	}

	// Soft cap: live state larger than a segment is kept whole.
	big := strings.Repeat("x", int(segmentBytes))
	f.send("big", big)
	for i := 0; i < 5; i++ {
		f.send("busy", strings.Repeat("y", 1024))
		f.drain("busy")
	}
	if got := f.queue("big"); len(got) != 1 || got[0].Content != big {
		t.Fatal("live state larger than a segment was dropped")
	}
	if w := strings.Join(warnings(), "\n"); !strings.Contains(w, "outgrows a ring segment") {
		t.Fatalf("warnings = %q, want the oversize state reported", w)
	}
	// And a fresh open restores everything from the newest segment alone.
	if err := ClosePersistent(); err != nil {
		t.Fatal(err)
	}
	if got := contents(f.queue("quiet")); got != "still here" {
		t.Fatalf("quiet mail after reopen = %q", got)
	}
}

func TestMemStoreImportsTheLegacyFilesOnUpgrade(t *testing.T) {
	f := newMemFixture(t)
	warnings := captureRingWarnings(t)
	if err := WithFileLock(f.named, func(s *StoreFile) error {
		s.Presence["lead"] = Presence{Name: "lead", Scope: ScopeWorktree, Owner: "amber-tide-fox", LastSeen: "2026-11-17T00:00:00Z"}
		s.Queues = AppendQueue(s.Queues, "lead", Envelope{Content: "named backlog"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := WithReplyFileLock(f.reply, func(s *ReplyStore) error {
		s.Entries["amber"] = ReplyEntry{LastSeen: storeNow().Format(time.RFC3339)}
		s.Queues = AppendQueue(s.Queues, "amber", Envelope{Content: "reply backlog"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(f.cache, HistoryDirName, "amber-tide-fox")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	good, _ := newHistoryName(storeNow().Add(-time.Minute))
	if err := writeLegacyHistoryEntry(dir, HistoryEntry{Name: good, Direction: HistorySent, Counterparty: "lead@worktree", Body: "hello"}); err != nil {
		t.Fatal(err)
	}
	bad, _ := newHistoryName(storeNow())
	if err := os.WriteFile(filepath.Join(dir, bad), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The previous version's listening marker files are not read.
	if err := os.MkdirAll(filepath.Join(f.cache, "mailbox-listening"), 0o755); err != nil {
		t.Fatal(err)
	}

	f.open()
	store := f.paths.Store()
	p, ok, q, err := store.NamedQueue(f.named, "lead")
	if err != nil || !ok || p.Owner != "amber-tide-fox" || contents(q) != "named backlog" {
		t.Fatalf("named import = %+v %v %+v %v", p, ok, q, err)
	}
	if q, _ := store.ReplyQueue("amber"); contents(q) != "reply backlog" {
		t.Fatalf("reply import = %+v", q)
	}
	h, _ := store.History("amber-tide-fox")
	if len(h) != 1 || h[0].Name != good || h[0].Body != "hello" {
		t.Fatalf("history import = %+v, want the good entry alone", h)
	}
	file, _ := LoadFile(f.named)
	if len(file.Queues) != 0 || file.ExportMark == "" || file.Presence["lead"].Owner != "amber-tide-fox" {
		t.Fatalf("the named file after import = %+v, want its queues cleared and the presence kept", file)
	}
	replyFile, _ := LoadReplyFile(f.reply)
	if len(replyFile.Queues) != 0 || replyFile.ExportMark == "" {
		t.Fatalf("the reply file after import = %+v", replyFile)
	}
	if w := strings.Join(warnings(), "\n"); w != "" {
		t.Fatalf("an unreadable history entry is skipped quietly; warnings = %q", w)
	}
}

func TestMemStoreSkipsAnUnreadableLegacyStore(t *testing.T) {
	f := newMemFixture(t)
	warnings := captureRingWarnings(t)
	if err := os.MkdirAll(filepath.Dir(f.named), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.named, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.open()
	f.send("lead", "still works")
	if got := contents(f.queue("lead")); got != "still works" {
		t.Fatalf("queue beside an unreadable legacy file = %q", got)
	}
	if w := strings.Join(warnings(), "\n"); !strings.Contains(w, "parse mailbox store") {
		t.Fatalf("warnings = %q, want the unreadable file reported", w)
	}
}

func TestMemStoreExportsForADowngradeAndReimportsWhatTheOldDaemonDid(t *testing.T) {
	f := newMemFixture(t)
	f.open()
	f.send("lead", "one")
	f.send("lead", "two")
	store := f.paths.Store()
	if err := store.Update("history", func(tx *Tx) error {
		_, err := tx.AppendHistory("amber-tide-fox", storeNow(), HistoryEntry{Direction: HistorySent, Counterparty: "lead@worktree", Body: "one"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.PostSystem("amber", "", "report", storeNow()); err != nil {
		t.Fatal(err)
	}
	if err := ClosePersistent(); err != nil {
		t.Fatal(err)
	}

	// (i) The previous version reads the exported files.
	file, err := LoadFile(f.named)
	if err != nil || contents(file.Queues["lead"]) != "one,two" {
		t.Fatalf("exported named file = %+v %v", file, err)
	}
	replyFile, err := LoadReplyFile(f.reply)
	if err != nil || contents(replyFile.Queues["amber"]) != "report" {
		t.Fatalf("exported reply file = %+v %v", replyFile, err)
	}
	names, err := legacyHistoryNames(filepath.Join(f.cache, HistoryDirName, "amber-tide-fox"))
	if err != nil || len(names) != 1 {
		t.Fatalf("exported history = %v %v", names, err)
	}

	// (ii) The previous version runs: it drains "one", queues "three", and
	// rewrites the file whole (dropping the export mark it does not know).
	file.ExportMark = ""
	file.Queues["lead"] = []Envelope{file.Queues["lead"][1], {Content: "three"}}
	rewritten, _ := json.Marshal(file)
	if err := os.WriteFile(f.named, rewritten, 0o644); err != nil {
		t.Fatal(err)
	}

	// It also drains the reply queue, queues a new reply, and registers a
	// presence.
	replyFile.ExportMark = ""
	replyFile.Queues["amber"] = []Envelope{{Content: "old reply"}}
	rewritten, _ = json.Marshal(replyFile)
	if err := os.WriteFile(f.reply, rewritten, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WithFileLock(f.named, func(s *StoreFile) error {
		s.Presence["newcomer"] = Presence{Name: "newcomer", Scope: ScopeWorktree, LastSeen: "2026-11-17T00:00:00Z"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	f.open()
	if got := contents(f.queue("lead")); got != "two,three" {
		t.Fatalf("queue after the downgrade round = %q, want two,three", got)
	}
	if q, _ := store.ReplyQueue("amber"); contents(q) != "old reply" {
		t.Fatalf("reply queue after the round = %+v, want the old daemon's reply alone", q)
	}
	if _, ok, _, _ := store.NamedQueue(f.named, "newcomer"); !ok {
		t.Fatal("the old daemon's presence was not imported")
	}
	// A second round stays consistent: export, reopen, nothing doubles.
	if err := ClosePersistent(); err != nil {
		t.Fatal(err)
	}
	f.open()
	if got := contents(f.queue("lead")); got != "two,three" {
		t.Fatalf("queue after a second round = %q", got)
	}
	if q, _ := store.ReplyQueue("amber"); contents(q) != "old reply" {
		t.Fatalf("reply queue after a second round = %+v", q)
	}
}

func TestMemStorePlainRestartImportsNothingTwice(t *testing.T) {
	f := newMemFixture(t)
	f.open()
	f.send("lead", "one")
	for i := 0; i < 3; i++ {
		if err := ClosePersistent(); err != nil {
			t.Fatal(err)
		}
		f.open()
	}
	if got := contents(f.queue("lead")); got != "one" {
		t.Fatalf("queue after restarts = %q, want one", got)
	}
	// A crash after an export: the next open takes the marked files as
	// its own baseline.
	if err := ClosePersistent(); err != nil {
		t.Fatal(err)
	}
	d := f.open()
	f.kill(d)
	f.open()
	if got := contents(f.queue("lead")); got != "one" {
		t.Fatalf("queue after an export then a crash = %q, want one", got)
	}
}

func TestMemStoreHistoryStampsRiseAcrossOpens(t *testing.T) {
	f := newMemFixture(t)
	at := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	var names []string
	for i := 0; i < 3; i++ {
		// Transient: each call opens and releases the ring.
		if err := f.paths.Store().Update("history", func(tx *Tx) error {
			n, err := tx.AppendHistory("amber-tide-fox", at, HistoryEntry{Direction: HistoryRecv, Body: fmt.Sprint(i)})
			names = append(names, n...)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if !(names[0] < names[1] && names[1] < names[2]) {
		t.Fatalf("history names do not rise across opens: %v", names)
	}
}

func TestMemStoreTransientReadsWhileAnotherProcessHoldsTheRing(t *testing.T) {
	f := newMemFixture(t)
	f.send("lead", "committed")
	if err := f.paths.WriteListeningMarker(ListeningMarker{SessionKey: "amber-tide-fox", PID: 7}); err != nil {
		t.Fatal(err)
	}
	holder := flock.New(filepath.Join(f.ring, ringLockName))
	if locked, err := holder.TryLock(); err != nil || !locked {
		t.Fatalf("hold the ring: %v %v", locked, err)
	}
	defer holder.Unlock() //nolint:errcheck

	if got := contents(f.queue("lead")); got != "committed" {
		t.Fatalf("read beside the holder = %q", got)
	}
	if m, ok, err := f.paths.ReadListeningMarker("amber-tide-fox"); err != nil || !ok || m.PID != 7 {
		t.Fatalf("marker beside the holder = %+v %v %v", m, ok, err)
	}
	err := f.paths.Store().WithLock(f.named, func(*StoreFile) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("a write beside the holder: err = %v, want the unsupported refusal", err)
	}
}

func TestMemStoreSecondPersistentOpenWaitsForThePredecessor(t *testing.T) {
	f := newMemFixture(t)
	prev := persistentLockWait
	persistentLockWait = 3 * time.Second
	t.Cleanup(func() { persistentLockWait = prev })
	holder := flock.New(filepath.Join(f.ring, ringLockName))
	if err := os.MkdirAll(f.ring, 0o755); err != nil {
		t.Fatal(err)
	}
	if locked, err := holder.TryLock(); err != nil || !locked {
		t.Fatalf("hold the ring: %v %v", locked, err)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = holder.Unlock()
	}()
	if _, err := OpenPersistent(f.paths); err != nil {
		t.Fatalf("a successor that outwaits its predecessor's release: %v", err)
	}
}

// TestMemStoreReadersSeeMemoryOnlyMailAndNeverTheStaleFiles pins Decision
// 5: the wait's peek and the hook peek read the store, so mail that exists
// only in memory (and the ring) is seen, and what a legacy file holds
// after the open is not.
func TestMemStoreReadersSeeMemoryOnlyMailAndNeverTheStaleFiles(t *testing.T) {
	f := newMemFixture(t)
	machine, err := f.paths.Machine()
	if err != nil {
		t.Fatal(err)
	}
	const key = "amber-tide-fox"
	f.open()
	if err := f.paths.Store().WithLock(machine, func(s *StoreFile) error {
		s.Presence["lead"] = Presence{Name: "lead", Scope: ScopeMachine, Owner: key, LastSeen: "2026-11-17T00:00:00Z"}
		s.Queues = AppendQueue(s.Queues, "lead", Envelope{Content: "named live"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.paths.Store().PostSystem(ReplyID(key), "", "reply live", storeNow()); err != nil {
		t.Fatal(err)
	}
	// Stale files: nothing the store holds, written behind its back.
	if err := WithFileLock(machine, func(s *StoreFile) error {
		s.Queues = AppendQueue(s.Queues, "lead", Envelope{Content: "stale"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := WithReplyFileLock(f.reply, func(s *ReplyStore) error {
		s.Queues = AppendQueue(s.Queues, ReplyID(key), Envelope{Content: "stale"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	got, err := Wait(WaitTarget{SessionKey: key, Slug: "lead@machine", Paths: f.paths}, WaitOptions{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if contents(got.Named) != "named live" || contents(got.Reply) != "reply live" {
		t.Fatalf("wait peek = named %q reply %q, want the live mail alone", contents(got.Named), contents(got.Reply))
	}
	notify, unread, err := f.paths.ShouldNotifyNamedInboxUnread("lead@machine", "")
	if err != nil || !notify || unread != 1 {
		t.Fatalf("hook peek = %v %d %v, want the one live message", notify, unread, err)
	}
}

// TestMemStoreHeldRingDoesNotStallOtherRootsOrRepeatTheWait pins that a
// persistent open waiting on a held ring blocks neither another cache
// root's calls nor, once it failed, later calls for its own root.
func TestMemStoreHeldRingDoesNotStallOtherRootsOrRepeatTheWait(t *testing.T) {
	held := newMemFixture(t)
	other := newMemFixture(t)
	prev := persistentLockWait
	persistentLockWait = time.Second
	t.Cleanup(func() { persistentLockWait = prev })
	other.open()
	if err := os.MkdirAll(held.ring, 0o755); err != nil {
		t.Fatal(err)
	}
	holder := flock.New(filepath.Join(held.ring, ringLockName))
	if locked, err := holder.TryLock(); err != nil || !locked {
		t.Fatalf("hold the ring: %v %v", locked, err)
	}
	defer holder.Unlock() //nolint:errcheck
	EnablePersistent()

	failed := make(chan error, 1)
	go func() {
		_, err := held.paths.Store().LoadReply()
		failed <- err
	}()
	time.Sleep(100 * time.Millisecond) // the open is waiting on the ring
	start := time.Now()
	other.send("lead", "not stalled")
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("another root's call took %v behind the held ring's open", d)
	}
	if err := <-failed; err == nil || !strings.Contains(err.Error(), "held by another process") {
		t.Fatalf("the held ring's open: %v", err)
	}
	start = time.Now()
	if _, err := held.paths.Store().LoadReply(); err == nil {
		t.Fatal("a call inside the retry window succeeded on a held ring")
	}
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Fatalf("a call inside the retry window waited %v again", d)
	}
}

// TestMemStoreExportRewritesHistoryAPruneRemoved pins that the export
// trusts its baseline only for history files still on disk.
func TestMemStoreExportRewritesHistoryAPruneRemoved(t *testing.T) {
	f := newMemFixture(t)
	f.open()
	if err := f.paths.Store().Update("history", func(tx *Tx) error {
		_, err := tx.AppendHistory("amber-tide-fox", storeNow(), HistoryEntry{Direction: HistorySent, Body: "one"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := ClosePersistent(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(f.cache, HistoryDirName, "amber-tide-fox")
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	f.open()
	if err := ClosePersistent(); err != nil {
		t.Fatal(err)
	}
	if names, err := legacyHistoryNames(dir); err != nil || len(names) != 1 {
		t.Fatalf("history after a prune and an export = %v %v, want the entry rewritten", names, err)
	}
}
