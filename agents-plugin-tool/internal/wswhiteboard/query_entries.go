package wswhiteboard

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const defaultQueryLast = 10

var (
	entryQueryFields     = []string{"entries", "since", "last", "panel", "entry", "kind"}
	itemQuerySelectors   = []string{"id", "ids", "status", "depth", "fields", "body"}
	streamQuerySelectors = []string{"entries", "since", "last", "kind"}
)

// QueryEntry is a directly retrieved immutable entry. Only a direct panel
// lookup populates a panel payload; discovery/stream results never do.
type QueryEntry struct {
	Topic string
	Entry Entry
}

type entryQueryFilter struct {
	topics              map[string]bool
	text, entries, kind string
	closed              bool
	last                int
	sinceID             int
	sinceTime           *time.Time
}

// Query routes explicit item/content selectors to the retained item query;
// otherwise discovery is metadata-only. Filters never implicitly retrieve
// current content or panel bodies. Direct locators fail explicitly when stale.
func Query(state *State, payload any) (QueryResult, []string) {
	p := &problems{}
	obj, ok := payloadObject(p, payload)
	if !ok {
		return QueryResult{}, p.List
	}
	known := append(append([]string(nil), queryFields...), entryQueryFields...)
	checkKnownFields(p, "payload", obj, known)
	unknownProblems := len(p.List)
	hasAny := func(fields []string) bool {
		for _, field := range fields {
			if _, ok := obj[field]; ok {
				return true
			}
		}
		return false
	}
	_, panel := obj["panel"]
	_, entry := obj["entry"]
	legacy, stream := hasAny(itemQuerySelectors), hasAny(streamQuerySelectors)
	if panel && entry {
		p.Add("payload: panel and entry cannot be combined")
	}
	if panel || entry {
		for _, field := range known {
			if field == "panel" || field == "entry" || field == "topic" {
				continue
			}
			if _, present := obj[field]; present {
				p.Add("payload.%s: cannot be combined with panel or entry retrieval", field)
			}
		}
	} else if legacy && stream {
		p.Add("payload: item/content selectors (id, ids, status, depth, fields, body) cannot be combined with stream selectors (entries, since, last, kind)")
	}
	if len(p.List) > unknownProblems {
		return QueryResult{}, p.List
	}
	if legacy {
		// The retained parser aggregates unknown fields and invalid values.
		return queryItems(state, payload)
	}
	var result QueryResult
	var more []string
	if panel || entry {
		result, more = queryDirect(state, obj, panel)
	} else {
		result, more = queryDiscovery(state, obj)
	}
	p.List = append(p.List, more...)
	if len(p.List) > 0 {
		return QueryResult{}, p.List
	}
	return result, nil
}

func queryLocator(id, prefix string) int {
	if !strings.HasPrefix(id, prefix) {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, prefix))
	if err != nil || n < 1 || prefix+strconv.Itoa(n) != id {
		return 0
	}
	return n
}

func queryTopics(p *problems, obj map[string]any, state *State) map[string]bool {
	var out map[string]bool
	if raw, present := obj["topic"]; present {
		out = stringSet(stringList(p, "payload.topic", raw, nil))
		if state != nil {
			resolved := map[string]bool{}
			for name := range out {
				resolved[state.resolveTopic(name)] = true
			}
			out = resolved
		}
	}
	return out
}

func findQueryEntry(state *State, id string) (QueryEntry, bool) {
	if state != nil {
		for _, doc := range state.Topics {
			for _, entry := range doc.Entries {
				if entry.ID == id {
					return QueryEntry{Topic: doc.Name, Entry: entry}, true
				}
			}
		}
	}
	return QueryEntry{}, false
}

func queryDirect(state *State, obj map[string]any, panel bool) (QueryResult, []string) {
	p := &problems{}
	selector := "entry"
	if panel {
		selector = "panel"
	}
	id, valid := requiredString(p, "payload."+selector, obj[selector])
	if valid && queryLocator(id, "e") == 0 && (!panel || queryLocator(id, "p") == 0) {
		if panel {
			p.Add("payload.panel: expected pN (latest panel) or eN (exact panel version); got %q", id)
		} else {
			p.Add("payload.entry: expected eN; got %q", id)
		}
	}
	topics := queryTopics(p, obj, state)
	if len(p.List) > 0 {
		return QueryResult{}, p.List
	}
	var found QueryEntry
	ok := false
	if panel && queryLocator(id, "p") != 0 {
		if state != nil {
			for _, doc := range state.Topics {
				for _, e := range doc.Entries {
					if e.Kind == EntryPanel && e.PanelID == id && (!ok || queryLocator(e.ID, "e") > queryLocator(found.Entry.ID, "e")) {
						found, ok = QueryEntry{Topic: doc.Name, Entry: e}, true
					}
				}
			}
		}
	} else {
		found, ok = findQueryEntry(state, id)
	}
	if !ok || topics != nil && !topics[found.Topic] {
		p.Add("payload.%s: %s not found in selected topics (stale or missing locator)", selector, id)
	} else if panel && found.Entry.Kind != EntryPanel {
		p.Add("payload.panel: %s is a %s log entry; use entry:%q", id, found.Entry.Kind, id)
	} else if !panel && found.Entry.Kind == EntryPanel {
		p.Add("payload.entry: %s is a panel version; use panel:%q", id, id)
	}
	if len(p.List) > 0 {
		return QueryResult{}, p.List
	}
	return QueryResult{Mode: selector, Direct: &found}, nil
}

func parseEntryQuery(p *problems, obj map[string]any, state *State) entryQueryFilter {
	f := entryQueryFilter{last: defaultQueryLast}
	f.topics = queryTopics(p, obj, state)
	if raw, present := obj["text"]; present {
		if s, ok := requiredString(p, "payload.text", raw); ok {
			f.text = strings.ToLower(s)
		}
	}
	// Like the item query, a named topic or text search includes closed topics
	// unless explicitly overridden; unfiltered discovery folds them.
	f.closed = f.topics != nil || f.text != ""
	if closed, present := boolField(p, obj, "closed"); present {
		f.closed = closed
	}
	if raw, present := obj["entries"]; present {
		f.entries, _ = requiredEnum(p, "payload.entries", raw, []string{"log", "panel", "all"})
	}
	if raw, present := obj["kind"]; present {
		f.kind, _ = requiredEnum(p, "payload.kind", raw, []string{EntryAgent, EntrySystem})
	}
	if f.kind != "" && f.entries == "panel" {
		p.Add("payload.kind: cannot be combined with entries:panel (kind selects agent or system logs)")
	}
	if raw, present := obj["last"]; present {
		if n, ok := intValue(p, "payload.last", raw); ok {
			if n < 1 {
				p.Add("payload.last: must be positive")
			} else {
				f.last = n
			}
		}
	}
	if raw, present := obj["since"]; present {
		if s, ok := requiredString(p, "payload.since", raw); ok {
			if strings.HasPrefix(s, "e") {
				f.sinceID = queryLocator(s, "e")
				if f.sinceID == 0 {
					p.Add("payload.since: expected an RFC3339 time or canonical eN entry ID; got %q", s)
				} else if _, held := findQueryEntry(state, s); !held {
					p.Add("payload.since: %s not found (stale or missing locator)", s)
				}
			} else {
				at, err := time.Parse(time.RFC3339Nano, s)
				if err != nil {
					p.Add("payload.since: expected an RFC3339 time or eN entry ID; got %q", s)
				} else {
					f.sinceTime = &at
				}
			}
		}
	}
	return f
}

func (f entryQueryFilter) matches(e Entry) bool {
	if f.entries == "log" && e.Kind == EntryPanel || f.entries == "panel" && e.Kind != EntryPanel {
		return false
	}
	if f.kind != "" && e.Kind != f.kind {
		return false
	}
	if f.sinceID != 0 && queryLocator(e.ID, "e") <= f.sinceID {
		return false
	}
	if f.sinceTime != nil {
		at, err := time.Parse(time.RFC3339Nano, e.At)
		if err != nil || !at.After(*f.sinceTime) {
			return false
		}
	}
	return f.text == "" || strings.Contains(strings.ToLower(e.Recap), f.text) || strings.Contains(strings.ToLower(e.Text), f.text)
}

func queryItemText(item Item, text string) bool {
	has := func(s string) bool { return strings.Contains(strings.ToLower(s), text) }
	return has(item.Title) || has(item.Recommendation) || has(item.Rationale) || has(item.Answer) || has(item.AnswerQuote) || slices.ContainsFunc(item.Options, has) || slices.ContainsFunc(item.Links, has)
}

func queryDiscovery(state *State, obj map[string]any) (QueryResult, []string) {
	p := &problems{}
	f := parseEntryQuery(p, obj, state)
	if len(p.List) > 0 {
		return QueryResult{}, p.List
	}
	r := QueryResult{Mode: "discovery", Last: f.last}
	if f.entries != "" {
		r.Mode = "stream"
	}
	if state == nil {
		return r, nil
	}
	items := append([]Item(nil), state.Items...)
	slices.SortFunc(items, func(a, b Item) int { return cmp.Compare(a.ID, b.ID) })
	var order []string
	add := func(key string) {
		if !slices.Contains(order, key) {
			order = append(order, key)
		}
	}
	for _, item := range items {
		add(item.Topic)
	}
	for _, doc := range state.Topics {
		add(doc.Name)
	}
	for _, name := range state.ClosedTopics {
		add(name)
	}
	for _, name := range order {
		if f.topics != nil && !f.topics[name] {
			continue
		}
		var all []Item
		textMatch := f.text == ""
		for _, item := range items {
			if item.Topic == name {
				all = append(all, item)
				if f.text != "" && queryItemText(item, f.text) {
					textMatch = true
				}
			}
		}
		var selected []Entry
		if index := state.topicDocIndex(name); index >= 0 {
			doc := state.Topics[index]
			if f.text != "" && strings.Contains(strings.ToLower(doc.Body), f.text) {
				textMatch = true
			}
			for _, entry := range doc.Entries {
				if f.matches(entry) {
					selected = append(selected, entry)
					textMatch = true
				}
			}
		}
		if !textMatch {
			continue
		}
		if state.topicClosed(name) && !f.closed {
			r.FoldedTopics++
			r.FoldedItems += len(all)
			continue
		}
		topic := makeQueryTopic(state, name, all, false)
		if r.Mode == "discovery" {
			// Discovery folds panel versions by stable pN after filtering, so a
			// text match in an older version still exposes its exact eN locator.
			latest := map[string]string{}
			panelCount := 0
			for _, e := range selected {
				if e.Kind == EntryPanel {
					latest[e.PanelID] = e.ID
					panelCount++
				}
			}
			var logs []Entry
			for _, e := range selected {
				if e.Kind != EntryPanel {
					logs = append(logs, e)
				} else if latest[e.PanelID] == e.ID {
					e.Text = ""
					topic.Panels = append(topic.Panels, e)
				}
			}
			topic.OmittedPanels = panelCount - len(topic.Panels)
			selected = logs
		}
		if len(selected) > f.last {
			topic.OmittedEntries = len(selected) - f.last
			selected = selected[len(selected)-f.last:]
		}
		for _, e := range selected {
			if e.Kind == EntryPanel {
				e.Text = ""
			}
			topic.Entries = append(topic.Entries, e)
		}
		r.Topics = append(r.Topics, topic)
	}
	return r, nil
}

func makeQueryTopic(state *State, name string, items []Item, body bool) QueryTopic {
	t := QueryTopic{Name: name, Label: state.TopicLabel(name), Closed: state.topicClosed(name), Counts: map[string]int{}}
	if state.stemmed(name) {
		t.Stem = name
	}
	if t.Closed {
		t.CloseResult = state.CloseResults[name]
	}
	var latest time.Time
	touch := func(at string) {
		if changed, err := time.Parse(time.RFC3339Nano, at); err == nil && changed.After(latest) {
			latest = changed
		}
	}
	for _, item := range items {
		t.Counts[item.Status]++
		touch(item.ChangedAt)
	}
	if index := state.topicDocIndex(name); index >= 0 {
		doc := state.Topics[index]
		t.BodyLen = utf8.RuneCountInString(doc.Body)
		if body {
			t.Body = doc.Body
		}
		touch(doc.ChangedAt)
		for _, e := range doc.Entries {
			touch(e.At)
		}
		if agent, ok := doc.LatestAgent(); ok {
			t.LatestAgent = &agent
		}
	}
	if !latest.IsZero() {
		t.LastChange = latest.UTC().Format(time.RFC3339Nano)
	}
	return t
}

func queryAt(at string) string {
	if at == "" {
		return "unknown-time"
	}
	return at
}

func writeQueryCurrent(sb *strings.Builder, e *Entry) {
	if e != nil {
		provenance := ""
		if e.Imported {
			provenance = " [imported]"
		}
		line := fmt.Sprintf("current %s%s %s: %s", e.ID, provenance, queryAt(e.At), e.Text)
		sb.WriteString(strings.ReplaceAll(line, "\n", "\n  ") + "\n")
	}
}

func formatEntryQuery(r QueryResult) string {
	if r.Direct != nil {
		e := r.Direct.Entry
		header := fmt.Sprintf("entry %s topic %q [%s] %s", e.ID, r.Direct.Topic, e.Kind, queryAt(e.At))
		if r.Mode == "panel" {
			header = fmt.Sprintf("panel %s entry %s topic %q %s", e.PanelID, e.ID, r.Direct.Topic, queryAt(e.At))
		}
		if e.Imported {
			header += " [imported]"
		}
		return header + "\n" + e.Text
	}
	var sb strings.Builder
	if len(r.Topics) == 0 {
		sb.WriteString("no matching topics\n")
	}
	for _, topic := range r.Topics {
		sb.WriteString(topicLine(topic) + "\n")
		writeQueryCurrent(&sb, topic.LatestAgent)
		for _, e := range topic.Entries {
			kind, text := e.Kind, e.Text
			if e.Kind == EntryPanel {
				kind, text = "panel "+e.PanelID, e.Recap
			}
			provenance := ""
			if e.Imported {
				provenance = " [imported]"
			}
			writeIndented(&sb, "  ", fmt.Sprintf("%s [%s]%s %s: %s", e.ID, kind, provenance, queryAt(e.At), text))
		}
		for _, panel := range topic.Panels {
			writeIndented(&sb, "  ", fmt.Sprintf("panel %s (%s) %s: %s", panel.PanelID, panel.ID, queryAt(panel.At), panel.Recap))
		}
		var omitted []string
		if topic.OmittedEntries > 0 {
			omitted = append(omitted, fmt.Sprintf("%d matching entries (last: %d)", topic.OmittedEntries, r.Last))
		}
		if topic.OmittedPanels > 0 {
			omitted = append(omitted, fmt.Sprintf("%d older matching panel versions (entries: panel)", topic.OmittedPanels))
		}
		if len(omitted) > 0 {
			sb.WriteString("  omitted: " + strings.Join(omitted, "; ") + "\n")
		}
	}
	if line := omittedLine(r); line != "" {
		sb.WriteString(line + "\n")
	}
	return sb.String()
}
