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

func stateViewOutputs(t *testing.T, view View) []string {
	t.Helper()
	outputs := []string{render(t, view)}
	if view.Layout == LayoutFull {
		regions, err := RenderRegions(view)
		if err != nil {
			t.Fatal(err)
		}
		outputs = append(outputs, string(regions.Board))
	}
	return outputs
}

func TestTopicStateViewResearchingProjection(t *testing.T) {
	s := stateBoard(t)
	stateApply(t, s, stateCommand(StateResearching, `owner="investigator &quot;&lt;&amp;&gt;&#39;" eta_minutes="7"`), stateNow)
	for name, layout := range layouts() {
		view := BuildView(s, ViewInput{Layout: layout, ReplyID: "r"})
		if len(view.Topics) != 1 || !reflect.DeepEqual(view.Topics[0].State, s.TopicStates()["Work"].State) || view.Topics[0].State.Run != nil || view.Topics[0].StateDiagnostic != nil {
			t.Fatalf("%s: research projection: %+v", name, view.Topics)
		}
	}
	stateApply(t, s, `<wb:topic name="Work"><wb:close result="Done"/></wb:topic>`, stateNow)
	for _, layout := range layouts() {
		view := BuildView(s, ViewInput{Layout: layout})
		if len(view.Topics) != 1 || !view.Topics[0].Closed || view.Topics[0].State != nil {
			t.Fatalf("closed research projection: %+v", view.Topics)
		}
		for _, out := range stateViewOutputs(t, view) {
			wantNone(t, stateTopicTag(t, out, "Work"), "data-state")
			wantNone(t, out, `<span class="topic-state">`, `<span class="state-estimate">`)
		}
	}
	stateApply(t, s, `<wb:topic name="Work"><wb:log>Reopened</wb:log></wb:topic>`, stateNow.Add(time.Hour))
	for _, layout := range layouts() {
		for _, out := range stateViewOutputs(t, BuildView(s, ViewInput{Layout: layout, ReplyID: "r"})) {
			wantNone(t, stateTopicTag(t, out, "Work"), "data-state")
			wantNone(t, out, `<span class="topic-state">`, `<span class="state-estimate">`)
		}
	}
}

func TestTopicStateViewOperationalStates(t *testing.T) {
	for _, value := range []string{StateDiscussing, StateWaiting, StateResearching, StateImplementing, StateAwaitingConfirmation} {
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
			for _, lang := range []string{"en", "ko"} {
				for name, layout := range layouts() {
					view := BuildView(s, ViewInput{Layout: layout, ReplyID: "r"})
					view.Lang = lang
					current := s.TopicStates()["Work"]
					if len(view.Topics) != 1 || !reflect.DeepEqual(view.Topics[0].State, current.State) || view.Topics[0].StateDiagnostic != nil {
						t.Fatalf("%s: projection = %+v", name, view.Topics)
					}
					for _, out := range stateViewOutputs(t, view) {
						labels := map[string]string{
							StateAwaitingConfirmation: "Awaiting confirmation",
							StateWaiting:              "Waiting",
							StateDiscussing:           "Discussing",
							StateImplementing:         "Implementing",
							StateResearching:          "Researching",
						}
						if lang == "ko" {
							labels = map[string]string{
								StateAwaitingConfirmation: "확인 대기",
								StateWaiting:              "대기",
								StateDiscussing:           "논의 중",
								StateImplementing:         "구현 중",
								StateResearching:          "연구 중",
							}
						}
						classes := map[string]string{StateAwaitingConfirmation: "confirm", StateWaiting: "wait", StateDiscussing: "discuss", StateImplementing: "impl", StateResearching: "research"}
						wantAll(t, out, `<span class="topic-state dw"><span class="sb `+classes[value], `tabindex="0" role="img" aria-label="`+labels[value], `<use href="#i-st-`+classes[value]+`"/>`, `<span class="tname" title="Work">Work</span>`)
						wantNone(t, out, `<span class="stb `, `<span class="eta`, `<span class="state-estimate">`, `aria-live=`)
						if value == StateImplementing {
							wantAll(t, out, `<small>/10m</small>`, `<span class="who">executor &#34;&lt;&amp;&gt;&#39;</span>`)
						} else {
							wantAll(t, out, `class="sb `+classes[value]+` solo"`)
						}
						tag := stateTopicTag(t, out, "Work")
						wantAll(t, tag, `data-state="`+value+`"`)
						wantNone(t, tag, "data-state-diagnostic")
						if value != StateImplementing {
							wantNone(t, tag, "data-state-owner", "data-state-eta-minutes", "data-state-started-at", "data-state-deadline-at", "data-state-run")
							continue
						}
						run := current.State.Run
						wantAll(t, tag, `data-state-owner="executor &#34;&lt;&amp;&gt;&#39;"`, `data-state-eta-minutes="10"`,
							`data-state-started-at="`+run.StartedAt+`"`, `data-state-deadline-at="`+run.DeadlineAt+`"`, `data-state-run="`+run.ID+`"`, `data-state-initial-eta-minutes="7"`, `data-state-extensions="1"`)
					}
				}
			}
		})
	}
}

func TestTopicStateViewResearchingBadgeMetadata(t *testing.T) {
	s := stateBoard(t)
	// Successive declarations replace metadata; omission clears each field
	// independently. Every server layout and live-region projection agrees.
	for _, attrs := range []string{
		`owner="&lt;img src=x onerror=attack()&gt;" eta_minutes="7"`,
		`eta_minutes="11"`, `owner="next investigator"`, "",
	} {
		stateApply(t, s, stateCommand(StateResearching, attrs), stateNow)
		current := s.TopicStates()["Work"].State
		for _, lang := range []string{"en", "ko"} {
			for name, layout := range layouts() {
				view := BuildView(s, ViewInput{Layout: layout, ReplyID: "r"})
				view.Lang = lang
				for _, out := range stateViewOutputs(t, view) {
					tag := stateTopicTag(t, out, "Work")
					wantAll(t, tag, `data-state="researching"`)
					wantNone(t, tag, "data-state-run", "data-state-started-at", "data-state-deadline-at")
					wantNone(t, out, `<span class="eta`, `<span class="own"><img`)
					label, estimate := "Researching", fmt.Sprintf("Estimate %d min", current.ETAMinutes)
					if lang == "ko" {
						label, estimate = "연구 중", fmt.Sprintf("예상 %d분", current.ETAMinutes)
					}
					wantAll(t, out, `<span class="topic-state dw"><span class="sb research`, `aria-label="`+label, `<use href="#i-st-research"/>`)
					if current.Owner == "" {
						wantNone(t, tag, "data-state-owner")
						wantNone(t, out, `<span class="who">`)
					} else {
						owner := current.Owner
						if strings.HasPrefix(owner, "<img") {
							owner = "&lt;img src=x onerror=attack()&gt;"
						}
						wantAll(t, tag, `data-state-owner="`+owner+`"`)
						wantAll(t, out, `<span class="who">`+owner+`</span>`)
					}
					if current.ETAMinutes == 0 {
						wantNone(t, tag, "data-state-eta-minutes")
						wantNone(t, out, `<span class="state-estimate">`)
					} else {
						wantAll(t, tag, fmt.Sprintf(`data-state-eta-minutes="%d"`, current.ETAMinutes))
						wantAll(t, out, `<span class="tl">`+estimate+`</span>`, fmt.Sprintf(`<span class="t est"><small>~%dm</small></span>`, current.ETAMinutes))
					}
					if current.Run != nil {
						t.Fatalf("%s: research acquired a run", name)
					}
				}
			}
		}
	}
	stateApply(t, s, stateCommand(StateImplementing, `owner="executor" eta_minutes="3"`), stateNow.Add(time.Hour))
	for _, layout := range layouts() {
		for _, out := range stateViewOutputs(t, BuildView(s, ViewInput{Layout: layout, ReplyID: "r"})) {
			wantNone(t, out, `<span class="state-estimate">`, `<span class="stb research">`)
			wantAll(t, stateTopicTag(t, out, "Work"), `data-state-started-at="`+s.TopicStates()["Work"].State.Run.StartedAt+`"`)
		}
	}
}

func TestTopicStateViewAbsenceAndDiagnostics(t *testing.T) {
	for _, tc := range []struct{ name, raw, code string }{
		{name: "legacy"},
		{name: "malformed", raw: `{"v":1,"op":"set","value":"unknown"}`, code: "malformed"},
		{name: "unsupported", raw: `{"v":2,"op":"set","value":"waiting"}`, code: "unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &State{Topics: []TopicDoc{{Name: "Work", Entries: []Entry{{ID: "e1", Kind: EntryAgent, Text: "Latest human summary", At: stateNow.Format(time.RFC3339Nano)}}}}, EntrySeq: 1}
			// Conversation text cannot supply an operational state.
			stateApply(t, s, `<wb:topic name="Work"><wb:log>implementing; owner executor; ETA 7 minutes</wb:log></wb:topic>`, stateNow)
			if tc.raw != "" {
				stateApply(t, s, stateCommand(StateWaiting, ""), stateNow)
				stateRaw(s, "Work", fmt.Sprintf("e%d", s.EntrySeq+1), tc.raw)
			}
			for _, layout := range layouts() {
				view := BuildView(s, ViewInput{Layout: layout, ReplyID: "r"})
				current := view.Topics[0]
				if current.State != nil || !reflect.DeepEqual(current.StateDiagnostic, s.TopicStates()["Work"].Diagnostic) {
					t.Fatalf("unexpected authority: %+v", current)
				}
				for _, out := range stateViewOutputs(t, view) {
					wantNone(t, out, `<span class="topic-state">`, `<span class="stb `, `<span class="own">`, `<span class="eta`)
					tag := stateTopicTag(t, out, "Work")
					wantNone(t, tag, `data-state=`, "data-state-owner", "data-state-eta-minutes", "data-state-started-at", "data-state-deadline-at", "data-state-run")
					if tc.code == "" {
						wantNone(t, tag, "data-state-diagnostic")
					} else {
						wantAll(t, tag, `data-state-diagnostic="`+tc.code+`"`)
					}
				}
			}
		})
	}
}

func TestTopicStateViewCloseResetAndClosedWindows(t *testing.T) {
	s := &State{}
	for i := 0; i <= ClosedCap; i++ {
		name := fmt.Sprintf("closed-%d", i)
		value := StateImplementing
		if i%2 == 0 {
			value = StateResearching
		}
		stateApply(t, s, `<wb:topic name="`+name+`"><wb:log>Work</wb:log><wb:state value="`+value+`" owner="executor" eta_minutes="7"/><wb:close result="Done"/></wb:topic>`, stateNow)
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
		wantNone(t, out, `<span class="topic-state">`, `<span class="stb `, `<span class="own">`, `<span class="eta`)
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
	wantNone(t, string(out), `<span class="topic-state">`, `<span class="stb `, `<span class="own">`, `<span class="eta`)
}
