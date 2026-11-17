package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// The stale-topic nudge: leads rarely close finished topics, so an update
// result reminds them, at most once an hour, when the board has many open
// topics and some have sat untouched with nothing waiting on the user.
const (
	// staleNudgeTopics is how many open (not closed) topics the board needs
	// before the nudge considers it crowded.
	staleNudgeTopics = 5
	// staleNudgeAge is how long a topic's items, content, and stream must go
	// unchanged before it counts as stale.
	staleNudgeAge = 60 * time.Minute
	// staleNudgeNames caps the topic names the nudge lists.
	staleNudgeNames = 5
)

// stampChanges records now as the change time of every item, body, and
// recap the batch changed (Changed == mark, or RecapChanged == mark for a
// recap). An item, body, or recap with no change time, from a store written
// before the field, is stamped too: its age is unknown, so it counts from the
// first update that sees it rather than reading as stale at once.
func (s *State) stampChanges(mark int, now time.Time) {
	at := now.UTC().Format(time.RFC3339)
	for i := range s.Items {
		if s.Items[i].Changed == mark || s.Items[i].ChangedAt == "" {
			s.Items[i].ChangedAt = at
		}
	}
	for i := range s.Topics {
		d := &s.Topics[i]
		if d.Body != "" && (d.Changed == mark || d.ChangedAt == "") {
			d.ChangedAt = at
		}
		if d.Recap != "" && (d.RecapChanged == mark || d.RecapAt == "") {
			d.RecapAt = at
		}
	}
}

// staleReceipt is internal bookkeeping, not a user-visible topic event.
func staleReceipt(e Entry) bool {
	if e.Kind != EntrySystem || e.Text != "Stale topic nudge sent" {
		return false
	}
	var receipt struct {
		StaleRevision string `json:"stale_revision"`
	}
	return json.Unmarshal(e.BoardNotice, &receipt) == nil && receipt.StaleRevision != ""
}

// staleNudge returns the nudge line for the board at now and records it as
// the last nudge, or returns "" and records nothing. A topic is stale when it
// is open, none of its items is open (an assumed item reads as settled), and
// neither its items, content, legacy recap, nor stream changed for
// staleNudgeAge; the stalest are listed first.
func (s *State) staleNudge(now time.Time) string {
	revision := s.boardRevision()
	lastID := 0
	lastRevision := ""
	for _, topic := range s.Topics {
		for _, entry := range topic.Entries {
			var receipt struct {
				StaleRevision string `json:"stale_revision"`
			}
			id := folderNumber(entry.ID, "e")
			if id > lastID && json.Unmarshal(entry.BoardNotice, &receipt) == nil && receipt.StaleRevision != "" {
				lastID, lastRevision = id, receipt.StaleRevision
			}
		}
	}
	if lastRevision == revision {
		return ""
	}
	var order []string
	latest := map[string]time.Time{}
	waiting := map[string]bool{}
	touch := func(topic, at string) {
		changed, _ := time.Parse(time.RFC3339, at)
		prev, seen := latest[topic]
		if !seen {
			order = append(order, topic)
		}
		if !seen || changed.After(prev) {
			latest[topic] = changed
		}
	}
	for _, item := range s.Items {
		touch(item.Topic, item.ChangedAt)
		if item.Status == StatusOpen {
			waiting[item.Topic] = true
		}
	}
	for _, doc := range s.Topics {
		touch(doc.Name, doc.ChangedAt)
		if doc.RecapAt != "" {
			touch(doc.Name, doc.RecapAt)
		}
		for _, entry := range doc.Entries {
			// Ordinary system events and receipts are not lead activity. Explicit
			// state/dependency records, however, are authored board updates.
			if entry.Kind != EntrySystem || len(entry.TopicState) != 0 || len(entry.WaitingOn) != 0 {
				touch(doc.Name, entry.At)
			}
		}
	}
	open := 0
	var stale []string
	for _, topic := range order {
		if s.topicClosed(topic) {
			continue
		}
		open++
		if !waiting[topic] && now.Sub(latest[topic]) >= staleNudgeAge {
			stale = append(stale, topic)
		}
	}
	if open < staleNudgeTopics || len(stale) == 0 {
		return ""
	}
	sort.SliceStable(stale, func(i, j int) bool { return latest[stale[i]].Before(latest[stale[j]]) })
	names := strings.Join(stale[:min(len(stale), staleNudgeNames)], ", ")
	if len(stale) > staleNudgeNames {
		names += ", ..."
	}
	s.LastNudgeAt = now.UTC().Format(time.RFC3339) // older daemons retain their debounce
	receipt, _ := json.Marshal(struct {
		StaleRevision string `json:"stale_revision"`
	}{revision})
	s.appendEntry(stale[0], Entry{Kind: EntrySystem, Text: "Stale topic nudge sent", BoardNotice: receipt}, now)
	return fmt.Sprintf("%d of %d open topics unchanged for %d+ min (%s); if finished, close them with wb:close and a one-line result.",
		len(stale), open, int(staleNudgeAge/time.Minute), names)
}

// transitionNudge reminds the lead, per topic, when this batch moved it out
// of implementing into awaiting-confirmation or waiting and it is left with
// no open ask of any class: finished work should be closed, and work that
// needs the user should carry a check or question. before is the fold of
// topic states ahead of the batch. The line is result text only; nothing is
// stored.
func (s *State) transitionNudge(before map[string]TopicStateView) string {
	after := s.TopicStates()
	var topics []string
	for topic, view := range after {
		prior := before[topic]
		if view.State == nil || prior.State == nil || prior.State.Value != StateImplementing ||
			view.State.Entry == prior.State.Entry || s.topicClosed(topic) {
			continue
		}
		if view.State.Value != StateAwaitingConfirmation && view.State.Value != StateWaiting {
			continue
		}
		if !s.topicHasOpenItem(topic) {
			topics = append(topics, topic)
		}
	}
	sort.Strings(topics)
	lines := make([]string, len(topics))
	for i, topic := range topics {
		lines[i] = fmt.Sprintf("topic %s: moved to %s with no open ask — if the work is finished, close it (wb:close); if something needs the user's confirmation, add a check or question.",
			s.TopicLabel(topic), after[topic].State.Value)
	}
	return strings.Join(lines, "\n")
}

func (s *State) topicHasOpenItem(topic string) bool {
	for _, item := range s.Items {
		if item.Topic == topic && item.Status == StatusOpen {
			return true
		}
	}
	return false
}
