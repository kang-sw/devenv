package mcp

import (
	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
)

// agentProfileKnobEntry is the config registry template for one
// agent.profile.<name> key, generated per key like worktreeProfileKnobEntry.
// Its value is a structured object (wsagent.Profile), so config.tune routes
// it to the resolver's structured writer; session scope is refused. No
// profile is built in.
func agentProfileKnobEntry(name string) configKeyEntry {
	return configKeyEntry{
		Key:        wsagent.ProfilePrefix + name,
		WriterTool: "config.tune",
		ResetTool:  "config.tune",
		SelectorFields: []tuningField{{
			Name:        "scope",
			Description: "Storage scope: project (machine-local, the default), repo (the committed .ws-workflow/config.json; include it in your next git.commit), or global. Never session.",
			Enum:        worktreeProfileScopeEnum,
		}},
		ValueFields: []tuningField{{
			Name:        "value",
			Description: "The whole profile object (see value_schema); it replaces any lower-scope definition of the same name. Omit when reset is true.",
			Required:    true,
		}},
		NoAgentVisible: true,
		ResolverBacked: true,
		Structured:     true,
	}
}

// agentProfileTuningKnob is the config.list knob for the agent.profile.<name>
// family. A profile that fails to resolve surfaces as the knob's error rather
// than failing the whole listing.
func agentProfileTuningKnob(resolver *wsconfig.Resolver) (configKeyEntry, tuningKnob) {
	entry := agentProfileKnobEntry("<name>")
	knob := tuningKnob{
		ID:          entry.Key,
		Kind:        "agent_profile",
		Description: "One agent profile per key: a named headless agent conversation the dashboard daemon's agent runner starts from the working root it is asked to run in. The value is a whole profile object (value_schema); a higher scope replaces a lower scope's profile of the same name, with no field merge. Nothing is built in and every capability defaults off: backend (claude or pi) is the only required field, and the built-in tools, ws tools, MCP servers, plugins, and skills the agent may use are each an explicit allowlist. repo scope is the committed .ws-workflow/config.json of that working root, so a repository can carry the profiles its own conversations need. current lists every resolved profile with the scope and file it came from.",
		Writer:      tuningWriter{Tool: entry.WriterTool, FixedArguments: map[string]string{"key": entry.Key}},
		Reset: &tuningWriter{
			Tool:           entry.ResetTool,
			FixedArguments: map[string]string{"key": entry.Key, "reset": "true"},
		},
		SelectorFields: entry.SelectorFields,
		ValueFields:    entry.ValueFields,
		ValueSchema:    wsagent.ProfileValueSchema(),
		RepoMember:     "structured_overrides",
	}
	if profiles, err := wsagent.ResolveProfiles(resolver); err != nil {
		knob.Error = err.Error()
	} else {
		knob.Current = profiles
	}
	return entry, knob
}
