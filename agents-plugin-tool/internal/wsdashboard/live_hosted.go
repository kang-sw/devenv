package wsdashboard

import (
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// The live layer of the MCP sessions the daemon hosts (proxy.go). A ws-mcp
// in its own process tells the daemon about a live change by writing the
// session's snapshot file and nudging, and the daemon re-reads the session
// from disk. A server the daemon hosts is in this process: it hands each
// snapshot over in memory (HostedSlot.Publish), the daemon serves that copy
// in place of the file to every snapshot reader, and the session's pages
// wake with a live-only wake, which re-reads no session store but the
// snapshot (and the ring after an append). The hosted writer still flushes
// the file at its heartbeat interval, at a turn's end, and when its server
// closes, so a replacement daemon starts from it; the slot's Close drops the
// copy, and the file is what renders after.

// LiveHost is the daemon side of a hosted server's live layer.
type LiveHost interface {
	// OpenHostedSlot claims the in-memory snapshot slot of the session whose
	// registry directory is sessionDir; false when this daemon does not
	// serve that directory (a session under another cache home), and the
	// writer keeps writing the file and nudging. A slot opened later for
	// the same session takes the copy over.
	OpenHostedSlot(sessionDir string) (HostedSlot, bool)
}

// HostedSlot is one hosted session's in-memory snapshot hand-off.
type HostedSlot interface {
	// Publish serves snap as the session's snapshot. A change beyond its
	// updatedAt wakes the session's pages with a live-only wake; a
	// heartbeat (only updatedAt moved) wakes nothing. The index entry's
	// SnapshotUpdatedAt follows either way, with no Rev raised.
	Publish(snap wslive.Snapshot)
	// RingAppended reports entries appended to the session's ring file: a
	// live-only wake whose render re-reads the ring.
	RingAppended()
	// Close drops the copy while this slot still owns it, so the snapshot
	// file renders again; the session is re-read from disk.
	Close()
}

// ObservedFoldSlot is optional on a HostedSlot. A writer whose state seeded
// from the session's whole evidence directory (wslive.ReadObservedFold
// reported it complete) before it opened the slot calls FoldsObserved before
// its first Publish:
// it applies every record it writes there, so its copies hold the whole
// fold, and the daemon serves their evidence with no evidence read
// (SnapshotCache.snapshotResult). An undeclared slot's copies are overlaid
// with the directory as a file's are.
type ObservedFoldSlot interface {
	FoldsObserved()
}

// LiveSlotOwner is optional: only a daemon-owned generation can admit summary
// mutations and ring writes. Publish suppression alone cannot fence disk writes.
// The release must run before Publish, RingAppended, or Close.
type LiveSlotOwner interface {
	LockOwner() (release func(), owned bool)
}

// LiveSnapshotHolder is an optional interface of a SessionSource that reads
// snapshots through a cache of its own (its change detector): the daemon
// hands it every hosted copy as well, so its fingerprint and
// SnapshotUpdatedAt follow the copy, not the file.
type LiveSnapshotHolder interface {
	SetLiveSnapshot(stateDir, replyID string, snap wslive.Snapshot)
	ClearLiveSnapshot(stateDir, replyID string)
}

// Wake tokens of a live-only wake (broker): liveDep names a session's
// snapshot change, ringDep a ring append. A reply-id never holds a colon,
// so neither collides with one.
func liveDep(replyID string) string { return "live:" + replyID }
func ringDep(replyID string) string { return "ring:" + replyID }

// inboxDep permits a session live render, but unlike a snapshot-only wake it
// must refresh held and drained inbox reads (readSessionWith).
func inboxDep(replyID string) string { return "inbox:" + replyID }

// liveToken reports a live-only wake token.
func liveToken(token string) bool {
	return strings.HasPrefix(token, "live:") || strings.HasPrefix(token, "ring:") || strings.HasPrefix(token, "inbox:")
}

// hostedSlots is the daemon's slot ownership: the slot that currently owns
// each session's copy, and each session's count of hosted ring appends
// (the rail's message cache keys on it beside the index revision).
//
// mu is held only briefly and never across file I/O a writer does under
// its ownership. ownerLocks holds one mutex per session, created lazily and
// never deleted (every slot generation of a session shares it): a hosted
// writer holds its own session's mutex through its write (LockOwner), and a
// takeover or close of that session waits for it, while other sessions'
// slots go on. Lock order: ownerLock(id) -> mu.
type hostedSlots struct {
	mu         sync.Mutex
	next       uint64
	owners     map[string]uint64
	rings      map[string]uint64
	unpin      map[string]func() error
	ownerLocks map[string]*sync.Mutex
}

// ownerLock returns replyID's slot ownership mutex. mu is released before
// the caller locks it.
func (l *hostedSlots) ownerLock(replyID string) *sync.Mutex {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ownerLocks == nil {
		l.ownerLocks = map[string]*sync.Mutex{}
	}
	m := l.ownerLocks[replyID]
	if m == nil {
		m = &sync.Mutex{}
		l.ownerLocks[replyID] = m
	}
	return m
}

// ringGen is replyID's count of hosted ring appends.
func (l *hostedSlots) ringGen(replyID string) uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rings[replyID]
}

// hasOwner reports whether a live hosted slot currently owns replyID.
func (l *hostedSlots) hasOwner(replyID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.owners[replyID]
	return ok
}

// drop forgets a session that left the index.
func (l *hostedSlots) drop(replyID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.rings, replyID)
}

// railRev is the rail message cache's revision of a session: its index
// revision, moved by each hosted ring append (which raises no Rev).
func railRev(rev, ringGen uint64) uint64 {
	return rev ^ (ringGen * 0x9E3779B97F4A7C15)
}

// OpenHostedSlot implements LiveHost.
func (d *Daemon) OpenHostedSlot(sessionDir string) (HostedSlot, bool) {
	replyID := filepath.Base(sessionDir)
	if !ValidReplyID(replyID) || filepath.Clean(SessionDir(d.dir, replyID)) != filepath.Clean(sessionDir) {
		return nil, false
	}
	select {
	case <-d.stop:
		return nil, false
	default:
	}
	// A takeover waits for the replaced slot's write in progress (LockOwner).
	owner := d.live.ownerLock(replyID)
	owner.Lock()
	defer owner.Unlock()
	d.live.mu.Lock()
	defer d.live.mu.Unlock()
	if d.live.owners == nil {
		d.live.owners = map[string]uint64{}
	}
	if _, held := d.live.owners[replyID]; held {
		// A takeover: the slot it replaces may still write evidence that the
		// new writer's seed missed, so the copy no longer stands for the
		// directory (cleared with the copy).
		d.snapshots.shareObserved(d.dir, replyID)
	} else {
		// Pin the conversation ring in daemon RAM while a HostedSlot owns it.
		if d.live.unpin == nil {
			d.live.unpin = map[string]func() error{}
		}
		d.live.unpin[replyID] = wslive.PinRing(wslive.RingDir(SessionDir(d.dir, replyID)))
	}
	d.live.next++
	d.live.owners[replyID] = d.live.next
	return &daemonLiveSlot{d: d, replyID: replyID, id: d.live.next}, true
}

type daemonLiveSlot struct {
	d       *Daemon
	replyID string
	id      uint64
	closed  atomic.Bool
	folded  atomic.Bool // ObservedFoldSlot
}

// FoldsObserved implements ObservedFoldSlot.
func (s *daemonLiveSlot) FoldsObserved() { s.folded.Store(true) }

// LockOwner holds this session's ownership mutex, not the daemon-wide mu,
// through the caller's write: one session's stalled ring I/O blocks only a
// takeover or close of that session.
func (s *daemonLiveSlot) LockOwner() (func(), bool) {
	owner := s.d.live.ownerLock(s.replyID)
	owner.Lock()
	s.d.live.mu.Lock()
	owned := !s.closed.Load() && s.d.live.owners[s.replyID] == s.id
	s.d.live.mu.Unlock()
	if !owned {
		owner.Unlock()
		return nil, false
	}
	return owner.Unlock, true
}

func (s *daemonLiveSlot) Publish(snap wslive.Snapshot) {
	d := s.d
	d.live.mu.Lock()
	if s.closed.Load() || d.live.owners[s.replyID] != s.id {
		d.live.mu.Unlock()
		return
	}
	changed := d.snapshots.setLive(d.dir, s.replyID, snap, s.folded.Load())
	if h, ok := d.source.(LiveSnapshotHolder); ok {
		h.SetLiveSnapshot(d.dir, s.replyID, snap)
	}
	d.live.mu.Unlock()
	d.planUsage.pushClaude(&snap)
	if d.index != nil && snap.UpdatedAt > 0 {
		d.index.SetSnapshotUpdatedAt(s.replyID, time.UnixMilli(snap.UpdatedAt), snapshotStaleAt(&snap))
	}
	if changed {
		d.stats.WakeLive.Add(1)
		d.broker.wake(liveDep(s.replyID))
		d.acks.poke(s.replyID)
	}
}

func (s *daemonLiveSlot) RingAppended() {
	d := s.d
	d.live.mu.Lock()
	if d.live.rings == nil {
		d.live.rings = map[string]uint64{}
	}
	d.live.rings[s.replyID]++
	d.live.mu.Unlock()
	d.stats.WakeLive.Add(1)
	d.broker.wake(ringDep(s.replyID))
	d.acks.poke(s.replyID)
}

func (s *daemonLiveSlot) Close() {
	if s.closed.Swap(true) {
		return
	}
	d := s.d
	// Waits for this slot's write in progress (LockOwner), as a takeover does.
	owner := d.live.ownerLock(s.replyID)
	owner.Lock()
	d.live.mu.Lock()
	owned := d.live.owners[s.replyID] == s.id
	var unpin func() error
	if owned {
		delete(d.live.owners, s.replyID)
		unpin = d.live.unpin[s.replyID]
		delete(d.live.unpin, s.replyID)
		d.snapshots.ClearLive(d.dir, s.replyID)
		if h, ok := d.source.(LiveSnapshotHolder); ok {
			h.ClearLiveSnapshot(d.dir, s.replyID)
		}
	}
	d.live.mu.Unlock()
	owner.Unlock()
	if !owned {
		return
	}
	if unpin != nil {
		_ = unpin()
	}
	// The file is the session's snapshot again: a full re-read, as a nudge
	// would cause, and a full wake.
	if d.index != nil {
		d.index.Reread(s.replyID)
	}
	d.stats.WakeSession.Add(1)
	d.broker.wake(s.replyID)
}
