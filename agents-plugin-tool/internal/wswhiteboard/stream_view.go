package wswhiteboard

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// HistoryWindow chunks the complete entry projection internally. The public
// timeline selects anchors across all chunks and ships the rest as inert gaps.
const HistoryWindow = 50

// EntryView is an immutable entry with display-only disclosure and clock data.
// Ms is At in Unix milliseconds for the hover's relative time. Current marks
// the topic's latest agent line, the current-state line. Every panel version
// renders at its own write position: Version counts from 1 in write order up
// to Of, an earlier (Historical) version links to its panel's Newest version,
// and each version to its Prior one. BoundName is the panel's topic-local
// name, which legacy id-only reposts do not repeat.
type EntryView struct {
	Entry
	Clock, Stamp  string
	Ms            int64
	Open          bool
	Historical    bool
	Current       bool
	Version, Of   int
	Newest, Prior VersionRef
	BoundName     string
	// Item is the current settled item projected at its recorded transition.
	// It is display-only; Entry and durable item/event records stay unchanged.
	Item *ItemView
}

// VersionRef names one panel version for a link: its entry ID, 1-based
// version number and clock.
type VersionRef struct {
	ID, Clock string
	Version   int
}

// Label is the version as its links show it: "v2 · 09:55", or "v2" when the
// version has no recorded time.
func (r VersionRef) Label() string {
	if r.Clock == "" {
		return fmt.Sprintf("v%d", r.Version)
	}
	return fmt.Sprintf("v%d · %s", r.Version, r.Clock)
}

// VersionLabel labels this panel version like VersionRef.Label.
func (e EntryView) VersionLabel() string {
	return VersionRef{ID: e.ID, Clock: e.Clock, Version: e.Version}.Label()
}

// panelEventPattern matches the system event every panel post appends. The
// panel row itself shows that post, so the history leaves the event out; the
// store and query keep it.
var panelEventPattern = regexp.MustCompile(`^Panel \[p[1-9][0-9]*\]\(#p[1-9][0-9]*\) (?:added|re-posted): `)

func panelEvent(e Entry) bool {
	return e.Kind == EntrySystem && e.ItemID == 0 && panelEventPattern.MatchString(e.Text)
}

// OwnerAnswer includes actual owner input (a choice or a comment), never
// the lead's assumption. Both reuse attributed system entries.
func (e EntryView) OwnerAnswer() bool {
	prefix := fmt.Sprintf("Item {{%d}} ", e.ItemID)
	return e.Kind == EntrySystem && e.ItemID > 0 && (strings.HasPrefix(e.Text, prefix+"answered [") || strings.HasPrefix(e.Text, prefix+"user commented ["))
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

// StreamSegment is a latest panel card, or the log run between two panels,
// every row of it shown.
type StreamSegment struct {
	Panel EntryView
	Rows  []StreamRow
}

// IsPanel reports a panel card segment.
func (s StreamSegment) IsPanel() bool { return s.Panel.ID != "" }

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

// Segments lays the page out as one chronological stream: each panel version
// a card at its own write position, the log lines between two panels one
// run, every line shown. An owner answer is the owner's own line, so it stays
// a row of its own instead of folding into a system chip.
func (p EntryPage) Segments() []StreamSegment {
	var out []StreamSegment
	var rows []StreamRow
	flush := func() {
		if len(rows) == 0 {
			return
		}
		out = append(out, StreamSegment{Rows: rows})
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
		case run.Entry.Item != nil:
			flush()
			out = append(out, StreamSegment{Rows: []StreamRow{{Entry: run.Entry}}})
		case run.Entry.Kind == EntryPanel:
			flush()
			out = append(out, StreamSegment{Panel: run.Entry})
		default:
			rows = append(rows, StreamRow{Entry: run.Entry})
		}
	}
	flush()
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

// PanelVersions counts the topic's panel versions across every history page.
func (t TopicView) PanelVersions() int {
	n := 0
	for _, page := range append([]EntryPage{t.Stream}, t.Older...) {
		for _, run := range page.Runs {
			if len(run.Systems) == 0 && run.Entry.Kind == EntryPanel {
				n++
			}
		}
	}
	return n
}

func streamView(doc TopicDoc, closed bool, loc *time.Location, items []ItemView) (latest []EntryView, stream EntryPage, older []EntryPage) {
	if loc == nil {
		loc = time.Local
	}
	last, counts, names := map[string]int{}, map[string]int{}, map[string]string{}
	for i, e := range doc.Entries {
		if e.Kind == EntryPanel {
			last[e.PanelID] = i
			counts[e.PanelID]++
			if e.PanelName != "" {
				names[e.PanelID] = e.PanelName
			}
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
	prior := map[string]VersionRef{}
	for i, e := range doc.Entries {
		if panelEvent(e) {
			continue
		}
		v := viewOf(e)
		if e.Kind == EntryPanel {
			v.Historical = last[e.PanelID] != i
			v.Of, v.BoundName, v.Prior = counts[e.PanelID], names[e.PanelID], prior[e.PanelID]
			v.Version = v.Prior.Version + 1
			prior[e.PanelID] = VersionRef{ID: e.ID, Clock: v.Clock, Version: v.Version}
			if !v.Historical {
				latest = append(latest, v)
			}
		}
		all = append(all, v)
	}
	for i := range all {
		if all[i].Kind == EntryPanel {
			all[i].Newest = prior[all[i].PanelID]
		}
	}
	// Distinct latest panel versions, not the last three panel records.
	expanded := map[string]bool{}
	for i := max(0, len(latest)-3); i < len(latest); i++ {
		expanded[latest[i].ID] = !closed
	}
	current := -1
	for i := range all {
		all[i].Open = expanded[all[i].ID]
		if all[i].Kind == EntryAgent {
			current = i
		}
	}
	if current >= 0 {
		all[current].Current = true
	}
	all = projectHistoryItems(all, items)
	page := func(entries []EntryView) EntryPage {
		out := EntryPage{Count: len(entries)}
		for _, entry := range entries {
			n := len(out.Runs)
			if entry.Kind == EntrySystem && entry.Item == nil {
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
