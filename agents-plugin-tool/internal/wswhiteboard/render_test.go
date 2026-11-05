package wswhiteboard

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

const sampleNow = `<p>One update writes the full page and an embed; the embed keeps the latest five.</p>
<svg viewBox="0 0 640 170" role="img" xmlns="http://www.w3.org/2000/svg">
  <defs><marker id="ah" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto"><path class="arrow" d="M0,0 L8,4 L0,8 z"/></marker></defs>
  <rect class="box" x="10" y="55" width="120" height="50" rx="8"/><text x="24" y="77">lead</text><text class="sub" x="24" y="93">one update per turn</text>
  <rect class="box store" x="200" y="55" width="140" height="50" rx="8"/><text class="on" x="214" y="77">session store</text><text class="sub on" x="214" y="93">items · seq · archive</text>
  <rect class="box" x="430" y="10" width="200" height="50" rx="8"/><text x="444" y="32">full · &lt;id&gt;.html</text><text class="sub" x="444" y="48">overwrite + stamp</text>
  <rect class="box" x="430" y="100" width="200" height="50" rx="8"/><text x="444" y="122">embed · &lt;id&gt;-&lt;hash&gt;.html</text><text class="sub" x="444" y="138">new file, keep 5</text>
  <path class="edge" d="M130,80 H198" marker-end="url(#ah)"/>
  <path class="edge" d="M340,72 C385,72 390,35 428,35" marker-end="url(#ah)"/>
  <path class="edge" d="M340,88 C385,88 390,125 428,125" marker-end="url(#ah)"/>
  <path class="edge dash" d="M180,146 H395 V48 H428" marker-end="url(#ah)"/>
</svg>`

// sampleState is a board with every item kind: open policy, assumed, open
// technical and announced, answered, withdrawn, and superseded items across
// several topics, one of them closed. Seq 12 changed Now; the previous embed
// was at seq 10.
func sampleState() *State {
	st := &State{Counter: 90, Seq: 12, NowSeq: 12, LastEmbedSeq: 10, SettleSeq: 20, Now: sampleNow, ClosedTopics: []string{"초기 범위"}}
	add := func(item Item) { st.Items = append(st.Items, item) }
	settled := func(id int, topic, title string, order, changed int) Item {
		return Item{ID: id, Class: ClassTechnical, Topic: topic, Title: title, Status: StatusAnswered, Answer: "ok", AnswerQuote: "looks fine", Settled: order, Changed: changed}
	}

	add(Item{ID: 77, Class: ClassPolicy, Topic: "embed retention", Title: "How old embed files are cleaned up",
		Options:        []string{"delete the previous embed on every render", "keep the latest five", "keep everything until session GC"},
		Recommendation: "b: bounds the file count while chat history keeps rendering",
		Rationale:      "If the host rereads the file, a deleted embed turns blank in chat history.\nEmbeds in a host folder are never deleted.",
		Links:          []string{"follows {{69}}", "see {{999}} in the archive"},
		Status:         StatusOpen, Changed: 12})
	add(Item{ID: 78, Class: ClassTechnical, Topic: "embed retention", Title: "Eviction order follows the render sequence",
		Rationale: "not mtime - two renders in one second share an mtime, and copies change it", Status: StatusOpen, Changed: 11})
	add(Item{ID: 69, Class: ClassPolicy, Topic: "embed retention", Title: "Whiteboards live in ai-docs/whiteboards/",
		Options: []string{"ai-docs/whiteboards/", "ai-docs/whiteboards.local/"}, Status: StatusAnswered, Answer: "a",
		AnswerQuote: "no need for a local folder", Settled: 9, Changed: 6})

	add(Item{ID: 73, Class: ClassPolicy, Topic: "embed layout", Title: "Where Now sits in the embed",
		Options:        []string{"always below", "always above", "above when this render changed it, otherwise below the topics so the questions come first"},
		Recommendation: "c: a new explanation is read first, an old one stays out of the way",
		Status:         StatusOpen, Changed: 12})
	add(Item{ID: 76, Class: ClassPolicy, Topic: "embed layout", Title: "Recommendation shows as a star plus a line",
		Options: []string{"star only", "star and recommendation line"}, Recommendation: "b",
		Status: StatusAssumed, Answer: "b", AnswerQuote: "71-76 look good", Changed: 11})
	add(Item{ID: 71, Class: ClassAnnounced, Topic: "embed layout", Title: "Full page has two columns, panels in the sidebar",
		Status: StatusAnswered, Answer: "ok", AnswerQuote: "71-76 look good", Settled: 18, Changed: 11})
	add(Item{ID: 72, Class: ClassAnnounced, Topic: "embed layout", Title: "Embed has one column",
		Status: StatusAnswered, Answer: "ok", AnswerQuote: "71-76 look good", Settled: 19, Changed: 11, Links: []string{"pairs with {{71}}"}})

	add(Item{ID: 80, Class: ClassTechnical, Topic: "Now 입력", Title: "now_path로 HTML 파일을 받아 update 시점에 저장",
		Rationale: "render마다 다시 읽는 방식 대신 - 파일이 지워지거나 바뀌어도 보드가 조용히 달라지지 않음", Status: StatusOpen, Changed: 12})
	add(Item{ID: 79, Class: ClassAnnounced, Topic: "Now 입력", Title: "템플릿 문구는 기호 위주, 산문 없음",
		Rationale: "근거: 사용자 지시 (템플릿 텍스트 최소화)", Status: StatusOpen, Changed: 12})
	add(settled(81, "Now 입력", "Now is an HTML fragment, not a document", 20, 12))
	add(settled(82, "Now 입력", "Now is stored at update time", 15, 9))
	add(Item{ID: 83, Class: ClassTechnical, Topic: "Now 입력", Title: "Reread now_path on every render", Status: StatusWithdrawn, Settled: 16, Changed: 9})
	add(Item{ID: 84, Class: ClassPolicy, Topic: "Now 입력", Title: "Now as a separate drawing page", Options: []string{"yes", "no"}, Status: StatusSuperseded, Settled: 17, Changed: 9})

	for i := 0; i < 8; i++ {
		add(settled(52+i, "tool surface", fmt.Sprintf("Tool surface decision %d", i+1), i+1, 4))
	}
	add(settled(40, "초기 범위", "첫 범위는 세션 보드 하나", 10, 3))
	add(settled(41, "초기 범위", "Drawing stays on the whiteboard", 11, 3))
	add(settled(42, "초기 범위", "Panels are collapsible", 12, 3))
	return st
}

func samplePanels() []Panel {
	return []Panel{
		{ID: "todo", Title: "Todos", Icon: "todo", Count: "2/5", Rows: []PanelRow{
			{State: "done", Label: "mockup", Text: "First mockup round"},
			{State: "done", Label: "symbols", Text: "Replace chrome prose with symbols"},
			{State: "wip", Label: "topics", Text: "Settle the topic-first layout"},
			{State: "pending", Label: "commit", Text: "Commit the templates and update the ticket"},
			{State: "defer", Label: "windows", Text: "Windows sharing-violation check"},
		}},
		{ID: "agenda", Title: "Agendas", Icon: "list", Count: "2", Rows: []PanelRow{
			{Label: "whiteboard.render", Text: "Template port in progress; (73) and (77) open"},
			{Label: "whiteboard.review", Text: "Review after the port lands"},
		}},
	}
}

func sampleView(layout Layout) View {
	return BuildView(sampleState(), ViewInput{Layout: layout, ReplyID: "barrette-regain-lumpiness", Panels: samplePanels()})
}

func render(t *testing.T, view View) string {
	t.Helper()
	out, err := RenderPage(view)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}
	return out
}

func layouts() map[string]Layout {
	return map[string]Layout{"full": LayoutFull, "embed": LayoutEmbed}
}

func TestRenderPageDocumentShape(t *testing.T) {
	htmlTag := regexp.MustCompile(`<html[^>]*>`)
	for name, layout := range layouts() {
		out := render(t, sampleView(layout))
		if !strings.HasPrefix(out, "<!DOCTYPE html>") {
			t.Errorf("%s: page does not start with the doctype: %.40q", name, out)
		}
		if tag := htmlTag.FindString(out); tag == "" || strings.Contains(tag, "lang") {
			t.Errorf("%s: html tag %q missing or carries a lang attribute", name, tag)
		}
		if strings.Contains(out, " lang=") {
			t.Errorf("%s: page carries a lang attribute", name)
		}
		if !strings.Contains(out, `<body class="`+name+`">`) {
			t.Errorf("%s: body class missing", name)
		}
	}
}

func TestRenderPageIsSelfContained(t *testing.T) {
	external := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(src|href)\s*=\s*["']?\s*(https?:)?//`),
		regexp.MustCompile(`(?i)url\(\s*["']?\s*(https?:)?//`),
		regexp.MustCompile(`(?i)@import`),
		regexp.MustCompile(`(?i)<link\b`),
		regexp.MustCompile(`(?i)<script[^>]+src=`),
	}
	for name, layout := range layouts() {
		out := render(t, sampleView(layout))
		for _, re := range external {
			if loc := re.FindStringIndex(out); loc != nil {
				t.Errorf("%s: external reference %q matches %s", name, out[loc[0]:min(loc[1]+40, len(out))], re)
			}
		}
		if !strings.Contains(out, `<symbol id="i-check"`) || !strings.Contains(out, `<use href="#i-check"/>`) {
			t.Errorf("%s: icons do not come from the inline sprite", name)
		}
		if strings.Contains(out, "navigator.clipboard") {
			t.Errorf("%s: page uses navigator.clipboard", name)
		}
		if !strings.Contains(out, `execCommand("copy")`) {
			t.Errorf("%s: copy does not use execCommand", name)
		}
	}
}

func TestRenderPageChangeSignalOnlyOnFull(t *testing.T) {
	full := render(t, sampleView(LayoutFull))
	if !strings.Contains(full, `<html data-stamp="barrette-regain-lumpiness.stamp.js">`) {
		t.Errorf("full page lacks the data-stamp attribute")
	}
	for _, want := range []string{"window.wbStamp", "setInterval(poll, 1500)", `id="wb-banner"`, `"wb-scroll"`, `"wb-open-"`} {
		if !strings.Contains(full, want) {
			t.Errorf("full page lacks %q", want)
		}
	}
	// Scroll restore runs after every synchronous fill: the panel open-state
	// restore and the form's first build.
	restore := strings.LastIndex(full, `getItem("wb-scroll")`)
	if restore < strings.Index(full, `getItem("wb-open-"`) || restore < strings.LastIndex(full, "build();") {
		t.Errorf("scroll restore is not the last synchronous step")
	}

	view := sampleView(LayoutEmbed)
	view.StampFile = "stray.stamp.js" // the embed ignores a stamp even when one is set
	embed := render(t, view)
	for _, banned := range []string{"data-stamp", "wbStamp", "sessionStorage", "wb-banner", "location.reload"} {
		if strings.Contains(embed, banned) {
			t.Errorf("embed carries %q", banned)
		}
	}
}

func TestRenderPageNowPlacement(t *testing.T) {
	for name, layout := range layouts() {
		view := sampleView(layout)
		view.NowAbove = true
		out := render(t, view)
		now, topic := strings.Index(out, `<details class="now"`), strings.Index(out, `<section class="topic`)
		if now < 0 || topic < 0 || now > topic {
			t.Errorf("%s: Now (%d) is not above the topics (%d)", name, now, topic)
		}

		view.NowAbove = false
		out = render(t, view)
		now, last := strings.Index(out, `<details class="now"`), strings.LastIndex(out, `<section class="topic`)
		if now < 0 || now < last {
			t.Errorf("%s: Now (%d) is not below the topics (%d)", name, now, last)
		}

		view.Now = ""
		if out = render(t, view); strings.Contains(out, `<details class="now"`) {
			t.Errorf("%s: empty Now still renders the slot", name)
		}
	}
}

func TestRenderPageEscapesItemTextButNotNow(t *testing.T) {
	st := &State{Seq: 2, NowSeq: 2, Now: `<b id="now-raw">raw <i>now</i></b>`}
	st.Items = []Item{
		{ID: 1, Class: ClassPolicy, Topic: "<script>t()</script>", Title: "<script>alert(1)</script>",
			Options: []string{`<img src=x onerror=alert(2)>`, "b"}, Recommendation: "a: <b>why</b>",
			Rationale: "<script>r()</script>", Links: []string{`<a href="javascript:x">{{2}}</a>`}, Status: StatusOpen},
		{ID: 2, Class: ClassTechnical, Topic: "t", Title: `"quoted" & <tag>`, Status: StatusAnswered, Answer: "ok", AnswerQuote: "</div><script>q()</script>", Settled: 1},
	}
	panels := []Panel{{ID: `x"><script>p()</script>`, Title: "<script>pt()</script>", Icon: `"><script>`, Rows: []PanelRow{{State: `"><b>`, Label: "<l>", Text: "<script>pr()</script>"}}}}
	for name, layout := range layouts() {
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r", Panels: panels}))
		for _, raw := range []string{"<script>alert(1)", "<script>t()", "<img src=x", "<script>r()", "<b>why</b>", `href="javascript`, "<script>q()", "<script>p()", "<script>pt()", "<script>pr()", "<tag>"} {
			if strings.Contains(out, raw) {
				t.Errorf("%s: item text %q is not escaped", name, raw)
			}
		}
		for _, escaped := range []string{"&lt;script&gt;alert(1)&lt;/script&gt;", "&#34;quoted&#34; &amp; &lt;tag&gt;"} {
			if !strings.Contains(out, escaped) {
				t.Errorf("%s: escaped text %q missing", name, escaped)
			}
		}
		if !strings.Contains(out, `<b id="now-raw">raw <i>now</i></b>`) {
			t.Errorf("%s: Now fragment is not written unescaped", name)
		}
		if !strings.Contains(out, `<use href="#i-list"/>`) {
			t.Errorf("%s: unknown panel icon did not fall back to the list symbol", name)
		}
		if got := strings.Count(out, "<script>"); got != 1 {
			t.Errorf("%s: page holds %d script elements, want only its own", name, got)
		}
	}
}

func TestRenderPageTopicBlockOrder(t *testing.T) {
	out := render(t, sampleView(LayoutFull))
	start := strings.Index(out, `<span class="tname">embed layout</span>`)
	if start < 0 {
		t.Fatal("embed layout topic missing")
	}
	block := out[start:]
	block = block[:strings.Index(block, "</section>")]
	policy, assumed, settled := strings.Index(block, `id="item-73"`), strings.Index(block, `id="item-76"`), strings.Index(block, `id="item-72"`)
	if !(policy >= 0 && policy < assumed && assumed < settled) {
		t.Errorf("block order policy %d, assumed %d, settled %d", policy, assumed, settled)
	}
	if !strings.Contains(block, `<span class="chip policy" title="to answer">`) || !strings.Contains(block, `<span class="chip assumed" title="assumed, confirm or correct">`) {
		t.Errorf("per-class count chips missing from the title bar")
	}

	start = strings.Index(out, `<span class="tname">embed retention</span>`)
	block = out[start:]
	block = block[:strings.Index(block, "</section>")]
	policy, ack, settled := strings.Index(block, `id="item-77"`), strings.Index(block, `id="item-78"`), strings.Index(block, `id="item-69"`)
	if !(policy >= 0 && policy < ack && ack < settled) {
		t.Errorf("block order policy %d, ack %d, settled %d", policy, ack, settled)
	}
}

func TestRenderPagePolicyOptionsAndPicks(t *testing.T) {
	out := render(t, sampleView(LayoutFull))
	for _, want := range []string{
		`<div class="card policy" id="item-77" data-id="77">`,
		`<button class="opt rec" data-opt="b" aria-pressed="false"><span class="k">b<svg class="i" aria-hidden="true"><use href="#i-star"/></svg></span>`,
		`<b><svg class="i" aria-hidden="true"><use href="#i-star"/></svg>b</b> bounds the file count`,
		// the assumed item's picked option is static, never a form button
		`<span class="opt static rec picked"><span class="k">b`,
		`<button class="btn mini" data-v="ok" aria-pressed="false" title="confirm">`,
		// a settled policy item shows its picked option
		`<span class="pick" title="picked"><span class="k">a</span>ai-docs/whiteboards/</span>`,
		`<input type="checkbox" data-id="78">`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("full page lacks %q", want)
		}
	}
	// one note button per waiting item: (77), (78), (73), (76), (80), (79)
	if got := strings.Count(out, `<button class="btn mini note-toggle" title="note">`); got != 6 {
		t.Errorf("%d note buttons, want one per waiting item (6)", got)
	}
}

func TestRenderPageQuietAndCollapsedSettled(t *testing.T) {
	full := render(t, sampleView(LayoutFull))
	// The closed topic's settled items sit behind a collapsed chip in its one
	// title line.
	start := strings.Index(full, `<section class="topic quiet closed">`)
	if start < 0 {
		t.Fatal("closed settled-only topic is not quiet")
	}
	block := full[start:]
	block = block[:strings.Index(block, "</section>")]
	if strings.Contains(block, `class="tbody"`) {
		t.Errorf("quiet topic renders a body")
	}
	if !strings.HasPrefix(block[strings.Index(block, `<details class="settled"`):], `<details class="settled"><summary title="settled">`) {
		t.Errorf("quiet topic's settled chip is not collapsed")
	}
	for _, id := range []int{40, 41, 42} {
		if !strings.Contains(block, fmt.Sprintf(`id="item-%d"`, id)) {
			t.Errorf("collapsed settled item %d missing from the quiet topic", id)
		}
	}

	// tool surface: its three most recent settled items are among the ten
	// expanded board-wide, the other five sit in a nested collapsed +5.
	start = strings.Index(full, `<span class="tname">tool surface</span>`)
	block = full[start:]
	block = block[:strings.Index(block, "</section>")]
	if !strings.Contains(block, `<details class="settled" open><summary title="settled">`) {
		t.Errorf("expanded settled rows are not shown open")
	}
	more := strings.Index(block, `<details class="more"><summary title="older settled items">+5</summary>`)
	if more < 0 {
		t.Fatalf("nested +5 details missing: %s", block)
	}
	for id := 52; id < 60; id++ {
		at := strings.Index(block, fmt.Sprintf(`id="item-%d"`, id))
		if at < 0 {
			t.Errorf("settled item %d missing", id)
		} else if collapsed := id < 57; collapsed != (at > more) {
			t.Errorf("settled item %d collapsed=%v but sits at %d against +N at %d", id, collapsed, at, more)
		}
	}

	embed := render(t, sampleView(LayoutEmbed))
	if !strings.Contains(embed, `<section class="topic quiet"><div class="tbar"><span class="tname">tool surface</span>`) {
		t.Errorf("embed: settled-only topic with nothing new is not quiet")
	}
}

func TestRenderPageNewBadgeOnlyOnEmbed(t *testing.T) {
	const badge = `<span class="new" title="new since the previous embed"></span>`
	embed := render(t, sampleView(LayoutEmbed))
	for _, id := range []int{77, 78, 76, 72} {
		at := strings.Index(embed, fmt.Sprintf(`id="item-%d"`, id))
		end := strings.Index(embed[at:], "</div>")
		if at < 0 || !strings.Contains(embed[at:at+end], badge) {
			t.Errorf("embed: item %d lacks the new badge", id)
		}
	}
	if !strings.Contains(embed, `<span class="chip fresh" title="new since the previous embed"><span class="new"></span>`) {
		t.Errorf("embed: fresh count chip missing")
	}

	view := sampleView(LayoutFull)
	for i := range view.Topics {
		for j := range view.Topics[i].Policy {
			view.Topics[i].Policy[j].New = true
		}
		view.Topics[i].Fresh = 3
	}
	full := render(t, view)
	if strings.Contains(full, `class="new"`) || strings.Contains(full, "chip fresh") {
		t.Errorf("full page shows new badges")
	}
}

func TestRenderPageReplyBar(t *testing.T) {
	view := sampleView(LayoutEmbed)
	if !view.BulkAck || !view.Answerable {
		t.Fatalf("sample board should be answerable with a bulk checkbox")
	}
	out := render(t, view)
	if !strings.Contains(out, `<input type="checkbox" id="ack-all"><svg class="i" aria-hidden="true"><use href="#i-check"/></svg>4</label>`) {
		t.Errorf("bulk checkbox missing or miscounted")
	}
	// Embed: reply bar right after the topics, panels collapsed after it.
	reply, lastTopic, panel := strings.Index(out, `<div class="reply`), strings.LastIndex(out, "</section>"), strings.Index(out, `<details class="panel"`)
	if !(lastTopic < reply && reply < panel) {
		t.Errorf("embed order: topics end %d, reply %d, panels %d", lastTopic, reply, panel)
	}

	view.BulkAck = false
	if out = render(t, view); strings.Contains(out, `id="ack-all"`) || !strings.Contains(out, `<div class="reply solo">`) {
		t.Errorf("bulk checkbox renders without BulkAck")
	}
	view.Answerable = false
	if out = render(t, view); strings.Contains(out, `id="reply-text"`) {
		t.Errorf("reply bar renders on a board with nothing to answer")
	}
}

func TestRenderPagePanels(t *testing.T) {
	full := render(t, sampleView(LayoutFull))
	for _, want := range []string{`<details class="panel" id="p-todo" open>`, `<details class="panel" id="p-agenda" open>`, `<aside class="side">`, `<li class="done" title="mockup"><span class="box">[x]</span>`, `<li class="keyed"><code>whiteboard.render</code>`} {
		if !strings.Contains(full, want) {
			t.Errorf("full page lacks %q", want)
		}
	}
	if strings.Index(full, `<aside class="side">`) < strings.Index(full, `<main>`) {
		t.Errorf("sidebar precedes the main column")
	}
	embed := render(t, sampleView(LayoutEmbed))
	for _, want := range []string{`<details class="panel" id="p-todo">`, `<details class="panel" id="p-agenda">`, `<div class="side flat">`} {
		if !strings.Contains(embed, want) {
			t.Errorf("embed lacks %q", want)
		}
	}
	if strings.Contains(embed, `<aside`) || strings.Contains(embed, `class="grid`) {
		t.Errorf("embed uses the two-column layout")
	}

	view := sampleView(LayoutFull)
	view.Panels = nil
	if out := render(t, view); !strings.Contains(out, `<div class="grid solo">`) || strings.Contains(out, "<aside") {
		t.Errorf("full page without panels still reserves a sidebar")
	}
}

func TestRenderPageLinkAnchors(t *testing.T) {
	out := render(t, sampleView(LayoutFull))
	if !strings.Contains(out, `follows <a href="#item-69">(69)</a>`) {
		t.Errorf("link to an item on the page is not an anchor")
	}
	if !strings.Contains(out, `see {{999}} in the archive`) || strings.Contains(out, `href="#item-999"`) {
		t.Errorf("link to an ID the store does not hold should stay verbatim")
	}
	if !strings.Contains(out, `pairs with <a href="#item-71">(71)</a>`) {
		t.Errorf("settled row link is not an anchor")
	}
	if regexp.MustCompile(`id="i-\d`).MatchString(out) {
		t.Errorf("item ids collide with the sprite namespace")
	}
}

// TestRenderPageItemReferences checks the {{nn}} resolution in every
// lead-authored field: an item on the page anchors as (nn), an item the store
// holds but the page omits renders as plain (nn), an ID the store does not
// hold stays verbatim, and prose parentheses and non-digit brace tokens are
// literal. The user's answer quote is never resolved.
func TestRenderPageItemReferences(t *testing.T) {
	st := &State{Seq: 3, SettleSeq: 2}
	st.Items = []Item{
		{ID: 1, Class: ClassTechnical, Topic: "t", Title: "target", Status: StatusOpen},
		{ID: 2, Class: ClassPolicy, Topic: "t", Title: "title {{01}} step (2)",
			Options: []string{"option {{01}}", "option {{40}}"}, Recommendation: "a: rec {{01}} {{my-tag}}",
			Rationale: "why {{01}}\nwhy {{99}}\nhuge {{99999999999999999999}}", Links: []string{"link {{01}}", "step (2)"}, Status: StatusOpen},
		{ID: 3, Class: ClassPolicy, Topic: "t", Title: "assumed {{01}}", Options: []string{"x {{01}}", "y"},
			Status: StatusAssumed, Answer: "a: reading {{01}}", AnswerQuote: "quote {{01}}"},
		{ID: 4, Class: ClassTechnical, Topic: "t", Title: "settled {{01}}", Status: StatusAnswered,
			Answer: "free reading {{01}}", AnswerQuote: "said {{01}}", Settled: 1},
		{ID: 5, Class: ClassTechnical, Topic: "t", Title: "ack", Rationale: "ack why {{01}}", Status: StatusOpen},
		{ID: 6, Class: ClassPolicy, Topic: "t", Title: "unanswered-letter {{01}}", Options: []string{"p", "q"},
			Status: StatusAssumed, Answer: "plain {{01}}", AnswerQuote: "q"},
	}
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	// The store holds (40) but no topic renders it: an archived settled item.
	view.Stored[40] = true
	out := render(t, view)

	anchor := `<a href="#item-1">(01)</a>`
	for _, want := range []string{
		"title " + anchor + " step (2)", // title, with literal prose parentheses
		"option " + anchor,              // option text
		"rec " + anchor + " {{my-tag}}", // recommendation, with a verbatim brace token
		"why " + anchor,                 // rationale
		"why {{99}}",                    // an ID the store does not hold
		"huge {{99999999999999999999}}", // an ID past int range stays verbatim
		"link " + anchor,                // links
		"step (2)</span>",               // a prose link
		"assumed " + anchor,             // assumed title
		"x " + anchor,                   // static option text
		"reading " + anchor,             // assumed answer reading after the letter
		"plain " + anchor,               // assumed answer reading without a letter
		"settled " + anchor,             // settled title
		"free reading " + anchor,        // settled answer reading
		"ack why " + anchor,             // ack rationale
		"option (40)",                   // archived item: plain (nn)
		"quote {{01}}",                  // answer quote stays literal
		"said {{01}}",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page lacks %q", want)
		}
	}
	if strings.Contains(out, `href="#item-40"`) || strings.Contains(out, `href="#item-99"`) {
		t.Errorf("a reference to an item off the page became an anchor")
	}
}

func TestRenderPageItemReferenceKeepsEscaping(t *testing.T) {
	st := &State{Seq: 2}
	st.Items = []Item{
		{ID: 1, Class: ClassTechnical, Topic: "t", Title: `<b>x</b>{{01}}<script>s()</script>`,
			Rationale: `<i>r</i> {{01}}`, Links: []string{`<em>l</em>{{01}}`}, Status: StatusOpen},
	}
	out := render(t, BuildView(st, ViewInput{Layout: LayoutEmbed, ReplyID: "r"}))
	for _, want := range []string{
		`&lt;b&gt;x&lt;/b&gt;<a href="#item-1">(01)</a>&lt;script&gt;s()&lt;/script&gt;`,
		`&lt;i&gt;r&lt;/i&gt; <a href="#item-1">(01)</a>`,
		`&lt;em&gt;l&lt;/em&gt;<a href="#item-1">(01)</a>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page lacks %q", want)
		}
	}
	if strings.Contains(out, "<script>s()") {
		t.Errorf("item text next to a reference is not escaped")
	}
}

func TestRenderPageEmbedSizeBudget(t *testing.T) {
	long := strings.Repeat("A fairly long sentence about the trade-off, written the way a lead explains it. ", 4)
	st := &State{Seq: 30, NowSeq: 30, LastEmbedSeq: 20, Now: sampleNow}
	settle := 0
	for i := 1; i <= 60; i++ {
		item := Item{ID: i, Topic: fmt.Sprintf("topic %d", i%6), Title: fmt.Sprintf("Item %d: %s", i, long[:120]), Rationale: long, Links: []string{fmt.Sprintf("see {{%d}}", max(1, i-1))}, Changed: 15 + i%16}
		switch i % 4 {
		case 0:
			item.Class, item.Status = ClassPolicy, StatusOpen
			item.Options, item.Recommendation = []string{long[:90], long[:140], long[:60], long}, "b: "+long[:100]
		case 1:
			item.Class, item.Status = ClassTechnical, StatusOpen
		case 2:
			item.Class, item.Status, item.Answer, item.AnswerQuote = ClassAnnounced, StatusAssumed, "ok", long[:80]
		default:
			settle++
			item.Class, item.Status, item.Answer, item.AnswerQuote, item.Settled = ClassTechnical, StatusAnswered, "ok", long[:80], settle
		}
		st.Items = append(st.Items, item)
	}
	panels := samplePanels()
	rows := make([]PanelRow, 20)
	for i := range rows {
		rows[i] = PanelRow{Label: fmt.Sprintf("key.%d", i), Text: long[:100]}
	}
	panels = append(panels, Panel{ID: "extra", Title: "Extra", Icon: "board", Count: "20", Rows: rows})

	out := render(t, BuildView(st, ViewInput{Layout: LayoutEmbed, ReplyID: "size-check", Panels: panels}))
	const budget = 512 << 10
	t.Logf("populated embed: %d bytes (%.1f%% of 512 KiB)", len(out), 100*float64(len(out))/budget)
	if len(out) > budget/4 {
		t.Errorf("populated embed is %d bytes, not well under 512 KiB", len(out))
	}
}
