package mcp

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

func TestPlaybookReadLeadTicketReviewAutoAckFollowsKnob(t *testing.T) {
	read := func(t *testing.T, f *whiteboardFixture, name string) string {
		t.Helper()
		resp := f.call(t, "playbook.read", map[string]any{"name": name})
		if isToolError(resp) {
			t.Fatalf("playbook.read(%s): %s", name, resp)
		}
		body := toolText(t, resp)
		if strings.TrimSpace(body) == "" {
			t.Fatalf("playbook.read(%s): empty document", name)
		}
		return body
	}

	f := newWhiteboardFixture(t)
	on := read(t, f, "lead-ticket")
	offFixture := newWhiteboardFixture(t)
	if resp := offFixture.call(t, "config.tune", map[string]any{"key": wsconfig.ItemTicketReviewAutoAck, "value": "off", "scope": "session"}); isToolError(resp) {
		t.Fatalf("config.tune off: %s", resp)
	}
	wantOff := read(t, offFixture, "lead-ticket")
	if wantOff == on {
		t.Fatalf("knob off must change the rendered lead-ticket text")
	}

	discussOn := read(t, f, "lead-discuss")
	for _, scope := range []string{"session", "project", "repo"} {
		t.Run(scope, func(t *testing.T) {
			g := newWhiteboardFixture(t)
			resp := g.call(t, "config.tune", map[string]any{"key": wsconfig.ItemTicketReviewAutoAck, "value": "off", "scope": scope})
			if isToolError(resp) {
				t.Fatalf("config.tune %s: %s", scope, resp)
			}
			if !strings.Contains(toolText(t, resp), "[scope:"+scope+"]") {
				t.Fatalf("knob write must land in %s scope: %s", scope, resp)
			}
			if got := read(t, g, "lead-ticket"); got != wantOff {
				t.Fatalf("%s-scope off: rendered lead-ticket differs from the knob-off text:\n%s", scope, got)
			}
			if got := read(t, g, "lead-discuss"); got != discussOn {
				t.Fatalf("%s-scope off must leave other playbooks unchanged", scope)
			}
		})
	}
}
