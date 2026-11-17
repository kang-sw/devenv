package wswhiteboard

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDashboardAnswerRevisionDurableAndIdempotent(t *testing.T) {
	f := FolderStore{Dir: filepath.Join(t.TempDir(), "board")}
	now := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	if err := f.Update(nil, func(s *State) error {
		noticeTopic(t, s, "Work", StateWaiting, 0, now)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := f.RecordDashboardAnswer("inbox-1", now); err != nil {
			t.Fatal(err)
		}
	}
	s, err := f.Open(nil)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, doc := range s.Topics {
		for _, entry := range doc.Entries {
			if entry.Kind == EntrySystem && entry.Text == dashboardAnswerRevisionPrefix+`"inbox-1"` {
				count++
			}
		}
	}
	if count != 1 {
		t.Fatalf("got %d answer markers, want one", count)
	}
}
