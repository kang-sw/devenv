package wsdashboard

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// treeRows separates each agent and expandable peer from its descendants.
// Child slots exist only in immutable transport inputs, never in rendered HTML.
// A child edit therefore hashes/sends that child, not its ancestor's subtree.
// Shell or topology changes conservatively reset the connection with a full tree.
// Like the rail, rows hash the complete rendered input, not snapshot revisions.
type treeRows struct {
	order   []string
	parents []int
	rows    map[string]treeRowUnit
	shell   string
}

type treeRowUnit struct {
	Key  string `json:"key"`
	Hash string `json:"hash"`
	HTML string `json:"html"`
}

type treeRowsEvent struct {
	Hash  string        `json:"hash"`
	Order []string      `json:"order"`
	Rows  []treeRowUnit `json:"rows"`
}

func treeRowSlot(i int) string { return fmt.Sprintf("<!--ws-tree-row-%d-->", i) }

type treeRowSpan struct {
	start, end, depth, index, parent int
	key                              string
	children                         []*treeRowSpan
}

func splitTreeRows(raw string) *treeRows {
	if strings.Contains(raw, "<!--ws-tree-row-") {
		return nil
	}
	out := &treeRows{order: []string{}, parents: []int{}, rows: map[string]treeRowUnit{}}
	var all, roots, active []*treeRowSpan
	z := html.NewTokenizer(strings.NewReader(raw))
	depth, offset := 0, 0
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			if z.Err() != io.EOF || depth != 0 || len(active) != 0 {
				return nil
			}
			break
		}
		n := len(z.Raw())
		switch kind {
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			attrs := map[string]string{}
			for _, a := range tok.Attr {
				attrs[a.Key] = a.Val
			}
			classes := " " + attrs["class"] + " "
			agent := strings.Contains(classes, " agent ")
			peer := tok.Data == "details" && strings.Contains(classes, " peer ")
			if agent || peer {
				if kind != html.StartTagToken || (agent && tok.Data != "div") {
					return nil
				}
				span := &treeRowSpan{start: offset, depth: depth, index: len(all), parent: -1}
				if peer {
					if attrs["data-peer"] == "" {
						return nil
					}
					span.key = "peer-" + attrs["data-peer"]
				}
				if len(active) > 0 {
					p := active[len(active)-1]
					span.parent = p.index
					p.children = append(p.children, span)
				} else {
					roots = append(roots, span)
				}
				all = append(all, span)
				active = append(active, span)
			} else if tok.Data == "details" && len(active) > 0 {
				span := active[len(active)-1]
				if depth == span.depth+1 {
					key := attrs["data-key"]
					if span.key != "" || !strings.HasPrefix(key, "agent-") || key == "agent-" {
						return nil
					}
					span.key = key
				}
			}
			if kind == html.StartTagToken && !voidBoardTag(tok.Data) {
				depth++
			}
		case html.EndTagToken:
			depth--
			if depth < 0 {
				return nil
			}
			if len(active) > 0 && depth == active[len(active)-1].depth {
				span := active[len(active)-1]
				span.end = offset + n
				if span.key == "" {
					return nil
				}
				active = active[:len(active)-1]
			}
		}
		offset += n
	}
	skeleton := func(start, end int, children []*treeRowSpan) string {
		var b strings.Builder
		for _, child := range children {
			b.WriteString(raw[start:child.start])
			b.WriteString(treeRowSlot(child.index))
			start = child.end
		}
		b.WriteString(raw[start:end])
		return b.String()
	}
	for _, span := range all {
		if _, dup := out.rows[span.key]; dup {
			return nil
		}
		markup := skeleton(span.start, span.end, span.children)
		out.order = append(out.order, span.key)
		out.parents = append(out.parents, span.parent)
		out.rows[span.key] = treeRowUnit{Key: span.key, Hash: newRegion(markup).Hash, HTML: markup}
	}
	out.shell = skeleton(0, len(raw), roots)
	return out
}

type treeRowState struct {
	previous    *treeRows
	initialized bool
}

func (s *treeRowState) event(region renderedRegion, next *treeRows) (string, any) {
	if next != nil && s.previous != nil && next.shell == s.previous.shell && slices.Equal(next.order, s.previous.order) && slices.Equal(next.parents, s.previous.parents) {
		e := treeRowsEvent{Hash: region.Hash, Order: next.order, Rows: []treeRowUnit{}}
		for _, key := range next.order {
			row := next.rows[key]
			if row.Hash != s.previous.rows[key].Hash {
				e.Rows = append(e.Rows, row)
			}
		}
		return "tree-rows", e
	}
	return "region", regionEvent{Name: regionTree, Hash: region.Hash, HTML: string(region.HTML)}
}
