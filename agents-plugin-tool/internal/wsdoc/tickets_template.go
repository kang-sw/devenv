package wsdoc

import "fmt"

// ticketFrontmatter is the shared Frontmatter block from ticket-conventions.md.
const ticketFrontmatter = `### Frontmatter

` + "```yaml" + `
---
title: <title>
related:             # optional; map of stem → relationship note
  260301-feat-foo: prerequisite
parent:              # optional; epic stem (e.g., 260401-epic-auth-rewrite)
completed:           # YYYY-MM-DD, added on move to .done/
---
` + "```"

// ticketBodyActionable is the Body block for feat/bug/refactor/chore.
const ticketBodyActionable = `
### Body (actionable: ` + "`feat`, `bug`, `refactor`, `chore`" + `)

` + "```markdown" + `
# <title>

## Background

<problem or goal — what and why>

## Phases

### Phase 1: <title>

<goals, constraints, rationale, rejected alternatives, suggested approaches>

### Result (<short-hash>) - YYYY-MM-DD

<what was implemented, deviations from plan, key findings for future phases>

#### Edition (<short-hash>) - YYYY-MM-DD

<later tweak or follow-up implementation pass for this completed phase>
` + "```" + `

Default to a single Phase 1. Add another phase when a later one is sequentially
dependent — it builds on an earlier phase's landed Result; separate review or
verification scope on its own is rarely reason enough.

Optional sections — add between ` + "`## Background`" + ` and ` + "`## Phases`" + ` when relevant:

- ` + "`## Decisions`" + ` — design choices with rationale and rejected alternatives.
- ` + "`## Constraints`" + ` — non-obvious boundaries (performance, compatibility, etc.).
- ` + "`## Prior Art`" + ` — existing patterns or components to reuse.`

// ticketBodyResearch is the Body block for research.
const ticketBodyResearch = `
### Body (category = ` + "`research`" + `)

` + "```markdown" + `
# <title>

## Background

<question or context>

## <Topic heading>

<findings, decisions, rejected alternatives>
` + "```" + `

Research tickets have no phases. Sections after ` + "`## Background`" + ` are freeform topic headings.`

// ticketBodyEpic is the Epic body block.
const ticketBodyEpic = `
### Epic body (category = ` + "`epic`" + `)

` + "```markdown" + `
# <title>

## Scope

<included milestone scope>

## Non-Scope

<explicit exclusions>

## Child Tickets

- ` + "`<stem>`" + ` - <slice purpose/status/dependency note>
- Planned: <child ticket description>

## Cross-Child Decisions

<invariants that child tickets must preserve>

## Completion Criteria

- Done: <conditions for moving the epic to .done/>
- Dropped: <conditions for moving the epic to .dropped/>
- Deferred: <scope intentionally left for a later epic or child>
` + "```"

// TicketTemplate returns the fill-in body skeleton for a given ticket type.
// It returns the shared Frontmatter block followed by the type-specific Body
// section.
func TicketTemplate(typeStr string) (string, error) {
	switch typeStr {
	case "feat", "bug", "refactor", "chore":
		return ticketFrontmatter + "\n" + ticketBodyActionable, nil
	case "research":
		return ticketFrontmatter + "\n" + ticketBodyResearch, nil
	case "epic":
		return ticketFrontmatter + "\n" + ticketBodyEpic, nil
	default:
		return "", fmt.Errorf("unknown ticket type %q; accepted: feat, bug, refactor, chore, research, epic", typeStr)
	}
}
