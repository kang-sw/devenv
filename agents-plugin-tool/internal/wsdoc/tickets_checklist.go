package wsdoc

import (
	"fmt"
)

// ticketChecklistAcceptedTypes mirrors TicketTemplate's accepted category set.
var ticketChecklistAcceptedTypes = map[string]bool{
	"feat": true, "bug": true, "refactor": true, "chore": true,
	"research": true, "epic": true,
}

// ticketChecklistContent is the phase:"content" capture checklist. This
// constant is the checklist's single source; no playbook restates it.
const ticketChecklistContent = `1. Capture every settled decision, contract, agreed API/type/event/UI sketch (literal, not prose-flattened), rejected alternative, constraint, forward-compatibility guardrail, and verification expectation; include suggested implementation strategy only when it was agreed, constrains implementation, or is needed to recover the intended contract.
2. Exclude anything unconfirmed — return it to the Open Decision Queue instead of writing it; exclude source-local edit notes unless they are settled constraints.`

// ticketChecklistIntent is the phase:"intent" conversation-fidelity check: the
// one question the promotion-time sage review cannot answer, because the sage
// reviewer never sees the conversation. It is deliberately three items and
// shared by non-research categories — enumeration of what to capture belongs to the content
// checklist, epic shape to the ticket conventions, and
// fix-then-summarize to the skill that runs the check. This constant is the
// checklist's single source; no playbook restates it.
const ticketChecklistIntent = `1. Test: could a fresh implementer build a materially different caller-visible, workflow, API, or verification result from the settled discussion without contradicting the ticket? If yes, capture the missing settled decision.
2. Check that API/type/event/UI sketches were preserved literally, not prose-flattened.
3. Check that no unconfirmed mechanism choice, future-scope hint, Result Forward note, or focus "Next" line was written.`

const ticketChecklistResearchContent = `1. Capture evidence-backed observations under Outcome Ledger > Verified Findings, user-confirmed normative choices under Confirmed Decisions, and explicitly rejected alternatives under Rejected Alternatives; preserve agreed API/type/event/UI sketches literally.
2. Research may retain explicitly non-authoritative Proposals and Open Questions in the Outcome Ledger. Keep unconfirmed choices out of Confirmed Decisions; send choices needing settlement to the Open Decision Queue. Exclude source-local edit notes unless they are settled constraints.`

const ticketChecklistResearchIntent = `1. Check that the Outcome Ledger distinguishes evidence-backed Verified Findings, user-confirmed Confirmed Decisions, and explicitly Rejected Alternatives without promoting narrative into contract.
2. Check that agreed API/type/event/UI sketches were preserved literally, not prose-flattened.
3. Check that unconfirmed candidates and unresolved choices are explicitly non-authoritative Proposals and Open Questions in the Outcome Ledger; they may remain in research but never become actionable authority.`

// TicketChecklist returns the checklist item list for a ticket-authoring phase,
// as data the caller installs into a single todo.add instruction. It mirrors
// TicketTemplate's shape: a pure, root-free switch returning canned markdown
// text held only here.
func TicketChecklist(typeStr, phase string) (string, error) {
	if !ticketChecklistAcceptedTypes[typeStr] {
		return "", fmt.Errorf("unknown ticket type %q; accepted: feat, bug, refactor, chore, research, epic", typeStr)
	}
	switch phase {
	case "content":
		if typeStr == "research" {
			return ticketChecklistResearchContent, nil
		}
		return ticketChecklistContent, nil
	case "intent":
		if typeStr == "research" {
			return ticketChecklistResearchIntent, nil
		}
		return ticketChecklistIntent, nil
	default:
		return "", fmt.Errorf("unknown ticket checklist phase %q; accepted: content, intent", phase)
	}
}
