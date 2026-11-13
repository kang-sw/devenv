package wswhiteboard

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

const (
	EntryPanel  = "panel"
	EntryAgent  = "agent"
	EntrySystem = "system"

	// MaxPanelBytes caps a panel payload in UTF-8 bytes, not characters.
	// Larger material belongs in an external file linked from the panel.
	MaxPanelBytes = 2 * 1024 * 1024
)

// Entry is one immutable topic-stream record. Every entry, including each
// full panel version, has a board-wide durable eN ID. PanelID is the stable,
// board-wide pN target shared by a panel's versions; their entry IDs differ.
// Text is the raw panel payload or log text. Imported marks legacy provenance.
// Folder-store schema versioning belongs to the persistence boundary, not State.
type Entry struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	PanelID  string `json:"panel_id,omitempty"`
	ItemID   int    `json:"item_id,omitempty"`
	Recap    string `json:"recap,omitempty"`
	Text     string `json:"text,omitempty"`
	At       string `json:"at"`
	Imported bool   `json:"imported,omitempty"`
}

// LatestAgent returns the last agent-authored log entry, never a panel or
// system event. Stream order, not timestamps, breaks equal-time ties.
func (d TopicDoc) LatestAgent() (Entry, bool) {
	for i := len(d.Entries) - 1; i >= 0; i-- {
		if d.Entries[i].Kind == EntryAgent {
			return d.Entries[i], true
		}
	}
	return Entry{}, false
}

// LatestAgent returns topic's latest agent log entry, if any.
func (s *State) LatestAgent(topic string) (Entry, bool) {
	if s != nil {
		if i := s.topicDocIndex(s.resolveTopic(topic)); i >= 0 {
			return s.Topics[i].LatestAgent()
		}
	}
	return Entry{}, false
}

func (s *State) appendEntry(topic string, entry Entry, now time.Time) {
	s.EntrySeq++
	entry.ID = fmt.Sprintf("e%d", s.EntrySeq)
	entry.At = now.UTC().Format(time.RFC3339Nano)
	d := s.topicDoc(topic)
	d.Entries = append(d.Entries, entry)
}

func (s *State) event(topic string, now time.Time, format string, args ...any) {
	s.appendEntry(topic, Entry{Kind: EntrySystem, Text: fmt.Sprintf(format, args...)}, now)
}

// ItemID is trusted attribution, never reconstructed from authored text.
func (s *State) eventItem(topic string, id int, now time.Time, format string, args ...any) {
	s.appendEntry(topic, Entry{Kind: EntrySystem, ItemID: id, Text: fmt.Sprintf(format, args...)}, now)
}

// Only accepted activity reopens a topic. Appending a system event by itself
// must not do so (notably close/withdrawal events).
func (s *State) reopenWithEvent(topic string, now time.Time) {
	if s.topicClosed(topic) {
		s.reopenTopic(topic)
		s.event(topic, now, "Topic reopened.")
	}
}

var panelIDPattern = regexp.MustCompile(`^p[1-9][0-9]*$`)

func applyPanel(p *problems, work *State, cmd command, topicOf topicResolver, now time.Time) {
	before := len(p.List)
	topic, topicOK := topicOf(cmd)
	recap, recapOK := wsxmlcmd.RequiredAttr(p, cmd, "recap")
	id, override := cmd.Attr("id")
	if override {
		if !panelIDPattern.MatchString(id) {
			p.Add("%s: must be an existing panel ID (pN); got %q", cmd.At("id"), id)
		} else {
			owner := ""
			for _, doc := range work.Topics {
				for _, entry := range doc.Entries {
					if entry.Kind == EntryPanel && entry.PanelID == id {
						owner = doc.Name
						break
					}
				}
			}
			if owner == "" {
				p.Add("%s: panel %s does not exist; omit id to add a panel", cmd.At("id"), id)
			} else if topicOK && owner != topic {
				p.Add("%s: panel %s belongs to topic %q", cmd.At("id"), id, owner)
			}
		}
	}
	if !utf8.ValidString(cmd.Body) {
		p.Add("%s: panel payload must be valid UTF-8", cmd.Label())
	}
	if len(cmd.Body) > MaxPanelBytes {
		p.Add("%s: panel payload exceeds 2 MiB of UTF-8; split it into panels or link an external file", cmd.Label())
	}
	if fullDocument(cmd.Body) {
		p.Add("%s: a panel takes markdown with inline HTML, not a full document (found <!DOCTYPE or <html)", cmd.Label())
	}
	if len(p.List) != before || !topicOK || !recapOK {
		return
	}
	if !override {
		work.PanelCounter++
		id = fmt.Sprintf("p%d", work.PanelCounter)
	}
	work.reopenWithEvent(topic, now)
	recap = strings.TrimSpace(recap)
	work.appendEntry(topic, Entry{Kind: EntryPanel, PanelID: id, Recap: recap, Text: cmd.Body}, now)
	action := "added"
	if override {
		action = "re-posted"
	}
	work.event(topic, now, "Panel [%s](#%s) %s: %s", id, id, action, recap)
}

func applyLog(p *problems, work *State, cmd command, topicOf topicResolver, now time.Time) {
	topic, topicOK := topicOf(cmd)
	line := strings.TrimSpace(cmd.Body)
	if !wsxmlcmd.NonEmpty(p, cmd.Label()+" body", line) {
		return
	}
	if !inlineLog(cmd.Body) {
		p.Add("%s: a log line takes inline Markdown only, without images, HTML, or block structures; put substantial material in a panel", cmd.Label())
		return
	}
	if !topicOK {
		return
	}
	work.reopenWithEvent(topic, now)
	work.appendEntry(topic, Entry{Kind: EntryAgent, Text: line}, now)
}

// One paragraph on one source line is inline Markdown. Code spans can quote
// HTML or image syntax, but actual images/HTML/block nodes cannot enter a log.
func inlineLog(line string) bool {
	if !utf8.ValidString(line) || strings.ContainsAny(line, "\r\n") {
		return false
	}
	doc := docParser.Parse(text.NewReader([]byte(line)))
	if doc.ChildCount() != 1 || doc.FirstChild().Kind() != ast.KindParagraph {
		return false
	}
	ok := true
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && (n.Kind() == ast.KindImage || n.Kind() == ast.KindRawHTML) {
			ok = false
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return ok
}
