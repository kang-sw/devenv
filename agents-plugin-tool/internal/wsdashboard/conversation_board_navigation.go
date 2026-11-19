package wsdashboard

import (
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func boardTopicTarget(topic string) string {
	if topic == "" {
		return ""
	}
	return "topic-" + hex.EncodeToString([]byte(topic))
}

// BoardNavigationTargets projects the already loaded board, including closed
// topics and retained entries. It performs no file reads or client lookups.
func BoardNavigationTargets(state *wswhiteboard.State) map[string]string {
	targets := map[string]string{}
	if state == nil {
		return targets
	}
	for _, item := range state.Items {
		targets["item-"+strconv.Itoa(item.ID)] = boardTopicTarget(item.Topic)
	}
	for _, topic := range state.Topics {
		for _, entry := range topic.Entries {
			targets[entry.ID] = boardTopicTarget(topic.Name)
			if entry.PanelID != "" {
				targets[entry.PanelID] = boardTopicTarget(topic.Name)
			}
		}
	}
	return targets
}

// These are presentation-only chips; the stored inbox/ring payload stays intact.
type conversationChip struct {
	IDs, Value, Note, Topic, Text string
	Target, TopicTarget           string
}

func answerChips(lines []wsinbox.AnswerLine, targets map[string]string) []conversationChip {
	var out []conversationChip
	for _, line := range lines {
		if line.Topic != "" {
			target := boardTopicTarget(line.Topic)
			out = append(out, conversationChip{Topic: line.Topic, Note: line.Note, Target: target, TopicTarget: target})
			continue
		}
		for _, group := range strings.Split(line.IDs, ",") {
			first, last, ranged := strings.Cut(group, "..")
			start, err := strconv.Atoi(first)
			end := start
			if ranged {
				end, err = strconv.Atoi(last)
			}
			// Invalid legacy IDs stay literal; valid ranges expose every item.
			if err != nil || start < 1 || end < start || end > 999999 {
				out = append(out, conversationChip{IDs: group, Value: line.Value, Note: line.Note})
				continue
			}
			for id := start; id <= end; id++ {
				label := strconv.Itoa(id)
				if !ranged {
					label = first
				}
				target := "item-" + strconv.Itoa(id)
				out = append(out, conversationChip{IDs: label, Value: line.Value, Note: line.Note, Target: target, TopicTarget: targets[target]})
			}
		}
	}
	return out
}

func mentionChips(mentions []wsinbox.Mention, targets map[string]string) []conversationChip {
	var out []conversationChip
	for _, m := range mentions {
		target, topic := boardTopicTarget(m.Topic), boardTopicTarget(m.Topic)
		if conversationEntryID.MatchString(m.Entry) {
			target = m.Entry
		} else if id, err := strconv.Atoi(m.ID); err == nil && id > 0 && id <= 999999 {
			target = "item-" + strconv.Itoa(id)
			// An item may have moved since the quoted selection was sent.
			topic = targets[target]
		}
		if current := targets[target]; current != "" {
			topic = current
		}
		out = append(out, conversationChip{IDs: m.ID, Topic: m.Topic, Text: m.Text, Target: target, TopicTarget: topic})
	}
	return out
}

func (e ConvEntry) AnswerChips() []conversationChip  { return answerChips(e.Answers, e.BoardTargets) }
func (e ConvEntry) MentionChips() []conversationChip { return mentionChips(e.Mentions, e.BoardTargets) }
func (h HeldView) AnswerChips() []conversationChip   { return answerChips(h.Answers(), h.BoardTargets) }
func (h HeldView) MentionChips() []conversationChip  { return mentionChips(h.Mentions, h.BoardTargets) }

func attributeBoardNavigation(cv *ConversationView, targets map[string]string) {
	var entries func([]ConvEntry)
	entries = func(list []ConvEntry) {
		for i := range list {
			list[i].BoardTargets = targets
			entries(list[i].UserBlocks)
		}
	}
	entries(cv.Entries)
	for i := range cv.Held {
		cv.Held[i].BoardTargets = targets
	}
}

type conversationBoardText struct {
	Text, Target, TopicTarget string
}

// Match the board's digits-only item grammar (and its durable panel grammar).
// Unlike markdown, these card previews are plain text, not interpreted HTML.
var conversationBoardRef = regexp.MustCompile(`\{\{(p[1-9][0-9]*|[0-9]+)\}\}`)

func conversationBoardTextParts(text string, targets map[string]string) []conversationBoardText {
	var out []conversationBoardText
	last := 0
	for _, m := range conversationBoardRef.FindAllStringSubmatchIndex(text, -1) {
		ref := text[m[2]:m[3]]
		target, label := ref, ref
		if !strings.HasPrefix(ref, "p") {
			id, err := strconv.Atoi(ref)
			if err != nil || id < 1 || id > 999999 {
				continue
			}
			label = "(" + strconv.Itoa(id) + ")"
			target = "item-" + strconv.Itoa(id)
		}
		if m[0] > last {
			out = append(out, conversationBoardText{Text: text[last:m[0]]})
		}
		out = append(out, conversationBoardText{Text: label, Target: target, TopicTarget: targets[target]})
		last = m[1]
	}
	if last < len(text) {
		out = append(out, conversationBoardText{Text: text[last:]})
	}
	return out
}

// Only referenced destinations enter the entry cache key. Unrelated board
// updates must not invalidate every conversation row.
func boardNavigationKey(e ConvEntry) string {
	var b strings.Builder
	for _, chip := range append(e.AnswerChips(), e.MentionChips()...) {
		b.WriteString("\x00" + chip.Target + "\x00" + chip.TopicTarget)
	}
	for _, topic := range e.WhiteboardCard().Topics {
		for _, row := range topic.Rows {
			for _, part := range row.Parts {
				b.WriteString("\x00" + part.Target + "\x00" + part.TopicTarget)
			}
		}
	}
	for _, block := range e.UserBlocks {
		b.WriteString(boardNavigationKey(block))
	}
	return b.String()
}
