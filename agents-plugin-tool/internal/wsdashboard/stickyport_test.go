package wsdashboard

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// holdPort listens on a free loopback port and returns it with a release func.
func holdPort(t *testing.T) (int, func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var once bool
	return l.Addr().(*net.TCPAddr).Port, func() {
		if !once {
			once = true
			_ = l.Close()
		}
	}
}

// startAt starts a daemon preferring port, returns its bound port, and stops it.
func startAt(t *testing.T, dir string, port int) int {
	t.Helper()
	d, err := StartDaemon(DaemonOptions{Dir: dir, Port: port, Version: "1.0.0", LockWait: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	got := d.Endpoint().Port
	d.Close()
	waitDone(t, d)
	return got
}

func recordedPort(t *testing.T, dir string) int {
	t.Helper()
	port, ok := readLastPort(dir)
	if !ok {
		t.Fatal("no last-port record")
	}
	return port
}

func TestStickyFallbackPort(t *testing.T) {
	// A released port can be taken by another process before the test binds it
	// again, and the scenario needs the same port free across daemon restarts,
	// so a collision restarts the scenario on a fresh port.
	for attempt := 0; attempt < 10; attempt++ {
		if stickyFallbackScenario(t, t.TempDir()) {
			return
		}
	}
	t.Fatal("no free preferred port held steady across 10 attempts")
}

// stickyFallbackScenario runs the sticky-fallback checks and reports false
// when a port collision with another process, not a product fault, cut it short.
func stickyFallbackScenario(t *testing.T, dir string) bool {
	t.Helper()
	preferred, release := holdPort(t)
	release()

	// Free preferred port: bound and recorded.
	got := startAt(t, dir, preferred)
	if got != preferred {
		// Another process took the port between release and bind: the daemon
		// fell back, which is the behaviour under test, not a fault here.
		return false
	}
	if rec := recordedPort(t, dir); rec != preferred {
		t.Fatalf("free preferred: bound %d, record %d, want %d", got, rec, preferred)
	}

	// Preferred port held by a foreign listener: falls back and records it.
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(preferred)))
	if err != nil {
		return false
	}
	defer l.Close()
	fallback := startAt(t, dir, preferred)
	if fallback == preferred || fallback == 0 || recordedPort(t, dir) != fallback {
		t.Fatalf("held preferred: bound %d, record %d", fallback, recordedPort(t, dir))
	}

	// Restart with the preferred port still held: the recorded port returns.
	if got := startAt(t, dir, preferred); got != fallback {
		// The recorded port itself can be taken by another process meanwhile.
		probe, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(fallback)))
		if err != nil {
			return false
		}
		_ = probe.Close()
		t.Fatalf("restart with preferred held: bound %d, want recorded %d", got, fallback)
	}

	// Preferred port free again: it wins and the record follows.
	_ = l.Close()
	if got := startAt(t, dir, preferred); got != preferred {
		return false
	}
	if rec := recordedPort(t, dir); rec != preferred {
		t.Fatalf("preferred freed: bound %d, record %d, want %d", got, rec, preferred)
	}
	return true
}

func TestStickyFallbackSkippedWhenRecordedPortAlsoHeld(t *testing.T) {
	dir := t.TempDir()
	preferred, releasePreferred := holdPort(t)
	defer releasePreferred()
	recorded, releaseRecorded := holdPort(t)
	defer releaseRecorded()
	if err := writeLastPort(dir, recorded); err != nil {
		t.Fatal(err)
	}
	got := startAt(t, dir, preferred)
	if got == preferred || got == recorded || got == 0 {
		t.Fatalf("both held: bound %d (preferred %d, recorded %d)", got, preferred, recorded)
	}
}

func TestMalformedLastPortRecordIsIgnored(t *testing.T) {
	for _, content := range []string{"", "not-a-port", "0", "-5", "70000", "12 34"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, lastPortFileName), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		preferred, release := holdPort(t)
		got := startAt(t, dir, preferred)
		release()
		if got == preferred || got == 0 {
			t.Fatalf("record %q: bound %d with %d held", content, got, preferred)
		}
	}
}

func TestOSAssignedRequestIgnoresLastPortRecord(t *testing.T) {
	dir := t.TempDir()
	recorded, release := holdPort(t)
	release()
	if err := writeLastPort(dir, recorded); err != nil {
		t.Fatal(err)
	}
	if ports := portCandidates(dir, 0); len(ports) != 0 {
		t.Fatalf("candidates for port 0 = %v, want none", ports)
	}
}
