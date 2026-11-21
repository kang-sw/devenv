package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// These are old-writer bytes, deliberately not a marshaled current State.
// Whitespace and an obsolete unknown field catch accidental round-tripping.
const legacyWhiteboardJSON = `{
  "counter": 42, "seq": 9, "settle_seq": 1,
  "now": "obsolete snapshot field",
  "items": [{"id":42,"class":"policy","topic":"stemless / CON: ..","title":"Original question","status":"answered","answer":"b","answer_quote":"(42) b, verbatim owner quote","answered_at":"2026-11-01T10:00:00Z","changed":8,"settled":1}],
  "topics": [
    {"name":"stemless / CON: ..","body":"original content\n","changed":7,"recap":"imported decision","recap_changed":8,"recap_at":"2026-11-01T09:00:00Z"},
    {"name":"recap only","body":"","changed":0,"recap":"only a summary","recap_at":"2026-11-01T09:00:00Z"},
    {"name":"closed","body":"closed content","changed":3,"recap":"finished","recap_at":"2026-10-31T09:00:00Z"}
  ],
  "closed_topics": ["closed"], "close_results": {"closed":"done"}
}
`

func whiteboardTreeForTest(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if rel == ".lock" && runtime.GOOS == "windows" {
				// The flock is a LockFileEx byte-range lock on Windows, so a
				// read of the lock file fails while a test holds it; its
				// presence is still recorded. The product never reads it.
				out[rel] = ""
				return nil
			}
			out[rel] = string(readFileForTest(t, path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", dir, err)
	}
	return out
}

func assertWhiteboardUncommittedForTest(t *testing.T, dir string) {
	t.Helper()
	if !exists(dir) {
		return
	}
	for path := range whiteboardTreeForTest(t, dir) {
		if path != ".lock" {
			t.Fatalf("refused first render left board data or output: %s", path)
		}
	}
}

func assertWhiteboardTreeForTest(t *testing.T, dir string, want map[string]string) {
	t.Helper()
	got := whiteboardTreeForTest(t, dir)
	if len(got) != len(want) {
		t.Fatalf("whiteboard file count changed: %d, want %d", len(got), len(want))
	}
	for path, data := range want {
		if next, ok := got[path]; !ok || next != data {
			t.Fatalf("whiteboard file changed: %s", path)
		}
	}
}

// installLegacyWhiteboardForTest installs an actual old board representation,
// returning the board-bearing file and, for inline records, its exact bytes.
func installLegacyWhiteboardForTest(t *testing.T, f *whiteboardFixture, layout, board string) (string, []byte) {
	t.Helper()
	d, _ := f.s.sessions.dirs()
	if layout == "split" {
		path := d.partPath(f.key, partWhiteboard)
		if err := os.WriteFile(path, []byte(board), 0o600); err != nil {
			t.Fatal(err)
		}
		return path, []byte(board)
	}
	if err := os.RemoveAll(d.keyDir(f.key)); err != nil {
		t.Fatal(err)
	}
	root, _ := json.Marshal(f.root)
	inline := fmt.Sprintf("{\n  \"schema_version\":1, \"root\":%s, \"scope\":\"lead\",\n  \"whiteboard\": %s,\n  \"note\":\"old session\"\n}\n", root, strings.TrimSpace(board))
	path := writeRawLegacyRecordForTest(t, f.s.sessions, f.key, inline)
	return path, []byte(inline)
}

func TestWhiteboardLegacyConversionThroughReadersAndRestart(t *testing.T) {
	for _, layout := range []string{"inline", "split"} {
		for _, reader := range []string{"query", "dashboard", "session state"} {
			t.Run(layout+"/"+reader, func(t *testing.T) {
				f := newWhiteboardFixture(t)
				legacyPath, original := installLegacyWhiteboardForTest(t, f, layout, legacyWhiteboardJSON)
				reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
				switch reader {
				case "query":
					if resp := f.call(t, "whiteboard.query", map[string]any{"payload": map[string]any{"id": 42, "depth": "full"}}); isToolError(resp) || !strings.Contains(toolText(t, resp), "verbatim owner quote") {
						t.Fatalf("legacy query: %s", resp)
					}
				case "dashboard":
					src := dashboardPageSource(t, nil, f.s.sessions.boards)
					if page := readPageOK(t, src, reg); !page.HasBoard || page.Seq != 9 {
						t.Fatalf("legacy dashboard: %+v", page)
					}
				case "session state":
					if resp := f.call(t, "workflow_state", map[string]any{}); isToolError(resp) {
						t.Fatalf("legacy session state: %s", resp)
					}
				}
				if !exists(filepath.Join(f.pageDir(), ".board.sqlite")) {
					t.Fatal("first read did not convert to authoritative DB")
				}
				if got := readFileForTest(t, legacyPath); !bytes.Equal(got, original) {
					t.Fatal("conversion changed the old board-bearing file")
				}
				rec, err := f.s.sessions.readWhiteboard(f.key)
				if err != nil {
					t.Fatal(err)
				}
				if rec.Whiteboard.Counter != 42 || rec.Whiteboard.Seq != 9 || len(rec.Whiteboard.Items) != 1 || len(rec.Whiteboard.Topics) != 3 || rec.Whiteboard.EntrySeq != 6 || rec.Whiteboard.PanelCounter != 2 {
					t.Fatalf("conversion lost snapshot or fabricated history: %+v", rec.Whiteboard)
				}
				item := rec.Whiteboard.Items[0]
				if item.ID != 42 || item.Answer != "b" || item.AnswerQuote != "(42) b, verbatim owner quote" || item.Status != wswhiteboard.StatusAnswered {
					t.Fatalf("conversion changed owner answer: %+v", item)
				}
				for i, doc := range rec.Whiteboard.Topics {
					wantEntries := 1
					if doc.Body != "" {
						wantEntries = 2
					}
					if doc.Name == "closed" {
						wantEntries++
						last := doc.Entries[len(doc.Entries)-1]
						if last.Kind != wswhiteboard.EntryAgent || last.Text != "done" {
							t.Fatalf("close result not migrated: %+v", last)
						}
					}
					if doc.Recap != "" || len(doc.Entries) != wantEntries {
						t.Fatalf("recap was not converted exactly once: %+v", doc)
					}
					if doc.Body != "" {
						panel := doc.Entries[1]
						if panel.Kind != wswhiteboard.EntryPanel || !panel.LegacyContent || !panel.Imported || panel.Text != doc.Body || panel.At != doc.ChangedAt {
							t.Fatalf("content migration lost payload/time: %+v", panel)
						}
					}
					e := doc.Entries[0]
					if e.ID != fmt.Sprintf("e%d", i+1) || e.Kind != wswhiteboard.EntryAgent || !e.Imported || e.At == "" {
						t.Fatalf("import lost entry provenance: %+v", e)
					}
				}
				if doc := rec.Whiteboard.Topics[0]; doc.Name != "stemless / CON: .." || doc.Body != "original content\n" || doc.Entries[0].Text != "imported decision" || doc.Entries[0].At != "2026-11-01T09:00:00Z" {
					t.Fatalf("stemless topic changed: %+v", doc)
				}
				if len(rec.Whiteboard.ClosedTopics) != 1 || rec.Whiteboard.ClosedTopics[0] != "closed" || rec.Whiteboard.CloseResults["closed"] != "" {
					t.Fatalf("conversion lost closed state: %+v", rec.Whiteboard)
				}

				// A daemon restart is a fresh server and fresh read caches. Opening
				// twice must not re-import equal-time recap entries.
				f.s = restartTestBoardOwner(t, f.s, f.root, "restart")
				for i := 0; i < 2; i++ {
					reopened, err := f.s.sessions.readWhiteboard(f.key)
					if err != nil {
						t.Fatal(err)
					}
					before, _ := json.Marshal(rec.Whiteboard)
					after, _ := json.Marshal(reopened.Whiteboard)
					if !bytes.Equal(before, after) {
						t.Fatalf("restart changed imported snapshot: %s, want %s", after, before)
					}
				}

				// An ordinary key write must not invoke destructive legacy migration.
				if err := f.s.sessions.setOverride(f.key, "test.item", "new metadata"); err != nil {
					t.Fatal(err)
				}
				if got := readFileForTest(t, legacyPath); !bytes.Equal(got, original) {
					t.Fatal("metadata update changed the old board-bearing file")
				}
				d, _ := f.s.sessions.dirs()
				rollbackPath := d.partPath(f.key, partWhiteboard)
				var expected []byte
				if layout == "inline" {
					var old map[string]json.RawMessage
					if err := json.Unmarshal(original, &old); err != nil {
						t.Fatal(err)
					}
					expected = old["whiteboard"]
				} else {
					expected = original
				}
				if got := readFileForTest(t, rollbackPath); !bytes.Equal(got, expected) {
					t.Fatal("split rollback board is not the original JSON bytes")
				}
				f.update(t, `<wb:topic name="stemless / CON: .."><wb:log>post-conversion only</wb:log><wb:panel recap="new panel">new payload</wb:panel></wb:topic>`)
				if got := readFileForTest(t, legacyPath); !bytes.Equal(got, original) {
					t.Fatal("new-format update dual-wrote the old representation")
				}
				if got := readFileForTest(t, rollbackPath); !bytes.Equal(got, expected) {
					t.Fatal("new-format update dual-wrote the rollback board")
				}

				// An old reader knows only the pre-entry board shape. It sees the
				// preserved pre-conversion snapshot, not post-conversion discussion.
				var rollback struct {
					Counter int `json:"counter"`
					Seq     int `json:"seq"`
					Topics  []struct {
						Body  string `json:"body"`
						Recap string `json:"recap"`
					} `json:"topics"`
					Items []struct {
						ID          int    `json:"id"`
						AnswerQuote string `json:"answer_quote"`
					} `json:"items"`
				}
				if err := json.Unmarshal(readFileForTest(t, rollbackPath), &rollback); err != nil {
					t.Fatal(err)
				}
				if rollback.Seq != 9 || rollback.Counter != 42 || len(rollback.Topics) != 3 || rollback.Topics[0].Recap != "imported decision" || rollback.Topics[0].Body != "original content\n" || rollback.Items[0].ID != 42 || rollback.Items[0].AnswerQuote != item.AnswerQuote {
					t.Fatalf("old-reader rollback snapshot changed: %+v", rollback)
				}
			})
		}
	}
}

func TestWhiteboardLegacyEmptyBoardConvertsOnce(t *testing.T) {
	for _, layout := range []string{"inline", "split"} {
		t.Run(layout, func(t *testing.T) {
			f := newWhiteboardFixture(t)
			path, before := installLegacyWhiteboardForTest(t, f, layout, "{ \"counter\": 0, \"seq\": 2 }\n")
			for i := 0; i < 2; i++ {
				rec, err := f.s.sessions.readWhiteboard(f.key)
				if err != nil || rec.Whiteboard == nil || rec.Whiteboard.Seq != 2 || rec.Whiteboard.EntrySeq != 0 || len(rec.Whiteboard.Topics) != 0 {
					t.Fatalf("empty board open: %+v, %v", rec.Whiteboard, err)
				}
			}
			if !exists(filepath.Join(f.pageDir(), ".board.sqlite")) || !bytes.Equal(readFileForTest(t, path), before) {
				t.Fatal("empty board was not durably converted with legacy bytes preserved")
			}
		})
	}
}

func TestWhiteboardNewerSchemaRefusesReadersAndWriters(t *testing.T) {
	f := newWhiteboardFixture(t)
	legacyPath, original := installLegacyWhiteboardForTest(t, f, "split", legacyWhiteboardJSON)
	if err := os.MkdirAll(f.pageDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.pageDir(), "meta.json")
	data := []byte(fmt.Sprintf(`{"schema_version":%d}`, wswhiteboard.FolderSchemaVersion+1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"whiteboard.query", "whiteboard.update"} {
		resp := f.call(t, tool, map[string]any{})
		if !isToolError(resp) || !strings.Contains(toolText(t, resp), "newer schema_version") {
			t.Fatalf("%s must refuse newer legacy folder schema: %s", tool, resp)
		}
	}
	called := false
	err := f.s.sessions.mutateRecord(f.key, partWhiteboard, func(r *sessionRecord) error { called = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "newer schema_version") || called {
		t.Fatalf("writer did not guard newer schema: %v, called %v", err, called)
	}
	if !bytes.Equal(readFileForTest(t, path), data) || !bytes.Equal(readFileForTest(t, legacyPath), original) {
		t.Fatal("refusal changed legacy data")
	}
}

func TestSessionRecordNewerSchemaPreservesLegacyBoard(t *testing.T) {
	for _, layout := range []string{"inline", "split"} {
		t.Run(layout, func(t *testing.T) {
			f := newWhiteboardFixture(t)
			boardPath, boardBefore := installLegacyWhiteboardForTest(t, f, layout, legacyWhiteboardJSON)
			path := splitKeyPath(t, f.s.sessions, f.key)
			if layout == "inline" {
				path = boardPath
			}
			var record map[string]json.RawMessage
			if err := json.Unmarshal(readFileForTest(t, path), &record); err != nil {
				t.Fatal(err)
			}
			record["schema_version"] = json.RawMessage(fmt.Sprint(sessionRecordSchemaVersion + 1))
			newer, _ := json.Marshal(record)
			if err := os.WriteFile(path, newer, 0o600); err != nil {
				t.Fatal(err)
			}
			_, openErr := f.s.sessions.readWhiteboard(f.key)
			for _, err := range []error{
				f.s.sessions.setOverride(f.key, "test.item", "no write"),
				openErr,
			} {
				if err == nil || !strings.Contains(err.Error(), "newer than supported") {
					t.Fatalf("newer session schema was not explicitly refused: %v", err)
				}
			}
			if !bytes.Equal(readFileForTest(t, path), newer) || exists(filepath.Join(f.pageDir(), "meta.json")) {
				t.Fatal("newer session schema was rewritten or converted")
			}
			if layout == "split" && !bytes.Equal(readFileForTest(t, boardPath), boardBefore) {
				t.Fatal("newer session refusal changed the split rollback board")
			}
		})
	}
}

func TestWhiteboardDBChangeInvalidatesDashboardWithoutSeqChange(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="t"><wb:add class="check"><wb:title>before-marker</wb:title></wb:add><wb:panel recap="a panel">payload</wb:panel><wb:log>an agent line</wb:log></wb:topic>`)
	src := dashboardPageSource(t, nil, f.s.sessions.boards)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	first := readPageOK(t, src, reg)
	readPageOK(t, src, reg)
	beforeRenders := boardRenders(src)
	fingerprint := readFingerprint(t, src, reg)
	readPageOK(t, src, reg)
	if boardRenders(src) != beforeRenders {
		t.Fatal("unchanged board missed memo")
	}
	for _, path := range first.Watch {
		rel, err := filepath.Rel(f.pageDir(), path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("dashboard watches board authority path: %s", path)
		}
	}
	if err := f.s.sessions.mutateRecord(f.key, partWhiteboard, func(r *sessionRecord) error {
		r.Whiteboard.Items[0].Title = "after-marker"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if readFingerprint(t, src, reg) == fingerprint {
		t.Fatal("DB change kept dashboard fingerprint")
	}
	next := readPageOK(t, src, reg)
	if next.Seq != first.Seq || !strings.Contains(string(next.Board.Board), "after-marker") || boardRenders(src) != beforeRenders+1 {
		t.Fatalf("equal-sequence DB change did not invalidate dashboard memo: seq %d, renders %d", next.Seq, boardRenders(src))
	}
}

func TestWhiteboardGenericMutationKeepsEntryLocatorsAfterRestart(t *testing.T) {
	f := newWhiteboardFixture(t)
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	apply := func(commands string) {
		t.Helper()
		if err := f.s.sessions.mutateRecord(f.key, partWhiteboard, func(r *sessionRecord) error {
			_, problems := wswhiteboard.Apply(r.Whiteboard, commands, wswhiteboard.Env{Now: at})
			if len(problems) > 0 {
				return fmt.Errorf("apply: %v", problems)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	apply(`<wb:topic name="t"><wb:log>same text</wb:log><wb:log>same text</wb:log><wb:panel recap="version one">first payload</wb:panel></wb:topic>`)
	first, err := f.s.sessions.readWhiteboard(f.key)
	if err != nil {
		t.Fatal(err)
	}
	entries := append([]wswhiteboard.Entry(nil), first.Whiteboard.Topics[0].Entries...)
	if len(entries) != 5 || entries[0].ID == entries[2].ID || entries[0].At != entries[2].At || entries[3].PanelID != "p1" {
		t.Fatalf("equal-time entries lost durable identities: %+v", entries)
	}
	wantDefault := wswhiteboard.Entry{ID: "e2", Kind: wswhiteboard.EntrySystem, At: at.Format(time.RFC3339Nano), Text: "Topic state: discussing", TopicState: json.RawMessage(`{"v":1,"op":"set","value":"discussing"}`)}
	if !reflect.DeepEqual(entries[1], wantDefault) {
		t.Fatalf("default event metadata changed: %+v", entries[1])
	}
	apply(`<wb:topic name="t"><wb:panel id="p1" recap="version two">second payload</wb:panel></wb:topic>`)
	f.s = restartTestBoardOwner(t, f.s, f.root, "restart")
	restarted := f.s
	rec, err := restarted.sessions.readWhiteboard(f.key)
	if err != nil {
		t.Fatal(err)
	}
	got := rec.Whiteboard.Topics[0].Entries
	if len(got) != 7 || got[5].PanelID != "p1" || got[5].ID == entries[3].ID || got[5].Text != "second payload" {
		t.Fatalf("panel re-post did not retain distinct durable versions: %+v", got)
	}
	for i, entry := range entries {
		if !reflect.DeepEqual(got[i], entry) {
			t.Fatalf("restart redirected locator %s: %+v, want %+v", entry.ID, got[i], entry)
		}
	}
	d, _ := restarted.sessions.dirs()
	if exists(d.partPath(f.key, partWhiteboard)) {
		t.Fatal("generic board mutations created an obsolete cache authority")
	}
}

func TestWhiteboardCommittedStoreNeverFallsBackToLegacy(t *testing.T) {
	f := newWhiteboardFixture(t)
	legacyPath, original := installLegacyWhiteboardForTest(t, f, "split", legacyWhiteboardJSON)
	f.update(t, `<wb:topic name="new"><wb:log>committed DB state</wb:log></wb:topic>`)
	if err := f.s.sessions.boards.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.pageDir(), ".board.sqlite")); err != nil {
		t.Fatal(err)
	}
	f.s = newTestBoardOwner(t, f.root, "restart")
	if resp := f.call(t, "whiteboard.query", map[string]any{}); !isToolError(resp) || !strings.Contains(toolText(t, resp), "rollback") {
		t.Fatalf("missing DB must not resurrect old board: %s", resp)
	}
	if !bytes.Equal(readFileForTest(t, legacyPath), original) {
		t.Fatal("failed recovery changed rollback snapshot")
	}
}
