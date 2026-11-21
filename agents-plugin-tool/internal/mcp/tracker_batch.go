// Tracker batch events, references, and identifiers.
package mcp

import (
	"errors"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

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
	// splitOff marks the second item.assign of cmd, holding the targets
	// whose default completion differs from its first target's
	// (completionClasses).
	splitOff bool
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
	// storeCmd is the store command (register, sync, unregister, relabel,
	// hide, unhide), when the batch is one.
	storeCmd *wsxmlcmd.Command
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

// itemRefList reads an optional space-separated list of item IDs, each in
// either form or $N; an empty list writes an empty field.
func (b *trackerBatch) itemRefList(cmd wsxmlcmd.Command, ev *trackerEvent, name string) {
	b.refList(cmd, ev, name, wstracker.TypeItemCreate, "item")
}

// labelRefList reads the labels attribute: label IDs as the query's labels
// line prints them, or $N for a tk:roles.add_label earlier in the batch.
func (b *trackerBatch) labelRefList(cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.refList(cmd, ev, "labels", wstracker.TypeAddLabel, "label")
}

func (b *trackerBatch) labelRef(cmd wsxmlcmd.Command, ev *trackerEvent) {
	b.ref(cmd, ev, "label", wstracker.TypeAddLabel, "label")
}

func (b *trackerBatch) refList(cmd wsxmlcmd.Command, ev *trackerEvent, name, creator, noun string) {
	v, ok := cmd.Attr(name)
	if !ok {
		return
	}
	list := []trackerListRef{}
	for _, tok := range strings.Fields(v) {
		if id, idx, ok := b.parseRef(cmd, name, tok, creator, noun); ok {
			list = append(list, trackerListRef{id: id, event: idx})
		}
	}
	ev.refLists[name] = list
}

// trackerColor is a label colour as written: trimmed and lower-cased, so
// #D14343 is the #d14343 the store requires.
func trackerColor(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

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
		article := "a"
		if noun == "item" {
			article = "an"
		}
		b.p.Add("%s: must be %s %s ID such as maya.pc-5, or $N; got %q", cmd.At(name), article, noun, v)
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

// parseTrackerRuleID returns the stored form of a rule id given in either
// form: the assign event's id, then ~root for a targets rule (maya.pc-5~raj
// or maya.pc#5~raj); an older levels rule is the event id alone.
func parseTrackerRuleID(s string) (string, error) {
	ev, root, hasRoot := strings.Cut(s, "~")
	id, err := parseTrackerID(ev)
	if err != nil {
		return "", err
	}
	if !hasRoot {
		return id, nil
	}
	if root == "" {
		return "", errors.New("no root")
	}
	return id + "~" + root, nil
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
//
// reach returns the members below a store's member writer in its org tree
// (member id -> depth) as the store holds it now; writerDefaults reads it
// to pick an item.assign's default completion.
