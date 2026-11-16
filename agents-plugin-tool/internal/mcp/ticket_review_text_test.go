package mcp

import (
	"os"
	"path/filepath"
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
	for _, v := range ticketReviewAutoAckVariants["lead-ticket"] {
		if !strings.Contains(on, v.on) || strings.Contains(wantOff, v.on) || !strings.Contains(wantOff, v.off) {
			t.Fatalf("knob off must replace the lead-ticket text in place:\n%s", v.on)
		}
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

// Each knob-off substitution anchors on shipped text: every on-text occurs
// exactly once in its playbook in both packages, so a later edit of that
// text fails here instead of silently appending the knob-off text.
func TestTicketReviewAutoAckOnTextsPinShippedPlaybooks(t *testing.T) {
	for _, pkg := range []string{"agents-plugin", "agents-plugin-wsflow"} {
		for name, variants := range ticketReviewAutoAckVariants {
			data, err := os.ReadFile(filepath.Join("..", "..", "..", pkg, "rsrc", name, name+".md"))
			if err != nil {
				t.Fatalf("%s/%s: %v", pkg, name, err)
			}
			for _, v := range variants {
				if n := strings.Count(string(data), v.on); n != 1 {
					t.Errorf("%s/%s: on-text occurs %d times, want 1:\n%s", pkg, name, n, v.on)
				}
			}
		}
	}
}

// ticket-handler is a rendered worker playbook: playbook.render applies the
// knob-off text as playbook.read does.
func TestTicketHandlerReviewAutoAckFollowsKnobOnRenderAndRead(t *testing.T) {
	handler := ticketReviewAutoAckVariants["ticket-handler"]
	render := func(t *testing.T, f *whiteboardFixture) string {
		t.Helper()
		resp := f.call(t, "playbook.render", map[string]any{"name": "ticket-handler"})
		if isToolError(resp) {
			t.Fatalf("playbook.render: %s", resp)
		}
		data, err := os.ReadFile(strings.TrimSpace(strings.SplitN(toolText(t, resp), "\n", 2)[0]))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	read := func(t *testing.T, f *whiteboardFixture) string {
		t.Helper()
		resp := f.call(t, "playbook.read", map[string]any{"name": "ticket-handler"})
		if isToolError(resp) {
			t.Fatalf("playbook.read: %s", resp)
		}
		return toolText(t, resp)
	}
	on := newWhiteboardFixture(t)
	for _, body := range []string{render(t, on), read(t, on)} {
		for _, v := range handler {
			if !strings.Contains(body, v.on) {
				t.Fatalf("knob on: ticket-handler lacks its shipped text:\n%s", v.on)
			}
		}
	}
	off := newWhiteboardFixture(t)
	if resp := off.call(t, "config.tune", map[string]any{"key": wsconfig.ItemTicketReviewAutoAck, "value": "off", "scope": "session"}); isToolError(resp) {
		t.Fatalf("config.tune off: %s", resp)
	}
	for _, body := range []string{render(t, off), read(t, off)} {
		for _, v := range handler {
			if strings.Contains(body, v.on) || !strings.Contains(body, v.off) {
				t.Fatalf("knob off: ticket-handler must carry the knob-off text in place:\n%s", v.off)
			}
		}
	}
}
