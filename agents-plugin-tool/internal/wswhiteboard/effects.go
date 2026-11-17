package wswhiteboard

import (
	"encoding/json"
	"slices"
	"strconv"
)

// Effect is one semantic transition made by an accepted Apply batch. It is
// transient, never part of the authoritative board store. Values are plain
// text, including panel payloads: consumers must not execute or render them.
// Kind/Field form a closed variant (EffectFields). IDs are resolved board IDs.
type Effect struct {
	Kind    string `json:"kind"`
	Topic   string `json:"topic,omitempty"`
	ID      int    `json:"id,omitempty"`
	EntryID string `json:"entryId,omitempty"`
	PanelID string `json:"panelId,omitempty"`
	Field   string `json:"field"`
	Before  string `json:"before,omitempty"`
	After   string `json:"after,omitempty"`
	// Label is the topic's display label when it differs from Topic (the
	// stem), stamped on every topic-scoped effect so a renderer needs no
	// board lookup. Additive: carriers without it render the stem.
	Label string `json:"label,omitempty"`
}

// stampEffectLabels fills Effect.Label from the committed state. A stemless
// topic's label is its own name and stays empty.
func stampEffectLabels(effects []Effect, s *State) {
	for i := range effects {
		if t := effects[i].Topic; t != "" {
			if l := s.TopicLabel(t); l != t {
				effects[i].Label = l
			}
		}
	}
}

// EffectFields is the supported display contract, not a command grammar.
func EffectFields(kind string) []string {
	switch kind {
	case "item":
		return []string{"presence", "topic", "class", "title", "options", "recommendation", "rationale", "status", "answer", "answer-quote", "links"}
	case "topic":
		return []string{"presence", "label", "body", "tickets", "closed", "result"}
	case "panel":
		return []string{"payload", "recap"}
	case "log":
		return []string{"text"}
	case "setting":
		return []string{"id-floor", "embed-dir"}
	}
	return nil
}

// effectSnapshot copies mutable containers only. The command helpers replace
// strings and slice values, never edit existing entries or list elements.
// Copying panel bytes for each command would multiply large-board costs.
func effectSnapshot(s *State) *State {
	out := *s
	out.Items = slices.Clone(s.Items)
	out.Topics = slices.Clone(s.Topics)
	out.ClosedTopics = slices.Clone(s.ClosedTopics)
	out.Labels = map[string]string{}
	for k, v := range s.Labels {
		out.Labels[k] = v
	}
	out.CloseResults = map[string]string{}
	for k, v := range s.CloseResults {
		out.CloseResults[k] = v
	}
	return &out
}

func effectList(v []string) string {
	if len(v) == 0 {
		return ""
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// semanticEffects compares the typed states immediately around a command,
// not event prose, timestamps, generation counters, or requested arguments.
// Bookkeeping changes alone produce nothing. Repeated commands retain their
// transitions, even if a later command changes a value back in the same batch.
func semanticEffects(before, after *State) []Effect {
	var out []Effect
	add := func(e Effect, prev, next string) {
		if prev != next {
			e.Before, e.After = prev, next
			out = append(out, e)
		}
	}
	oldItems := map[int]Item{}
	for _, item := range before.Items {
		oldItems[item.ID] = item
	}
	for _, item := range after.Items {
		prev, exists := oldItems[item.ID]
		e := Effect{Kind: "item", Topic: item.Topic, ID: item.ID}
		field := func(name, a, b string) { e.Field = name; add(e, a, b) }
		if !exists {
			field("presence", "", "present")
		}
		field("topic", prev.Topic, item.Topic)
		field("class", prev.Class, item.Class)
		field("title", prev.Title, item.Title)
		field("options", effectList(prev.Options), effectList(item.Options))
		field("recommendation", prev.Recommendation, item.Recommendation)
		field("rationale", prev.Rationale, item.Rationale)
		field("status", prev.Status, item.Status)
		field("answer", prev.Answer, item.Answer)
		field("answer-quote", prev.AnswerQuote, item.AnswerQuote)
		field("links", effectList(prev.Links), effectList(item.Links))
		delete(oldItems, item.ID)
	}
	// Keep removals in board order, never map iteration order.
	for _, item := range before.Items {
		if _, exists := oldItems[item.ID]; exists {
			out = append(out, Effect{Kind: "item", Topic: item.Topic, ID: item.ID, Field: "presence", Before: "present"})
		}
	}
	oldEntries := map[string]bool{}
	for _, d := range before.Topics {
		for _, e := range d.Entries {
			oldEntries[e.ID] = true
		}
	}
	var topics []string
	for _, s := range []*State{before, after} {
		for _, d := range s.Topics {
			if !slices.Contains(topics, d.Name) {
				topics = append(topics, d.Name)
			}
		}
		for _, i := range s.Items {
			if !slices.Contains(topics, i.Topic) {
				topics = append(topics, i.Topic)
			}
		}
	}
	for _, topic := range topics {
		e := Effect{Kind: "topic", Topic: topic}
		field := func(name, a, b string) { e.Field = name; add(e, a, b) }
		present := func(s *State) string {
			if s.topicKnown(topic) {
				return "present"
			}
			return ""
		}
		closed := func(s *State) string {
			if s.topicClosed(topic) {
				return "closed"
			}
			return ""
		}
		// A group's label can precede its first content command. Until the
		// topic exists that label is provisional (pruneLabels drops unused
		// groups), so attribute it when the topic actually becomes present.
		label := func(s *State) string {
			if !s.topicKnown(topic) {
				return topic
			}
			return s.TopicLabel(topic)
		}
		field("presence", present(before), present(after))
		field("label", label(before), label(after))
		field("body", before.TopicBody(topic), after.TopicBody(topic))
		a, _ := baseDoc(before, topic)
		b, _ := baseDoc(after, topic)
		field("tickets", effectList(a.Tickets), effectList(b.Tickets))
		field("closed", closed(before), closed(after))
		field("result", before.CloseResults[topic], after.CloseResults[topic])
		// New immutable entries are identified by their typed IDs. System
		// prose is deliberately excluded; its underlying state is above.
		for _, entry := range b.Entries {
			if entry.ID == "" || oldEntries[entry.ID] {
				continue
			}
			e := Effect{Topic: topic, EntryID: entry.ID, PanelID: entry.PanelID}
			switch entry.Kind {
			case EntryPanel:
				e.Kind, e.Field, e.After = "panel", "payload", entry.Text
				out = append(out, e)
				e.Field, e.After = "recap", entry.Recap
				out = append(out, e)
			case EntryAgent:
				e.Kind, e.Field, e.After = "log", "text", entry.Text
				out = append(out, e)
			}
		}
	}
	// Minted/adopted IDs already appear on item effects. A floor command
	// alone changes the high-water mark without an item to attribute it to.
	if before.Counter != after.Counter && len(after.Items) == len(before.Items) {
		add(Effect{Kind: "setting", Field: "id-floor"}, strconv.Itoa(before.Counter), strconv.Itoa(after.Counter))
	}
	add(Effect{Kind: "setting", Field: "embed-dir"}, before.EmbedDir, after.EmbedDir)
	return out
}
