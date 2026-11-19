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

// TestSummaryWriterLockUsesInMemoryOwnershipFence pins the hosted live writer's
// in-memory ownership fence (lockSummaryWriter): once the row is known in this
// process, ownership checks make zero disk registration reads while still
// refusing a row naming another process or taken by a terminal synchronously.
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

	// Cold cache miss after Forget reads once; subsequent warm checks read zero.
	wsdashboard.ForgetRegistrationCache(dir, "lead")
	if ok, reads := lock(); !ok || reads != 1 {
		t.Fatalf("cold owned row: ok=%v reads=%d, want true and 1", ok, reads)
	}
	if ok, reads := lock(); !ok || reads != 0 {
		t.Fatalf("warm owned row: ok=%v reads=%d, want true and 0", ok, reads)
	}

	other := row
	other.HostPID = 1
	write(other)
	if ok, reads := lock(); ok || reads != 0 || c.foreignRow {
		t.Fatalf("row naming another process: ok=%v reads=%d foreign=%v, want false, 0, false", ok, reads, c.foreignRow)
	}

	taken := row
	taken.Hosted, taken.HostPID = false, 0
	write(taken)
	if ok, reads := lock(); ok || reads != 0 || !c.foreignRow {
		t.Fatalf("row a terminal took: ok=%v reads=%d foreign=%v, want false, 0, true", ok, reads, c.foreignRow)
	}
}
