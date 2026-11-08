package wswhiteboard

import (
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
	// staleNudgeAge is how long a topic's items and body must go unchanged
	// before it counts as stale.
	staleNudgeAge = 60 * time.Minute
	// staleNudgeInterval is the least time between two nudges on one board.
	staleNudgeInterval = 60 * time.Minute
	// staleNudgeNames caps the topic names the nudge lists.
	staleNudgeNames = 5
)

// stampChanges records now as the change time of every item and body the
// batch changed (Changed == mark). An item or body with no change time, from
// a store written before the field, is stamped too: its age is unknown, so it
// counts from the first update that sees it rather than reading as stale at
// once.
func (s *State) stampChanges(mark int, now time.Time) {
	at := now.UTC().Format(time.RFC3339)
	for i := range s.Items {
		if s.Items[i].Changed == mark || s.Items[i].ChangedAt == "" {
			s.Items[i].ChangedAt = at
		}
	}
	for i := range s.Topics {
		if s.Topics[i].Changed == mark || s.Topics[i].ChangedAt == "" {
			s.Topics[i].ChangedAt = at
		}
	}
}

// staleNudge returns the nudge line for the board at now and records it as
// the last nudge, or returns "" and records nothing. A topic is stale when it
// is open, none of its items is open or assumed, and neither its items nor its
// body changed for staleNudgeAge; the stalest are listed first.
func (s *State) staleNudge(now time.Time) string {
	if last, err := time.Parse(time.RFC3339, s.LastNudgeAt); err == nil && now.Sub(last) < staleNudgeInterval {
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
		if item.Status == StatusOpen || item.Status == StatusAssumed {
			waiting[item.Topic] = true
		}
	}
	for _, doc := range s.Topics {
		touch(doc.Name, doc.ChangedAt)
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
	s.LastNudgeAt = now.UTC().Format(time.RFC3339)
	return fmt.Sprintf("%d of %d open topics unchanged for %d+ min (%s); if finished, close them with wb:close and a one-line result.",
		len(stale), open, int(staleNudgeAge/time.Minute), names)
}
