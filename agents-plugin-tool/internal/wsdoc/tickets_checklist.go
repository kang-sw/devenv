package wsdoc

import "fmt"

// ticketChecklistAcceptedTypes mirrors TicketTemplate's accepted category set.
var ticketChecklistAcceptedTypes = map[string]bool{
	"feat": true, "bug": true, "refactor": true, "chore": true,
	"research": true, "workset": true, "epic": true,
}

// ticketChecklistContent is the phase:"content" checklist, extracted verbatim
// from lead-write-ticket.md's "On: Apply Ticket Content" section (category-invariant).
const ticketChecklistContent = `1. Capture every settled decision, contract, agreed API/type/event/UI sketch (literal, not prose-flattened), rejected alternative, constraint, forward-compatibility guardrail, and verification expectation; include suggested implementation strategy only when it was agreed, constrains implementation, or is needed to recover the intended contract.
2. Exclude anything unconfirmed — return it to the Open Decision Queue instead of writing it; exclude source-local edit notes unless they are settled constraints.`

// ticketChecklistIntentItem5Epic and ticketChecklistIntentItem5Workset are the
// category-conditional halves of item 5 from "On: Intent Review". Other
// categories omit item 5 entirely (it does not apply to them) and the
// remaining items renumber accordingly.
const ticketChecklistIntentItem5Epic = "5. For `epic`, check that detailed implementation material stayed out of the epic and moved to a child-ticket invocation."
const ticketChecklistIntentItem5Workset = "5. For `workset`, check that it did not create parent-child semantics, decomposition ownership, or implementation phases."

// TicketChecklist returns the checklist item list for a ticket-authoring phase,
// as data the caller installs into a single todo.append instruction. It mirrors
// TicketTemplate's shape: a pure, root-free switch returning canned markdown
// text, extracted verbatim from lead-write-ticket.md.
func TicketChecklist(typeStr, phase string) (string, error) {
	if !ticketChecklistAcceptedTypes[typeStr] {
		return "", fmt.Errorf("unknown ticket type %q; accepted: feat, bug, refactor, chore, research, workset, epic", typeStr)
	}
	switch phase {
	case "content":
		return ticketChecklistContent, nil
	case "intent":
		items := []string{
			"1. Re-read the written/edited ticket against the conversation and cross-ticket decision review, against the categories in **Apply Ticket Content**.",
			"2. Test: could a fresh implementer build a materially different caller-visible, workflow, API, or verification result from the settled discussion without contradicting the ticket? If yes, capture the missing settled decision.",
			"3. Check that API/type/event/UI sketches were preserved literally, not prose-flattened.",
			"4. Check that no unconfirmed mechanism choice, future-scope hint, Result Forward note, or focus \"Next\" line was written.",
		}
		next := 5
		switch typeStr {
		case "epic":
			items = append(items, ticketChecklistIntentItem5Epic)
			next = 6
		case "workset":
			items = append(items, ticketChecklistIntentItem5Workset)
			next = 6
		}
		items = append(items,
			fmt.Sprintf("%d. Fix confirmed gaps in-place; return unconfirmed gaps to the Open Decision Queue instead of writing them.", next),
			fmt.Sprintf("%d. Present a brief correction summary, or confirm nothing was missed.", next+1),
		)
		text := items[0]
		for _, item := range items[1:] {
			text += "\n" + item
		}
		return text, nil
	default:
		return "", fmt.Errorf("unknown ticket checklist phase %q; accepted: content, intent", phase)
	}
}
