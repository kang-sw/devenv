package wsinbox

import (
	"regexp"
	"strings"
)

// A mention is text the user selected inside a board topic card and added to
// the message, so the lead reads it as what the message points at. The
// message carries each one as an element after the answer block and before
// the typed text, in the order the user added them:
//
//	<wb:mention topic="ship-gate" id="316">selected &lt;text&gt;</wb:mention>
//
// topic is the topic's key (its stem, or its name when it has none, as a
// topic note names it); id is present only when the whole selection lay in
// one item card. In the attribute "&", "<", ">", and the quote are escaped;
// in the content "&", "<", and ">", so the first "</wb:mention>" ends it.
// Elements are separated by one newline.
const (
	mentionOpen  = "<wb:mention topic=\""
	mentionClose = "</wb:mention>"
)

var mentionHead = regexp.MustCompile(`^<wb:mention topic="([^"]*)"(?: id="([0-9]{1,6})")?>`)

// Mention is one mention: the topic's key, the item ID when the selection
// lay in one item card ("" otherwise), and the selected text.
type Mention struct {
	Topic string `json:"topic"`
	ID    string `json:"id,omitempty"`
	Text  string `json:"text"`
}

var (
	mentionAttrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	mentionTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	mentionUnescaper   = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&amp;", "&")
)

// MentionBlock renders mentions as their elements, one per line; "" when
// there are none.
func MentionBlock(mentions []Mention) string {
	var lines []string
	for _, m := range mentions {
		id := ""
		if m.ID != "" {
			id = ` id="` + m.ID + `"`
		}
		lines = append(lines, mentionOpen+mentionAttrEscaper.Replace(m.Topic)+`"`+id+">"+mentionTextEscaper.Replace(m.Text)+mentionClose)
	}
	return strings.Join(lines, "\n")
}

// ParseMentions takes the mention elements text starts with (after leading
// blank space) out of it, returning them unescaped and the text after them.
// Parsing stops at the first thing that is not a well-formed element; with
// none, text comes back whole.
func ParseMentions(text string) (mentions []Mention, rest string) {
	body := strings.TrimLeft(text, " \t\r\n")
	for {
		m := mentionHead.FindStringSubmatch(body)
		if m == nil {
			break
		}
		after := body[len(m[0]):]
		end := strings.Index(after, mentionClose)
		if end < 0 || strings.Contains(after[:end], "<") {
			break
		}
		tail := after[end+len(mentionClose):]
		if tail != "" && tail[0] != '\n' && !strings.HasPrefix(tail, "\r\n") {
			break
		}
		mentions = append(mentions, Mention{Topic: mentionUnescaper.Replace(m[1]), ID: m[2], Text: mentionUnescaper.Replace(after[:end])})
		body = strings.TrimLeft(tail, "\r\n")
	}
	if len(mentions) == 0 {
		return nil, text
	}
	return mentions, body
}
