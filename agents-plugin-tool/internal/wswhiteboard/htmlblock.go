package wswhiteboard

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// rawTextHTMLBlock is goldmark's HTML block parser with one repair: an HTML
// block that goldmark ends at the first blank line (CommonMark types 6 and 7,
// opened by a line such as <div ...>) keeps going while a raw-text element
// (script, style, pre, textarea) opened inside it is still unclosed. A
// mockup panel is typically <style>, <div>..</div>, then <script> on the next
// line; the script joins the <div> block, and without this repair the first
// blank line inside it hands the rest of the script to markdown, which
// injects <p> and <br> into the code. The element's content reaches the
// frame as typed, as the browser reads it.
type rawTextHTMLBlock struct {
	parser.BlockParser
}

// rawTextKey holds a rawTextState in the parse context.
var rawTextKey = parser.NewContextKey()

// rawTextState maps each open type 6/7 block to the raw-text tag it is inside
// at the end of its last line, so each line is scanned once.
type rawTextState map[ast.Node]string

var (
	rawTextOpen  = regexp.MustCompile(`(?i)<(script|style|pre|textarea)(?:[\s/>]|$)`)
	rawTextClose = map[string]*regexp.Regexp{}
)

func init() {
	for _, tag := range []string{"script", "style", "pre", "textarea"} {
		rawTextClose[tag] = regexp.MustCompile(`(?i)</` + tag + `(?:[\s>]|$)`)
	}
}

// rawTextOpenAfter returns the raw-text tag still open after line, given the
// tag open before it ("" for none).
func rawTextOpenAfter(line []byte, open string) string {
	s := string(line)
	for {
		if open != "" {
			loc := rawTextClose[open].FindStringIndex(s)
			if loc == nil {
				return open
			}
			s, open = s[loc[1]:], ""
			continue
		}
		m := rawTextOpen.FindStringSubmatchIndex(s)
		if m == nil {
			return ""
		}
		open, s = strings.ToLower(s[m[2]:m[3]]), s[m[1]:]
	}
}

func (b rawTextHTMLBlock) track(pc parser.Context) rawTextState {
	st, _ := pc.Get(rawTextKey).(rawTextState)
	if st == nil {
		st = rawTextState{}
		pc.Set(rawTextKey, st)
	}
	return st
}

// endsAtBlank reports whether node is a block goldmark closes at a blank line.
func endsAtBlank(node ast.Node) bool {
	t := node.(*ast.HTMLBlock).HTMLBlockType
	return t == ast.HTMLBlockType6 || t == ast.HTMLBlockType7
}

func (b rawTextHTMLBlock) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	node, state := b.BlockParser.Open(parent, reader, pc)
	if node != nil && endsAtBlank(node) {
		b.track(pc)[node] = rawTextOpenAfter(line, "")
	}
	return node, state
}

func (b rawTextHTMLBlock) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	if !endsAtBlank(node) {
		return b.BlockParser.Continue(node, reader, pc)
	}
	line, segment := reader.PeekLine()
	st := b.track(pc)
	open := st[node]
	if open != "" && util.IsBlank(line) {
		node.Lines().Append(segment)
		reader.AdvanceToEOL()
		return parser.Continue | parser.NoChildren
	}
	state := b.BlockParser.Continue(node, reader, pc)
	if state&parser.Continue != 0 {
		st[node] = rawTextOpenAfter(line, open)
	} else {
		delete(st, node)
	}
	return state
}

func newRawTextHTMLBlockParser() parser.BlockParser {
	return rawTextHTMLBlock{parser.NewHTMLBlockParser()}
}
