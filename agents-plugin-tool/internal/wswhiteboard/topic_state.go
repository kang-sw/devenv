package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

const (
	StateDiscussing           = "discussing"
	StateWaiting              = "waiting"
	StateResearching          = "researching"
	StateImplementing         = "implementing"
	StateAwaitingConfirmation = "awaiting-confirmation"
	maxETAMinutes             = int64(math.MaxInt64 / int64(time.Minute))
)

var topicStateValues = []string{StateDiscussing, StateWaiting, StateResearching, StateImplementing, StateAwaitingConfirmation}

// TopicRun is a derived execution snapshot, never independently persisted.
// DeadlineAt is computed from StartedAt and the total wall-clock budget.
type TopicRun struct {
	ID                string `json:"id"`
	StartedAt         string `json:"started_at"`
	Owner             string `json:"owner"`
	InitialETAMinutes int64  `json:"initial_eta_minutes"`
	ETAMinutes        int64  `json:"eta_minutes"`
	ExtensionCount    int    `json:"extension_count"`
	DeadlineAt        string `json:"deadline_at,omitempty"`
}

type TopicState struct {
	Value string    `json:"value"`
	Entry string    `json:"entry"`
	Run   *TopicRun `json:"run,omitempty"`
	// Research attributes are informational; only Run establishes a clock.
	Owner      string `json:"owner,omitempty"`
	ETAMinutes int64  `json:"eta_minutes,omitempty"`
}

type StateDiagnostic struct {
	Entry string `json:"entry"`
	Code  string `json:"code"`
}

// TopicStateView separates ordinary absence from unknown current authority.
type TopicStateView struct {
	State      *TopicState
	Diagnostic *StateDiagnostic
}

type topicStateRecord struct {
	V          int       `json:"v"`
	Op         string    `json:"op"`
	Value      string    `json:"value,omitempty"`
	From       string    `json:"from,omitempty"`
	Run        *TopicRun `json:"run,omitempty"`
	Owner      string    `json:"owner,omitempty"`
	ETAMinutes int64     `json:"eta_minutes,omitempty"`
	ElapsedMS  *int64    `json:"elapsed_ms,omitempty"`
	Reason     string    `json:"reason,omitempty"`
}

type stateEvent struct {
	topic string
	entry Entry
}

// Numeric board-global order is the authority, including closed histories.
// Recap conversion assigns late IDs to imported entries, not historical epochs.
func (s *State) stateEvents() []stateEvent {
	var events []stateEvent
	if s != nil {
		for _, doc := range s.Topics {
			for _, e := range doc.Entries {
				if e.Kind == EntrySystem && !e.Imported && e.ItemID == 0 {
					events = append(events, stateEvent{doc.Name, e})
				}
			}
		}
	}
	slices.SortFunc(events, func(a, b stateEvent) int {
		return folderNumber(a.entry.ID, "e") - folderNumber(b.entry.ID, "e")
	})
	return events
}

// TopicStates folds immutable producer records. Imported is a convention, not
// authentication: an editor of authoritative local files can forge these records
// or the inspected historical producer's close literal.
func (s *State) TopicStates() map[string]TopicStateView {
	out := map[string]TopicStateView{}
	for _, event := range s.stateEvents() {
		foldStateEvent(out, event)
	}
	return out
}

func foldStateEvent(out map[string]TopicStateView, event stateEvent) {
	e, topic := event.entry, event.topic
	prior := out[topic]
	if len(e.TopicState) == 0 {
		// Bounded adapter for the schema-2 applyClose producer. Authored
		// log/panel/item/answer text cannot choose this kind/attribution.
		if (prior.State != nil || prior.Diagnostic != nil) && strings.HasPrefix(e.Text, "Topic closed: ") {
			out[topic] = TopicStateView{}
		}
		return
	}
	r, code := parseTopicState(e)
	if code != "" {
		out[topic] = TopicStateView{Diagnostic: &StateDiagnostic{Entry: e.ID, Code: code}}
		return
	}
	if r.Op == "reset" {
		out[topic] = TopicStateView{}
		return
	}
	if r.Op == "extend" && !validExtension(prior, r) {
		out[topic] = TopicStateView{Diagnostic: &StateDiagnostic{Entry: e.ID, Code: "malformed"}}
		return
	}
	out[topic] = TopicStateView{State: &TopicState{Value: r.Value, Entry: e.ID, Run: r.Run, Owner: r.Owner, ETAMinutes: r.ETAMinutes}}
	// A completed run belongs to history, not to the new work state.
	if r.Value != StateImplementing {
		out[topic].State.Run = nil
	}
}

func validStateText(value string, maxBytes int) bool {
	return value == strings.TrimSpace(value) && len(value) > 0 && len(value) <= maxBytes && utf8.ValidString(value) && !strings.ContainsFunc(value, unicode.IsControl)
}

func stateTimestamp(at time.Time) (string, bool) {
	at = at.UTC()
	text := at.Format(time.RFC3339Nano)
	parsed, err := time.Parse(time.RFC3339Nano, text)
	return text, err == nil && parsed.Equal(at)
}

func stateDeadline(start time.Time, minutes int64) (string, bool) {
	if minutes < 1 || minutes > maxETAMinutes {
		return "", false
	}
	if _, ok := stateTimestamp(start); !ok {
		return "", false
	}
	deadline := start.Add(time.Duration(minutes) * time.Minute)
	text, ok := stateTimestamp(deadline)
	return text, ok && deadline.After(start)
}

func validRun(run *TopicRun, entry Entry) bool {
	if run == nil || folderNumber(run.ID, "e") == 0 || folderNumber(run.ID, "e") > folderNumber(entry.ID, "e") || !validStateText(run.Owner, 256) || run.InitialETAMinutes < 1 || run.InitialETAMinutes > run.ETAMinutes || run.ExtensionCount < 0 {
		return false
	}
	if run.ExtensionCount == 0 && run.InitialETAMinutes != run.ETAMinutes || run.ExtensionCount > 0 && run.InitialETAMinutes >= run.ETAMinutes {
		return false
	}
	start, err := time.Parse(time.RFC3339Nano, run.StartedAt)
	if err != nil {
		return false
	}
	canonical, ok := stateTimestamp(start)
	if !ok || canonical != run.StartedAt {
		return false
	}
	deadline, ok := stateDeadline(start, run.ETAMinutes)
	if !ok {
		return false
	}
	run.DeadlineAt = deadline
	return true
}

func parseTopicState(e Entry) (topicStateRecord, string) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(e.TopicState, &fields) != nil || fields == nil {
		return topicStateRecord{}, "malformed"
	}
	var version int
	if json.Unmarshal(fields["v"], &version) != nil || version == 0 {
		return topicStateRecord{}, "malformed"
	}
	if version != 1 {
		return topicStateRecord{}, "unsupported"
	}
	var op string
	if json.Unmarshal(fields["op"], &op) != nil || op == "" {
		return topicStateRecord{}, "malformed"
	}
	if op != "set" && op != "reset" && op != "extend" {
		return topicStateRecord{}, "unsupported"
	}
	var r topicStateRecord
	if json.Unmarshal(e.TopicState, &r) != nil || folderNumber(e.ID, "e") == 0 {
		return r, "malformed"
	}
	at, err := time.Parse(time.RFC3339Nano, e.At)
	if err != nil {
		return r, "malformed"
	}
	if _, ok := stateTimestamp(at); !ok {
		return r, "malformed"
	}
	if r.From != "" && !slices.Contains(topicStateValues, r.From) || r.ElapsedMS != nil && *r.ElapsedMS < 0 {
		return r, "malformed"
	}
	// Presence matters: null/empty research attributes are not valid optional
	// values, and no other state or operation owns these top-level fields.
	for _, key := range []string{"owner", "eta_minutes"} {
		if raw, present := fields[key]; present {
			if r.Op != "set" || r.Value != StateResearching || string(raw) == "null" ||
				key == "owner" && !validStateText(r.Owner, 256) ||
				key == "eta_minutes" && (r.ETAMinutes < 1 || r.ETAMinutes > maxETAMinutes) {
				return r, "malformed"
			}
		}
	}
	if r.Run != nil {
		var snapshot map[string]json.RawMessage
		if json.Unmarshal(fields["run"], &snapshot) != nil {
			return r, "malformed"
		}
		for _, key := range []string{"id", "started_at", "owner", "initial_eta_minutes", "eta_minutes", "extension_count"} {
			if raw, present := snapshot[key]; !present || string(raw) == "null" {
				return r, "malformed"
			}
		}
		if !validRun(r.Run, e) {
			return r, "malformed"
		}
	}
	if r.Op == "extend" {
		if r.Value != "" && r.Value != StateImplementing || r.From != "" || r.Run == nil || r.Run.ID == e.ID || r.ElapsedMS != nil || !validStateText(r.Reason, 2048) {
			return r, "malformed"
		}
		r.Value = StateImplementing // extend carries the run, not another state set
		return r, ""
	}
	if r.Reason != "" {
		return r, "malformed"
	}
	if r.Op == "reset" {
		if r.Value != "" || r.Run != nil && (r.From != StateImplementing || r.ElapsedMS == nil) || r.ElapsedMS != nil && r.Run == nil || r.From == StateImplementing && r.Run == nil {
			return r, "malformed"
		}
		return r, ""
	}
	if !slices.Contains(topicStateValues, r.Value) || r.From == r.Value && r.Value != StateResearching {
		return r, "malformed"
	}
	if r.Value == StateImplementing {
		if r.Run == nil || r.Run.ID != e.ID || r.Run.StartedAt != e.At || r.Run.ExtensionCount != 0 || r.ElapsedMS != nil {
			return r, "malformed"
		}
	} else if r.Run != nil && (r.From != StateImplementing || r.ElapsedMS == nil) || r.ElapsedMS != nil && r.Run == nil || r.From == StateImplementing && r.Run == nil {
		return r, "malformed"
	}
	return r, ""
}

func applyTopicState(p *problems, work *State, cmd command, topicOf topicResolver, now time.Time) {
	before := len(p.List)
	topic, topicOK := topicOf(cmd)
	value, valueOK := wsxmlcmd.EnumAttr(p, cmd, "value", topicStateValues)
	owner, hasOwner := cmd.Attr("owner")
	etaText, hasETA := cmd.Attr("eta_minutes")
	var eta int64
	if valueOK && (value == StateImplementing || value == StateResearching) {
		owner = strings.TrimSpace(owner)
		if (value == StateImplementing || hasOwner) && (!hasOwner || !validStateText(owner, 256)) {
			p.Add("%s: required, 1-256 UTF-8 bytes after trimming, without control characters", cmd.At("owner"))
		}
		var err error
		eta, err = strconv.ParseInt(etaText, 10, 64)
		if (value == StateImplementing || hasETA) && (!hasETA || etaText == "" || strings.ContainsFunc(etaText, func(r rune) bool { return r < '0' || r > '9' }) || err != nil || eta < 1 || eta > maxETAMinutes) {
			p.Add("%s: required positive decimal integer, at most %d", cmd.At("eta_minutes"), maxETAMinutes)
		}
	} else if hasOwner || hasETA {
		p.Add("%s: owner and eta_minutes apply only to implementing or researching", cmd.Label())
	}
	on, hasOn := cmd.Attr("on")
	var targets []string
	if hasOn {
		if value != StateWaiting {
			p.Add("%s: on applies only to waiting", cmd.At("on"))
		} else if topicOK {
			targets = normalizeWaitingOn(p, work, cmd, topic, on)
		}
	}
	if !topicOK || !valueOK {
		return
	}
	if !work.topicKnown(topic) || work.topicClosed(topic) {
		p.Add("%s: state requires a known open topic %q; accepted content or item activity must create/reopen it first", cmd.Label(), topic)
		return
	}
	prior := work.TopicStates()[topic]
	if prior.Diagnostic != nil && prior.Diagnostic.Code == "unsupported" {
		p.Add("%s: topic %q has unsupported state metadata at %s; state mutation requires supported code", cmd.Label(), topic, prior.Diagnostic.Entry)
		return
	}
	if len(p.List) != before {
		return
	}
	if prior.State != nil && prior.State.Value == value {
		if value == StateImplementing && (prior.State.Run.Owner != owner || prior.State.Run.ETAMinutes != eta) {
			p.Add("%s: implementing owner/ETA differs; change ETA with wb:eta, or explicitly exit/re-enter to replace the executor", cmd.Label())
		}
		if value == StateWaiting && !slices.Equal(work.waitingDeclarations()[topic].Targets, targets) {
			if _, ok := stateTimestamp(now); !ok {
				p.Add("%s: server time is outside UTC RFC3339Nano bounds", cmd.Label())
				return
			}
			work.appendEntry(topic, Entry{Kind: EntrySystem, Text: "waiting on changed: " + strings.Join(targets, " "), WaitingOn: waitingPayload(targets)}, now)
		}
		if value != StateResearching || prior.State.Owner == owner && prior.State.ETAMinutes == eta {
			return
		}
	}
	if _, ok := stateTimestamp(now); !ok {
		p.Add("%s: server time is outside UTC RFC3339Nano bounds", cmd.Label())
		return
	}
	r := topicStateRecord{V: 1, Op: "set", Value: value}
	if value == StateResearching {
		r.Owner, r.ETAMinutes = owner, eta
	}
	if prior.State != nil {
		r.From = prior.State.Value
		if prior.State.Run != nil {
			r.Run = prior.State.Run
			elapsed := runElapsed(r.Run, now)
			r.ElapsedMS = &elapsed
		}
	}
	if value == StateImplementing {
		if _, ok := stateDeadline(now, eta); !ok {
			p.Add("%s: implementing start/deadline is outside UTC RFC3339Nano bounds", cmd.Label())
			return
		}
		start, _ := stateTimestamp(now)
		r.Run = &TopicRun{ID: fmt.Sprintf("e%d", work.EntrySeq+1), StartedAt: start, Owner: owner, InitialETAMinutes: eta, ETAMinutes: eta}
	}
	work.appendStateEvent(topic, r, "Topic state: "+stateTransitionText(r), now)
	if value == StateWaiting && len(targets) > 0 {
		doc := work.topicDoc(topic)
		doc.Entries[len(doc.Entries)-1].WaitingOn = waitingPayload(targets)
	}
}

func runElapsed(run *TopicRun, now time.Time) int64 {
	start, _ := time.Parse(time.RFC3339Nano, run.StartedAt)
	return max(int64(0), now.UTC().UnixMilli()-start.UnixMilli())
}

func stateTransitionText(r topicStateRecord) string {
	text := r.Value
	if r.Op == "reset" {
		text = "cleared"
	}
	if r.From != "" {
		text = r.From + " -> " + text
	}
	if r.Owner != "" {
		text += "; research owner " + r.Owner
	}
	if r.ETAMinutes != 0 {
		text += fmt.Sprintf("; research estimate %d minutes (informational)", r.ETAMinutes)
	}
	if r.Run != nil {
		text += fmt.Sprintf("; owner %s; estimate %d minutes (initial %d)", r.Run.Owner, r.Run.ETAMinutes, r.Run.InitialETAMinutes)
		if r.ElapsedMS != nil {
			text += fmt.Sprintf("; actual %d ms", *r.ElapsedMS)
		}
	}
	return text
}

func (s *State) appendStateEvent(topic string, r topicStateRecord, text string, now time.Time) {
	if r.Run != nil {
		run := *r.Run
		run.DeadlineAt = "" // derived projection is never persisted as authority
		r.Run = &run
	}
	data, err := json.Marshal(r)
	if err != nil {
		panic(err)
	}
	s.appendEntry(topic, Entry{Kind: EntrySystem, Text: text, TopicState: data}, now)
}

func (s *State) closeStateEvent(topic string, now time.Time) {
	r := topicStateRecord{V: 1, Op: "reset"}
	prior := s.TopicStates()[topic]
	if prior.State != nil {
		r.From, r.Run = prior.State.Value, prior.State.Run
		if r.Run != nil {
			elapsed := runElapsed(r.Run, now)
			r.ElapsedMS = &elapsed
		}
	}
	text := "Topic closed."
	if r.Run != nil {
		text += "; " + stateTransitionText(r)
	}
	s.appendStateEvent(topic, r, text, now)
}

func (s *State) structuralWarnings() string {
	states := s.TopicStates()
	var discussing, waiting []string
	implementing := false
	for topic, view := range states {
		if s.topicClosed(topic) || view.State == nil {
			continue
		}
		switch view.State.Value {
		case StateImplementing:
			implementing = true
		case StateWaiting:
			waiting = append(waiting, topic)
		case StateDiscussing:
			open := false
			for _, item := range s.Items {
				if item.Topic == topic && item.Status == StatusOpen {
					open = true
					break
				}
			}
			if !open {
				discussing = append(discussing, topic)
			}
		}
	}
	names := func(topics []string) string {
		sort.Strings(topics)
		for i, topic := range topics {
			topics[i] = fmt.Sprintf("%q", s.TopicLabel(topic))
		}
		return strings.Join(topics, ", ")
	}
	var warnings []string
	if len(discussing) > 0 {
		warnings = append(warnings, "discussing_without_open_items: "+names(discussing))
	}
	if len(waiting) > 0 && !implementing {
		warnings = append(warnings, "waiting_without_implementing: "+names(waiting))
	}
	return strings.Join(warnings, "\n")
}
