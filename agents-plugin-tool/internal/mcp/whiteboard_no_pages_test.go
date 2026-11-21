package mcp

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestWhiteboardUpdatesCommitWithoutDerivedFiles(t *testing.T) {
	f := newWhiteboardFixture(t)
	if _, err := os.Stat(f.dir()); !os.IsNotExist(err) {
		t.Fatal("created board before use")
	}
	f.call(t, "todo.add", map[string]any{"key": "before", "title": "before the board"})
	if _, err := os.Stat(f.dir()); !os.IsNotExist(err) {
		t.Fatal("todo created a board")
	}
	if resp := f.call(t, "whiteboard.update", map[string]any{"commands": `<wb:unknown/>`}); !isToolError(resp) {
		t.Fatal("invalid first update was accepted")
	}
	if excludeEntries(t, f.root) != 0 {
		t.Fatal("refused first update changed the git exclusion")
	}
	text := f.update(t, "<wb:topic name=\"t\"><wb:panel recap=\"diagram\">```mermaid\ngraph LR\nA-->B\n```</wb:panel></wb:topic>")
	if resultLine(t, text, "full: ") != "page not yet served" {
		t.Fatal(text)
	}
	if excludeEntries(t, f.root) != 1 {
		t.Fatal("board data missing git exclusion")
	}
	name, _ := wswhiteboard.MermaidAsset()
	for _, path := range []string{"index.html", "index.stamp.js", name} {
		if _, err := os.Stat(filepath.Join(f.pageDir(), path)); !os.IsNotExist(err) {
			t.Fatal("created retired output", path)
		}
	}
	rec, err := f.s.sessions.readWhiteboard(f.key)
	if err != nil || rec.Whiteboard == nil || rec.Whiteboard.Seq != 1 {
		t.Fatal("board did not commit once", err)
	}
}

func TestWhiteboardMailboxWritesKeepBoardBytesAndRefreshServedView(t *testing.T) {
	hf := newHistoryFixture(t, time.Now())
	entry, ok := hf.serverB.sessions.lookup(hf.keyB)
	if !ok {
		t.Fatal("missing session")
	}
	server := newTestBoardOwner(t, entry.root, "test")
	f := &whiteboardFixture{s: server, key: hf.keyB, root: entry.root}
	f.update(t, `<wb:topic name="t"><wb:log>retained</wb:log></wb:topic>`)
	_ = f.servedPage(t)
	before := whiteboardTreeForTest(t, f.pageDir())
	mustSend(t, hf.serverB, 10, hf.keyB, "id:"+wsmailbox.ReplyID(hf.keyA), "fresh-outgoing")
	if !strings.Contains(f.servedPage(t), "fresh-outgoing") {
		t.Fatal("mailbox.send left the served panel stale")
	}
	mustSend(t, hf.serverA, 11, hf.keyA, "id:"+f.replyID(), "fresh-incoming")
	if text := toolText(t, f.call(t, "mailbox.recv", map[string]any{})); !strings.Contains(text, "fresh-incoming") {
		t.Fatal(text)
	}
	if !strings.Contains(f.servedPage(t), "fresh-incoming") {
		t.Fatal("mailbox.recv left the served panel stale")
	}
	assertWhiteboardTreeForTest(t, f.pageDir(), before)
}

func TestWhiteboardPanelWritesKeepBoardBytesAndRefreshServedView(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="t"><wb:log>board</wb:log></wb:topic>`)
	swept := make(chan struct{})
	f.source = dashboardPageSource(t, nil, f.s.sessions.boards)
	f.source.store.boardRetirement = &whiteboardRetirement{sweep: func(dir string, live func() (map[string]bool, bool)) {
		sweepRetiredWhiteboardPages(dir, live)
		close(swept)
	}}
	_ = f.servedPage(t)
	// Old outputs recreated later belong to the already-swept interval. Wait
	// for that boundary explicitly, including on hosts with slow file scans.
	select {
	case <-swept:
	case <-time.After(5 * time.Second):
		t.Fatal("first cleanup did not finish")
	}
	before := whiteboardTreeForTest(t, f.pageDir())
	actions := []struct {
		tool         string
		args         map[string]any
		want, absent string
	}{
		{"todo.add", map[string]any{"key": "task", "title": "fresh-todo"}, "fresh-todo", ""},
		{"todo.check", map[string]any{"key": "task", "status": "done"}, "fresh-todo", ""},
		{"agenda.set", map[string]any{"key": "fresh-agenda", "value": map[string]any{"text": "fresh-value"}}, "fresh-value", ""},
		{"agenda.clear", map[string]any{"key": "fresh-agenda"}, "fresh-todo", "fresh-value"},
	}
	for _, action := range actions {
		if resp := f.call(t, action.tool, action.args); isToolError(resp) {
			t.Fatal(resp)
		}
		page := f.servedPage(t)
		if !strings.Contains(page, action.want) || action.absent != "" && strings.Contains(page, action.absent) {
			t.Fatalf("stale served panel after %s", action.tool)
		}
		assertWhiteboardTreeForTest(t, f.pageDir(), before)
	}
	// Even an old writer's outputs are untouched by updates and panel actions.
	for _, name := range []string{"index.html", "index.stamp.js"} {
		mustWrite(t, f.pageDir(), name, "old derived output")
	}
	f.call(t, "agenda.set", map[string]any{"key": "again", "value": 2})
	f.update(t, `<wb:topic name="t"><wb:log>next</wb:log></wb:topic>`)
	for _, name := range []string{"index.html", "index.stamp.js"} {
		if !bytes.Equal(readFileForTest(t, filepath.Join(f.pageDir(), name)), []byte("old derived output")) {
			t.Fatal("action touched derived file", name)
		}
	}
}
