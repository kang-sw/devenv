package wswhiteboard

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// HistoryWindow is the initial topic window and each explicit older page.
const HistoryWindow = 50

// EntryView is an immutable entry with display-only disclosure and clock data.
// Ms is At in Unix milliseconds for the hover's relative time. Current marks
// the topic's latest agent line, the current-state line. A latest panel
// version carries its earlier versions, newest first, in Versions: they render
// folded under it, not at their own chronological position.
type EntryView struct {
	Entry
	Clock, Stamp string
	Ms           int64
	Open         bool
	Historical   bool
	Current      bool
	Versions     []EntryView
}

// OwnerAnswer reports a system event that records the owner's answer. The
// prefix is the one applyAnswer writes; an assumed answer is not the owner's.
func (e EntryView) OwnerAnswer() bool {
	return e.Kind == EntrySystem && e.ItemID > 0 && strings.HasPrefix(e.Text, fmt.Sprintf("Item {{%d}} answered [", e.ItemID))
}

var panelImagePattern = regexp.MustCompile(`!\[[^\]]*\]\(|<img\b`)

// ImageCount and LineCount are a panel payload's size hint on its title row.
func (e EntryView) ImageCount() int { return len(panelImagePattern.FindAllStringIndex(e.Text, -1)) }

func (e EntryView) LineCount() int {
	text := strings.TrimRight(e.Text, "\n")
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
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

// streamRunRows is how many rows a log run between panels shows before it
// folds behind one "log n lines" row; the newest run keeps that many visible.
const streamRunRows = 3

// StreamRow is one row of a log run: an agent line (Entry), or adjacent
// system events folded behind one chip (Systems).
type StreamRow struct {
	Entry   EntryView
	Systems []EntryView
}

func (r StreamRow) entries() []EntryView {
	if len(r.Systems) > 0 {
		return r.Systems
	}
	return []EntryView{r.Entry}
}

// Owner reports a row holding an owner answer.
func (r StreamRow) Owner() bool {
	for _, e := range r.entries() {
		if e.OwnerAnswer() {
			return true
		}
	}
	return false
}

// Range is the row's first and last clock, or one clock when they match.
func (r StreamRow) Range() string { return clockRange(r.entries()) }

// StreamSegment is a latest panel card, or the log run between two panels:
// Fold holds the rows behind its fold, Rows the rows shown below it.
type StreamSegment struct {
	Panel EntryView
	Fold  []StreamRow
	Rows  []StreamRow
}

// IsPanel reports a panel card segment.
func (s StreamSegment) IsPanel() bool { return s.Panel.ID != "" }

func (s StreamSegment) foldEntries() []EntryView {
	var out []EntryView
	for _, r := range s.Fold {
		out = append(out, r.entries()...)
	}
	return out
}

// FoldKey is the fold's first entry ID, its disclosure key and anchor.
func (s StreamSegment) FoldKey() string { return s.Fold[0].entries()[0].ID }

// FoldCount and FoldRange label the fold: its entry count and clock range.
func (s StreamSegment) FoldCount() int    { return len(s.foldEntries()) }
func (s StreamSegment) FoldRange() string { return clockRange(s.foldEntries()) }

// FoldOwner reports an owner answer behind the fold.
func (s StreamSegment) FoldOwner() bool {
	for _, r := range s.Fold {
		if r.Owner() {
			return true
		}
	}
	return false
}

func clockRange(entries []EntryView) string {
	if len(entries) == 0 {
		return ""
	}
	first, last := entries[0].Clock, entries[len(entries)-1].Clock
	if first == last {
		return first
	}
	return first + "–" + last
}

// Segments lays the page out as one chronological stream: each latest panel
// a card, the log lines between two panels one run. A run longer than
// streamRunRows folds whole; tail marks the newest page, whose last run keeps
// its newest rows visible under the fold. An owner answer is the owner's own
// line, so it stays a row of its own instead of folding into a system chip.
// Earlier panel versions render inside their latest card
// (EntryView.Versions), so their slots are skipped, and system events on
// either side of such a slot join one chip.
func (p EntryPage) Segments(tail bool) []StreamSegment {
	var out []StreamSegment
	var rows []StreamRow
	flush := func(last bool) {
		if len(rows) == 0 {
			return
		}
		seg := StreamSegment{Rows: rows}
		if len(rows) > streamRunRows {
			cut := len(rows)
			if last {
				cut -= streamRunRows
			}
			seg.Fold, seg.Rows = rows[:cut], rows[cut:]
		}
		out = append(out, seg)
		rows = nil
	}
	for _, run := range p.Runs {
		switch {
		case len(run.Systems) > 0:
			for _, e := range run.Systems {
				n := len(rows)
				switch {
				case e.OwnerAnswer():
					rows = append(rows, StreamRow{Entry: e})
				case n > 0 && len(rows[n-1].Systems) > 0:
					rows[n-1].Systems = append(rows[n-1].Systems, e)
				default:
					rows = append(rows, StreamRow{Systems: []EntryView{e}})
				}
			}
		case run.Entry.Kind == EntryPanel:
			if run.Entry.Historical {
				continue
			}
			flush(false)
			out = append(out, StreamSegment{Panel: run.Entry})
		default:
			rows = append(rows, StreamRow{Entry: run.Entry})
		}
	}
	flush(tail)
	return out
}

// StreamTotal counts the topic's entries across every history page.
func (t TopicView) StreamTotal() int {
	n := t.Stream.Count
	for _, page := range t.Older {
		n += page.Count
	}
	return n
}

// StreamLines counts the topic's log entries, agent lines and system events.
func (t TopicView) StreamLines() int {
	n := 0
	for _, page := range append([]EntryPage{t.Stream}, t.Older...) {
		for _, run := range page.Runs {
			if len(run.Systems) > 0 {
				n += len(run.Systems)
			} else if run.Entry.Kind != EntryPanel {
				n++
			}
		}
	}
	return n
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
			v.Clock, v.Stamp, v.Ms = at.In(loc).Format("15:04"), at.In(loc).Format("2006-01-02 15:04"), at.UnixMilli()
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
	versions := map[string][]EntryView{}
	current := -1
	for i := range all {
		all[i].Open = expanded[all[i].ID]
		if all[i].Historical {
			versions[all[i].PanelID] = append([]EntryView{all[i]}, versions[all[i].PanelID]...)
		}
		if all[i].Kind == EntryAgent {
			current = i
		}
	}
	if current >= 0 {
		all[current].Current = true
	}
	for i := range all {
		if all[i].Kind == EntryPanel && !all[i].Historical {
			all[i].Versions = versions[all[i].PanelID]
		}
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
