package wswhiteboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"golang.org/x/net/html"
)

// panelDiff is a render-only projection of the latest immutable version pair.
// The shared block matcher remains the authority for both topic and panel marks.
type panelDiff struct {
	Added, Modified, Removed int
	old                      string
	edits                    []blockEdit
}

func diffPanel(old, cur string) *panelDiff {
	keys := func(body string) []string {
		src := []byte(body)
		units := bodyUnits(docParser.Parse(text.NewReader(src)), src)
		out := make([]string, len(units))
		for i, u := range units {
			out[i] = u.key
		}
		return out
	}
	d := &panelDiff{old: old, edits: pairBlocks(keys(old), keys(cur))}
	if len(d.edits) == 0 {
		return nil
	}
	for _, e := range d.edits {
		switch e.Kind {
		case MarkNew:
			d.Added++
		case MarkMod:
			d.Modified++
		case MarkDel:
			d.Removed += e.oldTo - e.oldFrom
		}
	}
	return d
}

type wordEdit struct {
	Kind string `json:"k"`
	Text string `json:"t"`
}

var diffWordPattern = regexp.MustCompile(`[\p{L}\p{N}_]+|\s+|[^\p{L}\p{N}_\s]`)

// diffWords keeps whitespace and punctuation, and bounds the quadratic LCS.
// A very large block degrades to one replacement rather than blocking render.
func diffWords(old, cur string) []wordEdit {
	a, b := diffWordPattern.FindAllString(old, -1), diffWordPattern.FindAllString(cur, -1)
	var out []wordEdit
	var run strings.Builder
	kind := ""
	flush := func() {
		if run.Len() > 0 {
			out = append(out, wordEdit{kind, run.String()})
			run.Reset()
		}
	}
	add := func(nextKind, value string) {
		if value == "" {
			return
		}
		if nextKind != kind {
			flush()
			kind = nextKind
		}
		run.WriteString(value)
	}
	if len(a)*len(b) > maxDiffCells {
		add(MarkDel, old)
		add(MarkNew, cur)
		flush()
		return out
	}
	lcs := make([][]int32, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	for i, j := 0, 0; i < len(a) || j < len(b); {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			add("equal", a[i])
			i, j = i+1, j+1
		case i < len(a) && (j == len(b) || lcs[i+1][j] >= lcs[i][j+1]):
			add(MarkDel, a[i])
			i++
		default:
			add(MarkNew, b[j])
			j++
		}
	}
	flush()
	return out
}

type panelChange struct {
	Kind  string     `json:"k"`
	After bool       `json:"after,omitempty"`
	Old   string     `json:"old,omitempty"`
	Words []wordEdit `json:"words,omitempty"`
}

type panelDiffData struct {
	Prior   VersionRef    `json:"prior"`
	Current VersionRef    `json:"current"`
	Changes []panelChange `json:"changes"`
}

// panelSnapshot removes active HTML before any old content reaches an overlay.
// A list item's nested lists are separate units, not part of its word diff.
func (p *pageData) panelSnapshot(n ast.Node, src []byte, salt string) (string, string) {
	var buf bytes.Buffer
	// The shared renderer unwraps a document's only paragraph. A snapshot
	// needs a block element for both text mapping and deletion placement.
	wrap := n.Kind() == ast.KindParagraph && n.Parent().Kind() == ast.KindDocument && n.PreviousSibling() == nil && n.NextSibling() == nil
	if wrap {
		buf.WriteString("<p>")
	}
	if err := p.rawMarkdown().Renderer().Render(&buf, src, n); err != nil {
		return "", ""
	}
	if wrap {
		buf.WriteString("</p>")
	}
	safe := string(p.sanitizeReply(buf.String(), salt))
	doc, err := html.Parse(strings.NewReader(safe))
	if err != nil {
		return "", ""
	}
	var plain strings.Builder
	listItem := n.Kind() == ast.KindListItem
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || (listItem && (n.Data == "ul" || n.Data == "ol"))) {
			if n.Parent != nil {
				n.Parent.RemoveChild(n)
			}
			return
		}
		if n.Type == html.TextNode && n.Parent != nil && n.Parent.Data != "body" && n.Parent.Data != "head" {
			plain.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			walk(c)
			c = next
		}
	}
	walk(doc)
	var body *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "body" {
			body = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(doc)
	buf.Reset()
	if body != nil {
		for c := body.FirstChild; c != nil; c = c.NextSibling {
			_ = html.Render(&buf, c)
		}
	}
	return strings.TrimSpace(buf.String()), plain.String()
}

func (p *pageData) renderPanel(e EntryView) template.HTML {
	if e.Diff == nil {
		return p.convertWith(p.rawMarkdown(), e.Text, "entry/"+e.ID, nil)
	}
	oldSrc := []byte(e.Diff.old)
	oldUnits := bodyUnits(p.rawMarkdown().Parser().Parse(text.NewReader(oldSrc)), oldSrc)
	data := panelDiffData{Prior: e.Prior, Current: VersionRef{ID: e.ID, Clock: e.Clock, Version: e.Version}}
	out := p.convertWith(p.rawMarkdown(), e.Text, "entry/"+e.ID, func(doc ast.Node, src []byte) {
		units := bodyUnits(doc, src)
		for _, edit := range e.Diff.edits {
			if edit.Block > len(units) || edit.oldTo > len(oldUnits) {
				continue // Cosmetic marks tolerate parser differences.
			}
			change := panelChange{Kind: edit.Kind, After: edit.Block == len(units)}
			var oldText strings.Builder
			for i := edit.oldFrom; i < edit.oldTo; i++ {
				h, text := p.panelSnapshot(oldUnits[i].node, oldSrc, fmt.Sprintf("panel-diff/%s/%d", e.ID, i))
				change.Old += h
				oldText.WriteString(text)
			}
			if edit.Kind == MarkMod && edit.Block < len(units) {
				_, current := p.panelSnapshot(units[edit.Block].node, src, "panel-current/"+e.ID)
				change.Words = diffWords(oldText.String(), current)
			}
			mark := &changeNode{kind: strconv.Itoa(len(data.Changes)), panel: true}
			data.Changes = append(data.Changes, change)
			if edit.Block < len(units) {
				n := units[edit.Block].node
				n.Parent().InsertBefore(n.Parent(), n, mark)
			} else {
				doc.AppendChild(doc, mark)
			}
		}
	})
	raw, _ := json.Marshal(data)
	// json.Marshal escapes <, > and &, so authored content cannot terminate
	// this comment. Unlike an element, metadata leaves :last-child unchanged.
	return out + template.HTML(`<!--wbpdg-data:`+string(raw)+`-->`)
}
