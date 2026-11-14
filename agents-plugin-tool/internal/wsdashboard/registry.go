package wsdashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// The machine session registry: one directory per registered session under
// <state dir>/sessions/<reply-id>/, holding the session's registration file.
// A stdio ws-mcp writes the registration of every top-level lead session it
// serves while that session's dashboard key resolves on, and the daemon's
// agent runner writes one (Agent set) for each conversation it hosts; the
// daemon's index only reads them. Registering per session, not per project root, is what keeps a
// key-off session off the dashboard when another key-on ws-mcp serves the
// same root.
//
// The per-session directory is also where the session's other dashboard
// stores (its live snapshot and conversation ring) belong, so the daemon finds
// them from the reply-id alone.
const (
	sessionsDirName      = "sessions"
	registrationFileName = "registration.json"
	// registrationLockName is the row lock's file in the session directory
	// (LockRegistration).
	registrationLockName = "registration.json.lock"
	// registrationLockTimeout bounds one wait for the row lock.
	registrationLockTimeout = 2 * time.Second

	registrationSchemaVersion = 1

	// AgentRecordFileName is the agent runner's conversation record in a
	// session directory; its presence marks the directory as the runner's.
	AgentRecordFileName = "agent.json"

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
// Liveness rests on the heartbeat alone: a reused pid can never make an
// ended session look live. The two process ids a row may carry (HostPID,
// HarnessPID) only ever refuse a change of the session's holder
// (HostMayRun, HarnessMayRun), so a reused pid can at worst refuse one.
type Registration struct {
	SchemaVersion int `json:"schema_version"`
	// ReplyID is the session's reply-id, never its session key.
	ReplyID string `json:"reply_id"`
	// Project is the session's root path, as its session record holds it.
	Project string `json:"project"`
	// Harness names the host the session runs in (claude, codex, pi), when
	// the serving ws-mcp knows it.
	Harness string `json:"harness,omitempty"`
	// GitCommonDir is the session root's git common directory (git
	// rev-parse --git-common-dir, absolute), which the serving ws-mcp reads
	// once when it registers, so the session rail groups a linked worktree
	// under its repository without running git per session. Empty when the
	// root is not in a repository, and from an older ws-mcp; the rail then
	// reads the root's .git itself (railRepoRoot).
	GitCommonDir string `json:"git_common_dir,omitempty"`
	RegisteredAt string `json:"registered_at,omitempty"`
	// HeartbeatAt is refreshed every HeartbeatInterval while the serving
	// ws-mcp runs and the session made a keyed call recently.
	HeartbeatAt string `json:"heartbeat_at,omitempty"`
	// EndedAt is set when the serving ws-mcp exits cleanly.
	EndedAt string `json:"ended_at,omitempty"`
	// Agent marks a conversation the daemon's own agent runner hosts: no
	// ws-mcp serves it and it has no session record, so the daemon reads it
	// from its live stores alone (agentSessions).
	Agent bool `json:"agent,omitempty"`
	// Unlisted keeps the session out of the index page; its own page is
	// still served.
	Unlisted bool `json:"unlisted,omitempty"`
	// Hosted marks a lead session the daemon's agent runner spawned and
	// feeds (a headless harness under a session holder). The runner, not a
	// ws-mcp, writes this row; the session keeps its session record, so the
	// daemon reads it as any lead session.
	Hosted bool `json:"hosted,omitempty"`
	// HostPID is the hosted session's harness process: the one process the
	// hosted-session flag holds for. The runner writes 0 before each spawn
	// and the child's PID right after it, so a harness reading 0 waits
	// briefly and one reading another PID runs as a plain session.
	HostPID int `json:"host_pid,omitempty"`
	// HostStopped is set by the runner on a hosted row written while the
	// session has no harness process and none is starting. It tells a
	// stopped session (HostPID 0, HostStopped set) from a spawn under way
	// (HostPID 0 alone), which HostMayRun reads.
	HostStopped bool `json:"host_stopped,omitempty"`
	// Exit details are optional; a nil code means the runner did not record one.
	HostExitCode   *int   `json:"host_exit_code,omitempty"`
	HostExitSignal *int   `json:"host_exit_signal,omitempty"`
	HostExitReason string `json:"host_exit_reason,omitempty"`
	HostStoppedAt  string `json:"host_stopped_at,omitempty"`
	// HarnessPID is the harness process of the ws-mcp that wrote this row
	// (a session a terminal harness runs), when its launcher reported one.
	// It never makes the session live; HarnessMayRun reads it.
	HarnessPID int `json:"harness_pid,omitempty"`
	// MCPOwnerPID identifies the launcher whose proxy connection registered
	// this terminal session. A successor daemon uses an actual reconnect
	// from that launcher to resume heartbeats; the PID alone is not liveness.
	MCPOwnerPID int `json:"mcp_owner_pid,omitempty"`
	// LastCallAt is the latest keyed activity known to the serving ws-mcp,
	// persisted on registration/heartbeat/end. Reconnection resumes the idle
	// budget from this stamp rather than counting a restart as activity.
	LastCallAt string `json:"last_call_at,omitempty"`
}

// HostMayRun reports whether a hosted row's harness process may still run,
// or be starting: what refuses a terminal harness logging in with the
// session's key. A recorded process is asked of alive; a row that names
// none says it is stopped (HostStopped) or ended (EndedAt), and otherwise
// (a spawn under way, or a row from a runner that wrote no HostStopped)
// reads as running.
func (r Registration) HostMayRun(alive func(pid int) bool) bool {
	if r.HostPID > 0 {
		return alive(r.HostPID)
	}
	return !r.HostStopped && r.EndedAt == ""
}

// HarnessMayRun reports whether the terminal harness behind a row that is
// not hosted may still run: what refuses the daemon taking the session
// over. A recorded process is asked of alive; a row that names none (from
// a ws-mcp that recorded no harness process) reads as running unless the
// session ended cleanly (EndedAt), since a heartbeat also stops for a
// session that only went idle.
func (r Registration) HarnessMayRun(alive func(pid int) bool) bool {
	if r.HarnessPID > 0 {
		return alive(r.HarnessPID)
	}
	return r.EndedAt == ""
}

// LockRegistration takes the row lock of session replyID under dir,
// creating its directory, and returns its release. The daemon's runner and
// a ws-mcp both rewrite a hosted session's row when the session changes
// hands (a terminal login taking a stopped hosted session, the daemon
// taking a terminal session over), and each decides from the row it reads:
// the lock makes that read, the decision, and the write one step. A
// terminal ws-mcp takes it for every row write too, so it never writes
// back a row the daemon took over meanwhile. A
// wait past registrationLockTimeout is an error, so a caller never acts on
// a row it could not lock.
func LockRegistration(dir, replyID string) (unlock func(), err error) {
	if !ValidReplyID(replyID) {
		return nil, fmt.Errorf("invalid reply-id %q", replyID)
	}
	sessionDir := SessionDir(dir, replyID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(sessionDir, registrationLockName)
	fl := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), registrationLockTimeout)
	defer cancel()
	locked, err := fl.TryLockContext(ctx, 20*time.Millisecond)
	if errors.Is(err, context.DeadlineExceeded) || (err == nil && !locked) {
		return nil, fmt.Errorf("timed out after %s waiting for the registration lock %s", registrationLockTimeout, lockPath)
	}
	if err != nil {
		return nil, fmt.Errorf("acquire the registration lock: %w", err)
	}
	return func() { _ = fl.Unlock() }, nil
}

// RegistrationPath is the registration file of session replyID under dir:
// what a hosted session's harness reads to check HostPID.
func RegistrationPath(dir, replyID string) string {
	return filepath.Join(SessionDir(dir, replyID), registrationFileName)
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
	if err := wsstate.AtomicReplaceFile(name, filepath.Join(sessionDir, registrationFileName)); err != nil {
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
//
// A conversation the daemon's agent runner hosts (registration Agent, or a
// directory holding AgentRecordFileName) is not pruned while the runner may
// serve it: it heartbeats only while the daemon runs, so a daemon down past
// retention would otherwise lose its record, held sends, and backend
// session before the runner re-registers it. The exception is a lead
// session the runner no longer serves (its record is closed, or handed to
// a terminal): its row is no longer the runner's to refresh, so it ages
// as any ended session's, measured from the row. A handed-off session
// whose harness conversation lives in the directory itself is kept all the
// same (agentRecordEnd.terminalConversation). Every other agent record
// (the tracker's support agent among them) keeps the runner's lifetime.
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
		sessionDir := SessionDir(dir, entry.Name())
		record, hosted := readAgentRecordEnd(sessionDir)
		if hosted && (!record.retired() || record.terminalConversation(sessionDir)) {
			continue
		}
		last := time.Time{}
		if reg, err := ReadRegistration(dir, entry.Name()); err == nil {
			if reg.Agent {
				continue
			}
			last = reg.LastSeen()
		} else if hosted {
			// A retired lead with no readable row to age it by is left to
			// the runner.
			continue
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

// agentRecordEnd is what the registry reads of the agent runner's record
// (AgentRecordFileName, written by internal/wsagent): whether it is a lead
// session's, and whether the runner still serves it.
type agentRecordEnd struct {
	Lead        bool   `json:"lead"`
	ClosedAt    string `json:"closed_at"`
	HandedOffAt string `json:"handed_off_at"`
	Backend     string `json:"backend"`
	SessionFile string `json:"session_file"`
}

// terminalConversation reports a session handed to a terminal whose
// harness conversation is a file inside sessionDir, the session's
// directory: a pi session the daemon started, which names no session_file
// or, once taken over again, names the one in its directory (a session a
// terminal started names its own file elsewhere). The terminal resumes on
// that file, so from the hand-off on the directory holds a conversation
// the user's terminal harness owns, and deleting it would delete that
// conversation. A claude conversation lives in the harness's own store.
func (r agentRecordEnd) terminalConversation(sessionDir string) bool {
	if !r.Lead || r.HandedOffAt == "" || r.Backend != "pi" {
		return false
	}
	return r.SessionFile == "" || strings.HasPrefix(filepath.Clean(r.SessionFile), filepath.Clean(sessionDir)+string(filepath.Separator))
}

// retired reports a lead session the runner serves no more: closed, or
// handed to a terminal.
func (r agentRecordEnd) retired() bool {
	return r.Lead && (r.ClosedAt != "" || r.HandedOffAt != "")
}

// readAgentRecordEnd reads sessionDir's agent record; hosted is false when
// the directory holds none. An unreadable record is hosted and not
// retired, so it is never pruned.
func readAgentRecordEnd(sessionDir string) (record agentRecordEnd, hosted bool) {
	raw, err := os.ReadFile(filepath.Join(sessionDir, AgentRecordFileName))
	if errors.Is(err, os.ErrNotExist) {
		return agentRecordEnd{}, false
	}
	if err != nil || json.Unmarshal(raw, &record) != nil {
		return agentRecordEnd{}, true
	}
	return record, true
}
