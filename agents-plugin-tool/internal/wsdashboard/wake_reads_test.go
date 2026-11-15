package wsdashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// BenchmarkWakeReads112 exercises only the read side of a live wake: a hosted
// snapshot, four rail sessions, refresh stamps, and an unchanged folder board.
// It deliberately excludes rendering, transport, listeners, and shared state.
func BenchmarkWakeReads112(b *testing.B) {
	dir := b.TempDir()
	now := time.Now()
	board := wswhiteboard.FolderStore{Dir: filepath.Join(dir, "board")}
	if err := board.Update(nil, func(s *wswhiteboard.State) error {
		for i := 0; i < 112; i++ {
			s.Topics = append(s.Topics, wswhiteboard.TopicDoc{Name: fmt.Sprintf("topic-%d", i), Body: "unchanged topic body", Entries: []wswhiteboard.Entry{{ID: fmt.Sprintf("e%d", i+1), Kind: wswhiteboard.EntryAgent, Text: "history", At: "2026-11-15T00:00:00Z"}}})
		}
		s.EntrySeq = 112
		return nil
	}); err != nil {
		b.Fatal(err)
	}
	var folders wswhiteboard.FolderCache
	var snapshots SnapshotCache
	index := NewIndex(IndexOptions{})
	ids := []string{"sessiona", "sessionb", "sessionc", "sessiond"}
	for _, id := range ids {
		reg := Registration{ReplyID: id, HeartbeatAt: now.UTC().Format(time.RFC3339)}
		index.sessions[id] = &indexEntry{session: Session{Registration: reg}}
		sd := SessionDir(dir, id)
		if err := os.MkdirAll(sd, 0700); err != nil {
			b.Fatal(err)
		}
		snap := wslive.Snapshot{SchemaVersion: 1, ReplyID: id, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000}
		snapshots.SetLive(dir, id, snap)
		records := make([]wslive.ObservedRecord, 32)
		for j := range records {
			records[j] = wslive.ObservedRecord{ModelObservation: wslive.ModelObservation{ID: fmt.Sprintf("response-%d", j), Model: fmt.Sprintf("model-%d", j%3), Source: "pi.assistant", At: int64(j + 1)}}
		}
		if err := wslive.AppendObserved(sd, records); err != nil {
			b.Fatal(err)
		}
	}
	reader := pageReader{dir: dir, index: index, snapshots: &snapshots, now: func() time.Time { return now }}
	inputs := refreshInputs{ids: ids, watch: []string{wslive.SnapshotPath(SessionDir(dir, ids[0]))}}
	wake := func() {
		if _, err := folders.Open(board, nil); err != nil {
			b.Fatal(err)
		}
		for _, id := range ids {
			if snapshots.Snapshot(dir, id) == nil {
				b.Fatal("missing snapshot")
			}
		}
		reader.sessionStamp(ids[0], inputs)
		reader.indexStamp()
	}
	wake()
	b.ResetTimer()
	opensBefore := snapshots.observed.Opens()
	for i := 0; i < b.N; i++ {
		wake()
	}
	b.StopTimer()
	b.ReportMetric(float64(snapshots.observed.Opens()-opensBefore)/float64(b.N), "record-opens/op")
}
