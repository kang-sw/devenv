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
		return toolText(t, resp)
	}
	f := newWhiteboardFixture(t)
	on := read(t, f, "ticket-handler")
	leadOn := read(t, f, "lead-ticket")
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
			if got := read(t, g, "ticket-handler"); got == on {
				t.Fatalf("%s-scope off must change the handler", scope)
			}
			if got := read(t, g, "lead-ticket"); got != leadOn {
				t.Fatalf("%s-scope off must leave the lead's relay unchanged", scope)
			}
			if got := read(t, g, "lead-discuss"); got != discussOn {
				t.Fatalf("%s-scope off must leave other playbooks unchanged", scope)
			}
		})
	}
}

func TestTicketHandlerReviewAutoAckFollowsKnobOnRenderAndRead(t *testing.T) {
	f := newWhiteboardFixture(t)
	for _, method := range []string{"playbook.read", "playbook.render"} {
		t.Run(method, func(t *testing.T) {
			body := func() string {
				t.Helper()
				resp := f.call(t, method, map[string]any{"name": "ticket-handler"})
				if isToolError(resp) {
					t.Fatalf("%s: %s", method, resp)
				}
				text := toolText(t, resp)
				if method == "playbook.render" {
					data, err := os.ReadFile(strings.TrimSpace(strings.SplitN(text, "\n", 2)[0]))
					if err != nil {
						t.Fatal(err)
					}
					// The render-minted credential block changes on every call.
					_, text, _ = strings.Cut(string(data), "---\n\n")
				}
				if strings.TrimSpace(text) == "" || strings.Contains(text, "<!-- ws:override:") {
					t.Fatalf("%s: empty body or unresolved markers", method)
				}
				return text
			}
			if resp := f.call(t, "config.tune", map[string]any{"key": wsconfig.ItemTicketReviewAutoAck, "value": "off", "scope": "session"}); isToolError(resp) {
				t.Fatal(resp)
			}
			off := body()
			if resp := f.call(t, "config.tune", map[string]any{"key": wsconfig.ItemTicketReviewAutoAck, "value": "on", "scope": "session"}); isToolError(resp) {
				t.Fatal(resp)
			}
			if on := body(); on == off {
				t.Fatalf("%s: knob on/off must differ beyond credentials", method)
			}
		})
	}
}

func TestTicketKnobOverridePoints(t *testing.T) {
	f := newWhiteboardFixture(t)
	points := map[string]string{
		"TicketHandlerPersistConfirmation": "ticket-handler",
		"TicketHandlerReviewAutoAck":       "ticket-handler",
		"TicketQueueWhiteboard":            "lead-ticket",
	}
	for point, name := range points {
		t.Run(point, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "..", "agents-plugin", "rsrc", name, name+".md"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(data), "<!-- ws:override:"+point+" -->") != 1 ||
				strings.Count(string(data), "<!-- ws:/override:"+point+" -->") != 1 {
				t.Fatal("expected one marker pair")
			}
			knob, value := wsconfig.ItemTicketReviewAutoAck, "off"
			if point == "TicketQueueWhiteboard" {
				knob, value = wsconfig.ItemWorkflowWhiteboardEnabled, "on"
			}
			if resp := f.call(t, "config.tune", map[string]any{"key": knob, "value": value, "scope": "session"}); isToolError(resp) {
				t.Fatal(resp)
			}
			lookup, err := buildOverrideLookup(f.s, f.key)
			if err != nil {
				t.Fatal(err)
			}
			block := "<!-- ws:override:" + point + " -->\nSEED\n<!-- ws:/override:" + point + " -->"
			if got := applyOverrideMarkers(block, "claude", lookup); got == "SEED" || strings.Contains(got, "<!-- ws:") {
				t.Fatal("knob fallback must resolve the point in place")
			}
			// A later all-bucket user override must beat the knob fallback,
			// even when the harness-specific bucket misses.
			for _, bucket := range []string{"*", "claude"} {
				resp := f.call(t, "config.tune", map[string]any{"key": "prompt." + point, "harness": bucket, "scope": "session", "value": "OVERRIDE-" + bucket})
				if isToolError(resp) {
					t.Fatal(resp)
				}
				lookup, err = buildOverrideLookup(f.s, f.key)
				if err != nil {
					t.Fatal(err)
				}
				if got := applyOverrideMarkers(block, "claude", lookup); got != "OVERRIDE-"+bucket {
					t.Fatalf("stored %s override lost: %q", bucket, got)
				}
			}
			keyless, err := buildOverrideLookup(f.s, "")
			if err != nil {
				t.Fatal(err)
			}
			if got := applyOverrideMarkers(block, "claude", keyless); got != "SEED" {
				t.Fatalf("keyless render must use config defaults, not session overrides: %q", got)
			}
		})
	}
}

func TestTicketWhiteboardOverrideKeepsHarnessBuckets(t *testing.T) {
	f := newWhiteboardFixture(t)
	if resp := f.call(t, "config.tune", map[string]any{"key": wsconfig.ItemWorkflowWhiteboardEnabled, "harness": "claude", "value": "on", "scope": "session"}); isToolError(resp) {
		t.Fatal(resp)
	}
	lookup, err := buildOverrideLookup(f.s, f.key)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lookup("TicketQueueWhiteboard", "all"); !ok {
		t.Fatal("claude's whiteboard bucket must enable the fallback")
	}
	f.s.sessionHarness = "codex"
	lookup, err = buildOverrideLookup(f.s, f.key)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lookup("TicketQueueWhiteboard", "all"); ok {
		t.Fatal("claude's whiteboard bucket must not enable codex")
	}
}
