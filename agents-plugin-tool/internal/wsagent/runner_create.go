// runner_create.go holds conversation creation and backend resolution.
package wsagent

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
)

type requestError struct {
	status int
	msg    string
}

func (e *requestError) Error() string { return e.msg }

func badRequest(format string, args ...any) error {
	return &requestError{status: http.StatusBadRequest, msg: fmt.Sprintf(format, args...)}
}

var errUnknown = &requestError{status: http.StatusNotFound, msg: "unknown agent conversation"}
var errClosed = &requestError{status: http.StatusConflict, msg: "the conversation is closed"}
var errHandedOff = &requestError{status: http.StatusConflict, msg: "the session was handed to a terminal: it runs there, not in the dashboard"}

// endedError is the refusal of a write to a conversation no loop serves.
func endedError(rec Record) error {
	if rec.ClosedAt == "" && rec.handedOff() {
		return errHandedOff
	}
	return errClosed
}

// LeadOptions are a lead conversation's spawn choices; an empty one takes
// the profile's value (the harness default when it has none).
type LeadOptions struct {
	// HireRequestID and HireLabel are set only by CreateHireLead, never HTTP.
	HireRequestID   string
	HireLabel       string
	Model           string
	Effort          string
	PermissionMode  string
	PluginIsolation string
	// Common intent is transient; selection is stored separately from Record.
	PermissionLevel   PermissionLevel
	PermissionFullAck bool
	permissionUser    bool // set by the trusted control owner, never request JSON
}

// leadPluginIsolation supplies the additive field's legacy default.
func leadPluginIsolation(value string) string {
	if value == "" {
		return "default"
	}
	return value
}

// Create opens a conversation of profile bound to root. It resolves and
// checks the profile (backend, instruction file) now, so a bad launch fails
// here; the process itself starts on the first send. A lead profile is
// refused: its conversations open through CreateLead, behind the trusted
// gate.
func (r *Runner) Create(profile, root string) (State, error) {
	return r.create(profile, root, nil, nil)
}

// CreateLead opens a hosted lead session of harness backend on root, with
// the builtin lead profile of that harness (or the user's definition of the
// same name). The caller has passed the trusted gate.
func (r *Runner) CreateLead(backend, root string, opts LeadOptions) (State, error) {
	profile := LeadProfileFor(strings.TrimSpace(backend), r.opts.Backends)
	if profile == "" {
		return State{}, categorized(ErrInvalidProfile, badRequest("harness must be one of %s; got %q", strings.Join(r.leadHarnesses(), ", "), backend))
	}
	if opts.Model != "" && !modelPattern.MatchString(opts.Model) {
		return State{}, categorized(ErrInvalidProfile, badRequest("model %q must match %s", opts.Model, modelPattern.String()))
	}
	if opts.Effort != "" && !effortPattern.MatchString(opts.Effort) {
		return State{}, categorized(ErrInvalidProfile, badRequest("effort %q must be a lowercase word", opts.Effort))
	}
	modes := permissionModes(strings.TrimSpace(backend), []map[string]BackendRegistration{r.opts.Backends})
	if opts.PermissionMode != "" && !slices.Contains(modes, opts.PermissionMode) {
		return State{}, categorized(ErrInvalidProfile, badRequest("permission_mode %q must be one of %s", opts.PermissionMode, strings.Join(modes, ", ")))
	}
	opts.PluginIsolation = leadPluginIsolation(opts.PluginIsolation)
	if opts.PluginIsolation != "default" && opts.PluginIsolation != "isolated" {
		return State{}, categorized(ErrInvalidProfile, badRequest("plugin_isolation must be default or isolated; got %q", opts.PluginIsolation))
	}
	return r.create(profile, root, &opts, nil)
}

// create opens a conversation; lead is a lead's spawn choices, spawn a
// spawned conversation's (Runner.Spawn), at most one of them set.
func (r *Runner) create(profile, root string, lead *LeadOptions, spawn *SpawnSpec) (State, error) {
	root = strings.TrimSpace(root)
	if !filepath.IsAbs(root) {
		return State{}, badRequest("root must be an absolute path; got %q", root)
	}
	root = filepath.Clean(root)
	if info, err := wsstate.Stat(root); err != nil || !info.IsDir() {
		return State{}, badRequest("root %s is not a directory", root)
	}
	rp, err := r.resolve(profile, root)
	if err != nil {
		return State{}, err
	}
	if rp.Definition.Lead != (lead != nil) {
		if lead == nil {
			return State{}, &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("agent profile %q is a lead profile: a lead session is spawned from the dashboard on this machine", rp.Name)}
		}
		return State{}, badRequest("agent profile %q is not a lead profile (lead: true)", rp.Name)
	}
	if spawn != nil {
		releaseAdmission := r.holdParentAdmission(spawn.Parent)
		defer releaseAdmission()
		if spawn.Tier != "" && rp.Definition.Backend != BackendPi && !isAgentWorkerProfile(rp.Name) {
			return State{}, badRequest("pi.spawn: profile %q must be a non-lead Pi profile", rp.Name)
		}
		if err := r.checkChildProfile(spawn.Parent, rp.Name, rp.Definition); err != nil {
			return State{}, err
		}
		level, source, err := resolveWorkerPermission(rp.Definition, spawn.PermissionLevel, spawn.PermissionLevel != "", nil)
		if err != nil {
			return State{}, err
		}
		level = r.legacyDetachedWorkerLevel(spawn.Parent, level, source)
		if spawn.PermissionLevel != "" && !spawn.delegatedAuthority {
			current := legacyWorkerPermission(rp.Definition)
			if current.rank() == 0 {
				return State{}, badRequest("the worker's current permission level is unresolved")
			}
			if err := authorizePermission(current, level, spawn.permissionUser, spawn.PermissionFullAck); err != nil {
				return State{}, err
			}
		}
		if err := r.admitWorkerLevel(spawn.Parent, level, spawn.delegatedAuthority || source == "legacy-derived"); err != nil {
			return State{}, err
		}
	}
	if lead != nil && lead.PermissionLevel != "" {
		base := Record{Lead: true, Backend: rp.Definition.Backend, PermissionMode: rp.Definition.PermissionMode}
		if lead.PermissionMode != "" {
			base.PermissionMode = lead.PermissionMode
		}
		if !r.supportsLeadPermission(base.Backend, lead.PermissionLevel) {
			return State{}, badRequest("permission_level %q is not supported on %s", lead.PermissionLevel, base.Backend)
		}
		if err := authorizePermission(legacyLeadPermission(base), lead.PermissionLevel, lead.permissionUser, lead.PermissionFullAck); err != nil {
			return State{}, err
		}
	}
	var key string
	if lead != nil {
		// A lead key lives at the canonical git root, which is then the
		// session's root: its registration and session record must agree.
		if root, err = r.opts.LeadKeys.LeadRoot(root); err != nil {
			return State{}, badRequest("a lead session needs a git worktree: %v", err)
		}
		if key, err = r.opts.LeadKeys.MintLead(root); err != nil {
			return State{}, fmt.Errorf("mint the lead session key: %w", err)
		}
	} else {
		keyRoot := root
		if spawn != nil && rp.Definition.Backend == BackendPi && spawn.Cwd != "" {
			keyRoot = spawn.Cwd
		}
		if key, err = r.opts.Keys.Mint(keyRoot); err != nil {
			return State{}, fmt.Errorf("mint ws session key: %w", err)
		}
	}
	now := r.opts.Now()
	rec := Record{
		ReplyID:   wsmailbox.ReplyID(key),
		Profile:   rp.Name,
		Backend:   rp.Definition.Backend,
		Root:      root,
		Model:     rp.Definition.Model,
		Effort:    rp.Definition.Effort,
		WSKey:     key,
		Listed:    rp.Definition.ListInIndex,
		CreatedAt: now.UTC().Format(time.RFC3339),
	}
	if lead != nil {
		rec.Lead = true
		rec.PluginIsolation = leadPluginIsolation(lead.PluginIsolation)
		rec.PermissionMode = rp.Definition.PermissionMode
		if lead.Model != "" {
			rec.Model = lead.Model
		}
		if lead.Effort != "" {
			rec.Effort = lead.Effort
		}
		if lead.PermissionMode != "" {
			rec.PermissionMode = lead.PermissionMode
		}
		if len(r.opts.Backends[rec.Backend].PermissionModes) == 0 {
			rec.PermissionMode = ""
		}
		rec.selectPermissionMode(rec.PermissionMode)
	}
	if spawn != nil {
		spawn.apply(&rec)
	}
	if !wsdashboard.ValidReplyID(rec.ReplyID) {
		return State{}, fmt.Errorf("minted key gives an invalid reply-id %q", rec.ReplyID)
	}
	dir := wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return State{}, err
	}
	if lead != nil && lead.PermissionLevel != "" {
		if err := writePermissionSelection(dir, lead.PermissionLevel); err != nil {
			return State{}, err
		}
		if mode := nativePermissionMode(rec.Backend, lead.PermissionLevel); mode != "" {
			rec.PermissionMode = mode
		}
	}
	if spawn != nil && spawn.PermissionLevel != "" {
		writer := writeGenericPermissionSelection
		if spawn.delegatedAuthority {
			writer = writeDelegatedPermissionSelection
		}
		if err := writer(dir, spawn.PermissionLevel); err != nil {
			return State{}, err
		}
	}
	if spawn != nil && spawn.SystemPrompt != "" {
		if err := writeFileAtomic(filepath.Join(dir, systemPromptFile), []byte(spawn.SystemPrompt), 0o600); err != nil {
			return State{}, err
		}
	}
	if lead != nil && lead.HireRequestID != "" {
		if err := wsdashboard.MarkHired(r.opts.Dir, rec.ReplyID, lead.HireRequestID, lead.HireLabel); err != nil {
			return State{}, err
		}
	}
	if err := writeRecord(dir, rec); err != nil {
		return State{}, err
	}
	var seed []wslive.SlashCommand
	if rec.Lead {
		// A new lead offers its playbook commands before its process
		// first reports the harness's own.
		seed = r.withPlaybookCommands(rec, r.lastCommands(rec))
	}
	var claimWidth func()
	if spawn != nil {
		claimWidth = spawn.claimWidth
	}
	c := r.startConvWith(rec, seed, claimWidth != nil, claimWidth, false)
	if spawn != nil {
		return r.spawned(c, spawn.Prompt)
	}
	return c.state(), nil
}

// lastCommands is the command list in the persisted snapshot of the
// newest other hosted lead session of rec's root and profile that has
// one; nil when none has. A new lead's process starts only at its first
// send, and the composer offers commands only from the snapshot's list,
// so without this an empty session would offer none until then.
func (r *Runner) lastCommands(rec Record) []wslive.SlashCommand {
	r.mu.Lock()
	recs := make([]Record, 0, len(r.convs)+len(r.closed))
	convs := make([]*conv, 0, len(r.convs))
	for _, c := range r.convs {
		convs = append(convs, c)
	}
	for _, prev := range r.closed {
		recs = append(recs, prev)
	}
	r.mu.Unlock()
	for _, c := range convs {
		recs = append(recs, c.record())
	}
	recs = slices.DeleteFunc(recs, func(prev Record) bool {
		return !prev.Lead || prev.ReplyID == rec.ReplyID || prev.Root != rec.Root || prev.Profile != rec.Profile || prev.Backend != rec.Backend
	})
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].CreatedAt != recs[j].CreatedAt {
			return recs[i].CreatedAt > recs[j].CreatedAt
		}
		return recs[i].ReplyID > recs[j].ReplyID
	})
	for _, prev := range recs {
		snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(wsdashboard.SessionDir(r.opts.Dir, prev.ReplyID)))
		if err == nil && len(snap.Commands) > 0 {
			return snap.Commands
		}
	}
	return nil
}

// resolve resolves profile through root and checks everything a launch
// needs: a backend this daemon has, an existing instruction file, and a
// ws-mcp command when the profile allows ws tools.
func (r *Runner) resolve(profile, root string) (ResolvedProfile, error) {
	var rp ResolvedProfile
	var err error
	if r.fixedProfile != nil {
		if profile != r.fixedProfile.Name {
			return ResolvedProfile{}, badRequest("private answer runner profile mismatch")
		}
		rp = *r.fixedProfile
	} else {
		rp, err = ResolveProfile(r.opts.Config, root, strings.TrimSpace(profile), r.opts.Backends)
	}
	if err != nil {
		return ResolvedProfile{}, categorized(ErrInvalidProfile, badRequest("%v", err))
	}
	if _, err := r.backend(rp.Name, rp.Definition.Backend); err != nil {
		return ResolvedProfile{}, err
	}
	if path := rp.Definition.InstructionsPath(root); path != "" {
		info, err := wsstate.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return ResolvedProfile{}, categorized(ErrInvalidProfile, badRequest("agent profile %q: instruction file %s does not exist", rp.Name, path))
		}
	}
	if len(rp.Definition.WSTools) > 0 && r.opts.WSMCP == nil {
		return ResolvedProfile{}, categorized(ErrInvalidProfile, badRequest("agent profile %q allows ws tools, but this daemon cannot attach ws-mcp", rp.Name))
	}
	if !rp.Definition.Lead && r.opts.WSMCP != nil && !slices.Contains(rp.Definition.WSTools, "compact") {
		rp.Definition.WSTools = append(slices.Clone(rp.Definition.WSTools), "compact")
	}
	if rp.Definition.Lead && (r.opts.Holders == nil || r.opts.LeadKeys == nil) {
		return ResolvedProfile{}, categorized(ErrInvalidProfile, badRequest("agent profile %q is a lead profile, but this daemon runs no session holders", rp.Name))
	}
	return rp, nil
}

// backend is shared by profile validation and both launch paths. Keep
// unavailable backends explicit, including when rejoining a held process.
func (r *Runner) backend(profile, name string) (Backend, error) {
	backend := r.opts.Backends[name].Backend
	if backend == nil {
		return nil, categorized(ErrBackendUnavailable, badRequest("agent profile %q: backend %q is not available in this daemon", profile, name))
	}
	return backend, nil
}

func (c *conv) startBackend(spec StartSpec) (Process, error) {
	backend, err := c.r.backend(c.rec.Profile, c.rec.Backend)
	if err != nil {
		return nil, err
	}
	proc, err := backend.Start(context.Background(), spec)
	return proc, categorized(ErrLaunchFailed, err)
}

// Get reads one conversation's state.
