package wsdashboard

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

// splitPanelRows keys the native top-level panel cards by their existing p-* IDs.
// Nested agenda disclosures belong to the card, not independent transport rows.
// Complete rendered HTML is hashed; shell/order changes and unfamiliar markup
// reset to full regions. The source still renders panels as before.
func splitPanelRows(raw string) *keyedRows {
	if strings.Contains(raw, "<!--ws-panel-row-") {
		return nil
	}
	out := &keyedRows{order: []string{}, rows: map[string]keyedRow{}}
	z := html.NewTokenizer(strings.NewReader(raw))
	var stack []string
	offset, start, end := 0, -1, 0
	var key string
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			if z.Err() != io.EOF || len(stack) != 0 || start >= 0 {
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
				if _, dup := attrs[a.Key]; dup {
					return nil
				}
				attrs[a.Key] = a.Val
			}
			panel := strings.Contains(" "+attrs["class"]+" ", " panel ")
			if len(stack) == 0 {
				key = attrs["id"]
				if !panel || tok.Data != "details" || kind != html.StartTagToken || !strings.HasPrefix(key, "p-") || len(key) <= 2 {
					return nil
				}
				if _, dup := out.rows[key]; dup {
					return nil
				}
				start = offset
				out.shell = append(out.shell, raw[end:start])
			} else if panel {
				return nil
			}
			if kind == html.StartTagToken && !voidBoardTag(tok.Data) {
				stack = append(stack, tok.Data)
			}
		case html.EndTagToken:
			tok := z.Token()
			if len(stack) == 0 || stack[len(stack)-1] != tok.Data {
				return nil
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				end = offset + n
				markup := raw[start:end]
				out.order = append(out.order, key)
				out.rows[key] = keyedRow{Key: key, Hash: newRegion(markup).Hash, HTML: markup}
				start = -1
			}
		case html.TextToken:
			if len(stack) == 0 && strings.TrimSpace(string(z.Raw())) != "" {
				return nil
			}
		case html.DoctypeToken:
			return nil
		}
		offset += n
	}
}
