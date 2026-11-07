package wstracker

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Domain event types. The identity types are wsledger's (wsledger.TypeMemberJoin
// and the roles.* constants there).
const (
	TypeItemCreate        = "item.create"
	TypeItemEdit          = "item.edit"
	TypeItemHide          = "item.hide"
	TypeItemAssign        = "item.assign"
	TypeItemHandoff       = "item.handoff"
	TypeItemAccept        = "item.accept"
	TypeItemDecline       = "item.decline"
	TypeItemReturn        = "item.return"
	TypeItemUnassign      = "item.unassign"
	TypeItemStatus        = "item.status"
	TypeItemClose         = "item.close"
	TypeItemConfirm       = "item.confirm"
	TypeItemReject        = "item.reject"
	TypeItemReopen        = "item.reopen"
	TypeCommentAdd        = "comment.add"
	TypeCommentEdit       = "comment.edit"
	TypeCommentHide       = "comment.hide"
	TypeSetLead           = "roles.set_lead"
	TypeSetPriorityLevels = "roles.set_priority_levels"
)

// Origins. An empty origin is human.
const (
	OriginHuman     = "human"
	OriginAgent     = "agent"
	OriginAgentAuto = "agent-auto"
)

// Payload shapes, for writers. The fold reads payloads leniently (as the
// reference engine does): a field of the wrong type fails the event's rule
// with the rule's own reason rather than a decode error.

// ItemCreate is the payload of item.create.
type ItemCreate struct {
	Kind     string `json:"kind"` // task, todo, topic
	Title    string `json:"title"`
	Body     string `json:"body,omitempty"`
	Project  string `json:"project,omitempty"`
	Due      string `json:"due,omitempty"` // YYYY-MM-DD
	Priority *int   `json:"priority,omitempty"`
	Parent   string `json:"parent,omitempty"`
}

// ItemAssign is the payload of item.assign.
type ItemAssign struct {
	Item       string   `json:"item"`
	Mode       string   `json:"mode"` // all, chosen
	Levels     int      `json:"levels,omitempty"`
	People     []string `json:"people,omitempty"`
	Completion string   `json:"completion,omitempty"` // assignee (default), assigner
}

// ItemHandoff is the payload of item.handoff.
type ItemHandoff struct {
	Item     string `json:"item"`
	To       string `json:"to"`
	Assignee string `json:"assignee,omitempty"`
	Note     string `json:"note,omitempty"`
}

// ItemRef is the payload of item.hide, item.accept, item.decline,
// item.close, item.unassign, item.return, item.confirm, item.reject, and
// item.reopen (Assignee and Note where the event takes them).
type ItemRef struct {
	Item     string `json:"item"`
	Assignee string `json:"assignee,omitempty"`
	Note     string `json:"note,omitempty"`
}

// ItemStatus is the payload of item.status.
type ItemStatus struct {
	Item   string `json:"item"`
	Status string `json:"status"` // todo, doing, blocked
	Note   string `json:"note,omitempty"`
}

// ItemEdit is the payload of item.edit: a raw object, because a present
// null clears due and an absent field is untouched.
type ItemEdit map[string]any

// CommentAdd is the payload of comment.add.
type CommentAdd struct {
	Item    string `json:"item"`
	Body    string `json:"body"`
	ReplyTo string `json:"reply_to,omitempty"`
}

// CommentRef is the payload of comment.edit (Body) and comment.hide.
type CommentRef struct {
	Comment string `json:"comment"`
	Body    string `json:"body,omitempty"`
}

// SetLead is the payload of roles.set_lead.
type SetLead struct {
	Member string   `json:"member"`
	Leads  []string `json:"leads"`
}

// SetPriorityLevels is the payload of roles.set_priority_levels.
type SetPriorityLevels struct {
	Names   []string `json:"names"`
	Default int      `json:"default"`
}

// obj is a decoded payload. Missing and null differ, as in JSON.
type obj map[string]any

func parsePayload(raw json.RawMessage) obj {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return obj{}
	}
	var o map[string]any
	if err := json.Unmarshal(raw, &o); err != nil || o == nil {
		return obj{}
	}
	return obj(o)
}

func (p obj) has(k string) bool { _, ok := p[k]; return ok }

// null reports a missing or null field (JS "== null").
func (p obj) null(k string) bool { return p[k] == nil }

func (p obj) str(k string) string { s, _ := p[k].(string); return s }

// truthy reports JS truthiness of a field.
func (p obj) truthy(k string) bool { return truthy(p[k]) }

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case float64:
		return x != 0 && !math.IsNaN(x)
	case bool:
		return x
	}
	return true
}

// disp renders a field as JS String() would, for reasons.
func (p obj) disp(k string) string {
	v, ok := p[k]
	if !ok {
		return "undefined"
	}
	return jsString(v)
}

func jsString(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case float64:
		return jsNum(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			if e != nil {
				parts[i] = jsString(e)
			}
		}
		return strings.Join(parts, ",")
	}
	return "[object Object]"
}

func jsNum(f float64) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// intOf returns a field as an integer when it is one (Number.isInteger).
func (p obj) intOf(k string) (int, bool) {
	f, ok := p[k].(float64)
	if !ok || f != math.Trunc(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return int(f), true
}

// list returns an array field (nil when missing or not an array).
func (p obj) list(k string) ([]any, bool) {
	l, ok := p[k].([]any)
	return l, ok
}

// strList returns an array field's elements in their JS String() form.
func (p obj) strList(k string) []string {
	l, _ := p.list(k)
	out := make([]string, len(l))
	for i, e := range l {
		out[i] = jsString(e)
	}
	return out
}

// alias is the display form of an event id: maya.pc#15 -> maya.pc-15.
func alias(id string) string { return strings.Replace(id, "#", "-", 1) }

// truncate60 mirrors the engine's show(): longer than 60 UTF-16 units keeps
// 57 and appends "...".
func truncate60(s string) string {
	u := utf16.Encode([]rune(s))
	if len(u) <= 60 {
		return s
	}
	return string(utf16.Decode(u[:57])) + "..."
}

var dueDigits = func(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i, c := range s {
		if i == 4 || i == 7 {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// validDue mirrors /^\d{4}-\d{2}-\d{2}$/ plus Date.parse: month 1-12 and
// day 1-31 (Date.parse accepts 2026-02-30 and rolls it over).
func validDue(v any) bool {
	s, ok := v.(string)
	if !ok || !dueDigits(s) {
		return false
	}
	m, _ := strconv.Atoi(s[5:7])
	d, _ := strconv.Atoi(s[8:10])
	return m >= 1 && m <= 12 && d >= 1 && d <= 31
}
