package wswhiteboard

import (
	"fmt"
	"time"
)

const legacyContentRecap = "Current content"

// legacyContentPanel recovers the fixed panel from immutable history, including
// an intervening old writer's id-only repost without the optional marker.
func (d TopicDoc) legacyContentPanel() (Entry, bool) {
	id := ""
	for _, e := range d.Entries {
		if e.Kind == EntryPanel && e.LegacyContent {
			id = e.PanelID
			break
		}
	}
	if id != "" {
		for i := len(d.Entries) - 1; i >= 0; i-- {
			if e := d.Entries[i]; e.Kind == EntryPanel && e.PanelID == id {
				return e, true
			}
		}
	}
	return Entry{}, false
}

// appendLegacyContent retains Body as the legacy writer/query projection. The
// optional Entry marker, not regenerated topic metadata or a user-chosen name,
// binds the one fixed panel without colliding with authored named panels.
func (s *State) appendLegacyContent(topic, body string, now time.Time) {
	d := s.topicDoc(topic)
	latest, ok := d.legacyContentPanel()
	if !ok {
		s.PanelCounter++
		latest.PanelID = fmt.Sprintf("p%d", s.PanelCounter)
	}
	s.appendEntry(topic, Entry{Kind: EntryPanel, PanelID: latest.PanelID, LegacyContent: true, Recap: legacyContentRecap, Text: body}, now)
}

// migrateLegacyContent appends snapshots without rewriting prior entries or
// inventing an edit time. Body remains readable/writable by v0.50.0. A later
// old body/edit write is imported once on reopen; equal projections do nothing.
func (s *State) migrateLegacyContent() bool {
	changed := false
	for i := range s.Topics {
		d := &s.Topics[i]
		latest, ok := d.legacyContentPanel()
		if (!ok && d.Body == "") || (ok && latest.Text == d.Body) {
			continue
		}
		at := d.ChangedAt
		if ok {
			// Old body/edit calls leave an event; old id-only reposts leave
			// Body untouched. Their log positions settle which projection won.
			bodyPos, panelPos, markedPos := -1, -1, -1
			for j, e := range d.Entries {
				if e.Kind == EntryPanel && e.PanelID == latest.PanelID && e.LegacyContent {
					markedPos = j
				}
				if e.Kind == EntrySystem && (e.Text == "Content updated." || e.Text == "Content edited." || e.Text == "Content cleared.") {
					bodyPos = j
					if d.Body == "" {
						at = e.At
					}
				}
				if e.ID == latest.ID {
					panelPos = j
				}
			}
			if !latest.LegacyContent && panelPos > bodyPos {
				if bodyPos > markedPos {
					// The old writer retained a distinct body snapshot before
					// its id repost. Keep those bytes too, then repeat the
					// winning panel as latest; committed history cannot move.
					s.EntrySeq++
					d.Entries = append(d.Entries, Entry{ID: fmt.Sprintf("e%d", s.EntrySeq), Kind: EntryPanel, PanelID: latest.PanelID, LegacyContent: true, Recap: legacyContentRecap, Text: d.Body, At: at, Imported: true})
					s.EntrySeq++
					winner := latest.clone()
					winner.ID, winner.LegacyContent, winner.Imported = fmt.Sprintf("e%d", s.EntrySeq), true, true
					d.Entries = append(d.Entries, winner)
				}
				d.Body, d.ChangedAt, d.Marks, d.BodyTimeProof = latest.Text, latest.At, nil, nil
				changed = true
				continue
			}
		} else {
			s.PanelCounter++
			latest.PanelID = fmt.Sprintf("p%d", s.PanelCounter)
		}
		s.EntrySeq++
		d.Entries = append(d.Entries, Entry{ID: fmt.Sprintf("e%d", s.EntrySeq), Kind: EntryPanel, PanelID: latest.PanelID, LegacyContent: true, Recap: legacyContentRecap, Text: d.Body, At: at, Imported: true})
		changed = true
	}
	return changed
}
