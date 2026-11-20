package wsagent

import (
	"io"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// registrationReads counts the registration file reads ReadRegistration made.
func registrationReads() int64 {
	var n int64
	for site, count := range wsstate.FileIOCounters() {
		if strings.HasPrefix(site, "file_io_site:wsdashboard/registry.go:") && strings.HasSuffix(site, ":read_file") {
			n += count
		}
	}
	return n
}

// TestSummaryWriterLockReadsTheRowOnce pins the hosted live writer's ownership
// check (lockSummaryWriter) to one registration file read per lock (7225f1a83),
// ensuring external cross-process takeovers and concurrent unlocked reads can
// never poison ownership.
func TestSummaryWriterLockReadsTheRowOnce(t *testing.T) {
	dir := t.TempDir()
	r := NewRunner(Options{Dir: dir, Log: io.Discard})
	c := &conv{r: r, rec: Record{ReplyID: "lead", Root: t.TempDir(), Backend: BackendPi, Lead: true, HostPID: 4242}}
	r.summaryOwners = map[string]*conv{"lead": c}
	row := wsdashboard.Registration{ReplyID: "lead", Project: c.rec.Root, Harness: BackendPi, Hosted: true, HostPID: 4242}
	write := func(reg wsdashboard.Registration) {
		t.Helper()
		if err := wsdashboard.WriteRegistration(dir, reg); err != nil {
			t.Fatal(err)
		}
	}
	lock := func() (bool, int64) {
		t.Helper()
		before := registrationReads()
		release, ok := c.lockSummaryWriter()
		if ok {
			release()
		}
		return ok, registrationReads() - before
	}
	write(row)

	if ok, reads := lock(); !ok || reads != 1 {
		t.Fatalf("owned row: ok=%v reads=%d, want true and 1", ok, reads)
	}

	other := row
	other.HostPID = 1
	write(other)
	if ok, reads := lock(); ok || reads != 1 || c.isForeign() {
		t.Fatalf("row naming another process: ok=%v reads=%d foreign=%v, want false, 1, false", ok, reads, c.isForeign())
	}

	// Simulate an unlocked concurrent read seeing the old row before an external
	// process rewrites registration.json (terminal takeover, R1 Findings 1 & 2):
	// lockSummaryWriter must still read fresh state under OS flock and refuse.
	write(row)
	if _, err := wsdashboard.ReadRegistration(dir, "lead"); err != nil {
		t.Fatal(err)
	}
	taken := row
	taken.Hosted, taken.HostPID = false, 0
	write(taken)
	if ok, reads := lock(); ok || reads != 1 || !c.isForeign() {
		t.Fatalf("row a terminal took: ok=%v reads=%d foreign=%v, want false, 1, true", ok, reads, c.isForeign())
	}
}
