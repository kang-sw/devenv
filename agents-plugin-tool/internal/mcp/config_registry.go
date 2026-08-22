package mcp

import (
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// Shared enum value slices used by both the config.* tools() schema literals
// (server.go tools()) and the configRegistry entries below, so schema
// construction and registry-driven validation/catalog projection always read
// the identical list. This is Phase 1 of 260814 (config-registry-extract): a
// pure internal refactor — nothing about tools/list schema shape or order
// changes, only where these enum values live.
var (
	onOffEnum           = []string{"on", "off"}
	preferMercenaryEnum = []string{"on", "off", "hide"}
	agentsTierEnum      = []string{"small", "medium", "large", "xlarge"}
	agentsEffortEnum    = []string{"", "none", "low", "medium", "high", "xhigh"}
	promptHarnessEnum   = []string{"claude", "codex", "*"}
)

// configKeyEntry is the per-key config registry row: the single source of
// truth for a config.* knob's value schema, harness/no-agent applicability,
// and lead-authority requirement. It replaces what were previously four
// independently-scattered sources of the same information — the inline
// dispatch validators, the tuning-catalog's tools()-schema scrape
// (tuningFieldFromSchema), and the lead-only/session-key-required/
// no-agent-hidden gating tables' hardcoded config.* name lists — with one
// lookup surface.
//
// AllowedScopes/DefaultScope are deliberately NOT struct fields: they
// delegate to the existing wsconfig scope registry (wsconfig.GlobalOnly /
// wsconfig.DefaultScope) via the GlobalOnly/DefaultScope methods below, so
// this registry never duplicates wsconfig's scope declarations.
type configKeyEntry struct {
	// Key is the wsconfig item key this entry describes (or, for agents.tier
	// and prompt.<id>, the catalog knob id — neither is a registered
	// wsconfig item; see ResolverBacked).
	Key string
	// WriterTool is the MCP tool name that writes this key.
	WriterTool string
	// ResetTool is the MCP tool name that resets this key back to its
	// builtin/inherited default, or "" when no dedicated reset exists.
	ResetTool string
	// SelectorFields are non-value arguments (e.g. harness, scope, tier)
	// that narrow which slot a write/reset targets.
	SelectorFields []tuningField
	// ValueFields are the value-carrying arguments, validated against their
	// declared Enum (when non-empty) before a write is accepted.
	ValueFields []tuningField
	// HarnessApplicable marks knobs whose value varies per harness.
	HarnessApplicable bool
	// NoAgentVisible reports whether this knob stays visible in the
	// agentless wsflow (no-agent) tuning catalog and tool surface.
	NoAgentVisible bool
	// RequiresLeadAuthority marks knobs that require a lead-scoped session
	// key beyond the config.* prefix gate (today: exactly the global-only
	// workflow-preference writers — see the 260814 Phase 1 plan's
	// GlobalOnly-derivation finding).
	RequiresLeadAuthority bool
	// ResolverBacked reports whether this key is read/written through
	// wsconfig.Resolver.Get/Set/Unset. False only for agents.tier, which
	// writes structured AgentTier values via
	// wsconfig.SetAgentsTierForHarness directly rather than the resolver's
	// flat map[string]string overlay (260814 Phase 1 plan's
	// resolver-bypass finding: folding it in would be a resolver capability
	// extension, not a pure internal refactor).
	ResolverBacked bool
}

// GlobalOnly reports whether this key's writes are constrained to
// global/builtin scope. Delegates to wsconfig.GlobalOnly for resolver-backed
// keys; agents.tier is not resolver-backed and has no session/global write
// path today, so it is fixed to project scope rather than global-only.
func (e configKeyEntry) GlobalOnly() bool {
	if !e.ResolverBacked {
		return false
	}
	return wsconfig.GlobalOnly(e.Key)
}

// DefaultScope reports this key's declared default write scope. Delegates to
// wsconfig.DefaultScope for resolver-backed keys; agents.tier is fixed to
// wsconfig.ScopeProject (its only write path today) rather than inventing an
// allowed-scopes declaration that doesn't exist yet.
func (e configKeyEntry) DefaultScope() wsconfig.Scope {
	if !e.ResolverBacked {
		return wsconfig.ScopeProject
	}
	return wsconfig.DefaultScope(e.Key)
}

// configRegistry holds the 5 static per-key entries. The dynamic prompt.*
// family is not represented here — it is generated per discovered override
// point by promptKnobEntry at catalog-build time (see buildTuningCatalog).
var configRegistry = []configKeyEntry{
	{
		Key:        wsconfig.ItemWorkflowPreferSubagent,
		WriterTool: "config.workflow_prefer_subagent",
		ResetTool:  "config.workflow_prefer_subagent",
		ValueFields: []tuningField{{
			Name:        "value",
			Description: "Desired mode: on or off. Omit when reset is true.",
			Enum:        onOffEnum,
		}},
		NoAgentVisible:        true,
		RequiresLeadAuthority: wsconfig.GlobalOnly(wsconfig.ItemWorkflowPreferSubagent),
		ResolverBacked:        true,
	},
	{
		Key:        wsconfig.ItemWorkflowPreferMercenary,
		WriterTool: "config.workflow_prefer_mercenary",
		ValueFields: []tuningField{{
			Name:        "value",
			Description: "Desired mode: on, off, or hide.",
			Enum:        preferMercenaryEnum,
			Required:    true,
		}},
		NoAgentVisible:        false,
		RequiresLeadAuthority: wsconfig.GlobalOnly(wsconfig.ItemWorkflowPreferMercenary),
		ResolverBacked:        true,
	},
	{
		Key:        wsconfig.ItemBootstrapAlarm,
		WriterTool: "config.bootstrap_alarm",
		ResetTool:  "config.bootstrap_alarm",
		ValueFields: []tuningField{{
			Name:        "value",
			Description: "Desired mode: on or off. Omit when reset is true.",
			Enum:        onOffEnum,
		}},
		NoAgentVisible:        true,
		RequiresLeadAuthority: wsconfig.GlobalOnly(wsconfig.ItemBootstrapAlarm),
		ResolverBacked:        true,
	},
	{
		Key:        wsconfig.ItemDocCoverageAlarm,
		WriterTool: "config.doc_coverage_alarm",
		ResetTool:  "config.doc_coverage_alarm",
		ValueFields: []tuningField{{
			Name:        "value",
			Description: "Desired mode: on or off. Omit when reset is true.",
			Enum:        onOffEnum,
		}},
		NoAgentVisible:        true,
		RequiresLeadAuthority: wsconfig.GlobalOnly(wsconfig.ItemDocCoverageAlarm),
		ResolverBacked:        true,
	},
	{
		// agents.tier has no wsconfig item key — it bypasses the resolver
		// entirely (see ResolverBacked doc above), so Key here is the
		// catalog knob id rather than a wsconfig.Item* constant.
		Key:        "agents.tier",
		WriterTool: "config.agents_tier",
		SelectorFields: []tuningField{
			{
				Name:        "tier",
				Description: "Capability tier to configure.",
				Enum:        agentsTierEnum,
				Required:    true,
			},
			{
				Name:        "harness",
				Description: "Optional harness alias key to configure. When omitted, ws uses the detected MCP session harness, or default when none is known.",
			},
		},
		ValueFields: []tuningField{
			{
				Name:        "backend",
				Description: "Optional backend name. When omitted, ws infers it from the model when possible.",
			},
			{
				Name:        "model",
				Description: "Concrete model for this alias.",
			},
			{
				Name:        "effort",
				Description: "Optional portable reasoning effort for this alias. Empty, omitted, or none leaves backend effort unset.",
				Enum:        agentsEffortEnum,
			},
		},
		HarnessApplicable:     true,
		NoAgentVisible:        true,
		RequiresLeadAuthority: false,
		ResolverBacked:        false,
	},
}

// promptKnobEntry returns the configKeyEntry template applied per discovered
// prompt override-point (see buildPromptOverrideListing/scanOverridePoints in
// server.go). This is a factory, not a configRegistry row, because prompt.*
// keys are discovered at catalog-build time from the shipped playbook tree
// rather than statically known ahead of time. Passing an empty pointID (used
// by configKeyEntryForTool) is valid: only Key varies with pointID, and
// callers that need Key look it up per discovered point directly.
func promptKnobEntry(pointID string) configKeyEntry {
	return configKeyEntry{
		Key:        "prompt." + pointID,
		WriterTool: "config.prompt.set",
		ResetTool:  "config.prompt.unset",
		SelectorFields: []tuningField{
			{
				Name:        "harness",
				Description: "Harness bucket the override applies to. When omitted, defaults to the current session's detected harness. Use * explicitly for cross-harness (all).",
				Enum:        promptHarnessEnum,
			},
			{
				Name:        "scope",
				Description: "Storage scope. When omitted the write lands in the item's declared default scope (project for unregistered prompt.* keys).",
				Enum:        wsconfig.ScopeSchemaEnum(),
			},
		},
		ValueFields: []tuningField{{
			Name:        "prompt",
			Description: "Override text that replaces the seed block at render time. Must be non-empty.",
			Required:    true,
		}},
		HarnessApplicable:     true,
		NoAgentVisible:        true,
		RequiresLeadAuthority: false,
		ResolverBacked:        true,
	}
}

// registryEntryByKey looks up a static configRegistry entry by its Key.
// Returns the zero configKeyEntry when not found (all callers pass a known
// wsconfig.Item*/"agents.tier" constant, so this should never miss).
func registryEntryByKey(key string) configKeyEntry {
	for _, entry := range configRegistry {
		if entry.Key == key {
			return entry
		}
	}
	return configKeyEntry{}
}

// configKeyEntryForTool resolves the registry entry for a config.* writer or
// reset tool name. Shared by the lead-only, session-key-required, and
// no-agent-hidden gating tables (workflowPreferenceWriterTool,
// toolSchemaRequiresSessionKey, noAgentHiddenTool) so those tables no longer
// hardcode independent copies of the same config.* tool-name list. The
// prompt.* family resolves via the shared template (pointID is irrelevant to
// the gating attributes these tables read, so an empty pointID is fine).
func configKeyEntryForTool(toolName string) (configKeyEntry, bool) {
	for _, entry := range configRegistry {
		if entry.WriterTool == toolName || (entry.ResetTool != "" && entry.ResetTool == toolName) {
			return entry, true
		}
	}
	switch toolName {
	case "config.prompt.set", "config.prompt.unset":
		return promptKnobEntry(""), true
	}
	return configKeyEntry{}, false
}

// requireConfigKeyEntry resolves the registry entry for a config.* writer or
// reset tool name, failing closed on a miss instead of the zero-value entry
// configKeyEntryForTool's ok==false would otherwise let a caller silently
// proceed with. A zero entry has nil ValueFields/SelectorFields, and
// enumContains(nil, ...) accepts any value — so a caller that discarded the
// ok result would validate vacuously (skip validation entirely) rather than
// reject the write. Every dispatch validator today passes a compile-time-
// known tool name that is always present in the registry, so the miss branch
// is unreachable now; it exists so a future caller that resolves an
// arbitrary runtime key string through this same path (e.g. Phase 2's
// config.tune) fails closed instead of silently skipping validation.
func requireConfigKeyEntry(toolName string) (configKeyEntry, error) {
	entry, ok := configKeyEntryForTool(toolName)
	if !ok {
		return configKeyEntry{}, fmt.Errorf("%s: internal error: no config registry entry for this tool", toolName)
	}
	return entry, nil
}

// fieldEnum returns the declared Enum for the named field within fields, or
// nil when the field is absent or declares no enum.
func fieldEnum(fields []tuningField, name string) []string {
	for _, f := range fields {
		if f.Name == name {
			return f.Enum
		}
	}
	return nil
}

// enumContains reports whether value is a member of enum. A nil/empty enum
// accepts any value (no declared constraint).
func enumContains(enum []string, value string) bool {
	if len(enum) == 0 {
		return true
	}
	for _, allowed := range enum {
		if allowed == value {
			return true
		}
	}
	return false
}

// validateEnumValue checks value against the named field's declared Enum
// (when the field declares one) and, on mismatch, returns an error using the
// exact wording the inline switch-case validators it replaces used — so
// error text stays verbatim across the 260814 Phase 1 refactor (tests assert
// on it).
func validateEnumValue(toolName string, fields []tuningField, fieldName, value string) error {
	enum := fieldEnum(fields, fieldName)
	if enumContains(enum, value) {
		return nil
	}
	return fmt.Errorf("%s: value must be one of %s; got %q", toolName, strings.Join(enum, ", "), value)
}
