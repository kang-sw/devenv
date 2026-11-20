package wsagent

import (
	"errors"
	"fmt"
	"slices"
)

// agyBypass is deliberately restricted to hosted agy leads: neither stray
// metadata on another backend nor a worker changes its approval policy.
// Bypass is stored with native default (selectPermissionMode) and this build
// keeps that mode while it holds (a native report does not move it). Any other
// mode means a writer that does not know the flag (an older daemon keeps it as
// an unknown member) changed the mode since: the stale flag prompts instead.
func (rec Record) agyBypass() bool {
	return rec.Lead && rec.Backend == BackendAgy && rec.AgyBypass && rec.PermissionMode == "default"
}

// allowAgyBypassPending also covers an old daemon's unanswered prompts replayed
// by a new daemon. addApproval already discarded ids durably answered before.
// A rejoin answers them at its end (replayDone), not while it replays. A
// prompt the process could not answer stays for the user; a failed write
// stopped the process, so nothing is left to answer.
func (c *conv) allowAgyBypassPending() {
	if c.replaying || !(c.rec.agyBypass() || !c.rec.Lead && c.r.opts.Backends[c.rec.Backend].WorkerAutoApprove) {
		return
	}
	answerer := controlsFor(c.proc).approval
	if answerer == nil {
		return
	}
	for _, approval := range slices.Clone(c.approvals) {
		err := c.sendApproval(answerer, approval, true, "")
		if err == nil {
			c.dropApproval(approval.ID, false)
			continue
		}
		fmt.Fprintf(c.r.opts.Log, "agent %s: auto-allow permission: %v\n", c.rec.ReplyID, err)
		if !errors.Is(err, errNoApprovalAnswer) {
			return
		}
	}
}

// selectedPermissionMode projects ws intent to the existing settings UI while
// keeping the persisted/native mode readable by older daemons.
func (rec Record) selectedPermissionMode() string {
	if rec.agyBypass() {
		return "bypass"
	}
	return rec.PermissionMode
}

func (rec Record) launchPermissionMode() string {
	if rec.agyBypass() {
		return "default"
	}
	return rec.PermissionMode
}

func (rec *Record) selectPermissionMode(mode string) {
	rec.PermissionMode = mode
	if rec.Lead && rec.Backend == BackendAgy {
		rec.AgyBypass = mode == "bypass"
		if rec.AgyBypass {
			rec.PermissionMode = "default"
		}
	}
}
