package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// tracker.show is a store support conversation's card-reference call: the
// chat renders the call as a mini card of the item from the open store's
// view state, so the result is only what the agent needs to go on.

const (
	trackerShowTool = "tracker.show"
	// trackerCardPrefix starts a tracker.show result that the chat draws
	// as a card: "card <store> <alias>".
	trackerCardPrefix = "card "
)

func (s *Server) handleTrackerShow(ctx context.Context, id json.RawMessage, args map[string]any) response {
	const tool = trackerShowTool
	_, support, err := s.trackerSessionKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if support == nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: a store's support conversation draws cards in that store's chat; a lead session has no chat to draw in, so read the item with tracker.query", tool))
	}
	raw, _ := args["item"].(string)
	item, err := parseTrackerID(strings.TrimSpace(raw))
	if err != nil {
		return toolErrorTextResponse(id, fmt.Sprintf("%s: item %q is not an item ID; pass the ID as tracker.query prints it, such as maya.pc-5", tool, raw))
	}
	h, err := s.trackerHost()
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	registered, err := h.Stores()
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	// The conversation's own store first: only its items become cards.
	order := []string{support.store}
	for _, e := range registered {
		if e.ID != support.store {
			order = append(order, e.ID)
		}
	}
	for _, store := range order {
		v, err := trackerStoreView(ctx, h, store)
		if err != nil {
			if store == support.store {
				return toolTextResponse(id, "", fmt.Errorf("%s: %s", tool, wstrackerhost.RedactCredentials(err.Error())))
			}
			continue // another store that cannot be read holds nothing to show
		}
		if _, ok := v.Items[item]; !ok {
			continue
		}
		if store == support.store {
			return toolTextResponse(id, formatTrackerCard(v, store, item), nil)
		}
		return toolTextResponse(id, fmt.Sprintf("%s is an item of store %s, not of this conversation's store %s, so no card is shown; answer from this text.\n%s",
			trackerAlias(item), store, support.store, formatTrackerCardFields(v, item)), nil)
	}
	return toolErrorTextResponse(id, fmt.Sprintf("%s: %s is not an item of any registered store; find its ID with tracker.query", tool, trackerAlias(item)))
}

// trackerStoreView is store's redacted view after its pre-read check.
func trackerStoreView(ctx context.Context, h *wstrackerhost.Host, store string) (*wstrackerhost.StoreView, error) {
	if err := h.PreRead(ctx, store); err != nil {
		return nil, err
	}
	v, err := h.View(ctx, store, 0)
	if err != nil {
		return nil, err
	}
	v.Redact()
	return v, nil
}

// formatTrackerCard is the card result: the card line the chat draws,
// then the card's fields for the agent, and a reminder that the user
// already sees them.
func formatTrackerCard(v *wstrackerhost.StoreView, store, id string) string {
	return fmt.Sprintf("%s%s %s\n%sThe chat shows this item as a card the user can open; refer to it rather than restating its fields.\n",
		trackerCardPrefix, store, trackerAlias(id), formatTrackerCardFields(v, id))
}

// formatTrackerCardFields is the mini card's fields, one per line: the
// item, its status, priority, and due date, and its open assignees.
func formatTrackerCardFields(v *wstrackerhost.StoreView, id string) string {
	it := v.Items[id]
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s %q\n", it.Alias, it.Kind, it.Title)
	var fields []string
	if st := trackerItemStatus(it); st != "" {
		fields = append(fields, "status "+st)
	}
	if it.Hidden {
		fields = append(fields, "hidden")
	}
	fields = append(fields, fmt.Sprintf("priority %d (%s)", it.PriorityLevel, it.PriorityName))
	due := "none"
	if it.Due != nil {
		due = *it.Due
	}
	fields = append(fields, "due "+due)
	sb.WriteString(strings.Join(fields, ", ") + "\n")
	var assignees []string
	for _, aid := range it.Assignments {
		if a, ok := v.Assignments[aid]; ok && a.Ended == nil {
			state := a.State
			if a.State == "live" {
				state = a.Status
			}
			assignees = append(assignees, trackerPerson(v, a.To)+" "+state)
		}
	}
	if len(assignees) == 0 {
		sb.WriteString("assignees: none\n")
	} else {
		sb.WriteString("assignees: " + strings.Join(assignees, ", ") + "\n")
	}
	return sb.String()
}
