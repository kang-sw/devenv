// Package wswhiteboard is the session whiteboard: the item store a lead
// session keeps in its state record, the validation and application of a
// whiteboard.update command string, the read-only query, the view model both pages
// render from, and the page templates. It does no file I/O of its own; the
// MCP layer owns persistence, the rendered files, and their lifecycle.
package wswhiteboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
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
	// fullDocumentPattern detects a whole HTML document where a topic body
	// takes markdown with inline HTML; fullDocument applies it to the raw
	// HTML only.
	fullDocumentPattern = regexp.MustCompile(`(?i)<!doctype|<html[\s>]`)
	// docParser parses a topic body as renderDoc's pipeline does (default
	// block and inline parsers, raw HTML on, tables, strikethrough, task lists, footnotes).
	docParser = goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.TaskList, extension.Footnote)).Parser()
)

// fullDocument reports whether a topic body carries a whole HTML document:
// a <!DOCTYPE or <html in its raw HTML (an HTML block or inline raw HTML).
// Text inside a code span or fence that only mentions one is not raw HTML,
// so a body that explains HTML is accepted.
func fullDocument(content string) bool {
	if !fullDocumentPattern.MatchString(content) {
		return false
	}
	src := []byte(content)
	found := false
	_ = ast.Walk(docParser.Parse(text.NewReader(src)), func(n ast.Node, entering bool) (ast.WalkStatus, error) {
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

// TopicDoc is a topic's body: block markdown with inline HTML written as is,
// rendered above the topic's items.
type TopicDoc struct {
	Name string `json:"name"`
	Body string `json:"body"`
	// Changed is the render sequence of the update that last changed the
	// body; it counts toward the topic's recency like an item's Changed.
	Changed int `json:"changed"`
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
	SettleSeq int `json:"settle_seq,omitempty"`
	// AnswerSeq is the render sequence of the last applied wb:answer that
	// is not an assumption: the board's mark of the user having answered
	// something. Zero before any such answer and on a board stored before
	// the field; LastAnswerSeq derives a stand-in for the latter.
	AnswerSeq    int      `json:"answer_seq,omitempty"`
	Items        []Item   `json:"items,omitempty"`
	ClosedTopics []string `json:"closed_topics,omitempty"`
	// Topics holds the topics that have a body, in first-set order. A store
	// written while the board had a Now slot keeps its "now" text on disk
	// until the next write; the field is gone, so nothing reads it.
	Topics []TopicDoc `json:"topics,omitempty"`
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
	// IsDir reports whether path is an existing directory (embed_dir).
	IsDir func(path string) bool
	// IsAbs reports whether path is absolute on the server's platform.
	IsAbs func(path string) bool
}

// Outcome is what an accepted update did.
type Outcome struct {
	// IDs holds the ID of every wb:add command, in command order.
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

// topicKnown reports whether topic has an item or a body.
func (s *State) topicKnown(topic string) bool {
	for _, item := range s.Items {
		if item.Topic == topic {
			return true
		}
	}
	return s.topicDocIndex(topic) >= 0
}

func (s *State) topicHasItems(topic string) bool {
	for _, item := range s.Items {
		if item.Topic == topic {
			return true
		}
	}
	return false
}

func (s *State) topicDocIndex(topic string) int {
	for i := range s.Topics {
		if s.Topics[i].Name == topic {
			return i
		}
	}
	return -1
}

// TopicBody returns topic's body, "" when it has none.
func (s *State) TopicBody(topic string) string {
	if s == nil {
		return ""
	}
	if i := s.topicDocIndex(topic); i >= 0 {
		return s.Topics[i].Body
	}
	return ""
}

// setTopicBody sets topic's body; a change marks the topic changed and, as
// an added item does, reopens a closed topic.
func (s *State) setTopicBody(topic, body string, mark int) {
	i := s.topicDocIndex(topic)
	if i >= 0 && s.Topics[i].Body == body {
		return
	}
	if i < 0 {
		s.Topics = append(s.Topics, TopicDoc{Name: topic})
		i = len(s.Topics) - 1
	}
	s.Topics[i].Body = body
	s.Topics[i].Changed = mark
	if s.topicClosed(topic) {
		s.reopenTopic(topic)
	}
}

// clearTopicBody drops topic's body. A topic left with no item is gone, so
// its closed mark goes too.
func (s *State) clearTopicBody(topic string) {
	i := s.topicDocIndex(topic)
	if i < 0 {
		return
	}
	s.Topics = append(s.Topics[:i], s.Topics[i+1:]...)
	if !s.topicHasItems(topic) {
		s.reopenTopic(topic)
	}
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

// LastAnswerSeq is the render sequence of the board's most recent user
// answer. A board stored before AnswerSeq existed has none recorded; the
// newest change among its answered items stands in, which can only be later
// than the real answer (a patch bumps Changed), never earlier.
func (s *State) LastAnswerSeq() int {
	if s.AnswerSeq > 0 {
		return s.AnswerSeq
	}
	last := 0
	for _, item := range s.Items {
		if item.Status == StatusAnswered {
			last = max(last, item.Changed)
		}
	}
	return last
}

// BeginRender advances the render sequence for a render that changes no item
// (the implicit todo and agenda refresh).
func (s *State) BeginRender() {
	s.Seq++
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

// Apply validates commands, one whiteboard.update command string (see
// commands.go), and applies it to state. Commands apply in the order written
// against a working copy; any problem leaves state untouched and every
// problem is returned. An accepted batch advances the render sequence once;
// the caller renders afterwards.
func Apply(state *State, commands string, env Env) (Outcome, []string) {
	p := &problems{}
	cmds := parseCommands(p, commands)

	work := state.clone()
	work.Seq++
	mark := work.Seq
	var out Outcome
	embedDir := false

	for _, cmd := range cmds {
		switch cmd.tag {
		case "wb:add":
			out.IDs = append(out.IDs, applyAdd(p, work, cmd, mark))
		case "wb:answer":
			applyAnswer(p, work, cmd, mark, env.Now)
		case "wb:patch":
			applyPatch(p, work, cmd, mark)
		case "wb:topic":
			applyTopic(p, work, cmd, mark)
		case "wb:edit":
			applyEdit(p, work, cmd, mark)
		case "wb:close":
			if topic, ok := requiredAttr(p, cmd, "topic"); ok {
				switch {
				case !work.topicKnown(topic):
					p.add("%s: no item or topic body has topic %q", cmd.at("topic"), topic)
				case !work.topicClosed(topic):
					work.ClosedTopics = append(work.ClosedTopics, topic)
				}
			}
		case "wb:floor":
			if floor, ok := intAttr(p, cmd, "id", true); ok && floor > work.Counter {
				work.Counter = floor
			}
		case "wb:embed-dir":
			embedDir = true
			dir, present := cmd.attr("path")
			switch {
			case !present:
				p.add("%s: required (\"\" clears it)", cmd.at("path"))
			case dir == "":
				work.EmbedDir = ""
			case !env.IsAbs(dir):
				p.add("%s: must be an absolute path; got %q", cmd.at("path"), dir)
			case !env.IsDir(dir):
				p.add("%s: %s is not an existing directory (the server never creates it)", cmd.at("path"), dir)
			default:
				work.EmbedDir = dir
			}
		}
	}
	if !embedDir && work.EmbedDir != "" && !env.IsDir(work.EmbedDir) {
		p.add("embed_dir %s (stored for this session) is no longer an existing directory; send <wb:embed-dir path=\"...\"/> with the host's current folder, or path=\"\" to clear it", work.EmbedDir)
	}

	if len(p.list) > 0 {
		return Outcome{}, p.list
	}
	work.evictArchive()
	*state = *work
	return out, nil
}

// itemFields are an add or patch command's item fields, read from its field
// tags; a nil slice or pointer is a field the command did not carry.
type itemFields struct {
	title, recommend, why *string
	options, links        []string
	hasOptions, hasLinks  bool
}

func readItemFields(p *problems, cmd command) itemFields {
	var f itemFields
	if s, ok := cmd.single(p, "wb:title"); ok {
		f.title = &s
	}
	if s, ok := cmd.single(p, "wb:recommend"); ok {
		f.recommend = &s
	}
	if s, ok := cmd.single(p, "wb:why"); ok {
		f.why = &s
	}
	if options := cmd.fieldsOf("wb:option"); len(options) > 0 {
		f.hasOptions = true
		f.options = checkOptions(p, cmd, options)
	}
	if links := cmd.fieldsOf("wb:link"); len(links) > 0 {
		f.hasLinks = true
		f.links = links
	}
	return f
}

func checkOptions(p *problems, cmd command, options []string) []string {
	for i, option := range options {
		if strings.TrimSpace(option) == "" {
			p.add("%s: option %s must be non-empty", cmd.at("<wb:option>"), OptionLetterFor(i))
		}
	}
	if len(options) > 26 {
		p.add("%s: at most 26 options (a-z); got %d", cmd.at("<wb:option>"), len(options))
	}
	return options
}

func applyAdd(p *problems, work *State, cmd command, mark int) int {
	item := Item{Status: StatusOpen, Changed: mark}
	before := len(p.list)
	if _, present := cmd.attr("id"); present {
		if id, ok := intAttr(p, cmd, "id", true); ok {
			if work.index(id) >= 0 {
				p.add("%s: %s is already in the store", cmd.at("id"), FormatID(id))
			} else {
				item.ID = id
			}
		}
	}
	item.Class, _ = enumAttr(p, cmd, "class", classValues)
	item.Topic, _ = requiredAttr(p, cmd, "topic")
	f := readItemFields(p, cmd)
	if f.title == nil {
		p.add("%s: required", cmd.at("<wb:title>"))
	} else if nonEmpty(p, cmd.at("<wb:title>"), *f.title) {
		item.Title = *f.title
	}
	item.Options = f.options
	item.Links = f.links
	if f.recommend != nil {
		item.Recommendation = *f.recommend
	}
	if f.why != nil {
		item.Rationale = *f.why
	}
	checkPolicyShape(p, cmd.label(), item)
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

func applyAnswer(p *problems, work *State, cmd command, mark int, now time.Time) {
	ids := answerIDs(p, cmd)
	answer, answerOK := requiredAttr(p, cmd, "value")
	quote := cmd.body
	quoteOK := nonEmpty(p, cmd.label()+" body (the user's verbatim words)", quote)
	assumed, _ := boolAttr(p, cmd, "assumed")
	if !answerOK || !quoteOK {
		return
	}
	for _, id := range ids {
		label := cmd.label() + " " + FormatID(id)
		idx := work.index(id)
		if idx < 0 {
			p.add("%s: not in the store", label)
			continue
		}
		item := &work.Items[idx]
		switch item.Status {
		case StatusWithdrawn, StatusSuperseded:
			p.add("%s: is %s; patch its status to open before answering it", label, item.Status)
			continue
		}
		if item.Class == ClassPolicy && len(item.Options) > 0 && optionLetter(answer, len(item.Options)) == "" {
			p.add("%s: a policy answer value starts with its option letter (%s); got %q", label, letterRange(len(item.Options)), answer)
			continue
		}
		item.Answer = answer
		item.AnswerQuote = quote
		item.AnsweredAt = now.UTC().Format(time.RFC3339)
		item.Changed = mark
		if assumed {
			item.Status = StatusAssumed
			item.Settled = 0
			continue
		}
		item.Status = StatusAnswered
		work.settle(item)
		work.AnswerSeq = mark
	}
}

// answerIDs reads an answer's id or ids attribute: exactly one of them, ids
// being space-separated positive numbers without repeats.
func answerIDs(p *problems, cmd command) []int {
	_, hasID := cmd.attr("id")
	raw, hasIDs := cmd.attr("ids")
	switch {
	case hasID && hasIDs:
		p.add("%s: takes id or ids, not both", cmd.label())
		return nil
	case hasID:
		if id, ok := intAttr(p, cmd, "id", true); ok {
			return []int{id}
		}
		return nil
	case !hasIDs:
		p.add("%s: id (or ids) required", cmd.label())
		return nil
	}
	var ids []int
	seen := map[int]bool{}
	for _, tok := range strings.Fields(raw) {
		id, err := strconv.Atoi(tok)
		if err != nil || id < 1 {
			p.add("%s: %q is not a positive number", cmd.at("ids"), tok)
			continue
		}
		if seen[id] {
			p.add("%s: %s is listed twice", cmd.at("ids"), FormatID(id))
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 && len(strings.Fields(raw)) == 0 {
		p.add("%s: must list at least one id", cmd.at("ids"))
	}
	return ids
}

// patchClearValues are the list fields a patch's clear attribute empties.
var patchClearValues = []string{"options", "links"}

func applyPatch(p *problems, work *State, cmd command, mark int) {
	before := len(p.list)
	idx := -1
	if id, ok := intAttr(p, cmd, "id", true); ok {
		if idx = work.index(id); idx < 0 {
			p.add("%s: %s is not in the store", cmd.at("id"), FormatID(id))
		}
	}
	f := readItemFields(p, cmd)
	status := ""
	if _, present := cmd.attr("status"); present {
		status, _ = enumAttr(p, cmd, "status", patchStatusValues)
	}
	clear := map[string]bool{}
	if raw, present := cmd.attr("clear"); present {
		for _, tok := range strings.Fields(raw) {
			if !slices.Contains(patchClearValues, tok) {
				p.add("%s: must name %s; got %q", cmd.at("clear"), strings.Join(patchClearValues, " or "), tok)
				continue
			}
			clear[tok] = true
		}
	}
	if clear["options"] && f.hasOptions {
		p.add("%s: clear=\"options\" and <wb:option> fields contradict each other", cmd.label())
	}
	if clear["links"] && f.hasLinks {
		p.add("%s: clear=\"links\" and <wb:link> fields contradict each other", cmd.label())
	}
	if idx < 0 {
		return
	}
	next := work.Items[idx]
	if f.title != nil && nonEmpty(p, cmd.at("<wb:title>"), *f.title) {
		next.Title = *f.title
	}
	if f.hasOptions || clear["options"] {
		if next.Status != StatusOpen {
			p.add("%s: %s is %s; options change only on an open item, since its answer is stored as an option letter", cmd.at("<wb:option>"), FormatID(next.ID), next.Status)
		} else {
			next.Options = f.options
		}
	}
	if f.recommend != nil {
		next.Recommendation = *f.recommend
	}
	if f.why != nil {
		next.Rationale = *f.why
	}
	if f.hasLinks || clear["links"] {
		next.Links = f.links
	}
	checkPolicyShape(p, cmd.label(), next)
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

// applyTopic replaces a topic's body whole; an empty (or blank) body clears
// it. The same body again is no change.
func applyTopic(p *problems, work *State, cmd command, mark int) {
	name, ok := requiredAttr(p, cmd, "name")
	if !ok {
		return
	}
	body := cmd.body
	if strings.TrimSpace(body) == "" {
		work.clearTopicBody(name)
		return
	}
	if fullDocument(body) {
		p.add("%s: a topic body takes markdown with inline HTML, not a full document (found <!DOCTYPE or <html)", cmd.label())
		return
	}
	work.setTopicBody(name, body, mark)
}

// applyEdit replaces one exact match of old in a topic's body with new, or
// every match with replace_all="true".
func applyEdit(p *problems, work *State, cmd command, mark int) {
	name, nameOK := requiredAttr(p, cmd, "topic")
	all, _ := boolAttr(p, cmd, "replace_all")
	old, hasOld := cmd.single(p, "wb:old")
	repl, hasNew := cmd.single(p, "wb:new")
	if !hasOld {
		p.add("%s: required", cmd.at("<wb:old>"))
	} else if old == "" {
		p.add("%s: must be non-empty", cmd.at("<wb:old>"))
	}
	if !hasNew {
		p.add("%s: required (empty deletes the match)", cmd.at("<wb:new>"))
	}
	if !nameOK || !hasOld || old == "" || !hasNew {
		return
	}
	i := work.topicDocIndex(name)
	if i < 0 {
		p.add("%s: topic %q has no body; set one with <wb:topic>", cmd.label(), name)
		return
	}
	body := work.Topics[i].Body
	switch n := strings.Count(body, old); {
	case n == 0:
		p.add("%s: <wb:old> matches nothing in topic %q's body", cmd.label(), name)
		return
	case n > 1 && !all:
		p.add("%s: <wb:old> matches %d places in topic %q's body; widen it to one or set replace_all=\"true\"", cmd.label(), n, name)
		return
	}
	next := strings.ReplaceAll(body, old, repl)
	if strings.TrimSpace(next) == "" {
		work.clearTopicBody(name)
		return
	}
	if fullDocument(next) {
		p.add("%s: the edited body would hold a full document (found <!DOCTYPE or <html)", cmd.label())
		return
	}
	work.setTopicBody(name, next, mark)
}

// checkPolicyShape checks what a policy item's options and recommendation
// must satisfy.
func checkPolicyShape(p *problems, label string, item Item) {
	if item.Recommendation == "" {
		return
	}
	if len(item.Options) == 0 {
		p.add("%s <wb:recommend>: names an option letter, so it needs options", label)
		return
	}
	if optionLetter(item.Recommendation, len(item.Options)) == "" {
		p.add("%s <wb:recommend>: starts with its option letter (%s), optionally followed by \": <why>\"; got %q", label, letterRange(len(item.Options)), item.Recommendation)
	}
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

var queryFields = []string{"topic", "status", "id"}

// Query returns the items matching payload's filters, archived settled items
// included, in ID order, and, when payload names a topic, that topic's body
// ("" when it has none), so a lead can quote it exactly in a wb:edit.
func Query(state *State, payload any) ([]Item, string, []string) {
	p := &problems{}
	obj, ok := payloadObject(p, payload)
	if !ok {
		return nil, "", p.list
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
		return nil, "", p.list
	}
	if state == nil {
		return nil, "", nil
	}
	var out []Item
	for _, item := range state.Items {
		if (topic != "" && item.Topic != topic) || (status != "" && item.Status != status) || (id != 0 && item.ID != id) {
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	body := ""
	if topic != "" {
		body = state.TopicBody(topic)
	}
	return out, body, nil
}

// FormatItems renders query results as compact text: a topic body first,
// between marker lines, then one header line per item and an indented line
// per non-empty detail.
func FormatItems(items []Item, body string) string {
	var sb strings.Builder
	if body != "" {
		sb.WriteString("topic body:\n<<<\n" + body + "\n>>>\n")
	}
	if len(items) == 0 {
		sb.WriteString("no matching items\n")
		return sb.String()
	}
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

// --- problems and value checks -------------------------------------------

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

// nonEmpty reports whether s holds non-blank text, recording a problem when not.
func nonEmpty(p *problems, label, s string) bool {
	if strings.TrimSpace(s) == "" {
		p.add("%s: must be non-empty", label)
		return false
	}
	return true
}

// requiredAttr returns a command's non-blank attribute.
func requiredAttr(p *problems, cmd command, name string) (string, bool) {
	v, present := cmd.attr(name)
	if !present {
		p.add("%s: required", cmd.at(name))
		return "", false
	}
	return v, nonEmpty(p, cmd.at(name), v)
}

func enumAttr(p *problems, cmd command, name string, values []string) (string, bool) {
	v, ok := requiredAttr(p, cmd, name)
	if !ok {
		return "", false
	}
	if !slices.Contains(values, v) {
		p.add("%s: must be one of %s; got %q", cmd.at(name), strings.Join(values, ", "), v)
		return "", false
	}
	return v, true
}

// intAttr returns a command's positive whole-number attribute; required
// records a problem when it is absent.
func intAttr(p *problems, cmd command, name string, required bool) (int, bool) {
	v, present := cmd.attr(name)
	if !present {
		if required {
			p.add("%s: required", cmd.at(name))
		}
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 {
		p.add("%s: must be a positive whole number; got %q", cmd.at(name), v)
		return 0, false
	}
	return n, true
}

// boolAttr returns a command's "true"/"false" attribute, false when absent.
func boolAttr(p *problems, cmd command, name string) (bool, bool) {
	v, present := cmd.attr(name)
	if !present {
		return false, true
	}
	switch v {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	p.add("%s: must be \"true\" or \"false\"; got %q", cmd.at(name), v)
	return false, false
}
