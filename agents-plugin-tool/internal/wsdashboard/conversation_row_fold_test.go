package wsdashboard

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestChildReportFoldedMailboxRow(t *testing.T) {
	for _, tc := range []struct {
		name, caption, text string
		truncated           bool
	}{
		{"caption only", `This agent's report was delivered from "worker" <message>`, "", false},
		{"body only", "", "report <script>literal</script>", false},
		{"caption and truncated body", "summary <finding>", "report <script>literal</script>", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ring := []wslive.RingRecord{ringRecord(1, wslive.RingEntry{
				Kind: wslive.RingWake, Cause: "subagent_report", Source: "Triage idea tickets part 00",
				Caption: tc.caption, Text: tc.text, Truncated: tc.truncated,
			})}
			html := presentationHTML(t, ring, PageContent{}, newEntryCache())
			start := `<details class="mailline recv child-report" data-entry="` + ring[0].Name + `" data-key="wake-` + ring[0].Name + `">`
			_, row, ok := strings.Cut(html, start)
			if !ok {
				t.Fatalf("missing closed, stably keyed mailbox disclosure: %s", html)
			}
			summary, body, ok := strings.Cut(row, "</summary>")
			if !ok || !strings.Contains(summary, `<span class="arr">←</span><b class="mfirst">Triage idea tickets part 00</b>`) || !strings.Contains(summary, `<time class="t"`) {
				t.Fatalf("folded report must show only arrow, title and clock: %s", html)
			}
			if strings.Contains(summary, "wake-caption") || strings.Contains(summary, "wake-body") || strings.Contains(summary, "mbody") || strings.Contains(html, "mail-icon") || strings.Contains(html, "wake-card") || strings.Contains(html, "data-mail-wake=") {
				t.Fatalf("report leaked body, mail icon or mail association into folded row: %s", html)
			}
			if !strings.HasPrefix(body, `<div class="mbody">`) || !strings.Contains(body, "</div></details>") {
				t.Fatalf("report body must remain inside the disclosure: %s", html)
			}
			if tc.caption != "" && !strings.Contains(body, `<span class="wake-caption">`) {
				t.Fatalf("caption missing from expanded body: %s", html)
			}
			if tc.text != "" && !strings.Contains(body, `<pre class="wake-body">report &lt;script&gt;literal&lt;/script&gt;`) {
				t.Fatalf("escaped report missing from expanded body: %s", html)
			}
			if strings.Contains(html, "<script>") || strings.Contains(html, "<finding>") || strings.Contains(html, "<message>") || (tc.truncated && !strings.Contains(body, " …</pre>")) {
				t.Fatalf("unsafe report text or missing truncation indicator: %s", html)
			}
		})
	}
}

func TestConversationMailboxIconOnlyOnInboundRows(t *testing.T) {
	for _, direction := range []string{wslive.MailRecv, wslive.MailSent} {
		t.Run(direction, func(t *testing.T) {
			ring := []wslive.RingRecord{ringRecord(1, wslive.RingEntry{
				Kind: wslive.RingMail, Direction: direction, Peer: "peer@machine", Title: "preview", HistoryKey: "h1",
			})}
			for _, page := range []PageContent{{}, {MailBodies: map[string]string{"h1": "original body"}}} {
				html := presentationHTML(t, ring, page, nil)
				if direction == wslive.MailRecv {
					if strings.Count(html, `class="i mail-icon"`) != 1 || !strings.Contains(html, `<span class="arr">←</span><svg class="i mail-icon"`) || !strings.Contains(html, `aria-hidden="true"`) {
						t.Fatalf("inbound icon missing or not immediately after the arrow: %s", html)
					}
				} else if strings.Contains(html, "mail-icon") || !strings.Contains(html, `<span class="arr">→</span>`) {
					t.Fatalf("outbound mailbox row changed: %s", html)
				}
			}
		})
	}
}
