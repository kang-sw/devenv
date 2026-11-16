package wswhiteboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCommandAcceptanceFolderFaultsAcceptedOnly(t *testing.T) {
	for _, stage := range []string{"intent-staged", "intent", "panel", "log-partial", "topic-meta", "session-meta", "complete", "publish"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			f := FolderStore{Dir: dir}
			if err := f.Update(nil, func(s *State) error { mustApply(t, s, topicXML("t", "base")); return nil }); err != nil {
				t.Fatal(err)
			}
			before := folderOpen(t, dir, nil)
			calls, commits, publishes := 0, 0, 0
			crash := errors.New("injected " + stage)
			f.Fault = func(at string) error {
				if at == stage {
					return crash
				}
				return nil
			}
			input := panelXML("t", "accepted panel", "retained payload") + `<wb:answer id="999" value="ok">rejected</wb:answer>` + logXML("t", "accepted log") + addXML(ClassCheck, "t", "accepted ask")
			err := f.UpdateAndPublishObserved(nil, func(s *State) error {
				calls++
				out, p := Apply(s, input, testEnv())
				if len(p) != 1 || out.Accepted != 3 {
					t.Fatalf("partial callback: %+v %v", out, p)
				}
				return nil
			}, func() { commits++ }, func() error {
				publishes++
				if stage == "publish" {
					return crash
				}
				return nil
			})
			if !errors.Is(err, crash) || calls != 1 {
				t.Fatal("fault/callback", err, calls)
			}
			got := folderOpen(t, dir, nil)
			if stage == "intent-staged" {
				if commits != 0 || !reflect.DeepEqual(before, got) {
					t.Fatal("uncommitted intent changed state")
				}
			} else {
				if commits != 1 || got.PanelCounter != 2 || got.Counter != 1 || len(got.Items) != 1 || got.Seq != before.Seq+1 {
					t.Fatal("commit count/counters", commits, got)
				}
				var panels, logs int
				for _, e := range got.Topics[0].Entries {
					if e.Kind == EntryPanel {
						panels++
					}
					if e.Kind == EntryAgent {
						logs++
					}
					if strings.Contains(e.Text, "rejected") {
						t.Fatal("rejected event persisted")
					}
				}
				if panels != 2 || logs != 1 {
					t.Fatal("recovery duplicated accepted entries", panels, logs)
				}
			}
			if publishes > 1 {
				t.Fatal("published multiple times")
			}
			tree := folderTree(t, dir)
			folderOpen(t, dir, nil)
			if !reflect.DeepEqual(tree, folderTree(t, dir)) || calls != 1 {
				t.Fatal("reopen replayed commands")
			}
		})
	}
}

func TestCommandAcceptanceDirectionalOldSourceReopen(t *testing.T) {
	binary := os.Getenv("WS_WHITEBOARD_COMPAT_OLD")
	if binary == "" {
		t.Skip("build testdata/compat-old in pinned pre-feature source and set WS_WHITEBOARD_COMPAT_OLD")
	}
	type result struct {
		State         *State
		Problems      []string
		Display, Page string
	}
	old := func(t *testing.T, dir, commands string) result {
		t.Helper()
		data, err := exec.Command(binary, dir, commands).CombinedOutput()
		if err != nil {
			t.Fatalf("old source fixture: %v: %s", err, data)
		}
		var r result
		if err := json.Unmarshal(data, &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	t.Run("old-statuses-and-history", func(t *testing.T) {
		dir := t.TempDir()
		var commands strings.Builder
		for n := 0; n < 5; n++ {
			commands.WriteString(addXML(ClassTechnical, "t", fmt.Sprintf("Old %d", n)))
		}
		commands.WriteString(`<wb:answer id="1" value="ok">old answer</wb:answer><wb:answer id="2" value="guess" assumed="true">old assumption</wb:answer><wb:patch id="3" status="withdrawn"/><wb:patch id="4" status="superseded"/>`)
		written := old(t, dir, commands.String())
		if len(written.Problems) > 0 {
			t.Fatal(written.Problems)
		}
		reopened := folderOpen(t, dir, nil)
		if !reflect.DeepEqual(written.State, reopened) || len(reopened.Topics[0].Entries) == 0 {
			t.Fatal("new reader changed old statuses/history")
		}
		for i, status := range []string{StatusAnswered, StatusAssumed, StatusWithdrawn, StatusSuperseded, StatusOpen} {
			if reopened.Items[i].Status != status {
				t.Fatal(reopened.Items)
			}
		}
	})
	for _, transition := range []string{"unrelated", "archive-pressure", "reask", "letter", "withdraw", "supersede"} {
		t.Run(transition, func(t *testing.T) {
			dir := t.TempDir()
			store := FolderStore{Dir: dir}
			if err := store.Update(nil, func(s *State) error {
				mustApply(t, s, addXML(ClassPolicy, "t", "Pick", "One", "Two")+`<wb:answer id="1" value="">verbatim &amp; <b>note</b></wb:answer>`)
				s.LastEmbedSeq = s.Seq
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			before := folderOpen(t, dir, nil)
			comment := item(t, before, 1)
			read := old(t, dir, "read")
			if !reflect.DeepEqual(item(t, read.State, 1), comment) || read.State.OpenAskCount() != 0 {
				t.Fatal("old reader changed comment")
			}
			if strings.Contains(read.Display, "user commented") || strings.Contains(read.Page, "lead&#39;s move") || !strings.Contains(read.Page, `class="wb-hstatus" title="answered"`) {
				t.Fatal("old fixture unexpectedly understands new status")
			}
			commands := logXML("other", "unrelated old write")
			switch transition {
			case "archive-pressure":
				var b strings.Builder
				for n := 0; n < ArchiveCap+5; n++ {
					b.WriteString(fmt.Sprintf(`<wb:topic name="archive"><wb:add class="technical"><wb:title>old %d</wb:title></wb:add></wb:topic><wb:answer id="%d" value="ok">old answer</wb:answer>`, n, n+2))
				}
				commands = b.String()
			case "reask":
				commands = `<wb:patch id="1" status="open"/>`
			case "letter":
				commands = `<wb:answer id="1" value="b">Two</wb:answer>`
			case "withdraw":
				commands = `<wb:patch id="1" status="withdrawn"/>`
			case "supersede":
				commands = `<wb:patch id="1" status="superseded"/>`
			}
			written := old(t, dir, commands)
			if len(written.Problems) > 0 {
				t.Fatal(written.Problems)
			}
			reopened := folderOpen(t, dir, nil)
			if !reflect.DeepEqual(reopened, written.State) || reopened.Seq != before.Seq+1 || reopened.LastEmbedSeq != before.LastEmbedSeq {
				t.Fatal("new reopen reconstructed or lost old data")
			}
			it := item(t, reopened, 1)
			switch transition {
			case "unrelated", "archive-pressure":
				if !reflect.DeepEqual(it, comment) {
					t.Fatal("old write lost comment fields", it, comment)
				}
			case "reask":
				if it.Status != StatusOpen || it.AnswerQuote != "" || it.AnsweredAt != "" {
					t.Fatal("old reask was resurrected", it)
				}
			case "letter":
				if it.Status != StatusAnswered || it.Answer != "b" || it.AnswerQuote != "Two" {
					t.Fatal("old choice was resurrected", it)
				}
			default:
				if it.Status != map[string]string{"withdraw": StatusWithdrawn, "supersede": StatusSuperseded}[transition] || it.AnswerQuote != comment.AnswerQuote {
					t.Fatal("old withdrawal not authoritative", it)
				}
			}
			var meta folderSession
			if err := json.Unmarshal(folderRead(t, filepath.Join(dir, "meta.json")), &meta); err != nil || meta.SchemaVersion != 2 {
				t.Fatal("schema changed", err)
			}
			// Unsupported empty-value requests to the old server refuse without
			// changing even publication/metadata bytes, not a version boundary.
			tree := folderTree(t, dir)
			refusal := old(t, dir, `<wb:answer id="1" value="">new caller comment</wb:answer>`)
			if len(refusal.Problems) == 0 || !strings.Contains(strings.Join(refusal.Problems, " "), "must be non-empty") || !reflect.DeepEqual(tree, folderTree(t, dir)) {
				t.Fatal("old unsupported request wrote data", refusal.Problems)
			}
		})
	}
}
