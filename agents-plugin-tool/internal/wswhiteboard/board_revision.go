package wswhiteboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

// boardRevision fingerprints board authority, not render bookkeeping or daemon
// receipts. Explicit state and waiting-on records are command-authored despite
// their system entry kind; ordinary system entries are not board changes.
func (s *State) boardRevision() string {
	type topic struct {
		Name, Body, Recap     string
		Changed, RecapChanged int
		Tickets               []string
		Entries               []Entry
		RevisionMarkers       []string
	}
	type board struct {
		Items        []Item
		Topics       []topic
		ClosedTopics []string
		CloseResults map[string]string
		Labels       map[string]string
	}
	b := board{Items: append([]Item(nil), s.Items...), ClosedTopics: s.ClosedTopics, CloseResults: s.CloseResults, Labels: s.Labels}
	for i := range b.Items {
		// Legacy age backfills and display-only times do not change authority.
		b.Items[i].ChangedAt = ""
	}
	for _, d := range s.Topics {
		t := topic{Name: d.Name, Body: d.Body, Recap: d.Recap, Changed: d.Changed, RecapChanged: d.RecapChanged, Tickets: d.Tickets}
		for _, e := range d.Entries {
			if boardRevisionMarker(e) {
				t.RevisionMarkers = append(t.RevisionMarkers, e.ID)
			}
			if e.Kind == EntryAgent || e.Kind == EntryPanel || len(e.TopicState) != 0 || len(e.WaitingOn) != 0 ||
				(e.Kind == EntrySystem && strings.HasPrefix(e.Text, dashboardAnswerRevisionPrefix)) {
				// Display provenance can be populated independently of the entry.
				e.Inputs, e.InputsFloor, e.Seq, e.AnswerSentAt = nil, "", 0, 0
				e.BoardNotice, e.UnblockedReceipt = nil, nil
				t.Entries = append(t.Entries, e)
			}
		}
		// A system-only topic is not user activity.
		if t.Body != "" || t.Recap != "" || len(t.Tickets) != 0 || len(t.Entries) != 0 || len(t.RevisionMarkers) != 0 {
			b.Topics = append(b.Topics, t)
		}
	}
	data, _ := json.Marshal(b)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

const revisionMarkerText = "Board revision recorded"

func boardRevisionMarker(e Entry) bool {
	var p struct {
		RevisionMarker bool `json:"revision_marker"`
	}
	return json.Unmarshal(e.BoardNotice, &p) == nil && p.RevisionMarker
}

func internalRevisionMarker(e Entry) bool {
	return e.Kind == EntrySystem && e.Text == revisionMarkerText && boardRevisionMarker(e)
}

// Metadata changes lack the change sequence or immutable authored entry that
// other board changes carry. Give them durable identity, even if reverted.
func (s *State) markMetadataRevision(before *State, effects []Effect, now time.Time) {
	topic := ""
	needed := false
	for _, effect := range effects {
		if effect.Kind == "setting" || effect.Kind == "topic" &&
			(effect.Field == "label" || effect.Field == "tickets" || effect.Field == "closed" || effect.Field == "result") {
			needed = true
			if topic == "" && s.topicDocIndex(effect.Topic) >= 0 {
				topic = effect.Topic
			}
		}
	}
	if !needed || len(s.Topics) == 0 {
		return
	}
	// Bind to a newly appended entry when possible; never alter committed
	// entries or overwrite other receipt metadata.
	marker := json.RawMessage(`{"revision_marker":true}`)
	for i := range s.Topics {
		for j := range s.Topics[i].Entries {
			e := &s.Topics[i].Entries[j]
			if folderNumber(e.ID, "e") > before.EntrySeq && len(e.BoardNotice) == 0 {
				e.BoardNotice = marker
				return
			}
		}
	}
	if topic == "" {
		topic = s.Topics[0].Name
	}
	s.appendEntry(topic, Entry{Kind: EntrySystem, Text: revisionMarkerText, BoardNotice: marker}, now)
}
