package wsagent

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func TestRegistrationHeartbeatFailureAndLiveness(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "blocked")
	if err := os.WriteFile(dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1000, 0).UTC()
	r := NewRunner(Options{Dir: dir, Now: func() time.Time { return now }, Log: io.Discard})
	c := &conv{r: r, rec: Record{ReplyID: "idle", Root: base, Backend: "pi", CreatedAt: now.Format(time.RFC3339), Lead: true}}
	if c.writeRegistration() || !c.lastReg.IsZero() {
		t.Fatal("failed registration advanced the heartbeat")
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if !c.writeRegistration() || !c.lastReg.Equal(now) {
		t.Fatal("registration retry failed")
	}
	now = now.Add(time.Second)
	if !c.writeRegistration() {
		t.Fatal("unchanged registration failed")
	}
	row, err := wsdashboard.ReadRegistration(dir, "idle")
	if err != nil || row.HeartbeatAt != c.lastReg.UTC().Format(time.RFC3339) || c.lastReg.Equal(now) {
		t.Fatalf("unchanged write moved the heartbeat: %+v, %v", row, err)
	}
	c.rec.HostPID = 123
	if !c.writeRegistration() || !c.lastReg.Equal(now) {
		t.Fatal("semantic registration change was skipped")
	}
	// Missing, unreadable and altered same-owner rows must be repaired even
	// inside the heartbeat interval; success is a real spawn reservation.
	path := filepath.Join(wsdashboard.SessionDir(dir, "idle"), "registration.json")
	for _, broken := range []string{"missing", "unreadable", "changed"} {
		now = now.Add(time.Second)
		switch broken {
		case "missing":
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		case "unreadable":
			if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
				t.Fatal(err)
			}
		case "changed":
			row, err := wsdashboard.ReadRegistration(dir, "idle")
			if err != nil {
				t.Fatal(err)
			}
			row.HostPID = 999
			if err := wsdashboard.WriteRegistration(dir, row); err != nil {
				t.Fatal(err)
			}
		}
		if !c.writeRegistration() || !c.lastReg.Equal(now) {
			t.Fatalf("%s row not repaired", broken)
		}
		row, err := wsdashboard.ReadRegistration(dir, "idle")
		if err != nil || row.HostPID != 123 || !row.Hosted {
			t.Fatalf("%s reservation: %+v %v", broken, row, err)
		}
	}
	for range 4 {
		now = now.Add(wsdashboard.HeartbeatInterval)
		if !c.writeRegistration() {
			t.Fatal("heartbeat write failed")
		}
		row, err := wsdashboard.ReadRegistration(dir, "idle")
		if err != nil || !row.Live(now.Add(wsdashboard.HeartbeatInterval)) {
			t.Fatalf("legacy registration reader lost liveness: %+v, %v", row, err)
		}
		if row.Live(now.Add(wsdashboard.LiveWindow + time.Second)) {
			t.Fatal("stopped registration did not go stale")
		}
	}
	row, err = wsdashboard.ReadRegistration(dir, "idle")
	if err != nil {
		t.Fatal(err)
	}
	row.Hosted, row.Agent, row.HostPID = false, false, 0
	if err := wsdashboard.WriteRegistration(dir, row); err != nil {
		t.Fatal(err)
	}
	if c.writeRegistration() || !c.foreignRow {
		t.Fatal("terminal handoff was overwritten")
	}
}

func TestHeartbeatRegistrationPrecedesBlockedSnapshot(t *testing.T) {
	base := t.TempDir()
	now := time.Unix(1000, 0).UTC()
	r := NewRunner(Options{Dir: base, Now: func() time.Time { return now }, Log: io.Discard})
	rec := Record{
		ReplyID: "hosted",
		Root:    t.TempDir(),
		Backend: BackendPi,
		Lead:    true,
		HostPID: 42,
	}
	c := &conv{r: r, rec: rec}
	c.live = newLiveWriter(wsdashboard.SessionDir(base, rec.ReplyID), rec.ReplyID, rec.Backend, r.opts.Now, io.Discard)
	c.live.setSlot(&idleSnapshotSlot{})
	c.live.owner = c.lockSummaryWriter
	r.summaryOwners = map[string]*conv{rec.ReplyID: c}

	snapshotStarted := make(chan struct{})
	c.live.beforeSnapshot = func() { close(snapshotStarted) }
	r.summaryMu.Lock()
	locked := true
	defer func() {
		if locked {
			r.summaryMu.Unlock()
		}
	}()
	done := make(chan struct{})
	go func() {
		c.heartbeat()
		close(done)
	}()
	select {
	case <-snapshotStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("heartbeat did not reach snapshot publication")
	}
	row, err := wsdashboard.ReadRegistration(base, rec.ReplyID)
	r.summaryMu.Unlock()
	locked = false
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("heartbeat did not finish after summary writer was released")
	}
	if err != nil {
		t.Fatalf("registration was not refreshed before blocked snapshot publication: %v", err)
	}
	if row.HeartbeatAt != now.Format(time.RFC3339) {
		t.Fatalf("registration heartbeat = %q, want %q", row.HeartbeatAt, now.Format(time.RFC3339))
	}
}
