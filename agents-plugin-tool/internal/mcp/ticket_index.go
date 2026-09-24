package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsindex"
)

// The ticket ownership index is an opt-in overlay on origin. Every entry
// point here keeps the "no ref, no change" contract: when the index is absent
// (no origin, no ref, cached absence, or an unreachable remote on a clone that
// never saw an index) the ticket tools behave and print exactly as before,
// and tickets.acquire / tickets.release answer a plain "ok" with no
// validation. All validation therefore runs inside the index's Prepare hook,
// which fires only once the index is known to be in use.

const indexMockText = "ok"

func (s *Server) indexNow() time.Time {
	if s.indexOpts.Now != nil {
		return s.indexOpts.Now()
	}
	return time.Now()
}

// openTicketIndex binds the index client for root; nil when root is not a git
// checkout (which is index-absent by definition).
func (s *Server) openTicketIndex(root string) *wsindex.Client {
	cl, err := wsindex.Open(context.Background(), root, s.indexOpts)
	if err != nil {
		return nil
	}
	return cl
}

// indexCaller is the caller's identity as seen from its checkout. CloneID is
// filled by the submission's Prepare hook, so an index-absent project never
// gets a clone id written.
type indexCaller struct {
	owner  wsindex.Owner
	branch string // "" on a detached HEAD
}

func resolveIndexCaller(ctx context.Context, root, explicitTrack string) indexCaller {
	c := indexCaller{owner: wsindex.Owner{Email: wsgit.CurrentUserEmail(ctx, wsgit.ExecRunner{}, root)}}
	if out, err := (wsgit.ExecRunner{}).RunGit(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil {
		c.branch = strings.TrimSpace(string(out))
	}
	if track := strings.TrimSpace(explicitTrack); track != "" {
		c.owner.Track = track
	} else if c.branch != "" {
		// One level only: the caller's merge root. A rootless impl/<stem> or
		// any implement/<stem> resolves to "" and so matches no lease.
		c.owner.Track = implementMergeRootFor(c.branch)
	}
	return c
}

// implBranchOf returns the branch to record as the lease's impl when the
// caller works on an impl branch.
func (c indexCaller) implBranch() string {
	if strings.HasPrefix(c.branch, "impl/") || strings.HasPrefix(c.branch, "implement/") {
		return c.branch
	}
	return ""
}

// acquireTrackError refuses an acquire whose track cannot be resolved.
func (c indexCaller) acquireTrackError(explicitTrack string) error {
	if c.branch == "" {
		return fmt.Errorf("tickets.acquire: HEAD is detached; acquire from a branch (the lease records the branch's work line as its track)")
	}
	if strings.TrimSpace(explicitTrack) == "" && c.owner.Track == "" {
		return fmt.Errorf("tickets.acquire: branch %s has no merge root to record as the lease's track; pass track explicitly", c.branch)
	}
	return nil
}

func ticketFileExists(root, stem string) bool {
	for _, dir := range []string{"idea", "todo", "ready", ".done", ".dropped"} {
		if info, err := os.Stat(filepath.Join(root, "ai-docs", "tickets", dir, stem+".md")); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func indexReportLine(report string) string {
	if strings.HasPrefix(report, "ticket-index:") {
		return report
	}
	return "ticket-index: " + report
}

func offlineWarning(op, stem string) string {
	return fmt.Sprintf("ticket-index: origin is unreachable; the %s of %s is recorded in this clone's pending log, unverified against origin, and is re-checked when a later ticket tool reaches origin", op, stem)
}

func holderText(o wsindex.Owner) string {
	return fmt.Sprintf("%s (track %s, clone %s)", o.Email, o.Track, o.CloneID)
}

// ---- piggyback registration --------------------------------------------------

// indexPiggyback submits one index write alongside a successful host
// operation (registration, or close's closed lease) and returns the extra
// output lines. It never fails the host operation: an index failure becomes
// one line, and an index-absent project gets "" so the host output is
// unchanged byte for byte.
func (s *Server) indexPiggyback(root, op, stem string, override *wsindex.Override) string {
	stem = strings.TrimSpace(stem)
	if stem == "" {
		return ""
	}
	cl := s.openTicketIndex(root)
	if cl == nil {
		return ""
	}
	ctx := context.Background()
	caller := resolveIndexCaller(ctx, root, "")
	sub := &wsindex.Submission{
		Entry:   wsindex.PendingEntry{Op: op, Stem: stem, Owner: caller.owner, Worktree: root, Override: override},
		Applier: &wsindex.Applier{Now: s.indexNow()},
	}
	sub.Prepare = func(ctx context.Context, online bool) error {
		return cl.LoadContext(ctx, sub, online, false)
	}
	res, outcome, err := cl.Submit(ctx, sub)
	var b strings.Builder
	for _, r := range res.Reports {
		b.WriteString(indexReportLine(r) + "\n")
	}
	switch {
	case err != nil:
		fmt.Fprintf(&b, "ticket-index: the %s of %s was not recorded in the index: %v\n", op, stem, err)
	case res.Status == wsindex.WritePending:
		b.WriteString(offlineWarning(op, stem) + "\n")
	}
	if outcome.Warning != "" {
		b.WriteString(indexReportLine(outcome.Warning) + "\n")
	}
	return b.String()
}

// ---- tickets.acquire / tickets.release ----------------------------------------

type indexVerbResult struct {
	Status     string   `json:"status"`
	TicketStem string   `json:"ticket_stem,omitempty"`
	Owner      any      `json:"owner,omitempty"`
	ImplBranch string   `json:"impl_branch,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
	Reports    []string `json:"reports,omitempty"`
}

func indexVerbResponse(id json.RawMessage, args map[string]any, r indexVerbResult) response {
	if wantsJSON(args) {
		return toolJSONResponse(id, r, nil)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", r.Status)
	if r.TicketStem != "" {
		fmt.Fprintf(&b, "ticket_stem: %s\n", r.TicketStem)
	}
	if o, ok := r.Owner.(wsindex.Owner); ok {
		fmt.Fprintf(&b, "owner: %s\n", holderText(o))
	}
	if r.ImplBranch != "" {
		fmt.Fprintf(&b, "impl_branch: %s\n", r.ImplBranch)
	}
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "warning: %s\n", w)
	}
	for _, rep := range r.Reports {
		fmt.Fprintf(&b, "report: %s\n", rep)
	}
	return toolTextResponse(id, b.String(), nil)
}

func indexMockResponse(id json.RawMessage, args map[string]any) response {
	if wantsJSON(args) {
		return toolJSONResponse(id, map[string]string{"status": "ok"}, nil)
	}
	return toolTextResponse(id, indexMockText, nil)
}

func indexErrorResponse(id json.RawMessage, err error, reports []string) response {
	text := err.Error()
	for _, r := range reports {
		text += "\nreport: " + indexReportLine(r)
	}
	return toolErrorTextResponse(id, text)
}

func (s *Server) handleTicketsAcquire(id json.RawMessage, args, meta map[string]any) response {
	return s.handleIndexVerb(id, args, meta, wsindex.OpAcquire)
}

func (s *Server) handleTicketsRelease(id json.RawMessage, args, meta map[string]any) response {
	return s.handleIndexVerb(id, args, meta, wsindex.OpRelease)
}

func (s *Server) handleIndexVerb(id json.RawMessage, args, meta map[string]any, op string) response {
	tool := "tickets." + op
	root, err := s.resolveToolRoot(args, meta)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	cl := s.openTicketIndex(root)
	if cl == nil {
		return indexMockResponse(id, args)
	}
	stem, _ := args["ticket_stem"].(string)
	stem = strings.TrimSpace(stem)
	explicitTrack := ""
	if op == wsindex.OpAcquire {
		explicitTrack, _ = args["track"].(string)
	}
	override := op == wsindex.OpAcquire && boolArgument(args["dangerously_override_lease_status"])
	reason, _ := args["reason"].(string)
	reason = strings.TrimSpace(reason)

	ctx := context.Background()
	caller := resolveIndexCaller(ctx, root, explicitTrack)
	entry := wsindex.PendingEntry{Op: op, Stem: stem, Owner: caller.owner, Worktree: root}
	if op == wsindex.OpAcquire {
		entry.ImplBranch = caller.implBranch()
		if override {
			entry.Override = &wsindex.Override{Reason: reason}
		}
	}
	sub := &wsindex.Submission{Entry: entry, Applier: &wsindex.Applier{Now: s.indexNow()}}
	sub.Prepare = func(ctx context.Context, online bool) error {
		if !wsindex.ValidStem(stem) {
			return fmt.Errorf("%s: ticket_stem must be a ticket stem (YYMMDD-category-name)", tool)
		}
		if op == wsindex.OpAcquire {
			if override && reason == "" {
				return fmt.Errorf("%s: dangerously_override_lease_status needs a non-empty reason", tool)
			}
			if err := caller.acquireTrackError(explicitTrack); err != nil {
				return err
			}
			if caller.owner.Email == "" {
				return fmt.Errorf("%s: git user.email is not set; the lease records the owner's email", tool)
			}
			if !ticketFileExists(root, stem) {
				return fmt.Errorf("%s: ticket %s does not exist in this checkout", tool, stem)
			}
		}
		return cl.LoadContext(ctx, sub, online, op == wsindex.OpAcquire)
	}
	res, outcome, err := cl.Submit(ctx, sub)
	if res.Status == wsindex.WriteAbsent {
		return indexMockResponse(id, args)
	}
	reports := make([]string, 0, len(res.Reports))
	for _, r := range res.Reports {
		reports = append(reports, indexReportLine(r))
	}
	if err != nil {
		var refusal *wsindex.RefusalError
		if errors.As(err, &refusal) {
			return indexErrorResponse(id, fmt.Errorf("%s refused: %s", tool, refusal.Message), reports)
		}
		return indexErrorResponse(id, fmt.Errorf("%s: %w", tool, err), reports)
	}
	out := indexVerbResult{Status: string(outcome.Effect), TicketStem: stem, Reports: reports}
	if outcome.Effect == wsindex.EffectNone {
		out.Status = "no_change"
	}
	if op == wsindex.OpAcquire {
		out.Owner = sub.Entry.Owner
		out.ImplBranch = sub.Entry.ImplBranch
	}
	if outcome.Warning != "" {
		out.Warnings = append(out.Warnings, outcome.Warning)
	}
	if res.Status == wsindex.WritePending {
		out.Status = "pending"
		out.Warnings = append(out.Warnings, offlineWarning(strings.TrimPrefix(tool, "tickets."), stem))
	}
	return indexVerbResponse(id, args, out)
}

// ---- tickets.index_init -------------------------------------------------------

func (s *Server) handleTicketsIndexInit(id json.RawMessage, args, meta map[string]any) response {
	root, err := s.resolveToolRoot(args, meta)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	cl := s.openTicketIndex(root)
	if cl == nil {
		return toolTextResponse(id, "", fmt.Errorf("tickets.index_init: %s is not a git checkout", root))
	}
	ctx := context.Background()
	if boolArgument(args["check"]) {
		state, checkErr := cl.Check(ctx)
		detail := ""
		if state == wsindex.CheckUnreachable && checkErr != nil {
			detail = checkErr.Error()
		}
		if wantsJSON(args) {
			value := map[string]string{"state": string(state)}
			if detail != "" {
				value["detail"] = detail
			}
			return toolJSONResponse(id, value, nil)
		}
		text := "state: " + string(state) + "\n"
		if detail != "" {
			text += "detail: " + firstLine(detail) + "\n"
		}
		return toolTextResponse(id, text, nil)
	}
	if !cl.HasOrigin(ctx) {
		return toolTextResponse(id, "", fmt.Errorf("tickets.index_init: this clone has no origin remote; the ticket index lives on origin"))
	}
	track, inventory, err := cl.InitSource(ctx)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("tickets.index_init: could not read origin: %w", err))
	}
	now := s.indexNow().UTC().Truncate(time.Second)
	initial := wsindex.NewIndex()
	initial.Meta.LastGC = &now
	for stem := range inventory.Open {
		if !inventory.Closed[stem] {
			initial.Register(stem, now)
		}
	}
	created, err := cl.Create(ctx, initial, fmt.Sprintf("ticket-index: init (%d open tickets from %s)", len(initial.Registrations), firstNonEmpty(track, "no review-track")))
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("tickets.index_init: %w", err))
	}
	status := "adopted"
	registered := 0
	if created.Created {
		status, registered = "created", len(initial.Registrations)
	}
	if wantsJSON(args) {
		return toolJSONResponse(id, map[string]any{"status": status, "registered": registered, "review_track": track}, nil)
	}
	text := fmt.Sprintf("status: %s\nreview_track: %s\n", status, firstNonEmpty(track, "(none)"))
	if created.Created {
		text += fmt.Sprintf("registered: %d open tickets\n", registered)
	} else {
		text += "note: another clone created the index first; this clone adopted it\n"
	}
	return toolTextResponse(id, text, nil)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
