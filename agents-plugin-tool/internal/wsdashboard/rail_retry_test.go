package wsdashboard

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestRailRetriesFailedInputsWithoutAnotherWake(t *testing.T) {
	for _, failedStore := range []string{"snapshot", "ring"} {
		t.Run(failedStore, func(t *testing.T) {
			now := time.Now()
			stats := &streamStats{}
			d := &Daemon{dir: t.TempDir(), log: io.Discard, stats: stats, broker: newBroker(stats), railMsgs: newRailMessages(), snapshots: &SnapshotCache{}}
			d.broker.after = func(time.Duration, func()) {}
			d.index = NewIndex(IndexOptions{Now: func() time.Time { return now }})
			for _, id := range []string{"changed", "unrelated"} {
				reg := Registration{ReplyID: id, Project: "/rail-retry-test", Harness: "pi", HeartbeatAt: stampFor(now)}
				d.index.sessions[id] = &indexEntry{session: Session{Registration: reg, Rev: 1}}
			}
			reads := map[string]int{}
			d.railInputs.onRead = func(id string) { reads[id]++ }
			failing, changed := false, false
			d.snapshots.files.stat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
			d.snapshots.load = func(path string) (parsedSnapshot, error) {
				if strings.Contains(path, "changed") && failing && failedStore == "snapshot" {
					return parsedSnapshot{}, errors.New("temporary snapshot read failure")
				}
				snap := wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000}
				snap.Lead.Phase = wslive.PhaseIdle
				if strings.Contains(path, "changed") && changed {
					snap.Lead.Phase = wslive.PhaseThinking
				}
				return parsedSnapshot{snap: snap, docOK: true}, nil
			}
			d.railMsgs.list = func(path string) ([]string, error) {
				if strings.Contains(path, "changed") && failing && failedStore == "ring" {
					return nil, errors.New("temporary ring listing failure")
				}
				return wslive.RingNames(path)
			}
			pages := pageReader{dir: d.dir, index: d.index, snapshots: d.snapshots, now: func() time.Time { return now }}
			src := d.railSource(pages, "en", "")
			cache := newPageCache(d.broker, stats)
			cache.now = func() time.Time { return now }
			cache.wait = func(delay time.Duration) { now = now.Add(delay) }
			page := cache.acquire(src.key)
			defer cache.release(src.key, page)
			cache.get(page, src, false)
			cache.get(page, src, true) // establish a valid refresh stamp
			if !page.stampOK {
				t.Fatal("fixture did not establish a reusable refresh stamp")
			}

			changed, failing = true, true
			d.index.sessions["changed"].session.Rev++
			if err := wslive.AppendRing(wslive.RingDir(SessionDir(d.dir, "changed")), []wslive.RingEntry{{Kind: wslive.RingReply, Text: "final-message"}}); err != nil {
				t.Fatal(err)
			}
			d.broker.wake("changed")
			cache.get(page, src, false)
			if !d.railInputs.retryPending() {
				t.Fatal("failed input became current")
			}
			at := d.broker.current()
			// Recovery changes neither files nor revisions and sends no wake.
			failing = false
			now = now.Add(sseRefreshInterval)
			last, _ := cache.get(page, src, true)
			if d.broker.current() != at || d.railInputs.retryPending() {
				t.Fatal("recovery required another wake or remained pending")
			}
			if reads["changed"] != 3 || reads["unrelated"] != 1 {
				t.Fatalf("input reads = %v; want failed session retried once and unrelated untouched", reads)
			}
			input := d.railInputs.sessions["changed"].input
			if input.Snapshot == nil || input.Snapshot.Lead.Phase != wslive.PhaseThinking || !strings.Contains(string(last.regions[regionRail].HTML), "final-message") {
				t.Fatal("recovered final snapshot/message did not reach the rail")
			}
		})
	}
}
