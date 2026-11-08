package wswhiteboard

import (
	"html"
	"regexp"
	"strings"
	"testing"
)

// TestRenderRegionsMatchThePage pins the dashboard's reuse contract: the
// board and panel fragments are exactly what the full page renders for the
// same view, so a served page cannot drift from the static one. A topic body
// is the one difference, framed in the regions
// (TestServedTopicBodyRendersInASandboxedFrame), so this board has none.
func TestRenderRegionsMatchThePage(t *testing.T) {
	view := sampleView(LayoutFull)
	page := render(t, view)
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	if regions.Board == "" || regions.Panels == "" {
		t.Fatalf("empty regions: %+v", regions)
	}
	if !strings.Contains(page, string(regions.Board)) {
		t.Errorf("the page does not hold the board fragment:\n%.400s", regions.Board)
	}
	if !strings.Contains(page, string(regions.Panels)) {
		t.Errorf("the page does not hold the panels fragment:\n%.400s", regions.Panels)
	}
	if regions.CountPolicy == 0 {
		t.Errorf("counts not carried: %+v", regions)
	}
	assets, err := PageAssets()
	if err != nil || assets.CSS == "" || assets.Sprite == "" || assets.FormScript == "" || assets.MermaidInit == "" {
		t.Fatalf("shared assets missing: %v", err)
	}
}

// TestRenderRegionsMermaidFlag pins the mermaid flag: set only when the board
// holds a mermaid block.
func TestRenderRegionsMermaidFlag(t *testing.T) {
	st := &State{Seq: 1, Items: []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "x", Status: StatusOpen, Options: []string{"a", "b"},
		Rationale: "```mermaid\ngraph TD; A-->B\n```"}}}
	regions, err := RenderRegions(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	if err != nil {
		t.Fatal(err)
	}
	if !regions.Mermaid || !strings.Contains(string(regions.Board), `<pre class="mermaid">`) {
		t.Fatalf("mermaid flag not set: %+v", regions.Mermaid)
	}
	plain, _ := RenderRegions(sampleView(LayoutFull))
	if plain.Mermaid {
		t.Fatalf("mermaid flag set on a board without a mermaid block")
	}
}

// TestFileTextRendersFileTargetsAsPathText pins the served-page link rule:
// with FileText set, a file link keeps its label and shows the path as text,
// a file image shows its alt text and path, a file autolink shows its path,
// and none of them yields an href or src; http links and in-page anchors are
// unaffected.
func TestFileTextRendersFileTargetsAsPathText(t *testing.T) {
	st := &State{Seq: 2, SettleSeq: 1}
	st.Items = []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "files", Status: StatusOpen, Options: []string{"a", "b"},
		Rationale: "see [the plan](ai-docs/plan.md) and [ai-docs/x.md](ai-docs/x.md)\n\n" +
			"![diagram](shots/a.png)\n\n" +
			"<file:///abs/notes.md> and [abs](/abs/b.md) and [web](https://example.com/) and [anchor](#item-1)"}}
	text := func(p string) string { return "REL:" + strings.TrimPrefix(p, "/") }
	out := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", FileText: text,
		FileLink: func(p string) string { return "MUST-NOT-USE/" + p }}))
	wantAll(t, out,
		`the plan <code class="fpath">REL:ai-docs/plan.md</code>`,
		`<code class="fpath">REL:ai-docs/x.md</code>`,
		`<span class="img-text">diagram <code class="fpath">REL:shots/a.png</code></span>`,
		`<code class="fpath">REL:abs/notes.md</code>`,
		`abs <code class="fpath">REL:abs/b.md</code>`,
		`<a href="https://example.com/"`,
		`<a href="#item-1">anchor</a>`,
	)
	wantNone(t, out, "MUST-NOT-USE", "file:", `src="shots`, `href="ai-docs`, "<img")
	// A label equal to the path is not repeated.
	if strings.Count(out, "ai-docs/x.md") != 2 { // the label and the code span
		t.Errorf("a label equal to its path must show once beside the path text")
	}
}

// TestImageSrcServesFileImages pins the served-image rule: with FileText and
// ImageSrc set, a file image renders as <img src> carrying the ImageSrc
// result (its query and fragment survive escaping, the destination's own
// suffix is dropped), an image whose ImageSrc result is empty keeps the
// path text, and a file link still renders path text.
func TestImageSrcServesFileImages(t *testing.T) {
	st := &State{Seq: 2, SettleSeq: 1}
	st.Items = []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "files", Status: StatusOpen, Options: []string{"a", "b"},
		Rationale: "![diagram](shots/a.png?x=1#frag)\n\n![outside](/abs/b.png)\n\n[the plan](shots/a.png)"}}
	imageSrc := func(p string) string {
		if p == "shots/a.png" {
			return "/s/r/image?path=shots%2Fa.png"
		}
		return ""
	}
	out := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r",
		FileText: func(p string) string { return "REL:" + p }, ImageSrc: imageSrc}))
	wantAll(t, out,
		`<img src="/s/r/image?path=shots%2Fa.png" alt="diagram">`,
		`<span class="img-text">outside <code class="fpath">REL:/abs/b.png</code></span>`,
		`the plan <code class="fpath">REL:shots/a.png</code>`,
	)
	wantNone(t, out, "x=1", "#frag", `<a href="/s/`)
	if strings.Count(out, "<img") != 1 {
		t.Errorf("only the served image renders as <img>:\n%s", out)
	}
}

// TestPanelRowPathText pins a linkless row's path: shown after its text.
func TestPanelRowPathText(t *testing.T) {
	panels := []Panel{{ID: "tickets", Title: "Tickets", Icon: "board", Rows: []PanelRow{
		{Label: "ready", Text: "A ticket", Path: "ai-docs/tickets/ready/x.md"},
	}}}
	out := render(t, BuildView(&State{}, ViewInput{Layout: LayoutFull, ReplyID: "r", Panels: panels}))
	wantAll(t, out, `<span>A ticket <code class="fpath">ai-docs/tickets/ready/x.md</code></span>`)
}

// TestPanelLanesRenderCards pins the kanban lane markup: a lane header with
// name, count, and note; a card with a category chip, a title that links or
// carries its path as a tooltip, and a stem line; and an empty lane saying so.
func TestPanelLanesRenderCards(t *testing.T) {
	panels := []Panel{{ID: "tickets", Title: "Tickets", Icon: "board", Lanes: []PanelLane{
		{Name: "ready", Count: "2", Rows: []PanelRow{
			{Label: "epic", Text: "An epic", Sub: "261101-epic-x", Link: "../t/x.md"},
			{Label: "feat", Text: "Served", Sub: "261102-feat-y", Path: "ai-docs/tickets/ready/y.md"},
		}},
		{Name: "todo", Count: "10", Note: "latest 10 of 12"},
	}}}
	out := render(t, BuildView(&State{}, ViewInput{Layout: LayoutFull, ReplyID: "r", Panels: panels}))
	wantAll(t, out,
		`<span class="lname">ready</span><span class="chip">2</span>`,
		`<span class="lnote">latest 10 of 12</span>`,
		`<li class="kcard"><span class="ctitle"><span class="chip cat epic">epic</span><a href="../t/x.md">An epic</a></span><span class="csub" title="261101-epic-x">261101-epic-x</span></li>`,
		`<span class="ctitle" title="ai-docs/tickets/ready/y.md"><span class="chip cat feat">feat</span>Served</span>`,
		`<li class="none">none</li>`,
	)
	wantNone(t, out, `<code class="fpath">ai-docs/tickets/ready/y.md</code>`)
}

// TestFileActionsRenderActionLinks pins the served-page link-action rule:
// with FileActions set, a file link, a file autolink, and the path text of
// an image ImageSrc does not serve become anchors marked data-wl="file"
// that carry the action's file view href, relative path, and absolute
// path; a path the action gives no href (outside the session root) is an
// anchor with no href; an http(s) link is marked data-wl="url"; an in-page
// anchor stays unmarked; and an action href that is not a scheme-less path
// on the serving host is dropped.
func TestFileActionsRenderActionLinks(t *testing.T) {
	st := &State{Seq: 2, SettleSeq: 1}
	st.Items = []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "files", Status: StatusOpen, Options: []string{"a", "b"},
		Rationale: "see [the plan](ai-docs/plan.md) and [away](/elsewhere/x.md)\n\n" +
			"![diagram](shots/a.bin)\n\n" +
			"<file:///root/notes.md> and [web](https://example.com/) and [anchor](#item-1) and [bad](evil.md) and [net](host.md)"}}
	actions := func(p string) FileAction {
		switch p {
		case "/elsewhere/x.md":
			return FileAction{Abs: "/elsewhere/x.md"}
		case "evil.md":
			return FileAction{Href: "javascript:alert(1)", Rel: "evil.md", Abs: "/root/evil.md"}
		case "host.md":
			return FileAction{Href: "//evil.example/x", Rel: "host.md", Abs: "/root/host.md"}
		}
		rel := strings.TrimPrefix(p, "/root/")
		return FileAction{Href: "/s/r/file?path=" + rel, Rel: rel, Abs: "/root/" + rel}
	}
	out := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r",
		FileText:    func(p string) string { return "REL:" + strings.TrimPrefix(p, "/root/") },
		ImageSrc:    func(string) string { return "" },
		FileActions: actions}))
	wantAll(t, out,
		`<a href="/s/r/file?path=ai-docs/plan.md" data-wl="file" data-rel="ai-docs/plan.md" data-abs="/root/ai-docs/plan.md" title="REL:ai-docs/plan.md">the plan</a>`,
		`<a tabindex="0" data-wl="file" data-abs="/elsewhere/x.md" title="REL:/elsewhere/x.md">away</a>`,
		`<span class="img-text">diagram <a href="/s/r/file?path=shots/a.bin" data-wl="file" data-rel="shots/a.bin" data-abs="/root/shots/a.bin" title="REL:shots/a.bin"><code class="fpath">REL:shots/a.bin</code></a></span>`,
		`<a href="/s/r/file?path=notes.md" data-wl="file" data-rel="notes.md" data-abs="/root/notes.md" title="REL:notes.md">file:///root/notes.md</a>`,
		`<a href="https://example.com/" target="_blank" rel="noopener noreferrer" data-wl="url">web</a>`,
		`<a href="#item-1">anchor</a>`,
		`<a tabindex="0" data-wl="file" data-rel="evil.md" data-abs="/root/evil.md" title="REL:evil.md">bad</a>`,
		`<a tabindex="0" data-wl="file" data-rel="host.md" data-abs="/root/host.md" title="REL:host.md">net</a>`,
	)
	wantNone(t, out, "javascript:", "evil.example", "file:///root/notes.md\"", `<code class="fpath">REL:ai-docs/plan.md</code>`)
}

// TestPanelRowPathActionLink pins a served page's ticket row: with link
// actions the Text is the file action link, titled with the path, on a card
// and on a plain row; a Link row and the page without link actions keep their
// markup, and an action href that is not a daemon path is dropped.
func TestPanelRowPathActionLink(t *testing.T) {
	panels := []Panel{{ID: "tickets", Title: "Tickets", Icon: "board",
		Rows: []PanelRow{{Label: "ready", Text: "Plain", Path: "ai-docs/tickets/ready/z.md"}},
		Lanes: []PanelLane{{Name: "ready", Count: "3", Rows: []PanelRow{
			{Label: "feat", Text: "Served", Sub: "261102-feat-y", Path: "ai-docs/tickets/ready/y.md"},
			{Label: "feat", Text: "Outside", Path: "/elsewhere/o.md"},
			{Label: "feat", Text: "Evil", Path: "evil.md"},
		}}}}}
	actions := func(p string) FileAction {
		switch p {
		case "/elsewhere/o.md":
			return FileAction{Abs: "/elsewhere/o.md"}
		case "evil.md":
			return FileAction{Href: "javascript:alert(1)", Rel: "evil.md", Abs: "/root/evil.md"}
		}
		return FileAction{Href: "/s/r/file?path=" + p, Rel: p, Abs: "/root/" + p}
	}
	in := ViewInput{Layout: LayoutFull, ReplyID: "r", Panels: panels, FileText: func(p string) string { return p }, FileActions: actions}
	out := render(t, BuildView(&State{}, in))
	wantAll(t, out,
		`<span class="ctitle"><span class="chip cat feat">feat</span><a href="/s/r/file?path=ai-docs/tickets/ready/y.md" data-wl="file" data-rel="ai-docs/tickets/ready/y.md" data-abs="/root/ai-docs/tickets/ready/y.md" title="ai-docs/tickets/ready/y.md">Served</a></span>`,
		`<a tabindex="0" data-wl="file" data-abs="/elsewhere/o.md" title="/elsewhere/o.md">Outside</a>`,
		`<a tabindex="0" data-wl="file" data-rel="evil.md" data-abs="/root/evil.md" title="evil.md">Evil</a>`,
		`<span><a href="/s/r/file?path=ai-docs/tickets/ready/z.md" data-wl="file" data-rel="ai-docs/tickets/ready/z.md" data-abs="/root/ai-docs/tickets/ready/z.md" title="ai-docs/tickets/ready/z.md">Plain</a></span>`,
	)
	wantNone(t, out, "javascript:", `<code class="fpath">`)
	if panels[0].Lanes[0].Rows[0].IsAction() || panels[0].Rows[0].IsAction() {
		t.Error("BuildView changed the caller's panels")
	}
	in.FileActions = nil
	wantNone(t, markup(render(t, BuildView(&State{}, in))), "data-wl")
}

// TestFileActionsZeroKeepsPathText: a zero FileAction leaves a file target
// to FileText's path text, and a page without FileActions marks no link.
func TestFileActionsZeroKeepsPathText(t *testing.T) {
	st := &State{Seq: 2, SettleSeq: 1}
	st.Items = []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "files", Status: StatusOpen, Options: []string{"a", "b"},
		Rationale: "see [the plan](ai-docs/plan.md) and [web](https://example.com/)"}}
	zero := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r",
		FileText: func(p string) string { return p }, FileActions: func(string) FileAction { return FileAction{} }}))
	wantAll(t, zero, `the plan <code class="fpath">ai-docs/plan.md</code>`, `data-wl="url"`)
	wantNone(t, zero, `data-wl="file"`)
	plain := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", FileText: func(p string) string { return p }}))
	wantNone(t, markup(plain), "data-wl")
}

// TestDiskBoardKeepsFileLinks: a board written to disk (FileLink, no
// FileText) keeps its file: and relative links and marks none of them.
func TestDiskBoardKeepsFileLinks(t *testing.T) {
	st := &State{Seq: 2, SettleSeq: 1}
	st.Items = []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "files", Status: StatusOpen, Options: []string{"a", "b"},
		Rationale: "see [the plan](ai-docs/plan.md) and [abs](file:///abs/b.md) and [web](https://example.com/)"}}
	out := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r",
		FileLink:    func(p string) string { return "file://" + p },
		FileActions: func(string) FileAction { return FileAction{Href: "/s/r/file?path=x", Abs: "/x"} }}))
	wantAll(t, out, `<a href="file://ai-docs/plan.md">the plan</a>`, `<a href="file:///abs/b.md">abs</a>`)
	wantNone(t, markup(out), "data-wl", "/s/r/file")
}

// markup is page without its script elements: the viewer script names
// data-wl links and data: image sources it reads, which no markup holds.
func markup(page string) string {
	return scriptElement.ReplaceAllString(page, "")
}

var scriptElement = regexp.MustCompile(`(?s)<script>.*?</script>`)

// frameDocs returns the srcdoc document of every topic-body frame in board,
// keyed by topic name, unescaped as a browser reads the attribute.
func frameDocs(t *testing.T, board string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`<iframe class="doc-frame" title="([^"]*)" data-doc="([^"]*)" sandbox="[^"]*" srcdoc="([^"]*)"></iframe>`).FindAllStringSubmatch(board, -1) {
		name := html.UnescapeString(m[2])
		if html.UnescapeString(m[1]) != name {
			t.Errorf("frame title %q differs from its data-doc %q", m[1], m[2])
		}
		out[name] = html.UnescapeString(m[3])
	}
	if got := strings.Count(board, "<iframe"); got != len(out) {
		t.Fatalf("board holds %d frames, %d parsed:\n%.600s", got, len(out), board)
	}
	return out
}

// TestServedTopicBodyRendersInASandboxedFrame pins the served page's body
// trust: RenderRegions puts a body's HTML, scripts included, in a srcdoc
// frame with allow-scripts but never allow-same-origin, so none of it runs
// in the daemon's origin, and the frame document carries the page stylesheet
// (the theme variables), the frame script, and the body's own markup. The
// disk page keeps rendering the body inline.
func TestServedTopicBodyRendersInASandboxedFrame(t *testing.T) {
	const doc = "**plan**\n\n<canvas id=\"c\" width=\"40\" height=\"20\"></canvas>\n<script>\nfetch('/s/r/message', {method: 'POST'});\n</script>\n\nSee {{1}}"
	st := &State{Seq: 2, Topics: []TopicDoc{{Name: "doc", Body: doc, Changed: 2}}, Items: []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "x", Status: StatusOpen, Options: []string{"a", "b"}}}}
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	board := string(regions.Board)
	if !strings.Contains(board, `<div class="doc-body framed"><iframe class="doc-frame" title="doc" data-doc="doc" sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox" srcdoc="`) {
		t.Fatalf("body frame missing or its sandbox changed:\n%.600s", board)
	}
	wantNone(t, board, "allow-same-origin", "<script", "<canvas")
	assets, _ := loadAssets()
	doc1 := frameDocs(t, board)["doc"]
	wantAll(t, doc1, `<base target="_blank">`, "--wb-bg:", "<strong>plan</strong>", `<canvas id="c"`, "fetch('/s/r/message'", assets.docFrame, `<body class="doc-frame"><div class="doc-body md">`,
		`See <a href="#item-1">(01)</a>`)
	if strings.Contains(doc1, "<script src=") {
		t.Errorf("a body without a mermaid block loads mermaid in its frame")
	}

	page := render(t, view)
	wantAll(t, page, `<div class="doc-body md"><p><strong>plan</strong></p>`, "fetch('/s/r/message'")
	wantNone(t, page, "<iframe", "srcdoc")
}

// TestServedBoardFramesEveryBodySeparately: one frame per body, each carrying
// its topic's name (escaped) and only its own text, closed topics included.
func TestServedBoardFramesEveryBodySeparately(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "one", "x")+
		`<wb:topic name='say "hi" &amp; <go>'>second body</wb:topic>`+
		topicXML("one", "first body")+topicXML("closed", "third body")+`<wb:close topic="closed"/>`+addXML(ClassTechnical, "no body", "y"))
	regions, err := RenderRegions(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	if err != nil {
		t.Fatal(err)
	}
	docs := frameDocs(t, string(regions.Board))
	if len(docs) != 3 {
		t.Fatalf("frames = %d, want one per body", len(docs))
	}
	for name, want := range map[string]string{"one": "first body", `say "hi" &amp; <go>`: "second body", "closed": "third body"} {
		doc, ok := docs[name]
		if !ok || !strings.Contains(doc, `<div class="doc-body md">`+want+`</div>`) {
			t.Errorf("frame %q = %.200q, want %q", name, doc, want)
		}
		for other, text := range map[string]string{"one": "first body", `say "hi" &amp; <go>`: "second body", "closed": "third body"} {
			if other != name && strings.Contains(doc, text) {
				t.Errorf("frame %q holds %q's text", name, other)
			}
		}
	}
	// The name reaches the attribute escaped, so it cannot end the attribute.
	wantAll(t, string(regions.Board), `data-doc="say &#34;hi&#34; &amp;amp; &lt;go&gt;"`)
	// A closed topic's body sits in a closed fold, framed all the same.
	wantAll(t, string(regions.Board), `<details class="tdoc" data-key="doc:closed"><summary`)
}

// TestFramedBodyImagesTakeDocImageSrc pins where DocImageSrc applies: a file
// image in a framed body takes it, an empty result falls back to ImageSrc,
// and an item's image and the disk page's body keep ImageSrc.
func TestFramedBodyImagesTakeDocImageSrc(t *testing.T) {
	st := &State{Seq: 2, Topics: []TopicDoc{{Name: "doc", Body: "![a](a.png) ![big](big.png)", Changed: 2}},
		Items: []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "x", Status: StatusOpen, Options: []string{"a", "b"}, Rationale: "![item](a.png)"}}}
	in := ViewInput{Layout: LayoutFull, ReplyID: "r",
		FileText: func(p string) string { return p },
		ImageSrc: func(p string) string { return "/route/" + p },
		DocImageSrc: func(p string) string {
			if p == "big.png" {
				return ""
			}
			return "data:image/png;base64,QUJD"
		}}
	regions, err := RenderRegions(BuildView(st, in))
	if err != nil {
		t.Fatal(err)
	}
	doc := frameDocs(t, string(regions.Board))["doc"]
	wantAll(t, doc, `<img src="data:image/png;base64,QUJD" alt="a">`, `<img src="/route/big.png" alt="big">`)
	wantAll(t, string(regions.Board), `<img src="/route/a.png" alt="item">`)
	wantNone(t, string(regions.Board), `src="data:image/png;base64,QUJD" alt="item"`)

	page := render(t, BuildView(st, in))
	wantNone(t, markup(page), "data:image")
	wantAll(t, page, `<img src="/route/a.png" alt="a">`)

	// Without DocImageSrc the frame's images take ImageSrc.
	in.DocImageSrc = nil
	regions, err = RenderRegions(BuildView(st, in))
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, frameDocs(t, string(regions.Board))["doc"], `<img src="/route/a.png" alt="a">`)
}

// TestRenderRegionsMermaidInBodyAndItems pins which page loads mermaid: the
// host page for a block outside the frames, the frame for a block in its own
// body, each alone.
func TestRenderRegionsMermaidInBodyAndItems(t *testing.T) {
	const fence = "```mermaid\ngraph TD; A-->B\n```"
	mermaid, _ := MermaidAsset()
	item := Item{ID: 1, Class: ClassPolicy, Topic: "t", Title: "x", Options: []string{"a", "b"}, Status: StatusOpen}
	withMermaid := item
	withMermaid.Rationale = fence
	for name, tc := range map[string]struct {
		items    []Item
		body     string
		wantHost bool
		wantDoc  bool
	}{
		"body only":       {[]Item{item}, fence, false, true},
		"item only":       {[]Item{withMermaid}, "plain body", true, false},
		"both":            {[]Item{withMermaid}, fence, true, true},
		"neither":         {[]Item{item}, "plain body", false, false},
		"empty-body page": {[]Item{withMermaid}, "", true, false},
	} {
		st := &State{Seq: 2, Items: tc.items}
		if tc.body != "" {
			st.Topics = []TopicDoc{{Name: "doc", Body: tc.body, Changed: 2}}
		}
		regions, err := RenderRegions(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
		if err != nil {
			t.Fatal(err)
		}
		if regions.Mermaid != tc.wantHost {
			t.Errorf("%s: host mermaid = %v, want %v", name, regions.Mermaid, tc.wantHost)
		}
		if tc.body != "" {
			doc := frameDocs(t, string(regions.Board))["doc"]
			if has := strings.Contains(doc, `<script src="`+mermaid+`"></script>`); has != tc.wantDoc {
				t.Errorf("%s: frame loads mermaid = %v, want %v", name, has, tc.wantDoc)
			}
		}
	}
}

// TestImageViewerScriptHooks pins where the image viewer runs: the full page
// carries it (and the SVG helpers it shares with the frames) with its overlay
// styles and icons, the embed snapshot does not, a served page's topic-body
// frame carries the posting side, and the dashboard reuses the script once
// through PageAssets.
func TestImageViewerScriptHooks(t *testing.T) {
	full := render(t, sampleView(LayoutFull))
	wantAll(t, full, `wbv-stage`, `function wbFlattenSvg(`, `role", "dialog"`, `aria-modal`, `.wbv-stage`, `id="i-zoom-in"`, `id="i-ext"`)
	embed := render(t, sampleView(LayoutEmbed))
	wantNone(t, embed, "function wbFlattenSvg(", `createObjectURL`)

	st := &State{Seq: 2, Topics: []TopicDoc{{Name: "doc", Body: "![a](a.png)", Changed: 2}}}
	regions, err := RenderRegions(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	if err != nil {
		t.Fatal(err)
	}
	doc := frameDocs(t, string(regions.Board))["doc"]
	wantAll(t, doc, `function wbFlattenSvg(`, `wbDoc: "view"`)
	wantNone(t, doc, `createObjectURL`)

	shared, err := PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, string(shared.ViewerScript), `function wbFlattenSvg(`, `wbviewerclose`, `message`)
	wantNone(t, string(shared.FormScript), `wbFlattenSvg`)
}
