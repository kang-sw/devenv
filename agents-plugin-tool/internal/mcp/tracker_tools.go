package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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
// playbook.read all quote.

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
	return fmt.Sprintf("%s is not available to a spawned session: the tracker records the user's work through the top-level lead. Put progress in your final report; your lead records it.", name)
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
var trackerEventAttrs = []string{"store", "origin"}

func trackerEventSpec(kind wsxmlcmd.Kind, attrs ...string) wsxmlcmd.Spec {
	return wsxmlcmd.Spec{Kind: kind, Attrs: append(append([]string{}, trackerEventAttrs...), attrs...)}
}

// trackerCommands is the command table, keyed by name without the prefix:
// each event type of the tracker's write schema under its own name, plus
// register and sync. A body is the event's text field: the item body, the
// comment, or the note.
var trackerCommands = map[string]trackerCommandDef{
	wstracker.TypeItemCreate: {trackerEventSpec(wsxmlcmd.Leaf, "kind", "title", "project", "due", "priority", "parent", "mentions"), buildItemCreate},
	wstracker.TypeItemEdit:   {trackerEventSpec(wsxmlcmd.Leaf, "item", "title", "project", "due", "priority", "mentions", "clear"), buildItemEdit},
	wstracker.TypeItemHide:   {trackerEventSpec(wsxmlcmd.Empty, "item"), buildItemOnly},
	wstracker.TypeItemAssign: {trackerEventSpec(wsxmlcmd.Empty, "item", "people", "levels", "completion"), buildItemAssign},
	wstracker.TypeItemHandoff: {trackerEventSpec(wsxmlcmd.Leaf, "item", "to", "assignee"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		b.itemRef(cmd, ev, "item")
		b.required(cmd, ev, "to")
		b.optional(cmd, ev, "assignee")
		b.note(cmd, ev)
	}},
	wstracker.TypeItemAccept:   {trackerEventSpec(wsxmlcmd.Leaf, "item"), buildItemNote},
	wstracker.TypeItemDecline:  {trackerEventSpec(wsxmlcmd.Leaf, "item"), buildItemNote},
	wstracker.TypeItemReturn:   {trackerEventSpec(wsxmlcmd.Leaf, "item", "assignee"), buildItemAssigneeNote},
	wstracker.TypeItemUnassign: {trackerEventSpec(wsxmlcmd.Empty, "item", "assignee"), buildItemOptionalAssignee},
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
	wstracker.TypeCommentAdd: {trackerEventSpec(wsxmlcmd.Leaf, "item", "reply_to", "mentions"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		b.itemRef(cmd, ev, "item")
		if _, ok := cmd.Attr("reply_to"); ok {
			b.commentRef(cmd, ev, "reply_to")
		}
		b.list(cmd, ev, "mentions")
		b.text(cmd, ev, "body", true)
	}},
	wstracker.TypeCommentEdit: {trackerEventSpec(wsxmlcmd.Leaf, "comment", "mentions"), func(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
		b.commentRef(cmd, ev, "comment")
		b.list(cmd, ev, "mentions")
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

var trackerGrammar = func() wsxmlcmd.Grammar {
	g := wsxmlcmd.Grammar{Prefix: trackerPrefix, Commands: map[string]wsxmlcmd.Spec{}}
	for name, def := range trackerCommands {
		g.Commands[name] = def.spec
	}
	return g
}()

func buildItemCreate(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
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
	b.list(cmd, ev, "mentions")
	b.text(cmd, ev, "body", false)
}

// buildItemEdit writes only the fields the command changes: an attribute
// present sets its field, a non-empty body sets the body, and clear names
// fields to empty (body) or remove (due, written as null).
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
	b.list(cmd, ev, "mentions")
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
			default:
				b.p.Add("%s: must list body or due; got %q", cmd.At("clear"), field)
			}
		}
	}
	changed := len(ev.payload)
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

// buildItemAssign maps people onto mode chosen and levels onto mode all.
func buildItemAssign(b *trackerBatch, cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.itemRef(cmd, ev, "item")
	people, hasPeople := cmd.Attr("people")
	_, hasLevels := cmd.Attr("levels")
	switch {
	case hasPeople == hasLevels:
		b.p.Add("%s: takes people or levels, one of the two", cmd.Label())
	case hasPeople:
		if wsxmlcmd.NonEmpty(b.p, cmd.At("people"), people) {
			ev.payload["mode"], ev.payload["people"] = "chosen", strings.Fields(people)
		}
	default:
		if n, ok := wsxmlcmd.IntAttr(b.p, cmd, "levels", true); ok {
			ev.payload["mode"], ev.payload["levels"] = "all", n
		}
	}
	if _, ok := cmd.Attr("completion"); ok {
		if v, ok := wsxmlcmd.EnumAttr(b.p, cmd, "completion", []string{"assignee", "assigner"}); ok {
			ev.payload["completion"] = v
		}
	}
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
	v = strings.TrimSpace(v)
	if rest, isRef := strings.CutPrefix(v, "$"); isRef {
		n, err := strconv.Atoi(rest)
		if err != nil || n < 1 || n >= cmd.Pos {
			b.p.Add("%s: $N names an earlier command of this batch by position; got %q", cmd.At(name), v)
			return
		}
		target, known := b.byPos[n]
		idx, isEvent := b.eventOf[n]
		if !known || !isEvent || strings.TrimPrefix(target.Tag, trackerPrefix) != creator {
			b.p.Add("%s: %s names command %d, which creates no %s; $N must name a <%s%s>", cmd.At(name), v, n, noun, trackerPrefix, creator)
			return
		}
		ev.refs[name] = idx
		return
	}
	id, err := parseTrackerID(v)
	if err != nil {
		b.p.Add("%s: must be an %s ID such as maya.pc-5, or $N; got %q", cmd.At(name), noun, v)
		return
	}
	ev.payload[name] = id
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
	unknownStore := func(cmd wsxmlcmd.Command, store string) {
		if len(ids) == 0 {
			p.Add("%s: no store is registered; register one with <%s%s>", cmd.At("store"), trackerPrefix, trackerRegister)
			return
		}
		p.Add("%s: %q is not a registered store (registered: %s)", cmd.At("store"), store, strings.Join(ids, ", "))
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
						unknownStore(cmd, store)
					}
				} else if len(ids) == 0 {
					p.Add("%s: no store is registered", cmd.Label())
				}
			}
			continue
		}
		ev := &trackerEvent{cmd: cmd, typ: name, origin: wstracker.OriginAgent, payload: map[string]any{}, refs: map[string]int{}}
		if store, ok := wsxmlcmd.RequiredAttr(p, cmd, "store"); ok {
			switch {
			case !known[store]:
				unknownStore(cmd, store)
			case b.store == "":
				b.store = store
			case store != b.store:
				p.Add("%s: %q; this batch writes to %q, and one batch writes to one store", cmd.At("store"), store, b.store)
			}
		}
		if _, ok := cmd.Attr("origin"); ok {
			if v, ok := wsxmlcmd.EnumAttr(p, cmd, "origin", trackerOrigins); ok {
				ev.origin = v
			}
		}
		def.build(b, cmd, ev)
		b.events = append(b.events, ev)
	}
	return b
}

// newEvents builds the batch's events for a store whose next event gets
// next, resolving each $N to the id its event gets.
func (b *trackerBatch) newEvents(next wsledger.EventID) ([]wsledger.NewEvent, error) {
	out := make([]wsledger.NewEvent, len(b.events))
	for i, ev := range b.events {
		payload := make(map[string]any, len(ev.payload)+len(ev.refs))
		for k, v := range ev.payload {
			payload[k] = v
		}
		for field, idx := range ev.refs {
			id := next
			id.Seq += idx
			payload[field] = id.String()
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
	key, err := sessionStateKey(tool, args)
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
			cmd := b.events[refusal.Index].cmd
			return toolErrorTextResponse(id, s.trackerRefusal(tool, []string{
				fmt.Sprintf("%s: the store would flag this write: %s", cmd.Label(), refusal.Reason),
			}))
		}
		if err == nil {
			var sb strings.Builder
			fmt.Fprintf(&sb, "%s: wrote %s\n", b.store, trackerCount(len(ids), "event"))
			for i, eventID := range ids {
				if i < len(b.events) {
					fmt.Fprintf(&sb, "%d %s %s\n", b.events[i].cmd.Pos, b.events[i].cmd.Tag, trackerAlias(eventID.String()))
				}
			}
			text = sb.String()
		}
	}
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %s", tool, redactRemoteCredentials(err.Error())))
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
// failure leaves nothing registered.
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
	if err != nil {
		return "", fmt.Errorf("%s: %w. Nothing was registered", cmd.Label(), err)
	}
	how := "joined"
	if res.Creator {
		how = "created"
	}
	where := "no remote"
	if res.Entry.Remote != "" {
		where = "remote " + redactRemoteCredentials(res.Entry.Remote)
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
			fmt.Fprintf(&sb, "%s: failed: %s\n", store, redactRemoteCredentials(err.Error()))
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
		parts = append(parts, fmt.Sprintf("warning: %s %s %s", w.Ref, w.Kind, w.Detail))
	}
	return strings.Join(parts, ", ")
}

// trackerQuery is one parsed tracker.query call.
type trackerQuery struct {
	store, scope, item          string
	kind, status, project, text string
	json                        bool
}

var (
	trackerScopes = []string{"mine", "received", "assigned", "team", "all"}
	trackerKinds  = []string{"task", "todo", "topic"}
	// trackerStatuses are the status filter's values: open (not done) and
	// hidden, then an item's derived statuses.
	trackerStatuses = []string{"open", "hidden", "todo", "doing", "blocked", "awaiting", "done", "pending", "unassigned", "declined", "returned"}
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
	fields := map[string]*string{"store": &q.store, "scope": &q.scope, "item": &q.item, "kind": &q.kind, "status": &q.status, "project": &q.project, "text": &q.text}
	names := make([]string, 0, len(payload))
	for name := range payload {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dst, known := fields[name]
		if !known {
			add("payload.%s: unknown (known: store, scope, item, kind, status, project, text)", name)
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
	narrowed := q.scope != "" || q.item != "" || q.kind != "" || q.status != "" || q.project != "" || q.text != ""
	switch {
	case q.store == "" && narrowed:
		add("payload: scope, item, and the filters need store")
	case q.item != "" && (q.scope != "" || q.kind != "" || q.status != "" || q.project != "" || q.text != ""):
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

// trackerStoreSummary is one row of the store list.
type trackerStoreSummary struct {
	Entry wstrackerhost.StoreEntry
	// View is nil when the store could not be read; Err says why.
	View *wstrackerhost.StoreView
	Err  error
}

func (s *Server) handleTrackerQuery(ctx context.Context, id json.RawMessage, args map[string]any) response {
	const tool = "tracker.query"
	key, err := sessionStateKey(tool, args)
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
			return toolJSONResponse(id, trackerStoreListJSON(summaries, profiles), nil)
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
		return toolTextResponse(id, "", fmt.Errorf("%s: %s", tool, redactRemoteCredentials(err.Error())))
	}
	v, err := h.View(ctx, q.store, 0)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %s", tool, redactRemoteCredentials(err.Error())))
	}
	redactStoreView(v)
	switch {
	case q.json:
		resp := toolJSONResponse(id, v, nil)
		_ = s.trackerSucceeded(key, "")
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

// remoteCredentials matches the userinfo of an http(s) URL, where a token
// or password sits; an ssh user (git@host) carries none.
var remoteCredentials = regexp.MustCompile(`(?i)(https?://)[^/@\s]+@`)

// redactRemoteCredentials hides the userinfo of every http(s) URL in s, so
// a token in a store's remote URL never reaches the agent's context.
func redactRemoteCredentials(s string) string {
	return remoteCredentials.ReplaceAllString(s, "${1}***@")
}

// redactStoreView redacts the view's remote URL and sync error in place.
func redactStoreView(v *wstrackerhost.StoreView) {
	if v.Store.Remote != nil {
		r := redactRemoteCredentials(*v.Store.Remote)
		v.Store.Remote = &r
	}
	v.Sync.Error = redactRemoteCredentials(v.Sync.Error)
}

// trackerStoreListJSON is the store list's format json shape.
func trackerStoreListJSON(summaries []trackerStoreSummary, profiles wstrackerhost.SavedProfiles) map[string]any {
	stores := make([]map[string]any, 0, len(summaries))
	for _, sum := range summaries {
		e := sum.Entry
		row := map[string]any{"id": e.ID, "label": e.DisplayLabel(), "member": e.Member, "remote": nil}
		if e.Remote != "" {
			row["remote"] = redactRemoteCredentials(e.Remote)
		}
		if sum.View != nil {
			redactStoreView(sum.View)
			row["sync"] = sum.View.Sync
			row["today"] = sum.View.My.Today
		} else if sum.Err != nil {
			row["error"] = redactRemoteCredentials(sum.Err.Error())
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
			fmt.Fprintf(&sb, ", unreadable: %s\n", redactRemoteCredentials(fmt.Sprint(sum.Err)))
			continue
		}
		redactStoreView(sum.View)
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
