package mcp

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// What a top-lead login records and checks about the harness conversation
// behind a key, so one conversation is never held by two processes: the
// dashboard daemon may host a session a terminal harness started, and a
// terminal harness may resume a session the daemon hosted.

// HostedLoginRefusalMarker is the token every refusal of hostedLoginGuard
// carries. A harness adapter matches it to tell this refusal from a key
// that is merely unusable (unknown, another root): here it must not fall
// back to minting a fresh key, and shows the refusal to the user instead.
const HostedLoginRefusalMarker = "hosted-session-held"

// harnessSessionMax bounds a reported harness session name and directory.
const harnessSessionMax = 4096

// hostedProcessAlive is the process liveness check of the login guard (a
// test seam).
var hostedProcessAlive = wsstate.ProcessAlive

// harnessProcessParent is the process-table lookup behind harnessPID (a test
// seam): the parent PID and image name of a process.
var harnessProcessParent = wsstate.ProcessParent

// harnessPID is the PID of this server's harness as its launcher reported
// it, with a Windows shell wrapper skipped (see wrappedHarnessPID); zero when
// it reported none. Every consumer of the harness PID reads it here, so they
// all see the same value.
func (s *Server) harnessPID() int {
	pid, _ := strconv.Atoi(strings.TrimSpace(s.getenv(envHarnessPID)))
	if pid <= 0 {
		return 0
	}
	return wrappedHarnessPID(runtime.GOOS, pid, harnessProcessParent)
}

// wrappedHarnessPID is pid, or on Windows, when pid is a cmd.exe process, its
// parent. A harness there commonly starts a stdio server as `cmd.exe /c ...`,
// so the process that started the launcher is that wrapper, not the harness.
// Exactly one level is skipped, so a nested harness is not mistaken for the
// outer one, and any lookup failure keeps pid.
func wrappedHarnessPID(goos string, pid int, parentOf func(int) (int, string, bool)) int {
	if goos != "windows" {
		return pid
	}
	if parent, image, ok := parentOf(pid); ok && parent > 0 && strings.EqualFold(image, "cmd.exe") {
		return parent
	}
	return pid
}

// recordHarnessSession stores the harness conversation a top-lead login
// reported (the bootstrap tool's harness_session and harness_cwd) on the
// key's record. It is best effort: a login that reports none, or reports
// something unusable (an empty name, a directory that is not absolute),
// records nothing and still succeeds.
func (s *Server) recordHarnessSession(key string, arguments map[string]any) {
	session, _ := arguments["harness_session"].(string)
	cwd, _ := arguments["harness_cwd"].(string)
	session, cwd = strings.TrimSpace(session), strings.TrimSpace(cwd)
	if session == "" && cwd == "" {
		return
	}
	if session == "" || len(session) > harnessSessionMax || len(cwd) > harnessSessionMax || !filepath.IsAbs(cwd) {
		s.debugEvent("session.harness_session_ignored", map[string]any{"reason": "harness_session must be non-empty and harness_cwd an absolute path"})
		return
	}
	cwd = filepath.Clean(cwd)
	harness := s.currentHarness()
	err := s.sessions.mutateRecord(key, partKey, func(record *sessionRecord) error {
		if record.Harness == harness && record.HarnessSession == session && record.HarnessCwd == cwd {
			return errRecordUnchanged
		}
		record.Harness, record.HarnessSession, record.HarnessCwd = harness, session, cwd
		return nil
	})
	if err != nil {
		s.debugEvent("session.harness_session_failed", map[string]any{"error": err.Error()})
	}
}

// hostedLoginRefusal is the guard's refusal of key; why is the clause that
// says what the guard saw.
func hostedLoginRefusal(key, why string) error {
	return fmt.Errorf("session bootstrap: relogin_session_key %q is refused (%s): the dashboard hosts this session, and %s. "+
		"Hand the session to the terminal from its dashboard page first (start the dashboard if it is not running), then resume it here. "+
		"No session key was issued.", key, HostedLoginRefusalMarker, why)
}

// hostedLoginGuard is the one point where a second holder of a session the
// dashboard daemon hosts is caught: a re-login with key, rooted at root, by
// a server that is not the hosted session's own.
//
// The session's registration row decides, read under its row lock so the
// daemon's own check-and-spawn cannot interleave:
//
//   - no row: accepted; there is nothing the daemon could take over.
//   - a row the daemon does not hold (not hosted): accepted, and the row is
//     marked as held by this server's harness (its process, no end mark, a
//     fresh heartbeat) whatever the session's dashboard key says. Without
//     the mark, a row an earlier run ended, or one the daemon handed to the
//     terminal, would still read as a session nothing runs, and the daemon
//     would take it over beside this harness.
//   - this server is the hosted session's own (its hosted key, or its
//     harness is the process the row names): accepted, and nothing changes.
//   - the row's harness process may run (Registration.HostMayRun): refused.
//     The daemon cannot stop a terminal harness that resumed the same
//     conversation, so the login is refused and the adapter stays keyless.
//   - otherwise (the session is stopped, closed, or its process died while
//     no daemon ran): accepted as a hand to the terminal. The row becomes
//     this server's, carrying its harness process, and the daemon lets the
//     conversation go when it next reads the row.
func (s *Server) hostedLoginGuard(key, root string) error {
	if s.hostedSessionKey() == key {
		return nil
	}
	d := &s.dashboard
	dir, err := d.stateDir()
	if err != nil {
		return nil
	}
	replyID := wsmailbox.ReplyID(key)
	// A session with no row takes no lock and creates nothing.
	seen, err := wsdashboard.ReadRegistration(dir, replyID)
	if err != nil {
		return nil
	}
	unlock, err := wsdashboard.LockRegistration(dir, replyID)
	if err != nil {
		if !seen.Hosted {
			s.debugEvent("dashboard.terminal_row_unmarked", map[string]any{"reply_id": replyID, "error": err.Error()})
			return nil
		}
		return hostedLoginRefusal(key, "its registration could not be locked to check whether its process still runs ("+err.Error()+")")
	}
	defer unlock()
	row, err := wsdashboard.ReadRegistration(dir, replyID)
	if err != nil {
		return nil
	}
	harness := s.harnessPID()
	now := d.clock()
	// held marks the row as this server's harness's.
	held := func() error {
		row.HarnessPID = harness
		row.MCPOwnerPID = s.ownerPID
		if name := s.currentHarness(); name != "" {
			row.Harness = name
		}
		row.LastCallAt = dashboardStamp(now)
		row.HeartbeatAt, row.EndedAt = dashboardStamp(now), ""
		return wsdashboard.WriteRegistration(dir, row)
	}
	if !row.Hosted {
		// The row stays the first keyed call's to register or remove
		// (dashboardTouch); this only keeps the daemon off it until then,
		// and for good when the session's dashboard key is off.
		if err := held(); err != nil {
			s.debugEvent("dashboard.terminal_row_unmarked", map[string]any{"reply_id": replyID, "error": err.Error()})
		}
		return nil
	}
	if harness > 0 && harness == row.HostPID {
		return nil
	}
	if row.HostMayRun(hostedProcessAlive) {
		return hostedLoginRefusal(key, "its process is running or starting there")
	}
	row.Hosted, row.HostPID, row.HostStopped = false, 0, false
	if err := held(); err != nil {
		return hostedLoginRefusal(key, "its registration could not be rewritten for this terminal ("+err.Error()+")")
	}
	// The row is this server's from here on: it heartbeats what it wrote.
	d.mu.Lock()
	if d.sessions == nil {
		d.sessions = map[string]*dashboardSession{}
	}
	d.sessions[key] = &dashboardSession{replyID: replyID, project: root, reg: row, lastCall: now}
	d.mu.Unlock()
	s.debugEvent("dashboard.hosted_handoff", map[string]any{"reply_id": replyID})
	return nil
}
