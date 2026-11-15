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
}

// BackendRegistrations exposes the runner's admitted set to in-process config
// readers. The returned map and slices are copies; backend instances are shared.
func (r *Runner) BackendRegistrations() map[string]BackendRegistration {
	out := make(map[string]BackendRegistration, len(r.opts.Backends))
	for name, reg := range r.opts.Backends {
		reg.PermissionModes = append([]string(nil), reg.PermissionModes...)
		reg.PermissionLevels = append([]PermissionLevel(nil), reg.PermissionLevels...)
		out[name] = reg
	}
	return out
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
	if backend == BackendClaude {
		return LeadPermissionModes
	}
	return nil
}

func profilePermissionModes(registries []map[string]BackendRegistration) []string {
	if len(registries) == 0 {
		return LeadPermissionModes
	}
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
