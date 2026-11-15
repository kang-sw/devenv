package wsdashboard

import (
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Child reports require a typed wake cause. Whiteboard-origin user records
// also carry dashboard inbox input, so their origin cannot demote user turns.
func (e ConvEntry) ChildReport() bool { return e.Kind == "wake" && e.Cause == "subagent_report" }

// Mail wakes cannot be paired by text, peer or nearby claim time: history
// drops the envelope's sent timestamp/identity, so repetition and reclaimed
// claims remain indistinguishable. Keep independent compact diagnostics.
func (e ConvEntry) MailWake() bool { return e.Kind == "wake" && e.Cause == "mail" }

var peerLeadFirstLine = regexp.MustCompile(`^\[(?:ws|wsflow) note: from a peer lead session \((?:(?:claude|codex|pi), )?not a worker\), with the same authority as yours\]$`)

// A matching stored note earns only a display badge. It is not an identity or
// privilege check; the original body remains available unchanged.
func (e ConvEntry) PeerLead() bool {
	if !e.HasBody {
		return false
	}
	line, _, _ := strings.Cut(e.Body, "\n")
	return peerLeadFirstLine.MatchString(line)
}

func (e ConvEntry) MailBody() string {
	if e.PeerLead() {
		_, body, _ := strings.Cut(e.Body, "\n")
		return body
	}
	return e.Body
}

func (e ConvEntry) MailPreview() string {
	if e.PeerLead() {
		_, rest, _ := strings.Cut(e.Body, "\n")
		line, _, _ := strings.Cut(rest, "\n")
		return line
	}
	return e.Title
}

type conversationEffectRow struct {
	Kind, Target, ID, Text, Class string
	Options                       []string
}

type conversationEffectTopic struct {
	Name, Label, Target string
	Rows                []conversationEffectRow
}

type conversationEffectCard struct {
	Topics []conversationEffectTopic
	Folded bool
}

var conversationEntryID = regexp.MustCompile(`^e[1-9][0-9]*$`)
var conversationPanelID = regexp.MustCompile(`^p[1-9][0-9]*$`)

// WhiteboardCard uses only the committed projection, never current board text.
// Missing item fields are not reconstructed. Panels expose recaps only, not
// their payloads, even as an in-chat escaped preview.
func (e ConvEntry) WhiteboardCard() conversationEffectCard {
	changes := e.WhiteboardChanges()
	card := conversationEffectCard{Folded: len(changes) > 0}
	for _, effect := range changes {
		index := -1
		for i := range card.Topics {
			if card.Topics[i].Name == effect.Topic {
				index = i
				break
			}
		}
		if index < 0 {
			target := ""
			if effect.Topic != "" {
				target = "topic-" + hex.EncodeToString([]byte(effect.Topic))
			}
			card.Topics = append(card.Topics, conversationEffectTopic{Name: effect.Topic, Label: effect.Topic, Target: target})
			index = len(card.Topics) - 1
		}
		topic := &card.Topics[index]
		if effect.Kind != "log" || effect.Topic == "" {
			card.Folded = false
		}
		switch effect.Kind {
		case "log":
			target := ""
			if conversationEntryID.MatchString(effect.EntryID) {
				target = effect.EntryID
			}
			topic.Rows = append(topic.Rows, conversationEffectRow{Kind: "log", Target: target, Text: effect.After})
		case "panel":
			// A payload and its recap describe one immutable panel revision.
			key := effect.EntryID + "/" + effect.PanelID
			row := effectRow(topic, "panel", key)
			if conversationPanelID.MatchString(effect.PanelID) {
				row.Target = effect.PanelID
			}
			if effect.Field == "recap" {
				row.Text = firstEffectLine(effect.After)
			}
		case "item":
			row := effectRow(topic, "item", strconv.Itoa(effect.ID))
			if effect.ID > 0 && effect.ID <= 999999 {
				row.Target = "item-" + strconv.Itoa(effect.ID)
			}
			switch effect.Field {
			case "title":
				row.Text = effect.After
			case "class":
				row.Class = effect.After
			case "options":
				_ = json.Unmarshal([]byte(effect.After), &row.Options)
			}
		case "topic":
			if effect.Field == "label" && effect.After != "" {
				topic.Label = effect.After
			}
		}
	}
	card.Folded = card.Folded && len(card.Topics) == 1
	return card
}

func effectRow(topic *conversationEffectTopic, kind, id string) *conversationEffectRow {
	for i := range topic.Rows {
		if topic.Rows[i].Kind == kind && topic.Rows[i].ID == id {
			return &topic.Rows[i]
		}
	}
	topic.Rows = append(topic.Rows, conversationEffectRow{Kind: kind, ID: id})
	return &topic.Rows[len(topic.Rows)-1]
}

// Panel inspection stays on the board. Other transitions retain escaped
// before/after diagnostics independently of the update card's fold state.
func (e ConvEntry) WhiteboardDetails() []wswhiteboard.Effect {
	var out []wswhiteboard.Effect
	for _, effect := range e.WhiteboardChanges() {
		if effect.Kind != "panel" {
			out = append(out, effect)
		}
	}
	return out
}

func firstEffectLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}
