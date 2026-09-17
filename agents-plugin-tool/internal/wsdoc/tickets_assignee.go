package wsdoc

import "strings"

// AssigneeGateWarning is the exact mismatch token appended to a ticket's
// assignee line in the compact-text projection and carried in
// AssigneeGate.Warning. It is a fixed string so a selector or a downstream
// caller can match it deterministically rather than parsing prose.
const AssigneeGateWarning = "NOT ASSIGNED TO YOU"

// AssigneeGate is the point-in-time ownership comparison between the current
// git identity and a ticket's assignee set. It is computed by the MCP layer
// (which supplies the identity and reads the ticket-assignee-aware flag) and
// attached to TicketInfo only when the feature is on, so it never appears in
// any projection while the feature is off.
type AssigneeGate struct {
	// CurrentEmail is the lower-cased identity the comparison used. Empty when
	// the caller's identity is unknown (git user.email unset, CI/bot).
	CurrentEmail string `json:"current_email,omitempty"`
	// AssignAny is true when the ticket names no assignee: anyone may take it.
	AssignAny bool `json:"assign_any"`
	// AssignedToCurrent is true when the current identity may take the ticket
	// without a warning: either AssignAny, the current email is in the assignee
	// set (case-insensitive any-of), or the identity is unknown (never steer a
	// caller we cannot identify away from an assigned ticket).
	AssignedToCurrent bool `json:"assigned_to_current"`
	// Warning is the human-readable mismatch token (AssigneeGateWarning),
	// populated only when the ticket is assigned to someone else. Empty
	// otherwise.
	Warning string `json:"warning,omitempty"`
}

// AssigneeGateFor computes the ownership gate for one ticket's assignee set
// against currentEmail (the caller's git user.email). Matching is
// case-insensitive and any-of: the caller owns the ticket if their email is in
// the set. An empty assignee set is assign-any; an empty currentEmail means the
// identity is unknown, in which case every ticket resolves to
// AssignedToCurrent so the feature never hard-blocks an unidentifiable caller.
func AssigneeGateFor(assignee []string, currentEmail string) AssigneeGate {
	email := strings.ToLower(strings.TrimSpace(currentEmail))
	if len(assignee) == 0 {
		return AssigneeGate{CurrentEmail: email, AssignAny: true, AssignedToCurrent: true}
	}
	if email == "" {
		return AssigneeGate{CurrentEmail: email, AssignedToCurrent: true}
	}
	for _, a := range assignee {
		if strings.ToLower(strings.TrimSpace(a)) == email {
			return AssigneeGate{CurrentEmail: email, AssignedToCurrent: true}
		}
	}
	return AssigneeGate{CurrentEmail: email, Warning: AssigneeGateWarning}
}
