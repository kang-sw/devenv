package wswhiteboard

import (
	"fmt"
	"testing"
)

func foldHistory(t *testing.T, openQuestion bool) TopicView {
	t.Helper()
	st := &State{}
	logs := func(n int, tag string) {
		for i := 0; i < n; i++ {
			mustApply(t, st, fmt.Sprintf(`<wb:topic name="t"><wb:log>%s %d</wb:log></wb:topic>`, tag, i))
		}
	}
	logs(10, "a")
	mustApply(t, st, `<wb:topic name="t"><wb:add class="policy"><wb:title>closed one</wb:title><wb:option>A</wb:option><wb:option>B</wb:option></wb:add></wb:topic>`)
	logs(60, "b")
	mustApply(t, st, `<wb:answer id="1" value="a">first</wb:answer>`)
	logs(60, "c")
	if openQuestion {
		mustApply(t, st, `<wb:topic name="t"><wb:add class="technical"><wb:title>still open</wb:title></wb:add></wb:topic>`)
		logs(60, "d")
	}
	mustApply(t, st, `<wb:topic name="t"><wb:add class="technical"><wb:title>closed two</wb:title></wb:add></wb:topic>`)
	logs(60, "e")
	last := 2
	if openQuestion {
		last = 3
	}
	mustApply(t, st, fmt.Sprintf(`<wb:answer id="%d" value="ok">second</wb:answer>`, last))
	logs(60, "f")
	return BuildView(st, ViewInput{Layout: LayoutFull}).Topics[0]
}

func TestTimelineClosedQuestionsFoldLikeLogs(t *testing.T) {
	tv := foldHistory(t, true)
	for i, p := range tv.Timeline {
		if p.Gap {
			continue
		}
		for _, row := range p.Rows {
			if row.Item != nil && row.Item.Status != StatusOpen {
				t.Fatalf("closed question item %d anchored in live part %d", row.Item.ID, i)
			}
			if row.Answer {
				t.Fatalf("answer row %s anchored in live part %d", row.Key, i)
			}
		}
	}
}

// An open question no longer splits the history: it folds like any log and
// its card stacks after the history, so the fold is the same single top gap
// as without it.
func TestTimelineOpenQuestionDoesNotSplitTheFold(t *testing.T) {
	tv := foldHistory(t, true)
	gaps := 0
	for i, p := range tv.Timeline {
		if p.Gap {
			gaps++
			if i != 0 {
				t.Fatalf("fold at part %d, want only the top", i)
			}
		}
	}
	if gaps != 1 || len(tv.Timeline) != 2 {
		t.Fatalf("parts=%d gaps=%d, want one top fold plus the live tail", len(tv.Timeline), gaps)
	}
	if len(tv.Asks) != 1 || len(tv.Asks[0].Asks) != 1 || tv.Asks[0].Asks[0].Item.Title != "still open" || tv.Asks[0].Asks[0].Entry == "" {
		t.Fatalf("open asks: %+v", tv.Asks)
	}
}

func TestTimelineWithoutOpenQuestionHasOneTopFold(t *testing.T) {
	tv := foldHistory(t, false)
	gaps := 0
	for i, p := range tv.Timeline {
		if p.Gap {
			gaps++
			if i != 0 {
				t.Fatalf("fold at part %d, want only the top", i)
			}
		}
	}
	if gaps != 1 || len(tv.Timeline) != 2 {
		t.Fatalf("parts=%d gaps=%d, want one top fold plus the live tail", len(tv.Timeline), gaps)
	}
}
