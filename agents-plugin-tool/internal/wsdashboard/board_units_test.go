package wsdashboard

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func topicUnitPage(t *testing.T, n int, edited bool) PageContent {
	t.Helper()
	st := &wswhiteboard.State{Seq: 1}
	for i := 0; i < n; i++ {
		text := fmt.Sprintf("Original body %d", i)
		if i == 0 && edited {
			text = "Edited body marker"
		}
		st.Topics = append(st.Topics, wswhiteboard.TopicDoc{Name: fmt.Sprintf("topic-%03d", i), Body: text, Changed: 1, ChangedAt: "2026-11-15T01:00:00Z", Entries: []wswhiteboard.Entry{{ID: fmt.Sprintf("e%d", i+1), Kind: wswhiteboard.EntryAgent, Text: fmt.Sprintf("Original log %d", i), At: "2026-11-15T01:00:00Z"}}})
	}
	if edited {
		st.Seq++
	}
	regions, topics, err := wswhiteboard.RenderRegionsWithTopics(wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: "boardsession"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	return PageContent{HasBoard: true, Seq: st.Seq, Board: regions, BoardUnits: NewBoardUnits(topics)}
}

func TestBoardTopicUnitsReuseAndParity(t *testing.T) {
	page := topicUnitPage(t, 112, false)
	for _, theme := range []string{"", "light", "dark"} {
		view := SessionView{ReplyID: "boardsession", HasBoard: true, Board: page.Board.Board, BoardUnits: page.BoardUnits, Theme: theme}
		regions, err := renderSessionRegions(view, nil)
		if err != nil {
			t.Fatal(err)
		}
		direct := page.BoardUnits.rendered(theme)
		parsed := splitBoardTopics(string(regions[regionBoard].HTML))
		if parsed == nil || len(parsed.order) != 112 || parsed.shell != direct.topics.shell {
			t.Fatal("units differ from legacy parser")
		}
		for _, id := range parsed.order {
			// A wire unit must be the exact section span, with no adjacent
			// whitespace; the client requires exactly one DOM element.
			want, got := parsed.topics[id], direct.topics.topics[id]
			if want != got {
				t.Fatalf("%s differs from parser", id)
			}
		}
		view.BoardUnits = nil
		legacy, err := renderSessionRegions(view, nil)
		if err != nil {
			t.Fatal(err)
		}
		if legacy[regionBoard] != regions[regionBoard] {
			t.Fatal("unit board differs from full region")
		}
		// No board template, hash, theme transformation, or parser work on a
		// non-board wake: the source's immutable board result is reused.
		view.BoardUnits = page.BoardUnits
		view.Label = "non-board update"
		again, err := renderSessionRegions(view, nil)
		if err != nil {
			t.Fatal(err)
		}
		if again[regionBoard] != direct.region || page.BoardUnits.rendered(theme).topics != direct.topics {
			t.Fatal("non-board render rebuilt topic units")
		}
	}
}

func TestBoardTopicUnitsStreams(t *testing.T) {
	f := startPageFixture(t)
	first := topicUnitPage(t, 3, false)
	f.src.setPage("boardsession", first)
	f.d.index.Nudge("boardsession", f.project)
	f.d.index.Settle()
	modern := openSSE(t, f.base+"/s/boardsession/events?regions=board&topics=1")
	old := openSSE(t, f.base+"/s/boardsession/events?regions=board")
	read := func(ch <-chan sseEvent, want string) sseEvent {
		t.Helper()
		events, closed := collect(ch)
		for _, e := range events {
			if e.name == want {
				return e
			}
		}
		t.Fatalf("closed=%t events=%+v, want %s", closed, events, want)
		return sseEvent{}
	}
	read(modern, "region")
	read(old, "region")
	next := topicUnitPage(t, 3, true)
	f.src.setPage("boardsession", next)
	f.d.index.Nudge("boardsession", f.project)
	f.d.index.Settle()
	event := read(modern, "board-topics")
	var patch boardTopicsEvent
	if err := json.Unmarshal([]byte(event.data), &patch); err != nil {
		t.Fatal(err)
	}
	if len(patch.Topics) != 1 || patch.Topics[0].Key != "topic-000" || !strings.Contains(patch.Topics[0].HTML, "Edited body marker") || len(patch.Order) != 3 {
		t.Fatalf("bad unit delta: %+v", patch)
	}
	var full regionEvent
	if err := json.Unmarshal([]byte(read(old, "region").data), &full); err != nil {
		t.Fatal(err)
	}
	if full.Hash != patch.Hash || strings.Count(full.HTML, `<section class="topic`) != 3 {
		t.Fatal("legacy full region differs from unit delta")
	}
	source := f.d.sessionPageSource("boardsession", "en", "")
	shared := f.d.cache.acquire(source.key)
	defer f.d.cache.release(source.key, shared)
	rendered, _ := f.d.cache.get(shared, source, false)
	if rendered.board != next.BoardUnits.rendered("").topics {
		t.Fatal("stream re-tokenized source topic units")
	}
}
