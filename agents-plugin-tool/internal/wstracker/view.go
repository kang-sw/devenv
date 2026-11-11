package wstracker

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// View is the derived view state of one viewer clone: everything a page
// shows, as plain JSON. Its encoding is the mockup README "View state"
// shape with the same field names, except store, sync, and clones, which
// the caller fills from the wsledger sync report. Lists hold ids into the
// maps.
type View struct {
	Viewer        Viewer                 `json:"viewer"`
	Priority      PriorityLevels         `json:"priority"`
	Attachments   AttachmentLimit        `json:"attachments"`
	Labels        []LabelView            `json:"labels"` // the live label list, in add order
	Members       map[string]MemberView  `json:"members"`
	Admins        []string               `json:"admins"`
	RolesProblems []string               `json:"rolesProblems"`
	Items         map[string]ItemView    `json:"items"`
	ItemOrder     []string               `json:"itemOrder"`
	Assignments   map[string]Assignment  `json:"assignments"`
	Requests      map[string]RequestView `json:"requests"` // take requests
	Rules         map[string]RuleView    `json:"rules"`
	Comments      map[string]CommentView `json:"comments"`
	Events        []EventView            `json:"events"`
	Notices       []Notice               `json:"notices"`
	My            My                     `json:"my"`
	// Stats.Commits is left 0: an event carries no commit identity, so the
	// caller fills it.
	Stats Stats          `json:"stats"`
	Heads map[string]int `json:"heads"`
	Now   string         `json:"now"`
}

type Viewer struct {
	Member    string         `json:"member"`
	Stream    string         `json:"stream"`
	Name      string         `json:"name"` // the display name derived from the profile
	Profile   *Profile       `json:"profile"`
	Admin     bool           `json:"admin"`
	Lead      *string        `json:"lead"`
	Followers []string       `json:"followers"`
	Reach     map[string]int `json:"reach"`
	IsMember  bool           `json:"isMember"`
}

// LabelView is one live entry of the store's label list; ID is its
// roles.add_label event's id, which items carry.
type LabelView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type PriorityLevels struct {
	Names   []string `json:"names"`
	Default int      `json:"default"`
}

// AttachmentLimit is the store's attachments.max_bytes setting. MaxBytes is
// a float64 because the reference engine accepts any positive integer
// number, including one beyond the int range.
type AttachmentLimit struct {
	MaxBytes float64 `json:"max_bytes"`
}

// AttachmentView is the metadata of one attachment; OID is the content's
// sha256.
type AttachmentView struct {
	OID  string  `json:"oid"`
	Name string  `json:"name"`
	Size float64 `json:"size"` // a byte count; float64 as in AttachmentLimit
	Mime string  `json:"mime"`
}

type MemberView struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"` // the display name derived from the profile
	Profile  Profile      `json:"profile"`
	Avatar   string       `json:"avatar"`
	Leads    []string     `json:"leads"`
	Keys     []string     `json:"keys"`
	Revoked  []RevokedKey `json:"revoked"`
	Removed  *string      `json:"removed"`
	Joined   *string      `json:"joined"`
	Admin    bool         `json:"admin"`
	Children []string     `json:"children"`
	Depth    *int         `json:"depth"`
	Role     string       `json:"role"`
}

type RevokedKey struct {
	Key string `json:"key"`
	By  string `json:"by"`
}

// Ref names an event and its write time.
type Ref struct {
	ID string `json:"id"`
	TS string `json:"ts"`
}

type ItemView struct {
	ID            string   `json:"id"`
	Alias         string   `json:"alias"`
	Kind          string   `json:"kind"`
	Title         string   `json:"title"`
	Body          string   `json:"body"`
	Project       string   `json:"project"`
	Labels        []string `json:"labels"` // live label ids, in the order written
	Due           *string  `json:"due"`
	Priority      int      `json:"priority"`
	PriorityLevel int      `json:"priorityLevel"`
	PriorityName  string   `json:"priorityName"`
	Parent        *string  `json:"parent"`
	Children      []string `json:"children"`
	Author        string   `json:"author"`
	TS            string   `json:"ts"`
	Hidden        bool     `json:"hidden"`
	Edited        *Ref     `json:"edited"`
	Status        *string  `json:"status"` // nil for topics
	Counted       bool     `json:"counted"`
	Rollup        Rollup   `json:"rollup"`
	LastActivity  string   `json:"lastActivity"`
	Started       *string  `json:"started"`
	Assignments   []string `json:"assignments"`
	Pending       []string `json:"pending"`
	Thread        []string `json:"thread"`
	Concurrent    int      `json:"concurrent"`
	Depth         int      `json:"depth"`
	Chain         []string `json:"chain"`
	// Mentions, Attachments, and Refs follow the shown body write.
	Mentions    []string         `json:"mentions"`
	Attachments []AttachmentView `json:"attachments"`
	Refs        []string         `json:"refs"`
	// Backrefs are the shown items, then the shown comments on shown items,
	// that reference this item, in fold order; its own thread is excluded.
	Backrefs []string `json:"backrefs"`
	// Open is the item's opening while it is open; Claimable reports that
	// the viewer may claim it now (in its scope, not its fallback holder, the
	// item not hidden). Requests are its take requests, in fold order.
	Open      *OpenView `json:"open"`
	Claimable bool      `json:"claimable"`
	Requests  []string  `json:"requests"`
	// CC are the CC'd members who are not removed, in the order they came
	// to be CC'd.
	CC []string `json:"cc"`
}

// OpenView is an open item's opening: the item.open event, its writer, and
// its scope (team: the opener's org subtree; store: every member).
type OpenView struct {
	ID    string `json:"id"`
	By    string `json:"by"`
	Scope string `json:"scope"`
	TS    string `json:"ts"`
}

// RequestView is a take request: By asks for the live assignment
// Assignment that Assignee holds. State is pending, approved, denied,
// withdrawn, or lapsed (the assignment ended first); Ended is the event
// that ended it. Assignment is nil when the request named nothing live.
type RequestView struct {
	ID         string  `json:"id"`
	Item       string  `json:"item"`
	By         string  `json:"by"`
	Assignee   string  `json:"assignee"`
	Assignment *string `json:"assignment"`
	State      string  `json:"state"`
	Note       *string `json:"note"`
	AnswerNote *string `json:"answerNote"`
	TS         string  `json:"ts"`
	Ended      *Ended  `json:"ended"`
}

type Rollup struct {
	Total   int            `json:"total"`
	Counts  map[string]int `json:"counts"`
	Blocked []string       `json:"blocked"`
}

type Ended struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	TS   string `json:"ts"`
	By   string `json:"by"`
}

type Assignment struct {
	ID           string   `json:"id"`
	Item         string   `json:"item"`
	To           string   `json:"to"`
	By           string   `json:"by"`
	Sender       string   `json:"sender"`
	Completion   string   `json:"completion"`
	State        string   `json:"state"`
	Forced       bool     `json:"forced"`
	Status       string   `json:"status"`
	Note         *string  `json:"note"`
	NoteBy       *string  `json:"noteBy"`
	Via          string   `json:"via"`
	TS           string   `json:"ts"`
	Inherited    bool     `json:"inherited"`
	RuleLevels   *float64 `json:"ruleLevels"`
	Rule         *string  `json:"rule"`        // the rule that made it, or the rule a request asks for
	RuleRequest  bool     `json:"ruleRequest"` // the request a rule rooted outside reach sends its root
	Fallback     bool     `json:"fallback"`    // derived: the task's creator (or an admin) holds what nobody else does
	HandoffOf    *string  `json:"handoffOf"`
	HandoffFrom  *string  `json:"handoffFrom"`
	HandoffNote  *string  `json:"handoffNote"`
	AnswerNote   *string  `json:"answerNote"`
	Restored     *Ref     `json:"restored"`
	Restores     *string  `json:"restores"`
	Accepted     *Ref     `json:"accepted"`
	LeadAtAccept *string  `json:"leadAtAccept"`
	Ended        *Ended   `json:"ended"`
	Depth        *int     `json:"depth"`
	StatusOrigin *string  `json:"statusOrigin"`
	StatusBy     *string  `json:"statusBy"`
	LastUpdate   string   `json:"lastUpdate"`
	Stale        bool     `json:"stale"`

	lastUpdate time.Time
}

// RuleView is an assignment rule (490)-(492). Levels nil is the whole
// subtree; State is live, pending (Request is open), or ended.
type RuleView struct {
	ID          string   `json:"id"`
	Item        string   `json:"item"`
	Via         string   `json:"via"`
	By          string   `json:"by"`
	Root        string   `json:"root"`
	IncludeRoot bool     `json:"includeRoot"`
	Levels      *float64 `json:"levels"`
	Completion  string   `json:"completion"`
	State       string   `json:"state"`
	Request     *string  `json:"request"`
	Ended       *string  `json:"ended"`
	TS          string   `json:"ts"`
}

type CommentView struct {
	ID      string  `json:"id"`
	Item    string  `json:"item"`
	Author  string  `json:"author"`
	Body    string  `json:"body"`
	ReplyTo *string `json:"reply_to"`
	TS      string  `json:"ts"`
	Edited  *Ref    `json:"edited"`
	Hidden  *string `json:"hidden"` // the comment.hide event id
	// Mentions, Attachments, and Refs follow the shown text write.
	Mentions    []string         `json:"mentions"`
	Attachments []AttachmentView `json:"attachments"`
	Refs        []string         `json:"refs"`
}

type Marker struct {
	Kind        string  `json:"kind"`
	Field       string  `json:"field"`
	Other       string  `json:"other"`
	OtherMember *string `json:"otherMember"`
	Winner      string  `json:"winner"`
	Mine        *string `json:"mine"`
	Theirs      *string `json:"theirs"`
}

type Superseded struct {
	Field string `json:"field"`
	By    string `json:"by"`
	Why   string `json:"why"` // later | human
}

type EventView struct {
	ID           string          `json:"id"`
	Alias        string          `json:"alias"`
	Stream       string          `json:"stream"`
	Member       string          `json:"member"`
	Seq          int             `json:"seq"`
	TS           string          `json:"ts"`
	Type         string          `json:"type"`
	Origin       string          `json:"origin"`
	Payload      json.RawMessage `json:"payload"`
	Status       string          `json:"status"` // applied | flagged | waiting
	Reason       *string         `json:"reason"`
	Ignored      bool            `json:"ignored"`
	Markers      []Marker        `json:"markers"`
	Superseded   []Superseded    `json:"superseded"`
	Thread       *string         `json:"thread"`
	ParentThread *string         `json:"parentThread"`
}

// Notice is a derived notice (285): accepted, handoff, done, blocked,
// declined, returned. Key is "acc:", "ho:", "dec:", or "ret:" plus the
// assignment id; a done or blocked key is "w:<assignment>:<status>:<ts>", so
// a later write of the same status is a new notice.
type Notice struct {
	Key        string
	Kind       string
	Assignment string
	Item       string
	To         string
	By         string
	Sender     string
	TS         string
	State      string
	// From is set on handoff, declined, and returned notices; the answer
	// fields only on declined and returned ones.
	From     *string
	HasFrom  bool
	Answered bool
	EndedBy  string
	Note     *string
	Restores *string
}

func (n Notice) MarshalJSON() ([]byte, error) {
	m := map[string]any{"key": n.Key, "kind": n.Kind, "assignment": n.Assignment, "item": n.Item, "to": n.To,
		"by": n.By, "sender": n.Sender, "ts": n.TS, "state": n.State}
	if n.HasFrom {
		m["from"] = n.From
	}
	if n.Answered {
		m["endedBy"], m["note"], m["restores"] = n.EndedBy, n.Note, n.Restores
	}
	return json.Marshal(m)
}

type My struct {
	Assignments []string `json:"assignments"`
	Today       Today    `json:"today"`
	Received    Received `json:"received"`
	Given       []string `json:"given"`
	Created     []string `json:"created"`
	Team        []Team   `json:"team"`
	Requests    MyReqs   `json:"requests"`
	Badge       []string `json:"badge"`
	Related     Related  `json:"related"`
	// CCNotices are the changes on the items in Related.CC (and on CC'd
	// items that closed) written after the viewer was CC'd, in fold order.
	// They are not in Badge.
	CCNotices []CCNotice `json:"ccNotices"`
}

// Related is the "related to me" scope. Both lists leave out hidden items,
// done items, and items the viewer created or holds a live or pending
// assignment on. CC: the items the viewer is CC'd on, the latest CC add
// first. Mentioned: the other items whose shown body or a shown comment
// (not the viewer's own) mentions the viewer, the newest mention first; a
// body mention dates from the write that set the shown body, a comment
// mention from the write that set the comment's shown text. Closed: the
// done items either list would hold, in fold order.
type Related struct {
	CC        []string `json:"cc"`
	Mentioned []string `json:"mentioned"`
	Closed    []string `json:"closed"`
}

// CCNotice is a change on an item the viewer is CC'd on: a status write
// (item.status, item.close, item.confirm, item.reject, item.reopen) or a
// comment (comment.add). Key is "cc:" plus Event. To is the assignee the
// status write names (nil for a comment); Status is item.status's status.
// None is made for the viewer's own events, for an item the viewer created
// or holds, or for a comment that mentions the viewer or replies to the
// viewer's comment, which notify on their own.
type CCNotice struct {
	Key    string  `json:"key"`
	Type   string  `json:"type"`
	Item   string  `json:"item"`
	Event  string  `json:"event"`
	By     string  `json:"by"`
	To     *string `json:"to"`
	Status *string `json:"status"`
	TS     string  `json:"ts"`
}

// MyReqs are the pending take requests on shown items, in fold order:
// Incoming, those the viewer may answer (the named assignment's holder or the
// item's creator) and did not file; Outgoing, those the viewer filed.
type MyReqs struct {
	Incoming []string `json:"incoming"`
	Outgoing []string `json:"outgoing"`
}

type Today struct {
	Work     []string `json:"work"`
	Pending  []string `json:"pending"`
	Confirms []string `json:"confirms"`
}

type Received struct {
	Pending  []string `json:"pending"`
	Answered []string `json:"answered"`
	Sent     []string `json:"sent"`
}

// Team is one member's row of the team roll-up (613 a): a member of the
// viewer's subtree, Depth from the viewer, or, for an admin viewer, a member
// outside it (Outside, Depth from the org-tree root, a root being 0).
// Counts are the member's live assignments per status, a creator-fallback
// holding left out (262 a); Load is those at a teamLoad status; Stale and
// Overdue (the item's due date passed) are subsets of Load; Fallback is the
// live fallback holdings not done. Topic holdings are in no list or count.
type Team struct {
	ID       string         `json:"id"`
	Depth    int            `json:"depth"`
	Outside  bool           `json:"outside"`
	Open     []string       `json:"open"`
	Pending  []string       `json:"pending"`
	Waiting  []string       `json:"waiting"`
	Counts   map[string]int `json:"counts"`
	Load     int            `json:"load"`
	Stale    int            `json:"stale"`
	Overdue  int            `json:"overdue"`
	Fallback int            `json:"fallback"`
	Last     *string        `json:"last"`
}

type Stats struct {
	Events     int `json:"events"`
	Commits    int `json:"commits"`
	Flagged    int `json:"flagged"`
	Concurrent int `json:"concurrent"`
}

// Derive reads every event the source holds and returns the view state of
// the viewer clone stream ("<member>.<clone>") at now. staleDays <= 0 means
// DefaultStaleDays. A zero now turns stale off.
func Derive(src Source, stream string, now time.Time, staleDays int) *View {
	events, _ := src.Events(wsledger.Cursor{})
	r := replay(events, src.Identity().Genesis, src.Concurrent)
	return r.view(stream, now, staleDays)
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func nz(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ref(n *node) *Ref {
	if n == nil {
		return nil
	}
	return &Ref{ID: n.id, TS: ts(n.ts)}
}

func memberOf(stream string) string {
	for i := 0; i < len(stream); i++ {
		if stream[i] == '.' {
			return stream[:i]
		}
	}
	return stream
}

// itemStatus derives an item's own status from its live assignments, the
// fallback included, so a task always has one (todo in a store with no
// holder to fall back on). A pending rule request holds nothing, so it does
// not make the item pending. idea only when every live holder is at idea:
// it ranks below todo.
func (r *replayed) itemStatus(it *item) string {
	if it.kind == "topic" {
		return ""
	}
	var live []*rec
	pending := false
	for _, x := range r.S.recsOn(it.id) {
		switch {
		case x.state == "live":
			live = append(live, x)
		case x.state == "pending" && !x.ruleRequest:
			pending = true
		}
	}
	if len(live) == 0 {
		if pending {
			return "pending"
		}
		return "todo"
	}
	all, idea, blocked, awaiting, doing := true, true, false, false, false
	for _, x := range live {
		all = all && x.status == "done"
		idea = idea && x.status == "idea"
		blocked = blocked || x.status == "blocked"
		awaiting = awaiting || x.status == "awaiting"
		doing = doing || x.status == "doing" || x.status == "done"
	}
	switch {
	case all:
		return "done"
	case blocked:
		return "blocked"
	case awaiting:
		return "awaiting"
	case doing:
		return "doing"
	case idea:
		return "idea"
	}
	return "todo"
}

// counted: pending assignments are excluded from status and roll-up, so an
// item whose only assignments are pending is not counted; nor is an item
// at idea.
func (r *replayed) counted(it *item, st string) bool {
	return it.kind != "topic" && st != "pending" && st != "idea"
}

func (r *replayed) view(stream string, now time.Time, staleDays int) *View {
	if staleDays <= 0 {
		staleDays = DefaultStaleDays
	}
	S := r.S
	v := memberOf(stream)
	me := S.members[v]
	R := reachMap(S.reach(v, 99))
	children := S.children()
	nowStr := ""
	if !now.IsZero() {
		nowStr = ts(now)
	}
	vw := &View{Now: nowStr, Heads: map[string]int{}, RolesProblems: rolesProblems(r.g.raw)}
	for k, q := range r.heads {
		vw.Heads[k] = q
	}

	// events
	for _, n := range r.nodes {
		e := EventView{ID: n.id, Alias: n.alias, Stream: n.stream, Member: n.member, Seq: n.seq, TS: ts(n.ts),
			Type: n.typ, Origin: n.origin, Payload: n.ev.Payload, Status: n.status, Reason: nz(n.reason),
			Ignored: n.ignored, Markers: []Marker{}, Superseded: []Superseded{}, Thread: nz(n.thread),
			ParentThread: nz(n.parentThread)}
		if len(e.Payload) == 0 || string(e.Payload) == "null" {
			e.Payload = json.RawMessage("{}")
		}
		for _, m := range n.markers {
			mk := Marker{Kind: m.kind, Field: m.field, Other: m.other, Winner: m.winner, Mine: nz(m.mine), Theirs: nz(m.theirs)}
			if o := r.byID[m.other]; o != nil {
				mk.OtherMember = &o.member
			}
			e.Markers = append(e.Markers, mk)
		}
		for _, x := range n.superseded {
			e.Superseded = append(e.Superseded, Superseded{Field: x.field, By: x.by, Why: x.why})
		}
		vw.Events = append(vw.Events, e)
	}

	// assignments
	vw.Assignments = map[string]Assignment{}
	var A []*Assignment
	for _, x := range S.recs {
		if x.fallback && x.state != "live" {
			continue // an ended fallback period holds nothing worth showing
		}
		last := x.ts
		if x.last != nil {
			last = x.last.ts
		}
		a := Assignment{ID: x.id, Item: x.item, To: x.to, By: x.by, Sender: x.sender, Completion: x.completion,
			State: x.state, Forced: x.forced, Status: x.status, Note: nz(x.note), NoteBy: nz(x.noteBy), Via: x.via,
			TS: ts(x.ts), Inherited: x.inherited != "", HandoffOf: nz(x.handoffOf), HandoffNote: nz(x.handoffNote),
			AnswerNote: nz(x.answerNote), Restored: ref(x.restored), Restores: nz(x.restores), Accepted: ref(x.accepted),
			LeadAtAccept: nz(x.leadAtAccept), StatusBy: nz(x.noteBy), LastUpdate: ts(last), lastUpdate: last, Fallback: x.fallback}
		if ru := S.ruleByID[x.rule]; ru != nil {
			a.Rule, a.RuleRequest, a.RuleLevels = &ru.id, x.ruleRequest, ruleLevels(ru)
		}
		if q := S.asg[x.handoffOf]; x.handoffOf != "" && q != nil {
			a.HandoffFrom = &q.to
		}
		if x.ended != nil {
			a.Ended = &Ended{ID: x.ended.id, Type: x.ended.typ, TS: ts(x.ended.ts), By: x.ended.member}
		}
		if x.to == x.by {
			zero := 0
			a.Depth = &zero
		} else if d, ok := reachMap(S.reach(x.by, 99))[x.to]; ok {
			a.Depth = &d
		}
		if x.last != nil {
			for _, w := range x.writes {
				if w == x.last {
					a.StatusOrigin = &x.last.origin
					break
				}
			}
		}
		a.Stale = x.state == "live" && !now.IsZero() && (x.status == "doing" || x.status == "blocked") &&
			now.Sub(last).Hours()/24 >= float64(staleDays)
		vw.Assignments[x.id] = a
		A = append(A, &a)
	}

	// items
	lastAct := map[string]time.Time{}
	threads := map[string][]string{}
	for _, n := range r.nodes {
		if n.status == "applied" && n.thread != "" && n.ts.After(lastAct[n.thread]) {
			lastAct[n.thread] = n.ts
		}
		for _, t := range []string{n.thread, n.parentThread} {
			if t != "" {
				threads[t] = append(threads[t], n.id)
			}
		}
	}
	// (528) Sibling order: each placement (a create, or a move the item's
	// position register shows), in fold order, goes right after its after
	// anchor, first for "", last when the field is absent (every item
	// created before it existed) or the anchor is not a sibling placed so
	// far. A hidden anchor keeps its place, so hiding a row moves no other
	// row; a moved item leaves a vacated slot behind, so a row anchored on it
	// stays where it was, and an anchor found only as vacated slots takes the
	// last of them. Concurrent inserts after one anchor: the later in fold
	// order lands nearer the anchor.
	type slot struct {
		id   string
		gone bool
	}
	slots := map[string][]slot{}
	where := map[string]string{}
	for _, pl := range S.place {
		if old, ok := where[pl.id]; ok {
			for i := range slots[old] {
				if x := &slots[old][i]; x.id == pl.id && !x.gone {
					x.gone = true
					break
				}
			}
		}
		key := ""
		if pl.parent != "" && S.items[pl.parent] != nil {
			key = pl.parent
		}
		l, at := slots[key], -1
		if pl.afterSet && pl.after != "" {
			for i, x := range l {
				if x.id == pl.after {
					at = i
					if !x.gone {
						break
					}
				}
			}
		}
		x := slot{id: pl.id}
		switch {
		case pl.afterSet && pl.after == "":
			l = append([]slot{x}, l...)
		case at < 0:
			l = append(l, x)
		default:
			l = append(l[:at+1], append([]slot{x}, l[at+1:]...)...)
		}
		slots[key] = l
		where[pl.id] = key
	}
	sib := map[string][]string{}
	for key, l := range slots {
		for _, x := range l {
			if !x.gone {
				sib[key] = append(sib[key], x.id)
			}
		}
	}
	// (531) Backlinks: the shown items, then the shown comments on shown
	// items, that reference an item, each once, in fold order; an item's own
	// thread excluded.
	backrefs := map[string][]string{}
	backref := func(t, from string) {
		if !contains(backrefs[t], from) {
			backrefs[t] = append(backrefs[t], from)
		}
	}
	for _, id := range S.iorder {
		if it := S.items[id]; !it.hidden {
			for _, t := range it.refs {
				if t != id && S.items[t] != nil {
					backref(t, id)
				}
			}
		}
	}
	for _, id := range S.corder {
		q := S.comments[id]
		if host := S.items[q.item]; !q.hidden && host != nil && !host.hidden {
			for _, t := range q.refs {
				if t != q.item && S.items[t] != nil {
					backref(t, id)
				}
			}
		}
	}

	vw.Items = map[string]ItemView{}
	status := map[string]string{}
	for _, id := range S.iorder {
		status[id] = r.itemStatus(S.items[id])
	}
	subtasks := func(id string) []*item {
		var out []*item
		for _, k := range S.iorder {
			if c := S.items[k]; c.parent == id && !c.hidden {
				out = append(out, c)
			}
		}
		return out
	}
	for _, id := range S.iorder {
		it := S.items[id]
		var chain []string
		for cur, d := it, 0; cur != nil && d < 50; d++ {
			chain = append(chain, cur.author)
			if cur.parent == "" {
				break
			}
			cur = S.items[cur.parent]
		}
		var started *time.Time
		var recIDs, pend []string
		for _, x := range S.recsOn(id) {
			if !x.fallback || x.state == "live" {
				recIDs = append(recIDs, x.id)
			}
			if x.state == "pending" {
				pend = append(pend, x.id)
			}
			for _, w := range x.writes {
				if w.typ == TypeItemStatus && w.p.str("status") == "doing" && (started == nil || w.ts.Before(*started)) {
					t := w.ts
					started = &t
				}
			}
		}
		lv := S.prio.level(it.priority)
		iv := ItemView{ID: id, Alias: it.alias, Kind: it.kind, Title: it.title, Body: it.body, Project: it.project, Labels: S.liveLabels(it.labels),
			Due: it.due, Priority: it.priority, PriorityLevel: lv, PriorityName: S.prio.names[lv], Author: it.author,
			TS: ts(it.ts), Hidden: it.hidden, Edited: ref(it.edited), Status: nz(status[id]),
			Counted: r.counted(it, status[id]), Children: []string{}, Assignments: orEmpty(recIDs), Pending: orEmpty(pend),
			Thread: orEmpty(threads[id]), Depth: len(chain) - 1,
			Mentions: append([]string{}, it.mentions...), Attachments: attachmentViews(it.attachments),
			Refs: append([]string{}, it.refs...), Backrefs: orEmpty(backrefs[id]), Requests: []string{}, CC: []string{}}
		for _, e := range it.cc {
			if S.liveMember(e.member) {
				iv.CC = append(iv.CC, e.member)
			}
		}
		if o := S.activeOpen(it); o != nil {
			iv.Open = &OpenView{ID: o.node.id, By: o.by, Scope: o.scope, TS: ts(o.node.ts)}
			iv.Claimable = !it.hidden && S.liveFallback(id).to != v && S.inScope(o, v)
		}
		for _, rid := range S.rorder {
			if S.reqs[rid].item == id {
				iv.Requests = append(iv.Requests, rid)
			}
		}
		if it.parent != "" && S.items[it.parent] != nil {
			iv.Parent = &it.parent
		}
		iv.LastActivity = ts(it.ts)
		if t, ok := lastAct[id]; ok {
			iv.LastActivity = ts(t)
		}
		if started != nil {
			s := ts(*started)
			iv.Started = &s
		}
		iv.Children = append(iv.Children, sib[id]...)
		// roll-up
		iv.Rollup = Rollup{Counts: map[string]int{}, Blocked: []string{}}
		for _, k := range subtasks(id) {
			if r.counted(k, status[k.id]) {
				iv.Rollup.Total++
				iv.Rollup.Counts[status[k.id]]++
			}
		}
		var walk func(id string, depth int)
		walk = func(id string, depth int) {
			if depth > 30 {
				return
			}
			for _, k := range subtasks(id) {
				if r.counted(k, status[k.id]) && status[k.id] == "blocked" {
					iv.Rollup.Blocked = append(iv.Rollup.Blocked, k.id)
				}
				walk(k.id, depth+1)
			}
		}
		walk(id, 0)
		for _, t := range iv.Thread {
			if len(r.byID[t].markers) > 0 {
				iv.Concurrent++
			}
		}
		for i := len(chain) - 1; i >= 0; i-- {
			iv.Chain = append(iv.Chain, chain[i])
		}
		vw.Items[id] = iv
	}
	vw.ItemOrder = []string{}
	var walkOrder func(parent string)
	walkOrder = func(parent string) {
		for _, id := range sib[parent] {
			vw.ItemOrder = append(vw.ItemOrder, id)
			walkOrder(id)
		}
	}
	walkOrder("")

	vw.Requests = map[string]RequestView{}
	for _, rid := range S.rorder {
		q := S.reqs[rid]
		rv := RequestView{ID: rid, Item: q.item, By: q.by, Assignee: q.holder, Assignment: nz(q.of), State: q.state,
			Note: nz(q.note), AnswerNote: nz(q.answerNote), TS: ts(q.node.ts)}
		if q.ended != nil {
			rv.Ended = &Ended{ID: q.ended.id, Type: q.ended.typ, TS: ts(q.ended.ts), By: q.ended.member}
		}
		vw.Requests[rid] = rv
	}

	vw.Rules = map[string]RuleView{}
	for _, ru := range S.rules {
		rv := RuleView{ID: ru.id, Item: ru.item, Via: ru.via, By: ru.by, Root: ru.root, IncludeRoot: ru.includeRoot,
			Levels: ruleLevels(ru), Completion: ru.completion, State: ru.state, Request: nz(ru.request), Ended: nz(ru.ended), TS: ts(ru.ts)}
		if ru.ended != "" {
			rv.State = "ended"
		}
		vw.Rules[ru.id] = rv
	}

	vw.Comments = map[string]CommentView{}
	for _, id := range S.corder {
		q := S.comments[id]
		cv := CommentView{ID: id, Item: q.item, Author: q.author, Body: q.body, ReplyTo: nz(q.replyTo), TS: ts(q.ts),
			Edited: ref(q.edited), Mentions: append([]string{}, q.mentions...), Attachments: attachmentViews(q.attachments),
			Refs: append([]string{}, q.refs...)}
		if q.hiddenBy != nil {
			cv.Hidden = &q.hiddenBy.id
		}
		vw.Comments[id] = cv
	}

	vw.Members = map[string]MemberView{}
	for _, id := range S.morder {
		m := S.members[id]
		mv := MemberView{ID: id, Name: m.profile.DisplayName(), Profile: m.profile, Avatar: m.profile.Avatar(), Leads: orEmpty(append([]string(nil), m.leads...)), Keys: []string{},
			Revoked: []RevokedKey{}, Admin: S.admins[id], Children: orEmpty(append([]string(nil), children[id]...))}
		for _, k := range m.keys {
			mv.Keys = append(mv.Keys, k.text)
		}
		for _, k := range m.revoked {
			mv.Revoked = append(mv.Revoked, RevokedKey{Key: k.key.text, By: k.by.id})
		}
		if m.removed != nil {
			mv.Removed = &m.removed.id
		}
		if m.joined != nil {
			mv.Joined = &m.joined.id
		}
		if d, ok := R[id]; ok {
			mv.Depth = &d
		}
		switch {
		case len(m.leads) == 0:
			mv.Role = "root"
		case len(children[id]) > 0:
			mv.Role = "lead"
		default:
			mv.Role = "member"
		}
		vw.Members[id] = mv
	}

	vw.Viewer = Viewer{Member: v, Stream: stream, Name: v, Admin: S.admins[v], Followers: orEmpty(append([]string(nil), children[v]...)),
		Reach: R, IsMember: me != nil && me.removed == nil}
	if me != nil {
		vw.Viewer.Name = me.profile.DisplayName()
		pr := me.profile
		vw.Viewer.Profile = &pr
		vw.Viewer.Lead = nz(me.lead())
	}
	vw.Priority = PriorityLevels{Names: append([]string(nil), S.prio.names...), Default: S.prio.def}
	vw.Attachments = AttachmentLimit{MaxBytes: S.attMax}
	vw.Labels = []LabelView{}
	for _, id := range S.lorder {
		if l := S.liveLabel(id); l != nil {
			vw.Labels = append(vw.Labels, LabelView{ID: id, Name: l.name, Color: l.color})
		}
	}
	vw.Admins = append([]string{}, S.aorder...)

	r.viewerLists(vw, A, v, children)

	vw.Stats = Stats{Events: len(vw.Events)}
	for _, e := range vw.Events {
		if e.Status != "applied" {
			vw.Stats.Flagged++
		}
		if len(e.Markers) > 0 {
			vw.Stats.Concurrent++
		}
	}
	return vw
}

// ruleLevels is a rule's levels for the view: nil for the whole subtree.
func ruleLevels(ru *rule) *float64 {
	if ru.levels == 0 {
		return nil
	}
	l := ru.levels
	return &l
}

func attachmentViews(l []attachment) []AttachmentView {
	out := make([]AttachmentView, len(l))
	for i, a := range l {
		out[i] = AttachmentView{OID: a.oid, Name: a.name, Size: a.size, Mime: a.mime}
	}
	return out
}

func orEmpty(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

// teamLoad are the statuses a team row's Load counts (613 a): the work in
// the member's hands; idea, awaiting (closed, waiting on the assigner's
// confirm), and done are not.
var teamLoad = map[string]bool{"todo": true, "doing": true, "blocked": true}

var rank = map[string]int{"blocked": 0, "doing": 1, "todo": 2, "idea": 3, "awaiting": 4, "done": 5}

func ids(l []*Assignment) []string {
	out := make([]string, len(l))
	for i, a := range l {
		out[i] = a.ID
	}
	return out
}

func filter(l []*Assignment, f func(*Assignment) bool) []*Assignment {
	var out []*Assignment
	for _, a := range l {
		if f(a) {
			out = append(out, a)
		}
	}
	return out
}

// viewerLists fills the viewer-specific lists: my, notices.
func (r *replayed) viewerLists(vw *View, A []*Assignment, v string, children map[string][]string) {
	items := vw.Items
	visible := func(a *Assignment) bool { it, ok := items[a.Item]; return ok && !it.Hidden }
	// (153 b) a held topic is ownership only: in my.assignments, received and
	// the pending badge, never in a status-based list or count
	notTopic := func(a *Assignment) bool { return items[a.Item].Kind != "topic" }
	byRank := func(l []*Assignment) {
		sort.SliceStable(l, func(i, j int) bool { return rank[l[i].Status] < rank[l[j].Status] })
	}
	byUrgency := func(l []*Assignment) { // priority desc, then due asc (none last), then status
		sort.SliceStable(l, func(i, j int) bool {
			ix, iy := items[l[i].Item], items[l[j].Item]
			if ix.PriorityLevel != iy.PriorityLevel {
				return ix.PriorityLevel > iy.PriorityLevel
			}
			dx, dy := "9999", "9999"
			if ix.Due != nil {
				dx = *ix.Due
			}
			if iy.Due != nil {
				dy = *iy.Due
			}
			if dx != dy {
				return dx < dy
			}
			return rank[l[i].Status] < rank[l[j].Status]
		})
	}
	mine := filter(A, func(a *Assignment) bool { return a.To == v && a.State == "live" && visible(a) })
	pendingToMe := filter(A, func(a *Assignment) bool { return a.To == v && a.State == "pending" && visible(a) })
	answered := filter(A, func(a *Assignment) bool {
		return a.To == v && !a.Forced && a.State != "pending" && (a.Accepted != nil || a.State == "declined") && visible(a)
	})
	sent := filter(A, func(a *Assignment) bool { return a.Sender == v && a.To != v && !a.Forced && visible(a) })
	confirms := filter(A, func(a *Assignment) bool {
		return a.By == v && a.To != v && a.State == "live" && a.Status == "awaiting"
	})
	my := My{Given: []string{}, Created: []string{}, Team: []Team{}, Badge: []string{},
		Requests: MyReqs{Incoming: []string{}, Outgoing: []string{}}}
	for _, rid := range r.S.rorder {
		q := r.S.reqs[rid]
		it := r.S.items[q.item]
		if q.state != "pending" || it == nil || it.hidden {
			continue
		}
		if q.by == v {
			my.Requests.Outgoing = append(my.Requests.Outgoing, rid)
		} else if q.holder == v || it.author == v || r.S.creatorGone(it, v) {
			my.Requests.Incoming = append(my.Requests.Incoming, rid)
		}
	}
	for _, id := range r.S.iorder {
		it := items[id]
		if it.Hidden {
			continue
		}
		given, open := false, false
		for _, a := range A {
			if a.Item != id || (a.State != "live" && a.State != "pending") {
				continue
			}
			open = true
			given = given || ((a.By == v || a.Sender == v) && a.To != v)
		}
		if given {
			my.Given = append(my.Given, id)
		}
		if it.Author == v && it.Kind != "topic" && !open {
			my.Created = append(my.Created, id)
		}
	}

	// Notices are derived, never events. accepted: to the accepter's lead at
	// accept time; handoff: to the original assigner when a handoff is
	// accepted; done / blocked: to the assigner when someone else set that
	// status on a live assignment (an awaiting one is in the confirm list);
	// declined / returned (285): to the sender, and for a returned handoff
	// also to the original assigner (291).
	vw.Notices = []Notice{}
	for _, a := range A {
		base := Notice{Assignment: a.ID, Item: a.Item, To: a.To, By: a.By, Sender: a.Sender, State: a.State}
		if a.Accepted != nil {
			nt := base
			nt.TS = a.Accepted.TS
			if a.LeadAtAccept != nil && *a.LeadAtAccept == v {
				nt.Key, nt.Kind = "acc:"+a.ID, "accepted"
				vw.Notices = append(vw.Notices, nt)
			} else if o, ok := vw.Assignments[deref(a.HandoffOf)]; a.HandoffOf != nil && ok && o.By == v {
				nt.Key, nt.Kind, nt.From, nt.HasFrom = "ho:"+a.ID, "handoff", a.HandoffFrom, true
				vw.Notices = append(vw.Notices, nt)
			}
		}
		if a.By == v && a.To != v && a.State == "live" && (a.Status == "done" || a.Status == "blocked") &&
			a.StatusBy != nil && *a.StatusBy != v {
			nt := base
			nt.TS, nt.Kind = a.LastUpdate, a.Status
			nt.Key = "w:" + a.ID + ":" + a.Status + ":" + a.LastUpdate
			vw.Notices = append(vw.Notices, nt)
		}
		if a.To == v || a.Ended == nil {
			continue
		}
		nt := base
		nt.TS, nt.EndedBy, nt.Note, nt.From, nt.Restores = a.Ended.TS, a.Ended.By, a.AnswerNote, a.HandoffFrom, a.Restores
		nt.HasFrom, nt.Answered = true, true
		if a.State == "declined" && a.Sender == v {
			d := nt
			d.Key, d.Kind = "dec:"+a.ID, "declined"
			vw.Notices = append(vw.Notices, d)
		}
		if a.State == "returned" && (a.Sender == v || (a.HandoffOf != nil && a.By == v)) && a.Ended.By != v {
			d := nt
			d.Key, d.Kind = "ret:"+a.ID, "returned"
			vw.Notices = append(vw.Notices, d)
		}
	}

	// The team roll-up: the viewer's subtree in tree order, then for an admin
	// every other live member, walked from the org-tree roots.
	today := ""
	if vw.Now != "" {
		today = vw.Now[:10]
	}
	R := vw.Viewer.Reach
	inTeam := map[string]bool{v: true}
	teamRow := func(id string, depth int, outside bool) {
		inTeam[id] = true
		recs := filter(A, func(a *Assignment) bool { return a.To == id && visible(a) && notTopic(a) })
		open := filter(recs, func(a *Assignment) bool { return a.State == "live" && a.Status != "done" })
		byRank(open)
		pend := filter(recs, func(a *Assignment) bool { return a.State == "pending" })
		waiting := filter(A, func(a *Assignment) bool {
			return a.Sender == id && a.To != id && a.State == "pending" && visible(a) && notTopic(a)
		})
		t := Team{ID: id, Depth: depth, Outside: outside, Open: ids(open), Pending: ids(pend), Waiting: ids(waiting),
			Counts: map[string]int{}}
		var last time.Time
		for _, a := range recs {
			if a.lastUpdate.After(last) {
				last = a.lastUpdate
			}
			if a.State != "live" {
				continue
			}
			if a.Fallback {
				if a.Status != "done" {
					t.Fallback++
				}
				continue
			}
			t.Counts[a.Status]++
			if !teamLoad[a.Status] {
				continue
			}
			t.Load++
			if a.Stale {
				t.Stale++
			}
			if due := items[a.Item].Due; today != "" && due != nil && *due != "" && *due < today {
				t.Overdue++
			}
		}
		if len(recs) > 0 {
			s := ts(last)
			t.Last = &s
		}
		my.Team = append(my.Team, t)
	}
	var walk func(lead string)
	walk = func(lead string) {
		for _, id := range children[lead] {
			d, ok := R[id]
			if inTeam[id] || !ok {
				continue
			}
			teamRow(id, d, false)
			walk(id)
		}
	}
	walk(v)
	if r.S.admins[v] {
		var outside func(list []string, depth int)
		outside = func(list []string, depth int) {
			for _, id := range list {
				if inTeam[id] {
					continue
				}
				inTeam[id] = true // a removed root is passed through, not shown
				if r.S.liveMember(id) {
					teamRow(id, depth, true)
				}
				outside(children[id], depth+1)
			}
		}
		var roots []string
		for _, id := range r.S.morder {
			if len(r.S.members[id].leads) == 0 {
				roots = append(roots, id)
			}
		}
		outside(roots, 0)
		// a member no root reaches (a lead chain through a removed lead) roots
		// its own branch
		for _, id := range r.S.morder {
			if r.S.liveMember(id) {
				outside([]string{id}, 0)
			}
		}
	}

	for _, a := range mine {
		if a.By != v && a.Forced && a.Status == "todo" && notTopic(a) {
			my.Badge = append(my.Badge, "a:"+a.ID)
		}
	}
	for _, a := range pendingToMe {
		my.Badge = append(my.Badge, "p:"+a.ID)
	}
	for _, rid := range my.Requests.Incoming {
		my.Badge = append(my.Badge, "q:"+rid)
	}
	for _, nt := range vw.Notices {
		my.Badge = append(my.Badge, nt.Key)
	}

	// The engine sorts mine and pendingToMe in place: the work list sorts the
	// rank-sorted assignments by urgency, and received.pending is the
	// urgency-sorted list.
	byRank(mine)
	my.Assignments = ids(mine)
	work := filter(mine, func(a *Assignment) bool { return a.Status != "done" && a.Status != "awaiting" && notTopic(a) })
	byUrgency(work)
	byUrgency(pendingToMe)
	my.Today = Today{Work: ids(work), Pending: ids(pendingToMe), Confirms: ids(confirms)}
	my.Received = Received{Pending: ids(pendingToMe), Answered: ids(answered), Sent: ids(sent)}
	my.Related, my.CCNotices = r.related(vw, v)
	vw.My = my
}

// related derives the viewer's Related lists and CC notices.
func (r *replayed) related(vw *View, v string) (Related, []CCNotice) {
	S := r.S
	rel := Related{CC: []string{}, Mentioned: []string{}, Closed: []string{}}
	notices := []CCNotice{}
	// involved: the viewer created the item or holds a live or pending
	// record on it; those items show in the viewer's own scopes.
	involved := func(it *item) bool {
		if it.author == v {
			return true
		}
		for _, x := range S.recsOn(it.id) {
			if x.to == v && (x.state == "live" || x.state == "pending") {
				return true
			}
		}
		return false
	}
	shown := func(it *item) bool { return !it.hidden && !involved(it) }
	done := func(id string) bool { st := vw.Items[id].Status; return st != nil && *st == "done" }
	ccOf := func(it *item) *ccEntry {
		for i := range it.cc {
			if it.cc[i].member == v {
				return &it.cc[i]
			}
		}
		return nil
	}
	// written is the event that set a register's shown value, else first.
	written := func(key string, first *node) *node {
		if len(S.reg[key]) > 0 {
			return S.regWinner(key)
		}
		return first
	}
	type dated struct {
		id string
		at time.Time
	}
	var cc, men []dated
	for _, id := range S.iorder {
		it := S.items[id]
		if !shown(it) {
			continue
		}
		if e := ccOf(it); e != nil {
			if done(id) {
				rel.Closed = append(rel.Closed, id)
			} else {
				cc = append(cc, dated{id, e.since.ts})
			}
			continue
		}
		var at time.Time
		found := false
		if contains(it.mentions, v) {
			at, found = written("it:"+id+":body", it.node).ts, true
		}
		for _, cid := range S.corder {
			q := S.comments[cid]
			if q.item != id || q.hidden || q.author == v || !contains(q.mentions, v) {
				continue
			}
			if t := written("cm:"+cid, r.byID[cid]).ts; !found || t.After(at) {
				at, found = t, true
			}
		}
		if !found {
			continue
		}
		if done(id) {
			rel.Closed = append(rel.Closed, id)
		} else {
			men = append(men, dated{id, at})
		}
	}
	for _, l := range []*[]dated{&cc, &men} {
		x := *l
		sort.SliceStable(x, func(i, j int) bool { return x[i].at.After(x[j].at) })
	}
	for _, d := range cc {
		rel.CC = append(rel.CC, d.id)
	}
	for _, d := range men {
		rel.Mentioned = append(rel.Mentioned, d.id)
	}

	for _, n := range r.order {
		if n.status != "applied" || n.member == v {
			continue
		}
		switch n.typ {
		case TypeItemStatus, TypeItemClose, TypeItemConfirm, TypeItemReject, TypeItemReopen, TypeCommentAdd:
		default:
			continue
		}
		it := S.items[n.p.str("item")]
		if it == nil || !shown(it) {
			continue
		}
		e := ccOf(it)
		if e == nil || n.pos <= e.since.pos {
			continue
		}
		nt := CCNotice{Key: "cc:" + n.id, Type: n.typ, Item: it.id, Event: n.id, By: n.member, TS: ts(n.ts)}
		if n.typ == TypeCommentAdd {
			q := S.comments[n.id]
			if q == nil || q.hidden || contains(q.mentions, v) {
				continue
			}
			if to := S.comments[q.replyTo]; to != nil && to.author == v {
				continue
			}
		} else {
			who := n.member
			if n.typ != TypeItemStatus && n.typ != TypeItemClose && n.p.truthy("assignee") {
				who = n.p.str("assignee")
			}
			nt.To = &who
			if n.typ == TypeItemStatus {
				nt.Status = nz(n.p.str("status"))
			}
		}
		notices = append(notices, nt)
	}
	return rel, notices
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
