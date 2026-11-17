package wsdashboard

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultPollInterval is the polling backstop: every registered session is
// re-read this often, which catches a store write whose nudge was lost.
const DefaultPollInterval = 5 * time.Minute

// indexClockInterval preserves the previous liveness-check cadence. Between
// safety scans only an expiry/recovery of known evidence needs a targeted read.
const indexClockInterval = 30 * time.Second

// SessionData is what a SessionSource reads from one session's stores.
type SessionData struct {
	// HasBoard reports whether the session has a whiteboard.
	HasBoard bool
	// CountPolicy is the board's open policy and check items (the amber
	// "answer needed" count), which the sessions index shows beside the
	// board link; zero without a board.
	CountPolicy int
	// Fingerprint changes whenever anything the source read changed, so the
	// index can tell a re-read that found a change from one that did not.
	Fingerprint string
	// NonLiveFingerprint and LiveStores retain the two independent fingerprint
	// parts for a live-only reread. They are in-memory evidence, never persisted.
	NonLiveFingerprint string
	LiveStores         LiveStoreFingerprint
	// SnapshotUpdatedAt is the live snapshot's updatedAt (its writer's last
	// write or heartbeat); zero when the session has no readable snapshot.
	SnapshotUpdatedAt time.Time
	// SnapshotStaleAt is the first instant the already-read snapshot is stale.
	// Zero means no snapshot evidence. This is an in-memory deadline only.
	SnapshotStaleAt time.Time
	// Resumable reports a session whose record names the harness
	// conversation behind it (its harness reported one at login), which
	// the daemon needs to take the session over from a terminal.
	Resumable bool
}

// SessionSource reads a registered session's on-disk stores. With no error,
// ok distinguishes a valid identity from genuine absence or mismatch. An
// error is transient: the index retains its last good entry and retries on
// the next polling backstop, even when that entry is ended.
type SessionSource interface {
	ReadSession(reg Registration) (data SessionData, ok bool, err error)
}

// LiveSessionSource refreshes only live stores, retaining the last full read's
// record/board/mailbox evidence. The index supplies a previous successful read;
// cold sessions and changed registration identities still use ReadSession.
type LiveSessionSource interface {
	ReadLiveSession(reg Registration, previous SessionData, stores LiveStores) (SessionData, bool, error)
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
	// Hidden comes only from the dashboard sidecar, never Registration.
	// Lifecycle is derived without changing liveness or retention behavior.
	Hidden    bool
	Lifecycle SessionLifecycle
	// CountPolicy is SessionData.CountPolicy as of the last read.
	CountPolicy int
	// Resumable is SessionData.Resumable as of the last read.
	Resumable bool
	// Label is the session's display label as of the last read
	// (label_route.go), empty when it has none.
	Label string
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
	// lastScheduled survives idle drain workers, so separate bursts still obey
	// the per-session one-second limit. Guarded by lanesMu; dropped on removal.
	lastScheduled map[string]time.Time
	// Instance-local scheduler clock/wait seams; nil uses wall time. Separate
	// from Now, which may be a fixed evidence clock in tests.
	scheduledNow  func() time.Time
	scheduledWait func(time.Duration)

	mu         sync.Mutex
	sessions   map[string]*indexEntry
	readErrors map[string]bool // transient reads, including not-yet-indexed sessions
	// Hosted clock handoffs can precede the first source read/insertion.
	liveClocks    map[string]indexLiveClock
	nextLiveClock uint64

	// loaded is set once a Refresh has read the whole registry (Loaded).
	loaded atomic.Bool
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
	full    bool
	stores  LiveStores
}

type indexEntry struct {
	session         Session
	fingerprint     string
	data            SessionData
	retry           bool // bypass endedApplied until a successful read
	snapshotStaleAt time.Time
	snapshotStale   bool
}

type indexLiveClock struct {
	generation  uint64
	at, staleAt time.Time
}

// NewIndex returns an empty index; Refresh fills it.
func NewIndex(opts IndexOptions) *Index {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Log == nil {
		opts.Log = io.Discard
	}
	x := &Index{opts: opts, sessions: map[string]*indexEntry{}, lanes: map[string]*lane{}, readErrors: map[string]bool{}}
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
	seen := map[string]bool{}
	regs, err := readRegistrations(x.opts.Dir, func(id string, err error) {
		// Remember unreadable startup rows too. This uses the listing's
		// existing read, not a second probe of an already failed file.
		seen[id] = true
		x.retry(id)
		fmt.Fprintf(x.opts.Log, "dashboard: read registration %s: %v\n", id, err)
	})
	if err != nil {
		fmt.Fprintf(x.opts.Log, "dashboard: read session registry: %v\n", err)
		return
	}
	for _, reg := range regs {
		seen[reg.ReplyID] = true
		if x.endedApplied(reg) {
			continue
		}
		x.reread(reg.ReplyID)
	}
	x.mu.Lock()
	var gone []string
	for id := range x.sessions {
		if !seen[id] {
			gone = append(gone, id)
		}
	}
	for id := range x.readErrors {
		if !seen[id] && x.sessions[id] == nil {
			gone = append(gone, id)
		}
	}
	x.mu.Unlock()
	for _, id := range gone {
		// A listing omits unreadable registrations too. Confirm absence;
		// unreadability must retain the last valid identity.
		x.reread(id)
	}
	x.loaded.Store(true)
}

// Loaded reports whether a Refresh has read the whole registry at least
// once: until then a session the index lacks may only be unread yet, so a
// store keyed by reply-id (the rail layout) must not drop it as forgotten.
func (x *Index) Loaded() bool { return x.loaded.Load() }

// hierarchyReady checks the requester's row and stored ancestors only. An
// unreadable ancestor must not become false top-level authority, but unrelated
// corrupt registrations must not stall approval replay. A missing ancestor
// without a read error was authoritatively removed by the completed listing.
func (x *Index) hierarchyReady(replyID string, ancestors ...string) bool {
	if x == nil || !x.Loaded() {
		return false
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.sessions[replyID] == nil || x.readErrors[replyID] {
		return false
	}
	for _, id := range ancestors {
		if x.readErrors[id] {
			return false
		}
	}
	return true
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
	same := ok && !entry.retry && entry.session.Registration == reg
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
		if (errors.Is(err, os.ErrNotExist) || errors.Is(err, errRegistrationMismatch)) && x.holds(replyID) {
			x.schedule(replyID, true, AllLiveStores)
		}
		return false
	}
	if filepath.Clean(reg.Project) != filepath.Clean(project) {
		return false
	}
	x.schedule(replyID, true, AllLiveStores)
	return true
}

// Reread schedules a full re-read off the caller. Scheduled reads start at
// most once per second per session, with a trailing read for requests arriving
// during a read or its cooldown. Different sessions still read in parallel.
// A malformed reply-id is ignored.
func (x *Index) Reread(replyID string) {
	if ValidReplyID(replyID) {
		x.schedule(replyID, true, AllLiveStores)
	}
}

// RereadLive schedules a runner write without refreshing record, board, ticket
// or mailbox evidence. A caller that knows the changed store can name it;
// unknown writes conservatively refresh all live stores. Full requests win
// when coalesced with live-only requests.
func (x *Index) RereadLive(replyID string, stores ...LiveStores) {
	if !ValidReplyID(replyID) {
		return
	}
	changed := AllLiveStores
	if len(stores) > 0 {
		changed = 0
		for _, store := range stores {
			changed |= store
		}
	}
	x.schedule(replyID, false, changed)
}

// SetSnapshotUpdatedAt sets the indexed session's SnapshotUpdatedAt in
// place, for a hosted session's in-memory snapshot hand-off
// (LiveSlot.Publish): like a heartbeat-only re-read, it raises no Rev and
// calls no change hook. A handoff arriving during the first source read is
// retained until insertion; it does not create or authorize an index entry.
func (x *Index) SetSnapshotUpdatedAt(replyID string, at, staleAt time.Time) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.liveClocks == nil {
		x.liveClocks = map[string]indexLiveClock{}
	}
	x.nextLiveClock++
	x.liveClocks[replyID] = indexLiveClock{generation: x.nextLiveClock, at: at, staleAt: staleAt}
	if entry, ok := x.sessions[replyID]; ok {
		entry.session.SnapshotUpdatedAt = at
		entry.snapshotStaleAt = staleAt
	}
}

// RefreshClock examines only evidence already in memory. A due liveness
// transition schedules one targeted authoritative read, rather than declaring
// a legacy writer dead from an old cached heartbeat. Healthy hosted beats
// move their deadlines directly, so an idle clock tick does no filesystem work.
func (x *Index) RefreshClock() {
	now := x.opts.Now()
	x.mu.Lock()
	var due []string
	for id, entry := range x.sessions {
		s := entry.session
		stale := !entry.snapshotStaleAt.IsZero() && !now.Before(entry.snapshotStaleAt)
		// Keep checking already-stale, non-ended writers at the previous
		// cadence too: a legacy writer may resume without sending a nudge.
		if s.EndedAt == "" && (!s.Registration.Live(now) || stale || s.Live != s.Registration.Live(now) || s.Lifecycle != DeriveSessionLifecycle(s.Registration, s.Hidden, now) || stale != entry.snapshotStale) {
			due = append(due, id)
		}
	}
	x.mu.Unlock()
	for _, id := range due {
		x.Reread(id)
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
func (x *Index) schedule(replyID string, full bool, stores LiveStores) {
	x.lanesMu.Lock()
	defer x.lanesMu.Unlock()
	l := x.lanes[replyID]
	if l != nil && l.running {
		l.queued = true
		l.full = l.full || full
		l.stores |= stores
		return
	}
	l = x.hold(replyID)
	l.running, l.queued, l.full, l.stores = true, true, full, stores
	x.inflight++
	go x.drain(replyID, l)
}

// drain is a session's re-read worker: it re-reads while re-reads are
// queued, then ends.
func (x *Index) drain(replyID string, l *lane) {
	for {
		l.mu.Lock()
		x.lanesMu.Lock()
		if !l.queued {
			l.running = false
			x.unhold(replyID, l)
			x.inflight--
			if x.inflight == 0 {
				x.idle.Broadcast()
			}
			x.lanesMu.Unlock()
			l.mu.Unlock()
			return
		}
		now := time.Now()
		if x.scheduledNow != nil {
			now = x.scheduledNow()
		}
		if delay := x.lastScheduled[replyID].Add(time.Second).Sub(now); delay > 0 {
			x.lanesMu.Unlock()
			l.mu.Unlock()
			wait := time.Sleep
			if x.scheduledWait != nil {
				wait = x.scheduledWait
			}
			wait(delay)
			continue // keep folding requests until the trailing read starts
		}
		full, stores := l.full, l.stores
		l.queued, l.full, l.stores = false, false, 0
		if x.lastScheduled == nil {
			x.lastScheduled = map[string]time.Time{}
		}
		x.lastScheduled[replyID] = now
		x.lanesMu.Unlock()
		x.readLaneLocked(replyID, full, stores)
		l.mu.Unlock()
	}
}

// reread re-reads one session under its lane on the caller.
func (x *Index) reread(replyID string) {
	x.lanesMu.Lock()
	l := x.hold(replyID)
	x.lanesMu.Unlock()
	x.readLane(replyID, l, true, AllLiveStores)
	x.lanesMu.Lock()
	x.unhold(replyID, l)
	x.lanesMu.Unlock()
}

// readLane confirms registration absence under the session lane. An
// unreadable registration retains the entry for the next polling backstop.
func (x *Index) readLane(replyID string, l *lane, full bool, stores LiveStores) {
	l.mu.Lock()
	defer l.mu.Unlock()
	x.readLaneLocked(replyID, full, stores)
}

func (x *Index) readLaneLocked(replyID string, full bool, stores LiveStores) {
	reg, err := ReadRegistration(x.opts.Dir, replyID)
	var changed bool
	switch {
	case err == nil:
		changed = x.applyRead(replyID, &reg, full, stores)
	case errors.Is(err, os.ErrNotExist) || errors.Is(err, errRegistrationMismatch):
		changed = x.apply(replyID, nil)
	default:
		x.retry(replyID)
		fmt.Fprintf(x.opts.Log, "dashboard: read registration %s: %v\n", replyID, err)
	}
	if changed && x.opts.OnChange != nil {
		x.opts.OnChange(replyID)
	}
}

// apply re-reads one session from its registration (nil when it has none)
// and its stores, and reports whether the index entry changed.
func (x *Index) apply(replyID string, reg *Registration) bool {
	return x.applyRead(replyID, reg, true, AllLiveStores)
}

func (x *Index) applyRead(replyID string, reg *Registration, full bool, stores LiveStores) bool {
	// A hosted heartbeat can publish while the source read is in flight.
	// Its newer clock handoff must not be rolled back by that older read.
	x.mu.Lock()
	liveGeneration := x.liveClocks[replyID].generation
	var previous SessionData
	if entry := x.sessions[replyID]; entry != nil && reg != nil &&
		!entry.retry && entry.session.Project == reg.Project && entry.session.Agent == reg.Agent {
		previous = entry.data
	} else {
		full = true
	}
	x.mu.Unlock()
	var data SessionData
	keep := reg != nil
	if keep {
		if _, err := os.Stat(reg.Project); errors.Is(err, os.ErrNotExist) {
			keep = false // the project root was removed
		}
	}
	var label string
	var hidden bool
	if keep {
		var err error
		if live, ok := x.opts.Source.(LiveSessionSource); ok && !full {
			data, keep, err = live.ReadLiveSession(*reg, previous, stores)
		} else {
			data, keep, err = x.opts.Source.ReadSession(*reg)
		}
		if err != nil {
			x.retry(replyID)
			fmt.Fprintf(x.opts.Log, "dashboard: read session %s: %v\n", replyID, err)
			return false
		}
		label = readSessionLabel(SessionDir(x.opts.Dir, replyID))
		if reg.Hosted {
			hidden, err = ReadSessionHidden(x.opts.Dir, replyID)
			if err != nil {
				// Match the runner's fail-closed launch gate, retaining the row.
				hidden = true
				fmt.Fprintf(x.opts.Log, "dashboard: read visibility %s: %v\n", replyID, err)
			}
		}
	}
	now := x.opts.Now()

	x.mu.Lock()
	defer x.mu.Unlock()
	delete(x.readErrors, replyID)
	entry, known := x.sessions[replyID]
	if !keep {
		delete(x.liveClocks, replyID)
		x.lanesMu.Lock()
		delete(x.lastScheduled, replyID)
		x.lanesMu.Unlock()
		if !known {
			return false
		}
		delete(x.sessions, replyID)
		fmt.Fprintf(x.opts.Log, "dashboard: session %s removed (project %s)\n", replyID, entry.session.Project)
		return true
	}
	if clock := x.liveClocks[replyID]; clock.generation != 0 && clock.generation != liveGeneration {
		data.SnapshotUpdatedAt = clock.at
		data.SnapshotStaleAt = clock.staleAt
	}
	snapshotStale := !data.SnapshotStaleAt.IsZero() && !now.Before(data.SnapshotStaleAt)
	next := Session{Registration: *reg, Live: reg.Live(now), HasBoard: data.HasBoard, CountPolicy: data.CountPolicy, Resumable: data.Resumable, Label: label, SnapshotUpdatedAt: data.SnapshotUpdatedAt, Hidden: hidden, Lifecycle: DeriveSessionLifecycle(*reg, hidden, now)}
	if !known {
		next.Rev = 1
		x.sessions[replyID] = &indexEntry{session: next, data: data, fingerprint: data.Fingerprint, snapshotStaleAt: data.SnapshotStaleAt, snapshotStale: snapshotStale}
		fmt.Fprintf(x.opts.Log, "dashboard: session %s added (project %s)\n", replyID, reg.Project)
		return true
	}
	entry.retry = false
	entry.data = data
	sameClock := entry.snapshotStale == snapshotStale
	entry.snapshotStaleAt, entry.snapshotStale = data.SnapshotStaleAt, snapshotStale
	next.Rev = entry.session.Rev
	same := next
	same.SnapshotUpdatedAt = entry.session.SnapshotUpdatedAt
	// Retain fresh heartbeat evidence without waking every page for a timestamp
	// alone. Live and Lifecycle are still compared, including stale/recovery.
	same.HeartbeatAt = entry.session.HeartbeatAt
	if same == entry.session && data.Fingerprint == entry.fingerprint && sameClock {
		entry.session = next
		return false
	}
	next.Rev++
	entry.session = next
	entry.fingerprint = data.Fingerprint
	return true
}

// retry keeps ended entries eligible for the daemon's scheduled Refresh.
// It changes no revision, invokes no removal hook, and forgets no caches.
func (x *Index) retry(replyID string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.readErrors[replyID] = true
	if entry := x.sessions[replyID]; entry != nil {
		entry.retry = true
	}
}

// readError distinguishes a not-yet-indexed unreadable session from one
// known to be absent. Registration errors are checked afresh for requests
// arriving before the first successful registry sweep.
func (x *Index) readError(replyID string) error {
	x.mu.Lock()
	pending := x.readErrors[replyID]
	x.mu.Unlock()
	if pending {
		return errors.New("session read pending retry")
	}
	_, err := ReadRegistration(x.opts.Dir, replyID)
	if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, errRegistrationMismatch) {
		return err
	}
	return nil
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
		s.Lifecycle = DeriveSessionLifecycle(s.Registration, s.Hidden, now)
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
		now := x.opts.Now()
		s.Live = s.Registration.Live(now)
		s.Lifecycle = DeriveSessionLifecycle(s.Registration, s.Hidden, now)
	}
	return s, ok
}
