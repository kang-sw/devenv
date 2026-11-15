package wsdashboard

import (
	"encoding/json"
	stdhtml "html"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

const boardNudgeEnvelope = "[ws mailbox] 1 message(s) arrived for session_key id:self. They were already received through mailbox.recv; do not call it again for these.\n\nmail from unknown (2026-11-15T06:55:49Z):\n"

func TestConversationBoardNudgeWakeRender(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{"overdue", "Whiteboard lead nudge (overdue).\nOverdue topics:\n- Release: UI <board>; owner id:lead; initial/current ETA 10/10 minutes; elapsed 600001 ms; extensions 0\nReconcile declared work state.", "보드 알림 · 기한 초과: Release: UI <board>"},
		{"unblocked", "Whiteboard lead nudge (unblocked).\n- Waiting: UI: ready to proceed; resolved targets: plan\nReconcile the waiting declaration.", "보드 알림 · 차단 해제: Waiting: UI"},
		{"hourly", "Whiteboard lead nudge (hourly).\nOpen topics:\n- Planning: open\nOverdue topics:\n- none\nReconcile declared work state.", "보드 알림 · 정기 확인: Planning"},
		{"two topics", "Whiteboard lead nudge (unblocked).\n- One: ready to proceed; resolved targets: plan\n- Two: ready to proceed; resolved targets: plan", "보드 알림 · 차단 해제: One, Two"},
		{"multiple topics and reasons", "Whiteboard lead nudge (overdue, hourly).\nOpen topics:\n- One: implementing\n- Two: open\n- Three: waiting\n- Four: open\nOverdue topics:\n- One; owner id:lead; initial/current ETA 10/10 minutes; elapsed 600001 ms; extensions 1\n  - extension to 10 minutes: more work", "보드 알림 · 기한 초과, 정기 확인: One, Two +2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := boardNudgeEnvelope + tc.body
			ring := []wslive.RingRecord{ringRecord(1, wslive.RingEntry{Kind: wslive.RingWake, Cause: "mail", Source: "mailbox", Caption: strings.ReplaceAll(raw, "\n", " "), Text: raw})}
			original, _ := json.Marshal(ring)
			cache := newEntryCache()
			v := historyView(ring, "", PageContent{}, sessionLookup{})
			v.Lang = "ko"
			for i := 0; i < 2; i++ {
				set, err := renderSessionRegions(v, cache)
				if err != nil {
					t.Fatal(err)
				}
				html := string(set[regionEntries].HTML)
				for _, want := range []string{tc.want, `data-entry="` + ring[0].Name + `"`, `class="mail-notification"`, `data-key="wake-` + ring[0].Name + `"`} {
					if !strings.Contains(stdhtml.UnescapeString(html), want) {
						t.Errorf("missing %q: %s", want, html)
					}
				}
				if strings.Count(stdhtml.UnescapeString(html), raw) != 1 || strings.Count(html, "[ws mailbox]") != 1 || strings.Count(html, "Whiteboard lead nudge (") != 1 || strings.Count(html, "<details") != 1 || strings.Count(html, `<pre class="wake-body">`) != 1 {
					t.Fatalf("raw wake lost, duplicated, or split across disclosures: %s", html)
				}
				for _, bad := range []string{"연결된 메일 없음", "메일 알림", "delivery-meta", "data-mail-wake=", "<board>", `class="delivery-details" open`} {
					if strings.Contains(html, bad) {
						t.Errorf("unexpected %q: %s", bad, html)
					}
				}
			}
			after, _ := json.Marshal(ring)
			if string(original) != string(after) || cache.buildCount() != 1 {
				t.Fatal("display mutated ring records or bypassed entry cache")
			}
			v.Lang = "en"
			set, err := renderSessionRegions(v, cache)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(set[regionEntries].HTML), "Board nudge · ") || cache.buildCount() != 2 {
				t.Fatal("nudge summary reused the wrong language's cached HTML")
			}
		})
	}
}

func TestConversationBoardNudgeWakeFallback(t *testing.T) {
	const body = "Whiteboard lead nudge (overdue).\nOverdue topics:\n- One; owner lead; initial/current ETA 10/10 minutes"
	for _, tc := range []struct {
		name   string
		change func(*wslive.RingEntry)
	}{
		{"ordinary unmatched mail", func(e *wslive.RingEntry) {
			e.Text = strings.Replace(boardNudgeEnvelope, "from unknown", "from peer@machine", 1) + "Ordinary unmatched mail <literal>"
		}},
		{"peer sender", func(e *wslive.RingEntry) { e.Text = strings.Replace(e.Text, "from unknown", "from peer@machine", 1) }},
		{"nonempty peer", func(e *wslive.RingEntry) { e.Peer = "peer@machine" }},
		{"invalid timestamp", func(e *wslive.RingEntry) { e.Text = strings.Replace(e.Text, "2026-11-15T06:55:49Z", "invalid", 1) }},
		{"multiple messages", func(e *wslive.RingEntry) { e.Text = strings.Replace(e.Text, "1 message(s)", "2 message(s)", 1) }},
		{"unknown reason", func(e *wslive.RingEntry) { e.Text = strings.Replace(e.Text, "(overdue)", "(other)", 1) }},
		{"missing reason", func(e *wslive.RingEntry) { e.Text = strings.Replace(e.Text, "(overdue)", "()", 1) }},
		{"malformed header", func(e *wslive.RingEntry) { e.Text = strings.Replace(e.Text, "(overdue).", "(overdue)", 1) }},
		{"prefixed body", func(e *wslive.RingEntry) {
			e.Text = strings.Replace(e.Text, "Whiteboard lead", "prefix Whiteboard lead", 1)
		}},
		{"missing topics", func(e *wslive.RingEntry) {
			e.Text = boardNudgeEnvelope + "Whiteboard lead nudge (overdue).\nOverdue topics:\n- none"
		}},
		{"malformed topic", func(e *wslive.RingEntry) {
			e.Text = strings.Replace(e.Text, "- One; owner lead; initial/current ETA 10/10 minutes", "- One", 1)
		}},
		{"empty topic", func(e *wslive.RingEntry) { e.Text = strings.Replace(e.Text, "- One;", "- ;", 1) }},
		{"truncated", func(e *wslive.RingEntry) { e.Truncated = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := wslive.RingEntry{Kind: wslive.RingWake, Cause: "mail", Text: boardNudgeEnvelope + body}
			tc.change(&e)
			e.Source, e.Caption = "mailbox", strings.ReplaceAll(e.Text, "\n", " ")
			html := presentationHTML(t, []wslive.RingRecord{ringRecord(1, e)}, PageContent{}, nil)
			if !strings.Contains(html, "Unmatched") || strings.Contains(html, "Board nudge ·") || strings.Count(html, "[ws mailbox]") != 1 || strings.Count(stdhtml.UnescapeString(html), e.Text) != 1 || strings.Count(html, "<details") != 1 {
				t.Fatalf("fallback lost its badge or duplicated raw diagnostics: %s", html)
			}
		})
	}
}

func TestConversationBoardNudgeWakeEnvelopeVariants(t *testing.T) {
	const body = "Whiteboard lead nudge (unblocked).\n- One: ready to proceed; resolved targets: plan"
	for _, prefix := range []string{
		boardNudgeEnvelope,
		strings.Replace(boardNudgeEnvelope, "[ws mailbox]", "[wsflow mailbox]", 1),
		strings.Replace(boardNudgeEnvelope, " (2026-11-15T06:55:49Z)", "", 1),
	} {
		e := ConvEntry{RingEntry: wslive.RingEntry{Kind: wslive.RingWake, Cause: "mail", Text: prefix + body}}
		if got := e.BoardNudgeSummary(); got != "Board nudge · Unblocked: One" {
			t.Errorf("summary = %q", got)
		}
		e.Kind = wslive.RingUser
		if e.BoardNudgeSummary() != "" {
			t.Fatal("untyped prose classified as a nudge")
		}
	}
}
