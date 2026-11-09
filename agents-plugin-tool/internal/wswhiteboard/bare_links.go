package wswhiteboard

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Every markdown pipeline links a bare http(s) URL and a bare file path in
// markdown text. They are inline parsers, so code spans, code blocks, and
// raw HTML blocks keep them literal, and they yield the nodes an author's
// own link would (an autolink for a URL, a link for a path), so the
// renderer sends them through resolveTarget and writes them exactly as it
// writes a written link to the same destination. One exception: a code span
// whose whole content is one bare path links as that path (codePathParser).

var (
	// bareURL is goldmark's linkify URL pattern cut to http(s), with a host
	// that needs no ".tld", so a loopback or LAN address
	// (http://127.0.0.1:29417/s/x) links too, and with no quote, angle
	// bracket, square bracket, brace, or backtick a URL may end in, so
	// prose around a URL stays prose.
	bareURL = regexp.MustCompile(`^https?://[-a-zA-Z0-9@:%._+~#=]{1,256}(?:[/#?][-a-zA-Z0-9@:%_+.~#$!?&/=();,]*)?`)
	// noMatch turns off linkify's www. and email autolinks.
	noMatch = regexp.MustCompile(`[^\x00-\x{10FFFF}]`)

	// barePath is a whole path run (pathRunLength) that links: an absolute
	// path of two or more segments, a drive path of two or more segments,
	// or a relative path holding a "/" whose last segment has a file
	// extension; each with an optional ":line" or ":line:col". Group 1 is
	// the path, the link's destination.
	barePath = regexp.MustCompile(`^(` +
		`/` + pathSegment + `(?:/` + pathSegment + `)+` +
		`|[A-Za-z]:[\\/]` + pathSegment + `(?:[\\/]` + pathSegment + `)+` +
		`|` + pathSegment + `(?:/` + pathSegment + `)*/[\p{L}\p{N}_.+@%-]*\.[A-Za-z][A-Za-z0-9]*` +
		`)(?::[0-9]+(?::[0-9]+)?)?$`)

	// rawSkipOpen and rawSkipClose match the raw inline tags whose content
	// takes no generated link: an anchor, since anchors do not nest, and
	// <code>, as a code span does.
	rawSkipOpen  = regexp.MustCompile(`(?i)^<(a|code)[\s/>]`)
	rawSkipClose = regexp.MustCompile(`(?i)^</(a|code)\s*>`)
)

// pathSegment holds no "~", so a home path ("~/x/a.md") never links as
// the relative or absolute path it would otherwise contain.
const pathSegment = `[\p{L}\p{N}_.+@%-]+`

// barePathAttr marks a link the path parser generated.
var barePathAttr = []byte("wbBarePath")

// bareLinkParsers are the URL and path parsers, after every parser an author
// writes a link or tag with.
func bareLinkParsers() []util.PrioritizedValue {
	url := extension.NewLinkifyParser(
		extension.WithLinkifyAllowedProtocols([]string{"http:", "https:"}),
		extension.WithLinkifyURLRegexp(bareURL),
		extension.WithLinkifyWWWRegexp(noMatch),
		extension.WithLinkifyEmailRegexp(noMatch),
	)
	return []util.PrioritizedValue{
		util.Prioritized(outsideRawTags{url}, 600),
		util.Prioritized(outsideRawTags{barePathParser{}}, 700),
	}
}

// outsideRawTags runs an inline parser except inside a raw <a> or <code>
// (rawSkipOpen) the paragraph has opened and not yet closed.
type outsideRawTags struct{ parser.InlineParser }

func (p outsideRawTags) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	if inRawTag(parent, block.Source()) {
		return nil
	}
	return p.InlineParser.Parse(parent, block, pc)
}

// inRawTag reports whether the nearest raw <a> or <code> tag before the
// current position among parent's children opens one. Inline parsing keeps
// a paragraph's children flat until it closes, so the siblings are enough.
func inRawTag(parent ast.Node, source []byte) bool {
	for c := parent.LastChild(); c != nil; c = c.PreviousSibling() {
		raw, ok := c.(*ast.RawHTML)
		if !ok {
			continue
		}
		var tag []byte
		for i := 0; i < raw.Segments.Len(); i++ {
			s := raw.Segments.At(i)
			tag = append(tag, s.Value(source)...)
		}
		if rawSkipClose.Match(tag) {
			return false
		}
		if rawSkipOpen.Match(tag) {
			return true
		}
	}
	return false
}

// barePathParser links a bare file path (barePath). It starts where
// goldmark's linkify starts, after a space, "(", or "*", or at a line head
// (also the point after another inline node), where goldmark hands a
// leading "/", "_", or "~" to that character's parsers instead. A path
// glued to a word ("and/or", the "/a.go" of "x/a.go") is never one.
type barePathParser struct{}

func (barePathParser) Trigger() []byte { return []byte{' ', '(', '*', '_', '~', '/'} }

func (barePathParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	if pc.IsInLinkLabel() {
		return nil
	}
	line, segment := block.PeekLine()
	lead := 0
	if c := line[0]; util.IsSpace(c) || c == '(' || c == '*' {
		lead = 1
	} else if isPathRune(block.PrecendingCharacter()) {
		return nil
	}
	rest := line[lead:]
	n := pathRunLength(rest)
	// Sentence punctuation after a path is not part of it; a ":" is kept
	// only before a line number, so a trailing one always goes.
	for n > 0 && (rest[n-1] == '.' || rest[n-1] == ':') {
		n--
	}
	m := barePath.FindSubmatchIndex(rest[:n])
	if m == nil {
		return nil
	}
	if lead != 0 {
		ast.MergeOrAppendTextSegment(parent, segment.WithStop(segment.Start+lead))
	}
	start := segment.Start + lead
	block.Advance(lead + n)
	link := ast.NewLink()
	link.Destination = append([]byte(nil), rest[m[2]:m[3]]...)
	link.AppendChild(link, ast.NewTextSegment(text.NewSegment(start, start+n)))
	link.SetAttribute(barePathAttr, true)
	return link
}

// codePathParser is the code span parser, except that a code span whose
// whole content, trimmed, is one bare path (barePath) comes back inside the
// link barePathParser would generate for that path, so the renderer resolves
// and writes it as it does the bare path, with the code span as its label;
// where the bare path would stay text, the span stays a plain code span. A
// span with anything beside the path, a URL, and a span inside a link label
// or a raw <a> or <code> stay literal.
type codePathParser struct{ parser.InlineParser }

func (p codePathParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	node := p.InlineParser.Parse(parent, block, pc)
	span, ok := node.(*ast.CodeSpan)
	if !ok || pc.IsInLinkLabel() || inRawTag(parent, block.Source()) {
		return node
	}
	content := strings.TrimSpace(plainText(block.Source(), span))
	m := barePath.FindStringSubmatchIndex(content)
	if m == nil {
		return node
	}
	link := ast.NewLink()
	link.Destination = []byte(content[m[2]:m[3]])
	link.AppendChild(link, span)
	link.SetAttribute(barePathAttr, true)
	return link
}

// isBarePath reports whether n is a link the path parser or codePathParser
// generated.
func isBarePath(n ast.Node) bool {
	_, ok := n.Attribute(barePathAttr)
	return ok
}

// pathRunLength is the byte length of the run of path characters s starts
// with.
func pathRunLength(s []byte) int {
	n := 0
	for n < len(s) {
		r, size := utf8.DecodeRune(s[n:])
		if !isPathRune(r) {
			break
		}
		n += size
	}
	return n
}

// isPathRune reports whether r may sit in a path run: a letter, a digit, a
// segment character, a separator, or the colon of a drive or a line suffix.
func isPathRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(`_.~+@%-/\:`, r)
}
