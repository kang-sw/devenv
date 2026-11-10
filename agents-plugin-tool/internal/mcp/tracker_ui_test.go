package mcp

import (
	"strings"
	"testing"
)

// TestTrackerUICommandLines pins tracker.ui's result: each op the
// tracker view offers comes back with its "ui <op> <arg>" first line, the
// line the agent runner reads; a focus names an item of the
// conversation's store, and optionally a comment on its thread.
func TestTrackerUICommandLines(t *testing.T) {
	f := newTrackerSupportFixture(t)
	t.Setenv("WS_MCP_ALLOWED_TOOLS", supportAllowlist)
	comment := trackerWrote(t, f.trackerUpdate(t, `<tk:store name="me"><tk:comment.add item="`+f.meItem+`">look here</tk:comment.add></tk:store>`))["1"]
	for _, c := range []struct {
		args map[string]any
		line string
	}{
		{map[string]any{"op": "search", "query": "  parser fix "}, "ui search parser fix"},
		{map[string]any{"op": "search", "query": `status:todo "the parser"`}, `ui search status:todo "the parser"`},
		{map[string]any{"op": "focus", "item": f.meItem}, "ui focus " + f.meItem},
		{map[string]any{"op": "focus", "item": f.meItem, "comment": comment}, "ui focus " + f.meItem + " " + comment},
		{map[string]any{"op": "focus", "item": strings.Replace(f.meItem, "-", "#", 1)}, "ui focus " + f.meItem},
		{map[string]any{"op": "scope", "scope": "received"}, "ui scope received"},
		{map[string]any{"op": "view", "mode": "gantt"}, "ui view gantt"},
	} {
		out := toolText(t, f.call(t, trackerUITool, f.as(f.support, c.args)))
		first, rest, _ := strings.Cut(out, "\n")
		if first != c.line || !strings.Contains(rest, "last step of your reply") {
			t.Errorf("tracker.ui %v = %q, want first line %q", c.args, out, c.line)
		}
	}
}

// TestTrackerUIRefusals pins the checks before any ui line: an unknown op,
// a missing or multi-line query, a scope or mode outside the tracker
// view's lists, an item outside the conversation's store, and a comment
// that is not on that item's thread are each a tool error naming the
// problem; a lead and a spawned key that is no support conversation are
// refused too.
func TestTrackerUIRefusals(t *testing.T) {
	f := newTrackerSupportFixture(t)
	t.Setenv("WS_MCP_ALLOWED_TOOLS", supportAllowlist)
	otherComment := trackerWrote(t, f.trackerUpdate(t, `<tk:store name="other"><tk:comment.add item="`+f.other+`">elsewhere</tk:comment.add></tk:store>`))["1"]
	second := trackerWrote(t, f.trackerUpdate(t, `<tk:store name="me"><tk:item.create kind="todo" title="Second"/></tk:store>`))["1"]
	onSecond := trackerWrote(t, f.trackerUpdate(t, `<tk:store name="me"><tk:comment.add item="`+second+`">on the second</tk:comment.add></tk:store>`))["1"]
	for _, c := range []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"op": "press", "query": "accept"}, `unknown op "press"`},
		{map[string]any{}, "missing required: op"},
		{map[string]any{"op": "search"}, "needs a query"},
		{map[string]any{"op": "search", "query": "two\nlines"}, "one line"},
		{map[string]any{"op": "search", "query": strings.Repeat("x", 201)}, "longer than 200"},
		{map[string]any{"op": "search", "query": "fix due:<tomorrow"}, "not valid search grammar"},
		{map[string]any{"op": "scope", "scope": "everyone"}, `scope "everyone" is not an Items scope`},
		{map[string]any{"op": "view", "mode": "tree"}, `mode "tree" is not an Items view mode`},
		{map[string]any{"op": "focus"}, "needs an item"},
		{map[string]any{"op": "focus", "item": "not an id"}, "is not an item ID"},
		{map[string]any{"op": "focus", "item": f.other}, "not an item of this conversation's store me"},
		{map[string]any{"op": "focus", "item": "ada.nowhere-99"}, "not an item of this conversation's store me"},
		{map[string]any{"op": "focus", "item": f.meItem, "comment": onSecond}, "not a comment on the thread of " + f.meItem},
		{map[string]any{"op": "focus", "item": f.meItem, "comment": otherComment}, "not a comment on the thread of"},
		{map[string]any{"op": "focus", "item": f.meItem, "comment": second}, "not a comment on the thread of"},
	} {
		resp := f.call(t, trackerUITool, f.as(f.support, c.args))
		if !isToolError(resp) || !strings.Contains(strings.ReplaceAll(resp, `\"`, `"`), c.want) {
			t.Errorf("tracker.ui %v = %s, want a tool error naming %q", c.args, resp, c.want)
		}
	}
	// A lead that allowlists the tool has no chat whose view it could move.
	if resp := f.call(t, trackerUITool, map[string]any{"op": "scope", "scope": "all"}); !isToolError(resp) || !strings.Contains(resp, "no such chat") {
		t.Fatalf("a lead's tracker.ui should be refused: %s", resp)
	}
	bare, err := f.s.sessions.mint(f.meDir, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp := f.call(t, trackerUITool, f.as(bare, map[string]any{"op": "scope", "scope": "all"})); !isToolError(resp) || !strings.Contains(resp, "only a tracker store's support conversation moves the tracker view") {
		t.Fatalf("a non-support spawned key must be refused tracker.ui, with its own reason: %s", resp)
	}
}
