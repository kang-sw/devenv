package wswhiteboard

import (
	"strings"
	"testing"
	"time"
)

func TestTopicStateCompactTemplateTimes(t *testing.T) {
	now := time.Now().UTC().Add(-30 * time.Second)
	run := &TopicRun{ID: "e1", StartedAt: now.Add(-5 * time.Minute).Format(time.RFC3339Nano), Owner: `executor <&>`, InitialETAMinutes: 10, ETAMinutes: 20, ExtensionCount: 1, DeadlineAt: now.Add(15 * time.Minute).Format(time.RFC3339Nano)}
	for _, layout := range layouts() {
		for _, tc := range []struct {
			name  string
			state *TopicState
			want  []string
		}{
			{"eta", &TopicState{Value: StateImplementing, Run: run}, []string{`class="wb-state-pill impl"`, `<span class="wb-state-time"><b>5</b><small>/20m</small></span>`, `ETA 20분 중 5분 경과`, `연장 1회 · 최초 10분`, `<span class="who">executor &lt;&amp;&gt;</span>`}},
			{"no-run", &TopicState{Value: StateImplementing}, []string{`class="wb-state-pill impl solo"`, `aria-label="구현 중"`}},
			{"estimate", &TopicState{Value: StateResearching, ETAMinutes: 30}, []string{`class="wb-state-pill research"`, `<span class="wb-state-time est"><small>~30m</small></span>`, `aria-label="연구 중 · 예상 30분"`}},
			{"no-estimate", &TopicState{Value: StateResearching, Owner: "researcher"}, []string{`class="wb-state-pill research solo"`, `aria-label="연구 중 · researcher"`}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				view := View{Layout: layout, Lang: "ko", Topics: []TopicView{{Name: tc.name, Label: `Long "<&> title`, State: tc.state}}}
				for _, out := range stateViewOutputs(t, view) {
					wantAll(t, out, tc.want...)
					wantAll(t, out, `class="tname" title="Long &#34;&lt;&amp;&gt; title"`, `role="tooltip" popover="manual" hidden`)
					wantNone(t, out, `<span class="eta`, `<span class="state-estimate">`, `<span class="stb `)
				}
			})
		}
		for _, topic := range []TopicView{{Closed: true, State: &TopicState{Value: StateWaiting}}, {State: &TopicState{Value: StateWaiting}, StateDiagnostic: &StateDiagnostic{Code: "malformed"}}, {}} {
			for _, out := range stateViewOutputs(t, View{Layout: layout, Topics: []TopicView{topic}}) {
				wantNone(t, out, `<span class="topic-state dw">`, `<span class="wb-state-pill `)
			}
		}
	}
}

func TestTopicStateCompactElapsedBoundaries(t *testing.T) {
	run := &TopicRun{StartedAt: stateNow.Format(time.RFC3339Nano), Owner: "executor", ETAMinutes: 30, InitialETAMinutes: 30}
	for _, tc := range []struct {
		delta   time.Duration
		elapsed int64
		over    bool
	}{
		{-time.Second, 0, false}, {30 * time.Minute, 30, false}, {31*time.Minute - time.Millisecond, 30, false}, {31 * time.Minute, 31, true},
	} {
		b := stateBadge("ko", &TopicState{Value: StateImplementing, Run: run}, stateNow.Add(tc.delta))
		if b.Elapsed != tc.elapsed || b.Over != tc.over || strings.Contains(b.Description, "초과") != tc.over {
			t.Fatalf("%v: %+v", tc.delta, b)
		}
	}
	css, err := embeddedAsset("page-items.css")
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, string(css), ".dw .wb-state-pill.wait{--c:#b9bdc6}")
	wantNone(t, string(css), ".stb.wb-state-wait", "flex-basis:100%")
}
