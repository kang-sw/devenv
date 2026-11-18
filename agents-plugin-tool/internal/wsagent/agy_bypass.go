package wsagent

import "slices"

// agyBypass is deliberately restricted to hosted agy leads: neither stray
// metadata on another backend nor a worker changes its approval policy.
func (rec Record) agyBypass() bool {
	return rec.Lead && rec.Backend == BackendAgy && rec.AgyBypass
}

// allowAgyBypassPending also covers an old daemon's unanswered prompts replayed
// by a new daemon. addApproval already discarded ids durably answered before.
func (c *conv) allowAgyBypassPending() error {
	if c.rec.agyBypass() {
		for _, approval := range slices.Clone(c.approvals) {
			if err := c.answerApproval(approval.ID, true, ""); err != nil {
				return err
			}
		}
	}
	return nil
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
