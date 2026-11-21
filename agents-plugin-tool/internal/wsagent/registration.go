package wsagent

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
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
	// PlaybookCommands marks a backend whose hosted sessions run without
	// ws's plugin skills and reach ws only as the ReservedMCPServer server.
	// Its hosted lead offers the package's lead playbooks as dashboard
	// commands (Options.PlaybookCommands), which the runner expands into
	// the playbook body (Options.ExpandCommand), and text ws renders for
	// its sessions names that server and those commands instead of the
	// package's tool and skill namespace.
	PlaybookCommands bool
	// Profiles are this backend's runtime-owned builtin profile definitions.
	// They feed the config floor but are not fields of the persisted Profile
	// shape. Support marks the profiles offered by tracker support, in order.
	Profiles []BuiltinProfileRegistration
	// PlaybookTerminology supplies static harness wording for playbook renders.
	PlaybookTerminology PlaybookTerminology
}

// PlaybookTerminology holds static native-tool idioms. Model, effort, and
// namespace-dependent bindings are resolved when a playbook is rendered.
type PlaybookTerminology struct {
	ExploreAgent, SpawnIdiom, ContinueIdiom string
}

// BuiltinProfileRegistration is one builtin profile owned by a backend's
// registration. Definition is serialized into the existing Profile shape;
// Support is runtime-only descriptor data.
type BuiltinProfileRegistration struct {
	Name       string
	Definition Profile
	Support    bool
}

type namedBackendRegistration struct {
	name string
	reg  BackendRegistration
}

// builtinRegistrations is the immutable descriptor floor. Its order preserves
// the advertised config selectors and tracker-support preference.
var builtinRegistrations = sync.OnceValue(func() []namedBackendRegistration {
	backends := []registrationBackend{ClaudeBackend{}, CodexBackend{}, PiBackend{}, AgyBackend{}}
	out := make([]namedBackendRegistration, 0, len(backends))
	for _, backend := range backends {
		name, reg := backend.registration()
		out = append(out, namedBackendRegistration{name: name, reg: cloneBackendRegistration(reg)})
	}
	return out
})

func builtinRegistration(name string) BackendRegistration {
	for _, value := range builtinRegistrations() {
		if value.name == name {
			return value.reg
		}
	}
	return BackendRegistration{}
}

// BuiltinHarnessNames returns the built-in harness names in config listing
// order. Callers can append their selector or storage sentinel to the result.
func BuiltinHarnessNames() []string {
	values := builtinRegistrations()
	names := make([]string, 0, len(values))
	for _, value := range values {
		names = append(names, value.name)
	}
	return names
}

// BuiltinPlaybookTerminology returns the built-in static bindings, or zero
// values when the harness uses the host-neutral wording.
func BuiltinPlaybookTerminology(harness string) PlaybookTerminology {
	return builtinRegistration(harness).PlaybookTerminology
}

// BuiltinBackendRegistrations returns fresh runtime descriptors for every
// built-in harness. Callers may mutate the returned map and values.
func BuiltinBackendRegistrations() map[string]BackendRegistration {
	values := builtinRegistrations()
	out := make(map[string]BackendRegistration, len(values))
	for _, value := range values {
		reg := cloneBackendRegistration(value.reg)
		if reg.Catalog != nil {
			catalog := *reg.Catalog
			catalog.Entries = slices.Clone(catalog.Entries)
			for i := range catalog.Entries {
				catalog.Entries[i].Values = slices.Clone(catalog.Entries[i].Values)
			}
			reg.Catalog = &catalog
		}
		if reg.NativeCompaction != nil {
			protocol := *reg.NativeCompaction
			reg.NativeCompaction = &protocol
		}
		out[value.name] = reg
	}
	return out
}

func cloneProfile(p Profile) Profile {
	p.Tools = slices.Clone(p.Tools)
	p.WSTools = slices.Clone(p.WSTools)
	p.Plugins = slices.Clone(p.Plugins)
	if p.MCPServers != nil {
		p.MCPServers = maps.Clone(p.MCPServers)
		for name, raw := range p.MCPServers {
			p.MCPServers[name] = slices.Clone(raw)
		}
	}
	if p.IdleTimeoutSeconds != nil {
		seconds := *p.IdleTimeoutSeconds
		p.IdleTimeoutSeconds = &seconds
	}
	return p
}

func cloneBuiltinProfiles(in []BuiltinProfileRegistration) []BuiltinProfileRegistration {
	out := slices.Clone(in)
	for i := range out {
		out[i].Definition = cloneProfile(out[i].Definition)
	}
	return out
}

func cloneBackendRegistration(reg BackendRegistration) BackendRegistration {
	reg.PermissionModes = slices.Clone(reg.PermissionModes)
	reg.PermissionLevels = slices.Clone(reg.PermissionLevels)
	reg.DirectTools = slices.Clone(reg.DirectTools)
	reg.Profiles = cloneBuiltinProfiles(reg.Profiles)
	return reg
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
		out[name] = cloneBackendRegistration(reg)
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
		names = BuiltinHarnessNames()
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
	return builtinRegistration(backend).PermissionModes
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
