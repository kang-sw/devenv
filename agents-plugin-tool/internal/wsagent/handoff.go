package wsagent

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// A lead conversation moves between the dashboard daemon and a terminal
// harness in both directions, keeping its ws session key, and so its
// reply-id, page, agenda, todos, and board. The session's registration row
// says which side holds it (wsdashboard.Registration.Hosted), and each
// move is one step under the row's lock (wsdashboard.LockRegistration):
//
//   - TakeOver: a session a terminal harness ran becomes a hosted one,
//     resuming the harness conversation its key record names, once that
//     harness has exited.
//   - HandOff: a hosted session's process ends and its row becomes a
//     terminal's; the state then carries the command that resumes it.
//   - Reopen: a closed hosted session is open again.
//
// A terminal login that finds a hosted session stopped takes its row by
// itself (conv.release); HandOff is the same move asked for from the page.

func conflict(format string, args ...any) error {
	return &requestError{status: http.StatusConflict, msg: fmt.Sprintf(format, args...)}
}

// TakeoverNoSession is why a session whose key record names no harness
// conversation cannot be taken over. The rail shows the same sentence on
// the disabled entry of such a row.
const TakeoverNoSession = "its harness reported no conversation to resume (it last ran with an older ws plugin)"

// TakeOver makes the terminal lead session id a hosted one: the daemon
// adopts its ws key, writes its row as a hosted row, and resumes the
// harness conversation its key record names, in the directory that
// harness ran in, on the session's next input or mail.
//
// It is refused while the session's terminal harness may still run
// (Registration.HarnessMayRun): two processes on one harness conversation
// corrupt it, and the daemon cannot end a terminal process. A session the
// daemon hosts, an agent conversation, a session of a harness other than
// claude and pi, one whose lead key is gone (which includes every session
// outside a git worktree), and one whose key record names no harness
// conversation are refused too.
func (r *Runner) TakeOver(id string) (State, error) {
	if !wsdashboard.ValidReplyID(id) {
		return State{}, errUnknown
	}
	if r.opts.LeadKeys == nil || r.opts.Holders == nil {
		return State{}, badRequest("this daemon hosts no lead sessions")
	}
	r.mu.Lock()
	_, open := r.convs[id]
	r.mu.Unlock()
	if open {
		return State{}, conflict("the dashboard already hosts this session")
	}
	row, err := wsdashboard.ReadRegistration(r.opts.Dir, id)
	if err != nil {
		return State{}, errUnknown
	}
	if row.Agent {
		return State{}, badRequest("an agent conversation is not a lead session")
	}
	if row.Hosted {
		return State{}, conflict("the dashboard hosts this session already; reopen it if it is closed")
	}
	ls, ok := r.opts.LeadKeys.LeadSession(row.Project, id)
	if !ok {
		return State{}, conflict("the session has no lead key at a git worktree left, so it cannot be taken over")
	}
	backend := ls.Harness
	if backend == "" {
		backend = row.Harness
	}
	profile := LeadProfileFor(backend)
	if profile == "" {
		return State{}, badRequest("only a claude or pi session can be taken over; this one ran under %q", backend)
	}
	if ls.Session == "" {
		return State{}, conflict("the session cannot be taken over: %s", TakeoverNoSession)
	}
	if ls.Cwd != "" {
		if info, err := os.Stat(ls.Cwd); err != nil || !info.IsDir() {
			return State{}, conflict("the directory the session ran in is gone: %s", ls.Cwd)
		}
	}
	if backend == BackendPi {
		if info, err := os.Stat(ls.Session); err != nil || !info.Mode().IsRegular() {
			return State{}, conflict("the session's file is gone: %s", ls.Session)
		}
	}
	rp, err := r.resolve(profile, ls.Root)
	if err != nil {
		return State{}, err
	}
	if err := r.opts.LeadKeys.EnsureLead(ls.Root, ls.Key); err != nil {
		return State{}, fmt.Errorf("ws session key: %w", err)
	}

	// From the check that the terminal harness is gone to the hosted row,
	// one step: a login that reads the row after it is refused or takes
	// the session back, and never runs beside the daemon's process.
	unlock, err := wsdashboard.LockRegistration(r.opts.Dir, id)
	if err != nil {
		return State{}, err
	}
	locked := true
	defer func() {
		if locked {
			unlock()
		}
	}()
	if row, err = wsdashboard.ReadRegistration(r.opts.Dir, id); err != nil {
		return State{}, errUnknown
	}
	if row.Hosted {
		return State{}, conflict("the dashboard hosts this session already")
	}
	if row.HarnessMayRun(r.opts.Alive) {
		if row.HarnessPID > 0 {
			return State{}, conflict("the session's terminal process still runs (pid %d); exit it first", row.HarnessPID)
		}
		return State{}, conflict("the session did not end cleanly and its row names no process, so it cannot be told from one that still runs; resume it in its terminal and exit it first")
	}

	now := r.opts.Now()
	dir := wsdashboard.SessionDir(r.opts.Dir, id)
	rec := Record{
		ReplyID:        id,
		Model:          rp.Definition.Model,
		Effort:         rp.Definition.Effort,
		PermissionMode: rp.Definition.PermissionMode,
		CreatedAt:      row.RegisteredAt,
	}
	if old, err := readRecord(dir); err == nil && old.Lead && old.WSKey == ls.Key && old.Backend == rp.Definition.Backend {
		// A session the daemon hosted before: its settings, what it still
		// holds for the session (undelivered mail), and a holder it could
		// not end carry over.
		rec = old
		rec.ClosedAt, rec.HandedOffAt = "", ""
	}
	if rec.CreatedAt == "" {
		rec.CreatedAt = now.UTC().Format(time.RFC3339)
	}
	rec.Profile, rec.Backend, rec.Root, rec.WSKey = rp.Name, rp.Definition.Backend, ls.Root, ls.Key
	rec.Lead, rec.Listed = true, rp.Definition.ListInIndex
	if rec.Backend != BackendClaude {
		rec.PermissionMode = ""
	}
	rec.Cwd, rec.SessionFile = "", ""
	if ls.Cwd != "" && ls.Cwd != ls.Root {
		rec.Cwd = ls.Cwd
	}
	if rec.Backend == BackendPi {
		rec.SessionFile = ls.Session
	} else {
		rec.SessionID = ls.Session
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return State{}, err
	}
	if err := writeRecord(dir, rec); err != nil {
		return State{}, err
	}
	reg := registration(rec, now)
	reg.HostStopped = rec.HostPID == 0
	if err := wsdashboard.WriteRegistration(r.opts.Dir, reg); err != nil {
		return State{}, err
	}
	unlock()
	locked = false
	r.mu.Lock()
	delete(r.closed, id)
	r.mu.Unlock()
	fmt.Fprintf(r.opts.Log, "agent %s: taken over from a terminal session\n", id)
	return r.startConv(rec).state(), nil
}

// HandOff hands the hosted lead session id to the terminal: its process
// ends, mail no longer resumes it, its row becomes a terminal's (ended, so
// a later takeover is not refused), and the returned state carries the
// command that resumes the conversation there (State.ResumeCommand). It is
// refused for a session with no harness conversation yet, and when the
// session's process could not be ended.
func (r *Runner) HandOff(id string) (State, error) {
	return r.act(id, func(c *conv) error { return c.handOff() })
}

func (c *conv) handOff() error {
	if !c.rec.Lead {
		return badRequest("hand to terminal applies to a lead session only")
	}
	if resumeCommand(c.rec, c.dir) == "" {
		return conflict("the session has no conversation yet, so there is nothing to resume in a terminal; close it instead")
	}
	c.stopProcess("")
	// A holder an earlier stop could not end is tried once more: the
	// terminal must be the conversation's only process.
	if err := c.retireHolder(); err != nil {
		return fmt.Errorf("the session was not handed over: %w", err)
	}
	unlock, ok := c.lockRow()
	if !ok {
		if c.foreignRow {
			// A terminal login took the row meanwhile: the same outcome.
			c.release()
			return nil
		}
		return fmt.Errorf("the session's registration could not be locked; try again")
	}
	now := c.r.opts.Now()
	reg := registration(c.record(), now)
	reg.Hosted, reg.HostPID, reg.HostStopped = false, 0, false
	reg.EndedAt = now.UTC().Format(time.RFC3339)
	err := wsdashboard.WriteRegistration(c.r.opts.Dir, reg)
	unlock()
	if err != nil {
		return fmt.Errorf("rewrite the session's registration: %w", err)
	}
	c.foreignRow = true
	c.release()
	return nil
}

// Reopen opens the closed hosted lead session id again: it keeps its key,
// its harness conversation, and its settings, and resumes on its next
// input or mail. It is refused once the session's row is a terminal's (a
// terminal login took the closed session); such a session comes back
// through TakeOver.
func (r *Runner) Reopen(id string) (State, error) {
	r.mu.Lock()
	_, open := r.convs[id]
	rec, closed := r.closed[id]
	r.mu.Unlock()
	switch {
	case open:
		return State{}, conflict("the session is open")
	case !closed:
		return State{}, errUnknown
	case !rec.Lead:
		return State{}, badRequest("reopen applies to a lead session only")
	case rec.ClosedAt == "":
		return State{}, errHandedOff
	}
	if r.opts.LeadKeys == nil || r.opts.Holders == nil {
		return State{}, badRequest("this daemon hosts no lead sessions")
	}
	if _, err := r.resolve(rec.Profile, rec.Root); err != nil {
		return State{}, err
	}
	unlock, err := wsdashboard.LockRegistration(r.opts.Dir, id)
	if err != nil {
		return State{}, err
	}
	if rowForeign(r.opts.Dir, id) {
		unlock()
		return State{}, conflict("the session runs in a terminal now; take it over from the ended list once that terminal session has exited")
	}
	r.mu.Lock()
	if _, still := r.closed[id]; !still {
		r.mu.Unlock()
		unlock()
		return State{}, conflict("the session is open")
	}
	delete(r.closed, id)
	r.mu.Unlock()
	rec.ClosedAt = ""
	err = writeRecord(wsdashboard.SessionDir(r.opts.Dir, id), rec)
	unlock()
	if err != nil {
		r.mu.Lock()
		rec.ClosedAt = r.opts.Now().UTC().Format(time.RFC3339)
		r.closed[id] = rec
		r.mu.Unlock()
		return State{}, err
	}
	return r.startConv(rec).state(), nil
}

// resumeCommand is the shell command that resumes a lead session's harness
// conversation in a terminal, or "" when it has none yet: the directory
// the harness resolves its sessions by, then the harness's own resume. A
// pi session is named by its file, since pi resolves a bare id only inside
// its session directory and a session the daemon started keeps its file in
// the daemon's.
func resumeCommand(rec Record, sessionDir string) string {
	if !rec.Lead {
		return ""
	}
	cd := "cd " + shellQuote(rec.workDir()) + " && "
	switch rec.Backend {
	case BackendClaude:
		if rec.SessionID == "" {
			return ""
		}
		return cd + "claude --resume " + shellQuote(rec.SessionID)
	case BackendPi:
		file := rec.SessionFile
		if file == "" {
			file = piSessionFile(sessionDir)
			if _, err := os.Stat(file); err != nil {
				return ""
			}
		}
		return cd + "pi --session " + shellQuote(file)
	}
	return ""
}

// shellQuote quotes s for a POSIX shell when it holds anything but plain
// path characters.
func shellQuote(s string) string {
	plain := s != ""
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-+:@%,=", r)) {
			plain = false
			break
		}
	}
	if plain {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
