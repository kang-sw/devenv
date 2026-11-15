package wsdashboard

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

func TestConversationMailWakeAssociationFallbacks(t *testing.T) {
	const self = "sweatradartakingoverwrite"
	const peer = "id:suspectcaboosetalonunretired"
	const body = "synthetic body\n\nmail from embedded@machine (2026-11-15T06:55:49Z):\nnot another envelope"
	const text = "[ws mailbox] 1 message(s) arrived for session_key id:" + self + ". They were already received through mailbox.recv; do not call it again for these.\n\nmail from " + peer + " (2026-11-15T06:55:49Z):\n" + body
	for _, tc := range []struct {
		name   string
		change func([]ConvEntry) []ConvEntry
		want   bool
	}{
		{"unique", nil, true},
		{"wsflow", func(es []ConvEntry) []ConvEntry {
			es[1].Text = strings.Replace(es[1].Text, "[ws mailbox]", "[wsflow mailbox]", 1)
			return es
		}, true},
		{"without timestamp", func(es []ConvEntry) []ConvEntry {
			es[1].Text = strings.Replace(es[1].Text, " (2026-11-15T06:55:49Z)", "", 1)
			return es
		}, true},
		{"other recipient", func(es []ConvEntry) []ConvEntry {
			es[1].Text = strings.Replace(es[1].Text, "id:"+self, "id:elsewhere", 1)
			return es
		}, false},
		{"other sender", func(es []ConvEntry) []ConvEntry {
			es[1].Text = strings.Replace(es[1].Text, peer, "id:elsewhere", 1)
			return es
		}, false},
		{"other body", func(es []ConvEntry) []ConvEntry { es[0].Body = "different"; return es }, false},
		{"expired history", func(es []ConvEntry) []ConvEntry { es[0].HasBody = false; return es }, false},
		{"sent only", func(es []ConvEntry) []ConvEntry { es[0].Direction = wslive.MailSent; return es }, false},
		{"reclaimed mail", func(es []ConvEntry) []ConvEntry { return append(es, es[0]) }, false},
		{"repeated wake", func(es []ConvEntry) []ConvEntry { return append(es, es[1]) }, false},
		{"truncated", func(es []ConvEntry) []ConvEntry { es[1].Truncated = true; return es }, false},
		{"untyped", func(es []ConvEntry) []ConvEntry { es[1].Cause = "message"; return es }, false},
		{"invalid timestamp", func(es []ConvEntry) []ConvEntry {
			es[1].Text = strings.Replace(es[1].Text, "2026-11-15T06:55:49Z", "invalid", 1)
			return es
		}, false},
		{"multiple messages", func(es []ConvEntry) []ConvEntry {
			es[1].Text = strings.Replace(es[1].Text, "1 message(s)", "2 message(s)", 1)
			return es
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			es := []ConvEntry{
				{RingEntry: wslive.RingEntry{Kind: wslive.RingMail, Direction: wslive.MailRecv, Peer: peer}, Key: "mail", Body: body, HasBody: true},
				{RingEntry: wslive.RingEntry{Kind: wslive.RingWake, Cause: "mail", Text: text}, Key: "wake"},
			}
			if tc.change != nil {
				es = tc.change(es)
			}
			raw := es[1].Text
			linkMailWakes(es, self)
			if es[1].MatchedMailWake() != tc.want || es[1].Text != raw || es[1].Key != "wake" || es[0].Key != "mail" {
				t.Fatalf("wrong association or changed identity/diagnostics: %+v", es)
			}
			if tc.want && (es[1].Body != body || es[1].MailDeliveryText() != raw) {
				t.Fatal("lost mail body or raw diagnostics")
			}
		})
	}
}

func TestConversationMailWakeReplyIDHeader(t *testing.T) {
	const key = "bankbook-maverick-liberty"
	const peer = "id:suspectcaboosetalonunretired"
	const body = "[ws note: from a peer lead session (pi, not a worker), with the same authority as yours]\nSynthetic regression <text>"
	self := wsmailbox.ReplyID(key)
	for _, recipient := range []string{"id:" + self, key} {
		t.Run(recipient, func(t *testing.T) {
			text := "[ws mailbox] 1 message(s) arrived for session_key " + recipient + ". They were already received through mailbox.recv; do not call it again for these.\n\nmail from " + peer + " (2026-11-15T06:55:49Z):\n" + body
			ring := []wslive.RingRecord{
				ringRecord(1, wslive.RingEntry{Kind: wslive.RingMail, Direction: wslive.MailRecv, Peer: peer, HistoryKey: "h1", Title: "Synthetic regression"}),
				ringRecord(2, wslive.RingEntry{Kind: wslive.RingWake, Cause: "mail", Text: text}),
			}
			lookup := sessionLookup{session: func(id string) (Session, bool) {
				s := liveSession(id, "/proj", testNow)
				s.Label = "EXEC 4"
				return s, id == "suspectcaboosetalonunretired"
			}}
			page := PageContent{MailBodies: map[string]string{"h1": body}}
			v := buildSessionView(pageInputs{Session: liveSession(self, "/proj", testNow), Snapshot: liveSnap(testNow, wslive.Lead{}), Ring: ring, Now: testNow, Page: page, Lookup: lookup})
			if e := v.Conversation.Entries[1]; e.Kind != wslive.RingMail || !e.PeerLinked || !e.PeerLead() || e.Body != body {
				t.Fatalf("wake did not receive linked peer-mail presentation: %+v", e)
			}
			cache := newEntryCache()
			set, err := renderSessionRegions(v, cache)
			if err != nil {
				t.Fatal(err)
			}
			html := string(set[regionEntries].HTML)
			if strings.Contains(html, "mail-notification") || strings.Count(html, `href="/s/suspectcaboosetalonunretired"`) != 2 || !strings.Contains(html, "Synthetic regression &lt;text&gt;") || !strings.Contains(html, "[ws mailbox]") || strings.Count(html, ">EXEC 4</a>") != 2 {
				t.Fatalf("missing linked presentation or raw diagnostics: %s", html)
			}
			// Losing retained history must undo the association on the next
			// render, including when the entry cache already holds the match.
			v = buildSessionView(pageInputs{Session: liveSession(self, "/proj", testNow), Snapshot: liveSnap(testNow, wslive.Lead{}), Ring: ring, Now: testNow, Lookup: lookup})
			set, err = renderSessionRegions(v, cache)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(set[regionEntries].HTML), "mail-notification") {
				t.Fatal("expired history left a stale wake association")
			}
			if ring[1].Kind != wslive.RingWake || ring[1].Text != text {
				t.Fatal("presentation mutated stored wake")
			}
		})
	}
}
