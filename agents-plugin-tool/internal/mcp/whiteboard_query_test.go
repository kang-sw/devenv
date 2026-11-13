package mcp

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func queryWhiteboardForTest(t *testing.T, f *whiteboardFixture, payload map[string]any) string {
	t.Helper()
	resp := f.call(t, "whiteboard.query", map[string]any{"payload": payload})
	if isToolError(resp) {
		t.Fatalf("query %+v: %s", payload, toolText(t, resp))
	}
	return toolText(t, resp)
}

func queryPayloadForTest(t *testing.T, text string) string {
	t.Helper()
	parts := strings.SplitN(text, "\n", 2)
	if len(parts) != 2 {
		t.Fatalf("missing direct payload header: %q", text)
	}
	return parts[1]
}

func TestWhiteboardQueryFreshSessionConversionAndRecoverySmoke(t *testing.T) {
	for _, layout := range []string{"inline", "split"} {
		t.Run(layout, func(t *testing.T) {
			f := newWhiteboardFixture(t)
			legacyPath, original := installLegacyWhiteboardForTest(t, f, layout, legacyWhiteboardJSON)
			const topic = "stemless / CON: .."
			text := queryWhiteboardForTest(t, f, map[string]any{"topic": topic})
			if !strings.Contains(text, "current e1 [imported]") || !strings.Contains(text, "e1 [agent] [imported]") || strings.Contains(text, "original content") {
				t.Fatalf("conversion discovery/provenance: %s", text)
			}
			imported := queryWhiteboardForTest(t, f, map[string]any{"entry": "e1"})
			if !strings.Contains(strings.SplitN(imported, "\n", 2)[0], "[imported]") || queryPayloadForTest(t, imported) != "imported decision" {
				t.Fatalf("direct imported entry: %q", imported)
			}

			const payload = "\n<svg><text>payload &amp; exact</text></svg>\n\n"
			f.update(t, `<wb:topic name="`+topic+`"><wb:panel recap="smoke panel">`+"\n"+payload+"\n"+`</wb:panel><wb:log>current agent line</wb:log><wb:add class="policy"><wb:title>new question</wb:title><wb:option>Yes</wb:option><wb:option>No</wb:option></wb:add></wb:topic><wb:answer id="43" value="a">owner exact words</wb:answer>`)
			rec, err := f.s.sessions.readWhiteboard(f.key)
			if err != nil {
				t.Fatal(err)
			}
			var panel wswhiteboard.Entry
			var agent wswhiteboard.Entry
			for _, doc := range rec.Whiteboard.Topics {
				if doc.Name != topic {
					continue
				}
				for _, entry := range doc.Entries {
					if entry.Kind == wswhiteboard.EntryPanel {
						panel = entry
					}
					if entry.Kind == wswhiteboard.EntryAgent {
						agent = entry
					}
				}
			}
			if panel.ID == "" || agent.ID == "" {
				t.Fatal("accepted commands did not produce entries")
			}
			text = queryWhiteboardForTest(t, f, map[string]any{"topic": topic})
			if !strings.Contains(text, "panel p1 ("+panel.ID+")") || !strings.Contains(text, "current "+agent.ID) || strings.Contains(text, "payload &amp; exact") || strings.Contains(text, "original content") {
				t.Fatalf("discovery payload leak or missing metadata: %s", text)
			}
			item := queryWhiteboardForTest(t, f, map[string]any{"id": 43, "depth": "full"})
			if !strings.Contains(item, `answer_quote: "owner exact words"`) {
				t.Fatalf("legacy item quote lost: %s", item)
			}
			body := queryWhiteboardForTest(t, f, map[string]any{"topic": topic, "body": true})
			if !strings.Contains(body, "original content\n") || strings.Contains(body, payload) {
				t.Fatalf("explicit content retrieval: %s", body)
			}
			if queryPayloadForTest(t, queryWhiteboardForTest(t, f, map[string]any{"panel": "p1"})) != payload {
				t.Fatal("latest panel payload changed bytes")
			}

			// Payload retrieval supports full repost, not a panel-targeted edit.
			f.update(t, `<wb:topic name="`+topic+`"><wb:panel id="p1" recap="full repost">`+"\n"+payload+"\n"+`</wb:panel></wb:topic>`)
			page := string(readFileForTest(t, f.pagePath()))
			if !strings.Contains(page, "full repost") || !strings.Contains(page, "current agent line") {
				t.Fatal("committed state did not render")
			}
			revive := toolText(t, f.call(t, "workflow_state", map[string]any{}))
			if !strings.Contains(revive, "### Whiteboard topics\n- "+topic+": current agent line\n") || strings.Contains(revive, "Whiteboard recaps") || strings.Contains(revive, "full repost") {
				t.Fatalf("revive did not use latest agent only: %s", revive)
			}

			// A mention resolves the exact old version/log after daemon restart.
			f.s = NewServer(f.root, "restart-query")
			mentions := []wsinbox.Mention{{Topic: topic, Entry: panel.ID, Text: "selected panel"}, {Topic: topic, Entry: agent.ID, Text: "selected log"}}
			parsed, rest := wsinbox.ParseMentions(wsinbox.MentionBlock(mentions))
			if len(parsed) != 2 || rest != "" {
				t.Fatal("entry mentions did not parse")
			}
			if got := queryPayloadForTest(t, queryWhiteboardForTest(t, f, map[string]any{"topic": parsed[0].Topic, "panel": parsed[0].Entry})); got != payload {
				t.Fatal("panel mention redirected or payload changed")
			}
			if got := queryPayloadForTest(t, queryWhiteboardForTest(t, f, map[string]any{"topic": parsed[1].Topic, "entry": parsed[1].Entry})); got != "current agent line" {
				t.Fatal("log mention redirected")
			}
			if latest := queryWhiteboardForTest(t, f, map[string]any{"panel": "p1"}); strings.Contains(strings.SplitN(latest, "\n", 2)[0], "entry "+panel.ID+" ") {
				t.Fatal("stable target did not advance to the repost")
			}
			for _, selector := range []string{"panel", "entry"} {
				resp := f.call(t, "whiteboard.query", map[string]any{"payload": map[string]any{selector: "e999999"}})
				if !isToolError(resp) || !strings.Contains(toolText(t, resp), "stale or missing locator") {
					t.Fatalf("missing %s locator was redirected: %s", selector, resp)
				}
			}
			if err := f.s.sessions.setOverride(f.key, "test.query", fmt.Sprint(layout)); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(readFileForTest(t, legacyPath), original) {
				t.Fatal("conversion/update/query/render/revive/restart changed preserved legacy board bytes")
			}
		})
	}
}
