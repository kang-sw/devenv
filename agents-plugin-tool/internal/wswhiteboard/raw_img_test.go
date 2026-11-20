package wswhiteboard

import (
	"strings"
	"testing"
)

// TestRawImageTagTakesTheMarkdownImagePath pins the raw <img> safety net: in
// a framed topic body and a framed panel, a typed <img> whose src is a file
// path gets what ![](path) gets, keeping its other attributes: an in-root
// path the inlined DocImageSrc data, an outside path no src but the relay
// marker, listed on the frame element for the host's fetch. An http(s) or
// data: src and a tag inside a script stay as typed.
func TestRawImageTagTakesTheMarkdownImagePath(t *testing.T) {
	const raw = "<img src=\"/x/out.png\" alt=\"out\" width=\"40\" style=\"border:1px solid\">\n\n" +
		"in <img src=\"shots/a.png\" alt=\"in\"/> web <img src=\"https://example.com/w.png\"> inline <img src=\"data:image/png;base64,QQ==\">\n\n" +
		"<script>var s = '<img src=\"/x/out.png\">';</script>\n"
	st := &State{Seq: 2, Topics: []TopicDoc{{Name: "doc", Body: raw, Changed: 2,
		Entries: []Entry{{ID: "e1", Kind: EntryPanel, PanelID: "p1", Recap: "mock", Text: raw}}}}}
	in := ViewInput{Layout: LayoutFull, ReplyID: "r",
		FileText: func(p string) string { return p },
		ImageSrc: func(string) string { return "" },
		DocImageSrc: func(p string) string {
			if p == "shots/a.png" {
				return "data:image/png;base64,AAA"
			}
			return ""
		},
		FileActions: func(p string) FileAction { return FileAction{Abs: p} },
		OutsideImage: func(p string) FileAction {
			if p == "/x/out.png" {
				return FileAction{Href: "/s/r/image?path=/x/out.png", Abs: p, Image: true, Outside: true}
			}
			return FileAction{}
		}}
	regions, err := RenderRegions(BuildView(st, in))
	if err != nil {
		t.Fatal(err)
	}
	board := string(regions.Board)
	docs := frameDocs(t, board)
	if len(docs) != 2 {
		t.Fatalf("want a body frame and a panel frame, got %d: %v", len(docs), keys(docs))
	}
	for name, doc := range docs {
		wantAll(t, doc,
			`<img data-relay="/s/r/image?path=/x/out.png" alt="out" width="40" style="border:1px solid">`,
			`<img src="data:image/png;base64,AAA" alt="in">`,
			`<img src="https://example.com/w.png">`,
			`<img src="data:image/png;base64,QQ==">`,
			`var s = '<img src="/x/out.png">';`,
		)
		if strings.Count(doc, `src="/x/out.png"`) != 1 || strings.Contains(doc, `src="shots/a.png"`) {
			t.Errorf("frame %s keeps a raw file path src:\n%s", name, doc)
		}
	}
	if n := strings.Count(board, `data-relay="/s/r/image?path=/x/out.png" data-sandbox=`); n != 2 {
		t.Errorf("frames listing the raw image's relay = %d, want 2 (body and panel)", n)
	}
}

// TestRawImagesLeavesOtherHTMLByteIdentical: raw HTML holding no rewritten
// <img> is written exactly as typed.
func TestRawImagesLeavesOtherHTMLByteIdentical(t *testing.T) {
	p := &pageData{}
	p.framing, p.inDoc = true, true
	for _, s := range []string{
		`<div class=x><IMG alt=no-src> <img src="https://e/x.png"></div>`,
		`<img src="data:image/png;base64,QQ==" >`,
		`<img src="/x.png"`,
		`<style>img{}</style><p>&amp; <b>x</b>`,
	} {
		if got := string(p.rawImages([]byte(s))); got != s {
			t.Errorf("rawImages(%q) = %q", s, got)
		}
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
