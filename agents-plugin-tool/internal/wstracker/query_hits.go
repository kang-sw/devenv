package wstracker

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// SearchHit is one place an item of a search matched: the palette lists one
// row per hit. Field is "body" or "comment" when a free-text term occurs in
// the item's body or in a shown comment (Comment is that comment's id, Text
// an excerpt around the earliest occurrence), and "item" with empty Text when
// neither did: the item matched on its title, alias, id, or project, or
// only on field filters.
type SearchHit struct {
	ID      string
	Title   string
	Alias   string
	Kind    string
	Status  string // the derived status, "" for a topic
	Hidden  bool
	Field   string // "body", "comment", or "item"
	Comment string // the comment id, for Field "comment"
	Text    string // the excerpt, for "body" and "comment"
}

// maxCommentHits caps the comment hits one item contributes.
const maxCommentHits = 3

// Excerpt window, in runes: context before the match and the total length.
const (
	excerptBefore = 40
	excerptTotal  = 160
)

// SearchHits returns the places the items of v that pass q matched, grouped
// per item in the order Search returns the items (tree order, hidden items
// included when they pass). An item contributes its body hit, then up to
// maxCommentHits comment hits in thread order, or a single "item" hit when
// neither matched.
func (v *View) SearchHits(q Query) []SearchHit {
	out := []SearchHit{}
	var terms []string
	for _, t := range q.terms {
		if t.key == "" {
			terms = append(terms, t.value)
		}
	}
	for _, id := range v.Search(q) {
		it := v.Items[id]
		hit := func(field, comment, text string) SearchHit {
			h := SearchHit{ID: it.ID, Title: it.Title, Alias: it.Alias, Kind: it.Kind, Hidden: it.Hidden, Field: field, Comment: comment, Text: text}
			if it.Status != nil {
				h.Status = *it.Status
			}
			return h
		}
		n := len(out)
		if ex, ok := excerptOf(it.Body, terms); ok {
			out = append(out, hit("body", "", ex))
		}
		comments := 0
		for _, cid := range it.Thread {
			if comments == maxCommentHits {
				break
			}
			c, ok := v.Comments[cid]
			if !ok || c.Hidden != nil {
				continue
			}
			if ex, ok := excerptOf(c.Body, terms); ok {
				out = append(out, hit("comment", cid, ex))
				comments++
			}
		}
		if len(out) == n {
			out = append(out, hit("item", "", ""))
		}
	}
	return out
}

// excerptOf returns an excerpt of s around the earliest occurrence of any of
// terms (lower-cased, matched as match() does: case-insensitively), and
// whether one occurs. Offsets are counted in runes: ToLower maps rune by
// rune, so a rune index of the lower-cased text is the same index of s,
// while its byte offsets are not.
func excerptOf(s string, terms []string) (string, bool) {
	lower := strings.ToLower(s)
	start := -1
	for _, t := range terms {
		i := strings.Index(lower, t)
		if i < 0 {
			continue
		}
		rs := utf8.RuneCountInString(lower[:i])
		if start < 0 || rs < start {
			start = rs
		}
	}
	if start < 0 {
		return "", false
	}

	// Collapse whitespace runs to one space, trimming both ends, and keep
	// where each original rune landed so the match start can be located.
	// The window is anchored on the start; a longer match is cut at its end.
	orig := []rune(s)
	text := make([]rune, 0, len(orig))
	at := make([]int, len(orig)+1)
	space := false
	for i, r := range orig {
		if unicode.IsSpace(r) {
			if len(text) > 0 && !space {
				text = append(text, ' ')
				space = true
			}
			at[i] = max(len(text)-1, 0)
			continue
		}
		space = false
		at[i] = len(text)
		text = append(text, r)
	}
	at[len(orig)] = len(text)
	if space {
		text = text[:len(text)-1]
	}
	ms := min(at[start], max(len(text)-1, 0))

	from := max(ms-excerptBefore, 0)
	to := min(from+excerptTotal, len(text))
	var b strings.Builder
	if from > 0 {
		b.WriteString("…")
	}
	b.WriteString(string(text[from:to]))
	if to < len(text) {
		b.WriteString("…")
	}
	return b.String(), true
}
