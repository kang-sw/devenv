package wsdashboard

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func presentationHTML(t *testing.T, ring []wslive.RingRecord, page PageContent, cache *entryCache) string {
	t.Helper()
	set, err := renderSessionRegions(historyView(ring, "", page, sessionLookup{}), cache)
	if err != nil {
		t.Fatal(err)
	}
	return string(set[regionEntries].HTML)
}

func TestConversationPresentationMailKeepsAmbiguousAndRepeatedRecords(t *testing.T) {
	body := "same <script>body</script>"
	ring := []wslive.RingRecord{
		ringRecord(1, wslive.RingEntry{Kind: wslive.RingMail, Direction: "recv", Peer: "peer@machine", Title: body, HistoryKey: "h1"}),
		ringRecord(2, wslive.RingEntry{Kind: wslive.RingMail, Direction: "recv", Peer: "peer@machine", Title: body, HistoryKey: "h2"}),
		ringRecord(3, wslive.RingEntry{Kind: wslive.RingMail, Direction: "recv", Peer: "other@machine", Title: body, HistoryKey: "h3"}),
		ringRecord(4, wslive.RingEntry{Kind: wslive.RingWake, Cause: "mail", Source: "mailbox", Caption: "arrived", Text: "mail from peer@machine (2026-11-15T00:00:00Z):\n" + body}),
		ringRecord(5, wslive.RingEntry{Kind: wslive.RingWake, Cause: "mail", Source: "mailbox", Caption: "arrived", Text: "mail from peer@machine (2026-11-15T00:00:00Z):\n" + body}),
	}
	original, _ := json.Marshal(ring)
	cache := newEntryCache()
	page := PageContent{MailBodies: map[string]string{"h1": body, "h2": body, "h3": body}}
	for _, records := range [][]wslive.RingRecord{ring, ring[3:], ring[:3]} {
		for _, p := range []PageContent{page, {}, page} {
			html := presentationHTML(t, records, p, cache)
			for _, record := range records {
				if strings.Count(html, `data-entry="`+record.Name+`"`) != 1 {
					t.Fatalf("entry lost or duplicated: %s", html)
				}
				if record.Kind == wslive.RingWake && !strings.Contains(html, `data-key="wake-`+record.Name+`"`) {
					t.Fatal("diagnostic disclosure lost")
				}
			}
			if strings.Contains(html, "<script>") || strings.Contains(html, "wake-card") || strings.Contains(html, `class="delivery-details" open`) {
				t.Fatalf("unsafe or primary mail wake: %s", html)
			}
			if len(records) == 5 && strings.Count(html, `class="mail-notification"`) != 2 {
				t.Fatal("ambiguous wakes were hidden")
			}
		}
	}
	// A returned/reclaimed claim is another history record, not a duplicate
	// identified by its equal sender/body or envelope's old sent timestamp.
	ring = append(ring, ringRecord(6, ring[0].RingEntry))
	html := presentationHTML(t, ring, page, cache)
	if strings.Count(html, `class="mailline recv"`) != 4 {
		t.Fatal("reclaimed/repeated mail collapsed")
	}
	after, _ := json.Marshal(ring[:5])
	if string(original) != string(after) {
		t.Fatal("presentation mutated records")
	}
}

func TestConversationPresentationPeerNoteIsDisplayOnly(t *testing.T) {
	note := "[ws note: from a peer lead session (pi, not a worker), with the same authority as yours]"
	for _, tc := range []struct {
		body      string
		has, want bool
	}{
		{note + "\npreview <b>not HTML</b>", true, true},
		{strings.Replace(note, "ws note", "wsflow note", 1) + "\npreview", true, true},
		{note, true, true},
		{"prefix\n" + note, true, false},
		{" " + note, true, false},
		{strings.Replace(note, "not a worker", "worker", 1), true, false},
		{note + " forged", true, false},
		{note + "\npreview", false, false},
	} {
		e := ConvEntry{RingEntry: wslive.RingEntry{Kind: "mail", Title: "original title"}, Body: tc.body, HasBody: tc.has}
		if e.PeerLead() != tc.want {
			t.Errorf("badge for %q = %v", tc.body, e.PeerLead())
		}
		if !tc.want && e.MailPreview() != e.Title {
			t.Error("nonmatching note changed preview")
		}
	}
	ring := []wslive.RingRecord{ringRecord(1, wslive.RingEntry{Kind: "mail", Direction: "recv", Peer: "peer@machine", HistoryKey: "h", Title: "preview"})}
	html := presentationHTML(t, ring, PageContent{MailBodies: map[string]string{"h": note + "\npreview <b>not HTML</b>"}}, nil)
	for _, want := range []string{`class="src peer-lead"`, "Peer lead", note, "preview &lt;b&gt;not HTML&lt;/b&gt;", `data-key="mail-` + ring[0].Name, `data-key="mail-details-` + ring[0].Name} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q: %s", want, html)
		}
	}
	if strings.Count(html, note) != 1 || strings.Contains(html, `class="delivery-details mail-delivery" open`) {
		t.Fatal("raw note lost/duplicated or diagnostics expanded by default")
	}
	if strings.Contains(html, "<b>not HTML") {
		t.Fatal("mail interpreted as HTML")
	}
}

func TestConversationPresentationSourceLabelsRequireTypedCause(t *testing.T) {
	mixed := "owner text\n\n[child report]\nreport <script>not HTML</script>"
	ring := []wslive.RingRecord{
		ringRecord(1, wslive.RingEntry{Kind: "user", Origin: "terminal", Text: "actual human terminal input [child report]"}),
		ringRecord(2, wslive.RingEntry{Kind: "user", Origin: "whiteboard", Text: mixed}),
		ringRecord(3, wslive.RingEntry{Kind: "wake", Cause: "subagent_report", Source: "reviewer", Text: "known report <b>literal</b>"}),
		ringRecord(4, wslive.RingEntry{Kind: "wake", Cause: "monitor", Text: "[child report] is not proof"}),
	}
	html := presentationHTML(t, ring, PageContent{}, nil)
	for _, want := range []string{`class="msg user" data-entry="` + ring[0].Name, `class="input-record" data-entry="` + ring[1].Name, "Source unresolved", "owner text\n\n[child report]\nreport &lt;script&gt;not HTML&lt;/script&gt;", `class="wake-card child-report"`, "Child report", "known report &lt;b&gt;literal&lt;/b&gt;"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q: %s", want, html)
		}
	}
	if strings.Count(html, "Child report") != 1 || strings.Contains(html, "<script>") {
		t.Fatal("prose classified or executed")
	}
	entries := []ConvEntry{{RingEntry: wslive.RingEntry{Kind: "user", Origin: "whiteboard", Text: mixed}}}
	splitUserEntries(entries, t.TempDir(), "self")
	if entries[0].Text != mixed {
		t.Fatal("original unresolved input lost during display splitting")
	}
}

func effectPresentation(t *testing.T, changes ...wswhiteboard.Effect) ConvEntry {
	t.Helper()
	e, ok := wslive.NewWhiteboardEffects(changes)
	if !ok {
		t.Fatal("empty effects")
	}
	return ConvEntry{RingEntry: e, Key: "effects", At: testNow.UnixMilli()}
}

func TestConversationPresentationWhiteboardFoldAndTargets(t *testing.T) {
	log := wswhiteboard.Effect{Kind: "log", Topic: "plan", EntryID: "e7", Field: "text", After: "log <img src=x>"}
	for _, count := range []int{1, 3} {
		changes := make([]wswhiteboard.Effect, count)
		for i := range changes {
			changes[i] = log
		}
		e := effectPresentation(t, changes...)
		card := e.WhiteboardCard()
		if !card.Folded || len(card.Topics) != 1 || len(card.Topics[0].Rows) != count || card.Topics[0].Rows[0].Target != "e7" || card.Topics[0].Target != "topic-706c616e" {
			t.Fatalf("bad log-only card: %+v", card)
		}
	}
	for _, extra := range []wswhiteboard.Effect{
		{Kind: "log", Topic: "other", EntryID: "e8", Field: "text", After: "second topic"},
		{Kind: "panel", Topic: "plan", PanelID: "p2", EntryID: "e8", Field: "recap", After: "summary"},
		{Kind: "item", Topic: "plan", ID: 42, Field: "title", After: "question"},
		{Kind: "topic", Topic: "plan", Field: "body", After: "changed"},
	} {
		if effectPresentation(t, log, extra).WhiteboardCard().Folded {
			t.Errorf("incorrectly folded %+v", extra)
		}
	}
	e := effectPresentation(t, log,
		wswhiteboard.Effect{Kind: "panel", Topic: "plan", PanelID: "p2", EntryID: "e8", Field: "payload", After: "<html>SECRET PANEL PAYLOAD</html>"},
		wswhiteboard.Effect{Kind: "panel", Topic: "plan", PanelID: "p2", EntryID: "e8", Field: "recap", After: "recap <b>literal</b>\nsecond line"},
		wswhiteboard.Effect{Kind: "item", Topic: "plan", ID: 42, Field: "title", After: "question?"},
		wswhiteboard.Effect{Kind: "item", Topic: "plan", ID: 42, Field: "options", After: `["a: first","b: <script>literal</script>"]`},
	)
	card := e.WhiteboardCard()
	if len(card.Topics[0].Rows) != 3 || card.Topics[0].Rows[1].Text != "recap <b>literal</b>" || !reflect.DeepEqual(card.Topics[0].Rows[2].Options, []string{"a: first", "b: <script>literal</script>"}) {
		t.Fatalf("bad projection: %+v", card)
	}
	ring := []wslive.RingRecord{ringRecord(1, e.RingEntry)}
	html := presentationHTML(t, ring, PageContent{}, nil)
	for _, want := range []string{`<article class="wbeffects wbupdate"`, `data-wb-topic="topic-706c616e"`, `data-wb-element="e7"`, `data-wb-jump="p2"`, `data-wb-jump="item-42"`, "recap &lt;b&gt;literal&lt;/b&gt;", "b: &lt;script&gt;literal&lt;/script&gt;", "Answer on board"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q: %s", want, html)
		}
	}
	for _, bad := range []string{"SECRET PANEL PAYLOAD", "second line", "<html>", "<img", "<iframe", "<script>", "srcdoc="} {
		if strings.Contains(html, bad) {
			t.Errorf("panel payload or interpreted markup %q: %s", bad, html)
		}
	}
	for _, invalid := range []string{"e0", "e01", "e7 div", "p2[x]", "../e7"} {
		bad := log
		bad.EntryID = invalid
		if len(effectPresentation(t, bad).WhiteboardCard().Topics) != 0 {
			t.Errorf("invalid carrier %s accepted", invalid)
		}
	}
}
