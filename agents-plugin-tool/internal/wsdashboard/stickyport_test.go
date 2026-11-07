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
	dir := t.TempDir()
	preferred, release := holdPort(t)
	release()

	// Free preferred port: bound and recorded.
	if got := startAt(t, dir, preferred); got != preferred || recordedPort(t, dir) != preferred {
		t.Fatalf("free preferred: bound %d, record %d, want %d", got, recordedPort(t, dir), preferred)
	}

	// Preferred port held by a foreign listener: falls back and records it.
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(preferred)))
	if err != nil {
		t.Fatal(err)
	}
	fallback := startAt(t, dir, preferred)
	if fallback == preferred || fallback == 0 || recordedPort(t, dir) != fallback {
		t.Fatalf("held preferred: bound %d, record %d", fallback, recordedPort(t, dir))
	}

	// Restart with the preferred port still held: the recorded port returns.
	if got := startAt(t, dir, preferred); got != fallback {
		t.Fatalf("restart with preferred held: bound %d, want recorded %d", got, fallback)
	}

	// Preferred port free again: it wins and the record follows.
	_ = l.Close()
	if got := startAt(t, dir, preferred); got != preferred || recordedPort(t, dir) != preferred {
		t.Fatalf("preferred freed: bound %d, record %d, want %d", got, recordedPort(t, dir), preferred)
	}
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
