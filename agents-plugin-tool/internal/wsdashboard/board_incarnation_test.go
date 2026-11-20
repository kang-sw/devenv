package wsdashboard

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

type incarnationSource struct {
	SessionSource
	folder   wswhiteboard.FolderStore
	indexLag bool
}

func (s incarnationSource) ReadSession(reg Registration) (SessionData, bool, error) {
	data, ok, err := s.SessionSource.ReadSession(reg)
	if s.indexLag {
		data.HasBoard = false // render remains current while the index lags
	}
	return data, ok, err
}

func (s incarnationSource) LockBoardIncarnation(_ Registration, token string) (func(), bool) {
	return s.folder.LockIncarnation(token)
}

func (s incarnationSource) ReadPage(reg Registration) (PageContent, bool, error) {
	return s.SessionSource.(PageSource).ReadPage(reg)
}

func TestBoardIncarnationSendAdmission(t *testing.T) {
	testBoardIncarnationSendAdmission(t, false)
}

func TestBoardIncarnationSendAdmissionWithIndexLag(t *testing.T) {
	testBoardIncarnationSendAdmission(t, true)
}

func testBoardIncarnationSendAdmission(t *testing.T, indexLag bool) {
	t.Helper()
	root := t.TempDir()
	board := func(dir string) wswhiteboard.FolderStore {
		t.Helper()
		f := wswhiteboard.FolderStore{Dir: filepath.Join(root, dir)}
		err := f.Update(nil, func(s *wswhiteboard.State) error {
			_, problems := wswhiteboard.Apply(s, `<wb:topic name="work"><wb:add class="check"><wb:title>Approve</wb:title></wb:add></wb:topic>`, wswhiteboard.Env{Now: time.Now()})
			if len(problems) != 0 {
				return fmt.Errorf("apply: %v", problems)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	dest, source := board("dest"), board("source")
	old, err := dest.Open(nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := source.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(root, "board.zip")
	if err := os.WriteFile(zipPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	f := startPageFixture(t, func(o *DaemonOptions) {
		o.Sessions = incarnationSource{SessionSource: o.Sessions, folder: dest, indexLag: indexLag}
	})
	dir := sessionInbox(f.dir, "livesession")
	send := func(token *string) int {
		t.Helper()
		raw := `{"chips":[{"id":"01","answer":"ok"}]`
		if token != nil {
			raw += fmt.Sprintf(`,"boardIncarnation":%q`, *token)
		}
		status, answer := f.d.messageWrite(dir, "livesession", []byte(raw+"}"), true, time.Now())
		if status == http.StatusConflict && answer != "board-replaced" {
			t.Fatalf("unexpected refusal: %v", answer)
		}
		return status
	}
	if got := send(&old.LoadedAt); got != http.StatusOK {
		t.Fatalf("initial tagged send = %d", got)
	}
	// Even before root publication, a load has closed admission. This send
	// must fail immediately, without waiting on the board's file lock.
	loading := dest
	loading.Fault = func(stage string) error {
		if stage == "intent" {
			if got := send(&old.LoadedAt); got != http.StatusConflict {
				t.Errorf("send during load = %d", got)
			}
		}
		return nil
	}
	if _, _, err := loading.LoadArchive(zipPath, time.Now()); err != nil {
		t.Fatal(err)
	}
	current, err := dest.Open(nil)
	if err != nil {
		t.Fatal(err)
	}
	if session, ok := f.d.index.Session("livesession"); !ok || session.HasBoard == indexLag {
		t.Fatalf("fixture did not establish index lag=%v: %+v, %v", indexLag, session, ok)
	}
	// Refusal does not even touch the inbox or board files.
	before := wsstate.FileIOCounters()["file_io_total"]
	if got := send(&old.LoadedAt); got != http.StatusConflict {
		t.Fatalf("stale tagged send after load = %d", got)
	}
	if after := wsstate.FileIOCounters()["file_io_total"]; after != before {
		t.Fatalf("stale admission added %d file operations", after-before)
	}
	code, _, refusal := postJSON(t, f.base+"/s/livesession/message", f.base, fmt.Sprintf(`{"boardIncarnation":%q,"chips":[{"id":"01","answer":"ok"}]}`, old.LoadedAt))
	if code != http.StatusConflict || strings.TrimSpace(refusal) != "board-replaced" {
		t.Fatalf("HTTP stale send = %d, %q", code, refusal)
	}
	if got := send(&current.LoadedAt); got != http.StatusOK {
		t.Fatalf("current tagged send = %d", got)
	}
	if got := send(nil); got != http.StatusOK {
		t.Fatalf("missing-field legacy send = %d", got)
	}
	f.src.setPage("livesession", PageContent{HasBoard: true, BoardIncarnation: current.LoadedAt})
	_, html := get(t, f.base+"/s/livesession?frame=1")
	if !strings.Contains(html, `data-board-incarnation="`+current.LoadedAt+`"`) {
		t.Fatal("rendered page omitted the board incarnation")
	}
	wantHeld := 3
	if indexLag {
		status, answer := f.d.messageWrite(dir, "livesession", []byte(`{"text":"plain untagged during index lag"}`), true, time.Now())
		if status != http.StatusOK {
			t.Fatalf("plain untagged send during index lag = %d, %v", status, answer)
		}
		wantHeld++
	}
	held, err := wsinbox.Held(dir)
	if err != nil || len(held) != wantHeld {
		t.Fatalf("held = %d, %v; stale sends must record nothing", len(held), err)
	}
	code, _, refusal = postJSON(t, f.base+"/s/livesession/steer", f.base, fmt.Sprintf(`{"id":%q,"boardIncarnation":%q}`, held[0].ID(), old.LoadedAt))
	if code != http.StatusConflict || strings.TrimSpace(refusal) != "board-replaced" {
		t.Fatalf("HTTP stale steer = %d, %q", code, refusal)
	}
	// A journal accepted before load must not attach its arrival marker to
	// the replacement, even when replay runs after publication.
	if err := dest.RecordDashboardAnswer("pre-load-id", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := dest.RecordDashboardAnswer("current-id", time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	after, err := dest.Open(nil)
	if err != nil {
		t.Fatal(err)
	}
	currentMarker := false
	for _, topic := range after.Topics {
		for _, entry := range topic.Entries {
			if strings.Contains(entry.Text, "pre-load-id") {
				t.Fatal("old answer journal attached to replacement")
			}
			currentMarker = currentMarker || strings.Contains(entry.Text, "current-id")
		}
	}
	if !currentMarker {
		t.Fatal("current answer journal was not attached")
	}
}

func TestComposerBoardIncarnation(t *testing.T) {
	src := mustAsset(t, "dashboard.js")
	runNode(t, "assembled dashboard syntax", fmt.Sprintf("new (require('node:vm').Script)(%q);", src))
	runNode(t, "composer incarnation", selectionCodecSource(t)+composerToastPrelude+composerSendSource(t, src)+composerFunc(t, src, "boardDraft")+outboxDOM+`
var sending = false, boardIncarnation = "rendered-old", rid = "session", draftTimer = null;
function inlineEditor() { return null; }
var saves = 0, reloads = 0, postedBody;
saveDraft = function () { saves++; };
window.location = { reload: function () { reloads++; } };
function noteWarn() {}
chipList = function () { return [{ id: "01", answer: "ok" }]; };
function boardItem() { return null; }
var reply;
post = function (route, body, done) { postedBody = body; reply = done; };
ctext.value = "answer"; sendMessage();
check("send carries rendered incarnation", postedBody.boardIncarnation, "rendered-old");
reply(false, "board-replaced");
check("stale response reloads page", reloads, 1);
check("failed send saved before reload", saves, 2);
check("refused send never acknowledged", outbox.filter(function (m) { return m.acked; }).length, 0);
boardIncarnation = "replacement";
var restored = boardDraft({ boardIncarnation: "rendered-old", text: "keep my text", chips: [{ id: "01", answer: "ok" }] });
check("saved old-incarnation chips are not restored onto replacement", restored.chips, []);
check("free text survives replacement reload", restored.text, "keep my text");
process.stdout.write(bad.join("\n"));
`)
	if !strings.Contains(src, `var boardIncarnation = root.getAttribute("data-board-incarnation")`) {
		t.Fatal("incarnation must be frozen from the rendered page, not live-swapped markup")
	}
	runNode(t, "steer incarnation", composerToastPrelude+composerFunc(t, src, "post")+`
var rid = "session", boardIncarnation = "rendered-old", captured, refused = false, reloads = 0, draftTimer = null;
function saveDraft() {}
var fetch = function (url, options) {
  captured = JSON.parse(options.body);
  return Promise.resolve({ ok: false, status: 409, text: function () { return Promise.resolve("board-replaced\n"); } });
};
window.fetch = fetch;
window.location = { reload: function () { reloads++; } };
post("steer", { id: "held" }, function (ok, answer) { refused = !ok && answer === "board-replaced"; });
setTimeout(function () {
  check("steer carries rendered incarnation", captured.boardIncarnation, "rendered-old");
  check("stale steer refused", refused, true);
  check("stale steer reloads", reloads, 1);
  process.stdout.write(bad.join("\n"));
}, 0);
`)
}

func TestComposerRestoredAnswerIncarnation(t *testing.T) {
	src := mustAsset(t, "dashboard.js")
	runNode(t, "restored answer incarnation", composerToastPrelude+composerFunc(t, src, "restoreHeldNow")+`
var boardIncarnation = "2026-11-18T00:00:00.000Z", MARKER = "image ([0-9]+)";
function inlineEditor() { return null; }
function noteWarn() {}
function splitSelectionText(text) { return { text: text, groups: [] }; }
function markerNums() { return {}; }
function restoreTopics() { return []; }
function shiftMentions() {}
function validMentions(v) { return v || []; }
function validCloseRequests(v) { return v || []; }
function validPicks(v) { return v || []; }
function validConsoles(v) { return v || []; }
function putChip(c) { chips[c.id] = c; }
restoreHeldNow([{ sentAt: Date.parse(boardIncarnation) - 1, chips: [{ id: "01", answer: "old" }] }]);
check("cancelled pre-load answer cannot attach to reused ID", chips["01"], undefined);
restoreHeldNow([{ sentAt: Date.parse(boardIncarnation) + 1, chips: [{ id: "01", answer: "current" }] }]);
check("current cancelled answer can be restored", chips["01"].answer, "current");
process.stdout.write(bad.join("\n"));
`)
}
