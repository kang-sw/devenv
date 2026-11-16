package wsdashboard

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestIdleTouchesInboxGateKeepsReadThroughAdmission(t *testing.T) {
	f := startPageFixture(t)
	path := wslive.SnapshotPath(SessionDir(f.dir, "livesession"))
	if f.d.snapshots.Snapshot(f.dir, "livesession") == nil {
		t.Fatal("warm snapshot missing")
	}
	// Removing or invalidating a fresh file must immediately refuse writes,
	// without waiting for an index sweep or trusting the presentation memo.
	for _, change := range []string{"removed", "corrupt", "adapterless"} {
		switch change {
		case "removed":
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		case "corrupt":
			if err := os.WriteFile(path, []byte("bad json"), 0600); err != nil {
				t.Fatal(err)
			}
		case "adapterless":
			writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 30000})
		}
		if code, _, raw := postJSON(t, f.base+"/s/livesession/message", f.base, `{"text":"no adapter"}`); code != http.StatusConflict {
			t.Fatalf("%s admitted: %d %s", change, code, raw)
		}
	}
	fresh := wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 30000}
	writeSnapshot(t, f.dir, "livesession", &fresh)
	if code, _, raw := postJSON(t, f.base+"/s/livesession/message", f.base, `{"text":"recovered"}`); code != http.StatusOK {
		t.Fatalf("recovery: %d %s", code, raw)
	}
	// A hosted slot remains authoritative even with no disk snapshot.
	f.d.snapshots.SetLive(f.dir, "livesession", fresh)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if code, _, raw := postJSON(t, f.base+"/s/livesession/message", f.base, `{"text":"hosted"}`); code != http.StatusOK {
		t.Fatalf("hosted: %d %s", code, raw)
	}
}

func TestIdleTouchesExternalSnapshotRecoversThroughIndexToStream(t *testing.T) {
	f := startPageFixture(t, withRefresh)
	now := time.Now()
	reg := register(t, f.dir, "foreignagent", f.project, now)
	reg.Agent = true
	if err := WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	writeSnapshot(t, f.dir, "foreignagent", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.Add(-time.Hour).UnixMilli(), HeartbeatMs: 30000})
	f.d.Index().Refresh() // production agentSessions -> HashLiveStores -> shared cache
	events := openStream(t, f.base+"/s/foreignagent/events")
	waitRegionHolding(t, events, regionMode, modeBoard)
	before, _ := f.d.Index().Session("foreignagent")
	// No external nudge: the real index backstop discovers the liveness
	// transition and propagates it to the stream.
	writeSnapshot(t, f.dir, "foreignagent", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 30000})
	f.d.Index().Refresh()
	after, _ := f.d.Index().Session("foreignagent")
	if after.Rev != before.Rev+1 {
		t.Fatalf("liveness recovery did not bump Rev: %d -> %d", before.Rev, after.Rev)
	}
	waitRegionHolding(t, events, regionMode, modeLive)
}
