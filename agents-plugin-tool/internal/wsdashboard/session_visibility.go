package wsdashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SessionVisibilityFile is dashboard-owned state, outside both typed rewrite
// domains (agent.json and registration.json). It shares the session's existing
// directory/prune lifetime; old runtimes may display the row normally.
const SessionVisibilityFile = "visibility.json"

// SessionLifecycle is the dashboard's non-visual lifecycle projection. It is
// distinct from registration heartbeat liveness and introduces no new timer.
type SessionLifecycle string

const (
	SessionActiveOn        SessionLifecycle = "active_on"
	SessionAsleep          SessionLifecycle = "asleep"
	SessionArchived        SessionLifecycle = "archived"
	SessionPendingDeletion SessionLifecycle = "pending_deletion"
)

// DeriveSessionLifecycle preserves external ownership: a terminal row never
// acquires archive controls from a retained sidecar. Explicitly ended rows
// remain in the existing retention/prune domain, even with an archive sidecar.
// An archived open lead stays archived across a daemon outage/stale heartbeat.
func DeriveSessionLifecycle(reg Registration, hidden bool, now time.Time) SessionLifecycle {
	if reg.EndedAt != "" {
		return SessionPendingDeletion
	}
	if reg.Hosted && hidden {
		return SessionArchived
	}
	if !reg.Live(now) {
		return SessionPendingDeletion
	}
	if reg.Hosted && reg.HostStopped {
		return SessionAsleep
	}
	return SessionActiveOn
}

// ReadSessionHidden restores archived visibility. Absence preserves legacy
// visibility; callers must not turn a read error into permission to launch.
func ReadSessionHidden(dir, replyID string) (bool, error) {
	if !ValidReplyID(replyID) {
		return false, fmt.Errorf("invalid reply-id %q", replyID)
	}
	raw, err := os.ReadFile(filepath.Join(SessionDir(dir, replyID), SessionVisibilityFile))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var state struct {
		Hidden bool `json:"hidden"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return false, err
	}
	return state.Hidden, nil
}

// WriteSessionHidden atomically replaces only the dashboard's sidecar. A
// failed temporary write leaves the last good visibility untouched.
func WriteSessionHidden(dir, replyID string, hidden bool) error {
	if !ValidReplyID(replyID) {
		return fmt.Errorf("invalid reply-id %q", replyID)
	}
	sessionDir := SessionDir(dir, replyID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(struct {
		Hidden bool `json:"hidden"`
	}{hidden})
	if err != nil {
		return err
	}
	return writePrivateFile(sessionDir, SessionVisibilityFile, append(raw, '\n'))
}
