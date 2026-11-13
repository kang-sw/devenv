package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestWorkflowManualAndStateReportWhiteboardReadErrors(t *testing.T) {
	for _, tool := range []string{"workflow_manual", "workflow_state"} {
		for _, failure := range []string{"newer_schema", "missing_panel", "lock_timeout"} {
			t.Run(tool+"/"+failure, func(t *testing.T) {
				f := newWhiteboardFixture(t)
				if err := f.s.sessions.enterMode(f.key, "recovery", json.RawMessage(`{"marker":"agenda-survives"}`), []todoItem{{Key: "recover", Title: "todo-survives", Status: todoWip}}); err != nil {
					t.Fatal(err)
				}
				f.update(t, `<wb:topic name="recovery"><wb:panel recap="stored panel">panel payload</wb:panel><wb:log>board-survives</wb:log></wb:topic>`)

				var want string
				switch failure {
				case "newer_schema":
					path := filepath.Join(f.pageDir(), "meta.json")
					var meta map[string]json.RawMessage
					if err := json.Unmarshal(readFileForTest(t, path), &meta); err != nil {
						t.Fatal(err)
					}
					meta["schema_version"] = json.RawMessage(fmt.Sprint(wswhiteboard.FolderSchemaVersion + 1))
					data, err := json.Marshal(meta)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, data, 0o600); err != nil {
						t.Fatal(err)
					}
					want = "newer schema_version"
				case "missing_panel":
					paths, err := filepath.Glob(filepath.Join(f.pageDir(), "*", "panels", "*.json"))
					if err != nil || len(paths) != 1 {
						t.Fatalf("find stored panel: %v, %v", paths, err)
					}
					if err := os.Remove(paths[0]); err != nil {
						t.Fatal(err)
					}
					want = "whiteboard panel " + strings.TrimSuffix(filepath.Base(paths[0]), ".json") + " missing"
				case "lock_timeout":
					lock := flock.New(filepath.Join(f.pageDir(), ".lock"))
					if err := lock.Lock(); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						if err := lock.Unlock(); err != nil {
							t.Error(err)
						}
					})
					want = "whiteboard folder"
				}
				tree := whiteboardTreeForTest(t, f.pageDir())
				resp := f.call(t, tool, map[string]any{})
				text := toolText(t, resp)
				if !isToolError(resp) || !strings.Contains(text, tool+": open whiteboard: ") || !strings.Contains(text, want) {
					t.Fatalf("board read failure must report its real error, not an absent session: %s", resp)
				}
				if failure == "lock_timeout" && !strings.Contains(text, "deadline exceeded") && !strings.Contains(text, "timed out") {
					t.Fatalf("lock failure lost its timeout cause: %s", text)
				}
				for _, absent := range []string{"no stored session record", "no restorable state", "lead-revive", "## Session Key", "(no todos)"} {
					if strings.Contains(text, absent) {
						t.Fatalf("read failure misreported %q: %s", absent, text)
					}
				}
				assertWhiteboardTreeForTest(t, f.pageDir(), tree)
				// The explicit error does not discard agenda/todos; their own
				// readers remain usable while the board cannot be opened.
				rec, ok := f.s.sessions.readState(f.key, partAgenda|partTodos)
				if !ok || string(rec.Agenda["recovery"]) != `{"marker":"agenda-survives"}` || len(rec.Todos) != 1 || rec.Todos[0].Title != "todo-survives" || rec.Todos[0].Status != todoWip {
					t.Fatalf("board read failure changed accessible agenda/todos: %+v, %v", rec, ok)
				}
			})
		}
	}
}
