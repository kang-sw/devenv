package wswhiteboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
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
		Items            []Item
		Topics           []topic
		ClosedTopics     []string
		CloseResults     map[string]string
		Labels           map[string]string
		MetadataRevision uint64 `json:",omitempty"`
	}
	b := board{Items: append([]Item(nil), s.Items...), ClosedTopics: s.ClosedTopics, CloseResults: s.CloseResults, Labels: s.Labels, MetadataRevision: s.MetadataRevision}
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
	return e.Kind == EntrySystem && (e.Text == revisionMarkerText && boardRevisionMarker(e) ||
		strings.HasPrefix(e.Text, dashboardAnswerRevisionPrefix))
}

// Metadata changes lack the change sequence or immutable authored entry that
// other board changes carry. Give them durable identity, even if reverted.
func (s *State) markMetadataRevision(effects []Effect) {
	for _, effect := range effects {
		if effect.Kind == "setting" || effect.Kind == "topic" &&
			(effect.Field == "label" || effect.Field == "tickets" || effect.Field == "closed" || effect.Field == "result") {
			s.MetadataRevision++
			return
		}
	}
}
