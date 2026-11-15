package wsdashboard

import (
	"io"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// railRows is the immutable decomposition of a shared rail render. Rows are
// non-nested .rs elements identified by data-rid, including their inert hover
// evidence. Shell fragments retain every byte between rows and their group
// boundaries. An unfamiliar shape, or a changed shell, uses the full region.
// Rows still render on every wake: time, layout and worktree inputs matter.
type railRows struct {
	order []string
	rows  map[string]railRowUnit
	shell []string
}

type railRowUnit struct {
	Key  string `json:"key"`
	Hash string `json:"hash"`
	HTML string `json:"html"`
}

type railRowsEvent struct {
	Hash  string        `json:"hash"`
	Order []string      `json:"order"`
	Rows  []railRowUnit `json:"rows"`
}

func splitRailRows(raw string) *railRows {
	rows := &railRows{order: []string{}, rows: map[string]railRowUnit{}}
	z := html.NewTokenizer(strings.NewReader(raw))
	depth, offset, start, rowDepth, end := 0, 0, -1, 0, 0
	var key string
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			if z.Err() != io.EOF || depth != 0 || start >= 0 {
				return nil
			}
			rows.shell = append(rows.shell, raw[end:])
			return rows
		}
		n := len(z.Raw())
		switch kind {
		case html.StartTagToken, html.SelfClosingTagToken:
			token := z.Token()
			attrs := map[string]string{}
			for _, a := range token.Attr {
				attrs[a.Key] = a.Val
			}
			if strings.Contains(" "+attrs["class"]+" ", " rs ") {
				if start >= 0 || token.Data != "div" || kind != html.StartTagToken || attrs["data-rid"] == "" {
					return nil
				}
				key = attrs["data-rid"]
				if _, duplicate := rows.rows[key]; duplicate {
					return nil
				}
				start, rowDepth = offset, depth
				rows.shell = append(rows.shell, raw[end:start])
			}
			if kind == html.StartTagToken && !voidBoardTag(token.Data) {
				depth++
			}
		case html.EndTagToken:
			depth--
			if depth < 0 {
				return nil
			}
			if start >= 0 && depth == rowDepth {
				end = offset + n
				unit := newRegion(raw[start:end])
				rows.rows[key] = railRowUnit{Key: key, Hash: unit.Hash, HTML: string(unit.HTML)}
				rows.order = append(rows.order, key)
				start = -1
			}
		}
		offset += n
	}
}

// Baselines belong to a connection, never the shared page. Reconnects send a
// full region even with h=current; an unprovable shell also resets by full.
type railRowState struct {
	previous    *railRows
	initialized bool // a successful full send, even when decomposition is unavailable
}

func (s *railRowState) event(region renderedRegion, next *railRows) (string, any) {
	if next != nil && s.previous != nil && slices.Equal(next.shell, s.previous.shell) {
		e := railRowsEvent{Hash: region.Hash, Order: next.order, Rows: []railRowUnit{}}
		for _, key := range next.order {
			row := next.rows[key]
			if old, ok := s.previous.rows[key]; !ok || old.Hash != row.Hash {
				e.Rows = append(e.Rows, row)
			}
		}
		return "rail-rows", e
	}
	return "region", regionEvent{Name: regionRail, Hash: region.Hash, HTML: string(region.HTML)}
}
