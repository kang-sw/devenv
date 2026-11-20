package wsagent

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// PermissionLevel is common intent, independent of native modes and capability
// allowlists. It is not a sandbox guarantee.
type PermissionLevel string

const (
	PermissionReadOnly     PermissionLevel = "read-only"
	PermissionWorkdirWrite PermissionLevel = "workdir-write"
	PermissionFull         PermissionLevel = "full"
)

// LeadPermissionLevels returns the ordered common intent choices. Registration
// advertises these for supported lead paths only; workers do not consume them.
func LeadPermissionLevels() []PermissionLevel {
	return []PermissionLevel{PermissionReadOnly, PermissionWorkdirWrite, PermissionFull}
}

func (level PermissionLevel) rank() int {
	switch level {
	case PermissionReadOnly:
		return 1
	case PermissionWorkdirWrite:
		return 2
	case PermissionFull:
		return 3
	}
	return 0
}

func nativePermissionMode(backend string, level PermissionLevel) string {
	if backend != BackendClaude {
		return ""
	}
	switch level {
	case PermissionReadOnly:
		return "plan"
	case PermissionWorkdirWrite:
		return "acceptEdits"
	case PermissionFull:
		return "bypassPermissions"
	}
	return ""
}

func legacyLeadPermission(rec Record) PermissionLevel {
	if rec.Backend == BackendPi {
		return PermissionWorkdirWrite
	}
	if rec.Backend != BackendClaude {
		return ""
	}
	switch rec.PermissionMode {
	case "plan":
		return PermissionReadOnly
	case "bypassPermissions":
		return PermissionFull
	case "", "auto", "default", "acceptEdits", "dontAsk":
		return PermissionWorkdirWrite
	}
	return ""
}

func permissionQuality(backend string, level PermissionLevel, active string) string {
	if backend == BackendClaude && level == PermissionReadOnly && active == "plan" {
		return "partial"
	}
	return "advisory"
}

// user is supplied by the trusted control owner, never decoded from JSON. The
// acknowledgement is necessary for explicit full selection, not user authority.
func authorizePermission(current, next PermissionLevel, user, ack bool) error {
	if next.rank() == 0 {
		return badRequest("permission_level must be read-only, workdir-write or full")
	}
	if next.rank() > current.rank() && !user {
		return &requestError{status: http.StatusForbidden, msg: "only the user may raise permission_level"}
	}
	if next == PermissionFull && (!user || !ack) {
		return &requestError{status: http.StatusForbidden, msg: "full permission selection requires the user and permission_full_ack after a warning"}
	}
	return nil
}

func (r *Runner) supportsLeadPermission(backend string, level PermissionLevel) bool {
	reg := r.opts.Backends[backend]
	return reg.Backend != nil && reg.LeadProfile != "" && (backend == BackendClaude || backend == BackendPi) && slices.Contains(reg.PermissionLevels, level)
}

func (r *Runner) leadPermission(rec Record, dir string) (PermissionLevel, string, error) {
	level, explicit, err := refreshPermissionSelection(dir)
	return r.resolveLeadPermission(rec, level, explicit, err)
}

func (r *Runner) resolveLeadPermission(rec Record, level PermissionLevel, explicit bool, err error) (PermissionLevel, string, error) {
	if err != nil {
		return "", "", err
	}
	if explicit {
		if !r.supportsLeadPermission(rec.Backend, level) {
			return "", "", badRequest("common lead permissions are not supported on %s", rec.Backend)
		}
		return level, "explicit", nil
	}
	return legacyLeadPermission(rec), "legacy-derived", nil
}

func resolveWorkerPermission(def Profile, level PermissionLevel, explicit bool, err error) (PermissionLevel, string, error) {
	if err != nil {
		return "", "", err
	}
	if explicit {
		return level, "explicit", nil
	}
	return legacyWorkerPermission(def), "legacy-derived", nil
}

// workerPermission reads this worker's durable selection, or derives the
// legacy level from the definition that will actually launch. Derived levels
// are never written back.
func (r *Runner) workerPermission(rec Record, def Profile, dir string) (PermissionLevel, string, error) {
	level, explicit, err := refreshPermissionSelection(dir)
	return resolveWorkerPermission(def, level, explicit, err)
}

// currentWorkerPermission resolves a parent conversation at admission time.
// Profile edits may change a legacy-derived level, so a launch never reuses a
// stored definition as the parent's current authority.
func (r *Runner) currentWorkerPermission(rec Record) (PermissionLevel, string, error) {
	dir := wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID)
	level, explicit, err := refreshPermissionSelection(dir)
	if err != nil || explicit {
		return resolveWorkerPermission(Profile{}, level, explicit, err)
	}
	rp, err := r.resolve(rec.Profile, rec.Root)
	if err != nil {
		return "", "", err
	}
	if rp.Definition.Backend != rec.Backend || rp.Definition.Lead {
		return "", "", fmt.Errorf("agent profile %q no longer resolves to this worker's %s backend", rec.Profile, rec.Backend)
	}
	return resolveWorkerPermission(rp.Definition, "", false, nil)
}

// permissionState is computed even for closed rows; cached records must not
// cache old enforcement evidence. Selection is cached separately and invalidated
// by permission writes.
func (r *Runner) permissionState(st State, rec Record, dir string) State {
	selected, explicit, readErr := cachedPermissionSelection(dir)
	var level PermissionLevel
	var source string
	var err error
	if rec.Lead {
		level, source, err = r.resolveLeadPermission(rec, selected, explicit, readErr)
	} else if explicit || readErr != nil {
		level, source, err = resolveWorkerPermission(Profile{}, selected, explicit, readErr)
	} else if st.PermissionLevel != "" {
		level, source = st.PermissionLevel, st.PermissionSource
	} else if rec.ResolvedDefinition != nil {
		level, source = legacyWorkerPermission(*rec.ResolvedDefinition), "legacy-derived"
	}
	if err != nil {
		st.Error = appendDiagnostic(st.Error, err.Error())
		return st
	}
	if level == "" {
		return st
	}
	st.PermissionLevel, st.PermissionSource = level, source
	active := ""
	if rec.Lead && st.Process {
		active = st.ActiveMode
	}
	st.PermissionEnforcement = permissionQuality(rec.Backend, level, active)
	return st
}

func appendDiagnostic(existing, diagnostic string) string {
	if existing == "" {
		return diagnostic
	}
	if existing == diagnostic || slices.Contains(strings.Split(existing, "; "), diagnostic) {
		return existing
	}
	return existing + "; " + diagnostic
}

// A worker projection is used only for the diagnostic. Unknown surfaces remain
// unresolved and never become an authority ceiling or applied selection.
func legacyWorkerPermission(def Profile) PermissionLevel {
	if slices.Equal(def.Tools, PiWorkerTools) {
		return PermissionWorkdirWrite
	}
	if len(def.MCPServers) != 0 || len(def.Plugins) != 0 {
		return ""
	}
	for _, tool := range def.Tools {
		switch strings.ToLower(ToolBase(tool)) {
		case "read", "grep", "glob", "find", "ls":
		case "bash", "edit", "write":
			return PermissionWorkdirWrite
		default:
			return ""
		}
	}
	for _, tool := range def.WSTools {
		switch tool {
		case "git.status", "git.diff", "git.log", "git.merge_base":
		default:
			return ""
		}
	}
	return PermissionReadOnly
}

func (c *conv) permissionNudge(level PermissionLevel, worker bool, active string) {
	rec := c.record()
	quality := permissionQuality(rec.Backend, level, active)
	limit := "filesystem, network and other tool effects are not proven confined"
	if active == "" && rec.Backend == BackendClaude {
		limit = "native mode observation is pending; confinement is unproved"
	}
	if worker {
		if level == "" {
			limit = "common worker authority is unresolved; the capability loadout is not confinement evidence"
		} else {
			limit = "current-parent admission passed; the capability loadout and harness controls do not prove confinement"
		}
	}
	label := string(level)
	if label == "" {
		label = "unresolved"
	}
	fmt.Fprintf(c.r.opts.Log, "agent %s: Permission %s is %s on %s: %s.\n", rec.ReplyID, label, quality, rec.Backend, limit)
}

// SetPermissionLevel is the authorized agent-lowering seam. It cannot elevate a
// request by supplying an acknowledgement; only trusted HTTP control owns user.
func (r *Runner) SetPermissionLevel(id string, level PermissionLevel) (State, error) {
	return r.act(id, func(c *conv) error { return c.setPermissionLevel(level, false, false) })
}

func (c *conv) setPermissionLevel(level PermissionLevel, user, ack bool) error {
	rec := c.record()
	if rec.Lead && !c.r.supportsLeadPermission(rec.Backend, level) {
		return badRequest("permission_level %q is not supported on %s", level, rec.Backend)
	}
	var current PermissionLevel
	var err error
	if rec.Lead {
		current, _, err = c.r.leadPermission(rec, c.dir)
	} else {
		rp, resolveErr := c.r.resolve(rec.Profile, rec.Root)
		if resolveErr != nil {
			return resolveErr
		}
		if rp.Definition.Backend != rec.Backend || rp.Definition.Lead {
			return badRequest("agent profile %q no longer resolves to this worker's %s backend", rec.Profile, rec.Backend)
		}
		current, _, err = c.r.workerPermission(rec, rp.Definition, c.dir)
	}
	if err != nil {
		return err
	}
	if current.rank() == 0 {
		return badRequest("the worker's current permission level is unresolved")
	}
	if err := authorizePermission(current, level, user, ack); err != nil {
		return err
	}
	if !rec.Lead && rec.Parent != nil {
		ceiling, err := c.r.agentCeiling(*rec.Parent)
		if err != nil {
			return badRequest("permission admission: %v", err)
		}
		if level.rank() > ceiling.rank() {
			return &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("permission admission: authority %s is above the parent's current level %s", level, ceiling)}
		}
	}
	if err := writePermissionSelection(c.dir, level); err != nil {
		return err
	}
	if rec.Lead {
		if mode := nativePermissionMode(rec.Backend, level); mode != "" {
			c.update(func(rec *Record) { rec.PermissionMode = mode })
			c.dirtyMode, c.reconcileMode = true, false
		}
	} else {
		c.setStatus(func() {
			c.workerPermissionLevel = level
			c.workerPermissionSource = "explicit"
		})
	}
	if !c.turn {
		c.applySettings()
	}
	c.permissionNudge(level, !rec.Lead, "")
	return nil
}

func (c *conv) setNativePermissionMode(mode string, user bool) error {
	rec := c.record()
	if !rec.Lead || len(c.r.opts.Backends[rec.Backend].PermissionModes) == 0 {
		return categorized(ErrUnsupported, badRequest("the permission mode applies to a claude lead session only"))
	}
	if rec.Backend == BackendAgy && mode == "bypass" && !rec.agyBypass() && !user {
		return &requestError{status: http.StatusForbidden, msg: "only the user may enable agy bypass through permission_mode"}
	}
	// Extension/native-only registrations retain their historical mode contract.
	if c.r.supportsLeadPermission(rec.Backend, PermissionReadOnly) {
		current, source, err := c.r.leadPermission(rec, c.dir)
		if err != nil {
			return err
		}
		next := legacyLeadPermission(Record{Lead: true, Backend: rec.Backend, PermissionMode: mode})
		if next == "" {
			return badRequest("native permission mode has no common intent mapping")
		}
		if next.rank() > current.rank() && !user {
			return &requestError{status: http.StatusForbidden, msg: "only the user may raise permission_level through permission_mode"}
		}
		if source == "explicit" && next != current {
			// A native-only legacy URL cannot carry a confirmed explicit full
			// selection. Keep old calls unchanged when no common choice exists.
			if err := authorizePermission(current, next, user, false); err != nil {
				return err
			}
			if err := writePermissionSelection(c.dir, next); err != nil {
				return err
			}
		}
	}
	if mode != rec.selectedPermissionMode() {
		c.update(func(rec *Record) { rec.selectPermissionMode(mode) })
		c.dirtyMode, c.reconcileMode = true, false
		if !c.turn {
			c.applySettings()
		}
		c.allowAgyBypassPending()
	}
	return nil
}

// A holder attach ignores launch flags. After replay, reconstruct delivery from
// the durable selection/native target instead of trusting a lost dirty flag.
// Replay observations remain evidence, never an acknowledgement of that target.
func (c *conv) reconcileLeadPermission() {
	// The loop owns permission state, but Send updates the shared record
	// concurrently. Copy it under mu before passing it by value.
	rec := c.record()
	if !rec.Lead || rec.Backend != BackendClaude || c.dirtyMode {
		return
	}
	level, source, err := c.r.leadPermission(rec, c.dir)
	if err != nil {
		// State projects the current selection error without retaining history
		// in the terminal error slot or letting it suppress a process exit.
		return
	}
	if source != "explicit" {
		return
	}
	target := rec.PermissionMode
	if legacyLeadPermission(rec) != level {
		target = nativePermissionMode(rec.Backend, level)
	}
	if target == c.activeMode {
		return
	}
	if target != rec.PermissionMode {
		c.update(func(rec *Record) { rec.PermissionMode = target })
	}
	c.dirtyMode, c.reconcileMode = true, true
}
