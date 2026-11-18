package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

var _ wsdashboard.TrackerProjectionHost = (*dashboardTracker)(nil)

func (t *dashboardTracker) Projection(ctx context.Context, id string) (wsdashboard.TrackerProjection, error) {
	if t.err != nil {
		return wsdashboard.TrackerProjection{}, redacted(t.err)
	}
	v, err := t.h.ViewWithin(ctx, id, 0, dashboardViewWait)
	if err != nil {
		return wsdashboard.TrackerProjection{}, redacted(err)
	}
	seen, err := t.h.Seen(id)
	if err != nil {
		return wsdashboard.TrackerProjection{}, redacted(err)
	}
	return projectTracker(id, v, wsdashboard.TrackerSeen{Badge: seen.Badge, Read: seen.Read})
}

// Phase 2 wire contract: row.data.item is ItemView minus body, bodyRev and
// attachments (thread contains ONLY event ids). The sibling assignments,
// requests and rules maps and notices/ccNotices lists belong to this item, not
// Globals. Consumers flatten these keyed records into their local view model.
// Detail data has body/bodyRev/attachments, comments keyed by id, events in
// thread order, and the existing html.items/html.comments shape. The dashboard
// wraps it with the row's detailVersion, never a version from a later host read.
// Feed uses plain strings, NOT HTML: snippet is at most 80 runes (ellipsis
// included); actors are member ids for client-side naming/localization. Variant
// fields preserve the sentence choices in tracker-panel.js without details.
type trackerItemProjection struct {
	Item        map[string]any                   `json:"item"`
	Assignments map[string]wstracker.Assignment  `json:"assignments"`
	Requests    map[string]wstracker.RequestView `json:"requests"`
	Rules       map[string]wstracker.RuleView    `json:"rules"`
	Notices     []wstracker.Notice               `json:"notices"`
	CCNotices   []wstracker.CCNotice             `json:"ccNotices"`
	Time        trackerItemTime                  `json:"time"`
	Flagged     int                              `json:"flagged"`
}

// Time makes browser-derived due chips and the 14-day recently-closed fold
// independently diffable without putting a ticking clock in every row. Stale
// is already present on each assignment. recentAssignments covers record rows;
// recentlyClosed uses the latest done live assignment, or lastActivity.
type trackerItemTime struct {
	DueState          string          `json:"dueState"`
	RecentlyClosed    bool            `json:"recentlyClosed"`
	RecentAssignments map[string]bool `json:"recentAssignments"`
}

type trackerDetailInput struct {
	Item        wstracker.ItemView               `json:"item"`
	Comments    map[string]wstracker.CommentView `json:"comments"`
	Events      []wstracker.EventView            `json:"events"`
	Summary     trackerItemProjection            `json:"summary"`
	ContextHash string                           `json:"contextHash"`
}

type trackerDetailData struct {
	Body        string                           `json:"body"`
	BodyRev     []string                         `json:"bodyRev"`
	Attachments []wstracker.AttachmentView       `json:"attachments"`
	Comments    map[string]wstracker.CommentView `json:"comments"`
	Events      []wstracker.EventView            `json:"events"`
	HTML        viewHTML                         `json:"html"`
}

type trackerFeedEntry struct {
	Key         string   `json:"key"`
	Kind        string   `json:"kind"`
	Item        string   `json:"item"`
	TS          string   `json:"ts"`
	Event       *string  `json:"ev"`
	By          string   `json:"by,omitempty"`
	Author      string   `json:"author,omitempty"`
	To          string   `json:"to,omitempty"`
	From        string   `json:"from,omitempty"`
	EndedBy     string   `json:"endedBy,omitempty"`
	StatusBy    string   `json:"statusBy,omitempty"`
	Status      string   `json:"status,omitempty"`
	Type        string   `json:"type,omitempty"`
	NoticeKind  string   `json:"noticeKind,omitempty"`
	Snippet     string   `json:"snippet,omitempty"`
	Handoff     bool     `json:"handoff,omitempty"`
	Inherited   bool     `json:"inherited,omitempty"`
	RuleLevels  *float64 `json:"ruleLevels,omitempty"`
	Due         string   `json:"due,omitempty"`
	Overdue     bool     `json:"overdue,omitempty"`
	BodyMention bool     `json:"bodyMention,omitempty"`
}

// projectTracker has explicit inputs and performs no host reads or markdown
// rendering. v and its folded contents must remain immutable, just as the host's
// published StoreView does. Hashing streams raw detail inputs into a digest;
// no detail response is materialized/encoded until Detail requests that id.
func projectTracker(id string, v *wstrackerhost.StoreView, seen wsdashboard.TrackerSeen) (wsdashboard.TrackerProjection, error) {
	out := wsdashboard.TrackerProjection{
		Items: map[string]wsdashboard.TrackerSummaryRow{}, Globals: map[string]json.RawMessage{},
		Keys: map[string]wsdashboard.TrackerSeen{},
		Seen: wsdashboard.TrackerSeen{Badge: append([]string{}, seen.Badge...), Read: append([]string{}, seen.Read...)},
	}
	if v == nil || v.View == nil {
		return out, fmt.Errorf("tracker: missing projection view")
	}
	// Redact precisely the fields View redacts, without mutating the input.
	cp := *v
	cp.Sync.WikiWarnings = append([]string(nil), v.Sync.WikiWarnings...)
	cp.Redact()
	v = &cp
	globals := map[string]any{
		"store": v.Store, "sync": v.Sync, "clones": v.Clones,
		"viewer": v.Viewer, "priority": v.Priority, "attachments": v.Attachments,
		"labels": v.Labels, "members": v.Members, "admins": v.Admins,
		"rolesProblems": v.RolesProblems, "itemOrder": v.ItemOrder,
		"my": v.My, "stats": v.Stats, "heads": v.Heads, "now": v.Now,
	}
	if len(v.Now) > 13 {
		// Keep dashboardViewTag's hourly identity, but retain a parseable
		// timestamp for the page's Date.parse(now) closed-window arithmetic.
		globals["now"] = v.Now[:13] + ":00:00Z"
	}
	// Removed labels still have names in historical thread lines. Carry only
	// the derived names, not the global roles-event payloads.
	labelNames := map[string]string{}
	for _, e := range v.Events {
		if e.Status != "applied" {
			continue
		}
		var p struct{ Label, Name string }
		if e.Type == "roles.add_label" || e.Type == "roles.edit_label" {
			if json.Unmarshal(e.Payload, &p) == nil {
				key := e.ID
				if e.Type == "roles.edit_label" {
					key = p.Label
				}
				labelNames[key] = p.Name
			}
		}
	}
	globals["labelNames"] = labelNames
	for k, value := range globals {
		raw, err := json.Marshal(value)
		if err != nil {
			return out, err
		}
		out.Globals[k] = raw
	}
	// Shared naming/permissions/formatting inputs used by the item window.
	// Do not hash stats, itemOrder or Now: unrelated counts and reload clocks
	// must not invalidate every item's detail. Event local/open flags are
	// restricted to this item's thread below rather than the whole sync list.
	contextHash, err := trackerProjectionHash(struct {
		Store       string
		Viewer      wstracker.Viewer
		Members     map[string]wstracker.MemberView
		Priority    wstracker.PriorityLevels
		Labels      []wstracker.LabelView
		Attachments wstracker.AttachmentLimit
	}{id, v.Viewer, v.Members, v.Priority, v.Labels, v.Attachments})
	if err != nil {
		return out, err
	}
	rows := map[string]trackerItemProjection{}
	inputs := map[string]trackerDetailInput{}
	for iid, it := range v.Items {
		rows[iid] = trackerItemProjection{Item: trackerSummaryItem(it),
			Assignments: map[string]wstracker.Assignment{}, Requests: map[string]wstracker.RequestView{},
			Rules: map[string]wstracker.RuleView{}, Notices: []wstracker.Notice{}, CCNotices: []wstracker.CCNotice{}}
		inputs[iid] = trackerDetailInput{Item: it, Comments: map[string]wstracker.CommentView{}, Events: []wstracker.EventView{}, ContextHash: contextHash}
	}
	for key, a := range v.Assignments {
		if row, ok := rows[a.Item]; ok {
			row.Assignments[key] = a
		}
	}
	for key, q := range v.Requests {
		if row, ok := rows[q.Item]; ok {
			row.Requests[key] = q
		}
	}
	for key, r := range v.Rules {
		if row, ok := rows[r.Item]; ok {
			row.Rules[key] = r
		}
	}
	for _, n := range v.Notices {
		if row, ok := rows[n.Item]; ok {
			row.Notices = append(row.Notices, n)
			rows[n.Item] = row
		}
	}
	for _, n := range v.My.CCNotices {
		if row, ok := rows[n.Item]; ok {
			row.CCNotices = append(row.CCNotices, n)
			rows[n.Item] = row
		}
	}
	for key, c := range v.Comments {
		if in, ok := inputs[c.Item]; ok {
			in.Comments[key] = c
		}
	}
	events := map[string]wstracker.EventView{}
	for _, e := range v.Events {
		events[e.ID] = e
	}
	// sync.unpushed can affect a thread's local/open decoration. Include only
	// this thread's membership, not unrelated pending events, in its hash.
	local, open := map[string]bool{}, map[string]bool{}
	for _, commit := range v.Sync.Unpushed {
		for _, eid := range commit.Events {
			local[eid] = true
			open[eid] = commit.Open
		}
	}
	for iid, in := range inputs {
		// The item window also renders incoming comment references and reply
		// parents. Include them in this bundle AND its version, so an edit to
		// another thread cannot leave this window's snippet stale.
		for _, cid := range in.Item.Backrefs {
			if c, ok := v.Comments[cid]; ok {
				in.Comments[cid] = c
			}
		}
		parents := []string{}
		for _, c := range in.Comments {
			parents = append(parents, trackerString(c.ReplyTo))
		}
		for _, cid := range parents {
			if parent, ok := v.Comments[cid]; ok {
				in.Comments[parent.ID] = parent
			}
		}
		flags := map[string][2]bool{}
		for _, eid := range in.Item.Thread {
			if e, ok := events[eid]; ok {
				in.Events = append(in.Events, e)
				if e.Status == "flagged" && trackerString(e.Thread) == iid {
					row := rows[iid]
					row.Flagged++
					rows[iid] = row
				}
				flags[eid] = [2]bool{local[eid], open[eid]}
			}
		}
		in.Summary = rows[iid]
		in.Summary.Time = trackerProjectionTime(in.Item, in.Summary.Assignments, v.Now)
		version, err := trackerProjectionHash(struct {
			Input trackerDetailInput
			Flags map[string][2]bool
		}{in, flags})
		if err != nil {
			return out, err
		}
		raw, err := json.Marshal(in.Summary)
		if err != nil {
			return out, err
		}
		out.Items[iid] = wsdashboard.TrackerSummaryRow{Data: raw, DetailVersion: version}
		inputs[iid] = in
	}
	feed := trackerProjectionFeed(v, events)
	// Read candidates cover ALL notifications, not just the retained feed.
	for _, e := range feed {
		keys := out.Keys[e.Item]
		keys.Read = append(keys.Read, e.Key)
		out.Keys[e.Item] = keys
	}
	badgeItems := map[string]string{}
	for _, a := range v.Assignments {
		badgeItems["a:"+a.ID], badgeItems["p:"+a.ID] = a.Item, a.Item
	}
	for _, q := range v.Requests {
		badgeItems["q:"+q.ID] = q.Item
	}
	for _, n := range v.Notices {
		badgeItems[n.Key] = n.Item
	}
	for _, key := range v.My.Badge {
		if iid, ok := badgeItems[key]; ok {
			keys := out.Keys[iid]
			keys.Badge = append(keys.Badge, key)
			out.Keys[iid] = keys
		}
	}
	for iid, keys := range out.Keys {
		sort.Strings(keys.Badge)
		sort.Strings(keys.Read)
		out.Keys[iid] = keys
	}
	if len(feed) > 200 {
		feed = feed[:200]
	}
	out.Feed, err = json.Marshal(feed)
	if err != nil {
		return out, err
	}
	out.Detail = func(ctx context.Context, ids []string) (map[string]json.RawMessage, error) {
		bundles := map[string]json.RawMessage{}
		for _, iid := range ids {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			in, ok := inputs[iid]
			if !ok {
				continue
			}
			if _, duplicate := bundles[iid]; duplicate {
				continue
			}
			// renderViewHTML already enforces hidden-comment and own-attachment
			// policy. Give it only this item's texts from the captured view.
			sub := &wstrackerhost.StoreView{View: &wstracker.View{Items: map[string]wstracker.ItemView{iid: in.Item}, Comments: in.Comments}}
			data := trackerDetailData{Body: in.Item.Body, BodyRev: in.Item.BodyRev, Attachments: in.Item.Attachments,
				Comments: in.Comments, Events: in.Events, HTML: renderViewHTML(id, sub)}
			raw, err := json.Marshal(data)
			if err != nil {
				return nil, err
			}
			bundles[iid] = raw
		}
		return bundles, nil
	}
	return out, nil
}

// Reflect fields, not encoded ItemView JSON: excluded payloads never enter the
// summary encoder. New ItemView list fields are retained automatically.
func trackerSummaryItem(it wstracker.ItemView) map[string]any {
	out := map[string]any{}
	value, typ := reflect.ValueOf(it), reflect.TypeOf(it)
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" && name != "body" && name != "bodyRev" && name != "attachments" {
			out[name] = value.Field(i).Interface()
		}
	}
	return out
}

// Hash raw typed values without JSON-encoding detail payloads on a summary
// read. Length prefixes and kind tags preserve boundaries; sorted string map
// keys remove Go map iteration order. Only exported fields are rendered inputs
// (Assignment also has an unexported time.Time bookkeeping field).
func trackerProjectionHash(value any) (string, error) {
	h := sha256.New()
	if err := trackerHashValue(h, reflect.ValueOf(value)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func trackerHashNumber(h hash.Hash, n uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], n)
	_, _ = h.Write(b[:])
}

func trackerHashValue(h hash.Hash, v reflect.Value) error {
	if !v.IsValid() {
		_, _ = h.Write([]byte{0})
		return nil
	}
	_, _ = h.Write([]byte{byte(v.Kind()) + 1})
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			trackerHashNumber(h, 0)
			return nil
		}
		trackerHashNumber(h, 1)
		return trackerHashValue(h, v.Elem())
	case reflect.String:
		s := v.String()
		trackerHashNumber(h, uint64(len(s)))
		_, _ = h.Write([]byte(s))
	case reflect.Bool:
		if v.Bool() {
			trackerHashNumber(h, 1)
		} else {
			trackerHashNumber(h, 0)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		trackerHashNumber(h, uint64(v.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		trackerHashNumber(h, v.Uint())
	case reflect.Float32, reflect.Float64:
		trackerHashNumber(h, math.Float64bits(v.Float()))
	case reflect.Slice, reflect.Array:
		trackerHashNumber(h, uint64(v.Len()))
		if v.Kind() == reflect.Slice {
			if v.IsNil() {
				trackerHashNumber(h, 0)
			} else {
				trackerHashNumber(h, 1)
			}
			if v.Type().Elem().Kind() == reflect.Uint8 {
				_, _ = h.Write(v.Bytes())
				return nil
			}
		}
		for i := 0; i < v.Len(); i++ {
			if err := trackerHashValue(h, v.Index(i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("tracker: unsupported projection hash map %s", v.Type())
		}
		if v.IsNil() {
			trackerHashNumber(h, 0)
		} else {
			trackerHashNumber(h, 1)
		}
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		trackerHashNumber(h, uint64(len(keys)))
		for _, key := range keys {
			if err := trackerHashValue(h, key); err != nil {
				return err
			}
			if err := trackerHashValue(h, v.MapIndex(key)); err != nil {
				return err
			}
		}
	case reflect.Struct:
		trackerHashNumber(h, uint64(v.NumField()))
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if field.PkgPath != "" {
				continue
			}
			if err := trackerHashValue(h, reflect.ValueOf(field.Name)); err != nil {
				return err
			}
			if err := trackerHashValue(h, v.Field(i)); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("tracker: unsupported projection hash input %s", v.Type())
	}
	return nil
}

func trackerFeedSnippet(s string) string {
	r := make([]rune, 0, 80)
	for _, ch := range s {
		if len(r) == 80 {
			return string(r[:77]) + "..."
		}
		r = append(r, ch)
	}
	return string(r)
}

func trackerProjectionTime(it wstracker.ItemView, assignments map[string]wstracker.Assignment, now string) trackerItemTime {
	out := trackerItemTime{RecentAssignments: map[string]bool{}}
	day := now
	if len(day) > 10 {
		day = day[:10]
	}
	if due := trackerString(it.Due); due != "" {
		out.DueState = "future"
		if due < day {
			out.DueState = "over"
		} else if due == day {
			out.DueState = "today"
		}
	}
	clock, err := time.Parse(time.RFC3339Nano, now)
	if err != nil {
		return out
	}
	recent := func(ts string) bool {
		at, err := time.Parse(time.RFC3339Nano, ts)
		return err == nil && clock.Sub(at) <= 14*24*time.Hour
	}
	closedAt := ""
	for aid, a := range assignments {
		if a.Status != "done" && a.Status != "dropped" {
			continue
		}
		out.RecentAssignments[aid] = recent(a.LastUpdate)
		if a.State == "live" && a.LastUpdate > closedAt {
			closedAt = a.LastUpdate
		}
	}
	if status := trackerString(it.Status); status == "done" || status == "dropped" {
		if closedAt == "" {
			closedAt = it.LastActivity
		}
		out.RecentlyClosed = recent(closedAt)
	}
	return out
}

func trackerString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// trackerProjectionFeed mirrors feedEntries in tracker-panel.js. It deliberately
// keeps the full candidate list until Keys are extracted; only the wire feed is
// capped. No payload, attachment metadata, or HTML is carried in these entries.
func trackerProjectionFeed(v *wstrackerhost.StoreView, events map[string]wstracker.EventView) []trackerFeedEntry {
	out := []trackerFeedEntry{}
	viewer, today := v.Viewer.Member, v.Now
	if len(today) > 10 {
		today = today[:10]
	}
	wrapped := func(s string) bool { return s == "done" || s == "dropped" }
	add := func(e trackerFeedEntry, eid string) {
		if it, ok := v.Items[e.Item]; ok && !it.Hidden {
			if eid != "" {
				e.Event = &eid
				if c, ok := v.Comments[eid]; ok {
					e.Author = c.Author
				} else {
					e.Author = events[eid].Member
				}
			}
			e.Snippet = trackerFeedSnippet(e.Snippet)
			out = append(out, e)
		}
	}
	// One event scan, not one scan per assignment. Prefer the latest event
	// at lastUpdate over the last matching applied status event, as statusEv.
	statusLast, statusAt := map[string]string{}, map[string]string{}
	byHolder := map[string][]string{}
	for aid, a := range v.Assignments {
		byHolder[a.Item+"\x00"+a.To] = append(byHolder[a.Item+"\x00"+a.To], aid)
	}
	for _, e := range v.Events {
		switch e.Type {
		case "item.status", "item.close", "item.confirm", "item.reject", "item.reopen":
		default:
			continue
		}
		if e.Status != "applied" {
			continue
		}
		var p struct{ Item, Assignee string }
		if json.Unmarshal(e.Payload, &p) != nil {
			continue
		}
		to := p.Assignee
		if e.Type == "item.close" || to == "" {
			to = e.Member
		}
		for _, aid := range byHolder[p.Item+"\x00"+to] {
			statusLast[aid] = e.ID
			if e.TS == v.Assignments[aid].LastUpdate {
				statusAt[aid] = e.ID
			}
		}
	}
	statusEvent := func(a wstracker.Assignment) string {
		if e := statusAt[a.ID]; e != "" {
			return e
		}
		return statusLast[a.ID]
	}
	for _, aid := range v.My.Assignments {
		a, ok := v.Assignments[aid]
		if !ok {
			continue
		}
		it, ok := v.Items[a.Item]
		if !ok || it.Kind == "topic" {
			continue
		}
		if a.By != viewer && a.Forced && a.Status == "todo" {
			add(trackerFeedEntry{Key: "a:" + a.ID, Kind: "assigned", Item: a.Item, TS: a.TS, By: a.By, Inherited: a.Inherited, RuleLevels: a.RuleLevels}, a.Via)
		}
		if due := trackerString(it.Due); due != "" && due <= today && !wrapped(a.Status) && !wrapped(trackerString(it.Status)) {
			state := "today"
			if due < today {
				state = "over"
			}
			add(trackerFeedEntry{Key: "d:" + a.ID + ":" + state, Kind: "due", Item: a.Item, TS: due + "T00:00:00Z", Due: due, Overdue: due < today}, "")
		}
	}
	for _, aid := range v.My.Received.Pending {
		if a, ok := v.Assignments[aid]; ok {
			add(trackerFeedEntry{Key: "p:" + a.ID, Kind: "pending", Item: a.Item, TS: a.TS, By: a.Sender, Handoff: a.HandoffOf != nil}, a.Via)
		}
	}
	for _, qid := range v.My.Requests.Incoming {
		if q, ok := v.Requests[qid]; ok {
			add(trackerFeedEntry{Key: "q:" + qid, Kind: "request", Item: q.Item, TS: q.TS, By: q.By, To: q.Assignee, Snippet: trackerString(q.Note)}, qid)
		}
	}
	for _, n := range v.Notices {
		a := v.Assignments[n.Assignment]
		e := trackerFeedEntry{Key: n.Key, Kind: n.Kind, NoticeKind: n.Kind, Item: n.Item, TS: n.TS, By: n.By, To: n.To,
			From: trackerString(n.From), EndedBy: n.EndedBy, StatusBy: trackerString(a.StatusBy), Status: a.Status, Handoff: a.HandoffOf != nil}
		eid := ""
		switch n.Kind {
		case "done", "blocked", "dropped":
			e.Snippet = trackerString(a.Note)
			eid = statusEvent(a)
		case "declined", "returned":
			e.Kind = "answer"
			e.Snippet = trackerString(n.Note)
			if a.Ended != nil {
				eid = a.Ended.ID
			}
		case "accepted", "handoff":
			e.Kind = "reference"
			e.Snippet = trackerString(n.Note)
			if a.Accepted != nil {
				eid = a.Accepted.ID
			}
		default:
			continue
		}
		add(e, eid)
	}
	for _, aid := range v.My.Today.Confirms {
		if a, ok := v.Assignments[aid]; ok {
			add(trackerFeedEntry{Key: "c:" + a.ID + ":" + a.LastUpdate, Kind: "confirm", Item: a.Item, TS: a.LastUpdate, To: a.To}, statusEvent(a))
		}
	}
	for _, c := range v.Comments {
		if c.Hidden != nil || c.Author == viewer {
			continue
		}
		if q, ok := v.Comments[trackerString(c.ReplyTo)]; ok && q.Author == viewer {
			add(trackerFeedEntry{Key: "r:" + c.ID, Kind: "reply", Item: c.Item, TS: c.TS, By: c.Author, Snippet: c.Body}, c.ID)
		}
		ts := c.TS
		if c.Edited != nil {
			ts = c.Edited.TS
		}
		for _, member := range c.Mentions {
			if member == viewer {
				add(trackerFeedEntry{Key: "m:" + c.ID + ":" + ts, Kind: "mention", Item: c.Item, TS: ts, By: c.Author, Snippet: c.Body}, c.ID)
				break
			}
		}
	}
	for _, it := range v.Items {
		if it.Author == viewer {
			continue
		}
		ts, eid := it.TS, it.ID
		if it.Edited != nil {
			ts, eid = it.Edited.TS, it.Edited.ID
		}
		for _, member := range it.Mentions {
			if member == viewer {
				add(trackerFeedEntry{Key: "m:" + it.ID + ":" + ts, Kind: "mention", Item: it.ID, TS: ts, By: it.Author, BodyMention: true}, eid)
				break
			}
		}
	}
	for _, n := range v.My.CCNotices {
		e := trackerFeedEntry{Key: n.Key, Kind: "cc", Type: n.Type, Item: n.Item, TS: n.TS, By: n.By, To: trackerString(n.To), Status: trackerString(n.Status)}
		var p struct{ Body, Note string }
		if event, ok := events[n.Event]; ok {
			_ = json.Unmarshal(event.Payload, &p)
		}
		switch n.Type {
		case "comment.add":
			e.Snippet = p.Body
			if c, ok := v.Comments[n.Event]; ok {
				e.Snippet = c.Body
			}
		case "item.status", "item.close", "item.confirm", "item.reject", "item.reopen":
			e.Snippet = p.Note
		default:
			continue
		}
		add(e, n.Event)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TS != out[j].TS {
			return out[i].TS > out[j].TS
		}
		return out[i].Key < out[j].Key
	})
	return out
}
