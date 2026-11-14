package wswhiteboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

var folderCrash = errors.New("injected folder crash")

func folderRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func folderTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() != ".lock" {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = string(folderRead(t, path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func folderOpen(t *testing.T, dir string, legacy *State) *State {
	t.Helper()
	state, err := (FolderStore{Dir: dir}).Open(legacy)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func folderFixture() *State {
	return &State{
		Counter: 7, Seq: 9, SettleSeq: 3, AnswerSeq: 8,
		Topics: []TopicDoc{
			{Name: "../CON: topic / 雪", Body: "  <svg>\n<script>x</script>\n</svg>\n", Changed: 6, ChangedAt: "2026-11-01T01:00:00Z", Recap: "old recap {{7}}", RecapAt: "2026-11-02T02:03:04Z", RecapChanged: 8},
			{Name: "recap-only", Recap: "old closed", RecapAt: "2026-10-01T01:02:03Z"},
		},
		Items: []Item{
			{ID: 7, Topic: "recap-only", Class: ClassPolicy, Status: StatusAnswered, Title: "choice", Answer: "b", AnswerQuote: "owner's exact words\n", AnsweredAt: "2026-10-01T01:00:00Z", Settled: 3},
			{ID: 2, Topic: "../CON: topic / 雪", Class: ClassCheck, Status: StatusOpen, Title: "verify", Options: []string{"a", "b"}, Rationale: "why", Links: []string{"a/b"}},
			{ID: 4, Topic: "recap-only", Class: ClassTechnical, Status: StatusWithdrawn, Title: "done"},
		},
		Labels:       map[string]string{"recap-only": "Closed topic"},
		ClosedTopics: []string{"recap-only", "empty closed"}, CloseResults: map[string]string{"recap-only": "Finished.", "empty closed": "No items."},
		LastEmbedSeq: 8, Embeds: []Embed{{Seq: 8, Name: "embed8.html"}}, EmbedDir: "D:/old/embed", LastNudgeAt: "2026-11-02T00:00:00Z",
	}
}

func folderAdd(s *State) error {
	s.Seq++
	s.PanelCounter++
	panelID := fmt.Sprintf("p%d", s.PanelCounter)
	for _, e := range []Entry{
		{Kind: EntryPanel, PanelID: panelID, Recap: "mockup", Text: " \n<wb:x a=\"雪\">\n<script>panel-body-secret</script>\n</wb:x>\r\n", At: "2026-11-13T00:00:00Z"},
		{Kind: EntryAgent, Text: "same time, later ID", At: "2026-11-13T00:00:00Z"},
		{Kind: EntryPanel, PanelID: panelID, Recap: "second version", Text: "new full version\n", At: "2026-11-13T00:00:00Z"},
	} {
		s.EntrySeq++
		e.ID = fmt.Sprintf("e%d", s.EntrySeq)
		s.Topics[0].Entries = append(s.Topics[0].Entries, e)
	}
	s.Topics[0].Body = "new current content"
	return nil
}

func TestFolderAbsentAndEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	f := FolderStore{Dir: dir}
	got, err := f.Open(nil)
	if err != nil || got != nil {
		t.Fatalf("Open(nil) = %#v, %v", got, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent open created directory: %v", err)
	}
	if err := f.Update(nil, func(s *State) error {
		if s == nil {
			t.Fatal("nil update state")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got = folderOpen(t, dir, folderFixture())
	if got == nil || len(got.Topics) != 0 || got.Counter != 0 {
		t.Fatalf("empty committed board fell back: %#v", got)
	}
	before := folderTree(t, dir)
	if err := f.Update(nil, func(*State) error { return folderCrash }); !errors.Is(err, folderCrash) {
		t.Fatal(err)
	}
	if after := folderTree(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatal("refused mutation wrote store")
	}
	// Even a committed empty board cannot resurrect legacy when metadata is lost.
	if err := os.Remove(filepath.Join(dir, "meta.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Open(folderFixture()); err == nil || !strings.Contains(err.Error(), "refusing legacy fallback") {
		t.Fatalf("missing committed metadata: %v", err)
	}
}

func TestFolderConversionAndRestart(t *testing.T) {
	root := t.TempDir()
	legacy := folderFixture()
	legacyBytes := folderJSON(legacy)
	legacyPath := filepath.Join(root, "preserved-whiteboard.json")
	if err := os.WriteFile(legacyPath, legacyBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "reply-id")
	got := folderOpen(t, dir, legacy)
	if !bytes.Equal(folderJSON(legacy), legacyBytes) {
		t.Fatal("conversion mutated legacy model")
	}
	if !bytes.Equal(folderRead(t, legacyPath), legacyBytes) {
		t.Fatal("conversion changed preserved legacy bytes")
	}
	want := legacy.clone()
	want.EntrySeq = 2
	for i := range want.Topics {
		d := &want.Topics[i]
		d.Entries = []Entry{{ID: fmt.Sprintf("e%d", i+1), Kind: EntryAgent, Text: d.Recap, At: d.RecapAt, Imported: true}}
		d.Recap, d.RecapAt, d.RecapChanged = "", "", 0
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("conversion lost snapshots\nwant %s\ngot  %s", folderJSON(want), folderJSON(got))
	}
	// Second open is read-only and ignores even a different stale legacy board.
	before := folderTree(t, dir)
	stale := &State{Counter: 999, Topics: []TopicDoc{{Name: "stale", Recap: "do not import"}}}
	if again := folderOpen(t, dir, stale); !reflect.DeepEqual(want, again) {
		t.Fatal("restart reimported legacy")
	}
	if !reflect.DeepEqual(before, folderTree(t, dir)) {
		t.Fatal("second open rewrote authority")
	}
	var meta folderSession
	if _, err := folderDecode(filepath.Join(dir, "meta.json"), &meta); err != nil {
		t.Fatal(err)
	}
	for _, ref := range meta.Topics {
		if !folderDirID(ref.Dir) || strings.Contains(ref.Dir, ref.Key) {
			t.Fatalf("unsafe directory mapping: %+v", ref)
		}
		var topic folderTopic
		if _, err := folderDecode(filepath.Join(dir, ref.Dir, "meta.json"), &topic); err != nil {
			t.Fatal(err)
		}
		if topic.SchemaVersion != FolderSchemaVersion {
			t.Fatal("unversioned topic")
		}
	}
}

func TestFolderImmutableVersionsAndLog(t *testing.T) {
	dir := t.TempDir()
	start := folderOpen(t, dir, folderFixture())
	want := start.clone()
	if err := folderAdd(want); err != nil {
		t.Fatal(err)
	}
	before := folderTree(t, dir)
	if err := (FolderStore{Dir: dir}).Update(nil, folderAdd); err != nil {
		t.Fatal(err)
	}
	if got := folderOpen(t, dir, nil); !reflect.DeepEqual(want, got) {
		t.Fatalf("restart mismatch\nwant %s\ngot %s", folderJSON(want), folderJSON(got))
	}
	after := folderTree(t, dir)
	for path, old := range before {
		if strings.HasSuffix(path, "log.jsonl") && !strings.HasPrefix(after[path], old) {
			t.Fatalf("log rewritten: %s", path)
		}
	}
	panels := 0
	for path, data := range after {
		if strings.Contains(path, "/panels/") {
			panels++
		}
		if strings.HasSuffix(path, "log.jsonl") && strings.Contains(data, "panel-body-secret") {
			t.Fatal("panel body leaked into log")
		}
		if strings.HasSuffix(path, "/meta.json") && strings.Contains(data, "\"entries\"") {
			t.Fatal("entries duplicated into topic metadata")
		}
	}
	if panels != 2 {
		t.Fatalf("panel versions = %d", panels)
	}
	// Render-only bookkeeping never changes log bytes or immutable versions.
	if err := (FolderStore{Dir: dir}).Update(nil, func(s *State) error {
		s.Seq++
		s.LastEmbedSeq = s.Seq
		s.Embeds = append(s.Embeds, Embed{Seq: s.Seq, Name: "render.html"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for path, data := range after {
		if strings.Contains(path, "/panels/") || strings.HasSuffix(path, "log.jsonl") {
			if string(folderRead(t, filepath.Join(dir, filepath.FromSlash(path)))) != data {
				t.Fatalf("render rewrote %s", path)
			}
		}
	}
}

func TestFolderInterleavedEntryAndItemOrder(t *testing.T) {
	dir := t.TempDir()
	want := &State{Counter: 3, EntrySeq: 4, PanelCounter: 1,
		Topics: []TopicDoc{
			{Name: "B", Entries: []Entry{{ID: "e2", Kind: EntrySystem, ItemID: 3, Text: "two", At: "same"}, {ID: "e4", Kind: EntryAgent, Text: "four", At: "same"}}},
			{Name: "A", Entries: []Entry{{ID: "e1", Kind: EntryPanel, PanelID: "p1", Recap: "one", Text: "\n raw payload \r\n", At: "same"}, {ID: "e3", Kind: EntryAgent, Text: "three", At: "same"}}},
		}, Items: []Item{{ID: 3, Topic: "A"}, {ID: 1, Topic: "B"}, {ID: 2, Topic: "A"}},
	}
	if got := folderOpen(t, dir, want); !reflect.DeepEqual(want, got) {
		t.Fatal("initial order changed")
	}
	if got := folderOpen(t, dir, nil); !reflect.DeepEqual(want, got) {
		t.Fatalf("restart order changed\n%s", folderJSON(got))
	}
}

func TestFolderRecoveryEveryBoundary(t *testing.T) {
	// Trace a real multi-panel, multi-topic commit, then crash at every boundary
	// occurrence, not just the first file of a kind.
	traceDir := t.TempDir()
	initial := folderOpen(t, traceDir, folderFixture())
	var trace []string
	if err := (FolderStore{Dir: traceDir, Fault: func(stage string) error { trace = append(trace, stage); return nil }}).Update(nil, folderAdd); err != nil {
		t.Fatal(err)
	}
	want := folderOpen(t, traceDir, nil)
	wantTree := folderTree(t, traceDir)
	seen := map[string]int{}
	for index, stage := range trace {
		seen[stage]++
		t.Run(fmt.Sprintf("%s-%d", stage, seen[stage]), func(t *testing.T) {
			dir := t.TempDir()
			folderOpen(t, dir, folderFixture())
			calls, callbackCalls := 0, 0
			f := FolderStore{Dir: dir, Fault: func(string) error {
				hit := calls
				calls++
				if hit == index {
					return folderCrash
				}
				return nil
			}}
			err := f.Update(nil, func(s *State) error { callbackCalls++; return folderAdd(s) })
			if !errors.Is(err, folderCrash) {
				t.Fatalf("fault not reached: %v", err)
			}
			got := folderOpen(t, dir, folderFixture())
			expected := want
			if stage == "intent-staged" {
				expected = initial
			}
			if !reflect.DeepEqual(expected, got) {
				t.Fatalf("recovery mismatch at %s\nwant %s\ngot %s", stage, folderJSON(expected), folderJSON(got))
			}
			if callbackCalls != 1 {
				t.Fatal("recovery replayed mutation callback")
			}
			if stage != "intent-staged" && !reflect.DeepEqual(wantTree, folderTree(t, dir)) {
				t.Fatal("replay duplicated or changed committed bytes")
			}
			before := folderTree(t, dir)
			folderOpen(t, dir, folderFixture())
			if !reflect.DeepEqual(before, folderTree(t, dir)) {
				t.Fatal("second recovery was not idempotent")
			}
		})
	}
	for _, required := range []string{"intent-staged", "intent", "panel-staged", "panel", "log-partial", "log", "topic-meta-staged", "topic-meta", "session-meta-staged", "session-meta", "receipt-staged", "cleanup", "complete"} {
		if seen[required] == 0 {
			t.Fatalf("missing fault boundary %s", required)
		}
	}
}

func TestFolderConversionRecoveryEveryBoundary(t *testing.T) {
	traceDir := t.TempDir()
	var trace []string
	f := FolderStore{Dir: traceDir, Fault: func(stage string) error { trace = append(trace, stage); return nil }}
	want, err := f.Open(folderFixture())
	if err != nil {
		t.Fatal(err)
	}
	for index, stage := range trace {
		t.Run(fmt.Sprintf("%02d-%s", index, stage), func(t *testing.T) {
			dir := t.TempDir()
			legacy := folderFixture()
			original := folderJSON(legacy)
			calls := 0
			f := FolderStore{Dir: dir, Fault: func(string) error {
				hit := calls
				calls++
				if hit == index {
					return folderCrash
				}
				return nil
			}}
			if _, err := f.Open(legacy); !errors.Is(err, folderCrash) {
				t.Fatalf("fault not reached: %v", err)
			}
			if !bytes.Equal(original, folderJSON(legacy)) {
				t.Fatal("failed conversion changed legacy")
			}
			// After intent, recovery succeeds without any legacy representation.
			recoveryLegacy := legacy
			if stage != "conversion" && stage != "intent-staged" {
				recoveryLegacy = nil
			}
			if got := folderOpen(t, dir, recoveryLegacy); !reflect.DeepEqual(want, got) {
				t.Fatalf("conversion recovery mismatch\nwant %s\ngot %s", folderJSON(want), folderJSON(got))
			}
			if got := folderOpen(t, dir, legacy); !reflect.DeepEqual(want, got) {
				t.Fatal("duplicate import")
			}
		})
	}
}

func TestFolderRecoveryRepeatedPartialAppend(t *testing.T) {
	dir := t.TempDir()
	folderOpen(t, dir, folderFixture())
	f := FolderStore{Dir: dir, Fault: func(stage string) error {
		if stage == "log-partial" {
			return folderCrash
		}
		return nil
	}}
	if err := f.Update(nil, folderAdd); !errors.Is(err, folderCrash) {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := f.Open(nil); !errors.Is(err, folderCrash) {
			t.Fatalf("recovery fault: %v", err)
		}
	}
	got := folderOpen(t, dir, nil)
	if len(got.Topics[0].Entries) != 4 || got.EntrySeq != 5 {
		t.Fatalf("duplicated entries: %s", folderJSON(got))
	}
}

func TestFolderNewerSchemaRefusalBeforeRecovery(t *testing.T) {
	for _, target := range []string{"session", "topic", "intent", "intent-session", "intent-topic", "receipt"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			folderOpen(t, dir, folderFixture())
			f := FolderStore{Dir: dir, Fault: func(stage string) error {
				if stage == "intent" {
					return folderCrash
				}
				return nil
			}}
			if err := f.Update(nil, folderAdd); !errors.Is(err, folderCrash) {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "meta.json")
			switch target {
			case "topic":
				path = filepath.Join(dir, "t1", "meta.json")
			case "intent", "intent-session", "intent-topic":
				path = filepath.Join(dir, "pending.json")
			case "receipt":
				path = filepath.Join(dir, "committed.json")
			}
			data := folderRead(t, path)
			if target == "intent-session" || target == "intent-topic" {
				var intent folderIntent
				if err := json.Unmarshal(data, &intent); err != nil {
					t.Fatal(err)
				}
				if target == "intent-session" {
					intent.Session.SchemaVersion++
				} else {
					intent.Topics[0].Data = bytes.Replace(intent.Topics[0].Data, []byte(`"schema_version":2`), []byte(`"schema_version":3`), 1)
				}
				data = folderJSON(intent)
			} else {
				data = bytes.Replace(data, []byte(`"schema_version":2`), []byte(`"schema_version":3`), 1)
			}
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
			before := folderTree(t, dir)
			if _, err := (FolderStore{Dir: dir}).Open(folderFixture()); err == nil || !strings.Contains(err.Error(), "schema_version") {
				t.Fatalf("newer schema accepted: %v", err)
			}
			called := false
			if err := (FolderStore{Dir: dir}).Update(nil, func(*State) error { called = true; return nil }); err == nil {
				t.Fatal("newer write accepted")
			}
			if called || !reflect.DeepEqual(before, folderTree(t, dir)) {
				t.Fatal("newer schema caused writes/recovery")
			}
		})
	}
}

func TestFolderUnreadableAuthorityNeverFallsBack(t *testing.T) {
	for _, target := range []string{"meta.json", "t1/meta.json", "t1/log.jsonl"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			folderOpen(t, dir, folderFixture())
			if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(target)), []byte("broken"), 0o644); err != nil {
				t.Fatal(err)
			}
			before := folderTree(t, dir)
			if _, err := (FolderStore{Dir: dir}).Open(folderFixture()); err == nil {
				t.Fatal("corrupt committed authority fell back")
			}
			if !reflect.DeepEqual(before, folderTree(t, dir)) {
				t.Fatal("corrupt authority rewritten")
			}
		})
	}
}

func TestFolderAppendOnlyValidation(t *testing.T) {
	cases := map[string]func(*State){
		"edit":          func(s *State) { s.Topics[0].Entries[0].Text = "changed" },
		"delete":        func(s *State) { s.Topics[0].Entries = s.Topics[0].Entries[1:] },
		"counter":       func(s *State) { s.EntrySeq-- },
		"panel-counter": func(s *State) { s.PanelCounter-- },
		"reorder":       func(s *State) { e := s.Topics[0].Entries; e[0], e[1] = e[1], e[0] },
		"duplicate":     func(s *State) { s.Topics[1].Entries = append(s.Topics[1].Entries, s.Topics[0].Entries[1]) },
		"remove-topic":  func(s *State) { s.Topics = s.Topics[1:]; s.Items = nil },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			folderOpen(t, dir, folderFixture())
			f := FolderStore{Dir: dir}
			if err := f.Update(nil, folderAdd); err != nil {
				t.Fatal(err)
			}
			before := folderTree(t, dir)
			if err := f.Update(nil, func(s *State) error { change(s); return nil }); err == nil {
				t.Fatal("non-append change accepted")
			}
			if !reflect.DeepEqual(before, folderTree(t, dir)) {
				t.Fatal("invalid change wrote store")
			}
		})
	}
}

func TestFolderTopicAdoptionKeepsDirectory(t *testing.T) {
	for _, withHistory := range []bool{false, true} {
		t.Run(fmt.Sprint(withHistory), func(t *testing.T) {
			dir := t.TempDir()
			legacy := &State{Topics: []TopicDoc{{Name: "old title", Body: "body"}}}
			if withHistory {
				legacy.Topics[0].Recap = "history"
			}
			folderOpen(t, dir, legacy)
			before := folderTree(t, dir)
			if err := (FolderStore{Dir: dir}).Update(nil, func(s *State) error {
				s.Topics[0].Name = "stem"
				s.Labels = map[string]string{"stem": "old title"}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			var meta folderSession
			if _, err := folderDecode(filepath.Join(dir, "meta.json"), &meta); err != nil {
				t.Fatal(err)
			}
			if len(meta.Topics) != 1 || meta.Topics[0].Dir != "t1" || meta.Topics[0].Key != "stem" {
				t.Fatalf("topic directory moved: %+v", meta.Topics)
			}
			if got := folderOpen(t, dir, nil); got.Topics[0].Name != "stem" || got.TopicLabel("stem") != "old title" {
				t.Fatal("adoption lost identity")
			}
			if string(folderRead(t, filepath.Join(dir, "t1", "log.jsonl"))) != before["t1/log.jsonl"] {
				t.Fatal("adoption rewrote history")
			}
		})
	}
}

func TestFolderConversionAdoptionIgnoresStaleCloseMapping(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(fmt.Sprint(closed), func(t *testing.T) {
			dir := t.TempDir()
			legacy := &State{
				Topics:       []TopicDoc{{Name: "Old title", Body: "body", Recap: "imported history", RecapAt: "2026-11-01T01:02:03Z"}},
				ClosedTopics: []string{"s"}, CloseResults: map[string]string{"s": "stale destination result"},
			}
			converted := folderOpen(t, dir, legacy)
			wantImport := []Entry{{ID: "e1", Kind: EntryAgent, Text: "imported history", At: legacy.Topics[0].RecapAt, Imported: true}}
			if !reflect.DeepEqual(entries(t, converted, "Old title"), wantImport) {
				t.Fatal("conversion lost imported entry identity")
			}
			f := FolderStore{Dir: dir}
			if closed {
				// Also retain panel files and the source's own system close event.
				if err := f.Update(nil, func(s *State) error {
					mustApply(t, s, `<wb:topic name="Old title"><wb:panel recap="mockup">immutable payload</wb:panel><wb:close result="source result"/></wb:topic>`)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			before := folderOpen(t, dir, nil)
			var beforeMeta folderSession
			if _, err := folderDecode(filepath.Join(dir, "meta.json"), &beforeMeta); err != nil {
				t.Fatal(err)
			}
			if len(beforeMeta.Topics) != 2 || beforeMeta.Topics[0].Key != "Old title" || beforeMeta.Topics[1].Key != "s" {
				t.Fatalf("missing retained/stale mappings: %+v", beforeMeta.Topics)
			}
			source := beforeMeta.Topics[0]
			if !folderDirID(source.Dir) {
				t.Fatalf("not an opaque topic directory: %q", source.Dir)
			}
			history := folderTree(t, dir)
			var adopted *State
			if err := f.Update(nil, func(s *State) error {
				mustApply(t, s, `<wb:topic name="s" label="Old title"/>`)
				adopted = s.clone()
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if len(adopted.Topics) != 1 || adopted.Topics[0].Name != "s" || adopted.TopicLabel("s") != "Old title" || adopted.TopicBody("s") != "body" {
				t.Fatalf("adoption lost topic identity: %s", folderJSON(adopted))
			}
			if adopted.EntrySeq != before.EntrySeq || !reflect.DeepEqual(adopted.Topics[0].Entries, before.Topics[0].Entries) {
				t.Fatal("adoption changed entry identity or appended events")
			}
			var wantClosed []string
			wantResult := ""
			if closed {
				wantClosed, wantResult = []string{"s"}, "source result"
			}
			if !reflect.DeepEqual(adopted.ClosedTopics, wantClosed) || len(adopted.CloseResults) != len(wantClosed) || adopted.CloseResults["s"] != wantResult {
				t.Fatalf("adoption retained stale destination close state: %s", folderJSON(adopted))
			}
			var afterMeta folderSession
			if _, err := folderDecode(filepath.Join(dir, "meta.json"), &afterMeta); err != nil {
				t.Fatal(err)
			}
			wantRef := source
			wantRef.Key = "s"
			if len(afterMeta.Topics) != 1 || afterMeta.Topics[0] != wantRef || afterMeta.NextTopic != beforeMeta.NextTopic {
				t.Fatalf("adoption changed directory/log mapping: %+v", afterMeta.Topics)
			}
			for path, data := range history {
				if strings.HasSuffix(path, "/log.jsonl") || strings.Contains(path, "/panels/") {
					if string(folderRead(t, filepath.Join(dir, filepath.FromSlash(path)))) != data {
						t.Fatalf("adoption rewrote history: %s", path)
					}
				}
			}
			after := folderTree(t, dir)
			for _, snapshot := range []*State{nil, legacy} {
				if got := folderOpen(t, dir, snapshot); !reflect.DeepEqual(got, adopted) {
					t.Fatalf("restart changed adopted entries or reimported legacy: %s", folderJSON(got))
				}
				if !reflect.DeepEqual(after, folderTree(t, dir)) {
					t.Fatal("restart rewrote authority or duplicated an import")
				}
			}
		})
	}
}

func TestFolderConcurrentWriters(t *testing.T) {
	dir := t.TempDir()
	folderOpen(t, dir, &State{Topics: []TopicDoc{{Name: "topic", Body: "body"}}})
	const writers = 12
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- (FolderStore{Dir: dir}).Update(folderFixture(), func(s *State) error {
				s.EntrySeq++
				s.Topics[0].Entries = append(s.Topics[0].Entries, Entry{ID: fmt.Sprintf("e%d", s.EntrySeq), Kind: EntryAgent, Text: fmt.Sprint(i), At: "equal time"})
				return nil
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got := folderOpen(t, dir, nil)
	if len(got.Topics[0].Entries) != writers || got.EntrySeq != writers {
		t.Fatalf("lost concurrent writes: %s", folderJSON(got))
	}
	texts := map[string]bool{}
	for i, e := range got.Topics[0].Entries {
		if e.ID != fmt.Sprintf("e%d", i+1) || texts[e.Text] {
			t.Fatalf("duplicate/reordered entry: %+v", e)
		}
		texts[e.Text] = true
	}
}

func TestFolderProcessWorker(t *testing.T) {
	dir := os.Getenv("WS_WHITEBOARD_FOLDER_TEST")
	if dir == "" {
		return
	}
	for range 3 {
		if err := (FolderStore{Dir: dir}).Update(nil, func(s *State) error {
			s.EntrySeq++
			s.Topics[0].Entries = append(s.Topics[0].Entries, Entry{ID: fmt.Sprintf("e%d", s.EntrySeq), Kind: EntryAgent, Text: fmt.Sprint(os.Getpid()), At: "equal time"})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFolderConcurrentProcesses(t *testing.T) {
	dir := t.TempDir()
	folderOpen(t, dir, &State{Topics: []TopicDoc{{Name: "topic", Body: "body"}}})
	var commands []*exec.Cmd
	var outputs []*bytes.Buffer
	for range 4 {
		cmd := exec.Command(os.Args[0], "-test.run=^TestFolderProcessWorker$")
		cmd.Env = append(os.Environ(), "WS_WHITEBOARD_FOLDER_TEST="+dir)
		out := &bytes.Buffer{}
		cmd.Stdout, cmd.Stderr = out, out
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, cmd)
		outputs = append(outputs, out)
	}
	for i, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("worker: %v\n%s", err, outputs[i])
		}
	}
	got := folderOpen(t, dir, nil)
	if len(got.Topics[0].Entries) != 12 || got.EntrySeq != 12 {
		t.Fatalf("lost cross-process writes: %s", folderJSON(got))
	}
}

func TestFolderPublishLockedAfterCommit(t *testing.T) {
	dir := t.TempDir()
	folderOpen(t, dir, &State{})
	f := FolderStore{Dir: dir}
	publishing, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- f.UpdateAndPublish(nil, func(s *State) error { s.Seq = 1; return nil }, func() error {
			var committed folderSession
			if _, err := folderDecode(filepath.Join(dir, "meta.json"), &committed); err != nil {
				return err
			}
			if committed.State.Seq != 1 {
				return fmt.Errorf("publish ran before commit")
			}
			close(publishing)
			<-release
			return os.WriteFile(filepath.Join(dir, "index.html"), []byte("first"), 0o644)
		})
	}()
	<-publishing
	entered := make(chan struct{})
	second := make(chan error, 1)
	go func() {
		second <- f.UpdateAndPublish(nil, func(s *State) error { close(entered); s.Seq = 2; return nil }, func() error { return os.WriteFile(filepath.Join(dir, "index.html"), []byte("second"), 0o644) })
	}()
	select {
	case <-entered:
		t.Error("another writer entered during publication")
	case <-time.After(40 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if string(folderRead(t, filepath.Join(dir, "index.html"))) != "second" {
		t.Fatal("stale derived output won")
	}
	if err := f.UpdateAndPublish(nil, func(s *State) error { s.Seq = 3; return nil }, func() error { return folderCrash }); !errors.Is(err, folderCrash) {
		t.Fatal(err)
	}
	if got := folderOpen(t, dir, nil); got.Seq != 3 {
		t.Fatal("publication failure rolled back authority")
	}
	called := false
	if err := f.UpdateAndPublish(nil, func(*State) error { return folderCrash }, func() error { called = true; return nil }); !errors.Is(err, folderCrash) || called {
		t.Fatal("refused mutation published")
	}
}
