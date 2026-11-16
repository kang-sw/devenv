package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestTimelineCoalescedCreationKeepsEveryQuestionAndProvenance(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="t"><wb:log>preceding context</wb:log></wb:topic>`)
	mustApply(t, st, `<wb:topic name="t"><wb:add class="technical"><wb:title>first</wb:title></wb:add><wb:add class="check"><wb:title>second</wb:title></wb:add><wb:add class="policy"><wb:title>third</wb:title><wb:option>A</wb:option><wb:option>B</wb:option></wb:add></wb:topic>`)
	var batch Entry
	for _, e := range st.Topics[0].Entries {
		if strings.HasPrefix(e.Text, "Items added: ") {
			batch = e
		}
	}
	if batch.ID == "" || batch.ItemID != 0 {
		t.Fatal("fixture must retain TASK3's coalesced durable event")
	}
	// A current class patch cannot move a question out of its recorded batch.
	mustApply(t, st, `<wb:patch id="1" class="announced"/>`)
	for i := 0; i < 60; i++ {
		mustApply(t, st, fmt.Sprintf(`<wb:topic name="t"><wb:log>later %d</wb:log></wb:topic>`, i))
	}
	before := string(mustMarshal(t, st))
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	_, held := timelineRows(view.Topics[0])
	var found bool
	for _, row := range held {
		if row.Key != batch.ID {
			continue
		}
		found = true
		if len(row.Created) != 3 || row.Created[0].Item.ID != 1 || row.Created[1].Item.ID != 2 || row.Created[2].Item.ID != 3 {
			t.Fatalf("coalesced questions lost or reordered: %+v", row.Created)
		}
	}
	if !found {
		t.Fatal("open questions no longer anchor: their coalesced batch folds with the rest of the history")
	}
	var stacked []int
	for _, g := range view.Topics[0].Asks {
		for _, a := range g.Asks {
			if a.Entry != batch.ID {
				t.Fatalf("stacked ask %d jumps to %q, want the batch %q", a.Item.ID, a.Entry, batch.ID)
			}
			stacked = append(stacked, a.Item.ID)
		}
	}
	if fmt.Sprint(stacked) != "[1 2 3]" {
		t.Fatalf("open questions stack in creation order, got %v", stacked)
	}
	out := markup(render(t, view))
	if strings.Count(out, `id="`+batch.ID+`"`) != 1 {
		t.Fatal("a coalesced event still has exactly one navigation anchor")
	}
	if got := strings.Count(out, `class="wb-askjump" href="#`+batch.ID+`"`); got != 3 {
		t.Fatalf("every stacked card jumps to the batch line, got %d jumps", got)
	}
	for _, id := range []int{1, 2, 3} {
		if strings.Count(out, fmt.Sprintf(`id="item-%d"`, id)) != 1 {
			t.Fatalf("item %d needs one permanent anchor", id)
		}
		key := fmt.Sprintf("ask:%d", id)
		if id == 1 {
			key = batch.ID // Retain the durable event's navigation target.
		}
		wantAll(t, out, fmt.Sprintf(`data-entry="%s" data-entry-item="%d" data-history-item="%d"`, key, id, id))
	}
	if before != string(mustMarshal(t, st)) {
		t.Fatal("timeline projection rewrote coalesced history")
	}
}

func TestTimelineEventlessItemsRetainStoredNineTwoOrder(t *testing.T) {
	st := &State{Items: []Item{
		{ID: 9, Topic: "legacy", Class: ClassTechnical, Title: "first", Status: StatusAnswered, Answer: "ok"},
		{ID: 2, Topic: "legacy", Class: ClassCheck, Title: "second", Status: StatusAssumed, Answer: "ok"},
	}}
	topic := topicView(t, st, LayoutFull, "legacy")
	live := allTimelineRows(topic)
	if len(live) != 2 || live[0].Item.ID != 9 || live[1].Item.ID != 2 || live[0].Clock != "" || live[1].Clock != "" {
		t.Fatalf("eventless timeline must keep undated stored order: %+v", live)
	}
	out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull})))
	if strings.Index(out, `id="item-9"`) >= strings.Index(out, `id="item-2"`) {
		t.Fatal("render reordered the stored [9, 2] prefix")
	}
}

func TestTimelineOwnerAnswerUsesSendClockThroughoutHistoryChip(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassPolicy, "t", "pick", "A", "B"))
	mustApply(t, st, `<wb:answer id="1" value="b">owner words</wb:answer>`)
	sent := time.Date(2026, 11, 15, 12, 29, 0, 0, time.UTC)
	st.Items[0].AnsweredAt = "2026-11-15T12:30:00Z"
	for i := range st.Topics[0].Entries {
		e := &st.Topics[0].Entries[i]
		if (EntryView{Entry: *e}).OwnerAnswer() {
			e.AnswerSentAt = sent.UnixMilli()
		}
	}
	view := BuildView(st, ViewInput{Layout: LayoutFull, Location: time.UTC})
	live, _ := timelineRows(view.Topics[0])
	for _, row := range live {
		if !row.Answer {
			continue
		}
		e := row.Page.Runs[0].Entry
		if row.Clock != "12:29" || e.Clock != "12:29" || e.Ms != sent.UnixMilli() || e.Stamp != "2026-11-15 12:29" {
			t.Fatalf("history chip/hover diverged from owner send time: %+v", row)
		}
		out := regexpHistoryArticle(markup(render(t, view)), row.Key)
		wantAll(t, out, `class="wb-hstatus"`, `datetime="2026-11-15T12:29:00Z"`, "owner words")
		wantNone(t, out, "2026-11-15T12:30:00Z")
		return
	}
	t.Fatal("owner answer row missing")
}

func TestTimelineRepublishedPanelRetainsOriginalInputIteration(t *testing.T) {
	stamp := json.RawMessage(`[{"inbox_id":"owner-1","sent_at":1000,"excerpt":"Original owner request","chip_ids":[]}]`)
	st := &State{Seq: 3, Topics: []TopicDoc{{Name: "t", Entries: []Entry{
		{ID: "e1", Kind: EntryPanel, PanelID: "p1", Recap: "original", Text: "original body", At: "2026-11-15T12:00:00Z", Seq: 1, Inputs: stamp},
		{ID: "e2", Kind: EntryAgent, Text: "intervening", At: "2026-11-15T12:01:00Z", Seq: 2},
		{ID: "e3", Kind: EntryPanel, PanelID: "p1", Recap: "latest", Text: "latest body", At: "2026-11-15T12:02:00Z", Seq: 3},
	}}}}
	view := BuildView(st, ViewInput{Layout: LayoutFull})
	live := allTimelineRows(view.Topics[0])
	if len(live) < 2 || len(live[0].Inputs) != 1 || live[0].Inputs[0].Excerpt != "Original owner request" {
		t.Fatal("republishing erased the original owner iteration", live)
	}
	out := markup(render(t, view))
	if strings.Count(out, `id="e1"`) != 1 || strings.Count(out, `id="p1"`) != 1 || strings.Count(out, "Original owner request") != 1 {
		t.Fatal("headers must not duplicate panel/version identities")
	}
	if strings.Index(out, "Original owner request") > strings.Index(out, `id="e2"`) {
		t.Fatal("owner iteration moved to the republished panel")
	}
	wantAll(t, out, `class="wb-entry panel historical" id="e1"`, "original body", "latest body")
}
