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
	// Fingerprint changes whenever anything the source read changed, so the
	// index can tell a re-read that found a change from one that did not.
	Fingerprint string
}

// SessionSource reads a registered session's on-disk stores. ok is false
// when the session no longer exists (its session record is gone), which
// drops it from the index.
type SessionSource interface {
	ReadSession(reg Registration) (data SessionData, ok bool)
}

// Session is one entry of the session index. It carries the reply-id and
// never the session key.
type Session struct {
	Registration
	// Live is Registration.Live at the time the index was read.
	Live     bool
	HasBoard bool
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

// Refresh re-reads every registered session: the polling backstop.
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
	next := Session{Registration: *reg, Live: reg.Live(now), HasBoard: data.HasBoard}
	if !known {
		next.Rev = 1
		x.sessions[replyID] = &indexEntry{session: next, fingerprint: data.Fingerprint}
		fmt.Fprintf(x.opts.Log, "dashboard: session %s added (project %s)\n", replyID, reg.Project)
		return true
	}
	next.Rev = entry.session.Rev
	if next == entry.session && data.Fingerprint == entry.fingerprint {
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
