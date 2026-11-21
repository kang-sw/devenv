package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowManualAndStateReportWhiteboardReadErrors(t *testing.T) {
	for _, tool := range []string{"workflow_manual", "workflow_state"} {
		for _, failure := range []string{"missing_db", "corrupt_db", "draining"} {
			t.Run(tool+"/"+failure, func(t *testing.T) {
				f := newWhiteboardFixture(t)
				if err := f.s.sessions.enterMode(f.key, "recovery", json.RawMessage(`{"marker":"agenda-survives"}`), []todoItem{{Key: "recover", Title: "todo-survives", Status: todoWip}}); err != nil {
					t.Fatal(err)
				}
				f.update(t, `<wb:topic name="recovery"><wb:panel recap="stored panel">panel payload</wb:panel><wb:log>board-survives</wb:log></wb:topic>`)

				var want string
				path := filepath.Join(f.pageDir(), ".board.sqlite")
				switch failure {
				case "missing_db", "corrupt_db":
					f.s = restartTestBoardOwner(t, f.s, f.root, "restart")
					if failure == "missing_db" {
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
					} else if err := os.WriteFile(path, []byte("unreadable sqlite"), 0o600); err != nil {
						t.Fatal(err)
					}
					want = "manual rollback"
				case "draining":
					if err := f.s.sessions.boards.Close(); err != nil {
						t.Fatal(err)
					}
					want = errWhiteboardDraining.Error()
				}
				tree := whiteboardTreeForTest(t, f.pageDir())
				resp := f.call(t, tool, map[string]any{})
				text := toolText(t, resp)
				if !isToolError(resp) || !strings.Contains(text, tool+": open whiteboard: ") || !strings.Contains(text, want) {
					t.Fatalf("board read failure must report its real error, not an absent session: %s", resp)
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
