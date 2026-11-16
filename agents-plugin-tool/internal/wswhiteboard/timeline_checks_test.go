package wswhiteboard

import (
	"fmt"
	"strings"
	"testing"
)

func TestTimelineChecksUseQuestionAnchorsAtAddPosition(t *testing.T) {
	for _, tc := range []struct {
		name, answer, chip, reading string
		assumed                     bool
	}{
		{name: "open", chip: "open"},
		{name: "ok", answer: "ok", chip: "ok"},
		{name: "no", answer: "no, use A", chip: "no", reading: "use A"},
		{name: "assumed", answer: "ok", chip: "ok", assumed: true},
		{name: "free text", answer: "looks right", chip: "answered", reading: "looks right"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &State{}
			mustApply(t, st, `<wb:topic name="t"><wb:log>Before check</wb:log><wb:add class="check"><wb:title>held</wb:title></wb:add><wb:log>After check</wb:log></wb:topic>`)
			var added Entry
			var beforeID, afterID string
			for _, e := range st.Topics[0].Entries {
				if e.ItemID == 1 && strings.HasPrefix(e.Text, "Item {{1}} added [check]") {
					added = e
				}
				if e.Text == "Before check" {
					beforeID = e.ID
				}
				if e.Text == "After check" {
					afterID = e.ID
				}
			}
			for i := 0; i < 60; i++ {
				mustApply(t, st, fmt.Sprintf(`<wb:topic name="t"><wb:log>later %d</wb:log></wb:topic>`, i))
			}
			if tc.answer != "" {
				mustApply(t, st, fmt.Sprintf(`<wb:answer id="1" value="%s" assumed="%t">owner words</wb:answer>`, tc.answer, tc.assumed))
			}
			view := BuildView(st, ViewInput{Layout: LayoutFull})
			open := tc.answer == ""
			// Checks, open or closed, never anchor the history: their ask
			// lines fold like logs.
			held := allTimelineRows(view.Topics[0])
			asks := 0
			for _, row := range held {
				if row.Ask {
					asks++
					if row.Key != added.ID || row.Item.ID != 1 || row.Page.Runs[0].Entry.ID != added.ID {
						t.Fatalf("check not at its add event: %+v", row)
					}
				}
			}
			live, _ := timelineRows(view.Topics[0])
			for _, row := range live {
				if row.Ask {
					t.Fatalf("check ask line anchored in the live history: %+v", row)
				}
			}
			if asks != 1 || len(held) < 50 {
				t.Fatalf("check anchor/paging: asks=%d held=%d", asks, len(held))
			}
			out := markup(render(t, view))
			start := strings.Index(out, `class="srow wb-hitem check`)
			if start < 0 {
				t.Fatal("check lacks the shared question row")
			}
			end := strings.Index(out[start:], `</summary>`)
			if open {
				// An open check's history row is its ask-time line alone.
				end = strings.Index(out[start:], `</article>`)
			}
			if end < 0 {
				t.Fatal("check lacks the shared question summary")
			}
			summary := out[start : start+end]
			wantAll(t, summary, `<span class="stext wb-htext">`, `<span class="id">(01)</span>`, `<b>held</b>`, `<span class="pick"`, `<span class="pt">`+tc.chip+`</span></span>`)
			if open {
				wantNone(t, summary, `id="item-1"`, `<details`, `ckrow`)
			} else {
				wantAll(t, summary, `id="item-1"`, `class="sx" data-key="question:1"`, `<summary><span class="stext wb-htext">`)
			}
			if tc.assumed {
				wantAll(t, summary, `data-key="question:1" open`)
			} else {
				wantNone(t, summary, `data-key="question:1" open`)
			}
			if tc.reading != "" {
				wantAll(t, out, `class="ans"`, `>`+tc.reading+`</div>`)
			}
			controls := 0
			if tc.assumed {
				controls = 1
				wantAll(t, out, `<div class="sbody"><div class="checks"><div class="ckrow`, `id="check-1"`)
			}
			if strings.Count(out, `id="item-1"`) != 1 || strings.Count(out, `id="check-1"`) != controls {
				t.Fatal("check anchor or real controls duplicated")
			}
			before, event, after := strings.Index(out, `id="`+beforeID+`"`), strings.Index(out, `id="`+added.ID+`"`), strings.Index(out, `id="`+afterID+`"`)
			if !(before >= 0 && before < event && event < start && start < after) {
				t.Fatalf("check moved from its event: before=%d event=%d check=%d after=%d", before, event, start, after)
			}
			stack := strings.Index(out, `<div class="tbody wb-asks">`)
			if !open {
				if stack >= 0 {
					t.Fatal("a settled check left an open-ask stack")
				}
				return
			}
			card := strings.Index(out, `<div class="ckrow" id="item-1" data-id="01">`)
			if stack < after || card < stack {
				t.Fatalf("open check card not stacked after the history: after=%d stack=%d card=%d", after, stack, card)
			}
			wantAll(t, out[card:], `<a class="wb-askjump" href="#`+added.ID+`"`)
		})
	}
}
