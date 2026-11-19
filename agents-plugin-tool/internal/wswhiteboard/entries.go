package wswhiteboard

import (
	"encoding/json"
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
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	PanelID string `json:"panel_id,omitempty"`
	// PanelName binds a topic-local name in retained immutable history. Legacy
	// id-only versions may omit it; the binding survives those reposts.
	PanelName string `json:"panel_name,omitempty"`
	// LegacyContent binds the fixed body/edit panel across old writer saves.
	LegacyContent bool   `json:"legacy_content,omitempty"`
	ItemID        int    `json:"item_id,omitempty"`
	Recap         string `json:"recap,omitempty"`
	Text          string `json:"text,omitempty"`
	At            string `json:"at"`
	Imported      bool   `json:"imported,omitempty"`
	// TopicState is immutable machine metadata, parsed separately from Text.
	TopicState       json.RawMessage `json:"topic_state,omitempty"`
	BoardNotice      json.RawMessage `json:"board_notice,omitempty"`
	WaitingOn        json.RawMessage `json:"waiting_on,omitempty"`
	UnblockedReceipt json.RawMessage `json:"unblocked_receipt,omitempty"`
	// Display-only batch provenance. Raw inputs distinguishes activation []
	// from an unstamped batch; none of these fields is model authority.
	Inputs       json.RawMessage `json:"inputs,omitempty"`
	InputsFloor  string          `json:"inputs_floor,omitempty"`
	Seq          int             `json:"seq,omitempty"`
	AnswerSentAt int64           `json:"answer_sent_at,omitempty"`
	unknown      map[string]json.RawMessage
}

// LatestAgent returns the newest agent-authored log entry, never a panel or
// system event. Equal timestamps use the larger durable eN ID.
func (d TopicDoc) LatestAgent() (Entry, bool) {
	var latest Entry
	var latestAt time.Time
	found := false
	for _, e := range d.Entries {
		if e.Kind != EntryAgent {
			continue
		}
		at, _ := time.Parse(time.RFC3339Nano, e.At)
		if !found || at.After(latestAt) || (at.Equal(latestAt) && folderNumber(e.ID, "e") > folderNumber(latest.ID, "e")) {
			latest, latestAt, found = e, at, true
		}
	}
	return latest, found
}

// LatestSummary returns the newest agent log or panel version for display.
// Timestamps decide recency; equal timestamps use the larger durable eN ID.
// System events never replace the head, and LatestAgent stays log-only.
func (d TopicDoc) LatestSummary() *Entry {
	var latest *Entry
	var latestAt time.Time
	for i := range d.Entries {
		e := &d.Entries[i]
		if e.Kind != EntryAgent && e.Kind != EntryPanel {
			continue
		}
		at, _ := time.Parse(time.RFC3339Nano, e.At)
		if latest == nil || at.After(latestAt) || (at.Equal(latestAt) && folderNumber(e.ID, "e") > folderNumber(latest.ID, "e")) {
			latest, latestAt = e, at
		}
	}
	return latest
}

// Summary is the display text of a log or panel, never a panel's payload.
func (e Entry) Summary() string {
	if e.Kind == EntryPanel {
		return e.Recap
	}
	return e.Text
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

func applyPanel(p *problems, work *State, origins bodyOrigins, cmd command, topicOf topicResolver, mark int, now time.Time, sourceRoot string) {
	before := len(p.List)
	topic, topicOK := topicOf(cmd)
	patch := len(cmd.Children) > 0
	name, hasName := cmd.Attr("name")
	if hasName {
		name = strings.TrimSpace(name)
		wsxmlcmd.NonEmpty(p, cmd.At("name"), name)
	}
	if patch && !hasName {
		// v0.50.0 refuses name before publishing a command. An id-only patch
		// could instead become a literal full replacement on that old parser.
		p.Add("%s: patches require name; id alone is not a safe patch selector", cmd.At("name"))
	}
	namedID := ""
	if topicOK && hasName {
		if i := work.topicDocIndex(topic); i >= 0 {
			for _, entry := range work.Topics[i].Entries {
				if entry.Kind == EntryPanel && entry.PanelName == name && name != "" {
					if namedID != "" && namedID != entry.PanelID {
						p.Add("%s: panel name %q has conflicting history", cmd.At("name"), name)
					}
					namedID = entry.PanelID
				}
			}
		}
	}
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
	if override && hasName && (namedID == "" || id != namedID) {
		p.Add("%s: id and name must resolve to the same existing panel", cmd.Label())
	}
	if !override && namedID != "" {
		id, override = namedID, true
	}
	var latest Entry
	if topicOK && override {
		if i := work.topicDocIndex(topic); i >= 0 {
			for _, entry := range work.Topics[i].Entries {
				if entry.Kind == EntryPanel && entry.PanelID == id {
					latest = entry
				}
			}
		}
	}
	recap, recapOK := cmd.Attr("recap")
	if patch && !recapOK {
		recap, recapOK = latest.Recap, latest.ID != ""
	} else {
		recap, recapOK = wsxmlcmd.RequiredAttr(p, cmd, "recap")
	}
	body := cmd.Body
	src, hasSrc := cmd.Attr("src")
	sourced := false
	if hasSrc {
		switch {
		case patch:
			p.Add("%s: src is a full payload from a file and cannot be combined with wb:change bundles", cmd.At("src"))
		case strings.TrimSpace(body) != "":
			p.Add("%s: src and inline panel content are two sources; give one", cmd.At("src"))
		default:
			text, err := ReadPanelSource(sourceRoot, src)
			if err != nil {
				p.Add("%s: %q: %v", cmd.At("src"), src, err)
			} else if !utf8.ValidString(text) {
				p.Add("%s: %q is not valid UTF-8 text; only text files can be a panel source", cmd.At("src"), src)
			} else {
				body, sourced = text, true
			}
		}
	}
	if patch {
		if latest.ID == "" {
			p.Add("%s: panel name %q does not exist; post a full payload first", cmd.At("name"), name)
		}
		if strings.TrimSpace(body) != "" {
			p.Add("%s: patches take only change wrappers, not text or a full payload outside them", cmd.Label())
		}
		body = latest.Text
		for _, change := range cmd.Children {
			old, hasOld := change.Single(p, "wb:old")
			repl, hasNew := change.Single(p, "wb:new")
			if !hasOld {
				p.Add("%s: <wb:old> required", change.Label())
			} else if old == "" {
				p.Add("%s: <wb:old> must be non-empty", change.Label())
			}
			if !hasNew {
				p.Add("%s: <wb:new> required (empty deletes the match)", change.Label())
			}
			if !hasOld || old == "" || !hasNew {
				continue
			}
			if change.LiteralAmbiguous("wb:old", body) {
				p.Add("%s: <wb:old> is ambiguous: its entities decode, and its text as written also occurs in panel %s; wrap the operand in <wb:raw>...</wb:raw> to match that literal text, or write the decoded characters directly", change.Label(), id)
				continue
			}
			match := strings.Index(body, old)
			switch {
			case match < 0:
				p.Add("%s: <wb:old> matches nothing in panel %s", change.Label(), id)
			case strings.Contains(body[match+1:], old):
				p.Add("%s: <wb:old> matches multiple places in panel %s; widen it to one", change.Label(), id)
			default:
				body = body[:match] + repl + body[match+len(old):]
			}
		}
	}
	if !utf8.ValidString(body) {
		p.Add("%s: panel payload must be valid UTF-8", cmd.Label())
	}
	if len(body) > MaxPanelBytes {
		p.Add("%s: panel payload exceeds 2 MiB of UTF-8; split it into panels or link an external file", cmd.Label())
	}
	if fullDocument(body) {
		if sourced {
			p.Add("%s: %q is a full HTML document (found <!DOCTYPE or <html); a panel source is a fragment, so save the body content with any <style> and no <html>, <head> or <body> wrapper", cmd.At("src"), src)
		} else {
			p.Add("%s: a panel takes markdown with inline HTML, not a full document (found <!DOCTYPE or <html)", cmd.Label())
		}
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
	legacy := false
	if doc, ok := baseDoc(work, topic); ok {
		fixed, exists := doc.legacyContentPanel()
		legacy = exists && fixed.PanelID == id
	}
	work.appendEntry(topic, Entry{Kind: EntryPanel, PanelID: id, PanelName: name, LegacyContent: legacy, Recap: recap, Text: body}, now)
	if legacy {
		if body == "" {
			work.clearTopicBody(topic)
		} else {
			work.setTopicBody(topic, body, mark, origins)
		}
	}
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
