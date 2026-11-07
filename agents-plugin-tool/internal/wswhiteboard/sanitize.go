package wswhiteboard

import (
	"bytes"
	"html/template"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/util"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// A finished lead reply renders markdown with raw HTML (TextTargets.HTML),
// and this file is the whole trust boundary on that HTML. The served pages
// send no CSP, so markup that slips through runs in the daemon origin, which
// accepts message and steer posts. The rendered fragment is therefore parsed
// and re-serialized, so a stray closing tag stays inside the reply and the
// browser only ever parses this file's output, and only allow-listed
// elements and attributes survive:
//
//   - replyDropped elements leave with their content; any other element
//     outside the allow-lists leaves and keeps its content.
//   - Every href, src, and srcset goes through resolveTarget, the markdown
//     link and image rule; a URL the markdown renderer already resolved is
//     taken from the page's record of it (writeTrustRef), never re-resolved.
//   - An inline style keeps only declarations free of url(, image-set(, src(,
//     expression(, @import, and backslash escapes, and loses position, so no
//     reply fetches through CSS or lays a layer over the page; the finished
//     bubble's contain: paint keeps the rest inside it.
//   - SVG keeps shapes, text, markers, and gradients: an id is prefixed with
//     svgIDPrefix, so it cannot collide with a page id, and a presentation
//     value may reference only such a local id through url(#...).

// trustAttr carries a writeTrustRef index on a link or image the markdown
// renderer resolved.
const trustAttr = "data-wbref"

// svgIDPrefix keeps a reply's SVG ids apart from the page's own ids.
const svgIDPrefix = "wbr-"

// trustedURL is one URL the markdown renderer resolved while a finished
// reply rendered, already URL-escaped as it wrote it.
type trustedURL struct {
	url   string
	image bool
}

func setOf(names ...string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

var (
	// replyDropped (HTML) and replySVGDropped run script, load, embed, or
	// submit another document, restyle the page outside the bubble, animate
	// an href, or hold raw text that is no reply content.
	replyDropped = setOf("script", "style", "template", "iframe", "frame", "frameset", "object", "embed",
		"applet", "noscript", "noembed", "noframes", "textarea", "title", "xmp", "plaintext", "select",
		"head", "base", "meta", "link", "video", "audio", "source", "track", "canvas")
	replySVGDropped = setOf("script", "style", "foreignobject", "animate", "animatecolor", "animatemotion",
		"animatetransform", "set", "use", "image", "a", "filter", "pattern")

	// replyElements maps each kept HTML element to the attributes it keeps
	// besides replyGlobal.
	replyElements = map[string]map[string]bool{}
	replyGlobal   = setOf("class", "title", "style", "dir", "lang", "role", "aria-label", "aria-hidden")

	replySVGElements = setOf("svg", "g", "defs", "title", "desc", "path", "rect", "circle", "ellipse",
		"line", "polyline", "polygon", "text", "tspan", "marker", "lineargradient", "radialgradient",
		"stop", "clippath", "mask")
	// replySVGAttrs are matched lower-cased; the parser's own spelling
	// (viewBox, refX) is what is written back.
	replySVGAttrs = setOf("class", "style", "id", "role", "aria-label", "aria-hidden",
		"viewbox", "preserveaspectratio", "width", "height", "x", "y", "x1", "y1", "x2", "y2",
		"cx", "cy", "r", "rx", "ry", "fx", "fy", "fr", "d", "points", "pathlength", "transform",
		"fill", "fill-opacity", "fill-rule", "stroke", "stroke-width", "stroke-opacity",
		"stroke-linecap", "stroke-linejoin", "stroke-dasharray", "stroke-dashoffset",
		"stroke-miterlimit", "opacity", "color", "display", "visibility", "font-family", "font-size",
		"font-weight", "font-style", "text-anchor", "dominant-baseline", "alignment-baseline",
		"baseline-shift", "letter-spacing", "word-spacing", "text-decoration", "dx", "dy", "rotate",
		"textlength", "lengthadjust", "marker-start", "marker-mid", "marker-end", "markerwidth",
		"markerheight", "markerunits", "refx", "refy", "orient", "offset", "stop-color",
		"stop-opacity", "gradientunits", "gradienttransform", "spreadmethod", "clip-path",
		"clip-rule", "clippathunits", "mask", "maskunits", "maskcontentunits", "vector-effect",
		"paint-order", "shape-rendering", "text-rendering")

	cssComment      = regexp.MustCompile(`(?s)/\*.*?(?:\*/|$)`)
	cssPropertyName = regexp.MustCompile(`^-{0,2}[a-z][a-z0-9-]*$`)
	// cssFetchOrEscape is what no kept CSS value holds: a resource fetch, a
	// legacy script hook, or an escape that could spell either.
	cssFetchOrEscape = regexp.MustCompile(`(?i)url\(|image-set\(|src\(|expression\(|@import|\\`)
	svgLocalRef      = regexp.MustCompile(`(?i)url\(\s*['"]?#([^'")\s\\]+)['"]?\s*\)`)
	srcsetDescriptor = regexp.MustCompile(`^[0-9.]+[wxh]$`)
)

func init() {
	for name, attrs := range map[string]string{
		"a": "href", "img": "src srcset alt width height",
		"p": "", "div": "", "span": "", "br": "", "hr": "", "wbr": "",
		"h1": "", "h2": "", "h3": "", "h4": "", "h5": "", "h6": "",
		"blockquote": "", "pre": "", "code": "", "kbd": "", "samp": "", "var": "",
		"em": "", "strong": "", "b": "", "i": "", "u": "", "s": "", "del": "", "ins": "", "mark": "",
		"small": "", "sub": "", "sup": "", "abbr": "", "cite": "", "dfn": "", "q": "", "bdi": "", "bdo": "",
		"time": "datetime",
		"ul": "", "ol": "start reversed type", "li": "value", "dl": "", "dt": "", "dd": "",
		"table": "width", "caption": "", "colgroup": "span width", "col": "span width",
		"thead": "", "tbody": "", "tfoot": "", "tr": "",
		"th": "colspan rowspan align valign width height scope", "td": "colspan rowspan align valign width height",
		"details": "open", "summary": "", "figure": "", "figcaption": "",
		"section": "", "article": "", "aside": "", "header": "", "footer": "",
	} {
		replyElements[name] = setOf(strings.Fields(attrs)...)
	}
}

// sanitizeReply parses the raw pipeline's output for a finished reply and
// writes back only what the allow-lists keep.
func (p *pageData) sanitizeReply(fragment string) template.HTML {
	context := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := xhtml.ParseFragment(strings.NewReader(fragment), context)
	if err != nil {
		// A strings.Reader never fails; escaped text is the safe fallback.
		return template.HTML(template.HTMLEscapeString(fragment))
	}
	root := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
	for _, n := range nodes {
		root.AppendChild(n)
	}
	p.sanitizeChildren(root)
	var buf bytes.Buffer
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if err := xhtml.Render(&buf, c); err != nil {
			return template.HTML(template.HTMLEscapeString(fragment))
		}
	}
	return template.HTML(strings.TrimRight(buf.String(), "\n"))
}

// sanitizeChildren keeps n's text, sanitizes its elements, and drops every
// other node (comments, doctypes).
func (p *pageData) sanitizeChildren(n *xhtml.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		switch c.Type {
		case xhtml.TextNode:
		case xhtml.ElementNode:
			p.sanitizeElement(c)
		default:
			n.RemoveChild(c)
		}
		c = next
	}
}

// sanitizeElement keeps, drops, or unwraps n. Its children are sanitized
// before an unwrap moves them up, so the caller's walk never revisits them.
func (p *pageData) sanitizeElement(n *xhtml.Node) {
	name := strings.ToLower(n.Data)
	parent := n.Parent
	switch n.Namespace {
	case "":
		if replyDropped[name] {
			parent.RemoveChild(n)
			return
		}
		if attrs, ok := replyElements[name]; ok {
			p.sanitizeChildren(n)
			p.keepHTMLAttrs(n, name, attrs)
			return
		}
	case "svg":
		if replySVGDropped[name] {
			parent.RemoveChild(n)
			return
		}
		if replySVGElements[name] {
			p.sanitizeChildren(n)
			keepSVGAttrs(n)
			return
		}
	}
	p.sanitizeChildren(n)
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		n.RemoveChild(c)
		parent.InsertBefore(c, n)
		c = next
	}
	parent.RemoveChild(n)
}

// keepHTMLAttrs keeps n's global and allowed attributes, filters its style,
// and hands the URL attributes of a link or an image to their resolvers.
func (p *pageData) keepHTMLAttrs(n *xhtml.Node, name string, allowed map[string]bool) {
	var kept []xhtml.Attribute
	urls := map[string]string{}
	ref := -1
	for _, a := range n.Attr {
		key := strings.ToLower(a.Key)
		switch {
		case a.Namespace != "":
			continue
		case key == trustAttr:
			if i, err := strconv.Atoi(a.Val); err == nil {
				ref = i
			}
			continue
		case !replyGlobal[key] && !allowed[key]:
			continue
		case key == "href" || key == "src" || key == "srcset":
			urls[key] = a.Val
			continue
		case key == "style":
			if a.Val = filterStyle(a.Val); a.Val == "" {
				continue
			}
		}
		kept = append(kept, xhtml.Attribute{Key: key, Val: a.Val})
	}
	n.Attr = kept
	switch name {
	case "a":
		p.keepAnchorHref(n, urls["href"], ref)
	case "img":
		p.keepImageSrc(n, urls, ref)
	}
}

// trustedRef returns the URL writeTrustRef recorded at i when it is of the
// asked kind.
func (p *pageData) trustedRef(i int, image bool) (string, bool) {
	if p.trusted == nil || i < 0 || i >= len(*p.trusted) {
		return "", false
	}
	t := (*p.trusted)[i]
	return t.url, t.image == image
}

// keepAnchorHref gives a link the href resolveTarget allows, with the
// attributes every resolved link carries; a refused target, or one that
// resolves to path text, leaves the anchor without an href, keeping its text.
func (p *pageData) keepAnchorHref(n *xhtml.Node, raw string, ref int) {
	href, ok := p.trustedRef(ref, false)
	if !ok {
		href = ""
		if resolved, path, ok := p.resolveTarget(raw, false); ok && path == "" {
			href = string(util.URLEscape([]byte(resolved), false))
		}
	}
	if href == "" {
		return
	}
	attrs := []xhtml.Attribute{{Key: "href", Val: href}}
	for _, a := range anchorAttrs(href) {
		attrs = append(attrs, xhtml.Attribute{Key: a[0], Val: a[1]})
	}
	n.Attr = append(attrs, n.Attr...)
}

// keepImageSrc gives an image the src resolveTarget allows, and the srcset
// candidates it allows; an image whose src it refuses, or resolves to path
// text, becomes its alt text and that path, as a markdown image does.
func (p *pageData) keepImageSrc(n *xhtml.Node, urls map[string]string, ref int) {
	src, path := "", ""
	if trusted, ok := p.trustedRef(ref, true); ok {
		src = trusted
	} else if raw := strings.TrimSpace(urls["src"]); raw != "" {
		resolved, text, ok := p.resolveTarget(raw, true)
		switch {
		case ok && text == "":
			src = string(util.URLEscape([]byte(resolved), false))
		case ok:
			path = text
		default:
			path = raw
		}
	}
	if src == "" {
		replaceImage(n, path)
		return
	}
	attrs := []xhtml.Attribute{{Key: "src", Val: src}}
	if set := p.resolveSrcset(urls["srcset"]); set != "" {
		attrs = append(attrs, xhtml.Attribute{Key: "srcset", Val: set})
	}
	n.Attr = append(attrs, n.Attr...)
}

// resolveSrcset keeps each candidate whose URL resolveTarget serves as an
// image, with its width or density descriptor.
func (p *pageData) resolveSrcset(set string) string {
	var kept []string
	for _, candidate := range strings.Split(set, ",") {
		fields := strings.Fields(candidate)
		if len(fields) == 0 {
			continue
		}
		resolved, path, ok := p.resolveTarget(fields[0], true)
		if !ok || path != "" {
			continue
		}
		out := string(util.URLEscape([]byte(resolved), false))
		for _, d := range fields[1:] {
			if srcsetDescriptor.MatchString(d) {
				out += " " + d
			}
		}
		kept = append(kept, out)
	}
	return strings.Join(kept, ", ")
}

// replaceImage puts an image's alt text and path in its place, the shape a
// markdown image on a served page takes (mdRenderer.image).
func replaceImage(n *xhtml.Node, path string) {
	alt := ""
	for _, a := range n.Attr {
		if a.Key == "alt" {
			alt = a.Val
		}
	}
	parent := n.Parent
	if alt != "" || path != "" {
		span := &xhtml.Node{Type: xhtml.ElementNode, Data: "span", DataAtom: atom.Span, Attr: []xhtml.Attribute{{Key: "class", Val: "img-text"}}}
		if alt != "" {
			span.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: alt})
		}
		if path != "" {
			code := &xhtml.Node{Type: xhtml.ElementNode, Data: "code", DataAtom: atom.Code, Attr: []xhtml.Attribute{{Key: "class", Val: "fpath"}}}
			code.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: path})
			span.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: " "})
			span.AppendChild(code)
		}
		parent.InsertBefore(span, n)
	}
	parent.RemoveChild(n)
}

// keepSVGAttrs keeps an SVG element's allowed attributes: style filtered,
// id prefixed, and every other value checked as a CSS value (svgValue),
// since the browser parses a presentation attribute as one.
func keepSVGAttrs(n *xhtml.Node) {
	var kept []xhtml.Attribute
	for _, a := range n.Attr {
		key := strings.ToLower(a.Key)
		if a.Namespace != "" || !replySVGAttrs[key] {
			continue
		}
		switch key {
		case "style":
			if a.Val = filterStyle(a.Val); a.Val == "" {
				continue
			}
		case "id":
			a.Val = svgIDPrefix + a.Val
		case "class", "role", "aria-label", "aria-hidden":
		default:
			v, ok := svgValue(a.Val)
			if !ok {
				continue
			}
			a.Val = v
		}
		kept = append(kept, xhtml.Attribute{Key: a.Key, Val: a.Val})
	}
	n.Attr = kept
}

// svgValue rewrites every local url(#id) reference to the prefixed id and
// refuses a value that still fetches, scripts, or escapes.
func svgValue(v string) (string, bool) {
	if cssFetchOrEscape.MatchString(svgLocalRef.ReplaceAllString(v, "")) {
		return "", false
	}
	return svgLocalRef.ReplaceAllString(v, "url(#"+svgIDPrefix+"$1)"), true
}

// filterStyle keeps the declarations of an inline style that name a plain
// property other than position and whose value passes cssFetchOrEscape.
// Comments go first, so none can split a name or a function the check reads;
// a split at a semicolon inside a string or parentheses only drops more.
func filterStyle(style string) string {
	style = cssComment.ReplaceAllString(style, "")
	var kept []string
	for _, decl := range strings.Split(style, ";") {
		name, value, ok := strings.Cut(decl, ":")
		name = strings.ToLower(strings.TrimSpace(name))
		value = strings.TrimSpace(value)
		if !ok || value == "" || name == "position" || !cssPropertyName.MatchString(name) || cssFetchOrEscape.MatchString(value) {
			continue
		}
		kept = append(kept, name+": "+value)
	}
	return strings.Join(kept, "; ")
}
