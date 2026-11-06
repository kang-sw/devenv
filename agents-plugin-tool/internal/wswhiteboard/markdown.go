package wswhiteboard

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
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
// output reaches the page template as trusted HTML, so this file is the whole
// escaping boundary on that text: neither pipeline has an HTML parser, so a
// tag the lead types is escaped text (html.WithUnsafe stays off as the
// backstop), and every link, autolink, and image destination goes through
// resolveTarget's allow-list. The Now slot stays the only HTML channel.

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

// markdown holds a page's two goldmark pipelines; both resolve item
// references and link targets against that page.
type markdown struct {
	block, inline goldmark.Markdown
}

func newMarkdown(p *pageData) *markdown {
	refs := util.Prioritized(itemRefParser{stored: p.Stored}, 100)
	// goldmark's default block parsers without the HTML block parser, which
	// would also let a line starting with a tag swallow the markdown after it
	// up to the next blank line.
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
	block := goldmark.New(
		goldmark.WithParser(parser.NewParser(
			parser.WithBlockParsers(blocks...),
			parser.WithInlineParsers(inlineParsers(refs)...),
			parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
		)),
		goldmark.WithExtensions(extension.Table, extension.Strikethrough),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			renderer.WithNodeRenderers(util.Prioritized(&mdRenderer{page: p}, 100)),
		),
	)
	// Inline only: the paragraph is the one block parser, so list markers,
	// headings, fences, and quotes stay literal text, and no paragraph
	// transformer turns a "[x]: y" title into a link definition.
	inlineParser := parser.NewParser(
		parser.WithBlockParsers(util.Prioritized(parser.NewParagraphParser(), 100)),
		parser.WithInlineParsers(inlineParsers(refs)...),
	)
	inline := goldmark.New(
		goldmark.WithParser(inlineParser),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(util.Prioritized(&mdRenderer{page: p, inline: true}, 100)),
		),
	)
	return &markdown{block: block, inline: inline}
}

// inlineParsers is goldmark's default inline set without the raw HTML
// parser, plus refs.
func inlineParsers(refs util.PrioritizedValue) []util.PrioritizedValue {
	return []util.PrioritizedValue{
		util.Prioritized(parser.NewCodeSpanParser(), 100),
		util.Prioritized(parser.NewLinkParser(), 200),
		util.Prioritized(parser.NewAutoLinkParser(), 300),
		util.Prioritized(parser.NewEmphasisParser(), 500),
		refs,
	}
}

func (p *pageData) markdown() *markdown {
	if p.md == nil {
		p.md = newMarkdown(p)
	}
	return p.md
}

// Inline renders s as inline markdown: emphasis, code, links, and item
// references; an image shows its alt text.
func (p *pageData) Inline(s string) template.HTML {
	return convert(p.markdown().inline, s)
}

// Block renders s as block markdown with hard-wrapped lines. Text that is
// one paragraph renders without the <p> wrapper, so it sits inline beside an
// option letter or a recommendation star.
func (p *pageData) Block(s string) template.HTML {
	return convert(p.markdown().block, s)
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

func convert(md goldmark.Markdown, s string) template.HTML {
	var buf bytes.Buffer
	if err := md.Convert([]byte(s), &buf); err != nil {
		// Convert fails only on a writer error, which a bytes.Buffer never
		// returns; escaped text is the safe fallback all the same.
		return template.HTML(template.HTMLEscapeString(s))
	}
	return template.HTML(strings.TrimRight(buf.String(), "\n"))
}

// resolveTarget maps a link or image destination to the href it renders
// with, or reports false when it must render as text. Links accept relative
// paths, file: URLs, http(s) URLs, and fragment-only in-page anchors; images
// accept relative paths and file: URLs only, so a page never fetches from the
// network. A relative path resolves against the session root and a bare
// absolute path is a file path; both, and every file: URL, go through the
// page's FileLink, which links them from wherever the page is written.
func (p *pageData) resolveTarget(dest string, image bool) (string, bool) {
	dest = strings.TrimSpace(dest)
	switch {
	case dest == "":
		return "", false
	case strings.HasPrefix(dest, "#"):
		return dest, !image
	case drivePath.MatchString(dest):
		return p.fileHref(dest, ""), true
	}
	u, err := url.Parse(dest)
	if err != nil {
		return "", false
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
		return dest, !image
	case "file":
		if u.Host != "" && u.Host != "localhost" {
			return "", false
		}
		path := u.Path
		if path == "" {
			path = u.Opaque
		}
		if len(path) > 2 && path[0] == '/' && drivePath.MatchString(path[1:]) {
			path = path[1:] // file:///C:/x names the drive path C:/x
		}
		if path == "" {
			return "", false
		}
		return p.fileHref(path, suffix), true
	case "":
		// "//host/x" is a network reference, not a file path.
		if u.Host != "" || u.Path == "" {
			return "", false
		}
		return p.fileHref(u.Path, suffix), true
	}
	return "", false
}

func (p *pageData) fileHref(path, suffix string) string {
	if p.FileLink != nil {
		return p.FileLink(path) + suffix
	}
	return filepath.ToSlash(path) + suffix
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
func (r *mdRenderer) link(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Link)
	href, ok := r.page.resolveTarget(destination(n.Destination), false)
	if !ok {
		return ast.WalkContinue, nil
	}
	if entering {
		writeAnchorOpen(w, href, n.Title)
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
	href, ok := "", false
	if n.AutoLinkType == ast.AutoLinkURL {
		href, ok = r.page.resolveTarget(string(n.URL(source)), false)
	}
	if !ok {
		_, _ = w.Write(label)
		return ast.WalkSkipChildren, nil
	}
	writeAnchorOpen(w, href, nil)
	_, _ = w.Write(label)
	_, _ = w.WriteString("</a>")
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

func writeAnchorOpen(w util.BufWriter, href string, title []byte) {
	_, _ = w.WriteString(`<a href="`)
	_, _ = w.Write(util.EscapeHTML(util.URLEscape([]byte(href), false)))
	_ = w.WriteByte('"')
	if len(title) > 0 {
		_, _ = w.WriteString(` title="`)
		_, _ = w.Write(util.EscapeHTML([]byte(destination(title))))
		_ = w.WriteByte('"')
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		// Leaving the board in place would drop an answer in progress.
		_, _ = w.WriteString(` target="_blank" rel="noopener noreferrer"`)
	}
	_ = w.WriteByte('>')
}

// image renders an allowed destination as <img> and any other, or any image
// in inline mode, as its alt text.
func (r *mdRenderer) image(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Image)
	src, ok := "", false
	if !r.inline {
		src, ok = r.page.resolveTarget(destination(n.Destination), true)
	}
	if !ok {
		writeAltText(w, source, n)
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString(`<img src="`)
	_, _ = w.Write(util.EscapeHTML(util.URLEscape([]byte(src), false)))
	_, _ = w.WriteString(`" alt="`)
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
	if strings.EqualFold(lang, "mermaid") {
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
	if strings.EqualFold(lang, "mermaid") {
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
