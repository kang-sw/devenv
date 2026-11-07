// Package wswhiteboard is the session whiteboard: the item store a lead
// session keeps in its state record, the validation and application of a
// whiteboard.update payload, the read-only query, the view model both pages
// render from, and the page templates. It does no file I/O of its own; the
// MCP layer owns persistence, the rendered files, and their lifecycle.
package wswhiteboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// Item classes, the workflow manual's asking classes.
const (
	ClassPolicy    = "policy"
	ClassTechnical = "technical"
	ClassAnnounced = "announced"
)

// Item statuses. Answered, withdrawn, and superseded items are settled;
// open and assumed items still wait on the user.
const (
	StatusOpen       = "open"
	StatusAnswered   = "answered"
	StatusAssumed    = "assumed"
	StatusWithdrawn  = "withdrawn"
	StatusSuperseded = "superseded"
)

const (
	// ExpandedSettled is how many of the most recently settled items render
	// expanded inside their topic blocks, counted board-wide.
	ExpandedSettled = 10
	// ArchiveCap bounds the settled items a session keeps; the oldest settled
	// item is evicted first and is no longer queryable.
	ArchiveCap = 500
	// EmbedRetention is how many embed snapshots a session keeps in the
	// repository whiteboard folder. Embeds written to a host embed folder are
	// never deleted by the server.
	EmbedRetention = 5
)

var (
	classValues       = []string{ClassPolicy, ClassTechnical, ClassAnnounced}
	statusValues      = []string{StatusOpen, StatusAnswered, StatusAssumed, StatusWithdrawn, StatusSuperseded}
	patchStatusValues = []string{StatusOpen, StatusWithdrawn, StatusSuperseded}
	// fullDocumentPattern detects a whole HTML document where the Now slot
	// takes markdown with inline HTML; fullDocument applies it to the raw
	// HTML only.
	fullDocumentPattern = regexp.MustCompile(`(?i)<!doctype|<html[\s>]`)
	// nowParser parses Now markdown as renderNow's pipeline does (default
	// block and inline parsers, raw HTML on, tables and strikethrough).
	nowParser = goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough)).Parser()
)

// fullDocument reports whether Now markdown carries a whole HTML document:
// a <!DOCTYPE or <html in its raw HTML (an HTML block or inline raw HTML).
// Text inside a code span or fence that only mentions one is not raw HTML,
// so a Now that explains HTML is accepted.
func fullDocument(content string) bool {
	if !fullDocumentPattern.MatchString(content) {
		return false
	}
	src := []byte(content)
	found := false
	_ = ast.Walk(nowParser.Parse(text.NewReader(src)), func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var raw bytes.Buffer
		switch n := n.(type) {
		case *ast.HTMLBlock:
			for i := 0; i < n.Lines().Len(); i++ {
				seg := n.Lines().At(i)
				raw.Write(seg.Value(src))
			}
			if n.HasClosure() {
				raw.Write(n.ClosureLine.Value(src))
			}
		case *ast.RawHTML:
			for i := 0; i < n.Segments.Len(); i++ {
				seg := n.Segments.At(i)
				raw.Write(seg.Value(src))
			}
		default:
			return ast.WalkContinue, nil
		}
		if fullDocumentPattern.Match(raw.Bytes()) {
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkSkipChildren, nil
	})
	return found
}

// Item is one asked item on the board.
type Item struct {
	ID             int      `json:"id"`
	Class          string   `json:"class"`
	Topic          string   `json:"topic"`
	Title          string   `json:"title"`
	Options        []string `json:"options,omitempty"`
	Recommendation string   `json:"recommendation,omitempty"`
	Rationale      string   `json:"rationale,omitempty"`
	Status         string   `json:"status"`
	Answer         string   `json:"answer,omitempty"`
	AnswerQuote    string   `json:"answer_quote,omitempty"`
	AnsweredAt     string   `json:"answered_at,omitempty"`
	Links          []string `json:"links,omitempty"`
	// Changed is the render sequence of the update that last changed the
	// item; the embed marks an item new when it changed after the previous
	// embed.
	Changed int `json:"changed"`
	// Settled is the item's settle order (a board-wide counter), zero while
	// the item is open or assumed. Rotation and archive eviction order by
	// it, never by time.
	Settled int `json:"settled,omitempty"`
}

// Embed records one embed snapshot written to the repository folder, so
// retention evicts by render sequence rather than file mtime.
type Embed struct {
	Seq  int    `json:"seq"`
	Name string `json:"name"`
}

// State is the whole whiteboard of one session. The MCP layer stores it in
// the session state record; a nil State means the session has no board.
type State struct {
	// Counter is the highest ID the store has minted, adopted, or been
	// floored to; a minted ID is Counter+1.
	Counter int `json:"counter"`
	// Seq is the render sequence: every render (an update or an implicit
	// todo/agenda refresh) increments it.
	Seq int `json:"seq"`
	// SettleSeq is the last settle order handed out.
	SettleSeq    int      `json:"settle_seq,omitempty"`
	Items        []Item   `json:"items,omitempty"`
	ClosedTopics []string `json:"closed_topics,omitempty"`
	Now          string   `json:"now,omitempty"`
	// NowSeq is the render sequence of the update that last changed Now.
	NowSeq int `json:"now_seq,omitempty"`
	// UpdateSeq is the render sequence of the latest accepted update, so an
	// implicit todo/agenda render does not make Now stale. Zero in a store
	// written before the field existed.
	UpdateSeq int `json:"update_seq,omitempty"`
	// LastEmbedSeq is the render sequence of the previous embed snapshot.
	LastEmbedSeq int     `json:"last_embed_seq,omitempty"`
	Embeds       []Embed `json:"embeds,omitempty"`
	// EmbedDir is the host embed folder, empty when embeds go to the
	// repository folder.
	EmbedDir string `json:"embed_dir,omitempty"`
}

// Env carries the side effects Apply needs, so the store stays testable.
type Env struct {
	Now time.Time
	// ReadFile reads a now_path markdown file once.
	ReadFile func(path string) ([]byte, error)
	// IsDir reports whether path is an existing directory (embed_dir).
	IsDir func(path string) bool
	// IsAbs reports whether path is absolute on the server's platform.
	IsAbs func(path string) bool
}

// Outcome is what an accepted update did.
type Outcome struct {
	// IDs holds the ID of every add entry, in add order.
	IDs []int
}

// FormatID renders an item ID as the board shows it: (nn), three digits
// past 99.
func FormatID(id int) string {
	return fmt.Sprintf("(%02d)", id)
}

// IsSettled reports whether status is a settled status.
func IsSettled(status string) bool {
	return status == StatusAnswered || status == StatusWithdrawn || status == StatusSuperseded
}

func (s *State) clone() *State {
	data, err := json.Marshal(s)
	if err != nil {
		panic(err) // State holds only JSON-safe values
	}
	var out State
	if err := json.Unmarshal(data, &out); err != nil {
		panic(err)
	}
	return &out
}

func (s *State) index(id int) int {
	for i := range s.Items {
		if s.Items[i].ID == id {
			return i
		}
	}
	return -1
}

func (s *State) topicKnown(topic string) bool {
	for _, item := range s.Items {
		if item.Topic == topic {
			return true
		}
	}
	return false
}

func (s *State) topicClosed(topic string) bool {
	for _, closed := range s.ClosedTopics {
		if closed == topic {
			return true
		}
	}
	return false
}

func (s *State) reopenTopic(topic string) {
	kept := s.ClosedTopics[:0]
	for _, closed := range s.ClosedTopics {
		if closed != topic {
			kept = append(kept, closed)
		}
	}
	s.ClosedTopics = kept
}

func (s *State) settle(item *Item) {
	s.SettleSeq++
	item.Settled = s.SettleSeq
}

// BeginRender advances the render sequence for a render that changes no item
// (the implicit todo and agenda refresh).
func (s *State) BeginRender() {
	s.Seq++
}

// NowFresh reports whether the latest update changed Now. A store without
// UpdateSeq judges against the current render, as it did before the field.
func (s *State) NowFresh() bool {
	latest := s.UpdateSeq
	if latest == 0 {
		latest = s.Seq
	}
	return s.Now != "" && s.NowSeq == latest
}

// evictArchive drops the oldest settled items beyond ArchiveCap.
func (s *State) evictArchive() {
	settled := 0
	for _, item := range s.Items {
		if IsSettled(item.Status) {
			settled++
		}
	}
	if settled <= ArchiveCap {
		return
	}
	orders := make([]int, 0, settled)
	for _, item := range s.Items {
		if IsSettled(item.Status) {
			orders = append(orders, item.Settled)
		}
	}
	sort.Ints(orders)
	cutoff := orders[settled-ArchiveCap-1] // evict every settle order <= cutoff
	kept := s.Items[:0]
	for _, item := range s.Items {
		if IsSettled(item.Status) && item.Settled <= cutoff {
			continue
		}
		kept = append(kept, item)
	}
	s.Items = kept
}

// Apply validates payload as one whiteboard.update batch and applies it to
// state. The batch is validated whole: any problem leaves state untouched and
// every problem is returned. An accepted batch applies in the order
// embed_dir, id_floor, add, answer, patch, close_topics, now, and advances
// the render sequence once; the caller renders afterwards.
func Apply(state *State, payload any, env Env) (Outcome, []string) {
	p := &problems{}
	obj, ok := payloadObject(p, payload)
	if !ok {
		return Outcome{}, p.list
	}
	checkKnownFields(p, "payload", obj, updateFields)

	work := state.clone()
	work.Seq++
	mark := work.Seq
	work.UpdateSeq = mark
	var out Outcome

	if raw, present := obj["embed_dir"]; present {
		if dir, ok := stringValue(p, "payload.embed_dir", raw); ok {
			switch {
			case dir == "":
				work.EmbedDir = ""
			case !env.IsAbs(dir):
				p.add("payload.embed_dir: must be an absolute path; got %q", dir)
			case !env.IsDir(dir):
				p.add("payload.embed_dir: %s is not an existing directory (the server never creates it)", dir)
			default:
				work.EmbedDir = dir
			}
		}
	} else if work.EmbedDir != "" && !env.IsDir(work.EmbedDir) {
		p.add("embed_dir %s (stored for this session) is no longer an existing directory; pass embed_dir again with the host's current folder, or \"\" to clear it", work.EmbedDir)
	}

	if raw, present := obj["id_floor"]; present {
		if floor, ok := intValue(p, "payload.id_floor", raw); ok && floor > work.Counter {
			work.Counter = floor
		}
	}

	for i, entry := range objectList(p, "payload.add", obj["add"]) {
		field := fmt.Sprintf("payload.add[%d]", i)
		out.IDs = append(out.IDs, applyAdd(p, work, field, entry, mark))
	}
	for i, entry := range objectList(p, "payload.answer", obj["answer"]) {
		applyAnswer(p, work, fmt.Sprintf("payload.answer[%d]", i), entry, mark, env.Now)
	}
	for i, entry := range objectList(p, "payload.patch", obj["patch"]) {
		applyPatch(p, work, fmt.Sprintf("payload.patch[%d]", i), entry, mark)
	}
	if raw, present := obj["close_topics"]; present {
		for i, topic := range stringList(p, "payload.close_topics", raw) {
			switch {
			case strings.TrimSpace(topic) == "":
				p.add("payload.close_topics[%d]: must be a non-empty topic", i)
			case !work.topicKnown(topic):
				p.add("payload.close_topics[%d]: no item has topic %q", i, topic)
			case !work.topicClosed(topic):
				work.ClosedTopics = append(work.ClosedTopics, topic)
			}
		}
	}

	_, hasNow := obj["now"]
	_, hasNowPath := obj["now_path"]
	if hasNow && hasNowPath {
		p.add("payload: now and now_path are exclusive; pass one")
	} else if hasNow || hasNowPath {
		var content string
		var ok bool
		if hasNow {
			content, ok = stringValue(p, "payload.now", obj["now"])
		} else if path, pathOK := stringValue(p, "payload.now_path", obj["now_path"]); pathOK {
			data, err := env.ReadFile(path)
			if err != nil {
				p.add("payload.now_path: cannot read %s: %v", path, err)
			} else {
				content, ok = string(data), true
			}
		}
		if ok {
			if fullDocument(content) {
				p.add("payload.now: the Now slot takes markdown with inline HTML, not a full document (found <!DOCTYPE or <html)")
			} else if content != work.Now {
				work.Now = content
				if content == "" {
					work.NowSeq = 0
				} else {
					work.NowSeq = mark
				}
			}
		}
	}

	if len(p.list) > 0 {
		return Outcome{}, p.list
	}
	work.evictArchive()
	*state = *work
	return out, nil
}

var (
	updateFields = []string{"add", "answer", "patch", "close_topics", "now", "now_path", "id_floor", "embed_dir"}
	addFields    = []string{"id", "class", "topic", "title", "options", "recommendation", "rationale", "links"}
	answerFields = []string{"id", "answer", "answer_quote", "assumed"}
	patchFields  = []string{"id", "title", "options", "recommendation", "rationale", "links", "status"}
	queryFields  = []string{"topic", "status", "id"}
)

func applyAdd(p *problems, work *State, field string, entry map[string]any, mark int) int {
	checkKnownFields(p, field, entry, addFields)
	item := Item{Status: StatusOpen, Changed: mark}
	before := len(p.list)
	if raw, present := entry["id"]; present {
		if id, ok := intValue(p, field+".id", raw); ok {
			if id < 1 {
				p.add("%s.id: must be a positive number; got %d", field, id)
			} else if work.index(id) >= 0 {
				p.add("%s.id: %s is already in the store", field, FormatID(id))
			} else {
				item.ID = id
			}
		}
	}
	item.Class, _ = requiredEnum(p, field+".class", entry["class"], classValues)
	item.Topic, _ = requiredString(p, field+".topic", entry["topic"])
	item.Title, _ = requiredString(p, field+".title", entry["title"])
	if raw, present := entry["options"]; present {
		item.Options = optionList(p, field+".options", raw)
	}
	if raw, present := entry["rationale"]; present {
		item.Rationale, _ = stringValue(p, field+".rationale", raw)
	}
	if raw, present := entry["links"]; present {
		item.Links = stringList(p, field+".links", raw)
	}
	if raw, present := entry["recommendation"]; present {
		item.Recommendation, _ = stringValue(p, field+".recommendation", raw)
	}
	checkPolicyShape(p, field, item)
	if len(p.list) > before {
		return item.ID
	}
	if item.ID == 0 {
		work.Counter++
		item.ID = work.Counter
	} else if item.ID > work.Counter {
		work.Counter = item.ID
	}
	if work.topicClosed(item.Topic) {
		work.reopenTopic(item.Topic)
	}
	work.Items = append(work.Items, item)
	return item.ID
}

func applyAnswer(p *problems, work *State, field string, entry map[string]any, mark int, now time.Time) {
	checkKnownFields(p, field, entry, answerFields)
	idx := lookupItem(p, work, field, entry)
	answer, answerOK := requiredString(p, field+".answer", entry["answer"])
	quote, quoteOK := requiredString(p, field+".answer_quote", entry["answer_quote"])
	assumed := false
	if raw, present := entry["assumed"]; present {
		assumed, _ = boolValue(p, field+".assumed", raw)
	}
	if idx < 0 || !answerOK || !quoteOK {
		return
	}
	item := &work.Items[idx]
	switch item.Status {
	case StatusWithdrawn, StatusSuperseded:
		p.add("%s: %s is %s; patch its status to open before answering it", field, FormatID(item.ID), item.Status)
		return
	}
	if item.Class == ClassPolicy && len(item.Options) > 0 && optionLetter(answer, len(item.Options)) == "" {
		p.add("%s.answer: a policy answer starts with its option letter (%s); got %q", field, letterRange(len(item.Options)), answer)
		return
	}
	item.Answer = answer
	item.AnswerQuote = quote
	item.AnsweredAt = now.UTC().Format(time.RFC3339)
	item.Changed = mark
	if assumed {
		item.Status = StatusAssumed
		item.Settled = 0
		return
	}
	item.Status = StatusAnswered
	work.settle(item)
}

func applyPatch(p *problems, work *State, field string, entry map[string]any, mark int) {
	checkKnownFields(p, field, entry, patchFields)
	idx := lookupItem(p, work, field, entry)
	if idx < 0 {
		// Still type-check the remaining fields so the refusal lists them.
		for _, name := range []string{"title", "recommendation", "rationale"} {
			if raw, present := entry[name]; present {
				stringValue(p, field+"."+name, raw)
			}
		}
		return
	}
	before := len(p.list)
	next := work.Items[idx]
	if raw, present := entry["title"]; present {
		if title, ok := requiredString(p, field+".title", raw); ok {
			next.Title = title
		}
	}
	if raw, present := entry["options"]; present {
		if next.Status != StatusOpen {
			p.add("%s.options: %s is %s; options change only on an open item, since its answer is stored as an option letter", field, FormatID(next.ID), next.Status)
		} else {
			next.Options = optionList(p, field+".options", raw)
		}
	}
	if raw, present := entry["recommendation"]; present {
		next.Recommendation, _ = stringValue(p, field+".recommendation", raw)
	}
	if raw, present := entry["rationale"]; present {
		next.Rationale, _ = stringValue(p, field+".rationale", raw)
	}
	if raw, present := entry["links"]; present {
		next.Links = stringList(p, field+".links", raw)
	}
	status := ""
	if raw, present := entry["status"]; present {
		status, _ = requiredEnum(p, field+".status", raw, patchStatusValues)
	}
	checkPolicyShape(p, field, next)
	if len(p.list) > before {
		return
	}
	next.Changed = mark
	if status != "" && status != next.Status {
		next.Status = status
		if status == StatusOpen {
			// Reopening asks the item again: its earlier answer no longer
			// stands. Withdrawn and superseded items keep theirs as history.
			next.Settled = 0
			next.Answer, next.AnswerQuote, next.AnsweredAt = "", "", ""
		} else {
			work.settle(&next)
		}
	}
	work.Items[idx] = next
}

// checkPolicyShape checks what a policy item's options and recommendation
// must satisfy.
func checkPolicyShape(p *problems, field string, item Item) {
	if item.Recommendation == "" {
		return
	}
	if len(item.Options) == 0 {
		p.add("%s.recommendation: names an option letter, so it needs options", field)
		return
	}
	if optionLetter(item.Recommendation, len(item.Options)) == "" {
		p.add("%s.recommendation: starts with its option letter (%s), optionally followed by \": <why>\"; got %q", field, letterRange(len(item.Options)), item.Recommendation)
	}
}

func lookupItem(p *problems, work *State, field string, entry map[string]any) int {
	raw, present := entry["id"]
	if !present {
		p.add("%s.id: required", field)
		return -1
	}
	id, ok := intValue(p, field+".id", raw)
	if !ok {
		return -1
	}
	idx := work.index(id)
	if idx < 0 {
		p.add("%s.id: %s is not in the store", field, FormatID(id))
	}
	return idx
}

// OptionLetterFor returns the letter of the option at index i (a, b, c...).
func OptionLetterFor(i int) string {
	return string(rune('a' + i))
}

// optionLetter returns the option letter s starts with when it is followed by
// the end of s or a non-letter, and the letter names one of count options.
func optionLetter(s string, count int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	letter := strings.ToLower(s[:1])
	if letter < "a" || letter > "z" || int(letter[0]-'a') >= count {
		return ""
	}
	if len(s) > 1 {
		next := s[1]
		if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') {
			return ""
		}
	}
	return letter
}

// SplitLetter splits an answer or recommendation into its option letter and
// the remaining text, trimmed of the separator.
func SplitLetter(s string, count int) (string, string) {
	letter := optionLetter(s, count)
	if letter == "" {
		return "", strings.TrimSpace(s)
	}
	rest := strings.TrimSpace(s)[1:]
	rest = strings.TrimLeft(rest, " :,;-–—)")
	return letter, strings.TrimSpace(rest)
}

func letterRange(count int) string {
	if count <= 1 {
		return "a"
	}
	return "a-" + OptionLetterFor(count-1)
}

// --- query ---------------------------------------------------------------

// Query returns the items matching payload's filters, archived settled items
// included, in ID order.
func Query(state *State, payload any) ([]Item, []string) {
	p := &problems{}
	obj, ok := payloadObject(p, payload)
	if !ok {
		return nil, p.list
	}
	checkKnownFields(p, "payload", obj, queryFields)
	topic, status, id := "", "", 0
	if raw, present := obj["topic"]; present {
		topic, _ = stringValue(p, "payload.topic", raw)
	}
	if raw, present := obj["status"]; present {
		status, _ = requiredEnum(p, "payload.status", raw, statusValues)
	}
	if raw, present := obj["id"]; present {
		id, _ = intValue(p, "payload.id", raw)
	}
	if len(p.list) > 0 {
		return nil, p.list
	}
	if state == nil {
		return nil, nil
	}
	var out []Item
	for _, item := range state.Items {
		if (topic != "" && item.Topic != topic) || (status != "" && item.Status != status) || (id != 0 && item.ID != id) {
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// FormatItems renders query results as compact text, one header line per
// item and an indented line per non-empty detail.
func FormatItems(items []Item) string {
	if len(items) == 0 {
		return "no matching items\n"
	}
	var sb strings.Builder
	for _, item := range items {
		fmt.Fprintf(&sb, "%s [%s] [%s] %s: %s\n", FormatID(item.ID), item.Status, item.Class, item.Topic, item.Title)
		for i, option := range item.Options {
			fmt.Fprintf(&sb, "  %s) %s\n", OptionLetterFor(i), option)
		}
		detail := func(name, value string) {
			if value != "" {
				fmt.Fprintf(&sb, "  %s: %s\n", name, value)
			}
		}
		detail("recommendation", item.Recommendation)
		detail("rationale", item.Rationale)
		detail("answer", item.Answer)
		if item.AnswerQuote != "" {
			detail("answer_quote", fmt.Sprintf("%q", item.AnswerQuote))
		}
		detail("answered_at", item.AnsweredAt)
		if len(item.Links) > 0 {
			detail("links", strings.Join(item.Links, ", "))
		}
	}
	return sb.String()
}

// --- payload parsing -----------------------------------------------------

type problems struct {
	list []string
}

func (p *problems) add(format string, args ...any) {
	p.list = append(p.list, fmt.Sprintf(format, args...))
}

func payloadObject(p *problems, payload any) (map[string]any, bool) {
	switch v := payload.(type) {
	case nil:
		return map[string]any{}, true
	case map[string]any:
		return v, true
	case string:
		p.add("payload: must be an object; got string (pass the object itself, not a JSON-encoded string)")
	default:
		p.add("payload: must be an object; got %s", typeName(payload))
	}
	return nil, false
}

func checkKnownFields(p *problems, field string, obj map[string]any, known []string) {
	var unknown []string
	for name := range obj {
		found := false
		for _, k := range known {
			if k == name {
				found = true
				break
			}
		}
		if !found {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	for _, name := range unknown {
		p.add("%s.%s: unknown field (known: %s)", field, name, strings.Join(known, ", "))
	}
}

// objectList returns the entries of an array-of-objects field, reporting a
// type-accurate problem for anything else. Entries that are not objects are
// reported and skipped.
func objectList(p *problems, field string, raw any) []map[string]any {
	if raw == nil {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		p.add("%s: must be an array of objects; got %s%s", field, typeName(raw), encodedHint(raw))
		return nil
	}
	out := make([]map[string]any, 0, len(list))
	for i, entry := range list {
		obj, ok := entry.(map[string]any)
		if !ok {
			p.add("%s[%d]: must be an object; got %s%s", field, i, typeName(entry), encodedHint(entry))
			continue
		}
		out = append(out, obj)
	}
	return out
}

func stringList(p *problems, field string, raw any) []string {
	if raw == nil {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		p.add("%s: must be an array of strings; got %s%s", field, typeName(raw), encodedHint(raw))
		return nil
	}
	out := make([]string, 0, len(list))
	for i, entry := range list {
		s, ok := entry.(string)
		if !ok {
			p.add("%s[%d]: must be a string; got %s", field, i, typeName(entry))
			continue
		}
		out = append(out, s)
	}
	return out
}

func optionList(p *problems, field string, raw any) []string {
	options := stringList(p, field, raw)
	for i, option := range options {
		if strings.TrimSpace(option) == "" {
			p.add("%s[%d]: must be a non-empty string", field, i)
		}
	}
	if len(options) > 26 {
		p.add("%s: at most 26 options (a-z); got %d", field, len(options))
	}
	return options
}

func stringValue(p *problems, field string, raw any) (string, bool) {
	s, ok := raw.(string)
	if !ok {
		p.add("%s: must be a string; got %s", field, typeName(raw))
		return "", false
	}
	return s, true
}

func requiredString(p *problems, field string, raw any) (string, bool) {
	if raw == nil {
		p.add("%s: required", field)
		return "", false
	}
	s, ok := stringValue(p, field, raw)
	if !ok {
		return "", false
	}
	if strings.TrimSpace(s) == "" {
		p.add("%s: must be non-empty", field)
		return "", false
	}
	return s, true
}

func requiredEnum(p *problems, field string, raw any, values []string) (string, bool) {
	s, ok := requiredString(p, field, raw)
	if !ok {
		return "", false
	}
	for _, v := range values {
		if s == v {
			return s, true
		}
	}
	p.add("%s: must be one of %s; got %q", field, strings.Join(values, ", "), s)
	return "", false
}

func intValue(p *problems, field string, raw any) (int, bool) {
	n, ok := raw.(float64)
	if !ok {
		p.add("%s: must be a number; got %s", field, typeName(raw))
		return 0, false
	}
	if n != float64(int(n)) {
		p.add("%s: must be a whole number; got %v", field, n)
		return 0, false
	}
	return int(n), true
}

func boolValue(p *problems, field string, raw any) (bool, bool) {
	b, ok := raw.(bool)
	if !ok {
		p.add("%s: must be a boolean; got %s", field, typeName(raw))
		return false, false
	}
	return b, true
}

// encodedHint adds the remedy for the commonest host mistake: an array or
// object passed as its JSON encoding.
func encodedHint(raw any) string {
	if _, ok := raw.(string); ok {
		return " (pass the value itself, not a JSON-encoded string)"
	}
	return ""
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
