package wsdashboard

import (
	"encoding/json"
	"html/template"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func topicStreamBoard(a, shell string, reverse bool) wswhiteboard.Regions {
	first := `<section class="topic" data-topic="A" data-th="` + a + `"><p>` + a + `</p></section>`
	second := `<section class="topic" data-topic="B" data-th="b1"><p>unchanged B</p></section>`
	if reverse {
		first, second = second, first
	}
	return wswhiteboard.Regions{Board: template.HTML(first + second + shell)}
}

// Both transports use the same connection-local baseline, while old pages
// keep their full-region protocol. The raw typed regions isolate the wire
// contract from the whiteboard renderer's independent hash tests.
func TestBoardTopicsStreams(t *testing.T) {
	f := startPageFixture(t)
	shell := `<div id="reply-bar">reply</div><button class="closed-more" data-closed-next="2">closed</button>`
	set := func(a, controls string, reverse bool, seq int) {
		p := f.src.page("boardsession")
		p.Seq, p.Board = seq, topicStreamBoard(a, controls, reverse)
		f.src.setPage("boardsession", p)
		if !f.d.index.Nudge("boardsession", f.project) {
			t.Fatal("nudge refused")
		}
		f.d.index.Settle()
	}
	set("a1", shell, false, 2)
	path := "/s/boardsession/events?regions=board&topics=1"
	sse := openSSE(t, f.base+path)
	ws := openWS(t, f.base, path, f.base, true).events
	old := openSSE(t, f.base+"/s/boardsession/events?regions=board")
	boardEvent := func(events <-chan sseEvent) sseEvent {
		t.Helper()
		all, closed := collect(events)
		var boards []sseEvent
		for _, e := range all {
			if e.name == "board-topics" {
				boards = append(boards, e)
			}
			if e.name == "region" {
				var r regionEvent
				if err := json.Unmarshal([]byte(e.data), &r); err != nil {
					t.Fatal(err)
				}
				if r.Name == regionBoard {
					boards = append(boards, e)
				}
			}
		}
		if closed || len(boards) != 1 {
			t.Fatalf("closed=%v, board events=%+v, all=%+v", closed, boards, all)
		}
		return boards[0]
	}
	parity := func(want string) sseEvent {
		t.Helper()
		a, b := boardEvent(sse), boardEvent(ws)
		if a != b || a.name != want {
			t.Fatalf("SSE=%+v WS=%+v, want %s", a, b, want)
		}
		return a
	}
	full := func(e sseEvent) regionEvent {
		t.Helper()
		var r regionEvent
		if e.name != "region" {
			t.Fatalf("want full region, got %+v", e)
		}
		if err := json.Unmarshal([]byte(e.data), &r); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(r.HTML, `data-topic="A"`) || !strings.Contains(r.HTML, `data-topic="B"`) {
			t.Fatalf("not a full board: %+v", r)
		}
		return r
	}
	initial := full(parity("region"))
	full(boardEvent(old))
	// A new connection cannot rely on a board hash supplied by a previous one.
	reconnect := openSSE(t, f.base+path+"&h="+url.QueryEscape("board."+initial.Hash))
	if r := full(boardEvent(reconnect)); r.Hash != initial.Hash {
		t.Fatalf("first h=current send changed hash: %+v", r)
	}
	// Settle initial refresh bookkeeping before measuring the wake's render.
	renders, reads := f.d.stats.SessionRenders.Load(), f.src.readCount("boardsession")
	set("a2", shell, true, 3)
	e := parity("board-topics")
	var patch boardTopicsEvent
	if err := json.Unmarshal([]byte(e.data), &patch); err != nil {
		t.Fatal(err)
	}
	if patch.Hash == initial.Hash || !reflect.DeepEqual(patch.Order, []string{"B", "A"}) || len(patch.Topics) != 1 || patch.Topics[0].Key != "A" || patch.Topics[0].Hash != "a2" || strings.Contains(patch.Topics[0].HTML, "unchanged B") {
		t.Fatalf("not changed-topic-only plus order: %+v", patch)
	}
	full(boardEvent(old))
	if got := f.d.stats.SessionRenders.Load() - renders; got != 1 {
		t.Fatalf("same-session connections rendered %d times, want 1", got)
	}
	if got := f.src.readCount("boardsession") - reads; got != 1 {
		t.Fatalf("same-session connections read %d times, want 1", got)
	}
	// Inspect the shared render too: both acquisitions return the same immutable
	// decomposition, not a per-connection re-split of identical section bytes.
	src := f.d.sessionPageSource("boardsession", "en", "")
	a, b := f.d.cache.acquire(src.key), f.d.cache.acquire(src.key)
	defer f.d.cache.release(src.key, a)
	defer f.d.cache.release(src.key, b)
	ra, _ := f.d.cache.get(a, src, false)
	rb, _ := f.d.cache.get(b, src, false)
	if ra.board == nil || ra.board != rb.board {
		t.Fatal("connections did not share topic decomposition identity")
	}
	// Reply-bar and closed-control changes independently require the full shell.
	set("a2", strings.Replace(shell, "reply</div>", "new reply</div>", 1), true, 4)
	full(parity("region"))
	full(boardEvent(old))
	set("a2", strings.Replace(shell, `data-closed-next="2"`, `data-closed-next="3"`, 1), true, 5)
	last := full(parity("region"))
	full(boardEvent(old))
	rews := openWS(t, f.base, path+"&h="+url.QueryEscape("board."+last.Hash), f.base, true)
	full(boardEvent(rews.events))
}
