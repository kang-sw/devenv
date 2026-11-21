// Tracker command grammar and event payload builders.
package mcp

import (
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

// agent-auto marks a write the agent made unasked.
var trackerOrigins = []string{wstracker.OriginAgent, wstracker.OriginAgentAuto}

// trackerCommandDef is one tk: command: its grammar, and how it maps onto
// its event's payload. build is nil for the store commands, which are not
// events (register, sync, unregister, relabel, hide, unhide).
type trackerCommandDef struct {
	spec  wsxmlcmd.Spec
	build func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent)
}

const (
	trackerRegister   = "register"
	trackerSync       = "sync"
	trackerUnregister = "unregister"
	trackerRelabel    = "relabel"
	trackerHide       = "hide"
	trackerUnhide     = "unhide"
)

// trackerSetStatuses are the statuses tk:item.status takes: the
// assignee's own (done goes through tk:item.close) and an org superior's
// wrap-up (blocked, done, dropped) together.
var trackerSetStatuses = []string{"idea", "todo", "doing", "blocked", "in-review", "done", "dropped"}

// trackerEventAttrs are the attributes every event command takes besides
// its own.
var trackerEventAttrs = []string{"origin"}

// trackerEventSpec is an event command: scoped, so it sits in the batch's
// tk:store group, which names its store. Its body (an item body, a comment,
// or a note) decodes like every command body (package wsxmlcmd).
func trackerEventSpec(kind wsxmlcmd.Kind, attrs ...string) wsxmlcmd.Spec {
	return wsxmlcmd.Spec{Kind: kind, Attrs: append(append([]string{}, trackerEventAttrs...), attrs...), Scoped: true}
}

// trackerCommands is the command table, keyed by name without the prefix:
// each event type of the tracker's write schema under its own name, plus
// the store commands. A body is the event's text field: the item body, the
// comment, or the note.
var trackerCommands = map[string]trackerCommandDef{
	wstracker.TypeItemCreate: {trackerEventSpec(wsxmlcmd.Leaf, "kind", "title", "project", "labels", "due", "start", "priority", "parent", "after", "mentions", "refs"), buildItemCreate},
	wstracker.TypeItemEdit:   {trackerEventSpec(wsxmlcmd.Leaf, "item", "title", "project", "labels", "due", "start", "priority", "mentions", "refs", "clear", "base"), buildItemEdit},
	wstracker.TypeItemHide:   {trackerEventSpec(wsxmlcmd.Empty, "item"), buildItemOnly},
	wstracker.TypeItemMove:   {trackerEventSpec(wsxmlcmd.Empty, "item", "parent", "after"), buildItemMove},
	wstracker.TypeItemAssign: {trackerEventSpec(wsxmlcmd.Empty, "item", "people", "rules", "levels", "completion"), buildItemAssign},
	wstracker.TypeItemHandoff: {trackerEventSpec(wsxmlcmd.Leaf, "item", "to", "assignee"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		b.itemRef(cmd, ev, "item")
		b.required(cmd, ev, "to")
		b.optional(cmd, ev, "assignee")
		b.note(cmd, ev)
	}},
	wstracker.TypeItemAccept:  {trackerEventSpec(wsxmlcmd.Leaf, "item"), buildItemNote},
	wstracker.TypeItemDecline: {trackerEventSpec(wsxmlcmd.Leaf, "item"), buildItemNote},
	wstracker.TypeItemReturn:  {trackerEventSpec(wsxmlcmd.Leaf, "item", "assignee"), buildItemAssigneeNote},
	wstracker.TypeItemUnassign: {trackerEventSpec(wsxmlcmd.Empty, "item", "assignee", "root", "rule"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		buildItemOptionalAssignee(b, cmd, ev)
		b.optional(cmd, ev, "root")
		if v, ok := cmd.Attr("rule"); ok {
			if id, err := parseTrackerRuleID(strings.TrimSpace(v)); err == nil {
				ev.payload["rule"] = id
			} else {
				b.p.Add("%s: must be a rule ID as the query prints it, such as maya.pc-5~raj; got %q", cmd.At("rule"), v)
			}
		}
	}},
	// assignee names another member's assignment (an org superior's
	// wrap-up, or the assigner's drop); the store judges which statuses
	// apply to whom, so the parser only holds the status set.
	wstracker.TypeItemStatus: {trackerEventSpec(wsxmlcmd.Leaf, "item", "status", "assignee"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		b.itemRef(cmd, ev, "item")
		if v, ok := wsxmlcmd.EnumAttr(b.p, cmd, "status", trackerSetStatuses); ok {
			ev.payload["status"] = v
		}
		b.optional(cmd, ev, "assignee")
		b.note(cmd, ev)
	}},
	wstracker.TypeItemClose:   {trackerEventSpec(wsxmlcmd.Leaf, "item"), buildItemNote},
	wstracker.TypeItemConfirm: {trackerEventSpec(wsxmlcmd.Leaf, "item", "assignee"), buildItemAssigneeNote},
	wstracker.TypeItemReject:  {trackerEventSpec(wsxmlcmd.Leaf, "item", "assignee"), buildItemAssigneeNote},
	wstracker.TypeItemReopen: {trackerEventSpec(wsxmlcmd.Leaf, "item", "assignee"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		buildItemOptionalAssignee(b, cmd, ev)
		b.note(cmd, ev)
	}},
	wstracker.TypeItemOpen: {trackerEventSpec(wsxmlcmd.Empty, "item", "scope"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		b.itemRef(cmd, ev, "item")
		if v, ok := wsxmlcmd.EnumAttr(b.p, cmd, "scope", []string{wstracker.ScopeTeam, wstracker.ScopeStore}); ok {
			ev.payload["scope"] = v
		}
	}},
	wstracker.TypeItemUnopen:    {trackerEventSpec(wsxmlcmd.Empty, "item"), buildItemOnly},
	wstracker.TypeItemClaim:     {trackerEventSpec(wsxmlcmd.Empty, "item"), buildItemOnly},
	wstracker.TypeItemRequest:   {trackerEventSpec(wsxmlcmd.Leaf, "item", "assignee"), buildItemAssigneeNote},
	wstracker.TypeItemApprove:   {trackerEventSpec(wsxmlcmd.Leaf, "item", "request"), buildItemRequestNote},
	wstracker.TypeItemDeny:      {trackerEventSpec(wsxmlcmd.Leaf, "item", "request"), buildItemRequestNote},
	wstracker.TypeItemUnrequest: {trackerEventSpec(wsxmlcmd.Empty, "item", "request"), buildItemRequest},
	wstracker.TypeItemCC:        {trackerEventSpec(wsxmlcmd.Empty, "item", "member"), buildItemMember},
	wstracker.TypeItemUncc:      {trackerEventSpec(wsxmlcmd.Empty, "item", "member"), buildItemMember},
	wstracker.TypeCommentAdd: {trackerEventSpec(wsxmlcmd.Leaf, "item", "reply_to", "mentions", "refs"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		b.itemRef(cmd, ev, "item")
		if _, ok := cmd.Attr("reply_to"); ok {
			b.commentRef(cmd, ev, "reply_to")
		}
		b.list(cmd, ev, "mentions")
		b.itemRefList(cmd, ev, "refs")
		b.text(cmd, ev, "body", true)
	}},
	wstracker.TypeCommentEdit: {trackerEventSpec(wsxmlcmd.Leaf, "comment", "mentions", "refs"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		b.commentRef(cmd, ev, "comment")
		b.list(cmd, ev, "mentions")
		b.itemRefList(cmd, ev, "refs")
		b.text(cmd, ev, "body", true)
	}},
	wstracker.TypeCommentHide: {trackerEventSpec(wsxmlcmd.Empty, "comment"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		b.commentRef(cmd, ev, "comment")
	}},
	wstracker.TypeMemberProfile: {trackerEventSpec(wsxmlcmd.Empty, "org", "family", "given", "member"), buildMemberProfile},
	wstracker.TypeSetLead: {trackerEventSpec(wsxmlcmd.Empty, "member", "leads"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		b.required(cmd, ev, "member")
		if v, ok := cmd.Attr("leads"); ok {
			ev.payload["leads"] = strings.Fields(v)
		} else {
			b.p.Add("%s: required (empty for no lead)", cmd.At("leads"))
		}
	}},
	wsledger.TypeAddAdmin:     {trackerEventSpec(wsxmlcmd.Empty, "member"), buildMemberOnly},
	wsledger.TypeRemoveAdmin:  {trackerEventSpec(wsxmlcmd.Empty, "member"), buildMemberOnly},
	wsledger.TypeRemoveMember: {trackerEventSpec(wsxmlcmd.Empty, "member"), buildMemberOnly},
	wsledger.TypeAddKey:       {trackerEventSpec(wsxmlcmd.Empty, "member", "key"), buildMemberKey},
	wsledger.TypeRevokeKey:    {trackerEventSpec(wsxmlcmd.Empty, "member", "key"), buildMemberKey},
	wstracker.TypeSetPriorityLevels: {trackerEventSpec(wsxmlcmd.Empty, "names", "default"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		if v, ok := wsxmlcmd.RequiredAttr(b.p, cmd, "names"); ok {
			ev.payload["names"] = strings.Fields(v)
		}
		if n, ok := trackerLevelAttr(b.p, cmd, "default", true); ok {
			ev.payload["default"] = n
		}
	}},
	wstracker.TypeSetAttachmentLimit: {trackerEventSpec(wsxmlcmd.Empty, "max_bytes"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		if n, ok := wsxmlcmd.IntAttr(b.p, cmd, "max_bytes", true); ok {
			ev.payload["max_bytes"] = n
		}
	}},
	wstracker.TypeAddLabel: {trackerEventSpec(wsxmlcmd.Empty, "name", "color"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		b.required(cmd, ev, "name")
		if v, ok := wsxmlcmd.RequiredAttr(b.p, cmd, "color"); ok {
			ev.payload["color"] = trackerColor(v)
		}
	}},
	wstracker.TypeEditLabel: {trackerEventSpec(wsxmlcmd.Empty, "label", "name", "color"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		b.labelRef(cmd, ev)
		b.optional(cmd, ev, "name")
		if v, ok := cmd.Attr("color"); ok {
			ev.payload["color"] = trackerColor(v)
		}
		_, name := cmd.Attr("name")
		_, color := cmd.Attr("color")
		if !name && !color {
			b.p.Add("%s: changes nothing; set name, color, or both", cmd.Label())
		}
	}},
	wstracker.TypeRemoveLabel: {trackerEventSpec(wsxmlcmd.Empty, "label"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		b.labelRef(cmd, ev)
	}},
	trackerRegister: {spec: wsxmlcmd.Spec{Kind: wsxmlcmd.Empty, Attrs: []string{"store", "member", "remote", "label", "org", "family", "given"}}},
	trackerSync:     {spec: wsxmlcmd.Spec{Kind: wsxmlcmd.Empty, Attrs: []string{"store"}}},
	// The machine-local store commands: they change this machine's registry
	// (and, for unregister, delete the local clone), never the store.
	trackerUnregister: {spec: wsxmlcmd.Spec{Kind: wsxmlcmd.Empty, Attrs: []string{"store", "confirm"}}},
	trackerRelabel:    {spec: wsxmlcmd.Spec{Kind: wsxmlcmd.Empty, Attrs: []string{"store", "label"}}},
	trackerHide:       {spec: wsxmlcmd.Spec{Kind: wsxmlcmd.Empty, Attrs: []string{"store"}}},
	trackerUnhide:     {spec: wsxmlcmd.Spec{Kind: wsxmlcmd.Empty, Attrs: []string{"store"}}},
}

// trackerGrammar is the tk: language: the command table, and the tk:store
// group that names the store of the event commands inside it, so a batch
// names its one store once.
var trackerGrammar = func() wsxmlcmd.Grammar {
	g := wsxmlcmd.Grammar{Prefix: trackerPrefix, Commands: map[string]wsxmlcmd.Spec{}, Group: &wsxmlcmd.Group{Name: "store", Attr: "name"}}
	for name, def := range trackerCommands {
		g.Commands[name] = def.spec
	}
	return g
}()

func buildItemCreate(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	// todo stays accepted on input; the store folds it as a task.
	if v, ok := wsxmlcmd.EnumAttr(b.p, cmd, "kind", []string{"task", "todo", "topic"}); ok {
		ev.payload["kind"] = v
	}
	b.required(cmd, ev, "title")
	b.optional(cmd, ev, "project")
	b.labelRefList(cmd, ev)
	b.optional(cmd, ev, "due")
	b.optional(cmd, ev, "start")
	if n, ok := trackerLevelAttr(b.p, cmd, "priority", false); ok {
		ev.payload["priority"] = n
	}
	if _, ok := cmd.Attr("parent"); ok {
		b.itemRef(cmd, ev, "parent")
	}
	// after="" puts the item first among its siblings; absent, last.
	if v, ok := cmd.Attr("after"); ok {
		if strings.TrimSpace(v) == "" {
			ev.payload["after"] = ""
		} else {
			b.itemRef(cmd, ev, "after")
		}
	}
	b.list(cmd, ev, "mentions")
	b.itemRefList(cmd, ev, "refs")
	b.text(cmd, ev, "body", false)
}

// buildItemEdit writes only the fields the command changes: an attribute
// present sets its field, a non-empty body sets the body, and clear names
// fields to empty (body) or remove (due and start, written as null).
func buildItemEdit(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.itemRef(cmd, ev, "item")
	if v, ok := cmd.Attr("title"); ok && wsxmlcmd.NonEmpty(b.p, cmd.At("title"), v) {
		ev.payload["title"] = v
	}
	if v, ok := cmd.Attr("project"); ok {
		ev.payload["project"] = v
	}
	// labels="" clears them; a list replaces them
	b.labelRefList(cmd, ev)
	if v, ok := cmd.Attr("due"); ok && wsxmlcmd.NonEmpty(b.p, cmd.At("due")+" (clear=\"due\" removes it)", v) {
		ev.payload["due"] = v
	}
	if v, ok := cmd.Attr("start"); ok && wsxmlcmd.NonEmpty(b.p, cmd.At("start")+" (clear=\"start\" removes it)", v) {
		ev.payload["start"] = v
	}
	if n, ok := trackerLevelAttr(b.p, cmd, "priority", false); ok {
		ev.payload["priority"] = n
	}
	b.list(cmd, ev, "mentions")
	b.itemRefList(cmd, ev, "refs")
	if cmd.Body != "" {
		ev.payload["body"] = cmd.Body
	}
	if v, ok := cmd.Attr("clear"); ok {
		for _, field := range strings.Fields(v) {
			switch field {
			case "body":
				if cmd.Body != "" {
					b.p.Add("%s: clear=\"body\" with a body; send one or the other", cmd.Label())
				}
				ev.payload["body"] = ""
			case "due":
				if _, set := cmd.Attr("due"); set {
					b.p.Add("%s: clear=\"due\" with a due attribute; send one or the other", cmd.Label())
				}
				ev.payload["due"] = nil
			case "start":
				if _, set := cmd.Attr("start"); set {
					b.p.Add("%s: clear=\"start\" with a start attribute; send one or the other", cmd.Label())
				}
				ev.payload["start"] = nil
			default:
				b.p.Add("%s: must list body, due, or start; got %q", cmd.At("clear"), field)
			}
		}
	}
	// base qualifies the body write, so it is read after it and changes
	// nothing on its own.
	changed := len(ev.payload) + len(ev.refLists)
	if _, ok := ev.payload["item"]; ok {
		changed--
	}
	if changed == 0 {
		b.p.Add("%s: changes nothing; set an attribute, a body, or clear", cmd.Label())
	}
	b.bodyBase(cmd, ev)
}

// bodyBase reads item.edit's base: the body revision the editor saw, as the
// item view prints it, written as the list of stored ids. $N is refused:
// an event of this batch is never a revision the editor saw.
func (b *trackerBatch) bodyBase(cmd wsxmlcmd.Command, ev *trackerEvent) {
	v, ok := cmd.Attr("base")
	if !ok {
		return
	}
	if _, body := ev.payload["body"]; !body {
		b.p.Add("%s: goes with a body or clear=\"body\"", cmd.At("base"))
		return
	}
	tokens := strings.Fields(v)
	if len(tokens) == 0 {
		b.p.Add("%s: must list the body revision the item view prints, such as maya.pc-5; got %q", cmd.At("base"), v)
		return
	}
	ids := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		if strings.HasPrefix(tok, "$") {
			b.p.Add("%s: takes the event IDs the item view prints, never $N (an event of this batch is no revision you saw); got %q", cmd.At("base"), tok)
			return
		}
		id, err := parseTrackerID(tok)
		if err != nil {
			b.p.Add("%s: must be event IDs of the body revision the item view prints, such as maya.pc-5; got %q", cmd.At("base"), tok)
			return
		}
		ids = append(ids, id)
	}
	ev.payload["base"] = ids
}

// buildItemMove reads item.move: parent is required, "" for top level
// (null in the payload); after as on item.create.
func buildItemMove(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.itemRef(cmd, ev, "item")
	if v, ok := cmd.Attr("parent"); !ok {
		b.p.Add("%s: required (an item ID, or \"\" for top level)", cmd.At("parent"))
	} else if strings.TrimSpace(v) == "" {
		ev.payload["parent"] = nil
	} else {
		b.itemRef(cmd, ev, "parent")
	}
	if v, ok := cmd.Attr("after"); ok {
		if strings.TrimSpace(v) == "" {
			ev.payload["after"] = ""
		} else {
			b.itemRef(cmd, ev, "after")
		}
	}
}

func buildItemOnly(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.itemRef(cmd, ev, "item")
}

// buildItemMember reads tk:item.cc and tk:item.uncc: member is the member
// ID added to or removed from the item's CC list.
func buildItemMember(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.itemRef(cmd, ev, "item")
	b.required(cmd, ev, "member")
}

func buildItemNote(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.itemRef(cmd, ev, "item")
	b.note(cmd, ev)
}

func buildItemAssigneeNote(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.itemRef(cmd, ev, "item")
	b.required(cmd, ev, "assignee")
	b.note(cmd, ev)
}

// buildItemRequest reads a take-request answer or withdrawal: request is
// the tk:item.request's event ID, or $N for one earlier in the batch.
func buildItemRequest(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.itemRef(cmd, ev, "item")
	b.ref(cmd, ev, "request", wstracker.TypeItemRequest, "request")
}

func buildItemRequestNote(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	buildItemRequest(b, cmd, ev)
	b.note(cmd, ev)
}

func buildItemOptionalAssignee(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.itemRef(cmd, ev, "item")
	b.optional(cmd, ev, "assignee")
}

// buildItemAssign maps people alone onto mode chosen and levels onto mode
// all; rules, with or without people, become a targets list: the people,
// then the rules.
func buildItemAssign(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.itemRef(cmd, ev, "item")
	people, hasPeople := cmd.Attr("people")
	rules, hasRules := cmd.Attr("rules")
	_, hasLevels := cmd.Attr("levels")
	switch {
	case hasLevels && (hasPeople || hasRules):
		b.p.Add("%s: levels stands alone; beside people or rules, write it as the rule rules=\"<your id>:N\"", cmd.Label())
	case hasLevels:
		if n, ok := wsxmlcmd.IntAttr(b.p, cmd, "levels", true); ok {
			ev.payload["mode"], ev.payload["levels"] = "all", n
		}
	case hasRules:
		targets := []any{}
		for _, person := range strings.Fields(people) {
			targets = append(targets, person)
		}
		if wsxmlcmd.NonEmpty(b.p, cmd.At("rules"), rules) {
			for _, tok := range strings.Fields(rules) {
				if rule, ok := parseTrackerRule(tok); ok {
					targets = append(targets, rule)
				} else {
					b.p.Add("%s: %q must be member:N (everyone within N levels below member) or member:* (the whole subtree), with member+ to include the member", cmd.At("rules"), tok)
				}
			}
		}
		ev.payload["targets"] = targets
	case hasPeople:
		if wsxmlcmd.NonEmpty(b.p, cmd.At("people"), people) {
			ev.payload["mode"], ev.payload["people"] = "chosen", strings.Fields(people)
		}
	default:
		b.p.Add("%s: takes people, rules, or both", cmd.Label())
	}
	if _, ok := cmd.Attr("completion"); ok {
		if v, ok := wsxmlcmd.EnumAttr(b.p, cmd, "completion", []string{"assignee", "assigner"}); ok {
			ev.payload["completion"] = v
		}
	}
}

// parseTrackerRule reads one rule of the rules grammar: root, + to include
// the root, then :N for N levels below it or :* for the whole subtree.
func parseTrackerRule(tok string) (map[string]any, bool) {
	root, depth, ok := strings.Cut(tok, ":")
	root, include := strings.CutSuffix(root, "+")
	if !ok || root == "" || strings.ContainsAny(root, "+:") {
		return nil, false
	}
	rule := map[string]any{"root": root}
	if include {
		rule["include_root"] = true
	}
	if depth != "*" {
		n, err := strconv.Atoi(depth)
		if err != nil || n < 1 {
			return nil, false
		}
		rule["levels"] = n
	}
	return rule, true
}

// buildMemberProfile writes the profile fields present; org and family may
// be empty, which clears them.
func buildMemberProfile(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	profile := map[string]any{}
	for _, field := range []string{"org", "family", "given"} {
		if v, ok := cmd.Attr(field); ok {
			profile[field] = v
		}
	}
	if len(profile) == 0 {
		b.p.Add("%s: set org, family, or given", cmd.Label())
	}
	ev.payload["profile"] = profile
	b.optional(cmd, ev, "member")
}

func buildMemberOnly(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.required(cmd, ev, "member")
}

func buildMemberKey(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.required(cmd, ev, "member")
	b.required(cmd, ev, "key")
}

// trackerLevelAttr returns a whole-number attribute that may be 0 (a
// priority level, the default level).
func trackerLevelAttr(p *wsxmlcmd.Problems, cmd wsxmlcmd.Command, name string, required bool) (int, bool) {
	v, present := cmd.Attr(name)
	if !present {
		if required {
			p.Add("%s: required", cmd.At(name))
		}
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		p.Add("%s: must be a whole number from 0; got %q", cmd.At(name), v)
		return 0, false
	}
	return n, true
}

// trackerEvent is one event command as planned: its payload, with the
// fields that name an event of the same batch ($N) left to resolve once
// the store's next event id is known.
