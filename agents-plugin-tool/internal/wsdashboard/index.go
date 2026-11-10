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
	// session a re-read found changed, added, or removed.
	OnChange func(replyID string)
	// Log receives one line per session added to or removed from the index;
	// nil discards them.
	Log io.Writer
}

// Index is the daemon's view of the registered sessions, rebuilt from disk
// alone: a replacement daemon builds the same index from the same files.
type Index struct {
	opts IndexOptions

	// rereadMu serializes Refresh and Nudge, so an older read of a session
	// never lands after a newer one.
	rereadMu sync.Mutex
	mu       sync.Mutex
	sessions map[string]*indexEntry
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
	return &Index{opts: opts, sessions: map[string]*indexEntry{}}
}

// Refresh re-reads every registered session: the polling backstop. An
// ended session the index already holds with this same registration is not
// re-read (endedApplied): nothing serves it, so only a nudge or a resume
// changes it, and a resume rewrites its registration.
func (x *Index) Refresh() {
	x.rereadMu.Lock()
	defer x.rereadMu.Unlock()
	regs, err := ReadRegistrations(x.opts.Dir)
	if err != nil {
		fmt.Fprintf(x.opts.Log, "dashboard: read session registry: %v\n", err)
		return
	}
	seen := map[string]bool{}
	var changed []string
	for _, reg := range regs {
		seen[reg.ReplyID] = true
		if x.endedApplied(reg) {
			continue
		}
		if x.apply(reg.ReplyID, &reg) {
			changed = append(changed, reg.ReplyID)
		}
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
		if x.apply(id, nil) {
			changed = append(changed, id)
		}
	}
	x.notify(changed)
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

// Nudge re-reads the one session a nudge names. It returns false, and
// changes nothing, when the reply-id is malformed, the session is not
// registered, or the registration names a different project.
func (x *Index) Nudge(replyID, project string) bool {
	if !ValidReplyID(replyID) || project == "" {
		return false
	}
	x.rereadMu.Lock()
	defer x.rereadMu.Unlock()
	reg, err := ReadRegistration(x.opts.Dir, replyID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// A registration removed since the last poll leaves the index now.
			if x.apply(replyID, nil) {
				x.notify([]string{replyID})
			}
		}
		return false
	}
	if filepath.Clean(reg.Project) != filepath.Clean(project) {
		return false
	}
	if x.apply(replyID, &reg) {
		x.notify([]string{replyID})
	}
	return true
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

func (x *Index) notify(ids []string) {
	if x.opts.OnChange == nil {
		return
	}
	for _, id := range ids {
		x.opts.OnChange(id)
	}
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
