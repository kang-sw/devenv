package wsconfig

import (
	"fmt"
	"time"
)

// lockTimeout is the maximum time to wait for a file lock before returning an error.
const lockTimeout = 10 * time.Second

// SetOptions controls a scoped set operation.
type SetOptions struct {
	// ExplicitScope, when non-empty, overrides the item's declared default scope.
	// Must be one of ScopeSession, ScopeProject, or ScopeGlobal.
	ExplicitScope Scope
	// SessionKey is required when ExplicitScope == ScopeSession or when the
	// item's declared default scope is ScopeSession.
	SessionKey string
	// CapabilityCheck is an optional hook that the caller uses to enforce
	// item-level write gating (e.g. lead-only restrictions). If set, the setter
	// calls it before writing and returns the error if it is non-nil.
	CapabilityCheck func(key string, targetScope Scope) error
}

// SessionReader provides read access to session-scoped overrides. Implemented
// by the session store adapter injected from the mcp layer.
type SessionReader interface {
	// GetOverride returns (value, true) when the session record for the given key
	// contains an Overrides entry for the given item key, or ("", false) otherwise.
	GetOverride(sessionKey, itemKey string) (string, bool)
}

// SessionWriter provides write access to session-scoped overrides.
type SessionWriter interface {
	// SetOverride writes an Overrides entry for the given item key/value into the
	// session record for the given session key. Returns an error if the session
	// key is not found or the write fails.
	SetOverride(sessionKey, itemKey, value string) error
	// DeleteOverride removes an Overrides entry for the given item key from the
	// session record for the given session key. Returns an error if the session
	// key is not found; removing an absent item key is a no-op (not an error).
	DeleteOverride(sessionKey, itemKey string) error
}

// Resolver resolves config item values across the session > project > global >
// builtin precedence chain. It is a value type; construct one with NewResolver.
type Resolver struct {
	opts     Options
	builtin  map[string]string
	sessionR SessionReader // nil when session scope is not available
	sessionW SessionWriter // nil when session scope writes are not supported
}

// NewResolver creates a Resolver. builtinDefaults provides the code-default
// floor values (builtin scope). sessionReader/sessionWriter may be nil; when
// nil, session scope is skipped during resolution and set operations that target
// session scope will return an error.
func NewResolver(opts Options, builtinDefaults map[string]string, sessionReader SessionReader, sessionWriter SessionWriter) Resolver {
	if builtinDefaults == nil {
		builtinDefaults = map[string]string{}
	}
	return Resolver{
		opts:     opts,
		builtin:  builtinDefaults,
		sessionR: sessionReader,
		sessionW: sessionWriter,
	}
}

// Get resolves the value for the given item key, walking
// session → project → repo → global → builtin. The returned ResolvedValue
// carries the value and the scope it resolved from. If the key is absent from
// all scopes, Scope is ScopeBuiltin and Value is "". Global-only items
// (GlobalOnly) skip the session/project/repo overlays entirely — see
// getGlobalOnly.
func (r *Resolver) Get(sessionKey, itemKey string) (ResolvedValue, error) {
	if GlobalOnly(itemKey) {
		return r.getGlobalOnly(itemKey)
	}

	// Session scope.
	if r.sessionR != nil && sessionKey != "" {
		if v, ok := r.sessionR.GetOverride(sessionKey, itemKey); ok {
			return ResolvedValue{Value: v, Scope: ScopeSession}, nil
		}
	}

	// Project scope.
	projectCfg, err := Load(r.opts)
	if err != nil {
		return ResolvedValue{}, fmt.Errorf("resolver: load project config: %w", err)
	}
	if projectCfg.Overrides != nil {
		if v, ok := projectCfg.Overrides[itemKey]; ok {
			return ResolvedValue{Value: v, Scope: ScopeProject}, nil
		}
	}

	// Repo scope (committed, version-tracked — shared across contributors, below
	// the machine-local project scope so a per-machine override can still win).
	repoCfg, err := loadRepoConfig(r.opts)
	if err != nil {
		return ResolvedValue{}, fmt.Errorf("resolver: load repo config: %w", err)
	}
	if repoCfg.Overrides != nil {
		if v, ok := repoCfg.Overrides[itemKey]; ok {
			return ResolvedValue{Value: v, Scope: ScopeRepo}, nil
		}
	}

	// Global scope.
	globalCfg, err := loadGlobalConfig(r.opts)
	if err != nil {
		return ResolvedValue{}, fmt.Errorf("resolver: load global config: %w", err)
	}
	if globalCfg.Overrides != nil {
		if v, ok := globalCfg.Overrides[itemKey]; ok {
			return ResolvedValue{Value: v, Scope: ScopeGlobal}, nil
		}
	}

	// Builtin floor.
	v := r.builtin[itemKey]
	return ResolvedValue{Value: v, Scope: ScopeBuiltin}, nil
}

func (r *Resolver) getGlobalOnly(itemKey string) (ResolvedValue, error) {
	globalCfg, err := loadGlobalConfig(r.opts)
	if err != nil {
		return ResolvedValue{}, fmt.Errorf("resolver: load global config: %w", err)
	}
	if globalCfg.Overrides != nil {
		if v, ok := globalCfg.Overrides[itemKey]; ok {
			return ResolvedValue{Value: v, Scope: ScopeGlobal}, nil
		}
	}
	v := r.builtin[itemKey]
	return ResolvedValue{Value: v, Scope: ScopeBuiltin}, nil
}

// Set writes the value for the given item key to the appropriate scope. The
// target scope is determined as: setOpts.ExplicitScope (when non-empty) else the
// item's declared default scope. Item-level capability gating is honored via the
// optional CapabilityCheck hook.
func (r *Resolver) Set(itemKey, value string, setOpts SetOptions) error {
	targetScope := setOpts.ExplicitScope
	if targetScope == "" {
		targetScope = DefaultScope(itemKey)
	}
	if GlobalOnly(itemKey) && targetScope != ScopeGlobal {
		return fmt.Errorf("resolver: item %q is global-only", itemKey)
	}

	// Capability check hook — item-level write gating.
	if setOpts.CapabilityCheck != nil {
		if err := setOpts.CapabilityCheck(itemKey, targetScope); err != nil {
			return err
		}
	}

	switch targetScope {
	case ScopeSession:
		if r.sessionW == nil {
			return fmt.Errorf("resolver: session writer not available")
		}
		if setOpts.SessionKey == "" {
			return fmt.Errorf("resolver: session_key required for session-scope set")
		}
		return r.sessionW.SetOverride(setOpts.SessionKey, itemKey, value)
	case ScopeProject:
		path, err := Path(r.opts)
		if err != nil {
			return err
		}
		return setOverrideInFile(path, itemKey, value)
	case ScopeGlobal:
		path, err := GlobalPath(r.opts)
		if err != nil {
			return err
		}
		return setOverrideInFile(path, itemKey, value)
	default:
		return fmt.Errorf("resolver: unsupported write scope %q", targetScope)
	}
}

// Unset resets an item to its next-broader-scope (or builtin) resolution by
// removing the override entry from the target scope. Session, project, and
// global scopes are all supported. Removing a key that does not exist is a
// no-op (not an error). Unset never writes an empty-string value — that is a
// distinct intent covered by Set with an explicit empty value.
func (r *Resolver) Unset(itemKey string, setOpts SetOptions) error {
	targetScope := setOpts.ExplicitScope
	if targetScope == "" {
		targetScope = DefaultScope(itemKey)
	}
	if GlobalOnly(itemKey) && targetScope != ScopeGlobal {
		return fmt.Errorf("resolver: item %q is global-only", itemKey)
	}

	// Capability check hook — mirrors Set's item-level write gating.
	if setOpts.CapabilityCheck != nil {
		if err := setOpts.CapabilityCheck(itemKey, targetScope); err != nil {
			return err
		}
	}

	switch targetScope {
	case ScopeProject:
		path, err := Path(r.opts)
		if err != nil {
			return err
		}
		return deleteOverrideInFile(path, itemKey)
	case ScopeGlobal:
		path, err := GlobalPath(r.opts)
		if err != nil {
			return err
		}
		return deleteOverrideInFile(path, itemKey)
	case ScopeSession:
		if r.sessionW == nil {
			return fmt.Errorf("resolver: session writer not available")
		}
		if setOpts.SessionKey == "" {
			return fmt.Errorf("resolver: session_key required for session-scope unset")
		}
		return r.sessionW.DeleteOverride(setOpts.SessionKey, itemKey)
	default:
		return fmt.Errorf("resolver: unsupported unset scope %q", targetScope)
	}
}

// deleteOverrideInFile removes overrides[key] from the config file at path
// through the shared locked writer. A missing key or missing file is a no-op
// that writes nothing; a missing file also creates no directory or lock file.
func deleteOverrideInFile(path, itemKey string) error {
	if !configFileExists(path) {
		return nil
	}
	_, err := updateConfigFile(path, func(cfg *Config) error {
		if _, exists := cfg.Overrides[itemKey]; !exists {
			return errConfigUnchanged
		}
		delete(cfg.Overrides, itemKey)
		return nil
	})
	return err
}

// GetBool resolves the value for itemKey and interprets it as a boolean.
// "true" (case-sensitive exact match) → true; any other value including empty
// or absent → false. Also returns the scope the value resolved from.
// This is a thin convenience wrapper over Get; the signature of Get is unchanged.
func (r *Resolver) GetBool(sessionKey, itemKey string) (bool, Scope, error) {
	rv, err := r.Get(sessionKey, itemKey)
	if err != nil {
		return false, ScopeBuiltin, err
	}
	return rv.Value == "true", rv.Scope, nil
}

// setOverrideInFileRMW sets overrides[key] to transform(current value) through
// the shared locked writer, so the new value may depend on the current one
// (for example an integer increment). current is "" when the key is absent.
func setOverrideInFileRMW(path, itemKey string, transform func(current string) string) error {
	_, err := updateConfigFile(path, func(cfg *Config) error {
		if cfg.Overrides == nil {
			cfg.Overrides = map[string]string{}
		}
		cfg.Overrides[itemKey] = transform(cfg.Overrides[itemKey])
		return nil
	})
	return err
}

// setOverrideInFile sets overrides[key] = value through the shared locked
// writer (see updateConfigFile).
func setOverrideInFile(path, itemKey, value string) error {
	return setOverrideInFileRMW(path, itemKey, func(string) string { return value })
}
