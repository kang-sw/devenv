package wswhiteboard

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

var waitingTicketPattern = regexp.MustCompile(`^\d{6}-[A-Za-z0-9_]+-[A-Za-z0-9_-]+$`)
var waitingExecPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)

type waitingOnRecord struct {
	V       int      `json:"v"`
	Targets []string `json:"targets"`
}

type waitingDeclaration struct {
	Entry   string
	Targets []string
}

// DependencyView is derived from a declaration and current board/ticket state.
// Topic is the board-local key; Anchor is supplied only for a loaded topic.
type DependencyView struct {
	Token, Label, Topic, Anchor, Status string
	Resolved, Derived                   bool
}

type WaitingView struct {
	Declaration string
	Targets     []DependencyView
	Ready       bool
}

func waitingToken(token string) bool {
	if token == "owner" {
		return true
	}
	kind, name, ok := strings.Cut(token, ":")
	if !ok || name == "" {
		return false
	}
	switch kind {
	case "topic":
		return validStateText(name, len(name)) && len(strings.Fields(name)) == 1
	case "ticket":
		return waitingTicketPattern.MatchString(name)
	case "exec":
		return waitingExecPattern.MatchString(name)
	}
	return false
}

func normalizeWaitingOn(p *problems, work *State, cmd command, topic, raw string) []string {
	var targets []string
	for _, token := range strings.Fields(raw) {
		if !waitingToken(token) {
			p.Add("%s: invalid waiting target %q (topic:<name>, ticket:<stem>, owner, exec:<label>)", cmd.At("on"), token)
			continue
		}
		if name, ok := strings.CutPrefix(token, "topic:"); ok {
			key := work.resolveTopic(name)
			if !work.topicKnown(key) || key == topic {
				p.Add("%s: topic target %q must be known and different from the waiting topic", cmd.At("on"), name)
				continue
			}
			token = "topic:" + key
		}
		if !slices.Contains(targets, token) {
			targets = append(targets, token)
		}
	}
	if len(targets) > 8 {
		p.Add("%s: at most 8 distinct waiting targets", cmd.At("on"))
	}
	return targets
}

// Fold alongside lifecycle authority, not by text or timestamps. Old closes,
// diagnostics and leaving waiting clear the list; dependency-only entries do not
// modify TopicState or its hourly eligibility epoch.
func (s *State) waitingDeclarations() map[string]waitingDeclaration {
	out := map[string]waitingDeclaration{}
	states := map[string]TopicStateView{}
	for _, event := range s.stateEvents() {
		e := event.entry
		foldStateEvent(states, event)
		st := states[event.topic].State
		if st == nil || st.Value != StateWaiting {
			delete(out, event.topic)
			continue
		}
		if len(e.TopicState) > 0 {
			delete(out, event.topic)
		}
		if len(e.WaitingOn) == 0 {
			continue
		}
		delete(out, event.topic) // malformed newest authority must not revive an old list
		var r waitingOnRecord
		if json.Unmarshal(e.WaitingOn, &r) != nil || r.V != 1 || r.Targets == nil || len(r.Targets) > 8 {
			continue
		}
		valid := true
		for i, token := range r.Targets {
			if !waitingToken(token) || slices.Contains(r.Targets[:i], token) {
				valid = false
				break
			}
		}
		if valid {
			out[event.topic] = waitingDeclaration{e.ID, r.Targets}
		}
	}
	return out
}

func waitingPayload(targets []string) json.RawMessage {
	if targets == nil {
		targets = []string{}
	}
	data, _ := json.Marshal(waitingOnRecord{1, targets})
	return data
}

func (s *State) openOwnerAsks() map[string]int {
	out := map[string]int{}
	for _, item := range s.Items {
		if item.Status == StatusOpen && (item.Class == ClassPolicy || item.Class == ClassCheck) {
			out[item.Topic]++
		}
	}
	return out
}

// WaitingTopics is shared by rendering and daemon reconciliation so readiness
// cannot depend on whether a page is open. Ticket reads happen only for declared
// ticket targets, once per distinct stem per evaluation.
func (s *State) WaitingTopics(tickets TicketResolver) map[string]WaitingView {
	out := map[string]WaitingView{}
	declarations := s.waitingDeclarations()
	asks := s.openOwnerAsks()
	seen := map[string]string{}
	for topic, v := range s.TopicStates() {
		if s.topicClosed(topic) || v.State == nil || v.State.Value != StateWaiting {
			continue
		}
		d := declarations[topic]
		w := WaitingView{Declaration: d.Entry, Ready: len(d.Targets) > 0}
		owner := false
		for _, token := range d.Targets {
			dep := DependencyView{Token: token, Label: token}
			switch {
			case strings.HasPrefix(token, "topic:"):
				dep.Topic = s.resolveTopic(strings.TrimPrefix(token, "topic:"))
				dep.Label = "topic:" + s.TopicLabel(dep.Topic)
				dep.Resolved = s.topicKnown(dep.Topic) && s.topicClosed(dep.Topic)
			case strings.HasPrefix(token, "ticket:"):
				badge := ticketBadges([]string{strings.TrimPrefix(token, "ticket:")}, tickets, seen)[0]
				dep.Status = badge.Status
				dep.Resolved = badge.Status == TicketDone || badge.Status == TicketDropped
			case token == "owner":
				owner = true
				dep.Resolved = asks[topic] == 0
			}
			w.Ready = w.Ready && dep.Resolved
			w.Targets = append(w.Targets, dep)
		}
		if asks[topic] > 0 {
			w.Ready = false
			if !owner {
				w.Targets = append(w.Targets, DependencyView{Token: "owner", Label: "owner", Derived: true})
			}
		}
		out[topic] = w
	}
	return out
}

// Hex encodes arbitrary legacy topic names without CSS/fragment ambiguity or
// collisions with item-N, pN and eN renderer IDs.
func topicAnchor(topic string) string { return "topic-" + hex.EncodeToString([]byte(topic)) }

type unblockedReceipt struct {
	V           int    `json:"v"`
	ReplyID     string `json:"reply_id"`
	Topic       string `json:"topic"`
	Declaration string `json:"declaration"`
}

type unblockedPlan struct {
	topic, message string
	receipt        unblockedReceipt
}

func (s *State) planUnblocked(reply string, dirs map[string]string, tickets TicketResolver) []unblockedPlan {
	sent := map[unblockedReceipt]bool{}
	for _, event := range s.stateEvents() {
		var r unblockedReceipt
		if json.Unmarshal(event.entry.UnblockedReceipt, &r) == nil && r.V == 1 && r.ReplyID == reply && folderDirID(r.Topic) && folderNumber(r.Declaration, "e") > 0 {
			sent[r] = true
		}
	}
	var out []unblockedPlan
	for topic, w := range s.WaitingTopics(tickets) {
		dir := dirs[topic]
		if !w.Ready || w.Declaration == "" || !folderDirID(dir) {
			continue
		}
		r := unblockedReceipt{1, reply, dir, w.Declaration}
		if sent[r] {
			continue
		}
		var targets []string
		for _, dep := range w.Targets {
			targets = append(targets, dep.Token)
		}
		out = append(out, unblockedPlan{topic, fmt.Sprintf("Whiteboard lead nudge (unblocked).\n- %s: ready to proceed; resolved targets: %s\nReconcile the waiting declaration with actual progress; the topic remains waiting until you declare its next state.", s.TopicLabel(topic), strings.Join(targets, ", ")), r})
	}
	slices.SortFunc(out, func(a, b unblockedPlan) int { return strings.Compare(a.receipt.Topic, b.receipt.Topic) })
	return out
}

func (s *State) appendUnblocked(p unblockedPlan, now time.Time) {
	data, _ := json.Marshal(p.receipt)
	s.appendEntry(p.topic, Entry{Kind: EntrySystem, Text: "Unblocked nudge sent", UnblockedReceipt: data}, now)
}
