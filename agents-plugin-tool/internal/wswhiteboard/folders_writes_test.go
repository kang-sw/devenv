package wswhiteboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func folderMetaIdentities(t *testing.T, dir string) map[string]os.FileInfo {
	t.Helper()
	var meta folderSession
	if _, err := folderDecode(filepath.Join(dir, "meta.json"), &meta); err != nil {
		t.Fatal(err)
	}
	out := map[string]os.FileInfo{}
	for _, ref := range meta.Topics {
		path := ref.Dir + "/meta.json"
		file, err := os.Open(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		// File.Stat captures the identity now; Windows os.Stat defers its
		// file-ID lookup until SameFile, after the path may be replaced.
		info, err := file.Stat()
		closeErr := file.Close() // Release the Windows reader hold before writes.
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		out[path] = info
	}
	return out
}

// Check both the durable decision and actual replacements, not just final bytes:
// rewriting identical metadata would leave the final bytes unchanged too.
func folderCheckMetaWrites(t *testing.T, f FolderStore, change func(*State) error, wantPaths ...string) {
	t.Helper()
	before := folderMetaIdentities(t, f.Dir)
	var paths []string
	counts := map[string]int{}
	pendingBytes := 0
	f.Fault = func(stage string) error {
		switch stage {
		case "intent", "panel", "topic-meta", "session-meta", "cleanup":
			counts[stage]++ // each follows one successful atomic replacement
		}
		if stage == "intent" {
			data := folderRead(t, filepath.Join(f.Dir, "pending.json"))
			pendingBytes = len(data)
			var intent folderIntent
			if err := json.Unmarshal(data, &intent); err != nil {
				t.Fatal(err)
			}
			for _, w := range intent.Topics {
				paths = append(paths, w.Path)
			}
		}
		if stage == "log" {
			counts[stage]++
		}
		return nil
	}
	if err := f.Update(nil, change); err != nil {
		t.Fatal(err)
	}
	total := counts["intent"] + counts["panel"] + counts["topic-meta"] + counts["session-meta"] + counts["cleanup"]
	t.Logf("topic_meta_replaces=%d total_atomic_replaces=%d pending_topics=%d pending_bytes=%d log_appends=%d paths=%v", counts["topic-meta"], total, len(paths), pendingBytes, counts["log"], paths)
	if !reflect.DeepEqual(paths, wantPaths) || counts["topic-meta"] != len(wantPaths) {
		t.Fatalf("metadata writes = %v (%d replacements), want %v", paths, counts["topic-meta"], wantPaths)
	}
	if counts["intent"] != 1 || counts["session-meta"] != 1 || counts["cleanup"] != 1 || total != len(wantPaths)+3 {
		t.Fatalf("unexpected atomic replacements: %v", counts)
	}
	for path, info := range before {
		after, err := os.Stat(filepath.Join(f.Dir, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		written := false
		for _, want := range wantPaths {
			written = written || path == want
		}
		if os.SameFile(info, after) == written {
			t.Fatalf("%s: identity changed=%v, want %v", path, !os.SameFile(info, after), written)
		}
	}
}

func TestFolderChangedTopicMeta112(t *testing.T) {
	f := folderIdleBoard(t)
	t.Run("document", func(t *testing.T) {
		folderCheckMetaWrites(t, f, func(s *State) error {
			s.Topics[0].Body = "changed only topic 0"
			return nil
		}, "t1/meta.json")
	})
	t.Run("item", func(t *testing.T) {
		folderCheckMetaWrites(t, f, func(s *State) error {
			s.Counter++
			s.Items = append(s.Items, Item{ID: s.Counter, Topic: "topic-1", Title: "new item"})
			return nil
		}, "t2/meta.json")
	})
	t.Run("session-only", func(t *testing.T) {
		folderCheckMetaWrites(t, f, func(s *State) error { s.Seq++; s.LastEmbedSeq = s.Seq; return nil })
	})
	t.Run("log-only", func(t *testing.T) {
		folderCheckMetaWrites(t, f, func(s *State) error {
			s.EntrySeq++
			s.Topics[0].Entries = append(s.Topics[0].Entries, Entry{ID: fmt.Sprintf("e%d", s.EntrySeq), Kind: EntryAgent, Text: "log-only change"})
			return nil
		})
	})
	t.Run("new-topic", func(t *testing.T) {
		folderCheckMetaWrites(t, f, func(s *State) error {
			s.Topics = append(s.Topics, TopicDoc{Name: "new", Body: "new metadata"})
			return nil
		}, "t113/meta.json")
	})
}

func TestFolderChangedTopicMetaRecovery(t *testing.T) {
	for _, stage := range []string{"intent", "log-partial", "topic-meta", "session-meta", "cleanup"} {
		t.Run(stage, func(t *testing.T) {
			f := folderIdleBoard(t)
			before := folderMetaIdentities(t, f.Dir)
			want := folderOpen(t, f.Dir, nil)
			change := func(s *State) error {
				s.Topics[0].Body = "committed despite crash"
				s.EntrySeq++
				s.Topics[0].Entries = append(s.Topics[0].Entries, Entry{ID: fmt.Sprintf("e%d", s.EntrySeq), Kind: EntryAgent, Text: "recover once"})
				return nil
			}
			if err := change(want); err != nil {
				t.Fatal(err)
			}
			f.Fault = func(hit string) error {
				if hit == stage {
					return folderCrash
				}
				return nil
			}
			observed := 0
			if err := f.UpdateAndPublishObserved(nil, change, func() { observed++ }, nil); !errors.Is(err, folderCrash) {
				t.Fatalf("fault not reached: %v", err)
			}
			var intent folderIntent
			if err := json.Unmarshal(folderRead(t, filepath.Join(f.Dir, "pending.json")), &intent); err != nil {
				t.Fatal(err)
			}
			if len(intent.Topics) != 1 || intent.Topics[0].Path != "t1/meta.json" || len(intent.Session.Topics) != 112 {
				t.Fatalf("pending did not skip unchanged topics: writes=%d refs=%d", len(intent.Topics), len(intent.Session.Topics))
			}
			for range 2 {
				if got := folderOpen(t, f.Dir, nil); !reflect.DeepEqual(want, got) {
					t.Fatalf("recovery mismatch\nwant %s\ngot %s", folderJSON(want), folderJSON(got))
				}
			}
			if observed != 1 {
				t.Fatalf("durable decision observed %d times", observed)
			}
			for path, info := range before {
				if path != "t1/meta.json" {
					after, err := os.Stat(filepath.Join(f.Dir, filepath.FromSlash(path)))
					if err != nil || !os.SameFile(info, after) {
						t.Fatalf("recovery replaced skipped %s: %v", path, err)
					}
				}
			}
		})
	}
}

func TestFolderChangedTopicMetaLifecycle(t *testing.T) {
	f := FolderStore{Dir: t.TempDir()}
	// Avoid the model's intentional first-update timestamp backfill on legacy
	// bodies, so the second topic really is unchanged by these commands.
	folderOpen(t, f.Dir, &State{Topics: []TopicDoc{
		{Name: "old title", Body: "body", ChangedAt: "2026-11-01T00:00:00Z"},
		{Name: "unchanged", Body: "leave alone", ChangedAt: "2026-11-01T00:00:00Z"},
	}})
	for _, commands := range []string{
		`<wb:topic name="stem" label="old title"/>`,
		`<wb:topic name="stem" label="New label"/>`,
		`<wb:topic name="stem"><wb:close result="Done"/></wb:topic>`,
		`<wb:topic name="stem"><wb:body>Reopened body</wb:body></wb:topic>`,
	} {
		t.Run(commands, func(t *testing.T) {
			var want *State
			folderCheckMetaWrites(t, f, func(s *State) error {
				mustApply(t, s, commands)
				want = s.clone()
				return nil
			}, "t1/meta.json")
			if got := folderOpen(t, f.Dir, nil); !reflect.DeepEqual(want, got) {
				t.Fatalf("lifecycle read mismatch\nwant %s\ngot %s", folderJSON(want), folderJSON(got))
			}
			if commands == `<wb:topic name="stem"><wb:body>Reopened body</wb:body></wb:topic>` && want.topicClosed("stem") {
				t.Fatal("body activity did not reopen topic")
			}
		})
	}
}

func TestFolderChangedTopicMetaNewMappingExistingBytes(t *testing.T) {
	f := FolderStore{Dir: t.TempDir()}
	folderOpen(t, f.Dir, &State{Topics: []TopicDoc{{Name: "old", Body: "body"}, {Name: "unchanged"}}})
	var topic folderTopic
	if err := json.Unmarshal(folderRead(t, filepath.Join(f.Dir, "t1", "meta.json")), &topic); err != nil {
		t.Fatal(err)
	}
	topic.Doc.Name = "renamed"
	// A rename without stable item/history anchors allocates a fresh directory.
	// Even identical bytes in an orphan at that path must not elide publication
	// of the new mapping.
	if err := os.MkdirAll(filepath.Join(f.Dir, "t3"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.Dir, "t3", "meta.json"), folderJSON(topic), 0o644); err != nil {
		t.Fatal(err)
	}
	folderCheckMetaWrites(t, f, func(s *State) error { s.Topics[0].Name = "renamed"; return nil }, "t3/meta.json")
	if got := folderOpen(t, f.Dir, nil); got.Topics[0].Name != "renamed" {
		t.Fatal("new directory mapping lost rename")
	}
}

func TestFolderChangedTopicMetaExactDiskBytes(t *testing.T) {
	f := folderIdleBoard(t)
	path := filepath.Join(f.Dir, "t2", "meta.json")
	data := folderRead(t, path)
	// Equivalent JSON is deliberately not byte-identical. Compare the actual
	// committed file, not a lossy reconstruction from the decoded State.
	if err := os.WriteFile(path, append([]byte(" \n"), data...), 0o644); err != nil {
		t.Fatal(err)
	}
	folderCheckMetaWrites(t, f, func(s *State) error { s.Seq++; return nil }, "t2/meta.json")
	if !bytes.Equal(folderRead(t, path), data) {
		t.Fatal("metadata was not serialized exactly")
	}
}
