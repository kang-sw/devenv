package wswhiteboard

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestDashboardAnswerMarkerDoesNotChangeTopicDisplay(t *testing.T) {
	f := FolderStore{Dir: filepath.Join(t.TempDir(), "board")}
	now := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	if err := f.Update(nil, func(s *State) error {
		noticeTopic(t, s, "Unrelated", StateWaiting, 0, now)
		noticeTopic(t, s, "Answered", StateWaiting, 0, now.Add(time.Minute))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err := f.Open(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.RecordDashboardAnswer("inbox-1", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	after, err := f.Open(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, layout := range []Layout{LayoutFull} {
		oldView := BuildView(before, ViewInput{Layout: layout, ReplyID: "r"})
		newView := BuildView(after, ViewInput{Layout: layout, ReplyID: "r"})
		if len(oldView.Topics) != len(newView.Topics) {
			t.Fatalf("layout %v: topic count changed", layout)
		}
		for i := range oldView.Topics {
			old, got := oldView.Topics[i], newView.Topics[i]
			if old.Name != got.Name || old.UpdatedAt != got.UpdatedAt || !reflect.DeepEqual(old.Stream, got.Stream) || !reflect.DeepEqual(old.Older, got.Older) {
				t.Fatalf("layout %v: answer marker changed topic display: before=%+v after=%+v", layout, old, got)
			}
		}
	}
}

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
