package wswhiteboard

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func stateTopicTag(t *testing.T, out, topic string) string {
	t.Helper()
	for _, part := range strings.Split(out, `<section `)[1:] {
		tag := strings.SplitN(part, ">", 2)[0]
		if strings.Contains(tag, `data-topic="`+topic+`"`) {
			return tag
		}
	}
	t.Fatalf("no topic %q in rendered output", topic)
	return ""
}

func TestTopicStateViewOperationalStates(t *testing.T) {
	for _, value := range topicStateValues {
		t.Run(value, func(t *testing.T) {
			s := stateBoard(t)
			attrs := ""
			if value == StateImplementing {
				attrs = `owner="executor &quot;&lt;&amp;&gt;&#39;" eta_minutes="7"`
			}
			stateApply(t, s, stateCommand(value, attrs), stateNow)
			if value == StateImplementing {
				stateApply(t, s, `<wb:topic name="Work"><wb:eta minutes="10" reason="more work"/></wb:topic>`, stateNow.Add(time.Minute))
			}
			for name, layout := range layouts() {
				view := BuildView(s, ViewInput{Layout: layout, ReplyID: "r"})
				current := s.TopicStates()["Work"]
				if len(view.Topics) != 1 || !reflect.DeepEqual(view.Topics[0].State, current.State) || view.Topics[0].StateDiagnostic != nil {
					t.Fatalf("%s: projection = %+v", name, view.Topics)
				}
				outputs := []string{render(t, view)}
				if layout == LayoutFull {
					regions, err := RenderRegions(view)
					if err != nil {
						t.Fatal(err)
					}
					outputs = append(outputs, string(regions.Board))
				}
				for _, out := range outputs {
					tag := stateTopicTag(t, out, "Work")
					wantAll(t, tag, `data-state="`+value+`"`)
					wantNone(t, tag, "data-state-diagnostic")
					if value != StateImplementing {
						wantNone(t, tag, "data-state-owner", "data-state-eta-minutes", "data-state-started-at", "data-state-deadline-at", "data-state-run")
						continue
					}
					run := current.State.Run
					wantAll(t, tag, `data-state-owner="executor &#34;&lt;&amp;&gt;&#39;"`, `data-state-eta-minutes="10"`,
						`data-state-started-at="`+run.StartedAt+`"`, `data-state-deadline-at="`+run.DeadlineAt+`"`, `data-state-run="`+run.ID+`"`)
				}
			}
		})
	}
}

func TestTopicStateViewAbsenceAndDiagnostics(t *testing.T) {
	for _, tc := range []struct{ name, raw, code string }{
		{name: "legacy"},
		{name: "malformed", raw: `{"v":1,"op":"set","value":"unknown"}`, code: "malformed"},
		{name: "unsupported", raw: `{"v":2,"op":"set","value":"waiting"}`, code: "unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := stateBoard(t)
			// Conversation text cannot supply an operational state.
			stateApply(t, s, `<wb:topic name="Work"><wb:log>implementing; owner executor; ETA 7 minutes</wb:log></wb:topic>`, stateNow)
			if tc.raw != "" {
				stateApply(t, s, stateCommand(StateWaiting, ""), stateNow)
				stateRaw(s, "Work", fmt.Sprintf("e%d", s.EntrySeq+1), tc.raw)
			}
			view := BuildView(s, ViewInput{Layout: LayoutFull, ReplyID: "r"})
			current := view.Topics[0]
			if current.State != nil || !reflect.DeepEqual(current.StateDiagnostic, s.TopicStates()["Work"].Diagnostic) {
				t.Fatalf("unexpected authority: %+v", current)
			}
			tag := stateTopicTag(t, render(t, view), "Work")
			wantNone(t, tag, `data-state=`, "data-state-owner", "data-state-eta-minutes", "data-state-started-at", "data-state-deadline-at", "data-state-run")
			if tc.code == "" {
				wantNone(t, tag, "data-state-diagnostic")
			} else {
				wantAll(t, tag, `data-state-diagnostic="`+tc.code+`"`)
			}
		})
	}
}

func TestTopicStateViewCloseResetAndClosedWindows(t *testing.T) {
	s := &State{}
	for i := 0; i <= ClosedCap; i++ {
		name := fmt.Sprintf("closed-%d", i)
		stateApply(t, s, `<wb:topic name="`+name+`"><wb:log>Work</wb:log><wb:state value="implementing" owner="executor" eta_minutes="7"/><wb:close result="Done"/></wb:topic>`, stateNow)
	}
	view := BuildView(s, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	if len(view.Topics) != ClosedCap || len(view.DeferredTopics) != 1 || view.ClosedMore != 1 {
		t.Fatalf("closed cap: shown %d deferred %d more %d", len(view.Topics), len(view.DeferredTopics), view.ClosedMore)
	}
	for _, topics := range [][]TopicView{view.Topics, view.DeferredTopics} {
		for _, topic := range topics {
			if !topic.Closed || topic.State != nil || topic.StateDiagnostic != nil {
				t.Fatalf("close retained authority: %+v", topic)
			}
		}
	}
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range []string{render(t, view), string(regions.Board)} {
		wantAll(t, out, `<template data-history-topic="closed-0">`)
		for i := 0; i <= ClosedCap; i++ {
			tag := stateTopicTag(t, out, fmt.Sprintf("closed-%d", i))
			wantAll(t, tag, " closed")
			wantNone(t, tag, "data-state")
		}
	}
	rest := BuildView(s, ViewInput{Layout: LayoutFull, ReplyID: "r", Closed: &ClosedWindow{From: ClosedCap, N: 1}})
	if len(rest.Topics) != 1 || rest.Topics[0].Name != "closed-0" || len(rest.DeferredTopics) != 0 {
		t.Fatalf("closed window: %+v", rest.Topics)
	}
	out, err := RenderClosedTopics(rest)
	if err != nil {
		t.Fatal(err)
	}
	wantNone(t, stateTopicTag(t, string(out), "closed-0"), "data-state")
}
