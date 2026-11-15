package wswhiteboard

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFolderCommitObservationUsesDurableIntentNotCallbackOrFinalError(t *testing.T) {
	for _, stage := range []string{"callback", "intent-staged", "intent", "panel", "log-partial", "topic-meta", "session-meta", "complete", "publish", "success"} {
		t.Run(stage, func(t *testing.T) {
			f := FolderStore{Dir: t.TempDir()}
			if err := f.Update(nil, func(s *State) error {
				mustApply(t, s, `<wb:topic name="one"><wb:body>Before</wb:body></wb:topic>`)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			before := folderTree(t, f.Dir)
			f.Fault = func(at string) error {
				if at == stage {
					return folderCrash
				}
				return nil
			}
			commits, publishes := 0, 0
			callbackSucceeded := false
			err := f.UpdateAndPublishObserved(nil, func(s *State) error {
				mustApply(t, s, `<wb:topic name="one"><wb:panel recap="New">New panel</wb:panel><wb:log>New log</wb:log><wb:body>After</wb:body></wb:topic>`)
				if stage == "callback" {
					return folderCrash
				}
				callbackSucceeded = true
				return nil
			}, func() {
				commits++
				// The observation is before the first materialization step.
				if _, err := os.Stat(filepath.Join(f.Dir, "pending.json")); err != nil {
					t.Fatalf("observation without installed intent: %v", err)
				}
			}, func() error {
				publishes++
				if commits != 1 {
					t.Fatal("publication before commitment observation")
				}
				if stage == "publish" {
					return folderCrash
				}
				return nil
			})
			wantCommit := stage != "callback" && stage != "intent-staged"
			if (commits == 1) != wantCommit || commits > 1 || callbackSucceeded != (stage != "callback") {
				t.Fatalf("callback=%v commits=%d for %s", callbackSucceeded, commits, stage)
			}
			if (err == nil) != (stage == "success") || err != nil && !errors.Is(err, folderCrash) {
				t.Fatalf("error for %s: %v", stage, err)
			}
			wantPublish := stage == "publish" || stage == "success"
			if (publishes == 1) != wantPublish {
				t.Fatalf("publishes=%d for %s", publishes, stage)
			}
			if !wantCommit {
				if _, err := os.Stat(filepath.Join(f.Dir, "pending.json")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("failed installation left a committed intent: %v", err)
				}
				if !reflect.DeepEqual(before, folderTree(t, f.Dir)) {
					t.Fatal("precommit refusal changed authority")
				}
			} else {
				// A new reader materializes the installed decision exactly once.
				// This does not call the observer or guarantee a ring projection.
				s := folderOpen(t, f.Dir, nil)
				if s.TopicBody("one") != "After" || s.PanelCounter != 1 {
					t.Fatalf("installed intent not recovered: %+v", s)
				}
				if commits != 1 || publishes > 1 {
					t.Fatal("recovery replayed observation/publication")
				}
			}
		})
	}
}

func TestFolderCommitObservationPreservesStoreBytesAndShape(t *testing.T) {
	plain, observed := FolderStore{Dir: t.TempDir()}, FolderStore{Dir: t.TempDir()}
	apply := func(s *State) error {
		mustApply(t, s, `<wb:topic name="shape" label="Shape"><wb:add class="check"><wb:title>Same bytes</wb:title></wb:add><wb:panel recap="Detail">Panel</wb:panel><wb:log>Log</wb:log></wb:topic>`)
		return nil
	}
	if err := plain.UpdateAndPublish(nil, apply, nil); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err := observed.UpdateAndPublishObserved(nil, apply, func() { calls++ }, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !reflect.DeepEqual(folderTree(t, plain.Dir), folderTree(t, observed.Dir)) {
		t.Fatal("observation changed persisted bytes, keys or shape")
	}
}
