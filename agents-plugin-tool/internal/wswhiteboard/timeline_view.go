package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Display-only metadata never supplies item state or changes append ordering.
type inputHeader struct {
	InboxID string      `json:"inbox_id"`
	SentAt  int64       `json:"sent_at"`
	Excerpt string      `json:"excerpt"`
	ChipIDs []string    `json:"chip_ids"`
	Clock   string      `json:"-"`
	Stamp   string      `json:"-"`
	Chips   []inputChip `json:"-"`
}

// inputChip is one answered/asked item an owner message referenced, with the
// answer to show inline when the message carried no text of its own.
type inputChip struct{ ID, Answer string }

type timelineRow struct {
	Key, From, To, Reasked, AnswerTarget string
	Page                                 EntryPage
	Item                                 *ItemView
	Created                              []timelineRow
	Ask, Answer, Preceding, LatestAnswer bool
	Clock, At                            string
	Ms                                   int64
	Stamp                                string
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
	firstAsk, lastAnswer := map[int]string{}, map[int]string{}
	changed := map[int]bool{}
	createdAt := map[string][]int{}
	for _, e := range all {
		createdAt[e.ID] = timelineCreated(e, byID)
		for _, id := range createdAt[e.ID] {
			if firstAsk[id] == "" {
				firstAsk[id] = e.ID
			}
		}
		if e.OwnerAnswer() {
			lastAnswer[e.ItemID] = e.ID
		}
		if e.Kind == EntryAgent || e.Kind == EntryPanel || e.ItemID > 0 || len(createdAt[e.ID]) > 0 {
			changed[e.Seq] = true
		}
	}
	var rows []timelineRow
	anchors := map[string]bool{}
	leads, openPanels := map[string]bool{}, map[string]bool{}
	// Converted items deliberately have no synthetic add event or clock.
	for _, item := range items {
		id := item.ID
		if item.Topic != t.Name || firstAsk[id] != "" {
			continue
		}
		iv := itemView(item)
		key := "item-" + strconv.Itoa(id)
		rows = append(rows, timelineRow{Key: key, Item: &iv, Ask: true, AnswerTarget: lastAnswer[id]})
		// Only an open question is pulled out of the fold; closed ones fold like logs.
		anchors[key] = item.Status == StatusOpen
	}
	seenBatch := map[int]bool{}
	preceding, cluster := "", false
	for _, e := range all {
		// The unified stream already carries undated settled-item prefixes.
		// The timeline's permanent head rows above also include open items.
		if e.Kind == "item" {
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
					if it, ok := byID[n]; ok {
						h.ChipIDs = append(h.ChipIDs, id)
						c := inputChip{ID: id}
						if it.Status == StatusAnswered {
							c.Answer = strings.TrimSpace(it.Answer)
							if r := []rune(c.Answer); len(r) > 40 {
								c.Answer = string(r[:40]) + "…"
							}
						}
						h.Chips = append(h.Chips, c)
					}
				}
				if h.Excerpt != "" || len(h.ChipIDs) > 0 {
					if h.SentAt > 0 {
						sent := time.UnixMilli(h.SentAt).In(loc)
						h.Clock, h.Stamp = sent.Format("15:04"), sent.Format("2006-01-02 15:04")
					}
					r.Inputs = append(r.Inputs, h)
				}
			}
		}
		for _, id := range createdAt[e.ID] {
			if firstAsk[id] == e.ID {
				iv := itemView(byID[id])
				r.Created = append(r.Created, timelineRow{Item: &iv, Ask: true, AnswerTarget: lastAnswer[id], Reasked: clock(byID[id].ReaskedAt)})
			}
		}
		if len(r.Created) > 0 && e.ItemID == 0 {
			r.Ask = true
			r.Clock, r.At = e.Clock, e.At
			anyOpen := false
			for _, row := range r.Created {
				if row.Item.Status == StatusOpen {
					anyOpen = true
					if preceding != "" {
						openPanels[preceding] = true
					}
				}
			}
			if anyOpen {
				anchors[e.ID] = true
				if !cluster && preceding != "" {
					anchors[preceding], leads[preceding] = true, true
				}
				cluster = true
			}
		}
		if firstAsk[e.ItemID] == e.ID || e.OwnerAnswer() {
			if item, ok := byID[e.ItemID]; ok {
				iv := itemView(item)
				r.Item, r.Ask, r.Answer = &iv, firstAsk[e.ItemID] == e.ID, e.OwnerAnswer()
				r.Clock, r.At, r.Reasked = e.Clock, e.At, clock(item.ReaskedAt)
				r.Ms, r.Stamp = e.Ms, e.Stamp
				r.AnswerTarget = lastAnswer[e.ItemID]
				if r.Answer {
					currentAnswer := e.ID == lastAnswer[e.ItemID] && (item.Answer != "" || item.Status == StatusCommented) && item.Status != StatusAssumed
					at := e.At
					if currentAnswer && item.AnsweredAt != "" {
						at = item.AnsweredAt
					}
					if e.AnswerSentAt > 0 {
						at = time.UnixMilli(e.AnswerSentAt).UTC().Format(time.RFC3339Nano)
					}
					r.Clock, r.At = clock(at), at
					r.Page.Runs[0].Entry.Clock, r.Page.Runs[0].Entry.At = r.Clock, at
					if displayAt, err := time.Parse(time.RFC3339Nano, at); err == nil {
						r.Page.Runs[0].Entry.Ms = displayAt.UnixMilli()
						r.Page.Runs[0].Entry.Stamp = displayAt.In(loc).Format("2006-01-02 15:04")
					}
					// Earlier answers use their immutable event text, never
					// the mutable latest answer/quote on the item.
					r.LatestAnswer = currentAnswer
				}
			}
			if r.Ask && r.Item != nil && r.Item.Status == StatusOpen {
				anchors[e.ID] = true
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
		if e.Kind == EntrySystem && e.Item == nil && !r.Ask && !r.Answer && !e.OwnerAnswer() {
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
		// An earlier panel version stays folded even when it leads a question.
		if openPanels[r.Key] && len(r.Page.Runs) > 0 && !r.Page.Runs[0].Entry.Historical {
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
func (r timelineData) ItemData() itemData {
	d := r.Page.ItemOf(*r.Item)
	if r.Event().ID == "" {
		d.History = &EntryView{}
	} else if r.Ask {
		d.AskClock, d.AskAt, d.AskReasked = r.Clock, r.At, r.Reasked
		d.AskMs, d.AskStamp = r.Ms, r.Stamp
	}
	return d
}
func (r timelineData) CreatedRows() []timelineData {
	var out []timelineData
	for i, child := range r.Created {
		row := r.timelineRow
		if i > 0 {
			row.Key = ""
		}
		row.Item, row.Created = child.Item, nil
		row.AnswerTarget, row.Reasked = child.AnswerTarget, child.Reasked
		out = append(out, timelineData{row, r.Page})
	}
	return out
}

// Creation coalescing keeps one durable event. Expand only its server-written
// class/ID list for display, retaining that event's position and clock.
func timelineCreated(e EntryView, items map[int]Item) []int {
	if e.Kind != EntrySystem {
		return nil
	}
	if _, ok := items[e.ItemID]; ok && e.ItemID > 0 && strings.HasPrefix(e.Text, fmt.Sprintf("Item {{%d}} added [", e.ItemID)) {
		return []int{e.ItemID}
	}
	if !strings.HasPrefix(e.Text, "Items added: ") {
		return nil
	}
	var ids []int
	for _, part := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(e.Text, "Items added: "), "."), ", ") {
		class, ref, ok := strings.Cut(part, " {{")
		if !ok || (class != ClassPolicy && class != ClassTechnical && class != ClassAnnounced && class != ClassCheck) || !strings.HasSuffix(ref, "}}") {
			continue
		}
		text := strings.TrimSuffix(ref, "}}")
		id, err := strconv.Atoi(text)
		if _, exists := items[id]; err == nil && exists && id > 0 && strconv.Itoa(id) == text {
			// Current class patches do not rewrite the immutable add event.
			ids = append(ids, id)
		}
	}
	return ids
}
func (r timelineData) Event() EntryView {
	if len(r.timelineRow.Page.Runs) == 0 {
		return EntryView{}
	}
	return r.timelineRow.Page.Runs[0].Entry
}
