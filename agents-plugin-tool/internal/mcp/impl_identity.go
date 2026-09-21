package mcp

import "github.com/kang-sw/devenv/internal/wskey"

// implTicketSuffix is the single authority for ticket-derived branch identity.
// Hashing is intentionally one-way; reverse lookup enumerates candidate stems.
func implTicketSuffix(stem string) string { return wskey.Derive(stem, 3) }

func implTicketBranch(base, stem string) string {
	return implementTargetBranchName(implementMergeRootFor(base), implTicketSuffix(stem))
}

// matchImplTicketStems preserves all matches so callers can refuse ambiguity.
func matchImplTicketStems(suffix string, candidates []string) []string {
	var matches []string
	for _, stem := range candidates {
		if implTicketSuffix(stem) == suffix {
			matches = append(matches, stem)
		}
	}
	return matches
}
