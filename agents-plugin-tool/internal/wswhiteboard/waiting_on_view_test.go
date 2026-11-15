package wswhiteboard

import (
	"fmt"
	"strings"
	"testing"
)

func TestWaitingOnViewPopoverReverseAndEscaping(t *testing.T) {
	s := waitingBoard(t)
	stateApply(t, s, `<wb:topic name="blocker" label="Blocker &quot;&lt;&amp;&gt;"><wb:close result="done"/></wb:topic>`, stateNow)
	stateApply(t, s, stateCommand("waiting", `on="topic:blocker ticket:261115-feat-done ticket:261115-bug-dropped ticket:261115-feat-missing"`), stateNow)
	stateApply(t, s, `<wb:topic name="another"><wb:log>wait</wb:log><wb:state value="waiting" on="topic:blocker"/></wb:topic><wb:topic name="not-waiting"><wb:log>work</wb:log><wb:state value="waiting" on="topic:blocker"/><wb:state value="discussing"/></wb:topic><wb:topic name="closed-waiter"><wb:log>done</wb:log><wb:state value="waiting" on="topic:blocker"/><wb:close result="done"/></wb:topic><wb:topic name="Work"><wb:add class="check"><wb:title>Check</wb:title></wb:add></wb:topic>`, stateNow)
	tickets := fakeTickets{"261115-feat-done": TicketDone, "261115-bug-dropped": TicketDropped}
	for _, layout := range layouts() {
		view := BuildView(s, ViewInput{Layout: layout, ReplyID: "r", Tickets: tickets})
		view.Lang = "ko"
		var work, blocker TopicView
		for _, topic := range view.Topics {
			if topic.Name == "Work" {
				work = topic
			}
			if topic.Name == "blocker" {
				blocker = topic
			}
		}
		if work.WaitingReady || len(work.Dependencies) != 5 || !work.Dependencies[4].Derived || len(blocker.Waiters) != 2 {
			t.Fatalf("projection: work %+v, reverse %+v", work, blocker.Waiters)
		}
		for _, out := range stateViewOutputs(t, view) {
			wantAll(t, out, `id="`+topicAnchor("blocker")+`"`, `href="#`+topicAnchor("blocker")+`"`, `href="#`+topicAnchor("Work")+`"`, "이 토픽을 기다리는 곳 2", `data-target="ticket:261115-feat-done" data-status="done"><span class="dot r"`, `data-target="ticket:261115-bug-dropped" data-status="dropped"><span class="dot x"`, `data-target="ticket:261115-feat-missing" data-status="unknown"`, "알 수 없음", "취소됨", `data-target="owner" title="열린 정책 질문 또는 확인 항목에서 파생됨"><span class="dot x"`, `topic:Blocker &#34;&lt;&amp;&gt;`)
			wantNone(t, out, `topic:Blocker "<`, "이 토픽을 기다리는 곳 3")
			wantNone(t, stateTopicTag(t, out, "Work"), "data-waiting-ready")
		}
	}
	stateApply(t, s, `<wb:answer id="1" value="ok">done</wb:answer>`, stateNow)
	tickets["261115-feat-missing"] = TicketDone
	for _, layout := range layouts() {
		view := BuildView(s, ViewInput{Layout: layout, ReplyID: "r", Tickets: tickets})
		view.Lang = "ko"
		for _, out := range stateViewOutputs(t, view) {
			wantAll(t, stateTopicTag(t, out, "Work"), `data-state="waiting"`, `data-waiting-ready="true"`)
			wantAll(t, out, `class="dbtn green"`, `class="sum green"`, `진행 가능 · 모든 대상 해결`, `style="width:100%"`)
		}
	}
	// A ticket reopening changes readiness without a new board entry.
	tickets["261115-feat-missing"] = TicketReady
	for _, topic := range BuildView(s, ViewInput{Tickets: tickets}).Topics {
		if topic.Name == "Work" && topic.WaitingReady {
			t.Fatal("ticket reopened but ready stayed set")
		}
	}
}

func TestWaitingOnViewUnloadedClosedTargetsStayPlain(t *testing.T) {
	s := waitingBoard(t)
	stateApply(t, s, `<wb:topic name="blocker"><wb:close result="done"/></wb:topic>`, stateNow)
	for i := 0; i < ClosedCap; i++ {
		stateApply(t, s, fmt.Sprintf(`<wb:topic name="closed-%d"><wb:log>done</wb:log><wb:close result="done"/></wb:topic>`, i), stateNow)
	}
	stateApply(t, s, stateCommand("waiting", `on="topic:blocker"`), stateNow)
	view := BuildView(s, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	for _, topic := range view.Topics {
		if topic.Name == "Work" {
			if !topic.WaitingReady || topic.Dependencies[0].Anchor != "" {
				t.Fatal("unloaded closed target links", topic.Dependencies)
			}
		}
	}
	out := render(t, view)
	wantAll(t, out, `data-target="topic:blocker"><span class="dot r" aria-hidden="true"></span><span class="nm">topic:Blocker</span>`)
	wantNone(t, out, `href="#`+topicAnchor("blocker")+`"`)
	rest := BuildView(s, ViewInput{Layout: LayoutFull, ReplyID: "r", Closed: &ClosedWindow{From: ClosedCap, N: 1}})
	html, err := RenderClosedTopics(rest)
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, string(html), "Topics waiting on this: 1", `href="#`+topicAnchor("Work")+`"`)
	if strings.Contains(topicAnchor(`a"<&`), `"`) {
		t.Fatal("unsafe anchor")
	}
}
