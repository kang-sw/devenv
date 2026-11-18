package wsagent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

func (c *conv) handle(ev Event) {
	quiet := ev.Replay
	if ev.Seq > 0 && !ev.Replay {
		c.lineOpen = ev.Seq
		if ev.LineEnd {
			c.lineOpen = 0
		}
	}
	switch {
	case ev.LineEnd:
		c.lineEnd(ev.Seq)
	case ev.ReplayDone:
		c.replayDone(ev.ReplayLost)
	case ev.ReplayGap:
		c.summaryGap = true
		if c.replaying {
			c.replayGap, c.replayBoundary = true, false
		}
	case ev.SessionID != "":
		if ev.SessionID != c.rec.SessionID {
			c.update(func(rec *Record) { rec.SessionID = ev.SessionID })
		}
	case ev.ReasoningSummary != nil:
		c.summaryProjection(ev, quiet)
	case ev.Harness != nil:
		c.harnessReported(*ev.Harness, quiet)
	case ev.Live != nil:
		if ev.Live.Kind == wslive.KindCommands {
			// The list is offered without the commands of unsupported items;
			// the dashboard runs only a command the list holds.
			live := *ev.Live
			c.commands = slices.Clone(live.Commands)
			live.Commands = allowedCommands(live.Commands, c.caps)
			ev.Live = &live
		}
		if !quiet && ev.Live.Kind == wslive.KindCompact && ev.Live.Phase == wslive.CompactStart {
			// The requested compaction started: its own turn holds kick
			// until it ends.
			c.holdForLever(false)
		}
		if ev.Live.Kind == wslive.KindReply && ev.Live.Final && !(quiet && c.rec.Parent != nil && ev.Seq <= c.rec.TurnStartSeq) {
			c.reply = ev.Live.Text
		}
		c.apply(*ev.Live, quiet)
	case ev.Card != nil:
		if !quiet && c.live.card(*ev.Card) {
			c.ringDirty = true
			c.r.notify(c.rec.ReplyID, c.rec.Root)
		}
	case ev.UI != nil:
		if !quiet && c.live.ui(*ev.UI) {
			c.ringDirty = true
			c.r.notify(c.rec.ReplyID, c.rec.Root)
		}
	case ev.TrackerWrite:
		if f := c.r.opts.TrackerChanged; f != nil && !quiet {
			f()
		}
	case ev.Fatal != nil:
		if quiet {
			return
		}
		if c.replaying && c.rec.Parent != nil {
			c.processExited()
			return
		}
		c.setStatus(func() { c.lastErr = ev.Fatal.Error() })
		fmt.Fprintf(c.r.opts.Log, "agent %s: %v\n", c.rec.ReplyID, ev.Fatal)
		c.stopProcess("")
	case ev.TurnStart:
		c.summaryStart(ev)
		if quiet && c.rec.Parent != nil && ev.Seq <= c.rec.TurnStartSeq {
			return
		}
		c.replayBoundary = c.replayBoundary || c.replaying
		c.wrapTurnSeen = c.wrapTurnSeen || (c.wrapHold && !quiet)
		c.beginTurn(quiet)
	case ev.TurnEnd:
		if quiet && c.rec.Parent != nil && ev.Seq <= c.rec.TurnStartSeq {
			return
		}
		c.replayBoundary = c.replayBoundary || c.replaying
		if !quiet {
			// Every turn end lifts an earlier hold; a lever turn arms anew.
			c.holdForLever(ev.CompactRequested && !c.interruptAsked)
			c.interruptAsked = false
		}
		c.summaryReason = ev.TurnReason
		c.live.summaryNaturalEnd = true
		c.endTurn(ev.TurnError, quiet)
		c.live.summaryNaturalEnd = false
		c.live.summarySource, c.summaryReason, c.summaryGap = "", "", false
	case ev.Approval != nil:
		c.addApproval(*ev.Approval, quiet)
	case ev.ApprovalDone != "":
		c.dropApproval(ev.ApprovalDone, quiet)
		c.forgetAnswered(ev.ApprovalDone)
	case ev.PermissionMode != "":
		if ev.PermissionMode != c.activeMode {
			c.setStatus(func() { c.activeMode = ev.PermissionMode })
			if c.rec.Lead && !quiet {
				if level, _, err := c.r.leadPermission(c.record(), c.dir); err == nil {
					c.permissionNudge(level, false, ev.PermissionMode)
				}
			}
			c.changed(quiet)
		}
		// The mode the harness runs in is the session's own: its next
		// launch resumes in it. A change of the user's that waits to be
		// applied stands until it is.
		if c.rec.Lead && !quiet && !c.dirtyMode && ev.PermissionMode != c.rec.PermissionMode && slices.Contains(permissionModes(c.rec.Backend, []map[string]BackendRegistration{c.r.opts.Backends}), ev.PermissionMode) {
			c.update(func(rec *Record) { rec.PermissionMode = ev.PermissionMode })
			c.changed(quiet)
		}
	case ev.HarnessState != "":
		c.harnessState = ev.HarnessState
		c.checkAction(quiet)
	case ev.SteerAnswer != nil:
		if ev.Seq > 0 && ev.Seq <= c.rec.SteerAnswerSeq {
			return
		}
		c.steerAnswered(ev.SteerAnswer.Refusal)
		c.update(func(rec *Record) { rec.SteerAnswers, rec.SteerAnswerSeq = c.steerAnswers, ev.Seq })
	case ev.UserTaken != "":
		c.steerTaken(ev.UserTaken)
	case ev.CommandError != "":
		// Replay rebuilds lifecycle state, not transient command diagnostics.
		if !quiet {
			c.setStatus(func() { c.lastErr = ev.CommandError })
			c.changed(false)
		}
	case ev.Notice != "":
		if !quiet {
			fmt.Fprintf(c.r.opts.Log, "agent %s: %s\n", c.rec.ReplyID, ev.Notice)
		}
	case ev.BackgroundTasks != nil:
		if n := *ev.BackgroundTasks; n != c.bgTasks || !sameCount(c.background, ev.Background) || !sameCount(c.monitors, ev.Monitors) {
			c.setStatus(func() {
				c.bgTasks = n
				c.background, c.monitors = copyCount(ev.Background), copyCount(ev.Monitors)
			})
			c.syncHost(quiet)
			c.changed(quiet)
			if !quiet {
				c.watched()
			}
			if n == 0 && !c.turn && !quiet {
				c.armIdle()
				c.restartIfDue()
			}
		}
	case ev.Alert != nil:
		c.alert(*ev.Alert, quiet)
	case ev.Retry != nil || ev.RetryEnd:
		c.retry = ev.Retry
		c.syncHost(quiet)
	case ev.MCPServers != nil:
		c.mcpReported(ev.MCPServers, quiet)
	case ev.MCPCheck:
		if !quiet {
			c.checkMCP(nil)
		}
	}
}

// changed tells the daemon a lead's state outside its live snapshot moved
// (pending approvals, the reported mode, the background task count).
func (c *conv) changed(quiet bool) {
	if !quiet {
		c.r.notify(c.rec.ReplyID, c.rec.Root)
	}
}

// beginTurn opens a turn the harness began with no send of the runner's: a
// background task's completion, a compaction, a pushed message. A turn the
// runner's own send opened is already running.
func (c *conv) beginTurn(quiet bool) {
	if c.turn {
		return
	}
	c.stopIdle()
	c.setStatus(func() {
		c.turn = true
		if !quiet {
			c.lastErr = ""
		}
	})
	c.apply(wslive.Event{Kind: wslive.KindTurnStart}, quiet)
	c.reply, c.replayTurnError = "", ""
	if !quiet {
		// Not the runner's send: the turn holds no new message, so it
		// keeps the last turn's mark.
		c.reply = ""
		c.hold(nil)
		if c.rec.Parent != nil {
			c.update(func(rec *Record) {
				rec.TurnOpen, rec.HeldTurn = true, nil
				rec.TurnStartSeq = c.seq
			})
		}
		c.watched()
	}
}

// endTurn closes the running turn, applies settings changed during it, and
// submits the messages held meanwhile.
func (c *conv) endTurn(turnErr string, quiet bool) {
	if c.wrapHold && !c.rec.Lead && c.rec.Parent != nil && !quiet {
		// A gap while streaming is reconciled at agent_settled, not left
		// dependent on acknowledgements that may have left the ring.
		c.probeWorkerRecovery()
		return
	}
	if !c.turn && !c.rec.Lead {
		return
	}
	ran := c.turn
	if c.turn {
		c.setStatus(func() { c.turn = false })
		c.apply(wslive.Event{Kind: wslive.KindTurnEnd, Reason: c.summaryReason}, quiet)
	}
	c.turnSettled(turnErr, quiet)
	if c.replaying && c.rec.Parent != nil {
		c.replayTurnError = turnErr
	}
	if quiet {
		return
	}
	// Observers must see the terminal diagnostic in the same emission as
	// the settled lifecycle, including mapped Pi agent_settled failures.
	if turnErr != "" {
		c.setStatus(func() { c.lastErr = turnErr })
	}
	if ran && !c.replaying {
		status := TurnDone
		if turnErr != "" {
			status = TurnFailed
		}
		c.settleTurn(status, turnErr, c.turnMark)
	}
	if c.replaying {
		return
	}
	c.wrapHold = false
	// A child's report steered into the turn that the turn did not take
	// goes as the next one (children.go).
	c.unsteer()
	c.applySettings()
	c.restartIfDue()
	c.kick()
	if !c.turn {
		c.armIdle()
	}
}

// addApproval records a waiting tool permission prompt; the session shows
// as awaiting approval while any waits.
func (c *conv) addApproval(a ApprovalRequest, quiet bool) {
	for _, have := range c.approvals {
		if have.ID == a.ID {
			return
		}
	}
	if slices.Contains(c.rec.Answered, a.ID) {
		// A replay of a prompt an earlier daemon already answered.
		return
	}
	c.setStatus(func() { c.approvals = append(c.approvals, a) })
	c.checkAction(quiet)
	c.showApprovals(quiet)
	if !quiet && (c.rec.agyBypass() || (!c.rec.Lead && c.r.opts.Backends[c.rec.Backend].WorkerAutoApprove)) {
		// Use the same answer path as a user, including at-most-once
		// tracking and a fatal write failure. The request remains traced.
		if err := c.answerApproval(a.ID, true, ""); err != nil {
			fmt.Fprintf(c.r.opts.Log, "agent %s: auto-allow permission: %v\n", c.rec.ReplyID, err)
		}
	}
}

// actionGrace is how long the harness may report that it waits on the user
// with no prompt for the runner to show before the session says so. The
// state line arrives just before its prompt's own line, and stays for a
// moment after a prompt's answer.
var actionGrace = 2 * time.Second

// actionNeededText is State.ActionNeeded for a wait nobody can answer here.
const actionNeededText = "The harness waits on an answer this page has no prompt for. Interrupt the turn or stop the session to go on."

// actionNeededTool names that wait in the live state's permission wait.
const actionNeededTool = "prompt"

// checkAction follows the harness's wait on the user: while it waits with
// no prompt of the runner's to answer (one this process inherited, or a
// request the runner has no form for), the session must not look like it
// works. The wait shows after actionGrace, and ends with the state, with a
// prompt's arrival, or with the process.
func (c *conv) checkAction(quiet bool) {
	if c.harnessState == HarnessWaiting && len(c.approvals) == 0 && c.proc != nil {
		if c.actionNeeded == "" && c.actionTimer == nil && !quiet {
			c.actionTimer = time.NewTimer(actionGrace)
		}
		return
	}
	if c.actionTimer != nil {
		c.actionTimer.Stop()
		c.actionTimer = nil
	}
	if c.actionNeeded != "" {
		c.setStatus(func() { c.actionNeeded = "" })
		c.showApprovals(quiet)
	}
}

// actionDue shows the wait once its grace has passed.
func (c *conv) actionDue() {
	if c.harnessState != HarnessWaiting || len(c.approvals) > 0 || c.proc == nil || c.actionNeeded != "" {
		return
	}
	fmt.Fprintf(c.r.opts.Log, "agent %s: the harness waits on the user (requires_action) with no permission prompt to answer\n", c.rec.ReplyID)
	c.setStatus(func() { c.actionNeeded = actionNeededText })
	c.showApprovals(false)
}

// dropApproval forgets a prompt that needs no answer any more, and reports
// whether it was waiting.
func (c *conv) dropApproval(id string, quiet bool) bool {
	for i, have := range c.approvals {
		if have.ID == id {
			c.setStatus(func() { c.approvals = append(c.approvals[:i:i], c.approvals[i+1:]...) })
			c.checkAction(quiet)
			c.showApprovals(quiet)
			return true
		}
	}
	return false
}

// forgetAnswered drops a settled prompt from the record's answered list
// ("" drops them all: their process is gone).
func (c *conv) forgetAnswered(id string) {
	if len(c.rec.Answered) == 0 || (id != "" && !slices.Contains(c.rec.Answered, id)) {
		return
	}
	c.update(func(rec *Record) {
		if id == "" {
			rec.Answered = nil
			return
		}
		rec.Answered = slices.DeleteFunc(slices.Clone(rec.Answered), func(have string) bool { return have == id })
	})
}

// clearApprovals settles every waiting prompt with no answer: the process
// they came from is gone.
func (c *conv) clearApprovals() {
	c.forgetAnswered("")
	if len(c.approvals) == 0 {
		return
	}
	c.setStatus(func() { c.approvals = nil })
	c.showApprovals(false)
}

// showApprovals carries the oldest waiting prompt to the live state as its
// permission wait, or a wait with no prompt to answer (actionNeeded), or
// ends the wait when neither is left.
func (c *conv) showApprovals(quiet bool) {
	ev := wslive.Event{Kind: wslive.KindPermission, Clear: true}
	if c.actionNeeded != "" {
		ev = wslive.Event{Kind: wslive.KindPermission, Tool: actionNeededTool}
	}
	if len(c.approvals) > 0 {
		a := c.approvals[0]
		var args any
		if json.Unmarshal(a.Input, &args) != nil {
			args = nil
		}
		ev = wslive.Event{Kind: wslive.KindPermission, Tool: a.Tool, Args: args}
	}
	c.apply(ev, quiet)
	c.changed(quiet)
}

// answerApproval writes the answer to the waiting prompt id. The prompt is
// forgotten before the write, so it is answered at most once.
func (c *conv) answerApproval(id string, allow bool, message string) error {
	conflict := func(msg string) error { return &requestError{status: http.StatusConflict, msg: msg} }
	if c.replaying {
		return conflict("the session is rejoining its process; answer again in a moment")
	}
	var req *ApprovalRequest
	for i := range c.approvals {
		if c.approvals[i].ID == id {
			a := c.approvals[i]
			req = &a
		}
	}
	answerer := controlsFor(c.proc).approval
	if req == nil || answerer == nil {
		return conflict("that permission prompt is not waiting: it was answered, withdrawn, or its process ended")
	}
	c.dropApproval(id, false)
	c.update(func(rec *Record) { rec.Answered = append(rec.Answered, id) })
	if err := answerer.AnswerApproval(*req, allow, message); err != nil {
		c.setStatus(func() { c.lastErr = "answer the permission prompt: " + err.Error() })
		c.stopProcess("")
		return fmt.Errorf("answer the permission prompt: %w", err)
	}
	return nil
}

// processExited handles a process whose output ended without the runner
// stopping it.
func (c *conv) processExited() {
	rejoining, proc := c.replaying, c.proc
	c.events = nil
	// No compaction follows from a process that is gone.
	c.holdForLever(false)
	c.interruptAsked = false
	if c.replaying && c.rec.Parent != nil {
		// Apply any completed replayed turn before handling an exited child.
		if c.rec.TurnOpen && !c.turn && c.replayBoundary {
			status := TurnDone
			if c.replayTurnError != "" {
				status = TurnFailed
			}
			c.settleTurn(status, c.replayTurnError, c.turnMark)
		} else {
			c.interrupted()
		}
	}
	if proc != nil {
		_ = proc.Close()
		c.releaseHolder(proc)
	}
	c.setStatus(func() {
		c.proc = nil
		if c.lastErr == "" {
			c.lastErr = "the backend process exited"
		}
	})
	c.processEnded()
	if rejoining {
		// The process ended while no daemon ran: what waited goes to a new
		// one.
		c.replaying, c.replayAsked = false, false
		c.kick()
	}
}

// processGone handles a rejoin whose holder no longer exists: the process
// ended with it while no daemon ran.
func (c *conv) processGone() {
	proc := c.proc
	c.events = nil
	if hp, ok := proc.(holderProcess); ok {
		hp.Detach()
	}
	c.setStatus(func() { c.proc = nil })
	c.update(func(rec *Record) { rec.HolderID, rec.HostPID, rec.HolderSeq = "", 0, 0 })
	c.writeRegistration()
	c.replaying, c.replayAsked = false, false
	c.interrupted()
	c.processEnded()
	c.kick()
}

// processEnded settles what ends with a process: its turn, its waiting
// prompts, and what it last reported.
func (c *conv) processEnded() {
	c.stopIdle()
	c.wrapHold = false
	if c.turn {
		c.setStatus(func() { c.turn = false })
		c.apply(wslive.Event{Kind: wslive.KindTurnEnd})
		// A turn the process took with it. A close settles none (its
		// parent ended it), and the daemon's exit leaves it to the next
		// daemon (interrupted).
		if !c.closed && !c.quitting {
			c.settleTurn(TurnFailed, c.lastError(), c.turnMark)
		}
	}
	c.r.revokeToken(c.rec.ReplyID)
	if c.rec.Capability != "" || c.rec.MCPToken != "" {
		c.update(func(rec *Record) { rec.Capability = ""; rec.forgetMCP() })
	}
	if c.requeueSteered() && !c.closed && !c.quitting {
		// The steered reports the process took with it go as the next
		// turn; nothing else would send them.
		c.signalInbox()
	}
	c.clearApprovals()
	c.harnessState = ""
	c.checkAction(false)
	hadBackgroundTasks := c.bgTasks != 0
	c.setStatus(func() {
		c.activeMode, c.bgTasks, c.mcpBroken = "", 0, nil
		c.background, c.monitors = nil, nil
	})
	if hadBackgroundTasks {
		// A settled child's background work may have suppressed its
		// parent's idle timer; ending the process must re-arm it too.
		c.watched()
	}
	c.mcpSeen, c.mcpChecking, c.mcpAgain, c.mcpAgainNames = nil, false, false, nil
	c.retry, c.alerted = nil, false
	c.syncHost(true)
	c.live.endAgents()
	c.live.writeSnapshot()
	c.r.notify(c.rec.ReplyID, c.rec.Root)
}

// releaseHolder forgets the holder of an ended lead process, unless it
// could not be ended: then the record keeps it, and the next start ends it
// before anything runs (retireHolder).
func (c *conv) releaseHolder(proc Process) {
	hp, ok := proc.(holderProcess)
	if !ok || c.rec.HolderID == "" {
		return
	}
	if c.replaying && !c.r.scanDone() {
		// ErrUnknownHolder is not proof of absence before the first scan.
		// Retain the resume/start fence even if another control stops this
		// restored transport while the archived holder is still unscanned.
		return
	}
	if err := hp.StopError(); err != nil {
		c.setStatus(func() { c.lastErr = "end the session's process: " + err.Error() })
		fmt.Fprintf(c.r.opts.Log, "agent %s: stop holder %s: %v\n", c.rec.ReplyID, c.rec.HolderID, err)
		return
	}
	c.seqDirty, c.ringDirty = false, false
	c.update(func(rec *Record) { rec.HolderID, rec.HostPID, rec.HolderSeq = "", 0, 0 })
	c.writeRegistration()
}

// stopProcess ends the process, if any; a running turn ends with it.
func (c *conv) stopProcess(reason string) {
	c.stopIdle()
	if c.proc == nil {
		return
	}
	proc := c.proc
	c.events = nil
	_ = proc.Close()
	c.setStatus(func() { c.proc = nil })
	c.releaseHolder(proc)
	// Close waits for the holder manager's exit write. Record the runner's
	// intent afterwards: EOF may otherwise look like a plain child exit.
	if reason != "" && c.rec.Lead && c.rec.HostPID == 0 {
		if unlock, ok := c.lockRow(); ok {
			reg, err := wsdashboard.ReadRegistration(c.r.opts.Dir, c.rec.ReplyID)
			if err == nil {
				reg.HostExitReason = reason
				reg.HostStoppedAt = c.r.opts.Now().UTC().Format(time.RFC3339)
				err = wsdashboard.WriteRegistration(c.r.opts.Dir, reg)
			}
			unlock()
			if err != nil {
				fmt.Fprintf(c.r.opts.Log, "agent %s: record stop reason: %v\n", c.rec.ReplyID, err)
			}
		}
	}
	if reason != "" {
		fmt.Fprintf(c.r.opts.Log, "agent %s: process stopped: %s\n", c.rec.ReplyID, reason)
	}
	c.replaying, c.replayAsked = false, false
	c.processEnded()
}

// letGo is the daemon's shutdown: a lead's process runs on under its
// holder, with the sequence applied so far in the record for the next
// daemon's rejoin; any other process ends.
func (c *conv) letGo() {
	c.quitting = true
	c.finishLine()
	if hp, ok := c.proc.(holderProcess); ok {
		c.persistSeq()
		if hp.Detach() {
			c.live.writeSnapshot()
			return
		}
	}
	c.stopProcess("")
}

// finishLine applies the rest of a holder line the loop stands in the
// middle of, so the sequence the record keeps covers every entry the line
// wrote and the next daemon's replay writes none of them again. The line's
// events are already on their way; a stream that stalls is not waited for.
func (c *conv) finishLine() {
	wait := time.After(time.Second)
	for c.lineOpen != 0 && c.events != nil {
		select {
		case ev, ok := <-c.events:
			if !ok {
				return
			}
			c.handle(ev)
		case <-wait:
			return
		}
	}
}

// idleNow reports whether the idle timer may stop the process. Read child
// state here too: a spawn notification may still be queued when the timer fires.
func (c *conv) idleNow() bool {
	if c.turn || c.replaying || c.bgTasks != 0 {
		return false
	}
	kind := ParentConversation
	if c.rec.Lead {
		kind = ParentLead
	}
	parent := ParentRef{Kind: kind, ReplyID: c.rec.ReplyID}
	c.r.mu.Lock()
	children := make([]*conv, 0, len(c.r.convs))
	for _, child := range c.r.convs {
		if child != c {
			children = append(children, child)
		}
	}
	c.r.mu.Unlock()
	for _, child := range children {
		child.mu.Lock()
		// Send coverage protects initial queuing, but not input retained
		// for retry after a settled launch failure. A held result still
		// waits for the child's own descendants.
		// A child whose start admission refused sleeps (wakeRefused).
		active := child.rec.Parent != nil && *child.rec.Parent == parent && !child.closed &&
			(child.turn || child.bgTasks != 0 || child.holding || !child.wakeRefused && child.sends > max(child.settled.mark, child.voided))
		child.mu.Unlock()
		if active {
			return false
		}
	}
	return true
}

// checkInterrupt takes the inbox's interrupt requests and interrupts the
// running turn when there is one; with no turn they are dropped.
func (c *conv) checkInterrupt() {
	recs, err := wsinbox.DrainInterrupt(c.inbox)
	if err != nil || len(recs) == 0 || !c.turn || c.proc == nil {
		return
	}
	c.interruptAsked = true
	if err := c.proc.Interrupt(); err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: interrupt: %v\n", c.rec.ReplyID, err)
		c.stopProcess("")
	}
}

// leverHoldCap bounds how long queued input waits, after a turn that
// carried a successful compaction lever call, for that compaction to start.
const leverHoldCap = 5 * time.Second

// holdForLever lifts the lever hold, ending its cap timer, and with arm
// starts it anew. Lifting submits nothing itself: the caller's own kick
// (a turn end's) or the compaction turn's end does.
func (c *conv) holdForLever(arm bool) {
	if c.leverStop != nil {
		c.leverStop()
	}
	c.leverC, c.leverStop = nil, nil
	if arm {
		c.leverC, c.leverStop = c.r.opts.Timer(leverHoldCap)
	}
}

func (c *conv) setSettings(model, effort *string) error {
	c.update(func(rec *Record) {
		if model != nil && *model != rec.Model {
			rec.Model = *model
			c.dirtyModel = true
		}
		if effort != nil && *effort != rec.Effort {
			rec.Effort = *effort
			c.dirtyEffort = true
		}
	})
	if !c.turn {
		c.applySettings()
	}
	return nil
}

// applySettings carries changed settings to the process at a turn
// boundary: in-stream when the backend can, and only otherwise by stopping
// the process so the next turn restarts it with the new values on the same
// backend session. A restart ends the session's background work with the
// process, so it is the last resort.
func (c *conv) applySettings() {
	model, effort, mode, reconcile := c.dirtyModel, c.dirtyEffort, c.dirtyMode, c.reconcileMode
	if c.replaying {
		return
	}
	c.dirtyModel, c.dirtyEffort, c.dirtyMode, c.reconcileMode = false, false, false, false
	if c.proc == nil || (!model && !effort && !mode) {
		return
	}
	rec := c.record()
	restart := false
	controls := controlsFor(c.proc)
	if model && controls.SetModel(rec.Model) != nil {
		restart = true
	}
	if effort && !restart && controls.SetEffort(rec.Effort) != nil {
		restart = true
	}
	if mode && !restart {
		level, _, err := c.r.leadPermission(rec, c.dir)
		if err != nil {
			c.setStatus(func() { c.lastErr = err.Error() })
			return
		}
		// The accepted native target is already in Record. Common selection
		// writes its mapping there; a permitted native control keeps its literal
		// mode. Only an actual new launch remaps explicit common intent.
		if err := controls.setPermissionMode(rec.launchPermissionMode()); err != nil {
			if reconcile {
				fmt.Fprintf(c.r.opts.Log, "agent %s: permission reconciliation: %v\n", rec.ReplyID, err)
				c.permissionNudge(level, false, c.activeMode)
			} else {
				restart = true
			}
		}
	}
	if restart {
		c.stopProcess("")
	}
}

func (c *conv) armIdle() {
	c.stopIdle()
	if c.proc != nil && c.idleAfter > 0 {
		c.idle = time.NewTimer(c.idleAfter)
	}
}

func (c *conv) stopIdle() {
	if c.idle != nil {
		c.idle.Stop()
		c.idle = nil
	}
}

// close ends the conversation's current lifetime. It marks it closed
// first, under mu, so a Send either wrote before (and its message is
// dropped below with the other held ones) or is refused.
func (c *conv) close() { c.closeIf(nil) }

// closeIf is close when done (nil for always) holds in the same mu hold
// that marks the conversation closed, and reports whether it closed. It
// runs in the loop.
func (c *conv) closeIf(done func() bool) bool {
	c.wmu.Lock()
	c.mu.Lock()
	if done != nil && !done() {
		c.mu.Unlock()
		c.wmu.Unlock()
		return false
	}
	c.rec.ClosedAt = c.r.opts.Now().UTC().Format(time.RFC3339)
	c.closed = true
	rec := c.rec
	c.mu.Unlock()
	if err := writeRecord(c.dir, rec); err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: record write: %v\n", rec.ReplyID, err)
	}
	c.wmu.Unlock()
	c.finishClose()
	return true
}

// finishClose is the rest of a close, once the conversation is marked
// closed. It runs in the loop.
func (c *conv) finishClose() {
	c.stopProcess("")
	if len(c.rec.Mail) > 0 || len(c.rec.Reports) > 0 {
		c.update(func(rec *Record) { rec.Mail, rec.Reports = nil, nil })
	}
	c.r.revokeToken(c.rec.ReplyID)
	c.hold(nil)
	c.dropHeld()
	c.live.setAdapter("")
	c.writeRegistration()
	if c.rec.TurnOpen {
		c.update(func(rec *Record) {
			finishActiveTurn(rec, c.r.opts.Now().UnixMilli())
			rec.TurnOpen = false
		})
	}
	c.signalSettled()
	c.watched()
}

// harnessReported resolves the backend's catalog profile at the version
// the process reported, demotes the items its init shape does not support,
// and offers the command list again under the result. A backend with no
// profile stays handle-only: no grades, no demotion, no warning.
func (c *conv) harnessReported(report HarnessReport, quiet bool) {
	profile := c.r.opts.Backends[c.rec.Backend].Catalog
	if profile == nil {
		return
	}
	role := RoleWorker
	if c.rec.Lead {
		role = RoleLead
	}
	caps := profile.Resolve(role, report.Version)
	caps.demote(report.Demoted)
	if c.caps != nil && reflect.DeepEqual(*c.caps, caps) {
		return
	}
	if !quiet && (caps.Unverified || len(caps.Demoted) > 0) {
		fmt.Fprintf(c.r.opts.Log, "agent %s: harness version %q (verified %s..%s) runs on %s's capabilities; demoted by its init: %v\n",
			c.rec.ReplyID, report.Version, caps.Oldest, caps.Newest, caps.ProfileVersion, caps.Demoted)
	}
	before := allowedCommands(c.commands, c.caps)
	c.setStatus(func() { c.caps = &caps })
	if c.commands != nil {
		if after := allowedCommands(c.commands, c.caps); !slices.Equal(after, before) {
			c.apply(wslive.Event{Kind: wslive.KindCommands, Commands: after}, quiet)
		}
	}
	c.changed(quiet)
}
