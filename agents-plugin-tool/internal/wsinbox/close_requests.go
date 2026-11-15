package wsinbox

import (
	"regexp"
	"strings"
)

// CloseRequest is the user's request to close a topic, not a board mutation.
// Reason may be empty; the agent decides whether the topic can be closed.
type CloseRequest struct {
	Topic  string `json:"topic"`
	Reason string `json:"reason"`
}

const closeRequestEnd = "</wb:close-request>"

var closeRequestHead = regexp.MustCompile(`^<wb:close-request topic="([^"]*)">`)

// CloseRequestBlock follows mentions inside the user envelope. Use the same
// escaping as mentions so neither a topic nor a reason can introduce markup.
func CloseRequestBlock(requests []CloseRequest) string {
	var lines []string
	for _, r := range requests {
		lines = append(lines, `<wb:close-request topic="`+mentionAttrEscaper.Replace(r.Topic)+`">`+mentionTextEscaper.Replace(r.Reason)+closeRequestEnd)
	}
	return strings.Join(lines, "\n")
}

// ParseCloseRequests removes only well-formed leading request elements.
// Malformed or non-leading markup remains ordinary user text.
func ParseCloseRequests(text string) (requests []CloseRequest, rest string) {
	body := strings.TrimLeft(text, " \t\r\n")
	for {
		m := closeRequestHead.FindStringSubmatch(body)
		if m == nil || m[1] == "" {
			break
		}
		after := body[len(m[0]):]
		end := strings.Index(after, closeRequestEnd)
		if end < 0 || strings.Contains(after[:end], "<") {
			break
		}
		tail := after[end+len(closeRequestEnd):]
		if tail != "" && tail[0] != '\n' && !strings.HasPrefix(tail, "\r\n") {
			break
		}
		requests = append(requests, CloseRequest{Topic: mentionUnescaper.Replace(m[1]), Reason: mentionUnescaper.Replace(after[:end])})
		body = strings.TrimLeft(tail, "\r\n")
	}
	if len(requests) == 0 {
		return nil, text
	}
	return requests, body
}
