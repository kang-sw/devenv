package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsreview"
)

// gitStatusResult keeps the generic git observation intact while allowing the
// MCP layer to attach workflow context. wsgit deliberately has no wsdoc
// dependency, so ticket ownership is resolved here rather than in wsgit.
type gitStatusResult struct {
	wsgit.StatusResult
	ImplTicket *implTicketStatus `json:"impl_ticket,omitempty"`
	// Leases lists the tickets the ticket index shows leased to this
	// worktree's owner triple; absent when the project has no index.
	Leases []string `json:"leases,omitempty"`
}

type implTicketStatus struct {
	State  string `json:"state"`
	Stem   string `json:"stem,omitempty"`
	Path   string `json:"path,omitempty"`
	Status string `json:"status,omitempty"`
	// Owner is the active ticket's lease from the ticket index (cache only);
	// absent when the project has no index.
	Owner *wsdoc.TicketOwnership `json:"owner,omitempty"`
}

func formatGitStatus(result wsgit.StatusResult) string {
	var b strings.Builder
	head := displayOrDash(result.Branch.Head)
	oid := shortHash(result.Branch.OID)
	if oid != "" {
		fmt.Fprintf(&b, "%s %s", head, oid)
	} else {
		fmt.Fprintf(&b, "%s", head)
	}
	if result.Branch.Upstream != "" {
		fmt.Fprintf(&b, " %s", result.Branch.Upstream)
	}
	if result.Branch.Ahead != 0 || result.Branch.Behind != 0 {
		fmt.Fprintf(&b, " ahead=%d behind=%d", result.Branch.Ahead, result.Branch.Behind)
	}
	if result.Clean {
		b.WriteString(" clean\n")
		return b.String()
	}
	fmt.Fprintf(&b, " dirty: %d files\n", len(result.ChangedFiles))
	for _, file := range result.ChangedFiles {
		status := file.Status
		if status == "" {
			status = file.IndexStatus + file.WorktreeStatus
		}
		if status == "" {
			status = "??"
		}
		if file.OldPath != "" {
			fmt.Fprintf(&b, "%s %s <- %s\n", status, file.Path, file.OldPath)
		} else {
			fmt.Fprintf(&b, "%s %s\n", status, file.Path)
		}
	}
	return b.String()
}

func formatGitStatusWithImplTicket(result wsgit.StatusResult, implTicket *implTicketStatus) string {
	text := formatGitStatus(result)
	if implTicket == nil {
		return text
	}
	switch implTicket.State {
	case "active":
		return text + fmt.Sprintf("active ticket: %s (%s)\n", implTicket.Stem, implTicket.Status)
	case "missing":
		return text + "nudge: current branch is impl/* but no active ticket matches; inspect before selecting another ticket\n"
	case "ambiguous":
		return text + "nudge: current branch is impl/* but multiple active tickets match; inspect before selecting another ticket\n"
	default:
		return text
	}
}

// activeImplTicket finds an active ticket only for a name-rooted implementation
// branch. Resolve mode includes active tickets hidden by a sparse checkout; an
// unreadable inventory is unsafe to label missing and therefore returns error.
func activeImplTicket(root, branch string, env []string) (*implTicketStatus, error) {
	_, suffix, ok := parseImplBranchRoot(branch)
	if !ok {
		return nil, nil
	}
	if err := verifyActiveTicketInventory(root); err != nil {
		return nil, err
	}
	candidates, err := wsdoc.TicketsFind(root, wsdoc.TicketFindOptions{
		Env:      env,
		Statuses: []string{"idea", "todo", "ready"}, Resolve: true,
	})
	if err != nil {
		return nil, fmt.Errorf("git.status: inspect active ticket inventory: %w", err)
	}
	stems := make([]string, 0, len(candidates))
	byStem := make(map[string]wsdoc.TicketInfo, len(candidates))
	for _, candidate := range candidates {
		stems = append(stems, candidate.Stem)
		byStem[candidate.Stem] = candidate
	}
	matches := matchImplTicketStems(suffix, stems)
	switch len(matches) {
	case 0:
		return &implTicketStatus{State: "missing"}, nil
	case 1:
		match := byStem[matches[0]]
		return &implTicketStatus{State: "active", Stem: match.Stem, Path: match.Path, Status: match.Status}, nil
	default:
		return &implTicketStatus{State: "ambiguous"}, nil
	}
}

// verifyActiveTicketInventory closes the gap in wsdoc's discovery-oriented
// walk, where an unreadable directory can look like an empty one. Missing
// individual status directories are normal; an absent board or an I/O error is
// not evidence that an impl branch has no owner.
func verifyActiveTicketInventory(root string) error {
	board := filepath.Join(root, "ai-docs", "tickets")
	if _, err := os.Stat(board); err != nil {
		return fmt.Errorf("git.status: inspect active ticket inventory: %w", err)
	}
	for _, status := range []string{"idea", "todo", "ready"} {
		_, err := os.ReadDir(filepath.Join(board, status))
		if err == nil || os.IsNotExist(err) {
			continue
		}
		return fmt.Errorf("git.status: inspect active ticket inventory: %w", err)
	}
	return nil
}

func formatGitLog(result wsgit.LogResult) string {
	var b strings.Builder
	if result.Range != "" {
		fmt.Fprintf(&b, "range: %s\n", result.Range)
	}
	if len(result.Commits) == 0 {
		return b.String()
	}
	for i, commit := range result.Commits {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "commit %s\n", commit.Hash)
		if commit.Subject != "" {
			fmt.Fprintf(&b, "subject: %s\n", commit.Subject)
		}
		if commit.Author != "" {
			fmt.Fprintf(&b, "author: %s\n", commit.Author)
		}
		if commit.Date != "" {
			fmt.Fprintf(&b, "date: %s\n", commit.Date)
		}
		if strings.TrimSpace(commit.Body) != "" {
			b.WriteString("\n")
			b.WriteString(strings.TrimSpace(commit.Body))
			b.WriteString("\n")
		}
	}
	return b.String()
}

func formatMergeBase(result wsgit.MergeBaseResult) string {
	return fmt.Sprintf("merge_base: %s (%s %s)\n", result.MergeBase, result.Base, result.Head)
}

func formatGitCommit(result wsgit.CommitResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "commit: %s\n", result.Hash)
	fmt.Fprintf(&b, "title: %s\n", result.Title)
	if len(result.Paths) > 0 {
		b.WriteString("paths:\n")
		for _, path := range result.Paths {
			fmt.Fprintf(&b, "  - %s\n", path)
		}
	}
	if len(result.TicketChanges) > 0 {
		b.WriteString("ticket_changes:\n")
		for _, change := range result.TicketChanges {
			fmt.Fprintf(&b, "  - %s", change.Stem)
			if change.FromStatus != "" || change.ToStatus != "" {
				fmt.Fprintf(&b, " %s->%s", displayOrDash(change.FromStatus), displayOrDash(change.ToStatus))
			}
			if change.ResultAdded {
				b.WriteString(" result")
				if change.ResultHeading != "" {
					fmt.Fprintf(&b, " %q", change.ResultHeading)
				}
			}
			if change.Path != "" {
				fmt.Fprintf(&b, " (%s)", change.Path)
			}
			b.WriteString("\n")
		}
	}
	if len(result.Followups) > 0 {
		b.WriteString("followups:\n")
		for _, f := range result.Followups {
			fmt.Fprintf(&b, "  - %s %s/%s: %s\n", f.ID, f.Level, f.Category, f.Content)
		}
	}
	if len(result.Warnings) > 0 {
		b.WriteString("warnings:\n")
		for _, warning := range result.Warnings {
			fmt.Fprintf(&b, "  - %s\n", warning)
		}
	}
	if len(result.Advisories) > 0 {
		b.WriteString("advisories:\n")
		for i, advisory := range result.Advisories {
			// Advisories are separated by a blank line. Without it a
			// multi-line block (a "## Parent Board" section) and the
			// single-block advisory after it run together as one wall of
			// text, which is only reachable now that Phase 2 puts both kinds
			// on this channel at once.
			if i > 0 {
				b.WriteString("\n")
			}
			// Each advisory may itself be multi-line; indent every line
			// consistently rather than only the first, so a multi-line
			// advisory does not lose its indentation on line 2+.
			for _, line := range strings.Split(advisory, "\n") {
				fmt.Fprintf(&b, "  %s\n", line)
			}
		}
	}
	return b.String()
}

// reviewRefSuffix renders a ledger entry's optional routed-ticket-stem Ref
// as its display-text " -> <ref>" tail, or "" when Ref is empty.
func reviewRefSuffix(ref string) string {
	if ref == "" {
		return ""
	}
	return " -> " + ref
}

// reviewMarkerJSON is review.marker's format=json structured output: the
// bare-SHA/structured frontier head a caller (the lead-ship gate, in
// particular) consumes without string-scraping the text-mode response.
type reviewMarkerJSON struct {
	Base    string `json:"base"`
	Head    string `json:"head"`
	Verdict string `json:"verdict"`
	Ref     string `json:"ref,omitempty"`
	Found   bool   `json:"found"`
}

func reviewMarkerJSONValue(entry wsreview.Entry, found bool) reviewMarkerJSON {
	return reviewMarkerJSON{Base: entry.Base, Head: entry.Head, Verdict: entry.Verdict, Ref: entry.Ref, Found: found}
}
