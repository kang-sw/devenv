package mcp

import (
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
)

// Shared enum value slices used by both the config.* tools() schema literals
// (server.go tools()) and the configRegistry entries below, so schema
// construction and registry-driven validation/catalog projection always read
// the identical list. This is Phase 1 of 260814 (config-registry-extract): a
// pure internal refactor — nothing about tools/list schema shape or order
// changes, only where these enum values live.
var (
	onOffEnum             = []string{"on", "off"}
	sageReviewEnum        = []string{"off", "ask", "auto"}
	reviewPhaseEnum       = []string{"off", "lite", "full"}
	agentsTierEnum        = []string{"small", "medium", "large", "xlarge"}
	promptHarnessEnum     = []string{"claude", "codex", "pi", "*"}
	agentsTierHarnessEnum = []string{"claude", "codex", "pi", "default"}
	agentsTierScopeEnum   = []string{"project", "global"}
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
	// key beyond the config.* prefix gate (today: the global-only
	// workflow-preference writers, whose entries derive it from
	// wsconfig.GlobalOnly, and workflow.whiteboard_enabled and
	// workflow.tracker_enabled, which set it explicitly because they resolve
	// through every scope).
	RequiresLeadAuthority bool
	// ResolverBacked reports whether this key is read/written through
	// wsconfig.Resolver.Get/Set/Unset. False only for agents.tier, which
	// writes structured AgentTier values via
	// wsconfig.SetAgentsTierForHarness directly rather than the resolver's
	// flat map[string]string overlay (260814 Phase 1 plan's
	// resolver-bypass finding: folding it in would be a resolver capability
	// extension, not a pure internal refactor).
	ResolverBacked bool
	// Declared is set for a key an adapter manifest declares (see
	// config_manifest.go); its type validates config.tune writes and its
	// DefaultScope replaces the wsconfig scope registry lookup.
	Declared *declaredConfigKey
	// Structured marks a key whose value is a JSON object stored in a config
	// file's structured_overrides member (the worktree.profile.<name> family)
	// rather than a string in overrides. It never has a session-scope value.
	Structured bool
}

// GlobalOnly reports whether this key's writes are constrained to
// global/builtin scope. Delegates to wsconfig.GlobalOnly for resolver-backed
// keys; agents.tier is a compound writer with explicit project/global handling
// in config.tune, so it is not global-only.
func (e configKeyEntry) GlobalOnly() bool {
	if !e.ResolverBacked {
		return false
	}
	return wsconfig.GlobalOnly(e.Key)
}

// DefaultScope reports this key's declared default write scope. Delegates to
// wsconfig.DefaultScope for resolver-backed keys; agents.tier defaults to its
// project-scope compound writer.
func (e configKeyEntry) DefaultScope() wsconfig.Scope {
	if e.Declared != nil {
		return e.Declared.DefaultScope
	}
	if !e.ResolverBacked {
		return wsconfig.ScopeProject
	}
	return wsconfig.DefaultScope(e.Key)
}

// RepoScoped reports whether the committed repo scope can supply this key.
// Delegates to wsconfig.RepoScoped, the single implementation of the rule.
func (e configKeyEntry) RepoScoped() bool {
	return wsconfig.RepoScoped(e.Key, e.ResolverBacked)
}

// RepoKey is the key a committed repo-scope file stores this knob under. A
// harness-bucketed knob stores one entry per harness bucket, so its key names
// the bucket set (claude|codex|pi|all, where all applies to every harness).
func (e configKeyEntry) RepoKey() string {
	if e.HarnessBucketed() {
		return e.Key + ".<" + strings.Join(harnessBuckets, "|") + ">"
	}
	return e.Key
}

// HarnessBucketed reports whether this knob stores one value per harness
// bucket under <Key>.<bucket> (see harnessBucketKey) and resolves through
// resolveHarnessBucketed. agents.tier is harness-applicable but not
// resolver-backed: its structured per-harness values have their own writer,
// so it never takes the bucket path.
func (e configKeyEntry) HarnessBucketed() bool {
	return e.HarnessApplicable && e.ResolverBacked
}

// harnessBuckets is the fixed set of harness buckets a harness-bucketed knob
// stores values under, in listing order. "all" is the stored spelling of the
// caller-facing "*" and applies to every harness.
var harnessBuckets = []string{"claude", "codex", "pi", "all"}

// harnessBucketKey is the stored key of key's value for one harness bucket;
// the caller-facing "*" is stored as "all".
func harnessBucketKey(key, harness string) string {
	if harness == "*" {
		harness = "all"
	}
	return key + "." + harness
}

// harnessBucketReadOrder lists the buckets a read for harness tries, in
// order: the harness's own bucket, then all. An unknown (empty) harness, or
// all itself, reads only all.
func harnessBucketReadOrder(harness string) []string {
	if harness == "" || harness == "*" || harness == "all" {
		return []string{"all"}
	}
	return []string{harness, "all"}
}

// resolveHarnessBucketed resolves a harness-bucketed knob bucket-major: each
// bucket in harnessBucketReadOrder through every scope the resolver reads
// (session, project, repo, global, then a builtin seeded on the bucket key),
// then the bare key through every scope and its builtin. The bare key holds
// legacy values stored before the knob was bucketed, read in the all
// position, and the knob's builtin default. An empty value is unset. It
// returns the stored key that answered.
func resolveHarnessBucketed(resolver *wsconfig.Resolver, sessionKey, key, harness string) (wsconfig.ResolvedValue, string, error) {
	for _, bucket := range harnessBucketReadOrder(harness) {
		bucketKey := harnessBucketKey(key, bucket)
		rv, err := resolver.Get(sessionKey, bucketKey)
		if err != nil {
			return wsconfig.ResolvedValue{}, "", err
		}
		if rv.Value != "" {
			return rv, bucketKey, nil
		}
	}
	rv, err := resolver.Get(sessionKey, key)
	if err != nil {
		return wsconfig.ResolvedValue{}, "", err
	}
	return rv, key, nil
}

// configRegistry holds the static per-key entries. The dynamic prompt.*
// family is not represented here — it is generated per discovered override
// point by promptKnobEntry at catalog-build time (see buildTuningCatalog).
var configRegistry = []configKeyEntry{
	{
		Key:        wsconfig.ItemWorkflowPreferSubagent,
		WriterTool: "config.tune",
		ResetTool:  "config.tune",
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
		Key:        wsconfig.ItemWorkflowWhiteboardEnabled,
		WriterTool: "config.tune",
		ResetTool:  "config.tune",
		SelectorFields: []tuningField{
			{
				Name:        "harness",
				Description: "Harness bucket the value applies to. When omitted, defaults to the current session's detected harness. Use * explicitly for every harness (all).",
				Enum:        promptHarnessEnum,
			},
			{
				Name:        "scope",
				Description: "Storage scope. When omitted the write lands in the item's declared default scope (global).",
				Enum:        wsconfig.ScopeSchemaEnum(),
			},
		},
		ValueFields: []tuningField{{
			Name:        "value",
			Description: "Desired mode: on or off. Omit when reset is true.",
			Enum:        onOffEnum,
		}},
		HarnessApplicable: true,
		NoAgentVisible:    true,
		// A lead-only knob at every scope: it shapes the lead's own channel
		// to the user, so a write needs a lead session_key.
		RequiresLeadAuthority: true,
		ResolverBacked:        true,
	},
	{
		Key:        wsconfig.ItemWorkflowTrackerEnabled,
		WriterTool: "config.tune",
		ResetTool:  "config.tune",
		SelectorFields: []tuningField{
			{
				Name:        "harness",
				Description: "Harness bucket the value applies to. When omitted, defaults to the current session's detected harness. Use * explicitly for every harness (all).",
				Enum:        promptHarnessEnum,
			},
			{
				Name:        "scope",
				Description: "Storage scope. When omitted the write lands in the item's declared default scope (global).",
				Enum:        wsconfig.ScopeSchemaEnum(),
			},
		},
		ValueFields: []tuningField{{
			Name:        "value",
			Description: "Desired mode: on or off. Omit when reset is true.",
			Enum:        onOffEnum,
		}},
		HarnessApplicable: true,
		NoAgentVisible:    true,
		// A lead-only knob at every scope: it shapes the lead's own
		// recording habit, so a write needs a lead session_key.
		RequiresLeadAuthority: true,
		ResolverBacked:        true,
	},
	{
		Key:        wsconfig.ItemBootstrapAlarm,
		WriterTool: "config.tune",
		ResetTool:  "config.tune",
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
		Key:        wsconfig.ItemSageReview,
		WriterTool: "config.tune",
		ResetTool:  "config.tune",
		SelectorFields: []tuningField{{
			Name:        "scope",
			Description: "Storage scope. When omitted the write lands in the item's declared default scope (project).",
			Enum:        wsconfig.ScopeSchemaEnum(),
		}},
		ValueFields: []tuningField{{
			Name:        "value",
			Description: "Completeness-stage review posture: off skips it, ask recommends it, auto requires it. Omit when reset is true.",
			Enum:        sageReviewEnum,
		}},
		NoAgentVisible: true,
		ResolverBacked: true,
	},
	{
		Key:        wsconfig.ItemSageReviewDesign,
		WriterTool: "config.tune",
		ResetTool:  "config.tune",
		SelectorFields: []tuningField{{
			Name:        "scope",
			Description: "Storage scope. When omitted the write lands in the item's declared default scope (project).",
			Enum:        wsconfig.ScopeSchemaEnum(),
		}},
		ValueFields: []tuningField{{
			Name:        "value",
			Description: "Design-stage review posture: off skips it, ask recommends it, auto requires it. Omit when reset is true.",
			Enum:        sageReviewEnum,
		}},
		NoAgentVisible: true,
		ResolverBacked: true,
	},
	{
		Key:        wsconfig.ItemTicketReviewAutoAck,
		WriterTool: "config.tune",
		ResetTool:  "config.tune",
		SelectorFields: []tuningField{{
			Name:        "scope",
			Description: "Storage scope. When omitted the write lands in the item's declared default scope (project).",
			Enum:        wsconfig.ScopeSchemaEnum(),
		}},
		ValueFields: []tuningField{{
			Name:        "value",
			Description: "Desired mode: on or off. Omit when reset is true.",
			Enum:        onOffEnum,
		}},
		NoAgentVisible: true,
		ResolverBacked: true,
	},
	{
		Key:        wsconfig.ItemReviewPhase,
		WriterTool: "config.tune",
		ResetTool:  "config.tune",
		SelectorFields: []tuningField{{
			Name:        "scope",
			Description: "Storage scope. When omitted the write lands in the item's declared default scope (project).",
			Enum:        wsconfig.ScopeSchemaEnum(),
		}},
		ValueFields: []tuningField{{
			Name:        "value",
			Description: "Per-phase independent code review: lite dispatches one medium-tier correctness and test-integrity reviewer for one pass, full dispatches the risk-keyed review with two rounds, off skips it. Omit when reset is true.",
			Enum:        reviewPhaseEnum,
		}},
		NoAgentVisible: true,
		ResolverBacked: true,
	},
	{
		// workflow.lang is the one free-form scalar knob: its value field
		// declares no Enum, which is what exempts it from config.tune's
		// lowercase-and-enum-check normalization (a language name such as
		// "Korean" is stored as written).
		Key:        wsconfig.ItemWorkflowLang,
		WriterTool: "config.tune",
		ResetTool:  "config.tune",
		SelectorFields: []tuningField{{
			Name:        "scope",
			Description: "Storage scope. When omitted the write lands in the item's declared default scope (global).",
			Enum:        wsconfig.ScopeSchemaEnum(),
		}},
		ValueFields: []tuningField{{
			Name:        "value",
			Description: "Language name, free-form (for example Korean). Omit when reset is true.",
		}},
		NoAgentVisible: true,
		ResolverBacked: true,
	},
	{
		// agents.tier has no wsconfig item key — it bypasses the resolver
		// entirely (see ResolverBacked doc above), so Key here is the
		// catalog knob id rather than a wsconfig.Item* constant.
		Key:        "agents.tier",
		WriterTool: "config.tune",
		ResetTool:  "config.tune",
		SelectorFields: []tuningField{
			{
				Name:        "harness",
				Description: "Optional harness alias key to configure. When omitted, ws uses the detected MCP session harness, or default when none is known.",
				Enum:        agentsTierHarnessEnum,
			},
			{
				Name:        "scope",
				Description: "Storage scope. When omitted the write lands in project scope.",
				Enum:        agentsTierScopeEnum,
			},
		},
		ValueFields: []tuningField{
			{
				Name:        "tier",
				Description: "Capability tier to configure; the only value field allowed when reset is true.",
				Enum:        agentsTierEnum,
				Required:    true,
			},
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
				Description: "Optional reasoning effort label for this alias. Empty, omitted, or none leaves backend effort unset; provider support is checked at launch.",
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
		WriterTool: "config.tune",
		ResetTool:  "config.tune",
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
	// config.list/config.tune are the post-collapse generic tools (260814
	// Phase 2), not per-key writers: every registry entry's WriterTool is now
	// "config.tune", so a naive match would resolve the first entry and wrongly
	// mark config.tune lead-only / no-agent-hidden. The tool-name-keyed gating
	// tables must not resolve an entry for them — authority, no-agent, and
	// session-key requirements are enforced per resolved key inside config.tune's
	// dispatch instead. config.resolve_agent (260905 Phase 3) is the same kind
	// of generic, non-per-key-writer tool, so it gets the same early return.
	// Every other config.* tool name was removed with the ten, so this
	// function now returns false for every live config.* tool; it is kept
	// because the gating tables still call it for arbitrary tool names.
	if toolName == "config.list" || toolName == "config.tune" || toolName == "config.resolve_agent" || toolName == "config.get" {
		return configKeyEntry{}, false
	}
	for _, entry := range configRegistry {
		if entry.WriterTool == toolName || (entry.ResetTool != "" && entry.ResetTool == toolName) {
			return entry, true
		}
	}
	return configKeyEntry{}, false
}

// resolveConfigEntryForKey resolves the registry entry for a config.tune runtime
// key argument (e.g. "workflow.prefer_subagent", "agents.tier",
// "prompt.UserPreferenceSection"). Unlike configKeyEntryForTool (keyed by tool
// name), this is keyed by the config key itself, which is what config.tune's
// generic dispatch receives. The dynamic prompt.* and worktree.profile.*
// families resolve through their shared templates keyed by the trimmed suffix.
func resolveConfigEntryForKey(key string) (configKeyEntry, bool) {
	for _, entry := range configRegistry {
		if entry.Key == key {
			return entry, true
		}
	}
	if strings.HasPrefix(key, "prompt.") {
		return promptKnobEntry(strings.TrimPrefix(key, "prompt.")), true
	}
	if strings.HasPrefix(key, worktreeProfilePrefix) {
		return worktreeProfileKnobEntry(strings.TrimPrefix(key, worktreeProfilePrefix)), true
	}
	if strings.HasPrefix(key, wsagent.ProfilePrefix) {
		return agentProfileKnobEntry(strings.TrimPrefix(key, wsagent.ProfilePrefix)), true
	}
	for _, k := range []declaredConfigKey{worktreeNudgeStaleDaysKey, worktreeNudgePreservedGBKey} {
		if k.Key == key {
			return k.entry(), true
		}
	}
	return configKeyEntry{}, false
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
