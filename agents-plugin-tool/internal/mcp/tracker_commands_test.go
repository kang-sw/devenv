package mcp

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

// trackerWriteStep is one tracker.update call of the command walk: the
// commands, and per written event (in command order) its type and the
// payload it must carry. A payload names an event of the walk by the
// placeholder "{name}", which the walk resolves to its stored id.
type trackerWriteStep struct {
	commands string
	events   []trackerWantEvent
}

type trackerWantEvent struct {
	typ     string
	name    string // names this event's id for later steps; "" for none
	payload map[string]any
}

// trackerWalk drives the session fixture's team store: ada writes through
// tracker.update, bo through its own host, and each side syncs before it
// writes, so every write is valid in its author's seen state.
type trackerWalk struct {
	*trackerSessionFixture
	ids map[string]string // name -> stored event id
}

func (w *trackerWalk) adaSync(t *testing.T) {
	t.Helper()
	w.trackerUpdate(t, `<tk:sync store="team"/>`)
}

func (w *trackerWalk) boSync(t *testing.T) {
	t.Helper()
	if _, err := w.bo.Sync(context.Background(), "team"); err != nil {
		t.Fatalf("bo syncs: %v", err)
	}
}

// byBo writes one event as bo after fetching ada's, publishes it, and ada
// fetches it; name records its id.
func (w *trackerWalk) byBo(t *testing.T, name, typ string, payload map[string]any) {
	t.Helper()
	w.adaSync(t)
	w.boSync(t)
	raw, err := json.Marshal(w.resolve(payload))
	if err != nil {
		t.Fatal(err)
	}
	ids, err := w.bo.Dispatch(context.Background(), "team", []wsledger.NewEvent{{Type: typ, Payload: raw}})
	if err != nil {
		t.Fatalf("bo writes %s: %v", typ, err)
	}
	if name != "" {
		w.ids[name] = ids[0].String()
	}
	w.boSync(t)
	w.adaSync(t)
}

// resolve replaces every "{name}" string in v with the named stored id.
func (w *trackerWalk) resolve(v any) any {
	switch x := v.(type) {
	case string:
		if strings.HasPrefix(x, "{") && strings.HasSuffix(x, "}") {
			if id, ok := w.ids[x[1:len(x)-1]]; ok {
				return id
			}
		}
		return x
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = w.resolve(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = w.resolve(e)
		}
		return out
	}
	return v
}

// expand replaces every {name} in a command string with the named id.
func (w *trackerWalk) expand(commands string) string {
	for name, id := range w.ids {
		commands = strings.ReplaceAll(commands, "{"+name+"}", id)
	}
	return commands
}

// ada runs one step through tracker.update and checks every written event's
// type, verdict, and exact payload as the store holds it.
func (w *trackerWalk) ada(t *testing.T, step trackerWriteStep) {
	t.Helper()
	out := w.trackerUpdate(t, w.expand(step.commands))
	wrote := trackerWrote(t, out)
	positions := make([]int, 0, len(wrote))
	for pos := range wrote {
		n, err := strconv.Atoi(pos)
		if err != nil {
			t.Fatal(err)
		}
		positions = append(positions, n)
	}
	sort.Ints(positions)
	if len(positions) != len(step.events) {
		t.Fatalf("wrote %d events, the step expects %d:\n%s", len(positions), len(step.events), out)
	}
	v := w.view(t)
	byID := map[string]wstracker.EventView{}
	for _, ev := range v.Events {
		byID[ev.ID] = ev
	}
	for i, pos := range positions {
		want := step.events[i]
		id, err := parseTrackerID(wrote[strconv.Itoa(pos)])
		if err != nil {
			t.Fatal(err)
		}
		if want.name != "" {
			w.ids[want.name] = id
		}
		ev, ok := byID[id]
		if !ok {
			t.Fatalf("event %s (%s) is not in the store", id, want.typ)
		}
		if ev.Type != want.typ || ev.Status != "applied" {
			t.Fatalf("event %s = %s %s (reason %v), want %s applied", id, ev.Type, ev.Status, ev.Reason, want.typ)
		}
		var got map[string]any
		if err := json.Unmarshal(ev.Payload, &got); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(w.resolve(want.payload))
		var exp map[string]any
		_ = json.Unmarshal(raw, &exp)
		if !reflect.DeepEqual(got, exp) {
			t.Fatalf("%s payload:\n got %s\nwant %s", want.typ, ev.Payload, raw)
		}
	}
}

// recStates lists the state and status of every assignment of item to
// member.
func recStates(v map[string]wstracker.Assignment, item, member string) []string {
	var out []string
	for _, a := range v {
		if a.Item == item && a.To == member {
			out = append(out, a.State+"/"+a.Status)
		}
	}
	sort.Strings(out)
	return out
}

// trackerSpareKey is a well-formed ssh-ed25519 public key line no member
// holds.
func trackerSpareKey() string {
	const typ = "ssh-ed25519"
	blob := binary.BigEndian.AppendUint32(nil, uint32(len(typ)))
	blob = append(blob, typ...)
	blob = binary.BigEndian.AppendUint32(blob, 32)
	for i := 0; i < 32; i++ {
		blob = append(blob, byte(i+1))
	}
	return typ + " " + base64.StdEncoding.EncodeToString(blob)
}

// TestTrackerUpdateWritesEveryEventCommand writes every tk: event command
// through tracker.update against a two-member team store, in a lifecycle
// that makes each one valid, and checks each event's exact payload (field
// names, null against absent, list splitting, numbers) and the folded view
// it produces.
func TestTrackerUpdateWritesEveryEventCommand(t *testing.T) {
	w := &trackerWalk{trackerSessionFixture: newTrackerSessionFixture(t), ids: map[string]string{}}
	w.adaSync(t) // ada learns bo's join
	covered := map[string]bool{}
	step := func(s trackerWriteStep) {
		t.Helper()
		w.ada(t, s)
		for _, e := range s.events {
			covered[e.typ] = true
		}
	}

	// Store settings and profiles.
	step(trackerWriteStep{`<tk:store name="team">
<tk:roles.set_priority_levels names="low mid high top" default="1"/>
<tk:roles.set_attachment_limit max_bytes="2048"/>
<tk:member.profile org="Acme" family="Kim" given="Ada"/>
<tk:member.profile member="bo" org="" given="Bobby"/>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeSetPriorityLevels, payload: map[string]any{"names": []any{"low", "mid", "high", "top"}, "default": 1}},
		{typ: wstracker.TypeSetAttachmentLimit, payload: map[string]any{"max_bytes": 2048}},
		{typ: wstracker.TypeMemberProfile, payload: map[string]any{"profile": map[string]any{"org": "Acme", "family": "Kim", "given": "Ada"}}},
		{typ: wstracker.TypeMemberProfile, payload: map[string]any{"profile": map[string]any{"org": "", "given": "Bobby"}, "member": "bo"}},
	}})
	v := w.view(t)
	if !reflect.DeepEqual(v.Priority, wstracker.PriorityLevels{Names: []string{"low", "mid", "high", "top"}, Default: 1}) || v.Attachments.MaxBytes != 2048 {
		t.Fatalf("settings: priority %+v, attachment limit %v", v.Priority, v.Attachments.MaxBytes)
	}
	if v.Members["ada"].Profile != (wstracker.Profile{Org: "Acme", Family: "Kim", Given: "Ada"}) || v.Members["bo"].Profile.Given != "Bobby" {
		t.Fatalf("profiles: ada %+v, bo %+v", v.Members["ada"].Profile, v.Members["bo"].Profile)
	}

	// The label list, and labels on items: $N names a label added earlier in
	// the batch, a colour is lower-cased, and labels="" clears.
	step(trackerWriteStep{`<tk:store name="team">
<tk:roles.add_label name="bug" color="#D14343"/>
<tk:roles.add_label name="ui" color="#3366cc"/>
<tk:roles.edit_label label="$2" name="design"/>
<tk:item.create kind="task" title="Labelled" labels="$1 $2"/>
<tk:item.edit item="$4" labels="$2"/>
<tk:roles.remove_label label="$1"/>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeAddLabel, name: "bug", payload: map[string]any{"name": "bug", "color": "#d14343"}},
		{typ: wstracker.TypeAddLabel, name: "ui", payload: map[string]any{"name": "ui", "color": "#3366cc"}},
		{typ: wstracker.TypeEditLabel, payload: map[string]any{"label": "{ui}", "name": "design"}},
		{typ: wstracker.TypeItemCreate, name: "labelled", payload: map[string]any{"kind": "task", "title": "Labelled", "labels": []any{"{bug}", "{ui}"}}},
		{typ: wstracker.TypeItemEdit, payload: map[string]any{"item": "{labelled}", "labels": []any{"{ui}"}}},
		{typ: wstracker.TypeRemoveLabel, payload: map[string]any{"label": "{bug}"}},
	}})
	v = w.view(t)
	if want := []wstracker.LabelView{{ID: w.ids["ui"], Name: "design", Color: "#3366cc"}}; !reflect.DeepEqual(v.Labels, want) {
		t.Fatalf("label list %+v, want %+v", v.Labels, want)
	}
	if got := v.Items[w.ids["labelled"]].Labels; !reflect.DeepEqual(got, []string{w.ids["ui"]}) {
		t.Fatalf("item labels %v", got)
	}
	step(trackerWriteStep{`<tk:store name="team">
<tk:item.edit item="` + w.ids["labelled"] + `" labels=""/>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemEdit, payload: map[string]any{"item": "{labelled}", "labels": []any{}}},
	}})

	// Create, edit with each clear, and a pending assignment the assigner
	// confirms (bo is not below ada yet).
	step(trackerWriteStep{`<tk:store name="team">
<tk:item.create kind="task" title="T1" project="p" due="2026-12-01" priority="3" mentions="bo">body one</tk:item.create>
<tk:item.create kind="todo" title="child" parent="$1"/>
<tk:item.edit item="$1" clear="due"/>
<tk:item.edit item="$1" clear="body"/>
<tk:item.edit item="$1" title="T1 renamed" priority="0"/>
<tk:item.assign item="$1" people="bo" completion="assigner"/>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemCreate, name: "t1", payload: map[string]any{"kind": "task", "title": "T1", "project": "p", "due": "2026-12-01", "priority": 3, "mentions": []any{"bo"}, "body": "body one"}},
		{typ: wstracker.TypeItemCreate, name: "child", payload: map[string]any{"kind": "todo", "title": "child", "parent": "{t1}"}},
		{typ: wstracker.TypeItemEdit, payload: map[string]any{"item": "{t1}", "due": nil}},
		{typ: wstracker.TypeItemEdit, payload: map[string]any{"item": "{t1}", "body": ""}},
		{typ: wstracker.TypeItemEdit, payload: map[string]any{"item": "{t1}", "title": "T1 renamed", "priority": 0}},
		{typ: wstracker.TypeItemAssign, payload: map[string]any{"item": "{t1}", "mode": "chosen", "people": []any{"bo"}, "completion": "assigner"}},
	}})
	v = w.view(t)
	t1 := v.Items[w.ids["t1"]]
	if t1.Due != nil || t1.Body != "" || t1.Title != "T1 renamed" || t1.Priority != 0 || t1.Project != "p" {
		t.Fatalf("T1 after the edits: %+v", t1)
	}
	if child := v.Items[w.ids["child"]]; child.Parent == nil || *child.Parent != w.ids["t1"] {
		t.Fatalf("child's parent: %+v", child.Parent)
	}
	if got := recStates(v.Assignments, w.ids["t1"], "bo"); !reflect.DeepEqual(got, []string{"pending/todo"}) {
		t.Fatalf("bo's T1 assignment after assign: %v", got)
	}

	w.byBo(t, "", wstracker.TypeItemAccept, map[string]any{"item": "{t1}"})
	step(trackerWriteStep{`<tk:store name="team"><tk:roles.set_lead member="bo" leads="ada"/></tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeSetLead, payload: map[string]any{"member": "bo", "leads": []any{"ada"}}},
	}})
	if leads := w.view(t).Members["bo"].Leads; !reflect.DeepEqual(leads, []string{"ada"}) {
		t.Fatalf("bo's leads: %v", leads)
	}

	w.byBo(t, "", wstracker.TypeItemClose, map[string]any{"item": "{t1}"})
	step(trackerWriteStep{`<tk:store name="team"><tk:item.reject item="{t1}" assignee="bo">not yet</tk:item.reject></tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemReject, payload: map[string]any{"item": "{t1}", "assignee": "bo", "note": "not yet"}},
	}})
	if got := recStates(w.view(t).Assignments, w.ids["t1"], "bo"); len(got) != 1 || strings.HasSuffix(got[0], "/in-review") {
		t.Fatalf("a rejected close must leave the assignment open again: %v", got)
	}
	w.byBo(t, "", wstracker.TypeItemClose, map[string]any{"item": "{t1}"})
	step(trackerWriteStep{`<tk:store name="team"><tk:item.confirm item="{t1}" assignee="bo">good</tk:item.confirm></tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemConfirm, payload: map[string]any{"item": "{t1}", "assignee": "bo", "note": "good"}},
	}})
	if got := recStates(w.view(t).Assignments, w.ids["t1"], "bo"); !reflect.DeepEqual(got, []string{"live/done"}) {
		t.Fatalf("a confirmed close is done: %v", got)
	}
	// ada is now bo's direct lead and bo accepted, so ada may return it.
	step(trackerWriteStep{`<tk:store name="team">
<tk:item.reopen item="{t1}" assignee="bo">again</tk:item.reopen>
<tk:item.return item="{t1}" assignee="bo">back to you</tk:item.return>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemReopen, payload: map[string]any{"item": "{t1}", "assignee": "bo", "note": "again"}},
		{typ: wstracker.TypeItemReturn, payload: map[string]any{"item": "{t1}", "assignee": "bo", "note": "back to you"}},
	}})
	if got := recStates(w.view(t).Assignments, w.ids["t1"], "bo"); len(got) != 1 || !strings.HasPrefix(got[0], "returned/") {
		t.Fatalf("a returned assignment: %v", got)
	}

	// A forced assignment an admin hands off; the target accepts, sets a
	// status, and closes.
	step(trackerWriteStep{`<tk:store name="team">
<tk:item.create kind="task" title="T2"/>
<tk:item.assign item="$1" people="bo" completion="assignee"/>
<tk:item.handoff item="$1" to="ada" assignee="bo">take it</tk:item.handoff>
<tk:item.accept item="$1">mine now</tk:item.accept>
<tk:item.status item="$1" status="blocked">waiting</tk:item.status>
<tk:item.close item="$1">done</tk:item.close>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemCreate, name: "t2", payload: map[string]any{"kind": "task", "title": "T2"}},
		{typ: wstracker.TypeItemAssign, payload: map[string]any{"item": "{t2}", "mode": "chosen", "people": []any{"bo"}, "completion": "assignee"}},
		{typ: wstracker.TypeItemHandoff, payload: map[string]any{"item": "{t2}", "to": "ada", "assignee": "bo", "note": "take it"}},
		{typ: wstracker.TypeItemAccept, payload: map[string]any{"item": "{t2}", "note": "mine now"}},
		{typ: wstracker.TypeItemStatus, payload: map[string]any{"item": "{t2}", "status": "blocked", "note": "waiting"}},
		{typ: wstracker.TypeItemClose, payload: map[string]any{"item": "{t2}", "note": "done"}},
	}})
	v = w.view(t)
	if bo, ada := recStates(v.Assignments, w.ids["t2"], "bo"), recStates(v.Assignments, w.ids["t2"], "ada"); !reflect.DeepEqual(bo, []string{"handed/todo"}) || len(ada) != 1 || !strings.HasSuffix(ada[0], "/done") {
		t.Fatalf("T2 after the handoff: bo %v, ada %v", bo, ada)
	}

	// bo, who is below ada, assigns ada: pending, and ada declines.
	w.byBo(t, "t3", wstracker.TypeItemCreate, map[string]any{"kind": "task", "title": "T3"})
	w.byBo(t, "", wstracker.TypeItemAssign, map[string]any{"item": "{t3}", "mode": "chosen", "people": []any{"ada"}})
	step(trackerWriteStep{`<tk:store name="team"><tk:item.decline item="{t3}">no time</tk:item.decline></tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemDecline, payload: map[string]any{"item": "{t3}", "note": "no time"}},
	}})
	if got := recStates(w.view(t).Assignments, w.ids["t3"], "ada"); len(got) != 1 || !strings.HasPrefix(got[0], "declined/") {
		t.Fatalf("a declined assignment: %v", got)
	}

	// Assign by levels, withdraw it; assign one person, unassign them; hide.
	step(trackerWriteStep{`<tk:store name="team">
<tk:item.create kind="task" title="T4"/>
<tk:item.assign item="$1" levels="1" completion="assignee"/>
<tk:item.unassign item="$1"/>
<tk:item.create kind="task" title="T5"/>
<tk:item.assign item="$4" people="bo"/>
<tk:item.unassign item="$4" assignee="bo"/>
<tk:item.create kind="topic" title="T6"/>
<tk:item.hide item="$7"/>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemCreate, name: "t4", payload: map[string]any{"kind": "task", "title": "T4"}},
		{typ: wstracker.TypeItemAssign, payload: map[string]any{"item": "{t4}", "mode": "all", "levels": 1, "completion": "assignee"}},
		{typ: wstracker.TypeItemUnassign, payload: map[string]any{"item": "{t4}"}},
		{typ: wstracker.TypeItemCreate, name: "t5", payload: map[string]any{"kind": "task", "title": "T5"}},
		{typ: wstracker.TypeItemAssign, payload: map[string]any{"item": "{t5}", "mode": "chosen", "people": []any{"bo"}, "completion": "assigner"}},
		{typ: wstracker.TypeItemUnassign, payload: map[string]any{"item": "{t5}", "assignee": "bo"}},
		{typ: wstracker.TypeItemCreate, name: "t6", payload: map[string]any{"kind": "topic", "title": "T6"}},
		{typ: wstracker.TypeItemHide, payload: map[string]any{"item": "{t6}"}},
	}})
	v = w.view(t)
	for _, s := range recStates(v.Assignments, w.ids["t4"], "bo") {
		if strings.HasPrefix(s, "live/") || strings.HasPrefix(s, "pending/") {
			t.Fatalf("a withdrawn levels assignment must leave nothing open: %v", recStates(v.Assignments, w.ids["t4"], "bo"))
		}
	}
	if got := recStates(v.Assignments, w.ids["t5"], "bo"); len(got) != 1 || !strings.HasPrefix(got[0], "unassigned/") {
		t.Fatalf("an unassigned assignment: %v", got)
	}
	if !v.Items[w.ids["t6"]].Hidden {
		t.Fatalf("T6 must be hidden")
	}

	// Move: T5 under T4, first; then T4 to top level (parent="").
	step(trackerWriteStep{`<tk:store name="team">
<tk:item.move item="{t5}" parent="{t4}" after=""/>
<tk:item.move item="{t4}" parent=""/>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemMove, payload: map[string]any{"item": "{t5}", "parent": "{t4}", "after": ""}},
		{typ: wstracker.TypeItemMove, payload: map[string]any{"item": "{t4}", "parent": nil}},
	}})
	if v = w.view(t); v.Items[w.ids["t5"]].Parent == nil || *v.Items[w.ids["t5"]].Parent != w.ids["t4"] || v.Items[w.ids["t4"]].Parent != nil {
		t.Fatalf("after the moves: T5 parent %v, T4 parent %v", v.Items[w.ids["t5"]].Parent, v.Items[w.ids["t4"]].Parent)
	}

	// Comments: add with mentions, edit, hide.
	step(trackerWriteStep{`<tk:store name="team">
<tk:comment.add item="{t1}" mentions="bo">first</tk:comment.add>
<tk:comment.edit comment="$1" mentions="bo">first, edited</tk:comment.edit>
<tk:comment.hide comment="$1"/>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeCommentAdd, name: "c1", payload: map[string]any{"item": "{t1}", "mentions": []any{"bo"}, "body": "first"}},
		{typ: wstracker.TypeCommentEdit, payload: map[string]any{"comment": "{c1}", "mentions": []any{"bo"}, "body": "first, edited"}},
		{typ: wstracker.TypeCommentHide, payload: map[string]any{"comment": "{c1}"}},
	}})
	if c := w.view(t).Comments[w.ids["c1"]]; c.Body != "first, edited" || c.Hidden == nil || !reflect.DeepEqual(c.Mentions, []string{"bo"}) {
		t.Fatalf("the comment after edit and hide: %+v", c)
	}

	// Taking work. ada opens T7, withdraws, and reopens it to the store; the
	// fallback holder cannot claim it, so is:claimable leaves it out and a
	// claim is flagged.
	step(trackerWriteStep{`<tk:store name="team">
<tk:item.create kind="task" title="T7"/>
<tk:item.open item="$1" scope="team"/>
<tk:item.unopen item="$1"/>
<tk:item.open item="$1" scope="store"/>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemCreate, name: "t7", payload: map[string]any{"kind": "task", "title": "T7"}},
		{typ: wstracker.TypeItemOpen, payload: map[string]any{"item": "{t7}", "scope": "team"}},
		{typ: wstracker.TypeItemUnopen, payload: map[string]any{"item": "{t7}"}},
		{typ: wstracker.TypeItemOpen, name: "open7", payload: map[string]any{"item": "{t7}", "scope": "store"}},
	}})
	if it := w.view(t).Items[w.ids["t7"]]; it.Open == nil || it.Open.Scope != "store" || it.Claimable {
		t.Fatalf("T7 after the reopen: open %+v, claimable %v", it.Open, it.Claimable)
	}
	if got := w.trackerQuery(t, map[string]any{"store": "team", "item": w.ids["t7"]}); !strings.Contains(got, "open for claiming to store by ada ("+trackerAlias(w.ids["open7"])+")\n") {
		t.Fatalf("the item view prints the opening:\n%s", got)
	}
	if text := w.trackerRefused(t, "tracker.update", map[string]any{"commands": w.expand(`<tk:store name="team"><tk:item.claim item="{t7}"/></tk:store>`)}); !strings.Contains(text, "the store would flag this write: ada already holds") {
		t.Fatalf("the fallback holder's claim must be flagged:\n%s", text)
	}
	// bo opens T8 to the store: claimable for ada, who claims it.
	w.byBo(t, "t8", wstracker.TypeItemCreate, map[string]any{"kind": "task", "title": "T8"})
	w.byBo(t, "", wstracker.TypeItemOpen, map[string]any{"item": "{t8}", "scope": "store"})
	claimable := w.trackerQuery(t, map[string]any{"store": "team", "scope": "all", "q": "is:claimable"})
	if !strings.Contains(claimable, trackerAlias(w.ids["t8"])+" [task todo] P") || !strings.Contains(claimable, "open to store, claimable by me") || strings.Contains(claimable, trackerAlias(w.ids["t7"])+" [") {
		t.Fatalf("is:claimable lists T8 and not T7:\n%s", claimable)
	}
	step(trackerWriteStep{`<tk:store name="team"><tk:item.claim item="{t8}"/></tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemClaim, payload: map[string]any{"item": "{t8}"}},
	}})
	if got := recStates(w.view(t).Assignments, w.ids["t8"], "ada"); !reflect.DeepEqual(got, []string{"live/todo"}) {
		t.Fatalf("ada's claim of T8: %v", got)
	}
	// bo claims T7; ada files take requests for it: one withdrawn, one
	// denied ($N names a request of the batch), one approved by its ID.
	w.byBo(t, "", wstracker.TypeItemClaim, map[string]any{"item": "{t7}"})
	step(trackerWriteStep{`<tk:store name="team">
<tk:item.request item="{t7}" assignee="bo">let me</tk:item.request>
<tk:item.unrequest item="{t7}" request="$1"/>
<tk:item.request item="{t7}" assignee="bo"/>
<tk:item.deny item="{t7}" request="$3">not this</tk:item.deny>
<tk:item.request item="{t7}" assignee="bo">please</tk:item.request>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemRequest, name: "rq1", payload: map[string]any{"item": "{t7}", "assignee": "bo", "note": "let me"}},
		{typ: wstracker.TypeItemUnrequest, payload: map[string]any{"item": "{t7}", "request": "{rq1}"}},
		{typ: wstracker.TypeItemRequest, name: "rq2", payload: map[string]any{"item": "{t7}", "assignee": "bo"}},
		{typ: wstracker.TypeItemDeny, payload: map[string]any{"item": "{t7}", "request": "{rq2}", "note": "not this"}},
		{typ: wstracker.TypeItemRequest, name: "rq3", payload: map[string]any{"item": "{t7}", "assignee": "bo", "note": "please"}},
	}})
	if got := w.trackerQuery(t, map[string]any{"store": "team", "item": w.ids["t7"]}); !strings.Contains(got, "take requests:\n- "+trackerAlias(w.ids["rq1"])+" ada asks for bo's assignment, withdrawn") ||
		!strings.Contains(got, trackerAlias(w.ids["rq2"])+" ada asks for bo's assignment, denied, answer \"not this\"") ||
		!strings.Contains(got, trackerAlias(w.ids["rq3"])+" ada asks for bo's assignment, pending, note \"please\"") {
		t.Fatalf("the item view prints the take requests:\n%s", got)
	}
	if mine := w.trackerQuery(t, map[string]any{"store": "team"}); !strings.Contains(mine, "my take requests, waiting for an answer:\n- "+trackerAlias(w.ids["t7"])+" P") {
		t.Fatalf("mine lists ada's pending request:\n%s", mine)
	}
	step(trackerWriteStep{`<tk:store name="team"><tk:item.approve item="{t7}" request="` + trackerAlias(w.ids["rq3"]) + `">ok</tk:item.approve></tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemApprove, payload: map[string]any{"item": "{t7}", "request": "{rq3}", "note": "ok"}},
	}})
	v = w.view(t)
	if ada, bo := recStates(v.Assignments, w.ids["t7"], "ada"), recStates(v.Assignments, w.ids["t7"], "bo"); !reflect.DeepEqual(ada, []string{"live/todo"}) || !reflect.DeepEqual(bo, []string{"handed/todo"}) {
		t.Fatalf("T7 after the approval: ada %v, bo %v", ada, bo)
	}

	// CC: bo's T9 is none of ada's; ada CCs herself and bo, then removes bo.
	// bo's comment on his T10 mentions ada. The related scope lists T9 under
	// CC and T10 under mentions, and the item view names the CC list.
	w.byBo(t, "t9", wstracker.TypeItemCreate, map[string]any{"kind": "task", "title": "T9"})
	w.byBo(t, "t10", wstracker.TypeItemCreate, map[string]any{"kind": "task", "title": "T10"})
	w.byBo(t, "", wstracker.TypeCommentAdd, map[string]any{"item": "{t10}", "body": "@ada look", "mentions": []any{"ada"}})
	step(trackerWriteStep{`<tk:store name="team">
<tk:item.cc item="{t9}" member="ada"/>
<tk:item.cc item="{t9}" member="bo"/>
<tk:item.uncc item="{t9}" member="bo"/>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemCC, payload: map[string]any{"item": "{t9}", "member": "ada"}},
		{typ: wstracker.TypeItemCC, payload: map[string]any{"item": "{t9}", "member": "bo"}},
		{typ: wstracker.TypeItemUncc, payload: map[string]any{"item": "{t9}", "member": "bo"}},
	}})
	if got := w.trackerQuery(t, map[string]any{"store": "team", "item": w.ids["t9"]}); !strings.Contains(got, "\ncc: ada (Ada Kim)\n") {
		t.Fatalf("the item view names the CC list:\n%s", got)
	}
	related := w.trackerQuery(t, map[string]any{"store": "team", "scope": "related"})
	ccAt := strings.Index(related, "items I am CC'd on, the latest CC first:\n- "+trackerAlias(w.ids["t9"])+" [")
	menAt := strings.Index(related, "items that mention me, the newest mention first:\n- "+trackerAlias(w.ids["t10"])+" [")
	if ccAt < 0 || menAt < ccAt {
		t.Fatalf("related lists T9 under CC, then T10 under mentions:\n%s", related)
	}
	var state struct {
		My struct {
			Related wstracker.Related `json:"related"`
		} `json:"my"`
	}
	js := toolText(t, w.call(t, "tracker.query", map[string]any{"payload": map[string]any{"store": "team"}, "format": "json"}))
	if err := json.Unmarshal([]byte(js), &state); err != nil || !reflect.DeepEqual(state.My.Related.CC, []string{w.ids["t9"]}) || !reflect.DeepEqual(state.My.Related.Mentioned, []string{w.ids["t10"]}) {
		t.Fatalf("the JSON view carries the related block: %v %+v", err, state.My.Related)
	}

	// Identity: admin role, a key added and revoked, a member removed.
	spare := trackerSpareKey()
	step(trackerWriteStep{`<tk:store name="team">
<tk:roles.add_admin member="bo"/>
<tk:roles.remove_admin member="bo"/>
<tk:roles.add_key member="ada" key="` + spare + `"/>
<tk:roles.revoke_key member="ada" key="` + spare + `"/>
<tk:roles.remove_member member="bo"/>
</tk:store>`, []trackerWantEvent{
		{typ: wsledger.TypeAddAdmin, payload: map[string]any{"member": "bo"}},
		{typ: wsledger.TypeRemoveAdmin, payload: map[string]any{"member": "bo"}},
		{typ: wsledger.TypeAddKey, payload: map[string]any{"member": "ada", "key": spare}},
		{typ: wsledger.TypeRevokeKey, payload: map[string]any{"member": "ada", "key": spare}},
		{typ: wsledger.TypeRemoveMember, payload: map[string]any{"member": "bo"}},
	}})
	v = w.view(t)
	if v.Members["bo"].Removed == nil || v.Members["bo"].Admin || len(v.Members["ada"].Revoked) != 1 {
		t.Fatalf("identity after the roles commands: bo %+v, ada revoked %+v", v.Members["bo"], v.Members["ada"].Revoked)
	}

	for name, def := range trackerCommands {
		if def.build != nil && !covered[name] {
			t.Errorf("tk:%s is never written by this walk", name)
		}
	}
}

// trackerRoots is the top-level part of a view's item order.
func trackerRoots(v *wstrackerhost.StoreView) []string {
	var out []string
	for _, id := range v.ItemOrder {
		if v.Items[id].Parent == nil {
			out = append(out, id)
		}
	}
	return out
}

// TestTrackerUpdateWritesStructureAttributes writes after, refs (with $N in
// the list), rules beside people, and unassign root through tracker.update,
// and checks the exact payloads, the folded view, and the item query text.
func TestTrackerUpdateWritesStructureAttributes(t *testing.T) {
	w := &trackerWalk{trackerSessionFixture: newTrackerSessionFixture(t), ids: map[string]string{}}
	w.adaSync(t) // ada learns bo's join

	w.ada(t, trackerWriteStep{`<tk:store name="team">
<tk:item.create kind="task" title="A"/>
<tk:item.create kind="task" title="C" after="$1"/>
<tk:item.create kind="task" title="B" after="$1" refs="$1 $2">see #A and #C</tk:item.create>
<tk:item.create kind="task" title="First" after=""/>
<tk:comment.add item="$1" refs="$3">cf B</tk:comment.add>
<tk:item.edit item="$1" refs="$3">now cites B</tk:item.edit>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemCreate, name: "a", payload: map[string]any{"kind": "task", "title": "A"}},
		{typ: wstracker.TypeItemCreate, name: "c", payload: map[string]any{"kind": "task", "title": "C", "after": "{a}"}},
		{typ: wstracker.TypeItemCreate, name: "b", payload: map[string]any{"kind": "task", "title": "B", "after": "{a}", "refs": []any{"{a}", "{c}"}, "body": "see #A and #C"}},
		{typ: wstracker.TypeItemCreate, name: "first", payload: map[string]any{"kind": "task", "title": "First", "after": ""}},
		{typ: wstracker.TypeCommentAdd, name: "cm", payload: map[string]any{"item": "{a}", "refs": []any{"{b}"}, "body": "cf B"}},
		{typ: wstracker.TypeItemEdit, payload: map[string]any{"item": "{a}", "refs": []any{"{b}"}, "body": "now cites B"}},
	}})
	v := w.view(t)
	if got, want := strings.Join(trackerRoots(v), " "), strings.Join([]string{w.ids["first"], w.ids["a"], w.ids["b"], w.ids["c"]}, " "); got != want {
		t.Fatalf("root order = %s, want %s", got, want)
	}
	if got := v.Items[w.ids["b"]].Backrefs; !reflect.DeepEqual(got, []string{w.ids["a"], w.ids["cm"]}) {
		t.Fatalf("B's backrefs = %v", got)
	}
	// Each task the batch creates and does not assign is self-assigned to
	// its creator, so it reads todo.
	full := w.trackerQuery(t, map[string]any{"store": "team", "item": w.ids["b"]})
	for _, want := range []string{
		"refs:\n- " + trackerAlias(w.ids["a"]) + " [task todo] P", " A\n- " + trackerAlias(w.ids["c"]) + " [task todo] P",
		"referenced by:\n- " + trackerAlias(w.ids["a"]) + " [task todo]",
		"- comment " + trackerAlias(w.ids["cm"]) + " by ada on " + trackerAlias(w.ids["a"]) + " A\n",
	} {
		if !strings.Contains(full, want) {
			t.Fatalf("item view missing %q:\n%s", want, full)
		}
	}
	if a := w.trackerQuery(t, map[string]any{"store": "team", "item": w.ids["a"]}); !strings.Contains(a, trackerAlias(w.ids["cm"])+" ada at") || !strings.Contains(a, ", refs "+trackerAlias(w.ids["b"])+":\n  cf B") {
		t.Fatalf("a comment head prints its refs:\n%s", a)
	}

	// bo is not below ada yet: the rule rooted at bo is a request to bo,
	// beside a person target in the same event.
	w.ada(t, trackerWriteStep{`<tk:store name="team">
<tk:item.create kind="task" title="R"/>
<tk:item.assign item="$1" people="ada" rules="bo+:*" completion="assigner"/>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemCreate, name: "r", payload: map[string]any{"kind": "task", "title": "R"}},
		{typ: wstracker.TypeItemAssign, name: "asg", payload: map[string]any{"item": "{r}", "targets": []any{"ada", map[string]any{"root": "bo", "include_root": true}}, "completion": "assigner"}},
	}})
	rid := w.ids["asg"] + "~bo"
	v = w.view(t)
	if ru := v.Rules[rid]; ru.State != "pending" || ru.Levels != nil || !ru.IncludeRoot {
		t.Fatalf("rule %s = %+v, want a pending whole-subtree rule with its root", rid, ru)
	}
	if r := w.trackerQuery(t, map[string]any{"store": "team", "item": w.ids["r"]}); !strings.Contains(r, "bo pending (by ada, rule request bo+:* "+trackerAlias(rid)+", assigner closes)") || !strings.Contains(r, "ada todo (by ada, assigner closes)") {
		t.Fatalf("the item view prints the rule request and the person:\n%s", r)
	}
	w.byBo(t, "", wstracker.TypeItemAccept, map[string]any{"item": "{r}"})
	v = w.view(t)
	if v.Rules[rid].State != "live" || v.Assignments[rid+">bo"].State != "live" {
		t.Fatalf("after bo accepts: rule %+v, bo's record %+v", v.Rules[rid], v.Assignments[rid+">bo"])
	}
	if r := w.trackerQuery(t, map[string]any{"store": "team", "item": w.ids["r"]}); !strings.Contains(r, "bo todo (by ada, rule bo+:* "+trackerAlias(rid)+", assigner closes)") {
		t.Fatalf("the item view prints the rule's record:\n%s", r)
	}
	// rule names the one rule to end, in the form the query prints.
	w.ada(t, trackerWriteStep{`<tk:store name="team"><tk:item.unassign item="{r}" root="bo" rule="` + trackerAlias(rid) + `"/></tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemUnassign, payload: map[string]any{"item": "{r}", "root": "bo", "rule": rid}},
	}})
	v = w.view(t)
	if v.Rules[rid].State != "ended" || v.Assignments[rid+">bo"].State != "unassigned" || v.Assignments[w.ids["asg"]+">ada"].State != "live" {
		t.Fatalf("unassign root ends the rule only: rule %+v, bo %+v, ada %+v", v.Rules[rid], v.Assignments[rid+">bo"], v.Assignments[w.ids["asg"]+">ada"])
	}

	text := w.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:store name="team">
<tk:item.assign item="ada.pc-1" rules="bo:0 :2 bo+ bo:*"/>
<tk:item.assign item="ada.pc-1" levels="1" rules="bo:1"/>
<tk:item.assign item="ada.pc-1" completion="assigner"/>
<tk:item.create kind="task" title="x" refs="nope $9"/>
<tk:item.unassign item="ada.pc-1" rule="nope~bo"/>
</tk:store>`})
	for _, want := range []string{
		`command 1 <tk:item.assign> (line 2) rules: "bo:0" must be member:N`,
		`command 1 <tk:item.assign> (line 2) rules: ":2" must be member:N`,
		`command 1 <tk:item.assign> (line 2) rules: "bo+" must be member:N`,
		`command 2 <tk:item.assign> (line 3): levels stands alone; beside people or rules, write it as the rule rules="<your id>:N"`,
		"command 3 <tk:item.assign> (line 4): takes people, rules, or both",
		`command 4 <tk:item.create> (line 5) refs: must be an item ID such as maya.pc-5, or $N; got "nope"`,
		"command 4 <tk:item.create> (line 5) refs: $N names an earlier command of this batch by position",
		`command 5 <tk:item.unassign> (line 6) rule: must be a rule ID as the query prints it, such as maya.pc-5~raj; got "nope~bo"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("refusal missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, `"bo:*"`) {
		t.Fatalf("bo:* is a valid rule:\n%s", text)
	}

	// The take-work commands' grammar.
	text = w.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:store name="team">
<tk:item.create kind="task" title="x"/>
<tk:item.open item="$1" scope="org"/>
<tk:item.open item="$1"/>
<tk:item.request item="$1">no assignee</tk:item.request>
<tk:item.approve item="$1" request="$1"/>
<tk:item.deny item="$1"/>
<tk:item.unrequest item="$1" request="nope"/>
</tk:store>`})
	for _, want := range []string{
		`command 2 <tk:item.open> (line 3) scope: must be one of team, store; got "org"`,
		"command 3 <tk:item.open> (line 4) scope: required",
		"command 4 <tk:item.request> (line 5) assignee: required",
		"command 5 <tk:item.approve> (line 6) request: $1 names command 1, which creates no request; $N must name a <tk:item.request>",
		"command 6 <tk:item.deny> (line 7) request: required",
		`command 7 <tk:item.unrequest> (line 8) request: must be a request ID such as maya.pc-5, or $N; got "nope"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("refusal missing %q:\n%s", want, text)
		}
	}
}

// TestTrackerUpdateCompletionDefaultAndSuperiorWrapUp writes through
// tracker.update against the two-member team store: an assign to a peer
// defaults to completion assignee (pending until accepted), one to a member
// below the writer defaults to assigner, and the writer, as that member's
// org superior, sets done on their assignment with assignee=, final with no
// review step.
func TestTrackerUpdateCompletionDefaultAndSuperiorWrapUp(t *testing.T) {
	w := &trackerWalk{trackerSessionFixture: newTrackerSessionFixture(t), ids: map[string]string{}}
	w.adaSync(t) // ada learns bo's join

	// bo is not below ada yet: a peer.
	w.ada(t, trackerWriteStep{`<tk:store name="team">
<tk:item.create kind="task" title="Peer"/>
<tk:item.assign item="$1" people="bo"/>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemCreate, name: "peer", payload: map[string]any{"kind": "task", "title": "Peer"}},
		{typ: wstracker.TypeItemAssign, payload: map[string]any{"item": "{peer}", "mode": "chosen", "people": []any{"bo"}, "completion": "assignee"}},
	}})
	if got := recStates(w.view(t).Assignments, w.ids["peer"], "bo"); !reflect.DeepEqual(got, []string{"pending/todo"}) {
		t.Fatalf("bo's assignment from a peer: %v", got)
	}

	w.ada(t, trackerWriteStep{`<tk:store name="team"><tk:roles.set_lead member="bo" leads="ada"/></tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeSetLead, payload: map[string]any{"member": "bo", "leads": []any{"ada"}}},
	}})
	w.ada(t, trackerWriteStep{`<tk:store name="team">
<tk:item.create kind="task" title="Sub"/>
<tk:item.assign item="$1" people="bo"/>
<tk:item.status item="$1" assignee="bo" status="done">wrapped up</tk:item.status>
</tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemCreate, name: "sub", payload: map[string]any{"kind": "task", "title": "Sub"}},
		{typ: wstracker.TypeItemAssign, payload: map[string]any{"item": "{sub}", "mode": "chosen", "people": []any{"bo"}, "completion": "assigner"}},
		{typ: wstracker.TypeItemStatus, payload: map[string]any{"item": "{sub}", "assignee": "bo", "status": "done", "note": "wrapped up"}},
	}})
	v := w.view(t)
	if got := recStates(v.Assignments, w.ids["sub"], "bo"); !reflect.DeepEqual(got, []string{"live/done"}) {
		t.Fatalf("bo's assignment after the superior's done: %v", got)
	}
	if st := v.Items[w.ids["sub"]].Status; st == nil || *st != "done" {
		t.Fatalf("the item after the superior's done: %v", st)
	}
	// A done record takes no further status write.
	if text := w.trackerRefused(t, "tracker.update", map[string]any{"commands": w.expand(`<tk:store name="team"><tk:item.status item="{sub}" assignee="bo" status="dropped"/></tk:store>`)}); !strings.Contains(text, "the store would flag this write: assignment is done; reopen it first") {
		t.Fatalf("a status on a done assignment must be flagged:\n%s", text)
	}
}

// The item's live assignee edits its body with base, the body revision the
// item view prints: an edit the creator made since merges in instead of
// being replaced.
func TestTrackerUpdateAssigneeEditsBodyWithBase(t *testing.T) {
	w := &trackerWalk{trackerSessionFixture: newTrackerSessionFixture(t), ids: map[string]string{}}
	w.adaSync(t) // ada learns bo's join
	w.byBo(t, "it", wstracker.TypeItemCreate, map[string]any{"kind": "task", "title": "Shared", "body": "a\nb\nc"})
	w.byBo(t, "", wstracker.TypeItemAssign, map[string]any{"item": "{it}", "mode": "chosen", "people": []any{"ada"}})
	w.ada(t, trackerWriteStep{`<tk:store name="team"><tk:item.accept item="{it}"/></tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemAccept, payload: map[string]any{"item": "{it}"}},
	}})
	seen := trackerAlias(w.ids["it"])
	if got := w.trackerQuery(t, map[string]any{"store": "team", "item": w.ids["it"]}); !strings.Contains(got, "body revision: "+seen+"\nbody:\na\nb\nc\n") {
		t.Fatalf("the item view must print the body revision before the body:\n%s", got)
	}
	// bo changes the first line; ada, from the revision she saw, the last.
	w.byBo(t, "", wstracker.TypeItemEdit, map[string]any{"item": "{it}", "body": "A\nb\nc"})
	w.ada(t, trackerWriteStep{`<tk:store name="team"><tk:item.edit item="{it}" base="` + seen + `">a
b
C</tk:item.edit></tk:store>`, []trackerWantEvent{
		{typ: wstracker.TypeItemEdit, name: "edit", payload: map[string]any{"item": "{it}", "body": "a\nb\nC", "base": []any{"{it}"}}},
	}})
	if got := w.view(t).Items[w.ids["it"]].Body; got != "A\nb\nC" {
		t.Fatalf("the edit must merge with bo's: body %q", got)
	}
	if got := w.trackerQuery(t, map[string]any{"store": "team", "item": w.ids["it"]}); !strings.Contains(got, "body revision: "+trackerAlias(w.ids["edit"])+"\n") {
		t.Fatalf("the revision must follow the merged write:\n%s", got)
	}
}

// base takes the body revision's event IDs in either form, written as the
// stored ids, and only beside a body; it changes nothing alone.
func TestTrackerUpdateItemEditBase(t *testing.T) {
	b := trackerPlan(t, `<tk:item.edit item="ada.pc-1" base="maya.pc#5 ada.pc-3">new</tk:item.edit>`+
		`<tk:item.edit item="ada.pc-1" clear="body" base="ada.pc-3"/>`)
	if got := b.events[0].payload["base"]; !reflect.DeepEqual(got, []string{"maya.pc#5", "ada.pc#3"}) {
		t.Fatalf("base = %#v", got)
	}
	if got := b.events[1].payload["base"]; !reflect.DeepEqual(got, []string{"ada.pc#3"}) {
		t.Fatalf("base with clear=body = %#v", got)
	}
	var problems wsxmlcmd.Problems
	planTrackerBatch(&problems, `<tk:store name="team"><tk:item.create kind="task" title="t"/>`+
		`<tk:item.edit item="ada.pc-1" base="ada.pc-3"/>`+
		`<tk:item.edit item="ada.pc-1" title="x" base="ada.pc-3"/>`+
		`<tk:item.edit item="ada.pc-1" base="ada.pc-3 maya">new</tk:item.edit>`+
		`<tk:item.edit item="ada.pc-1" base="$1">new</tk:item.edit>`+
		`<tk:item.edit item="ada.pc-1" base=" ">new</tk:item.edit></tk:store>`, []wstrackerhost.StoreEntry{{ID: "team", Member: "ada"}}, trackerTestReach)
	for _, want := range []string{
		`command 2 <tk:item.edit> (line 1): changes nothing`,
		`command 2 <tk:item.edit> (line 1) base: goes with a body or clear="body"`,
		`command 3 <tk:item.edit> (line 1) base: goes with a body or clear="body"`,
		`command 4 <tk:item.edit> (line 1) base: must be event IDs of the body revision the item view prints, such as maya.pc-5; got "maya"`,
		`command 5 <tk:item.edit> (line 1) base: takes the event IDs the item view prints, never $N`,
		`command 6 <tk:item.edit> (line 1) base: must list the body revision the item view prints`,
	} {
		if !slices.ContainsFunc(problems.List, func(p string) bool { return strings.HasPrefix(p, want) }) {
			t.Errorf("problems missing %q:\n%q", want, problems.List)
		}
	}
}
