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
	// Now is the Now slot's HTML fragment, written unescaped by design;
	// empty when the slot is clear.
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
}

// ViewInput is what BuildView needs beyond the State.
type ViewInput struct {
	Layout  Layout
	ReplyID string
	Panels  []Panel
}

// BuildView derives a page view from state.
func BuildView(state *State, in ViewInput) View {
	view := View{
		Layout:  in.Layout,
		ReplyID: in.ReplyID,
		Seq:     state.Seq,
		Now:     state.Now,
		Panels:  in.Panels,
		Stored:  map[int]bool{},
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
	sort.SliceStable(order, func(i, j int) bool { return rank(blocks[order[i]]) < rank(blocks[order[j]]) })
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
