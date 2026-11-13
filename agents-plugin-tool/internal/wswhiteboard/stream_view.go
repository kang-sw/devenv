package wswhiteboard

import "time"

// HistoryWindow is the initial topic window and each explicit older page.
const HistoryWindow = 50

// EntryView is an immutable entry with display-only disclosure and clock data.
// Historical panels stay folded at their original chronological position.
type EntryView struct {
	Entry
	Clock, Stamp string
	Open         bool
	Historical   bool
}

// EntryRun folds only adjacent system entries, never a panel or agent line.
type EntryRun struct {
	Entry   EntryView
	Systems []EntryView
}

// EntryPage is one chronological page. Older pages are inert until loaded.
type EntryPage struct {
	Runs  []EntryRun
	Count int
}

func streamView(doc TopicDoc, closed bool, loc *time.Location) (latest []EntryView, stream EntryPage, older []EntryPage) {
	if loc == nil {
		loc = time.Local
	}
	last := map[string]int{}
	for i, e := range doc.Entries {
		if e.Kind == EntryPanel {
			last[e.PanelID] = i
		}
	}
	viewOf := func(e Entry) EntryView {
		v := EntryView{Entry: e}
		if at, err := time.Parse(time.RFC3339Nano, e.At); err == nil {
			v.Clock, v.Stamp = at.In(loc).Format("15:04"), at.In(loc).Format("2006-01-02 15:04")
		}
		return v
	}
	var all []EntryView
	for i, e := range doc.Entries {
		v := viewOf(e)
		if e.Kind == EntryPanel {
			v.Historical = last[e.PanelID] != i
			if !v.Historical {
				latest = append(latest, v)
			}
		}
		all = append(all, v)
	}
	// Distinct latest panel versions, not the last three panel records.
	expanded := map[string]bool{}
	for i := max(0, len(latest)-3); i < len(latest); i++ {
		expanded[latest[i].ID] = !closed
	}
	for i := range all {
		all[i].Open = expanded[all[i].ID]
	}
	page := func(entries []EntryView) EntryPage {
		out := EntryPage{Count: len(entries)}
		for _, entry := range entries {
			n := len(out.Runs)
			if entry.Kind == EntrySystem {
				if n == 0 || len(out.Runs[n-1].Systems) == 0 {
					out.Runs = append(out.Runs, EntryRun{})
					n++
				}
				out.Runs[n-1].Systems = append(out.Runs[n-1].Systems, entry)
			} else {
				out.Runs = append(out.Runs, EntryRun{Entry: entry})
			}
		}
		return out
	}
	start := max(0, len(all)-HistoryWindow)
	stream = page(all[start:])
	// Nearest older page first, so loading prepends one page without reversal.
	for end := start; end > 0; {
		from := max(0, end-HistoryWindow)
		older = append(older, page(all[from:end]))
		end = from
	}
	return latest, stream, older
}
