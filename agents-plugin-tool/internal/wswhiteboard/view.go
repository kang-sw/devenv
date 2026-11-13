package wswhiteboard

import (
	"slices"
	"sort"
	"strings"
	"time"
)

// ClosedCap is how many closed topics a page shows, the most recently closed
// first. The served page fetches more through ClosedWindow; the file page and
// the embed name how many they leave out. A closed topic still waiting on the
// user is never capped, and the store keeps every topic.
const ClosedCap = 30

// Layout selects which page a View renders as.
type Layout int

const (
	// LayoutFull is the current-state dashboard the user keeps open.
	LayoutFull Layout = iota
	// LayoutEmbed is the point-in-time snapshot a host shows in its chat.
	LayoutEmbed
)

// View is everything a page template reads. BuildView derives it from a
// State; the templates make no decisions about rotation, ordering, or
// freshness of their own.
type View struct {
	Layout  Layout
	ReplyID string
	Seq     int
	// StampFile is the change-signal script the full page polls, a bare
	// file name beside the page. Empty for the embed.
	StampFile string
	Topics    []TopicView
	// DeferredTopics are capped closed topics, carried inert for explicit
	// panel/entry navigation without another stream protocol.
	DeferredTopics []TopicView
	// ClosedMore counts the closed topics past the ones Topics holds, in close
	// order; ClosedNext is the index (ClosedWindow.From) of the first of them.
	ClosedMore, ClosedNext int
	// Panels come from the section providers, in registration order.
	Panels []Panel
	// BulkAck shows the reply bar's bulk checkbox: the board holds an open
	// technical or announced item.
	BulkAck bool
	// Answerable reports whether any item takes an answer on this page.
	Answerable bool
	// Stored holds every item ID the store holds, so a {{nn}} reference to
	// an item the page omits still resolves as a known item.
	Stored map[int]bool
	// StoredPanels holds board-wide stable pN targets, including older pages.
	StoredPanels map[string]bool
	// FullBoardLink is the embed's link to the full board, relative by default.
	FullBoardLink string
	// FileLink turns a file path a markdown link or image names (relative
	// to the session root, or absolute) into the href the page uses from
	// the folder it is written to. Nil keeps the path as written.
	FileLink func(path string) string
	// FileText, when set, renders every file target (a relative or absolute
	// path, or a file: URL) as text instead: a link keeps its label followed
	// by the path FileText returns, and an image shows its alt text followed
	// by that path. A page served over http sets it, since a browser blocks
	// file: links there and a relative path would resolve against the
	// server's URL, not the session root. FileLink is then unused.
	FileText func(path string) string
	// ImageSrc, when set on a page with FileText, serves a file image: an
	// image whose destination is a file target (a relative or absolute path,
	// a drive path, or a file: URL) renders as <img src> with the src
	// ImageSrc returns for the path, and the destination's own query and
	// fragment are dropped. An empty result leaves the image to FileText's
	// path text (or FileLink's href without FileText); links and autolinks
	// never use it.
	ImageSrc func(path string) string
	// DocImageSrc, when set, serves a file image in a topic body of
	// RenderRegions' fragments in ImageSrc's place. That body renders in a
	// sandboxed frame whose requests carry no cookie, so a served page that
	// needs one to reach its image route returns the image inline, as a
	// data: URL. An empty result falls back to ImageSrc.
	DocImageSrc func(path string) string
	// FileActions, when set on a page with FileText, turns a file target
	// into an action link, the element a served page's link menu takes: a
	// file link or autolink renders as an anchor around its label, and the
	// path text of an image ImageSrc does not serve as an anchor around that
	// path. The anchor is marked data-wl="file" and carries the
	// FileAction's Href (only a scheme-less absolute path such as
	// "/s/<reply-id>/file?..."; anything else is dropped), Rel as data-rel,
	// and Abs as data-abs; it is titled with the FileText path unless the
	// link has a title of its own. A zero FileAction leaves the target to
	// FileText's path text. A page with FileActions also marks every http(s)
	// link it resolves data-wl="url". Raw anchors in a topic body never pass
	// through here and stay unmarked; a finished reply's raw anchors and
	// images do, through the reply sanitizer (sanitize.go).
	FileActions func(path string) FileAction
	// OutsideImage, when set on a page with FileActions, serves an image
	// file ImageSrc and FileActions leave out, one outside the session root
	// that only some viewers may load. A FileAction with an Href (Outside
	// set) makes an image of that path render as <img> on that Href, marked
	// data-out with the absolute path as data-abs, so the page can put the
	// path back when the image fails to load; and a link to it an action
	// link on that Href, marked data-out, whose open items the page shows
	// only where the image loads. A topic body uses it for an image only,
	// and only in its sandboxed frame (RenderRegions), which cannot load the
	// Href itself: the image keeps its path text, marked data-relay with the
	// Href, for the frame to ask its host page for the image (framedDoc).
	// An unframed body never consults it. A zero FileAction leaves the
	// target to ImageSrc and FileActions alone.
	OutsideImage func(path string) FileAction
	// Lang is the language of the text the page itself draws (a wsi18n
	// code; empty renders English). A page written to disk renders in
	// English: it has no viewer to follow. The served dashboard page sets
	// it, after BuildView, per viewer.
	Lang string
}

// FileAction is what a served page's link menu offers for one file target.
type FileAction struct {
	// Href opens the file on the serving daemon (its file view); empty
	// when the daemon would refuse it, as for a path outside the session
	// root, which the menu then offers to copy only.
	Href string
	// Rel is the path relative to the session root, with forward slashes;
	// empty outside the root.
	Rel string
	// Abs is the absolute path.
	Abs string
	// Image marks an image file the page may hand the host's native image
	// app (data-img).
	Image bool
	// Outside marks an image outside the session root whose Href only some
	// viewers may load (data-out; View.OutsideImage).
	Outside bool
}

// TopicView is one topic block.
type TopicView struct {
	// Name is the topic's key (its stem, else its label), which page-to-lead
	// lines carry (data-topic) so the lead can address the topic with it;
	// Label is the name the page shows.
	Name, Label string
	// Doc is the topic's body, markdown with inline HTML written as is; empty
	// when it has none. It renders above the items: open unless the topic is
	// closed on the full page, and open on the embed only when it changed
	// since the previous embed (DocNew).
	Doc string
	// DocMarks are the change marks of the body's latest edit, in the block
	// numbering of bodyUnits.
	DocMarks []BlockMark
	// DocStale marks the body's change marks as no longer fresh: an item of
	// the topic changed after the body's latest edit (Item.Changed above
	// the body's Changed; a recap change does not count). The marks stay,
	// dimmed, until the next body edit.
	DocStale bool
	// DocNew marks a body changed since the previous embed (embed only).
	DocNew bool
	// Recap is the topic's recap, rendered by the item block-field rule;
	// empty when it has none. It shows at the top of the topic, above the
	// body, on both layouts while the topic is open (RecapOpen), and folds
	// with the body once the topic is closed (RecapFolded). RecapAt,
	// RecapMs, and RecapClock are its update time as UpdatedAt, UpdatedMs,
	// and UpdatedClock are the topic's; RecapStamp is its local date and
	// time, the time's hover title.
	Recap      string
	RecapAt    string
	RecapMs    int64
	RecapClock string
	RecapStamp string
	// LatestAgent supplies the current-state summary, never a system event.
	// Recap remains a read-only fallback for unconverted legacy views.
	LatestAgent *Entry
	// Stream is the newest HistoryWindow entries; Older holds inert pages,
	// nearest first. PanelRecaps lists latest distinct panels for the embed.
	Stream      EntryPage
	Older       []EntryPage
	PanelRecaps []EntryView
	// Policy and Acks are the topic's items waiting on the user; they never
	// collapse.
	Policy []ItemView
	Acks   []ItemView
	// Checks are the topic's check items, open, answered, or assumed, in the
	// order they were added: the check list under the recap, where an
	// answered or assumed check stays, locked, instead of moving to the
	// settled items. A withdrawn or superseded check is a settled item like
	// any other. In a closed topic the list folds with the settled items.
	Checks []ItemView
	// Expanded holds settled items shown inside the block (an assumed item
	// among them, ordered by its settle order like the rest): on the full page
	// the topic's share of the most recent settled items board-wide, on the
	// embed the settled items new since the previous embed. Newest first.
	Expanded []ItemView
	// Collapsed holds every other settled item, behind the topic's settled
	// count chip. Newest first.
	Collapsed []ItemView
	// Quiet marks a block with nothing waiting, nothing expanded, and no body
	// or recap showing; it collapses to one title line (and a closed body's
	// fold).
	Quiet bool
	// Closed marks a topic the lead closed.
	Closed bool
	// Result is the closed topic's close result, empty while the topic is
	// open or when its close gave none (a store written before results).
	Result string
	// UpdatedAt is the topic's latest change, the newest change time among
	// its items, body, and recap (RFC 3339, UTC), empty when none has one. UpdatedMs
	// is the same instant in Unix milliseconds for the page script, and
	// UpdatedClock its local HH:MM, which the embed shows.
	UpdatedAt    string
	UpdatedMs    int64
	UpdatedClock string
	// Fresh counts the items new since the previous embed (embed only).
	Fresh int
	// Tickets are the topic's linked tickets, in the order the lead wrote
	// them, each with the status ViewInput.Tickets resolved at render time.
	Tickets []TicketBadge
	// Live marks a topic with live work: at least one linked ticket still
	// idea, todo, or ready whose category is actionable (liveCategories).
	// The page gives it a glow.
	Live bool
}

// Linked ticket statuses: a ticket's status directory, or TicketUnknown for
// a stem the resolver does not find.
const (
	TicketIdea    = "idea"
	TicketTodo    = "todo"
	TicketReady   = "ready"
	TicketDone    = "done"
	TicketDropped = "dropped"
	TicketUnknown = "unknown"
)

// liveCategories are the ticket categories (a stem's second segment) whose
// open tickets light a topic's glow: work to carry out. A linked research or
// epic ticket still shows as a badge, but never makes a topic live.
var liveCategories = []string{"feat", "bug", "refactor", "chore"}

// TicketResolver resolves a linked ticket stem to its status: the status
// directory that holds it (TicketIdea, TicketTodo, TicketReady, TicketDone,
// or TicketDropped), false when none does. The host supplies it; this
// package never reads a ticket tree.
type TicketResolver interface {
	TicketStatus(stem string) (status string, ok bool)
}

// TicketBadge is one linked ticket as a topic bar shows it.
type TicketBadge struct {
	// Stem is the full stem; Name is the stem without its date prefix, the
	// badge's text.
	Stem, Name string
	// Status is the resolved status, TicketUnknown when the resolver does
	// not find the stem (or the view has no resolver).
	Status string
}

// live reports whether the badge's ticket is open work of an actionable
// category.
func (b TicketBadge) live() bool {
	switch b.Status {
	case TicketIdea, TicketTodo, TicketReady:
	default:
		return false
	}
	parts := strings.SplitN(b.Name, "-", 2)
	return slices.Contains(liveCategories, parts[0])
}

// ticketBadges resolves stems through resolve (nil resolves none), caching
// each stem's status in seen across the view's topics.
func ticketBadges(stems []string, resolve TicketResolver, seen map[string]string) []TicketBadge {
	var out []TicketBadge
	for _, stem := range stems {
		status, ok := seen[stem]
		if !ok {
			status = TicketUnknown
			if resolve != nil {
				if s, found := resolve.TicketStatus(stem); found && slices.Contains([]string{TicketIdea, TicketTodo, TicketReady, TicketDone, TicketDropped}, s) {
					status = s
				}
			}
			seen[stem] = status
		}
		name := stem
		if _, rest, cut := strings.Cut(stem, "-"); cut {
			name = rest
		}
		out = append(out, TicketBadge{Stem: stem, Name: name, Status: status})
	}
	return out
}

// HeadSummary reports whether the topic's head carries its summary line: an
// open topic with a latest agent line or a check list. The line holds the
// agent line (never a legacy recap or a system event) and the check pill.
func (t TopicView) HeadSummary() bool {
	return !t.Closed && (t.LatestAgent != nil || t.ChecksShown())
}

// LegacyRecapOpen reports whether an open topic shows an unconverted legacy
// recap, as a card under the head: it has no agent line to stand in the
// head.
func (t TopicView) LegacyRecapOpen() bool { return t.RecapOpen() && t.LatestAgent == nil }

// DocOpen reports whether the topic's body renders open.
func (t TopicView) DocOpen(embed bool) bool {
	if t.Doc == "" || t.Closed {
		return false
	}
	return !embed || t.DocNew
}

// RecapOpen reports whether the recap shows at the top of the topic: it has
// one and is open.
func (t TopicView) RecapOpen() bool { return t.Summary() != "" && !t.Closed }

// Summary is the latest agent line, or a legacy recap when not converted.
func (t TopicView) Summary() string {
	if t.LatestAgent != nil {
		return t.LatestAgent.Text
	}
	return t.Recap
}

// RecapFolded reports whether the recap folds with the body: it has one and
// is closed.
func (t TopicView) RecapFolded() bool { return t.Summary() != "" && t.Closed }

// HasFold reports whether the topic renders the body's fold: it has a body,
// or a recap folded into it.
func (t TopicView) HasFold() bool { return t.Doc != "" || t.RecapFolded() }

// SettledCount is the topic's settled total, the count chip's number; a
// closed topic's checks fold with its settled items and count among them.
func (t TopicView) SettledCount() int {
	n := len(t.Expanded) + len(t.Collapsed)
	if t.Closed {
		n += len(t.Checks)
	}
	return n
}

// ChecksShown reports whether the check list shows under the recap: the
// topic has checks and is open.
func (t TopicView) ChecksShown() bool { return len(t.Checks) > 0 && !t.Closed }

// OpenChecks counts the topic's checks still waiting on the user.
func (t TopicView) OpenChecks() int {
	n := 0
	for _, c := range t.Checks {
		if c.Status == StatusOpen {
			n++
		}
	}
	return n
}

// AnsweredChecks counts the topic's checks the user answered, or the lead
// assumed an answer for: the check list's progress count.
func (t TopicView) AnsweredChecks() int { return len(t.Checks) - t.OpenChecks() }

// HideableChecks counts the checks the open check list's hide toggle folds
// away: those the user answered. An assumed check stays, since the user can
// still veto it.
func (t TopicView) HideableChecks() int {
	n := 0
	for _, c := range t.Checks {
		if c.Status == StatusAnswered {
			n++
		}
	}
	return n
}

// AskCount is the topic bar's amber "answer needed" count: its open policy
// items and open checks.
func (t TopicView) AskCount() int { return len(t.Policy) + t.OpenChecks() }

// waiting reports whether anything in the topic waits on the user.
func (t TopicView) waiting() bool { return t.AskCount() > 0 || len(t.Acks) > 0 }

// ItemView is one item as a page shows it.
type ItemView struct {
	ID    int
	Label string // (nn)
	Class string
	// Status is the store status.
	Status    string
	Title     string
	Options   []OptionView
	RecLetter string // recommended option letter, empty when none
	RecText   string // the recommendation's reason, may be empty
	Rationale string
	Answer    string
	// AnswerLetter and AnswerText split a policy answer into its option
	// letter and the rest.
	AnswerLetter string
	AnswerText   string
	AnswerQuote  string
	AnsweredAt   string
	// Links are the item's link strings; a page resolves their {{nn}}
	// references as it does the other lead-authored text.
	Links []string
	// New marks an item changed since the previous embed (embed only).
	New bool
}

// OptionView is one lettered policy option.
type OptionView struct {
	Letter      string
	Text        string
	Recommended bool
	// Picked marks the option a settled or assumed answer chose.
	Picked bool
}

// Panel is one collapsible auxiliary section supplied by a section provider.
type Panel struct {
	// ID is stable across renders; the full page keys its open state on it.
	ID    string
	Title string
	// Icon names a sprite symbol the templates ship (for example "todo",
	// "list").
	Icon  string
	Count string
	Rows  []PanelRow
	// Folded rows render after Rows behind a closed "done N" fold.
	Folded []PanelRow
	// Lanes render before Rows as kanban lanes: side by side when the panel
	// is wide enough, stacked otherwise. A lane's rows render as cards.
	Lanes []PanelLane
	// Tree renders after Rows as foldable nodes, one root each (see
	// JSONTree).
	Tree []TreeNode
}

// PanelLane is one kanban lane of a panel: a header and a vertical stack of
// cards.
type PanelLane struct {
	Name string
	// Count is the lane's card count chip; Note is a short muted remark
	// after it (for example that the lane lists only the latest cards).
	Count string
	Note  string
	// Rows render as cards: Label is the small chip, Text the clamped title,
	// Sub the muted second line, and Link or Path as for a plain row.
	Rows []PanelRow
}

// PanelRow is one line of a panel.
type PanelRow struct {
	// State is a short state token the templates style (for example
	// "done", "wip", "pending"); empty for none.
	State string
	Label string
	Text  string
	// Link, when set, makes Text a link: a relative path or an absolute
	// file: URL the provider built (see RenderPage); empty for none.
	Link string
	// Path is the row's file as text, shown after Text when the row has no
	// Link: a served page lists a ticket by its repo-relative path instead
	// of linking it. A card shows it as a tooltip rather than as body text.
	Path string
	// Action, set by BuildView on a page with link actions (FileActions and
	// FileText) for a row with a Path, makes Text the served page's file
	// action link, titled with Path; the zero value leaves Path as above.
	Action FileAction
	// Sub is a card's muted second line; plain rows ignore it.
	Sub string
}

// IsAction reports whether the row's Text renders as a file action link.
func (r PanelRow) IsAction() bool { return r.Action.Href != "" || r.Action.Abs != "" }

// withActions returns panels with each row's Path resolved through
// FileActions into the row's Action, the way fileTarget resolves a markdown
// file target (including its daemon-path check on the href). The input is
// left as it was.
func withActions(panels []Panel, fileActions func(string) FileAction) []Panel {
	row := func(r PanelRow) PanelRow {
		if r.Path != "" && r.Link == "" {
			r.Action = fileActions(r.Path)
			if !daemonPath(r.Action.Href) {
				r.Action.Href = ""
			}
		}
		return r
	}
	rows := func(in []PanelRow) []PanelRow {
		if in == nil {
			return nil
		}
		out := make([]PanelRow, len(in))
		for i, r := range in {
			out[i] = row(r)
		}
		return out
	}
	out := make([]Panel, len(panels))
	for i, p := range panels {
		p.Rows, p.Folded = rows(p.Rows), rows(p.Folded)
		if p.Lanes != nil {
			lanes := make([]PanelLane, len(p.Lanes))
			for j, l := range p.Lanes {
				l.Rows = rows(l.Rows)
				lanes[j] = l
			}
			p.Lanes = lanes
		}
		out[i] = p
	}
	return out
}

// ViewInput is what BuildView needs beyond the State.
type ViewInput struct {
	Layout        Layout
	ReplyID       string
	Panels        []Panel
	FullBoardLink string
	// FileLink, FileText, ImageSrc, DocImageSrc, FileActions, and
	// OutsideImage are copied to the View.
	FileLink     func(path string) string
	FileText     func(path string) string
	ImageSrc     func(path string) string
	DocImageSrc  func(path string) string
	FileActions  func(path string) FileAction
	OutsideImage func(path string) FileAction
	// Tickets resolves each topic's linked ticket stems to their status;
	// nil shows every linked ticket as unknown.
	Tickets TicketResolver
	// Closed, when set, builds the view of the capped closed topics alone,
	// the served page's show-more fetch; nil builds the whole board with the
	// first ClosedCap of them.
	Closed *ClosedWindow
	// Location is the time zone of UpdatedClock; nil is the local zone.
	Location *time.Location
}

// ClosedWindow selects capped closed topics by index in close order, the
// most recently closed first: N of them from From.
type ClosedWindow struct {
	From, N int
}

// BuildView derives a page view from state.
func BuildView(state *State, in ViewInput) View {
	view := View{
		Layout:        in.Layout,
		ReplyID:       in.ReplyID,
		Seq:           state.Seq,
		Panels:        in.Panels,
		Stored:        map[int]bool{},
		StoredPanels:  map[string]bool{},
		FullBoardLink: in.FullBoardLink,
		FileLink:      in.FileLink,
		FileText:      in.FileText,
		ImageSrc:      in.ImageSrc,
		DocImageSrc:   in.DocImageSrc,
		FileActions:   in.FileActions,
		OutsideImage:  in.OutsideImage,
	}
	if view.FullBoardLink == "" {
		view.FullBoardLink = in.ReplyID + "/index.html"
	}
	for _, doc := range state.Topics {
		for _, entry := range doc.Entries {
			if entry.Kind == EntryPanel {
				view.StoredPanels[entry.PanelID] = true
			}
		}
	}
	if in.FileActions != nil && in.FileText != nil {
		view.Panels = withActions(in.Panels, in.FileActions)
	}
	for _, item := range state.Items {
		view.Stored[item.ID] = true
	}
	if in.Layout == LayoutFull {
		view.StampFile = in.ReplyID + ".stamp.js"
	}

	expanded := map[int]bool{}
	if in.Layout == LayoutFull {
		for _, id := range recentSettled(state) {
			expanded[id] = true
		}
	}

	var order []string
	blocks := map[string]*TopicView{}
	itemsChanged := map[string]int{} // newest Item.Changed per topic
	blockOf := func(topic string) *TopicView {
		block, ok := blocks[topic]
		if !ok {
			block = &TopicView{Name: topic, Label: state.TopicLabel(topic), Closed: state.topicClosed(topic)}
			blocks[topic] = block
			order = append(order, topic)
		}
		return block
	}
	for _, item := range state.Items {
		block := blockOf(item.Topic)
		iv := itemView(item)
		itemsChanged[item.Topic] = max(itemsChanged[item.Topic], item.Changed)
		if in.Layout == LayoutEmbed && item.Changed > state.LastEmbedSeq {
			iv.New = true
			block.Fresh++
		}
		switch {
		case inCheckList(item):
			block.Checks = append(block.Checks, iv)
		case item.Status == StatusOpen && item.Class == ClassPolicy:
			block.Policy = append(block.Policy, iv)
		case item.Status == StatusOpen:
			block.Acks = append(block.Acks, iv)
		default:
			show := expanded[item.ID]
			if in.Layout == LayoutEmbed {
				show = iv.New && !block.Closed
			}
			if show {
				block.Expanded = append(block.Expanded, iv)
			} else {
				block.Collapsed = append(block.Collapsed, iv)
			}
		}
	}

	ticketStatus := map[string]string{}
	for _, doc := range state.Topics {
		block := blockOf(doc.Name)
		block.Tickets = ticketBadges(doc.Tickets, in.Tickets, ticketStatus)
		block.Live = slices.ContainsFunc(block.Tickets, TicketBadge.live)
		block.Doc = doc.Body
		block.DocMarks = doc.Marks
		block.DocStale = itemsChanged[doc.Name] > doc.Changed
		block.DocNew = in.Layout == LayoutEmbed && doc.Body != "" && doc.Changed > state.LastEmbedSeq
		block.Recap = doc.Recap
		if latest, ok := doc.LatestAgent(); ok {
			block.LatestAgent = &latest
		}
		block.PanelRecaps, block.Stream, block.Older = streamView(doc, block.Closed, in.Location)
		if in.Layout == LayoutEmbed {
			block.Doc, block.DocMarks, block.DocNew = "", nil, false
			block.Stream, block.Older = EntryPage{}, nil
		}
	}
	stampUpdated(state, blocks, in.Location)
	for _, block := range blocks {
		if block.Closed {
			block.Result = state.CloseResults[block.Name]
		}
	}

	settledOrder := map[int]int{}
	for _, item := range state.Items {
		settledOrder[item.ID] = state.settleOrder(item)
	}
	newestFirst := func(list []ItemView) {
		sort.SliceStable(list, func(i, j int) bool { return settledOrder[list[i].ID] > settledOrder[list[j].ID] })
	}
	// Quiet follows what the page shows, so an unread body that stays folded
	// (the embed, an unchanged body) does not make a topic look active.
	quiet := func(t *TopicView) bool {
		return !t.waiting() && len(t.Expanded) == 0 && !t.ChecksShown() && !t.DocOpen(in.Layout == LayoutEmbed) && !t.RecapOpen() && (t.Closed || len(t.PanelRecaps) == 0)
	}
	for _, name := range order {
		block := blocks[name]
		newestFirst(block.Expanded)
		newestFirst(block.Collapsed)
		block.Quiet = quiet(block)
		if len(block.Acks) > 0 {
			view.BulkAck = true
		}
		if block.waiting() {
			view.Answerable = true
		}
	}
	// The topic touched most recently rises, whatever waits on the user;
	// topics that rose in the same update order by why they rose (bumpRank),
	// and equal ranks keep first-appearance order (the stable sort).
	activity := map[string]int{}
	for _, item := range state.Items {
		activity[item.Topic] = max(activity[item.Topic], item.Changed)
	}
	for _, doc := range state.Topics {
		activity[doc.Name] = max(activity[doc.Name], doc.Recency())
	}
	rank := bumpRank(state, activity)
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if blocks[a].UpdatedMs != blocks[b].UpdatedMs {
			return blocks[a].UpdatedMs > blocks[b].UpdatedMs
		}
		if activity[a] != activity[b] {
			return activity[a] > activity[b]
		}
		return rank[a] < rank[b]
	})
	// A closed topic with nothing waiting goes last, the most recently
	// closed first, and only a window of them renders (ClosedCap).
	closedAt := map[string]int{}
	for i, topic := range state.ClosedTopics {
		closedAt[topic] = i
	}
	var shown, capped []string
	for _, name := range order {
		if blocks[name].Closed && !blocks[name].waiting() {
			capped = append(capped, name)
		} else {
			shown = append(shown, name)
		}
	}
	sort.SliceStable(capped, func(i, j int) bool { return closedAt[capped[i]] > closedAt[capped[j]] })
	from, n := 0, ClosedCap
	if in.Closed != nil {
		shown = nil
		from, n = max(in.Closed.From, 0), max(in.Closed.N, 0)
	}
	from = min(from, len(capped))
	end := min(from+n, len(capped))
	shown = append(shown, capped[from:end]...)
	view.ClosedNext, view.ClosedMore = end, len(capped)-end
	for _, name := range shown {
		view.Topics = append(view.Topics, *blocks[name])
	}
	if in.Closed == nil && in.Layout == LayoutFull {
		for _, name := range capped[end:] {
			view.DeferredTopics = append(view.DeferredTopics, *blocks[name])
		}
	}
	return view
}

// Why a topic rose at its latest activity mark, strongest first.
const (
	bumpPolicy = iota // a policy or check item asked (added or re-asked) and still open
	bumpAck           // a technical or announced item asked and still open
	bumpDoc           // the topic body or recap changed
	bumpOther         // anything else: an answer, a close, an edit that asks nothing
)

// bumpRank maps each topic to the strongest reason it rose at its activity
// mark (the latest update that touched it). A topic with no reason recorded
// at that mark ranks bumpOther.
func bumpRank(state *State, activity map[string]int) map[string]int {
	rank := map[string]int{}
	bump := func(topic string, r int) {
		if cur, ok := rank[topic]; !ok || r < cur {
			rank[topic] = r
		}
	}
	for _, item := range state.Items {
		if item.Changed != activity[item.Topic] {
			continue
		}
		switch {
		case item.Asked == 0 || item.Asked != item.Changed || item.Status != StatusOpen:
			bump(item.Topic, bumpOther)
		case item.Class == ClassPolicy || item.Class == ClassCheck:
			bump(item.Topic, bumpPolicy)
		default:
			bump(item.Topic, bumpAck)
		}
	}
	for _, doc := range state.Topics {
		if at := activity[doc.Name]; at > 0 && (doc.Changed == at || doc.RecapChanged == at) {
			bump(doc.Name, bumpDoc)
		}
	}
	for topic := range activity {
		if _, ok := rank[topic]; !ok {
			rank[topic] = bumpOther
		}
	}
	return rank
}

// stampUpdated sets each block's latest change from the change times of its
// items, body, and recap, and its recap's own update time; a time that does
// not parse (a store written before change times) counts as none.
func stampUpdated(state *State, blocks map[string]*TopicView, loc *time.Location) {
	if loc == nil {
		loc = time.Local
	}
	latest := map[string]time.Time{}
	touch := func(topic, at string) {
		if t, err := time.Parse(time.RFC3339, at); err == nil && t.After(latest[topic]) {
			latest[topic] = t
		}
	}
	for _, item := range state.Items {
		touch(item.Topic, item.ChangedAt)
	}
	for _, doc := range state.Topics {
		touch(doc.Name, doc.ChangedAt)
		touch(doc.Name, doc.RecapAt)
		for _, entry := range doc.Entries {
			touch(doc.Name, entry.At)
		}
		at, summary := doc.RecapAt, doc.Recap
		if agent, ok := doc.LatestAgent(); ok {
			at, summary = agent.At, agent.Text
		}
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil && summary != "" {
			block := blocks[doc.Name]
			block.RecapAt = t.UTC().Format(time.RFC3339)
			block.RecapMs = t.UnixMilli()
			block.RecapClock = t.In(loc).Format("15:04")
			block.RecapStamp = t.In(loc).Format("2006-01-02 15:04")
		}
	}
	for topic, t := range latest {
		block := blocks[topic]
		block.UpdatedAt = t.UTC().Format(time.RFC3339)
		block.UpdatedMs = t.UnixMilli()
		block.UpdatedClock = t.In(loc).Format("15:04")
	}
}

// recentSettled returns the IDs of the ExpandedSettled most recently settled
// items outside closed topics, counted board-wide. An assumed item rotates
// with them: the page shows it as a settled row. A check that stays in its
// topic's check list (inCheckList) takes no place among them.
func recentSettled(state *State) []int {
	type entry struct{ id, order int }
	var settled []entry
	for _, item := range state.Items {
		if (IsSettled(item.Status) || item.Status == StatusAssumed) && !inCheckList(item) && !state.topicClosed(item.Topic) {
			settled = append(settled, entry{item.ID, state.settleOrder(item)})
		}
	}
	sort.Slice(settled, func(i, j int) bool { return settled[i].order > settled[j].order })
	if len(settled) > ExpandedSettled {
		settled = settled[:ExpandedSettled]
	}
	ids := make([]int, len(settled))
	for i, e := range settled {
		ids[i] = e.id
	}
	return ids
}

// inCheckList reports whether item belongs to its topic's check list
// (TopicView.Checks): a check that is open, answered, or assumed.
func inCheckList(item Item) bool {
	if item.Class != ClassCheck {
		return false
	}
	switch item.Status {
	case StatusOpen, StatusAnswered, StatusAssumed:
		return true
	}
	return false
}

func itemView(item Item) ItemView {
	iv := ItemView{
		ID:          item.ID,
		Label:       FormatID(item.ID),
		Class:       item.Class,
		Status:      item.Status,
		Title:       item.Title,
		Rationale:   item.Rationale,
		Answer:      item.Answer,
		AnswerQuote: item.AnswerQuote,
		AnsweredAt:  item.AnsweredAt,
		Links:       item.Links,
	}
	count := len(item.Options)
	if item.Recommendation != "" {
		iv.RecLetter, iv.RecText = SplitLetter(item.Recommendation, count)
	}
	if item.Answer != "" && count > 0 {
		iv.AnswerLetter, iv.AnswerText = SplitLetter(item.Answer, count)
	}
	for i, option := range item.Options {
		letter := OptionLetterFor(i)
		iv.Options = append(iv.Options, OptionView{
			Letter:      letter,
			Text:        option,
			Recommended: letter == iv.RecLetter,
			Picked:      item.Status != StatusOpen && letter == iv.AnswerLetter,
		})
	}
	return iv
}
