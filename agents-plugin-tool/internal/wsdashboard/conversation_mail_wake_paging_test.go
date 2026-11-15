package wsdashboard

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestConversationMailWakePreservesPagingAndTurns(t *testing.T) {
	const body = "one delivered mail"
	const text = "[ws mailbox] 1 message(s) arrived for session_key id:self. They were already received through mailbox.recv; do not call it again for these.\n\nmail from peer@machine:\n" + body
	dir := t.TempDir()
	summary := rendererSummary()
	summary.Requests[0].Status = wslive.SummaryComplete
	summary.Requests[0].Parts[0].Status = wslive.SummaryComplete
	history := &wslive.SummaryHistory{V: 1, Commit: strings.Repeat("b", 64), Requests: summary.Requests}
	if err := wslive.AppendRing(dir, []wslive.RingEntry{
		{Kind: wslive.RingUser, Text: "older user"},
		{Kind: wslive.RingMail, Direction: wslive.MailRecv, Peer: "peer@machine", HistoryKey: "h1", Title: body},
		{Kind: wslive.RingTurn, Done: 1, SummaryHistory: history},
		{Kind: wslive.RingWake, Cause: "mail", Text: text},
		{Kind: wslive.RingReply, Text: "after delivery"},
	}); err != nil {
		t.Fatal(err)
	}
	page, err := wslive.ReadRingPage(dir, "", 4)
	if err != nil || len(page.Records) != 4 || page.End {
		t.Fatalf("newest ring page: %+v, %v", page, err)
	}
	v := historyView(page.Records, "", PageContent{MailBodies: map[string]string{"h1": body}}, sessionLookup{})
	cv := v.Conversation
	if len(cv.Entries) != 4 || cv.Mails != 1 || cv.Users != 0 || !cv.Entries[0].MailWakeSuppressed || !cv.Entries[2].MatchedMailWake() || cv.Entries[1].Kind != wslive.RingTurn || cv.Entries[1].Done != 1 {
		t.Fatalf("association disturbed ring indexing, counts or turn: %+v", cv)
	}
	for i, e := range cv.Entries {
		if e.Key != page.Records[i].Name || e.At != page.Records[i].At.UnixMilli() {
			t.Fatalf("entry %d moved from its ring position/time: %+v", i, e)
		}
	}
	regions, err := renderSessionRegions(v, newEntryCache())
	if err != nil {
		t.Fatal(err)
	}
	html := string(regions[regionEntries].HTML)
	summaryKey := `data-key="summary-turn-` + page.Records[1].Name + `"`
	if strings.Count(html, summaryKey) != 1 || strings.Index(html, summaryKey) > strings.Index(html, `class="mail-block"`) {
		t.Fatalf("mail projection moved or duplicated the natural-turn summary: %s", html)
	}
	marker := `<span hidden data-entry="` + page.Before + `" data-mail-wake="suppressed"></span>`
	if !strings.Contains(html, marker) || strings.Count(html, "data-entry=") != 4 || strings.Count(html, `class="mailline recv"`) != 1 || strings.Count(html, `class="mail-block"`) != 1 {
		t.Fatalf("suppression lost the oldest paging key or duplicated mail: %s", html)
	}
	if strings.Index(html, marker) > strings.Index(html, `class="turnline"`) || strings.Index(html, `class="turnline"`) > strings.Index(html, `class="mail-block"`) {
		t.Fatalf("mail did not stay at the wake after the existing turn: %s", html)
	}
	older, err := wslive.ReadRingPage(dir, page.Before, 4)
	if err != nil || !older.End || len(older.Records) != 1 || older.Records[0].Text != "older user" {
		t.Fatalf("hidden mail cursor skipped/repeated history: %+v, %v", older, err)
	}
	past, err := wslive.ReadRingPage(dir, older.Before, 4)
	if err != nil || !past.End || len(past.Records) != 0 || past.Before != "" {
		t.Fatalf("paging did not terminate: %+v, %v", past, err)
	}
}
