package wswhiteboard

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Query depths: one line per topic, then one line per item under its topic,
// then the item's detail lines. Summary sits between items and full: an
// open item prints its full ask, a settled one only its answer.
const (
	DepthTopics  = "topics"
	DepthItems   = "items"
	DepthSummary = "summary"
	DepthFull    = "full"
)

var (
	queryFields       = []string{"topic", "ids", "id", "status", "text", "depth", "fields", "body", "closed"}
	depthValues       = []string{DepthTopics, DepthItems, DepthSummary, DepthFull}
	detailFieldValues = []string{"options", "recommendation", "rationale", "answer", "answered_at", "answer_quote", "links"}
)

// QueryTopic is one topic of a query result.
type QueryTopic struct {
	// Name is the topic's key; Stem repeats it for a stemmed topic and is
	// "" for a stemless one. Label is the name the topic shows.
	Name, Stem, Label string
	Closed            bool
	CloseResult       string
	// Counts counts every item of the topic by status, whatever the filters.
	Counts map[string]int
	// BodyLen is the body's length in characters; Body is the body itself,
	// set only when the query prints bodies.
	BodyLen int
	Body    string
	// Tickets are the topic's linked ticket stems, in the order written.
	Tickets []string
	// LatestAgent is the current-state line, never a panel or system event.
	LatestAgent *Entry
	// Entries contain selected stream metadata/log text; panel Text is never
	// populated here. Panels is discovery's latest matching version per pN.
	Entries, Panels               []Entry
	OmittedEntries, OmittedPanels int
	// LastChange is the latest change time of items, content, or entries,
	// RFC 3339 UTC, "" when none is recorded.
	LastChange string
	// Items are the matching items in ID order; none at depth topics.
	Items []Item
}

// QueryResult holds topic/item or stream metadata, one direct entry when
// requested, and counts explaining what the defaults left out.
type QueryResult struct {
	// Mode is empty for explicit legacy item/content queries.
	Mode   string
	Last   int
	Direct *QueryEntry
	Depth  string
	// Fields are the detail fields printed at depth full; nil prints all.
	Fields []string
	Topics []QueryTopic
	// Hidden counts by status the items of the listed topics that the
	// unfiltered default status (open only) left out, at depth items or full.
	Hidden map[string]int
	// FoldedTopics and FoldedItems count the closed topics left out, and
	// their items, when closed topics are not listed.
	FoldedTopics, FoldedItems int
	// Matches names, per item a text filter matched, the fields it matched
	// in; nil without a text filter.
	Matches map[int][]string
}

// queryFilter is a validated query payload with its defaults applied.
type queryFilter struct {
	topics, statuses map[string]bool // nil: no filter
	ids              map[int]bool    // nil: no filter
	text             string          // lowercased; "": no filter
	depth            string
	fields           []string
	body, closed     bool
	defaultStatus    bool // statuses came from the unfiltered default
}

// parseQuery validates payload and applies the per-filter defaults: they
// come from the most specific target present (ids or id, then topic, then
// text, then none), fields without depth means depth full, and an explicit
// depth, status, body, or closed always overrides them.
func parseQuery(p *problems, payload any) queryFilter {
	var f queryFilter
	obj, ok := payloadObject(p, payload)
	if !ok {
		return f
	}
	checkKnownFields(p, "payload", obj, queryFields)
	var topics, statuses []string
	if raw, present := obj["topic"]; present {
		topics = stringList(p, "payload.topic", raw, nil)
	}
	if raw, present := obj["status"]; present {
		statuses = stringList(p, "payload.status", raw, statusValues)
	}
	if raw, present := obj["fields"]; present {
		f.fields = stringList(p, "payload.fields", raw, detailFieldValues)
	}
	var ids []int
	if raw, present := obj["id"]; present {
		if id, ok := intValue(p, "payload.id", raw); ok {
			ids = append(ids, id)
		}
	}
	if raw, present := obj["ids"]; present {
		ids = append(ids, intList(p, "payload.ids", raw)...)
	}
	if raw, present := obj["text"]; present {
		if s, ok := requiredString(p, "payload.text", raw); ok {
			f.text = strings.ToLower(s)
		}
	}
	depth, hasDepth := "", false
	if raw, present := obj["depth"]; present {
		depth, hasDepth = requiredEnum(p, "payload.depth", raw, depthValues)
	}
	body, hasBody := boolField(p, obj, "body")
	closed, hasClosed := boolField(p, obj, "closed")

	_, hasID := obj["id"]
	_, hasIDs := obj["ids"]
	target := ""
	switch {
	case hasID || hasIDs:
		target = "ids"
	case len(topics) > 0:
		target = "topic"
	case f.text != "":
		target = "text"
	}
	f.topics = stringSet(topics)
	f.statuses = stringSet(statuses)
	if hasID || hasIDs {
		f.ids = map[int]bool{}
		for _, id := range ids {
			f.ids[id] = true
		}
	}
	f.depth = DepthItems
	if target == "ids" || target == "topic" {
		f.depth = DepthSummary
	}
	if _, hasFields := obj["fields"]; hasFields {
		// fields selects detail lines at depth full only; any other default
		// depth would silently drop the fields the caller asked for.
		f.depth = DepthFull
	}
	if hasDepth {
		f.depth = depth
	}
	if f.statuses == nil && target == "" {
		f.statuses = map[string]bool{StatusOpen: true}
		f.defaultStatus = true
	}
	// Current content is retrieved only by explicit body:true, at every depth.
	if hasBody {
		f.body = body
	}
	f.closed = target != ""
	if hasClosed {
		f.closed = closed
	}
	return f
}

// queryItems retains explicit item/content queries: filters AND together
// (id and ids union), topics in first-item order with content-only topics
// after them, and items in ID order.
func queryItems(state *State, payload any) (QueryResult, []string) {
	p := &problems{}
	f := parseQuery(p, payload)
	if len(p.List) > 0 {
		return QueryResult{}, p.List
	}
	r := QueryResult{Depth: f.depth, Fields: f.fields}
	if state == nil {
		return r, nil
	}
	if f.topics != nil {
		resolved := map[string]bool{}
		for name := range f.topics {
			resolved[state.resolveTopic(name)] = true
		}
		f.topics = resolved
	}
	items := append([]Item(nil), state.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	var order []string
	seen := map[string]bool{}
	addTopic := func(name string) {
		if !seen[name] {
			seen[name] = true
			order = append(order, name)
		}
	}
	for _, item := range items {
		addTopic(item.Topic)
	}
	for _, doc := range state.Topics {
		addTopic(doc.Name)
	}
	// Only a query that names items (ids or text) drops a topic none of
	// whose items match; otherwise every topic the filters reach is listed.
	namesItems := f.ids != nil || f.text != ""
	for _, name := range order {
		if f.topics != nil && !f.topics[name] {
			continue
		}
		var all, matched, hidden []Item
		for _, item := range items {
			if item.Topic != name {
				continue
			}
			all = append(all, item)
			fields, ok := f.matches(item)
			if !ok {
				continue
			}
			if f.statuses != nil && !f.statuses[item.Status] {
				hidden = append(hidden, item)
				continue
			}
			matched = append(matched, item)
			if fields != nil {
				if r.Matches == nil {
					r.Matches = map[int][]string{}
				}
				r.Matches[item.ID] = fields
			}
		}
		if namesItems && len(matched) == 0 {
			continue
		}
		closed := state.topicClosed(name)
		if closed && !f.closed {
			r.FoldedTopics++
			r.FoldedItems += len(all)
			continue
		}
		t := makeQueryTopic(state, name, all, f.body)
		if f.depth != DepthTopics {
			t.Items = matched
			if f.defaultStatus {
				for _, item := range hidden {
					if r.Hidden == nil {
						r.Hidden = map[string]int{}
					}
					r.Hidden[item.Status]++
				}
			}
		}
		r.Topics = append(r.Topics, t)
	}
	return r, nil
}

// matches applies every item filter but status: ids and text. With a text
// filter it also returns the fields the text matched in, in a fixed order.
// Matching reads the stored text, markup included.
func (f queryFilter) matches(item Item) ([]string, bool) {
	if f.ids != nil && !f.ids[item.ID] {
		return nil, false
	}
	if f.text == "" {
		return nil, true
	}
	has := func(s string) bool { return strings.Contains(strings.ToLower(s), f.text) }
	var fields []string
	if has(item.Title) {
		fields = append(fields, "title")
	}
	if slices.ContainsFunc(item.Options, has) {
		fields = append(fields, "options")
	}
	if has(item.Rationale) {
		fields = append(fields, "rationale")
	}
	if has(item.Answer) {
		fields = append(fields, "answer")
	}
	return fields, len(fields) > 0
}

// FormatQuery renders compact discovery/stream metadata or the retained
// explicit item/content tree. Current-state lines and explicit content print
// as stored; item markup follows Plain/PlainHTML and owner quotes stay exact.
// Direct retrieval emits one header followed by the untouched payload bytes.
func FormatQuery(r QueryResult) string {
	if r.Mode != "" {
		return formatEntryQuery(r)
	}
	var sb strings.Builder
	if len(r.Topics) == 0 {
		sb.WriteString("no matching items\n")
	}
	// quoted maps a printed answer quote and its answer time to the item
	// that printed it, so a grouped answer's quote prints once.
	quoted := map[[2]string]int{}
	for _, t := range r.Topics {
		sb.WriteString(topicLine(t) + "\n")
		writeQueryCurrent(&sb, t.LatestAgent)
		if t.Body != "" {
			sb.WriteString("topic body:\n<<<\n" + t.Body + "\n>>>\n")
		}
		for _, item := range t.Items {
			line := fmt.Sprintf("%s [%s] [%s] %s", FormatID(item.ID), item.Status, item.Class, Plain(item.Title))
			if fields := r.Matches[item.ID]; len(fields) > 0 {
				line += " [match: " + strings.Join(fields, ", ") + "]"
			}
			writeIndented(&sb, "  ", line)
			switch r.Depth {
			case DepthFull:
				writeDetails(&sb, item, r.Fields, quoted)
			case DepthSummary:
				writeSummary(&sb, item, quoted)
			}
		}
	}
	if line := omittedLine(r); line != "" {
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

func topicLine(t QueryTopic) string {
	state := "open"
	if t.Closed {
		state = "closed"
	}
	parts := []string{"items: " + statusCounts(t.Counts)}
	if t.BodyLen > 0 {
		parts = append(parts, fmt.Sprintf("body: %d chars", t.BodyLen))
	}
	if t.LastChange != "" {
		parts = append(parts, "changed: "+t.LastChange)
	}
	if t.CloseResult != "" {
		parts = append(parts, "result: "+Plain(t.CloseResult))
	}
	if len(t.Tickets) > 0 {
		parts = append(parts, "tickets: "+strings.Join(t.Tickets, " "))
	}
	name := fmt.Sprintf("%q", Plain(t.Label))
	if t.Stem != "" {
		name = t.Stem + " " + name
	}
	return fmt.Sprintf("topic %s [%s] %s", name, state, strings.Join(parts, "; "))
}

// statusCounts lists the non-zero counts in status order, "none" when all
// are zero.
func statusCounts(counts map[string]int) string {
	var parts []string
	for _, status := range statusValues {
		if n := counts[status]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", status, n))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

// writeIndented writes line at indent, every continuation line of a
// multi-line value at the same indent, so it never reads as a line of its
// own at column 0.
func writeIndented(sb *strings.Builder, indent, line string) {
	sb.WriteString(indent + strings.ReplaceAll(line, "\n", "\n"+indent) + "\n")
}

// writeDetails writes an item's detail lines at depth full, the fields
// selected (all for nil).
func writeDetails(sb *strings.Builder, item Item, fields []string, quoted map[[2]string]int) {
	want := func(name string) bool { return fields == nil || slices.Contains(fields, name) }
	detail := func(name, value string) {
		if value != "" {
			writeIndented(sb, "    ", name+": "+value)
		}
	}
	if want("options") {
		for i, option := range item.Options {
			writeIndented(sb, "    ", OptionLetterFor(i)+") "+PlainHTML(option))
		}
	}
	if want("recommendation") {
		detail("recommendation", PlainHTML(item.Recommendation))
	}
	if want("rationale") {
		detail("rationale", PlainHTML(item.Rationale))
	}
	if want("answer") {
		detail("answer", Plain(item.Answer))
	}
	if want("answer_quote") && item.AnswerQuote != "" {
		key := [2]string{item.AnswerQuote, item.AnsweredAt}
		if first, ok := quoted[key]; ok {
			detail("answer_quote", "same as "+FormatID(first))
		} else {
			quoted[key] = item.ID
			detail("answer_quote", fmt.Sprintf("%q", item.AnswerQuote))
		}
	}
	if want("answered_at") {
		detail("answered_at", item.AnsweredAt)
	}
	if want("links") && len(item.Links) > 0 {
		links := make([]string, len(item.Links))
		for i, link := range item.Links {
			links[i] = Plain(link)
		}
		detail("links", strings.Join(links, ", "))
	}
}

// writeSummary writes an item's detail lines at depth summary: an open
// item's full ask, an answered or assumed item's one decoded answer line,
// and nothing for a withdrawn or superseded one.
func writeSummary(sb *strings.Builder, item Item, quoted map[[2]string]int) {
	switch item.Status {
	case StatusOpen:
		writeDetails(sb, item, nil, quoted)
	case StatusAnswered, StatusAssumed:
		line := "answer: " + decodedAnswer(item)
		if item.Status == StatusAssumed {
			line += " (assumed)"
		}
		writeIndented(sb, "    ", line)
	}
}

// decodedAnswer reads a policy answer that starts with an option letter as
// "<letter>: <option text>", then " - <note>" when the value carries text
// after the letter; any other value prints as stored.
func decodedAnswer(item Item) string {
	if item.Class == ClassPolicy && len(item.Options) > 0 {
		if letter, note := SplitLetter(item.Answer, len(item.Options)); letter != "" {
			out := letter + ": " + PlainHTML(item.Options[letter[0]-'a'])
			if note != "" {
				out += " - " + Plain(note)
			}
			return out
		}
	}
	return Plain(item.Answer)
}

// omittedLine names what the defaults left out and the filter that shows
// it, so an omission is not read as absence; "" when nothing was.
func omittedLine(r QueryResult) string {
	var parts []string
	if n := sumCounts(r.Hidden); n > 0 {
		parts = append(parts, fmt.Sprintf("%s not open (%s), shown by status, topic, or ids", plural(n, "item"), statusCounts(r.Hidden)))
	}
	if r.FoldedTopics > 0 {
		parts = append(parts, fmt.Sprintf("%s with %s, listed by closed: true or topic", plural(r.FoldedTopics, "closed topic"), plural(r.FoldedItems, "item")))
	}
	if len(parts) == 0 {
		return ""
	}
	return "omitted: " + strings.Join(parts, "; ")
}

// plural counts n of noun, adding an s past one.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func sumCounts(counts map[string]int) int {
	n := 0
	for _, c := range counts {
		n += c
	}
	return n
}

func stringSet(list []string) map[string]bool {
	if len(list) == 0 {
		return nil
	}
	set := make(map[string]bool, len(list))
	for _, s := range list {
		set[s] = true
	}
	return set
}

// --- payload value checks ------------------------------------------------

func payloadObject(p *problems, payload any) (map[string]any, bool) {
	switch v := payload.(type) {
	case nil:
		return map[string]any{}, true
	case map[string]any:
		return v, true
	case string:
		p.Add("payload: must be an object; got string (pass the object itself, not a JSON-encoded string)")
	default:
		p.Add("payload: must be an object; got %s", typeName(payload))
	}
	return nil, false
}

func checkKnownFields(p *problems, field string, obj map[string]any, known []string) {
	var unknown []string
	for name := range obj {
		if !slices.Contains(known, name) {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	for _, name := range unknown {
		p.Add("%s.%s: unknown field (known: %s)", field, name, strings.Join(known, ", "))
	}
}

// stringList reads one non-empty string or a non-empty list of them; with
// values, each must be one of values. A problem drops the value.
func stringList(p *problems, field string, raw any, values []string) []string {
	check := func(field string, raw any) (string, bool) {
		if values != nil {
			return requiredEnum(p, field, raw, values)
		}
		return requiredString(p, field, raw)
	}
	switch v := raw.(type) {
	case string:
		if s, ok := check(field, v); ok {
			return []string{s}
		}
		return nil
	case []any:
		if len(v) == 0 {
			p.Add("%s: must list at least one value", field)
			return nil
		}
		var out []string
		for i, elem := range v {
			if s, ok := check(fmt.Sprintf("%s[%d]", field, i), elem); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		p.Add("%s: must be a string or a list of strings; got %s", field, typeName(raw))
		return nil
	}
}

// intList reads a non-empty list of whole numbers.
func intList(p *problems, field string, raw any) []int {
	list, ok := raw.([]any)
	if !ok {
		p.Add("%s: must be a list of numbers; got %s", field, typeName(raw))
		return nil
	}
	if len(list) == 0 {
		p.Add("%s: must list at least one id", field)
		return nil
	}
	var out []int
	for i, elem := range list {
		if n, ok := intValue(p, fmt.Sprintf("%s[%d]", field, i), elem); ok {
			out = append(out, n)
		}
	}
	return out
}

// boolField reads an optional boolean field and whether it was given.
func boolField(p *problems, obj map[string]any, name string) (bool, bool) {
	raw, present := obj[name]
	if !present {
		return false, false
	}
	b, ok := raw.(bool)
	if !ok {
		p.Add("payload.%s: must be a boolean; got %s", name, typeName(raw))
		return false, false
	}
	return b, true
}

func stringValue(p *problems, field string, raw any) (string, bool) {
	s, ok := raw.(string)
	if !ok {
		p.Add("%s: must be a string; got %s", field, typeName(raw))
		return "", false
	}
	return s, true
}

func requiredString(p *problems, field string, raw any) (string, bool) {
	if raw == nil {
		p.Add("%s: required", field)
		return "", false
	}
	s, ok := stringValue(p, field, raw)
	if !ok {
		return "", false
	}
	if strings.TrimSpace(s) == "" {
		p.Add("%s: must be non-empty", field)
		return "", false
	}
	return s, true
}

func requiredEnum(p *problems, field string, raw any, values []string) (string, bool) {
	s, ok := requiredString(p, field, raw)
	if !ok {
		return "", false
	}
	if slices.Contains(values, s) {
		return s, true
	}
	p.Add("%s: must be one of %s; got %q", field, strings.Join(values, ", "), s)
	return "", false
}

func intValue(p *problems, field string, raw any) (int, bool) {
	n, ok := raw.(float64)
	if !ok {
		p.Add("%s: must be a number; got %s", field, typeName(raw))
		return 0, false
	}
	if n != float64(int(n)) {
		p.Add("%s: must be a whole number; got %v", field, n)
		return 0, false
	}
	return int(n), true
}

// typeName names a decoded JSON value's type for a type-accurate message.
func typeName(value any) string {
	switch value.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", value)
	}
}
