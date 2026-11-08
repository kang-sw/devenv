package wsagent

import (
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

type nopMapper struct{}

func (nopMapper) mapLine([]byte, lineReplies) []Event { return nil }

// TestExitedChildIsNotHeldByAGrandchildsStderr: a process the child left
// running with the child's stderr does not keep the exited child's events
// open past the wait delay.
func TestExitedChildIsNotHeldByAGrandchildsStderr(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 6 >/dev/null </dev/null & exit 0")
	p, err := startStreamProcess(cmd, nopMapper{}, claudeProtocol{})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(streamWaitDelay + 3*time.Second)
	for {
		select {
		case ev, ok := <-p.Events():
			if !ok {
				return
			}
			if ev.Fatal != nil {
				t.Fatalf("a clean exit with stderr held is no failure: %v", ev.Fatal)
			}
		case <-deadline:
			t.Fatal("the events stayed open while a grandchild held stderr")
		}
	}
}

// scriptMapper maps a stand-in child's stdout lines: "fatal <msg>" is a
// Fatal, any other line an event carrying the line as its SessionID.
type scriptMapper struct{}

func (scriptMapper) mapLine(line []byte, _ lineReplies) []Event {
	if msg, ok := strings.CutPrefix(string(line), "fatal "); ok {
		return []Event{{Fatal: errors.New(msg)}}
	}
	return []Event{{SessionID: string(line)}}
}

func startScript(t *testing.T, script string) *streamProcess {
	t.Helper()
	p, err := startStreamProcess(exec.Command("sh", "-c", script), scriptMapper{}, claudeProtocol{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

// collect reads p's events until they close; the deadline only bounds a
// hang (a stand-in that should have ended sleeps 30 s).
func collect(t *testing.T, p *streamProcess) []Event {
	t.Helper()
	var out []Event
	deadline := time.After(15 * time.Second)
	for {
		select {
		case ev, ok := <-p.Events():
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("the events never closed; got %d events", len(out))
		}
	}
}

// awaitLine waits for the stand-in's stdout line want.
func awaitLine(t *testing.T, p *streamProcess, want string) {
	t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case ev, ok := <-p.Events():
			if !ok {
				t.Fatalf("the events closed before %q", want)
			}
			if ev.SessionID == want {
				return
			}
		case <-deadline:
			t.Fatalf("no %q line", want)
		}
	}
}

func signalOf(t *testing.T, p *streamProcess) (syscall.Signal, bool) {
	t.Helper()
	ws, ok := p.cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		t.Skip("no wait status on this platform")
	}
	return ws.Signal(), ws.Signaled()
}

func TestNonZeroExitIsFatalWithStderrTail(t *testing.T) {
	evs := collect(t, startScript(t, "head -c 5000 /dev/zero | tr '\\0' a >&2; echo boom >&2; exit 3"))
	if len(evs) != 1 || evs[0].Fatal == nil {
		t.Fatalf("events = %+v, want one Fatal", evs)
	}
	msg := evs[0].Fatal.Error()
	if !strings.HasPrefix(msg, "sh exited: a") || !strings.HasSuffix(msg, "aboom") || len(msg) > len("sh exited: ")+4<<10 {
		t.Fatalf("Fatal = %.80q... (%d bytes), want the stderr tail (at most 4 KiB) after \"sh exited: \"", msg, len(msg))
	}

	evs = collect(t, startScript(t, "exit 3"))
	if len(evs) != 1 || evs[0].Fatal == nil || evs[0].Fatal.Error() != "sh exited: exit status 3" {
		t.Fatalf("no stderr: events = %+v, want the exit status", evs)
	}
}

// A mapper's Fatal (a refused launch) kills the child at once rather than
// waiting for it to exit.
func TestFatalKillsTheChild(t *testing.T) {
	p := startScript(t, "echo 'fatal launch refused'; exec sleep 30")
	evs := collect(t, p)
	if len(evs) == 0 || evs[0].Fatal == nil || evs[0].Fatal.Error() != "launch refused" {
		t.Fatalf("events = %+v, want the mapper's Fatal first", evs)
	}
	if sig, ok := signalOf(t, p); !ok || sig != syscall.SIGKILL {
		t.Fatalf("child ended by %v (signaled %v), want SIGKILL", sig, ok)
	}
}

// A stdout read error (here a line over the scanner cap) ends the turn as a
// Fatal and kills the child, instead of leaving the child blocked on a pipe
// nobody reads.
func TestScannerErrorIsFatal(t *testing.T) {
	restore := maxLineBytes
	maxLineBytes = 1 << 10
	t.Cleanup(func() { maxLineBytes = restore })
	p := startScript(t, "head -c 4096 /dev/zero | tr '\\0' a; echo; exec sleep 30")
	evs := collect(t, p)
	if len(evs) == 0 || evs[0].Fatal == nil || !strings.Contains(evs[0].Fatal.Error(), "token too long") {
		t.Fatalf("events = %+v, want a read Fatal first", evs)
	}
	if !strings.HasPrefix(evs[0].Fatal.Error(), "reading sh output: ") {
		t.Fatalf("Fatal = %q, want it to name the command", evs[0].Fatal)
	}
	if sig, ok := signalOf(t, p); !ok || sig != syscall.SIGKILL {
		t.Fatalf("child ended by %v (signaled %v), want SIGKILL", sig, ok)
	}
}

// Close escalates: input EOF, then SIGTERM, then SIGKILL, closeGrace apart.
// Each stand-in prints "ready" once its signal disposition is set.
func TestCloseEscalates(t *testing.T) {
	restore := closeGrace
	closeGrace = 200 * time.Millisecond
	t.Cleanup(func() { closeGrace = restore })
	for _, c := range []struct {
		name, script string
		signal       syscall.Signal // 0: a normal exit
	}{
		{"exits at input EOF", "echo ready; exec cat >/dev/null", 0},
		{"ignores EOF", "echo ready; exec sleep 30", syscall.SIGTERM},
		{"ignores EOF and SIGTERM", "trap '' TERM; echo ready; exec sleep 30", syscall.SIGKILL},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := startScript(t, c.script)
			awaitLine(t, p, "ready")
			p.Close()
			sig, signaled := signalOf(t, p)
			switch {
			case c.signal == 0 && (signaled || !p.cmd.ProcessState.Success()):
				t.Fatalf("want a clean exit at EOF, got %v", p.cmd.ProcessState)
			case c.signal != 0 && (!signaled || sig != c.signal):
				t.Fatalf("child ended by %v (signaled %v), want %v", sig, signaled, c.signal)
			}
		})
	}
}

func TestRequestOutcomes(t *testing.T) {
	restore := controlTimeout
	t.Cleanup(func() { controlTimeout = restore })

	controlTimeout = 100 * time.Millisecond
	silent := startScript(t, "exec cat >/dev/null")
	if err := silent.request("interrupt", map[string]any{"subtype": "interrupt"}); err == nil || err.Error() != "no answer to interrupt within 100ms" {
		t.Fatalf("silent child: %v", err)
	}

	// A child that exits after reading the request answers it with its exit,
	// long before the timeout.
	controlTimeout = time.Minute
	exiting := startScript(t, "exec head -n 1 >/dev/null")
	start := time.Now()
	if err := exiting.request("interrupt", map[string]any{"subtype": "interrupt"}); err == nil || err.Error() != "the process exited" {
		t.Fatalf("exiting child: %v", err)
	}
	if time.Since(start) > 20*time.Second {
		t.Fatal("the exit did not end the request")
	}
}
