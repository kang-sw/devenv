package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Topic identities, creation sites and touches belong to one transaction,
// never the persisted board or the command-transition Effects contract.
type topicUpdate struct {
	key, base string
	creation  int
	explicit  bool
	// Last semantic writer per item field, so no-op or net-cancelled
	// commands cannot claim an applied kind through another command's diff.
	itemKinds map[int]map[string]string
}

type topicUpdates struct {
	byKey map[string]*topicUpdate
	order []*topicUpdate
}

func newTopicUpdates(s *State) *topicUpdates {
	u := &topicUpdates{byKey: map[string]*topicUpdate{}}
	for _, doc := range s.Topics {
		u.byKey[doc.Name] = &topicUpdate{key: doc.Name, base: doc.Name, creation: -1}
	}
	for _, item := range s.Items {
		u.byKey[item.Topic] = &topicUpdate{key: item.Topic, base: item.Topic, creation: -1}
	}
	return u
}

func (u *topicUpdates) touch(key string) *topicUpdate {
	t := u.byKey[key]
	if t == nil {
		t = &topicUpdate{key: key, creation: -1}
		u.byKey[key] = t
	}
	if !slices.Contains(u.order, t) {
		u.order = append(u.order, t)
	}
	return t
}

func (u *topicUpdates) group(before, after *State, cmd command, key string) {
	if key == "" {
		return
	}
	// Only adoption moves a known key. A new label on an already stemmed
	// topic changes its display name, not its identity or its creation status.
	label, labelled := cmd.Scope.Extra("label")
	if labelled && !before.topicKnown(key) && !before.stemmed(label) && before.topicKnown(label) && after.topicKnown(key) {
		t := u.byKey[label]
		delete(u.byKey, label)
		// An earlier provisional touch of the destination must keep its place.
		if provisional := u.byKey[key]; provisional != nil {
			for i, touched := range u.order {
				if touched == provisional {
					u.order[i] = t
				}
			}
		}
		t.key = key
		u.byKey[key] = t
	}
	u.touch(key)
}

func (u *topicUpdates) touchCommand(s *State, cmd command, topicOf topicResolver) []string {
	var topics []string
	add := func(key string) {
		u.touch(key)
		if !slices.Contains(topics, key) {
			topics = append(topics, key)
		}
	}
	if key, ok := topicOf(cmd); ok {
		add(key)
	} else if cmd.Tag == "wb:answer" || cmd.Tag == "wb:patch" {
		var ids []int
		if cmd.Tag == "wb:answer" {
			ids = answerIDs(&problems{}, cmd)
		} else {
			raw, _ := cmd.Attr("id")
			id, _ := strconv.Atoi(raw)
			ids = []int{id}
		}
		for _, id := range ids {
			if i := s.index(id); i >= 0 {
				key := s.Items[i].Topic
				add(key)

			}
		}
	}
	return topics
}

func (u *topicUpdates) recordKinds(cmd command, effects []Effect) {
	for _, e := range effects {
		if e.Kind != "item" {
			continue
		}
		t := u.byKey[e.Topic]
		if t.itemKinds == nil {
			t.itemKinds = map[int]map[string]string{}
		}
		if t.itemKinds[e.ID] == nil {
			t.itemKinds[e.ID] = map[string]string{}
		}
		t.itemKinds[e.ID][e.Field] = cmd.Tag
	}
}

func (u *topicUpdates) created(before, after *State, key string, index int, assign bool, now time.Time, p *problems) {
	if key == "" || before.topicKnown(key) || !after.topicKnown(key) {
		return
	}
	t := u.touch(key)
	// Adoption already carries the prior identity, even though its new key
	// was absent before this command. Reopening likewise is never creation.
	if t.base != "" || t.creation >= 0 {
		return
	}
	t.creation = index
	if assign {
		if _, ok := stateTimestamp(now); !ok {
			p.Add("topic %q: server time is outside UTC RFC3339Nano bounds", key)
			return
		}
		r := topicStateRecord{V: 1, Op: "set", Value: StateDiscussing}
		after.appendStateEvent(key, r, "Topic state: "+stateTransitionText(r), now)
	}
}

func (u *topicUpdates) defaults() map[int]bool {
	out := map[int]bool{}
	for _, t := range u.order {
		if t.creation >= 0 && !t.explicit {
			out[t.creation] = true
		}
	}
	return out
}

// Ignore authority entry IDs for net comparison: an exit and return to the
// same non-running state is not a semantic change. Run identity is meaningful.
func summaryState(view TopicStateView) string {
	value := struct {
		Value      string
		Run        *TopicRun
		Diagnostic *StateDiagnostic
	}{Diagnostic: view.Diagnostic}
	if view.State != nil {
		value.Value, value.Run = view.State.Value, view.State.Run
	}
	data, _ := json.Marshal(value)
	return string(data)
}

func (u *topicUpdates) summaries(before, after *State) []string {
	if len(u.order) == 0 {
		return nil
	}
	// Provisional destination and source touches can converge on one adopted
	// identity. Deduplicate before alignment: renameTopic clears stale target
	// lifecycle state, so applying the same move twice would erase its base.
	var ordered []*topicUpdate
	seen := map[*topicUpdate]bool{}
	for _, t := range u.order {
		if !seen[t] {
			seen[t] = true
			ordered = append(ordered, t)
		}
	}
	// Align adopted identities in the comparison copy only. Otherwise every
	// moved item/body/panel looks newly added and the old topic looks removed.
	base := before.clone()
	for _, t := range ordered {
		if t.base != "" && t.base != t.key {
			base.renameTopic(t.base, t.key)
		}
	}
	effects := semanticEffects(base, after)
	oldStates, states := base.TopicStates(), after.TopicStates()
	oldWaiting, waiting := base.waitingDeclarations(), after.waitingDeclarations()
	var out []string
	for _, t := range ordered {
		if t.base == "" && after.topicKnown(t.key) {
			value, source := "none", "default"
			if state := states[t.key].State; state != nil {
				value = state.Value
			}
			if t.explicit {
				source = "explicit"
			}
			tags := "state: " + value + ", " + source
			if after.topicClosed(t.key) {
				tags += ", closed"
			}
			out = append(out, "topic added: "+t.key+" ("+tags+")")
			continue
		}
		var net []Effect
		for _, e := range effects {
			if e.Topic == t.key {
				net = append(net, e)
			}
		}
		stateChanged := summaryState(oldStates[t.key]) != summaryState(states[t.key])
		waitingChanged := !slices.Equal(oldWaiting[t.key].Targets, waiting[t.key].Targets)
		renamed := t.base != "" && (t.base != t.key || before.TopicLabel(t.base) != after.TopicLabel(t.key))
		adopted := t.base != "" && !before.stemmed(t.base) && after.stemmed(t.key)
		if len(net) == 0 && !stateChanged && !waitingChanged && !renamed && !adopted {
			continue
		}
		tags := summaryTags(t, net, base, after)
		if waitingChanged {
			tags = append(tags, "waiting on")
		}
		if stateChanged && !after.topicClosed(t.key) {
			old, next := oldStates[t.key].State, states[t.key].State
			if old != nil && next != nil && old.Value == StateImplementing && next.Value == StateImplementing && old.Run.ID == next.Run.ID {
				tags = append(tags, fmt.Sprintf("eta %dm", next.Run.ETAMinutes))
			} else {
				value := "none"
				if next != nil {
					value = next.Value
				}
				tags = append(tags, "state: "+value)
			}
		}
		if renamed {
			tags = append(tags, "renamed from "+before.TopicLabel(t.base))
		} else if adopted {
			tags = append(tags, "adopted")
		}
		line := "topic updated: " + t.key
		if len(tags) > 0 {
			line += " (" + strings.Join(tags, ", ") + ")"
		}
		out = append(out, line)
	}
	return out
}

func summaryTags(t *topicUpdate, effects []Effect, before, after *State) []string {
	added, answered, patched := map[int]bool{}, map[int]bool{}, map[int]bool{}
	panels, logs := map[string]bool{}, map[string]bool{}
	body, tickets, closed, reopened, result := false, false, false, false, false
	for _, e := range effects {
		switch e.Kind {
		case "item":
			if e.Field == "presence" && e.After == "present" {
				added[e.ID] = true
			}
			kind := t.itemKinds[e.ID][e.Field]
			if kind == "wb:answer" && (e.Field == "status" || e.Field == "answer" || e.Field == "answer-quote") {
				if i := after.index(e.ID); i >= 0 && (after.Items[i].Status == StatusAnswered || after.Items[i].Status == StatusAssumed) {
					answered[e.ID] = true
				}
			}
			if kind == "wb:patch" {
				patched[e.ID] = true
			}
		case "panel":
			panels[e.EntryID] = true
		case "log":
			logs[e.EntryID] = true
		case "topic":
			switch e.Field {
			case "body":
				body = true
			case "tickets":
				tickets = true
			case "closed":
				closed, reopened = e.After == "closed", e.After == ""
			case "result":
				result = true
			}
		}
	}
	var tags []string
	for _, count := range []struct {
		name string
		n    int
	}{{"added", len(added)}, {"answered", len(answered)}, {"patched", len(patched)}, {"panels", len(panels)}, {"logged", len(logs)}} {
		if count.n > 0 {
			tags = append(tags, fmt.Sprintf("%s %d", count.name, count.n))
		}
	}
	if body {
		tags = append(tags, "body")
	}
	if tickets {
		tags = append(tags, "tickets")
	}
	if closed {
		tags = append(tags, "closed")
	} else if reopened {
		tags = append(tags, "reopened")
	} else if result && before.topicClosed(t.key) {
		tags = append(tags, "result")
	}
	return tags
}
