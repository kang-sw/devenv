package wswhiteboard

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"html/template"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Lead-authored item text renders as markdown: titles and ack-row reasons
// inline only, options, rationale, and recommendations as blocks. goldmark's
// output reaches the page template as trusted HTML, so this file and
// inline_tags.go are the whole escaping boundary on that text: neither the
// block nor the inline pipeline has an HTML parser, so a tag the lead types
// is escaped text unless it is one of inline_tags.go's allowlisted tags on
// a board page, which become markdown nodes with no attribute (html.WithUnsafe
// stays off as the backstop), and every link, autolink, and image destination goes
// through resolveTarget's allow-list. The raw pipeline passes HTML
// through and serves two callers only: topic bodies, written as is, and a
// finished lead reply, which sanitize.go filters afterwards.

// mermaidJS is the vendored mermaid build (assets/mermaid/README.md). Pages
// never inline it: the page writer stores it once per folder under
// MermaidAsset's name and a page that holds a mermaid block loads it from
// there.
//
//go:embed assets/mermaid/mermaid.min.js
var mermaidJS []byte

var (
	mermaidOnce sync.Once
	mermaidName string

	itemRefPrefix = regexp.MustCompile(`^\{\{([0-9]+)\}\}`)
	drivePath     = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
)

// MermaidAsset returns the shared mermaid script's file name, which carries
// a hash of the embedded bytes so a new version never overwrites an old one
// a page still loads, and the bytes to write under it.
func MermaidAsset() (name string, data []byte) {
	mermaidOnce.Do(func() {
		sum := sha256.Sum256(mermaidJS)
		mermaidName = "wb-mermaid-" + hex.EncodeToString(sum[:])[:12] + ".js"
	})
	return mermaidName, mermaidJS
}

// UsesMermaid reports whether a rendered page loads the shared mermaid
// script, so the page writer stores it beside the page.
func UsesMermaid(page string) bool {
	name, _ := MermaidAsset()
	return strings.Contains(page, `<script src="`+name+`">`)
}

// markdown holds a page's goldmark pipelines; all resolve item references
// and link targets against that page.
type markdown struct {
	block, inline goldmark.Markdown
	// raw is block with raw HTML passed through, built on first use (topic
	// bodies and finished lead replies only).
	raw goldmark.Markdown
}

func newMarkdown(p *pageData) *markdown {
	refs := util.Prioritized(itemRefParser{stored: p.Stored}, 100)
	// Inline only: the paragraph is the one block parser, so list markers,
	// headings, fences, and quotes stay literal text, and no paragraph
	// transformer turns a "[x]: y" title into a link definition.
	inlineOptions := []parser.Option{
		parser.WithBlockParsers(util.Prioritized(parser.NewParagraphParser(), 100)),
		parser.WithInlineParsers(inlineParsers(refs, false, !p.noInlineTags)...),
	}
	if !p.noInlineTags {
		inlineOptions = append(inlineOptions, parser.WithASTTransformers(util.Prioritized(inlineTagTransformer{}, 100)))
	}
	inlineParser := parser.NewParser(inlineOptions...)
	inline := goldmark.New(
		goldmark.WithParser(inlineParser),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(util.Prioritized(&mdRenderer{page: p, inline: true}, 100)),
		),
	)
	return &markdown{block: blockMarkdown(p, refs, false), inline: inline}
}

// blockMarkdown is the block pipeline with hard-wrapped lines. Without
// rawHTML it has goldmark's default block parsers minus the HTML block parser,
// which would also let a line starting with a tag swallow the markdown after
// it up to the next blank line, and reads the allowlisted inline tags unless
// the page sets noInlineTags. With rawHTML that parser, the raw HTML inline
// parser, and html.WithUnsafe are on, so typed HTML is written as is.
func blockMarkdown(p *pageData, refs util.PrioritizedValue, rawHTML bool) goldmark.Markdown {
	blocks := []util.PrioritizedValue{
		util.Prioritized(parser.NewSetextHeadingParser(), 100),
		util.Prioritized(parser.NewThematicBreakParser(), 200),
		util.Prioritized(parser.NewListParser(), 300),
		util.Prioritized(parser.NewListItemParser(), 400),
		util.Prioritized(parser.NewCodeBlockParser(), 500),
		util.Prioritized(parser.NewATXHeadingParser(), 600),
		util.Prioritized(parser.NewFencedCodeBlockParser(), 700),
		util.Prioritized(parser.NewBlockquoteParser(), 800),
		util.Prioritized(parser.NewParagraphParser(), 1000),
	}
	options := []renderer.Option{
		html.WithHardWraps(),
		renderer.WithNodeRenderers(util.Prioritized(&mdRenderer{page: p}, 100)),
	}
	inlineTags := !rawHTML && !p.noInlineTags
	parserOptions := []parser.Option{
		parser.WithInlineParsers(inlineParsers(refs, rawHTML, inlineTags)...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	}
	if rawHTML {
		blocks = append(blocks, util.Prioritized(parser.NewHTMLBlockParser(), 900))
		options = append(options, html.WithUnsafe())
	}
	if inlineTags {
		parserOptions = append(parserOptions, parser.WithASTTransformers(util.Prioritized(inlineTagTransformer{}, 100)))
	}
	return goldmark.New(
		goldmark.WithParser(parser.NewParser(append(parserOptions, parser.WithBlockParsers(blocks...))...)),
		goldmark.WithExtensions(
			extension.Table, extension.Strikethrough, extension.TaskList,
			extension.NewFootnote(extension.WithFootnoteIDPrefixFunction(func(ast.Node) []byte {
				return []byte(p.footnotePrefix)
			})),
		),
		goldmark.WithRendererOptions(options...),
	)
}

// inlineParsers is goldmark's default inline set plus refs, with the raw
// HTML parser when rawHTML and the allowlisted inline tags (inline_tags.go)
// when inlineTags.
func inlineParsers(refs util.PrioritizedValue, rawHTML, inlineTags bool) []util.PrioritizedValue {
	list := []util.PrioritizedValue{
		util.Prioritized(parser.NewCodeSpanParser(), 100),
		util.Prioritized(parser.NewLinkParser(), 200),
		util.Prioritized(parser.NewAutoLinkParser(), 300),
		util.Prioritized(parser.NewEmphasisParser(), 500),
		refs,
	}
	if rawHTML {
		list = append(list, util.Prioritized(parser.NewRawHTMLParser(), 400))
	}
	if inlineTags {
		list = append(list, util.Prioritized(inlineTagParser{}, 400))
	}
	return list
}

func (p *pageData) markdown() *markdown {
	if p.md == nil {
		p.md = newMarkdown(p)
	}
	return p.md
}

// rawMarkdown is the page's raw HTML pipeline.
func (p *pageData) rawMarkdown() goldmark.Markdown {
	md := p.markdown()
	if md.raw == nil {
		md.raw = blockMarkdown(p, util.Prioritized(itemRefParser{stored: p.Stored}, 100), true)
	}
	return md.raw
}

// renderDoc renders a topic body: block markdown with raw HTML written as
// is, neither balanced nor filtered, so an inline script still runs. Item
// references resolve in its markdown text, never inside its HTML. marks are
// the body's change marks, written into the HTML as comments (applyMarks).
func (p *pageData) renderDoc(s string, marks []BlockMark) template.HTML {
	if len(marks) == 0 {
		return p.convert(p.rawMarkdown(), s)
	}
	return p.convertWith(p.rawMarkdown(), s, func(doc ast.Node, src []byte) { applyMarks(doc, src, marks) })
}

// Inline renders s as inline markdown: emphasis, code, links, and item
// references; an image shows its alt text.
func (p *pageData) Inline(s string) template.HTML {
	return p.convert(p.markdown().inline, s)
}

// Block renders s as block markdown with hard-wrapped lines. Text that is
// one paragraph renders without the <p> wrapper, so it sits inline beside an
// option letter or a recommendation star.
func (p *pageData) Block(s string) template.HTML {
	return p.convert(p.markdown().block, s)
}

// FirstLine renders the first non-blank line of s as inline markdown, the
// way a settled row's picked-option chip shows a long option.
func (p *pageData) FirstLine(s string) template.HTML {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return p.Inline(line)
		}
	}
	return ""
}

// IsBlock reports whether s holds block markdown: anything but one
// paragraph, or one paragraph that shows an image. An item whose options
// hold any block stacks them as cards.
func (p *pageData) IsBlock(s string) bool {
	source := []byte(s)
	doc := p.markdown().block.Parser().Parse(text.NewReader(source))
	first := doc.FirstChild()
	if first == nil {
		return false
	}
	if first.NextSibling() != nil || first.Kind() != ast.KindParagraph {
		return true
	}
	image := false
	_ = ast.Walk(first, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == ast.KindImage {
			image = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return image
}

// footnoteIDPrefix is the id prefix of the footnotes in one text: a hash of
// that text, so two texts on one page that each define [^1] link to their
// own note. Identical texts share it, and their links meet the first copy.
func footnoteIDPrefix(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprintf("%s%08x-", footnoteIDPrefixBase, h.Sum32())
}

// convert renders s through md, one of the page's pipelines.
func (p *pageData) convert(md goldmark.Markdown, s string) template.HTML {
	return p.convertWith(md, s, nil)
}

// convertWith is convert with a step between parsing and rendering: prepare
// may edit the parsed document.
func (p *pageData) convertWith(md goldmark.Markdown, s string, prepare func(doc ast.Node, src []byte)) template.HTML {
	// Rendering is sequential per page; the footnote renderer reads the prefix
	// through its closure (blockMarkdown).
	p.footnotePrefix = footnoteIDPrefix(s)
	var buf bytes.Buffer
	var err error
	if prepare == nil {
		err = md.Convert([]byte(s), &buf)
	} else {
		src := []byte(s)
		doc := md.Parser().Parse(text.NewReader(src))
		prepare(doc, src)
		err = md.Renderer().Render(&buf, src, doc)
	}
	if err != nil {
		// Convert fails only on a writer error, which a bytes.Buffer never
		// returns; escaped text is the safe fallback all the same.
		return template.HTML(template.HTMLEscapeString(s))
	}
	return template.HTML(strings.TrimRight(buf.String(), "\n"))
}

// target is a resolved link or image destination: an href (an image's src),
// a file target's path text, or a file target's action link.
type target struct {
	// href is the anchor's href or the image's src; empty for path text and
	// for an action link, which carries its own.
	href string
	// text is a file target's path text (FileText); empty otherwise.
	text string
	// action is a file target's link actions on a page with FileActions;
	// zero when the target has none.
	action FileAction
}

// isAction reports whether the target renders as an action link.
func (t target) isAction() bool { return t.action.Href != "" || t.action.Abs != "" }

// resolveTarget maps a link or image destination to the target it renders
// with, or reports false when it must render as text. Links accept relative
// paths, file: URLs, http(s) URLs, and fragment-only in-page anchors; images
// accept relative paths and file: URLs only, so a page never fetches from the
// network. A relative path resolves against the session root and a bare
// absolute path is a file path; both, and every file: URL, go through the
// page's FileLink, which links them from wherever the page is written. On a
// page with FileText a file target instead comes back as path text, with an
// empty href, and on a page with FileActions also as an action link. An
// image file target on a page with ImageSrc comes back as the src ImageSrc
// returns, unless it returns empty (see fileTarget).
func (p *pageData) resolveTarget(dest string, image bool) (target, bool) {
	dest = strings.TrimSpace(dest)
	switch {
	case dest == "":
		return target{}, false
	case strings.HasPrefix(dest, "#"):
		return target{href: dest}, !image
	case drivePath.MatchString(dest):
		return p.fileTarget(dest, "", image), true
	}
	u, err := url.Parse(dest)
	if err != nil {
		return target{}, false
	}
	suffix := ""
	if u.RawQuery != "" {
		suffix += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		suffix += "#" + u.EscapedFragment()
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return target{href: dest}, !image
	case "file":
		if u.Host != "" && u.Host != "localhost" {
			return target{}, false
		}
		path := u.Path
		if path == "" {
			path = u.Opaque
		}
		if len(path) > 2 && path[0] == '/' && drivePath.MatchString(path[1:]) {
			path = path[1:] // file:///C:/x names the drive path C:/x
		}
		if path == "" {
			return target{}, false
		}
		return p.fileTarget(path, suffix, image), true
	case "":
		// "//host/x" is a network reference, not a file path.
		if u.Host != "" || u.Path == "" {
			return target{}, false
		}
		return p.fileTarget(u.Path, suffix, image), true
	}
	return target{}, false
}

// fileTarget resolves a file path: for an image in a framed topic body, as
// the src DocImageSrc returns, when that is non-empty; for an image on a page with
// ImageSrc, as the src ImageSrc returns (the destination's own query and fragment are
// dropped), when that is non-empty; else as path text on a page with
// FileText, with the FileActions action when the page has them, else as the
// href fileHref builds.
func (p *pageData) fileTarget(path, suffix string, image bool) target {
	if image && p.framing && p.DocImageSrc != nil {
		if src := p.DocImageSrc(path); src != "" {
			return target{href: src}
		}
	}
	if image && p.ImageSrc != nil {
		if src := p.ImageSrc(path); src != "" {
			return target{href: src}
		}
	}
	if p.FileText != nil {
		t := target{text: p.FileText(path)}
		if t.text == "" {
			t.text = filepath.ToSlash(path)
		}
		if p.linkActions() {
			t.action = p.FileActions(path)
			if !daemonPath(t.action.Href) {
				t.action.Href = ""
			}
		}
		return t
	}
	return target{href: p.fileHref(path, suffix)}
}

// linkActions reports whether the page renders the served page's action
// links: it has FileActions and FileText.
func (p *pageData) linkActions() bool { return p.FileActions != nil && p.FileText != nil }

// daemonPath reports whether href is a scheme-less absolute path on the
// serving host, the only href an action link takes from FileActions: no
// scheme, no host ("//host/x" or a backslash a browser reads as one), no
// control character.
func daemonPath(href string) bool {
	if len(href) < 2 || href[0] != '/' || href[1] == '/' || href[1] == '\\' {
		return false
	}
	for i := 0; i < len(href); i++ {
		if href[i] < 0x21 || href[i] == 0x7f || href[i] == '\\' {
			return false
		}
	}
	return true
}

// pathURLEscaper keeps a decoded "#" or "?" in a file name from starting a
// fragment or query; a file: URL from FileLink already has them escaped.
var pathURLEscaper = strings.NewReplacer("#", "%23", "?", "%3F")

// fileHref links a file path through FileLink and checks the href it gets
// back, not only the destination it started from: a relative result whose
// first segment holds a colon ("javascript:alert(1)", for a file of that
// name beside the page, or a decoded "javascript%3A...") would read as a
// scheme, so it is anchored with "./". Only a scheme-less href or a file:
// URL leaves here as is.
func (p *pageData) fileHref(path, suffix string) string {
	href := filepath.ToSlash(path)
	if p.FileLink != nil {
		href = p.FileLink(path)
	}
	href = pathURLEscaper.Replace(href)
	if u, err := url.Parse(href); err != nil || (u.Scheme != "" && !strings.EqualFold(u.Scheme, "file")) {
		href = "./" + href
	}
	return href + suffix
}

// itemRefParser turns {{nn}} into an item reference node. Running as a
// goldmark inline parser keeps a reference inside a code span or code block
// literal. An ID the store does not hold stays verbatim text.
type itemRefParser struct {
	stored map[int]bool
}

func (itemRefParser) Trigger() []byte { return []byte{'{'} }

func (r itemRefParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	m := itemRefPrefix.FindSubmatch(line)
	if m == nil {
		return nil
	}
	id, err := strconv.Atoi(string(m[1]))
	if err != nil || !r.stored[id] {
		return nil
	}
	block.Advance(len(m[0]))
	return &itemRefNode{ID: id}
}

var kindItemRef = ast.NewNodeKind("WhiteboardItemRef")

type itemRefNode struct {
	ast.BaseInline
	ID int
}

func (n *itemRefNode) Kind() ast.NodeKind { return kindItemRef }

func (n *itemRefNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"ID": strconv.Itoa(n.ID)}, nil)
}

// mdRenderer overrides goldmark's HTML renderer for every node that carries
// a destination, for mermaid fences, for item references, and for paragraphs.
type mdRenderer struct {
	page   *pageData
	inline bool
}

func (r *mdRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindParagraph, r.paragraph)
	reg.Register(ast.KindLink, r.link)
	reg.Register(ast.KindAutoLink, r.autoLink)
	reg.Register(ast.KindImage, r.image)
	reg.Register(ast.KindFencedCodeBlock, r.fencedCode)
	reg.Register(kindItemRef, r.itemRef)
	reg.Register(kindChange, renderChange)
	reg.Register(kindInlineTag, renderInlineTag)
	reg.Register(kindInlineBreak, renderInlineBreak)
	reg.Register(kindKbd, renderKbd)
}

// paragraph drops the <p> wrapper around a document's only paragraph, and
// every wrapper in inline mode, where paragraphs join with a space.
func (r *mdRenderer) paragraph(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if r.inline {
		if entering && n.PreviousSibling() != nil {
			_ = w.WriteByte(' ')
		}
		return ast.WalkContinue, nil
	}
	if n.Parent().Kind() == ast.KindDocument && n.PreviousSibling() == nil && n.NextSibling() == nil {
		return ast.WalkContinue, nil
	}
	if entering {
		_, _ = w.WriteString("<p>")
	} else {
		_, _ = w.WriteString("</p>\n")
	}
	return ast.WalkContinue, nil
}

// link renders an allowed destination as an anchor and any other as its
// text alone.
func (r *mdRenderer) link(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Link)
	t, ok := r.page.resolveTarget(destination(n.Destination), false)
	if !ok {
		return ast.WalkContinue, nil
	}
	if t.isAction() {
		// A file link on a served page with link actions: the label is the
		// action link.
		if entering {
			r.page.writeActionOpen(w, t, n.Title)
		} else {
			_, _ = w.WriteString("</a>")
		}
		return ast.WalkContinue, nil
	}
	if t.text != "" {
		// A file link on a served page: the label, then the path as text
		// unless the label already is that path.
		if !entering && plainText(source, n) != t.text {
			writePathText(w, t.text)
		}
		return ast.WalkContinue, nil
	}
	if entering {
		r.writeAnchorOpen(w, t.href, n.Title)
	} else {
		_, _ = w.WriteString("</a>")
	}
	return ast.WalkContinue, nil
}

// autoLink is a separate node kind from link, so "<javascript:...>" would
// bypass a link-only override.
func (r *mdRenderer) autoLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.AutoLink)
	label := util.EscapeHTML(n.Label(source))
	t, ok := target{}, false
	if n.AutoLinkType == ast.AutoLinkURL {
		t, ok = r.page.resolveTarget(string(n.URL(source)), false)
	}
	switch {
	case !ok:
		_, _ = w.Write(label)
	case t.isAction():
		r.page.writeActionOpen(w, t, nil)
		_, _ = w.Write(label)
		_, _ = w.WriteString("</a>")
	case t.text != "":
		writePathText(w, t.text)
	default:
		r.writeAnchorOpen(w, t.href, nil)
		_, _ = w.Write(label)
		_, _ = w.WriteString("</a>")
	}
	return ast.WalkSkipChildren, nil
}

// destination resolves the backslash escapes and entity references goldmark
// leaves in a link or image destination or title for its renderer, so
// "java&#x73;cript:" is classified as the javascript: URL a browser would
// read it as.
func destination(raw []byte) string {
	v := util.UnescapePunctuations(raw)
	v = util.ResolveNumericReferences(v)
	return string(util.ResolveEntityNames(v))
}

// writeAnchorOpen opens an anchor to a resolved link's href (linkAttrs).
func (r *mdRenderer) writeAnchorOpen(w util.BufWriter, href string, title []byte) {
	attrs := r.page.linkAttrs(href)
	_, _ = w.WriteString(`<a`)
	writeAttrs(w, attrs[:1])
	r.page.writeTrustRef(w, attrs, false)
	if len(title) > 0 {
		writeAttrs(w, [][2]string{{"title", destination(title)}})
	}
	writeAttrs(w, attrs[1:])
	_ = w.WriteByte('>')
}

// linkAttrs are the attributes a resolved link that is not an action link
// opens with besides its title: the URL-escaped href, and for http(s) the
// new-tab pair and, on a page with link actions, the url marker
// (data-wl="url") the link menu keys on. The markdown link writer and the
// reply sanitizer both take them from here, so an attribute added here
// reaches every link either one resolves.
func (p *pageData) linkAttrs(href string) [][2]string {
	attrs := [][2]string{{"href", string(util.URLEscape([]byte(href), false))}}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		// Leaving the board in place would drop an answer in progress.
		attrs = append(attrs, [2]string{"target", "_blank"}, [2]string{"rel", "noopener noreferrer"})
		if p.linkActions() {
			attrs = append(attrs, [2]string{"data-wl", "url"})
		}
	}
	return attrs
}

// writeActionOpen opens a file target's action link (View.FileActions),
// titled with the link's own title or else the target's path text.
func (p *pageData) writeActionOpen(w util.BufWriter, t target, title []byte) {
	attrs := actionAttrs(t)
	_, _ = w.WriteString(`<a`)
	writeAttrs(w, attrs)
	p.writeTrustRef(w, attrs, false)
	if len(title) > 0 {
		writeAttrs(w, [][2]string{{"title", destination(title)}})
	} else {
		writeAttrs(w, [][2]string{{"title", t.text}})
	}
	_ = w.WriteByte('>')
}

// actionAttrs are the attributes a file target's action link opens with
// besides its title: an href to the file view when the action has one, else
// a tabindex for an anchor the link menu offers to copy only, then the file
// marker (data-wl="file") with the root-relative and absolute paths. The
// markdown writer and the reply sanitizer both take them from here.
func actionAttrs(t target) [][2]string {
	var attrs [][2]string
	if t.action.Href != "" {
		attrs = append(attrs, [2]string{"href", string(util.URLEscape([]byte(t.action.Href), false))})
	} else {
		attrs = append(attrs, [2]string{"tabindex", "0"})
	}
	attrs = append(attrs, [2]string{"data-wl", "file"})
	if t.action.Rel != "" {
		attrs = append(attrs, [2]string{"data-rel", t.action.Rel})
	}
	if t.action.Abs != "" {
		attrs = append(attrs, [2]string{"data-abs", t.action.Abs})
	}
	return attrs
}

// writeAttrs writes each attribute with its value HTML-escaped.
func writeAttrs(w util.BufWriter, attrs [][2]string) {
	for _, a := range attrs {
		_, _ = w.WriteString(" " + a[0] + `="`)
		_, _ = w.Write(util.EscapeHTML([]byte(a[1])))
		_ = w.WriteByte('"')
	}
}

// writeTrustRef marks a link or image the markdown renderer resolved while a
// finished reply renders (pageData.trusted): the mark is an index into the
// page's own record of the attributes it opened with, so the sanitizer takes
// them from that record, never from the markup, and a forged or misplaced
// mark can only yield attributes resolveTarget already produced, of the same
// kind.
func (p *pageData) writeTrustRef(w util.BufWriter, attrs [][2]string, image bool) {
	if p.trusted == nil {
		return
	}
	*p.trusted = append(*p.trusted, trustedTarget{attrs: attrs, image: image})
	_, _ = w.WriteString(` ` + trustAttr + `="` + strconv.Itoa(len(*p.trusted)-1) + `"`)
}

// image renders an allowed destination as <img> and any other, or any image
// in inline mode, as its alt text.
func (r *mdRenderer) image(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Image)
	t, ok := target{}, false
	if !r.inline {
		t, ok = r.page.resolveTarget(destination(n.Destination), true)
	}
	if t.text != "" {
		// A board image on a served page: its alt text and its path, never
		// a file: URL or a path the browser would resolve against the server;
		// with link actions the path is an action link.
		_, _ = w.WriteString(`<span class="img-text">`)
		writeAltText(w, source, n)
		if t.isAction() {
			_ = w.WriteByte(' ')
			r.page.writeActionOpen(w, t, nil)
			writePathCode(w, t.text)
			_, _ = w.WriteString(`</a>`)
		} else {
			writePathText(w, t.text)
		}
		_, _ = w.WriteString(`</span>`)
		return ast.WalkSkipChildren, nil
	}
	if !ok {
		writeAltText(w, source, n)
		return ast.WalkSkipChildren, nil
	}
	escaped := string(util.URLEscape([]byte(t.href), false))
	_, _ = w.WriteString(`<img src="`)
	_, _ = w.Write(util.EscapeHTML([]byte(escaped)))
	_ = w.WriteByte('"')
	r.page.writeTrustRef(w, [][2]string{{"src", escaped}}, true)
	_, _ = w.WriteString(` alt="`)
	writeAltText(w, source, n)
	_ = w.WriteByte('"')
	if len(n.Title) > 0 {
		_, _ = w.WriteString(` title="`)
		_, _ = w.Write(util.EscapeHTML([]byte(destination(n.Title))))
		_ = w.WriteByte('"')
	}
	_ = w.WriteByte('>')
	return ast.WalkSkipChildren, nil
}

// writePathText writes a file path a served page shows instead of a link.
func writePathText(w util.BufWriter, path string) {
	_ = w.WriteByte(' ')
	writePathCode(w, path)
}

// writePathCode writes a file path as a path code span.
func writePathCode(w util.BufWriter, path string) {
	_, _ = w.WriteString(`<code class="fpath">`)
	_, _ = w.Write(util.EscapeHTML([]byte(path)))
	_, _ = w.WriteString(`</code>`)
}

// plainText is the unescaped text under n.
func plainText(source []byte, n ast.Node) string {
	var sb strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.Text:
			sb.Write(t.Segment.Value(source))
		case *ast.String:
			sb.Write(t.Value)
		default:
			sb.WriteString(plainText(source, c))
		}
	}
	return sb.String()
}

// writeAltText writes the escaped plain text under n.
func writeAltText(w util.BufWriter, source []byte, n ast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.Text:
			html.DefaultWriter.Write(w, t.Segment.Value(source))
			if t.SoftLineBreak() || t.HardLineBreak() {
				_ = w.WriteByte(' ')
			}
		case *ast.String:
			_, _ = w.Write(util.EscapeHTML(t.Value))
		case *itemRefNode:
			_, _ = w.WriteString(FormatID(t.ID))
		default:
			writeAltText(w, source, c)
		}
	}
}

// fencedCode renders a mermaid block as <pre class="mermaid"> holding its
// source, which the shared mermaid script draws and a host that cannot load
// that script still shows as readable text; any other fence is a code block.
func (r *mdRenderer) fencedCode(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.FencedCodeBlock)
	lang := string(n.Language(source))
	mermaid := strings.EqualFold(lang, "mermaid") && !r.page.noMermaid
	if mermaid {
		r.page.Mermaid = true
		_, _ = w.WriteString(`<pre class="mermaid">`)
	} else {
		_, _ = w.WriteString("<pre><code")
		if lang != "" {
			_, _ = w.WriteString(` class="language-`)
			_, _ = w.Write(util.EscapeHTML([]byte(lang)))
			_ = w.WriteByte('"')
		}
		_ = w.WriteByte('>')
	}
	for i := 0; i < n.Lines().Len(); i++ {
		segment := n.Lines().At(i)
		_, _ = w.Write(util.EscapeHTML(segment.Value(source)))
	}
	if mermaid {
		_, _ = w.WriteString("</pre>\n")
	} else {
		_, _ = w.WriteString("</code></pre>\n")
	}
	return ast.WalkSkipChildren, nil
}

// itemRef renders a reference to an item on the page as an in-page anchor
// and any other stored item as plain (nn); inside a link it is plain text,
// since anchors do not nest.
func (r *mdRenderer) itemRef(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*itemRefNode)
	label := FormatID(n.ID)
	inLink := false
	for a := n.Parent(); a != nil; a = a.Parent() {
		if a.Kind() == ast.KindLink {
			inLink = true
			break
		}
	}
	if r.page.onPage[n.ID] && !inLink {
		_, _ = w.WriteString(`<a href="#item-` + strconv.Itoa(n.ID) + `">` + label + `</a>`)
	} else {
		_, _ = w.WriteString(label)
	}
	return ast.WalkSkipChildren, nil
}
