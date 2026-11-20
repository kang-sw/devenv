package wswhiteboard

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
)

func TestTopicUnitsUnfamiliarLayoutFallsBack(t *testing.T) {
	unit := TopicUnit{ID: "A", HTML: `<section class="topic" data-topic="A"></section>`, after: "\n"}
	topics := []TopicView{{Name: "A"}}
	rendered := map[string]TopicUnit{"A": unit}
	for _, board := range []template.HTML{"", "<div>prefix</div>" + unit.HTML + unit.after, unit.HTML} {
		if collectTopicRegions(board, topics, rendered) != nil {
			t.Fatal("unfamiliar board layout supplied units")
		}
	}
	board := unit.HTML + unit.after + `<div>controls</div>`
	if got := collectTopicRegions(board, topics, rendered); got == nil || got.Board != board || got.Shell != "\n<div>controls</div>" {
		t.Fatal("known board layout lost bytes")
	}
}

func TestTopicUnitsMatchFullRender(t *testing.T) {
	st := streamFixture()
	// Include a capped topic, cross-topic item/panel links, and a body frame.
	mustApply(t, st, `<wb:topic name="units"><wb:body>Body {{1}} and {{p1}}.</wb:body><wb:log>Latest log</wb:log></wb:topic>`)
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "fixture"})
	if len(view.Topics) < 2 {
		t.Fatal("fixture needs topics")
	}
	view.DeferredTopics = append(view.DeferredTopics, view.Topics[len(view.Topics)-1])
	view.Topics = view.Topics[:len(view.Topics)-1]
	view.ClosedMore, view.ClosedNext = 1, 30
	assets, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"en", "ko"} {
		view.Lang = lang
		eager := newPageData(view, assets)
		var board, panels bytes.Buffer
		if err := eager.tmpl.ExecuteTemplate(&board, "board", eager); err != nil {
			t.Fatal(err)
		}
		for _, panel := range view.Panels {
			if err := eager.tmpl.ExecuteTemplate(&panels, "panel", eager.PanelOf(panel)); err != nil {
				t.Fatal(err)
			}
		}
		type cached struct {
			revision string
			unit     TopicUnit
		}
		memo := map[string]cached{}
		renders := 0
		reuse := func(id, revision string, render func() (TopicUnit, error)) (TopicUnit, error) {
			if old, ok := memo[id]; ok && old.revision == revision {
				return old.unit, nil
			}
			renders++
			unit, err := render()
			memo[id] = cached{revision, unit}
			return unit, err
		}
		regions, units, err := RenderRegionsWithTopics(view, reuse)
		if err != nil {
			t.Fatal(err)
		}
		if string(regions.Board) != board.String() || string(regions.Panels) != panels.String() || regions.Mermaid != eager.Mermaid {
			t.Fatal("unit render differs from eager full render")
		}
		for _, unit := range units.Topics {
			if strings.TrimSpace(string(unit.HTML)) != string(unit.HTML) {
				t.Fatal("topic unit includes whitespace outside its section")
			}
			if unit.Hash != topicHTMLHash(unit.HTML) {
				t.Fatal("unit hash differs from data-th")
			}
		}
		if string(units.Board) != board.String() || !strings.Contains(string(units.Shell), "data-history-topic=") {
			t.Fatal("unit assembly lost shell/deferred topics")
		}
		before := renders
		again, _, err := RenderRegionsWithTopics(view, reuse)
		if err != nil {
			t.Fatal(err)
		}
		if again != regions || renders != before {
			t.Fatal("unchanged view rendered topics or lost output")
		}
		// A reference's known/on-page state is shared render input, not part
		// of the authored topic itself. A cache must never keep its old link.
		view.Stored[99999] = !view.Stored[99999]
		if _, _, err := RenderRegionsWithTopics(view, reuse); err != nil {
			t.Fatal(err)
		}
		if renders == before {
			t.Fatal("reference membership did not invalidate topics")
		}
	}
}
