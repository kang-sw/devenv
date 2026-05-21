---
name: lead-skill-authoring
description: Author or audit ws workflow skills and agent prompts using the repository skill-authoring rules. Use when creating, porting, normalizing, or reviewing SKILL.md files, agent prompt files, workflow instructions, invariants, constraints, handlers, judgments, templates, or doctrine text.
---

# Skill Authoring

Canonical repository reference for skill and agent authoring.
Use this reference when authoring or auditing ws skills and agent prompts.
Apply rules directly; add local procedure only when the target needs it.

## Principles

These apply to both skill and agent documents.

### Reader model

- The audience is a model re-reading under attention pressure, not a leisurely human reader.
- One-liners survive pressure; paragraphs are skipped. Every rule fits one line.
- Preserve full grammar when compression could change order, ownership, or safety.

### Layout

- Directives at top, rationale (if any) as a single Doctrine paragraph at bottom. Never interleave.
- Mechanical rules and soft judgments do not mix. Soft decision points must be separated and stated explicitly.
- Use Markdown hierarchy to route attention before adding prose.
- Prefer command-shaped fragments over explanatory paragraphs.
- For dense routing or rule lists, prefer short sections, named groups, fixed lookup tables, and command-shaped lists over long flat lists.
- Use Markdown structure before introducing custom notation.

### Content rules

- Skills stand alone. Refer to other skills only as explicit invocation targets, such as `ws:<skill>` or host-specific slash commands.
- Agents stand alone. Do not reference session state or conversation history.
- Use examples only when they prevent repeated wrong execution.
- For user shorthand, name the general intent first and list shorthand only as trigger examples.

### Iteration

- Repeatedly violated rule -> mechanize with structure instead of restating it.
- Compress before adding: delete filler, merge duplicates, keep exact technical nouns.
- After each authoring pass, re-read additions and cut.
- After skill, agent, or prompt edits, run a fresh-reader audit: context-dependent wording, contradictions, duplication, orphan references, missing end-state or output instructions.
- After doctrine, terminology, routing, layout, or audit-gate edits, run a downstream consistency sweep across affected skill surfaces; conservative reviewers report findings only, then the lead classifies fixes.

### Skill semantics

- Skill-to-skill handoffs share the active conversation; write `Continue through <skill>` without a carry block.
- Skill-level user-approval gates apply only on direct user invocation; chained invocations re-ask only for safety, deletion, or explicit consent rules.
- If a route references another skill without instructing invocation or delegation, perform the referenced steps locally.
- The calling lead skill/session owns active-conversation judgments; use `ws/subquery` only for self-contained artifact, source, spec, or ticket evidence.
- When invoking another skill, name only the target skill and entry route; do not decide that skill's internal judgments in advance.
- Use "arguments" only for formal tool, command, or template parameters.

### Invariant / Constraint checklist

Check every invariant or constraint after drafting. Every answer is yes/no.

- **Falsifiable?** - Can you describe a concrete violation? If not, it is a wish, not a rule.
- **Actionable?** - Does it say what to *do*, not just what to *avoid*?
- **One line?** - If it needs a paragraph to state, it is not yet distilled.
- **Context-free?** - Understandable without reading the surrounding file?
- **Non-redundant?** - Does it say something no other line already covers?
- **Universal?** - Is it a constraint that holds in all situations, not a step at a specific point?
- **Doctrine-aligned?** - Does it follow from the file's stated doctrine?

Grouped invariant lists are allowed when a skill has many hard rules:

```text
Group Name
- <invariant>
- <invariant>
```

Group names classify invariants only; they are not rules. Do not nest groups or
bullets. Do not put handler steps, routing policy, Git branch policy, or
rationale in invariant groups.

### Doctrine format

Doctrine has two jobs: name the finite resource, then add the guiding principle.
Use measurable nouns such as "context window", not fuzzy nouns such as "quality".
Test: invariants should re-derive from the named resource.

## On: Fresh-Reader Audit

1. Read only the assigned target text.
2. Report material execution blockers: wrong-behavior wording, contradictions, orphan references, or missing required output/end-state instructions.
3. For each finding, include quote, issue, and suggested rewrite or delete.
4. If no material issue remains, say so clearly.

## On: Downstream Consistency Sweep

1. Select affected skill, agent, prompt, spec, mental-model, test, and mirrored-package surfaces from the edited doctrine, terminology, route, layout, or audit gate.
2. Use a conservative finding-only first pass for broad skill-surface scans.
3. Classify each finding as `fix`, `intentional difference`, or `out of scope`.
4. Edit only accepted `fix` findings; record intentional differences when drift would otherwise look stale.

## Skill Layout

Use this order. Omit sections the skill does not need.
Hierarchy clarifies responsibility; handlers preserve required order.

1. **Invariants** - unambiguous imperatives, zero interpretation cost, skimmable.
2. **Event handlers** (`On: X`) - numbered step lists per entry point. Consistent sub-structure across siblings.
3. **Judgments** - soft decision points extracted from handlers. Name them (`judge: <name>`) and centralize criteria here; handlers reference by name. A fixed lookup table with unambiguous triggers is a routing rule in the handler, not a judgment.
4. **Templates** - structured output formats: brief formats, agent-delegation call formats, addenda. Procedures belong in handlers.
5. **Doctrine** - one paragraph, the guiding principle.

Adapt section names to the document's reading pattern; keep the principles.

## Agent Layout

Top-to-bottom order. Simpler agents use the subset they need.

1. **Identity** - one sentence: what you are and what you do. Not a persona essay.
2. **Constraints** - scope boundaries, hard rules, what you never do. Same checklist as skill invariants.
3. **Process** - how you work, step by step. Equivalent to skill handlers but typically a single linear flow rather than multiple event-driven entry points.
4. **Heuristics** - decision tables, escalation criteria. Equivalent to skill judgments. Omit if the agent's decisions are purely mechanical.
5. **Output** - structured return format. Every agent must define what it sends back to the caller.
6. **Doctrine** - one paragraph, the guiding principle.

Agents start with no session context. Keep them self-contained. Inject team
communication rules from the calling skill, not the agent definition.

## Doctrine

Skill and agent files are reread under attention pressure. Every choice
optimizes for **executability under pressure**: skimmable imperatives first,
mechanical structure where judgment fails, preserved judgment where mechanism
would lose signal, rationale collapsed into one guiding principle. When ambiguous,
choose what the pressured model will execute reliably.
