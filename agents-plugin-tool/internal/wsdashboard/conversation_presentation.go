package wsdashboard

import (
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// Child reports require a typed wake cause. Whiteboard-origin user records
// also carry dashboard inbox input, so their origin cannot demote user turns.
func (e ConvEntry) ChildReport() bool { return e.Kind == "wake" && e.Cause == "subagent_report" }

// Wakes without an unambiguous display association keep compact diagnostics.
// Repeated/reclaimed records lack shared delivery identity and stay separate.
func (e ConvEntry) MailWake() bool { return e.Kind == "wake" && e.Cause == "mail" }

// AgentBadge, AgentPreview and AgentExpands feed the shared agent row frame
// (child reports and subagent/background notifications).
func (e ConvEntry) AgentBadge() string {
	if e.ChildReport() {
		return wsi18n.T(e.Lang, "entry.childreport")
	}
	return ""
}

func (e ConvEntry) AgentPreview() string {
	if e.Caption != "" {
		return e.Caption
	}
	line, _, _ := strings.Cut(strings.TrimSpace(e.Text), "\n")
	if line == "" && !e.ChildReport() {
		return e.Cause
	}
	return line
}

func (e ConvEntry) AgentExpands() bool {
	return e.ChildReport() || e.Text != "" || (e.Cause != "subagent_done" && e.Cause != "subagent_failed" && e.Cause != "background_exit")
}

func (e ConvEntry) BoardNudgeSummary() string {
	reasons, topics := e.boardNudge()
	if len(topics) == 0 {
		return ""
	}
	for i, reason := range reasons {
		switch reason {
		case "overdue":
			reasons[i] = wsi18n.T(e.Lang, "entry.board.nudge.overdue")
		case "unblocked":
			reasons[i] = wsi18n.T(e.Lang, "entry.board.nudge.unblocked")
		case "hourly":
			reasons[i] = wsi18n.T(e.Lang, "entry.board.nudge.hourly")
		}
	}
	preview := strings.Join(topics[:min(2, len(topics))], ", ")
	if len(topics) > 2 {
		preview += " +" + strconv.Itoa(len(topics)-2)
	}
	if len(e.BoardNudgeMails) > 1 {
		return wsi18n.T(e.Lang, "entry.board.nudge.batch", strings.Join(reasons, ", "), len(e.BoardNudgeMails), preview)
	}
	return wsi18n.T(e.Lang, "entry.board.nudge", strings.Join(reasons, ", "), preview)
}

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
	Kind, Target, ID, Text string
}

type conversationEffectTopic struct {
	Name, Label, Target string
	Rows                []conversationEffectRow
	Items               []conversationEffectRow
}

type conversationEffectCard struct {
	Topics []conversationEffectTopic
}

var conversationEntryID = regexp.MustCompile(`^e[1-9][0-9]*$`)
var conversationPanelID = regexp.MustCompile(`^p[1-9][0-9]*$`)

// WhiteboardCard uses only the committed projection, never current board text.
// Missing item fields are not reconstructed. Panels expose recaps only, not
// their payloads, even as an in-chat escaped preview.
func (e ConvEntry) WhiteboardCard() conversationEffectCard {
	changes := e.WhiteboardChanges()
	card := conversationEffectCard{}
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

		case "topic":
			if effect.Field == "label" && effect.After != "" {
				topic.Label = effect.After
			}
		}
	}
	return card
}

func effectRow(topic *conversationEffectTopic, kind, id string) *conversationEffectRow {
	rows := &topic.Rows
	if kind == "item" {
		rows = &topic.Items
	}
	for i := range *rows {
		if (*rows)[i].Kind == kind && (*rows)[i].ID == id {
			return &(*rows)[i]
		}
	}
	*rows = append(*rows, conversationEffectRow{Kind: kind, ID: id})
	return &(*rows)[len(*rows)-1]
}

func firstEffectLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}
