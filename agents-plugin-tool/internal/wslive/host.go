package wslive

import (
	"fmt"
	"reflect"
	"slices"
)

// RingNotice is a typed failure a host that runs the session itself (the
// agent runner) read from its harness's stream: a login needed, a model
// unavailable, or a usage limit (Notice). It is not an adapter event kind.
const RingNotice = "notice"

// Notice kinds.
const (
	// NoticeLogin: the harness needs the user to log in again.
	NoticeLogin = "login"
	// NoticeModel: the model the session asked for is not available.
	NoticeModel = "model"
	// NoticeUsageLimit: the plan's usage limit refuses requests until
	// ResetsAt (when known).
	NoticeUsageLimit = "usage_limit"
)

// NoticeTextRunes caps a notice's text.
const NoticeTextRunes = 600

// Notice is a typed failure line: its kind, the harness's own text, and
// for a usage limit the time it resets (epoch ms; 0 when unknown).
type Notice struct {
	Kind     string `json:"kind"`
	Text     string `json:"text,omitempty"`
	ResetsAt int64  `json:"resetsAt,omitempty"`
}

// ValidNoticeKind reports whether kind is one of the notice kinds.
func ValidNoticeKind(kind string) bool {
	return kind == NoticeLogin || kind == NoticeModel || kind == NoticeUsageLimit
}

// Notice writes a typed failure line at now, after the reply streamed so
// far and the open tool gap.
func (st *State) Notice(n Notice, now int64) (Result, error) {
	if !ValidNoticeKind(n.Kind) {
		return Result{}, fmt.Errorf("notice kind must be %q, %q, or %q", NoticeLogin, NoticeModel, NoticeUsageLimit)
	}
	n.Text = cleanLine(st.scrub.scrub(n.Text), NoticeTextRunes)
	n.ResetsAt = max(n.ResetsAt, 0)
	return st.lineAfterReply(RingEntry{Kind: RingNotice, Notice: &n}, now), nil
}

// Host is what a host that runs the session's harness itself (the agent
// runner) reports about it beyond the adapter's events. A session an
// adapter feeds has none.
type Host struct {
	// BackgroundTasks counts the harness's running background tasks (its
	// shells, monitors, and background subagents), as its stream last
	// reported the whole list.
	BackgroundTasks int `json:"backgroundTasks,omitempty"`
	// Background and Monitors split the aggregate when task types are observable.
	// Nil is unknown, distinct from a known zero; BackgroundTasks keeps its meaning.
	Background *int `json:"background,omitempty"`
	Monitors   *int `json:"monitors,omitempty"`
	// Failure is the typed failure that keeps the session from working (a
	// login needed, a model unavailable, a usage limit) until a turn
	// succeeds; nil when none.
	Failure *Notice `json:"failure,omitempty"`
	// MCPBroken names the MCP servers whose connection broke and did not
	// come back in-stream, sorted.
	MCPBroken []string `json:"mcpBroken,omitempty"`
	// Retry is the API request retry under way; nil when none.
	Retry *Retry `json:"retry,omitempty"`
}

// Retry is an API request the harness retries: attempt N of MaxRetries
// (0 when unknown), with the error and HTTP status it retries after.
type Retry struct {
	Attempt    int    `json:"attempt"`
	MaxRetries int    `json:"maxRetries,omitempty"`
	Error      string `json:"error,omitempty"`
	Status     int    `json:"status,omitempty"`
}

// RetryErrorRunes caps a retry's error text.
const RetryErrorRunes = 200

// SetHost replaces the session's host state (nil, or a zero one, clears
// it) and reports whether the snapshot changed.
func (st *State) SetHost(h *Host) bool {
	var next *Host
	if h != nil {
		c := *h
		c.BackgroundTasks = max(c.BackgroundTasks, 0)
		c.Background, c.Monitors = taskCount(c.Background), taskCount(c.Monitors)
		if c.Failure != nil {
			f := *c.Failure
			f.Text = cleanLine(st.scrub.scrub(f.Text), NoticeTextRunes)
			c.Failure = &f
		}
		c.MCPBroken = slices.Clone(c.MCPBroken)
		slices.Sort(c.MCPBroken)
		if len(c.MCPBroken) == 0 {
			c.MCPBroken = nil
		}
		if c.Retry != nil {
			r := *c.Retry
			r.Error = cleanLine(st.scrub.scrub(r.Error), RetryErrorRunes)
			c.Retry = &r
		}
		if !reflect.DeepEqual(c, Host{}) {
			next = &c
		}
	}
	if reflect.DeepEqual(next, st.host) {
		return false
	}
	st.host = next
	return true
}

func (h *Host) clone() *Host {
	if h == nil {
		return nil
	}
	c := *h
	c.Background, c.Monitors = taskCount(h.Background), taskCount(h.Monitors)
	if h.Failure != nil {
		f := *h.Failure
		c.Failure = &f
	}
	c.MCPBroken = slices.Clone(h.MCPBroken)
	if h.Retry != nil {
		r := *h.Retry
		c.Retry = &r
	}
	return &c
}
