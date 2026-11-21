// runner_operations.go holds conversation queries and controls.
package wsagent

import (
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
)

func (r *Runner) Get(id string) (State, error) {
	r.mu.Lock()
	c, open := r.convs[id]
	rec, closed := r.closed[id]
	var st State
	if closed && !open {
		st = r.closedState(rec)
	}
	r.mu.Unlock()
	switch {
	case open:
		c.refreshHeld() // explicit API reads retain authoritative held counts
		if c.record().Lead {
			revalidatePermissionSelection(c.dir)
		}
		return c.state(), nil
	case closed:
		return st, nil
	}
	return State{}, errUnknown
}

// List reads every conversation's state, oldest first.
func (r *Runner) List() []State {
	r.mu.Lock()
	out := make([]State, 0, len(r.convs)+len(r.closed))
	convs := make([]*conv, 0, len(r.convs))
	for _, c := range r.convs {
		convs = append(convs, c)
	}
	for _, rec := range r.closed {
		out = append(out, r.memoryClosedState(rec))
	}
	r.mu.Unlock()
	for _, c := range convs {
		out = append(out, c.memoryState())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt < out[j].CreatedAt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Send holds text as the conversation's next user message. It is submitted
// now when no turn runs. A spawned Pi worker takes it as a native steer
// during a running turn; other conversations hold it until that turn ends.
// A closed spawned Pi child is reopened on its existing session file;
// other closed conversations still refuse sends.
// It reports whether the message is still held.
//
// Send returns once the message is in the conversation's inbox: it neither
// waits for the loop nor starts the process, which the loop does on its
// signal (inboxChanged), so a busy loop or a slow start never holds the
// caller. A start failure reaches the conversation's State.Error, not this
// answer. The inbox write is serialized against close under c.mu, so a send
// racing CloseConversation either lands before the close (which drops it
// as it drops every held message) or is refused, never written into a
// closed conversation's inbox that nothing reads.
func (r *Runner) Send(id, text string) (State, error) {
	if strings.TrimSpace(text) == "" {
		return State{}, badRequest("text is empty")
	}
	st, err := r.send(id, text)
	if err != errClosed {
		return st, err
	}
	if rec, ok := r.recordOf(id); ok && !rec.Lead && rec.Parent != nil && rec.Backend == BackendPi {
		// A close may land between the open check and the inbox write.
		// Once that close is done (resumeMu), a second try reopens it as
		// any closed Pi child.
		r.resumeMu.Lock()
		r.resumeMu.Unlock()
		st, err = r.send(id, text)
	}
	return st, err
}

func (r *Runner) send(id, text string) (State, error) {
	c, err := r.open(id)
	if err != nil {
		// Keep revival and admission in one fence against explicit close.
		// Open sends do not take resumeMu, so a slow manual close still
		// refuses them promptly rather than blocking behind its cascade.
		r.resumeMu.Lock()
		defer r.resumeMu.Unlock()
		c, err = r.resumePiChild(id)
		if err != nil {
			return State{}, err
		}
	}
	if r.opts.sendAdmitting != nil {
		r.opts.sendAdmitting(id)
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	c.mu.Lock()
	if c.closed {
		err := endedError(c.rec)
		c.mu.Unlock()
		return State{}, err
	}
	steer := c.rec.Parent != nil && c.rec.Backend == BackendPi && !c.rec.Lead
	_, _, err = wsinbox.Send(c.inbox, wsinbox.Entry{Parts: []string{text}, Steer: steer}, r.opts.Now())
	if err == nil {
		// After the write, under the lock kick drains with: a turn whose
		// drain counted this send holds its message (Wait).
		c.sends++
		c.held++
		c.rec.Sends = c.sends
		c.cancelModelWaitBackoffForInput()
		if c.rec.Parent != nil {
			// Any message resets a spawned child's waiting expiry.
			c.rec.LastActiveAt = r.opts.Now().UTC().Format(time.RFC3339)
		}
	}
	c.mu.Unlock()
	if err != nil {
		return State{}, err
	}
	c.inboxMutated(1)
	// The inbox already owns the text durably. Coverage and the child's
	// activity stamp ride the next immediate record write or shutdown flush.
	c.recordDirty = true
	c.signalInbox()
	return c.state(), nil
}

// Interrupt stops the running turn, if any, and takes back the held
// messages, returning their text, as the session page's interrupt does.
// What a drain already took and queued behind a slash command (Record.Queued)
// is not held: the inbox shows it received, so it stays and is sent after
// the interrupted turn ends, as the page's interrupt leaves it too.
func (r *Runner) Interrupt(id string) (State, []string, error) {
	var returned []string
	st, err := r.act(id, func(c *conv) error {
		c.mu.Lock()
		held, _, err := wsinbox.Interrupt(c.inbox, r.opts.Now())
		if err == nil {
			c.held = 0
		}
		c.mu.Unlock()
		if err != nil {
			return err
		}
		c.inboxMutated(len(held))
		for _, rec := range held {
			returned = append(returned, rec.Entry.Text())
		}
		c.checkInterrupt()
		return nil
	})
	return st, returned, err
}

// Stop ends the conversation's process; held messages stay held, and the
// next send resumes the backend session.
func (r *Runner) Stop(id string) (State, error) {
	return r.act(id, func(c *conv) error {
		c.stopProcess("")
		return nil
	})
}

// IdleLead is a lead session SleepIdle may stop: one JSON entry of the
// idle list.
type IdleLead struct {
	ID      string `json:"id"`
	Label   string `json:"label,omitempty"`
	Backend string `json:"backend"`
}

// sleepable reports a lead conversation whose process may be stopped for
// power saving: it runs, and no turn, replay, background task, active child
// or approval prompt keeps it busy (the idle timer's own test, idleNow).
// Run in the conversation's loop.
func (c *conv) sleepable() bool {
	return c.rec.Lead && !c.closed && c.proc != nil && len(c.approvals) == 0 && c.idleNow()
}

// IdleLeads lists the lead sessions with a running process that SleepIdle
// would stop, and counts the other running ones (busy or waiting for an
// approval), which it keeps.
func (r *Runner) IdleLeads() (idle []IdleLead, kept int) {
	idle = []IdleLead{}
	r.mu.Lock()
	convs := make([]*conv, 0, len(r.convs))
	for _, c := range r.convs {
		convs = append(convs, c)
	}
	r.mu.Unlock()
	created := map[string]string{}
	for _, c := range convs {
		c.do(func() {
			if !c.rec.Lead || c.closed || c.proc == nil {
				return
			}
			if !c.sleepable() {
				kept++
				return
			}
			idle = append(idle, IdleLead{ID: c.rec.ReplyID, Label: c.rec.Label, Backend: c.rec.Backend})
			created[c.rec.ReplyID] = c.rec.CreatedAt
		})
	}
	sort.Slice(idle, func(i, j int) bool {
		if a, b := created[idle[i].ID], created[idle[j].ID]; a != b {
			return a < b
		}
		return idle[i].ID < idle[j].ID
	})
	return idle, kept
}

// SleepIdle stops each named lead session as Stop does, but only when it is
// still idle in its own loop; one that became busy (or is no lead process
// any more) is skipped. Held messages stay held and the next send resumes
// a stopped session.
func (r *Runner) SleepIdle(ids []string) (slept, skipped []string) {
	slept, skipped = []string{}, []string{}
	for _, id := range ids {
		stopped := false
		_, err := r.act(id, func(c *conv) error {
			if c.sleepable() {
				c.stopProcess("")
				stopped = true
			}
			return nil
		})
		if err == nil && stopped {
			slept = append(slept, id)
		} else {
			skipped = append(skipped, id)
		}
	}
	return slept, skipped
}

// Restart ends a lead conversation's process and starts a new one on the
// same backend session at once: the way back from an MCP connection that
// did not recover in-stream. It ends the session's background work with
// the process, so the dashboard asks first.
func (r *Runner) Restart(id string) (State, error) {
	return r.act(id, func(c *conv) error {
		if !c.rec.Lead {
			return badRequest("restart applies to a lead session only")
		}
		return c.restartProcess()
	})
}

// IsLead reports whether id is a lead conversation, open or closed: one
// whose control writes take the trusted gate.
func (r *Runner) IsLead(id string) bool {
	r.mu.Lock()
	c, open := r.convs[id]
	rec, closed := r.closed[id]
	r.mu.Unlock()
	if open {
		return c.record().Lead
	}
	return closed && rec.Lead
}

func (r *Runner) isChildRecord(rec Record) bool {
	if rec.Parent != nil {
		return true
	}
	return r != nil && r.opts.CardBound != nil && r.opts.CardBound(rec.ReplyID)
}

// IsChild reports whether id is a child conversation whose settings are fixed
// at spawn: either a spawned worker (Record.Parent != nil) or a card-bound
// subordinate lead reported by Options.CardBound.
func (r *Runner) IsChild(id string) bool {
	r.mu.Lock()
	c, open := r.convs[id]
	rec, closed := r.closed[id]
	r.mu.Unlock()
	if open {
		return r.isChildRecord(c.record())
	}
	if closed {
		return r.isChildRecord(rec)
	}
	return r.opts.CardBound != nil && r.opts.CardBound(id)
}

// SessionMeta returns in-memory composer control metadata for replyID without
// touching disk (wsdashboard.AgentSessionMetaHost).
func (r *Runner) SessionMeta(replyID string) (wsdashboard.AgentSessionMeta, bool) {
	r.mu.Lock()
	c, open := r.convs[replyID]
	rec, closed := r.closed[replyID]
	r.mu.Unlock()
	active := false
	if open {
		c.mu.Lock()
		rec = c.rec
		closed = c.closed
		active = !closed && (c.turn || c.holding || len(c.approvals) != 0)
		c.mu.Unlock()
	} else if !closed {
		return wsdashboard.AgentSessionMeta{}, false
	}
	return wsdashboard.AgentSessionMeta{
		Hosted:  rec.Lead,
		IsChild: r.isChildRecord(rec),
		Closed:  closed,
		Active:  active,
		Model:   rec.Model,
		Effort:  rec.Effort,
	}, true
}

// AnswerApproval answers the waiting tool permission prompt requestID of a
// lead conversation: allow runs the tool call as asked, deny refuses it
// with message for the model. A prompt is answered once: an id that is not
// waiting (answered from another tab, withdrawn, or of an ended process) is
// a conflict.
func (r *Runner) AnswerApproval(id, requestID string, allow bool, message string) (State, error) {
	return r.act(id, func(c *conv) error {
		return c.answerApproval(requestID, allow, message)
	})
}

// SetPermissionMode changes an admitted lead conversation's permission mode.
// Like a model or effort change it applies at the next turn boundary,
// in-stream when the harness takes it; the mode the harness then reports is
// State.ActiveMode.
func (r *Runner) SetPermissionMode(id, mode string) (State, error) {
	return r.setPermissionMode(id, mode, false)
}

func (r *Runner) setPermissionMode(id, mode string, user bool) (State, error) {
	modes := LeadPermissionModes
	if rec, ok := r.recordOf(id); ok {
		modes = permissionModes(rec.Backend, []map[string]BackendRegistration{r.opts.Backends})
	}
	if !slices.Contains(modes, mode) {
		return State{}, categorized(ErrInvalidProfile, badRequest("permission_mode %q must be one of %s", mode, strings.Join(modes, ", ")))
	}
	return r.act(id, func(c *conv) error {
		return c.setNativePermissionMode(mode, user)
	})
}

// CloseConversation ends the conversation: its process stops, held
// messages are dropped, and its session directory stays readable. A spawned
// Pi child can be reopened by Send; other closed conversations refuse it.
// Its open descendants through conversation parents, and a lead's Pi
// children (children.go), close first, deepest first. A conversation parent
// whose done turn waited for this child's report is released.
func (r *Runner) CloseConversation(id string) (State, error) {
	r.resumeMu.Lock()
	defer r.resumeMu.Unlock()
	if _, err := r.open(id); err != nil {
		return State{}, err
	}
	for _, d := range r.descendants(id) {
		_, _ = r.closeOne(d)
	}
	st, err := r.closeOne(id)
	if err != nil {
		return st, err
	}
	r.releaseParent(id)
	return st, nil
}

// releaseParent releases the done turn conversation id's conversation
// parent held for it (releaseHeld), once id closed.
func (r *Runner) releaseParent(id string) {
	if rec, ok := r.recordOf(id); ok && rec.Parent != nil && rec.Parent.Kind == ParentConversation {
		if parent := r.conv(rec.Parent.ReplyID); parent != nil {
			parent.do(parent.releaseHeld)
		}
	}
}

// closeOne closes conversation id alone.
func (r *Runner) closeOne(id string) (State, error) {
	st, err := r.act(id, func(c *conv) error {
		c.close()
		return nil
	})
	if err != nil {
		return st, err
	}
	r.mu.Lock()
	if c, ok := r.convs[id]; ok {
		delete(r.convs, id)
		r.closed[id] = c.record()
		if r.closedRows == nil {
			r.closedRows = map[string]State{}
		}
		r.closedRows[id] = st
	}
	r.mu.Unlock()
	return st, nil
}

// SetSettings changes the conversation's model or effort (nil leaves one
// unchanged, "" returns it to the harness default). The change applies at
// the next turn boundary: in-stream when the backend has a control for it,
// else by restarting the process on the same backend session.
func (r *Runner) SetSettings(id string, model, effort *string) (State, error) {
	if model != nil && *model != "" && !modelPattern.MatchString(*model) {
		return State{}, badRequest("model %q must match %s", *model, modelPattern.String())
	}
	if effort != nil && *effort != "" && !effortPattern.MatchString(*effort) {
		return State{}, badRequest("effort %q must be a lowercase word", *effort)
	}
	return r.act(id, func(c *conv) error {
		return c.setSettings(model, effort)
	})
}

// open returns the open conversation id: errClosed for a closed one,
// errUnknown for one the runner never had.
func (r *Runner) open(id string) (*conv, error) {
	r.mu.Lock()
	c, open := r.convs[id]
	rec, closed := r.closed[id]
	r.mu.Unlock()
	switch {
	case open:
		return c, nil
	case closed:
		return nil, endedError(rec)
	}
	return nil, errUnknown
}

// act runs fn in the conversation's loop and returns its state after.
func (r *Runner) act(id string, fn func(c *conv) error) (State, error) {
	c, err := r.open(id)
	if err != nil {
		return State{}, err
	}
	if !c.do(func() {
		if c.closed {
			err = endedError(c.rec)
			return
		}
		err = fn(c)
	}) {
		return State{}, endedError(c.record())
	}
	if err != nil {
		return State{}, err
	}
	return c.state(), nil
}

// Conversation statuses a State reports.
