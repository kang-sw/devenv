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

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
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

// Item statuses. Answered, withdrawn, and superseded items are settled; open
// items still wait on the user. An assumed item carries the lead's reading of
// a vague reply: it takes a settle order and the page shows it among the
// settled items, but it is not IsSettled, so archive eviction never drops it
// and the user's later words still land on its ID.
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
	// ChangedAt is the wall-clock time (RFC 3339) of that update, which the
	// stale-topic nudge ages topics by; see stampChanges for a store written
	// before the field.
	ChangedAt string `json:"changed_at,omitempty"`
	// Settled is the item's settle order (a board-wide counter), zero while
	// the item is open. Rotation and archive eviction order by it, never by
	// time.
	Settled int `json:"settled,omitempty"`
	// ReaskedAt is the wall-clock time (RFC 3339, millisecond precision) of
	// the item's latest re-ask: a patch that changed its class or options, or
	// set it back to open from settled or assumed. Empty when it was never
	// re-asked. The dashboard drops sent marks older than it (ReaskTimes).
	ReaskedAt string `json:"reasked_at,omitempty"`
}

// reaskLayout is ReaskedAt's format: RFC 3339 with milliseconds, the
// precision of the dashboard inbox's send times it is compared with.
const reaskLayout = "2006-01-02T15:04:05.000Z07:00"

// OpenPolicyCount is the number of open policy items, the count the full
// page's header shows (pageData.CountPolicy): every open policy item renders,
// since a topic that waits on one is never cut by the closed-topic cap. A nil
// state has none.
func (s *State) OpenPolicyCount() int {
	if s == nil {
		return 0
	}
	n := 0
	for _, item := range s.Items {
		if item.Status == StatusOpen && item.Class == ClassPolicy {
			n++
		}
	}
	return n
}

// ReaskTimes maps each re-asked item's ID to its latest re-ask time in epoch
// milliseconds; an item never re-asked, or with an unreadable time, is
// absent. A nil state has none.
func (s *State) ReaskTimes() map[int]int64 {
	if s == nil {
		return nil
	}
	var out map[int]int64
	for _, item := range s.Items {
		if item.ReaskedAt == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, item.ReaskedAt)
		if err != nil {
			continue
		}
		if out == nil {
			out = map[int]int64{}
		}
		out[item.ID] = at.UnixMilli()
	}
	return out
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
	// ChangedAt is the wall-clock time of that update, as an item's.
	ChangedAt string `json:"changed_at,omitempty"`
	// Marks are the change marks of the body's latest edit (diffBodies). Only
	// a body edit replaces them: the first body has none, the same body again
	// keeps them, and no other write touches them.
	Marks []BlockMark `json:"marks,omitempty"`
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
	AnswerSeq int    `json:"answer_seq,omitempty"`
	Items     []Item `json:"items,omitempty"`
	// ClosedTopics lists the closed topics in close order, the most recently
	// closed last: a close appends, a reopen removes.
	ClosedTopics []string `json:"closed_topics,omitempty"`
	// CloseResults holds each topic's close result, the one sentence its
	// latest wb:close gave on how it ended. A reopen keeps it (the page shows
	// it only while the topic is closed) and the next close replaces it; a
	// topic closed before the field has none.
	CloseResults map[string]string `json:"close_results,omitempty"`
	// Topics holds the topics that have a body, in first-set order. A store
	// written while the board had a Now slot keeps its "now" text on disk
	// until the next write; the field is gone, so nothing reads it.
	Topics []TopicDoc `json:"topics,omitempty"`
	// Labels maps each stemmed topic's stem, its key, to its label (see
	// topics.go). A topic absent here is stemless: its key is its label. An
	// entry goes when its topic disappears (pruneLabels); a store written
	// before the field has every topic stemless.
	Labels map[string]string `json:"labels,omitempty"`
	// LastEmbedSeq is the render sequence of the previous embed snapshot.
	LastEmbedSeq int     `json:"last_embed_seq,omitempty"`
	Embeds       []Embed `json:"embeds,omitempty"`
	// EmbedDir is the host embed folder, empty when embeds go to the
	// repository folder.
	EmbedDir string `json:"embed_dir,omitempty"`
	// LastNudgeAt is when an update result last carried the stale-topic
	// nudge (RFC 3339); empty, as on a store written before the field, is
	// never.
	LastNudgeAt string `json:"last_nudge_at,omitempty"`
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
	// Withdrawn holds the ID of every item a wb:close withdrew, in command
	// order.
	Withdrawn []int
	// Info is the note on Withdrawn, then the note on items left holding
	// options under a class other than policy, then the stale-topic nudge
	// (see staleNudge), "" when the batch carries none. It is for the caller's
	// tool result only; no page shows it.
	Info string
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
// an added item does, reopens a closed topic. Change marks are per update
// batch: base is the state as the batch began, and the marks diff against
// the body it held there, so several edits to one topic in a batch keep all
// their marks and a body the batch created marks nothing. Edits that bring
// the body back to the one the batch began with change nothing: the topic
// keeps that body's marks and Changed.
func (s *State) setTopicBody(topic, body string, mark int, base *State) {
	i := s.topicDocIndex(topic)
	if i >= 0 && s.Topics[i].Body == body {
		return
	}
	if i < 0 {
		s.Topics = append(s.Topics, TopicDoc{Name: topic})
		i = len(s.Topics) - 1
	}
	if base != nil {
		if bi := base.topicDocIndex(topic); bi >= 0 && base.Topics[bi].Body == body {
			s.Topics[i] = base.Topics[bi]
			return
		}
	}
	s.Topics[i].Marks = nil
	if prev := base.TopicBody(topic); prev != "" {
		s.Topics[i].Marks = diffBodies(prev, body)
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

// settleOrder is the order the page rotates a settled or assumed item by.
// An assumed item stored before assumed items took a settle order has
// Settled 0; it ranks as newer than every settle order (several such items
// by Changed), since sorting it as the oldest would collapse it at once.
func (s *State) settleOrder(item Item) int {
	if item.Settled == 0 && item.Status == StatusAssumed {
		return s.SettleSeq + item.Changed
	}
	return item.Settled
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
	var optionWarn []int
	embedDir := false
	// closedBy names, per item a close in this batch withdrew, that close.
	closedBy := map[int]string{}
	// keys holds each wb:topic group's resolved topic key, resolved once
	// when the batch reaches the group so all its commands act on one
	// topic; "" when the group's name or label was refused.
	keys := map[*wsxmlcmd.Scope]string{}
	topicOf := func(cmd command) (string, bool) {
		name, ok := cmd.Scoped()
		if !ok {
			return "", false
		}
		key, seen := keys[cmd.Scope]
		if !seen {
			key = work.resolveTopic(name)
			keys[cmd.Scope] = key
		}
		return key, key != ""
	}

	for _, cmd := range cmds {
		if cmd.IsGroup() {
			keys[cmd.Scope] = applyLabel(p, work, cmd)
			continue
		}
		switch cmd.Tag {
		case "wb:add":
			id := applyAdd(p, work, cmd, topicOf, mark)
			out.IDs = append(out.IDs, id)
			optionWarn = work.noteNonPolicyOptions(optionWarn, id)
		case "wb:answer":
			applyAnswer(p, work, cmd, mark, env.Now, closedBy)
		case "wb:patch":
			optionWarn = work.noteNonPolicyOptions(optionWarn, applyPatch(p, work, cmd, mark, env.Now))
		case "wb:body":
			applyBody(p, work, state, cmd, topicOf, mark)
		case "wb:edit":
			applyEdit(p, work, state, cmd, topicOf, mark)
		case "wb:close":
			withdrawn := applyClose(p, work, cmd, topicOf, mark)
			for _, id := range withdrawn {
				closedBy[id] = cmd.Label()
			}
			out.Withdrawn = append(out.Withdrawn, withdrawn...)
		case "wb:floor":
			if floor, ok := wsxmlcmd.IntAttr(p, cmd, "id", true); ok && floor > work.Counter {
				work.Counter = floor
			}
		case "wb:embed-dir":
			embedDir = true
			dir, present := cmd.Attr("path")
			switch {
			case !present:
				p.Add("%s: required (\"\" clears it)", cmd.At("path"))
			case dir == "":
				work.EmbedDir = ""
			case !env.IsAbs(dir):
				p.Add("%s: must be an absolute path; got %q", cmd.At("path"), dir)
			case !env.IsDir(dir):
				p.Add("%s: %s is not an existing directory (the server never creates it)", cmd.At("path"), dir)
			default:
				work.EmbedDir = dir
			}
		}
	}
	if !embedDir && work.EmbedDir != "" && !env.IsDir(work.EmbedDir) {
		p.Add("embed_dir %s (stored for this session) is no longer an existing directory; send <wb:embed-dir path=\"...\"/> with the host's current folder, or path=\"\" to clear it", work.EmbedDir)
	}

	if len(p.List) > 0 {
		return Outcome{}, p.List
	}
	work.evictArchive()
	work.pruneLabels()
	work.stampChanges(mark, env.Now)
	out.Info = joinInfo(withdrawnInfo(out.Withdrawn), nonPolicyOptionsInfo(work, optionWarn), work.staleNudge(env.Now))
	*state = *work
	return out, nil
}

// applyClose closes a topic with its result. Closing a closed topic again
// replaces its result and keeps its place in the close order. Every item of
// the topic still open is withdrawn, as a patch to withdrawn would (settled
// now, answer fields kept as history); a later reopen of the topic does not
// restore them. An assumed item already reads as settled and stays assumed.
// It returns the withdrawn IDs.
func applyClose(p *problems, work *State, cmd command, topicOf topicResolver, mark int) []int {
	topic, topicOK := topicOf(cmd)
	result, resultOK := wsxmlcmd.RequiredAttr(p, cmd, "result")
	if !topicOK || !resultOK {
		return nil
	}
	if !work.topicKnown(topic) {
		p.Add("%s: no item or topic body has topic %q", cmd.Label(), topic)
		return nil
	}
	if !work.topicClosed(topic) {
		work.ClosedTopics = append(work.ClosedTopics, topic)
	}
	if work.CloseResults == nil {
		work.CloseResults = map[string]string{}
	}
	work.CloseResults[topic] = strings.TrimSpace(result)
	var withdrawn []int
	for i := range work.Items {
		item := &work.Items[i]
		if item.Topic != topic || item.Status != StatusOpen {
			continue
		}
		item.Status = StatusWithdrawn
		item.Changed = mark
		work.settle(item)
		withdrawn = append(withdrawn, item.ID)
	}
	return withdrawn
}

// withdrawnInfo is the update result's note on the items a close withdrew,
// "" when it withdrew none.
func withdrawnInfo(ids []int) string {
	if len(ids) == 0 {
		return ""
	}
	labels := make([]string, len(ids))
	for i, id := range ids {
		labels[i] = FormatID(id)
	}
	return "wb:close withdrew " + strings.Join(labels, ", ") + " (still open in the closed topic)."
}

// noteNonPolicyOptions appends id to ids when the item it names now holds
// options under a class other than policy, and is not listed yet. Options
// are judged after the command, so an add, a patch adding options, and a
// reclass away from policy that keeps them all count.
func (s *State) noteNonPolicyOptions(ids []int, id int) []int {
	idx := s.index(id)
	if id == 0 || idx < 0 || slices.Contains(ids, id) {
		return ids
	}
	if item := s.Items[idx]; item.Class != ClassPolicy && len(item.Options) > 0 {
		ids = append(ids, id)
	}
	return ids
}

// nonPolicyOptionsInfo is the update result's note on items that hold
// options under a class other than policy, "" when there are none. Only a
// policy item's options are offered for a pick.
func nonPolicyOptionsInfo(s *State, ids []int) string {
	if len(ids) == 0 {
		return ""
	}
	labels := make([]string, 0, len(ids))
	for _, id := range ids {
		if idx := s.index(id); idx >= 0 {
			labels = append(labels, FormatID(id)+" ["+s.Items[idx].Class+"]")
		}
	}
	if len(labels) == 0 {
		return ""
	}
	return "Options on a non-policy item: " + strings.Join(labels, ", ") + ". Options are a pick for a policy question; reclass it with <wb:patch id=\"..\" class=\"policy\"> or clear them with clear=\"options\"."
}

// joinInfo joins the non-empty info sentences with a space.
func joinInfo(parts ...string) string {
	var kept []string
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, " ")
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
	if s, ok := cmd.Single(p, "wb:title"); ok {
		f.title = &s
	}
	if s, ok := cmd.Single(p, "wb:recommend"); ok {
		f.recommend = &s
	}
	if s, ok := cmd.Single(p, "wb:why"); ok {
		f.why = &s
	}
	if options := cmd.FieldsOf("wb:option"); len(options) > 0 {
		f.hasOptions = true
		f.options = checkOptions(p, cmd, options)
	}
	if links := cmd.FieldsOf("wb:link"); len(links) > 0 {
		f.hasLinks = true
		f.links = links
	}
	return f
}

func checkOptions(p *problems, cmd command, options []string) []string {
	for i, option := range options {
		if strings.TrimSpace(option) == "" {
			p.Add("%s: option %s must be non-empty", cmd.At("<wb:option>"), OptionLetterFor(i))
		}
	}
	if len(options) > 26 {
		p.Add("%s: at most 26 options (a-z); got %d", cmd.At("<wb:option>"), len(options))
	}
	return options
}

func applyAdd(p *problems, work *State, cmd command, topicOf topicResolver, mark int) int {
	item := Item{Status: StatusOpen, Changed: mark}
	before := len(p.List)
	if _, present := cmd.Attr("id"); present {
		if id, ok := wsxmlcmd.IntAttr(p, cmd, "id", true); ok {
			if work.index(id) >= 0 {
				p.Add("%s: %s is already in the store", cmd.At("id"), FormatID(id))
			} else {
				item.ID = id
			}
		}
	}
	item.Class, _ = wsxmlcmd.EnumAttr(p, cmd, "class", classValues)
	topic, topicOK := topicOf(cmd)
	item.Topic = topic
	f := readItemFields(p, cmd)
	if f.title == nil {
		p.Add("%s: required", cmd.At("<wb:title>"))
	} else if wsxmlcmd.NonEmpty(p, cmd.At("<wb:title>"), *f.title) {
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
	checkPolicyShape(p, cmd.Label(), item)
	if len(p.List) > before || !topicOK {
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

// applyAnswer records an answer. closedBy names the close earlier in the
// batch that withdrew an item, so answering it says to move the answer
// before that close rather than to reopen the item.
func applyAnswer(p *problems, work *State, cmd command, mark int, now time.Time, closedBy map[int]string) {
	ids := answerIDs(p, cmd)
	answer, answerOK := wsxmlcmd.RequiredAttr(p, cmd, "value")
	quote := cmd.Body
	quoteOK := wsxmlcmd.NonEmpty(p, cmd.Label()+" body (the user's verbatim words)", quote)
	assumed, _ := wsxmlcmd.BoolAttr(p, cmd, "assumed")
	if !answerOK || !quoteOK {
		return
	}
	for _, id := range ids {
		label := cmd.Label() + " " + FormatID(id)
		idx := work.index(id)
		if idx < 0 {
			p.Add("%s: not in the store", label)
			continue
		}
		item := &work.Items[idx]
		switch item.Status {
		case StatusWithdrawn, StatusSuperseded:
			if by, ok := closedBy[id]; ok && item.Status == StatusWithdrawn {
				p.Add("%s: %s earlier in this batch withdrew it; put the answers before the close", label, by)
				continue
			}
			p.Add("%s: is %s; patch its status to open before answering it", label, item.Status)
			continue
		}
		if item.Class == ClassPolicy && len(item.Options) > 0 && optionLetter(answer, len(item.Options)) == "" {
			p.Add("%s: a policy answer value starts with its option letter (%s); got %q", label, letterRange(len(item.Options)), answer)
			continue
		}
		item.Answer = answer
		item.AnswerQuote = quote
		item.AnsweredAt = now.UTC().Format(time.RFC3339)
		item.Changed = mark
		work.settle(item)
		if assumed {
			item.Status = StatusAssumed
			continue
		}
		item.Status = StatusAnswered
		work.AnswerSeq = mark
	}
}

// answerIDs reads an answer's id or ids attribute: exactly one of them, ids
// being space-separated positive numbers without repeats.
func answerIDs(p *problems, cmd command) []int {
	_, hasID := cmd.Attr("id")
	raw, hasIDs := cmd.Attr("ids")
	switch {
	case hasID && hasIDs:
		p.Add("%s: takes id or ids, not both", cmd.Label())
		return nil
	case hasID:
		if id, ok := wsxmlcmd.IntAttr(p, cmd, "id", true); ok {
			return []int{id}
		}
		return nil
	case !hasIDs:
		p.Add("%s: id (or ids) required", cmd.Label())
		return nil
	}
	var ids []int
	seen := map[int]bool{}
	for _, tok := range strings.Fields(raw) {
		id, err := strconv.Atoi(tok)
		if err != nil || id < 1 {
			p.Add("%s: %q is not a positive number", cmd.At("ids"), tok)
			continue
		}
		if seen[id] {
			p.Add("%s: %s is listed twice", cmd.At("ids"), FormatID(id))
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 && len(strings.Fields(raw)) == 0 {
		p.Add("%s: must list at least one id", cmd.At("ids"))
	}
	return ids
}

// patchClearValues are the list fields a patch's clear attribute empties.
var patchClearValues = []string{"options", "links"}

// applyPatch edits one item and returns its ID, 0 when the id attribute is
// unusable. Class and options change only on an item that is open once the
// patch's own status change applies, so one patch can reopen a settled item,
// reclass it, and give it new options: a stored answer means something only
// under the class and options it was given. A patch that really changes the
// class or the options, or sets a settled or assumed item back to open, is a
// re-ask and stamps ReaskedAt with now.
func applyPatch(p *problems, work *State, cmd command, mark int, now time.Time) int {
	before := len(p.List)
	idx, id := -1, 0
	if n, ok := wsxmlcmd.IntAttr(p, cmd, "id", true); ok {
		id = n
		if idx = work.index(n); idx < 0 {
			p.Add("%s: %s is not in the store", cmd.At("id"), FormatID(n))
		}
	}
	f := readItemFields(p, cmd)
	status := ""
	if _, present := cmd.Attr("status"); present {
		status, _ = wsxmlcmd.EnumAttr(p, cmd, "status", patchStatusValues)
	}
	class := ""
	if _, present := cmd.Attr("class"); present {
		class, _ = wsxmlcmd.EnumAttr(p, cmd, "class", classValues)
	}
	clear := map[string]bool{}
	if raw, present := cmd.Attr("clear"); present {
		for _, tok := range strings.Fields(raw) {
			if !slices.Contains(patchClearValues, tok) {
				p.Add("%s: must name %s; got %q", cmd.At("clear"), strings.Join(patchClearValues, " or "), tok)
				continue
			}
			clear[tok] = true
		}
	}
	if clear["options"] && f.hasOptions {
		p.Add("%s: clear=\"options\" and <wb:option> fields contradict each other", cmd.Label())
	}
	if clear["links"] && f.hasLinks {
		p.Add("%s: clear=\"links\" and <wb:link> fields contradict each other", cmd.Label())
	}
	if idx < 0 {
		return 0
	}
	prev := work.Items[idx]
	next := prev
	// The open checks below read the status this patch leaves the item in.
	after := prev.Status
	if status != "" {
		after = status
	}
	if f.title != nil && wsxmlcmd.NonEmpty(p, cmd.At("<wb:title>"), *f.title) {
		next.Title = *f.title
	}
	if f.hasOptions || clear["options"] {
		if after != StatusOpen {
			p.Add("%s: %s is %s; options change only on an open item, since its answer is stored as an option letter (send status=\"open\" in the same patch to re-ask it)", cmd.At("<wb:option>"), FormatID(next.ID), after)
		} else {
			next.Options = f.options
		}
	}
	if class != "" {
		if after != StatusOpen {
			p.Add("%s: %s is %s; class changes only on an open item, since its answer was given under its class (send status=\"open\" in the same patch to re-ask it)", cmd.At("class"), FormatID(next.ID), after)
		} else {
			next.Class = class
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
	checkPolicyShape(p, cmd.Label(), next)
	if len(p.List) > before {
		return id
	}
	next.Changed = mark
	reopened := false
	if status != "" && status != next.Status {
		next.Status = status
		if status == StatusOpen {
			// Reopening asks the item again: its earlier answer no longer
			// stands. Withdrawn and superseded items keep theirs as history.
			reopened = true
			next.Settled = 0
			next.Answer, next.AnswerQuote, next.AnsweredAt = "", "", ""
		} else {
			work.settle(&next)
		}
	}
	if reopened || next.Class != prev.Class || !slices.Equal(next.Options, prev.Options) {
		next.ReaskedAt = now.UTC().Format(reaskLayout)
	}
	work.Items[idx] = next
	return id
}

// applyBody replaces a topic's body whole; an empty (or blank) body clears
// it. The same body again is no change.
func applyBody(p *problems, work, base *State, cmd command, topicOf topicResolver, mark int) {
	name, ok := topicOf(cmd)
	if !ok {
		return
	}
	body := cmd.Body
	if strings.TrimSpace(body) == "" {
		work.clearTopicBody(name)
		return
	}
	if fullDocument(body) {
		p.Add("%s: a topic body takes markdown with inline HTML, not a full document (found <!DOCTYPE or <html)", cmd.Label())
		return
	}
	work.setTopicBody(name, body, mark, base)
}

// applyEdit replaces one exact match of old in a topic's body with new, or
// every match with replace_all="true".
func applyEdit(p *problems, work, base *State, cmd command, topicOf topicResolver, mark int) {
	name, nameOK := topicOf(cmd)
	all, _ := wsxmlcmd.BoolAttr(p, cmd, "replace_all")
	old, hasOld := cmd.Single(p, "wb:old")
	repl, hasNew := cmd.Single(p, "wb:new")
	if !hasOld {
		p.Add("%s: required", cmd.At("<wb:old>"))
	} else if old == "" {
		p.Add("%s: must be non-empty", cmd.At("<wb:old>"))
	}
	if !hasNew {
		p.Add("%s: required (empty deletes the match)", cmd.At("<wb:new>"))
	}
	if !nameOK || !hasOld || old == "" || !hasNew {
		return
	}
	i := work.topicDocIndex(name)
	if i < 0 {
		p.Add("%s: topic %q has no body; set one with <wb:body>", cmd.Label(), name)
		return
	}
	body := work.Topics[i].Body
	switch n := strings.Count(body, old); {
	case n == 0:
		p.Add("%s: <wb:old> matches nothing in topic %q's body", cmd.Label(), name)
		return
	case n > 1 && !all:
		p.Add("%s: <wb:old> matches %d places in topic %q's body; widen it to one or set replace_all=\"true\"", cmd.Label(), n, name)
		return
	}
	next := strings.ReplaceAll(body, old, repl)
	if strings.TrimSpace(next) == "" {
		work.clearTopicBody(name)
		return
	}
	if fullDocument(next) {
		p.Add("%s: the edited body would hold a full document (found <!DOCTYPE or <html)", cmd.Label())
		return
	}
	work.setTopicBody(name, next, mark, base)
}

// checkPolicyShape checks what a policy item's options and recommendation
// must satisfy.
func checkPolicyShape(p *problems, label string, item Item) {
	if item.Recommendation == "" {
		return
	}
	if len(item.Options) == 0 {
		p.Add("%s <wb:recommend>: names an option letter, so it needs options", label)
		return
	}
	if optionLetter(item.Recommendation, len(item.Options)) == "" {
		p.Add("%s <wb:recommend>: starts with its option letter (%s), optionally followed by \": <why>\"; got %q", label, letterRange(len(item.Options)), item.Recommendation)
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
