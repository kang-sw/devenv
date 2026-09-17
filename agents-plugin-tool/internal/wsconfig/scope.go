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

	// ItemSageReview is the layered config key for the sage review gate on ticket
	// writes. Value "auto" runs reviewers unconditionally after a todo/ready commit;
	// "ask" prompts the user first; absent/empty/"off" disables the gate entirely.
	// Builtin default: auto (gate runs unless a project/session/global override
	// disables it).
	ItemSageReview = "sage_review"

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
	RegisterDefaultScope(ItemSageReviewDesignTier, ScopeProject)
	RegisterDefaultScope(ItemSageReviewCompletenessTier, ScopeProject)
	// workflow.lang defaults to global scope: language is a cross-project user preference.
	RegisterDefaultScope(ItemWorkflowLang, ScopeGlobal)
	// worktree_pool defaults to project scope: the pool location is a per-project
	// choice (declared explicitly even though ScopeProject is the fallback, to
	// keep every item's scope declaration visible in one place).
	RegisterDefaultScope(ItemWorktreePool, ScopeProject)
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
