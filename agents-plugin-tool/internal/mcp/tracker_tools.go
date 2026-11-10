package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

// The tracker tools: tracker.update applies one batch of tk: commands (the
// shared command language, package wsxmlcmd) to the machine's tracker
// stores through wstrackerhost, and tracker.query reads them. Both are
// always listed and work whatever workflow.tracker_enabled says; a
// successful call turns that knob on at session scope, which adds the
// tracker section to the workflow manual. The command table and rules live
// in the tracker playbook, which the manual section, the refusals, and
// playbook.read all quote. tracker.show (tracker_show.go) is listed only
// where a support conversation's profile allows it; the same gate admits it
// for that conversation alone, and trackerLeadOnlyDenial refuses it to any
// other spawned session.

const (
	trackerPlaybookName = "tracker"
	trackerPrefix       = "tk:"
	// trackerRegisterTimeout bounds a tk:register: a clone of the remote,
	// the join, and its push. Sync bounds itself (wstrackerhost).
	trackerRegisterTimeout = 3 * time.Minute
)

func isTrackerTool(name string) bool {
	return name == "tracker.update" || name == "tracker.query"
}

func trackerLeadOnlyDenial(name string) string {
	if name == trackerShowTool {
		return fmt.Sprintf("%s is not available to a spawned session: only a tracker store's support conversation draws item cards, in that store's chat.", name)
	}
	return fmt.Sprintf("%s is not available to a spawned session: the tracker records the user's work through the top-level lead. Put progress in your final report; your lead records it.", name)
}

// trackerSessionKey returns the caller's session key when it names a
// top-level lead, or a store's support conversation whose profile names
// tool; for the latter it also returns the conversation's scope, whose
// store pins its writes. The call gate refuses a spawned key only when it
// finds the key, so a key the store cannot find (made up, stale, expired)
// would pass it; this refuses one before any registry read or write.
func (s *Server) trackerSessionKey(tool string, args map[string]any) (string, *trackerSupportScope, error) {
	key, err := sessionStateKey(tool, args)
	if err != nil {
		return "", nil, err
	}
	entry, found := s.sessions.lookup(key)
	if !found {
		return "", nil, mailboxUnknownSessionError(tool)
	}
	if entry.isTopLevelLead() {
		return key, nil, nil
	}
	if scope, ok := s.trackerSupport(key, entry); ok && scope.tools[tool] {
		return key, &scope, nil
	}
	return "", nil, errors.New(trackerLeadOnlyDenial(tool))
}

// supportPinProblems records what a support conversation's batch may not
// do: reach the network (register or sync), or write to a store other than
// the conversation's own.
func supportPinProblems(p *wsxmlcmd.Problems, b *trackerBatch, store string) {
	if b.network != nil {
		p.Add("%s: a store's support conversation cannot register or sync a store; the user does that from the tracker drawer", b.network.Label())
	}
	// A batch writes to the one store its <tk:store> group names (b.store);
	// a second group is already refused by the parse.
	if b.store != "" && b.store != store && len(b.events) > 0 {
		at := b.events[0].cmd.Label()
		if sc := b.events[0].cmd.Scope; sc != nil {
			at = sc.At()
		}
		p.Add("%s: %q; this conversation writes to its own store %q only, and every other store is read-only here", at, b.store, store)
	}
}

// trackerOrigins are the origins a tracker.update write may carry: every
// write through this tool is an agent's, so human is not among them, and
// agent-auto marks a write the agent made unasked.
var trackerOrigins = []string{wstracker.OriginAgent, wstracker.OriginAgentAuto}

// trackerCommandDef is one tk: command: its grammar, and how it maps onto
// its event's payload. build is nil for the two commands that are not
// events (register, sync).
type trackerCommandDef struct {
	spec  wsxmlcmd.Spec
	build func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent)
}

const (
	trackerRegister = "register"
	trackerSync     = "sync"
)

// trackerEventAttrs are the attributes every event command takes besides
// its own.
var trackerEventAttrs = []string{"origin"}

// trackerEventSpec is an event command: scoped, so it sits in the batch's
// tk:store group, which names its store. Its body (an item body, a comment,
// or a note) is shown as plain text, never as raw HTML, and matched against
// nothing, so an escaped body is decoded like the attributes.
func trackerEventSpec(kind wsxmlcmd.Kind, attrs ...string) wsxmlcmd.Spec {
	return wsxmlcmd.Spec{Kind: kind, Attrs: append(append([]string{}, trackerEventAttrs...), attrs...), DecodeBody: true, Scoped: true}
}

// trackerCommands is the command table, keyed by name without the prefix:
// each event type of the tracker's write schema under its own name, plus
// register and sync. A body is the event's text field: the item body, the
// comment, or the note.
var trackerCommands = map[string]trackerCommandDef{
	wstracker.TypeItemCreate: {trackerEventSpec(wsxmlcmd.Leaf, "kind", "title", "project", "due", "priority", "parent", "after", "mentions", "refs", "stage"), buildItemCreate},
	wstracker.TypeItemEdit:   {trackerEventSpec(wsxmlcmd.Leaf, "item", "title", "project", "due", "priority", "mentions", "refs", "stage", "clear"), buildItemEdit},
	wstracker.TypeItemHide:   {trackerEventSpec(wsxmlcmd.Empty, "item"), buildItemOnly},
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
	wstracker.TypeItemUnassign: {trackerEventSpec(wsxmlcmd.Empty, "item", "assignee", "root"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		buildItemOptionalAssignee(b, cmd, ev)
		b.optional(cmd, ev, "root")
	}},
	wstracker.TypeItemStatus: {trackerEventSpec(wsxmlcmd.Leaf, "item", "status"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		b.itemRef(cmd, ev, "item")
		if v, ok := wsxmlcmd.EnumAttr(b.p, cmd, "status", []string{"todo", "doing", "blocked"}); ok {
			ev.payload["status"] = v
		}
		b.note(cmd, ev)
	}},
	wstracker.TypeItemClose:   {trackerEventSpec(wsxmlcmd.Leaf, "item"), buildItemNote},
	wstracker.TypeItemConfirm: {trackerEventSpec(wsxmlcmd.Leaf, "item", "assignee"), buildItemAssigneeNote},
	wstracker.TypeItemReject:  {trackerEventSpec(wsxmlcmd.Leaf, "item", "assignee"), buildItemAssigneeNote},
	wstracker.TypeItemReopen: {trackerEventSpec(wsxmlcmd.Leaf, "item", "assignee"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		buildItemOptionalAssignee(b, cmd, ev)
		b.note(cmd, ev)
	}},
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
	trackerRegister: {spec: wsxmlcmd.Spec{Kind: wsxmlcmd.Empty, Attrs: []string{"store", "member", "remote", "label", "org", "family", "given"}}},
	trackerSync:     {spec: wsxmlcmd.Spec{Kind: wsxmlcmd.Empty, Attrs: []string{"store"}}},
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
	b.optional(cmd, ev, "due")
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
	b.stage(cmd, ev, "without stage the item starts in todo")
	b.list(cmd, ev, "mentions")
	b.itemRefList(cmd, ev, "refs")
	b.text(cmd, ev, "body", false)
}

// buildItemEdit writes only the fields the command changes: an attribute
// present sets its field, a non-empty body sets the body, and clear names
// fields to empty (body) or remove (due, and stage, which moves the item to
// todo; written as null).
func buildItemEdit(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.itemRef(cmd, ev, "item")
	if v, ok := cmd.Attr("title"); ok && wsxmlcmd.NonEmpty(b.p, cmd.At("title"), v) {
		ev.payload["title"] = v
	}
	if v, ok := cmd.Attr("project"); ok {
		ev.payload["project"] = v
	}
	if v, ok := cmd.Attr("due"); ok && wsxmlcmd.NonEmpty(b.p, cmd.At("due")+" (clear=\"due\" removes it)", v) {
		ev.payload["due"] = v
	}
	if n, ok := trackerLevelAttr(b.p, cmd, "priority", false); ok {
		ev.payload["priority"] = n
	}
	b.stage(cmd, ev, `clear="stage" moves the item to todo`)
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
			case "stage":
				if _, set := cmd.Attr("stage"); set {
					b.p.Add("%s: clear=\"stage\" with a stage attribute; send one or the other", cmd.Label())
				}
				ev.payload["stage"] = nil
			default:
				b.p.Add("%s: must list body, due, or stage; got %q", cmd.At("clear"), field)
			}
		}
	}
	changed := len(ev.payload) + len(ev.refLists)
	if _, ok := ev.payload["item"]; ok {
		changed--
	}
	if changed == 0 {
		b.p.Add("%s: changes nothing; set an attribute, a body, or clear", cmd.Label())
	}
}

func buildItemOnly(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.itemRef(cmd, ev, "item")
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
type trackerEvent struct {
	cmd     wsxmlcmd.Command
	typ     string
	origin  string
	payload map[string]any
	// refs maps a payload field to the event index (in the batch) of the
	// event it names.
	refs map[string]int
	// refLists maps a list field to its entries in order: a stored id, or
	// the event index of the event it names.
	refLists map[string][]trackerListRef
	// selfAssign marks the creator self-assignment the writer adds right
	// after the item.create of cmd (writerDefaults); it has no command.
	selfAssign bool
}

// trackerListRef is one entry of an ID list: id when given, else the batch
// event index event.
type trackerListRef struct {
	id    string
	event int
}

// trackerBatch is one tracker.update batch as planned.
type trackerBatch struct {
	p    *wsxmlcmd.Problems
	cmds []wsxmlcmd.Command
	// events holds the event commands in order; events[i] is event i of the
	// Dispatch call, so a *wstracker.Refusal's Index names events[i].cmd.
	events []*trackerEvent
	// byPos maps a command position to its command (an unknown command
	// takes a position but is not parsed), eventOf to its event index.
	byPos   map[int]wsxmlcmd.Command
	eventOf map[int]int
	store   string
	// network is the register or sync command, when the batch is one.
	network *wsxmlcmd.Command
}

func (b *trackerBatch) required(cmd wsxmlcmd.Command, ev *trackerEvent, name string) {
	if v, ok := wsxmlcmd.RequiredAttr(b.p, cmd, name); ok {
		ev.payload[name] = v
	}
}

func (b *trackerBatch) optional(cmd wsxmlcmd.Command, ev *trackerEvent, name string) {
	if v, ok := cmd.Attr(name); ok && wsxmlcmd.NonEmpty(b.p, cmd.At(name), v) {
		ev.payload[name] = v
	}
}

// stage reads the optional stage attribute, whose one value is review;
// other says what the item gets without it.
func (b *trackerBatch) stage(cmd wsxmlcmd.Command, ev *trackerEvent, other string) {
	v, ok := cmd.Attr("stage")
	if !ok {
		return
	}
	if strings.TrimSpace(v) != "review" {
		b.p.Add("%s: must be review (%s); got %q", cmd.At("stage"), other, v)
		return
	}
	ev.payload["stage"] = "review"
}

func (b *trackerBatch) list(cmd wsxmlcmd.Command, ev *trackerEvent, name string) {
	if v, ok := cmd.Attr(name); ok {
		ev.payload[name] = strings.Fields(v)
	}
}

func (b *trackerBatch) note(cmd wsxmlcmd.Command, ev *trackerEvent) {
	if cmd.Body != "" {
		ev.payload["note"] = cmd.Body
	}
}

func (b *trackerBatch) text(cmd wsxmlcmd.Command, ev *trackerEvent, field string, required bool) {
	if required && !wsxmlcmd.NonEmpty(b.p, cmd.Label()+" body", cmd.Body) {
		return
	}
	if cmd.Body != "" {
		ev.payload[field] = cmd.Body
	}
}

func (b *trackerBatch) itemRef(cmd wsxmlcmd.Command, ev *trackerEvent, name string) {
	b.ref(cmd, ev, name, wstracker.TypeItemCreate, "item")
}

// itemRefList reads an optional space-separated list of item IDs, each in
// either form or $N; an empty list writes an empty field.
func (b *trackerBatch) itemRefList(cmd wsxmlcmd.Command, ev *trackerEvent, name string) {
	v, ok := cmd.Attr(name)
	if !ok {
		return
	}
	list := []trackerListRef{}
	for _, tok := range strings.Fields(v) {
		if id, idx, ok := b.parseRef(cmd, name, tok, wstracker.TypeItemCreate, "item"); ok {
			list = append(list, trackerListRef{id: id, event: idx})
		}
	}
	ev.refLists[name] = list
}

func (b *trackerBatch) commentRef(cmd wsxmlcmd.Command, ev *trackerEvent, name string) {
	b.ref(cmd, ev, name, wstracker.TypeCommentAdd, "comment")
}

// ref reads an attribute naming an item or comment: an event id in its
// stored form (maya.pc#5) or the form the query prints (maya.pc-5), or $N
// for what an earlier command N of the batch, of type creator, creates.
func (b *trackerBatch) ref(cmd wsxmlcmd.Command, ev *trackerEvent, name, creator, noun string) {
	v, ok := wsxmlcmd.RequiredAttr(b.p, cmd, name)
	if !ok {
		return
	}
	id, idx, ok := b.parseRef(cmd, name, strings.TrimSpace(v), creator, noun)
	switch {
	case !ok:
	case idx >= 0:
		ev.refs[name] = idx
	default:
		ev.payload[name] = id
	}
}

// parseRef reads one reference v of attribute name: the stored id, or for
// $N the event index of the event command N creates (idx -1 otherwise).
func (b *trackerBatch) parseRef(cmd wsxmlcmd.Command, name, v, creator, noun string) (id string, idx int, ok bool) {
	if rest, isRef := strings.CutPrefix(v, "$"); isRef {
		n, err := strconv.Atoi(rest)
		if err != nil || n < 1 || n >= cmd.Pos {
			b.p.Add("%s: $N names an earlier command of this batch by position; got %q", cmd.At(name), v)
			return "", -1, false
		}
		target, known := b.byPos[n]
		idx, isEvent := b.eventOf[n]
		if !known || !isEvent || strings.TrimPrefix(target.Tag, trackerPrefix) != creator {
			b.p.Add("%s: %s names command %d, which creates no %s; $N must name a <%s%s>", cmd.At(name), v, n, noun, trackerPrefix, creator)
			return "", -1, false
		}
		return "", idx, true
	}
	id, err := parseTrackerID(v)
	if err != nil {
		b.p.Add("%s: must be an %s ID such as maya.pc-5, or $N; got %q", cmd.At(name), noun, v)
		return "", -1, false
	}
	return id, -1, true
}

// parseTrackerID returns the stored form of an event id given in either
// form: maya.pc#5, or the display alias maya.pc-5 (the seq follows the last
// '-', since a member or clone id may itself hold '-').
func parseTrackerID(s string) (string, error) {
	if !strings.Contains(s, "#") {
		dash := strings.LastIndexByte(s, '-')
		if dash < 0 {
			return "", errors.New("no seq")
		}
		s = s[:dash] + "#" + s[dash+1:]
	}
	id, err := wsledger.ParseEventID(s)
	if err != nil || id.Broken != "" || id.Seq < 1 {
		return "", errors.New("not an event id")
	}
	return id.String(), nil
}

// trackerCount is n with noun, plural unless n is 1.
func trackerCount(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// trackerAlias is an event id's display form, maya.pc#5 -> maya.pc-5.
func trackerAlias(id string) string { return strings.Replace(id, "#", "-", 1) }

// planTrackerBatch parses commands and validates the batch against the
// registered stores, recording every problem in p. A batch with a problem
// writes nothing.
func planTrackerBatch(p *wsxmlcmd.Problems, commands string, registered []wstrackerhost.StoreEntry) *trackerBatch {
	b := &trackerBatch{p: p, byPos: map[int]wsxmlcmd.Command{}, eventOf: map[int]int{}}
	b.cmds = wsxmlcmd.Parse(p, trackerGrammar, commands)
	if len(b.cmds) == 0 && len(p.List) == 0 {
		p.Add("commands: no %s command given", trackerPrefix)
	}
	known := map[string]bool{}
	ids := make([]string, 0, len(registered))
	for _, e := range registered {
		known[e.ID] = true
		ids = append(ids, e.ID)
	}
	unknownStore := func(at, store string) {
		if len(ids) == 0 {
			p.Add("%s: no store is registered; register one with <%s%s>", at, trackerPrefix, trackerRegister)
			return
		}
		p.Add("%s: %q is not a registered store (registered: %s)", at, store, strings.Join(ids, ", "))
	}
	// Number the event commands first, so a $N can name a later-parsed
	// command's event index while it is planned.
	for _, cmd := range b.cmds {
		b.byPos[cmd.Pos] = cmd
		name := strings.TrimPrefix(cmd.Tag, trackerPrefix)
		if trackerCommands[name].build != nil {
			b.eventOf[cmd.Pos] = len(b.eventOf)
		}
	}
	groups := map[*wsxmlcmd.Scope]bool{}
	for i := range b.cmds {
		cmd := b.cmds[i]
		name := strings.TrimPrefix(cmd.Tag, trackerPrefix)
		def := trackerCommands[name]
		if def.build == nil {
			if len(b.cmds) > 1 {
				p.Add("%s: reaches the network and cannot be rolled back, so it stands alone in its batch", cmd.Label())
			}
			b.network = &b.cmds[i]
			switch name {
			case trackerRegister:
				if store, ok := wsxmlcmd.RequiredAttr(p, cmd, "store"); ok && known[store] {
					p.Add("%s: %q is already registered on this machine", cmd.At("store"), store)
				}
				wsxmlcmd.RequiredAttr(p, cmd, "member")
			case trackerSync:
				if store, ok := cmd.Attr("store"); ok {
					if !known[store] {
						unknownStore(cmd.At("store"), store)
					}
				} else if len(ids) == 0 {
					p.Add("%s: no store is registered", cmd.Label())
				}
			}
			continue
		}
		ev := &trackerEvent{cmd: cmd, typ: name, origin: wstracker.OriginAgent, payload: map[string]any{}, refs: map[string]int{}, refLists: map[string][]trackerListRef{}}
		// The parser has refused a command outside a group and a group
		// with no name; each group is checked once, at its first command.
		if scope := cmd.Scope; scope != nil && !groups[scope] {
			switch {
			case len(groups) > 0:
				p.Add("%s: a second <%sstore> group; one batch writes to one store, so it holds one group", scope.Label(), trackerPrefix)
			case scope.Value == "":
			case !known[scope.Value]:
				unknownStore(scope.At(), scope.Value)
			default:
				b.store = scope.Value
			}
			groups[scope] = true
		}
		if _, ok := cmd.Attr("origin"); ok {
			if v, ok := wsxmlcmd.EnumAttr(p, cmd, "origin", trackerOrigins); ok {
				ev.origin = v
			}
		}
		def.build(b, cmd, ev)
		b.events = append(b.events, ev)
	}
	for _, e := range registered {
		if e.ID == b.store && e.Member != "" {
			b.writerDefaults(e.Member)
		}
	}
	return b
}

// writerDefaults fills what the writer, member writer of the batch's
// store, decides for an agent as the dashboard does for a person. An
// item.assign without completion gets it stored explicitly: assignee when
// the writer is its only target, else assigner (a rule or levels always
// covers someone else, since the store refuses one that covers nobody); the
// fold's absent-means-assignee rule stays for old events. An item created
// outside review (a topic too, which the writer then holds) that no
// item.assign of the batch names by $N is self-assigned (people = the writer, assignee closes) by an item.assign
// inserted right after its create, so a later command of the batch acts on
// an item already assigned, as on the dashboard; every event index is
// renumbered to match.
func (b *trackerBatch) writerDefaults(writer string) {
	assigned := map[int]bool{}
	for _, ev := range b.events {
		if ev.typ != wstracker.TypeItemAssign {
			continue
		}
		if idx, ok := ev.refs["item"]; ok {
			assigned[idx] = true
		}
		if _, ok := ev.payload["completion"]; !ok {
			ev.payload["completion"] = "assigner"
			if people, ok := ev.payload["people"].([]string); ok && ev.payload["mode"] == "chosen" && len(people) > 0 {
				self := true
				for _, person := range people {
					self = self && person == writer
				}
				if self {
					ev.payload["completion"] = "assignee"
				}
			}
		}
	}
	events := make([]*trackerEvent, 0, len(b.events))
	moved := make([]int, len(b.events))
	for i, ev := range b.events {
		moved[i] = len(events)
		events = append(events, ev)
		if ev.typ != wstracker.TypeItemCreate || ev.payload["stage"] == "review" || assigned[i] {
			continue
		}
		events = append(events, &trackerEvent{
			cmd: ev.cmd, typ: wstracker.TypeItemAssign, origin: ev.origin, selfAssign: true,
			payload:  map[string]any{"mode": "chosen", "people": []string{writer}, "completion": "assignee"},
			refs:     map[string]int{"item": i},
			refLists: map[string][]trackerListRef{},
		})
	}
	if len(events) == len(b.events) {
		return
	}
	for _, ev := range events {
		for field, idx := range ev.refs {
			ev.refs[field] = moved[idx]
		}
		for _, list := range ev.refLists {
			for j := range list {
				if list[j].id == "" {
					list[j].event = moved[list[j].event]
				}
			}
		}
	}
	for pos, idx := range b.eventOf {
		b.eventOf[pos] = moved[idx]
	}
	b.events = events
}

// newEvents builds the batch's events for a store whose next event gets
// next, resolving each $N to the id its event gets.
func (b *trackerBatch) newEvents(next wsledger.EventID) ([]wsledger.NewEvent, error) {
	out := make([]wsledger.NewEvent, len(b.events))
	for i, ev := range b.events {
		payload := make(map[string]any, len(ev.payload)+len(ev.refs)+len(ev.refLists))
		for k, v := range ev.payload {
			payload[k] = v
		}
		at := func(idx int) string {
			id := next
			id.Seq += idx
			return id.String()
		}
		for field, idx := range ev.refs {
			payload[field] = at(idx)
		}
		for field, list := range ev.refLists {
			ids := make([]string, len(list))
			for i, r := range list {
				ids[i] = r.id
				if r.id == "" {
					ids[i] = at(r.event)
				}
			}
			payload[field] = ids
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		out[i] = wsledger.NewEvent{Type: ev.typ, Origin: ev.origin, Payload: raw}
	}
	return out, nil
}

func (s *Server) handleTrackerUpdate(ctx context.Context, id json.RawMessage, args map[string]any) response {
	const tool = "tracker.update"
	key, support, err := s.trackerSessionKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	commands := ""
	if raw, present := args["commands"]; present {
		str, ok := raw.(string)
		if !ok {
			return toolErrorTextResponse(id, s.trackerRefusal(tool, []string{
				"commands: must be a string of " + trackerPrefix + " commands, not a JSON value",
			}))
		}
		commands = str
	}
	h, err := s.trackerHost()
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	registered, err := h.Stores()
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	var problems wsxmlcmd.Problems
	b := planTrackerBatch(&problems, commands, registered)
	if support != nil {
		supportPinProblems(&problems, b, support.store)
	}
	if len(problems.List) > 0 {
		return toolErrorTextResponse(id, s.trackerRefusal(tool, problems.List))
	}
	var text string
	switch {
	case b.network != nil && strings.TrimPrefix(b.network.Tag, trackerPrefix) == trackerRegister:
		text, err = trackerRegisterCommand(ctx, h, *b.network)
	case b.network != nil:
		text, err = trackerSyncCommand(ctx, h, *b.network, registered)
	default:
		var ids []wsledger.EventID
		ids, err = h.DispatchWith(ctx, b.store, b.newEvents)
		var refusal *wstracker.Refusal
		if errors.As(err, &refusal) && refusal.Index >= 0 && refusal.Index < len(b.events) {
			ev := b.events[refusal.Index]
			what := "this write"
			if ev.selfAssign {
				what = "the self-assignment written with this create"
			}
			return toolErrorTextResponse(id, s.trackerRefusal(tool, []string{
				fmt.Sprintf("%s: the store would flag %s: %s", ev.cmd.Label(), what, refusal.Reason),
			}))
		}
		if err == nil {
			var sb strings.Builder
			fmt.Fprintf(&sb, "%s: wrote %s\n", b.store, trackerCount(len(ids), "event"))
			for i, eventID := range ids {
				switch {
				case i >= len(b.events):
				case b.events[i].selfAssign:
					fmt.Fprintf(&sb, "%d %s%s %s (self-assignment)\n", b.events[i].cmd.Pos, trackerPrefix, wstracker.TypeItemAssign, trackerAlias(eventID.String()))
				default:
					fmt.Fprintf(&sb, "%d %s %s\n", b.events[i].cmd.Pos, b.events[i].cmd.Tag, trackerAlias(eventID.String()))
				}
			}
			text = sb.String()
		}
	}
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %s", tool, wstrackerhost.RedactCredentials(err.Error())))
	}
	return toolTextResponse(id, s.trackerSucceeded(key, text), nil)
}

// trackerSucceeded turns the session's tracker knob on after a successful
// tracker call and appends a warning to text when that write failed.
func (s *Server) trackerSucceeded(key, text string) string {
	if err := s.enableTrackerKnobForSession(key); err != nil {
		text = strings.TrimRight(text, "\n") + "\nwarning: " + err.Error() + "\n"
	}
	return text
}

// trackerRegisterCommand registers a store from a tk:register command; a
// failure leaves nothing registered on this machine, and says so, and what
// the remote already holds when the failure came after the push.
func trackerRegisterCommand(ctx context.Context, h *wstrackerhost.Host, cmd wsxmlcmd.Command) (string, error) {
	req := wstrackerhost.RegisterRequest{Origin: wstracker.OriginAgent}
	req.ID, _ = cmd.Attr("store")
	req.Member, _ = cmd.Attr("member")
	req.Remote, _ = cmd.Attr("remote")
	req.Label, _ = cmd.Attr("label")
	if given, ok := cmd.Attr("given"); ok {
		org, _ := cmd.Attr("org")
		family, _ := cmd.Attr("family")
		req.Profile = &wstracker.Profile{Org: org, Family: family, Given: given}
	} else if _, hasOrg := cmd.Attr("org"); hasOrg {
		return "", fmt.Errorf("%s: org and family need given; send all three or none (none uses the profile used last)", cmd.Label())
	} else if _, hasFamily := cmd.Attr("family"); hasFamily {
		return "", fmt.Errorf("%s: org and family need given; send all three or none (none uses the profile used last)", cmd.Label())
	}
	ctx, cancel := context.WithTimeout(ctx, trackerRegisterTimeout)
	defer cancel()
	res, err := h.Register(ctx, req)
	if errors.Is(err, wsledger.ErrIDTaken) {
		return "", fmt.Errorf("%s: member %q is taken in that store; register under another member id. Nothing was registered", cmd.Label(), req.Member)
	}
	var held *wstrackerhost.RemoteHeldError
	if errors.As(err, &held) {
		what := fmt.Sprintf("member %s's join", req.Member)
		if held.Creator {
			what = fmt.Sprintf("the new store, with member %s as its sole admin", req.Member)
		}
		return "", fmt.Errorf("%s: %w. Nothing was registered on this machine, but the remote already holds %s; send the same tk:register again to finish (this machine's key is already that member, so no new join is written)", cmd.Label(), err, what)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w. Nothing was registered", cmd.Label(), err)
	}
	how := "joined"
	if res.Creator {
		how = "created"
	}
	where := "no remote"
	if res.Entry.Remote != "" {
		where = "remote " + wstrackerhost.RedactCredentials(res.Entry.Remote)
	}
	return fmt.Sprintf("registered %s: %s, member %s, %s\nprofile: %s\n", res.Entry.ID, how, res.Entry.Member, where, formatTrackerProfile(res.Profile)), nil
}

// trackerSyncCommand runs the apply path (push, then fetch) for the named
// store or every store; any store that fails fails the call, after the
// others ran.
func trackerSyncCommand(ctx context.Context, h *wstrackerhost.Host, cmd wsxmlcmd.Command, registered []wstrackerhost.StoreEntry) (string, error) {
	var targets []string
	if store, ok := cmd.Attr("store"); ok {
		targets = []string{store}
	} else {
		for _, e := range registered {
			targets = append(targets, e.ID)
		}
	}
	var sb strings.Builder
	failed := 0
	for _, store := range targets {
		rep, err := h.Sync(ctx, store)
		if err != nil {
			failed++
			fmt.Fprintf(&sb, "%s: failed: %s\n", store, wstrackerhost.RedactCredentials(err.Error()))
			continue
		}
		fmt.Fprintf(&sb, "%s: %s\n", store, formatTrackerSyncReport(rep))
	}
	if failed > 0 {
		return "", fmt.Errorf("%s: %d of %d stores failed to sync; nothing was lost, and the unpushed writes push on the next sync\n%s", cmd.Label(), failed, len(targets), strings.TrimRight(sb.String(), "\n"))
	}
	return sb.String(), nil
}

func formatTrackerSyncReport(rep wsledger.SyncReport) string {
	parts := []string{}
	if rep.Pushed {
		parts = append(parts, "pushed")
	} else {
		parts = append(parts, "nothing pushed")
	}
	parts = append(parts, trackerCount(rep.Received, "event")+" received")
	if rep.Pending.Count > 0 {
		parts = append(parts, trackerCount(rep.Pending.Count, "write")+" still unpushed")
	}
	if rep.Pending.KeyNotRegistered {
		parts = append(parts, "this machine's key is not registered in the store yet, so its writes wait for an admin's roles.add_key")
	}
	if rep.Pending.IDTaken {
		parts = append(parts, "this machine's member id was taken by another join; register under another member id")
	}
	if rep.Pending.ForeignAdvance {
		parts = append(parts, "the remote holds writes under this clone that it did not make, so it cannot push")
	}
	for _, w := range rep.Warnings {
		// A wiki sync warning carries git's error text, which may name the
		// remote with its credentials.
		parts = append(parts, fmt.Sprintf("warning: %s %s %s", w.Ref, w.Kind, wstrackerhost.RedactCredentials(w.Detail)))
	}
	return strings.Join(parts, ", ")
}

// trackerQuery is one parsed tracker.query call.
type trackerQuery struct {
	store, scope, item          string
	kind, status, project, text string
	// q is the payload's search text, parsed into search by the
	// dashboard search box's grammar (wstracker.ParseQuery).
	q      string
	search wstracker.Query
	json   bool
}

var (
	trackerScopes = []string{"mine", "received", "assigned", "team", "all"}
	// todo is not a filter value: the store folds a todo create as a task.
	trackerKinds = wstracker.QueryKinds
	// trackerStatuses are the status filter's values, shared with the
	// search grammar's status: key: open (not done) and hidden, then an
	// item's derived statuses.
	trackerStatuses = wstracker.QueryStatuses
)

// parseTrackerQuery reads the payload object and format argument.
func parseTrackerQuery(rawPayload, rawFormat any) (trackerQuery, []string) {
	var q trackerQuery
	var problems []string
	add := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }
	if rawFormat != nil {
		switch rawFormat {
		case "json":
			q.json = true
		case "", "text":
		default:
			add("format: must be \"json\" or omitted; got %v", rawFormat)
		}
	}
	payload := map[string]any{}
	if rawPayload != nil {
		m, ok := rawPayload.(map[string]any)
		if !ok {
			add("payload: must be an object")
		}
		payload = m
	}
	fields := map[string]*string{"store": &q.store, "scope": &q.scope, "item": &q.item, "kind": &q.kind, "status": &q.status, "project": &q.project, "text": &q.text, "q": &q.q}
	names := make([]string, 0, len(payload))
	for name := range payload {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dst, known := fields[name]
		if !known {
			add("payload.%s: unknown (known: store, scope, item, kind, status, project, text, q)", name)
			continue
		}
		v, ok := payload[name].(string)
		if !ok {
			add("payload.%s: must be a string", name)
			continue
		}
		*dst = strings.TrimSpace(v)
	}
	enum := func(name, v string, values []string) {
		if v == "" {
			return
		}
		for _, allowed := range values {
			if v == allowed {
				return
			}
		}
		add("payload.%s: must be one of %s; got %q", name, strings.Join(values, ", "), v)
	}
	enum("scope", q.scope, trackerScopes)
	enum("kind", q.kind, trackerKinds)
	enum("status", q.status, trackerStatuses)
	if search, err := wstracker.ParseQuery(q.q); err != nil {
		add("payload.q: %v", err)
	} else {
		q.search = search
	}
	narrowed := q.scope != "" || q.item != "" || q.filtered()
	switch {
	case q.store == "" && narrowed:
		add("payload: scope, item, and the filters need store")
	case q.item != "" && (q.scope != "" || q.filtered()):
		add("payload.item: shows one item; drop scope and the filters")
	case q.json && narrowed:
		add("format: json returns the store's whole view state; drop scope, item, and the filters")
	}
	if q.item != "" {
		id, err := parseTrackerID(q.item)
		if err != nil {
			add("payload.item: must be an item ID such as maya.pc-5; got %q", q.item)
		}
		q.item = id
	}
	if q.scope == "" {
		q.scope = "mine"
	}
	return q, problems
}

// filtered reports a query with any item filter: kind, status, project,
// text, or q.
func (q trackerQuery) filtered() bool {
	return q.kind != "" || q.status != "" || q.project != "" || q.text != "" || q.q != ""
}

// trackerStoreSummary is one row of the store list.
type trackerStoreSummary struct {
	Entry wstrackerhost.StoreEntry
	// View is nil when the store could not be read; Err says why.
	View *wstrackerhost.StoreView
	Err  error
}

func (s *Server) handleTrackerQuery(ctx context.Context, id json.RawMessage, args map[string]any) response {
	const tool = "tracker.query"
	key, _, err := s.trackerSessionKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	q, problems := parseTrackerQuery(args["payload"], args["format"])
	if len(problems) > 0 {
		return toolErrorTextResponse(id, s.trackerRefusal(tool, problems))
	}
	h, err := s.trackerHost()
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	registered, err := h.Stores()
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	if q.store == "" {
		profiles, err := h.Profiles()
		if err != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
		}
		summaries := make([]trackerStoreSummary, 0, len(registered))
		for _, e := range registered {
			sum := trackerStoreSummary{Entry: e}
			if sum.Err = h.PreRead(ctx, e.ID); sum.Err == nil {
				sum.View, sum.Err = h.View(ctx, e.ID, 0)
			}
			summaries = append(summaries, sum)
		}
		if q.json {
			resp := toolJSONResponse(id, trackerStoreListJSON(summaries, profiles), nil)
			// JSON output carries no warning line; the knob write is best effort.
			_ = s.trackerSucceeded(key, "")
			return resp
		}
		return toolTextResponse(id, s.trackerSucceeded(key, formatTrackerStoreList(summaries, profiles)), nil)
	}
	found := false
	for _, e := range registered {
		found = found || e.ID == q.store
	}
	if !found {
		ids := make([]string, 0, len(registered))
		for _, e := range registered {
			ids = append(ids, e.ID)
		}
		reason := "no store is registered"
		if len(ids) > 0 {
			reason = "registered: " + strings.Join(ids, ", ")
		}
		return toolErrorTextResponse(id, s.trackerRefusal(tool, []string{fmt.Sprintf("payload.store: %q is not a registered store (%s)", q.store, reason)}))
	}
	if err := h.PreRead(ctx, q.store); err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %s", tool, wstrackerhost.RedactCredentials(err.Error())))
	}
	v, err := h.View(ctx, q.store, 0)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %s", tool, wstrackerhost.RedactCredentials(err.Error())))
	}
	v.Redact()
	switch {
	case q.json:
		resp := toolJSONResponse(id, v, nil)
		_ = s.trackerSucceeded(key, "") // as for the JSON store list
		return resp
	case q.item != "":
		if _, ok := v.Items[q.item]; !ok {
			return toolErrorTextResponse(id, s.trackerRefusal(tool, []string{fmt.Sprintf("payload.item: %s is not an item of %s", trackerAlias(q.item), q.store)}))
		}
		return toolTextResponse(id, s.trackerSucceeded(key, formatTrackerItem(v, q.item)), nil)
	default:
		return toolTextResponse(id, s.trackerSucceeded(key, formatTrackerScope(v, q)), nil)
	}
}

// trackerRefusal lists every problem, then the tool's input grammar from
// the tracker playbook, so one retry can fix the call: for tracker.update
// the shared command syntax and the "## Commands" section (commandRefusal),
// for tracker.query the "## Query" section.
func (s *Server) trackerRefusal(tool string, problems []string) string {
	if tool != "tracker.query" {
		return commandRefusal(tool, problems, s.trackerPlaybookSection("## Commands"), trackerPlaybookName)
	}
	var sb strings.Builder
	writeRefusalProblems(&sb, tool, problems)
	if grammar := s.trackerPlaybookSection("## Query"); grammar != "" {
		sb.WriteString("\n" + grammar + "\n")
	} else {
		fmt.Fprintf(&sb, "\nRead the %s playbook with %s/playbook.read for the input format.\n", trackerPlaybookName, RuntimeNamespace())
	}
	return sb.String()
}

// trackerPlaybookSection returns the tracker playbook's section under
// heading; "" when it cannot be read.
func (s *Server) trackerPlaybookSection(heading string) string {
	rsrcRoot, err := resolveRsrcRoot("")
	if err != nil {
		return ""
	}
	body, _, err := renderPlaybookBody(s, rsrcRoot, trackerPlaybookName, nil, wsconfig.Options{}, "", "", "", nil, "")
	if err != nil {
		return ""
	}
	return markdownSection(body, heading)
}

// trackerManualSection is the workflow manual's knob-on tracker section:
// the tracker playbook's content under "### Tracker" (its title dropped,
// its headings demoted under the section), so the lead needs no extra read.
func trackerManualSection(s *Server, rsrcRoot string) (string, error) {
	playbook, _, err := renderPlaybookBody(s, rsrcRoot, trackerPlaybookName, nil, wsconfig.Options{}, "", "", "", nil, "")
	if err != nil {
		return "", fmt.Errorf("render %s playbook: %w", trackerPlaybookName, err)
	}
	var sb strings.Builder
	sb.WriteString("### Tracker\n\n")
	inFence := false
	for _, line := range strings.Split(strings.TrimSpace(playbook), "\n") {
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
		}
		if !inFence {
			if strings.HasPrefix(line, "# ") {
				continue
			}
			if strings.HasPrefix(line, "## ") {
				line = "##" + line
			}
		}
		sb.WriteString(line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

// trackerStoreListJSON is the store list's format json shape.
func trackerStoreListJSON(summaries []trackerStoreSummary, profiles wstrackerhost.SavedProfiles) map[string]any {
	stores := make([]map[string]any, 0, len(summaries))
	for _, sum := range summaries {
		e := sum.Entry
		row := map[string]any{"id": e.ID, "label": e.DisplayLabel(), "member": e.Member, "remote": nil}
		if e.Remote != "" {
			row["remote"] = wstrackerhost.RedactCredentials(e.Remote)
		}
		if sum.View != nil {
			sum.View.Redact()
			row["sync"] = sum.View.Sync
			row["today"] = sum.View.My.Today
		} else if sum.Err != nil {
			row["error"] = wstrackerhost.RedactCredentials(sum.Err.Error())
		}
		stores = append(stores, row)
	}
	return map[string]any{"stores": stores, "profiles": profiles}
}

// formatTrackerStoreList is the store list's text: one block per store
// (its id and label, member, remote and sync state, and the user's open
// work counts), then the saved profiles, the one used last first.
func formatTrackerStoreList(summaries []trackerStoreSummary, profiles wstrackerhost.SavedProfiles) string {
	var sb strings.Builder
	if len(summaries) == 0 {
		sb.WriteString("stores: none registered\n")
	} else {
		sb.WriteString("stores:\n")
	}
	for _, sum := range summaries {
		e := sum.Entry
		fmt.Fprintf(&sb, "- %s", e.ID)
		if e.Label != "" {
			fmt.Fprintf(&sb, " (%s)", e.Label)
		}
		fmt.Fprintf(&sb, ", member %s", e.Member)
		if sum.View == nil {
			fmt.Fprintf(&sb, ", unreadable: %s\n", wstrackerhost.RedactCredentials(fmt.Sprint(sum.Err)))
			continue
		}
		sum.View.Redact()
		fmt.Fprintf(&sb, ", %s\n", formatTrackerSyncLine(sum.View))
		t := sum.View.My.Today
		fmt.Fprintf(&sb, "  my work %d, to answer %d, to confirm %d\n", len(t.Work), len(t.Pending), len(t.Confirms))
	}
	if len(profiles.Profiles) == 0 {
		sb.WriteString("profiles: none saved\n")
		return sb.String()
	}
	sb.WriteString("profiles:\n")
	if last, ok := profiles.LastProfile(); ok {
		fmt.Fprintf(&sb, "- %s (used last)\n", formatTrackerProfile(last))
	}
	for i, p := range profiles.Profiles {
		if i != profiles.Last {
			fmt.Fprintf(&sb, "- %s\n", formatTrackerProfile(p))
		}
	}
	return sb.String()
}

// formatTrackerSyncLine is a store's remote and sync state on one line.
func formatTrackerSyncLine(v *wstrackerhost.StoreView) string {
	if v.Store.Remote == nil {
		// A store with no remote has nowhere to push, so no unpushed count.
		return "no remote"
	}
	parts := []string{"remote " + *v.Store.Remote, "offline"}
	if v.Sync.Online {
		parts[1] = "online"
	}
	if v.Sync.SyncedAt != nil {
		parts = append(parts, "synced "+*v.Sync.SyncedAt)
	} else {
		parts = append(parts, "never synced")
	}
	if v.Sync.Error != "" {
		parts = append(parts, "last error: "+v.Sync.Error)
	}
	unpushed := 0
	for _, c := range v.Sync.Unpushed {
		unpushed += len(c.Events)
	}
	if unpushed > 0 {
		parts = append(parts, fmt.Sprintf("unpushed writes %d", unpushed))
	}
	return strings.Join(parts, ", ")
}

// formatTrackerProfile is a profile as one line: org, then the fields.
func formatTrackerProfile(p wstracker.Profile) string {
	s := fmt.Sprintf("given=%q", p.Given)
	if p.Family != "" {
		s = fmt.Sprintf("family=%q ", p.Family) + s
	}
	if p.Org != "" {
		s = fmt.Sprintf("org=%q ", p.Org) + s
	}
	return s
}
