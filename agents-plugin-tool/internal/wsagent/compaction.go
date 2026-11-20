package wsagent

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// compactCommandTimeout bounds how long a dispatched native compact command
// may wait without a harness response before stopping the stalled process.
const compactCommandTimeout = 10 * time.Minute

func (r BackendRegistration) CompactionProtocol() CompactionProtocol {
	if r.NativeCompaction != nil {
		return *r.NativeCompaction
	}
	if r.Catalog != nil {
		return r.Catalog.Compaction
	}
	return CompactionProtocol{}
}

type compactRequest struct {
	resume bool
	err    error
}

// CompactResult is the additive compact tool's result. Queued does not claim
// completion: only a native successful end can restore context.
type CompactResult struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// Compact accepts self's key or an owned descendant's spawn address/key.
// The presented key is the authority on child connections, as for mailbox
// control; no key or lineage changes during compaction.
func (r *Runner) Compact(callerKey, target string, resume *bool) CompactResult {
	target = strings.TrimSpace(target)
	var own, dest *conv
	r.mu.Lock()
	for _, c := range r.convs {
		rec := c.record()
		if rec.WSKey == callerKey {
			own = c
		}
		if rec.WSKey == target || AgentAddress(rec.ReplyID) == target {
			dest = c
		}
	}
	r.mu.Unlock()
	if own == nil {
		return CompactResult{"unsupported", "the caller is not a hosted session"}
	}
	if target == "" || target == callerKey {
		dest = own
	}
	if dest == nil {
		return CompactResult{"error", "target is not an open hosted session"}
	}
	if dest != own && !r.compactDescendant(own.record(), dest.record()) {
		return CompactResult{"error", "target is not this session's descendant"}
	}
	again := dest == own
	if resume != nil {
		again = *resume
	}
	out := CompactResult{"error", "session closed"}
	dest.do(func() {
		if why := dest.compactUnsupported(); why != "" {
			out = CompactResult{"unsupported", why}
			return
		}
		if dest.proc == nil {
			out = CompactResult{"error", "compaction requires a running harness"}
			return
		}
		if dest.compactPending != nil || dest.compacting || dest.compactCommand {
			out = CompactResult{"queued", "compaction already pending"}
			return
		}
		req := &compactRequest{resume: again}
		dest.compactPending = req
		out = CompactResult{"queued", "native compaction waits for the idle boundary"}
		dest.kick()
		if req.err != nil {
			out = CompactResult{"error", req.err.Error()}
		}
	})
	return out
}

func (r *Runner) compactDescendant(own, child Record) bool {
	for child.Parent != nil {
		if child.Parent.ReplyID == own.ReplyID {
			return true
		}
		parent := r.conv(child.Parent.ReplyID)
		if parent == nil {
			return false
		}
		child = parent.record()
	}
	return false
}

func (c *conv) compactUnsupported() string {
	p := c.r.opts.Backends[c.rec.Backend].CompactionProtocol()
	if !p.DetectsLifecycle() {
		return "native compaction lifecycle is unsupported by this harness descriptor"
	}
	if (c.rec.Lead && !p.LeadCommand) || (!c.rec.Lead && !p.WorkerCommand) {
		return "native compaction command is unsupported for this role"
	}
	if c.caps != nil && !c.caps.Allows(ItemCompaction) {
		return "the session catalog reports compaction unsupported"
	}
	return ""
}

// compactionConfigGen is bumped in memory whenever config.tune writes or resets
// a compaction key so running conversations re-resolve thresholds lazily at the
// next threshold check without file polling or watchers.
var compactionConfigGen atomic.Uint64

// NotifyCompactionConfigChanged signals running conversations to re-resolve
// compaction thresholds on their next threshold check.
func NotifyCompactionConfigChanged() {
	compactionConfigGen.Add(1)
}

// loadCompaction runs at process start, loading thresholds and resetting
// process-scoped usage observations.
func (c *conv) loadCompaction() {
	c.loadCompactionThresholds()
	c.compactLoaded = true
	c.compactUsageFresh = false
	c.compactPrevTokens, c.compactPrevWindow = 0, 0
}

func (c *conv) loadCompactionThresholds() {
	// Capture generation before reading config so a concurrent tune during the
	// read leaves compactGen behind and triggers one more reload next check.
	gen := compactionConfigGen.Load()
	cfg := c.r.opts.Config
	cfg.RepoRoot = c.rec.Root
	if roots, ok := c.r.opts.Keys.(interface{ Root(string) (string, bool) }); ok {
		if root, found := roots.Root(c.rec.WSKey); found {
			cfg.RepoRoot = root
		}
	}
	reader, _ := c.r.opts.Keys.(wsconfig.SessionReader)
	r := wsconfig.NewResolver(cfg, nil, reader, nil)
	p := c.r.opts.Backends[c.rec.Backend].CompactionProtocol()
	role := "worker"
	if c.rec.Lead {
		role = "lead"
	}
	c.compactAdvisoryEnabled = true
	if c.rec.Lead && p.AdvisoryEnabled != "" {
		v, err := r.Get(c.rec.WSKey, p.AdvisoryEnabled)
		c.compactAdvisoryEnabled = err == nil && v.Value != "false"
	}
	for _, slot := range []struct {
		name string
		to   *wsconfig.Threshold
	}{{"advisory", &c.compactAdvisory}, {"hard", &c.compactHard}} {
		key := "compaction." + role + "_" + slot.name
		v, _, err := r.Compaction(c.rec.WSKey, key, c.rec.Backend, p.LegacyKey(key))
		if err == nil {
			*slot.to, err = wsconfig.ParseThreshold(v.Value)
		}
		if err != nil {
			fmt.Fprintf(c.r.opts.Log, "agent %s: %s: %v\n", c.rec.ReplyID, key, err)
			*slot.to = wsconfig.Threshold{}
		}
	}
	c.compactGen = gen
}

// refreshCompaction lazily reloads thresholds when config.tune has modified a
// compaction key since this conversation last loaded them. Steady-state checks
// perform only an atomic load with zero file I/O.
func (c *conv) refreshCompaction() {
	if !c.compactLoaded || c.r == nil {
		return
	}
	if compactionConfigGen.Load() == c.compactGen {
		return
	}
	c.loadCompactionThresholds()
	if c.compactUsageFresh && !c.compacting {
		if c.compactTokens < c.compactAdvisory.Point(c.compactWindow) {
			c.compactAdvised = false
		}
		if c.compactTokens < c.compactHard.Point(c.compactWindow) {
			c.compactHardIssued = false
		}
	}
}

func (c *conv) observeCompaction(ev wslive.Event) {
	if ev.Kind == wslive.KindTurnStart {
		c.compactUsageFresh, c.compactAwaitTurn = false, false
	}
	if ev.Kind != wslive.KindStatus || ev.Context == nil || c.compactAwaitTurn {
		return
	}
	ctx := ev.Context
	c.compactUsageFresh = false
	if ctx.Tokens == nil || *ctx.Tokens < 0 || *ctx.Tokens > wslive.MaxContextTokens || ctx.Window <= 0 {
		return
	}
	c.compactTokens, c.compactWindow = float64(*ctx.Tokens), float64(ctx.Window)
	c.compactUsageFresh = true
	c.refreshCompaction()
	if c.compactTokens < c.compactAdvisory.Point(c.compactWindow) {
		c.compactAdvised = false
	}
	if c.compactTokens < c.compactHard.Point(c.compactWindow) {
		c.compactHardIssued = false
	}
}

// checkUsageDropCompaction detects native compaction completion on harnesses
// whose descriptor declares UsageDropRatio rather than explicit wire events.
// A window change resets the baseline (model/tokenizer switch, not compaction).
func (c *conv) checkUsageDropCompaction(ev wslive.Event) {
	if c.r == nil || ev.Kind != wslive.KindStatus || ev.Context == nil || ev.Context.Tokens == nil {
		return
	}
	p := c.r.opts.Backends[c.rec.Backend].CompactionProtocol()
	if p.UsageDropRatio <= 0 || p.UsageDropRatio >= 1 {
		return
	}
	used, window := *ev.Context.Tokens, ev.Context.Window
	if used <= 0 || used > wslive.MaxContextTokens || window <= 0 {
		return
	}
	if c.compactPrevWindow > 0 && window != c.compactPrevWindow {
		c.compactPrevTokens, c.compactPrevWindow = 0, window
		return
	}
	prev := c.compactPrevTokens
	c.compactPrevTokens, c.compactPrevWindow = used, window
	if prev <= 0 {
		return
	}
	dropped := prev - used
	if dropped < p.UsageDropMinDelta || float64(used) >= float64(prev)*p.UsageDropRatio {
		return
	}
	trigger := wslive.TriggerAuto
	if c.compacting {
		trigger = wslive.TriggerManual
	} else {
		c.compacting = true
	}
	c.compactEvent(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactEnd, Trigger: trigger})
	// This sample is already post-compaction: keep it as the new baseline and
	// fresh usage so the next turn does not re-trigger on the same drop.
	c.compactPrevTokens = used
	c.compactAwaitTurn = false
	c.compactTokens, c.compactWindow, c.compactUsageFresh = float64(used), float64(window), true
	c.apply(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactEnd, Trigger: trigger})
	if !c.compactCommand {
		c.stopCompactWatchdog()
	}
}

// compactIdle precedes inbox reads. All crossing/lifecycle state is in memory.
func (c *conv) compactIdle() bool {
	if c.compacting || c.compactCommand {
		return true
	}
	if c.compactPending == nil && c.compactUsageFresh {
		c.refreshCompaction()
		if c.compactTokens >= c.compactHard.Point(c.compactWindow) && !c.compactHardIssued {
			c.compactHardIssued, c.compactAdvised = true, true
			if why := c.compactUnsupported(); why == "" {
				c.compactPending = &compactRequest{}
			} else {
				c.setStatus(func() { c.lastErr = why })
				if c.compactAdvisoryEnabled {
					c.compactNote += fmt.Sprintf("Context usage is %.0f tokens (%.0f%%); hard point %.0f tokens (%.0f%%). Native compaction is unsupported: %s. Write durable state to your agenda before compacting.\n", c.compactTokens, c.compactTokens/c.compactWindow*100, c.compactHard.Point(c.compactWindow), c.compactHard.Point(c.compactWindow)/c.compactWindow*100, why)
				}
			}
		} else if c.compactAdvisoryEnabled && !c.compactAdvised && c.compactTokens >= c.compactAdvisory.Point(c.compactWindow) {
			c.compactAdvised = true
			point := c.compactAdvisory.Point(c.compactWindow)
			if !math.IsInf(point, 0) {
				if c.compactUnsupported() == "" {
					c.compactNote += fmt.Sprintf("\nContext usage is %.0f tokens (%.0f%%); advisory point %.0f tokens (%.0f%%). Write durable state to your agenda, then call ws/compact at a safe boundary before continuing. No summary prose is requested.\n", c.compactTokens, c.compactTokens/c.compactWindow*100, point, point/c.compactWindow*100)
				} else {
					c.compactNote += fmt.Sprintf("\nContext usage is %.0f tokens (%.0f%%); advisory point %.0f tokens (%.0f%%). Write durable state to your agenda before compacting. No summary prose is requested.\n", c.compactTokens, c.compactTokens/c.compactWindow*100, point, point/c.compactWindow*100)
				}
			}
		}
	}
	if req := c.compactPending; req != nil {
		if c.proc == nil {
			c.compactPending = nil
			c.setStatus(func() { c.lastErr = "compaction requires a running harness" })
			c.releaseHeld()
			return false
		}
		c.compactPending = nil
		c.compactResume, c.compactCommand = req.resume, true
		c.compactEvent(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactStart})
		c.apply(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactStart, Trigger: wslive.TriggerManual})
		if err := c.runCommand(wsinbox.Command{Name: "compact"}); err != nil {
			req.err = err
			c.compactCommand = false
			c.stopCompactWatchdog()
			c.compactEvent(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactFailed})
			c.apply(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactFailed})
			c.setStatus(func() { c.lastErr = "compaction: " + err.Error() })
			c.releaseHeld()
			return false
		}
		return true
	}
	if c.compactResume && c.compactNote != "" && c.proc != nil {
		text := c.compactNote + "\nContinue the work that was pending."
		c.compactResume = false
		if err := c.proc.Send(text); err != nil {
			c.setStatus(func() { c.lastErr = err.Error() })
			c.releaseHeld()
			return false
		}
		c.compactNote = ""
		c.beginTurn(false)
		return true
	}
	return false
}

func (c *conv) compactHoldsReport() bool {
	return (c.compactPending != nil && c.compactPending.resume) ||
		((c.compacting || c.compactCommand || c.compactNote != "") && c.compactResume)
}

func (c *conv) armCompactWatchdog() {
	c.stopCompactWatchdog()
	if c.r != nil && c.r.opts.Timer != nil {
		c.compactC, c.compactStop = c.r.opts.Timer(compactCommandTimeout)
	}
}

func (c *conv) stopCompactWatchdog() {
	if c.compactStop != nil {
		c.compactStop()
	}
	c.compactC, c.compactStop = nil, nil
}

func (c *conv) compactTimedOut() {
	c.compactC, c.compactStop = nil, nil
	if !c.compacting && !c.compactCommand {
		return
	}
	c.compactEvent(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactFailed})
	c.apply(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactFailed, Text: "compaction timed out"})
	c.setStatus(func() { c.lastErr = "compaction timed out" })
	c.stopProcess("compaction_timeout")
	c.changed(false)
	c.kick()
	c.releaseHeld()
}

func (c *conv) compactEvent(ev wslive.Event) {
	if ev.Kind != wslive.KindCompact {
		return
	}
	switch ev.Phase {
	case wslive.CompactStart:
		c.stopIdle()
		c.compacting = true
		c.compactUsageFresh = false
		c.compactAdvised, c.compactHardIssued = true, true
	case wslive.CompactFailed:
		c.compacting, c.compactResume = false, false
	case wslive.CompactEnd:
		// Duplicate ends carry neither a second note nor another continue turn.
		if !c.compacting {
			return
		}
		c.compacting = false
		// A stats answer queued before compaction may still carry old tokens.
		// Only the next model turn can produce a fresh post-compaction sample.
		c.compactAwaitTurn = true
		c.compactPrevTokens = 0
		c.compactAdvised, c.compactHardIssued, c.compactUsageFresh = false, false, false
		c.compactNote = fmt.Sprintf("Your ws session key is `%s`. Context was compacted. Fresh session state is available through the lead-only ws workflow_state tool. Keep using this same key.\n", c.rec.WSKey)
		if !c.r.opts.Backends[c.rec.Backend].CompactionProtocol().PromptPersists {
			if c.rec.Lead {
				c.compactNote += c.leadManual(c.record())
			} else {
				if c.systemPrompt == "" && c.rec.SystemPrompt && c.dir != "" {
					if raw, err := os.ReadFile(filepath.Join(c.dir, systemPromptFile)); err == nil {
						c.systemPrompt = string(raw)
					}
				}
				if c.systemPrompt != "" {
					c.compactNote += c.systemPrompt + "\n"
				}
			}
		}
	}
}

func (c *conv) compactInput(text string) string {
	if c.compactNote == "" {
		return text
	}
	text = strings.TrimSpace(c.compactNote) + "\n\n" + text
	return text
}
