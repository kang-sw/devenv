package mcp

import (
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsdoc"
)

// scopeAnnouncement computes the session-bootstrap sparse-checkout scope
// banner, or "" when no scope is active. Silent case (by design, not a bug):
//   - core.sparseCheckout is unset, or the worktree has no active
//     sparse-checkout pattern file — wsdoc.TicketScope degrades to
//     {Active:false, Hidden:0} after at most two os.Stat calls in that case,
//     spawning no git subprocess (see internal/wsdoc/tickets_scope.go).
//
// A resolution error is treated the same as inactive: a scope-detection
// failure must not block workflow_manual from rendering its body.
func scopeAnnouncement(root string) string {
	info, err := wsdoc.TicketScope(root, []string{"ready", "todo"})
	if err != nil || !info.Active {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("> **Sparse-checkout scope is active.** ")
	if info.Hidden == 0 {
		sb.WriteString("ai-docs/tickets/ready/ and ai-docs/tickets/todo/ are scoped, but no ticket is currently hidden.")
	} else {
		sb.WriteString(fmt.Sprintf(
			"%d ticket(s) hidden in ai-docs/tickets/ready/ and ai-docs/tickets/todo/ (stems: %s).",
			info.Hidden, strings.Join(info.HiddenStems, ", "),
		))
	}
	sb.WriteString(" See ai-docs/ref/worktree-ticket-scope.md; restore full visibility with `git sparse-checkout disable`.")
	return sb.String()
}
