package wsconfig

// Scope identifies which config layer holds a value. The resolution order is:
// session > project > repo > global > builtin (highest to lowest precedence).
type Scope string

const (
	// ScopeSession is the ephemeral per-key session store (keys/<key>.json).
	ScopeSession Scope = "session"
	// ScopeProject is the per-project, machine-local file (~/.ws@<id>/config.json).
	ScopeProject Scope = "project"
	// ScopeRepo is the committed, version-tracked project file
	// (<repo-root>/.ws-workflow/config.json). Unlike ScopeProject it is checked
	// into the downstream repository, so it carries a project-wide decision that
	// is shared across every contributor and read deterministically by the tools.
	// It sits below the machine-local project scope so a per-machine override can
	// still win for local experimentation, and above global so a committed
	// project baseline overrides a cross-project user default. Read-only here:
	// the file is hand-edited (or edited by a dedicated flow), not written via
	// config.tune — hence it is absent from ScopeSchemaEnum.
	ScopeRepo Scope = "repo"
	// ScopeGlobal is the cross-project global file (~/.ws/config.json).
	ScopeGlobal Scope = "global"
	// ScopeBuiltin is the code-default floor; returned when no file scope holds the key.
	ScopeBuiltin Scope = "builtin"
)

// Well-known item key constants for registered config items. Use these instead of
// raw string literals to ensure consistent naming across packages.
const (
	// ItemWorkflowPreferSubagent is the global preference for default
	// delegation of eligible general work through lead-delegate. Builtin default: off.
	ItemWorkflowPreferSubagent = "workflow.prefer_subagent"

	// ItemSageReview is the layered config key for the completeness stage of the
	// sage review gate at actionable ready/ promotion. Value "auto" requires the
	// completeness reviewer; "ask" recommends it; absent/empty/"off" skips it.
	// The design stage has its own key (ItemSageReviewDesign); this key no longer
	// governs it. Builtin default: auto.
	ItemSageReview = "sage_review"

	// ItemSageReviewDesign is the layered config key for the design stage of the
	// sage review gate at actionable ready/ promotion, with the same off|ask|auto
	// domain as ItemSageReview. Epics are exempt: an epic's explicitly invoked
	// design review runs regardless of this key. Builtin default: off — design
	// review is an opt-in for users who hand most work to agents.
	ItemSageReviewDesign = "sage_review_design"

	// ItemReviewPhase selects the worker's per-phase independent code review that
	// route.resolve_implement allocates. Values: "lite" (one fresh medium-tier
	// reviewer covering correctness and test integrity, one pass, whatever the
	// risk facts say), "full" (the risk-keyed allocation and two-round
	// protocol), or "off" (no per-phase review; the pre-merge range review
	// remains the integration net). An explicit policy.review.override of
	// single/partitioned wins over every value. Builtin default: lite.
	ItemReviewPhase = "review_phase"

	// ItemSageReviewDesignTier is the model capability tier for the design reviewer
	// delegate. Accepted values mirror the ws tier vocabulary (small/medium/large/xlarge).
	// Builtin default: "large".
	ItemSageReviewDesignTier = "sage_review_design_tier"

	// ItemSageReviewCompletenessTier is the model capability tier for the
	// completeness reviewer delegate. Builtin default: "medium".
	ItemSageReviewCompletenessTier = "sage_review_completeness_tier"

	// ItemWorkflowLang is the layered config key for the user's preferred
	// conversation language. When set, playbook.read injects a language-binding
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

	// ItemBootstrapAlarm gates the session-bootstrap staleness warning that
	// fires when a downstream project's root AGENTS.md Template Version tag is
	// behind the shipped lead-bootstrap template's tag. Values: "on" (builtin
	// default) or "off". Global-only: this is a cross-project user preference
	// about warning noise, not a per-project opt-in.
	ItemBootstrapAlarm = "bootstrap_alarm"

	// ItemTicketAssigneeAware is the opt-in project gate for ticket assignee
	// awareness. Values: "on" or "off" (builtin default: off). It is meant to be
	// a shared, deterministic team decision, so a downstream project sets it in
	// the committed repo scope (.ws-workflow/config.json); it still resolves
	// through the normal chain, so a per-machine project override can win locally
	// (the feature is opt-in coordination, not adversarial enforcement — see
	// 260917-feat-ws-committed-project-config-scope's settled overridability
	// decision). Off means the entire assignee feature is inert. The key is
	// hyphenated (not the underscore form other items use) because it is the
	// literal key a downstream author hand-writes into .ws-workflow/config.json,
	// documented as `ticket-assignee-aware`; config.json keys are the item key
	// verbatim.
	ItemTicketAssigneeAware = "ticket-assignee-aware"

	// ItemWorktreePool is the pool location where worktree.acquire provisions
	// and recycles per-worker Git worktrees. The value is either an absolute
	// path or a template containing the $(GitRoot) and/or $(GitRootDirName)
	// tokens ($(GitRoot) binds to the primary (main) worktree root so a call
	// from any linked worktree resolves to the one shared pool;
	// $(GitRootDirName) is filepath.Base($(GitRoot))). The builtin default is
	// a filesystem sibling of the repo, outside the working tree, so IDEs do
	// not index the pooled worktrees — see
	// agents-plugin-tool/internal/mcp/worktree_tools.go's
	// defaultWorktreePoolTemplate for the exact template string.
	// provisionWorktree (not resolvePoolRoot, which stays a pure string
	// function) falls back to the in-tree legacyInTreePoolTemplate when the
	// sibling parent is not writable. Resolved through the layered config
	// (project/global/session files + builtin) rather than a dedicated
	// config.tune writer.
	ItemWorktreePool = "worktree_pool"
)

func init() {
	RegisterGlobalOnly(ItemWorkflowPreferSubagent)
	RegisterGlobalOnly(ItemWorkflowSkepticalPosture)
	RegisterGlobalOnly(ItemBootstrapAlarm)
	// sage_review* keys default to project scope: they are project-level opt-ins
	// that should persist across sessions for the same project.
	RegisterDefaultScope(ItemSageReview, ScopeProject)
	RegisterDefaultScope(ItemSageReviewDesign, ScopeProject)
	// review_phase defaults to project scope like the sage_review* keys: a
	// project-level choice that persists across sessions. A lead's session-scope
	// value reaches the workers it spawns through the session parent walk.
	RegisterDefaultScope(ItemReviewPhase, ScopeProject)
	RegisterDefaultScope(ItemSageReviewDesignTier, ScopeProject)
	RegisterDefaultScope(ItemSageReviewCompletenessTier, ScopeProject)
	// workflow.lang defaults to global scope: language is a cross-project user preference.
	RegisterDefaultScope(ItemWorkflowLang, ScopeGlobal)
	// worktree_pool defaults to project scope: the pool location is a per-project
	// choice (declared explicitly even though ScopeProject is the fallback, to
	// keep every item's scope declaration visible in one place).
	RegisterDefaultScope(ItemWorktreePool, ScopeProject)
	// ticket-assignee-aware is a committed team decision with no config.tune
	// writer; declaring it makes config.list's scoped view always list it with
	// its repo-scope flag, so a team sees the key before it commits one.
	RegisterDefaultScope(ItemTicketAssigneeAware, ScopeProject)
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

// RepoScoped reports whether the committed repo scope can supply an item key:
// a resolver-backed key that is not global-only. A key outside the resolver
// (agents.tier, whose repo file contributes only its overrides map) never
// reads the flat repo overlay, and a global-only key's resolution skips the
// session/project/repo overlays (see Resolver.Get). This is the single
// implementation of the repo-flag rule; config.list's scoped view and the MCP
// tuning catalog both call it.
func RepoScoped(key string, resolverBacked bool) bool {
	return resolverBacked && !GlobalOnly(key)
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

// RegisteredItemKeys returns every item key that declares a default scope or
// global-only status. Callers that must keep their own keys disjoint from the
// registered items (an adapter key manifest's namespace check) read it.
func RegisteredItemKeys() []string {
	keys := make([]string, 0, len(scopeRegistry))
	for key := range scopeRegistry {
		keys = append(keys, key)
	}
	return keys
}
