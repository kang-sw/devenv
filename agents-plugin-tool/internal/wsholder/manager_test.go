package wsholder

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests run real holders: the embedded script under the python3 (or
// python) on PATH, started detached as the daemon starts them. Every holder
// a test leaves behind is killed by its descriptor's PID in cleanup.

// testPython is the interpreter the tests run holders under.
func testPython(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			if out, err := exec.Command(path, "-c", "import sys; print(sys.version_info >= (3, 10))").Output(); err == nil && strings.TrimSpace(string(out)) == "True" {
				return path
			}
		}
	}
	t.Skip("no python3 (3.10 or newer) on PATH")
	return ""
}

// tickChild is a pipe child that echoes each stdin line as "echo:<line>",
// prints "tick <n>" every 50 ms on its own, and ends on "quit" or after a
// minute, so a leaked one does not stay.
const tickChild = `
import sys, threading, time
def ticks():
    n = 0
    while n < 1200:
        n += 1
        print("tick %d" % n, flush=True)
        time.sleep(0.05)
    sys.exit(0)
threading.Thread(target=ticks, daemon=True).start()
for line in sys.stdin:
    line = line.strip()
    if line == "quit":
        break
    print("echo:" + line, flush=True)
sys.exit(7)
`

// collector is a Sink that keeps every event per holder.
type collector struct {
	mu     sync.Mutex
	events map[string][]Event
}

func newCollector() *collector { return &collector{events: map[string][]Event{}} }

func (c *collector) HolderEvent(id string, ev Event) {
	c.mu.Lock()
	c.events[id] = append(c.events[id], ev)
	c.mu.Unlock()
}

func (c *collector) snapshot(id string) []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Event(nil), c.events[id]...)
}

// output is the bytes of id's output records so far.
func (c *collector) output(id string) string {
	var b bytes.Buffer
	for _, ev := range c.snapshot(id) {
		if ev.Event == "output" {
			b.Write(ev.Data)
		}
	}
	return b.String()
}

// seqs are the sequence numbers of id's records so far, in arrival order.
func (c *collector) seqs(id string) []uint64 {
	var out []uint64
	for _, ev := range c.snapshot(id) {
		if ev.Seq > 0 {
			out = append(out, ev.Seq)
		}
	}
	return out
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// testHoldersDir is a holders directory whose leftover holders are killed
// when the test ends.
func testHoldersDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), DirName)
	t.Cleanup(func() {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			// A test may write a descriptor that names its own process.
			if desc, err := ReadDescriptor(filepath.Join(dir, entry.Name())); err == nil && desc.PID != os.Getpid() && processAlive(desc.PID) {
				_ = killHolder(desc.PID)
			}
		}
	})
	return dir
}

// testManager is a manager on dir with short holder periods; it is closed
// when the test ends.
func testManager(t *testing.T, dir string, sink Sink, mutate ...func(*Options)) *Manager {
	t.Helper()
	opts := Options{
		Dir:        dir,
		Python:     testPython(t),
		Sink:       sink,
		holderArgs: []string{"--idle-exit-seconds", "60", "--stop-grace-seconds", "1"},
	}
	for _, f := range mutate {
		f(&opts)
	}
	m := New(opts)
	t.Cleanup(m.Close)
	return m
}

func spawnTicker(t *testing.T, m *Manager, id string) ChildStatus {
	t.Helper()
	child, err := m.Client(id).Spawn(SpawnSpec{Argv: []string{testPython(t), "-c", tickChild}, Mode: "pipe"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if child.State != "running" || child.PID <= 0 {
		t.Fatalf("spawned child = %+v, want it running", child)
	}
	return child
}

func contiguousFrom(seqs []uint64, first uint64) bool {
	for i, s := range seqs {
		if s != first+uint64(i) {
			return false
		}
	}
	return len(seqs) > 0
}

// TestChildSurvivesADaemonRestart pins the holder's purpose: the child of a
// holder keeps running while one manager (a daemon) closes and another
// starts, the successor finds the holder through its descriptor and replays
// the whole ring, a reattach inside one manager continues after the last
// sequence it saw, and a client that kept its own last sequence across the
// restart receives no record twice.
func TestChildSurvivesADaemonRestart(t *testing.T) {
	dir := testHoldersDir(t)
	first := newCollector()
	m1 := testManager(t, dir, first)
	id, err := m1.Launch(json.RawMessage(`{"kind":"test","label":"ticker"}`))
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	child := spawnTicker(t, m1, id)
	if err := m1.Client(id).Input([]byte("one\n"), false); err != nil {
		t.Fatalf("Input: %v", err)
	}
	waitFor(t, "the echo on the first daemon", func() bool { return strings.Contains(first.output(id), "echo:one") })

	// The daemon goes away; the child keeps producing output meanwhile.
	m1.Close()
	seen := first.seqs(id)
	if !contiguousFrom(seen, 1) {
		t.Fatalf("first daemon's sequences = %v, want 1.. without holes", seen)
	}
	keptLast := seen[len(seen)-1]
	time.Sleep(300 * time.Millisecond)

	second := newCollector()
	m2 := testManager(t, dir, second)
	m2.Scan()
	listing := m2.List()
	if len(listing.Holders) != 1 || !listing.Holders[0].Attached || listing.Holders[0].ID != id {
		t.Fatalf("after the restart the listing = %+v, want holder %s attached", listing, id)
	}
	if got := listing.Holders[0]; got.Child == nil || got.Child.State != "running" || got.Child.PID != child.PID {
		t.Fatalf("after the restart the child = %+v, want pid %d still running", got.Child, child.PID)
	}
	if string(listing.Holders[0].Session) != `{"kind": "test", "label": "ticker"}` && !strings.Contains(string(listing.Holders[0].Session), `"ticker"`) {
		t.Errorf("session descriptor = %s", listing.Holders[0].Session)
	}
	// A new daemon process replays the whole ring: what the first saw, then
	// what the child wrote while nobody was attached.
	waitFor(t, "the replay on the second daemon", func() bool {
		s := second.seqs(id)
		return len(s) > 0 && s[len(s)-1] > keptLast+3
	})
	if s := second.seqs(id); !contiguousFrom(s, 1) {
		t.Fatalf("second daemon's sequences = %v, want the whole ring from 1 without holes", s)
	}
	if out := second.output(id); !strings.HasPrefix(out, first.output(id)) {
		t.Fatalf("the replay does not begin with what the first daemon saw")
	}

	// A dropped connection inside one daemon process: the reattach continues
	// after the last sequence it saw.
	m2.Client(id).Close()
	waitFor(t, "the drop to show", func() bool { return m2.Client(id) == nil })
	before := len(second.seqs(id))
	m2.Scan()
	waitFor(t, "records after the reattach", func() bool { return len(second.seqs(id)) > before+3 })
	if s := second.seqs(id); !contiguousFrom(s, 1) {
		t.Fatalf("sequences across the reattach = %v, want no duplicate and no hole", s)
	}

	// A client outside the daemon that kept its last sequence across the
	// restart asks for what follows it and gets no duplicate.
	desc, err := ReadDescriptor(filepath.Join(dir, id))
	if err != nil {
		t.Fatal(err)
	}
	outside := newCollector()
	client, _, err := Attach(context.Background(), desc, func(ev Event) { outside.HolderEvent(id, ev) })
	if err != nil {
		t.Fatalf("outside attach: %v", err)
	}
	defer client.Close()
	if _, _, err := client.Replay(keptLast + 1); err != nil {
		t.Fatalf("outside replay: %v", err)
	}
	waitFor(t, "the outside client's records", func() bool { return len(outside.seqs(id)) > 3 })
	if s := outside.seqs(id); !contiguousFrom(s, keptLast+1) {
		t.Fatalf("outside client's sequences = %v, want them to start at %d without holes", s, keptLast+1)
	}
	for _, ev := range outside.snapshot(id) {
		if ev.Event == "gap" {
			t.Fatalf("outside client got a gap %+v with its sequence still in the ring", ev)
		}
	}
	// The outside attach took the slot over; the daemon's connection ended.
	waitFor(t, "the takeover to detach the daemon", func() bool { return m2.Client(id) == nil })
	client.Close()

	// The daemon reattaches on its next scan and stops the holder through
	// the protocol: the child ends and the holder removes its directory.
	m2.Scan()
	if m2.Client(id) == nil {
		t.Fatalf("the daemon did not reattach: %+v", m2.List())
	}
	if err := m2.Stop(id); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if processAlive(desc.PID) || processAlive(child.PID) {
		t.Errorf("after the stop: holder alive %v, child alive %v", processAlive(desc.PID), processAlive(child.PID))
	}
	if _, err := os.Stat(filepath.Join(dir, id)); !os.IsNotExist(err) {
		t.Errorf("the holder directory still exists after the stop: %v", err)
	}
	if got := m2.List().Holders; len(got) != 0 {
		t.Errorf("listing after the stop = %+v, want none", got)
	}
}

// launchWithChild starts a holder with a running ticker child through a
// manager that is then closed, as a daemon that has gone away leaves it.
func launchWithChild(t *testing.T, dir string) (Descriptor, ChildStatus) {
	t.Helper()
	m := testManager(t, dir, nil)
	id, err := m.Launch(nil)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	child := spawnTicker(t, m, id)
	desc, err := ReadDescriptor(filepath.Join(dir, id))
	if err != nil {
		t.Fatal(err)
	}
	m.Close()
	return desc, child
}

// rewriteDescriptor changes fields of a holder's descriptor on disk.
func rewriteDescriptor(t *testing.T, dir, id string, fields map[string]any) {
	t.Helper()
	path := filepath.Join(dir, id, descriptorName)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for k, v := range fields {
		doc[k] = v
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

// closedPort is a loopback port nothing listens on.
func closedPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// unattachable makes the running holder desc one a daemon cannot attach to,
// for the given cause.
func unattachable(t *testing.T, dir string, desc Descriptor, cause Cause) {
	t.Helper()
	switch cause {
	case CauseNoAnswer:
		rewriteDescriptor(t, dir, desc.ID, map[string]any{"port": closedPort(t)})
	case CauseToken:
		rewriteDescriptor(t, dir, desc.ID, map[string]any{"token": "not-the-token"})
	case CauseProtocol:
		// A daemon two versions ahead of the holder no longer speaks its
		// protocol.
		old := protocolVersion
		protocolVersion = old + 2
		t.Cleanup(func() { protocolVersion = old })
	}
}

// TestUnattachedHolderIsListedWithItsCauseAndKilledUnderTheGuard pins the
// three ways a live holder stays unattached, each listed with its cause,
// and that stopping one kills it under the start-time guard, its child
// included, and removes its directory.
func TestUnattachedHolderIsListedWithItsCauseAndKilledUnderTheGuard(t *testing.T) {
	for _, cause := range []Cause{CauseNoAnswer, CauseProtocol, CauseToken} {
		t.Run(string(cause), func(t *testing.T) {
			dir := testHoldersDir(t)
			desc, child := launchWithChild(t, dir)
			unattachable(t, dir, desc, cause)

			m := testManager(t, dir, nil)
			m.Scan()
			listing := m.List()
			if len(listing.Holders) != 1 {
				t.Fatalf("listing = %+v, want the one holder", listing)
			}
			got := listing.Holders[0]
			if got.Attached || got.Cause != cause || got.ID != desc.ID || got.PID != desc.PID {
				t.Fatalf("holder = %+v, want %s unattached with cause %q", got, desc.ID, cause)
			}
			if !processAlive(desc.PID) || !processAlive(child.PID) {
				t.Fatalf("a failed attach ended something: holder alive %v, child alive %v", processAlive(desc.PID), processAlive(child.PID))
			}

			if err := m.Stop(desc.ID); err != nil {
				t.Fatalf("Stop: %v", err)
			}
			if processAlive(desc.PID) {
				t.Errorf("the holder (pid %d) is alive after the kill", desc.PID)
			}
			waitFor(t, "the child to end with its holder", func() bool { return !processAlive(child.PID) })
			if _, err := os.Stat(filepath.Join(dir, desc.ID)); !os.IsNotExist(err) {
				t.Errorf("the holder directory still exists after the kill: %v", err)
			}
			if rest := m.List().Holders; len(rest) != 0 {
				t.Errorf("listing after the kill = %+v, want none", rest)
			}
		})
	}
}

// TestGuardedKillIsRefusedOnAStartTimeMismatch pins the guard itself: when
// the process under a holder's PID did not start when the descriptor says,
// the PID may have been reused, so nothing is killed and the holder stays
// listed.
func TestGuardedKillIsRefusedOnAStartTimeMismatch(t *testing.T) {
	dir := testHoldersDir(t)
	desc, child := launchWithChild(t, dir)
	unattachable(t, dir, desc, CauseToken)

	killed := false
	m := testManager(t, dir, nil, func(o *Options) {
		// Both clocks disagree with the descriptor, so the refusal holds
		// whichever one this OS matches on.
		o.procStart = func(pid int) (time.Time, error) {
			real, err := processStartTime(pid)
			return real.Add(startTimeTolerance + time.Second), err
		}
		o.procTicks = func(pid int) (uint64, error) {
			real, err := processStartTicks(pid)
			if real != 0 {
				real++
			}
			return real, err
		}
		o.kill = func(pid int) error { killed = true; return killHolder(pid) }
	})
	m.Scan()
	if err := m.Stop(desc.ID); !errors.Is(err, ErrStartTimeMismatch) {
		t.Fatalf("Stop = %v, want ErrStartTimeMismatch", err)
	}
	if killed || !processAlive(desc.PID) || !processAlive(child.PID) {
		t.Fatalf("a refused kill ended something: kill called %v, holder alive %v, child alive %v", killed, processAlive(desc.PID), processAlive(child.PID))
	}
	if rest := m.List().Holders; len(rest) != 1 || rest[0].ID != desc.ID {
		t.Errorf("listing after the refusal = %+v, want the holder still listed", rest)
	}
	if _, err := ReadDescriptor(filepath.Join(dir, desc.ID)); err != nil {
		t.Errorf("the descriptor is gone after a refused kill: %v", err)
	}
}

// TestScanClearsADescriptorWhoseProcessIsAnother pins the cleanup of a
// holder that died without removing its directory: once its PID belongs to
// another process, which neither answers the endpoint nor started when the
// descriptor says, the scan removes the directory and lists nothing. A
// holder that merely does not answer is kept
// (TestUnattachedHolderIsListedWithItsCauseAndKilledUnderTheGuard).
func TestScanClearsADescriptorWhoseProcessIsAnother(t *testing.T) {
	dir := testHoldersDir(t)
	holderDir := filepath.Join(dir, "departed")
	if err := os.MkdirAll(holderDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// The PID is alive (this test's own process) but started long after
	// the recorded start.
	desc, _ := json.Marshal(Descriptor{Schema: 1, ID: "departed", PID: os.Getpid(), Port: closedPort(t), Token: "t", Protocol: 1, StartedAt: 1, ScriptHash: "h"})
	if err := os.WriteFile(filepath.Join(holderDir, descriptorName), desc, 0o600); err != nil {
		t.Fatal(err)
	}

	killed := false
	m := testManager(t, dir, nil, func(o *Options) {
		o.kill = func(int) error { killed = true; return nil }
	})
	m.Scan()
	if rest := m.List().Holders; len(rest) != 0 {
		t.Errorf("listing = %+v, want the departed holder cleared", rest)
	}
	if _, err := os.Stat(holderDir); !os.IsNotExist(err) {
		t.Errorf("the departed holder's directory still exists: %v", err)
	}
	if killed {
		t.Error("clearing a departed holder killed the process that now has its PID")
	}
}

// TestGuardedKillHoldsAfterAWallClockStep pins the boot-clock match: where
// the OS derives a creation time from the boot clock, a wall clock that
// stepped after the holder started moves that time away from the recorded
// start, and the holder is still recognized and killed.
func TestGuardedKillHoldsAfterAWallClockStep(t *testing.T) {
	dir := testHoldersDir(t)
	desc, child := launchWithChild(t, dir)
	if desc.StartTicks == 0 {
		t.Skip("this OS keeps a stored creation time; a holder records no boot-clock start")
	}
	unattachable(t, dir, desc, CauseToken)

	m := testManager(t, dir, nil, func(o *Options) {
		o.procStart = func(pid int) (time.Time, error) {
			real, err := processStartTime(pid)
			return real.Add(-10 * time.Second), err
		}
	})
	m.Scan()
	if err := m.Stop(desc.ID); err != nil {
		t.Fatalf("Stop after a wall-clock step = %v, want the holder killed", err)
	}
	waitFor(t, "the holder to end", func() bool { return !processAlive(desc.PID) })
	waitFor(t, "the child to end with its holder", func() bool { return !processAlive(child.PID) })
}

// TestRecordedStartMatchesTheOSCreationTime pins what the guard relies on:
// the start time a holder records sits within the tolerance of the creation
// time the OS reports for its process.
func TestRecordedStartMatchesTheOSCreationTime(t *testing.T) {
	dir := testHoldersDir(t)
	desc, _ := launchWithChild(t, dir)
	started, err := processStartTime(desc.PID)
	if err != nil {
		t.Fatalf("processStartTime: %v", err)
	}
	recorded := time.Unix(0, int64(desc.StartedAt*float64(time.Second)))
	if diff := started.Sub(recorded); diff > startTimeTolerance || diff < -startTimeTolerance {
		t.Fatalf("OS creation time %s and recorded start %s differ by %s, more than the %s tolerance", started, recorded, diff, startTimeTolerance)
	}
	ticks, err := processStartTicks(desc.PID)
	if err != nil {
		t.Fatalf("processStartTicks: %v", err)
	}
	if ticks != desc.StartTicks {
		t.Fatalf("OS start ticks %d, recorded start ticks %d", ticks, desc.StartTicks)
	}
}

// TestRunFindsHoldersAtStartAndOnTheRescan pins discovery: a starting
// daemon attaches to the holders already running, and a holder that
// appears later is found on the rescan without a restart.
func TestRunFindsHoldersAtStartAndOnTheRescan(t *testing.T) {
	dir := testHoldersDir(t)
	early, _ := launchWithChild(t, dir)
	// The second holder runs from the start but registers later: its
	// descriptor is held aside until the daemon is up.
	late, _ := launchWithChild(t, dir)
	registered := filepath.Join(dir, late.ID, descriptorName)
	aside := filepath.Join(t.TempDir(), descriptorName)
	if err := os.Rename(registered, aside); err != nil {
		t.Fatal(err)
	}

	m := testManager(t, dir, nil, func(o *Options) { o.RescanInterval = 100 * time.Millisecond })
	go m.Run()
	attached := func(id string) bool {
		for _, h := range m.List().Holders {
			if h.ID == id {
				return h.Attached
			}
		}
		return false
	}
	waitFor(t, "the holder running at start", func() bool { return attached(early.ID) })

	if got := m.List().Holders; len(got) != 1 {
		t.Fatalf("holders before the second registers = %+v, want only the first", got)
	}
	if err := os.Rename(aside, registered); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the holder that appeared later", func() bool { return attached(late.ID) })
	if !attached(early.ID) {
		t.Errorf("the first holder lost its attach: %+v", m.List())
	}

	// A holder that has gone is dropped on a rescan, with its directory.
	if err := killHolder(late.PID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the dead holder to leave the listing", func() bool {
		for _, h := range m.List().Holders {
			if h.ID == late.ID {
				return false
			}
		}
		_, err := os.Stat(filepath.Join(dir, late.ID))
		return os.IsNotExist(err)
	})
	m.Close()
	// Closing the daemon's side ends no holder.
	if !processAlive(early.PID) {
		t.Errorf("closing the manager ended holder %d", early.PID)
	}
}

// TestInterpreterFallsBackToPython3OnPath pins the interpreter choice: the
// configured one when set, otherwise python3 from PATH, and a start failure
// that names both when neither exists.
func TestInterpreterFallsBackToPython3OnPath(t *testing.T) {
	python := testPython(t)
	dir := testHoldersDir(t)
	var asked []string
	m := testManager(t, dir, nil, func(o *Options) {
		o.Python = ""
		o.lookPath = func(name string) (string, error) {
			asked = append(asked, name)
			return python, nil
		}
	})
	id, err := m.Launch(nil)
	if err != nil {
		t.Fatalf("Launch with no configured interpreter: %v", err)
	}
	if len(asked) != 1 || asked[0] != "python3" {
		t.Fatalf("PATH was asked for %v, want python3 once", asked)
	}
	if err := m.Stop(id); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	none := testManager(t, testHoldersDir(t), nil, func(o *Options) {
		o.Python = ""
		o.lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	})
	_, err = none.Launch(json.RawMessage(`{"label":"lost"}`))
	if err == nil || !strings.Contains(err.Error(), PythonEnv) || !strings.Contains(err.Error(), "python3") {
		t.Fatalf("Launch with no interpreter at all = %v, want an error naming %s and python3", err, PythonEnv)
	}
	failures := none.List().Failures
	if len(failures) != 1 || failures[0].Message != err.Error() || string(failures[0].Session) != `{"label":"lost"}` {
		t.Fatalf("failures = %+v, want the one start failure with its session", failures)
	}
}

// TestFailedStartsAreListed pins that a holder that cannot start, or ends
// before it registers, is kept for the listing, newest first and bounded,
// and leaves no directory behind.
func TestFailedStartsAreListed(t *testing.T) {
	dir := testHoldersDir(t)
	missing := testManager(t, dir, nil, func(o *Options) { o.Python = filepath.Join(dir, "no-such-python") })
	if _, err := missing.Launch(nil); err == nil {
		t.Fatal("Launch with a missing interpreter succeeded")
	}
	if got := missing.List().Failures; len(got) != 1 || !strings.Contains(got[0].Message, "no-such-python") {
		t.Fatalf("failures = %+v, want one naming the interpreter", got)
	}

	early := testManager(t, dir, nil, func(o *Options) { o.holderArgs = []string{"--no-such-option"} })
	for range maxFailures + 2 {
		if _, err := early.Launch(nil); err == nil || !strings.Contains(err.Error(), "ended before it registered") {
			t.Fatalf("Launch of a holder that exits at once = %v, want it reported as ended before it registered", err)
		}
	}
	if got := early.List().Failures; len(got) != maxFailures {
		t.Fatalf("%d failures listed, want the newest %d", len(got), maxFailures)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != scriptDirName {
			t.Errorf("a failed start left %s behind", entry.Name())
		}
	}
}

// TestRefusedBreakawayStartsAHolderMarkedNotRestartSafe pins the Windows
// job fallback: when the start that asks to leave the starter's job is
// refused, the holder still starts, inside the job, and is listed as not
// restart-safe.
func TestRefusedBreakawayStartsAHolderMarkedNotRestartSafe(t *testing.T) {
	dir := testHoldersDir(t)
	var attempts []bool
	applies := true
	m := testManager(t, dir, nil, func(o *Options) {
		o.breakaway = &applies
		o.start = func(exe string, args []string, cwd string, breakaway bool) (*exec.Cmd, error) {
			attempts = append(attempts, breakaway)
			if breakaway {
				return nil, errors.New("access is denied")
			}
			return startDetached(exe, args, cwd, false)
		}
	})
	id, err := m.Launch(nil)
	if err != nil {
		t.Fatalf("Launch after a refused breakaway: %v", err)
	}
	if len(attempts) != 2 || !attempts[0] || attempts[1] {
		t.Fatalf("start attempts (breakaway) = %v, want [true false]", attempts)
	}
	holders := m.List().Holders
	if len(holders) != 1 || holders[0].ID != id || !holders[0].Attached || holders[0].RestartSafe {
		t.Fatalf("holders = %+v, want %s attached and not restart-safe", holders, id)
	}
	if err := m.Stop(id); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// A start that leaves the job, or needs no breakaway, is restart-safe.
	plain := testManager(t, dir, nil)
	id, err = plain.Launch(nil)
	if err != nil {
		t.Fatal(err)
	}
	if holders := plain.List().Holders; len(holders) != 1 || !holders[0].RestartSafe {
		t.Fatalf("holders = %+v, want %s restart-safe", holders, id)
	}
	if err := plain.Stop(id); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// TestAttachSpeaksTheHoldersOlderProtocol pins version tolerance from the
// daemon's side: a daemon one version ahead still attaches to the holder,
// speaking the holder's version.
func TestAttachSpeaksTheHoldersOlderProtocol(t *testing.T) {
	dir := testHoldersDir(t)
	desc, _ := launchWithChild(t, dir)
	old := protocolVersion
	protocolVersion = old + 1
	t.Cleanup(func() { protocolVersion = old })

	client, info, err := Attach(context.Background(), desc, nil)
	if err != nil {
		t.Fatalf("Attach from a daemon one version ahead: %v", err)
	}
	defer client.Close()
	if info.Protocol != old || desc.Protocol != old {
		t.Fatalf("spoken protocol = %d (descriptor %d), want the holder's %d", info.Protocol, desc.Protocol, old)
	}
	if status, err := client.Status(); err != nil || status.Child.State != "running" {
		t.Fatalf("Status over the older protocol = %+v, %v", status, err)
	}
}

// TestStagedScriptsNoLiveHolderRunsArePrunedAtStart pins the staged script
// cleanup: at daemon start a script that no live holder's descriptor names
// is removed, while the embedded script's copy, a script a live holder
// runs, and a freshly staged one stay.
func TestStagedScriptsNoLiveHolderRunsArePrunedAtStart(t *testing.T) {
	dir := testHoldersDir(t)
	current, err := StageScript(dir)
	if err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Dir(current)
	const liveHash = "1111111111111111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	inUse := filepath.Join(binDir, stagedScriptName(liveHash))
	stale := filepath.Join(binDir, stagedScriptName("2222222222222222"))
	fresh := filepath.Join(binDir, stagedScriptName("3333333333333333"))
	other := filepath.Join(binDir, "notes.txt")
	long := time.Now().Add(-time.Hour)
	for _, path := range []string{inUse, stale, fresh, other} {
		if err := os.WriteFile(path, []byte("# staged"), 0o600); err != nil {
			t.Fatal(err)
		}
		if path != fresh {
			if err := os.Chtimes(path, long, long); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.Chtimes(current, long, long); err != nil {
		t.Fatal(err)
	}
	// A live holder from an older build: its process is alive (this test's
	// own), its endpoint does not answer, and its descriptor names the
	// script it runs.
	holderDir := filepath.Join(dir, "older")
	if err := os.MkdirAll(holderDir, 0o700); err != nil {
		t.Fatal(err)
	}
	self, err := processStartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	ticks, err := processStartTicks(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	desc, _ := json.Marshal(Descriptor{Schema: 1, ID: "older", PID: os.Getpid(), Port: closedPort(t), Token: "t", Protocol: 1, StartedAt: float64(self.UnixNano()) / 1e9, StartTicks: ticks, ScriptHash: liveHash})
	if err := os.WriteFile(filepath.Join(holderDir, descriptorName), desc, 0o600); err != nil {
		t.Fatal(err)
	}

	m := New(Options{Dir: dir, RescanInterval: time.Hour})
	go m.Run()
	defer m.Close()
	waitFor(t, "the stale script to be pruned", func() bool {
		_, err := os.Stat(stale)
		return os.IsNotExist(err)
	})
	for _, path := range []string{current, inUse, fresh, other} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s was removed: %v", filepath.Base(path), err)
		}
	}
	if holders := m.List().Holders; len(holders) != 1 || holders[0].Attached || holders[0].Cause != CauseNoAnswer {
		t.Errorf("holders = %+v, want the older holder unattached with no answer", holders)
	}
}

// TestScriptIsStagedUnderItsContentHash pins staging: the embedded script
// lands under a name taken from its content, an intact copy is reused
// rather than rewritten, and a damaged copy is replaced.
func TestScriptIsStagedUnderItsContentHash(t *testing.T) {
	dir := filepath.Join(t.TempDir(), DirName)
	path, err := StageScript(dir)
	if err != nil {
		t.Fatalf("StageScript: %v", err)
	}
	sum := sha256.Sum256(script)
	want := filepath.Join(dir, scriptDirName, "holder-"+hex.EncodeToString(sum[:])[:16]+".py")
	if path != want {
		t.Fatalf("staged at %s, want %s", path, want)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, script) {
		t.Fatalf("staged content differs from the embedded script (%v)", err)
	}
	long := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, long, long); err != nil {
		t.Fatal(err)
	}
	if again, err := StageScript(dir); err != nil || again != path {
		t.Fatalf("second StageScript = %s, %v", again, err)
	}
	if info, err := os.Stat(path); err != nil || !info.ModTime().Equal(long) {
		t.Fatalf("an intact staged copy was rewritten")
	}
	if err := os.WriteFile(path, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := StageScript(dir); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, script) {
		t.Fatalf("a damaged staged copy was not replaced")
	}
}

// TestAConsumersReplayIsNoLoss pins what a consumer of the manager's events
// gets beside the manager's own bookkeeping: a sink added after New receives
// the holders' events beside the one New was given, RingBytes sizes a new holder's ring, and a replay the
// consumer asks for from a sequence the ring has dropped reports the gap to
// the sink without counting records lost, since the manager had received
// them all.
func TestAConsumersReplayIsNoLoss(t *testing.T) {
	dir := testHoldersDir(t)
	given := newCollector()
	m := testManager(t, dir, given, func(o *Options) { o.RingBytes = 512 })
	sink := newCollector()
	m.AddSink(sink)
	id, err := m.Launch(json.RawMessage(`{"kind":"test"}`))
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	spawnTicker(t, m, id)
	// Far more than 512 bytes of ticks: the ring has dropped its first
	// records by now, and the sink has received every one of them.
	waitFor(t, "the ticker to outrun its ring", func() bool { return len(sink.seqs(id)) > 200 })
	if seqs := sink.seqs(id); !contiguousFrom(seqs, 1) {
		t.Fatalf("the sink's sequences = %v..., want 1.. without holes", seqs[:min(len(seqs), 8)])
	}
	if len(given.seqs(id)) < 200 {
		t.Errorf("the sink New was given received %d records, want what the added one received", len(given.seqs(id)))
	}
	lost := func() uint64 {
		for _, h := range m.List().Holders {
			if h.ID == id {
				return h.Lost
			}
		}
		t.Fatalf("holder %s is not listed", id)
		return 0
	}
	if n := lost(); n != 0 {
		t.Fatalf("before the replay the holder has lost %d records, want none", n)
	}
	first, _, err := m.Client(id).Replay(1)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if first <= 1 {
		t.Fatalf("the ring's first sequence after 200 records in 512 bytes = %d, want the ring to have dropped records", first)
	}
	waitFor(t, "the replay's gap to reach the sink", func() bool {
		for _, ev := range sink.snapshot(id) {
			if ev.Event == "gap" && ev.From == 1 && ev.To == first {
				return true
			}
		}
		return false
	})
	if n := lost(); n != 0 {
		t.Errorf("after a consumer's replay the holder has lost %d records, want none: the manager received them all", n)
	}
}
