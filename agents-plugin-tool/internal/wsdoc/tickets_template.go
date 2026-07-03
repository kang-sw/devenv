package wsdoc

import "fmt"

// ticketFrontmatter is the shared Frontmatter block from ticket-conventions.md lines 66–88.
const ticketFrontmatter = `### Frontmatter

` + "```yaml" + `
---
title: <title>
related:             # optional; map of stem → relationship note
  260301-feat-foo: prerequisite
spec:                # optional; list of spec-stems this ticket implements
  - 260421-feat-example
spec-remove:         # optional; list of spec-stems this ticket's implementation will remove
  - 260421-feat-removed-feature
parent:              # optional; epic stem (e.g., 260401-epic-auth-rewrite)
plans:               # maps phases to plan path stems under ai-docs/.plans/ (without .md)
  phase-1: 2026-03/28-1430.event-serialization
skeletons:           # legacy: maps phases to skeleton artifact commit hashes
  phase-1: abc1234
related-mental-model:  # optional; mental-model stems (filename without .md) consulted
  - workflow-routing   #   during ticket authoring — recovery hint for future sessions
completed:           # YYYY-MM-DD, added on move to .done/
---
` + "```" + `

Both ` + "`plans:`" + ` and legacy ` + "`skeletons:`" + ` list only phases that have artifacts — omit phases without an artifact (no null placeholders). Absence of ` + "`skeletons:`" + ` means "not needed"; normal implementation routing does not create new skeleton artifacts.`

// ticketBodyActionable is the Body block for feat/bug/refactor/chore (lines 90–116).
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

### Phase 2: <title>

...
` + "```" + `

Optional sections — add between ` + "`## Background`" + ` and ` + "`## Phases`" + ` when relevant:

- ` + "`## Decisions`" + ` — design choices with rationale and rejected alternatives.
- ` + "`## Constraints`" + ` — non-obvious boundaries (performance, compatibility, etc.).
- ` + "`## Prior Art`" + ` — existing patterns or components to reuse.
- ` + "`## Spec Impact`" + ` — ready-only spec addressing when no existing stem yet covers the behavior; include target spec area, expected caller-visible change, and ` + "`Contract-first spec: yes|no`" + `.`

// ticketBodyResearch is the Body block for research (lines 125–139).
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

// ticketBodyWorkset is the Workset body block (lines 141–168).
const ticketBodyWorkset = `
### Workset body (category = ` + "`workset`" + `)

` + "```markdown" + `
# <title>

## Context

<why this operating set exists>

## Tickets

- ` + "`<stem-or-path>`" + ` - <status; role in this workset; dependency note>

## Planned References

- ` + "`<provisional label>`" + ` - <intended role; creation condition>

## Focus

<current session, goal, sprint, or temporary operating focus>

## Exit Criteria

- Done: <conditions for closing this workset>
- Deferred: <what moves out of this workset>
` + "```" + `

Workset bodies define a non-hierarchical ticket collection, not decomposition. Included tickets do not set ` + "`parent:`" + ` to the workset; planned references do not receive status, path, or ` + "`parent:`" + ` until a real ticket exists.`

// ticketBodyEpic is the Epic body block (lines 170–197).
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
// It returns the shared Frontmatter block followed by the type-specific Body section,
// extracted verbatim from ticket-conventions.md lines 64–199.
func TicketTemplate(typeStr string) (string, error) {
	switch typeStr {
	case "feat", "bug", "refactor", "chore":
		return ticketFrontmatter + "\n" + ticketBodyActionable, nil
	case "research":
		return ticketFrontmatter + "\n" + ticketBodyResearch, nil
	case "workset":
		return ticketFrontmatter + "\n" + ticketBodyWorkset, nil
	case "epic":
		return ticketFrontmatter + "\n" + ticketBodyEpic, nil
	default:
		return "", fmt.Errorf("unknown ticket type %q; accepted: feat, bug, refactor, chore, research, workset, epic", typeStr)
	}
}
