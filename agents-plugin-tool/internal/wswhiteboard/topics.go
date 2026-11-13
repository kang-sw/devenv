package wswhiteboard

import (
	"regexp"
	"slices"
	"strings"

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

// A topic is keyed by one string: items carry it (Item.Topic), and so do
// its content and entries (TopicDoc.Name), its close state, and its close result. A
// stemmed topic's key is a short stem the lead chose, and its label, the
// name the page shows, lives in State.Labels; a stemless topic, such as
// every topic of a store written before stems, is keyed by its label.
// Stems and labels share one namespace of topic names, so every name
// resolves to at most one topic (resolveTopic).

// stemPattern is what a topic stem looks like.
var stemPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// topicResolver resolves a scoped command's group to its topic key, false
// when the group carries no usable name (the problem is recorded already).
type topicResolver func(cmd command) (string, bool)

// stemmed reports whether key is a stemmed topic's stem.
func (s *State) stemmed(key string) bool {
	_, ok := s.Labels[key]
	return ok
}

// TopicLabel is the name a topic shows: its label when stemmed, else its
// key.
func (s *State) TopicLabel(key string) string {
	if s != nil {
		if label, ok := s.Labels[key]; ok {
			return label
		}
	}
	return key
}

// labelOwner returns the stem whose label is name, "" when none.
func (s *State) labelOwner(name string) string {
	for stem, label := range s.Labels {
		if label == name {
			return stem
		}
	}
	return ""
}

// resolveTopic resolves a topic name, from a wb:topic group or a query's
// topic filter, to its key: a stem first, then a stemless topic's key, then
// a stemmed topic's label, so a reference that still uses an adopted
// topic's old title keeps working. A name matching none is returned as is:
// a new stemless topic, or no topic at all for a query.
func (s *State) resolveTopic(name string) string {
	if s.stemmed(name) || s.topicKnown(name) {
		return name
	}
	if stem := s.labelOwner(name); stem != "" {
		return stem
	}
	return name
}

// nameTaken reports whether name is already another topic's name: a stem
// or a label of a topic other than self, or a stemless key other than self.
func (s *State) nameTaken(name, self string) bool {
	for stem, label := range s.Labels {
		if stem != self && (stem == name || label == name) {
			return true
		}
	}
	return name != self && !s.stemmed(name) && s.topicKnown(name)
}

// ticketStemPattern is what a linked ticket stem looks like: the ticket
// store's YYMMDD-category-name.
var ticketStemPattern = regexp.MustCompile(`^\d{6}-[\w-]+$`)

// applyGroup applies a wb:topic group that carries an extra attribute (label
// or tickets) and returns the key its commands act on, "" when it was
// refused. A label applies first (applyLabel); tickets then replace the
// topic's linked stems, a space-separated list that tickets="" clears. Like a
// label they are metadata: no recency change, no reopen, no event. A group
// with tickets and no command must name a known topic unless it links at
// least one stem, which makes a new topic persist.
func applyGroup(p *problems, work *State, cmd command) string {
	scope := cmd.Scope
	raw, hasTickets := scope.Extra("tickets")
	var stems []string
	ticketsOK := true
	if hasTickets {
		stems, ticketsOK = ticketStems(p, cmd.At("tickets"), raw)
	}
	key := ""
	if _, labelled := scope.Extra("label"); labelled {
		key = applyLabel(p, work, cmd, len(stems) > 0)
	} else if scope.Value != "" {
		key = work.resolveTopic(scope.Value)
		if scope.Commands == 0 && len(stems) == 0 && !work.topicKnown(key) {
			p.Add("%s: holds no commands, and %q names no topic to clear tickets on", cmd.Label(), scope.Value)
			return ""
		}
	}
	if key != "" && hasTickets && ticketsOK {
		work.setTopicTickets(key, stems)
	}
	return key
}

// ticketStems reads a tickets attribute: space-separated ticket stems, each
// once (a repeat is dropped), "" for none.
func ticketStems(p *problems, at, raw string) ([]string, bool) {
	var stems []string
	ok := true
	for _, stem := range strings.Fields(raw) {
		if !ticketStemPattern.MatchString(stem) {
			p.Add("%s: %q is not a ticket stem (YYMMDD-category-name)", at, stem)
			ok = false
			continue
		}
		if !slices.Contains(stems, stem) {
			stems = append(stems, stem)
		}
	}
	return stems, ok
}

// setTopicTickets replaces topic's linked ticket stems; none clears them,
// dropping a topic document left with nothing else.
func (s *State) setTopicTickets(topic string, stems []string) {
	if len(stems) > 0 {
		s.topicDoc(topic).Tickets = stems
		return
	}
	if i := s.topicDocIndex(topic); i >= 0 {
		s.Topics[i].Tickets = nil
		s.dropTopicDoc(topic)
	}
}

// applyLabel applies a wb:topic group that carries a label and returns the
// key its commands act on, "" when it was refused. The group's name must be
// a stem. A known stem takes the label (a rename); a stemless topic keyed
// by the name takes it as its stem (adoption in place); an unknown stem
// adopts the stemless topic keyed by the label, moving everything keyed by
// it to the stem, or else starts a new stemmed topic, which an empty group
// cannot do since a topic with nothing in it does not persist, unless the
// group links tickets (linking).
func applyLabel(p *problems, work *State, cmd command, linking bool) string {
	scope := cmd.Scope
	name := scope.Value
	label, _ := scope.Extra("label")
	at := cmd.Label() + " label"
	labelOK := wsxmlcmd.NonEmpty(p, at, label)
	if name == "" || !labelOK {
		return ""
	}
	if !stemPattern.MatchString(name) {
		p.Add("%s name: with label, name is a stem (lowercase letters, digits, and hyphens, at most 40, starting with a letter or digit); got %q", cmd.Label(), name)
		return ""
	}
	taken := func(self string) bool {
		if work.nameTaken(label, self) {
			p.Add("%s: %q is already another topic's name", at, label)
			return true
		}
		return false
	}
	switch {
	case work.stemmed(name), work.topicKnown(name):
		if taken(name) {
			return ""
		}
	default:
		if stem := work.labelOwner(name); stem != "" {
			p.Add("%s name: %q is topic %s's label", cmd.Label(), name, stem)
			return ""
		}
		if !work.stemmed(label) && work.topicKnown(label) {
			work.renameTopic(label, name)
			break
		}
		if taken(name) {
			return ""
		}
		if scope.Commands == 0 && !linking {
			p.Add("%s: holds no commands, and %q names no topic to rename or adopt; a new topic needs a command", cmd.Label(), name)
			return ""
		}
	}
	if work.Labels == nil {
		work.Labels = map[string]string{}
	}
	work.Labels[name] = label
	return name
}

// renameTopic moves everything keyed by from to the key to: items, content,
// entries, linked tickets, legacy recap, close state, and close result. A close mark or result already under to is
// stale (archive eviction drops a topic's items but not its close state), so
// it goes first rather than closing or doubling the moved topic.
func (s *State) renameTopic(from, to string) {
	s.reopenTopic(to)
	delete(s.CloseResults, to)
	for i := range s.Items {
		if s.Items[i].Topic == from {
			s.Items[i].Topic = to
		}
	}
	if i := s.topicDocIndex(from); i >= 0 {
		s.Topics[i].Name = to
	}
	for i, closed := range s.ClosedTopics {
		if closed == from {
			s.ClosedTopics[i] = to
		}
	}
	if result, ok := s.CloseResults[from]; ok {
		delete(s.CloseResults, from)
		s.CloseResults[to] = result
	}
}

// pruneLabels drops the label of every stem whose topic is gone (no item,
// content, entries, or legacy recap left), so a stale stem never blocks a new topic.
func (s *State) pruneLabels() {
	for stem := range s.Labels {
		if !s.topicKnown(stem) {
			delete(s.Labels, stem)
		}
	}
	if len(s.Labels) == 0 {
		s.Labels = nil
	}
}
