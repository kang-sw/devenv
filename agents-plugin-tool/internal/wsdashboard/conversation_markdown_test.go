package wsdashboard

import (
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
		`href="https://example.com"`, "docs/a.md", "<a>raw f</a>", `<code class="fpath">https://example.com/n.png</code>`,
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

func TestLeadReplyMermaidFenceStaysACodeBlock(t *testing.T) {
	html := leadMarkdownView(t, []wslive.RingEntry{{Kind: wslive.RingReply, Text: "```mermaid\ngraph TD; A-->B\n```"}}, "")
	if strings.Contains(html, `class="mermaid"`) || !strings.Contains(html, `<code class="language-mermaid">`) {
		t.Errorf("mermaid fence in a bubble: %s", html)
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
