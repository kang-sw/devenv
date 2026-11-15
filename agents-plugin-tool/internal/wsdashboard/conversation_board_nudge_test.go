package wsdashboard

import (
	"encoding/json"
	"fmt"
	stdhtml "html"
	"slices"
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
		{"zero messages", func(e *wslive.RingEntry) { e.Text = strings.Replace(e.Text, "1 message(s)", "0 message(s)", 1) }},
		{"overflow count", func(e *wslive.RingEntry) {
			e.Text = strings.Replace(e.Text, "1 message(s)", "999999999999999999999999 message(s)", 1)
		}},
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
		strings.Replace(boardNudgeEnvelope, "1 message(s)", "12 message(s)", 1),
		"mail from unknown (2026-11-15T06:55:49Z):\n",
		"mail from unknown:\n",
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

func unblockedNudgeMail(topic string) string {
	return "mail from unknown (2026-11-15T06:55:49Z):\nWhiteboard lead nudge (unblocked).\n- " + topic + ": ready to proceed; resolved targets: owner\nReconcile the waiting declaration."
}

// Mirrors mcp.formatMailBatch and wsagent.runner: the first ring wake carries
// the batch header, and all subsequent wakes carry only their own envelope.
func boardNudgeBatchRing(texts ...string) []wslive.RingRecord {
	ring := make([]wslive.RingRecord, 0, len(texts))
	for i, text := range texts {
		if i == 0 {
			text = fmt.Sprintf("[ws mailbox] %d message(s) arrived for session_key id:self. They were already received through mailbox.recv; do not call it again for these.\n\n", len(texts)) + text
		}
		ring = append(ring, ringRecord(i+1, wslive.RingEntry{Kind: wslive.RingWake, Cause: "mail", Source: "mailbox", Text: text}))
	}
	return ring
}

func TestConversationBoardNudgeTwelveMailBatch(t *testing.T) {
	var texts []string
	for i := 1; i <= 12; i++ {
		texts = append(texts, unblockedNudgeMail(fmt.Sprintf("Topic <%02d>", i)))
	}
	ring := boardNudgeBatchRing(texts...)
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
		plain := stdhtml.UnescapeString(html)
		if !strings.Contains(plain, "보드 알림 · 차단 해제 12건: Topic <01>, Topic <02> +10") {
			t.Fatalf("batch summary missing: %s", html)
		}
		if strings.Count(html, `class="mail-notification"`) != 1 || strings.Count(html, "<details") != 1 || strings.Count(html, `data-mail-wake="suppressed"`) != 11 || strings.Count(html, "data-entry=") != 12 || strings.Count(html, "[ws mailbox]") != 1 {
			t.Fatalf("batch did not render as one row plus keyed markers: %s", html)
		}
		if strings.Contains(html, "연결된 메일 없음") || strings.Contains(html, "메일 알림") || strings.Contains(html, "<01>") {
			t.Fatal("batch has unmatched badge or unescaped text")
		}
		for j, text := range texts {
			if strings.Count(plain, text) != 1 || !strings.Contains(html, `data-entry="`+ring[j].Name+`"`) {
				t.Fatalf("raw mail or key %d lost/duplicated", j)
			}
		}
	}
	after, _ := json.Marshal(ring)
	if string(original) != string(after) || cache.buildCount() != 12 {
		t.Fatal("batch mutated ring or bypassed cache")
	}
}

func TestConversationBoardNudgeMixedBatchAndMalformedMember(t *testing.T) {
	const peer = "mail from peer@machine (2026-11-15T06:55:49Z):\nOrdinary peer mail <literal>"
	const bad = "mail from unknown (invalid):\nWhiteboard lead nudge (unblocked).\n- Malformed: ready to proceed; resolved targets: owner"
	for _, middle := range []string{peer, bad} {
		t.Run(strings.Split(middle, "\n")[0], func(t *testing.T) {
			ring := boardNudgeBatchRing(unblockedNudgeMail("One"), unblockedNudgeMail("Two"), middle, unblockedNudgeMail("Three"), unblockedNudgeMail("Four"))
			v := historyView(ring, "", PageContent{}, sessionLookup{})
			entries := v.Conversation.Entries
			if len(entries) != 5 || !entries[1].MailWakeSuppressed || !entries[4].MailWakeSuppressed || entries[2].MailWakeSuppressed || entries[2].BoardNudgeSummary() != "" || entries[2].Text != middle {
				t.Fatalf("peer/malformed member did not break the run: %+v", entries)
			}
			html := presentationHTML(t, ring, PageContent{}, nil)
			for _, want := range []string{"Board nudge · Unblocked · 2 messages: One, Two", "Board nudge · Unblocked · 2 messages: Three, Four", "Unmatched", stdhtml.EscapeString(middle)} {
				if !strings.Contains(html, want) {
					t.Errorf("missing %q: %s", want, html)
				}
			}
			if strings.Count(html, `class="mail-notification"`) != 3 || strings.Count(html, "<details") != 3 || strings.Count(html, `data-mail-wake="suppressed"`) != 2 || strings.Count(html, "data-entry=") != 5 {
				t.Fatalf("unexpected mixed rows: %s", html)
			}
			for _, r := range ring {
				if strings.Count(stdhtml.UnescapeString(html), r.Text) != 1 {
					t.Fatal("mixed batch lost or duplicated raw mail")
				}
			}
		})
	}
}

func TestConversationBoardNudgeBatchPeerFirstKeepsStoredMail(t *testing.T) {
	const peer = "id:peer"
	const body = "Retained peer mail"
	ring := boardNudgeBatchRing("mail from "+peer+":\n"+body, unblockedNudgeMail("One"), unblockedNudgeMail("Two"))
	ring = append([]wslive.RingRecord{ringRecord(0, wslive.RingEntry{Kind: wslive.RingMail, Direction: wslive.MailRecv, Peer: peer, HistoryKey: "h", Title: body})}, ring...)
	page := PageContent{MailBodies: map[string]string{"h": body}}
	v := historyView(ring, "", page, sessionLookup{})
	entries := v.Conversation.Entries
	if entries[0].MailWakeSuppressed || entries[1].MatchedMailWake() || entries[1].MailWakeSuppressed || entries[1].BoardNudgeSummary() != "" || len(entries[2].BoardNudgeMails) != 2 || !entries[3].MailWakeSuppressed {
		t.Fatalf("multi-message peer pairing changed or headless nudge run missing: %+v", entries)
	}
	html := presentationHTML(t, ring, page, nil)
	if strings.Count(html, `class="mailline recv"`) != 1 || strings.Count(html, `class="mail-notification"`) != 2 || strings.Contains(html, `data-mail-wake="matched"`) || !strings.Contains(html, "Board nudge · Unblocked · 2 messages: One, Two") {
		t.Fatalf("stored peer mail or batch-head peer wake changed: %s", html)
	}
}

func TestConversationBoardNudgeBatchOrderAndBoundaries(t *testing.T) {
	first := "mail from unknown:\nWhiteboard lead nudge (hourly, unblocked).\nOpen topics:\n- Same: waiting\n- First: open"
	second := "mail from unknown:\nWhiteboard lead nudge (overdue, hourly).\nOverdue topics:\n- Same; owner lead; initial/current ETA 10/10 minutes\n- Last; owner lead; initial/current ETA 10/10 minutes"
	ring := boardNudgeBatchRing(first, second)
	v := historyView(ring, "", PageContent{}, sessionLookup{})
	r, topics := v.Conversation.Entries[0].boardNudge()
	if !slices.Equal(r, []string{"hourly", "unblocked", "overdue"}) || !slices.Equal(topics, []string{"Same", "First", "Last"}) {
		t.Fatalf("first-seen order/deduplication lost: %v %v", r, topics)
	}
	html := presentationHTML(t, ring, PageContent{}, nil)
	if !strings.Contains(stdhtml.UnescapeString(html), "Board nudge · Hourly check-in, Unblocked, Overdue · 2 messages: Same, First +1") {
		t.Fatalf("ordered summary missing: %s", html)
	}

	for _, tc := range []struct {
		name string
		edit func([]wslive.RingRecord)
	}{
		{"new wake", func(rs []wslive.RingRecord) {
			rs[1].Text = boardNudgeBatchRing(second)[0].Text
		}},
		{"count exhausted", func(rs []wslive.RingRecord) {
			rs[0].Text = strings.Replace(rs[0].Text, "2 message(s)", "1 message(s)", 1)
		}},
		{"source changed", func(rs []wslive.RingRecord) { rs[1].Source = "other" }},
		{"non-mail wake", func(rs []wslive.RingRecord) { rs[1].Cause = "message" }},
		{"truncated member", func(rs []wslive.RingRecord) { rs[1].Truncated = true }},
		{"missing header", func(rs []wslive.RingRecord) { rs[0].Text = first }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rs := slices.Clone(ring)
			tc.edit(rs)
			v := historyView(rs, "", PageContent{}, sessionLookup{})
			for _, e := range v.Conversation.Entries {
				if len(e.BoardNudgeMails) != 0 || e.MailWakeSuppressed {
					t.Fatalf("collapsed across boundary: %+v", e)
				}
			}
		})
	}
}

func TestConversationBoardNudgeBatchEntryCacheAndWindow(t *testing.T) {
	ring := boardNudgeBatchRing(unblockedNudgeMail("One"), unblockedNudgeMail("Two"), unblockedNudgeMail("Three"))
	cache := newEntryCache()
	var states, keys []string
	for _, count := range []int{1, 2, 3, 2, 1} {
		v := historyView(ring[:count], "", PageContent{}, sessionLookup{})
		e := v.Conversation.Entries[0]
		states = append(states, e.BoardNudgeState())
		keys = append(keys, entryCacheKey(e))
		set, err := renderSessionRegions(v, cache)
		if err != nil {
			t.Fatal(err)
		}
		html := string(set[regionEntries].HTML)
		if count == 1 {
			if strings.Contains(html, "data-mail-wake=") || !strings.Contains(html, "Board nudge · Unblocked: One") {
				t.Fatalf("single row changed: %s", html)
			}
		} else if !strings.Contains(html, fmt.Sprintf("Board nudge · Unblocked · %d messages:", count)) || !strings.Contains(html, `data-mail-wake="`+e.BoardNudgeState()+`"`) {
			t.Fatalf("cached batch size/state stale: %s", html)
		}
	}
	if states[0] != "" || states[1] == states[2] || states[1] != states[3] || keys[0] == keys[1] || keys[1] == keys[2] || keys[1] != keys[3] || cache.buildCount() != 5 {
		t.Fatalf("cache/window state not reversible: states=%v keys=%v builds=%d", states, keys, cache.buildCount())
	}
	// A page starting inside a batch has no proof of shared wake membership.
	v := historyView(ring[1:], "", PageContent{}, sessionLookup{})
	for _, e := range v.Conversation.Entries {
		if e.MailWakeSuppressed || len(e.BoardNudgeMails) > 0 || e.BoardNudgeSummary() == "" {
			t.Fatal("headless page lost individual nudge rows")
		}
	}
}
