package wsdashboard

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestSteerBoardlessAndPlainEntries(t *testing.T) {
	stale := "stale"
	for _, tc := range []struct {
		name               string
		agent, board, chip bool
		token              *string
		want               int
	}{
		{"agent legacy", true, false, false, nil, http.StatusOK},
		{"agent tagged", true, false, false, &stale, http.StatusOK},
		{"agent tagged chip without board", true, false, true, &stale, http.StatusOK},
		{"missing board plain", false, false, false, &stale, http.StatusOK},
		{"unopened board plain", false, true, false, &stale, http.StatusOK},
		{"lagging index stale chip", false, false, true, &stale, http.StatusConflict},
		{"board stale chip", false, true, true, &stale, http.StatusConflict},
		{"board legacy chip", false, true, true, nil, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			reg := Registration{ReplyID: "conversation", Agent: tc.agent}
			d := &Daemon{
				log: io.Discard,
				index: &Index{opts: IndexOptions{Now: time.Now}, sessions: map[string]*indexEntry{
					reg.ReplyID: {session: Session{Registration: reg, HasBoard: tc.board}},
				}},
				// Agent forwarding rejects a board lock. The non-agent folder
				// is deliberately unopened, so its incarnation gate rejects too.
				source: agentSessions{inner: incarnationSource{folder: wswhiteboard.FolderStore{Dir: filepath.Join(root, "unopened")}}},
			}
			dir := sessionInbox(root, reg.ReplyID)
			entry := wsinbox.Entry{Parts: []string{"held text"}}
			if tc.chip {
				entry.Chips = []wsinbox.Chip{{ID: "01", Answer: "ok"}}
			}
			name, _, err := wsinbox.Send(dir, entry, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			raw := fmt.Sprintf(`{"id":%q`, (wsinbox.Record{Name: name}).ID())
			if tc.token != nil {
				raw += fmt.Sprintf(`,"boardIncarnation":%q`, *tc.token)
			}
			d.inboxMu.Lock()
			status, answer := d.inboxWrite(dir, reg.ReplyID, "steer", []byte(raw+"}"), time.Now())
			d.inboxMu.Unlock()
			if status != tc.want {
				t.Fatalf("steer = %d, %v; want %d", status, answer, tc.want)
			}
			held, err := wsinbox.Held(dir)
			if err != nil || len(held) != 1 {
				t.Fatalf("held = %d, %v", len(held), err)
			}
			if held[0].Entry.Steer != (tc.want == http.StatusOK) {
				t.Fatal("refusal changed entry or accepted steer did not publish")
			}
			if tc.want == http.StatusConflict && (answer != "board-replaced" || held[0].Name != name) {
				t.Fatalf("stale board entry was not restored: %v, %s", answer, held[0].Name)
			}
			if tc.agent {
				status, answer := d.messageWrite(dir, reg.ReplyID, []byte(`{"text":"plain untagged agent send"}`), true, time.Now())
				if status != http.StatusOK {
					t.Fatalf("plain agent send = %d, %v", status, answer)
				}
			}
		})
	}
}

func TestBoardlessPageOmitsIncarnation(t *testing.T) {
	for _, board := range []bool{false, true} {
		html, err := renderSessionPage(SessionView{HasBoard: board}, nil, false, "")
		if err != nil {
			t.Fatal(err)
		}
		// Empty is a valid token for an existing, never-loaded board. Only
		// a board-less page must omit the attribute, producing null in JS.
		if strings.Contains(html, ` data-board-incarnation=""`) != board {
			t.Fatalf("board=%v: wrong incarnation attribute presence", board)
		}
	}
}

func TestComposerBoardlessSteerOmitsIncarnation(t *testing.T) {
	src := mustAsset(t, "dashboard.js")
	runNode(t, "boardless steer", composerToastPrelude+composerFunc(t, src, "post")+`
var rid = "session", boardIncarnation = null, captured;
var fetch = function (url, options) {
  captured = JSON.parse(options.body);
  return Promise.resolve({ ok: true, text: function () { return Promise.resolve("{}"); } });
};
window.fetch = fetch;
post("steer", { id: "plain" }, function () {});
check("no tag without a rendered board incarnation", Object.hasOwn(captured, "boardIncarnation"), false);
boardIncarnation = "";
post("steer", { id: "board-chip" }, function () {});
check("never-loaded board retains its valid empty token", captured.boardIncarnation, "");
boardIncarnation = "current";
post("steer", { id: "board-chip" }, function () {});
check("loaded board retains incarnation protection", captured.boardIncarnation, "current");
process.stdout.write(bad.join("\n"));
`)
}
