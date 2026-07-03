package wsconfig

// Scope identifies which config layer holds a value. The resolution order is:
// session > project > global > builtin (highest to lowest precedence).
type Scope string

const (
	// ScopeSession is the ephemeral per-key session store (keys/<key>.json).
	ScopeSession Scope = "session"
	// ScopeProject is the per-project file (~/.ws@<id>/config.json).
	ScopeProject Scope = "project"
	// ScopeGlobal is the cross-project global file (~/.ws/config.json).
	ScopeGlobal Scope = "global"
	// ScopeBuiltin is the code-default floor; returned when no file scope holds the key.
	ScopeBuiltin Scope = "builtin"
)

// Well-known item key constants for registered config items. Use these instead of
// raw string literals to ensure consistent naming across packages.
const (
	// ItemPreferMercenary is the retired unprefixed mercenary preference key.
	// Keep the constant for legacy record tests and orphaned local state checks;
	// new code must use ItemWorkflowPreferMercenary and must not migrate this key.
	ItemPreferMercenary = "prefer_mercenary"

	// ItemWorkflowPreferSubagent is the global bootstrap preference for loading
	// strict subagent posture with the workflow manual. Builtin default: off.
	ItemWorkflowPreferSubagent = "workflow.prefer_subagent"

	// ItemWorkflowPreferMercenary is the global workflow preference for mercenary
	// visibility and default render guidance. Builtin default: hide.
	ItemWorkflowPreferMercenary = "workflow.prefer_mercenary"

	// ItemSageReview is the layered config key for the sage review gate on ticket
	// writes. Value "auto" runs reviewers unconditionally after a todo/ready commit;
	// "ask" prompts the user first; absent/empty/"off" disables the gate entirely.
	// Builtin default: off (absent = disabled).
	ItemSageReview = "sage_review"

	// ItemSageReviewDesignTier is the model capability tier for the design reviewer
	// delegate. Accepted values mirror the ws tier vocabulary (small/medium/large/xlarge).
	// Builtin default: "large".
	ItemSageReviewDesignTier = "sage_review_design_tier"

	// ItemSageReviewCompleteness controls whether the completeness reviewer runs
	// alongside the design reviewer. Value "true" enables it; "false" disables.
	// Builtin default: "true".
	ItemSageReviewCompleteness = "sage_review_completeness"

	// ItemSageReviewCompletenessTier is the model capability tier for the
	// completeness reviewer delegate. Builtin default: "medium".
	ItemSageReviewCompletenessTier = "sage_review_completeness_tier"

	// ItemWorkflowLang is the layered config key for the user's preferred
	// conversation language. When set, playbook.print injects a language-binding
	// instruction into the UserPreferenceSection seed of lead-workflow-manual.
	// Empty string means no binding (no injection). Declared default scope:
	// ScopeGlobal (language is a cross-project user preference).
	ItemWorkflowLang = "workflow.lang"

	// ItemWorkflowSkepticalPosture controls whether the workflow manual renders
	// a skeptical-posture block reminding the AI to treat user claims as
	// hypotheses requiring evidence rather than ground truth. This exists because
	// LLMs tend to accept user statements (e.g. casual examples, remembered names)
	// at face value and propagate them without verification.
	// Values: "on" (default) or "off".
	ItemWorkflowSkepticalPosture = "workflow.skeptical_posture"
)

func init() {
	RegisterGlobalOnly(ItemWorkflowPreferSubagent)
	RegisterGlobalOnly(ItemWorkflowPreferMercenary)
	RegisterGlobalOnly(ItemWorkflowSkepticalPosture)
	// sage_review* keys default to project scope: they are project-level opt-ins
	// that should persist across sessions for the same project.
	RegisterDefaultScope(ItemSageReview, ScopeProject)
	RegisterDefaultScope(ItemSageReviewDesignTier, ScopeProject)
	RegisterDefaultScope(ItemSageReviewCompleteness, ScopeProject)
	RegisterDefaultScope(ItemSageReviewCompletenessTier, ScopeProject)
	// workflow.lang defaults to global scope: language is a cross-project user preference.
	RegisterDefaultScope(ItemWorkflowLang, ScopeGlobal)
}

// ResolvedValue carries a config item value together with the scope it was
// resolved from, enabling get/show to report which layer provided the value.
type ResolvedValue struct {
	Value string
	Scope Scope
}

// scopeRegistry maps item keys to their declared default write scope. Items
// absent from the registry default to ScopeProject (see DefaultScope).
var scopeRegistry = map[string]Scope{}
var globalOnlyRegistry = map[string]struct{}{}

// RegisterDefaultScope declares the default write scope for an item key. Call
// this at package init time for items that should write to a non-project scope
// by default. Items not registered default to ScopeProject.
func RegisterDefaultScope(key string, scope Scope) {
	scopeRegistry[key] = scope
}

// RegisterGlobalOnly declares a config item that always resolves and writes
// through global/builtin scopes, skipping session and project overlays.
func RegisterGlobalOnly(key string) {
	RegisterDefaultScope(key, ScopeGlobal)
	globalOnlyRegistry[key] = struct{}{}
}

// GlobalOnly reports whether an item is constrained to global/builtin scopes.
func GlobalOnly(key string) bool {
	_, ok := globalOnlyRegistry[key]
	return ok
}

// DefaultScope returns the declared default write scope for the given item key,
// falling back to ScopeProject when the item has no declaration.
func DefaultScope(key string) Scope {
	if s, ok := scopeRegistry[key]; ok {
		return s
	}
	return ScopeProject
}

// ScopeSchemaEnum returns the allowed scope values as a string slice for use in
// MCP tool inputSchema enum properties. This is the shared schema fragment that
// every scope-aware config tool can consume.
func ScopeSchemaEnum() []string {
	return []string{"session", "project", "global"}
}
