package wswhiteboard

import (
	"bytes"
	"fmt"
	"hash/fnv"
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
//     link and image rule, and a resolved link opens with the attributes the
//     markdown writer gives it (linkAttrs, actionAttrs), so a served page's
//     link menu takes it like any other; a link or image the markdown
//     renderer already resolved takes its attributes from the page's record
//     of them (writeTrustRef), never from the markup.
//   - An inline style keeps only declarations free of url(, image-set(, src(,
//     expression(, @import, and backslash escapes, and loses position, so no
//     reply fetches through CSS or lays a layer over the page; the finished
//     bubble's contain: paint keeps the rest inside it.
//   - SVG keeps shapes, text, markers, and gradients: an id is prefixed with
//     svgIDPrefix and a hash of the reply, so it collides neither with a page
//     id nor with another reply's same-named id, and a presentation value may
//     reference only such a local id through url(#...).

// trustAttr carries a writeTrustRef index on a link or image the markdown
// renderer resolved.
const trustAttr = "data-wbref"

// svgIDPrefix keeps a reply's SVG ids apart from the page's own ids; the
// reply hash after it (sanitizeReply) keeps them apart from another reply's.
const svgIDPrefix = "wbr-"

// trustedTarget is the attributes a link or image the markdown renderer
// resolved while a finished reply rendered opened with (linkAttrs,
// actionAttrs, or an image's src), its URL already URL-escaped.
type trustedTarget struct {
	attrs [][2]string
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
		"small": "", "sub": "", "sup": "", "abbr": "", "cite": "", "dfn": "", "q": "", "bdi": "", "bdo": "", "time": "datetime",
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
	h := fnv.New32a()
	h.Write([]byte(fragment))
	p.svgIDs = fmt.Sprintf("%s%08x-", svgIDPrefix, h.Sum32())
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
			keepSVGAttrs(n, p.svgIDs)
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

// trustedRef returns the attributes writeTrustRef recorded at i when they
// are of the asked kind.
func (p *pageData) trustedRef(i int, image bool) ([][2]string, bool) {
	if p.trusted == nil || i < 0 || i >= len(*p.trusted) {
		return nil, false
	}
	t := (*p.trusted)[i]
	return t.attrs, t.image == image
}

// keepAnchorHref gives a link the attributes the markdown link writer would
// open it with: an href resolveTarget allows with linkAttrs, or a file
// target's action link with actionAttrs, titled with its path unless it has
// a title. A refused target, or one that resolves to path text, leaves the
// anchor without an href, keeping its text.
func (p *pageData) keepAnchorHref(n *xhtml.Node, raw string, ref int) {
	attrs, ok := p.trustedRef(ref, false)
	if !ok {
		attrs = nil
		t, ok := p.resolveTarget(raw, false)
		switch {
		case !ok:
		case t.isAction():
			attrs = actionAttrs(t)
			if !hasAttr(n, "title") {
				n.Attr = append(n.Attr, xhtml.Attribute{Key: "title", Val: t.text})
			}
		case t.text == "":
			attrs = p.linkAttrs(t.href)
		}
	}
	n.Attr = append(nodeAttrs(attrs), n.Attr...)
}

// keepImageSrc gives an image the src resolveTarget allows, and the srcset
// candidates it allows; an image whose src it refuses, or resolves to path
// text, becomes its alt text and that path, as a markdown image does, the
// path an action link when the target has one.
func (p *pageData) keepImageSrc(n *xhtml.Node, urls map[string]string, ref int) {
	attrs, ok := p.trustedRef(ref, true)
	if !ok {
		attrs = nil
		raw := strings.TrimSpace(urls["src"])
		t, ok := p.resolveTarget(raw, true)
		switch {
		case !ok:
			replaceImage(n, raw, target{})
			return
		case t.text != "":
			replaceImage(n, t.text, t)
			return
		}
		attrs = [][2]string{{"src", string(util.URLEscape([]byte(t.href), false))}}
	}
	if set := p.resolveSrcset(urls["srcset"]); set != "" {
		attrs = append(attrs, [2]string{"srcset", set})
	}
	n.Attr = append(nodeAttrs(attrs), n.Attr...)
}

// nodeAttrs turns writer attribute pairs into parsed-node attributes.
func nodeAttrs(attrs [][2]string) []xhtml.Attribute {
	out := make([]xhtml.Attribute, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, xhtml.Attribute{Key: a[0], Val: a[1]})
	}
	return out
}

// hasAttr reports whether n carries the attribute key.
func hasAttr(n *xhtml.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
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
		t, ok := p.resolveTarget(fields[0], true)
		if !ok || t.text != "" {
			continue
		}
		out := string(util.URLEscape([]byte(t.href), false))
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
// markdown image on a served page takes (mdRenderer.image): the path inside
// t's action link when t is one.
func replaceImage(n *xhtml.Node, path string, t target) {
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
			if t.isAction() {
				link := &xhtml.Node{Type: xhtml.ElementNode, Data: "a", DataAtom: atom.A,
					Attr: append(nodeAttrs(actionAttrs(t)), xhtml.Attribute{Key: "title", Val: t.text})}
				link.AppendChild(code)
				code = link
			}
			span.AppendChild(code)
		}
		parent.InsertBefore(span, n)
	}
	parent.RemoveChild(n)
}

// keepSVGAttrs keeps an SVG element's allowed attributes: style filtered,
// id prefixed with prefix, and every other value checked as a CSS value
// (svgValue), since the browser parses a presentation attribute as one.
func keepSVGAttrs(n *xhtml.Node, prefix string) {
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
			a.Val = prefix + a.Val
		case "class", "role", "aria-label", "aria-hidden":
		default:
			v, ok := svgValue(a.Val, prefix)
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
func svgValue(v, prefix string) (string, bool) {
	if cssFetchOrEscape.MatchString(svgLocalRef.ReplaceAllString(v, "")) {
		return "", false
	}
	return svgLocalRef.ReplaceAllString(v, "url(#"+prefix+"$1)"), true
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
