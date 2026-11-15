package wsdashboard

import (
	"fmt"
	"io"
	"strings"

	"golang.org/x/net/html"
)

// splitTreeRows separates each agent and expandable peer from its descendants.
// Child slots exist only in immutable transport inputs, never in rendered HTML.
// A child edit therefore hashes/sends that child, not its ancestor's subtree.
// Shell or topology changes conservatively reset the connection with a full tree.
// Like the rail, rows hash the complete rendered input, not snapshot revisions.
func treeRowSlot(i int) string { return fmt.Sprintf("<!--ws-tree-row-%d-->", i) }

type treeRowSpan struct {
	start, end, depth, index, parent int
	key                              string
	children                         []*treeRowSpan
}

func splitTreeRows(raw string) *keyedRows {
	if strings.Contains(raw, "<!--ws-tree-row-") {
		return nil
	}
	out := &keyedRows{order: []string{}, parents: []int{}, rows: map[string]keyedRow{}}
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
		out.rows[span.key] = keyedRow{Key: span.key, Hash: newRegion(markup).Hash, HTML: markup}
	}
	out.shell = []string{skeleton(0, len(raw), roots)}
	return out
}
