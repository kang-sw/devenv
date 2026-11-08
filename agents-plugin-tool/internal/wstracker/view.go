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
	Members       map[string]MemberView  `json:"members"`
	Admins        []string               `json:"admins"`
	RolesProblems []string               `json:"rolesProblems"`
	Items         map[string]ItemView    `json:"items"`
	ItemOrder     []string               `json:"itemOrder"`
	Assignments   map[string]Assignment  `json:"assignments"`
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

type PriorityLevels struct {
	Names   []string `json:"names"`
	Default int      `json:"default"`
}

// AttachmentLimit is the store's attachments.max_bytes setting.
type AttachmentLimit struct {
	MaxBytes int `json:"max_bytes"`
}

// AttachmentView is the metadata of one attachment; OID is the content's
// sha256.
type AttachmentView struct {
	OID  string `json:"oid"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	Mime string `json:"mime"`
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
	// Mentions and Attachments follow the shown body write.
	Mentions    []string         `json:"mentions"`
	Attachments []AttachmentView `json:"attachments"`
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

type CommentView struct {
	ID      string  `json:"id"`
	Item    string  `json:"item"`
	Author  string  `json:"author"`
	Body    string  `json:"body"`
	ReplyTo *string `json:"reply_to"`
	TS      string  `json:"ts"`
	Edited  *Ref    `json:"edited"`
	Hidden  *string `json:"hidden"` // the comment.hide event id
	// Mentions and Attachments follow the shown text write.
	Mentions    []string         `json:"mentions"`
	Attachments []AttachmentView `json:"attachments"`
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

// Notice is a derived notice (285): accepted, handoff, declined, returned.
// Key is "acc:", "ho:", "dec:", or "ret:" plus the assignment id.
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
	Badge       []string `json:"badge"`
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

type Team struct {
	ID      string   `json:"id"`
	Depth   int      `json:"depth"`
	Open    []string `json:"open"`
	Pending []string `json:"pending"`
	Waiting []string `json:"waiting"`
	Done    int      `json:"done"`
	Blocked int      `json:"blocked"`
	Stale   int      `json:"stale"`
	Last    *string  `json:"last"`
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

// itemStatus derives an item's own status from its live assignments.
func (r *replayed) itemStatus(it *item) string {
	if it.kind == "topic" {
		return ""
	}
	recs := r.S.recsOn(it.id)
	var live []*rec
	pending := false
	var last *rec
	for _, x := range recs {
		switch {
		case x.state == "live":
			live = append(live, x)
		case x.state == "pending":
			pending = true
		}
		// the latest ended one; among ties (one event ended several) the first
		if x.ended != nil && (last == nil || later(x.ended, last.ended)) {
			last = x
		}
	}
	if len(live) == 0 {
		if pending {
			return "pending"
		}
		if last != nil && (last.state == "declined" || last.state == "returned") {
			return last.state
		}
		return "unassigned"
	}
	all, blocked, awaiting, doing := true, false, false, false
	for _, x := range live {
		all = all && x.status == "done"
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
	}
	return "todo"
}

// counted: pending assignments are excluded from status and roll-up, so an
// item whose only assignments are pending (or declined / returned) is not
// counted.
func (r *replayed) counted(it *item, st string) bool {
	return it.kind != "topic" && st != "pending" && st != "declined" && st != "returned"
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
		last := x.ts
		if x.last != nil {
			last = x.last.ts
		}
		a := Assignment{ID: x.id, Item: x.item, To: x.to, By: x.by, Sender: x.sender, Completion: x.completion,
			State: x.state, Forced: x.forced, Status: x.status, Note: nz(x.note), NoteBy: nz(x.noteBy), Via: x.via,
			TS: ts(x.ts), Inherited: x.inherited != "", HandoffOf: nz(x.handoffOf), HandoffNote: nz(x.handoffNote),
			AnswerNote: nz(x.answerNote), Restored: ref(x.restored), Restores: nz(x.restores), Accepted: ref(x.accepted),
			LeadAtAccept: nz(x.leadAtAccept), StatusBy: nz(x.noteBy), LastUpdate: ts(last), lastUpdate: last}
		for _, ru := range S.rules {
			if ru.id == x.via {
				l := ru.levels
				a.RuleLevels = &l
			}
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
			recIDs = append(recIDs, x.id)
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
		iv := ItemView{ID: id, Alias: it.alias, Kind: it.kind, Title: it.title, Body: it.body, Project: it.project,
			Due: it.due, Priority: it.priority, PriorityLevel: lv, PriorityName: S.prio.names[lv], Author: it.author,
			TS: ts(it.ts), Hidden: it.hidden, Edited: ref(it.edited), Status: nz(status[id]),
			Counted: r.counted(it, status[id]), Children: []string{}, Assignments: orEmpty(recIDs), Pending: orEmpty(pend),
			Thread: orEmpty(threads[id]), Depth: len(chain) - 1,
			Mentions: append([]string{}, it.mentions...), Attachments: attachmentViews(it.attachments)}
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
		for _, k := range S.iorder {
			if S.items[k].parent == id {
				iv.Children = append(iv.Children, k)
			}
		}
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
		for _, id := range S.iorder {
			p := ""
			if pp := vw.Items[id].Parent; pp != nil {
				p = *pp
			}
			if p == parent {
				vw.ItemOrder = append(vw.ItemOrder, id)
				walkOrder(id)
			}
		}
	}
	walkOrder("")

	vw.Comments = map[string]CommentView{}
	for _, id := range S.corder {
		q := S.comments[id]
		cv := CommentView{ID: id, Item: q.item, Author: q.author, Body: q.body, ReplyTo: nz(q.replyTo), TS: ts(q.ts),
			Edited: ref(q.edited), Mentions: append([]string{}, q.mentions...), Attachments: attachmentViews(q.attachments)}
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

var rank = map[string]int{"blocked": 0, "doing": 1, "todo": 2, "awaiting": 3, "done": 4}

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
	my := My{Given: []string{}, Created: []string{}, Team: []Team{}, Badge: []string{}}
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
	// accepted; declined / returned (285): to the sender, and for a returned
	// handoff also to the original assigner (291).
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

	R3 := reachMap(r.S.reach(v, 3))
	var walk func(lead string)
	walk = func(lead string) {
		for _, id := range children[lead] {
			d, ok := R3[id]
			if !ok {
				continue
			}
			recs := filter(A, func(a *Assignment) bool { return a.To == id && visible(a) })
			open := filter(recs, func(a *Assignment) bool { return a.State == "live" && a.Status != "done" })
			byRank(open)
			pend := filter(recs, func(a *Assignment) bool { return a.State == "pending" })
			waiting := filter(A, func(a *Assignment) bool { return a.Sender == id && a.To != id && a.State == "pending" && visible(a) })
			t := Team{ID: id, Depth: d, Open: ids(open), Pending: ids(pend), Waiting: ids(waiting)}
			var last time.Time
			for _, a := range recs {
				if a.State == "live" && a.Status == "done" {
					t.Done++
				}
				if a.lastUpdate.After(last) {
					last = a.lastUpdate
				}
			}
			for _, a := range open {
				if a.Status == "blocked" {
					t.Blocked++
				}
				if a.Stale {
					t.Stale++
				}
			}
			if len(recs) > 0 {
				s := ts(last)
				t.Last = &s
			}
			my.Team = append(my.Team, t)
			if d < 3 {
				walk(id)
			}
		}
	}
	walk(v)

	for _, a := range mine {
		if a.By != v && a.Forced && a.Status == "todo" {
			my.Badge = append(my.Badge, "a:"+a.ID)
		}
	}
	for _, a := range pendingToMe {
		my.Badge = append(my.Badge, "p:"+a.ID)
	}
	for _, nt := range vw.Notices {
		my.Badge = append(my.Badge, nt.Key)
	}

	// The engine sorts mine and pendingToMe in place: the work list sorts the
	// rank-sorted assignments by urgency, and received.pending is the
	// urgency-sorted list.
	byRank(mine)
	my.Assignments = ids(mine)
	work := filter(mine, func(a *Assignment) bool { return a.Status != "done" && a.Status != "awaiting" })
	byUrgency(work)
	byUrgency(pendingToMe)
	my.Today = Today{Work: ids(work), Pending: ids(pendingToMe), Confirms: ids(confirms)}
	my.Received = Received{Pending: ids(pendingToMe), Answered: ids(answered), Sent: ids(sent)}
	vw.My = my
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
