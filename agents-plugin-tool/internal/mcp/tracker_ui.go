package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// tracker.ui is a store support conversation's dashboard UI command: an
// operation the user can do on the store's tracker view (search, focus,
// scope, view), which the conversation's chat performs once, live. The
// result's first line "ui <op> <arg>" is what the agent runner reads into
// the conversation's ring; the tool checks the command first, so a command
// the view could not perform is an error and produces no line.

const (
	trackerUITool = "tracker.ui"
	// trackerUIPrefix starts a tracker.ui result's command line.
	trackerUIPrefix = "ui "
)

func (s *Server) handleTrackerUI(ctx context.Context, id json.RawMessage, args map[string]any) response {
	const tool = trackerUITool
	_, support, err := s.trackerSessionKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if support == nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: a store's support conversation moves the tracker view of the chat it talks in; a lead session has no such chat", tool))
	}
	str := func(key string) string { v, _ := args[key].(string); return strings.TrimSpace(v) }
	refuse := func(format string, a ...any) response {
		return toolErrorTextResponse(id, tool+": "+fmt.Sprintf(format, a...))
	}
	op := str("op")
	cmd := wslive.UICommand{Op: op}
	var does string
	switch op {
	case wslive.UISearch:
		raw, _ := args["query"].(string)
		q := strings.TrimSpace(raw)
		if q == "" {
			return refuse("search needs a query")
		}
		if strings.ContainsAny(q, "\r\n") {
			return refuse("the query is one line of the search box; drop the line breaks")
		}
		if utf8.RuneCountInString(q) > wslive.UIQueryRunes {
			return refuse("the query is longer than %d characters", wslive.UIQueryRunes)
		}
		cmd.Arg = q
		does = fmt.Sprintf("types %q into the Items search box", q)
	case wslive.UIFocus:
		raw := str("item")
		if raw == "" {
			return refuse("focus needs an item")
		}
		item, err := parseTrackerID(raw)
		if err != nil {
			return refuse("item %q is not an item ID; pass the ID as tracker.query prints it, such as maya.pc-5", raw)
		}
		h, err := s.trackerHost()
		if err != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
		}
		v, err := trackerStoreView(ctx, h, support.store)
		if err != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %s", tool, wstrackerhost.RedactCredentials(err.Error())))
		}
		it, ok := v.Items[item]
		if !ok {
			return refuse("%s is not an item of this conversation's store %s; the view stays in its store, so answer an item of another store in text and name its store", trackerAlias(item), support.store)
		}
		cmd.Arg = trackerAlias(item)
		does = "opens item " + cmd.Arg
		if rawc := str("comment"); rawc != "" {
			cid, err := parseTrackerID(rawc)
			if err != nil {
				return refuse("comment %q is not a comment ID; pass the ID as tracker.query prints it in the item's comments", rawc)
			}
			if _, isComment := v.Comments[cid]; !isComment || !slices.Contains(it.Thread, cid) {
				return refuse("%s is not a comment on the thread of %s; read the item's comments with tracker.query", trackerAlias(cid), cmd.Arg)
			}
			cmd.Comment = trackerAlias(cid)
			does += " at comment " + cmd.Comment
		}
	case wslive.UIScope:
		cmd.Arg = str("scope")
		if !slices.Contains(wslive.UIScopes, cmd.Arg) {
			return refuse("scope %q is not an Items scope; pass one of %s", cmd.Arg, strings.Join(wslive.UIScopes, ", "))
		}
		does = "switches the Items scope to " + cmd.Arg
	case wslive.UIView:
		cmd.Arg = str("mode")
		if !slices.Contains(wslive.UIViewModes, cmd.Arg) {
			return refuse("mode %q is not an Items view mode; pass one of %s", cmd.Arg, strings.Join(wslive.UIViewModes, ", "))
		}
		does = "switches the Items view to " + cmd.Arg + " (a mode the current scope does not offer leaves the view as it is)"
	default:
		return refuse("unknown op %q; pass one of %s", op, strings.Join(wslive.UIOps, ", "))
	}
	if err := cmd.Validate(); err != nil {
		return refuse("%v", err)
	}
	return toolTextResponse(id, formatTrackerUI(cmd, does), nil)
}

// formatTrackerUI is the command result: the ui line the runner reads,
// then what the user's view does, for the agent.
func formatTrackerUI(cmd wslive.UICommand, does string) string {
	line := trackerUIPrefix + cmd.Op + " " + cmd.Arg
	if cmd.Comment != "" {
		line += " " + cmd.Comment
	}
	return fmt.Sprintf("%s\nThe user's chat %s, if the chat is open; nothing in the store changes. Send UI commands as the last step of your reply.\n", line, does)
}
