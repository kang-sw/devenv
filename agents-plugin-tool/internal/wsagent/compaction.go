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

// RestoreCause identifies why the same transient hosted-state bundle is being
// delivered. It is runtime-only and is never written to a conversation record.
type RestoreCause string

const (
	RestoreCompaction RestoreCause = "compaction"
	RestoreRestart    RestoreCause = "restart"

	compactionRestoreMarker = "\x00ws-restore:compaction\x00"
	restartRestoreMarker    = "\x00ws-restore:restart\x00"
)

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
	resume   bool
	internal bool
	err      error
}

type compactionNudge struct {
	text       string
	level      int // 1 for soft, 2 and 3 for the two milestones
	standalone bool
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
		req := &compactRequest{resume: again, internal: dest.compactInternalTurn}
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

func (c *conv) compactMilestones(window float64) (m1, m2 float64, ok bool) {
	if !c.compactAdvisoryEnabled || window <= 0 {
		return 0, 0, false
	}
	soft := c.compactAdvisory.Point(window)
	hard := c.compactHard.Point(window)
	if math.IsInf(soft, 0) || math.IsInf(hard, 0) || !(hard > soft) {
		return 0, 0, false
	}
	step := (hard - soft) / 3
	return soft + step, soft + 2*step, true
}

func (c *conv) usageCompaction(window int64) *wslive.UsageCompaction {
	if !c.compactLoaded || window <= 0 {
		return nil
	}
	w := float64(window)
	finite := func(v float64) int64 {
		if math.IsInf(v, 0) || math.IsNaN(v) || v <= 0 {
			return 0
		}
		return int64(math.Round(v))
	}
	soft := finite(c.compactAdvisory.Point(w))
	hard := finite(c.compactHard.Point(w))
	var milestones []int64
	if m1, m2, ok := c.compactMilestones(w); ok {
		milestones = []int64{int64(math.Round(m1)), int64(math.Round(m2))}
	}
	if soft == 0 && hard == 0 && len(milestones) == 0 {
		return nil
	}
	return &wslive.UsageCompaction{
		Soft:            soft,
		Hard:            hard,
		Milestones:      milestones,
		AdvisoryEnabled: c.compactAdvisoryEnabled,
	}
}

func (c *conv) disarmCompactionLatches() {
	if c.compactTokens < c.compactAdvisory.Point(c.compactWindow) {
		c.compactAdvised = false
	}
	if m1, m2, ok := c.compactMilestones(c.compactWindow); ok {
		if c.compactTokens < m1 {
			c.compactM1Issued = false
		}
		if c.compactTokens < m2 {
			c.compactM2Issued = false
		}
	} else {
		c.compactM1Issued, c.compactM2Issued = false, false
	}
	if c.compactTokens < c.compactHard.Point(c.compactWindow) {
		c.compactHardIssued = false
	}
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
		c.disarmCompactionLatches()
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
	c.disarmCompactionLatches()
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
	if (strings.HasPrefix(c.compactNote, restartRestoreMarker) || strings.HasPrefix(c.compactNote, compactionRestoreMarker)) && !c.r.startupReady() {
		return true
	}
	if c.compactPending == nil && c.compactUsageFresh {
		c.refreshCompaction()
		hard := c.compactHard.Point(c.compactWindow)
		if c.compactTokens >= hard && !c.compactHardIssued {
			c.compactHardIssued, c.compactAdvised, c.compactM1Issued, c.compactM2Issued = true, true, true, true
			c.compactNudge = compactionNudge{}
			if why := c.compactUnsupported(); why == "" {
				c.compactPending = &compactRequest{}
			} else {
				c.setStatus(func() { c.lastErr = why })
				if c.compactAdvisoryEnabled {
					c.compactNote += fmt.Sprintf("Context usage is %.0f tokens (%.0f%%); hard point %.0f tokens (%.0f%%). Native compaction is unsupported: %s. Write durable state to your agenda before compacting.\n", c.compactTokens, c.compactTokens/c.compactWindow*100, hard, hard/c.compactWindow*100, why)
				}
			}
		} else {
			c.stageCompactionNudge(hard)
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
		c.compactResumeInternal = req.resume && req.internal
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
		text := c.renderRestoreNote(c.compactNote) + "\nContinue the work that was pending."
		c.compactResume = false
		if err := c.proc.Send(text); err != nil {
			c.compactResumeInternal = false
			c.setStatus(func() { c.lastErr = err.Error() })
			c.releaseHeld()
			return false
		}
		c.compactNote = ""
		c.compactInternalTurn, c.compactResumeInternal = c.compactResumeInternal, false
		c.beginTurn(false)
		return true
	}
	if strings.HasPrefix(c.compactNote, restartRestoreMarker) && c.proc != nil {
		if c.restartRestoreHasInput() {
			// Let the ordinary turn path prepend the bundle instead of opening
			// a standalone restore turn ahead of input that already waits.
			return false
		}
		text := c.renderRestoreNote(c.compactNote)
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

// stageCompactionNudge prepares the note and the levels its delivery covers.
// A jump from below soft still uses the first advisory's wording.
func (c *conv) stageCompactionNudge(hard float64) {
	soft := c.compactAdvisory.Point(c.compactWindow)
	level := 0
	var point float64
	if c.compactAdvisoryEnabled && c.compactTokens >= soft {
		level, point = 1, soft
		if m1, m2, ok := c.compactMilestones(c.compactWindow); ok {
			switch {
			case c.compactTokens >= m2:
				level, point = 3, m2
			case c.compactTokens >= m1:
				level, point = 2, m1
			}
		}
	}
	if level == 0 || c.compactAdvised && (level == 1 || level == 2 && c.compactM1Issued || level == 3 && c.compactM2Issued) {
		c.compactNudge = compactionNudge{}
		return
	}
	standalone := c.compactUnsupported() == ""
	if c.compactNudge.level == level && c.compactNudge.standalone == standalone {
		return
	}
	usage := fmt.Sprintf("Context usage is %.0f tokens (%.0f%%); ", c.compactTokens, c.compactTokens/c.compactWindow*100)
	if c.compactAdvised {
		usage += fmt.Sprintf("compaction reminder %d/3 (%.0f tokens, hard point %.0f tokens). ", level-1, point, hard)
	} else {
		usage += fmt.Sprintf("advisory point %.0f tokens (%.0f%%). ", soft, soft/c.compactWindow*100)
	}
	action := "Write durable state to your agenda before compacting."
	if standalone {
		action = "Write durable state to your agenda, then call ws/compact at a safe boundary before continuing."
	}
	c.compactNudge = compactionNudge{usage + action + " No summary prose is requested.", level, standalone}
}

func (c *conv) consumeCompactionNudge() {
	if level := c.compactNudge.level; level > 0 {
		c.compactAdvised = true
		c.compactM1Issued = c.compactM1Issued || level >= 2
		c.compactM2Issued = c.compactM2Issued || level >= 3
		c.compactNudge = compactionNudge{}
	}
}

func (c *conv) compactionNudgeReady() bool {
	return c.compactNudge.standalone && c.compactUsageFresh && c.proc != nil &&
		!c.compacting && !c.compactCommand && c.compactPending == nil &&
		c.bgTasks == 0 && c.bgGraceTimer == nil
}

func (c *conv) sendCompactionNudge() {
	if !c.compactionNudgeReady() || c.hasActiveDescendants() {
		return
	}
	c.releaseHeld()
	if c.heldRes != nil {
		return
	}
	if err := c.proc.Send(c.compactNudge.text); err != nil {
		// Keep the note for ordinary input. A failed internal send has no
		// queued work to restart the harness for.
		c.setStatus(func() { c.lastErr = "send compaction advisory: " + err.Error() })
		c.stopProcess("")
		return
	}
	c.consumeCompactionNudge()
	c.compactInternalTurn = true
	c.beginTurn(false)
}

func (c *conv) restartRestoreHasInput() bool {
	if len(c.rec.Queued) > 0 || len(c.rec.Mail) > 0 || len(c.rec.Reports) > 0 {
		return true
	}
	held, err := wsinbox.Held(c.inbox)
	return err != nil || len(held) > 0
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
		c.compactNudge = compactionNudge{}
		c.compactAdvised, c.compactM1Issued, c.compactM2Issued, c.compactHardIssued = true, true, true, true
	case wslive.CompactFailed:
		c.compacting, c.compactResume = false, false
		c.compactResumeInternal = false
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
		c.compactAdvised, c.compactM1Issued, c.compactM2Issued, c.compactHardIssued, c.compactUsageFresh = false, false, false, false, false
		c.queueRestore(RestoreCompaction)
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
	if c.compactNudge.text != "" {
		text = c.compactNudge.text + "\n\n" + text
	}
	if c.compactNote != "" {
		text = strings.TrimSpace(c.renderRestoreNote(c.compactNote)) + "\n\n" + text
	}
	return text
}

func (c *conv) queueRestore(cause RestoreCause) {
	marker := compactionRestoreMarker
	if cause == RestoreRestart {
		marker = restartRestoreMarker
	}
	if c.r == nil || c.r.opts.RestoreBundle == nil {
		if cause == RestoreCompaction {
			c.compactNote = fmt.Sprintf("Your ws session key is `%s`. Context was compacted. Fresh session state is available through the lead-only ws workflow_state tool. Keep using this same key.\n", c.rec.WSKey)
		}
		return
	}
	c.compactNote = marker
}

// renderRestoreNote resolves the marker at the last possible point before a
// send. The marker stays in compactNote until that send succeeds, so a failed
// send retries with newly read state instead of caching a partial bundle.
func (c *conv) renderRestoreNote(note string) string {
	cause, tail := RestoreCause(""), ""
	switch {
	case strings.HasPrefix(note, compactionRestoreMarker):
		cause, tail = RestoreCompaction, strings.TrimPrefix(note, compactionRestoreMarker)
	case strings.HasPrefix(note, restartRestoreMarker):
		cause, tail = RestoreRestart, strings.TrimPrefix(note, restartRestoreMarker)
	default:
		return note
	}
	rec := c.record()
	cfg := c.r.opts.Config
	cfg.RepoRoot = rec.Root
	caller := LeadCaller(rec.ReplyID, rec.Root, rec.Backend, cfg)
	if !rec.Lead {
		caller = c.r.holderCaller(rec)
	}
	bundle, err := c.r.opts.RestoreBundle(rec.Backend, rec.workDir(), rec.WSKey, cause, c.r.AgentList(caller))
	if err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: restore bundle: %v\n", rec.ReplyID, err)
		bundle = fmt.Sprintf("Your ws session key is `%s`. The hosted session-state restore bundle could not be rendered after %s: %v. Call workflow_state with this same key before continuing.", rec.WSKey, cause, err)
	}
	if strings.TrimSpace(tail) != "" {
		bundle = strings.TrimSpace(bundle) + "\n\n" + strings.TrimSpace(tail)
	}
	return bundle
}
