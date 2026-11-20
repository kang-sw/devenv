package wsagent

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wslive"
)

// DefaultAgyModelWaitTimeoutSeconds is the ws package's default: seconds of raw
// ACP session/update silence during an active agy model wait before ws cancels
// and retries once with `continue`. The wsflow package supplies zero instead.
const DefaultAgyModelWaitTimeoutSeconds = 45

// agyAPIErrorPrefix matches agy's in-band API error agent_message_chunk text
// (after trimming leading/trailing whitespace such as the leading newline).
const agyAPIErrorPrefix = "⚠ API error"

// loadModelWait reads agy.model_wait_timeout_seconds once at process start for
// hosted agy leads and workers. Never touches disk during turns or timer ticks.
func (c *conv) loadModelWait() {
	if c.rec.Backend != BackendAgy {
		c.modelWaitTimeout = 0
		return
	}
	cfg := c.r.opts.Config
	cfg.RepoRoot = c.rec.Root
	if roots, ok := c.r.opts.Keys.(interface{ Root(string) (string, bool) }); ok {
		if root, found := roots.Root(c.rec.WSKey); found {
			cfg.RepoRoot = root
		}
	}
	reader, _ := c.r.opts.Keys.(wsconfig.SessionReader)
	sec := DefaultAgyModelWaitTimeoutSeconds
	if c.r.opts.AgyModelWaitTimeoutSeconds != nil {
		sec = *c.r.opts.AgyModelWaitTimeoutSeconds
	}
	r := wsconfig.NewResolver(cfg, map[string]string{
		wsconfig.ItemAgyModelWaitTimeoutSeconds: strconv.Itoa(sec),
	}, reader, nil)
	rv, err := r.Get(c.rec.WSKey, wsconfig.ItemAgyModelWaitTimeoutSeconds)
	if err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: %s: %v\n", c.rec.ReplyID, wsconfig.ItemAgyModelWaitTimeoutSeconds, err)
	} else if n, perr := strconv.Atoi(strings.TrimSpace(rv.Value)); perr != nil || n < 0 {
		fmt.Fprintf(c.r.opts.Log, "agent %s: %s: invalid value %q; using %d\n", c.rec.ReplyID, wsconfig.ItemAgyModelWaitTimeoutSeconds, rv.Value, sec)
	} else {
		sec = n
	}
	c.modelWaitTimeout = time.Duration(sec) * time.Second
}

// startModelWait arms the model-wait watchdog for a freshly sent ordinary agy
// prompt turn. Compaction, replayed, and unsolicited turns do not call this.
func (c *conv) startModelWait() {
	c.disarmModelWait()
	if c.rec.Backend != BackendAgy || c.modelWaitTimeout <= 0 || c.replaying || c.compacting || c.compactCommand {
		return
	}
	c.modelWaitArmed = true
	c.resetModelWaitTimer()
}

func (c *conv) resetModelWaitTimer() {
	c.stopModelWaitTimer()
	if !c.modelWaitArmed || c.modelWaitCancelling || c.modelWaitFailErr != "" || c.modelWaitTimeout <= 0 {
		return
	}
	if c.modelWaitPending > 0 || len(c.approvals) > 0 {
		return
	}
	if c.r != nil && c.r.opts.Timer != nil {
		c.modelWaitC, c.modelWaitStop = c.r.opts.Timer(c.modelWaitTimeout)
	}
}

func (c *conv) stopModelWaitTimer() {
	if c.modelWaitStop != nil {
		c.modelWaitStop()
	}
	c.modelWaitC, c.modelWaitStop = nil, nil
}

func (c *conv) disarmModelWait() {
	c.stopModelWaitTimer()
	c.modelWaitArmed = false
	c.modelWaitRetries = 0
	c.modelWaitPending = 0
	c.modelWaitCancelling = false
	c.modelWaitRetryFromAPIError = false
	c.modelWaitFailErr = ""
}

// onModelActivity handles a raw ACP session/update activity signal carrying
// the current count of pending tool calls (including daemon run_command calls).
func (c *conv) onModelActivity(pending int) {
	if !c.modelWaitArmed {
		return
	}
	c.modelWaitPending = pending
	if c.modelWaitCancelling || c.modelWaitFailErr != "" {
		return
	}
	if pending > 0 || len(c.approvals) > 0 {
		c.stopModelWaitTimer()
		return
	}
	c.resetModelWaitTimer()
}

func (c *conv) modelWaitTimedOut() {
	c.modelWaitC, c.modelWaitStop = nil, nil
	if !c.modelWaitArmed || !c.turn || c.proc == nil || c.modelWaitCancelling || c.modelWaitFailErr != "" {
		return
	}
	if c.modelWaitPending > 0 || len(c.approvals) > 0 {
		return
	}
	dur := formatWatchdogDuration(c.modelWaitTimeout)
	if c.modelWaitRetries == 0 {
		c.modelWaitCancelling = true
		fmt.Fprintf(c.r.opts.Log, "agent %s: agy model stream stalled for %s; interrupting to retry once\n", c.rec.ReplyID, dur)
		if err := c.proc.Interrupt(); err != nil {
			c.failModelWaitImmediately(fmt.Sprintf("agy model stream stalled for %s and interrupt failed: %v", dur, err))
		}
		return
	}
	reason := fmt.Sprintf("agy model stream stalled for %s twice", dur)
	if c.modelWaitRetryFromAPIError {
		reason = fmt.Sprintf("agy model stream stalled for %s after API error retry", dur)
	}
	c.modelWaitFailErr = reason
	fmt.Fprintf(c.r.opts.Log, "agent %s: %s\n", c.rec.ReplyID, reason)
	if err := c.proc.Interrupt(); err != nil {
		c.failModelWaitImmediately(fmt.Sprintf("%s (interrupt failed: %v)", reason, err))
	}
}

// interceptModelWaitTurnEnd intercepts the cancelled TurnEnd after timeout #1
// or an in-band "⚠ API error" TurnEnd while the single retry budget is unused,
// keeps the logical ws turn open, sends `continue`, and rearms the watchdog.
func (c *conv) interceptModelWaitTurnEnd(ev Event) bool {
	if !c.modelWaitArmed || !c.turn || c.proc == nil {
		return false
	}
	if c.modelWaitCancelling {
		c.modelWaitCancelling = false
		// If the prompt actually finished normally before session/cancel took
		// effect (and not on an in-band API error), let it settle as done.
		if ev.TurnReason != wslive.TurnAborted && ev.TurnError == "" && ev.APIError == "" {
			return false
		}
		c.modelWaitRetries = 1
		c.modelWaitRetryFromAPIError = false
		c.modelWaitPending = 0
		c.reply = ""
		if err := c.proc.Send("continue"); err != nil {
			c.failModelWaitImmediately(fmt.Sprintf("agy model stream stalled for %s and retry send failed: %v", formatWatchdogDuration(c.modelWaitTimeout), err))
			return true
		}
		c.resetModelWaitTimer()
		return true
	}
	if ev.APIError != "" && c.modelWaitFailErr == "" && c.modelWaitRetries == 0 {
		c.modelWaitRetries = 1
		c.modelWaitRetryFromAPIError = true
		c.modelWaitPending = 0
		c.reply = ""
		fmt.Fprintf(c.r.opts.Log, "agent %s: agy reported in-band API error; retrying once with continue (%s)\n", c.rec.ReplyID, ev.APIError)
		if err := c.proc.Send("continue"); err != nil {
			c.failModelWaitImmediately(fmt.Sprintf("agy API error retry send failed: %v (%s)", err, ev.APIError))
			return true
		}
		c.resetModelWaitTimer()
		return true
	}
	return false
}

// modelWaitTurnError returns the TurnFailed error message to apply when a
// watched turn settles after exhausting its single retry budget.
func (c *conv) modelWaitTurnError(ev Event) string {
	if !c.modelWaitArmed {
		return ""
	}
	if c.modelWaitFailErr != "" {
		return c.modelWaitFailErr
	}
	if ev.APIError != "" && c.modelWaitRetries >= 1 {
		return "agy API error after retry: " + ev.APIError
	}
	return ""
}

func (c *conv) failModelWaitImmediately(reason string) {
	c.disarmModelWait()
	c.setStatus(func() { c.lastErr = reason })
	fmt.Fprintf(c.r.opts.Log, "agent %s: %s\n", c.rec.ReplyID, reason)
	c.stopProcess("")
}

func formatWatchdogDuration(d time.Duration) string {
	if d >= time.Second && d%time.Second == 0 {
		return fmt.Sprintf("%ds", int64(d/time.Second))
	}
	return d.String()
}
