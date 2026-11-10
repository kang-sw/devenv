package wsdashboard

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// DefaultPollInterval is the polling backstop: every registered session is
// re-read this often, which catches a store write whose nudge was lost.
const DefaultPollInterval = 30 * time.Second

// SessionData is what a SessionSource reads from one session's stores.
type SessionData struct {
	// HasBoard reports whether the session has a whiteboard.
	HasBoard bool
	// CountPolicy is the board's open policy items, which the sessions
	// index shows beside the board link; zero without a board.
	CountPolicy int
	// Fingerprint changes whenever anything the source read changed, so the
	// index can tell a re-read that found a change from one that did not.
	Fingerprint string
	// SnapshotUpdatedAt is the live snapshot's updatedAt (its writer's last
	// write or heartbeat); zero when the session has no readable snapshot.
	SnapshotUpdatedAt time.Time
}

// SessionSource reads a registered session's on-disk stores. ok is false
// when the session no longer exists (its session record is gone), which
// drops it from the index.
type SessionSource interface {
	ReadSession(reg Registration) (data SessionData, ok bool)
}

// SessionForgetter is an optional interface of a SessionSource that keeps
// per-session caches: the daemon calls ForgetSession once a session left
// the index, so they do not outlive it.
type SessionForgetter interface {
	ForgetSession(replyID string)
}

// Session is one entry of the session index. It carries the reply-id and
// never the session key.
type Session struct {
	Registration
	// Live is Registration.Live at the time the index was read.
	Live     bool
	HasBoard bool
	// CountPolicy is SessionData.CountPolicy as of the last read.
	CountPolicy int
	// SnapshotUpdatedAt ages the session's live snapshot (SessionData). A
	// change of it alone is a heartbeat, not a session change: it does not
	// raise Rev.
	SnapshotUpdatedAt time.Time
	// Rev increases each time a re-read finds the session changed.
	Rev uint64
}

// IndexOptions configures NewIndex.
type IndexOptions struct {
	// Dir is the daemon state directory holding the session registry.
	Dir    string
	Source SessionSource
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
	// OnChange is called, outside the index lock, with the reply-id of each
	// session a re-read found changed, added, or removed. It runs on the
	// re-read's goroutine under that session's lane, so calls for different
	// sessions may run concurrently, and calls for one session never do.
	OnChange func(replyID string)
	// Log receives one line per session added to or removed from the index;
	// nil discards them.
	Log io.Writer
}

// Index is the daemon's view of the registered sessions, rebuilt from disk
// alone: a replacement daemon builds the same index from the same files.
type Index struct {
	opts IndexOptions

	// lanesMu guards lanes and inflight; idle is signaled when inflight
	// drops to zero (Settle).
	lanesMu  sync.Mutex
	lanes    map[string]*lane
	inflight int
	idle     *sync.Cond

	mu       sync.Mutex
	sessions map[string]*indexEntry
}

// lane is one session's re-read state. Re-reads are serialized per session,
// never across sessions, so a slow read of one session delays no other.
type lane struct {
	// mu is held across one re-read of the session: its registration read,
	// its store read, the apply, and the change hook. That keeps an older
	// read of the session from landing after a newer one, and lets a
	// Refresh sweep confirm a removal against the registration as it is
	// now rather than as its listing saw it.
	mu sync.Mutex
	// The rest is guarded by Index.lanesMu. refs counts the lane's holders
	// (a Refresh step, a drain worker); the lane is forgotten with its last.
	// running reports a drain worker; queued reports a re-read requested
	// that the worker has not started yet, so requests arriving while one
	// re-read runs fold into one more.
	refs    int
	running bool
	queued  bool
}

type indexEntry struct {
	session     Session
	fingerprint string
}

// NewIndex returns an empty index; Refresh fills it.
func NewIndex(opts IndexOptions) *Index {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Log == nil {
		opts.Log = io.Discard
	}
	x := &Index{opts: opts, sessions: map[string]*indexEntry{}, lanes: map[string]*lane{}}
	x.idle = sync.NewCond(&x.lanesMu)
	return x
}

// Refresh re-reads every registered session: the polling backstop. It runs
// on its caller, one session at a time, each under that session's lane
// only. The listing it starts from only finds the sessions to re-read:
// each re-read reads the registration again, and a session missing from
// the listing is removed only when its registration is still gone, so a
// session registered, resumed, or ended during the sweep is never removed
// or rolled back by the sweep's older listing. An ended session the index
// already holds with this same registration is not re-read (endedApplied):
// nothing serves it, so only a nudge or a resume changes it, and a resume
// rewrites its registration.
func (x *Index) Refresh() {
	regs, err := ReadRegistrations(x.opts.Dir)
	if err != nil {
		fmt.Fprintf(x.opts.Log, "dashboard: read session registry: %v\n", err)
		return
	}
	seen := map[string]bool{}
	for _, reg := range regs {
		seen[reg.ReplyID] = true
		if x.endedApplied(reg) {
			continue
		}
		x.reread(reg.ReplyID, false)
	}
	x.mu.Lock()
	var gone []string
	for id := range x.sessions {
		if !seen[id] {
			gone = append(gone, id)
		}
	}
	x.mu.Unlock()
	for _, id := range gone {
		// The listing skips an unreadable registration, which drops it.
		x.reread(id, true)
	}
}

// endedApplied reports whether reg is an ended registration the index
// already applied unchanged and whose project root still exists, so a
// refresh may skip re-reading it. The first refresh after a session ended
// still re-reads it (the entry holds the registration from before), and a
// removed root still drops it. A session that only stopped heartbeating
// (no EndedAt) is re-read as before.
func (x *Index) endedApplied(reg Registration) bool {
	if reg.EndedAt == "" {
		return false
	}
	x.mu.Lock()
	entry, ok := x.sessions[reg.ReplyID]
	same := ok && entry.session.Registration == reg
	x.mu.Unlock()
	if !same {
		return false
	}
	_, err := os.Stat(reg.Project)
	return !errors.Is(err, os.ErrNotExist)
}

// Nudge schedules a re-read of the one session a nudge names (Reread) and
// returns without waiting for it. It returns false, and schedules nothing
// for a session the index does not hold, when the reply-id is malformed,
// the session is not registered, or the registration names a different
// project; a registration removed since the last poll schedules the re-read
// that drops it.
func (x *Index) Nudge(replyID, project string) bool {
	if !ValidReplyID(replyID) || project == "" {
		return false
	}
	reg, err := ReadRegistration(x.opts.Dir, replyID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && x.holds(replyID) {
			x.schedule(replyID)
		}
		return false
	}
	if filepath.Clean(reg.Project) != filepath.Clean(project) {
		return false
	}
	x.schedule(replyID)
	return true
}

// Reread schedules a re-read of one session off the caller: the session's
// registration and stores are read again on a worker of that session's
// own, and the change hook runs there. Requests that arrive while the
// session's re-read runs fold into one more re-read; sessions re-read in
// parallel. A malformed reply-id is ignored.
func (x *Index) Reread(replyID string) {
	if ValidReplyID(replyID) {
		x.schedule(replyID)
	}
}

// Settle waits until every re-read Nudge and Reread scheduled has landed,
// including the ones scheduled while it waits.
func (x *Index) Settle() {
	x.lanesMu.Lock()
	for x.inflight > 0 {
		x.idle.Wait()
	}
	x.lanesMu.Unlock()
}

// holds reports whether the index holds replyID.
func (x *Index) holds(replyID string) bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	_, ok := x.sessions[replyID]
	return ok
}

// hold returns replyID's lane with one more holder; lanesMu is held.
func (x *Index) hold(replyID string) *lane {
	l := x.lanes[replyID]
	if l == nil {
		l = &lane{}
		x.lanes[replyID] = l
	}
	l.refs++
	return l
}

// unhold drops one holder of replyID's lane, forgetting the lane with its
// last; lanesMu is held.
func (x *Index) unhold(replyID string, l *lane) {
	l.refs--
	if l.refs == 0 {
		delete(x.lanes, replyID)
	}
}

// schedule asks for one more re-read of replyID, starting the session's
// drain worker when none runs.
func (x *Index) schedule(replyID string) {
	x.lanesMu.Lock()
	defer x.lanesMu.Unlock()
	l := x.lanes[replyID]
	if l != nil && l.running {
		l.queued = true
		return
	}
	l = x.hold(replyID)
	l.running, l.queued = true, true
	x.inflight++
	go x.drain(replyID, l)
}

// drain is a session's re-read worker: it re-reads while re-reads are
// queued, then ends.
func (x *Index) drain(replyID string, l *lane) {
	for {
		x.lanesMu.Lock()
		if !l.queued {
			l.running = false
			x.unhold(replyID, l)
			x.inflight--
			if x.inflight == 0 {
				x.idle.Broadcast()
			}
			x.lanesMu.Unlock()
			return
		}
		l.queued = false
		x.lanesMu.Unlock()
		x.readLane(replyID, l, false)
	}
}

// reread re-reads one session under its lane on the caller.
func (x *Index) reread(replyID string, dropUnreadable bool) {
	x.lanesMu.Lock()
	l := x.hold(replyID)
	x.lanesMu.Unlock()
	x.readLane(replyID, l, dropUnreadable)
	x.lanesMu.Lock()
	x.unhold(replyID, l)
	x.lanesMu.Unlock()
}

// readLane re-reads one session from its registration as it is now: a
// removed registration drops it, and so does an unreadable one when
// dropUnreadable is set; otherwise an unreadable registration leaves the
// entry as it is. It holds the lane's mu across the read and the change
// hook.
func (x *Index) readLane(replyID string, l *lane, dropUnreadable bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	reg, err := ReadRegistration(x.opts.Dir, replyID)
	var changed bool
	switch {
	case err == nil:
		changed = x.apply(replyID, &reg)
	case errors.Is(err, os.ErrNotExist) || dropUnreadable:
		changed = x.apply(replyID, nil)
	default:
		fmt.Fprintf(x.opts.Log, "dashboard: read registration %s: %v\n", replyID, err)
	}
	if changed && x.opts.OnChange != nil {
		x.opts.OnChange(replyID)
	}
}

// apply re-reads one session from its registration (nil when it has none)
// and its stores, and reports whether the index entry changed.
func (x *Index) apply(replyID string, reg *Registration) bool {
	var data SessionData
	keep := reg != nil
	if keep {
		if _, err := os.Stat(reg.Project); errors.Is(err, os.ErrNotExist) {
			keep = false // the project root was removed
		}
	}
	if keep {
		data, keep = x.opts.Source.ReadSession(*reg)
	}
	now := x.opts.Now()

	x.mu.Lock()
	defer x.mu.Unlock()
	entry, known := x.sessions[replyID]
	if !keep {
		if !known {
			return false
		}
		delete(x.sessions, replyID)
		fmt.Fprintf(x.opts.Log, "dashboard: session %s removed (project %s)\n", replyID, entry.session.Project)
		return true
	}
	next := Session{Registration: *reg, Live: reg.Live(now), HasBoard: data.HasBoard, CountPolicy: data.CountPolicy, SnapshotUpdatedAt: data.SnapshotUpdatedAt}
	if !known {
		next.Rev = 1
		x.sessions[replyID] = &indexEntry{session: next, fingerprint: data.Fingerprint}
		fmt.Fprintf(x.opts.Log, "dashboard: session %s added (project %s)\n", replyID, reg.Project)
		return true
	}
	next.Rev = entry.session.Rev
	same := next
	same.SnapshotUpdatedAt = entry.session.SnapshotUpdatedAt
	if same == entry.session && data.Fingerprint == entry.fingerprint {
		entry.session.SnapshotUpdatedAt = next.SnapshotUpdatedAt
		return false
	}
	next.Rev++
	entry.session = next
	entry.fingerprint = data.Fingerprint
	return true
}

// Sessions returns every indexed session ordered by project, then reply-id,
// with Live evaluated now.
func (x *Index) Sessions() []Session {
	now := x.opts.Now()
	x.mu.Lock()
	out := make([]Session, 0, len(x.sessions))
	for _, entry := range x.sessions {
		s := entry.session
		s.Live = s.Registration.Live(now)
		out = append(out, s)
	}
	x.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Project != out[j].Project {
			return out[i].Project < out[j].Project
		}
		return out[i].ReplyID < out[j].ReplyID
	})
	return out
}

// Session returns one indexed session, with Live evaluated now.
func (x *Index) Session(replyID string) (Session, bool) {
	x.mu.Lock()
	entry, ok := x.sessions[replyID]
	var s Session
	if ok {
		s = entry.session
	}
	x.mu.Unlock()
	if ok {
		s.Live = s.Registration.Live(x.opts.Now())
	}
	return s, ok
}
