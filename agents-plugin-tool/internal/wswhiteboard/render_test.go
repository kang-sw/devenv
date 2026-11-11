package wswhiteboard

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

const sampleDoc = `<p>One update writes the full page and an embed; the embed keeps the latest five.</p>
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
// several topics, one of them closed, and no topic body. The previous embed
// was at seq 10.
func sampleState() *State {
	st := &State{Counter: 90, Seq: 12, LastEmbedSeq: 10, SettleSeq: 21, ClosedTopics: []string{"초기 범위"}}
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
		Status: StatusAssumed, Answer: "b", AnswerQuote: "71-76 look good", Settled: 21, Changed: 11})
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
	// The page carries its own seq so the poller compares every stamp, the
	// first included, against the version it was rendered at.
	if !strings.Contains(full, `<html data-stamp="barrette-regain-lumpiness.stamp.js" data-seq="12">`) {
		t.Errorf("full page lacks the data-stamp and data-seq attributes")
	}
	for _, want := range []string{"window.wbStamp", `getAttribute("data-seq")`, "v !== ownSeq", "setInterval(poll, 1500)", `id="wb-banner"`, `"wb-scroll"`, `"wb-open-"`} {
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
	for _, banned := range []string{"data-stamp", "data-seq", "wbStamp", "sessionStorage", "wb-banner", "location.reload"} {
		if strings.Contains(embed, banned) {
			t.Errorf("embed carries %q", banned)
		}
	}
}

// docTag finds the topic body's <details> tag on a page.
var docTag = regexp.MustCompile(`<details class="tdoc"[^>]*>`)

// TestRenderPageTopicBodyPlacement pins where a topic body sits: inside its
// topic block, between the title bar and the items, open on the full page
// unless the topic is closed, and open on the embed only when it changed
// since the previous embed.
func TestRenderPageTopicBodyPlacement(t *testing.T) {
	for name, layout := range layouts() {
		st := sampleState()
		st.Topics = []TopicDoc{{Name: "embed layout", Body: "layout **notes**", Changed: 12}, {Name: "초기 범위", Body: "closed notes", Changed: 3}, {Name: "tool surface", Body: "old notes", Changed: 4}}
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r"}))
		start := strings.Index(out, `<span class="tname">embed layout</span>`)
		block := out[start:]
		block = block[:strings.Index(block, "</section>")]
		doc, items := strings.Index(block, `<details class="tdoc"`), strings.Index(block, `class="tbody"`)
		if doc < 0 || items < 0 || doc > items {
			t.Errorf("%s: the body (%d) is not above the items (%d)", name, doc, items)
		}
		wantAll(t, block, `<summary title="topic notes"><svg class="i" aria-hidden="true"><use href="#i-book"/></svg>`, `<div class="doc-body md">layout <strong>notes</strong></div>`)
		if tag := docTag.FindString(block); tag != `<details class="tdoc" data-key="doc:embed layout" open>` {
			t.Errorf("%s: body tag = %q, want it open (changed at seq 12, after the embed at 10)", name, tag)
		}
		// A body that did not change since the previous embed is open only on the full page.
		start = strings.Index(out, `<span class="tname">tool surface</span>`)
		block = out[start:]
		block = block[:strings.Index(block, "</section>")]
		tag := docTag.FindString(block)
		wantOpen := layout == LayoutFull
		if open := strings.HasSuffix(tag, " open>"); open != wantOpen {
			t.Errorf("%s: unchanged body tag %q, open = %v, want %v", name, tag, open, wantOpen)
		}
		// A closed topic folds its body on both pages.
		start = strings.Index(out, `<span class="tname" title="closed">초기 범위</span>`)
		if start < 0 {
			t.Fatalf("%s: closed topic missing", name)
		}
		block = out[start:]
		block = block[:strings.Index(block, "</section>")]
		if tag := docTag.FindString(block); tag != `<details class="tdoc" data-key="doc:초기 범위">` {
			t.Errorf("%s: closed topic's body tag = %q, want a closed fold", name, tag)
		}
		if !strings.Contains(block, "closed notes") {
			t.Errorf("%s: a closed topic still carries its body", name)
		}
		// A topic with no body has no body block.
		start = strings.Index(out, `<span class="tname">embed retention</span>`)
		block = out[start:]
		block = block[:strings.Index(block, "</section>")]
		if strings.Contains(block, "tdoc") {
			t.Errorf("%s: a topic without a body renders one", name)
		}
	}
}

// TestRenderPageContentOnlyTopic: a topic with a body and no items renders
// its block with the body, and closing it makes the block quiet with the body
// in a closed fold.
func TestRenderPageContentOnlyTopic(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("notes", "# Plan\n\n- one"))
	out := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantAll(t, out, `<section class="topic has-doc" data-topic="notes"><div class="tbar"><span class="tlead"><span class="tname">notes</span></span>`,
		`<details class="tdoc" data-key="doc:notes" open>`, "<h1>Plan</h1>", "<li>one</li>")
	wantNone(t, out, `class="tbody"`)
	// Nothing to answer: the reply bar waits hidden for a topic note.
	wantAll(t, out, `<div class="reply solo" data-idle hidden>`)

	mustApply(t, st, `<wb:topic name="notes"><wb:close result="done"/></wb:topic>`)
	out = render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantAll(t, out, `<section class="topic quiet closed has-doc" data-topic="notes"><div class="tbar"><span class="tlead"><span class="tname" title="closed">notes</span><span class="tresult" title="how the topic ended">done</span></span>`,
		`<details class="tdoc" data-key="doc:notes"><summary`, "<h1>Plan</h1>")
	wantNone(t, out, `data-key="doc:notes" open`)
}

// TestRenderPageTopicKeyAttribute: every topic section carries its name as
// data-topic, escaped as an attribute value, the key a live page matches
// topics by across board swaps.
func TestRenderPageTopicKeyAttribute(t *testing.T) {
	for name, layout := range layouts() {
		st := sampleState()
		st.Topics = []TopicDoc{{Name: `say "hi" & <go>`, Body: "notes", Changed: 12}}
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r"}))
		if !strings.Contains(out, `<section class="topic has-doc" data-topic="say &#34;hi&#34; &amp; &lt;go&gt;"><div class="tbar">`) {
			t.Errorf("%s: topic key attribute missing or unescaped", name)
		}
		if strings.Count(out, `<section class="topic`) != len(regexp.MustCompile(`<section class="topic[^"]*" data-topic="`).FindAllString(out, -1)) {
			t.Errorf("%s: a topic section without data-topic", name)
		}
	}
}

// TestRenderPageTopicBodyToggleBelongsToTheBar pins the toggle's hook: a topic
// with a body is marked has-doc (the bar reserves room for the button) and the
// stylesheet pins the summary onto the bar instead of giving it a row; a
// topic without a body carries neither the mark nor a details.
func TestRenderPageTopicBodyToggleBelongsToTheBar(t *testing.T) {
	for name, layout := range layouts() {
		st := sampleState()
		st.Topics = []TopicDoc{{Name: "embed layout", Body: "layout notes", Changed: 12}}
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r"}))
		wantAll(t, out, `<section class="topic has-doc" data-topic="embed layout"><div class="tbar"><span class="tlead"><span class="tname">embed layout</span></span>`,
			`.topic.has-doc > .tbar { padding-right:`, `details.tdoc > summary { position: absolute;`)
		start := strings.Index(out, `<span class="tname">embed retention</span>`)
		if start < 0 {
			t.Fatalf("%s: topic missing", name)
		}
		if open := strings.LastIndex(out[:start], `<section class="topic`); strings.Contains(out[open:start], "has-doc") {
			t.Errorf("%s: a topic without a body is marked has-doc", name)
		}
	}
}

// TestRenderPageEmbedMarksANewBody: the embed's body summary carries the new
// dot only when the body changed since the previous embed.
func TestRenderPageEmbedMarksANewBody(t *testing.T) {
	st := &State{Seq: 12, LastEmbedSeq: 10, Topics: []TopicDoc{{Name: "fresh", Body: "f", Changed: 12}, {Name: "stale", Body: "s", Changed: 5}}}
	embed := render(t, BuildView(st, ViewInput{Layout: LayoutEmbed, ReplyID: "r"}))
	wantAll(t, embed,
		`data-key="doc:fresh" open><summary title="topic notes"><svg class="i" aria-hidden="true"><use href="#i-book"/></svg><span class="new" title="new since the previous embed"></span></summary>`,
		`data-key="doc:stale"><summary title="topic notes"><svg class="i" aria-hidden="true"><use href="#i-book"/></svg></summary>`)
	full := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantNone(t, full, `class="new"`)
	wantAll(t, full, `data-key="doc:fresh" open>`, `data-key="doc:stale" open>`)
}

func TestRenderPageEscapesItemTextButNotTopicBody(t *testing.T) {
	st := &State{Seq: 2, Topics: []TopicDoc{{Name: "t", Body: `<b id="doc-raw">raw <i>doc</i></b>`}}}
	st.Items = []Item{
		{ID: 1, Class: ClassPolicy, Topic: "<script>t()</script>", Title: "<script>alert(1)</script>",
			Options: []string{`<img src=x onerror=alert(2)>`, "b"}, Recommendation: "a: <u>why</u>",
			Rationale: "<script>r()</script>", Links: []string{`<a href="javascript:x">{{2}}</a>`}, Status: StatusOpen},
		{ID: 2, Class: ClassTechnical, Topic: "t", Title: `"quoted" & <tag>`, Status: StatusAnswered, Answer: "ok", AnswerQuote: "</div><script>q()</script>", Settled: 1},
	}
	panels := []Panel{{ID: `x"><script>p()</script>`, Title: "<script>pt()</script>", Icon: `"><script>`, Rows: []PanelRow{{State: `"><b>`, Label: "<l>", Text: "<script>pr()</script>"}}}}
	for name, layout := range layouts() {
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r", Panels: panels}))
		for _, raw := range []string{"<script>alert(1)", "<script>t()", "<img src=x", "<script>r()", "<u>why</u>", `href="javascript`, "<script>q()", "<script>p()", "<script>pt()", "<script>pr()", "<tag>"} {
			if strings.Contains(out, raw) {
				t.Errorf("%s: item text %q is not escaped", name, raw)
			}
		}
		// Item text is markdown now: goldmark escapes a quote as &quot;, not &#34;.
		for _, escaped := range []string{"&lt;script&gt;alert(1)&lt;/script&gt;", "&quot;quoted&quot; &amp; &lt;tag&gt;"} {
			if !strings.Contains(out, escaped) {
				t.Errorf("%s: escaped text %q missing", name, escaped)
			}
		}
		if !strings.Contains(out, `<b id="doc-raw">raw <i>doc</i></b>`) {
			t.Errorf("%s: topic body is not written unescaped", name)
		}
		if !strings.Contains(out, `<use href="#i-list"/>`) {
			t.Errorf("%s: unknown panel icon did not fall back to the list symbol", name)
		}
		// Its own, after the client string table every page carries.
		if got := strings.Count(out, "<script>"); got != 2 {
			t.Errorf("%s: page holds %d script elements, want the string table and its own", name, got)
		}
	}
}

// docBody renders a board whose topic "doc" holds body, with item 90 on the
// page for references, and returns that body's inner HTML and the page.
func docBody(t *testing.T, body string, in ViewInput) (inner, page string) {
	t.Helper()
	st := &State{Seq: 2, Topics: []TopicDoc{{Name: "doc", Body: body, Changed: 2}}, Items: []Item{{ID: 90, Class: ClassTechnical, Topic: "t", Title: "target", Status: StatusOpen}}}
	in.ReplyID = "r"
	page = render(t, BuildView(st, in))
	const open = `<div class="doc-body md">`
	start := strings.Index(page, open)
	end := strings.Index(page, "</div></details>")
	if start < 0 || end < start {
		t.Fatalf("no topic body in:\n%s", page)
	}
	return page[start+len(open) : end], page
}

// TestTopicBodyRendersMarkdownWithInlineHTML pins the body's renderer: block
// markdown as in item fields, mermaid included, with raw HTML and inline SVG
// written as is, on the disk board and the served page alike.
func TestTopicBodyRendersMarkdownWithInlineHTML(t *testing.T) {
	body := "# Plan\n\n- one\n- **two**\n\n```mermaid\ngraph TD; A-->B\n```\n\n" + sampleDoc
	for name, in := range map[string]ViewInput{
		"disk":   {Layout: LayoutFull},
		"embed":  {Layout: LayoutEmbed},
		"served": {Layout: LayoutFull, FileText: func(p string) string { return p }, ImageSrc: func(string) string { return "" }},
	} {
		inner, page := docBody(t, body, in)
		wantAll(t, inner, "<h1>Plan</h1>", "<li><strong>two</strong></li>", `<pre class="mermaid">graph TD; A--&gt;B`, sampleDoc)
		if !UsesMermaid(page) {
			t.Errorf("%s: a mermaid block in a body does not load the mermaid script", name)
		}
	}
	// A page without a mermaid block loads no mermaid script.
	if _, page := docBody(t, "plain", ViewInput{Layout: LayoutFull}); UsesMermaid(page) {
		t.Errorf("a body without a mermaid block loads mermaid")
	}
	st := &State{Seq: 2, Topics: []TopicDoc{{Name: "doc", Body: body, Changed: 2}}}
	regions, err := RenderRegions(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	if err != nil {
		t.Fatal(err)
	}
	// The served board renders the same body inside its frame, which loads
	// mermaid itself, so the host page need not.
	doc := frameDocs(t, string(regions.Board))["doc"]
	mermaid, _ := MermaidAsset()
	wantAll(t, doc, "<h1>Plan</h1>", sampleDoc, `<pre class="mermaid">`, `<script src="`+mermaid+`"></script>`)
	if regions.Mermaid {
		t.Errorf("a mermaid block inside a body frame makes the host page load mermaid")
	}
}

// TestTopicBodyKeepsInlineScript pins the body's trust: a canvas drawn by an
// inline script reaches the page unchanged, blank lines inside the script
// included, since a script block runs to its closing tag. The canvas line,
// markdown text with an inline tag, gains a paragraph.
func TestTopicBodyKeepsInlineScript(t *testing.T) {
	script := "<script>\nconst g = document.getElementById(\"c\").getContext(\"2d\");\n\ng.fillRect(0, 0, 10, 10);\n</script>"
	body, page := docBody(t, "<canvas id=\"c\" width=\"120\" height=\"40\"></canvas>\n"+script, ViewInput{Layout: LayoutFull})
	if want := "<p><canvas id=\"c\" width=\"120\" height=\"40\"></canvas></p>\n" + script; body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
	if got := strings.Count(page, "<script>"); got != 3 {
		t.Errorf("page holds %d script elements, want the string table, its own and the body's", got)
	}
}

// TestTopicBodyHTMLCaveats pins the rules the whiteboard playbook states for
// HTML in a body: a block tag keeps the lines after it raw up to a blank
// line, an indented line after a blank line becomes a code block, a fragment
// that starts with an inline tag is markdown text whose line breaks render
// as <br>, and {{nn}} resolves in markdown text but not inside HTML.
func TestTopicBodyHTMLCaveats(t *testing.T) {
	body, _ := docBody(t, "<div>\n**raw**\n</div>\n\n**md**", ViewInput{Layout: LayoutFull})
	wantAll(t, body, "<div>\n**raw**\n</div>", "<strong>md</strong>")

	body, _ = docBody(t, "<svg viewBox=\"0 0 10 10\">\n<rect width=\"4\" height=\"4\"/>\n\n    <text>x</text>\n</svg>", ViewInput{Layout: LayoutFull})
	wantAll(t, body, "<pre><code>&lt;text&gt;x&lt;/text&gt;")

	body, _ = docBody(t, "<b>Status</b> green\n<i>next</i> deploy", ViewInput{Layout: LayoutFull})
	if body != "<b>Status</b> green<br>\n<i>next</i> deploy" {
		t.Errorf("inline-tag fragment = %q", body)
	}

	body, _ = docBody(t, "See {{90}}\n\n<div title=\"{{90}}\">{{90}}</div>", ViewInput{Layout: LayoutFull})
	wantAll(t, body, `See <a href="#item-90">(90)</a>`, `<div title="{{90}}">{{90}}</div>`)

	// The body a lead writes in the command string reaches the page as written.
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "x")+"<wb:topic name=\"t\"><wb:body>\n## 배경\n\n<details><summary>접기</summary>\n<p>1 < 2 & 3 > 2</p>\n</details>\n</wb:body></wb:topic>")
	page := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantAll(t, page, "<h2>배경</h2>", "<details><summary>접기</summary>\n<p>1 < 2 & 3 > 2</p>\n</details>")
}

func TestRenderPageTopicBlockOrder(t *testing.T) {
	out := render(t, sampleView(LayoutFull))
	start := strings.Index(out, `<span class="tname">embed layout</span>`)
	if start < 0 {
		t.Fatal("embed layout topic missing")
	}
	block := out[start:]
	block = block[:strings.Index(block, "</section>")]
	// The assumed item is the newest settled row, above the older ones.
	policy, list, assumed, settled := strings.Index(block, `id="item-73"`), strings.Index(block, `<details class="settled"`), strings.Index(block, `id="item-76"`), strings.Index(block, `id="item-72"`)
	if !(policy >= 0 && policy < list && list < assumed && assumed < settled) {
		t.Errorf("block order policy %d, settled list %d, assumed %d, settled %d", policy, list, assumed, settled)
	}
	if !strings.Contains(block, `<span class="chip policy" title="to answer">`) || strings.Contains(block, "chip assumed") {
		t.Errorf("title bar chips: want the policy count and no assumed count")
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
		// a settled policy item shows its picked option
		`<span class="pick" title="picked"><span class="k">a</span><span class="pt">ai-docs/whiteboards/</span></span>`,
		`<input type="checkbox" data-id="78">`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("full page lacks %q", want)
		}
	}
	// one note button per waiting item, (77), (78), (73), (80), (79), and one
	// on the assumed settled row (76)
	if got := strings.Count(out, `<button class="btn mini note-toggle" title="note">`); got != 6 {
		t.Errorf("%d note buttons, want one per waiting item and assumed row (6)", got)
	}
}

// An assumed item renders as a settled row: the picked-option chip and the
// lead's reading, a small assumed marker, and a pen whose note becomes the
// user's veto or comment. It counts nowhere as waiting: no header or topic
// chip, no bulk checkbox, and its topic is quiet once nothing else waits.
func TestRenderPageAssumedItemIsASettledRow(t *testing.T) {
	out := render(t, sampleView(LayoutFull))
	wantAll(t, out,
		`<div class="srow" id="item-76" data-id="76"><details class="sx" data-key="item:76"><summary><span class="stext"><span class="id">(76)</span><span class="amark" title="assumed, confirm or correct">assumed</span><b>Recommendation shows as a star plus a line</b> <span class="pick" title="picked"><span class="k">b</span>`,
		`“71-76 look good”</span></span></summary>`,
		// The pen and note follow the row's details as siblings, outside its
		// toggle path, so they show and work while the row is closed.
		`</div></details><button class="btn mini note-toggle" title="note"><svg class="i" aria-hidden="true"><use href="#i-pen"/></svg></button><textarea class="note" rows="1" placeholder="✎" title="correction" hidden></textarea></div>`)
	wantNone(t, out, "card assumed", "chip assumed", `data-v="ok"`)
	if got := strings.Count(out, `class="amark"`); got != 1 {
		t.Errorf("%d assumed markers, want 1", got)
	}
	// A settled row that is not assumed has neither a marker nor a pen.
	wantAll(t, out, `<div class="srow" id="item-72"><span class="stext"><span class="id">(72)</span><b>`)

	// A topic holding only an assumed item waits on nothing: the item rotates
	// with the settled rows, the header carries no count, and once closed the
	// topic is quiet and the row keeps its pen.
	st := &State{}
	mustApply(t, st, addXML(ClassPolicy, "t", "p", "x", "y"))
	mustApply(t, st, `<wb:answer id="1" value="a" assumed="true">sure</wb:answer>`)
	open := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	if topic := open.Topics[0]; len(topic.Policy) != 0 || len(topic.Acks) != 0 || len(topic.Expanded) != 1 {
		t.Fatalf("assumed-only topic = %+v", topic)
	}
	if open.BulkAck || open.Answerable {
		t.Fatalf("an assumed item made the board answerable: %+v", open)
	}
	mustApply(t, st, `<wb:topic name="t"><wb:close result="done"/></wb:topic>`)
	if got := item(t, st, 1).Status; got != StatusAssumed {
		t.Fatalf("close changed the assumed item to %s", got)
	}
	closed := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantAll(t, closed, `<section class="topic quiet closed" data-topic="t">`,
		`<div class="srow" id="item-1" data-id="01">`,
		`<button class="btn mini note-toggle" title="note"><svg class="i" aria-hidden="true"><use href="#i-pen"/></svg></button><textarea class="note" rows="1" placeholder="✎" title="correction" hidden></textarea></div>`,
		`<div class="reply solo" data-idle hidden>`)
	head := closed[strings.Index(closed, `<div class="top">`):]
	head = head[:strings.Index(head, "</div>")]
	wantNone(t, head, "chip")
}

// A settled row with more to show unfolds to its original item: a details
// keyed item:<ID> whose summary is the compact row and whose body holds the
// rationale, every option (static: no answer-control attributes, the pick
// marked by a class), and the recommendation. Links stay in the compact row
// only. A row with nothing more to show stays plain.
func TestRenderPageSettledRowExpands(t *testing.T) {
	st := &State{Counter: 9, Seq: 3, SettleSeq: 5, Items: []Item{
		{ID: 1, Class: ClassPolicy, Topic: "t", Title: "where", Options: []string{"here", "there", "nowhere"},
			Recommendation: "b: closer", Rationale: "context **why**", Links: []string{"see {{2}}"},
			Status: StatusAnswered, Answer: "b", AnswerQuote: "fine", Settled: 1, Changed: 2},
		{ID: 2, Class: ClassPolicy, Topic: "t", Title: "gone", Options: []string{"x", "y"}, Status: StatusWithdrawn, Settled: 2, Changed: 2},
		{ID: 3, Class: ClassTechnical, Topic: "t", Title: "tech", Rationale: "because `x`", Status: StatusAnswered, Answer: "ok", Settled: 3, Changed: 2},
		{ID: 4, Class: ClassAnnounced, Topic: "t", Title: "bare", Links: []string{"only a link"}, Status: StatusAnswered, Answer: "ok", Settled: 4, Changed: 2},
	}}
	for name, layout := range layouts() {
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r"}))
		row := func(id int) string {
			at := strings.LastIndex(out[:max(0, strings.Index(out, fmt.Sprintf(`" id="item-%d"`, id)))], `<div class="srow`)
			if at < 0 {
				t.Fatalf("%s: settled row %d missing", name, id)
			}
			r := out[at:]
			r = r[:strings.Index(r, "</section>")]
			if next := strings.Index(r[1:], `<div class="srow`); next >= 0 {
				r = r[:next+1]
			}
			return r
		}

		policy := row(1)
		wantAll(t, policy,
			`<div class="srow" id="item-1"><details class="sx" data-key="item:1"><summary><span class="stext">`,
			`<span class="link"><svg class="i" aria-hidden="true"><use href="#i-link"/></svg>see <a href="#item-2">(02)</a></span></span></span></summary><div class="sbody">`,
			`<div class="sbody"><div class="ctx md">context <strong>why</strong></div><div class="opts">`,
			`<span class="opt static"><span class="k">a</span><span>here</span></span>`,
			`<span class="opt static rec picked"><span class="k">b<svg class="i" aria-hidden="true"><use href="#i-star"/></svg></span><span>there</span></span>`,
			`<span class="opt static"><span class="k">c</span><span>nowhere</span></span>`,
			`<div class="rec-line md" title="recommendation"><b><svg class="i" aria-hidden="true"><use href="#i-star"/></svg>b</b> closer</div></div></details></div>`)
		wantNone(t, policy, "data-opt", "aria-pressed", "<button", `role="button"`, "data-id")
		if got := strings.Count(policy, "#i-link"); got != 1 {
			t.Errorf("%s: links render %d times in the row, want once in the compact row", name, got)
		}

		void := row(2)
		wantAll(t, void, `<details class="sx" data-key="item:2">`,
			`<span class="opt static"><span class="k">a</span><span>x</span></span><span class="opt static"><span class="k">b</span><span>y</span></span>`)
		wantNone(t, void, "picked", "rec-line")

		wantAll(t, row(3), `<details class="sx" data-key="item:3">`, `<div class="sbody"><div class="ctx md">because <code>x</code></div></div></details>`)

		bare := row(4)
		wantAll(t, bare, `<div class="srow" id="item-4"><span class="stext">`, "only a link")
		wantNone(t, bare, "<details", "sbody")
	}
}

// A settled row's picked-option chip is a flex row; inline code and an item
// link in the option must stay inside one text item, not become flex items of
// their own that wrap into narrow columns.
func TestRenderPagePickedChipKeepsInlineMarkupInOneTextItem(t *testing.T) {
	st := &State{Counter: 9, Seq: 3, SettleSeq: 5, Items: []Item{
		{ID: 1, Class: ClassTechnical, Topic: "t", Title: "rules", Status: StatusAnswered, Answer: "ok", Settled: 1, Changed: 2},
		{ID: 2, Class: ClassPolicy, Topic: "t", Title: "tools", Options: []string{"two `tracker.query` tools per {{1}} rule", "b"},
			Status: StatusAnswered, Answer: "a", Settled: 2, Changed: 2},
	}}
	out := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	want := `<span class="pick" title="picked"><span class="k">a</span><span class="pt">two <code>tracker.query</code> tools per <a href="#item-1">`
	if !strings.Contains(out, want) {
		t.Errorf("picked chip lacks one text container around inline markup, want %q", want)
	}
}

func TestRenderPageQuietAndCollapsedSettled(t *testing.T) {
	full := render(t, sampleView(LayoutFull))
	// The closed topic's settled items sit behind a collapsed chip in its one
	// title line.
	start := strings.Index(full, `<section class="topic quiet closed" data-topic="`)
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

	// tool surface: its two most recent settled items are among the ten
	// expanded board-wide (the assumed (76) rotates with them), the other six
	// sit in a nested collapsed +6.
	start = strings.Index(full, `<span class="tname">tool surface</span>`)
	block = full[start:]
	block = block[:strings.Index(block, "</section>")]
	if !strings.Contains(block, `<details class="settled" open><summary title="settled">`) {
		t.Errorf("expanded settled rows are not shown open")
	}
	more := strings.Index(block, `<details class="more"><summary title="older settled items">+6</summary>`)
	if more < 0 {
		t.Fatalf("nested +6 details missing: %s", block)
	}
	for id := 52; id < 60; id++ {
		at := strings.Index(block, fmt.Sprintf(`id="item-%d"`, id))
		if at < 0 {
			t.Errorf("settled item %d missing", id)
		} else if collapsed := id < 58; collapsed != (at > more) {
			t.Errorf("settled item %d collapsed=%v but sits at %d against +N at %d", id, collapsed, at, more)
		}
	}

	embed := render(t, sampleView(LayoutEmbed))
	if !strings.Contains(embed, `<section class="topic quiet" data-topic="tool surface"><div class="tbar"><span class="tlead"><span class="tname">tool surface</span></span>`) {
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
	// The bulk checkbox counts the open acknowledgements (78), (79), (80); an
	// assumed item reads as settled and is not among them.
	if !strings.Contains(out, `<input type="checkbox" id="ack-all"><svg class="i" aria-hidden="true"><use href="#i-check"/></svg>3</label>`) {
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
	if out = render(t, view); !strings.Contains(out, `<div class="reply solo" data-idle hidden>`) {
		t.Errorf("a board with nothing to answer does not keep its reply bar hidden for topic notes")
	}
}

func TestRenderPagePanels(t *testing.T) {
	full := render(t, sampleView(LayoutFull))
	for _, want := range []string{`<details class="panel" id="p-todo" open>`, `<details class="panel" id="p-agenda" open>`, `<aside class="side">`, `<li class="done" title="mockup"><span class="box"><svg class="i" aria-hidden="true"><use href="#i-box-done"/></svg></span>`, `<li class="keyed"><code>whiteboard.render</code>`} {
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
		"x " + anchor,                   // assumed row's picked option
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
		{ID: 1, Class: ClassTechnical, Topic: "t", Title: `<u>x</u>{{01}}<script>s()</script>`,
			Rationale: `<s>r</s> {{01}}`, Links: []string{`<em>l</em>{{01}}`}, Status: StatusOpen},
	}
	out := render(t, BuildView(st, ViewInput{Layout: LayoutEmbed, ReplyID: "r"}))
	for _, want := range []string{
		`&lt;u&gt;x&lt;/u&gt;<a href="#item-1">(01)</a>&lt;script&gt;s()&lt;/script&gt;`,
		`&lt;s&gt;r&lt;/s&gt; <a href="#item-1">(01)</a>`,
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

// TestPagesLeaveOutTheTrackerStrings: a whiteboard page, full or embed,
// never loads the dashboard's tracker renderer, so its client table carries
// every client string except the tracker's (js.tk.*).
func TestPagesLeaveOutTheTrackerStrings(t *testing.T) {
	st := &State{Seq: 1}
	for _, layout := range []Layout{LayoutFull, LayoutEmbed} {
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r"}))
		if !strings.Contains(out, `"js.ago"`) {
			t.Errorf("layout %v: the page lacks its client strings", layout)
		}
		if strings.Contains(out, `"js.tk.`) {
			t.Errorf("layout %v: the page carries the tracker strings", layout)
		}
	}
}

func TestRenderPageEmbedSizeBudget(t *testing.T) {
	long := strings.Repeat("A fairly long sentence about the trade-off, written the way a lead explains it. ", 4)
	st := &State{Seq: 30, LastEmbedSeq: 20, Topics: []TopicDoc{{Name: "topic 1", Body: sampleDoc, Changed: 30}, {Name: "topic 2", Body: sampleDoc, Changed: 5}}}
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
	// The embed's ticket panel lists every ready ticket with an absolute
	// file: link (an embed_dir outside the repository), and its mailbox
	// panel is one line.
	tickets := make([]PanelRow, 100)
	for i := range tickets {
		tickets[i] = PanelRow{Label: "ready", Text: long[:100], Link: fmt.Sprintf("file:///home/someone/projects/a-long-repository-name/ai-docs/tickets/ready/2611%02d-feat-a-fairly-long-ticket-stem-%d.md", i%30, i)}
	}
	panels = append(panels,
		Panel{ID: "tickets", Title: "Tickets", Icon: "board", Count: "100 ready", Rows: tickets},
		Panel{ID: "mailbox", Title: "Mailbox", Icon: "mail", Count: "3 unread", Rows: []PanelRow{{Label: "someone@worktree", Text: "3 unread"}}},
	)

	out := render(t, BuildView(st, ViewInput{Layout: LayoutEmbed, ReplyID: "size-check", Panels: panels}))
	const budget = 512 << 10
	t.Logf("populated embed: %d bytes (%.1f%% of 512 KiB)", len(out), 100*float64(len(out))/budget)
	// About a third of the limit: the stress is past any real board (60
	// items, 100 ready tickets), and the embed has no truncation to fall back
	// on. 35% left the topic bar's pen, note box, and updated time room;
	// 40% adds the settled rows' expanded bodies (each settled item's
	// rationale and options ride in the snapshot so it unfolds without script).
	if len(out) > budget*40/100 {
		t.Errorf("populated embed is %d bytes, not well under 512 KiB", len(out))
	}
}

// TestRenderPagePanelRowLinks pins a panel row's link: a relative path and
// a file: URL render as the provider built them (html/template would rewrite
// an untrusted file: URL to #ZgotmplZ), while any other scheme keeps the
// template's URL filtering, and a row without a link renders plain text.
func TestRenderPagePanelRowLinks(t *testing.T) {
	panels := []Panel{{ID: "tickets", Title: "Tickets", Icon: "board", Rows: []PanelRow{
		{Label: "ready", Text: "Relative", Link: "../tickets/ready/261106-feat-a.md"},
		{Label: "ready", Text: "Absolute", Link: "file:///home/u/my%20repo/ai-docs/tickets/ready/261106-feat-b.md"},
		{Label: "todo", Text: "Script", Link: "javascript:alert(1)"},
		{Label: "todo", Text: "Plain"},
		{State: "pending", Label: "k", Text: "Stateful", Link: "../tickets/todo/261106-feat-c.md"},
	}}}
	for name, layout := range map[string]Layout{"full": LayoutFull, "embed": LayoutEmbed} {
		out := render(t, BuildView(&State{}, ViewInput{Layout: layout, ReplyID: "r", Panels: panels}))
		for _, want := range []string{
			`<li class="keyed"><code>ready</code><span><a href="../tickets/ready/261106-feat-a.md">Relative</a></span></li>`,
			`<li class="keyed"><code>ready</code><span><a href="file:///home/u/my%20repo/ai-docs/tickets/ready/261106-feat-b.md">Absolute</a></span></li>`,
			`<a href="#ZgotmplZ">Script</a>`,
			`<li class="keyed"><code>todo</code><span>Plain</span></li>`,
			`<span class="box"><svg class="i" aria-hidden="true"><use href="#i-box"/></svg></span><span><a href="../tickets/todo/261106-feat-c.md">Stateful</a></span>`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: page lacks %q", name, want)
			}
		}
	}
}

// TestRenderPageTodoRowIcons pins the checklist boxes: each state shows its
// sprite symbol, never the old text marker, and folded rows sit behind a
// closed "done N" details after the open rows.
func TestRenderPageTodoRowIcons(t *testing.T) {
	panels := []Panel{{ID: "todo", Title: "Todos", Icon: "todo", Count: "1/4",
		Rows: []PanelRow{
			{State: "wip", Label: "a", Text: "Working"},
			{State: "pending", Label: "b", Text: "Waiting"},
			{State: "defer", Label: "c", Text: "Later"},
		},
		Folded: []PanelRow{{State: "done", Label: "d", Text: "Finished"}},
	}}
	out := render(t, BuildView(&State{}, ViewInput{Layout: LayoutFull, ReplyID: "r", Panels: panels}))
	for _, want := range []string{`href="#i-box-wip"`, `href="#i-box"`, `href="#i-box-defer"`, `href="#i-box-done"`,
		`<details class="folded"><summary class="chip" title="done">done 1</summary>`} {
		if !strings.Contains(out, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	for _, bad := range []string{"[x]", "[~]", "[>]", "[ ]"} {
		if strings.Contains(out, bad) {
			t.Errorf("page still renders text marker %q", bad)
		}
	}
	if strings.Index(out, "Working") > strings.Index(out, `class="folded"`) || strings.Index(out, `class="folded"`) > strings.Index(out, "Finished") {
		t.Errorf("order is not open rows, fold, done rows")
	}
	if strings.Contains(out, `class="folded" open`) {
		t.Errorf("done fold must be closed by default")
	}
}

// TestOpenQuestionPillIsOnTheFullPageOnly pins where the open-question pill
// script ships: the full page and the dashboard's shared form script carry
// it, the embed (a snapshot with no scrolling board) does not.
func TestOpenQuestionPillIsOnTheFullPageOnly(t *testing.T) {
	if out := render(t, sampleView(LayoutFull)); !strings.Contains(out, "data-wb-title") || !strings.Contains(out, ".oq-pill") {
		t.Errorf("full page lacks the open-question pill script or style")
	}
	if out := render(t, sampleView(LayoutEmbed)); strings.Contains(out, "data-wb-title") {
		t.Errorf("embed carries the open-question pill script")
	}
	assets, err := PageAssets()
	if err != nil || !strings.Contains(string(assets.FormScript), "data-wb-title") {
		t.Errorf("shared form script lacks the open-question pill (err %v)", err)
	}
}

// TestTopicRailIsOnTheFullPageOnly pins where the topic rail (the sticky
// current-topic bar and its jump capsules) ships: the full page and the
// dashboard's shared form script carry its script, the embed does not, and
// the shared stylesheet carries its rules.
func TestTopicRailIsOnTheFullPageOnly(t *testing.T) {
	if out := render(t, sampleView(LayoutFull)); !strings.Contains(out, "window.wbRail") || !strings.Contains(out, ".wb-rail-wrap") {
		t.Errorf("full page lacks the topic rail script or style")
	}
	if out := render(t, sampleView(LayoutEmbed)); strings.Contains(out, "window.wbRail") {
		t.Errorf("embed carries the topic rail script")
	}
	assets, err := PageAssets()
	if err != nil || !strings.Contains(string(assets.FormScript), "window.wbRail") {
		t.Errorf("shared form script lacks the topic rail (err %v)", err)
	}
}

// TestTopicRailBarCapsuleMotion pins the rail's bar-to-capsule motion
// (rail.js morph): it runs only without reduced motion and never on a
// run's first draw, reads its starting boxes before a redraw and animates
// after it, a stuck bar draws below the top capsule stack (z-index 1 under
// the wrapper's 3) so the card's end pushes it under the stack and a short
// topic straddling the stick line never covers the capsules, a stuck bar's
// rounded corners are filled with the page colour so the card scrolling
// under it does not show through them, both edges morph (the bottom stack's near
// capsule grows into a bar that leaves the stack, and a shown bar sinking
// behind it shrinks into the capsule), the copy that morphs stands out of
// flow in its edge's wrapper, and the rail's teardown drops both copies.
// The morph changes a solid copy by height alone: a capsule is as wide as
// its item's bar (no inset, the far one standing back by type and opacity
// only), the copy keeps the target's left edge and width while its top,
// height, corners, and fill move, the two contents take turns inside it
// with no fade of the copy itself, a top-edge bar box is cut to the part
// showing under the stack so the copy never covers the capsules, and a
// page-coloured plate stands in for a hidden stuck bar.
func TestTopicRailBarCapsuleMotion(t *testing.T) {
	assets, err := PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	js, css := string(assets.FormScript), string(assets.CSS)
	for _, want := range []string{
		`function motion() { return !!d.body.animate && !(window.matchMedia && matchMedia("(prefers-reduced-motion: reduce)").matches); }`,
		`var pre = motion() ? before(s) : null;`,
		"if (pre) after(pre);\n    drawn = !!s;",
		`if (!drawn || !s) return p;`,
		`if (bnear && !s.down.some(function (i) { return topics[i] === bnear; })) p.bot = capGhost(slots.bot.near);`,
		`else if (next && btns("bot").map(shown).indexOf(next) < 0) p.bot = barGhost(next, true);`,
		`["top", "bot"].forEach(function (e) {`,
		`var wrap = wraps[edge], w = wrap.getBoundingClientRect()`,
		`wrap.appendChild(el);`,
		"stuck = null;\n    dropGhost();\n  };",
		`el.style.left = to.left - w.left + "px";`,
		`el.style.width = to.width + "px";`,
		`{ top: from.top - w.top + "px", height: from.height + "px", borderRadius: bar ? a.radius : from.height / 2 + "px", backgroundColor: a.bg },`,
		`{ top: to.top - w.top + "px", height: to.height + "px", borderRadius: bar ? to.height / 2 + "px" : b.radius, backgroundColor: b.bg }`,
		`el.firstChild.animate([{ opacity: 1 }, { opacity: 0, offset: 0.5 }, { opacity: 0 }], { duration: DUR, fill: "forwards" });`,
		`el.lastChild.animate([{ opacity: 0 }, { opacity: 0, offset: 0.5 }, { opacity: 1 }], { duration: DUR, fill: "forwards" });`,
		`into.animate([{ opacity: 0 }, { opacity: 0 }], { duration: DUR });`,
		`var top = Math.max(r.top, lim), bottom = Math.max(r.bottom, lim);`,
		`if (p.top) p.top.r = shows(p.top.r);`,
		`if (!bar && edge === "top") to = shows(to);`,
		`if (!bar && into.classList.contains("wb-stuck")) {`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("the rail script lacks %q", want)
		}
	}
	for _, want := range []string{
		`.wb-rail .topic > .tbar.wb-stuck { box-shadow: var(--wb-shadow), 0 -6px 0 6px var(--wb-bg); }`,
		`.wb-rail .topic.quiet:has(> .tnote) > .tbar.wb-stuck { border-bottom-left-radius: 0; border-bottom-right-radius: 0; }`,
		`.wb-rail .topic > .tbar { position: sticky; top: var(--wb-rail-top, 0px); z-index: 1; }`,
		`.wb-rail-wrap.up { top: var(--wb-rail-inset, 0px); z-index: 3; }`,
		`.wb-rail-wrap .wb-rail-ghost { position: absolute;`,
		`.wb-rail-ghost > * { position: absolute; left: 0; top: 50%; width: 100%; margin: 0; transform: translateY(-50%); background: transparent;`,
		`.wb-cap { pointer-events: auto; display: flex; align-items: center; gap: 6px; height: 17px; margin: 0; padding: 0 10px;`,
		`.wb-cap.far { font-size: 10px; font-weight: 600; opacity: .6; }`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("the shared stylesheet lacks %q", want)
		}
	}
	for _, gone := range []string{"b.opacity = 0;", "translateY(-4px)", "margin: 0 18px", "margin: 0 6px; padding: 0 10px"} {
		if strings.Contains(js+css, gone) {
			t.Errorf("the rail still carries the old fading or inset capsule (%q)", gone)
		}
	}
}

// TestTopicRailRailsHostItems pins the rail's generic item hook: besides
// the topics it rails any board node the host marks with data-rail, in
// board order, leaves out an item with no height, takes a marked node's
// bar, capsule text, and capsule icon from data-rail-bar, data-rail-name,
// and data-rail-icon, keys the draw by item key so a topic and a marked
// node never collide, and gives a marked node's name the type size of its
// bar or capsule in the morph's copies.
func TestTopicRailRailsHostItems(t *testing.T) {
	assets, err := PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	js, css := string(assets.FormScript), string(assets.CSS)
	for _, want := range []string{
		`var ITEMS = "section.topic[data-topic], [data-rail]";`,
		`function barOf(t) { return t.querySelector(isTopic(t) ? ":scope > .tbar" : "[data-rail-bar]"); }`,
		`function keyOf(t) { return isTopic(t) ? t.getAttribute("data-topic") : "\u0001" + t.getAttribute("data-rail"); }`,
		`if (dirty) { found = Array.prototype.slice.call(box.querySelectorAll(ITEMS)); dirty = false; }`,
		`if (r.height) { topics.push(t); rects.push(r); }`,
		`var nm = t.querySelector("[data-rail-name]"), ic = t.querySelector("[data-rail-icon]");`,
		`bar: cur >= 0 ? barOf(topics[cur]) : null,`,
		`return topics[i] ? keyOf(topics[i]) : "";`,
		`var TN = ".tname, [data-rail-name], .cn";`,
		`<span class="ci" hidden></span><span class="cn"></span>`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("the rail script lacks %q", want)
		}
	}
	if strings.Contains(js, `querySelector(":scope > .tbar") : null`) || strings.Contains(js, "b.children[1]") {
		t.Error("the rail script still reads a topic's bar or a capsule's slots directly")
	}
	if !strings.Contains(css, ".wb-cap .ci { flex: none; display: inline-flex; }") {
		t.Error("the shared stylesheet does not lay out a capsule's icon")
	}
}

// TestTopicRailEndOfBoard pins the rail at the end of the board: a bar in
// view that the rest of the scroll cannot lift above the bottom stack's
// line (a short last item at max scroll) gets no bottom capsule, which
// would only cover it (and, with the bottom morph, shrink it into the
// capsule). The bottom wrapper's top margin is the board's bottom scroll
// margin: the bottom stack's reserve (rail.js BOT) plus a gap, so the last
// item can scroll fully clear of the stack.
func TestTopicRailEndOfBoard(t *testing.T) {
	assets, err := PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	js := string(assets.FormScript)
	if !strings.Contains(string(assets.CSS), ".wb-rail-wrap.dn { bottom: 0; margin-top: calc(14px + 2 * 17px + 3px + 4px + 16px); }") || !strings.Contains(js, "var CAP = 17, GAP = 3, PAD = 4, FADE = 14;") || !strings.Contains(js, "var BOT = FADE + 2 * CAP + GAP + PAD;") {
		t.Error("the board's bottom scroll margin is not the bottom stack's reserve plus a gap")
	}
	for _, want := range []string{
		"return Math.max(0, e.scrollHeight - e.clientHeight - e.scrollTop);",
		"var line = vb - replyOff - BOT, below = [], rest = scrollRest();",
		"if (rects[k].top >= line && !(rects[k].top < vb && rects[k].top - rest >= line)) below.push(k);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("the rail script lacks %q", want)
		}
	}
}

// A standalone page renders in the language its view names: the static text
// (tooltips, the folded count) comes from the language's table, the board's
// own content (titles, panel rows) stays as written, and the page carries the
// client table its scripts read. A view with no language is English.
func TestRenderPageFollowsViewLanguage(t *testing.T) {
	en := render(t, sampleView(LayoutFull))
	view := sampleView(LayoutFull)
	view.Lang = "ko"
	ko := render(t, view)

	for _, want := range []string{`title="` + wsi18n.T("ko", "chip.answer") + `"`, `title="` + wsi18n.T("ko", "wb.allok") + `"`} {
		if !strings.Contains(ko, want) {
			t.Errorf("Korean page lacks %q", want)
		}
	}
	for _, english := range []string{wsi18n.T("en", "chip.answer"), wsi18n.T("en", "wb.allok")} {
		if !strings.Contains(en, `title="`+english+`"`) {
			t.Errorf("English page lacks %q", english)
		}
		if strings.Contains(ko, `title="`+english+`"`) {
			t.Errorf("Korean page still carries the English %q", english)
		}
	}
	if !strings.Contains(ko, `lang: "ko"`) || !strings.Contains(en, `lang: "en"`) {
		t.Error("the page does not carry its language's client table")
	}
	for _, keep := range []string{"Tool surface decision 1", "첫 범위는 세션 보드 하나", "First mockup round"} {
		if !strings.Contains(ko, keep) {
			t.Errorf("Korean page lost board content %q", keep)
		}
	}
	if strings.Contains(ko, " lang=") {
		t.Error("the page sets a lang attribute")
	}
}
