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
	// TypeSetAttachmentLimit changes the store's attachments.max_bytes.
	TypeSetAttachmentLimit = "roles.set_attachment_limit"
	// The store's label list: an admin adds, renames or recolours, and
	// removes labels; an item carries label ids in its labels field. A
	// label's id is its roles.add_label event's id.
	TypeAddLabel    = "roles.add_label"
	TypeEditLabel   = "roles.edit_label"
	TypeRemoveLabel = "roles.remove_label"
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

// Attachment is the metadata of one attached file; OID is the sha256 of
// its content. Only metadata is carried: the content itself is stored apart
// from the events.
type Attachment struct {
	OID  string `json:"oid"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	Mime string `json:"mime"`
}

// ItemCreate is the payload of item.create. Mentions lists member ids (the
// body keeps @name for display), Refs the item ids the body references with
// #ID; Mentions, Attachments, and Refs ride with the body. After is the
// sibling the item follows: nil leaves it last, "" puts it first.
type ItemCreate struct {
	Kind        string       `json:"kind"` // task, todo, topic
	Title       string       `json:"title"`
	Body        string       `json:"body,omitempty"`
	Project     string       `json:"project,omitempty"`
	Due         string       `json:"due,omitempty"` // YYYY-MM-DD
	Priority    *int         `json:"priority,omitempty"`
	Parent      string       `json:"parent,omitempty"`
	After       *string      `json:"after,omitempty"`
	Mentions    []string     `json:"mentions,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Refs        []string     `json:"refs,omitempty"`
	Labels      []string     `json:"labels,omitempty"` // label ids from the store's list
}

// ItemAssign is the payload of item.assign: Targets, or the older Mode with
// Levels (all) or People (chosen).
type ItemAssign struct {
	Item       string         `json:"item"`
	Targets    []AssignTarget `json:"targets,omitempty"`
	Mode       string         `json:"mode,omitempty"` // all, chosen
	Levels     int            `json:"levels,omitempty"`
	People     []string       `json:"people,omitempty"`
	Completion string         `json:"completion,omitempty"` // assignee (default), assigner
}

// AssignTarget is one item.assign target: a member (Person), or a subtree
// rule rooted at Root covering Root itself with IncludeRoot and everyone up
// to Levels below it (0: the whole subtree).
type AssignTarget struct {
	Person      string
	Root        string
	IncludeRoot bool
	Levels      int
}

// MarshalJSON writes a person as its member id and a rule as {root,
// include_root?, levels?}.
func (t AssignTarget) MarshalJSON() ([]byte, error) {
	if t.Root == "" {
		return json.Marshal(t.Person)
	}
	m := map[string]any{"root": t.Root}
	if t.IncludeRoot {
		m["include_root"] = true
	}
	if t.Levels > 0 {
		m["levels"] = t.Levels
	}
	return json.Marshal(m)
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
// item.reopen (Assignee and Note where the event takes them; Root, the root
// of the rules to end, on item.unassign only).
type ItemRef struct {
	Item     string `json:"item"`
	Assignee string `json:"assignee,omitempty"`
	Root     string `json:"root,omitempty"`
	Note     string `json:"note,omitempty"`
}

// Taking work: the creator opens a task nobody else holds to a scope and a
// member in it claims it; a member requests someone's live assignment and
// its holder or the creator approves or denies the request.
const (
	TypeItemOpen      = "item.open"
	TypeItemUnopen    = "item.unopen"
	TypeItemClaim     = "item.claim"
	TypeItemRequest   = "item.request"
	TypeItemApprove   = "item.approve"
	TypeItemDeny      = "item.deny"
	TypeItemUnrequest = "item.unrequest"
)

// Opening scopes: the opener's org subtree, or every member of the store.
const (
	ScopeTeam  = "team"
	ScopeStore = "store"
)

// ItemOpen is the payload of item.open. item.unopen and item.claim take an
// ItemRef with Item only.
type ItemOpen struct {
	Item  string `json:"item"`
	Scope string `json:"scope"` // team, store
}

// ItemRequest is the payload of item.request: a take request for the live
// assignment Assignee holds on Item.
type ItemRequest struct {
	Item     string `json:"item"`
	Assignee string `json:"assignee"`
	Note     string `json:"note,omitempty"`
}

// RequestRef is the payload of item.approve, item.deny, and item.unrequest:
// Request is the item.request event's id.
type RequestRef struct {
	Item    string `json:"item"`
	Request string `json:"request"`
	Note    string `json:"note,omitempty"`
}

// ItemStatus is the payload of item.status.
type ItemStatus struct {
	Item   string `json:"item"`
	Status string `json:"status"` // todo, doing, blocked
	Note   string `json:"note,omitempty"`
}

// ItemEdit is the payload of item.edit: a raw object, because a present
// null clears due and an absent field is untouched. mentions, attachments,
// and refs go only with a body, and replace the body's lists.
type ItemEdit map[string]any

// CommentAdd is the payload of comment.add.
type CommentAdd struct {
	Item        string       `json:"item"`
	Body        string       `json:"body"`
	ReplyTo     string       `json:"reply_to,omitempty"`
	Mentions    []string     `json:"mentions,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Refs        []string     `json:"refs,omitempty"`
}

// CommentRef is the payload of comment.edit (Body, Mentions, Attachments,
// Refs) and comment.hide.
type CommentRef struct {
	Comment     string       `json:"comment"`
	Body        string       `json:"body,omitempty"`
	Mentions    []string     `json:"mentions,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Refs        []string     `json:"refs,omitempty"`
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

// SetAttachmentLimit is the payload of roles.set_attachment_limit.
type SetAttachmentLimit struct {
	MaxBytes int64 `json:"max_bytes"`
}

// AddLabel is the payload of roles.add_label. Name is unique in the list
// (case-insensitive) and has no surrounding spaces; Color is #rrggbb in
// lowercase hex.
type AddLabel struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// EditLabel is the payload of roles.edit_label: Label is the label's id,
// and Name, Color, or both change.
type EditLabel struct {
	Label string `json:"label"`
	Name  string `json:"name,omitempty"`
	Color string `json:"color,omitempty"`
}

// RemoveLabel is the payload of roles.remove_label.
type RemoveLabel struct {
	Label string `json:"label"`
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

// jsNum is Number.prototype.toString: positional between 1e-6 and 1e21,
// exponent form outside, with no leading zero in the exponent (1e-7, 1e+21).
func jsNum(f float64) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	if a := math.Abs(f); a != 0 && (a < 1e-6 || a >= 1e21) {
		s := strconv.FormatFloat(f, 'e', -1, 64)
		if n := len(s); s[n-4] == 'e' && s[n-2] == '0' {
			s = s[:n-2] + s[n-1:]
		}
		return s
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// intOf returns a field as an int when it is an integer (Number.isInteger)
// within the int32 range. Go's conversion of an out-of-range float to int is
// platform-defined (arm64 saturates, amd64 yields MinInt64), so a larger
// integer reports not-ok instead; every caller range-checks the value against
// a small bound and words not-ok and out-of-range alike.
func (p obj) intOf(k string) (int, bool) {
	f, ok := p[k].(float64)
	if !ok || f != math.Trunc(f) || f < math.MinInt32 || f > math.MaxInt32 {
		return 0, false
	}
	return int(f), true
}

// posIntOf returns a field that is a positive integer (Number.isInteger and
// > 0) as the float64 the reference engine holds. It has no upper bound and
// never converts to int, so a value beyond the int range folds the same on
// every platform.
func (p obj) posIntOf(k string) (float64, bool) {
	f, ok := p[k].(float64)
	if !ok || f != math.Trunc(f) || math.IsInf(f, 0) || f <= 0 {
		return 0, false
	}
	return f, true
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
