// runner_startup.go holds Runner construction and startup recovery.
package wsagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsstate"
)

func NewRunner(opts Options) *Runner {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Timer == nil {
		opts.Timer = func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTimer(d)
			return t.C, func() { t.Stop() }
		}
	}
	if opts.AgyModelWaitBackoff == nil {
		opts.AgyModelWaitBackoff = defaultAgyModelWaitBackoff
	}
	if opts.Log == nil {
		opts.Log = io.Discard
	}
	if opts.heartbeat <= 0 {
		opts.heartbeat = wslive.HeartbeatInterval
	}
	if opts.Trusted == nil {
		opts.Trusted = wsdashboard.TrustedWrite
	}
	if opts.Alive == nil {
		opts.Alive = wsstate.ProcessAlive
	}
	r := &Runner{opts: opts, convs: map[string]*conv{}, closed: map[string]Record{}, quit: make(chan struct{}), scanned: make(chan struct{}), rejoined: make(chan struct{}), startupLoaded: make(chan struct{}), notify: func(string, string) {}}
	if opts.Holders != nil {
		// Before the manager's first scan, so a predecessor's holders are
		// seen attaching.
		opts.Holders.AddSink(&r.holders)
	}
	return r
}

// SetLiveHost binds the daemon's RAM projection before Start.
func (r *Runner) SetLiveHost(host wsdashboard.LiveHost) { r.opts.LiveHost = host }

// Start loads every conversation record from the session registry, with no
// process (lazy resume), and starts serving them. notify is called after
// each write to a conversation's stores, so the daemon re-reads it. The
// daemon calls Start once it holds its lock.
func (r *Runner) Start(notify func(replyID, project string)) {
	startupLoaded := make(chan struct{})
	r.startupMu.Lock()
	r.startupLoaded = startupLoaded
	r.startupMu.Unlock()
	if notify != nil {
		r.notify = notify
	}
	entries, _ := wsstate.ReadDir(filepath.Join(r.opts.Dir, "sessions"))
	var open []Record
	for _, e := range entries {
		if !e.IsDir() || !wsdashboard.ValidReplyID(e.Name()) {
			continue
		}
		dir := wsdashboard.SessionDir(r.opts.Dir, e.Name())
		rec, err := readRecord(dir)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(r.opts.Log, "agent: skip %s: %v\n", e.Name(), err)
			}
			continue
		}
		if rec.ClosedAt != "" || rec.handedOff() {
			// A handed-off session is a terminal harness's: nothing here
			// writes its row, watches its mail, or starts a process for it.
			r.loadClosed(rec)
		} else {
			open = append(open, rec)
		}
	}
	gone := map[string]bool{}
	for _, id := range r.expiredAtStart(open) {
		// Deepest first: a child waiting past its expiry is deleted with
		// its descendants, as the heartbeat would (expire).
		sessionDir := wsdashboard.SessionDir(r.opts.Dir, id)
		wslive.DropRing(wslive.RingDir(sessionDir))
		if err := os.RemoveAll(sessionDir); err != nil {
			fmt.Fprintf(r.opts.Log, "agent %s: delete: %v\n", id, err)
			continue
		}
		gone[id] = true
	}
	open = slices.DeleteFunc(open, func(rec Record) bool { return gone[rec.ReplyID] })
	orphans := orphansAtStart(open)
	startup := make([]*conv, 0, len(open))
	for _, rec := range open {
		if orphans[rec.ReplyID] && r.closeAtStart(rec) {
			continue
		}
		// A lead whose process may still run under its holder rejoins
		// it there, before the scan below replays the holder's records.
		// A turn the last daemon's exit cut off settles there too.
		startup = append(startup, r.startConvAtStartup(rec, nil))
	}
	close(startupLoaded)
	for _, c := range startup {
		c.do(func() {
			// Re-run only startup work whose earlier signal may have been
			// consumed by the all-records-loaded barrier. A held inbox send on
			// a processless conversation remains lazy until a new wake, as it
			// did before restart restore existed.
			if c.proc != nil || len(c.rec.Queued) > 0 || len(c.rec.Mail) > 0 || len(c.rec.Reports) > 0 {
				c.kick()
			}
		})
	}
	// All records are loaded now, including children sorted after their lead.
	for _, st := range r.List() {
		if c := r.conv(st.ID); c != nil {
			c.do(func() {
				// Release a persisted HeldTurn whose background work ended
				// with the previous daemon (open children still hold it).
				if c.rec.Parent != nil && c.rec.HolderID == "" {
					c.releaseHeld()
				}
				c.spawnDirty = true
				c.live.writeSnapshot()
			})
		}
	}
	// A child loaded before its parent found no parent to report to.
	r.deliverPending()
	r.startHeartbeatSweeper()
	if r.opts.Holders == nil {
		// No holder to wait for.
		r.markScanned()
		r.rejoinOnce.Do(func() { close(r.rejoined) })
		return
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.opts.Holders.Scan()
		r.markScanned()
		r.holdersScanned()
		r.rejoinOnce.Do(func() { close(r.rejoined) })
	}()
}

func (r *Runner) startupReady() bool {
	if r == nil {
		return true
	}
	r.startupMu.RLock()
	startupLoaded := r.startupLoaded
	r.startupMu.RUnlock()
	if startupLoaded == nil {
		return true
	}
	select {
	case <-startupLoaded:
		return true
	default:
		return false
	}
}

// loadClosed lists closed (or handed-off) record rec at Start.
func (r *Runner) loadClosed(rec Record) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed[rec.ReplyID] = rec
	r.indexChildLocked(rec)
	st := r.closedState(rec) // prime immutable presentation once at startup
	if r.closedRows == nil {
		r.closedRows = map[string]State{}
	}
	r.closedRows[rec.ReplyID] = st
}

// closeAtStart closes open record rec at Start, with no loop started, and
// reports whether it did.
func (r *Runner) closeAtStart(rec Record) bool {
	now := r.opts.Now()
	rec.ClosedAt = now.UTC().Format(time.RFC3339)
	// An orphan's cut-off turn ends here, as close ends a running one.
	finishActiveTurn(&rec, now.UnixMilli())
	rec.TurnOpen = false
	if err := writeRecord(wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID), rec); err != nil {
		fmt.Fprintf(r.opts.Log, "agent %s: record write: %v\n", rec.ReplyID, err)
		return false
	}
	if err := wsdashboard.WriteRegistration(r.opts.Dir, registration(rec, now)); err != nil {
		fmt.Fprintf(r.opts.Log, "agent %s: registration write: %v\n", rec.ReplyID, err)
	}
	r.notify(rec.ReplyID, rec.Root)
	r.loadClosed(rec)
	return true
}

// orphansAtStart picks the open spawned children a daemon start closes
// instead of serving (closeAtStart): one whose conversation parent is
// closed or gone, so no report of it reaches one, or that descends from
// one. A finished child is not closed: it sleeps (agent_tools.go).
func orphansAtStart(open []Record) map[string]bool {
	isOpen := map[string]bool{}
	for _, rec := range open {
		isOpen[rec.ReplyID] = true
	}
	done := map[string]bool{}
	stays := func(id string) bool { return isOpen[id] && !done[id] }
	for changed := true; changed; {
		changed = false
		for _, rec := range open {
			id := rec.ReplyID
			// Legacy direct non-Pi children keep their existing next-launch
			// admission refusal when a parent disappears. Held workers of
			// every backend use the former Pi orphan cleanup.
			if done[id] || rec.Lead || rec.Parent == nil || (rec.Backend != BackendPi && rec.HolderID == "") {
				continue
			}
			if rec.Parent.Kind == ParentConversation && !stays(rec.Parent.ReplyID) {
				done[id], changed = true, true
			}
		}
	}
	return done
}

// markScanned closes scanned, once however often it is called.
func (r *Runner) markScanned() {
	r.scanOnce.Do(func() { close(r.scanned) })
}

// holdersScanned follows the first holder scan of this daemon: each
// rejoined process learns where its replay ends (or that its holder is
// gone), and a lead holder no conversation runs under is ended, so no
// second process of a session stays behind a crash between a spawn and its
// record.
func (r *Runner) holdersScanned() {
	// Listed before the conversations are taken and asked: a conversation
	// opened after the listing launched no holder in it, and a holder a
	// start launched meanwhile is in its record by the time the
	// conversation's loop answers.
	listed := r.opts.Holders.List().Holders
	r.mu.Lock()
	convs := make([]*conv, 0, len(r.convs))
	for _, c := range r.convs {
		convs = append(convs, c)
	}
	r.mu.Unlock()
	owned := map[string]bool{}
	for _, c := range convs {
		c.do(func() {
			c.checkReattach()
			if c.hidden {
				c.stopProcess("")
				if err := c.retireHolder(); err != nil {
					c.setStatus(func() { c.lastErr = "archive process: " + err.Error() })
				}
			}
			if c.proc == nil {
				// A start kick put off until the scan (a rejoin that failed).
				c.kick()
			}
			if id := c.rec.HolderID; id != "" {
				owned[id] = true
			}
		})
	}
	select {
	case <-r.quit:
		// The daemon closes: a loop that ended on it let its process go
		// under its holder for the next daemon (letGo) and answered nothing
		// above, so its holder would read as no conversation's. The next
		// daemon's first scan sweeps instead.
		return
	default:
	}
	for _, info := range listed {
		var meta holderSession
		if json.Unmarshal(info.Session, &meta) != nil || (meta.Kind != holderKindLead && meta.Kind != holderKindPiWorker && meta.Kind != holderKindAgentWorker) || owned[info.ID] || r.holders.isRejected(info.ID) {
			continue
		}
		fmt.Fprintf(r.opts.Log, "agent: ending conversation holder %s of %s: no open conversation runs under it\n", info.ID, meta.ReplyID)
		if err := r.opts.Holders.Stop(info.ID); err != nil {
			fmt.Fprintf(r.opts.Log, "agent: stop holder %s: %v\n", info.ID, err)
		}
	}
}

// scanDone reports whether the holder manager's first scan of this daemon
// returned. Before it the manager lists only the holders attached so far,
// so a holder a record names that is missing from the listing may still
// run: nothing may take it for gone, or start a second process beside it.
func (r *Runner) scanDone() bool {
	select {
	case <-r.scanned:
		return true
	default:
		return false
	}
}

// Close ends every conversation's process and stops the runner; the
// conversations stay open on disk and resume lazily under the next daemon.
// A lead conversation's process is let go, not ended: it runs on under its
// session holder, and the next daemon rejoins it.
