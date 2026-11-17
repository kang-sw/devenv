package wswhiteboard

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCloseWithoutResultAndLegacyResult(t *testing.T) {
	s := &State{}
	now := time.Date(2026, 11, 17, 1, 2, 3, 0, time.UTC)
	stateApply(t, s, `<wb:topic name="t"><wb:log>first</wb:log><wb:close/></wb:topic>`, now)
	if !s.topicClosed("t") || s.CloseResults["t"] != "" {
		t.Fatalf("close: %+v", s)
	}
	stateApply(t, s, `<wb:topic name="t"><wb:log>reopened</wb:log><wb:close result="legacy conclusion"/></wb:topic>`, now.Add(time.Minute))
	d := s.Topics[s.topicDocIndex("t")]
	if !s.topicClosed("t") || s.CloseResults["t"] != "" || d.LatestSummary().Text != "legacy conclusion" {
		t.Fatalf("legacy close: %+v", d)
	}
	n := len(d.Entries)
	if n < 2 || d.Entries[n-2].Kind != EntryAgent || d.Entries[n-1].Text != "Topic closed." {
		t.Fatalf("close ordering: %+v", d.Entries)
	}
}

func TestCloseRefusalHidesLegacyAttribute(t *testing.T) {
	s := &State{}
	problems := refused(t, s, `<wb:topic name="t"><wb:close force="yes"/></wb:topic>`)
	wantProblems(t, problems, `unknown attribute force (it takes none)`)
}

func TestCloseResultMigrationRecovery(t *testing.T) {
	dir := t.TempDir()
	at := "2026-11-17T01:02:03Z"
	legacy := &State{EntrySeq: 1, ClosedTopics: []string{"t"}, CloseResults: map[string]string{"t": "old conclusion"}, Topics: []TopicDoc{{Name: "t", Entries: []Entry{{ID: "e1", Kind: EntrySystem, Text: "Topic closed: old conclusion", At: at}}}}}
	crash := errors.New("crash")
	_, err := (FolderStore{Dir: dir, Fault: func(stage string) error {
		if stage == "intent" {
			return crash
		}
		return nil
	}}).Open(legacy)
	if !errors.Is(err, crash) {
		t.Fatalf("expected interrupted intent: %v", err)
	}
	first := folderOpen(t, dir, legacy)
	second := folderOpen(t, dir, legacy)
	for _, s := range []*State{first, second} {
		d := s.Topics[s.topicDocIndex("t")]
		if s.CloseResults["t"] != "" || len(d.Entries) != 2 || d.Entries[1].At != at || d.LatestSummary().Text != "old conclusion" {
			t.Fatalf("migration: %+v %+v", s.CloseResults, d.Entries)
		}
	}
	if strings.TrimSpace(first.Topics[0].Entries[1].Text) != "old conclusion" {
		t.Fatal("lost result")
	}
}

func TestExistingFolderCloseResultMigration(t *testing.T) {
	dir := t.TempDir()
	store := FolderStore{Dir: dir}
	legacy := &State{EntrySeq: 2, ClosedTopics: []string{"t"}, CloseResults: map[string]string{"t": "old"}, Topics: []TopicDoc{{Name: "t", Entries: []Entry{
		{ID: "e1", Kind: EntrySystem, Text: "Topic closed: old", At: "2026-11-17T01:00:00Z"},
		{ID: "e2", Kind: EntryAgent, Text: "newer", At: "2026-11-17T02:00:00Z"},
	}}}}
	intent, err := store.plan(nil, nil, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.commit(intent); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		s := folderOpen(t, dir, nil)
		d := s.Topics[s.topicDocIndex("t")]
		latest, ok := d.LatestAgent()
		if len(d.Entries) != 3 || d.Entries[2].At != "2026-11-17T01:00:00Z" || s.CloseResults["t"] != "" || d.LatestSummary().Summary() != "newer" || !ok || latest.Text != "newer" {
			t.Fatalf("existing migration: %+v", d.Entries)
		}
	}
}

func TestCloseResultMigrationUsesLatestLegacyResultUpdate(t *testing.T) {
	legacy := &State{EntrySeq: 3, ClosedTopics: []string{"t"}, CloseResults: map[string]string{"t": "revised"}, Topics: []TopicDoc{{Name: "t", Entries: []Entry{
		{ID: "e1", Kind: EntrySystem, Text: "Topic closed: original", At: "2026-11-17T01:00:00Z"},
		{ID: "e2", Kind: EntryAgent, Text: "intervening", At: "2026-11-17T02:00:00Z"},
		{ID: "e3", Kind: EntrySystem, Text: "Topic close result updated: revised", At: "2026-11-17T03:00:00Z"},
	}}}}
	dir := t.TempDir()
	for i := 0; i < 2; i++ {
		s := folderOpen(t, dir, legacy)
		d := s.Topics[s.topicDocIndex("t")]
		latest, ok := d.LatestAgent()
		if len(d.Entries) != 4 || d.Entries[3].At != "2026-11-17T03:00:00Z" || s.CloseResults["t"] != "" || !ok || latest.Text != "revised" || d.LatestSummary().Text != "revised" {
			t.Fatalf("updated legacy result migration: %+v", d.Entries)
		}
	}
}

func TestCloseResultMigrationWithoutCloseEventUsesKnownTime(t *testing.T) {
	legacy := &State{EntrySeq: 1, ClosedTopics: []string{"t"}, CloseResults: map[string]string{"t": "conclusion"}, Topics: []TopicDoc{{Name: "t", Entries: []Entry{{ID: "e1", Kind: EntryAgent, Text: "earlier recap", At: "2026-11-17T01:00:00Z"}}}}}
	s := folderOpen(t, t.TempDir(), legacy)
	d := s.Topics[s.topicDocIndex("t")]
	if got := d.LatestSummary(); got.Text != "conclusion" || got.At != "2026-11-17T01:00:00Z" {
		t.Fatalf("fallback must preserve conclusion as summary: %+v", got)
	}
}
