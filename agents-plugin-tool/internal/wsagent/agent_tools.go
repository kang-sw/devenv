package wsagent

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

// agent_tools.go is the agent.* tool family's runner side: agent.spawn,
// agent.interrupt, agent.list and agent.tiers, the delivery seam that wakes
// a sleeping child, and the child-agent-waiting expiry.
//
//   - Spawn. A child runs the internal agent-worker profile of the harness
//     its tier names (AgentWorkerProfile): the common ws tool loadout
//     (AgentWSTools), the tier's model and effort, the caller's system
//     prompt appended to the harness's own, and an authority no higher
//     than its parent's current level, stored as its explicit selection
//     (permission.json). The parent is the Runner spawn record
//     (Record.Parent); ownership, holds, expiry and cascades walk it.
//   - Admission. A child's authority is admitted against its parent's
//     current level at spawn and at every process start (childAdmission).
//     A refused start keeps its input and leaves it asleep; the delivery
//     seam (Deliver) tells the sender once per refused message.
//   - Sleep. A child's process ends on its idle timer; its record stays
//     open, and the next message starts it again. A child asleep with no
//     turn is waiting (SessionChildWaiting); after the configured days
//     without a send or settled turn it is closed and deleted with its
//     descendants, deepest first (expireIfDue).

// AgentWorkerProfile is the internal builtin profile an agent.spawn child
// of backend runs.
func AgentWorkerProfile(backend string) string { return "agent-worker-" + backend }

func isAgentWorkerProfile(name string) bool {
	// Reserve the internal namespace even for an unregistered backend:
	// compatibility spawn paths must never bypass agent.spawn admission.
	return strings.HasPrefix(name, "agent-worker-")
}

// RemovesSubagents reports whether rec's launches remove the harness's
// native subagent tool (StartSpec.RemoveSubagents), so that it delegates
// through agent.spawn alone: a hosted lead and an agent.spawn child do.
// The internal and dashboard profiles that rely on the native tool (the
// Claude tracker support profile, whose only tool it is) keep it.
func RemovesSubagents(rec Record) bool {
	return rec.Lead || isAgentWorkerProfile(rec.Profile)
}

// MaxSpawnWidth caps an agent's awake direct children; a sleeping child
// does not count, and a wake through Deliver ignores the cap.
const MaxSpawnWidth = 99

// DefaultChildWaitingDays is wsconfig.ItemAgentChildWaitingDays's builtin
// default.
const DefaultChildWaitingDays = 15

// EnvPermissionLevel carries a spawned child's authority to its attached
// ws-mcp, which refuses the mutating ws tools for read-only. Blank is none.
const EnvPermissionLevel = "WS_MCP_PERMISSION_LEVEL"

// systemPromptFile is the session directory's copy of an agent.spawn
// system prompt (Record.SystemPrompt).
const systemPromptFile = "system_prompt.md"

// maxSystemPrompt bounds a system_prompt_path read.
const maxSystemPrompt = 1 << 20

// AgentCaller is the agent.* caller: the parent a spawn records, and the
// root, directory, harness and config its tier and cwd resolve in.
type AgentCaller struct {
	Parent ParentRef
	// Root anchors the child's mail and config; WorkDir (empty: Root) is
	// what a relative cwd, worktree or system_prompt_path is joined to.
	Root    string
	WorkDir string
	// Harness is the caller's own harness: a bare tier resolves there.
	Harness string
	// Config is where agents.tier and agent.tier_aliases are read.
	Config wsconfig.Options
	// holder is a token holder's own record (a nested spawn): its depth
	// and tier cap the child's. Nil for a lead.
	holder *Record
}

// LeadCaller is a lead session's agent.* caller: replyID is the lead's
// mailbox reply-id, the parent its children record.
func LeadCaller(replyID, root, harness string, cfg wsconfig.Options) AgentCaller {
	return AgentCaller{Parent: ParentRef{Kind: ParentLead, ReplyID: replyID}, Root: root, Harness: harness, Config: cfg}
}

// holderCaller is a token holder's agent.* caller.
func (r *Runner) holderCaller(rec Record) AgentCaller {
	cfg := r.opts.Config
	cfg.RepoRoot = rec.Root
	return AgentCaller{
		Parent:  ParentRef{Kind: ParentConversation, ReplyID: rec.ReplyID},
		Root:    rec.Root,
		WorkDir: rec.workDir(),
		Harness: rec.Backend,
		Config:  cfg,
		holder:  &rec,
	}
}

func (c AgentCaller) base() string {
	if c.WorkDir != "" {
		return c.WorkDir
	}
	return c.Root
}

// AgentSpawnRequest is agent.spawn's arguments.
type AgentSpawnRequest struct {
	Prompt           string `json:"prompt"`
	Tier             string `json:"tier"`
	SystemPrompt     string `json:"system_prompt,omitempty"`
	SystemPromptPath string `json:"system_prompt_path,omitempty"`
	Authority        string `json:"authority"`
	Cwd              string `json:"cwd,omitempty"`
	Worktree         string `json:"worktree,omitempty"`
	Label            string `json:"label,omitempty"`
}

// AgentSpawnResult is agent.spawn's answer: the child's mailbox address,
// its control key, and its status (AgentStatus).
type AgentSpawnResult struct {
	Address string `json:"address"`
	Status  string `json:"status"`
}

// AgentInterruptResult is agent.interrupt's answer: the child's status
// and the messages it had not taken yet, handed back.
type AgentInterruptResult struct {
	Address  string   `json:"address"`
	Status   string   `json:"status"`
	Returned []string `json:"returned"`
}

// AgentChild is one agent.list entry.
type AgentChild struct {
	Address string `json:"address"`
	Label   string `json:"label,omitempty"`
	// Tier is <harness>.<tier>.
	Tier      string          `json:"tier"`
	Authority PermissionLevel `json:"authority,omitempty"`
	// State is AgentStatus's value.
	State string `json:"state"`
	// LastReport is the first line of its last settled turn's final
	// message, with that turn's status.
	LastReport string `json:"last_report,omitempty"`
	Model      string `json:"model,omitempty"`
	Effort     string `json:"effort,omitempty"`
	Cwd        string `json:"cwd,omitempty"`
	Held       int    `json:"held,omitempty"`
	Error      string `json:"error,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// AgentTierEntry is one spawnable tier and what it resolves to.
type AgentTierEntry struct {
	Tier   string `json:"tier"`
	Model  string `json:"model"`
	Effort string `json:"effort,omitempty"`
}

// AgentAliasEntry is one agent.tier_aliases entry: its target and, when
// the target is spawnable now, the model it resolves to; Error says why
// not.
type AgentAliasEntry struct {
	Alias  string `json:"alias"`
	Target string `json:"target"`
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
	Error  string `json:"error,omitempty"`
}

// AgentTiers is agent.tiers' answer: bare tiers on the caller's harness
// (Native), other harnesses' <harness>.<tier> (Ext), and the user's
// aliases.
type AgentTiers struct {
	Harness string            `json:"harness"`
	Native  []AgentTierEntry  `json:"native"`
	Ext     []AgentTierEntry  `json:"ext"`
	Aliases []AgentAliasEntry `json:"aliases"`
	// Unavailable names each registered worker harness no tier of which is
	// spawnable now, and why.
	Unavailable []string `json:"unavailable,omitempty"`
}

// Agent statuses: a turn runs; turn ended with background work running; the
// first message waits for the process; a process waits for input; asleep (no
// process, no turn); closed.
const (
	AgentRunning    = "running"
	AgentBackground = "background"
	AgentStarting   = "starting"
	AgentIdle       = "idle"
	AgentWaiting    = "waiting"
	AgentClosed     = "closed"
)

// AgentStatus is st's agent.* status.
func AgentStatus(st State) string {
	switch {
	case st.Status == StatusClosed || st.Status == StatusHandedOff:
		return AgentClosed
	case st.Status == StatusRunning:
		return AgentRunning
	case st.BackgroundTasks > 0:
		return AgentBackground
	case st.Process:
		return AgentIdle
	case st.Held > 0 && st.Error == "":
		return AgentStarting
	}
	return AgentWaiting
}

// AgentAddress is conversation id's mailbox address.
func AgentAddress(id string) string { return "id:" + id }

func agentID(address string) string {
	return strings.TrimPrefix(strings.TrimSpace(address), "id:")
}

// spawnableHarness reports why harness has no spawnable tier now: not
// registered, lead-only, or not installed. "" is spawnable.
func (r *Runner) spawnableHarness(harness string) string {
	reg, ok := r.opts.Backends[harness]
	if !ok || reg.Backend == nil {
		return fmt.Sprintf("harness %q is not registered in this daemon", harness)
	}
	if !reg.WorkerCapable || reg.WorkerProfile == "" {
		return fmt.Sprintf("harness %q is registered for hosted leads only, not for spawned workers", harness)
	}
	if ok, why := supportBackendAvailability(harness, reg.Backend); !ok {
		return why
	}
	if harness == BackendPi && r.opts.ChildCheck != nil {
		if err := r.opts.ChildCheck(); err != nil {
			return err.Error()
		}
	}
	return ""
}

// workerHarnesses are the registered worker-role backends, sorted.
func (r *Runner) workerHarnesses() []string {
	var out []string
	for name, reg := range r.opts.Backends {
		if reg.WorkerCapable && reg.WorkerProfile != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func tierEntries(prefix string, tiers map[string]wsconfig.AgentTier) []AgentTierEntry {
	var out []AgentTierEntry
	for _, t := range []string{"small", "medium", "large", "xlarge"} {
		if m, ok := tiers[t]; ok {
			out = append(out, AgentTierEntry{Tier: prefix + t, Model: m.Model, Effort: m.Effort})
		}
	}
	return out
}

// AgentTiers lists what caller may pass as agent.spawn's tier.
func (r *Runner) AgentTiers(caller AgentCaller) (AgentTiers, error) {
	out := AgentTiers{Harness: caller.Harness, Native: []AgentTierEntry{}, Ext: []AgentTierEntry{}, Aliases: []AgentAliasEntry{}}
	usable := map[string]map[string]wsconfig.AgentTier{}
	for _, h := range r.workerHarnesses() {
		if why := r.spawnableHarness(h); why != "" {
			out.Unavailable = append(out.Unavailable, h+": "+why)
			continue
		}
		tiers, err := wsconfig.HarnessTiers(caller.Config, h)
		if err != nil {
			return AgentTiers{}, fmt.Errorf("read agents.tier: %w", err)
		}
		usable[h] = tiers
		if h == caller.Harness {
			out.Native = tierEntries("", tiers)
		}
		out.Ext = append(out.Ext, tierEntries(h+".", tiers)...)
	}
	aliases, err := tierAliases(caller.Config)
	if err != nil {
		return AgentTiers{}, err
	}
	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		target := aliases[name]
		entry := AgentAliasEntry{Alias: name, Target: target}
		h, t, _ := strings.Cut(target, ".")
		if tiers, ok := usable[h]; !ok {
			entry.Error = fmt.Sprintf("harness %q has no spawnable tier now", h)
		} else if m, ok := tiers[t]; !ok {
			entry.Error = fmt.Sprintf("tier %q is not set for %s", t, h)
		} else {
			entry.Model, entry.Effort = m.Model, m.Effort
		}
		out.Aliases = append(out.Aliases, entry)
	}
	return out, nil
}

// tierAliases reads agent.tier_aliases (global-only) for cfg.
func tierAliases(cfg wsconfig.Options) (map[string]string, error) {
	resolver := wsconfig.NewResolver(cfg, map[string]string{wsconfig.ItemAgentTierAliases: ""}, nil, nil)
	raw, err := resolver.Get("", wsconfig.ItemAgentTierAliases)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", wsconfig.ItemAgentTierAliases, err)
	}
	return wsconfig.ParseTierAliases(raw.Value)
}

// resolveTier resolves agent.spawn's tier for caller: a bare tier on the
// caller's harness, <harness>.<tier>, or an alias. A tier no spawnable
// harness sets fails with the valid list.
func (r *Runner) resolveTier(caller AgentCaller, tier string) (harness, bare string, mapping wsconfig.AgentTier, err error) {
	raw := strings.ToLower(strings.TrimSpace(tier))
	if raw == "" {
		return "", "", mapping, r.tierRefusal(caller, "tier is required")
	}
	switch h, t, dotted := strings.Cut(raw, "."); {
	case dotted:
		harness, bare = h, wsconfig.NormalizedTier(t)
	case wsconfig.NormalizedTier(raw) != "":
		harness, bare = caller.Harness, wsconfig.NormalizedTier(raw)
	default:
		aliases, aerr := tierAliases(caller.Config)
		if aerr != nil {
			return "", "", mapping, aerr
		}
		target, ok := aliases[raw]
		if !ok {
			return "", "", mapping, r.tierRefusal(caller, fmt.Sprintf("tier %q is neither a tier, <harness>.<tier>, nor an alias in %s", tier, wsconfig.ItemAgentTierAliases))
		}
		h, t, _ := strings.Cut(target, ".")
		harness, bare = h, t
	}
	if bare == "" {
		return "", "", mapping, r.tierRefusal(caller, fmt.Sprintf("tier %q names no tier (small, medium, large, xlarge)", tier))
	}
	if why := r.spawnableHarness(harness); why != "" {
		return "", "", mapping, r.tierRefusal(caller, fmt.Sprintf("tier %q: %s", tier, why))
	}
	tiers, err := wsconfig.HarnessTiers(caller.Config, harness)
	if err != nil {
		return "", "", mapping, fmt.Errorf("read agents.tier: %w", err)
	}
	mapping, ok := tiers[bare]
	if !ok {
		return "", "", mapping, r.tierRefusal(caller, fmt.Sprintf("tier %q: %s.%s has no model in agents.tier (config.tune key \"agents.tier\" sets one)", tier, harness, bare))
	}
	return harness, bare, mapping, nil
}

// tierRefusal is a 400 naming why and every tier caller may spawn now.
func (r *Runner) tierRefusal(caller AgentCaller, why string) error {
	valid := []string{}
	if list, err := r.AgentTiers(caller); err == nil {
		for _, e := range list.Native {
			valid = append(valid, e.Tier)
		}
		for _, e := range list.Ext {
			valid = append(valid, e.Tier)
		}
		for _, a := range list.Aliases {
			if a.Error == "" {
				valid = append(valid, a.Alias)
			}
		}
	}
	if len(valid) == 0 {
		return badRequest("agent.spawn: %s; no tier is spawnable now (agent.tiers says why)", why)
	}
	return badRequest("agent.spawn: %s; valid tiers: %s", why, strings.Join(valid, ", "))
}

// admissionRefusal is a process start refused because the conversation's
// authority is above its parent's current level: its input stays, and it
// stays asleep.
type admissionRefusal struct{ reason string }

func (e *admissionRefusal) Error() string { return e.reason }

// holdParentAdmission keeps a parent's current selection stable from the
// child's admission read through the operation that follows it. Permission
// controls take the same session slot exclusively while changing selection.
func (r *Runner) holdParentAdmission(parent *ParentRef) func() {
	if parent == nil {
		return func() {}
	}
	slot := permissionSlot(wsdashboard.SessionDir(r.opts.Dir, parent.ReplyID))
	slot.admission.RLock()
	return slot.admission.RUnlock
}

// agentCeiling is the highest authority parent's children may take: its
// current level. A lead parent's reply-id resolves to its Runner record.
// agent.spawn delegation and omitted legacy Pi spawns retain the historical
// detached-lead ceiling; new explicit generic workers require current parent
// state. A conversation parent always needs its record and current level.
func (r *Runner) agentCeiling(parent ParentRef, detachedLeadFallback bool) (PermissionLevel, error) {
	rec, ok := r.recordOf(parent.ReplyID)
	dir := wsdashboard.SessionDir(r.opts.Dir, parent.ReplyID)
	if parent.Kind == ParentLead {
		if !ok || !rec.Lead {
			if detachedLeadFallback {
				return PermissionWorkdirWrite, nil
			}
			return "", fmt.Errorf("the parent lead %s has no hosted record, so its permission level is unknown", parent.ReplyID)
		}
		level, _, err := r.leadPermission(rec, dir)
		if err != nil {
			return "", fmt.Errorf("the parent lead's permission level is unreadable: %w", err)
		}
		if level == "" {
			if detachedLeadFallback {
				return PermissionWorkdirWrite, nil
			}
			return "", fmt.Errorf("the parent lead %s has no permission level of its own", parent.ReplyID)
		}
		return level, nil
	}
	if !ok {
		return "", fmt.Errorf("the parent conversation %s has no record, so its permission level is unknown", parent.ReplyID)
	}
	level, _, err := r.currentWorkerPermission(rec)
	if err != nil {
		return "", fmt.Errorf("the parent conversation's permission level is unreadable: %w", err)
	}
	if level.rank() == 0 {
		return "", fmt.Errorf("the parent conversation %s has no permission level of its own", parent.ReplyID)
	}
	return level, nil
}

// admitWorkerLevel applies the current-parent ceiling to a worker creation or
// launch. Root workers have no ceiling; a child with unresolved own authority
// is pending rather than guessed.
func (r *Runner) admitWorkerLevel(parent *ParentRef, level PermissionLevel, detachedLeadFallback bool) error {
	if parent == nil {
		return nil
	}
	if level.rank() == 0 {
		return &admissionRefusal{reason: "permission admission: the worker's own permission level is unresolved"}
	}
	ceiling, err := r.agentCeiling(*parent, detachedLeadFallback)
	if err != nil {
		return &admissionRefusal{reason: "permission admission: " + err.Error()}
	}
	if level.rank() > ceiling.rank() {
		return &admissionRefusal{reason: fmt.Sprintf("permission admission: authority %s is above the parent's current level %s", level, ceiling)}
	}
	return nil
}

// legacyDetachedWorkerLevel preserves the omitted top-level pi.spawn contract:
// an external lead has no Runner record, but its existing fixed Pi child
// loadout remains workdir-write/advisory. Hosted and conversation parents must
// still resolve normally, and explicit new generic selections get no fallback.
func (r *Runner) legacyDetachedWorkerLevel(parent *ParentRef, level PermissionLevel, source string) PermissionLevel {
	if level.rank() != 0 || source != "legacy-derived" || parent == nil || parent.Kind != ParentLead {
		return level
	}
	if _, ok := r.recordOf(parent.ReplyID); !ok {
		return PermissionWorkdirWrite
	}
	return level
}

// detachedWorkerFallback distinguishes new generic explicit workers (stored
// false) from agent.spawn (true), while absence preserves sidecars written
// before immutable provenance was introduced. Omitted legacy Pi spawns retain
// their pre-change detached-lead behavior without writing a sidecar.
func detachedWorkerFallback(dir, source string) bool {
	if source == "legacy-derived" {
		return true
	}
	delegated, set := delegatedPermissionState(dir)
	return delegated || !set
}

// childAdmission resolves a worker's current explicit or legacy-derived level,
// then admits it against its parent's current level. No derived value is
// stored, and an unresolved child never guesses a ceiling.
func (r *Runner) childAdmission(rec Record, def Profile) (PermissionLevel, string, error) {
	if rec.Lead {
		return "", "", nil
	}
	dir := wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID)
	level, source, err := r.workerPermission(rec, def, dir)
	if err != nil {
		return "", "", &admissionRefusal{reason: fmt.Sprintf("permission admission: own level unreadable: %v", err)}
	}
	if isAgentWorkerProfile(rec.Profile) && source != "explicit" {
		return "", "", &admissionRefusal{reason: "permission admission: agent.spawn child has no explicit authority record"}
	}
	level = r.legacyDetachedWorkerLevel(rec.Parent, level, source)
	if err := r.admitWorkerLevel(rec.Parent, level, detachedWorkerFallback(dir, source)); err != nil {
		return "", "", err
	}
	return level, source, nil
}

// childLevel is rec's explicit authority, "" for none.
func childLevel(dir string) PermissionLevel {
	level, explicit, err := cachedPermissionSelection(dir)
	if err != nil || !explicit {
		return ""
	}
	return level
}

// awakeLocked reports an awake child for the width cap and its parent's
// sleep: a process, a turn or background task, a held done turn, an initial
// spawn before its first Send, or input or claimed mail it has not taken
// whose start was not refused. Under mu.
func (c *conv) awakeLocked() bool {
	return !c.closed && (c.proc != nil || c.turn || c.bgTasks != 0 || c.holding || c.spawnPending ||
		(!c.wakeRefused && (c.sends > max(c.settled.mark, c.voided) || len(c.rec.Mail) > 0)))
}

// awakeChildren counts parent's open, awake direct children.
func (r *Runner) awakeChildren(parent ParentRef) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.awakeChildrenLocked(parent)
}

// awakeChildrenLocked counts parent's awake direct children plus in-flight
// width-checked spawns. Caller holds r.mu.
func (r *Runner) awakeChildrenLocked(parent ParentRef) int {
	n := r.pendingSpawns[parent]
	for _, c := range r.convs {
		c.mu.Lock()
		if c.rec.Parent != nil && *c.rec.Parent == parent && c.awakeLocked() {
			n++
		}
		c.mu.Unlock()
	}
	return n
}

// releasePendingSpawnLocked drops one in-flight spawn slot for parent.
// Caller holds r.mu.
func (r *Runner) releasePendingSpawnLocked(parent ParentRef) {
	if r.pendingSpawns[parent] <= 1 {
		delete(r.pendingSpawns, parent)
		return
	}
	r.pendingSpawns[parent]--
}

// reserveSpawnWidth checks parent's awake direct children under r.mu and
// reserves a slot for an in-flight agent.spawn. claimLocked transfers the
// slot when the conversation is registered in r.convs under r.mu; release
// drops an unclaimed slot on error.
func (r *Runner) reserveSpawnWidth(parent ParentRef) (claimLocked func(), release func(), err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n := r.awakeChildrenLocked(parent); n >= MaxSpawnWidth {
		return nil, nil, &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("agent.spawn: the spawn width cap is %d awake direct children, and this agent has %d; wait for one to finish (a sleeping child does not count)", MaxSpawnWidth, n)}
	}
	if r.pendingSpawns == nil {
		r.pendingSpawns = map[ParentRef]int{}
	}
	r.pendingSpawns[parent]++
	done := false
	claimLocked = func() {
		if done {
			return
		}
		done = true
		r.releasePendingSpawnLocked(parent)
	}
	release = func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		claimLocked()
	}
	return claimLocked, release, nil
}

// depthRefusal is the spawn depth cap's refusal.
func depthRefusal(tool string, depth int) error {
	return &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("%s: the spawn depth cap is %d (a lead is depth 0, each spawned agent one deeper); this conversation is at depth %d, so it spawns no subagent: do the work itself", tool, MaxSpawnDepth, depth)}
}

// AgentSpawn starts a child of caller (agent.spawn).
func (r *Runner) AgentSpawn(caller AgentCaller, req AgentSpawnRequest) (AgentSpawnResult, error) {
	const tool = "agent.spawn"
	if strings.TrimSpace(req.Prompt) == "" {
		return AgentSpawnResult{}, badRequest("%s: prompt is required", tool)
	}
	level := PermissionLevel(strings.TrimSpace(req.Authority))
	if level == "" {
		return AgentSpawnResult{}, badRequest("%s: authority is required: read-only, workdir-write, or full", tool)
	}
	if level.rank() == 0 {
		return AgentSpawnResult{}, badRequest("%s: authority must be read-only, workdir-write, or full; got %q", tool, req.Authority)
	}
	if req.SystemPrompt != "" && req.SystemPromptPath != "" {
		return AgentSpawnResult{}, badRequest("%s: give system_prompt or system_prompt_path, not both", tool)
	}
	if strings.TrimSpace(req.Cwd) != "" && strings.TrimSpace(req.Worktree) != "" {
		return AgentSpawnResult{}, badRequest("%s: give cwd or worktree, not both", tool)
	}
	depth := 0
	if h := caller.holder; h != nil {
		d, ok := r.depth(*h)
		if !ok {
			return AgentSpawnResult{}, &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("%s: this conversation's spawn chain cannot be traced to its lead, so its depth is unknown and it spawns no subagent", tool)}
		}
		depth = d
	}
	if depth+1 > MaxSpawnDepth {
		return AgentSpawnResult{}, depthRefusal(tool, depth)
	}
	harness, bare, mapping, err := r.resolveTier(caller, req.Tier)
	if err != nil {
		return AgentSpawnResult{}, err
	}
	if h := caller.holder; h != nil {
		if h.Tier == "" {
			return AgentSpawnResult{}, &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("%s: this conversation has no recorded tier, so no tier cap for its subagents can be set; it spawns none", tool)}
		}
		if tierRank(bare) > tierRank(h.Tier) {
			return AgentSpawnResult{}, &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("%s: tier %q is above this conversation's own tier %q, which caps its subagents' tier; pick %q or lower", tool, bare, h.Tier, h.Tier)}
		}
	}
	ceiling, err := r.agentCeiling(caller.Parent, true)
	if err != nil {
		return AgentSpawnResult{}, &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("%s: permission admission: %v", tool, err)}
	}
	if level.rank() > ceiling.rank() {
		return AgentSpawnResult{}, &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("%s: authority %s is above this session's own level %s, which caps its children; pick %s or lower", tool, level, ceiling, ceiling)}
	}
	system := req.SystemPrompt
	if path := strings.TrimSpace(req.SystemPromptPath); path != "" {
		if !filepath.IsAbs(path) {
			path = filepath.Join(caller.base(), path)
		}
		raw, err := readBounded(path, maxSystemPrompt)
		if err != nil {
			return AgentSpawnResult{}, badRequest("%s: system_prompt_path: %v", tool, err)
		}
		system = string(raw)
	}
	root, cwd := caller.Root, strings.TrimSpace(req.Cwd)
	if wt := strings.TrimSpace(req.Worktree); wt != "" {
		if !filepath.IsAbs(wt) {
			wt = filepath.Join(caller.base(), wt)
		}
		wt = filepath.Clean(wt)
		if _, err := os.Lstat(filepath.Join(wt, ".git")); err != nil {
			return AgentSpawnResult{}, badRequest("%s: worktree %s is not a git worktree root (no .git entry)", tool, wt)
		}
		root, cwd = wt, wt
	} else if cwd == "" {
		cwd = caller.base()
	} else if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(caller.base(), cwd)
	}
	label := req.Label
	if strings.TrimSpace(label) == "" {
		label = req.Prompt
	}
	parent := caller.Parent
	claimWidth, releaseWidth, err := r.reserveSpawnWidth(parent)
	if err != nil {
		return AgentSpawnResult{}, err
	}
	defer releaseWidth()
	st, err := r.Spawn(SpawnSpec{
		Profile:            r.opts.Backends[harness].WorkerProfile,
		Root:               root,
		Cwd:                cwd,
		Model:              mapping.Model,
		Effort:             mapping.Effort,
		Tier:               bare,
		Prompt:             req.Prompt,
		Label:              label,
		Parent:             &parent,
		PermissionLevel:    level,
		delegatedAuthority: true,
		SystemPrompt:       system,
		claimWidth:         claimWidth,
	})
	if err != nil {
		return AgentSpawnResult{}, err
	}
	// A close of a holder that ran meanwhile listed its descendants before
	// this child existed: the child closes with it here.
	if h := caller.holder; h != nil {
		if c := r.conv(h.ReplyID); c == nil || c.isClosed() {
			_, _ = r.CloseConversation(st.ID)
			return AgentSpawnResult{}, errClosed
		}
	}
	return AgentSpawnResult{Address: AgentAddress(st.ID), Status: AgentStatus(st)}, nil
}

func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, limit)
	}
	return os.ReadFile(path)
}

// ownAgent is address's conversation id when it is caller's direct child,
// else an ownership error.
func (r *Runner) ownAgent(tool string, caller AgentCaller, address string) (string, error) {
	id := agentID(address)
	if rec, ok := r.recordOf(id); ok && rec.Parent != nil && *rec.Parent == caller.Parent {
		return id, nil
	}
	return "", &requestError{status: http.StatusNotFound, msg: fmt.Sprintf("%s: %s is not an agent this session spawned (agent.list lists them)", tool, address)}
}

// AgentInterrupt interrupts caller's child's running turn and hands back
// the messages it had not taken yet; the child stays.
func (r *Runner) AgentInterrupt(caller AgentCaller, address string) (AgentInterruptResult, error) {
	id, err := r.ownAgent("agent.interrupt", caller, address)
	if err != nil {
		return AgentInterruptResult{}, err
	}
	st, returned, err := r.Interrupt(id)
	if err != nil {
		return AgentInterruptResult{}, err
	}
	if returned == nil {
		returned = []string{}
	}
	return AgentInterruptResult{Address: AgentAddress(id), Status: AgentStatus(st), Returned: returned}, nil
}

// AgentList lists caller's direct children, oldest first.
func (r *Runner) AgentList(caller AgentCaller) []AgentChild {
	states := r.Children(caller.Parent)
	out := make([]AgentChild, 0, len(states))
	for _, st := range states {
		rec, _ := r.recordOf(st.ID)
		child := AgentChild{
			Address:   AgentAddress(st.ID),
			Label:     rec.Label,
			Tier:      st.Backend,
			Authority: childLevel(wsdashboard.SessionDir(r.opts.Dir, st.ID)),
			State:     AgentStatus(st),
			Model:     st.Model,
			Effort:    st.Effort,
			Cwd:       rec.workDir(),
			Held:      st.Held,
			Error:     st.Error,
			CreatedAt: st.CreatedAt,
		}
		if rec.Tier != "" {
			child.Tier = st.Backend + "." + rec.Tier
		}
		if last := rec.LastTurn; last != nil {
			child.LastReport = reportSummary(last.Result)
		}
		out = append(out, child)
	}
	return out
}

// reportSummary is a settled turn's status and the first line of its
// final message.
func reportSummary(res TurnResult) string {
	line, _, _ := strings.Cut(strings.TrimSpace(res.Message), "\n")
	if runes := []rune(line); len(runes) > labelRunes {
		line = string(runes[:labelRunes-1]) + "…"
	}
	if line == "" && res.Error != "" {
		line = strings.Join(strings.Fields(res.Error), " ")
	}
	if line == "" {
		return res.Status
	}
	return res.Status + ": " + line
}

// Delivery is one message for the delivery seam: its text and the sender's
// reply-id, told when the wake it asks is refused ("" tells no one).
type Delivery struct {
	From string
	Text string
}

// Deliver hands d to conversation id as its next input, waking it when it
// sleeps; the width cap does not apply. When it sleeps and its admission
// is refused now, the message still queues, it stays asleep, and the
// sender gets one notice (Options.WakeRefused).
func (r *Runner) Deliver(id string, d Delivery) (State, error) {
	var refusal error
	if c := r.conv(id); c != nil {
		c.mu.Lock()
		asleep := c.proc == nil && !c.turn
		rec := c.rec
		c.mu.Unlock()
		if asleep {
			rp, resolveErr := r.resolve(rec.Profile, rec.Root)
			if resolveErr != nil {
				refusal = resolveErr
			} else {
				_, _, refusal = r.childAdmission(rec, rp.Definition)
			}
		}
	}
	st, err := r.Send(id, d.Text)
	if err != nil {
		return st, err
	}
	if refusal != nil && d.From != "" && r.opts.WakeRefused != nil {
		r.opts.WakeRefused(d.From, id, refusal.Error())
	}
	return st, nil
}

// mailRelation is how sender, a reply-id, relates to rec through the
// Runner spawn lineage (Record.Parent): "parent" for rec's parent, "child"
// for a conversation rec spawned, else "peer"; "" for no sender (system
// mail). It is computed at each delivery, never stored.
func (r *Runner) mailRelation(rec Record, sender string) string {
	switch {
	case sender == "":
		return ""
	case rec.Parent != nil && rec.Parent.ReplyID == sender:
		return "parent"
	}
	if s, ok := r.recordOf(sender); ok && s.Parent != nil && s.Parent.ReplyID == rec.ReplyID {
		return "child"
	}
	return "peer"
}

// childWaitingExpiry is how long a child may wait before it is deleted:
// agent.child_waiting_days, read once per runner.
func (r *Runner) childWaitingExpiry() time.Duration {
	r.expiryOnce.Do(func() {
		days := DefaultChildWaitingDays
		if r.opts.ChildWaitingDays > 0 {
			days = r.opts.ChildWaitingDays
		} else {
			resolver := wsconfig.NewResolver(r.opts.Config, map[string]string{wsconfig.ItemAgentChildWaitingDays: strconv.Itoa(DefaultChildWaitingDays)}, nil, nil)
			if rv, err := resolver.Get("", wsconfig.ItemAgentChildWaitingDays); err == nil {
				if n, err := strconv.Atoi(strings.TrimSpace(rv.Value)); err == nil && n > 0 {
					days = n
				}
			}
		}
		r.expiry = time.Duration(days) * 24 * time.Hour
	})
	return r.expiry
}

// childWaitingLocked reports a spawned child asleep with no turn and no
// input or claimed mail about to start one (a refused wake's does not
// count). Under mu.
func (c *conv) childWaitingLocked() bool {
	return !c.closed && !c.rec.Lead && c.rec.Parent != nil && c.proc == nil && !c.turn && !c.spawnPending &&
		(c.wakeRefused || c.sends <= max(c.settled.mark, c.voided) && len(c.rec.Mail) == 0)
}

// lastActive is a spawned record's expiry clock start.
func lastActive(rec Record) time.Time {
	for _, v := range []string{rec.LastActiveAt, rec.CreatedAt} {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return time.Time{}
}

// expireIfDue has the runner delete a child waiting past its expiry
// (expireLater). It runs in the loop, on the heartbeat: no file is read.
func (c *conv) expireIfDue() {
	c.mu.Lock()
	due := !c.expiring && c.childWaitingLocked()
	rec := c.rec
	c.mu.Unlock()
	if !due {
		return
	}
	at := lastActive(rec)
	if at.IsZero() || c.r.opts.Now().Sub(at) < c.r.childWaitingExpiry() {
		return
	}
	c.expiring = true
	c.r.expireLater(rec.ReplyID)
}

// expireLater deletes waiting child id and its descendants, deepest first,
// off the caller's goroutine, unless a descendant is awake or id woke
// meanwhile.
func (r *Runner) expireLater(id string) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.expire(id)
	}()
}

// expire is expireLater's body; it reports whether id was deleted.
func (r *Runner) expire(id string) bool {
	c := r.conv(id)
	if c == nil {
		return false
	}
	c.mu.Lock()
	waiting := c.childWaitingLocked()
	c.mu.Unlock()
	desc := r.descendants(id)
	busy := !waiting
	for _, d := range desc {
		if dc := r.conv(d); dc != nil {
			dc.mu.Lock()
			busy = busy || dc.awakeLocked()
			dc.mu.Unlock()
		}
	}
	if busy {
		c.do(func() { c.expiring = false })
		return false
	}
	if _, err := r.CloseConversation(id); err != nil {
		if errors.Is(err, errClosed) {
			// Someone closed it meanwhile (a dashboard close): what they
			// closed is theirs to delete, not the expiry's.
			return false
		}
		fmt.Fprintf(r.opts.Log, "agent %s: expire: %v\n", id, err)
		// A later heartbeat tries again.
		c.do(func() { c.expiring = false })
		return false
	}
	for _, d := range append(desc, id) {
		r.forget(d)
	}
	return true
}

var _ wsdashboard.AgentForgetter = (*Runner)(nil)

// ForgetClosed drops closed or handed-off conversation id from the runner's
// in-memory state without touching disk or notifying the daemon, for a caller
// that is purging the session directory itself.
func (r *Runner) ForgetClosed(id string) {
	r.forgetClosed(id)
}

func (r *Runner) forgetClosed(id string) (Record, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.closed[id]
	delete(r.closed, id)
	delete(r.closedRows, id)
	if ok && rec.Parent != nil && r.childrenIndex != nil {
		delete(r.childrenIndex[*rec.Parent], id)
		if len(r.childrenIndex[*rec.Parent]) == 0 {
			delete(r.childrenIndex, *rec.Parent)
		}
	}
	return rec, ok
}

// forget drops closed conversation id from the runner and deletes its
// session directory.
func (r *Runner) forget(id string) {
	rec, ok := r.forgetClosed(id)
	if !ok {
		return
	}
	sessionDir := wsdashboard.SessionDir(r.opts.Dir, id)
	wslive.DropRing(wslive.RingDir(sessionDir))
	if err := os.RemoveAll(sessionDir); err != nil {
		fmt.Fprintf(r.opts.Log, "agent %s: delete: %v\n", id, err)
	}
	r.notify(id, rec.Root)
}

// expiredAtStart picks the open spawned children a daemon start deletes
// instead of serving, deepest first: each quiet (no turn open or held, no
// input) past its expiry whose descendants are all quiet, with those
// descendants. A child whose input waits on a refused wake is not quiet
// here; its loop's heartbeat deletes it once its start is refused again.
func (r *Runner) expiredAtStart(open []Record) []string {
	now := r.opts.Now()
	expiry := r.childWaitingExpiry()
	byID := map[string]Record{}
	kids := map[string][]string{}
	for _, rec := range open {
		byID[rec.ReplyID] = rec
		if rec.Parent != nil {
			kids[rec.Parent.ReplyID] = append(kids[rec.Parent.ReplyID], rec.ReplyID)
		}
	}
	quiet := func(rec Record) bool {
		mark := rec.Voided
		if rec.LastTurn != nil {
			mark = max(mark, rec.LastTurn.Mark)
		}
		return !rec.Lead && rec.Parent != nil && rec.HolderID == "" && !rec.TurnOpen && rec.HeldTurn == nil &&
			rec.Sends <= mark && len(rec.Queued)+len(rec.Mail)+len(rec.Reports)+len(rec.Steered)+len(rec.PendingReports) == 0
	}
	seen := map[string]bool{}
	var subtree func(id string, out *[]string) bool
	subtree = func(id string, out *[]string) bool {
		if seen[id] {
			return false
		}
		seen[id] = true
		ok := true
		for _, k := range kids[id] {
			ok = subtree(k, out) && quiet(byID[k]) && ok
		}
		*out = append(*out, id)
		return ok
	}
	var out []string
	var visit func(id string)
	visit = func(id string) {
		rec := byID[id]
		if at := lastActive(rec); quiet(rec) && !at.IsZero() && now.Sub(at) >= expiry {
			var tree []string
			saved := maps.Clone(seen)
			if subtree(id, &tree) {
				out = append(out, tree...)
				return
			}
			seen = saved
		}
		seen[id] = true
		for _, k := range kids[id] {
			if !seen[k] {
				visit(k)
			}
		}
	}
	for _, rec := range open {
		if rec.Parent == nil || byID[rec.Parent.ReplyID].ReplyID == "" {
			visit(rec.ReplyID)
		}
	}
	return out
}

// agentTokenRequest is the token route's body for the agent.* ops.
type agentTokenRequest struct {
	AgentSpawnRequest
	Address string `json:"address"`
}

// serveAgentToken serves a token holder's agent.* op (serveToken).
func (r *Runner) serveAgentToken(w http.ResponseWriter, req *http.Request, caller *conv, op string) {
	var body agentTokenRequest
	if !decode(w, req, &body) {
		return
	}
	ac := r.holderCaller(caller.record())
	switch op {
	case "agent.spawn":
		res, err := r.AgentSpawn(ac, body.AgentSpawnRequest)
		writeAnswer(w, http.StatusCreated, res, err)
	case "agent.interrupt":
		res, err := r.AgentInterrupt(ac, body.Address)
		writeAnswer(w, http.StatusOK, res, err)
	case "agent.list":
		writeAnswer(w, http.StatusOK, map[string]any{"agents": r.AgentList(ac)}, nil)
	case "agent.tiers":
		res, err := r.AgentTiers(ac)
		writeAnswer(w, http.StatusOK, res, err)
	default:
		http.Error(w, "unknown agent route", http.StatusNotFound)
	}
}
