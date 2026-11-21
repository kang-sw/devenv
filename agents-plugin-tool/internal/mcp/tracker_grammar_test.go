// Tracker grammar tests.
package mcp

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

// A raw multi-line Korean markdown body goes in unescaped and comes back
// byte for byte; $N names what earlier commands of the batch create.
func TestTrackerUpdateWritesRawBodiesAndBatchReferences(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	body := "## 배경\n\n파서가 `<a href=\"x\">` 태그와 \"따옴표\", 'quote', & 기호를 그대로 둔다.\n- 첫째\n- 둘째"
	out := f.trackerUpdate(t, `<tk:store name="me">
<tk:item.create kind="task" title="파서 정리" due="2026-11-20" project="ws">
`+body+`
</tk:item.create>
<tk:item.assign item="$1" people="ada"/>
<tk:comment.add item="$1">첫 댓글
두 줄</tk:comment.add>
<tk:comment.add item="$1" reply_to="$3">답글</tk:comment.add>
<tk:item.status item="$1" status="doing" origin="agent-auto">시작</tk:item.status>
</tk:store>`)
	if !strings.Contains(out, "me: wrote 5 events") {
		t.Fatalf("update output: %s", out)
	}
	ids := trackerWrote(t, out)
	item, first, second := ids["1"], ids["3"], ids["4"]

	h, err := f.s.trackerHost()
	if err != nil {
		t.Fatal(err)
	}
	v, err := h.View(context.Background(), "me", 0)
	if err != nil {
		t.Fatal(err)
	}
	it := v.Items[strings.Replace(item, "-", "#", 1)]
	if it.Body != body || it.Title != "파서 정리" {
		t.Fatalf("body must round-trip byte for byte:\n got %q\nwant %q", it.Body, body)
	}
	reply := v.Comments[strings.Replace(second, "-", "#", 1)]
	if reply.ReplyTo == nil || *reply.ReplyTo != strings.Replace(first, "-", "#", 1) {
		t.Fatalf("$3 must name the first comment: %+v", reply)
	}
	for _, ev := range v.Events {
		if ev.Type == wstracker.TypeItemStatus && ev.Origin != wstracker.OriginAgentAuto {
			t.Fatalf("origin agent-auto must reach the event: %+v", ev)
		}
		if ev.Type == wstracker.TypeItemCreate && ev.Origin != wstracker.OriginAgent {
			t.Fatalf("an event without origin must be the agent's: %+v", ev)
		}
	}

	full := f.trackerQuery(t, map[string]any{"store": "me", "item": item})
	for _, want := range []string{
		item + ` task "파서 정리"`,
		"status doing", "due 2026-11-20", "project ws",
		"body:\n" + body + "\n",
		`ada doing (by ada), note "시작"`,
		first + " ada at", "  첫 댓글\n  두 줄",
		second + " ada at", "reply to " + first,
	} {
		if !strings.Contains(full, want) {
			t.Fatalf("item view missing %q:\n%s", want, full)
		}
	}
	mine := f.trackerQuery(t, map[string]any{"store": "me"})
	for _, want := range []string{"store me, viewer ada (Ada Kim), admin", "today: my work 1", "my assignments:\n- " + item + " P", "파서 정리: ada doing (by ada)"} {
		if !strings.Contains(mine, want) {
			t.Fatalf("mine scope missing %q:\n%s", want, mine)
		}
	}
	// The item ID's # form is accepted too.
	if alt := f.trackerQuery(t, map[string]any{"store": "me", "item": strings.Replace(item, "-", "#", 1)}); !strings.Contains(alt, "파서 정리") {
		t.Fatalf("the stored id form must find the item: %s", alt)
	}
	if none := f.trackerQuery(t, map[string]any{"store": "me", "scope": "all", "kind": "topic"}); !strings.Contains(none, "no rows pass the filters") {
		t.Fatalf("a filter that matches nothing must say so: %s", none)
	}
	// todo is no filter value: the store folds a todo create as a task.
	if bad := f.trackerRefused(t, "tracker.query", map[string]any{"payload": map[string]any{"store": "me", "kind": "todo"}}); !strings.Contains(bad, "must be one of task, topic") {
		t.Fatalf("a todo kind filter must be refused with the two kinds: %s", bad)
	}
	if hit := f.trackerQuery(t, map[string]any{"store": "me", "scope": "all", "text": "따옴표"}); !strings.Contains(hit, item+" [task doing]") {
		t.Fatalf("a text filter must match the body: %s", hit)
	}

	resp := f.call(t, "tracker.query", map[string]any{"payload": map[string]any{"store": "me"}, "format": "json"})
	var state struct {
		Store struct {
			Name string `json:"name"`
		} `json:"store"`
		Items map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(toolText(t, resp)), &state); err != nil || state.Store.Name != "me" || len(state.Items) != 1 {
		t.Fatalf("format json must return the store's view state: %v %s", err, toolText(t, resp))
	}
}

// Every problem the batch has is listed at once, before any write, with a
// pointer to the playbook.
func TestTrackerUpdateListsEveryProblem(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:register store="x" member="a"/>
<tk:store name="me">
<tk:item.create kind="epic" title="t" origin="human"/>
<tk:item.assign item="$1" people="a" levels="2"/>
<tk:comment.add item="$9"></tk:comment.add>
<tk:item.edit item="$3"/>
<tk:item.close item="maya"/>
</tk:store>
<tk:store name="other"><tk:item.hide store="me" item="$1"/></tk:store>
<tk:item.hide item="$1"/>`})
	for _, want := range []string{
		"command 1 <tk:register> (line 1): reaches the network and cannot be rolled back, so it stands alone in its batch",
		"command 2 <tk:item.create> (line 3) kind: must be one of task, todo, topic",
		"command 2 <tk:item.create> (line 3) origin: must be one of agent, agent-auto",
		"command 3 <tk:item.assign> (line 4): levels stands alone; beside people or rules",
		"command 4 <tk:comment.add> (line 5) item: $N names an earlier command of this batch by position",
		"command 4 <tk:comment.add> (line 5) body: must be non-empty",
		"command 5 <tk:item.edit> (line 6) item: $3 names command 3, which creates no item; $N must name a <tk:item.create>",
		"command 5 <tk:item.edit> (line 6): changes nothing",
		"command 6 <tk:item.close> (line 7) item: must be an item ID such as maya.pc-5, or $N",
		"<tk:store> (line 9): a second <tk:store> group; one batch writes to one store, so it holds one group",
		"command 7 <tk:item.hide> (line 9): unknown attribute store (known: origin, item)",
		`command 8 <tk:item.hide> (line 10): goes inside a <tk:store name="..."> group`,
		"Syntax and commands: the `tracker` playbook (ws/playbook.read).",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("refusal missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "## Command syntax") || strings.Contains(text, "## Commands") {
		t.Fatalf("the refusal must not repeat playbook sections:\n%s", text)
	}
	// The group names a registered store, once for all its commands; the
	// network commands stay at the root.
	text = f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:store name="other"><tk:item.hide item="ada.pc-1"/><tk:item.hide item="ada.pc-2"/></tk:store>`})
	if want := `- <tk:store> (line 1) name: "other" is not a registered store (registered: me)` + "\n"; strings.Count(text, want) != 1 {
		t.Fatalf("refusal must name the unregistered store once:\n%s", text)
	}
	text = f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:store name="me"><tk:sync/></tk:store>`})
	if !strings.Contains(text, "command 1 <tk:sync> (line 1): goes at the root, outside <tk:store> (line 1)") {
		t.Fatalf("refusal must keep tk:sync at the root:\n%s", text)
	}
	text = f.trackerRefused(t, "tracker.update", map[string]any{"commands": map[string]any{"x": 1}})
	if !strings.Contains(text, "commands: must be a string") {
		t.Fatalf("a JSON commands value must be refused: %s", text)
	}
	text = f.trackerRefused(t, "tracker.query", map[string]any{"payload": map[string]any{"store": "me", "scope": "everything", "item": "x-1"}, "format": "json"})
	for _, want := range []string{"payload.scope: must be one of", "payload.item: shows one item", "## Query", "tracker.query payload:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("query refusal missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "## Commands") {
		t.Fatalf("the query refusal must carry the Query section only:\n%s", text)
	}
}

// Every tk: attribute and body decodes like any command body: a title and
// project, an item body, a note, and a comment; CDATA and a <tk:raw>
// segment stay literal.
func TestTrackerUpdateDecodesEscapedText(t *testing.T) {
	var problems wsxmlcmd.Problems
	b := planTrackerBatch(&problems, `<tk:store name="team"><tk:item.create kind="task" title="Parse &lt;key&gt; &amp; value" project="a&#38;b">body &lt;b&gt;</tk:item.create>`+
		`<tk:item.edit item="$1" title="&quot;t&quot;">new &lt;b&gt;</tk:item.edit>`+
		`<tk:item.status item="$1" status="doing">waiting on &lt;key&gt;</tk:item.status>`+
		`<tk:item.close item="$1">done &amp; dusted <![CDATA[&lt;kept&gt;]]></tk:item.close>`+
		`<tk:comment.add item="$1">see &lt;b&gt;</tk:comment.add>`+
		`<tk:comment.edit comment="$5">edit &lt;b&gt;</tk:comment.edit>`+
		`<tk:comment.add item="$1">a &lt; b <tk:raw><i>&amp;</i> </tk:comment.add></tk:raw></tk:comment.add></tk:store>`,
		[]wstrackerhost.StoreEntry{{ID: "team"}}, trackerTestReach)
	if len(problems.List) > 0 {
		t.Fatalf("problems: %q", problems.List)
	}
	want := []map[string]any{
		{"title": "Parse <key> & value", "project": "a&b", "body": "body <b>"},
		{"title": `"t"`, "body": "new <b>"},
		{"note": "waiting on <key>"},
		{"note": "done & dusted &lt;kept&gt;"},
		{"body": "see <b>"},
		{"body": "edit <b>"},
		{"body": "a < b <i>&amp;</i> </tk:comment.add>"},
	}
	for i, fields := range want {
		for k, v := range fields {
			if got := b.events[i].payload[k]; got != v {
				t.Errorf("event %d (%s) %s = %q, want %q", i+1, b.events[i].typ, k, got, v)
			}
		}
	}
}

// stage is retired: the writer refuses it on a create or an edit and as a
// clear field, and item.status takes idea.
func TestTrackerUpdateRefusesStage(t *testing.T) {
	b := trackerPlan(t, `<tk:item.create kind="task" title="t"/><tk:item.status item="$1" status="idea"/>`)
	if got := b.events[1].payload["status"]; got != "idea" {
		t.Fatalf("item.status idea = %v", got)
	}
	var problems wsxmlcmd.Problems
	planTrackerBatch(&problems, `<tk:store name="team"><tk:item.create kind="task" title="t" stage="review"/>`+
		`<tk:item.edit item="$1" stage="review"/>`+
		`<tk:item.edit item="$1" clear="stage"/></tk:store>`, []wstrackerhost.StoreEntry{{ID: "team", Member: "ada"}}, trackerTestReach)
	for _, want := range []string{
		`command 1 <tk:item.create> (line 1): unknown attribute stage (known: `,
		`command 2 <tk:item.edit> (line 1): unknown attribute stage (known: `,
		`command 3 <tk:item.edit> (line 1) clear: must list body, due, or start; got "stage"`,
	} {
		if !slices.ContainsFunc(problems.List, func(p string) bool { return strings.HasPrefix(p, want) }) {
			t.Errorf("problems missing %q:\n%q", want, problems.List)
		}
	}
}
