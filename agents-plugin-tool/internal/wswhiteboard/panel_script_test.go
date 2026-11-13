package wswhiteboard

import (
	"strings"
	"testing"
)

// panelScript is the shape that broke in production: blank lines, &&, <
// comparisons, and a '<mark>' string inside a script.
const panelScript = `(function () {
  var rows = document.querySelectorAll('.x');

  if (rows.length > 0 && rows.length < 9) {
    var tag = '<mark>';

    for (var i = 0; i < rows.length; i++) {
      rows[i].innerHTML = tag + i + '</mark>';
    }
  }


  window.done = true;
})();`

// TestPanelScriptReachesFrameVerbatim: a <script> or <style> in a panel body
// reaches the frame byte for byte whatever precedes it and however many
// blank lines it holds, while the markdown around it still renders. The
// <div> line opens a CommonMark HTML block that goldmark ends at a blank
// line, which used to hand the rest of the script to markdown.
func TestPanelScriptReachesFrameVerbatim(t *testing.T) {
	const style = "<style>\n.x { color: red; }\n\n.y > .x { margin: 0 }\n</style>"
	const css = ".x { color: red; }\n\n.y > .x { margin: 0 }"
	cases := map[string]string{
		"style, div, script": style + "\n<div class=\"x\">\n  <b>hi</b>\n</div>\n<script>" + panelScript + "</script>",
		"div then script":    "<div class=\"x\">one</div>\n<script>" + panelScript + "</script>",
		"script on own line": "<div class=\"x\">one</div>\n<script>\n" + panelScript + "\n</script>\n",
		"script after text":  "intro\n<script>" + panelScript + "</script>",
		"span line":          "<span class=\"x\">one</span>\n<script>" + panelScript + "</script>",
		"uppercase tags":     "<DIV class=\"x\">one</DIV>\n<SCRIPT>" + panelScript + "</SCRIPT>",
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			payload += "\n\nafter **markdown**\n\n<script>var second = 1 &&\n\n2;</script>"
			st := &State{}
			mustApply(t, st, panelXML("mock", "mockup", payload))
			regions, err := RenderRegions(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
			if err != nil {
				t.Fatal(err)
			}
			doc := frameDocs(t, string(regions.Board))["entry:e1"]
			if !strings.Contains(doc, panelScript) {
				t.Errorf("panel script is not verbatim in the frame:\n%s", doc)
			}
			wantAll(t, doc, "var second = 1 &&\n\n2;</script>", "after <strong>markdown</strong>")
			if strings.Contains(name, "style") {
				wantAll(t, doc, css)
			}
			wantNone(t, doc[strings.Index(doc, "(function"):strings.Index(doc, "window.done")], "<p>", "<br")
		})
	}
}

// TestTopicBodyScriptReachesFrameVerbatim: wb:body shares the panel's
// pipeline, so it gets the same guarantee, and a <pre> with blank lines too.
func TestTopicBodyScriptReachesFrameVerbatim(t *testing.T) {
	body := "<div class=\"x\">one</div>\n<pre>\nline 1\n\nline 3 && <b>\n</pre>\n<div class=\"y\">two</div>\n<textarea>\na\n\nb\n</textarea>\n<script>" + panelScript + "</script>"
	st := &State{Seq: 1, Topics: []TopicDoc{{Name: "doc", Body: body, Changed: 1}}}
	regions, err := RenderRegions(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	if err != nil {
		t.Fatal(err)
	}
	doc := frameDocs(t, string(regions.Board))["doc"]
	wantAll(t, doc, panelScript, "<pre>\nline 1\n\nline 3 && <b>\n</pre>", "<textarea>\na\n\nb\n</textarea>")
}

// TestFencedRawTextTagsStayCode: a script tag shown in a fenced code block is
// still code, escaped, not raw HTML.
func TestFencedRawTextTagsStayCode(t *testing.T) {
	page := &pageData{}
	out := string(page.renderDoc("<div>x</div>\n\n```html\n<script>\nvar a = 1;\n\nvar b = 2;\n</script>\n```\n", nil))
	wantAll(t, out, "<pre><code", "&lt;script&gt;")
	wantNone(t, out, "<script>")
}

func TestRawTextOpenAfter(t *testing.T) {
	for _, c := range []struct{ line, open, want string }{
		{"<div>", "", ""},
		{"<script>", "", "script"},
		{"<script>a()</script>", "", ""},
		{"<script>a()</script><style>", "", "style"},
		{"<SCRIPT type=x>", "", "script"},
		{"</script> <pre>", "script", "pre"},
		{"var a = '<scripty>'", "", ""},
		{"x < y && z", "script", "script"},
		{"<textarea", "", "textarea"},
	} {
		if got := rawTextOpenAfter([]byte(c.line), c.open); got != c.want {
			t.Errorf("rawTextOpenAfter(%q, %q) = %q, want %q", c.line, c.open, got, c.want)
		}
	}
}
