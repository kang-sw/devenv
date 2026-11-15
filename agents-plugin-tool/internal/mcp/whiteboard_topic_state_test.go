package mcp

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestWhiteboardTopicStateResponseAndRestart(t *testing.T) {
	f := newWhiteboardFixture(t)
	out := f.update(t, `<wb:topic name="state" label="State" tickets="261114-feat-example"><wb:log>Human summary</wb:log><wb:state value="discussing"/></wb:topic>`)
	if !strings.Contains(out, `info: discussing_without_open_items: "State"`) {
		t.Fatalf("missing response-only warning: %s", out)
	}
	if page := string(readFileForTest(t, f.pagePath())); strings.Contains(page, "discussing_without_open_items") {
		t.Fatal("response-only warning leaked onto page")
	}
	f.update(t, `<wb:topic name="state"><wb:add class="check"><wb:title>Verify</wb:title></wb:add><wb:state value="implementing" owner="executor" eta_minutes="40"/></wb:topic>`)
	restarted := NewServer(f.root, "restart")
	rec, err := restarted.sessions.readWhiteboard(f.key)
	if err != nil {
		t.Fatal(err)
	}
	state := rec.Whiteboard.TopicStates()["state"].State
	if state == nil || state.Run == nil || state.Run.DeadlineAt == "" || state.Run.Owner != "executor" {
		t.Fatalf("missing persisted state: %+v", state)
	}
	for _, payload := range []any{nil, map[string]any{"topic": "state", "body": true}, map[string]any{"topic": "state", "entries": "all", "kind": "agent"}} {
		text := toolText(t, f.call(t, "whiteboard.query", map[string]any{"payload": payload}))
		for _, want := range []string{"state: implementing; entry: " + state.Entry, "deadline_at: " + state.Run.DeadlineAt, "tickets: 261114-feat-example", "Human summary"} {
			if !strings.Contains(text, want) {
				t.Fatalf("query %v lacks %q: %s", payload, want, text)
			}
		}
	}
	f.update(t, `<wb:topic name="state"><wb:close result="Done"/><wb:log>Reopened</wb:log></wb:topic>`)
	rec, err = restarted.sessions.readWhiteboard(f.key)
	if err != nil || rec.Whiteboard.TopicStates()["state"].State != nil || rec.Whiteboard.Items[0].Status != wswhiteboard.StatusWithdrawn {
		t.Fatalf("reopen restored state or ask: %v", err)
	}
}

func TestWhiteboardTopicStateCutoverAndAtomicRefusal(t *testing.T) {
	f := newWhiteboardFixture(t)
	legacyPath, original := installLegacyWhiteboardForTest(t, f, "split", legacyWhiteboardJSON)
	f.update(t, `<wb:topic name="recap only"><wb:state value="implementing" owner="executor" eta_minutes="1"/></wb:topic>`)
	if !bytes.Equal(original, readFileForTest(t, legacyPath)) {
		t.Fatal("state update rewrote rollback snapshot")
	}
	logPath := filepath.Join(f.pageDir(), "t2", "log.jsonl")
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(log), []byte{'\n'})
	if len(lines) != 2 || bytes.Contains(lines[0], []byte("topic_state")) || !bytes.Contains(lines[1], []byte("topic_state")) {
		t.Fatalf("state did not follow legacy cutover: %s", log)
	}
	before := whiteboardTreeForTest(t, f.pageDir())
	resp := f.call(t, "whiteboard.update", map[string]any{"commands": `<wb:topic name="recap only"><wb:log>Must roll back</wb:log><wb:state value="implementing" owner="different" eta_minutes="1"/></wb:topic>`})
	if !isToolError(resp) || !strings.Contains(toolText(t, resp), "implementing owner/ETA differs") {
		t.Fatalf("expected atomic state refusal: %s", resp)
	}
	assertWhiteboardTreeForTest(t, f.pageDir(), before)
}
