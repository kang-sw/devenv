package wswhiteboard

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Item fields render a small allowlist of inline tags a lead types out of
// HTML habit: <code>, <b>, <strong>, <i>, <em>, <kbd>, and <br>. They are
// not HTML: inlineTagParser reads them as markdown nodes (a code span,
// strong, emphasis, a line break, and kbdNode), so no typed HTML reaches the
// page and no attribute can ride along. A tag carrying any attribute, an
// unclosed tag, a mismatched closing tag, and every other tag stay escaped
// text, as every tag was before. Only a board page's item pipelines
// (newMarkdown) use it: the raw pipeline keeps its raw HTML parser, and
// RenderText's pages set noInlineTags.

// inlineTagOpen matches an allowlisted tag with no attribute at the start of
// a line remainder: name, then an optional "/" (a self-closing <br/>).
var inlineTagOpen = regexp.MustCompile(`(?i)^<(/?)(code|b|strong|i|em|kbd|br)[ \t]*(/?)>`)

// inlineTagParser reads an allowlisted tag at '<'. <code> takes its content
// literally up to </code>, like a backtick span; <br> is a line break; the
// others become markers that inlineTagTransformer pairs once the paragraph
// is parsed, so their content is parsed as inline markdown in between.
type inlineTagParser struct{}

func (inlineTagParser) Trigger() []byte { return []byte{'<'} }

func (inlineTagParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, segment := block.PeekLine()
	m := inlineTagOpen.FindSubmatch(line)
	if m == nil {
		return nil
	}
	closing, name, slash := len(m[1]) > 0, strings.ToLower(string(m[2])), len(m[3]) > 0
	switch {
	case name == "br":
		if closing {
			return nil
		}
		block.Advance(len(m[0]))
		return &inlineBreakNode{}
	case slash:
		return nil // only <br> self-closes
	case name == "code":
		if closing {
			return nil // a stray </code> is text
		}
		return parseInlineCode(block, len(m[0]))
	}
	block.Advance(len(m[0]))
	return &inlineTagNode{name: name, closing: closing, segment: segment.WithStop(segment.Start + len(m[0]))}
}

// inlineCodeClose finds the first </code>, any case.
var inlineCodeClose = regexp.MustCompile(`(?i)</code>`)

// parseInlineCode reads <code> content literally up to the first </code>,
// across the paragraph's lines, into a code span; with no </code> it reads
// nothing and the tag stays text.
func parseInlineCode(block text.Reader, open int) ast.Node {
	l, pos := block.Position()
	block.Advance(open)
	node := ast.NewCodeSpan()
	for {
		line, segment := block.PeekLine()
		if line == nil {
			block.SetPosition(l, pos)
			return nil
		}
		if loc := inlineCodeClose.FindIndex(line); loc != nil {
			if loc[0] > 0 {
				node.AppendChild(node, ast.NewRawTextSegment(segment.WithStop(segment.Start+loc[0])))
			}
			block.Advance(loc[1])
			return node
		}
		node.AppendChild(node, ast.NewRawTextSegment(segment))
		block.AdvanceLine()
	}
}

var (
	kindInlineTag   = ast.NewNodeKind("WhiteboardInlineTag")
	kindInlineBreak = ast.NewNodeKind("WhiteboardInlineBreak")
	kindKbd         = ast.NewNodeKind("WhiteboardKbd")
)

// inlineTagNode is an opening or closing <b>, <strong>, <i>, <em>, or <kbd>
// marker. inlineTagTransformer replaces a matched pair with the node it
// names; an unmatched marker stays and renders as its escaped source text.
type inlineTagNode struct {
	ast.BaseInline
	name    string
	closing bool
	segment text.Segment
}

func (n *inlineTagNode) Kind() ast.NodeKind { return kindInlineTag }

func (n *inlineTagNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Tag": string(n.segment.Value(source))}, nil)
}

// inlineBreakNode is a <br>, rendered as a hard line break renders.
type inlineBreakNode struct{ ast.BaseInline }

func (n *inlineBreakNode) Kind() ast.NodeKind { return kindInlineBreak }

func (n *inlineBreakNode) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

// kbdNode is a matched <kbd> pair, rendered as <kbd> with no attribute.
type kbdNode struct{ ast.BaseInline }

func (n *kbdNode) Kind() ast.NodeKind { return kindKbd }

func (n *kbdNode) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

// inlineTagTransformer pairs the markers among each node's children: a
// closing marker matches the innermost open marker only when that marker has
// its name (any case), and the nodes between them move into the node the
// pair names. A closing marker that matches nothing, and an open marker left
// unclosed, stay as text.
type inlineTagTransformer struct{}

func (inlineTagTransformer) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	var parents []ast.Node
	seen := map[ast.Node]bool{}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == kindInlineTag && !seen[n.Parent()] {
			seen[n.Parent()] = true
			parents = append(parents, n.Parent())
		}
		return ast.WalkContinue, nil
	})
	for _, parent := range parents {
		pairInlineTags(parent)
	}
}

func pairInlineTags(parent ast.Node) {
	var open []*inlineTagNode
	for c := parent.FirstChild(); c != nil; {
		next := c.NextSibling()
		marker, ok := c.(*inlineTagNode)
		switch {
		case !ok:
		case !marker.closing:
			open = append(open, marker)
		case len(open) > 0 && open[len(open)-1].name == marker.name:
			opener := open[len(open)-1]
			open = open[:len(open)-1]
			wrapped := inlineTagTarget(marker.name)
			for n := opener.NextSibling(); n != marker; {
				after := n.NextSibling()
				parent.RemoveChild(parent, n)
				wrapped.AppendChild(wrapped, n)
				n = after
			}
			parent.ReplaceChild(parent, opener, wrapped)
			parent.RemoveChild(parent, marker)
		}
		c = next
	}
}

// inlineTagTarget is the node a matched pair becomes.
func inlineTagTarget(name string) ast.Node {
	switch name {
	case "b", "strong":
		return ast.NewEmphasis(2)
	case "i", "em":
		return ast.NewEmphasis(1)
	}
	return &kbdNode{}
}

// renderInlineTag writes an unmatched marker as its escaped source text.
func renderInlineTag(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.Write(util.EscapeHTML(node.(*inlineTagNode).segment.Value(source)))
	}
	return ast.WalkContinue, nil
}

// renderInlineBreak writes a <br> as goldmark writes a hard line break.
func renderInlineBreak(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<br>\n")
	}
	return ast.WalkContinue, nil
}

func renderKbd(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<kbd>")
	} else {
		_, _ = w.WriteString("</kbd>")
	}
	return ast.WalkContinue, nil
}
