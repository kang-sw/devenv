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
	"time"
)

// This is the pre-proof TopicDoc shape used by the schema-2 typed writers.
// Keep it independent of TopicDoc: aliasing the new type would accidentally
// preserve the very optional field this compatibility fixture must discard.
type preProofTopicDoc struct {
	Name         string      `json:"name"`
	Body         string      `json:"body"`
	Entries      []Entry     `json:"entries,omitempty"`
	Changed      int         `json:"changed"`
	ChangedAt    string      `json:"changed_at,omitempty"`
	Marks        []BlockMark `json:"marks,omitempty"`
	Recap        string      `json:"recap,omitempty"`
	RecapChanged int         `json:"recap_changed,omitempty"`
	RecapAt      string      `json:"recap_at,omitempty"`
	Tickets      []string    `json:"tickets,omitempty"`
}

func preProofWriterRoundTrip(t *testing.T, st *State, edit bool) {
	t.Helper()
	var docs []preProofTopicDoc
	data, err := json.Marshal(st.Topics)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &docs); err != nil {
		t.Fatal(err)
	}
	if edit {
		st.Seq++
		docs[0].Body += "\n\nold writer mutation"
		docs[0].Changed = st.Seq
		docs[0].ChangedAt = testNow.Format(time.RFC3339) // Deliberately the same second.
		docs[0].Marks = marks(2, MarkNew)
	}
	data, err = json.Marshal(docs)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("body_time_proof")) {
		t.Fatal("old shape retained unknown proof")
	}
	var back []TopicDoc
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	st.Topics = back
}

func TestBodyTimeOldNewWriterCompatibility(t *testing.T) {
	for _, edit := range []bool{false, true} {
		t.Run(fmt.Sprintf("old-writer-body-edit-%v", edit), func(t *testing.T) {
			dir := t.TempDir()
			f := FolderStore{Dir: dir}
			if err := f.Update(nil, func(st *State) error {
				mustApply(t, st, topicXML("t", "one\n\ntwo"))
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			before := folderOpen(t, dir, nil).Topics[0]
			assertBodyTime(t, before, before.ChangedAt)
			if err := f.Update(nil, func(st *State) error {
				preProofWriterRoundTrip(t, st, edit)
				// An old writer also serializes topics on unrelated accepted updates.
				if !edit {
					st.Seq++
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			st := folderOpen(t, dir, nil)
			if st.Topics[0].BodyTimeProof != nil {
				t.Fatal("proof loss fixture did not lose proof")
			}
			assertBodyTime(t, st.Topics[0], "")
			applyBodyTimeAt(t, st, "", testNow.Add(time.Hour))
			assertBodyTime(t, st.Topics[0], "")
			if edit {
				// Captured evidence cannot authenticate another batch, even with
				// the same second. Old writers are allowed loss, not false age.
				st.Topics[0].BodyTimeProof = before.BodyTimeProof
				assertBodyTime(t, st.Topics[0], "")
			}
			applyBodyTimeAt(t, st, topicXML("t", "a real new writer edit"), testNow.Add(2*time.Hour))
			assertBodyTime(t, st.Topics[0], testNow.Add(2*time.Hour).Format(time.RFC3339))
		})
	}
}

func TestBodyTimeFolderCommitAndReplay(t *testing.T) {
	for _, stage := range []string{"intent-staged", "intent", "log-partial", "topic-meta-staged", "topic-meta", "session-meta", "cleanup"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			f := FolderStore{Dir: dir}
			if err := f.Update(nil, func(st *State) error {
				mustApply(t, st, topicXML("t", "old body"))
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			before := folderOpen(t, dir, nil)
			want := before.clone()
			applyBodyTimeAt(t, want, topicXML("t", "new body"), testNow.Add(time.Hour))
			callback, publication := 0, 0
			f.Fault = func(hit string) error {
				if hit == stage {
					return folderCrash
				}
				return nil
			}
			err := f.UpdateAndPublish(nil, func(st *State) error {
				callback++
				applyBodyTimeAt(t, st, topicXML("t", "new body"), testNow.Add(time.Hour))
				return nil
			}, func() error { publication++; return nil })
			if !errors.Is(err, folderCrash) || callback != 1 || publication != 0 {
				t.Fatalf("commit boundary: err=%v callback=%d publication=%d", err, callback, publication)
			}
			var exactTopic []byte
			if stage != "intent-staged" {
				var pending folderIntent
				if _, err := folderDecode(filepath.Join(dir, "pending.json"), &pending); err != nil {
					t.Fatal(err)
				}
				if len(pending.Topics) != 1 {
					t.Fatal("missing committed topic bytes")
				}
				exactTopic = pending.Topics[0].Data
				var topic folderTopic
				if err := json.Unmarshal(exactTopic, &topic); err != nil {
					t.Fatal(err)
				}
				if topic.Doc.Body != "new body" {
					t.Fatal("intent omitted body")
				}
				assertBodyTime(t, *topic.Doc, want.Topics[0].ChangedAt)
			}
			got := folderOpen(t, dir, nil)
			if stage == "intent-staged" {
				want = before
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("body/proof replay mismatch\nwant %s\ngot %s", folderJSON(want), folderJSON(got))
			}
			assertBodyTime(t, got.Topics[0], want.Topics[0].ChangedAt)
			if exactTopic != nil && !bytes.Equal(exactTopic, folderRead(t, filepath.Join(dir, "t1", "meta.json"))) {
				t.Fatal("recovery did not retain exact committed body/proof bytes")
			}
			tree := folderTree(t, dir)
			folderOpen(t, dir, nil)
			if !reflect.DeepEqual(tree, folderTree(t, dir)) {
				t.Fatal("repeated replay rewrote committed proof")
			}
		})
	}
	// A derived-output failure after durable commit keeps authoritative age.
	dir := t.TempDir()
	f := FolderStore{Dir: dir}
	if err := f.UpdateAndPublish(nil, func(st *State) error {
		mustApply(t, st, topicXML("t", "committed body"))
		return nil
	}, func() error { return folderCrash }); !errors.Is(err, folderCrash) {
		t.Fatal(err)
	}
	got := folderOpen(t, dir, nil)
	assertBodyTime(t, got.Topics[0], testNow.Format(time.RFC3339))
}

func TestBodyTimePortableFolderCopyAndAdoption(t *testing.T) {
	root := t.TempDir()
	source := FolderStore{Dir: filepath.Join(root, "source")}
	if err := source.Update(nil, func(st *State) error {
		mustApply(t, st, topicXML("Legacy", "one\n\ntwo"))
		mustApply(t, st, topicXML("Legacy", "one\n\ntwo changed"))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	origin := folderOpen(t, source.Dir, nil).Topics[0]
	destination := FolderStore{Dir: filepath.Join(root, "different-root", "different-board")}
	for rel, data := range folderTree(t, source.Dir) {
		path := filepath.Join(destination.Dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	copied := folderOpen(t, destination.Dir, nil)
	assertBodyTime(t, copied.Topics[0], origin.ChangedAt)
	if err := destination.Update(nil, func(st *State) error {
		applyBodyTimeAt(t, st, `<wb:topic name="adopted" label="Legacy"><wb:body>away</wb:body><wb:body>one

two changed</wb:body></wb:topic>`, testNow.Add(time.Hour))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got := folderOpen(t, destination.Dir, nil).Topics[0]
	assertBodyTime(t, got, origin.ChangedAt)
	if got.Name != "adopted" || got.Changed != origin.Changed || !reflect.DeepEqual(got.BodyTimeProof, origin.BodyTimeProof) || !reflect.DeepEqual(got.Marks, origin.Marks) {
		t.Fatal("copy/adoption minted new age or lost original body batch")
	}
	if err := destination.Update(nil, func(st *State) error {
		applyBodyTimeAt(t, st, `<wb:topic name="adopted" label="Renamed"/>`, testNow.Add(2*time.Hour))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertBodyTime(t, folderOpen(t, destination.Dir, nil).Topics[0], origin.ChangedAt)
	// Moving the whole authority, including its folder mapping, is not an edit.
	moved := filepath.Join(root, "moved-again")
	if err := os.Rename(destination.Dir, moved); err != nil {
		t.Fatal(err)
	}
	assertBodyTime(t, folderOpen(t, moved, nil).Topics[0], origin.ChangedAt)
}
