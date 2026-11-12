package wsagent

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// processTable is the machine's process table as a takeover test sets it.
type processTable struct {
	mu    sync.Mutex
	alive map[int]bool
}

func (p *processTable) set(pid int, alive bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.alive == nil {
		p.alive = map[int]bool{}
	}
	p.alive[pid] = alive
}

func (p *processTable) check(pid int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.alive[pid]
}

// terminalSession registers a lead session a terminal harness ran: its
// row as that harness's ws-mcp wrote it, and its key record as the key
// store answers for it. edit changes the row before it is written.
func (h *leadHarness) terminalSession(id string, ls LeadSession, edit func(row *wsdashboard.Registration)) wsdashboard.Registration {
	h.t.Helper()
	ls.Root = h.root
	stamp := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	row := wsdashboard.Registration{ReplyID: id, Project: h.root, Harness: ls.Harness, RegisteredAt: stamp, HeartbeatAt: stamp, EndedAt: stamp}
	if edit != nil {
		edit(&row)
	}
	if err := wsdashboard.WriteRegistration(h.dir, row); err != nil {
		h.t.Fatal(err)
	}
	h.keys.mu.Lock()
	if h.keys.sessions == nil {
		h.keys.sessions = map[string]LeadSession{}
	}
	h.keys.sessions[id] = ls
	h.keys.mu.Unlock()
	return h.row(id)
}

func (h *leadHarness) minted() int {
	h.keys.mu.Lock()
	defer h.keys.mu.Unlock()
	return h.keys.n
}

// TestTakeoverWaitsForTheTerminalHarnessToExit: a takeover is refused while
// the terminal harness the row names runs, allowed once it is gone, and
// for a row that names no process allowed only when the session ended
// cleanly. A refusal changes nothing.
func TestTakeoverWaitsForTheTerminalHarnessToExit(t *testing.T) {
	h := newLeadHarness(t)
	procs := &processTable{}
	h.opts.Alive = procs.check
	h.start()
	procs.set(4001, true)
	procs.set(4002, false)
	cases := []struct {
		id      string
		edit    func(row *wsdashboard.Registration)
		refused string
	}{
		{id: "alivepid", edit: func(row *wsdashboard.Registration) { row.HarnessPID = 4001 }, refused: "pid 4001"},
		// A live process refuses even a row that says it ended.
		{id: "alivenoend", edit: func(row *wsdashboard.Registration) { row.HarnessPID, row.EndedAt = 4001, "" }, refused: "pid 4001"},
		{id: "deadpid", edit: func(row *wsdashboard.Registration) { row.HarnessPID = 4002 }},
		// The harness died without its ws-mcp marking the end.
		{id: "deadnoend", edit: func(row *wsdashboard.Registration) { row.HarnessPID, row.EndedAt = 4002, "" }},
		{id: "nopidended"},
		{id: "nopidopen", edit: func(row *wsdashboard.Registration) { row.EndedAt = "" }, refused: "did not end cleanly"},
	}
	for _, c := range cases {
		before := h.terminalSession(c.id, LeadSession{Key: "key-" + c.id, Harness: BackendClaude, Session: "sess-" + c.id, Cwd: h.root}, c.edit)
		st, err := h.runner.TakeOver(c.id)
		if c.refused != "" {
			if !isConflict(err) || !strings.Contains(err.Error(), c.refused) {
				t.Fatalf("%s: TakeOver = %v, want a conflict naming %q", c.id, err, c.refused)
			}
			if got := h.row(c.id); got != before {
				t.Fatalf("%s: a refused takeover rewrote the row:\n got %+v\nwant %+v", c.id, got, before)
			}
			if _, err := os.Stat(recordPath(wsdashboard.SessionDir(h.dir, c.id))); err == nil {
				t.Fatalf("%s: a refused takeover wrote a record", c.id)
			}
			if _, err := h.runner.Get(c.id); err == nil {
				t.Fatalf("%s: a refused takeover opened a conversation", c.id)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: TakeOver: %v", c.id, err)
		}
		if row := h.row(c.id); st.ID != c.id || !st.Lead || st.Status != StatusIdle || !row.Hosted || !row.HostStopped || row.EndedAt != "" || row.HarnessPID != 0 {
			t.Fatalf("%s: state %+v, row %+v", c.id, st, row)
		}
	}
	// The harness exits: the refused session can now be taken.
	procs.set(4001, false)
	if _, err := h.runner.TakeOver("alivepid"); err != nil {
		t.Fatalf("TakeOver after the harness exited: %v", err)
	}
	if h.holders.opCount("") != 0 {
		t.Fatalf("a takeover starts no process by itself: %v", h.holders.opList())
	}
}

// TestTakeoverAdoptsTheKeyAndResumesWhereTheHarnessRan: the daemon hosts
// a taken-over claude session under the session's own key (it mints none),
// resumes the harness conversation the key record names in the directory
// that harness ran in, and keeps that directory over a later start and a
// daemon restart.
func TestTakeoverAdoptsTheKeyAndResumesWhereTheHarnessRan(t *testing.T) {
	h := newLeadHarness(t).start()
	sub := filepath.Join(h.root, "pkg", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	const id, key = "termsess", "terminal-lead-key"
	h.terminalSession(id, LeadSession{Key: key, Harness: BackendClaude, Session: "sess-term", Cwd: sub}, func(row *wsdashboard.Registration) { row.HarnessPID = 4002 })
	h.mail.deliver(key, "From a@x: waiting since the terminal exited")

	st, err := h.runner.TakeOver(id)
	if err != nil {
		t.Fatal(err)
	}
	if st.ID != id || !st.Lead || st.Backend != BackendClaude || st.Profile != LeadProfileClaude || st.Root != h.root || st.Page != "/s/"+id {
		t.Fatalf("taken-over state = %+v", st)
	}
	rec := h.record(id)
	if rec.WSKey != key || rec.SessionID != "sess-term" || rec.Cwd != sub || rec.SessionFile != "" || !rec.Lead {
		t.Fatalf("record = %+v", rec)
	}
	if h.minted() != 0 || !slices.Contains(h.keys.ensured, key) {
		t.Fatalf("the session keeps its key (minted %d, ensured %v)", h.minted(), h.keys.ensured)
	}
	// Mail that waited is delivered to the resumed conversation.
	holder := h.holder(id)
	check := func(holder, when string) {
		t.Helper()
		spec := h.holders.spawned(holder)
		if resume, _ := flagValue(spec.Argv, "--resume"); resume != "sess-term" || spec.Cwd != sub {
			t.Fatalf("%s: spawn resumes %q in %q, want sess-term in %q: %v", when, resume, spec.Cwd, sub, spec.Argv)
		}
		if got, _ := specEnv(spec, EnvHostedSessionKey); got != key {
			t.Fatalf("%s: the harness runs on key %q, want %q", when, got, key)
		}
		var session holderSession
		for _, info := range h.holders.List().Holders {
			if info.ID == holder {
				_ = json.Unmarshal(info.Session, &session)
			}
		}
		if session.Cwd != sub || session.ReplyID != id {
			t.Fatalf("%s: holder descriptor = %+v", when, session)
		}
	}
	check(holder, "the first start")
	if got := h.userInputs(holder); len(got) != 1 || !strings.Contains(got[0], "waiting since the terminal exited") {
		t.Fatalf("the waiting mail should open the first turn: %q", got)
	}
	h.holders.out(holder, turnLines("sess-term", "u1", "back")...)
	h.idle(id)
	if row := h.row(id); !row.Hosted || row.HostPID == 0 || row.Project != h.root {
		t.Fatalf("row of the running session: %+v", row)
	}

	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	h.send(id, "again")
	eventually(t, "a second process", func() bool { return h.holders.opCount("spawn") == 2 })
	second := h.holder(id)
	check(second, "a later start")
	h.holders.out(second, turnLines("sess-term", "u2", "ok")...)
	h.idle(id)

	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	h.restart()
	h.send(id, "after the daemon restarted")
	eventually(t, "a third process", func() bool { return h.holders.opCount("spawn") == 3 })
	check(h.holder(id), "after a daemon restart")
}

// TestTakeoverOfAPiSessionRunsOnItsOwnSessionFile: a pi session is resumed
// on the session file its harness reported, in pi's own session directory
// for it, not on a file of the daemon's.
func TestTakeoverOfAPiSessionRunsOnItsOwnSessionFile(t *testing.T) {
	h := newLeadHarness(t).start()
	file := filepath.Join(t.TempDir(), "pi-sessions", "2026-11-12_abc.jsonl")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	const id = "pisess"
	h.terminalSession(id, LeadSession{Key: "pi-lead-key", Harness: BackendPi, Session: file, Cwd: h.root}, nil)
	if _, err := h.runner.TakeOver(id); !isConflict(err) || !strings.Contains(err.Error(), "file is gone") {
		t.Fatalf("TakeOver with no session file = %v, want a conflict", err)
	}
	if err := os.WriteFile(file, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := h.runner.TakeOver(id)
	if err != nil {
		t.Fatal(err)
	}
	if rec := h.record(id); st.Backend != BackendPi || st.PermissionMode != "" || rec.SessionFile != file || rec.Cwd != "" || rec.SessionID != "" {
		t.Fatalf("state %+v, record %+v", st, rec)
	}
	h.send(id, "hello")
	spec := h.holders.spawned(h.holder(id))
	if session, _ := flagValue(spec.Argv, "--session"); session != file || slices.Contains(spec.Argv, "--session-dir") || spec.Cwd != h.root {
		t.Fatalf("spawn = %v in %q, want --session %s and no --session-dir", spec.Argv, spec.Cwd, file)
	}
}

// TestTakeoverRefusals: what is no terminal lead session the daemon can
// host is refused, and a session with no recorded harness conversation
// says why.
func TestTakeoverRefusals(t *testing.T) {
	h := newLeadHarness(t).start()
	hosted := h.lead(BackendClaude, LeadOptions{}).ID
	if _, err := h.runner.TakeOver(hosted); !isConflict(err) {
		t.Fatalf("TakeOver of a hosted session = %v, want a conflict", err)
	}
	if _, err := h.runner.TakeOver("nosuchsession"); err != errUnknown {
		t.Fatalf("TakeOver of an unknown session = %v, want unknown", err)
	}
	h.terminalSession("nosess", LeadSession{Key: "k1", Harness: BackendClaude}, nil)
	if _, err := h.runner.TakeOver("nosess"); !isConflict(err) || !strings.Contains(err.Error(), TakeoverNoSession) {
		t.Fatalf("TakeOver with no recorded conversation = %v", err)
	}
	h.terminalSession("codexsess", LeadSession{Key: "k2", Harness: "codex", Session: "s", Cwd: h.root}, nil)
	if _, err := h.runner.TakeOver("codexsess"); err == nil || isConflict(err) || !strings.Contains(err.Error(), "claude or pi") {
		t.Fatalf("TakeOver of a codex session = %v", err)
	}
	h.terminalSession("gonedir", LeadSession{Key: "k3", Harness: BackendClaude, Session: "s", Cwd: filepath.Join(h.root, "removed")}, nil)
	if _, err := h.runner.TakeOver("gonedir"); !isConflict(err) || !strings.Contains(err.Error(), "is gone") {
		t.Fatalf("TakeOver into a removed directory = %v", err)
	}
	// A session whose lead key is gone (a plain-folder session never had
	// one).
	stamp := time.Now().UTC().Format(time.RFC3339)
	if err := wsdashboard.WriteRegistration(h.dir, wsdashboard.Registration{ReplyID: "nokey", Project: h.root, Harness: BackendClaude, EndedAt: stamp}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.TakeOver("nokey"); !isConflict(err) || !strings.Contains(err.Error(), "no lead key") {
		t.Fatalf("TakeOver without a lead key = %v", err)
	}
	if n := len(h.runner.List()); n != 1 {
		t.Fatalf("refused takeovers opened conversations: %d", n)
	}
}

// TestHandOffGivesTheSessionToTheTerminal: handing a hosted session to the
// terminal ends its process, turns its row into an ended terminal row,
// answers the command that resumes it, stops resuming it on mail, and is
// not undone by a daemon restart. The session then comes back through a
// takeover, on the same record.
func TestHandOffGivesTheSessionToTheTerminal(t *testing.T) {
	h := newLeadHarness(t).start()
	fresh := h.lead(BackendClaude, LeadOptions{}).ID
	if _, err := h.runner.HandOff(fresh); !isConflict(err) {
		t.Fatalf("HandOff of a session with no conversation = %v, want a conflict", err)
	}
	if st := h.state(fresh); st.Status != StatusIdle {
		t.Fatalf("a refused hand-off changed the session: %+v", st)
	}
	// Closed, so the mail claims counted below are the other session's.
	if _, err := h.runner.CloseConversation(fresh); err != nil {
		t.Fatal(err)
	}

	id, holder := h.runLead("one")
	key := h.record(id).WSKey
	h.holders.out(holder, turnLines("sess-1", "u1", "done")...)
	h.idle(id)
	st, err := h.runner.HandOff(id)
	if err != nil {
		t.Fatal(err)
	}
	want := "cd " + h.root + " && claude --resume sess-1"
	if st.Status != StatusHandedOff || st.Process || st.ResumeCommand != want {
		t.Fatalf("handed-off state = %+v, want the resume command %q", st, want)
	}
	if h.holders.opCount("stop "+holder) != 1 {
		t.Fatalf("the process should be ended: %v", h.holders.opList())
	}
	row := h.row(id)
	if row.Hosted || row.HostPID != 0 || row.HostStopped || row.HarnessPID != 0 || row.EndedAt == "" || row.Project != h.root || row.Harness != BackendClaude {
		t.Fatalf("the row should be an ended terminal row: %+v", row)
	}
	if rec := h.record(id); rec.HandedOffAt == "" || rec.ClosedAt != "" || rec.HolderID != "" || rec.SessionID != "sess-1" {
		t.Fatalf("record = %+v", rec)
	}
	eventually(t, "the mail watch to end", func() bool { return !h.mail.watched(key) })
	spawns, drains := h.holders.opCount("spawn"), h.mail.drainCount()
	check := func(when string) {
		t.Helper()
		h.mail.deliver(key, "From a@x: "+when)
		time.Sleep(4 * h.opts.heartbeat)
		if got := h.row(id); got != row {
			t.Fatalf("%s: the row was rewritten:\n got %+v\nwant %+v", when, got, row)
		}
		if h.holders.opCount("spawn") != spawns || h.mail.drainCount() != drains {
			t.Fatalf("%s: mail resumed a handed-off session: %v, %d claims", when, h.holders.opList(), h.mail.drainCount())
		}
		if st := h.state(id); st.Status != StatusHandedOff || st.ResumeCommand != want {
			t.Fatalf("%s: state %+v", when, st)
		}
	}
	check("after the hand-off")
	if _, err := h.runner.Reopen(id); !isConflict(err) {
		t.Fatalf("Reopen of a handed-off session = %v, want a conflict", err)
	}
	h.restart()
	check("after a daemon restart")

	// The resume command reaches only a viewer on this machine.
	if st := decodeState(t, h.route(lanBrowser, http.MethodGet, "/agents/"+id, nil)); st.Status != StatusHandedOff || st.ResumeCommand != "" {
		t.Fatalf("GET from another device: %+v", st)
	}
	if st := decodeState(t, h.route(localBrowser, http.MethodGet, "/agents/"+id, nil)); st.ResumeCommand != want {
		t.Fatalf("GET from this machine: %+v", st)
	}

	// The terminal ran it and exited; the daemon takes it back.
	h.terminalSession(id, LeadSession{Key: key, Harness: BackendClaude, Session: "sess-1", Cwd: h.root}, nil)
	back, err := h.runner.TakeOver(id)
	if err != nil {
		t.Fatal(err)
	}
	if rec := h.record(id); back.Status != StatusIdle || rec.HandedOffAt != "" || rec.WSKey != key || rec.CreatedAt == "" || h.minted() != 2 {
		t.Fatalf("taken back: state %+v, record %+v, minted %d", back, rec, h.minted())
	}
	eventually(t, "the mail that waited to resume the session", func() bool { return h.holders.opCount("spawn") == spawns+1 })
	if resume, _ := flagValue(h.holders.spawned(h.holder(id)).Argv, "--resume"); resume != "sess-1" {
		t.Fatalf("the session should resume its conversation: %v", h.holders.spawned(h.holder(id)).Argv)
	}
}

// TestHandOffThroughTheRoute: the page's hand-to-terminal entry.
func TestHandOffThroughTheRoute(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess 1", "u1", "done")...)
	h.idle(id)
	r := h.route(localBrowser, http.MethodPost, "/agents/"+id+"/handoff", nil)
	if st := decodeState(t, r); r.status != http.StatusOK || st.Status != StatusHandedOff || st.ResumeCommand != "cd "+h.root+" && claude --resume 'sess 1'" {
		t.Fatalf("handoff = %d %s", r.status, r.body)
	}
	if r := h.route(localBrowser, http.MethodPost, "/agents/"+id+"/handoff", nil); r.status != http.StatusConflict {
		t.Fatalf("a second handoff = %d %s, want 409", r.status, r.body)
	}
}

// TestReopenOpensAClosedHostedSession: a closed hosted session is open
// again under its key and conversation; once a terminal login took the
// closed session's row, reopen is refused and the session comes back
// through a takeover instead.
func TestReopenOpensAClosedHostedSession(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	key := h.record(id).WSKey
	h.holders.out(holder, turnLines("sess-1", "u1", "done")...)
	h.idle(id)
	if _, err := h.runner.Reopen(id); !isConflict(err) {
		t.Fatalf("Reopen of an open session = %v, want a conflict", err)
	}
	if _, err := h.runner.CloseConversation(id); err != nil {
		t.Fatal(err)
	}
	if row := h.row(id); !row.Hosted || row.EndedAt == "" {
		t.Fatalf("a closed session's row: %+v", row)
	}
	// Closed sessions stay closed over a daemon restart, and reopen there.
	h.restart()
	r := h.route(localBrowser, http.MethodPost, "/agents/"+id+"/reopen", nil)
	st := decodeState(t, r)
	if r.status != http.StatusOK || st.Status != StatusIdle || st.ClosedAt != "" || !st.CanControl {
		t.Fatalf("reopen = %d %s", r.status, r.body)
	}
	if rec, row := h.record(id), h.row(id); rec.ClosedAt != "" || rec.WSKey != key || !row.Hosted || row.EndedAt != "" || !row.HostStopped {
		t.Fatalf("reopened: record %+v, row %+v", rec, row)
	}
	if !h.mail.watched(key) {
		t.Fatal("a reopened session's mailbox is watched again")
	}
	h.send(id, "two")
	eventually(t, "the reopened session's process", func() bool { return h.record(id).HolderID != "" })
	if resume, _ := flagValue(h.holders.spawned(h.holder(id)).Argv, "--resume"); resume != "sess-1" {
		t.Fatalf("a reopened session resumes its conversation: %v", h.holders.spawned(h.holder(id)).Argv)
	}
	h.holders.out(h.holder(id), turnLines("sess-1", "u2", "ok")...)
	h.idle(id)

	// Closed again, then a terminal login takes its row.
	if _, err := h.runner.CloseConversation(id); err != nil {
		t.Fatal(err)
	}
	taken := h.terminalTakes(id)
	if _, err := h.runner.Reopen(id); !isConflict(err) || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("Reopen after a terminal took the session = %v, want a conflict", err)
	}
	if got := h.row(id); got != taken {
		t.Fatalf("a refused reopen rewrote the terminal's row:\n got %+v\nwant %+v", got, taken)
	}
	if st := h.state(id); st.Status != StatusClosed {
		t.Fatalf("a refused reopen changed the session: %+v", st)
	}
	if _, err := h.runner.Reopen("nosuchsession"); err != errUnknown {
		t.Fatalf("Reopen of an unknown session = %v", err)
	}
}

// TestResumeCommand pins the command a handed-off session shows.
func TestResumeCommand(t *testing.T) {
	dir := t.TempDir()
	own := piSessionFile(dir)
	cases := []struct {
		rec  Record
		want string
	}{
		{Record{Lead: true, Backend: BackendClaude, Root: "/w/repo", SessionID: "abc-1"}, "cd /w/repo && claude --resume abc-1"},
		{Record{Lead: true, Backend: BackendClaude, Root: "/w/repo", Cwd: "/w/my repo/it's", SessionID: "abc-1"}, `cd '/w/my repo/it'\''s' && claude --resume abc-1`},
		{Record{Lead: true, Backend: BackendClaude, Root: "/w/repo"}, ""},
		{Record{Lead: true, Backend: BackendPi, Root: "/w/repo", SessionFile: "/h/.pi/s 1.jsonl"}, "cd /w/repo && pi --session '/h/.pi/s 1.jsonl'"},
		// A pi session the daemon started has no file until its first run.
		{Record{Lead: true, Backend: BackendPi, Root: "/w/repo"}, ""},
		{Record{Backend: BackendClaude, Root: "/w/repo", SessionID: "abc-1"}, ""},
	}
	for _, c := range cases {
		if got := resumeCommand(c.rec, dir); got != c.want {
			t.Errorf("resumeCommand(%+v) = %q, want %q", c.rec, got, c.want)
		}
	}
	if err := os.MkdirAll(filepath.Dir(own), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(own, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, want := resumeCommand(Record{Lead: true, Backend: BackendPi, Root: "/w/repo"}, dir), "cd /w/repo && pi --session "+own; got != want {
		t.Errorf("a daemon-started pi session = %q, want %q", got, want)
	}
}
