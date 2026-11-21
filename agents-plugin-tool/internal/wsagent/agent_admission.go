// agent_admission.go holds child authority and width admission.
package wsagent

import (
	"fmt"
	"net/http"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

type admissionRefusal struct{ reason string }

func (e *admissionRefusal) Error() string { return e.reason }

// holdParentAdmission keeps a parent's current selection stable from the
// child's admission read through the operation that follows it. Permission
// controls take the same session slot exclusively while changing selection.
func (r *Runner) holdParentAdmission(parent *ParentRef) func() {
	if parent == nil {
		return func() {}
	}
	slot := permissionSlot(wsdashboard.SessionDir(r.opts.Dir, parent.ReplyID))
	slot.admission.RLock()
	return slot.admission.RUnlock
}

// agentCeiling is the highest authority parent's children may take: its
// current level. A lead parent's reply-id resolves to its Runner record.
// agent.spawn delegation and omitted legacy Pi spawns retain the historical
// detached-lead ceiling; new explicit generic workers require current parent
// state. A conversation parent always needs its record and current level.
func (r *Runner) agentCeiling(parent ParentRef, detachedLeadFallback bool) (PermissionLevel, error) {
	rec, ok := r.recordOf(parent.ReplyID)
	dir := wsdashboard.SessionDir(r.opts.Dir, parent.ReplyID)
	if parent.Kind == ParentLead {
		if !ok || !rec.Lead {
			if detachedLeadFallback {
				return PermissionWorkdirWrite, nil
			}
			return "", fmt.Errorf("the parent lead %s has no hosted record, so its permission level is unknown", parent.ReplyID)
		}
		level, _, err := r.leadPermission(rec, dir)
		if err != nil {
			return "", fmt.Errorf("the parent lead's permission level is unreadable: %w", err)
		}
		if level == "" {
			if detachedLeadFallback {
				return PermissionWorkdirWrite, nil
			}
			return "", fmt.Errorf("the parent lead %s has no permission level of its own", parent.ReplyID)
		}
		return level, nil
	}
	if !ok {
		return "", fmt.Errorf("the parent conversation %s has no record, so its permission level is unknown", parent.ReplyID)
	}
	level, _, err := r.currentWorkerPermission(rec)
	if err != nil {
		return "", fmt.Errorf("the parent conversation's permission level is unreadable: %w", err)
	}
	if level.rank() == 0 {
		return "", fmt.Errorf("the parent conversation %s has no permission level of its own", parent.ReplyID)
	}
	return level, nil
}

// admitWorkerLevel applies the current-parent ceiling to a worker creation or
// launch. Root workers have no ceiling; a child with unresolved own authority
// is pending rather than guessed.
func (r *Runner) admitWorkerLevel(parent *ParentRef, level PermissionLevel, detachedLeadFallback bool) error {
	if parent == nil {
		return nil
	}
	if level.rank() == 0 {
		return &admissionRefusal{reason: "permission admission: the worker's own permission level is unresolved"}
	}
	ceiling, err := r.agentCeiling(*parent, detachedLeadFallback)
	if err != nil {
		return &admissionRefusal{reason: "permission admission: " + err.Error()}
	}
	if level.rank() > ceiling.rank() {
		return &admissionRefusal{reason: fmt.Sprintf("permission admission: authority %s is above the parent's current level %s", level, ceiling)}
	}
	return nil
}

// legacyDetachedWorkerLevel preserves the omitted top-level pi.spawn contract:
// an external lead has no Runner record, but its existing fixed Pi child
// loadout remains workdir-write/advisory. Hosted and conversation parents must
// still resolve normally, and explicit new generic selections get no fallback.
func (r *Runner) legacyDetachedWorkerLevel(parent *ParentRef, level PermissionLevel, source string) PermissionLevel {
	if level.rank() != 0 || source != "legacy-derived" || parent == nil || parent.Kind != ParentLead {
		return level
	}
	if _, ok := r.recordOf(parent.ReplyID); !ok {
		return PermissionWorkdirWrite
	}
	return level
}

// detachedWorkerFallback distinguishes new generic explicit workers (stored
// false) from agent.spawn (true), while absence preserves sidecars written
// before immutable provenance was introduced. Omitted legacy Pi spawns retain
// their pre-change detached-lead behavior without writing a sidecar.
func detachedWorkerFallback(dir, source string) bool {
	if source == "legacy-derived" {
		return true
	}
	delegated, set := delegatedPermissionState(dir)
	return delegated || !set
}

// childAdmission resolves a worker's current explicit or legacy-derived level,
// then admits it against its parent's current level. No derived value is
// stored, and an unresolved child never guesses a ceiling.
func (r *Runner) childAdmission(rec Record, def Profile) (PermissionLevel, string, error) {
	if rec.Lead {
		return "", "", nil
	}
	dir := wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID)
	level, source, err := r.workerPermission(rec, def, dir)
	if err != nil {
		return "", "", &admissionRefusal{reason: fmt.Sprintf("permission admission: own level unreadable: %v", err)}
	}
	if isAgentWorkerProfile(rec.Profile) && source != "explicit" {
		return "", "", &admissionRefusal{reason: "permission admission: agent.spawn child has no explicit authority record"}
	}
	level = r.legacyDetachedWorkerLevel(rec.Parent, level, source)
	if err := r.admitWorkerLevel(rec.Parent, level, detachedWorkerFallback(dir, source)); err != nil {
		return "", "", err
	}
	return level, source, nil
}

// childLevel is rec's explicit authority, "" for none.
func childLevel(dir string) PermissionLevel {
	level, explicit, err := cachedPermissionSelection(dir)
	if err != nil || !explicit {
		return ""
	}
	return level
}

// awakeLocked reports an awake child for the width cap and its parent's
// sleep: a process, a turn or background task, a held done turn, an initial
// spawn before its first Send, or input or claimed mail it has not taken
// whose start was not refused. Under mu.
func (c *conv) awakeLocked() bool {
	return !c.closed && (c.proc != nil || c.turn || c.bgTasks != 0 || c.holding || c.spawnPending ||
		(!c.wakeRefused && (c.sends > max(c.settled.mark, c.voided) || len(c.rec.Mail) > 0)))
}

// awakeChildren counts parent's open, awake direct children.
func (r *Runner) awakeChildren(parent ParentRef) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.awakeChildrenLocked(parent)
}

// awakeChildrenLocked counts parent's awake direct children plus in-flight
// width-checked spawns. Caller holds r.mu.
func (r *Runner) awakeChildrenLocked(parent ParentRef) int {
	n := r.pendingSpawns[parent]
	for _, c := range r.convs {
		c.mu.Lock()
		if c.rec.Parent != nil && *c.rec.Parent == parent && c.awakeLocked() {
			n++
		}
		c.mu.Unlock()
	}
	return n
}

// releasePendingSpawnLocked drops one in-flight spawn slot for parent.
// Caller holds r.mu.
func (r *Runner) releasePendingSpawnLocked(parent ParentRef) {
	if r.pendingSpawns[parent] <= 1 {
		delete(r.pendingSpawns, parent)
		return
	}
	r.pendingSpawns[parent]--
}

// reserveSpawnWidth checks parent's awake direct children under r.mu and
// reserves a slot for an in-flight agent.spawn. claimLocked transfers the
// slot when the conversation is registered in r.convs under r.mu; release
// drops an unclaimed slot on error.
func (r *Runner) reserveSpawnWidth(parent ParentRef) (claimLocked func(), release func(), err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n := r.awakeChildrenLocked(parent); n >= MaxSpawnWidth {
		return nil, nil, &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("agent.spawn: the spawn width cap is %d awake direct children, and this agent has %d; wait for one to finish (a sleeping child does not count)", MaxSpawnWidth, n)}
	}
	if r.pendingSpawns == nil {
		r.pendingSpawns = map[ParentRef]int{}
	}
	r.pendingSpawns[parent]++
	done := false
	claimLocked = func() {
		if done {
			return
		}
		done = true
		r.releasePendingSpawnLocked(parent)
	}
	release = func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		claimLocked()
	}
	return claimLocked, release, nil
}

// depthRefusal is the spawn depth cap's refusal.
func depthRefusal(tool string, depth int) error {
	return &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("%s: the spawn depth cap is %d (a lead is depth 0, each spawned agent one deeper); this conversation is at depth %d, so it spawns no subagent: do the work itself", tool, MaxSpawnDepth, depth)}
}
