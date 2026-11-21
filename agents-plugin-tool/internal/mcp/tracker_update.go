// Tracker update dispatch and store registry commands.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

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
	b := planTrackerBatch(&problems, commands, registered, func(store string) (map[string]int, error) {
		v, err := h.View(ctx, store, 0)
		if err != nil {
			return nil, err
		}
		return v.Viewer.Reach, nil
	})
	if support != nil {
		supportPinProblems(&problems, b, support.store)
	}
	if len(problems.List) > 0 {
		return toolErrorTextResponse(id, s.trackerRefusal(tool, problems.List))
	}
	var text string
	storeCmd := ""
	if b.storeCmd != nil {
		storeCmd = strings.TrimPrefix(b.storeCmd.Tag, trackerPrefix)
	}
	switch storeCmd {
	case trackerRegister:
		text, err = trackerRegisterCommand(ctx, h, *b.storeCmd)
	case trackerSync:
		text, err = trackerSyncCommand(ctx, h, *b.storeCmd, registered)
	case trackerUnregister:
		text, err = trackerUnregisterCommand(ctx, h, *b.storeCmd, registered)
	case trackerRelabel:
		text, err = trackerRelabelCommand(h, *b.storeCmd)
	case trackerHide, trackerUnhide:
		text, err = trackerHideCommand(h, *b.storeCmd, storeCmd == trackerHide)
	default:
		var ids []wsledger.EventID
		ids, err = h.DispatchWith(ctx, b.store, b.newEvents)
		var refusal *wstracker.Refusal
		if errors.As(err, &refusal) && refusal.Index >= 0 && refusal.Index < len(b.events) {
			ev := b.events[refusal.Index]
			what := "this write"
			switch {
			case ev.selfAssign:
				what = "the self-assignment written with this create"
			case ev.splitOff:
				what = fmt.Sprintf("the targets of this assign split off with completion %v", ev.payload["completion"])
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
				case b.events[i].splitOff:
					fmt.Fprintf(&sb, "%d %s %s (split off: completion %v)\n", b.events[i].cmd.Pos, b.events[i].cmd.Tag, trackerAlias(eventID.String()), b.events[i].payload["completion"])
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

// trackerUnregisterCommand removes a store from this machine: its registry
// entry and its local clone. The remote, when there is one, is left as it
// is; a store with no remote holds its only copy here, so the host removes
// it only when confirm names it. A refusal leaves the store as it was.
func trackerUnregisterCommand(ctx context.Context, h *wstrackerhost.Host, cmd wsxmlcmd.Command, registered []wstrackerhost.StoreEntry) (string, error) {
	store, _ := cmd.Attr("store")
	confirm, _ := cmd.Attr("confirm")
	var entry wstrackerhost.StoreEntry
	for _, e := range registered {
		if e.ID == store {
			entry = e
		}
	}
	err := h.Unregister(ctx, store, confirm)
	var refusal *wstrackerhost.UnregisterRefusal
	if errors.As(err, &refusal) {
		if refusal.Kind == wstrackerhost.UnregisterNeedsConfirm {
			return "", fmt.Errorf("%s: %q was not unregistered: %s. Its data is lost permanently; only when the user asked for that, send <%s%s store=%q confirm=%q/>", cmd.Label(), store, refusal.Reason, trackerPrefix, trackerUnregister, store, store)
		}
		return "", fmt.Errorf("%s: %q was not unregistered: %s", cmd.Label(), store, refusal.Reason)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w. Nothing was removed", cmd.Label(), err)
	}
	if entry.Remote == "" {
		return fmt.Sprintf("unregistered %s: removed from this machine, and its data deleted (it had no remote)\n", store), nil
	}
	return fmt.Sprintf("unregistered %s: removed from this machine, its local copy deleted; the remote %s was left as it is, so <%s%s> can register the store again\n", store, wstrackerhost.RedactCredentials(entry.Remote), trackerPrefix, trackerRegister), nil
}

// trackerRelabelCommand sets a store's display label on this machine;
// label="" clears it back to the store id.
func trackerRelabelCommand(h *wstrackerhost.Host, cmd wsxmlcmd.Command) (string, error) {
	store, _ := cmd.Attr("store")
	label, _ := cmd.Attr("label")
	e, err := h.Relabel(store, label)
	if errors.Is(err, wstrackerhost.ErrBadLabel) {
		return "", fmt.Errorf("%s: %w; at most %d characters and no control character. Nothing changed", cmd.At("label"), err, wstrackerhost.MaxLabelLen)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w. Nothing changed", cmd.Label(), err)
	}
	if e.Label == "" {
		return fmt.Sprintf("relabeled %s: label cleared, shown as its id\n", e.ID), nil
	}
	return fmt.Sprintf("relabeled %s: label %q\n", e.ID, e.Label), nil
}

// trackerHideCommand hides or shows a store on this machine's dashboard; a
// hidden store keeps working for every tracker read and write.
func trackerHideCommand(h *wstrackerhost.Host, cmd wsxmlcmd.Command, hidden bool) (string, error) {
	store, _ := cmd.Attr("store")
	e, err := h.SetHidden(store, hidden)
	if err != nil {
		return "", fmt.Errorf("%s: %w. Nothing changed", cmd.Label(), err)
	}
	if hidden {
		return fmt.Sprintf("hid %s: the dashboard leaves it out of its store list on this machine; the tracker tools still read and write it\n", e.ID), nil
	}
	return fmt.Sprintf("unhid %s: the dashboard lists it again\n", e.ID), nil
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
