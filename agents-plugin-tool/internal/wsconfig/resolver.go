package wsconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
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
}

// Resolver resolves config item values across the session > project > global >
// builtin precedence chain. It is a value type; construct one with NewResolver.
type Resolver struct {
	opts      Options
	builtin   map[string]string
	sessionR  SessionReader // nil when session scope is not available
	sessionW  SessionWriter // nil when session scope writes are not supported
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
// session → project → global → builtin. The returned ResolvedValue carries the
// value and the scope it resolved from. If the key is absent from all scopes,
// Scope is ScopeBuiltin and Value is "".
func (r *Resolver) Get(sessionKey, itemKey string) (ResolvedValue, error) {
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

// Set writes the value for the given item key to the appropriate scope. The
// target scope is determined as: setOpts.ExplicitScope (when non-empty) else the
// item's declared default scope. Item-level capability gating is honored via the
// optional CapabilityCheck hook.
func (r *Resolver) Set(itemKey, value string, setOpts SetOptions) error {
	targetScope := setOpts.ExplicitScope
	if targetScope == "" {
		targetScope = DefaultScope(itemKey)
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

// Unset removes an override entry from the target scope. Only project and
// global scopes are supported; session-scope overrides are ephemeral and do
// not need an explicit unset path. Removing a key that does not exist is a
// no-op (not an error).
func (r *Resolver) Unset(itemKey string, setOpts SetOptions) error {
	targetScope := setOpts.ExplicitScope
	if targetScope == "" {
		targetScope = DefaultScope(itemKey)
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
		return fmt.Errorf("resolver: session-scope override cannot be unset (ephemeral; expires with session)")
	default:
		return fmt.Errorf("resolver: unsupported unset scope %q", targetScope)
	}
}

// deleteOverrideInFile performs an flock-serialized read-modify-write on the
// config file at path, removing overrides[key]. A missing key or missing file
// is a no-op.
func deleteOverrideInFile(path, itemKey string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	lockPath := path + ".lock"
	fl := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()
	locked, err := fl.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return fmt.Errorf("acquire config lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("timed out waiting for config file lock: %s", lockPath)
	}
	defer fl.Unlock() //nolint:errcheck

	var cfg Config
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read config for update: %w", err)
	}
	if err == nil {
		if jerr := json.Unmarshal(raw, &cfg); jerr != nil {
			return fmt.Errorf("parse config for update: %w", jerr)
		}
	}
	if _, exists := cfg.Overrides[itemKey]; !exists {
		return nil
	}
	delete(cfg.Overrides, itemKey)

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("encode config: %w", err)
	}
	payload = append(payload, '\n')
	if _, werr := tmp.Write(payload); werr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp config: %w", werr)
	}
	if cerr := tmp.Close(); cerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp config: %w", cerr)
	}
	if rerr := os.Rename(tmpName, path); rerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic rename config: %w", rerr)
	}
	return nil
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

// setOverrideInFileRMW performs an flock-serialized read-modify-write on the
// config file at path. The transform function receives the current string value
// for itemKey (empty string when absent) and returns the new value to store.
// This generalizes setOverrideInFile for use-cases such as integer increment
// where the new value depends on the current value.
func setOverrideInFileRMW(path, itemKey string, transform func(current string) string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	lockPath := path + ".lock"
	fl := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()

	locked, err := fl.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return fmt.Errorf("acquire config lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("timed out waiting for config file lock: %s", lockPath)
	}
	defer fl.Unlock() //nolint:errcheck

	var cfg Config
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read config for update: %w", err)
	}
	if err == nil {
		if jerr := json.Unmarshal(raw, &cfg); jerr != nil {
			return fmt.Errorf("parse config for update: %w", jerr)
		}
	}

	if cfg.Overrides == nil {
		cfg.Overrides = map[string]string{}
	}
	cfg.Overrides[itemKey] = transform(cfg.Overrides[itemKey])

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("encode config: %w", err)
	}
	payload = append(payload, '\n')
	if _, werr := tmp.Write(payload); werr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp config: %w", werr)
	}
	if cerr := tmp.Close(); cerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp config: %w", cerr)
	}
	if rerr := os.Rename(tmpName, path); rerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic rename config: %w", rerr)
	}
	return nil
}

// setOverrideInFile performs an flock-serialized read-modify-write on the
// config file at path, setting overrides[key] = value. The file is written via
// a temp file + atomic rename to prevent partial reads by concurrent processes.
func setOverrideInFile(path, itemKey, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// Lock file is a sibling .lock file so the lock file survives atomic renames
	// of the config file itself.
	lockPath := path + ".lock"
	fl := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()

	locked, err := fl.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return fmt.Errorf("acquire config lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("timed out waiting for config file lock: %s", lockPath)
	}
	defer fl.Unlock() //nolint:errcheck

	// Read the existing file or start from empty Config.
	var cfg Config
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read config for update: %w", err)
	}
	if err == nil {
		if jerr := json.Unmarshal(raw, &cfg); jerr != nil {
			return fmt.Errorf("parse config for update: %w", jerr)
		}
	}

	// Modify.
	if cfg.Overrides == nil {
		cfg.Overrides = map[string]string{}
	}
	cfg.Overrides[itemKey] = value

	// Write to temp then rename.
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("encode config: %w", err)
	}
	payload = append(payload, '\n')
	if _, werr := tmp.Write(payload); werr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp config: %w", werr)
	}
	if cerr := tmp.Close(); cerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp config: %w", cerr)
	}
	if rerr := os.Rename(tmpName, path); rerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic rename config: %w", rerr)
	}
	return nil
}
