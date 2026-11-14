package wsdashboard

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

// boardTopics is an immutable decomposition of one shared page render. Only
// direct topic sections participate; deferred topics inside templates belong
// to shell. The board template puts topics in one contiguous run, followed by
// controls and deferred history. An unfamiliar layout falls back to regions.
type boardTopics struct {
	order  []string
	topics map[string]boardTopic
	shell  string
}

type boardTopic struct {
	Key  string `json:"key"`
	Hash string `json:"hash"`
	HTML string `json:"html"`
}

// boardTopicsEvent is opt-in and additive beside regionEvent. An empty
// topics list still carries an order change or deletion.
type boardTopicsEvent struct {
	Hash   string       `json:"hash"`
	Order  []string     `json:"order"`
	Topics []boardTopic `json:"topics"`
}

func splitBoardTopics(raw string) *boardTopics {
	b := &boardTopics{order: []string{}, topics: map[string]boardTopic{}}
	z := html.NewTokenizer(strings.NewReader(raw))
	depth, offset, start, end := 0, 0, -1, 0
	var topic boardTopic
	var shell strings.Builder
	pastTopics := false
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			if z.Err() != io.EOF || depth != 0 || start >= 0 {
				return nil
			}
			shell.WriteString(raw[end:])
			b.shell = shell.String()
			return b
		}
		n := len(z.Raw())
		if kind == html.StartTagToken || kind == html.SelfClosingTagToken {
			t := z.Token()
			if depth == 0 {
				attrs := map[string]string{}
				for _, a := range t.Attr {
					attrs[a.Key] = a.Val
				}
				isTopic := t.Data == "section" && strings.Contains(" "+attrs["class"]+" ", " topic ")
				if isTopic {
					if pastTopics || attrs["data-th"] == "" || attrs["data-topic"] == "" || kind != html.StartTagToken {
						return nil
					}
					topic = boardTopic{Key: attrs["data-topic"], Hash: attrs["data-th"]}
					if _, duplicate := b.topics[topic.Key]; duplicate {
						return nil
					}
					start = offset
					shell.WriteString(raw[end:start])
				} else if len(b.order) > 0 {
					pastTopics = true
				}
			}
			if kind == html.StartTagToken && !voidBoardTag(t.Data) {
				depth++
			}
		} else if kind == html.EndTagToken {
			depth--
			if depth < 0 {
				return nil
			}
			if depth == 0 && start >= 0 {
				end = offset + n
				topic.HTML = raw[start:end]
				b.order = append(b.order, topic.Key)
				b.topics[topic.Key] = topic
				start = -1
			}
		}
		offset += n
	}
}

func voidBoardTag(tag string) bool {
	switch tag {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

// boardTopicState is per connection, never shared with another stream. Even
// when h names the current board, the first send is full: a reconnect has no
// topic baseline. Failed writes never advance the connection's baseline.
type boardTopicState struct{ previous *boardTopics }

func (s *boardTopicState) event(region renderedRegion, next *boardTopics) (string, any) {
	if next != nil && s.previous != nil && next.shell == s.previous.shell {
		e := boardTopicsEvent{Hash: region.Hash, Order: next.order, Topics: []boardTopic{}}
		for _, key := range next.order {
			t := next.topics[key]
			if old, ok := s.previous.topics[key]; !ok || old.Hash != t.Hash {
				e.Topics = append(e.Topics, t)
			}
		}
		return "board-topics", e
	}
	return "region", regionEvent{Name: regionBoard, Hash: region.Hash, HTML: string(region.HTML)}
}
