package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Display-only metadata never supplies item state or changes append ordering.
type inputHeader struct {
	InboxID string   `json:"inbox_id"`
	SentAt  int64    `json:"sent_at"`
	Excerpt string   `json:"excerpt"`
	ChipIDs []string `json:"chip_ids"`
	Clock   string   `json:"-"`
}

type timelineRow struct {
	Key, From, To, Reasked, AnswerTarget string
	Page                                 EntryPage
	Item                                 *ItemView
	Ask, Answer, Preceding, LatestAnswer bool
	Clock, At                            string
	Inputs                               []inputHeader
}

func (r timelineRow) Count() int { return max(1, r.Page.Count) }

type timelinePart struct {
	Rows       []timelineRow
	Gap, Above bool
	Count      int
	From, To   string
}

func boardInputs(st *State) map[int][]inputHeader {
	out := map[int][]inputHeader{}
	for _, doc := range st.Topics {
		for _, e := range doc.Entries {
			if e.Seq > 0 && len(e.Inputs) > 0 {
				var inputs []inputHeader
				if json.Unmarshal(e.Inputs, &inputs) == nil {
					out[e.Seq] = inputs
				}
			}
		}
	}
	return out
}

func timelineView(t TopicView, items []Item, inputs map[int][]inputHeader, loc *time.Location) []timelinePart {
	if loc == nil {
		loc = time.Local
	}
	clock := func(at string) string {
		v, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return ""
		}
		return v.In(loc).Format("15:04")
	}
	byID := map[int]Item{}
	for _, item := range items {
		if item.Topic == t.Name {
			byID[item.ID] = item
		}
	}
	var all []EntryView
	for i := len(t.Older) - 1; i >= 0; i-- {
		all = append(all, timelineEntries(t.Older[i])...)
	}
	all = append(all, timelineEntries(t.Stream)...)
	firstAsk, lastAnswer, panel := map[int]string{}, map[int]string{}, map[string]string{}
	changed := map[int]bool{}
	for _, e := range all {
		if _, ok := byID[e.ItemID]; ok && e.Kind == EntrySystem && strings.HasPrefix(e.Text, fmt.Sprintf("Item {{%d}} added [", e.ItemID)) && firstAsk[e.ItemID] == "" {
			firstAsk[e.ItemID] = e.ID
		}
		if e.OwnerAnswer() {
			lastAnswer[e.ItemID] = e.ID
		}
		if e.Kind == EntryPanel && !e.Historical {
			panel[e.PanelID] = e.ID
		}
		if e.Kind == EntryAgent || e.Kind == EntryPanel || e.ItemID > 0 {
			changed[e.Seq] = true
		}
	}
	var rows []timelineRow
	anchors := map[string]bool{}
	leads, openPanels := map[string]bool{}, map[string]bool{}
	// Converted items deliberately have no synthetic add event or clock.
	var orphan []int
	for id := range byID {
		if firstAsk[id] == "" {
			orphan = append(orphan, id)
		}
	}
	sort.Ints(orphan)
	for _, id := range orphan {
		iv := itemView(byID[id])
		key := "item-" + strconv.Itoa(id)
		rows = append(rows, timelineRow{Key: key, Item: &iv, Ask: true, AnswerTarget: lastAnswer[id]})
		anchors[key] = true
	}
	seenBatch := map[int]bool{}
	preceding, cluster := "", false
	for _, e := range all {
		if e.Historical {
			// The nearest preceding version is represented at the latest
			// version's position, preserving the panel disclosure contract.
			preceding, cluster = panel[e.PanelID], false
			continue
		}
		r := timelineRow{Key: e.ID, From: e.Clock, To: e.Clock, Page: EntryPage{Count: 1, Runs: []EntryRun{{Entry: e}}}}
		if !seenBatch[e.Seq] && e.Seq > 0 {
			seenBatch[e.Seq] = true
			for _, input := range inputs[e.Seq] {
				h := input
				if !changed[e.Seq] {
					h.Excerpt = ""
				}
				h.ChipIDs = nil
				for _, id := range input.ChipIDs {
					n, _ := strconv.Atoi(id)
					if _, ok := byID[n]; ok {
						h.ChipIDs = append(h.ChipIDs, id)
					}
				}
				if h.Excerpt != "" || len(h.ChipIDs) > 0 {
					if h.SentAt > 0 {
						h.Clock = time.UnixMilli(h.SentAt).In(loc).Format("15:04")
					}
					r.Inputs = append(r.Inputs, h)
					anchors[e.ID] = true
				}
			}
		}
		if firstAsk[e.ItemID] == e.ID || e.OwnerAnswer() {
			if item, ok := byID[e.ItemID]; ok {
				iv := itemView(item)
				r.Item, r.Ask, r.Answer = &iv, firstAsk[e.ItemID] == e.ID, e.OwnerAnswer()
				r.Clock, r.At, r.Reasked = e.Clock, e.At, clock(item.ReaskedAt)
				r.AnswerTarget = lastAnswer[e.ItemID]
				if r.Answer {
					currentAnswer := e.ID == lastAnswer[e.ItemID] && item.Answer != "" && item.Status != StatusAssumed
					at := e.At
					if currentAnswer && item.AnsweredAt != "" {
						at = item.AnsweredAt
					}
					if e.AnswerSentAt > 0 {
						at = time.UnixMilli(e.AnswerSentAt).UTC().Format(time.RFC3339Nano)
					}
					r.Clock, r.At = clock(at), at
					// Earlier answers use their immutable event text, never
					// the mutable latest answer/quote on the item.
					r.LatestAnswer = currentAnswer
				}
			}
			anchors[e.ID] = true
			if r.Ask {
				if !cluster && preceding != "" {
					anchors[preceding] = true
					leads[preceding] = true
				}
				// A question-leading rich panel stays open while any ask in
				// the cluster waits, even outside the usual latest three.
				if preceding != "" && r.Item.Status == StatusOpen {
					openPanels[preceding] = true
				}
				cluster = true
			}
		}
		if e.Kind == EntryAgent || e.Kind == EntryPanel {
			preceding, cluster = e.ID, false
		}
		if e.Current {
			anchors[e.ID] = true
		}
		if e.Kind == EntrySystem && !r.Ask && !r.Answer && !e.OwnerAnswer() {
			r.Page.Runs = []EntryRun{{Systems: []EntryView{e}}}
		}
		rows = append(rows, r)
	}
	// Only the latest batch (one entry for legacy) joins the anchors by
	// default. Hidden history remains inert regardless of the 50-entry seam.
	if len(all) > 0 {
		last := all[len(all)-1]
		for _, e := range all {
			if (last.Seq > 0 && e.Seq == last.Seq) || e.ID == last.ID {
				anchors[e.ID] = true
			}
		}
	}
	var out []timelinePart
	for _, r := range rows {
		r.Preceding = leads[r.Key]
		if openPanels[r.Key] && len(r.Page.Runs) > 0 {
			r.Page.Runs[0].Entry.Open = true
		}
		gap := !anchors[r.Key]
		if len(out) == 0 || out[len(out)-1].Gap != gap {
			out = append(out, timelinePart{Gap: gap, Above: len(out) > 0, From: r.From})
		}
		p := &out[len(out)-1]
		merged := false
		if len(p.Rows) > 0 && len(r.Inputs) == 0 && len(r.Page.Runs) > 0 && len(r.Page.Runs[0].Systems) > 0 {
			prev := &p.Rows[len(p.Rows)-1]
			if len(prev.Page.Runs) > 0 && len(prev.Page.Runs[0].Systems) > 0 {
				prev.Page.Runs[0].Systems = append(prev.Page.Runs[0].Systems, r.Page.Runs[0].Systems...)
				prev.Page.Count += r.Count()
				prev.To = r.To
				merged = true
			}
		}
		if !merged {
			p.Rows = append(p.Rows, r)
		}
		p.Count += r.Count()
		p.To = r.To
	}
	return out
}

func timelineEntries(p EntryPage) []EntryView {
	var out []EntryView
	for _, run := range p.Runs {
		if len(run.Systems) > 0 {
			out = append(out, run.Systems...)
		} else {
			out = append(out, run.Entry)
		}
	}
	return out
}

// Template adapters keep the union of question, answer and entry rows private.
type timelineData struct {
	timelineRow
	Page *pageData
}

func (p *pageData) TimelineOf(r timelineRow) timelineData { return timelineData{r, p} }
func (r timelineData) EntryPage() entryPageData           { return r.Page.EntryPageOf(r.timelineRow.Page) }
func (r timelineData) ItemData() itemData                 { return r.Page.ItemOf(*r.Item) }
func (r timelineData) Event() EntryView {
	if len(r.timelineRow.Page.Runs) == 0 {
		return EntryView{}
	}
	return r.timelineRow.Page.Runs[0].Entry
}
