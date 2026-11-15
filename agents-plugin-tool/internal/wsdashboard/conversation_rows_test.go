package wsdashboard

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Native summary owns body-click and Enter/Space activation, while the peer
// anchor retains its own navigation. No secondary disclosure or click handler.
func TestConversationRowsShareNativeFrame(t *testing.T) {
	assets, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []ConvEntry{
		{RingEntry: wslive.RingEntry{Kind: "wake", Cause: "mail", Source: "mailbox", Caption: "arrived", Text: "raw <wake>"}, Key: "wake"},
		{RingEntry: wslive.RingEntry{Kind: "mail", Direction: "recv", Peer: "peer", Title: "preview"}, Key: "peer", HasBody: true, Body: "[ws note: from a peer lead session (pi, not a worker), with the same authority as yours]\nbody <mail>", PeerLinked: true, PeerReplyID: "other"},
		{RingEntry: wslive.RingEntry{Kind: "tools", Tools: toolsSummary()}, Key: "tools"},
		{RingEntry: wslive.RingEntry{Kind: "reply", Text: "reply"}, Key: "reply", HTML: "reply"},
	} {
		t.Run(e.Key, func(t *testing.T) {
			var buf bytes.Buffer
			if err := assets.tmpl.ExecuteTemplate(&buf, "entry", e); err != nil {
				t.Fatal(err)
			}
			html := buf.String()
			for _, want := range []string{`conv-row`, `class="row-icon`, `class="row-content`, `class="row-time"`, `data-entry="` + e.Key + `"`} {
				if !strings.Contains(html, want) {
					t.Errorf("missing frame %q: %s", want, html)
				}
			}
			if e.Kind != "reply" {
				if strings.Count(html, "<details") != 1 || strings.Count(html, `<summary class="conv-row">`) != 1 || strings.Count(html, `class="row-chevron"`) != 1 || strings.Contains(html, " open") {
					t.Fatalf("row is not one collapsed native disclosure: %s", html)
				}
			}
			if e.Key == "peer" && (!strings.Contains(html, `href="/s/other"`) || !strings.Contains(html, "body &lt;mail&gt;") || !strings.Contains(html, "[ws note:") || strings.Contains(html, "Delivery details")) {
				t.Fatal("peer link/body/raw delivery lost or second disclosure retained: " + html)
			}
			if e.Key == "wake" && !strings.Contains(html, "raw &lt;wake&gt;") {
				t.Fatal("wake diagnostics lost or interpreted")
			}
		})
	}
}

func TestConversationRowsItemChipsUseOnlySavedEffectsPerTopic(t *testing.T) {
	e := effectPresentation(t,
		wswhiteboard.Effect{Kind: "item", Topic: "plan", ID: 1031, Field: "title", After: "redundant item label"},
		wswhiteboard.Effect{Kind: "item", Topic: "plan", ID: 1031, Field: "answer", After: "yes"},
		wswhiteboard.Effect{Kind: "item", Topic: "plan", ID: 1032, Field: "answer", After: "yes"},
		wswhiteboard.Effect{Kind: "item", Topic: "other", ID: 1033, Field: "answer", After: "yes"},
		wswhiteboard.Effect{Kind: "item", Topic: "other", ID: 1000000, Field: "answer", After: "no target"},
		wswhiteboard.Effect{Kind: "item", ID: 1034, Field: "answer", After: "no topic"},
		wswhiteboard.Effect{Kind: "topic", Topic: "plan", Field: "label", After: "Plan label"},
		wswhiteboard.Effect{Kind: "log", Topic: "plan", EntryID: "e7", Field: "text", After: "agent <log>"},
	)
	html := presentationHTML(t, []wslive.RingRecord{ringRecord(1, e.RingEntry)}, PageContent{}, nil)
	if strings.Count(html, `class="wbupdate-items"`) != 3 || strings.Count(html, `>(1031)</a>`) != 1 {
		t.Fatal("not one deduplicated chip row per topic: " + html)
	}
	for _, want := range []string{
		`href="#item-1031" data-wb-jump="item-1031" data-wb-topic="topic-706c616e"`,
		`href="#item-1032" data-wb-jump="item-1032" data-wb-topic="topic-706c616e"`,
		`href="#item-1033" data-wb-jump="item-1033" data-wb-topic="topic-6f74686572"`,
		`<span class="wbupdate-item-id">(1000000)</span>`, `<span class="wbupdate-item-id">(1034)</span>`,
		`<b>Plan label</b>`, `class="wbupdate-key">plan`, `data-wb-element="e7"`, `agent &lt;log&gt;`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("lost chip/header/log contract %q: %s", want, html)
		}
	}
	for _, bad := range []string{"item-518", "item-520", "item-521", "item-523", "item-524", "item-1000000", "item-1034", "redundant item label", "Answer on board", "wbeffect-details", "wbeffect-values"} {
		if strings.Contains(html, bad) {
			t.Errorf("invented target/item or retained redundant UI %q: %s", bad, html)
		}
	}
}
