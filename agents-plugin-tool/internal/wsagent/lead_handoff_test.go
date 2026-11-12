package wsagent

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// terminalTakes rewrites the session's row as a terminal harness's login
// does once it found no process of the daemon's (the login guard of
// ws-mcp): under the row lock, into an unhosted row that names its own
// harness process. It returns the row it wrote.
func (h *leadHarness) terminalTakes(id string) wsdashboard.Registration {
	h.t.Helper()
	unlock, err := wsdashboard.LockRegistration(h.dir, id)
	if err != nil {
		h.t.Fatal(err)
	}
	defer unlock()
	row := readEventually(h.t, "read the row", func() (wsdashboard.Registration, error) { return wsdashboard.ReadRegistration(h.dir, id) })
	if !row.Hosted || row.HostMayRun(func(int) bool { return true }) {
		h.t.Fatalf("a terminal login would be refused on this row: %+v", row)
	}
	row.Hosted, row.HostPID, row.HostStopped, row.HarnessPID = false, 0, false, 9001
	row.Harness = "claude"
	if err := wsdashboard.WriteRegistration(h.dir, row); err != nil {
		h.t.Fatal(err)
	}
	return row
}

func (h *leadHarness) row(id string) wsdashboard.Registration {
	h.t.Helper()
	return readEventually(h.t, "read the row", func() (wsdashboard.Registration, error) { return wsdashboard.ReadRegistration(h.dir, id) })
}

// TestLeadRowSaysWhetherAProcessMayRun: the row a terminal login reads is
// a stopped one only while the session has no process and none is starting.
func TestLeadRowSaysWhetherAProcessMayRun(t *testing.T) {
	h := newLeadHarness(t).start()
	id := h.lead(BackendClaude, LeadOptions{}).ID
	alive := func(int) bool { return true }
	if row := h.row(id); !row.Hosted || !row.HostStopped || row.HostMayRun(alive) {
		t.Fatalf("a session that never started reads as stopped: %+v", row)
	}
	h.send(id, "one")
	holder := h.holder(id)
	eventually(t, "the row to name the process", func() bool { row := h.row(id); return row.HostPID != 0 && !row.HostStopped })
	if row := h.row(id); !row.HostMayRun(alive) {
		t.Fatalf("a running session's row: %+v", row)
	}
	h.holders.out(holder, turnLines("s1", "u1", "done")...)
	h.idle(id)
	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	if row := h.row(id); row.HostPID != 0 || !row.HostStopped || row.HostMayRun(alive) {
		t.Fatalf("a stopped session's row: %+v", row)
	}

	// A start that failed leaves a stopped row too, not a spawn under way.
	h.holders.set(func() { h.holders.spawnErr = errors.New("no room") })
	h.send(id, "two")
	eventually(t, "the failed start", func() bool { return h.state(id).Error != "" })
	if row := h.row(id); !row.HostStopped || row.HostPID != 0 {
		t.Fatalf("the row after a failed start: %+v", row)
	}
}

// TestLeadHandedToATerminalIsLetGo: once a terminal harness took the row
// of a stopped session, the daemon writes the row no more, claims no mail,
// starts no process for mail or a page message, gives back the mail it had
// claimed and not delivered, and does not take the session back when it
// starts again.
func TestLeadHandedToATerminalIsLetGo(t *testing.T) {
	h := newLeadHarness(t).start()
	id := h.lead(BackendClaude, LeadOptions{}).ID
	key := h.record(id).WSKey
	// Claimed mail no process took: the start fails.
	h.holders.set(func() { h.holders.spawnErr = errors.New("no room") })
	h.mail.deliver(key, "From a@x: ping")
	eventually(t, "the claimed mail to wait in the record", func() bool {
		return slices.Equal(h.record(id).Mail, []string{"From a@x: ping"}) && h.row(id).HostStopped
	})
	h.holders.set(func() { h.holders.spawnErr = nil })
	spawns, launches := h.holders.opCount("spawn"), h.holders.opCount("launch")

	taken := h.terminalTakes(id)
	eventually(t, "the session to be let go", func() bool { return h.state(id).Status == StatusHandedOff })
	rec := h.record(id)
	if rec.HandedOffAt == "" || rec.ClosedAt != "" || len(rec.Mail) != 0 {
		t.Fatalf("the record of a handed-off session: %+v", rec)
	}
	if got := h.mail.waiting(key); !slices.Equal(got, []string{"From a@x: ping"}) {
		t.Fatalf("claimed mail goes back to the mailbox, which holds %q", got)
	}
	eventually(t, "the mail watch to end", func() bool { return h.mail.stopped() == 1 && !h.mail.watched(key) })

	// Mail and page messages after the hand-off reach no process here.
	drains := h.mail.drainCount()
	h.mail.deliver(key, "From b@x: more")
	if _, err := h.runner.Send(id, "from the page"); !isConflict(err) {
		t.Fatalf("a message to a handed-off session = %v, want a conflict", err)
	}
	if _, err := h.runner.Restart(id); !isConflict(err) {
		t.Fatalf("a restart of a handed-off session = %v, want a conflict", err)
	}
	check := func(when string) {
		t.Helper()
		if got := h.row(id); got != taken {
			t.Fatalf("%s: the terminal's row was rewritten:\n got %+v\nwant %+v", when, got, taken)
		}
		if got := h.mail.drainCount(); got != drains {
			t.Fatalf("%s: mail was claimed for a handed-off session (%d claims, want %d)", when, got, drains)
		}
		if got := h.mail.waiting(key); !slices.Equal(got, []string{"From a@x: ping", "From b@x: more"}) {
			t.Fatalf("%s: the mailbox holds %q", when, got)
		}
		if h.holders.opCount("spawn") != spawns || h.holders.opCount("launch") != launches {
			t.Fatalf("%s: a process was started for a handed-off session: %v", when, h.holders.opList())
		}
		if h.mail.watched(key) {
			t.Fatalf("%s: the mailbox of a handed-off session is watched", when)
		}
		if st := h.state(id); st.Status != StatusHandedOff || st.HandedOffAt == "" || st.Process {
			t.Fatalf("%s: state %+v", when, st)
		}
	}
	// Several heartbeats pass.
	h.runner.InboxChanged(id)
	time.Sleep(6 * h.opts.heartbeat)
	check("after the hand-off")

	// The next daemon does not take the session back.
	h.restart()
	check("after a daemon restart")
	h.mail.deliver(key, "From c@x: again")
	if got := h.mail.drainCount(); got != drains {
		t.Fatalf("mail was claimed after the restart (%d claims, want %d)", got, drains)
	}
}

// TestLeadRowTakenBeforeASpawnStartsNoProcess: a page message that arrives
// between the terminal's login and the daemon noticing it must not start a
// second holder of the conversation.
func TestLeadRowTakenBeforeASpawnStartsNoProcess(t *testing.T) {
	h := newLeadHarness(t)
	// No heartbeat notices the row first: the spawn's own check must.
	h.opts.heartbeat = 0
	h.start()
	id := h.lead(BackendClaude, LeadOptions{}).ID
	taken := h.terminalTakes(id)
	// The message is accepted (nothing noticed yet) or refused; either
	// way no process starts.
	_, _ = h.runner.Send(id, "from the page")
	eventually(t, "the session to be let go", func() bool { return h.state(id).Status == StatusHandedOff })
	if h.holders.opCount("spawn") != 0 || h.holders.opCount("launch") != 0 {
		t.Fatalf("a process was started on a row the terminal holds: %v", h.holders.opList())
	}
	if got := h.row(id); got != taken {
		t.Fatalf("the terminal's row was rewritten:\n got %+v\nwant %+v", got, taken)
	}
}

// TestLeadRowTakenWhileNoDaemonRan: a session whose process died while no
// daemon ran, and which a terminal then resumed, is not rejoined, watched,
// or rewritten by the next daemon.
func TestLeadRowTakenWhileNoDaemonRan(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	key := h.record(id).WSKey
	h.holders.out(holder, turnLines("s1", "u1", "done")...)
	h.idle(id)
	h.runner.Close()
	h.holders.restart()
	h.holders.vanish(holder)
	// The row still names the process that is gone; the login guard takes
	// it on finding the process dead.
	unlock, err := wsdashboard.LockRegistration(h.dir, id)
	if err != nil {
		t.Fatal(err)
	}
	taken := h.row(id)
	if taken.HostPID == 0 || taken.HostMayRun(func(int) bool { return false }) {
		unlock()
		t.Fatalf("precondition: the row names a process that is gone: %+v", taken)
	}
	taken.Hosted, taken.HostPID, taken.HarnessPID = false, 0, 9001
	if err := wsdashboard.WriteRegistration(h.dir, taken); err != nil {
		unlock()
		t.Fatal(err)
	}
	unlock()
	stops, drains := h.mail.stopped(), h.mail.drainCount()
	ops := h.holders.opList()

	h.mail.deliver(key, "From a@x: ping")
	h.start()
	eventually(t, "the session to be let go", func() bool { return h.state(id).Status == StatusHandedOff })
	if got := h.row(id); got != taken {
		t.Fatalf("the terminal's row was rewritten:\n got %+v\nwant %+v", got, taken)
	}
	if h.mail.watched(key) || h.mail.drainCount() != drains || h.mail.stopped() != stops {
		t.Fatalf("the mailbox of a session the terminal holds was watched or claimed (watched %v, claims %d, want %d)", h.mail.watched(key), h.mail.drainCount(), drains)
	}
	if got := h.holders.opList(); !slices.Equal(got, ops) {
		t.Fatalf("the next daemon touched the session's holder: %v, before %v", got, ops)
	}
	if rec := h.record(id); rec.HandedOffAt == "" || rec.HolderID != "" {
		t.Fatalf("the record: %+v", rec)
	}
}

// TestLeadRowOfAnEarlierDaemonIsNotATerminals: a daemon of an earlier
// release knows no hosted lead; while it ran it served the session as an
// agent conversation and rewrote its row as one (Agent, no Hosted). The
// next daemon must take that row back, not read it as a terminal login and
// let the session go, which ends the process still running under its
// holder.
func TestLeadRowOfAnEarlierDaemonIsNotATerminals(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("s1", "u1", "done")...)
	h.idle(id)
	h.runner.Close()
	h.holders.restart()
	rec := h.record(id)
	stamp := time.Now().UTC().Format(time.RFC3339)
	old := wsdashboard.Registration{ReplyID: id, Project: rec.Root, Harness: rec.Backend, RegisteredAt: rec.CreatedAt, HeartbeatAt: stamp, Agent: true, Unlisted: !rec.Listed}
	if err := wsdashboard.WriteRegistration(h.dir, old); err != nil {
		t.Fatal(err)
	}

	h.start()
	eventually(t, "the row to be the daemon's again", func() bool {
		row := h.row(id)
		return row.Hosted && !row.Agent && row.HostPID == rec.HostPID
	})
	h.send(id, "two")
	eventually(t, "the rejoined process to take the message", func() bool { return slices.Equal(h.userInputs(holder), []string{"one", "two"}) })
	if st, rec := h.state(id), h.record(id); st.Status == StatusHandedOff || rec.HandedOffAt != "" || h.holders.opCount("stop") != 0 {
		t.Fatalf("the session was let go: %+v, record %+v, holder calls %v", st, rec, h.holders.opList())
	}
}
