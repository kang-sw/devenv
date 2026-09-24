package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsindex"
)

// ownershipView is the caller's view of the ticket index: the cached index
// with this clone's pending entries overlaid, resolved against the caller's
// owner triple and the local worktrees. It is nil in an index-absent project.
type ownershipView struct {
	state        wsindex.ViewState
	age          time.Duration
	pending      int
	reports      []string
	caller       wsindex.Owner
	leases       map[string]*wsindex.Lease // overlaid; nil lease means unowned
	provisional  map[string]bool
	originClosed map[string]bool
	worktrees    map[string]string // track -> local worktree path (this clone)
	overlay      *wsindex.Index
}

// loadOwnershipView reads the index for display. cachedOnly never contacts
// the remote (git.status); otherwise the read path's TTL and short fetch
// timeout apply (tickets.query, move/close guards).
func (s *Server) loadOwnershipView(root string, cachedOnly bool) *ownershipView {
	cl := s.openTicketIndex(root)
	if cl == nil {
		return nil
	}
	ctx := context.Background()
	var view wsindex.View
	var err error
	if cachedOnly {
		view, err = cl.ReadCached(ctx)
	} else {
		view, err = cl.Read(ctx)
	}
	if err != nil || view.State == wsindex.ViewAbsent {
		if err == nil && len(view.Reports) > 0 {
			// A discard happened on this read: the index is gone, but the
			// one-time report still belongs in this call's output.
			return &ownershipView{state: wsindex.ViewAbsent, reports: view.Reports}
		}
		return nil
	}
	caller := resolveIndexCaller(ctx, root, "")
	caller.owner.CloneID = cl.ExistingCloneID(ctx)
	v := &ownershipView{
		state:        view.State,
		age:          view.Age,
		pending:      len(view.Pending),
		reports:      view.Reports,
		caller:       caller.owner,
		leases:       map[string]*wsindex.Lease{},
		provisional:  map[string]bool{},
		originClosed: cl.OriginTicketsAt(ctx, cl.OriginTrack(ctx)).Closed,
		worktrees:    localTrackWorktrees(ctx, root),
	}
	if view.State == wsindex.ViewUnknown {
		return v
	}
	applier := &wsindex.Applier{Now: s.indexNow(), Closed: v.originClosed}
	overlay, err := cl.Overlay(ctx, applier)
	if err != nil {
		return v
	}
	v.overlay = overlay
	for stem, reg := range overlay.Registrations {
		v.leases[stem] = reg.Lease
		var base *wsindex.Lease
		if view.Index != nil {
			if b := view.Index.Registrations[stem]; b != nil {
				base = b.Lease
			}
		}
		if reg.Lease != nil && (base == nil || !wsindex.SameOwner(base.Owner(), reg.Lease.Owner())) {
			v.provisional[stem] = true
		}
	}
	return v
}

// localTrackWorktrees maps each worktree's track (its branch's merge root) to
// its path. A worktree on the track branch itself wins over an impl branch
// rooted there.
func localTrackWorktrees(ctx context.Context, root string) map[string]string {
	entries, err := listWorktrees(ctx, wsgit.ExecRunner{}, root)
	if err != nil {
		return nil
	}
	tracks := map[string]string{}
	exact := map[string]bool{}
	for _, w := range entries {
		branch := strings.TrimPrefix(w.Branch, "refs/heads/")
		if branch == "" || branch == w.Branch {
			continue
		}
		track := implementMergeRootFor(branch)
		if track == "" || exact[track] {
			continue
		}
		if branch == track {
			exact[track] = true
			tracks[track] = w.Path
		} else if _, ok := tracks[track]; !ok {
			tracks[track] = w.Path
		}
	}
	return tracks
}

// ownership resolves one stem for display and filtering.
func (v *ownershipView) ownership(stem, status string) *wsdoc.TicketOwnership {
	o := &wsdoc.TicketOwnership{Level: wsdoc.OwnershipUnowned, IndexState: string(v.state)}
	if v.state == wsindex.ViewStale && v.age >= 0 {
		o.CacheAgeSeconds = int(v.age.Seconds())
	}
	if v.originClosed[stem] && (status == "idea" || status == "todo" || status == "ready") {
		o.OriginClosed = true
	}
	if v.state == wsindex.ViewUnknown {
		o.Level = wsdoc.OwnershipUnknown
		return o
	}
	lease := v.leases[stem]
	if lease == nil {
		return o
	}
	holder := lease.Owner()
	o.Email, o.Track, o.Phase = lease.Email, lease.Track, lease.Phase
	o.TouchedAt = lease.TouchedAt.UTC().Format(time.RFC3339)
	if lease.Impl != nil {
		o.ImplBranch = lease.Impl.Branch
	}
	o.Provisional = v.provisional[stem]
	switch {
	case wsindex.SameOwner(holder, v.caller):
		o.Level = wsdoc.OwnershipSelf
	case v.caller.CloneID != "" && holder.CloneID == v.caller.CloneID && strings.EqualFold(holder.Email, v.caller.Email):
		o.Level = wsdoc.OwnershipLocal
		o.Worktree = v.worktrees[holder.Track]
	default:
		o.Level = wsdoc.OwnershipRemote
	}
	return o
}

// availableToCaller is the ownership filter: the caller's own and unowned
// tickets, plus unknown ones (acquire is the authoritative gate). A ticket
// closed on origin is excluded.
func availableToCaller(o *wsdoc.TicketOwnership) bool {
	if o == nil {
		return true
	}
	if o.OriginClosed {
		return false
	}
	switch o.Level {
	case wsdoc.OwnershipSelf, wsdoc.OwnershipUnowned, wsdoc.OwnershipUnknown:
		return true
	}
	return false
}

func annotateOwnership(v *ownershipView, tickets []wsdoc.TicketInfo) {
	if v == nil || v.state == wsindex.ViewAbsent {
		return
	}
	for i := range tickets {
		tickets[i].Ownership = v.ownership(tickets[i].Stem, tickets[i].Status)
	}
}

// ownershipLine renders one ticket's ownership for compact text; "" for the
// quiet case (unowned, fresh, not closed on origin).
func ownershipLine(o *wsdoc.TicketOwnership) string {
	if o == nil {
		return ""
	}
	var parts []string
	switch o.Level {
	case wsdoc.OwnershipSelf:
		parts = append(parts, fmt.Sprintf("yours (track %s)", o.Track))
	case wsdoc.OwnershipLocal:
		where := "another track of this clone"
		if o.Worktree != "" {
			where = "another worktree of this clone: " + o.Worktree
		}
		parts = append(parts, fmt.Sprintf("held by %s (track %s)", where, o.Track))
	case wsdoc.OwnershipRemote:
		parts = append(parts, fmt.Sprintf("held by %s (track %s)", o.Email, o.Track))
	case wsdoc.OwnershipUnknown:
		parts = append(parts, "unknown (origin unreachable and no cached index)")
	}
	if o.Level == wsdoc.OwnershipSelf || o.Level == wsdoc.OwnershipLocal || o.Level == wsdoc.OwnershipRemote {
		if o.Phase == wsindex.PhaseClosed {
			parts = append(parts, "phase closed (pending landing)")
		}
		if o.ImplBranch != "" {
			parts = append(parts, "impl "+o.ImplBranch)
		}
		parts = append(parts, "since "+o.TouchedAt)
		if o.Provisional {
			parts = append(parts, "provisional: pending offline acquire")
		}
	}
	if o.OriginClosed {
		parts = append(parts, "closed on origin (this checkout is stale; pull)")
	}
	if len(parts) == 0 {
		return ""
	}
	return "  ownership: " + strings.Join(parts, "; ") + "\n"
}

// formatTicketsWithOwnership renders compact text. Discovery collapses
// tickets held by others (and those closed on origin) to one line each after
// the full entries; point-resolve never collapses.
func formatTicketsWithOwnership(v *ownershipView, tickets []wsdoc.TicketInfo, collapse bool) string {
	var b strings.Builder
	var collapsed []wsdoc.TicketInfo
	for _, t := range tickets {
		if collapse && t.Ownership != nil && !availableToCaller(t.Ownership) {
			collapsed = append(collapsed, t)
			continue
		}
		b.WriteString(formatTickets([]wsdoc.TicketInfo{t}))
		b.WriteString(ownershipLine(t.Ownership))
	}
	if len(collapsed) > 0 {
		fmt.Fprintf(&b, "held elsewhere or closed on origin (%d):\n", len(collapsed))
		for _, t := range collapsed {
			fmt.Fprintf(&b, "  [%s] %s -%s", t.Status, t.Stem, strings.TrimPrefix(ownershipLine(t.Ownership), "  ownership:"))
		}
	}
	b.WriteString(v.trailer())
	return b.String()
}

// trailer renders the index-wide state lines: discard reports, stale cache,
// and the pending-count marker.
func (v *ownershipView) trailer() string {
	if v == nil {
		return ""
	}
	var b strings.Builder
	for _, r := range v.reports {
		b.WriteString(indexReportLine(r) + "\n")
	}
	if v.state == wsindex.ViewStale {
		age := "unknown age"
		if v.age >= 0 {
			age = "age " + v.age.Truncate(time.Second).String()
		}
		fmt.Fprintf(&b, "ticket-index: origin unreachable; ownership is from the cached index (%s)\n", age)
	}
	if v.pending > 0 {
		fmt.Fprintf(&b, "ticket-index: %d pending offline entries; the next ticket tool that reaches origin flushes them\n", v.pending)
	}
	return b.String()
}

// ---- move / close guard ------------------------------------------------------

// indexGuard is the owner-conflict check move and close run before touching
// the folder. It evaluates the overlaid cached view (online or offline); a
// lease held by a different email needs the override flag and a reason, the
// other rows proceed with a warning. An index-absent project skips it and
// ignores the override params.
type indexGuard struct {
	override *wsindex.Override
	warning  string
	reports  []string // discard reports from the guard's own read
}

func (s *Server) guardMoveClose(root, tool, stem string, args map[string]any) (indexGuard, error) {
	v := s.loadOwnershipView(root, false)
	if v == nil {
		return indexGuard{}, nil
	}
	g := indexGuard{reports: v.reports}
	if v.overlay == nil {
		return g, nil
	}
	lease := v.leases[strings.TrimSpace(stem)]
	if lease == nil {
		return g, nil
	}
	holder := lease.Owner()
	if wsindex.SameOwner(holder, v.caller) {
		return g, nil
	}
	flag := boolArgument(args["dangerously_override_lease_status"])
	reason, _ := args["reason"].(string)
	reason = strings.TrimSpace(reason)
	// A move rides the piggyback registration; its matrix row is register's.
	op := wsindex.OpClose
	if tool == "move" {
		op = wsindex.OpRegister
	}
	if wsindex.NeedsOverride(op, holder, v.caller) {
		if !flag {
			return indexGuard{}, fmt.Errorf("tickets.%s refused: %s is held by %s since %s; proceeding needs dangerously_override_lease_status: true with a non-empty reason, set only on the user's explicit instruction",
				tool, stem, holderText(holder), lease.TouchedAt.UTC().Format(time.RFC3339))
		}
		if reason == "" {
			return indexGuard{}, fmt.Errorf("tickets.%s: dangerously_override_lease_status needs a non-empty reason", tool)
		}
		g.override = &wsindex.Override{Holder: holder, Reason: reason}
		return g, nil
	}
	g.warning = fmt.Sprintf("ticket-index: %s is held by %s; the lease stays with its holder", stem, holderText(holder))
	return g, nil
}

func (g indexGuard) text() string {
	var b strings.Builder
	for _, r := range g.reports {
		b.WriteString(indexReportLine(r) + "\n")
	}
	if g.warning != "" {
		b.WriteString(g.warning + "\n")
	}
	return b.String()
}

// callerLeases lists the stems leased to the caller's own triple.
func (v *ownershipView) callerLeases() []string {
	var out []string
	for stem, lease := range v.leases {
		if lease != nil && wsindex.SameOwner(lease.Owner(), v.caller) {
			out = append(out, stem)
		}
	}
	sort.Strings(out)
	return out
}

// formatIndexStatus renders git.status's ticket index lines.
func formatIndexStatus(implTicket *implTicketStatus, leases []string) string {
	var b strings.Builder
	if implTicket != nil && implTicket.Owner != nil {
		if line := ownershipLine(implTicket.Owner); line != "" {
			b.WriteString("ticket owner:" + strings.TrimPrefix(line, "  ownership:"))
		} else {
			b.WriteString("ticket owner: none (unleased)\n")
		}
	}
	if len(leases) > 0 {
		fmt.Fprintf(&b, "leased to this track: %s\n", strings.Join(leases, ", "))
	}
	return b.String()
}
