package wsagent

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// BuiltinInstructionsPrefix marks a profile's instructions value that names
// an instruction text embedded in this binary ("builtin:<name>") instead of
// a file under the working root. A builtin profile cannot point at a file:
// its working root (a tracker store's clone, a bare ledger) holds none.
const BuiltinInstructionsPrefix = "builtin:"

// Tracker support profiles: one builtin profile per harness, since a
// profile fixes its backend. Each is an ordinary agent.profile.<name> key,
// so a project or global definition of the same name replaces it whole.
const (
	SupportProfileClaude = "tracker-support-claude"
	SupportProfilePi     = "tracker-support-pi"
	// supportInstructions is the builtin instruction text both share.
	supportInstructions = "tracker-support"
)

// SupportWSTools are the ws tools the support profiles allow: the tracker
// tools (tracker.show and tracker.ui draw in and drive the store's chat),
// the store's wiki tools, the read-only git search, and playbook.read for
// the tracker playbook's grammar.
var SupportWSTools = []string{"tracker.query", "tracker.update", "tracker.show", "tracker.ui", "wiki.query", "wiki.update", "git.search", "playbook.read"}

// SupportClaudeTools are the built-in tools the Claude support profile
// allows: Agent alone, so the agent may spawn subagents. Claude applies the
// launch's --tools and dontAsk allowlist to those subagents too, so they
// reach nothing the parent cannot (a subagent type whose every tool is
// outside the list is refused). Pi has no subagent tool and gets none.
var SupportClaudeTools = []string{"Agent"}

// Search answer profiles use only harness-native read tools. The answer
// service caps overrides and scopes Claude's rules at launch.
const (
	SearchAnswerProfileClaude = "search-answer-claude"
	SearchAnswerProfilePi     = "search-answer-pi"
)

func searchAnswerTools(backend string) []string {
	if backend == BackendClaude {
		return []string{"Read", "Grep", "Glob"}
	}
	return []string{"read", "grep", "find", "ls"}
}

// Lead profiles: one builtin profile per harness for the hosted lead
// sessions the dashboard spawns (Profile.Lead). A project or global
// definition of the same name replaces one whole, as for any profile.
const (
	LeadProfileClaude = "lead-claude"
	LeadProfilePi     = "lead-pi"
	LeadProfileAgy    = "lead-agy"
	LeadProfileCodex  = "lead-codex"
)

// LeadProfileFor is the builtin lead profile of a harness, "" for a harness
// that has none.
func LeadProfileFor(backend string, registries ...map[string]BackendRegistration) string {
	if len(registries) > 0 {
		return registries[0][backend].LeadProfile
	}
	profiles := BuiltinProfiles()
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		var p Profile
		if json.Unmarshal(profiles[name], &p) == nil && p.Backend == backend && p.Lead {
			return strings.TrimPrefix(name, ProfilePrefix)
		}
	}
	return ""
}

// PiWorkerProfile is the builtin profile of a Pi subagent a session spawns
// (Runner.Spawn): Pi's built-in coding tools and the few ws tools a
// delegated implementation or review prompt names, with no instruction
// file, since the spawner's rendered prompt is the first message.
const PiWorkerProfile = "pi-worker"

// PiMiniLeadProfile adds bounded ticket workflow tools to the coding worker.
const PiMiniLeadProfile = "pi-mini-lead"

// ExploreProfile is a local, read-only Pi researcher; the caller selects its tier.
const ExploreProfile = "explore"

// PiWorkerTools are Pi's built-in coding tools the pi-worker profile
// allows.
var PiWorkerTools = []string{"read", "bash", "edit", "write", "grep", "find", "ls"}

// PiWorkerWSTools are the ws tools the pi-worker profile allows: the commit
// an implementation prompt ends with, the git reads a commit and a diff
// review need, implementation-branch resolution, ticket creation and preflight
// verification, and the pi.* tools a spawning playbook needs. A worker's
// pi.* calls go to the daemon on its capability token (children.go), which
// caps the spawn tree's depth and tier. Ticket lifecycle operations remain
// on pi-mini-lead; worktree leasing has its own authorization boundary.
var PiWorkerWSTools = []string{"git.status", "git.diff", "git.log", "git.merge_base", "git.commit", "git.resolve_impl_branch", "tickets.create_empty", "tickets.verify", "pi.spawn", "pi.send", "pi.list", "pi.stop", "pi.transcript", "pi.wait", "compact", "agenda.set"}

// PiMiniLeadWSTools adds ticket workflow reads and lifecycle operations, but
// no lead-only tools or note, whiteboard, or tracker writes.
var PiMiniLeadWSTools = append(append([]string{}, PiWorkerWSTools...),
	"tickets.query", "tickets.acquire", "tickets.release", "tickets.close",
	"tickets.move", "tickets.checklist", "note.query",
	"playbook.read", "playbook.render", "path.generate", "route.resolve_implement",
)

// ChildSafeWSTools is the single harness-neutral allowlist of harmless ws MCP
// tools pre-approved for spawned children: read-only queries, or writes scoped
// to the ws cache, generated paths, or the child's own mailbox and session
// todo list, already authority-enforced server-side. Mutating repository,
// ticket, note, worktree, whiteboard, config mutation, and orchestration tools
// stay excluded; workdir-write children keep their existing behavior for those.
var ChildSafeWSTools = []string{
	"path.generate",
	"path.write",
	"tickets.query",
	"tickets.template",
	"tickets.checklist",
	"tickets.verify",
	"config.resolve_agent",
	"infra.read",
	"convention.read",
	"playbook.read",
	"note.query",
	"rationale.query",
	"git.status",
	"git.diff",
	"git.log",
	"git.merge_base",
	"git.resolve_impl_branch",
	"api.list",
	"project_tree",
	"runtime.read",
	"review.marker",
	"mailbox.send",
	"mailbox.recv",
	"todo.list",
	"todo.read",
	"todo.add",
	"todo.check",
	"todo.erase",
	"todo.clear",
	"todo.reorder",
}

// childSafeWSToolsFor returns the ChildSafeWSTools subset admitted by p: when
// p.WSTools is non-empty, only safe tools present in p.WSTools are returned;
// otherwise ChildSafeWSTools itself.
func childSafeWSToolsFor(p Profile) []string {
	if len(p.WSTools) == 0 {
		return slices.Clone(ChildSafeWSTools)
	}
	out := make([]string, 0, len(ChildSafeWSTools))
	for _, name := range ChildSafeWSTools {
		if slices.Contains(p.WSTools, name) {
			out = append(out, name)
		}
	}
	return out
}

// AgentWSTools are the common ws tools every agent.spawn child gets, on
// every harness: PiMiniLeadWSTools with agent.* in place of pi.*, the review
// ledger tools, plus ChildSafeWSTools. The child's authority does not select
// among them; a read-only child's ws-mcp refuses the mutating ones
// (EnvPermissionLevel).
var AgentWSTools = agentWSTools()

func agentWSTools() []string {
	out := []string{}
	for _, name := range PiMiniLeadWSTools {
		if !strings.HasPrefix(name, "pi.") {
			out = append(out, name)
		}
	}
	for _, name := range append([]string{"agent.spawn", "agent.interrupt", "agent.list", "agent.tiers", "review.stamp"}, ChildSafeWSTools...) {
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// AgentClaudeTools are a Claude agent.spawn child's native tools.
var AgentClaudeTools = []string{"Bash", "Read", "Edit", "Write", "Glob", "Grep"}

//go:embed instructions/tracker-support.md
var trackerSupportText string

//go:embed instructions/explore.md
var exploreText string

//go:embed instructions/agent-worker-agy.md
var agyWorkerText string

//go:embed instructions/agent-worker-codex.md
var codexWorkerText string

// builtinInstructionTexts are the embedded instruction texts by name.
var builtinInstructionTexts = map[string]string{
	supportInstructions:  trackerSupportText,
	ExploreProfile:       exploreText,
	"agent-worker-agy":   agyWorkerText,
	"agent-worker-codex": codexWorkerText,
}

// BuiltinInstructionNames are the names a builtin: instructions value may
// take, sorted.
func BuiltinInstructionNames() []string {
	names := make([]string, 0, len(builtinInstructionTexts))
	for name := range builtinInstructionTexts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// builtinInstructionName returns the name a builtin: instructions value
// carries, and whether the value is one.
func builtinInstructionName(value string) (string, bool) {
	name, ok := strings.CutPrefix(strings.TrimSpace(value), BuiltinInstructionsPrefix)
	return name, ok
}

// BuiltinProfiles is the builtin floor of the agent.profile.<name> family,
// keyed by full config key. A higher scope's profile of the same name
// replaces one whole.
func BuiltinProfiles() map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for name, backend := range map[string]string{SupportProfileClaude: BackendClaude, SupportProfilePi: BackendPi} {
		var tools []string
		if backend == BackendClaude {
			tools = SupportClaudeTools
		}
		raw, err := json.Marshal(Profile{
			Description:  fmt.Sprintf("Tracker store support agent (%s): talks with the user about one tracker store from the store's chat card; reads the user's stores and local git repositories, and writes to its own store only when asked.", backend),
			Backend:      backend,
			Tools:        tools,
			WSTools:      SupportWSTools,
			Instructions: BuiltinInstructionsPrefix + supportInstructions,
		})
		if err != nil {
			panic(err)
		}
		out[ProfilePrefix+name] = raw
	}
	raw, err := json.Marshal(Profile{
		Description: "Pi subagent (pi): a headless Pi worker a session spawns with a prompt and steers by its id; it reads, edits, and runs commands in its working directory, commits through ws, and may spawn Pi subagents of its own within the daemon's depth and tier caps.",
		Backend:     BackendPi,
		Tools:       PiWorkerTools,
		WSTools:     PiWorkerWSTools,
	})
	if err != nil {
		panic(err)
	}
	out[ProfilePrefix+PiWorkerProfile] = raw
	raw, err = json.Marshal(Profile{
		Description: "Pi mini-lead: a non-lead coding worker with bounded ticket workflow tools; may spawn only profiles within its launched tool set.",
		Backend:     BackendPi,
		Tools:       PiWorkerTools,
		WSTools:     PiMiniLeadWSTools,
	})
	if err != nil {
		panic(err)
	}
	out[ProfilePrefix+PiMiniLeadProfile] = raw
	for backend, tools := range map[string][]string{BackendClaude: AgentClaudeTools, BackendPi: PiWorkerTools, BackendAgy: nil, BackendCodex: nil} {
		p := Profile{
			Description: fmt.Sprintf("Agent worker (%s): the internal profile of a child agent.spawn starts on this harness; its tier picks the model, its system prompt the instructions, and its authority the permission level.", backend),
			Backend:     backend,
			Tools:       tools,
			WSTools:     AgentWSTools,
		}
		if _, ok := builtinInstructionTexts[AgentWorkerProfile(backend)]; ok {
			p.Instructions = BuiltinInstructionsPrefix + AgentWorkerProfile(backend)
		}
		raw, err := json.Marshal(p)
		if err != nil {
			panic(err)
		}
		out[ProfilePrefix+AgentWorkerProfile(backend)] = raw
	}
	raw, err = json.Marshal(Profile{
		Description:  "Local read-only Pi researcher reporting evidence from project files.",
		Backend:      BackendPi,
		Tools:        searchAnswerTools(BackendPi),
		Instructions: BuiltinInstructionsPrefix + ExploreProfile,
	})
	if err != nil {
		panic(err)
	}
	out[ProfilePrefix+ExploreProfile] = raw
	for name, backend := range map[string]string{SearchAnswerProfileClaude: BackendClaude, SearchAnswerProfilePi: BackendPi} {
		raw, err := json.Marshal(Profile{
			Description: fmt.Sprintf("Search answer agent (%s): reads the session board snapshot and project files; no shell, write, or network tools. Project-root confinement is best effort, not a sandbox.", backend),
			Backend:     backend,
			Tools:       searchAnswerTools(backend),
		})
		if err != nil {
			panic(err)
		}
		out[ProfilePrefix+name] = raw
	}
	for name, p := range map[string]Profile{
		LeadProfileClaude: {Backend: BackendClaude, PermissionMode: DefaultLeadPermissionMode},
		LeadProfilePi:     {Backend: BackendPi},
		LeadProfileAgy:    {Backend: BackendAgy},
		LeadProfileCodex:  {Backend: BackendCodex},
	} {
		p.Lead = true
		p.Description = fmt.Sprintf("Hosted lead session (%s): a headless session the dashboard spawns on a worktree, with the user's own harness setup.", p.Backend)
		raw, err := json.Marshal(p)
		if err != nil {
			panic(err)
		}
		out[ProfilePrefix+name] = raw
	}
	return out
}
