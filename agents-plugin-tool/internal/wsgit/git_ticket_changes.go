// Git ticket change detection and summary inference.
package wsgit

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type TicketChange struct {
	Stem          string `json:"stem"`
	Path          string `json:"path"`
	OldPath       string `json:"old_path,omitempty"`
	FromStatus    string `json:"from_status,omitempty"`
	ToStatus      string `json:"to_status,omitempty"`
	ResultAdded   bool   `json:"result_added,omitempty"`
	ResultHeading string `json:"result_heading,omitempty"`
}

func detectTicketChanges(ctx context.Context, runner Runner, root string) []TicketChange {
	var changes []TicketChange
	if out, err := runner.RunGit(ctx, root, "diff", "--cached", "--name-status", "--", "ai-docs/tickets"); err == nil {
		changes = append(changes, parseTicketNameStatus(out)...)
	}
	if out, err := runner.RunGit(ctx, root, "diff", "--cached", "--unified=0", "--", "ai-docs/tickets"); err == nil {
		for _, change := range parseTicketResultAdditions(out) {
			changes = mergeTicketResultAddition(changes, change)
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		return ticketChangeSortKey(changes[i]) < ticketChangeSortKey(changes[j])
	})
	return changes
}

func mergeTicketResultAddition(changes []TicketChange, result TicketChange) []TicketChange {
	for i, change := range changes {
		if change.Stem == result.Stem && change.Path == result.Path {
			changes[i].ResultAdded = result.ResultAdded
			changes[i].ResultHeading = result.ResultHeading
			return changes
		}
	}
	return append(changes, result)
}

func parseTicketNameStatus(out []byte) []TicketChange {
	var changes []TicketChange
	addsByStem := map[string][]TicketChange{}
	deletesByStem := map[string][]TicketChange{}
	explicitRenameStems := map[string]bool{}
	var passthrough []TicketChange

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		code := fields[0]
		path := fields[1]
		oldPath := ""
		if strings.HasPrefix(code, "R") && len(fields) >= 3 {
			oldPath = fields[1]
			path = fields[2]
		}
		change, ok := ticketChangeForPath(path)
		if !ok {
			continue
		}
		if oldPath != "" {
			if oldChange, ok := ticketChangeForPath(oldPath); ok {
				change.OldPath = oldPath
				change.FromStatus = oldChange.ToStatus
				explicitRenameStems[change.Stem] = true
			}
			changes = append(changes, change)
			continue
		}

		switch code {
		case "A":
			addsByStem[change.Stem] = append(addsByStem[change.Stem], change)
		case "D":
			deletesByStem[change.Stem] = append(deletesByStem[change.Stem], change)
		default:
			passthrough = append(passthrough, change)
		}
	}

	pairedAdds := map[string]bool{}
	pairedDeletes := map[string]bool{}
	for stem, adds := range addsByStem {
		deletes := deletesByStem[stem]
		if explicitRenameStems[stem] || len(adds) != 1 || len(deletes) != 1 {
			continue
		}
		add := adds[0]
		deleteChange := deletes[0]
		if add.ToStatus == deleteChange.ToStatus {
			continue
		}
		add.OldPath = deleteChange.Path
		add.FromStatus = deleteChange.ToStatus
		changes = append(changes, add)
		pairedAdds[stem] = true
		pairedDeletes[stem] = true
	}

	for stem, adds := range addsByStem {
		if pairedAdds[stem] {
			continue
		}
		for _, change := range adds {
			changes = append(changes, change)
		}
	}
	for stem, deletes := range deletesByStem {
		if pairedDeletes[stem] {
			continue
		}
		for _, change := range deletes {
			changes = append(changes, change)
		}
	}
	for _, change := range passthrough {
		changes = append(changes, change)
	}
	return changes
}

func parseTicketResultAdditions(out []byte) []TicketChange {
	var changes []TicketChange
	var current TicketChange
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			current = TicketChange{}
			if change, ok := ticketChangeForPath(strings.TrimPrefix(line, "+++ b/")); ok {
				current = change
			}
			continue
		}
		if current.Stem == "" || !isAddedTicketResultHeading(line) {
			continue
		}
		change := current
		change.ResultAdded = true
		change.ResultHeading = strings.TrimPrefix(line, "+")
		changes = append(changes, change)
	}
	return changes
}

func isAddedTicketResultHeading(line string) bool {
	return strings.HasPrefix(line, "+### Result") || strings.HasPrefix(line, "+#### Edition")
}

func ticketChangeForPath(path string) (TicketChange, bool) {
	status, stem, ok := ticketStatusStem(path)
	if !ok {
		return TicketChange{}, false
	}
	return TicketChange{Stem: stem, Path: path, ToStatus: status}, true
}

func ticketStatusStem(path string) (string, string, bool) {
	normalized := filepath.ToSlash(filepath.Clean(path))
	const prefix = "ai-docs/tickets/"
	if !strings.HasPrefix(normalized, prefix) || !strings.HasSuffix(normalized, ".md") {
		return "", "", false
	}
	rest := strings.TrimPrefix(normalized, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	status := parts[0]
	switch status {
	case "idea", "todo", "ready", "wip", ".done", ".dropped":
	default:
		return "", "", false
	}
	stem := strings.TrimSuffix(parts[1], ".md")
	if stem == "" {
		return "", "", false
	}
	return status, stem, true
}

func ticketChangeSortKey(change TicketChange) string {
	return strings.Join([]string{change.Stem, change.Path, change.OldPath, change.FromStatus, change.ToStatus}, "\x00")
}

func ticketChangeSummaries(changes []TicketChange) []string {
	summaries := make([]string, 0, len(changes))
	for _, change := range changes {
		switch {
		case change.FromStatus != "" && change.FromStatus != change.ToStatus && change.ResultAdded:
			summaries = append(summaries, fmt.Sprintf("%s: moved %s -> %s and added %s", change.Stem, change.FromStatus, change.ToStatus, change.ResultHeading))
		case change.FromStatus != "" && change.FromStatus != change.ToStatus:
			summaries = append(summaries, fmt.Sprintf("%s: moved %s -> %s", change.Stem, change.FromStatus, change.ToStatus))
		case change.ResultAdded:
			summaries = append(summaries, fmt.Sprintf("%s: added %s", change.Stem, change.ResultHeading))
		}
	}
	return summaries
}
