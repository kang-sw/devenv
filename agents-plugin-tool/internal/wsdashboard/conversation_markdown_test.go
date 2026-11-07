package wsdashboard

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// leadMarkdownView renders the conversation regions (entries, then convlive)
// of a live session whose ring holds the given entries and whose lead
// streams reply (empty for none).
func leadMarkdownView(t *testing.T, ring []wslive.RingEntry, reply string) string {
	t.Helper()
	now := testNow
	recs := make([]wslive.RingRecord, len(ring))
	for i, e := range ring {
		recs[i] = wslive.RingRecord{Name: "n" + string(rune('a'+i)), At: now.Add(time.Duration(i-len(ring)) * time.Minute), RingEntry: e}
	}
	lead := wslive.Lead{}
	if reply != "" {
		lead.Reply = &wslive.Reply{Text: reply}
	}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/proj", now), Snapshot: liveSnap(now, lead), Ring: recs, Now: now})
	regions, err := renderSessionRegions(v, nil)
	if err != nil {
		t.Fatal(err)
	}
	return string(regions[regionEntries].HTML) + string(regions[regionConvLive].HTML)
}

func TestLeadReplyRendersAsMarkdown(t *testing.T) {
	text := "# Plan\n\n- one\n- two\n\n```go\nfmt.Println(1)\n```\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"
	html := leadMarkdownView(t, []wslive.RingEntry{{Kind: wslive.RingReply, Text: text}}, "")
	for _, want := range []string{
		`<div class="bub md">`, "<h1>Plan</h1>", "<li>one</li>", `<pre><code class="language-go">fmt.Println(1)`, "<table>", "<td>2</td>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("reply lacks %q:\n%s", want, html)
		}
	}
}

// TestFinishedLeadReplyRendersSanitizedHTML pins a finished reply's inline
// HTML: kept, inline SVG included, inside the sanitizer's allow-list, with
// its URLs under the markdown link rule (wswhiteboard sanitize_test.go holds
// the full vector list).
func TestFinishedLeadReplyRendersSanitizedHTML(t *testing.T) {
	text := "<script>alert(1)</script> and <b>bold</b>\n\n<svg viewBox=\"0 0 20 10\"><rect width=\"8\" height=\"8\"/><text x=\"10\" y=\"8\">s</text></svg>\n\n" +
		"[x](javascript:alert(1)) [ok](https://example.com) [f](docs/a.md) <a href=\"docs/b.md\">raw f</a> <img src=\"https://example.com/n.png\" onerror=\"alert(2)\">\n\n![i](../outside.png) ![in](img/a.png)"
	html := leadMarkdownView(t, []wslive.RingEntry{{Kind: wslive.RingReply, Text: text}}, "")
	for _, bad := range []string{"<script>", "alert", "javascript:", `src="../outside.png"`, `src="https://`, "file:", `href="docs`} {
		if strings.Contains(html, bad) {
			t.Errorf("reply carries %q:\n%s", bad, html)
		}
	}
	for _, want := range []string{
		"<b>bold</b>", `<svg viewBox="0 0 20 10"><rect width="8" height="8"></rect><text x="10" y="8">s</text></svg>`,
		`<a href="https://example.com" target="_blank" rel="noopener noreferrer" data-wl="url">ok</a>`,
		`<a href="/s/self/file?path=docs%2Fa.md" data-wl="file" data-rel="docs/a.md" data-abs="` + filepath.Join("/proj", "docs", "a.md") + `" title="docs/a.md">f</a>`,
		`<a href="/s/self/file?path=docs%2Fb.md" data-wl="file" data-rel="docs/b.md" data-abs="` + filepath.Join("/proj", "docs", "b.md") + `" title="docs/b.md">raw f</a>`,
		`<span class="img-text"> <code class="fpath">https://example.com/n.png</code></span>`,
		`<span class="img-text">i <a tabindex="0" data-wl="file" data-abs="` + filepath.Join("/", "outside.png") + `" title="../outside.png"><code class="fpath">../outside.png</code></a></span>`,
		`src="/s/self/image?path=img%2Fa.png"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("reply lacks %q:\n%s", want, html)
		}
	}
}

// TestStreamingReplyShowsHTMLAsText pins the streaming rule: raw HTML stays
// text until the reply finishes, so a half-closed tag cannot shake the page.
func TestStreamingReplyShowsHTMLAsText(t *testing.T) {
	html := leadMarkdownView(t, nil, "<b>bold</b> and <details><summary>s")
	if !strings.Contains(html, "&lt;b&gt;bold&lt;/b&gt; and &lt;details&gt;&lt;summary&gt;s") || strings.Contains(html, "<b>bold") || strings.Contains(html, "<details>") {
		t.Errorf("streaming reply renders HTML:\n%s", html)
	}
}

// TestStrayClosingTagStaysInsideTheBubble pins the balancing: a finished
// reply's stray closing tags cannot close the bubble or the entry.
func TestStrayClosingTagStaysInsideTheBubble(t *testing.T) {
	html := leadMarkdownView(t, []wslive.RingEntry{{Kind: wslive.RingReply, Text: "</div></div></div><b>x"}}, "")
	if !strings.Contains(html, `<div class="bub md"><b>x</b></div></div>`) {
		t.Errorf("stray closing tags leaked out of the bubble:\n%s", html)
	}
}

// TestFinishedReplyCannotOverlayThePage pins the overlay guard: a reply's
// position:fixed is removed, and the finished bubble's paint containment
// keeps whatever else it draws inside it.
func TestFinishedReplyCannotOverlayThePage(t *testing.T) {
	html := leadMarkdownView(t, []wslive.RingEntry{{Kind: wslive.RingReply, Text: `<div style="position:fixed;inset:0;z-index:999;background:red">covered</div>`}}, "")
	if !strings.Contains(html, `<div style="inset: 0; z-index: 999; background: red">covered</div>`) || strings.Contains(html, "fixed") {
		t.Errorf("reply keeps a fixed layer:\n%s", html)
	}
	a, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(a.data.CSS), ".msg.fromlead[data-entry] > .bub.md { contain: paint; }") {
		t.Error("the finished reply bubble does not contain its paint")
	}
}

// TestMailAndUserEntriesKeepHTMLAsText pins the trust boundary: only a lead
// reply renders HTML; a user message and a mail body, from another author,
// show it as text.
func TestMailAndUserEntriesKeepHTMLAsText(t *testing.T) {
	now := testNow
	recs := []wslive.RingRecord{
		{Name: "na", At: now.Add(-2 * time.Minute), RingEntry: wslive.RingEntry{Kind: wslive.RingUser, Text: "<b>user</b>"}},
		{Name: "nb", At: now.Add(-time.Minute), RingEntry: wslive.RingEntry{Kind: wslive.RingMail, Direction: wslive.MailRecv, Peer: "id:peer", Title: "<i>title</i>", HistoryKey: "h1"}},
	}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/proj", now), Snapshot: liveSnap(now, wslive.Lead{}), Ring: recs,
		Page: PageContent{MailBodies: map[string]string{"h1": "<i>title</i>\n<script>x()</script>"}}, Now: now})
	regions, err := renderSessionRegions(v, nil)
	if err != nil {
		t.Fatal(err)
	}
	html := string(regions[regionEntries].HTML)
	for _, want := range []string{"&lt;b&gt;user&lt;/b&gt;", "&lt;i&gt;title&lt;/i&gt;", "&lt;script&gt;x()&lt;/script&gt;"} {
		if !strings.Contains(html, want) {
			t.Errorf("entries lack %q:\n%s", want, html)
		}
	}
	for _, bad := range []string{"<b>user", "<i>title", "<script>"} {
		if strings.Contains(html, bad) {
			t.Errorf("entries render %q as HTML:\n%s", bad, html)
		}
	}
}

// TestFinishedReplyDrawsMermaidStreamingKeepsCode: a finished reply's
// mermaid fence is a <pre class="mermaid"> the page draws once; the reply
// still streaming keeps it a code block, since it re-renders every tick.
func TestFinishedReplyDrawsMermaidStreamingKeepsCode(t *testing.T) {
	fence := "```mermaid\ngraph TD; A-->B\n```"
	finished := leadMarkdownView(t, []wslive.RingEntry{{Kind: wslive.RingReply, Text: fence}}, "")
	if !strings.Contains(finished, `<pre class="mermaid">graph TD; A--&gt;B`) || strings.Contains(finished, `language-mermaid`) {
		t.Errorf("finished reply does not draw mermaid: %s", finished)
	}
	streaming := leadMarkdownView(t, nil, fence)
	if strings.Contains(streaming, `class="mermaid"`) || !strings.Contains(streaming, `<code class="language-mermaid">`) {
		t.Errorf("streaming reply draws mermaid: %s", streaming)
	}
}

// TestLeadReplyLinksAreActionLinks: a finished and a streaming reply mark
// their links for the page's link menu: a file inside the session root
// links its file view with its root-relative and absolute paths, a file
// outside it is a copy-only action link with no href, and an http(s) link
// is marked as a URL. A user message's text has no links at all.
func TestLeadReplyLinksAreActionLinks(t *testing.T) {
	text := "[plan](docs/a.md) [away](/etc/hosts) [web](https://example.com/x)"
	for name, html := range map[string]string{
		"finished":  leadMarkdownView(t, []wslive.RingEntry{{Kind: wslive.RingReply, Text: text}}, ""),
		"streaming": leadMarkdownView(t, nil, text),
	} {
		for _, want := range []string{
			`<a href="/s/self/file?path=docs%2Fa.md" data-wl="file" data-rel="docs/a.md" data-abs="` + filepath.Join("/proj", "docs", "a.md") + `" title="docs/a.md">plan</a>`,
			`<a tabindex="0" data-wl="file" data-abs="` + filepath.FromSlash("/etc/hosts") + `" title="/etc/hosts">away</a>`,
			`<a href="https://example.com/x" target="_blank" rel="noopener noreferrer" data-wl="url">web</a>`,
		} {
			if !strings.Contains(html, want) {
				t.Errorf("%s reply lacks %q:\n%s", name, want, html)
			}
		}
	}
	user := leadMarkdownView(t, []wslive.RingEntry{{Kind: wslive.RingUser, Text: text}}, "")
	if strings.Contains(user, "data-wl") {
		t.Errorf("user message carries an action link: %s", user)
	}
}

// TestFinishedReplyRawLinksMatchMarkdownLinks pins the sanitizer to the
// link-actions rule: a raw <a> or <img> in a finished reply renders exactly
// as the markdown link or image to the same target does, action-link and url
// markers included, while a refused href leaves the anchor's text alone and a
// refused src leaves the image's path as text.
func TestFinishedReplyRawLinksMatchMarkdownLinks(t *testing.T) {
	render := replyMarkdown("/proj", "self").finished
	for _, target := range []string{"docs/a.md", "/etc/hosts", "../up.md", "https://example.com/x?a=1&b=2", "#sec"} {
		md, raw := string(render("[L]("+target+")")), string(render(`<a href="`+target+`">L</a>`))
		if md != raw || !strings.HasPrefix(md, "<a ") {
			t.Errorf("link %q:\nmarkdown %s\nraw      %s", target, md, raw)
		}
	}
	for _, target := range []string{"img/a.png", "docs/a.md", "/etc/x.png", "../up.png"} {
		md, raw := string(render("![o]("+target+")")), string(render(`<img alt="o" src="`+target+`">`))
		if md != raw {
			t.Errorf("image %q:\nmarkdown %s\nraw      %s", target, md, raw)
		}
	}
	for _, target := range []string{"javascript:alert(1)", "//evil.example/x", "mailto:a@b.example", "data:text/html,x", ""} {
		if got := string(render(`<a href="` + target + `">L</a>`)); got != "<a>L</a>" {
			t.Errorf("refused link %q: %s", target, got)
		}
	}
	for target, want := range map[string]string{
		"https://example.com/n.png": `<span class="img-text">o <code class="fpath">https://example.com/n.png</code></span>`,
		"javascript:alert(1)":       `<span class="img-text">o <code class="fpath">javascript:alert(1)</code></span>`,
		"":                          `<span class="img-text">o</span>`,
	} {
		if got := string(render(`<img alt="o" src="` + target + `">`)); got != want {
			t.Errorf("refused image %q: %s, want %s", target, got, want)
		}
	}
	if got := string(render(`<a href="https://example.com/x" data-wl="file" data-abs="/etc/passwd" data-rel="x" tabindex="0" data-wbref="7">forged</a> <a data-wl="url" data-wbref="0">bare</a>`)); got !=
		`<a href="https://example.com/x" target="_blank" rel="noopener noreferrer" data-wl="url">forged</a> <a>bare</a>` {
		t.Errorf("forged link markers survive: %s", got)
	}
}

// TestFinishedReplyCannotReachSystemOpen aims a finished reply at the
// system-open route (sysopen_route.go), which trusts a same-origin post from
// a loopback viewer: a text/plain form forging its JSON body, a formaction
// button, a raw link and image to the route, an SVG link, and a script
// handler. None may leave a form, a script hook, or a URL attribute that
// reaches the route; the route path survives only as copy-only path text.
func TestFinishedReplyCannotReachSystemOpen(t *testing.T) {
	text := `<form action="/s/self/open" method="post" enctype="text/plain"><input name='{"target":"https://evil.example/","x":"' value='"}'><button>go</button></form>` + "\n\n" +
		`<button formaction="/s/self/open" formmethod="post" formenctype="text/plain" name='{"target":"https://evil.example/"}'>b</button> ` +
		`<a href="/s/self/open" data-wl="url">o</a> <img alt="i" src="/s/self/open"> ` +
		`<img src="x" onerror="fetch('/s/self/open',{method:'POST',body:'{}'})">` + "\n\n" +
		`<svg><a href="/s/self/open"><text>s</text></a></svg>` + "\n\n" +
		`<a href="javascript:fetch('/s/self/open')">j</a> <a href="/s/self/open?target=https://evil.example/" formaction="/s/self/open">q</a>`
	got := string(replyMarkdown("/proj", "self").finished(text))
	for _, bad := range []string{"<form", "<input", "<button", "formaction", "enctype", "onerror", "fetch(", "javascript:", "evil.example/\"", `data-wl="url"`} {
		if strings.Contains(got, bad) {
			t.Errorf("reply keeps %q:\n%s", bad, got)
		}
	}
	if m := regexp.MustCompile(`\s(?:href|src|srcset|action)="[^"]*/open`).FindString(got); m != "" {
		t.Errorf("reply keeps a URL attribute to the route (%s):\n%s", m, got)
	}
	for _, want := range []string{
		`<a tabindex="0" data-wl="file" data-abs="` + filepath.FromSlash("/s/self/open") + `" title="/s/self/open">o</a>`,
		`<span class="img-text">i <a tabindex="0" data-wl="file" data-abs="` + filepath.FromSlash("/s/self/open") + `" title="/s/self/open"><code class="fpath">/s/self/open</code></a></span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("reply lacks %q:\n%s", want, got)
		}
	}
}

func TestStreamingReplyRendersPartialMarkdownWithCaret(t *testing.T) {
	html := leadMarkdownView(t, nil, "**done** so far\n\n```sh\nmake all")
	for _, want := range []string{
		`class="msg fromlead streaming"`, "<strong>done</strong>", `<pre><code class="language-sh">make all`, `<span class="caret"></span>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("streaming reply lacks %q:\n%s", want, html)
		}
	}
}

func TestUserEntryStaysPlainText(t *testing.T) {
	html := leadMarkdownView(t, []wslive.RingEntry{{Kind: wslive.RingUser, Text: "# not a heading *nor* <b>bold</b>"}}, "")
	if !strings.Contains(html, `<div class="bub">`) || !strings.Contains(html, "# not a heading *nor* &lt;b&gt;bold&lt;/b&gt;") {
		t.Errorf("user entry is not plain text: %s", html)
	}
	if strings.Contains(html, "<h1>") || strings.Contains(html, "<em>") {
		t.Errorf("user entry rendered as markdown: %s", html)
	}
}

func TestReplyRenderIsDeterministic(t *testing.T) {
	ring := []wslive.RingEntry{{Kind: wslive.RingReply, Text: "- a\n- b\n\n![i](img/a.png)"}}
	if leadMarkdownView(t, ring, "") != leadMarkdownView(t, ring, "") {
		t.Error("identical replies render differently")
	}
}
