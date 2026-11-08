package wswhiteboard

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// The topic header: the close result, the updated time, the topic note, and
// the closed-topic cap.

// TestCloseRequiresAResult pins wb:close's result: a close without one (or
// with a blank one) is refused, a close stores it, closing a closed topic
// again replaces it in place, and a reopen keeps it in the store.
func TestCloseRequiresAResult(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "a", "x")+addXML(ClassTechnical, "b", "y"))
	wantProblems(t, refused(t, st, `<wb:close topic="a"/>`), `command 1 <wb:close> (line 1) result: required`)
	wantProblems(t, refused(t, st, `<wb:close topic="a" result="  "/>`), `result: must be non-empty`)

	mustApply(t, st, `<wb:close topic="a" result="Shipped as planned."/><wb:close topic="b" result="Dropped."/>`)
	if st.CloseResults["a"] != "Shipped as planned." || st.CloseResults["b"] != "Dropped." {
		t.Fatalf("close results = %v", st.CloseResults)
	}
	mustApply(t, st, `<wb:close topic="a" result="Shipped, then patched."/>`)
	if st.CloseResults["a"] != "Shipped, then patched." || strings.Join(st.ClosedTopics, ",") != "a,b" {
		t.Fatalf("a second close: results %v, close order %v", st.CloseResults, st.ClosedTopics)
	}

	mustApply(t, st, addXML(ClassTechnical, "a", "again"))
	if st.topicClosed("a") || st.CloseResults["a"] != "Shipped, then patched." {
		t.Fatalf("a reopen: closed %v, results %v", st.topicClosed("a"), st.CloseResults)
	}
	mustApply(t, st, `<wb:close topic="a" result="Closed for good."/>`)
	if st.CloseResults["a"] != "Closed for good." || strings.Join(st.ClosedTopics, ",") != "b,a" {
		t.Fatalf("a re-close: results %v, close order %v", st.CloseResults, st.ClosedTopics)
	}
}

// TestCloseResultRendersBesideTheName pins where the result shows: dimmed
// beside a closed topic's name, on both layouts; hidden once the topic
// reopens; the new one after a re-close; and none on a topic closed before
// results existed.
func TestCloseResultRendersBesideTheName(t *testing.T) {
	const result = `<span class="tresult" title="how the topic ended">`
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "x")+`<wb:answer id="1" value="ok">ok</wb:answer>`+`<wb:close topic="t" result="Went with **plan b**."/>`)
	for _, layout := range layouts() {
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r"}))
		wantAll(t, out, `<span class="tname" title="closed">t</span>`+result+`Went with <strong>plan b</strong>.</span>`)
	}

	mustApply(t, st, addXML(ClassTechnical, "t", "reopened"))
	out := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantNone(t, out, result, "plan b")

	mustApply(t, st, `<wb:answer id="2" value="ok">ok</wb:answer><wb:close topic="t" result="Second ending."/>`)
	out = render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantAll(t, out, result+`Second ending.</span>`)
	wantNone(t, out, "plan b")

	// A store from before close results: closed, with no result to show.
	old := &State{Seq: 2, Items: []Item{{ID: 1, Class: ClassTechnical, Topic: "legacy", Title: "x", Status: StatusAnswered, Answer: "ok", Settled: 1}}, ClosedTopics: []string{"legacy"}}
	out = render(t, BuildView(old, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantAll(t, out, `<span class="tlead"><span class="tname" title="closed">legacy</span></span>`)
	wantNone(t, out, `class="tresult"`)
}

// TestTopicBarShowsItsLatestChange pins the updated time: the newest change
// among a topic's items and body; the full page writes it for the script to
// show as relative time (data-at, Unix ms) over a local HH:MM, the embed
// writes the HH:MM alone, and a topic without change times shows none.
func TestTopicBarShowsItsLatestChange(t *testing.T) {
	st := &State{}
	applyAt(t, st, addXML(ClassTechnical, "t", "x"), testNow)
	later := testNow.Add(95 * time.Minute)
	applyAt(t, st, topicXML("t", "body"), later)
	st.Items = append(st.Items, Item{ID: 9, Class: ClassTechnical, Topic: "old", Title: "from an old store", Status: StatusOpen})

	tz := time.FixedZone("test", 9*3600)
	full := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", Location: tz}))
	wantAll(t, full, fmt.Sprintf(`<time class="tago" datetime="2026-11-04T10:35:00Z" title="last change" data-at="%d">19:35</time>`, later.UnixMilli()))
	embed := render(t, BuildView(st, ViewInput{Layout: LayoutEmbed, ReplyID: "r", Location: tz}))
	wantAll(t, embed, `<time class="tago" datetime="2026-11-04T10:35:00Z" title="last change">19:35</time>`)
	wantNone(t, embed, "data-at=")

	for name, out := range map[string]string{"full": full, "embed": embed} {
		start := strings.Index(out, `<span class="tname">old</span>`)
		if start < 0 {
			t.Fatalf("%s: topic old missing", name)
		}
		bar := out[start:]
		bar = bar[:strings.Index(bar, "</div>")]
		if strings.Contains(bar, "tago") {
			t.Errorf("%s: a topic without change times shows one: %s", name, bar)
		}
	}
	// The full page carries the script that turns data-at into relative
	// time; the embed, a frozen snapshot, does not.
	wantAll(t, full, `querySelectorAll("time.tago[data-at]")`)
	wantNone(t, embed, `time.tago[data-at]`)
}

// TestTopicBarCarriesANotePen pins the topic note's markup: every topic bar
// on both layouts ends with a pen, the topic holds a hidden note box keyed by
// its name, the reply bar renders (hidden) even on a board with nothing to
// answer, and the form script builds the "[topic] text" line from it.
func TestTopicBarCarriesANotePen(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name='say "hi"'>body</wb:topic>`+addXML(ClassTechnical, "asks", "x"))
	for name, layout := range layouts() {
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r"}))
		pen := `<button class="btn mini tnote-toggle" title="note on this topic"><svg class="i" aria-hidden="true"><use href="#i-pen"/></svg></button></div>`
		wantAll(t, out, pen,
			`<textarea class="note tnote" data-topic="say &#34;hi&#34;" rows="1" placeholder="✎" title="note on this topic" hidden></textarea>`,
			`<textarea class="note tnote" data-topic="asks"`,
			`"[" + n.getAttribute("data-topic") + "] " + note`)
		if strings.Count(out, pen) != 2 {
			t.Errorf("%s: %d topic pens, want 2", name, strings.Count(out, pen))
		}
	}
	quiet := &State{}
	mustApply(t, quiet, topicXML("only", "body"))
	out := render(t, BuildView(quiet, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantAll(t, out, `<div class="reply solo" data-idle hidden>`, `id="reply-text"`, `id="reply-copy"`)
}

// closedBoard holds n closed topics c1..cn, closed in that order, each with
// one settled item, plus an open topic, a quiet open topic, and a closed
// topic still waiting on the user.
func closedBoard(t *testing.T, n int) *State {
	t.Helper()
	st := &State{}
	var cmds strings.Builder
	cmds.WriteString(addXML(ClassTechnical, "open", "waits") + addXML(ClassTechnical, "quiet", "settles"))
	for i := 1; i <= n; i++ {
		cmds.WriteString(addXML(ClassTechnical, fmt.Sprintf("c%d", i), "done"))
	}
	mustApply(t, st, cmds.String())
	var ids []string
	for id := 2; id <= n+2; id++ {
		ids = append(ids, fmt.Sprint(id))
	}
	mustApply(t, st, `<wb:answer ids="`+strings.Join(ids, " ")+`" value="ok">ok</wb:answer>`)
	cmds.Reset()
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&cmds, `<wb:close topic="c%d" result="Ending %d."/>`, i, i)
	}
	cmds.WriteString(addXML(ClassTechnical, "closed waiting", "still asks") + `<wb:close topic="closed waiting" result="Closed early."/>`)
	mustApply(t, st, cmds.String())
	return st
}

func viewTopicNames(v View) []string {
	var names []string
	for _, t := range v.Topics {
		names = append(names, t.Name)
	}
	return names
}

// TestClosedTopicsAreCapped pins the cap: closed topics with nothing waiting
// go last, the most recently closed first, and only ClosedCap of them build;
// open, quiet, and waiting topics never count against it; a ClosedWindow
// builds the rest alone.
func TestClosedTopicsAreCapped(t *testing.T) {
	st := closedBoard(t, 35)
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	names := viewTopicNames(view)
	if len(names) != 3+ClosedCap {
		t.Fatalf("%d topics built, want %d: %v", len(names), 3+ClosedCap, names)
	}
	for _, keep := range []string{"open", "closed waiting", "quiet"} {
		found := false
		for _, n := range names[:3] {
			found = found || n == keep
		}
		if !found {
			t.Errorf("%q is not ahead of the capped closed topics: %v", keep, names)
		}
	}
	if names[3] != "c35" || names[len(names)-1] != "c6" {
		t.Errorf("capped closed topics run %s..%s, want c35..c6", names[3], names[len(names)-1])
	}
	if view.ClosedMore != 5 || view.ClosedNext != ClosedCap {
		t.Errorf("ClosedMore %d, ClosedNext %d; want 5, %d", view.ClosedMore, view.ClosedNext, ClosedCap)
	}

	rest := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", Closed: &ClosedWindow{From: ClosedCap, N: ClosedCap}})
	if got := strings.Join(viewTopicNames(rest), ","); got != "c5,c4,c3,c2,c1" || rest.ClosedMore != 0 || rest.ClosedNext != 35 {
		t.Errorf("the rest = %s (more %d, next %d)", got, rest.ClosedMore, rest.ClosedNext)
	}
	page := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", Closed: &ClosedWindow{From: 30, N: 2}})
	if got := strings.Join(viewTopicNames(page), ","); got != "c5,c4" || page.ClosedMore != 3 || page.ClosedNext != 32 {
		t.Errorf("a two-topic page = %s (more %d, next %d)", got, page.ClosedMore, page.ClosedNext)
	}

	// Under the cap, nothing is left out.
	small := BuildView(closedBoard(t, 3), ViewInput{Layout: LayoutFull, ReplyID: "r"})
	if len(small.Topics) != 6 || small.ClosedMore != 0 {
		t.Errorf("an uncapped board built %d topics, more %d", len(small.Topics), small.ClosedMore)
	}
}

// TestClosedTopicCapRenders pins what each page shows past the cap: the file
// page and the embed name how many closed topics they leave out, the served
// board ends with a show-more control naming the next index, and the
// show-more answer renders the rest with no further control.
func TestClosedTopicCapRenders(t *testing.T) {
	st := closedBoard(t, 35)
	for name, layout := range layouts() {
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r"}))
		wantAll(t, out, `<div class="closed-more">5 more closed topics not shown</div>`, `<span class="tname" title="closed">c6</span>`)
		wantNone(t, out, `<span class="tname" title="closed">c5</span>`, `data-closed-next`)
		if c := strings.Count(out, `<section class="topic`); c != 3+ClosedCap {
			t.Errorf("%s: %d topics rendered, want %d", name, c, 3+ClosedCap)
		}
	}

	regions, err := RenderRegions(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	if err != nil {
		t.Fatal(err)
	}
	board := string(regions.Board)
	wantAll(t, board, `<button class="btn closed-more" data-closed-next="30">show more closed topics (5)</button>`)
	wantNone(t, board, `<div class="closed-more">`, `>c5</span>`)

	html, err := RenderClosedTopics(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", Closed: &ClosedWindow{From: 30, N: 30}}))
	if err != nil {
		t.Fatal(err)
	}
	rest := string(html)
	if !strings.HasPrefix(rest, `<div class="closed-page"><section class="topic`) || strings.Count(rest, `<section class="topic`) != 5 {
		t.Errorf("show-more answer:\n%.400s", rest)
	}
	wantAll(t, rest, `<span class="tname" title="closed">c5</span><span class="tresult" title="how the topic ended">Ending 5.</span>`, `class="btn mini tnote-toggle"`)
	wantNone(t, rest, `data-closed-next`, `id="reply-text"`)

	html, err = RenderClosedTopics(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", Closed: &ClosedWindow{From: 30, N: 2}}))
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, string(html), `<button class="btn closed-more" data-closed-next="32">show more closed topics (3)</button></div>`)
}
