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

// agyAPIErrorPrefix starts agy's in-band API error line. The full matcher is
// deliberately anchored to the accumulated reply tail: agy can split the
// marker across chunks, and later reply text means the stream recovered.
const agyAPIErrorPrefix = "⚠ API error"

const (
	agyStreamInterrupted = "⚠ The stream was interrupted. Please continue the task you were working on."
	modelFailureDrain    = time.Second
	modelWaitCancelGrace = 5 * time.Second
)

func agyReplyTailFailure(reply string) string {
	line := strings.TrimRight(reply, " \t\r\n")
	if i := strings.LastIndexByte(line, '\n'); i >= 0 {
		line = line[i+1:]
	}
	line = strings.TrimSuffix(line, "\r")
	if line == agyStreamInterrupted {
		return line
	}
	prefix := agyAPIErrorPrefix + " (attempt "
	if !strings.HasPrefix(line, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(line, prefix)
	end := strings.Index(rest, "): ")
	if end <= 0 || strings.TrimSpace(rest[end+3:]) == "" {
		return ""
	}
	for _, r := range rest[:end] {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return line
}

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
	if !c.modelWaitArmed || c.modelWaitCancelling || c.modelWaitFailure != "" || c.modelWaitFailErr != "" || c.modelWaitTimeout <= 0 {
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

func (c *conv) stopModelFailureDrain() {
	if c.modelWaitDrainStop != nil {
		c.modelWaitDrainStop()
	}
	c.modelWaitDrainC, c.modelWaitDrainStop = nil, nil
}

func (c *conv) stopModelWaitCancelGrace() {
	if c.modelWaitCancelStop != nil {
		c.modelWaitCancelStop()
	}
	c.modelWaitCancelC, c.modelWaitCancelStop = nil, nil
}

func (c *conv) armModelWaitCancelGrace() {
	c.stopModelWaitCancelGrace()
	if c.r != nil && c.r.opts.Timer != nil {
		c.modelWaitCancelC, c.modelWaitCancelStop = c.r.opts.Timer(modelWaitCancelGrace)
	}
}

func (c *conv) disarmModelWait() {
	c.stopModelWaitTimer()
	c.stopModelFailureDrain()
	c.stopModelWaitCancelGrace()
	c.modelWaitArmed = false
	c.modelWaitRetries = 0
	c.modelWaitPending = 0
	c.modelWaitCancelling = false
	c.modelWaitRetryFromAPIError = false
	c.modelWaitFailErr = ""
	c.modelWaitDiagnostic = ""
	c.modelWaitFailure = ""
	c.modelWaitFailureExcluded = false
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
		c.stopModelFailureDrain()
		if c.modelWaitFailure != "" {
			c.modelWaitFailureExcluded = true
		}
		c.modelWaitFailure = ""
		return
	}
	if c.modelWaitFailure != "" {
		return
	}
	c.resetModelWaitTimer()
}

// onModelFailure starts a short drain when an anchored failure line becomes
// the accumulated reply tail. Later model output clears it and restores the
// ordinary silence timer; only a still-terminal failure is cancelled.
func (c *conv) onModelFailure(failure string) {
	if !c.modelWaitArmed || c.modelWaitCancelling || c.modelWaitFailErr != "" {
		return
	}
	if failure == "" {
		c.modelWaitFailureExcluded = false
		if c.modelWaitFailure == "" {
			return
		}
		c.modelWaitFailure = ""
		c.stopModelFailureDrain()
		c.resetModelWaitTimer()
		return
	}
	if !c.turn || c.proc == nil || c.modelWaitPending > 0 || len(c.approvals) > 0 {
		c.modelWaitFailureExcluded = true
		return
	}
	c.modelWaitFailureExcluded = false
	c.modelWaitFailure = failure
	c.stopModelWaitTimer()
	c.stopModelFailureDrain()
	if c.r != nil && c.r.opts.Timer != nil {
		c.modelWaitDrainC, c.modelWaitDrainStop = c.r.opts.Timer(modelFailureDrain)
	}
}

func (c *conv) modelFailureDrainTimedOut() {
	c.modelWaitDrainC, c.modelWaitDrainStop = nil, nil
	if !c.modelWaitArmed || !c.turn || c.proc == nil || c.modelWaitFailure == "" || c.modelWaitCancelling || c.modelWaitFailErr != "" {
		return
	}
	if c.modelWaitPending > 0 || len(c.approvals) > 0 {
		return
	}
	if c.modelWaitRetries == 0 {
		c.modelWaitCancelling = true
		fmt.Fprintf(c.r.opts.Log, "agent %s: agy model failure remained terminal for %s; interrupting before one retry (%s)\n", c.rec.ReplyID, modelFailureDrain, c.modelWaitFailure)
		if err := c.proc.Interrupt(); err != nil {
			c.failModelWaitImmediately(fmt.Sprintf("agy model failure interrupt failed: %v (%s)", err, c.modelWaitFailure))
		} else {
			c.armModelWaitCancelGrace()
		}
		return
	}
	c.modelWaitFailErr = "agy model failure after retry: " + c.modelWaitFailure
	fmt.Fprintf(c.r.opts.Log, "agent %s: %s\n", c.rec.ReplyID, c.modelWaitFailErr)
	if err := c.proc.Interrupt(); err != nil {
		c.failModelWaitImmediately(fmt.Sprintf("%s (interrupt failed: %v)", c.modelWaitFailErr, err))
	} else {
		c.armModelWaitCancelGrace()
	}
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
		} else {
			c.armModelWaitCancelGrace()
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
	} else {
		c.armModelWaitCancelGrace()
	}
}

func (c *conv) modelWaitCancelTimedOut() {
	c.modelWaitCancelC, c.modelWaitCancelStop = nil, nil
	if !c.modelWaitArmed || !c.turn || c.proc == nil || (!c.modelWaitCancelling && c.modelWaitFailErr == "") {
		return
	}
	reason := c.modelWaitFailErr
	if reason == "" {
		reason = fmt.Sprintf("agy model stream did not stop within %s after interrupt", formatWatchdogDuration(modelWaitCancelGrace))
	} else {
		reason += fmt.Sprintf(" and did not stop within %s after interrupt", formatWatchdogDuration(modelWaitCancelGrace))
	}
	c.failModelWaitImmediately(reason)
}

// interceptModelWaitTurnEnd intercepts the cancelled TurnEnd after timeout #1
// or a terminal in-band model failure while the single retry budget is unused,
// keeps the logical ws turn open, sends `continue`, and rearms the watchdog.
func (c *conv) interceptModelWaitTurnEnd(ev Event) bool {
	if !c.modelWaitArmed || !c.turn || c.proc == nil {
		return false
	}
	if ev.ModelFailure != "" && (c.modelWaitFailureExcluded || c.modelWaitPending > 0 || len(c.approvals) > 0) {
		return false
	}
	if c.modelWaitCancelling {
		c.stopModelWaitCancelGrace()
		c.modelWaitCancelling = false
		// If the prompt actually finished normally before session/cancel took
		// effect (and not on an in-band failure), let it settle as done.
		if ev.TurnReason != wslive.TurnAborted && ev.TurnError == "" && ev.ModelFailure == "" && c.modelWaitFailure == "" {
			return false
		}
		failure := ev.ModelFailure
		if failure == "" {
			failure = c.modelWaitFailure
		}
		return c.retryModelWait(failure)
	}
	if ev.ModelFailure != "" && c.modelWaitFailErr == "" && c.modelWaitRetries == 0 {
		return c.retryModelWait(ev.ModelFailure)
	}
	return false
}

func (c *conv) retryModelWait(failure string) bool {
	c.stopModelFailureDrain()
	c.modelWaitRetries = 1
	c.modelWaitRetryFromAPIError = strings.HasPrefix(failure, agyAPIErrorPrefix)
	c.modelWaitPending = 0
	c.modelWaitFailure = ""
	c.modelWaitFailureExcluded = false
	c.modelWaitDiagnostic = c.reply
	c.reply = ""
	if failure == "" {
		fmt.Fprintf(c.r.opts.Log, "agent %s: agy model stream stalled for %s; retrying once with continue\n", c.rec.ReplyID, formatWatchdogDuration(c.modelWaitTimeout))
	} else {
		fmt.Fprintf(c.r.opts.Log, "agent %s: agy reported terminal in-band model failure; retrying once with continue (%s)\n", c.rec.ReplyID, failure)
	}
	if err := c.proc.Send("continue"); err != nil {
		reason := fmt.Sprintf("agy model stream stalled for %s and retry send failed: %v", formatWatchdogDuration(c.modelWaitTimeout), err)
		if failure != "" {
			reason = fmt.Sprintf("agy model failure retry send failed: %v (%s)", err, failure)
		}
		c.failModelWaitImmediately(reason)
		return true
	}
	c.resetModelWaitTimer()
	return true
}

// modelWaitTurnError returns the TurnFailed error message to apply when a
// watched turn settles after exhausting its single retry budget.
func (c *conv) modelWaitTurnError(ev Event) string {
	if !c.modelWaitArmed {
		return ""
	}
	if ev.ModelFailure != "" && (c.modelWaitFailureExcluded || c.modelWaitPending > 0 || len(c.approvals) > 0) {
		return ""
	}
	if c.modelWaitFailErr != "" {
		c.restoreModelWaitDiagnostic()
		return c.modelWaitFailErr
	}
	if ev.ModelFailure != "" && c.modelWaitRetries >= 1 {
		c.restoreModelWaitDiagnostic()
		if strings.HasPrefix(ev.ModelFailure, agyAPIErrorPrefix) {
			return "agy API error after retry: " + ev.ModelFailure
		}
		return "agy model failure after retry: " + ev.ModelFailure
	}
	return ""
}

func (c *conv) restoreModelWaitDiagnostic() {
	if c.reply == "" {
		c.reply = c.modelWaitDiagnostic
	}
}

func (c *conv) failModelWaitImmediately(reason string) {
	c.restoreModelWaitDiagnostic()
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
