package wsagent

import (
	"fmt"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// SetHidden archives only an open dashboard-hosted lead. The conversation and
// its held input remain intact. Restoring visibility alone launches nothing;
// queued input or mail uses the ordinary lazy wake path.
func (r *Runner) SetHidden(id string, hidden bool) (State, error) {
	return r.act(id, func(c *conv) error { return c.setHidden(hidden) })
}

func (c *conv) setHidden(hidden bool) error {
	if !c.rec.Lead {
		return badRequest("archive applies to a hosted lead session only")
	}
	unlock, ok := c.lockRow()
	if !ok {
		return conflict("the session is not owned by the dashboard")
	}
	// Fence the sidecar change against terminal takeover, as for mail/spawn.
	err := wsdashboard.WriteSessionHidden(c.r.opts.Dir, c.rec.ReplyID, hidden)
	if err == nil {
		c.setStatus(func() { c.hidden = hidden })
	}
	unlock()
	if err != nil {
		return err
	}
	if hidden {
		// Persist the no-start gate first: process-end/replay callbacks cannot
		// consume held input while the process is being stopped.
		c.stopProcess("")
		if err := c.retireHolder(); err != nil {
			// Do not report a successful processless archive when a holder
			// refused to stop. Restore visibility, but retain the error.
			if restore := wsdashboard.WriteSessionHidden(c.r.opts.Dir, c.rec.ReplyID, false); restore == nil {
				c.setStatus(func() { c.hidden = false })
			}
			return fmt.Errorf("the session was not archived: %w", err)
		}
	} else {
		c.signalMail()
		c.signalInbox()
	}
	c.writeRegistration()
	c.r.notify(c.rec.ReplyID, c.rec.Root)
	return nil
}

// PageInput is the explicit own-page-input exception to passive archive
// no-wake behavior. It is called only after a valid composer message was held;
// generic InboxChanged signals and Runner.Send remain passive.
func (r *Runner) PageInput(id string) error {
	c := r.conv(id)
	if c == nil {
		return errUnknown
	}
	var err error
	if !c.do(func() {
		if c.closed {
			err = endedError(c.rec)
		} else if c.hidden {
			err = c.setHidden(false)
		}
	}) {
		return errClosed
	}
	return err
}
