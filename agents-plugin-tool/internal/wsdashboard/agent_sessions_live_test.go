package wsdashboard

import (
	"bytes"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestLiveStoreFingerprintListsOnlyNamedStores(t *testing.T) {
	dir := t.TempDir()
	cache := &SnapshotCache{}
	cache.SetLive(dir, "alpha", wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: testNow.UnixMilli(), HeartbeatMs: 30000})
	names := map[string][]string{
		"model-evidence": {"evidence.json"}, "ring": {"one.json"},
		"inbox": {"held.json"}, "inbox-drained": {"sent.json"},
	}
	counts := map[string]int{}
	list := func(tag string, _ func() ([]string, error)) ([]string, error) {
		counts[tag]++
		return names[tag], nil
	}
	read := func(previous LiveStoreFingerprint, stores LiveStores) (LiveStoreFingerprint, string) {
		t.Helper()
		var h bytes.Buffer
		next, at, staleAt := cache.readLiveStoresClock(&h, dir, "alpha", testNow, previous, stores, list)
		if !at.Equal(testNow) || !staleAt.Equal(testNow.Add(90*time.Second+time.Millisecond)) {
			t.Fatalf("lost snapshot clock: %v %v", at, staleAt)
		}
		return next, h.String()
	}
	previous, initial := read(LiveStoreFingerprint{}, LiveRing)
	for _, tag := range []string{"model-evidence", "ring", "inbox", "inbox-drained"} {
		if counts[tag] != 1 {
			t.Fatalf("cold read did not initialize %s: %v", tag, counts)
		}
	}
	clear(counts)
	names["ring"] = append(names["ring"], "two.json")
	previous, ring := read(previous, LiveRing)
	if ring == initial || len(counts) != 1 || counts["ring"] != 1 {
		t.Fatalf("ring write listings = %v; want only ring", counts)
	}
	_, full := read(LiveStoreFingerprint{}, AllLiveStores)
	if ring != full {
		t.Fatal("named ring read dropped an unchanged store's fingerprint")
	}
	clear(counts)
	names["inbox"], names["inbox-drained"] = nil, append(names["inbox-drained"], "held.json")
	previous, inbox := read(previous, LiveInbox)
	if inbox == ring || len(counts) != 2 || counts["inbox"] != 1 || counts["inbox-drained"] != 1 {
		t.Fatalf("inbox write listings = %v; want held and drained only", counts)
	}
	_, full = read(LiveStoreFingerprint{}, AllLiveStores)
	if inbox != full {
		t.Fatal("named inbox read differs from full fingerprint")
	}
	clear(counts)
	_, same := read(previous, AllLiveStores)
	if same != inbox || len(counts) != 4 {
		t.Fatalf("unknown runner write must still check all four directories: %v", counts)
	}
	t.Log("directory listings: cold/unknown=4; named ring=1; named inbox=2 (held+drained)")
}

func TestInboxChangedUsesNamedStoreButPreservesFullBoardPoke(t *testing.T) {
	for _, hasBoard := range []bool{false, true} {
		name := "boardless"
		if hasBoard {
			name = "board"
		}
		t.Run(name, func(t *testing.T) {
			dir, project := t.TempDir(), t.TempDir()
			register(t, dir, "alpha", project, testNow)
			src := &liveCountingSource{fakeSource: newFakeSource()}
			src.set("alpha", SessionData{HasBoard: hasBoard})
			idx := NewIndex(IndexOptions{Dir: dir, Source: src})
			idx.Refresh()
			stats := &streamStats{}
			d := &Daemon{index: idx, stats: stats, broker: newBroker(stats)}
			session, _ := idx.Session("alpha")
			d.inboxChanged(session)
			idx.Settle()
			live, stores := src.liveCount()
			if hasBoard {
				if live != 0 || src.readCounts()["alpha"] != 2 {
					t.Fatal("board poke must still refresh record/board evidence")
				}
			} else if live != 1 || stores != LiveInbox || src.readCounts()["alpha"] != 1 {
				t.Fatalf("boardless inbox: live=%d stores=%b full=%d", live, stores, src.readCounts()["alpha"])
			}
		})
	}
}
