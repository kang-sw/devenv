package wswhiteboard

import (
	"bytes"
	"io"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
	xhtml "golang.org/x/net/html"
)

// rawHTMLRenderer writes the raw pipeline's typed HTML as goldmark's unsafe
// renderer does, except that in a framed topic body or panel a raw <img>
// whose src is a file path takes what a markdown image of that path gets
// (rawImages). Agents write ![](path) but sometimes type <img src="path">;
// the frame's opaque origin cannot load a bare path, so without this the
// image shows broken.
type rawHTMLRenderer struct{ page *pageData }

func (r *rawHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindRawHTML, r.rawHTML)
	reg.Register(ast.KindHTMLBlock, r.htmlBlock)
}

func (r *rawHTMLRenderer) rawHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n := node.(*ast.RawHTML)
	var b []byte
	for i := 0; i < n.Segments.Len(); i++ {
		seg := n.Segments.At(i)
		b = append(b, seg.Value(source)...)
	}
	_, _ = w.Write(r.page.rawImages(b))
	return ast.WalkSkipChildren, nil
}

func (r *rawHTMLRenderer) htmlBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.HTMLBlock)
	if entering {
		var b []byte
		for i := 0; i < n.Lines().Len(); i++ {
			line := n.Lines().At(i)
			b = append(b, line.Value(source)...)
		}
		html.DefaultWriter.SecureWrite(w, r.page.rawImages(b))
	} else if n.HasClosure() {
		html.DefaultWriter.SecureWrite(w, n.ClosureLine.Value(source))
	}
	return ast.WalkContinue, nil
}

// rawImages is raw HTML b with each <img> tag rewritten by rawImageTag, in
// a framed topic body or panel only; b itself when no tag changes. Script
// and style content is never read as tags (the tokenizer's raw-text state).
func (p *pageData) rawImages(b []byte) []byte {
	if !p.framing || !p.inDoc || !bytes.Contains(bytes.ToLower(b), []byte("<img")) {
		return b
	}
	z := xhtml.NewTokenizer(bytes.NewReader(b))
	var out bytes.Buffer
	changed := false
	for {
		tt := z.Next()
		// TagName lowercases the buffer Raw returns, so copy it first.
		raw := append([]byte(nil), z.Raw()...)
		if tt == xhtml.ErrorToken {
			out.Write(raw)
			if z.Err() == io.EOF {
				break
			}
			return b
		}
		if tt == xhtml.StartTagToken || tt == xhtml.SelfClosingTagToken {
			if name, hasAttr := z.TagName(); hasAttr && string(name) == "img" {
				if tag, ok := p.rawImageTag(z); ok {
					out.WriteString(tag)
					changed = true
					continue
				}
			}
		}
		out.Write(raw)
	}
	if !changed {
		return b
	}
	return out.Bytes()
}

// rawImageTag is the current <img> tag of z rewritten when its src is a
// destination resolveTarget accepts for an image (a file path; never an
// http(s), data:, or other URL): src becomes the src a markdown image of
// that path gets (DocImageSrc's, ImageSrc's), or, for an image
// the frame takes only through its host (relayImage), is dropped for a
// data-relay the frame script fills (doc-frame.js). Its other attributes
// stay. false leaves the tag as typed.
func (p *pageData) rawImageTag(z *xhtml.Tokenizer) (string, bool) {
	var attrs [][2]string
	src, srcAt := "", -1
	for more := true; more; {
		var k, v []byte
		k, v, more = z.TagAttr()
		if string(k) == "src" && srcAt < 0 {
			src, srcAt = string(v), len(attrs)
		}
		attrs = append(attrs, [2]string{string(k), string(v)})
	}
	if srcAt < 0 {
		return "", false
	}
	t, ok := p.resolveTarget(src, true)
	switch {
	case !ok:
		return "", false
	case t.href != "":
		attrs[srcAt][1] = t.href
	case t.relay != "":
		attrs[srcAt] = [2]string{"data-relay", t.relay}
	default:
		return "", false
	}
	var s strings.Builder
	s.WriteString("<img")
	for _, a := range attrs {
		s.WriteString(" " + a[0] + `="` + xhtml.EscapeString(a[1]) + `"`)
	}
	s.WriteString(">")
	return s.String(), true
}
