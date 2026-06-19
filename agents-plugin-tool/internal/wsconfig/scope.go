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

// ResolvedValue carries a config item value together with the scope it was
// resolved from, enabling get/show to report which layer provided the value.
type ResolvedValue struct {
	Value string
	Scope Scope
}

// scopeRegistry maps item keys to their declared default write scope. Items
// absent from the registry default to ScopeProject (see DefaultScope).
var scopeRegistry = map[string]Scope{}

// RegisterDefaultScope declares the default write scope for an item key. Call
// this at package init time for items that should write to a non-project scope
// by default. Items not registered default to ScopeProject.
func RegisterDefaultScope(key string, scope Scope) {
	scopeRegistry[key] = scope
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
