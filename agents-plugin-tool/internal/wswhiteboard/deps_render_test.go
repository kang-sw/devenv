package wswhiteboard

import (
	"strings"
	"testing"
)

func TestDependencyPopoverRenderStates(t *testing.T) {
	waiters := []DependencyView{{Label: `Waiting <&>`, Anchor: "topic-waiter"}, {Label: "Another", Anchor: "topic-another"}}
	for _, tc := range []struct {
		name  string
		topic TopicView
		want  []string
		gone  []string
	}{
		{"unresolved", TopicView{Dependencies: []DependencyView{
			{Token: "topic:blocker", Label: "topic:Blocker", Anchor: "topic-blocker"},
			{Token: "exec:EXEC1", Label: "exec:EXEC1"},
			{Token: "ticket:missing", Label: "ticket:missing", Status: "unknown"},
			{Token: "owner", Label: "owner", Resolved: true},
		}, Waiters: waiters}, []string{
			`class="dbtn amber"`, `href="#i-clock"`, `class="sum amber"`,
			`대상 4개 중 1개 해결, 3개 남음`, `style="width:25%"`,
			`class="wb-dot u"`, `class="wb-dot x"`, `class="wb-dot r"`, `class="row res"`,
			`class="nm" href="#topic-blocker"`, `class="nm">소유자</span>`,
			`아직 열려 있음`, `직접 해제해야 함`, `알 수 없음`, `해결됨`,
		}, []string{`class="dbtn green"`}},
		{"ready", TopicView{WaitingReady: true, Closed: true, Result: "Done", Dependencies: []DependencyView{
			{Token: "topic:done", Label: "topic:Done", Resolved: true},
			{Token: "owner", Label: "owner", Resolved: true},
		}, Waiters: waiters}, []string{
			`class="dbtn green"`, `class="sum green"`, `진행 가능 · 모든 대상 해결`,
			`style="width:100%"`, `class="wb-dot r"`, `class="row res"`,
			`class="tresult" title="주제의 결말">Done</span><span class="dw">`,
		}, []string{`class="dbtn amber"`, `class="wb-dot u"`, `class="wb-dot x"`}},
		{"waiters-only", TopicView{Waiters: waiters}, []string{
			`class="dbtn"`, `aria-label="이 토픽을 기다리는 곳 2"`,
		}, []string{`class="dbtn amber"`, `class="dbtn green"`, `class="sum `, `class="bar`, `class="wb-dot `}},
		{"none", TopicView{}, nil, []string{`class="dw"`, `class="dbtn`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, layout := range layouts() {
				topic := tc.topic
				topic.Name, topic.Label, topic.Anchor = "work", "Work", "topic-work"
				view := View{Layout: layout, ReplyID: "r", Lang: "ko", Topics: []TopicView{topic}}
				for _, out := range stateViewOutputs(t, view) {
					wantAll(t, out, tc.want...)
					wantNone(t, out, tc.gone...)
					wantNone(t, out, `class="topic-deps"`, `class="dep-reverse"`)
					if tc.name == "none" {
						continue
					}
					wantAll(t, out, `aria-expanded="false" aria-haspopup="dialog"`,
						`class="bd r">2</span>`, `class="pop" role="dialog"`, ` hidden>`,
						`class="sep"`, `class="lb">이 토픽을 기다리는 곳 2`,
						`class="row link" href="#topic-waiter"><span>Waiting &lt;&amp;&gt;</span><span class="ar">`,
						`class="row link" href="#topic-another"`)
					if !topic.Closed {
						wantAll(t, out, `class="tname">Work</span><span class="dw">`)
					}
				}
				out := render(t, view)
				if n := strings.Count(out, "if (d._wbDeps) return;"); n != 1 {
					t.Fatalf("standalone dependency script count: %d", n)
				}
			}
		})
	}
	// With targets but no reverse dependencies the tiny badge is omitted.
	out := render(t, View{Topics: []TopicView{{Name: "work", Dependencies: []DependencyView{{Token: "owner"}}}}})
	wantNone(t, out, `class="bd r"`, `class="sep"`, `class="row link"`)
	assets, err := PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(assets.FormScript), "if (d._wbDeps) return;"); n != 1 {
		t.Fatalf("dashboard shared dependency script count: %d", n)
	}
}
