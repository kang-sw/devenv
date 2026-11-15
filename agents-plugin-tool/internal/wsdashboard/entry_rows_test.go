package wsdashboard

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestEntryRowsFullHTMLAndHashes(t *testing.T) {
	ring := []wslive.RingRecord{
		ringRecord(1, wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginTerminal, Text: "question"}),
		ringRecord(2, wslive.RingEntry{Kind: wslive.RingReply, Text: "**reply**"}),
		ringRecord(3, wslive.RingEntry{Kind: wslive.RingMail, Direction: wslive.MailRecv, Peer: "peer@machine", HistoryKey: "h1", Title: "mail"}),
		ringRecord(4, wslive.RingEntry{Kind: wslive.RingTurn, Done: 2}),
		ringRecord(5, wslive.RingEntry{Kind: wslive.RingTools, Tools: &wslive.ToolSummary{Calls: 2}}),
		ringRecord(6, wslive.RingEntry{Kind: wslive.RingCompact, Trigger: "auto"}),
		ringRecord(7, wslive.RingEntry{Kind: wslive.RingCard, Card: &wslive.CardRef{Store: "fixture", Alias: "f1"}}),
		ringRecord(8, wslive.RingEntry{Kind: "notice", Notice: &wslive.Notice{Kind: "model", Text: "diagnostic"}}),
		ringRecord(9, wslive.RingEntry{Kind: wslive.RingCommand, Command: "/fixture", Text: "output"}),
		ringRecord(10, wslive.RingEntry{Kind: wslive.RingUI, UI: &wslive.UICommand{Op: "view", Arg: "board"}}),
		ringRecord(11, wslive.RingEntry{Kind: wslive.RingWhiteboardEffects, Text: "unsupported retained diagnostics"}),
		ringRecord(12, wslive.RingEntry{Kind: wslive.RingWake, Cause: "mail", Text: "unmatched"}),
	}
	for _, lang := range []string{"en", "ko"} {
		t.Run(lang, func(t *testing.T) {
			view := historyView(ring, "", PageContent{MailBodies: map[string]string{"h1": "mail body"}}, sessionLookup{})
			view.Lang = lang
			regions, err := renderSessionRegions(view, newEntryCache())
			if err != nil {
				t.Fatal(err)
			}
			region := regions[regionEntries]
			raw := string(region.HTML)
			rows := splitEntryRows(raw)
			if rows == nil || len(rows.order) != len(ring) {
				t.Fatalf("decomposition: %#v", rows)
			}
			var assembled strings.Builder
			for i, key := range rows.order {
				unit := rows.rows[key]
				if key != ring[i].Name || unit.Key != key || newRegion(unit.HTML).Hash != unit.Hash {
					t.Fatalf("row %d: %+v", i, unit)
				}
				assembled.WriteString(rows.shell[i])
				assembled.WriteString(unit.HTML)
			}
			assembled.WriteString(rows.shell[len(rows.order)])
			if assembled.String() != raw || newRegion(assembled.String()).Hash != region.Hash {
				t.Fatal("full entries HTML/hash changed")
			}
			if !strings.Contains(rows.shell[0], `class="thead"`) || !strings.Contains(rows.shell[0], `class="conv centries"`) {
				t.Fatal("title bar/container left the shell")
			}
		})
	}
}

func TestEntryRowsBoundariesAndFallback(t *testing.T) {
	raw := `<div class="thead">user 1</div><div class="centries"><div data-entry="a"><span data-entry="nested">one</span></div><span hidden data-entry="b"></span></div>`
	rows := splitEntryRows(raw)
	if rows == nil || len(rows.order) != 2 || rows.rows["nested"].Key != "" {
		t.Fatalf("not direct children only: %#v", rows)
	}
	nextRaw := strings.Replace(raw, "one", "two", 1)
	next := splitEntryRows(nextRaw)
	state := &keyedRowState{name: regionEntries, previous: rows, initialized: true}
	name, payload := state.event(newRegion(nextRaw), next)
	event, ok := payload.(keyedRowsEvent)
	if name != "entry-rows" || !ok || len(event.Rows) != 1 || event.Rows[0].Key != "a" || event.Hash != newRegion(nextRaw).Hash {
		t.Fatalf("one complete changed row: %s %#v", name, payload)
	}
	for label, changed := range map[string]string{
		"title":  strings.Replace(raw, "user 1", "user 2", 1),
		"order":  strings.Replace(strings.Replace(raw, `data-entry="a"`, `data-entry="b"`, 1), `hidden data-entry="b"`, `hidden data-entry="a"`, 1),
		"window": strings.Replace(raw, `data-entry="b"`, `data-entry="c"`, 1),
	} {
		if event, _ := state.event(newRegion(changed), splitEntryRows(changed)); event != "region" {
			t.Errorf("%s did not fall back to full", label)
		}
	}
	for _, invalid := range []string{
		`<div data-entry="a"></div>`,
		`<div class="centries"><div></div></div>`,
		`<div class="centries"><div data-entry="a"></div><div data-entry="a"></div></div>`,
		`<div class="centries">unkeyed text</div>`,
		`<div class="centries"><img data-entry="a"></div>`,
		`<div class="centries"></div><div class="centries"></div>`,
		`<div class="centries"><div data-entry="a">`,
	} {
		if splitEntryRows(invalid) != nil {
			t.Errorf("accepted uncertain markup %s", invalid)
		}
	}
	if splitEntryRows(`<div class="thead">empty</div><div class="centries"></div>`) == nil {
		t.Fatal("empty keyed container should be a valid baseline")
	}
	if name, _ := (&keyedRowState{name: regionEntries}).event(newRegion(raw), rows); name != "region" {
		t.Fatal("first send must be full")
	}
	if name, _ := state.event(newRegion(raw), nil); name != "region" {
		t.Fatal("uncertain decomposition must be full")
	}
}
