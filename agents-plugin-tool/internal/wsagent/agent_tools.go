package wsagent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

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
