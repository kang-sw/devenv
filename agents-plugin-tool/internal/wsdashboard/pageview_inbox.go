package wsdashboard

import (
	"github.com/kang-sw/devenv/internal/wsinbox"
)

// HeldView is a message the user sent from the page that the session has
// not received yet: it sits in the session's board-answer inbox until the
// host adapter drains it at the end of a turn, and can be cancelled until
// then.
type HeldView struct {
	// ID is the inbox entry's ID, which the cancel control names.
	ID string
	// At is the first send's time (epoch ms).
	At int64
	// Parts are the merged sends' texts; the bubble divides them.
	Parts []string
	Chips []wsinbox.Chip
}

// SentMark is the latest answer sent for one board item, from the inbox:
// Held while the message holding it is undrained.
type SentMark struct {
	ID, Answer, Note string
	Held             bool
}

// heldViews lists the held entries, oldest first.
func heldViews(held []wsinbox.Record) []HeldView {
	out := make([]HeldView, 0, len(held))
	for _, r := range held {
		out = append(out, HeldView{ID: r.ID(), At: r.Entry.SentAt, Parts: r.Entry.Parts, Chips: r.Entry.Chips})
	}
	return out
}

// sentMarks derives each board item's sent mark from the inbox: the newest
// chip for the item across the drained records and then the held entries, so
// a held answer shows over an earlier delivered one. The drained records are
// on disk, so a mark survives a page reload while the newest DrainedCap
// records hold it.
func sentMarks(drained, held []wsinbox.Record) []SentMark {
	var order []string
	marks := map[string]SentMark{}
	add := func(records []wsinbox.Record, isHeld bool) {
		for _, r := range records {
			for _, c := range r.Entry.Chips {
				if _, seen := marks[c.ID]; !seen {
					order = append(order, c.ID)
				}
				marks[c.ID] = SentMark{ID: c.ID, Answer: c.Answer, Note: c.Note, Held: isHeld}
			}
		}
	}
	add(drained, false)
	add(held, true)
	out := make([]SentMark, 0, len(order))
	for _, id := range order {
		out = append(out, marks[id])
	}
	return out
}
