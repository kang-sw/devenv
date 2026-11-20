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
	// WorkerCapable admits agent.spawn on this backend. WorkerProfile is
	// the internal profile it launches; both must be set to expose tiers.
	WorkerCapable bool
	WorkerProfile string
	// WorkerHTTPMCP attaches a worker's ws tools through the daemon's
	// HTTP route rather than a stdio launcher. Workers never persist it.
	WorkerHTTPMCP bool
	// WorkerAutoApprove answers worker permission requests through the
	// ordinary approval path, retaining their trace. Leads still ask.
	WorkerAutoApprove bool
	// PermissionLevels advertises supported lead selection paths only.
	PermissionLevels []PermissionLevel
	// Catalog is the backend's capability catalog (catalog.go): a ceiling
	// over the instance handles, never a replacement for them. Nil keeps
	// the backend handle-only: no hiding, demotion, or version warning.
	Catalog *HarnessProfile
	// NativeCompaction supplies the compaction descriptor for a backend
	// not yet ported to the full catalog (Pi).
	NativeCompaction *CompactionProtocol
	// ToolNameScheme names how this backend's host must spell ws's dotted
	// tool names on a connection of its hosted lead or worker (ToolNameSchemeSanitize
	// below); "" keeps the dotted names. Common code reads it through
	// ApplyToolNameScheme and never names a backend.
	ToolNameScheme string
	// CallFallback exposes call and a tool index on hosted lead connections
	// whose client may hide tools: agy drops definitions over 1,500 bytes
	// including its 39-byte description prefix; other clients may cap count.
	CallFallback bool
	// DirectTools limits hosted lead discovery only; nil lists all tools.
	DirectTools []string
	// ToolsAlwaysLoad marks this connection's listed tools as always loaded.
	ToolsAlwaysLoad bool
	// ManualHarness is the harness name the hosted lead's workflow manual
	// renders under; "" renders with none.
	ManualHarness string
	// PlanUsageActivity marks a user-started lead call from this harness as
	// activity for the daemon's existing plan-usage refresh path. Common MCP
	// code resolves the flag through ManualHarness and never names a backend.
	PlanUsageActivity bool
	// PlaybookCommands marks a backend whose hosted sessions run without
	// ws's plugin skills and reach ws only as the ReservedMCPServer server.
	// Its hosted lead offers the package's lead playbooks as dashboard
	// commands (Options.PlaybookCommands), which the runner expands into
	// the playbook body (Options.ExpandCommand), and text ws renders for
	// its sessions names that server and those commands instead of the
	// package's tool and skill namespace.
	PlaybookCommands bool
}

// ToolNameSchemeSanitize spells a dotted ws tool name the way the Pi bridge
// registers it (agents-plugin-pi/src/bridge.ts sanitizeToolName): each "."
// flattened to "_", which fits the ^[a-zA-Z0-9_-]{1,128}$ names an Anthropic
// API tool accepts. The bridge's "ws__" prefix is not applied: it separates
// Pi's one flat tool namespace, while agy already prefixes a forwarded tool
// with its server name (mcp_ws_).
const ToolNameSchemeSanitize = "sanitize"

// ApplyToolNameScheme spells raw under scheme; ok is false for a scheme
// no one defines. The empty scheme is the identity.
func ApplyToolNameScheme(scheme, raw string) (name string, ok bool) {
	switch scheme {
	case "":
		return raw, true
	case ToolNameSchemeSanitize:
		return strings.ReplaceAll(raw, ".", "_"), true
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
		reg.DirectTools = append([]string(nil), reg.DirectTools...)
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
	case BackendCodex:
		return CodexPermissionModes
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
