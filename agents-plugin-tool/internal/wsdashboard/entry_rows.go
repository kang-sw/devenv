package wsdashboard

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

// splitEntryRows decomposes only the direct children of .centries. The title bar
// and model history stay in the shell, outside the keyed history container.
// Hash every byte of each rendered row: mail availability, peer presentation
// and wake associations can change without a new ring entry or revision.
// Unfamiliar markup, shell changes or a different window/order use full regions.
func splitEntryRows(raw string) *keyedRows {
	out := &keyedRows{order: []string{}, rows: map[string]keyedRow{}}
	z := html.NewTokenizer(strings.NewReader(raw))
	depth, offset, boxDepth, start, end := 0, 0, -1, -1, 0
	seen := false
	var key string
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			if z.Err() != io.EOF || depth != 0 || start >= 0 || boxDepth >= 0 || !seen {
				return nil
			}
			out.shell = append(out.shell, raw[end:])
			return out
		}
		n := len(z.Raw())
		switch kind {
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			attrs := map[string]string{}
			for _, a := range tok.Attr {
				attrs[a.Key] = a.Val
			}
			if strings.Contains(" "+attrs["class"]+" ", " centries ") {
				if seen || tok.Data != "div" || kind != html.StartTagToken {
					return nil
				}
				seen, boxDepth = true, depth+1
			} else if boxDepth >= 0 && depth == boxDepth {
				key = attrs["data-entry"]
				if key == "" || start >= 0 || kind != html.StartTagToken || voidBoardTag(tok.Data) {
					return nil
				}
				if _, duplicate := out.rows[key]; duplicate {
					return nil
				}
				start = offset
				out.shell = append(out.shell, raw[end:start])
			}
			if kind == html.StartTagToken && !voidBoardTag(tok.Data) {
				depth++
			}
		case html.EndTagToken:
			depth--
			if depth < 0 {
				return nil
			}
			if start >= 0 && depth == boxDepth {
				end = offset + n
				markup := raw[start:end]
				out.order = append(out.order, key)
				out.rows[key] = keyedRow{Key: key, Hash: newRegion(markup).Hash, HTML: markup}
				start = -1
			}
			if boxDepth >= 0 && depth < boxDepth {
				boxDepth = -1
			}
		case html.TextToken:
			if boxDepth >= 0 && depth == boxDepth && strings.TrimSpace(string(z.Raw())) != "" {
				return nil
			}
		}
		offset += n
	}
}
