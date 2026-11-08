package wswhiteboard

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func marks(m ...any) []BlockMark {
	var out []BlockMark
	for i := 0; i < len(m); i += 2 {
		out = append(out, BlockMark{m[i].(int), m[i+1].(string)})
	}
	return out
}

func TestDiffBodiesBlockKinds(t *testing.T) {
	cases := []struct {
		name, old, cur string
		want           []BlockMark
	}{
		{"identical", "one\n\ntwo\n", "one\n\ntwo\n", nil},
		{"whitespace only", "one two\n\nthree\n", "one  two\n\n\nthree\n", nil},
		{"new paragraph at end", "one\n\ntwo\n", "one\n\ntwo\n\nthree\n", marks(2, MarkNew)},
		{"new paragraph in the middle", "one\n\ntwo\n", "one\n\nmid\n\ntwo\n", marks(1, MarkNew)},
		{"modified paragraph", "one\n\ntwo\n\nthree\n", "one\n\ntwo changed\n\nthree\n", marks(1, MarkMod)},
		{"deleted paragraph", "one\n\ntwo\n\nthree\n", "one\n\nthree\n", marks(1, MarkDel)},
		{"deleted last paragraph", "one\n\ntwo\n", "one\n", marks(1, MarkDel)},
		{"modified and new", "one\n\ntwo\n", "one\n\ntwo changed\n\nplus\n", marks(1, MarkMod, 2, MarkNew)},
		{"new block before a modified one pairs by similarity", "- alpha\n- gamma delta\n", "- alpha\n- brand new item\n- gamma delta changed\n", marks(1, MarkNew, 2, MarkMod)},
		{"a block swapped one for one is modified", "one\n\nold words\n\nend\n", "one\n\nfresh text\n\nend\n", marks(1, MarkMod)},
		{"unrelated blocks are new plus a deletion point", "one\n\nold words\n\ngone\n\nend\n", "one\n\nfresh text\n\nend\n", marks(1, MarkDel, 1, MarkNew)},
		{"modified heading level", "# Title\n\nbody\n", "## Title\n\nbody\n", marks(0, MarkMod)},
		{"list item added", "- a\n- b\n", "- a\n- b\n- c\n", marks(2, MarkNew)},
		{"list item modified", "- a\n- bee gee\n- c\n", "- a\n- bee gee zed\n- c\n", marks(1, MarkMod)},
		{"nested item added leaves its parent alone", "- a\n  - x\n- b\n", "- a\n  - x\n  - y\n- b\n", marks(2, MarkNew)},
		{"fenced code edited", "intro\n\n```go\nx := 1\n```\n", "intro\n\n```go\nx := 2\n```\n", marks(1, MarkMod)},
		{"fence language edited", "```go\nx\n```\n", "```rust\nx\n```\n", marks(0, MarkMod)},
		{"mermaid fence edited", "```mermaid\ngraph TD; A-->B\n```\n", "```mermaid\ngraph TD; A-->C\n```\n", marks(0, MarkMod)},
		{"table cell edited", "| a | b |\n|---|---|\n| 1 | 2 |\n", "| a | b |\n|---|---|\n| 1 | 3 |\n", marks(0, MarkMod)},
		{"raw html block edited", "intro\n\n<div class=\"x\">old</div>\n\nend\n", "intro\n\n<div class=\"x\">new</div>\n\nend\n", marks(1, MarkMod)},
		{"blockquote child edited", "> q1\n>\n> q2\n", "> q1\n>\n> q2 edited\n", marks(1, MarkMod)},
		{"link destination edited", "[a](https://x.test/1)\n", "[a](https://x.test/2)\n", marks(0, MarkMod)},
		{"modified plus a deletion", "a b\n\nb\n\nc\n", "a b2\n", marks(0, MarkMod, 1, MarkDel)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := diffBodies(tc.old, tc.cur); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("diffBodies = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDiffKeysDeletionSortsBeforeTheBlocksOwnMark(t *testing.T) {
	got := diffKeys([]string{"P a", "P b", "P c"}, []string{"P a", "P x", "P c"})
	if want := marks(1, MarkMod); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	got = diffKeys([]string{"P a", "P b", "P c", "P d"}, []string{"P z", "P d"})
	if want := marks(0, MarkDel, 0, MarkNew); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	got = diffKeys([]string{"P a x", "P b y", "P c"}, []string{"P a x2", "P b y2", "P c", "P d"})
	if want := marks(0, MarkMod, 1, MarkMod, 3, MarkNew); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func topicMarks(t *testing.T, st *State) []BlockMark {
	t.Helper()
	return topicEntry(t, st, "t").Marks
}

func TestTopicMarksFollowOnlyBodyEdits(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("t", "one\n\ntwo"))
	if got := topicMarks(t, st); got != nil {
		t.Fatalf("a first body marks nothing, got %v", got)
	}

	mustApply(t, st, topicXML("t", "one\n\ntwo\n\nthree"))
	want := marks(2, MarkNew)
	if got := topicMarks(t, st); !reflect.DeepEqual(got, want) {
		t.Fatalf("after the first edit: %v, want %v", got, want)
	}

	// Items, answers, patches, other topics, close/reopen, agenda-style
	// refreshes: none touch the marks.
	mustApply(t, st, addXML("policy", "t", "Q?", "A", "B"))
	mustApply(t, st, `<wb:answer id="1" value="a">A</wb:answer>`)
	mustApply(t, st, topicXML("other", "x\n\ny"))
	mustApply(t, st, topicXML("other", "x\n\nz"))
	mustApply(t, st, ``)
	if got := topicMarks(t, st); !reflect.DeepEqual(got, want) {
		t.Fatalf("a same-topic item write or another topic's edit changed the marks: %v, want %v", got, want)
	}

	// The same body again is no change and keeps them.
	mustApply(t, st, topicXML("t", "one\n\ntwo\n\nthree"))
	if got := topicMarks(t, st); !reflect.DeepEqual(got, want) {
		t.Fatalf("an identical wb:topic changed the marks: %v, want %v", got, want)
	}

	// A second body edit replaces them, wb:edit included.
	mustApply(t, st, `<wb:edit topic="t"><wb:old>two</wb:old><wb:new>TWO</wb:new></wb:edit>`)
	want = marks(1, MarkMod)
	if got := topicMarks(t, st); !reflect.DeepEqual(got, want) {
		t.Fatalf("after wb:edit: %v, want %v", got, want)
	}

	// An edit that changes no block drops them: the latest edit marked nothing.
	mustApply(t, st, `<wb:edit topic="t"><wb:old>one</wb:old><wb:new>one </wb:new></wb:edit>`)
	if got := topicMarks(t, st); got != nil {
		t.Fatalf("a block-neutral edit must leave no marks, got %v", got)
	}

	// Clearing the body and setting one again is a first body.
	mustApply(t, st, `<wb:topic name="t"></wb:topic>`)
	mustApply(t, st, topicXML("t", "fresh"))
	if got := topicMarks(t, st); got != nil {
		t.Fatalf("a body set after a clear is a first body, got %v", got)
	}
}

func TestTopicMarksSurviveReload(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("t", "one\n\ntwo"))
	mustApply(t, st, topicXML("t", "one\n\ntwo2\n\nthree"))
	want := marks(1, MarkMod, 2, MarkNew)
	var back State
	if err := json.Unmarshal(mustMarshal(t, st), &back); err != nil {
		t.Fatal(err)
	}
	if got := topicMarks(t, &back); !reflect.DeepEqual(got, want) {
		t.Fatalf("marks after a reload: %v, want %v", got, want)
	}
	mustApply(t, &back, addXML("technical", "t", "Another"))
	if got := topicMarks(t, &back); !reflect.DeepEqual(got, want) {
		t.Fatalf("marks after a reload and an item add: %v, want %v", got, want)
	}
}

func TestRenderDocWritesMarksAsComments(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("t", "intro\n\n- a\n- b\n\n```mermaid\ngraph TD; A-->B\n```\n\n<div>raw</div>\n\nend"))
	mustApply(t, st, topicXML("t", "intro\n\n- a\n- b\n- c\n\n```mermaid\ngraph TD; A-->B\n```\n\n<div>RAW</div>"))
	view := BuildView(st, ViewInput{Layout: LayoutFull})
	page, err := RenderPage(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<ul>\n<li>a</li>\n<li>b</li>\n<!--wbchg:new-->\n<li>c</li>", "<!--wbchg:mod-->\n<div>RAW</div>", "<!--wbchg:delafter-->"} {
		if !strings.Contains(page, want) {
			t.Fatalf("page lacks %q", want)
		}
	}
	if strings.Count(page, "\n<!--wbchg:new-->\n") != 1 || strings.Count(page, "\n<!--wbchg:mod-->\n") != 1 || strings.Contains(page, "\n<!--wbchg:del-->\n") {
		t.Fatal("the page must carry exactly the new and the modified mark beside the delafter one")
	}

	// The embed snapshot carries none.
	embed, err := RenderPage(BuildView(st, ViewInput{Layout: LayoutEmbed}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(embed, "wbchg:") {
		t.Fatal("the embed must not carry change marks")
	}

	// A framed body (the dashboard) carries them inside its frame document.
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(regions.Board), "wbchg:new") {
		t.Fatal("a framed body must carry its change marks")
	}

	// A mark past the body's blocks (stale) renders nothing and fails nothing.
	st.Topics[0].Marks = marks(99, MarkMod)
	if _, err := RenderPage(BuildView(st, ViewInput{Layout: LayoutFull})); err != nil {
		t.Fatal(err)
	}
}
