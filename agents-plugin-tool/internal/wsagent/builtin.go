package wsagent

import (
	_ "embed"
	"encoding/json"
	"fmt"
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

// Lead profiles: one builtin profile per harness for the hosted lead
// sessions the dashboard spawns (Profile.Lead). A project or global
// definition of the same name replaces one whole, as for any profile.
const (
	LeadProfileClaude = "lead-claude"
	LeadProfilePi     = "lead-pi"
)

// LeadProfileFor is the builtin lead profile of a harness, "" for a harness
// that has none.
func LeadProfileFor(backend string) string {
	switch backend {
	case BackendClaude:
		return LeadProfileClaude
	case BackendPi:
		return LeadProfilePi
	}
	return ""
}

// PiWorkerProfile is the builtin profile of a Pi subagent a session spawns
// (Runner.Spawn): Pi's built-in coding tools and the few ws tools a
// delegated implementation or review prompt names, with no instruction
// file, since the spawner's rendered prompt is the first message.
const PiWorkerProfile = "pi-worker"

// PiWorkerTools are Pi's built-in coding tools the pi-worker profile
// allows.
var PiWorkerTools = []string{"read", "bash", "edit", "write", "grep", "find", "ls"}

// PiWorkerWSTools are the ws tools the pi-worker profile allows: the commit
// an implementation prompt ends with, and the git reads a commit and a
// diff review need. No pi.* tool: a worker spawns no worker of its own.
var PiWorkerWSTools = []string{"git.status", "git.diff", "git.log", "git.merge_base", "git.commit"}

//go:embed instructions/tracker-support.md
var trackerSupportText string

// builtinInstructionTexts are the embedded instruction texts by name.
var builtinInstructionTexts = map[string]string{
	supportInstructions: trackerSupportText,
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
		Description: "Pi subagent (pi): a headless Pi worker a session spawns with a prompt and steers by its id; it reads, edits, and runs commands in its working directory, and commits through ws.",
		Backend:     BackendPi,
		Tools:       PiWorkerTools,
		WSTools:     PiWorkerWSTools,
	})
	if err != nil {
		panic(err)
	}
	out[ProfilePrefix+PiWorkerProfile] = raw
	for name, p := range map[string]Profile{
		LeadProfileClaude: {Backend: BackendClaude, PermissionMode: DefaultLeadPermissionMode},
		LeadProfilePi:     {Backend: BackendPi},
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
