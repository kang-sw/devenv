package wswhiteboard

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// servedReply renders text as a finished lead reply on a served page: a file
// target shows as "T:<path>", and a relative .png inside the session root is
// served by the image route.
func servedReply(text string) string {
	return string(RenderText(text, TextTargets{
		FileText: func(p string) string { return "T:" + p },
		ImageSrc: func(p string) string {
			if strings.HasSuffix(p, ".png") && !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "..") {
				return "/s/r/image?path=" + url.QueryEscape(p)
			}
			return ""
		},
		HTML: true,
	}))
}

func TestReplyHTMLKeepsInlineHTMLAndSVG(t *testing.T) {
	out := servedReply("**md** and <b>bold</b> <span style=\"color: red; font-weight: 700\" class=\"hl\" title=\"t\">red</span>\n\n" +
		"<details open><summary>more</summary>\n\nhidden *text*\n\n</details>\n\n" +
		"<table><tr><th colspan=\"2\" align=\"left\">h</th></tr><tr><td>1</td><td>2</td></tr></table>\n\n" +
		`<svg viewBox="0 0 100 40" role="img"><defs><marker id="ah" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto"><path d="M0,0 L8,4 L0,8 z"/></marker>` +
		`<linearGradient id="g"><stop offset="0" stop-color="#fff"/></linearGradient></defs>` +
		`<rect x="1" y="1" width="30" height="20" rx="4" fill="url(#g)"/><path class="edge" d="M30,10 H60" marker-end="url(#ah)"/><text x="64" y="14">label</text></svg>`)
	wantAll(t, out,
		"<strong>md</strong> and <b>bold</b>",
		`<span style="color: red; font-weight: 700" class="hl" title="t">red</span>`,
		"<details open=\"\"><summary>more</summary>", "hidden <em>text</em>",
		`<th colspan="2" align="left">h</th>`, "<td>2</td>",
		`<svg viewBox="0 0 100 40" role="img">`,
		`<text x="64" y="14">label</text>`,
	)
	prefix := svgPrefix(t, out)
	wantAll(t, out,
		`<marker id="`+prefix+`ah" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto">`,
		`<linearGradient id="`+prefix+`g">`, `fill="url(#`+prefix+`g)"`, `marker-end="url(#`+prefix+`ah)"`,
	)
}

// svgPrefix returns the id prefix out's first SVG id carries.
func svgPrefix(t *testing.T, out string) string {
	t.Helper()
	m := regexp.MustCompile(`id="(wbr-[0-9a-f]{8}-)`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no prefixed SVG id:\n%s", out)
	}
	return m[1]
}

// TestReplySVGIDsStayApartAcrossReplies pins the per-reply id prefix: two
// replies on one page that both define id="g" with different paint keep
// distinct ids, each referenced by its own reply.
func TestReplySVGIDsStayApartAcrossReplies(t *testing.T) {
	svg := func(color string) string {
		return `<svg><defs><linearGradient id="g"><stop stop-color="` + color + `"/></linearGradient></defs><rect fill="url(#g)"/></svg>`
	}
	a, b := servedReply(svg("red")), servedReply(svg("blue"))
	pa, pb := svgPrefix(t, a), svgPrefix(t, b)
	if pa == pb {
		t.Fatalf("both replies prefix their ids with %q", pa)
	}
	if !strings.Contains(a, `fill="url(#`+pa+`g)"`) || !strings.Contains(b, `fill="url(#`+pb+`g)"`) {
		t.Errorf("a reply references another's id:\n%s\n%s", a, b)
	}
}

// TestReplyHTMLBlockTagSwallowsFollowingLines pins goldmark's HTML block
// rule: a line starting with a block tag keeps the lines after it raw up to
// the next blank line, so markdown there is literal.
func TestReplyHTMLBlockTagSwallowsFollowingLines(t *testing.T) {
	out := servedReply("<details>\n**not bold**\n</details>\n\n**bold**")
	wantAll(t, out, "<details>\n**not bold**\n</details>", "<strong>bold</strong>")
}

// TestReplyHTMLBalancesStrayTags pins the server-side balancing: a stray
// closing tag is dropped and an unclosed one is closed inside the reply.
func TestReplyHTMLBalancesStrayTags(t *testing.T) {
	out := servedReply("</div></div></td></table><b>open\n\n<i>unclosed")
	if out != "<b>open\n<p><i>unclosed</i></p></b>" {
		t.Errorf("balanced reply = %q", out)
	}
}

// TestReplyHTMLStripsEveryScriptVector holds one adversarial case per vector
// the reply sanitizer must exclude; each must leave none of its marker.
func TestReplyHTMLStripsEveryScriptVector(t *testing.T) {
	cases := []struct {
		name, in string
		bad      []string
	}{
		{"script element", "<script>alert(1)</script>ok", []string{"<script", "alert"}},
		{"script in an svg", `<svg><script>alert(1)</script></svg>`, []string{"<script", "alert"}},
		{"on* attribute", `<img src="img/a.png" onerror="alert(1)"><p onclick="alert(2)">p</p>`, []string{"onerror", "onclick", "alert"}},
		{"javascript href", `<a href="javascript:alert(1)">j</a>`, []string{"javascript", "href"}},
		{"entity-encoded javascript href", `<a href="java&#x73;cript&colon;alert(1)">e</a>`, []string{"javascript", "href", "alert"}},
		{"control-character javascript href", "<a href=\"\x01javascript:alert(1)\">c</a>", []string{"javascript", "href"}},
		{"vbscript href", `<a href="vbscript:msgbox(1)">v</a>`, []string{"vbscript", "href"}},
		{"data href", `<a href="data:text/html,&lt;script&gt;alert(1)&lt;/script&gt;">d</a>`, []string{"data:", "href"}},
		{"data image", `<img src="data:image/svg+xml,&lt;svg onload=alert(1)&gt;">`, []string{"<img", "src="}},
		{"iframe", `<iframe src="https://evil.example/" srcdoc="&lt;script&gt;alert(1)&lt;/script&gt;"></iframe>`, []string{"iframe", "evil", "alert"}},
		{"frame", `<frameset><frame src="https://evil.example/"></frameset>`, []string{"frame", "evil"}},
		{"object", `<object data="https://evil.example/x.swf"><param name="a" value="b"></object>`, []string{"object", "evil", "param"}},
		{"embed", `<embed src="https://evil.example/x.swf">`, []string{"embed", "evil"}},
		{"base", `<base href="https://evil.example/">`, []string{"base", "evil"}},
		{"meta refresh", `<meta http-equiv="refresh" content="0;url=javascript:alert(1)">`, []string{"meta", "javascript"}},
		{"form and formaction", `<form action="https://evil.example/"><input name="q"><button formaction="https://evil.example/">go</button></form>`, []string{"form", "input", "button", "evil"}},
		{"svg animate on href", `<svg><a><animate attributeName="href" values="javascript:alert(1)"/><text>x</text></a></svg>`, []string{"animate", "javascript", "href"}},
		{"svg set on href", `<svg><a><set attributeName="href" to="javascript:alert(1)"/><text>x</text></a></svg>`, []string{"<set", "javascript", "href"}},
		{"style element", `<style>body { display: none }</style>s`, []string{"<style", "display"}},
		{"link element", `<link rel="stylesheet" href="https://evil.example/x.css">`, []string{"<link", "evil"}},
		{"media elements", `<video poster="https://evil.example/p.png" src="v.mp4"><source src="a.mp4"><track src="t.vtt"></video><audio src="a.mp3"></audio>`, []string{"video", "poster", "source", "track", "audio", "evil"}},
		{"style url", `<div style="background: url(https://evil.example/x.png); color: red">u</div>`, []string{"url(", "evil"}},
		{"style image-set", `<div style="background-image: image-set('https://evil.example/x.png' 1x)">i</div>`, []string{"image-set", "evil"}},
		{"style expression", `<div style="width: expression(alert(1))">x</div>`, []string{"expression", "alert"}},
		{"style import", `<div style="@import 'https://evil.example/x.css'; color: red">x</div>`, []string{"@import", "evil"}},
		{"style escape", `<div style="background: u\72 l(https://evil.example/x.png)">x</div>`, []string{`\`, "evil"}},
		{"style comment", `<div style="posi/**/tion: fixed; background: url/**/(https://evil.example/x.png)">x</div>`, []string{"position", "evil"}},
		{"svg external paint", `<svg><rect width="5" height="5" fill="url(https://evil.example/x.svg#p)"/></svg>`, []string{"fill", "evil"}},
		{"svg escaped paint", `<svg><rect width="5" height="5" fill="u\72 l(https://evil.example/x.svg#p)"/></svg>`, []string{"fill", "evil"}},
		{"svg use", `<svg><use href="https://evil.example/s.svg#x"/><use xlink:href="data:image/svg+xml,x"/></svg>`, []string{"<use", "evil", "data:"}},
		{"svg image", `<svg><image href="https://evil.example/x.png"/></svg>`, []string{"<image", "evil"}},
		{"svg link", `<svg><a href="javascript:alert(1)"><text>t</text></a></svg>`, []string{"javascript", "href"}},
		{"svg foreignObject", `<svg><foreignObject><img src="x" onerror="alert(1)"></foreignObject></svg>`, []string{"foreignObject", "onerror", "alert"}},
		{"mathml namespace confusion", `<math><mtext><table><mglyph><style><img src=x onerror=alert(1)>`, []string{"onerror", "alert", "<style"}},
		{"noscript raw text", `<noscript><p title="</noscript><img src=x onerror=alert(1)>"></noscript>`, []string{"onerror", "alert"}},
		{"template", `<template><img src=x onerror=alert(1)></template>`, []string{"template", "onerror"}},
		{"comment", `<!-- <script>alert(1)</script> -->c`, []string{"<!--", "alert"}},
		{"page id", `<div id="composer" name="x">d</div>`, []string{"id=", "name="}},
		{"author target and rel", `<a href="https://ok.example/" target="_self" rel="opener">o</a>`, []string{"_self", `rel="opener"`}},
		{"mailto", `<a href="mailto:a@example.com">m</a>`, []string{"mailto", "href"}},
		// The system-open route trusts a same-origin POST from a loopback
		// viewer, so no reply may post to, link to, or load any daemon path.
		{"system open via form", `<form method="post" action="/s/r/open"><input type="hidden" name="target" value="https://evil.example/"><button formaction="/s/r/open">open</button></form>`, []string{"<form", "action", "formaction", "<input", "<button", "evil"}},
		{"system open via link", `<a href="/s/r/open?target=https://evil.example/">open</a>`, []string{"href", "evil"}},
		{"system open via image", `<img src="/s/r/open?target=https://evil.example/x.png">`, []string{"<img", "src="}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := servedReply(c.in)
			for _, bad := range c.bad {
				if strings.Contains(out, bad) {
					t.Errorf("%q survives:\n%s", bad, out)
				}
			}
		})
	}
}

// TestReplyHTMLStyleLosesPosition pins the overlay guard: a position:fixed
// reply keeps its other declarations but cannot leave its bubble's flow.
func TestReplyHTMLStyleLosesPosition(t *testing.T) {
	out := servedReply(`<div style="position: fixed; inset: 0; z-index: 99; color: red">overlay</div>`)
	wantAll(t, out, `<div style="inset: 0; z-index: 99; color: red">overlay</div>`)
	wantNone(t, out, "position", "fixed")
}

// TestReplyHTMLURLsFollowTheMarkdownContract pins href and src in reply HTML
// to resolveTarget's rule: network images refused, file images served,
// http(s) links kept, and a refused or path-text target handled as markdown
// handles it.
func TestReplyHTMLURLsFollowTheMarkdownContract(t *testing.T) {
	out := servedReply(`<img src="https://example.com/x.png" alt="net"> <img src="img/a.png" alt="A" width="40"> <img src="../out.png" alt="O"> <img alt="none">` + "\n\n" +
		`<a href="https://example.com/p" title="t">web</a> <a href="docs/a.md">doc</a> <a href="#sec">sec</a> <a href="javascript:x()">js</a>` + "\n\n" +
		"![m](img/m.png) [md](https://example.com/m)")
	wantAll(t, out,
		`<span class="img-text">net <code class="fpath">https://example.com/x.png</code></span>`,
		`<img src="/s/r/image?path=img%2Fa.png" alt="A" width="40"/>`,
		`<span class="img-text">O <code class="fpath">T:../out.png</code></span>`,
		`<span class="img-text">none</span>`,
		`<a href="https://example.com/p" target="_blank" rel="noopener noreferrer" title="t">web</a>`,
		`<a>doc</a>`, `<a href="#sec">sec</a>`, `<a>js</a>`,
		`<img src="/s/r/image?path=img%2Fm.png" alt="m"/>`,
		`<a href="https://example.com/m" target="_blank" rel="noopener noreferrer">md</a>`,
	)
	wantNone(t, out, trustAttr, `src="https://`, `href="docs`, "javascript")
}

func TestReplyHTMLSrcsetResolvesPerCandidate(t *testing.T) {
	out := servedReply(`<img src="img/a.png" srcset="https://evil.example/a.png 1x, img/b.png 2x, ../c.png 3x, img/d.png bogus">` + "\n\n" +
		`<img src="img/a.png" srcset="https://evil.example/a.png 1x">`)
	wantAll(t, out,
		`<img src="/s/r/image?path=img%2Fa.png" srcset="/s/r/image?path=img%2Fb.png 2x, /s/r/image?path=img%2Fd.png"/>`,
		`<img src="/s/r/image?path=img%2Fa.png"/>`,
	)
	wantNone(t, out, "evil", "c.png", "bogus")
}

// TestReplyHTMLTrustRefCannotBeForged pins writeTrustRef's guarantee: a mark
// the author writes or captures yields only a URL the markdown renderer
// resolved, of the same kind, never the author's own.
func TestReplyHTMLTrustRefCannotBeForged(t *testing.T) {
	out := servedReply("[ok](https://ok.example/)\n\n" +
		`<a href="javascript:alert(1)" data-wbref="0">forged</a> <img src="https://evil.example/x.png" data-wbref="0"> <a href="javascript:alert(2)" data-wbref="7">out of range</a>`)
	wantAll(t, out, `<a href="https://ok.example/" target="_blank" rel="noopener noreferrer">forged</a>`, `<a>out of range</a>`)
	wantNone(t, out, "javascript", `src="https://evil`)

	// An unclosed attribute at the end of an HTML block captures the next
	// markdown link's mark onto the author's anchor.
	out = servedReply("<div><a href=\"javascript:alert(1)\" title=\"\n\n[ok](https://ok.example/)")
	wantNone(t, out, "javascript")
}

func TestReplyRenderWithoutHTMLStaysEscaped(t *testing.T) {
	out := string(RenderText("<b>x</b> [l](https://example.com)", TextTargets{}))
	wantAll(t, out, "&lt;b&gt;x&lt;/b&gt;", `<a href="https://example.com" target="_blank" rel="noopener noreferrer">l</a>`)
	wantNone(t, out, trustAttr)
}

// TestTaskListAndFootnotes pins the two extensions on every pipeline: a task
// list renders disabled checkboxes, a footnote links its note, and two texts
// that each define [^1] keep apart ids, in a finished reply (sanitized) as
// on a board field and in the escaped streaming render.
func TestTaskListAndFootnotes(t *testing.T) {
	const list = "- [ ] todo\n- [x] done"
	a := "first[^1]\n\n[^1]: note a"
	b := "second[^1]\n\n[^1]: note b"
	targets := TextTargets{HTML: true}
	for name, tt := range map[string]TextTargets{"reply": targets, "escaped": {}} {
		t.Run(name, func(t *testing.T) {
			out := string(RenderText(list, tt))
			if n := strings.Count(out, `type="checkbox"`); n != 2 || strings.Count(out, "disabled") != 2 || strings.Count(out, "checked") != 1 {
				t.Errorf("want two disabled checkboxes, one checked:\n%s", out)
			}
			wantAll(t, out, "todo", "done")
			oa, ob := string(RenderText(a, tt)), string(RenderText(b, tt))
			pa, pb := footnoteIDs(t, oa), footnoteIDs(t, ob)
			if pa == pb {
				t.Errorf("two texts share footnote prefix %q", pa)
			}
			wantAll(t, oa, `<sup id="`+pa+`fnref:1"><a href="#`+pa+`fn:1"`, `<li id="`+pa+`fn:1">`, `href="#`+pa+`fnref:1"`, "note a")
			wantAll(t, ob, `<li id="`+pb+`fn:1">`)
			wantNone(t, oa, pb)
		})
	}
	// Stable across renders of the same text.
	if x, y := RenderText(a, targets), RenderText(a, targets); x != y {
		t.Errorf("same text rendered two ways:\n%s\n%s", x, y)
	}
}

// footnoteIDs returns the footnote id prefix out's first footnote carries.
func footnoteIDs(t *testing.T, out string) string {
	t.Helper()
	m := regexp.MustCompile(`id="(wbf-[0-9a-f]{8}-)fnref:1"`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no footnote id:\n%s", out)
	}
	return m[1]
}

// TestReplySanitizerKeepsOnlyDisabledCheckboxesAndFootnoteIDs pins the two
// sanitizer allowances the extensions need: an input is a disabled checkbox
// or gone, and an id survives only on a footnote's sup and li, and only with
// the text's own footnote prefix (a reply cannot forge another message's
// footnote ids).
func TestReplySanitizerKeepsOnlyDisabledCheckboxesAndFootnoteIDs(t *testing.T) {
	out := servedReply(`<input type="text" value="x" name="n"> <input type="checkbox" checked onclick="x()" name="n"> <input type="CHECKBOX">` +
		"\n\n" + `<p id="item-3">p</p> <span id="wbf-00000000-fn:1">x</span> <li id="wbf-zz-fn:1">`)
	wantAll(t, out, `<input type="checkbox" disabled="" checked=""/>`, `<input type="checkbox" disabled=""/>`)
	if n := strings.Count(out, "<input"); n != 2 {
		t.Errorf("want two inputs, got %d:\n%s", n, out)
	}
	wantNone(t, out, "text", "onclick", "name=", "item-3", `<span id=`, "wbf-zz")

	// The text's own footnote ids survive; a foreign prefix, well-formed or
	// not, does not.
	own := servedReply("own[^1]\n\n[^1]: note\n\n" + `<sup id="wbf-00000000-fnref:1">f</sup>`)
	pre := footnoteIDs(t, own)
	wantAll(t, own, `<sup id="`+pre+`fnref:1">`, `<li id="`+pre+`fn:1">`)
	wantNone(t, own, "wbf-00000000")
}
