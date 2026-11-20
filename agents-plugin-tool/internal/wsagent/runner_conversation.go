// runner_conversation.go holds conversation lifecycle and registration.
package wsagent

import (
	"fmt"
	"slices"
	"sync"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

func (r *Runner) Close() {
	r.once.Do(func() { close(r.quit) })
	r.wg.Wait()
}

// InboxChanged tells the runner a conversation's inbox changed from the
// session page (a message, cancel, steer, or interrupt), so it delivers or
// interrupts as for its own routes. It returns at once: the loop does the
// work (inboxChanged). An unknown reply-id is ignored.
func (r *Runner) InboxChanged(replyID string) {
	if c := r.conv(replyID); c != nil {
		c.signalInbox()
	}
}

func (r *Runner) conv(replyID string) *conv {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.convs[replyID]
}

// summaryLock returns replyID's summary writer mutex, the same one for
// every conversation generation of the session. Only the map guard is
// taken, and it is released before the caller locks the mutex.
func (r *Runner) summaryLock(replyID string) *sync.Mutex {
	r.summaryMu.Lock()
	defer r.summaryMu.Unlock()
	if r.summaryLocks == nil {
		r.summaryLocks = map[string]*sync.Mutex{}
	}
	l := r.summaryLocks[replyID]
	if l == nil {
		l = &sync.Mutex{}
		r.summaryLocks[replyID] = l
	}
	return l
}

// setSummaryOwner makes c replyID's summary writer generation. It waits
// for a writer of the session that holds the summary lock (a ring or
// snapshot write, or a heartbeat sweep), so the owner never changes under
// one.
func (r *Runner) setSummaryOwner(replyID string, c *conv) {
	l := r.summaryLock(replyID)
	l.Lock()
	defer l.Unlock()
	r.summaryMu.Lock()
	defer r.summaryMu.Unlock()
	if r.summaryOwners == nil {
		r.summaryOwners = map[string]*conv{}
	}
	r.summaryOwners[replyID] = c
}

// summaryOwner reads replyID's summary writer generation, unpinned: it can
// change once this returns. Use lockSummaryOwner to decide a write on it.
func (r *Runner) summaryOwner(replyID string) *conv {
	r.summaryMu.Lock()
	defer r.summaryMu.Unlock()
	return r.summaryOwners[replyID]
}

// startConv starts serving rec. It writes the registration and the
// snapshot, and tells the daemon, before it takes r.mu to list the
// conversation, so no other conversation's Get, List, or act waits on
// those writes; r.mu is not held. seed, when not empty, is the command
// list the first snapshot shows (lastCommands); a process's own list
// replaces it.
func (r *Runner) startConv(rec Record, seed []wslive.SlashCommand) *conv {
	return r.startConvWith(rec, seed, false, nil, false)
}

func (r *Runner) startConvAtStartup(rec Record, seed []wslive.SlashCommand) *conv {
	return r.startConvWith(rec, seed, false, nil, true)
}

func (r *Runner) startConvWith(rec Record, seed []wslive.SlashCommand, spawnPending bool, claimWidth func(), startupRestore bool) *conv {
	dir := wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID)
	c := &conv{
		r:            r,
		dir:          dir,
		inbox:        wsinbox.Dir(dir),
		rec:          rec,
		live:         newLiveWriter(dir, rec.WSKey, rec.Backend, r.opts.Now, r.opts.Log),
		reqs:         make(chan func()),
		inboxC:       make(chan struct{}, 1),
		spawnC:       make(chan struct{}, 1),
		spawnDirty:   true,
		spawnPending: spawnPending,
		mailC:        make(chan struct{}, 1),
		done:         make(chan struct{}),
		settleC:      make(chan struct{}),
	}
	c.refreshAmbient()
	if !rec.Lead {
		if level, source, err := r.currentWorkerPermission(rec); err == nil {
			c.workerPermissionLevel = level
			c.workerPermissionSource = source
		} else {
			c.lastErr = appendDiagnostic(c.lastErr, "worker permission selection: "+err.Error())
		}
	}
	if startupRestore && r.opts.RestoreBundle != nil {
		// Install the marker before the loop or any inbox/mail signal can
		// kick. canKick's scan/replay gates then order delivery after holder
		// reconciliation without admitting sessions spawned during the scan.
		c.compactNote = restartRestoreMarker
	}
	c.refreshHeld()
	if rec.Lead {
		_, _, _ = refreshPermissionSelection(dir)
		r.setSummaryOwner(rec.ReplyID, c)
		c.live.owner = c.lockSummaryWriter
	}
	if rec.Lead {
		hidden, err := wsdashboard.ReadSessionHidden(r.opts.Dir, rec.ReplyID)
		if err != nil {
			// Fail closed before any startup inbox/mail/replay signal can kick.
			hidden = true
			c.lastErr = "read session visibility: " + err.Error()
			fmt.Fprintf(r.opts.Log, "agent %s: %s\n", rec.ReplyID, c.lastErr)
		}
		c.hidden = hidden
	}
	c.live.wake = func() {
		if slot := c.live.currentSlot(); slot != nil {
			slot.RingAppended()
		} else {
			r.notify(rec.ReplyID, rec.Root)
		}
	}
	c.live.beforeSnapshot = c.seedSpawnRows
	c.observations = c.live.st.ObservedResponses("")
	own := c.live.st.Snapshot(r.opts.Now().UnixMilli())
	if own.Lead.Observed != nil {
		observed := own.Lead.Observed.Current
		c.observed = &observed
	}
	if own.Lead.OutputRate != nil {
		rate := own.Lead.OutputRate.OutputTps
		c.outputTps = &rate
	}
	if usage := own.Usage; usage != nil && usage.Context != nil && usage.Context.Tokens != nil {
		tokens := float64(*usage.Context.Tokens)
		c.contextTokens = &tokens
	}
	if seed == nil && rec.Lead {
		// A rejoined lead's list is its snapshot's, which an earlier daemon
		// may have written without this runner's playbook commands.
		if merged := r.withPlaybookCommands(rec, own.Commands); !slices.Equal(merged, own.Commands) {
			seed = merged
		}
	}
	if len(seed) > 0 {
		c.live.applyQuiet(wslive.Event{Kind: wslive.KindCommands, Commands: seed})
	}
	c.sends, c.drained, c.turnMark, c.voided = rec.Sends, rec.Drained, rec.TurnMark, rec.Voided
	c.heldRes, c.holding = rec.HeldTurn, rec.HeldTurn != nil
	if c.heldRes != nil {
		c.heldRes.mark = rec.TurnMark
	}
	c.steered = slices.Clone(rec.Steered)
	for i := range c.steered {
		c.steered[i].Mark = rec.SteerMarks[c.steered[i].Seq]
	}
	c.steerSeq, c.steerAnswers = rec.SteerSeq, rec.SteerAnswers
	if last := rec.LastTurn; last != nil {
		// Persisted coverage keeps a wait for queued input from taking a
		// prior settlement. Legacy records retain their zero-count behavior.
		c.settled, c.settledReport = last.Result, last.ID
		c.settled.n, c.settled.mark = 1, last.Mark
		if !rec.TurnOpen && (last.Result.Status == TurnFailed || last.Result.Status == TurnInterrupted) {
			c.lastErr = last.Result.Error
		}
	}
	c.writeRegistration()
	// A row a terminal login took while no daemon ran: the loop lets the
	// session go before it takes anything else (settle), and nothing below
	// starts for it.
	foreign := c.isForeign()
	if !foreign {
		if r.opts.LiveHost != nil {
			if slot, ok := r.opts.LiveHost.OpenHostedSlot(dir); ok {
				c.live.setSlot(slot)
				if folder, ok := slot.(wsdashboard.ObservedFoldSlot); ok && c.live.observedSeeded {
					folder.FoldsObserved()
				}
			}
		}
		if rec.Lead {
			// Bind after the slot is fixed: summary retries can wake off-loop.
			c.live.summaryStore().SetWake(c.live.wake)
		}
		c.live.writeSnapshot()
	}
	if rec.HolderID != "" && !foreign {
		if rec.Parent != nil && rec.TurnOpen {
			c.turn = true
			c.live.applyQuiet(wslive.Event{Kind: wslive.KindTurnStart})
		}
		// Rejoined before the loop runs: what the loop takes first (waiting
		// mail, a message from the page) would otherwise start a second
		// process, and end this one to do it.
		c.reattach()
	}
	r.mu.Lock()
	r.convs[rec.ReplyID] = c
	r.indexChildLocked(rec)
	delete(r.closed, rec.ReplyID)
	delete(r.closedRows, rec.ReplyID)
	if claimWidth != nil {
		claimWidth()
	}
	r.wg.Add(1)
	go c.loop()
	r.mu.Unlock()
	// Do not expose a previous LastTurn while startup still reconciles the
	// later TurnOpen. The loop reconciles before it accepts this request.
	c.do(func() {})
	if (rec.Lead || rec.Parent != nil) && r.opts.Mail != nil && !foreign {
		// Mail that waited while no daemon ran is found by the first check.
		// A spawned child is watched asleep too: mail to its address wakes
		// it.
		c.stopMail = r.opts.Mail.Watch(rec.Root, rec.WSKey, c.signalMail)
		c.signalMail()
	}
	if (len(rec.Queued) > 0 || len(rec.Reports) > 0) && !foreign {
		// Drained input an earlier daemon queued: nothing else would send
		// it before the next message. A rejoin's replay end sends it
		// instead (kick waits for it).
		c.signalInbox()
	}
	return c
}

// requestError is an error a route answers with its status.
