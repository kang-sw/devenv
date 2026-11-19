package wsagent

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// conv is one open conversation. Its loop goroutine owns the process and
// every write but Send's inbox write; other goroutines read through state(),
// act through do(), and tell the loop of an inbox change through
// signalInbox().
type conv struct {
	r             *Runner
	dir           string
	inbox         string
	live          *liveWriter
	summaryGap    bool
	summaryReason string
	reqs          chan func()
	// inboxC (one slot) tells the loop the inbox changed (signalInbox).
	inboxC chan struct{}
	done   chan struct{}

	// mu guards what state() reads: rec and the status fields below. Send
	// holds it across its closed check and inbox write; close sets closed
	// under it before it drops the held messages.
	mu            sync.Mutex
	rec           Record
	proc          Process
	turn          bool
	lastErr       string
	closed        bool
	held          int // inbox messages, maintained at inbox changes/drains under mu
	hidden        bool
	contextTokens *float64
	outputTps     *float64
	observed      *wslive.ModelObservation
	observations  []wslive.ModelObservation
	batches       []wslive.BatchView
	// rejoinKeyPending retries a failed attach's key check on State reads
	// until it succeeds; healthy reads never touch the key store.
	rejoinKeyPending bool
	// approvals are a lead's waiting tool permission prompts, oldest first;
	// activeMode and bgTasks are what its harness last reported.
	approvals  []ApprovalRequest
	activeMode string
	bgTasks    int
	background *int
	monitors   *int
	// actionNeeded is State.ActionNeeded.
	actionNeeded string
	// failure and mcpBroken are State.Failure and State.MCPBroken.
	failure   *wslive.Notice
	mcpBroken []string
	// caps is the catalog profile resolved for the process's harness
	// version and demoted by its init shape (harnessReported); nil for a
	// backend without a profile or before the first report. commands is
	// the harness's last command list before allowedCommands took the
	// commands of unsupported items out of it.
	caps     *Capabilities
	commands []wslive.SlashCommand
	// heldKinds memoizes refreshHeld's reads under mu: held entry name ->
	// whether it is a message (not an interrupt entry).
	heldKinds map[string]bool

	// Compaction state is loop-owned, never persisted per observation.
	compactPending                                                               *compactRequest
	compacting, compactResume, compactCommand                                    bool
	compactC                                                                     <-chan time.Time
	compactStop                                                                  func()
	compactNote                                                                  string
	compactAdvisory, compactHard                                                 wsconfig.Threshold
	compactTokens, compactWindow                                                 float64
	compactPrevTokens, compactPrevWindow                                         int64
	compactUsageFresh, compactAdvised, compactHardIssued, compactAdvisoryEnabled bool
	compactAwaitTurn                                                             bool

	// loop-owned.
	events      <-chan Event
	idle        *time.Timer
	idleAfter   time.Duration
	dirtyModel  bool
	dirtyEffort bool
	dirtyMode   bool
	// reconcileMode marks replay-reconstructed intent, not a new user setting.
	// Failed reconciliation is advisory; it must not kill an unenforceable lead.
	reconcileMode bool
	lastReg       time.Time

	// A lead's or spawned child's own, loop-owned. mailC (one slot) tells
	// the loop the mailbox may hold mail (signalMail); stopMail ends the
	// watch.
	mailC    chan struct{}
	stopMail func()
	// spawnC coalesces daemon-owned descendant changes for this writer.
	spawnC     chan struct{}
	spawnDirty bool
	spawnNext  time.Time
	spawnTimer *time.Timer
	// spawning is set from the row write that precedes a spawn until the
	// spawn settled: the row then names no process and is not stopped, so
	// a terminal login is refused meanwhile (Registration.HostMayRun).
	// foreignRow is set once the session's row was found to be a terminal
	// harness's (lockRow, checkRow); the loop then lets the session go
	// (settle).
	spawning   bool
	foreignRow bool
	// replaying is set from a rejoin of the holder's process (reattach)
	// until its replay ends: no turn is submitted meanwhile, since the
	// replay is what tells whether one runs.
	replaying       bool
	replayAsked     bool
	replayTurnError string
	// replayGap is set when records left the holder's ring during the
	// replay, and replayBoundary when the replay showed a turn begin or
	// end after the last such loss. wrapHold is set from a rejoin that lost
	// records with no turn boundary after them (replayDone) until the
	// harness's next turn end: the turn is taken as running, since its
	// start may be among the lost records, and nothing is submitted. A
	// harness that can tell is asked whether a turn runs (probeWrapHold);
	// wrapTurnSeen marks a turn start seen since the ask.
	replayGap      bool
	replayBoundary bool
	wrapHold       bool
	wrapTurnSeen   bool
	// seq is the last holder record sequence applied whole; seqDirty that
	// the record does not hold it yet. live.ringDirty makes a ring line's
	// end persist it at once.
	seq      uint64
	seqDirty bool
	// lineOpen is the holder record sequence of a line some of whose events
	// are applied and whose LineEnd is not; 0 between lines.
	lineOpen uint64
	// harnessState is the state the harness last reported; actionTimer runs
	// while it waits on the user with no prompt to show (checkAction).
	harnessState string
	actionTimer  *time.Timer
	// leverC is the lever hold's cap timer and leverStop its stop: armed
	// at the end of a turn that carried a successful compaction lever call
	// (Event.CompactRequested), it holds kick until that compaction starts
	// (its CompactStart), leverHoldCap passes, the next turn ends, or the
	// process exits. interruptAsked is set when the runner interrupted the
	// running turn, until its end: an interrupted lever turn does not arm.
	leverC         <-chan time.Time
	leverStop      func()
	interruptAsked bool
	// The process's MCP recovery (runner_recovery.go). mcpSeen are the
	// servers it reported connected; mcpBroken (above) are those that broke
	// and did not come back in-stream, and a restart is due at the next
	// idle point (mcpRestartDue). mcpRestarted outlives the process: the
	// servers an automatic
	// restart was for that have not connected since. mcpChecking runs
	// while a check is off the loop, and mcpAgain asks for one more after
	// it, for mcpAgainNames.
	mcpSeen      map[string]bool
	mcpRestarted map[string]bool
	// mcpStale is a rejoined process whose ws-mcp route (Record.MCPURL)
	// this daemon does not serve; it restarts at the first idle point.
	mcpStale      bool
	mcpChecking   bool
	mcpAgain      bool
	mcpAgainNames []string
	// retry is the API request retry under way; alerted that the running
	// turn brought a typed failure, so its end clears none.
	retry   *wslive.Retry
	alerted bool
	// sendRetried is set when a turn's failed send armed its one automatic
	// retry (kick) and cleared by the next send that succeeds; a send that
	// fails again meanwhile waits for the next inbox or mail signal.
	sendRetried bool
	// systemPrompt is agent.spawn's system prompt (Record.SystemPrompt),
	// read once from the session directory; loop-owned.
	systemPrompt string
	// expiring is set while a child-agent-waiting expiry is dispatched
	// (expireIfDue) and not yet decided; loop-owned.
	expiring bool
	// wakeRefused is set while the last process start was refused by
	// permission admission (childAdmission): the input stays, and the
	// child counts as asleep. Under mu.
	wakeRefused bool

	// Turn settles (spawn.go). sends counts Runner.Send's messages and
	// settled is the last settled turn, settleC closed and replaced at each
	// settle and at close; voided is the sends no turn will take, since
	// the page cancelled them before a drain (voidCancelled); all four
	// under mu. drained is the sends the last drain covered, turnMark the
	// sends the running turn's messages covered, and reply its last reply
	// text; all three loop-owned. quitting is set at the daemon's
	// shutdown, whose cut-off turn the next daemon reports instead.
	sends    uint64
	settled  TurnResult
	settleC  chan struct{}
	voided   uint64
	drained  uint64
	turnMark uint64
	reply    string
	quitting bool

	// wmu orders the record writes of update, which other conversations'
	// goroutines also make (children.go takes a delivered report).
	wmu sync.Mutex
	// recordDirty is guarded by wmu. Only Send's coverage/activity stamp waits;
	// consumed input and replay-dedupe mutations still persist immediately.
	recordDirty bool

	// The spawn tree (children.go). settledReport is the pending report id
	// of the last settled turn, and parentWaits counts the conversation
	// parent's waits by target, and holding mirrors heldRes != nil; all
	// three under mu. Loop-owned: heldRes is a
	// done turn held for its children's reports; steered are the
	// reports and user messages steered into the running turn and not yet taken
	// by it, steerSeq the steer commands the process was written and
	// steerAnswers how many it answered.
	settledReport string
	parentWaits   map[uint64]int
	holding       bool
	heldRes       *TurnResult
	steered       []steeredReport
	steerSeq      int
	steerAnswers  int
}

// do runs fn in the loop and waits for it; false when the loop has ended.
func (c *conv) do(fn func()) bool {
	ran := make(chan struct{})
	select {
	case c.reqs <- func() { fn(); close(ran) }:
		<-ran
		return true
	case <-c.done:
		return false
	}
}

// signalInbox tells the loop the inbox changed, without waiting; signals
// sent before the loop takes one fold into it.
func (c *conv) signalInbox() {
	select {
	case c.inboxC <- struct{}{}:
	default:
	}
}

// signalMail tells the loop the session's mailbox may hold mail, without
// waiting.
func (c *conv) signalMail() {
	select {
	case c.mailC <- struct{}{}:
	default:
	}
}

// mailChanged claims the session's waiting mail and delivers it: steered
// into the running turn when the session takes a message mid-turn
// (steersMail), else into the record, so a message taken from the mailbox
// survives until it is delivered, and submitted with the held messages
// (kick), which wakes a sleeping child past its parent's width cap. Mail a
// sleeping child's refused wake keeps stays in the record, and each of its
// senders is told once per claim (Options.WakeRefused). It runs in the
// loop.
func (c *conv) mailChanged() {
	if c.closed || c.hidden || c.r.opts.Mail == nil {
		return
	}
	rec := c.record()
	relation := func(sender string) string { return c.r.mailRelation(rec, sender) }
	var mail []ClaimedMail
	var err error
	if rec.Lead {
		// The claim and the check that the session is still the daemon's
		// are one step: a terminal login that takes the row waits for the
		// lock, and what was claimed before it goes back to the mailbox
		// (release).
		unlock, ok := c.lockRow()
		if !ok {
			return
		}
		mail, err = c.r.opts.Mail.Drain(rec.Root, rec.WSKey, relation)
		unlock()
	} else {
		// A child's row is never a terminal's: its claim touches no file.
		mail, err = c.r.opts.Mail.Drain(rec.Root, rec.WSKey, relation)
	}
	if err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: mailbox: %v\n", rec.ReplyID, err)
	}
	var held []string
	for _, m := range mail {
		// Once a steer fails, the rest waits too, so the batch keeps its
		// order.
		if len(held) > 0 || !c.steerMail(m.Text) {
			held = append(held, m.Text)
		}
	}
	if len(held) > 0 {
		c.update(func(rec *Record) {
			rec.Mail = append(rec.Mail, held...)
			if rec.Parent != nil {
				// Any mail resets a spawned child's waiting expiry; a
				// steered one rides a turn, whose settle resets it.
				rec.LastActiveAt = c.r.opts.Now().UTC().Format(time.RFC3339)
			}
		})
	}
	if len(mail) > 0 {
		// The claim changed the session's mail history and held count. A
		// turn that runs submits nothing now, so nothing else would tell
		// the page.
		c.r.notify(rec.ReplyID, rec.Root)
	}
	// Mail claimed earlier and not yet delivered (a start that failed, a
	// daemon that ended) goes with it: nothing else would submit it.
	if len(c.rec.Mail) > 0 {
		c.kick()
	}
	if len(held) > 0 {
		c.tellRefusedWake(mail)
	}
}

// tellRefusedWake tells each sender of mail, claimed for a sleeping child
// whose wake admission kick just refused, that its mail waits
// (Options.WakeRefused), once per sender. It runs in the loop.
func (c *conv) tellRefusedWake(mail []ClaimedMail) {
	c.mu.Lock()
	refused, reason := c.wakeRefused, c.lastErr
	c.mu.Unlock()
	if !refused || c.r.opts.WakeRefused == nil {
		return
	}
	told := map[string]bool{}
	for _, m := range mail {
		if m.Sender == "" || told[m.Sender] {
			continue
		}
		told[m.Sender] = true
		c.r.opts.WakeRefused(m.Sender, c.rec.ReplyID, reason)
	}
}

// steersMail reports whether a mail claimed now reaches the running turn
// as a steer rather than waiting for the next turn: a turn runs that
// takes a message mid-turn, and the harness's catalog profile, when it has
// one and reported, does not grade steer unsupported. A lead steers
// through its steer handle, a spawned child through the acknowledged
// report path (reportSteerer) that keeps its report covering the message.
func (c *conv) steersMail() bool {
	if !c.turn || c.proc == nil || c.replaying || c.wrapHold || c.closed || c.hidden || c.compacting || c.compactCommand {
		return false
	}
	c.mu.Lock()
	caps := c.caps
	c.mu.Unlock()
	if caps != nil && !caps.Allows(ItemSteer) {
		return false
	}
	ctl := controlsFor(c.proc)
	if c.rec.Lead {
		return ctl.steer != nil
	}
	return ctl.reports != nil
}

// steerMail steers one claimed mail into the running turn when the session
// takes it now (steersMail), and reports whether it did; a failed steer
// leaves it for the next turn. It runs in the loop.
func (c *conv) steerMail(text string) bool {
	if !c.steersMail() {
		return false
	}
	ctl := controlsFor(c.proc)
	if c.rec.Lead {
		if err := ctl.steer.Steer(text); err != nil {
			fmt.Fprintf(c.r.opts.Log, "agent %s: steer a mail: %v\n", c.rec.ReplyID, err)
			return false
		}
		c.apply(*wakeEvent("mail", "mailbox", "", text).Live)
		return true
	}
	if err := ctl.reports.Steer(text); err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: steer a mail: %v\n", c.rec.ReplyID, err)
		return false
	}
	c.steerSeq++
	c.steered = append(c.steered, steeredReport{Text: text, Seq: c.steerSeq})
	c.saveSteered()
	c.apply(*wakeEvent("mail", "mailbox", "", text).Live)
	c.changed(false)
	return true
}

// inboxChanged takes the inbox's interrupt requests and submits the held
// messages, starting the process when needed (kick). It runs in the loop.
func (c *conv) inboxChanged() {
	c.refreshHeld()
	c.checkInterrupt()
	c.steer()
	c.kick()
}

// steer writes steered messages to the process while a turn runs:
// the user marked them to reach the harness now, not at the turn's end.
// They join the running turn, so no turn starts. Messages not marked stay
// held, with no turn running a marked one is an ordinary send (kick), and
// a backend that takes no message mid-turn, or a rejoin that cannot tell
// whether the turn runs (wrapHold), keeps it held.
func (c *conv) steer() {
	if !c.turn || c.proc == nil || c.replaying || c.wrapHold || c.closed || c.hidden || c.compacting || c.compactCommand {
		return
	}
	if !c.rec.Lead {
		c.steerWorker()
		return
	}
	st := controlsFor(c.proc).steer
	if st == nil {
		return
	}
	c.mu.Lock()
	recs, _ := wsinbox.DrainSteer(c.inbox)
	c.held = max(0, c.held-len(recs))
	c.mu.Unlock()
	var parts []string
	for _, rec := range recs {
		if text := strings.TrimSpace(rec.Entry.Text()); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return
	}
	text := strings.Join(parts, "\n\n")
	c.apply(wslive.Event{Kind: wslive.KindUser, Origin: wslive.OriginWhiteboard, Text: text})
	if err := st.Steer(text); err != nil {
		c.setStatus(func() { c.lastErr = "send the steered message: " + err.Error() })
		c.stopProcess("")
	}
}

func (c *conv) loop() {
	defer c.r.wg.Done()
	defer close(c.done)
	defer c.live.closeSlot()
	defer func() {
		c.stopCompactWatchdog()
		if c.spawnTimer != nil {
			c.spawnTimer.Stop()
		}
	}()
	// Before the first scan, Stop can report an existing holder unknown.
	// Keep its rejoined transport and persisted identity until holdersScanned
	// can establish absence or stop it; otherwise releaseHolder could forget
	// a still-running process and a later unhide could launch a second one.
	if c.hidden && c.r.scanDone() {
		c.stopProcess("")
		if err := c.retireHolder(); err != nil {
			c.setStatus(func() { c.lastErr = "archive process: " + err.Error() })
		}
	}
	beat := time.NewTicker(c.r.opts.heartbeat)
	defer beat.Stop()
	// Settle the predecessor before inbox signals (including nested reports)
	// can start a genuinely new turn.
	if c.rec.Parent != nil && c.rec.HolderID == "" {
		c.interrupted()
		if c.requeueSteered() {
			c.signalInbox()
		}
	}
	c.reportLast()
	if c.settle() {
		return
	}
	for {
		var idleC, actionC, leverC, compactC, spawnC <-chan time.Time
		leverC = c.leverC
		compactC = c.compactC
		if c.spawnTimer != nil {
			spawnC = c.spawnTimer.C
		}
		if c.idle != nil {
			idleC = c.idle.C
		}
		if c.actionTimer != nil {
			actionC = c.actionTimer.C
		}
		select {
		case fn := <-c.reqs:
			// An inbox change signaled before the request is handled
			// first, so a request sees what a send that returned before
			// it was made did, as when sends ran in the loop.
			select {
			case <-c.inboxC:
				c.inboxChanged()
			default:
			}
			fn()
		case <-c.inboxC:
			c.inboxChanged()
		case <-c.mailC:
			c.mailChanged()
		case <-c.spawnC:
			c.spawnDirty = true
			c.live.writeSnapshot()
			if c.live.currentSlot() == nil {
				c.changed(false)
			}
			if c.idleNow() {
				c.armIdle()
				c.restartIfDue()
			}
		case <-spawnC:
			c.spawnTimer, c.spawnNext = nil, time.Time{}
			c.live.writeSnapshot()
			if c.live.currentSlot() == nil {
				c.changed(false)
			}
		case ev, ok := <-c.events:
			if !ok {
				c.processExited()
			} else {
				c.handle(ev)
			}
		case <-idleC:
			c.idle = nil
			if c.idleNow() {
				c.stopProcess("idle_timeout")
			}
		case <-actionC:
			c.actionTimer = nil
			c.actionDue()
		case <-leverC:
			// No compaction started within the cap: what waited goes now.
			c.leverC, c.leverStop = nil, nil
			c.kick()
		case <-compactC:
			c.compactTimedOut()
		case <-beat.C:
			c.live.heartbeatSnapshot()
			c.persistSeq()
			if c.replaying {
				// A holder that attached late (a rescan) is found here.
				c.checkReattach()
			}
			if c.r.opts.Now().Sub(c.lastReg) >= wsdashboard.HeartbeatInterval {
				c.writeRegistration()
			} else {
				c.checkRow()
			}
			c.expireIfDue()
		case <-c.r.quit:
			c.letGo()
			// The watch's listening marker names this process: left behind,
			// it would name a dead one until the next daemon's watch.
			if c.stopMail != nil {
				c.stopMail()
			}
			return
		}
		if c.settle() {
			return
		}
	}
}

// settle follows each step of the loop: a session whose row a terminal
// harness took is let go (release), and it reports whether the loop ends
// (the conversation closed, or was let go), having stopped the mail watch.
func (c *conv) settle() bool {
	if c.foreignRow && !c.closed {
		c.release()
	}
	if !c.closed {
		return false
	}
	if c.stopMail != nil {
		c.stopMail()
	}
	return true
}

// refreshHeld reads only this conversation's inbox, never a global listing.
// It lists the inbox on every call, since another process may add or claim
// an entry, but opens only names it has not read yet: a held entry file is
// never rewritten (wsinbox.ReadHeld), so a known name's kind stays valid.
// The count is what wsinbox.Held would return: readable non-interrupt
// entries. An unreadable entry is not remembered and is retried next time.
func (c *conv) refreshHeld() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.r.opts.stateInboxRead != nil {
		c.r.opts.stateInboxRead(c.rec.ReplyID)
	}
	names, err := wsinbox.Names(c.inbox)
	if err != nil {
		return
	}
	kinds := make(map[string]bool, len(names))
	held := 0
	for _, name := range names {
		message, known := c.heldKinds[name]
		if !known {
			entry, err := wsinbox.ReadHeld(c.inbox, name)
			if err != nil {
				continue
			}
			message = !entry.Interrupt
		}
		kinds[name] = message
		if message {
			held++
		}
	}
	c.heldKinds, c.held = kinds, held
}

func (c *conv) state() State { return c.stateWithKeyCheck(true) }

func (c *conv) memoryState() State { return c.stateWithKeyCheck(false) }

func (c *conv) stateWithKeyCheck(checkKey bool) State {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := closedState(c.rec)
	st.Status, st.ClosedAt = StatusIdle, ""
	if c.turn {
		st.Status = StatusRunning
		if !c.closed && c.rec.Parent != nil && c.rec.TurnOpen {
			st.TurnStartedAt = c.rec.TurnStartedAt
		}
	}
	if c.closed {
		st.Status, st.ClosedAt = StatusClosed, c.rec.ClosedAt
		if c.rec.ClosedAt == "" && c.rec.HandedOffAt != "" {
			st.Status, st.HandedOffAt = StatusHandedOff, c.rec.HandedOffAt
			st.ResumeCommand = resumeCommand(c.rec, c.dir)
		}
	}
	st.Process = c.proc != nil
	st.Hidden = c.hidden
	if c.rec.Lead && !c.closed {
		st.Lifecycle = wsdashboard.SessionAsleep
		if c.hidden {
			st.Lifecycle = wsdashboard.SessionArchived
		} else if c.proc != nil {
			st.Lifecycle = wsdashboard.SessionActiveOn
		}
	} else if c.childWaitingLocked() {
		st.Lifecycle = wsdashboard.SessionChildWaiting
	}
	st.Held = c.held + len(c.rec.Mail) + len(c.rec.Queued) + len(c.rec.Reports)
	st.Error = c.lastErr
	if c.rejoinKeyPending && checkKey {
		if err := c.r.opts.LeadKeys.EnsureLead(c.rec.Root, c.rec.WSKey); err != nil {
			st.Error = appendDiagnostic(st.Error, "rejoin ws session key (advisory): "+err.Error())
		} else {
			c.rejoinKeyPending = false
		}
	}
	st.ContextTokens = c.contextTokens
	if c.observed != nil {
		observed := *c.observed
		st.Observed = &observed
	}
	st.Observations = append([]wslive.ModelObservation(nil), c.observations...)
	st.Batches = c.batches
	if c.turn && !c.closed {
		st.OutputTps = c.outputTps
	}
	if c.rec.Lead {
		st.ActiveMode, st.BackgroundTasks, st.ActionNeeded = c.activeMode, c.bgTasks, c.actionNeeded
		st.Background, st.Monitors = copyCount(c.background), copyCount(c.monitors)
		st.Approvals = append([]ApprovalRequest(nil), c.approvals...)
		if c.failure != nil {
			f := *c.failure
			st.Failure = &f
		}
		st.MCPBroken = slices.Clone(c.mcpBroken)
	}
	if c.caps != nil {
		caps := c.caps.clone()
		st.Capabilities = &caps
	}
	return c.r.permissionState(st, c.rec, c.dir)
}

func (c *conv) record() Record {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rec
}

// update changes the record under mu and persists it; wmu keeps the
// writes in the order of the changes.
func (c *conv) update(fn func(rec *Record)) {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	c.mu.Lock()
	fn(&c.rec)
	c.mu.Unlock()
	c.persistRecord()
}

// persistRecord requires wmu, so an immediate write also flushes deferred sends
// without letting an older copy overwrite a newer change.
func (c *conv) persistRecord() {
	rec := c.record()
	err := writeRecord(c.dir, rec)
	c.recordDirty = err != nil
	if err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: record write: %v\n", rec.ReplyID, err)
	}
}

func (c *conv) flushRecord() {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.recordDirty {
		c.persistRecord()
	}
}

func (c *conv) setStatus(fn func()) {
	c.mu.Lock()
	fn()
	c.mu.Unlock()
}

// writeRegistration writes the conversation's row and reports whether it
// did. A lead's row may have been taken by a terminal login (the session
// then is that harness's), so its write is one step with the check that
// the row is still the daemon's (lockRow): a row found taken is left as it
// is.
func (c *conv) writeRegistration() bool {
	rec := c.record()
	now := c.r.opts.Now()
	reg := registration(rec, now)
	// WriteRegistration normalizes this default before persisting it.
	reg.SchemaVersion = 1
	var previousRow *wsdashboard.Registration
	if rec.Lead {
		unlock, previous, ok := c.lockRowRead()
		if !ok {
			return false
		}
		defer unlock()
		reg.HostStopped = rec.HostPID == 0 && !c.spawning
		// The holder manager writes the last exit independently of the
		// runner's heartbeat; a regenerated row must not discard it.
		if previous != nil {
			previousRow = previous
			reg.HostExitCode = previous.HostExitCode
			reg.HostExitSignal = previous.HostExitSignal
			reg.HostExitReason = previous.HostExitReason
			reg.HostStoppedAt = previous.HostStoppedAt
		}
	}
	// Reuse only the row already read under the lead's ownership lock, not
	// process-local evidence: a spawn needs a real reservation on disk.
	if previousRow != nil && now.Sub(c.lastReg) < wsdashboard.HeartbeatInterval {
		previous := *previousRow
		previous.HeartbeatAt = reg.HeartbeatAt
		if previous == reg {
			return true // no semantic change; leave the scheduled heartbeat due
		}
	}
	err := wsdashboard.WriteRegistration(c.r.opts.Dir, reg)
	if err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: registration write: %v\n", rec.ReplyID, err)
		return false
	}
	c.lastReg = now
	c.r.notify(rec.ReplyID, rec.Root)
	return true
}

// rowForeign reports whether a session's row, as read, is no longer the
// daemon's: it exists and is neither a hosted nor an agent row. A terminal
// harness that logged in with the session's key while no process of the
// daemon's ran rewrote it (a hand to the terminal). A missing or unreadable
// row is the daemon's to write.
func rowForeign(dir, replyID string) bool {
	return isForeignRow(wsdashboard.ReadRegistration(dir, replyID))
}

// isForeignRow is rowForeign's test of a row as read.
func isForeignRow(row wsdashboard.Registration, err error) bool {
	// An agent row is no terminal's either, since a terminal login never
	// writes Agent: a daemon of a release that knew no hosted lead served
	// the session as an agent conversation while it ran, and its row is
	// the daemon's to take back.
	return err == nil && !row.Hosted && !row.Agent
}

// lockRow takes a lead session's row lock and checks the row is still the
// daemon's: the step every row write, mail claim, and spawn is guarded by.
// It returns the lock's release and true to go on. False means nothing may
// be done now: the row is a terminal harness's (foreignRow is set, and the
// loop lets the session go), or the lock could not be taken (logged; the
// caller's next try asks again).
func (c *conv) lockRow() (unlock func(), ok bool) {
	unlock, _, ok = c.lockRowRead()
	return unlock, ok
}

// lockRowRead is lockRow that also returns the row it checked under the
// lock (nil when missing or unreadable), so a caller that needs the row
// reads it once per lock: the live writer takes this lock for every ring
// and snapshot write (lockSummaryWriter).
func (c *conv) lockRowRead() (unlock func(), row *wsdashboard.Registration, ok bool) {
	if c.foreignRow {
		return nil, nil, false
	}
	unlock, err := wsdashboard.LockRegistration(c.r.opts.Dir, c.rec.ReplyID)
	if err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: %v\n", c.rec.ReplyID, err)
		return nil, nil, false
	}
	read, err := wsdashboard.ReadRegistration(c.r.opts.Dir, c.rec.ReplyID)
	if isForeignRow(read, err) {
		unlock()
		c.foreignRow = true
		return nil, nil, false
	}
	if err == nil {
		row = &read
	}
	return unlock, row, true
}

// checkRow reads a lead session's row between its own writes, without the
// lock, so a hand to the terminal shows within a heartbeat. Nothing is
// decided on this read alone: every guarded step checks again under the
// lock, and the row leaves the daemon only through a terminal login, which
// never gives it back.
func (c *conv) checkRow() {
	if c.rec.Lead && !c.foreignRow && rowForeign(c.r.opts.Dir, c.rec.ReplyID) {
		c.foreignRow = true
	}
}

// release lets a lead session go to the terminal harness that took its
// row: the process and its holder end (the login was accepted only with no
// live process), mail claimed and not delivered goes back to the session's
// mailbox, held dashboard messages are dropped as a close drops them, and
// the record becomes a handed-off one, which no daemon start serves again.
// The row is the terminal's: nothing here writes it. The loop ends after
// (settle), which stops the mail watch.
func (c *conv) release() {
	c.stopProcess("")
	if err := c.retireHolder(); err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: %v\n", c.rec.ReplyID, err)
	}
	mail := c.rec.Mail
	if len(mail) > 0 && c.r.opts.Mail != nil {
		if err := c.r.opts.Mail.Return(c.rec.Root, c.rec.WSKey, mail); err != nil {
			// Kept in the record: a later takeover delivers it.
			fmt.Fprintf(c.r.opts.Log, "agent %s: return claimed mail: %v\n", c.rec.ReplyID, err)
		} else {
			mail = nil
		}
	}
	c.update(func(rec *Record) {
		rec.HandedOffAt = c.r.opts.Now().UTC().Format(time.RFC3339)
		rec.Mail = mail
		c.closed = true
	})
	c.dropHeld()
	c.live.setAdapter("")
	rec := c.record()
	st := c.memoryState()
	c.r.mu.Lock()
	if c.r.convs[rec.ReplyID] == c {
		delete(c.r.convs, rec.ReplyID)
	}
	c.r.closed[rec.ReplyID] = rec
	if c.r.closedRows == nil {
		c.r.closedRows = map[string]State{}
	}
	c.r.closedRows[rec.ReplyID] = st
	c.r.mu.Unlock()
	fmt.Fprintf(c.r.opts.Log, "agent %s: handed to a terminal session\n", rec.ReplyID)
	c.r.notify(rec.ReplyID, rec.Root)
}

// dropHeld drops the dashboard messages held for a conversation that takes
// no more turns, the ones drained and still queued, and its pending
// interrupt requests.
func (c *conv) dropHeld() {
	if len(c.rec.Queued) > 0 {
		c.update(func(rec *Record) { rec.Queued = nil })
	}
	if held, err := wsinbox.Held(c.inbox); err == nil {
		for _, rec := range held {
			_, _ = wsinbox.Cancel(c.inbox, rec.Name)
		}
	}
	_, _ = wsinbox.DrainInterrupt(c.inbox)
	c.setStatus(func() { c.held = 0 })
}

// apply applies a wslive event and tells the daemon when it wrote. quiet
// (a replayed holder record) applies it to the state alone.
func (c *conv) apply(ev wslive.Event, quiet ...bool) {
	if len(quiet) == 0 || !quiet[0] {
		c.observeCompaction(ev)
	}
	if ev.Kind == wslive.KindReset {
		c.spawnDirty = true
	}
	usageChanged := false
	if ev.Kind == wslive.KindStatus {
		c.setStatus(func() {
			if ctx := ev.Context; ctx != nil {
				// An explicit unknown context (e.g. after compaction) clears
				// the previous figure in the row as well as the child's band.
				tokens := float64(0)
				if ctx.Tokens != nil {
					tokens = float64(*ctx.Tokens)
				}
				if tokens >= 0 && tokens <= wslive.MaxContextTokens && (c.contextTokens == nil || *c.contextTokens != tokens) {
					c.contextTokens, usageChanged = &tokens, true
				}
			}
			if rate := ev.OutputTps; rate != nil && *rate >= 0 && *rate <= wslive.MaxOutputTps && (c.outputTps == nil || *c.outputTps != *rate) {
				v := *rate
				c.outputTps, usageChanged = &v, true
			}
		})
	}
	if len(quiet) > 0 && quiet[0] {
		c.live.applyQuiet(ev)
	} else {
		c.live.apply(ev)
	}
	if ev.Kind == wslive.KindStatus && ev.Observed != nil {
		observations := c.live.st.ObservedResponses("")
		c.setStatus(func() {
			if !reflect.DeepEqual(c.observations, observations) {
				c.observations, usageChanged = observations, true
			}
		})
		if own := c.live.st.Snapshot(c.r.opts.Now().UnixMilli()).Lead.Observed; own != nil {
			c.setStatus(func() {
				if c.observed == nil || !reflect.DeepEqual(*c.observed, own.Current) {
					observed := own.Current
					c.observed, usageChanged = &observed, true
				}
			})
		}
	}
	activityChanged := false
	if (ev.Kind == wslive.KindAgent && ev.AgentID == "" && ev.Batches != nil) || ev.Kind == wslive.KindReset || ev.Kind == wslive.KindTurnEnd || ev.Kind == wslive.KindReply {
		batches := c.live.st.Snapshot(c.r.opts.Now().UnixMilli()).Lead.Batches
		c.setStatus(func() {
			if !reflect.DeepEqual(c.batches, batches) {
				c.batches, activityChanged = batches, true
			}
		})
	}
	if usageChanged || activityChanged {
		c.watched()
	}
}

// kick submits the next turn when no turn runs, starting the process first
// when there is none. A failed start leaves everything held.
//
// The turn is taken from the drained inbox, in order, behind what earlier
// drains queued (Record.Queued): the messages up to the first slash command
// are joined into one turn with a lead's claimed mail, and a command at the
// front runs alone, as its bare typed line, so the harness runs it as a
// command. What follows the turn's part stays queued for the turns after
// it, which each turn end submits; mail waits for the next message turn, or
// goes alone once nothing is queued.
func (c *conv) canKick() bool {
	return !c.closed && !c.hidden && !c.turn && !c.replaying && len(c.steered) == 0 && c.leverC == nil &&
		!(c.rec.Backend == BackendPi && c.r.opts.Holders != nil && !c.r.scanDone())
}

func (c *conv) kick() {
	if !c.canKick() {
		return
	}
	if c.compactIdle() {
		return
	}
	c.mu.Lock()
	held, err := wsinbox.Held(c.inbox)
	if err == nil {
		c.held = len(held)
	}
	c.mu.Unlock()
	mail := c.rec.Mail
	// A child's reports (children.go) go with the next message turn, as
	// mail does.
	reports := c.rec.Reports
	if (err != nil || len(held) == 0) && len(mail) == 0 && len(c.rec.Queued) == 0 && len(reports) == 0 {
		c.voidCancelled()
		return
	}
	if c.proc == nil {
		if c.rec.Lead && c.rec.HolderID != "" && !c.r.scanDone() {
			// The holder the record names may still run unseen: the start
			// waits for the first scan (holdersScanned kicks again), since
			// retireHolder would not find that holder to end it first.
			return
		}
		if err := c.startProcess(); err != nil {
			var refused *admissionRefusal
			isRefusal := errors.As(err, &refused)
			c.setStatus(func() { c.lastErr, c.wakeRefused = err.Error(), isRefusal })
			fmt.Fprintf(c.r.opts.Log, "agent %s: start: %v\n", c.rec.ReplyID, err)
			c.live.writeSnapshot()
			c.r.notify(c.rec.ReplyID, c.rec.Root)
			if isRefusal {
				// The input stays for the next admitted start; persist its
				// coverage so crash recovery cannot expire it as quiet.
				c.flushRecord()
				return
			}
			c.settleTurn(TurnFailed, err.Error(), c.sendCount())
			return
		}
		c.setStatus(func() { c.wakeRefused = false })
	}
	// Under mu, as Send writes: every send counted here is in the drain
	// (Wait's turn marks).
	c.mu.Lock()
	recs, _ := wsinbox.Drain(c.inbox)
	c.held = max(0, c.held-len(recs))
	covered := c.sends
	c.mu.Unlock()
	from := c.drained
	c.drained = covered
	queue := slices.Clone(c.rec.Queued)
	for _, rec := range recs {
		if cmd := rec.Entry.Command; cmd != nil {
			cmd := *cmd
			queue = append(queue, QueuedInput{Command: &cmd, After: from})
		} else if text := strings.TrimSpace(rec.Entry.Text()); text != "" {
			queue = append(queue, QueuedInput{Text: text, After: from})
		}
	}
	if len(queue) == 0 && len(mail) == 0 && len(reports) == 0 {
		// A cancel took the message after the start: the process waits for
		// input like after a turn.
		c.voidCancelled()
		c.armIdle()
		return
	}
	if len(recs) > 0 {
		// Drained entries are no longer in the inbox: until a turn sent
		// them, the record holds them.
		c.update(func(rec *Record) { rec.Queued = queue })
	}
	cmd, parts, rest := nextTurn(queue)
	if cmd != nil {
		mail, reports = nil, nil
	}
	// The turn covers the sends its messages hold: all the drain took
	// when nothing waits behind it, else only those before the drain that
	// took the first input left waiting (a leading slash command's turn
	// leaves the text drained with it for the next turn).
	mark := covered
	if len(rest) > 0 {
		mark = rest[0].After
	}
	c.stopIdle()
	startedAt := c.r.opts.Now().UnixMilli()
	c.setStatus(func() { c.turn, c.lastErr = true, "" })
	// The turn reaches the page only once its send went through: a failed
	// send leaves no user entry or turn line of its own (its error shows),
	// so its retry shows the message once. The process's events for the
	// turn are handled by this loop after kick returns, so they still
	// follow the turn's start.
	ordinary := cmd == nil
	if cmd == nil {
		err = c.proc.Send(c.compactInput(strings.Join(append(append(slices.Clone(parts), reports...), mail...), "\n\n")))
	} else if text, ok := c.playbookTurn(*cmd); ok {
		// The runner expands a playbook command itself: the harness has
		// no such command, and the turn carries the playbook's body.
		ordinary = true
		err = c.proc.Send(c.compactInput(text))
	} else {
		err = c.runCommand(*cmd)
	}
	if err != nil && cmd != nil && cmd.Name == "compact" {
		c.setStatus(func() { c.turn, c.lastErr = false, "compaction: "+err.Error() })
		c.compactEvent(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactFailed})
		c.apply(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactFailed})
		c.update(func(rec *Record) { rec.Queued = rest })
		c.signalInbox()
		return
	}
	if err != nil {
		// The turn never began: its process ends with no turn to close.
		c.setStatus(func() { c.turn, c.lastErr = false, "send turn: "+err.Error() })
		c.stopProcess("")
		// The turn stays queued (its mail stays claimed), and nothing else
		// would submit it before an unrelated signal: it is retried once on
		// a new process, through the loop's inbox signal rather than on
		// this stack. A send that fails again leaves it for the next inbox
		// or mail signal, so a send that always fails does not spin.
		if (len(c.rec.Queued) > 0 || len(c.rec.Mail) > 0 || len(c.rec.Reports) > 0) && !c.sendRetried {
			c.sendRetried = true
			c.signalInbox()
		} else {
			c.settleTurn(TurnFailed, c.lastError(), mark)
		}
		return
	}
	if ordinary {
		c.compactNote = ""
		c.compactResume = false
	}
	c.sendRetried = false
	c.turnMark, c.reply = mark, ""
	// This turn's settle supersedes a done turn held for the children.
	c.hold(nil)
	c.update(func(rec *Record) {
		rec.Queued = rest
		rec.Drained, rec.TurnMark, rec.Voided = c.drained, mark, c.voided
		rec.TurnStartedAt = startedAt
		rec.TurnStartSeq = max(c.seq, c.lineOpen)
		if len(mail) > 0 {
			rec.Mail = nil
		}
		if len(reports) > 0 {
			rec.Reports = nil
		}
		if rec.Parent != nil {
			rec.TurnOpen, rec.HeldTurn = true, nil
		}
	})
	// Mail shows on the page through the session's mail history, so only
	// the dashboard's own text is a user entry; a turn of mail alone is
	// marked as mail's.
	origin := wslive.OriginMail
	text := strings.Join(append(slices.Clone(parts), reports...), "\n\n")
	if cmd != nil {
		text = cmd.Line()
	}
	if text != "" {
		origin = wslive.OriginWhiteboard
		c.apply(wslive.Event{Kind: wslive.KindUser, Origin: origin, Text: text})
	}
	for _, text := range mail {
		c.apply(*wakeEvent("mail", "mailbox", "", text).Live)
	}
	c.apply(wslive.Event{Kind: wslive.KindTurnStart, Origin: origin})
	c.watched()
}

// voidCancelled counts the sends no drain took as voided when kick finds
// nothing to send with no turn running: every send a drain took went in a
// turn, so one the inbox no longer holds was cancelled from the page, and
// no turn will ever cover it. A conversation parent whose done turn waited
// for this child (openChildOf) is released. It runs in the loop.
func (c *conv) voidCancelled() {
	c.mu.Lock()
	held, err := wsinbox.Held(c.inbox)
	grew := err == nil && len(held) == 0 && c.sends > max(c.voided, c.settled.mark)
	if grew {
		c.voided = c.sends
	}
	parent := c.rec.Parent
	c.mu.Unlock()
	if grew && parent != nil && parent.Kind == ParentConversation {
		c.r.releaseLater(parent.ReplyID)
	}
}

// nextTurn splits queue at its first slash command: a command at the front
// is the turn (cmd), else the texts before the first command are (parts).
// rest is what waits for the turns after it; nil when nothing does.
func nextTurn(queue []QueuedInput) (cmd *wsinbox.Command, parts []string, rest []QueuedInput) {
	n := 0
	if len(queue) > 0 && queue[0].Command != nil {
		cmd, n = queue[0].Command, 1
	} else {
		for n < len(queue) && queue[n].Command == nil {
			parts = append(parts, queue[n].Text)
			n++
		}
	}
	if n < len(queue) {
		rest = slices.Clone(queue[n:])
	}
	return cmd, parts, rest
}
