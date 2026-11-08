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
// tools, the read-only git search, and playbook.read for the tracker
// playbook's grammar.
var SupportWSTools = []string{"tracker.query", "tracker.update", "tracker.show", "git.search", "playbook.read"}

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
		raw, err := json.Marshal(Profile{
			Description:  fmt.Sprintf("Tracker store support agent (%s): talks with the user about one tracker store from the store's chat card; reads the user's stores and local git repositories, and writes to its own store only when asked.", backend),
			Backend:      backend,
			WSTools:      SupportWSTools,
			Instructions: BuiltinInstructionsPrefix + supportInstructions,
		})
		if err != nil {
			panic(err)
		}
		out[ProfilePrefix+name] = raw
	}
	return out
}
