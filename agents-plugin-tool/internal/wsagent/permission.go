package wsagent

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
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

// permissionState is computed even for closed rows; cached records must not
// cache old enforcement evidence. Selection is cached separately and invalidated
// by permission writes; workers deliberately omit it.
func (r *Runner) permissionState(st State, rec Record, dir string) State {
	if !rec.Lead {
		return st
	}
	selected, explicit, readErr := cachedPermissionSelection(dir)
	level, source, err := r.resolveLeadPermission(rec, selected, explicit, readErr)
	if err != nil {
		st.Error = appendDiagnostic(st.Error, err.Error())
		return st
	}
	st.PermissionLevel, st.PermissionSource = level, source
	active := ""
	if st.Process {
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
	quality := permissionQuality(c.rec.Backend, level, active)
	limit := "filesystem, network and other tool effects are not proven confined"
	if active == "" && c.rec.Backend == BackendClaude {
		limit = "native mode observation is pending; confinement is unproved"
	}
	if worker {
		limit = "common worker posture is deferred; the unchanged capability loadout is not a sandbox"
	}
	label := string(level)
	if label == "" {
		label = "unresolved"
	}
	fmt.Fprintf(c.r.opts.Log, "agent %s: Permission %s is %s on %s: %s.\n", c.rec.ReplyID, label, quality, c.rec.Backend, limit)
}

// SetPermissionLevel is the authorized agent-lowering seam. It cannot elevate a
// request by supplying an acknowledgement; only trusted HTTP control owns user.
func (r *Runner) SetPermissionLevel(id string, level PermissionLevel) (State, error) {
	return r.act(id, func(c *conv) error { return c.setPermissionLevel(level, false, false) })
}

func (c *conv) setPermissionLevel(level PermissionLevel, user, ack bool) error {
	if !c.rec.Lead {
		return badRequest("common worker permission selection is deferred")
	}
	if !c.r.supportsLeadPermission(c.rec.Backend, level) {
		return badRequest("permission_level %q is not supported on %s", level, c.rec.Backend)
	}
	current, _, err := c.r.leadPermission(c.rec, c.dir)
	if err != nil {
		return err
	}
	if err := authorizePermission(current, level, user, ack); err != nil {
		return err
	}
	if err := writePermissionSelection(c.dir, level); err != nil {
		return err
	}
	if mode := nativePermissionMode(c.rec.Backend, level); mode != "" {
		c.update(func(rec *Record) { rec.PermissionMode = mode })
		c.dirtyMode, c.reconcileMode = true, false
	}
	if !c.turn {
		c.applySettings()
	}
	c.permissionNudge(level, false, "")
	return nil
}

func (c *conv) setNativePermissionMode(mode string, user bool) error {
	if !c.rec.Lead || len(c.r.opts.Backends[c.rec.Backend].PermissionModes) == 0 {
		return categorized(ErrUnsupported, badRequest("the permission mode applies to a claude lead session only"))
	}
	// Extension/native-only registrations retain their historical mode contract.
	if c.r.supportsLeadPermission(c.rec.Backend, PermissionReadOnly) {
		current, source, err := c.r.leadPermission(c.rec, c.dir)
		if err != nil {
			return err
		}
		next := legacyLeadPermission(Record{Lead: true, Backend: c.rec.Backend, PermissionMode: mode})
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
	if mode != c.rec.PermissionMode {
		c.update(func(rec *Record) { rec.PermissionMode = mode })
		c.dirtyMode, c.reconcileMode = true, false
		if !c.turn {
			c.applySettings()
		}
	}
	return nil
}

// A holder attach ignores launch flags. After replay, reconstruct delivery from
// the durable selection/native target instead of trusting a lost dirty flag.
// Replay observations remain evidence, never an acknowledgement of that target.
func (c *conv) reconcileLeadPermission() {
	if !c.rec.Lead || c.rec.Backend != BackendClaude || c.dirtyMode {
		return
	}
	level, source, err := c.r.leadPermission(c.rec, c.dir)
	if err != nil {
		// State projects the current selection error without retaining history
		// in the terminal error slot or letting it suppress a process exit.
		return
	}
	if source != "explicit" {
		return
	}
	target := c.rec.PermissionMode
	if legacyLeadPermission(c.rec) != level {
		target = nativePermissionMode(c.rec.Backend, level)
	}
	if target == c.activeMode {
		return
	}
	if target != c.rec.PermissionMode {
		c.update(func(rec *Record) { rec.PermissionMode = target })
	}
	c.dirtyMode, c.reconcileMode = true, true
}
