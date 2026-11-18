package wsagent

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// BackendRegistration is the admission descriptor of one registered backend.
// Optional process controls remain instance-owned (controlsFor), not promises
// made by a backend-wide flag. ChoiceLister is asserted on Backend itself.
// This descriptor is runtime-only: it adds nothing to persisted profiles.
type BackendRegistration struct {
	Backend         Backend
	LeadProfile     string
	PermissionModes []string
	// PermissionLevels advertises supported lead selection paths only.
	PermissionLevels []PermissionLevel
	// Catalog is the backend's capability catalog (catalog.go): a ceiling
	// over the instance handles, never a replacement for them. Nil keeps
	// the backend handle-only: no hiding, demotion, or version warning.
	Catalog *HarnessProfile
	// ToolNameScheme names how this backend's host must spell ws's dotted
	// tool names on a connection of its hosted lead (ToolNameSchemeSanitize
	// below); "" keeps the dotted names. Common code reads it through
	// ApplyToolNameScheme and never names a backend.
	ToolNameScheme string
	// ManualHarness is the harness name the hosted lead's workflow manual
	// renders under; "" renders with none.
	ManualHarness string
}

// ToolNameSchemeSanitize spells a dotted ws tool name the way the Pi bridge
// registers it (agents-plugin-pi/src/bridge.ts sanitizeToolName): a "ws__"
// prefix and each "." flattened to "_", which fits the
// ^[a-zA-Z0-9_-]{1,128}$ names an Anthropic API tool accepts.
const ToolNameSchemeSanitize = "sanitize"

// ApplyToolNameScheme spells raw under scheme; ok is false for a scheme
// no one defines. The empty scheme is the identity.
func ApplyToolNameScheme(scheme, raw string) (name string, ok bool) {
	switch scheme {
	case "":
		return raw, true
	case ToolNameSchemeSanitize:
		return "ws__" + strings.ReplaceAll(raw, ".", "_"), true
	}
	return "", false
}

// BackendRegistrations exposes the runner's admitted set to in-process config
// readers. The returned map and slices are copies; backend instances and
// catalogs are shared and read-only.
func (r *Runner) BackendRegistrations() map[string]BackendRegistration {
	out := make(map[string]BackendRegistration, len(r.opts.Backends))
	for name, reg := range r.opts.Backends {
		reg.PermissionModes = append([]string(nil), reg.PermissionModes...)
		reg.PermissionLevels = append([]PermissionLevel(nil), reg.PermissionLevels...)
		out[name] = reg
	}
	return out
}

// leadHarnesses are the registered backends with a lead profile, sorted.
func (r *Runner) leadHarnesses() []string {
	var names []string
	for name, reg := range r.opts.Backends {
		if reg.LeadProfile != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// profileBackends defaults only keyless/standalone profile readers to the
// builtin profile floor. Actual daemon admission always supplies its registry;
// this is not a second registry of process implementations or descriptors.
func profileBackends(registries []map[string]BackendRegistration) []string {
	var names []string
	if len(registries) > 0 {
		for name := range registries[0] {
			names = append(names, name)
		}
	} else {
		seen := map[string]bool{}
		for _, raw := range BuiltinProfiles() {
			var p Profile
			if json.Unmarshal(raw, &p) == nil && !seen[p.Backend] {
				seen[p.Backend] = true
				names = append(names, p.Backend)
			}
		}
	}
	sort.Strings(names)
	return names
}

// permissionModes preserves legacy Pi values accepted on reads and ignored at
// launch. Registered extensions use their own modes, not the Pi fallback.
func permissionModes(backend string, registries []map[string]BackendRegistration) []string {
	if backend == BackendPi {
		return LeadPermissionModes
	}
	if len(registries) > 0 {
		return registries[0][backend].PermissionModes
	}
	switch backend {
	case BackendClaude:
		return LeadPermissionModes
	case BackendAgy:
		return AgyPermissionModes
	}
	return nil
}

func profilePermissionModes(registries []map[string]BackendRegistration) []string {
	modes := []string{}
	seen := map[string]bool{}
	for _, name := range profileBackends(registries) {
		choices := permissionModes(name, registries)
		for _, mode := range choices {
			if !seen[mode] {
				seen[mode] = true
				modes = append(modes, mode)
			}
		}
	}
	return modes
}

func backendAlternatives(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		raw, _ := json.Marshal(name)
		quoted[i] = string(raw)
	}
	return strings.Join(quoted, " or ")
}

var (
	ErrBackendUnavailable = errors.New("backend unavailable")
	ErrInvalidProfile     = errors.New("invalid profile")
	ErrLaunchFailed       = errors.New("launch failed")
)

// categorizedError adds an internal classification without changing the
// original text, requestError status, or underlying error identity.
type categorizedError struct{ kind, cause error }

func (e *categorizedError) Error() string        { return e.cause.Error() }
func (e *categorizedError) Unwrap() error        { return e.cause }
func (e *categorizedError) Is(target error) bool { return target == e.kind }
func categorized(kind, cause error) error {
	if cause == nil {
		return nil
	}
	return &categorizedError{kind: kind, cause: cause}
}

// AdmitsLead is used by in-process consumers such as the hiring gate.
func (r *Runner) AdmitsLead(backend string) bool {
	return LeadProfileFor(backend, r.opts.Backends) != ""
}
