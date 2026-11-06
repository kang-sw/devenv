package wsdashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// The machine session registry: one directory per registered session under
// <state dir>/sessions/<reply-id>/, holding the session's registration file.
// A stdio ws-mcp writes the registration of every top-level lead session it
// serves while that session's dashboard key resolves on; the daemon only
// reads it. Registering per session, not per project root, is what keeps a
// key-off session off the dashboard when another key-on ws-mcp serves the
// same root.
//
// The per-session directory is also where the session's other dashboard
// stores (its live snapshot and conversation ring) belong, so the daemon finds
// them from the reply-id alone.
const (
	sessionsDirName      = "sessions"
	registrationFileName = "registration.json"

	registrationSchemaVersion = 1

	// HeartbeatInterval is how often a ws-mcp refreshes the heartbeat of each
	// session it still serves.
	HeartbeatInterval = 2 * time.Minute
	// LiveWindow is how recent a heartbeat must be for its session to count
	// as live: more than two heartbeats, so one late tick never reads as an
	// ended session.
	LiveWindow = 5 * time.Minute
	// RegistryRetention is how long an ended session's directory is kept
	// before a ws-mcp prunes it.
	RegistryRetention = 7 * 24 * time.Hour
)

// replyIDPattern bounds which strings may become a registry path. A reply-id
// is lowercase words concatenated; the nudge body that carries one is
// untrusted input.
var replyIDPattern = regexp.MustCompile(`^[a-z0-9]{1,64}$`)

// ValidReplyID reports whether id is safe to use as a registry directory name.
func ValidReplyID(id string) bool { return replyIDPattern.MatchString(id) }

// Registration is one session's registry entry. Readers ignore unknown fields
// and treat absent ones as empty, so a newer daemon reads what an older
// ws-mcp still writes and the reverse.
//
// Liveness rests on the heartbeat alone: no process id is recorded, so a
// reused pid can never make an ended session look live.
type Registration struct {
	SchemaVersion int `json:"schema_version"`
	// ReplyID is the session's reply-id, never its session key.
	ReplyID string `json:"reply_id"`
	// Project is the session's root path, as its session record holds it.
	Project string `json:"project"`
	// Harness names the host the session runs in (claude, codex, pi), when
	// the serving ws-mcp knows it.
	Harness      string `json:"harness,omitempty"`
	RegisteredAt string `json:"registered_at,omitempty"`
	// HeartbeatAt is refreshed every HeartbeatInterval while the serving
	// ws-mcp runs and the session made a keyed call recently.
	HeartbeatAt string `json:"heartbeat_at,omitempty"`
	// EndedAt is set when the serving ws-mcp exits cleanly.
	EndedAt string `json:"ended_at,omitempty"`
}

// Live is the served-session liveness rule: a session is live while it has
// not ended and its heartbeat is within LiveWindow of now. A crashed ws-mcp
// stops heartbeating, so its sessions read as ended within LiveWindow.
func (r Registration) Live(now time.Time) bool {
	if r.EndedAt != "" {
		return false
	}
	beat, err := time.Parse(time.RFC3339, r.HeartbeatAt)
	if err != nil {
		return false
	}
	return now.Sub(beat) <= LiveWindow
}

// LastSeen is the latest of the registration's stamps; zero when none parses.
func (r Registration) LastSeen() time.Time {
	var last time.Time
	for _, stamp := range []string{r.RegisteredAt, r.HeartbeatAt, r.EndedAt} {
		if t, err := time.Parse(time.RFC3339, stamp); err == nil && t.After(last) {
			last = t
		}
	}
	return last
}

// SessionDir is the registry directory of one session under the daemon state
// directory dir.
func SessionDir(dir, replyID string) string {
	return filepath.Join(dir, sessionsDirName, replyID)
}

// ReadRegistration reads one session's registration.
func ReadRegistration(dir, replyID string) (Registration, error) {
	if !ValidReplyID(replyID) {
		return Registration{}, fmt.Errorf("invalid reply-id %q", replyID)
	}
	raw, err := os.ReadFile(filepath.Join(SessionDir(dir, replyID), registrationFileName))
	if err != nil {
		return Registration{}, err
	}
	var reg Registration
	if err := json.Unmarshal(bytes.TrimSpace(raw), &reg); err != nil {
		return Registration{}, fmt.Errorf("parse registration %s: %w", replyID, err)
	}
	if reg.ReplyID != replyID {
		return Registration{}, fmt.Errorf("registration %s names reply-id %q", replyID, reg.ReplyID)
	}
	return reg, nil
}

// ReadRegistrations reads every readable registration, skipping a directory
// whose file is missing, unreadable, or does not parse. A missing registry
// is empty.
func ReadRegistrations(dir string) ([]Registration, error) {
	entries, err := os.ReadDir(filepath.Join(dir, sessionsDirName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Registration
	for _, entry := range entries {
		if !entry.IsDir() || !ValidReplyID(entry.Name()) {
			continue
		}
		if reg, err := ReadRegistration(dir, entry.Name()); err == nil {
			out = append(out, reg)
		}
	}
	return out, nil
}

// WriteRegistration replaces a session's registration atomically (temp file
// + rename), creating its directory.
func WriteRegistration(dir string, reg Registration) error {
	if !ValidReplyID(reg.ReplyID) {
		return fmt.Errorf("invalid reply-id %q", reg.ReplyID)
	}
	if reg.SchemaVersion == 0 {
		reg.SchemaVersion = registrationSchemaVersion
	}
	sessionDir := SessionDir(dir, reg.ReplyID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(sessionDir, registrationFileName+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, filepath.Join(sessionDir, registrationFileName)); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// RemoveRegistration deletes a session's registration file, which takes the
// session off the dashboard. A missing file is not an error.
func RemoveRegistration(dir, replyID string) error {
	if !ValidReplyID(replyID) {
		return fmt.Errorf("invalid reply-id %q", replyID)
	}
	err := os.Remove(filepath.Join(SessionDir(dir, replyID), registrationFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// PruneRegistrations deletes the directory of every session last seen more
// than retention before now, and of every directory without a readable
// registration that has not changed for as long. It returns how many it
// deleted.
func PruneRegistrations(dir string, now time.Time, retention time.Duration) int {
	entries, err := os.ReadDir(filepath.Join(dir, sessionsDirName))
	if err != nil {
		return 0
	}
	pruned := 0
	for _, entry := range entries {
		if !entry.IsDir() || !ValidReplyID(entry.Name()) {
			continue
		}
		last := time.Time{}
		if reg, err := ReadRegistration(dir, entry.Name()); err == nil {
			last = reg.LastSeen()
		}
		if last.IsZero() {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			last = info.ModTime()
		}
		if now.Sub(last) <= retention {
			continue
		}
		if os.RemoveAll(SessionDir(dir, entry.Name())) == nil {
			pruned++
		}
	}
	return pruned
}
