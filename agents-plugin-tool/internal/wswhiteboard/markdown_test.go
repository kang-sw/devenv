package wswhiteboard

import (
	"regexp"
	"strings"
	"testing"
)

// mdBoard renders a one-topic board whose items are the given ones plus a
// target item (90) the references point at, with a FileLink that marks
// every resolved file path, so a test sees what reached the link rule.
func mdBoard(t *testing.T, layout Layout, items ...Item) string {
	t.Helper()
	st := &State{Seq: 2, SettleSeq: 1}
	st.Items = append(items, Item{ID: 90, Class: ClassTechnical, Topic: "t", Title: "target", Status: StatusOpen})
	return render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r", FileLink: func(p string) string { return "L/" + p }}))
}

func wantAll(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("page lacks %q", want)
		}
	}
}

func wantNone(t *testing.T, out string, bads ...string) {
	t.Helper()
	for _, bad := range bads {
		if strings.Contains(out, bad) {
			t.Errorf("page holds %q", bad)
		}
	}
}

// TestMarkdownTitleIsInlineOnly pins the title: bold, italic, code, and
// links render; block syntax stays literal; an image shows its alt text.
func TestMarkdownTitleIsInlineOnly(t *testing.T) {
	for name, layout := range layouts() {
		t.Run(name, func(t *testing.T) {
			out := mdBoard(t, layout,
				Item{ID: 1, Class: ClassPolicy, Topic: "t", Title: "**bold** _it_ `code` [doc](docs/a.md) ![shot](s.png)", Options: []string{"x", "y"}, Status: StatusOpen},
				Item{ID: 2, Class: ClassTechnical, Topic: "t", Title: "- not a list", Status: StatusOpen},
				Item{ID: 3, Class: ClassTechnical, Topic: "t", Title: "# not a heading", Status: StatusOpen},
				Item{ID: 4, Class: ClassTechnical, Topic: "t", Title: "```mermaid fence```", Status: StatusOpen},
				Item{ID: 5, Class: ClassTechnical, Topic: "t", Title: "[x]: not-a-definition", Status: StatusOpen},
			)
			wantAll(t, out,
				`<span class="title"><strong>bold</strong> <em>it</em> <code>code</code> <a href="L/docs/a.md">doc</a> shot</span>`,
				`<span class="title">- not a list</span>`,
				`<span class="title"># not a heading</span>`,
				`<span class="title"><code>mermaid fence</code></span>`,
				`<span class="title">[x]: not-a-definition</span>`,
			)
			wantNone(t, out, "<img", "<ul>", "<h1>not", `class="mermaid"`)
		})
	}
}

// TestMarkdownBlockFields pins block markdown in options, rationale, and the
// recommendation: lists, code blocks, tables, and images.
func TestMarkdownBlockFields(t *testing.T) {
	out := mdBoard(t, LayoutFull, Item{ID: 1, Class: ClassPolicy, Topic: "t", Title: "q",
		Options:        []string{"- one\n- two", "plain"},
		Recommendation: "a: because\n\n```go\nx := 1\n```",
		Rationale:      "| k | v |\n|---|---|\n| a | 1 |\n\n![diagram](img/d.png)",
		Status:         StatusOpen})
	wantAll(t, out,
		"<ul>\n<li>one</li>\n<li>two</li>\n</ul>",
		`<pre><code class="language-go">x := 1`+"\n</code></pre>",
		"<th>k</th>", "<td>1</td>",
		`<img src="L/img/d.png" alt="diagram">`,
		`<div class="rec-line md" title="recommendation"><b>`,
		`<div class="ctx md">`,
	)
}

// TestMarkdownRawHTMLIsEscaped pins the escaping boundary: no tag the lead
// types reaches the page as markup, inline or at the start of a line, and a
// leading tag does not swallow the markdown after it.
func TestMarkdownRawHTMLIsEscaped(t *testing.T) {
	out := mdBoard(t, LayoutFull, Item{ID: 1, Class: ClassPolicy, Topic: "t", Title: "Option<T> <b>bold</b>",
		Options:        []string{`<img src=x onerror=alert(1)>`, "<script>o()</script>"},
		Recommendation: "a: <iframe src=x></iframe>",
		Rationale:      "<div onclick=x>block</div>\n**after**\n<!-- comment -->",
		Status:         StatusOpen})
	wantAll(t, out,
		"Option&lt;T&gt; &lt;b&gt;bold&lt;/b&gt;",
		"&lt;img src=x onerror=alert(1)&gt;",
		"&lt;script&gt;o()&lt;/script&gt;",
		"&lt;iframe src=x&gt;&lt;/iframe&gt;",
		"&lt;div onclick=x&gt;block&lt;/div&gt;<br>\n<strong>after</strong>",
		"&lt;!-- comment --&gt;",
	)
	wantNone(t, out, "<img src=x", "<script>o()", "<iframe", "<div onclick", "<b>bold", "<!-- comment", "raw HTML omitted")
}

// TestMarkdownItemReferences pins {{nn}} inside markdown: it resolves in
// emphasis and list items, stays literal in a code span and a code block,
// and is plain (nn) inside a link, since anchors do not nest.
func TestMarkdownItemReferences(t *testing.T) {
	out := mdBoard(t, LayoutFull, Item{ID: 1, Class: ClassPolicy, Topic: "t", Title: "**see {{90}}** `{{90}}`",
		Options:   []string{"- item {{90}}", "[link {{90}}](docs/x.md)"},
		Rationale: "code `{{90}}`\n\n```\nblock {{90}}\n```\n\n{{99}} stays",
		Status:    StatusOpen})
	anchor := `<a href="#item-90">(90)</a>`
	wantAll(t, out,
		"<strong>see "+anchor+"</strong> <code>{{90}}</code>",
		"<li>item "+anchor+"</li>",
		`<a href="L/docs/x.md">link (90)</a>`,
		"code <code>{{90}}</code>",
		"<pre><code>block {{90}}\n</code></pre>",
		"{{99}} stays",
	)
}

// TestMarkdownLinkAndImageTargets pins the target allow-list. Links take
// relative paths, absolute paths, file: URLs, http(s) URLs, and fragment-only
// anchors; images take relative paths, absolute paths, and file: URLs; any
// other target, autolinks included, renders as text.
func TestMarkdownLinkAndImageTargets(t *testing.T) {
	out := mdBoard(t, LayoutFull, Item{ID: 1, Class: ClassPolicy, Topic: "t", Title: "targets", Options: []string{"x", "y"}, Status: StatusOpen,
		Rationale: strings.Join([]string{
			"[rel](docs/a%20b.md#sec)",
			"[abs](/srv/notes.md)",
			"[fileurl](file:///srv/f.md)",
			"[web](https://example.com/x?a=1&b=2)",
			"[frag](#item-90)",
			"[js](javascript:alert(1))",
			"[jsmixed](JaVaScRiPt:alert(1))",
			"[jsentity](java&#x73;cript:alert(1))",
			"[data](data:text/html,x)",
			"[vb](vbscript:x)",
			"[net](//evil.example/x)",
			"[remotefile](file://evil.example/share/x)",
			`[quote](docs/q.md 't" onmouseover="x')`,
			`[esc](docs/a\_b.md)`,
			"<javascript:alert(2)>",
			"<https://auto.example/>",
			"<mailto:a@b.c>",
			"<a@b.c>",
		}, "\n") + "\n\n" + strings.Join([]string{
			"![imgrel](shots/a.png)",
			"![imgabs](/srv/b.png)",
			"![imgfile](file:///srv/c.png)",
			"![remote](https://example.com/r.png)",
			"![dataimg](data:image/png;base64,AAAA)",
			"![fragimg](#item-90)",
		}, "\n")})
	wantAll(t, out,
		`<a href="L/docs/a%20b.md#sec">rel</a>`,
		`<a href="L//srv/notes.md">abs</a>`,
		`<a href="L//srv/f.md">fileurl</a>`,
		`<a href="https://example.com/x?a=1&amp;b=2" target="_blank" rel="noopener noreferrer">web</a>`,
		`<a href="#item-90">frag</a>`,
		`<a href="L/docs/q.md" title="t&quot; onmouseover=&quot;x">quote</a>`,
		`<a href="L/docs/a_b.md">esc</a>`,
		"<br>\njs<br>\njsmixed<br>\njsentity<br>\ndata<br>\nvb<br>\nnet<br>\nremotefile<br>\n",
		`<a href="https://auto.example/" target="_blank" rel="noopener noreferrer">https://auto.example/</a>`,
		"javascript:alert(2)", "mailto:a@b.c", "a@b.c",
		`<img src="L/shots/a.png" alt="imgrel">`,
		`<img src="L//srv/b.png" alt="imgabs">`,
		`<img src="L//srv/c.png" alt="imgfile">`,
		"<br>\nremote<br>\ndataimg<br>\nfragimg",
	)
	wantNone(t, out, "javascript:alert(1)", `href="javascript`, `href="JaVa`, `href="data:`, `href="vbscript`, "evil.example",
		`href="mailto`, `src="https://`, `src="data:`, `src="#`, `onmouseover="x"`)
}

// TestMarkdownFileHrefCannotBecomeAScheme pins the check on the href a file
// path resolves to: a relative result whose first segment holds a colon
// would read as a scheme, so it is anchored with "./", whatever FileLink
// returned; a decoded "#" or "?" in a file name stays part of the path.
func TestMarkdownFileHrefCannotBecomeAScheme(t *testing.T) {
	rationale := "[a](./javascript:alert(1))\n[b](javascript%3Aalert%282%29)\n![c](javascript%3Ax.png)\n[d](notes%231.md#top)\n[e](q%3F.md)"
	for name, link := range map[string]func(string) string{
		"nil":  nil,
		"bare": func(p string) string { return strings.TrimPrefix(p, "./") }, // a page in the same folder
	} {
		t.Run(name, func(t *testing.T) {
			st := &State{Seq: 2}
			st.Items = []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "q", Options: []string{"x", "y"}, Rationale: rationale, Status: StatusOpen}}
			out := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", FileLink: link}))
			wantNone(t, out, `href="javascript`, `src="javascript`)
			wantAll(t, out, `">a</a>`, `href="./javascript:alert(2)">b</a>`, `src="./javascript:x.png"`,
				`href="notes%231.md#top">d</a>`, `href="q%3F.md">e</a>`)
		})
	}
}

// TestMarkdownRationaleHardWraps pins the line breaks of a block field: each
// line of a one-reason-per-line rationale keeps its own line.
func TestMarkdownRationaleHardWraps(t *testing.T) {
	out := mdBoard(t, LayoutFull, Item{ID: 1, Class: ClassPolicy, Topic: "t", Title: "q", Options: []string{"x", "y"},
		Rationale: "first reason\nsecond reason\nthird *reason*", Status: StatusOpen})
	wantAll(t, out, `<div class="ctx md">first reason<br>`+"\nsecond reason<br>\nthird <em>reason</em></div>")
}

// TestMarkdownAckReasonsAreInline pins an ack row: one .why per rationale
// line, each inline markdown only.
func TestMarkdownAckReasonsAreInline(t *testing.T) {
	out := mdBoard(t, LayoutFull, Item{ID: 1, Class: ClassAnnounced, Topic: "t", Title: "ack *title*",
		Rationale: "- *first*\n```go\n![img](a.png)", Status: StatusOpen})
	wantAll(t, out,
		`<span class="title">ack <em>title</em></span>`,
		`<span class="why">- <em>first</em></span>`,
		"<span class=\"why\">```go</span>",
		`<span class="why">img</span>`,
	)
	wantNone(t, out, "<ul>", "<pre>", "<img")
}

// TestMarkdownOptionStacking pins when an item's options stack as cards:
// any option with block markdown (a list, a code block, a table, an image, a
// mermaid block, or more than one paragraph) stacks all of them; one-line
// options, even with inline markdown or a hard-wrapped second line, keep the
// inline buttons. A stacked option is a focusable role="button" element.
func TestMarkdownOptionStacking(t *testing.T) {
	inline := []string{"plain", "with **bold** and [a link](docs/a.md)", "two\nlines"}
	block := map[string]string{
		"list":       "- a\n- b",
		"ordered":    "1. a\n2. b",
		"code":       "```\nx\n```",
		"indented":   "para\n\n    code",
		"table":      "| a |\n|---|\n| 1 |",
		"image":      "![shot](s.png)",
		"mermaid":    "```mermaid\ngraph TD\nA-->B\n```",
		"paragraphs": "one\n\ntwo",
		"quote":      "> quoted",
		"heading":    "# heading",
	}
	st := &State{Seq: 2}
	st.Items = []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "inline", Options: inline, Status: StatusOpen}}
	out := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantAll(t, out, `<div class="opts"><button class="opt" data-opt="a" aria-pressed="false"><span class="k">a</span><span>plain</span></button>`,
		`<span>two<br>`+"\nlines</span></button>")
	wantNone(t, out, "opts stack", `role="button" tabindex`)

	for name, opt := range block {
		t.Run(name, func(t *testing.T) {
			for _, status := range []string{StatusOpen, StatusAssumed} {
				st := &State{Seq: 2}
				st.Items = []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "block", Options: []string{"short", opt}, Status: status, Answer: "a", AnswerQuote: "q"}}
				out := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
				if status == StatusOpen {
					wantAll(t, out, `<div class="opts stack"><div class="opt" role="button" tabindex="0" data-opt="a" aria-pressed="false"><span class="k">a</span><div class="md">short</div></div>`)
					wantNone(t, out, `<button class="opt`)
				} else {
					wantAll(t, out, `<div class="opts stack"><div class="opt static picked"><span class="k">a</span><div class="md">short</div></div>`)
				}
			}
		})
	}
}

// TestMarkdownPickedChipShowsFirstLine pins a settled row's picked-option
// chip: the option's first line, as inline markdown.
func TestMarkdownPickedChipShowsFirstLine(t *testing.T) {
	st := &State{Seq: 2, SettleSeq: 1}
	st.Items = []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "q", Options: []string{"**Keep** it\n- detail\n```go\nx\n```", "other"},
		Status: StatusAnswered, Answer: "a", AnswerQuote: "q", Settled: 1}}
	out := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantAll(t, out, `<span class="pick" title="picked"><span class="k">a</span><span class="pt"><strong>Keep</strong> it</span></span>`)
	wantNone(t, out, "<li>detail", "<pre>")
}

// TestMarkdownMermaidBlock pins a mermaid block: its source sits escaped in
// <pre class="mermaid">, readable when the script cannot load; the page then
// loads the shared script by its bare relative name and runs the theme
// script, which resolves the --wb-* colours with getComputedStyle before
// mermaid.initialize. A page without a mermaid block loads neither.
func TestMarkdownMermaidBlock(t *testing.T) {
	name, data := MermaidAsset()
	if !strings.HasPrefix(name, "wb-mermaid-") || !strings.HasSuffix(name, ".js") || len(data) < 1<<20 {
		t.Fatalf("mermaid asset %q, %d bytes", name, len(data))
	}
	if again, _ := MermaidAsset(); again != name {
		t.Fatalf("asset name is not stable: %q then %q", name, again)
	}
	for layoutName, layout := range layouts() {
		t.Run(layoutName, func(t *testing.T) {
			// A policy card shows its rationale as blocks; an ack row's
			// reasons are inline only.
			out := mdBoard(t, layout, Item{ID: 1, Class: ClassPolicy, Topic: "t", Title: "diagram", Options: []string{"x", "y"}, Status: StatusOpen,
				Rationale: "```mermaid\ngraph LR\n  A[\"<b>lead</b>\"] --> B:::store\n```"})
			wantAll(t, out,
				"<pre class=\"mermaid\">graph LR\n  A[&quot;&lt;b&gt;lead&lt;/b&gt;&quot;] --&gt; B:::store\n</pre>",
				`<script src="`+name+`"></script>`,
			)
			if !UsesMermaid(out) {
				t.Errorf("UsesMermaid is false for a page with a mermaid block")
			}
			if strings.Index(out, `<pre class="mermaid">`) > strings.Index(out, `<script src="`) {
				t.Errorf("the mermaid script loads before the block it draws")
			}
			init := out[strings.Index(out, `<script src="`+name+`"></script>`):]
			wantAll(t, init,
				"getComputedStyle(probe).color",
				`c("--wb-card")`, `c("--wb-fg")`, `c("--wb-muted")`, `c("--wb-muted-fg")`, `c("--wb-now")`,
				"m.initialize(", `theme: "base"`, `securityLevel: "strict"`, "themeVariables:", "themeCSS:",
				".node.store rect", "var(--wb-now)", `m.run({ querySelector: "pre.mermaid" })`,
				"if (!m) return;",
			)
			if strings.Index(init, "getComputedStyle(probe)") > strings.Index(init, "m.initialize(") {
				t.Errorf("colours must resolve before mermaid.initialize")
			}
			if strings.Count(out, "<script") != 4 {
				t.Errorf("want the string table, the page script, the shared mermaid script, and the theme script, got %d", strings.Count(out, "<script"))
			}

			plain := mdBoard(t, layout, Item{ID: 1, Class: ClassPolicy, Topic: "t", Title: "no diagram", Options: []string{"x", "y"}, Rationale: "```go\nx\n```", Status: StatusOpen})
			if UsesMermaid(plain) || strings.Contains(plain, `class="mermaid"`) || strings.Contains(plain, "<script src=") || strings.Contains(plain, "m.initialize(") {
				t.Errorf("a page without a mermaid block mentions mermaid")
			}
		})
	}
}

// TestRenderTextMermaidOption pins RenderText's mermaid switch: on, a
// mermaid fence is a <pre class="mermaid"> for the host page to draw; off
// (the default), it is a plain code block.
func TestRenderTextMermaidOption(t *testing.T) {
	src := "```mermaid\ngraph TD; A-->B\n```"
	if on := string(RenderText(src, TextTargets{Mermaid: true})); !strings.Contains(on, `<pre class="mermaid">graph TD; A--&gt;B`) {
		t.Errorf("mermaid on: %s", on)
	}
	off := string(RenderText(src, TextTargets{}))
	if strings.Contains(off, `class="mermaid"`) || !strings.Contains(off, `<code class="language-mermaid">`) {
		t.Errorf("mermaid off: %s", off)
	}
}

// TestRenderTextFileActions: RenderText passes FileActions through, so a
// conversation bubble gets the served page's action links.
func TestRenderTextFileActions(t *testing.T) {
	out := string(RenderText("[a](docs/a.md) [w](https://example.com/)", TextTargets{
		FileText:    func(p string) string { return p },
		FileActions: func(p string) FileAction { return FileAction{Href: "/s/r/file?path=" + p, Rel: p, Abs: "/root/" + p} },
	}))
	wantAll(t, out, `<a href="/s/r/file?path=docs/a.md" data-wl="file" data-rel="docs/a.md" data-abs="/root/docs/a.md" title="docs/a.md">a</a>`, `data-wl="url"`)
}

// TestMarkdownBoardTaskListAndFootnotes pins both extensions in item block
// fields: a task list renders read-only checkboxes, and two fields that each
// define [^1] get distinct ids.
func TestMarkdownBoardTaskListAndFootnotes(t *testing.T) {
	out := mdBoard(t, LayoutFull, Item{ID: 1, Class: ClassPolicy, Topic: "t", Title: "q",
		Options:   []string{"- [ ] a\n- [x] b", "x[^1]\n\n[^1]: one"},
		Rationale: "y[^1]\n\n[^1]: two",
		Status:    StatusOpen})
	wantAll(t, out, `<input disabled="" type="checkbox"> a`, `<input checked="" disabled="" type="checkbox"> b`)
	ids := regexp.MustCompile(`<li id="(wbf-[0-9a-f]{8}-fn:1)">`).FindAllStringSubmatch(out, -1)
	if len(ids) != 2 || ids[0][1] == ids[1][1] {
		t.Errorf("want two distinct footnote ids, got %v", ids)
	}
}
