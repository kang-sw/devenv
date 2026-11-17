package wswhiteboard

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// folderPanelBoard seeds topics that each hold an agent entry and a panel,
// then ages every file past the memo's racy window, as a board is between
// commits in a long session.
func folderPanelBoard(tb testing.TB, topics int) FolderStore {
	tb.Helper()
	f := FolderStore{Dir: tb.TempDir()}
	if err := f.Update(nil, func(s *State) error {
		for i := 0; i < topics; i++ {
			s.EntrySeq += 2
			s.PanelCounter++
			s.Topics = append(s.Topics, TopicDoc{Name: fmt.Sprintf("topic-%d", i), Body: "unchanged topic body", Entries: []Entry{
				{ID: fmt.Sprintf("e%d", s.EntrySeq-1), Kind: EntryAgent, Text: "history", At: "2026-11-15T00:00:00Z"},
				{ID: fmt.Sprintf("e%d", s.EntrySeq), Kind: EntryPanel, PanelID: fmt.Sprintf("p%d", s.PanelCounter), Text: "panel body", At: "2026-11-15T00:00:00Z"},
			}})
		}
		s.migrateLegacyContent()
		return nil
	}); err != nil {
		tb.Fatal(err)
	}
	folderAge(tb, f.Dir)
	return f
}

func folderAge(tb testing.TB, dir string) {
	tb.Helper()
	old := time.Now().Add(-time.Hour)
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		return os.Chtimes(path, old, old)
	}); err != nil {
		tb.Fatal(err)
	}
}

// folderFresh decodes the board with no memo, as a new process would.
func folderFresh(t *testing.T, f FolderStore) []byte {
	t.Helper()
	forgetFolderMemo(f.Dir)
	state, err := FolderStore{Dir: f.Dir}.Open(nil)
	if err != nil {
		t.Fatal(err)
	}
	return folderJSON(state)
}

func folderAppendPanel(s *State, topic int, text string) {
	s.EntrySeq++
	s.PanelCounter++
	s.Topics[topic].Entries = append(s.Topics[topic].Entries, Entry{ID: fmt.Sprintf("e%d", s.EntrySeq), Kind: EntryPanel, PanelID: fmt.Sprintf("p%d", s.PanelCounter), Text: text, At: "2026-11-17T00:00:00Z"})
}

// A commit touching one topic costs the writer and the next reader a fixed
// number of reads, whatever the board size: untouched topics and committed
// panels come from the memo, revalidated by stat.
func TestFolderMemoCommitRereadsOnlyTouchedTopic(t *testing.T) {
	type counts struct{ guard, writer, open int }
	measure := func(t *testing.T, topics int) counts {
		f := folderPanelBoard(t, topics)
		var cache FolderCache
		if _, err := cache.Open(f, nil); err != nil {
			t.Fatal(err)
		}
		reads := countFolderReads(t)
		if err := f.guard(); err != nil {
			t.Fatal(err)
		}
		var c counts
		c.guard, *reads = *reads, 0
		if err := (FolderStore{Dir: f.Dir}).Update(nil, func(s *State) error {
			folderAppendPanel(s, 0, "new panel")
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		c.writer, *reads = *reads, 0
		got, err := cache.Open(f, nil)
		if err != nil {
			t.Fatal(err)
		}
		c.open = *reads
		entries := got.Topics[0].Entries
		if n := len(entries); n < 3 || entries[n-1].Text != "new panel" || entries[1].Text != "panel body" {
			t.Fatalf("touched topic entries = %+v", entries)
		}
		if want := folderFresh(t, f); !bytes.Equal(folderJSON(got), want) {
			t.Fatalf("memo read differs from a fresh decode\ngot  %s\nwant %s", folderJSON(got), want)
		}
		t.Logf("topics=%d reads: guard=%d writer=%d next open=%d", topics, c.guard, c.writer, c.open)
		return c
	}
	var small, large counts
	t.Run("small", func(t *testing.T) { small = measure(t, 4) })
	t.Run("large", func(t *testing.T) { large = measure(t, 40) })
	if small != large {
		t.Fatalf("reads grow with board size: 4 topics %+v, 40 topics %+v", small, large)
	}
	// One read each of meta.json, pending.json (absent) and committed.json.
	if large.guard != 3 {
		t.Fatalf("guard reads = %d, want 3", large.guard)
	}
	if large.open > 16 {
		t.Fatalf("next open after a one-topic commit read %d files", large.open)
	}
}

// TestFolderMemoProcessWorker is the other process of the test below.
func TestFolderMemoProcessWorker(t *testing.T) {
	dir := os.Getenv("WS_WHITEBOARD_MEMO_TEST")
	if dir == "" {
		return
	}
	if err := (FolderStore{Dir: dir}).Update(nil, func(s *State) error {
		s.Topics[1].Body = "external topic body!" // same length: meta.json size is unchanged
		folderAppendPanel(s, 2, "external panel")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Another process's commit reaches this process's cached reader and writer
// even with every topic memoized: nothing relies on a write-through.
func TestFolderMemoSeesAnotherProcessCommit(t *testing.T) {
	f := folderPanelBoard(t, 4)
	var cache FolderCache
	if _, err := cache.Open(f, nil); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestFolderMemoProcessWorker$")
	cmd.Env = append(os.Environ(), "WS_WHITEBOARD_MEMO_TEST="+f.Dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("external writer: %v\n%s", err, out)
	}
	got, err := cache.Open(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Topics[1].Body != "external topic body!" || got.Topics[2].Entries[len(got.Topics[2].Entries)-1].Text != "external panel" {
		t.Fatalf("missed another process's commit: %s", folderJSON(got))
	}
	// The writer's plan compares topic metadata through the memo too.
	if err := (FolderStore{Dir: f.Dir}).Update(nil, func(s *State) error {
		if s.Topics[1].Body != "external topic body!" {
			return errors.New("writer read a stale topic")
		}
		s.Topics[1].Body = "local write"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := cache.Open(f, nil); err != nil || got.Topics[1].Body != "local write" {
		t.Fatalf("local write after external commit: %v", err)
	}
	if want := folderFresh(t, f); !bytes.Equal(folderJSON(mustOpen(t, f)), want) {
		t.Fatal("memo read differs from a fresh decode")
	}
}

func mustOpen(t *testing.T, f FolderStore) *State {
	t.Helper()
	state, err := f.Open(nil)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

// An interrupted commit's pending intent is still replayed before a memoized
// board is served.
func TestFolderMemoPendingRecovery(t *testing.T) {
	f := folderPanelBoard(t, 4)
	var cache FolderCache
	if _, err := cache.Open(f, nil); err != nil {
		t.Fatal(err)
	}
	crash := FolderStore{Dir: f.Dir, Fault: func(stage string) error {
		if stage == "intent" {
			return errors.New("crash after intent")
		}
		return nil
	}}
	if err := crash.Update(nil, func(s *State) error {
		folderAppendPanel(s, 3, "recovered panel")
		return nil
	}); err == nil {
		t.Fatal("fault did not interrupt the commit")
	}
	if _, err := os.Stat(filepath.Join(f.Dir, "pending.json")); err != nil {
		t.Fatalf("no pending intent left: %v", err)
	}
	got, err := cache.Open(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if e := got.Topics[3].Entries; e[len(e)-1].Text != "recovered panel" {
		t.Fatalf("pending intent not replayed: %s", folderJSON(got))
	}
	if _, err := os.Stat(filepath.Join(f.Dir, "pending.json")); !os.IsNotExist(err) {
		t.Fatalf("pending not removed: %v", err)
	}
	if want := folderFresh(t, f); !bytes.Equal(folderJSON(got), want) {
		t.Fatal("memo read differs from a fresh decode")
	}
}
