package mcp

import (
	"strings"
	"testing"
)

func TestWhiteboardSuccessNudge(t *testing.T) {
	f := newWhiteboardFixture(t)
	count := func(s string) int { return strings.Count(s, whiteboardAppliedInfo) }
	lastLine := func(s string) string {
		lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
		return lines[len(lines)-1]
	}
	ok := f.update(t, `<wb:topic name="Work"><wb:log>start</wb:log></wb:topic>`)
	if count(ok) != 1 || lastLine(ok) != whiteboardAppliedInfo {
		t.Fatalf("success result: %q", ok)
	}
	f.update(t, `<wb:topic name="Work"><wb:state value="implementing" owner="w" eta_minutes="30"/></wb:topic>`)
	both := f.update(t, `<wb:topic name="Work"><wb:state value="waiting"/></wb:topic>`)
	if count(both) != 1 || lastLine(both) != whiteboardAppliedInfo || !strings.Contains(both, "moved to waiting with no open ask") {
		t.Fatalf("transition result: %q", both)
	}
	resp := f.call(t, "whiteboard.update", map[string]any{"commands": `<wb:bogus/>`})
	if !isToolError(resp) || strings.Contains(toolText(t, resp), "applied to the whiteboard") {
		t.Fatalf("refused result: %s", resp)
	}
}
