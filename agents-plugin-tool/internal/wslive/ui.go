package wslive

import (
	"fmt"
	"slices"
	"strings"
)

// UI commands: a store's support conversation drives its own store's
// tracker view through the operations the user can do there (open the
// Items search with a query, focus an item and its thread, switch the
// Items scope or view mode). A command is a ring entry like a card line,
// so it belongs to its conversation and only a page showing that chat
// sees it. The ring carries it unchanged on every replay; whether it acts
// is the page's call: a chat acts on a command it receives live, after it
// has loaded the conversation's history, and draws a replayed one as an
// inert line.

// UI command ops.
const (
	UISearch = "search"
	UIFocus  = "focus"
	UIScope  = "scope"
	UIView   = "view"
)

// UIOps are the UI command ops, in the order a tool description lists them.
var UIOps = []string{UISearch, UIFocus, UIScope, UIView}

// UIScopes and UIViewModes are a copy of the tracker renderer's Items panel
// scope keys (SCOPES) and the view mode keys its scopes offer (SCOPE_MODES,
// where the all scope's tree is the list mode) in wsdashboard's tracker.js.
// Keep the two sites in step: a wsdashboard test reads tracker.js and fails
// when they differ.
var (
	UIScopes    = []string{"mine", "received", "given", "team", "all"}
	UIViewModes = []string{"list", "kanban", "gantt", "calendar"}
)

// UIQueryRunes caps a search command's query.
const UIQueryRunes = 200

// UICommand is one dashboard UI command: Op, and Arg as the op reads it
// (the search query, the focused item's alias, the scope key, or the view
// mode key); Comment is a focus command's comment alias on that item's
// thread, or empty.
type UICommand struct {
	Op      string `json:"op"`
	Arg     string `json:"arg"`
	Comment string `json:"comment,omitempty"`
}

// Validate refuses a command whose op or arguments are outside what the
// tracker view offers; it trims a search query to one line first.
func (c *UICommand) Validate() error {
	switch c.Op {
	case UISearch:
		c.Arg = cleanLine(c.Arg, UIQueryRunes)
		if c.Arg == "" {
			return fmt.Errorf("ui search needs a query")
		}
	case UIFocus:
		if !cardIDPattern.MatchString(c.Arg) || (c.Comment != "" && !cardIDPattern.MatchString(c.Comment)) {
			return fmt.Errorf("ui focus item and comment must be 1-128 letters, digits, or ._- starting with a letter or digit")
		}
		return nil
	case UIScope:
		if !slices.Contains(UIScopes, c.Arg) {
			return fmt.Errorf("ui scope must be one of %s", strings.Join(UIScopes, ", "))
		}
	case UIView:
		if !slices.Contains(UIViewModes, c.Arg) {
			return fmt.Errorf("ui view must be one of %s", strings.Join(UIViewModes, ", "))
		}
	default:
		return fmt.Errorf("ui op must be one of %s", strings.Join(UIOps, ", "))
	}
	if c.Comment != "" {
		return fmt.Errorf("ui %s takes no comment", c.Op)
	}
	return nil
}

// UI applies a dashboard UI command the lead sent at now, as Card applies
// a card line: the reply streamed so far and the open tool gap are written
// first, so the command follows the text and calls that led to it. It is
// not an adapter event kind: only the agent runner, reading its own
// conversation's tool results, sends one.
func (st *State) UI(cmd UICommand, now int64) (Result, error) {
	if err := cmd.Validate(); err != nil {
		return Result{}, err
	}
	if cmd.Op == UISearch {
		cmd.Arg = st.scrub.scrub(cmd.Arg)
	}
	return st.lineAfterReply(RingEntry{Kind: RingUI, UI: &cmd}, now), nil
}
