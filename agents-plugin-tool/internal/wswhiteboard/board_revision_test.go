package wswhiteboard

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestBoardRevisionIgnoresSystemAndSurvivesReload(t *testing.T) {
	s := &State{}
	applyAt(t, s, bodies("A", "B"), testNow)
	revision := s.boardRevision()
	s.appendEntry("A", Entry{Kind: EntrySystem, Text: "reconciliation"}, testNow.Add(time.Minute))
	if got := s.boardRevision(); got != revision {
		t.Fatal("system event changed board revision")
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var reopened State
	if err := json.Unmarshal(data, &reopened); err != nil {
		t.Fatal(err)
	}
	if reopened.boardRevision() != revision {
		t.Fatal("revision changed across reload")
	}
	applyAt(t, &reopened, `<wb:topic name="A"><wb:log>progress</wb:log></wb:topic>`, testNow.Add(2*time.Minute))
	if reopened.boardRevision() == revision {
		t.Fatal("lead activity did not change revision")
	}
}

func TestMetadataRevisionDoesNotChangeTopicHistory(t *testing.T) {
	s := &State{}
	applyAt(t, s, bodies("A", "B"), testNow)
	before := s.clone()
	applyAt(t, s, `<wb:topic name="A" tickets="261112-feat-a"/>`, testNow.Add(time.Minute))
	applyAt(t, s, `<wb:topic name="A" tickets=""/>`, testNow.Add(2*time.Minute))
	if s.EntrySeq != before.EntrySeq || !reflect.DeepEqual(s.Topics, before.Topics) {
		t.Fatal("quiet metadata changed topic history or entry sequence")
	}
	if s.boardRevision() == before.boardRevision() {
		t.Fatal("metadata revert lost its revision identity")
	}
}

func TestMetadataRevisionFolderRecoveryAndOldWriter(t *testing.T) {
	f := FolderStore{Dir: filepath.Join(t.TempDir(), "board")}
	if err := f.Update(nil, func(s *State) error {
		applyAt(t, s, bodies("A"), testNow)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var initialMeta folderSession
	if _, err := folderDecode(filepath.Join(f.Dir, "meta.json"), &initialMeta); err != nil {
		t.Fatal(err)
	}
	f.Fault = func(stage string) error {
		if stage == "intent" {
			return errors.New("simulated crash")
		}
		return nil
	}
	if err := f.Update(nil, func(s *State) error {
		applyAt(t, s, `<wb:topic name="A" tickets="261112-feat-a"/>`, testNow.Add(time.Minute))
		return nil
	}); err == nil {
		t.Fatal("expected interrupted commit")
	}
	f.Fault = nil
	// Model an old daemon decoding the pending intent with a State struct that
	// does not know the new fields. Existing Panels writes retain exact Data.
	pendingPath := filepath.Join(f.Dir, "pending.json")
	pendingBytes, err := os.ReadFile(pendingPath)
	if err != nil {
		t.Fatal(err)
	}
	var typedPending folderIntent
	if err := json.Unmarshal(pendingBytes, &typedPending); err != nil {
		t.Fatal(err)
	}
	if typedPending.Session.NextTopic != initialMeta.NextTopic {
		t.Fatal("revision receipt consumed a topic directory number")
	}
	for _, write := range typedPending.Topics {
		if write.Path == folderRevisionDir+"/meta.json" {
			t.Fatal("revision receipt added a topic metadata write")
		}
	}
	foundReceipt := false
	for _, write := range typedPending.Panels {
		var file folderRevisionFile
		if json.Unmarshal(write.Data, &file) == nil && file.Revision.MetadataRevision != 0 {
			foundReceipt = true
			if !folderIntentPath(write.Path, "panel") {
				t.Fatal("receipt is not a legacy-compatible panel write")
			}
			for _, ref := range typedPending.Session.Topics {
				if ref.Dir == folderRevisionDir {
					t.Fatal("receipt shares a live topic directory")
				}
			}
		}
	}
	if !foundReceipt {
		t.Fatal("pending intent lacks replayable receipt bytes")
	}
	var pending map[string]any
	if err := json.Unmarshal(pendingBytes, &pending); err != nil {
		t.Fatal(err)
	}
	session := pending["session"].(map[string]any)
	oldState := session["state"].(map[string]any)
	delete(oldState, "metadata_revision")
	delete(oldState, "stale_nudge_revision")
	pendingBytes, err = json.Marshal(pending)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pendingPath, pendingBytes, 0600); err != nil {
		t.Fatal(err)
	}
	state, err := f.Open(nil)
	if err != nil || state.MetadataRevision == 0 {
		t.Fatalf("recovery lost revision: state=%+v err=%v", state, err)
	}
	want := state.boardRevision()

	// An older typed writer replaces session metadata without this field.
	path := filepath.Join(f.Dir, "meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	delete(meta["state"].(map[string]any), "metadata_revision")
	data, err = json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	state, err = f.Open(nil)
	if err != nil || state.boardRevision() != want {
		t.Fatalf("older writer lost durable revision: state=%+v err=%v", state, err)
	}
}

func TestMetadataRevisionSlotsRemainBoundedAndCrashSafe(t *testing.T) {
	f := FolderStore{Dir: filepath.Join(t.TempDir(), "board")}
	if err := f.Update(nil, func(s *State) error {
		applyAt(t, s, bodies("A"), testNow)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		command := `<wb:topic name="A" tickets="261112-feat-a"/>`
		if i%2 != 0 {
			command = `<wb:topic name="A" tickets=""/>`
		}
		if err := f.Update(nil, func(s *State) error {
			applyAt(t, s, command, testNow.Add(time.Duration(i+1)*time.Minute))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	files, err := os.ReadDir(filepath.Join(f.Dir, folderRevisionDir, "panels"))
	if err != nil || len(files) > 2 {
		t.Fatalf("revision slots not bounded: %v, %v", files, err)
	}
	before, err := f.Open(nil)
	if err != nil {
		t.Fatal(err)
	}
	f.Fault = func(stage string) error {
		if stage == "revision-slot-cleared" {
			return errors.New("interrupted before intent")
		}
		return nil
	}
	if err := f.Update(nil, func(s *State) error {
		applyAt(t, s, `<wb:topic name="A" tickets="261112-feat-a"/>`, testNow.Add(10*time.Minute))
		return nil
	}); err == nil {
		t.Fatal("expected interruption")
	}
	f.Fault = nil
	after, err := f.Open(nil)
	if err != nil || after.boardRevision() != before.boardRevision() || !reflect.DeepEqual(after.Topics, before.Topics) {
		t.Fatalf("pre-intent interruption lost active receipt: %v", err)
	}
}

func TestBoardRevisionMetadataRevertAndNoop(t *testing.T) {
	s := &State{}
	applyAt(t, s, bodies("A"), testNow)
	initial := s.boardRevision()
	applyAt(t, s, `<wb:topic name="A" tickets="261112-feat-a"/>`, testNow.Add(time.Minute))
	changed := s.boardRevision()
	if changed == initial {
		t.Fatal("ticket change did not change revision")
	}
	applyAt(t, s, `<wb:topic name="A" tickets="261112-feat-a"/>`, testNow.Add(2*time.Minute))
	if s.boardRevision() != changed {
		t.Fatal("no-op changed revision")
	}
	applyAt(t, s, `<wb:topic name="A" tickets=""/>`, testNow.Add(3*time.Minute))
	reverted := s.boardRevision()
	if reverted == initial || reverted == changed {
		t.Fatal("metadata revert reused an earlier revision")
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var reopened State
	if err := json.Unmarshal(data, &reopened); err != nil {
		t.Fatal(err)
	}
	if reopened.boardRevision() != reverted {
		t.Fatal("metadata revision did not survive reload")
	}
	applyAt(t, &reopened, `<wb:topic name="A" tickets="261112-feat-a"/><wb:topic name="A" tickets=""/>`, testNow.Add(4*time.Minute))
	if reopened.boardRevision() == reverted {
		t.Fatal("same-batch metadata edit and revert reused revision")
	}
}
