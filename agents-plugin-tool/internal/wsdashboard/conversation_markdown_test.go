package wsdashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// leadMarkdownView renders the conversation region of a live session whose
// ring holds the given entries and whose lead streams reply (empty for none).
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
	regions, err := renderSessionRegions(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(regions[regionConversation].HTML)
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
