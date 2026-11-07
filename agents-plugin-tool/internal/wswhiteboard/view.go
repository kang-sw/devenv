package wswhiteboard

import (
	"sort"
)

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
	// Now is the Now slot's markdown with inline HTML, the HTML written as is
	// by design; empty when the slot is clear.
	Now string
	// NowFresh marks Now changed by the latest update: it renders open, and
	// collapsed and greyed otherwise.
	NowFresh bool
	Topics   []TopicView
	// Panels come from the section providers, in registration order.
	Panels []Panel
	// BulkAck shows the reply bar's bulk checkbox: the board holds an open
	// technical or announced item or an assumed item.
	BulkAck bool
	// Answerable reports whether any item takes an answer on this page.
	Answerable bool
	// Stored holds every item ID the store holds, so a {{nn}} reference to
	// an item the page omits still resolves as a known item.
	Stored map[int]bool
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
	// NowImageSrc, when set, serves a file image in the Now slot of
	// RenderRegions' fragments in ImageSrc's place. That Now renders in a
	// sandboxed frame whose requests carry no cookie, so a served page that
	// needs one to reach its image route returns the image inline, as a
	// data: URL. An empty result falls back to ImageSrc.
	NowImageSrc func(path string) string
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
	// link it resolves data-wl="url". Raw anchors in the Now slot never pass
	// through here and stay unmarked; a finished reply's raw anchors and
	// images do, through the reply sanitizer (sanitize.go).
	FileActions func(path string) FileAction
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
}

// TopicView is one topic block.
type TopicView struct {
	Name string
	// Policy, Assumed, and Acks are the topic's items waiting on the user;
	// they never collapse.
	Policy  []ItemView
	Assumed []ItemView
	Acks    []ItemView
	// Expanded holds settled items shown inside the block: on the full page
	// the topic's share of the most recent settled items board-wide, on the
	// embed the settled items new since the previous embed. Newest first.
	Expanded []ItemView
	// Collapsed holds every other settled item, behind the topic's settled
	// count chip. Newest first.
	Collapsed []ItemView
	// Quiet marks a block with nothing waiting and nothing expanded; it
	// collapses to one title line.
	Quiet bool
	// Closed marks a topic the lead closed.
	Closed bool
	// Fresh counts the items new since the previous embed (embed only).
	Fresh int
}

// SettledCount is the topic's settled total, the count chip's number.
func (t TopicView) SettledCount() int {
	return len(t.Expanded) + len(t.Collapsed)
}

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
	Layout  Layout
	ReplyID string
	Panels  []Panel
	// FileLink, FileText, ImageSrc, NowImageSrc, and FileActions are copied
	// to the View.
	FileLink    func(path string) string
	FileText    func(path string) string
	ImageSrc    func(path string) string
	NowImageSrc func(path string) string
	FileActions func(path string) FileAction
}

// BuildView derives a page view from state.
func BuildView(state *State, in ViewInput) View {
	view := View{
		Layout:      in.Layout,
		ReplyID:     in.ReplyID,
		Seq:         state.Seq,
		Now:         state.Now,
		Panels:      in.Panels,
		Stored:      map[int]bool{},
		FileLink:    in.FileLink,
		FileText:    in.FileText,
		ImageSrc:    in.ImageSrc,
		NowImageSrc: in.NowImageSrc,
		FileActions: in.FileActions,
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
	view.NowFresh = state.NowFresh()

	expanded := map[int]bool{}
	if in.Layout == LayoutFull {
		for _, id := range recentSettled(state) {
			expanded[id] = true
		}
	}

	var order []string
	blocks := map[string]*TopicView{}
	for _, item := range state.Items {
		block, ok := blocks[item.Topic]
		if !ok {
			block = &TopicView{Name: item.Topic, Closed: state.topicClosed(item.Topic)}
			blocks[item.Topic] = block
			order = append(order, item.Topic)
		}
		iv := itemView(item)
		if in.Layout == LayoutEmbed && item.Changed > state.LastEmbedSeq {
			iv.New = true
			block.Fresh++
		}
		switch {
		case item.Status == StatusOpen && item.Class == ClassPolicy:
			block.Policy = append(block.Policy, iv)
		case item.Status == StatusOpen:
			block.Acks = append(block.Acks, iv)
		case item.Status == StatusAssumed:
			block.Assumed = append(block.Assumed, iv)
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

	settledOrder := map[int]int{}
	for _, item := range state.Items {
		settledOrder[item.ID] = item.Settled
	}
	newestFirst := func(list []ItemView) {
		sort.SliceStable(list, func(i, j int) bool { return settledOrder[list[i].ID] > settledOrder[list[j].ID] })
	}
	rank := func(t *TopicView) int {
		switch {
		case len(t.Policy) > 0:
			return 0
		case len(t.Assumed) > 0:
			return 1
		case len(t.Acks) > 0:
			return 2
		case len(t.Expanded) > 0:
			return 3
		default:
			return 4
		}
	}
	for _, name := range order {
		block := blocks[name]
		newestFirst(block.Expanded)
		newestFirst(block.Collapsed)
		block.Quiet = rank(block) == 4
		if len(block.Acks) > 0 || len(block.Assumed) > 0 {
			view.BulkAck = true
		}
		if len(block.Policy) > 0 || len(block.Assumed) > 0 || len(block.Acks) > 0 {
			view.Answerable = true
		}
	}
	// Within one rank, the topic touched most recently rises; equal activity
	// keeps first-appearance order (the stable sort).
	activity := map[string]int{}
	for _, item := range state.Items {
		activity[item.Topic] = max(activity[item.Topic], item.Changed)
	}
	sort.SliceStable(order, func(i, j int) bool {
		ri, rj := rank(blocks[order[i]]), rank(blocks[order[j]])
		if ri != rj {
			return ri < rj
		}
		return activity[order[i]] > activity[order[j]]
	})
	for _, name := range order {
		view.Topics = append(view.Topics, *blocks[name])
	}
	return view
}

// recentSettled returns the IDs of the ExpandedSettled most recently settled
// items outside closed topics, counted board-wide.
func recentSettled(state *State) []int {
	type entry struct{ id, order int }
	var settled []entry
	for _, item := range state.Items {
		if IsSettled(item.Status) && !state.topicClosed(item.Topic) {
			settled = append(settled, entry{item.ID, item.Settled})
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
