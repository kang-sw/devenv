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

// TestSummaryWriterLockReadsTheRowOnce pins the hosted live writer's
// ownership fence (lockSummaryWriter), which every ring and snapshot write of
// a hosted lead takes: one registration read per lock, and the fence still
// refuses a row that names another process or that a terminal took.
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
	if ok, reads := lock(); ok || reads != 1 || c.foreignRow {
		t.Fatalf("row naming another process: ok=%v reads=%d foreign=%v, want false, 1, false", ok, reads, c.foreignRow)
	}

	taken := row
	taken.Hosted, taken.HostPID = false, 0
	write(taken)
	if ok, reads := lock(); ok || reads != 1 || !c.foreignRow {
		t.Fatalf("row a terminal took: ok=%v reads=%d foreign=%v, want false, 1, true", ok, reads, c.foreignRow)
	}
}
