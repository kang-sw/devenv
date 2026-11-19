package wswhiteboard

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// rawTextHTMLBlock keeps type 6/7 HTML blocks open across blank lines while
// elements remain open, so indented mockup contents stay HTML, not Markdown.
type rawTextHTMLBlock struct {
	parser.BlockParser
}

// rawTextKey holds a rawTextState in the parse context.
var rawTextKey = parser.NewContextKey()

// Each open block retains its element depth and lexical state between lines.
type htmlBlockState struct {
	depth   int
	raw     string
	comment bool
}

type rawTextState map[ast.Node]htmlBlockState

var (
	rawTextOpen  = regexp.MustCompile(`(?i)<(script|style|pre|textarea)(?:[\s/>]|$)`)
	rawTextClose = map[string]*regexp.Regexp{}
	htmlTag      = regexp.MustCompile(`(?i)^<(/?)([a-z][a-z0-9-]*)(?:\s+(?:[^>"']|"[^"]*"|'[^']*')*)?\s*/?>`)
	voidTags     = map[string]bool{"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true, "input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true}
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

// scanHTMLBlock ignores comments and raw-text contents when counting tags.
func scanHTMLBlock(line []byte, state htmlBlockState) htmlBlockState {
	s := string(line)
	for len(s) > 0 {
		if state.comment {
			i := strings.Index(s, "-->")
			if i < 0 {
				break
			}
			state.comment, s = false, s[i+3:]
			continue
		}
		if state.raw != "" {
			loc := rawTextClose[state.raw].FindStringIndex(s)
			if loc == nil {
				break
			}
			s = s[loc[0]:]
			state.raw = ""
		}
		i := strings.IndexByte(s, '<')
		if i < 0 {
			break
		}
		s = s[i:]
		if strings.HasPrefix(s, "<!--") {
			state.comment, s = true, s[4:]
			continue
		}
		m := htmlTag.FindStringSubmatch(s)
		if m == nil {
			s = s[1:]
			continue
		}
		tag := strings.ToLower(m[2])
		if m[1] == "/" {
			if !voidTags[tag] && state.depth > 0 {
				state.depth--
			}
		} else if !voidTags[tag] && !strings.HasSuffix(m[0], "/>") {
			state.depth++
			state.raw = rawTextOpenAfter([]byte(m[0]), "")
		}
		s = s[len(m[0]):]
	}
	return state
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
		b.track(pc)[node] = scanHTMLBlock(line, htmlBlockState{})
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
	if (open.depth > 0 || open.raw != "" || open.comment) && util.IsBlank(line) {
		node.Lines().Append(segment)
		reader.AdvanceToEOL()
		return parser.Continue | parser.NoChildren
	}
	state := b.BlockParser.Continue(node, reader, pc)
	if state&parser.Continue != 0 {
		st[node] = scanHTMLBlock(line, open)
	} else {
		delete(st, node)
	}
	return state
}

func newRawTextHTMLBlockParser() parser.BlockParser {
	return rawTextHTMLBlock{parser.NewHTMLBlockParser()}
}
