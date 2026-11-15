package wsinbox

import (
	"regexp"
	"strings"
	"time"
)

const userTimeLayout = "2006-01-02T15:04-07:00"

var userBodyDelimiter = regexp.MustCompile(`</ws:user|&(?:amp;)*lt;/ws:user`)
var userOpen = regexp.MustCompile(`^<ws:user at="([0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}[+-][0-9]{2}:[0-9]{2})">`)

// Only delimiter-like entities gain a level: escaping every ampersand would
// change unrelated delivered text. Decode each match once, never recursively.
func escapeUserBody(text string) string {
	return userBodyDelimiter.ReplaceAllStringFunc(text, func(s string) string {
		if s[0] == '<' {
			return "&lt;/ws:user"
		}
		return "&amp;" + s[1:]
	})
}

func unescapeUserBody(text string) string {
	return userBodyDelimiter.ReplaceAllStringFunc(text, func(s string) string {
		if strings.HasPrefix(s, "&lt;") {
			return "</ws:user"
		}
		if strings.HasPrefix(s, "&amp;") {
			return "&" + s[len("&amp;"):]
		}
		return s
	})
}

// Text wraps each nonempty dashboard item once, regardless of delivery host.
// Commands remain native command lines; interrupt controls carry no text.
func (e Entry) Text() string {
	if e.Command != nil {
		return e.Command.Line()
	}
	body := e.bodyText()
	if body == "" {
		return ""
	}
	at := time.UnixMilli(e.SentAt).In(time.Local)
	if e.UTCOffsetMinutes != nil {
		at = at.In(time.FixedZone("", *e.UTCOffsetMinutes*60))
	}
	return `<ws:user at="` + at.Format(userTimeLayout) + `">` + "\n" + escapeUserBody(body) + "\n</ws:user>"
}

// UserMessage is an envelope's decoded content and exact timestamp, or text
// outside envelopes (At empty). Outside text keeps its position in the batch.
type UserMessage struct{ Text, At string }

// ParseUserMessages splits envelopes before any leading-anchored content
// parser runs. Legacy text is one plain block. A malformed delimiter or time
// rejects the whole parse so callers can display the original turn verbatim.
func ParseUserMessages(text string) ([]UserMessage, bool) {
	var blocks []UserMessage
	plain := func(s string) {
		if s = strings.Trim(s, "\r\n"); strings.TrimSpace(s) != "" {
			blocks = append(blocks, UserMessage{Text: s})
		}
	}
	for text != "" {
		start := strings.Index(text, "<ws:user")
		prefix := text
		if start >= 0 {
			prefix = text[:start]
		}
		if strings.Contains(prefix, "</ws:user") {
			return nil, false
		}
		plain(prefix)
		if start < 0 {
			break
		}
		text = text[start:]
		open := userOpen.FindStringSubmatch(text)
		if open == nil {
			return nil, false
		}
		at, err := time.Parse(userTimeLayout, open[1])
		if err != nil {
			return nil, false
		}
		_, offset := at.Zone()
		if offset < -720*60 || offset > 840*60 {
			return nil, false
		}
		text = text[len(open[0]):]
		end := strings.Index(text, "</ws:user")
		if end < 0 || !strings.HasPrefix(text[end:], "</ws:user>") {
			return nil, false
		}
		body := strings.TrimSuffix(strings.TrimPrefix(text[:end], "\n"), "\n")
		blocks = append(blocks, UserMessage{Text: unescapeUserBody(body), At: open[1]})
		text = text[end+len("</ws:user>"):]
	}
	return blocks, true
}
