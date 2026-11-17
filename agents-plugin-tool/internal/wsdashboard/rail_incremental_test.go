package wsdashboard

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestRailBurstReadsOnlyNamedSessionAndKeepsTrailingPass(t *testing.T) {
	now := time.Now()
	stats := &streamStats{}
	d := &Daemon{dir: t.TempDir(), log: io.Discard, stats: stats, broker: newBroker(stats), railMsgs: newRailMessages(), snapshots: &SnapshotCache{}}
	// No background timers or daemon: the page cache's wait advances time.
	d.broker.after = func(time.Duration, func()) {}
	d.index = NewIndex(IndexOptions{Now: func() time.Time { return now }})
	reads := map[string]int{}
	d.railInputs.onRead = func(id string) { reads[id]++ }
	for i := range 60 {
		id := fmt.Sprintf("session%d", i)
		reg := Registration{ReplyID: id, Project: "/rail-test", Harness: "pi", HeartbeatAt: stampFor(now)}
		d.index.sessions[id] = &indexEntry{session: Session{Registration: reg, Rev: 1, SnapshotUpdatedAt: now}}
		d.snapshots.SetLive(d.dir, id, wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000, Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseThinking}}})
	}
	pages := pageReader{dir: d.dir, index: d.index, snapshots: d.snapshots, now: func() time.Time { return now }}
	src := d.railSource(pages, "en", "")
	cache := newPageCache(d.broker, stats)
	cache.now = func() time.Time { return now }
	var waits []time.Duration
	cache.wait = func(delay time.Duration) { waits = append(waits, delay); now = now.Add(delay) }
	page := cache.acquire(src.key)
	defer cache.release(src.key, page)
	cache.get(page, src, false)
	if len(reads) != 60 {
		t.Fatalf("initial input reads = %d, want 60", len(reads))
	}

	// A hundred wakes naming one session: 99 before a pass and the last
	// while it renders. The latter must remain stale and get a trailing pass.
	d.index.sessions["session0"].session.Label = "burst"
	for range 99 {
		d.broker.wake("session0")
	}
	render := src.render
	passes := 0
	src.render = func(prev *liveRender) pageRender {
		passes++
		r := render(prev)
		if passes == 1 {
			d.index.sessions["session0"].session.Label = "trailing-final"
			d.broker.wake("session0")
		}
		return r
	}
	_, at := cache.get(page, src, false)
	if d.broker.unchangedSince(nil, at) {
		t.Fatal("a change during the render lost its trailing pass")
	}
	last, at := cache.get(page, src, false)
	cache.get(page, src, false)
	if passes != 2 || stats.RailRenders.Load() != 3 || len(waits) != 2 || waits[0] != time.Second || waits[1] != time.Second {
		t.Fatalf("burst passes=%d renders=%d waits=%v; want two passes one second apart after the initial render", passes, stats.RailRenders.Load(), waits)
	}
	if !d.broker.unchangedSince(nil, at) || !strings.Contains(string(last.regions[regionRail].HTML), "trailing-final") {
		t.Fatal("the last change was not rendered")
	}
	for id, n := range reads {
		want := 1
		if id == "session0" {
			want = 3
		}
		if n != want {
			t.Errorf("%s input reads = %d, want %d", id, n, want)
		}
	}
	t.Log("60 sessions / 100 wakes: 60 inputs per pass -> 1; two burst passes spaced 1 s, including trailing final state")

	// Unrelated/global wakes re-evaluate layout and clock but no session
	// stores. A refresh without wakes ages stale badges from memory too.
	d.broker.wake("")
	cache.get(page, src, false)
	now = now.Add(91 * time.Second)
	stale, _ := cache.get(page, src, true)
	if stale.regions[regionRail].Hash == last.regions[regionRail].Hash {
		t.Fatal("a clock-only refresh failed to age the live badges")
	}
	for id, n := range reads {
		if id != "session0" && n != 1 || id == "session0" && n != 3 {
			t.Fatalf("clock/global wake reread %s (%d reads)", id, n)
		}
	}

	// A heartbeat handed to the index without a revision restores a fresh
	// cached snapshot in memory; it must not leave the badge falsely stale.
	d.index.SetSnapshotUpdatedAt("session0", now, now.Add(90*time.Second))
	view := d.railBaseView(pages)
	if view.Live != 60 {
		t.Fatalf("registration liveness changed unexpectedly: %d", view.Live)
	}
	working := 0
	for _, ws := range view.Workspaces {
		for _, row := range ws.Sessions {
			if row.Working {
				working++
				if row.ReplyID != "session0" {
					t.Fatalf("stale session %s still working", row.ReplyID)
				}
			}
		}
	}
	if working != 1 {
		t.Fatalf("heartbeat handoff restored %d working rows, want only session0", working)
	}
	if reads["session0"] != 3 {
		t.Fatal("heartbeat-only handoff reread inputs")
	}
}
