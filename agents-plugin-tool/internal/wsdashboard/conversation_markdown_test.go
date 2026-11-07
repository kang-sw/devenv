package wsdashboard

import (
	"path/filepath"
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

func TestLeadReplyEscapesRawHTMLAndUnsafeTargets(t *testing.T) {
	text := "<script>alert(1)</script> and <b>bold</b>\n\n[x](javascript:alert(1)) [ok](https://example.com) [f](docs/a.md)\n\n![i](../outside.png) ![in](img/a.png)"
	html := leadMarkdownView(t, []wslive.RingEntry{{Kind: wslive.RingReply, Text: text}}, "")
	for _, bad := range []string{"<script>", "<b>bold", "javascript:", `src="../outside.png"`, "file:"} {
		if strings.Contains(html, bad) {
			t.Errorf("reply carries %q:\n%s", bad, html)
		}
	}
	for _, want := range []string{"&lt;script&gt;", "&lt;b&gt;bold", `href="https://example.com"`, "docs/a.md", `src="/s/self/image?path=img%2Fa.png"`} {
		if !strings.Contains(html, want) {
			t.Errorf("reply lacks %q:\n%s", want, html)
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
