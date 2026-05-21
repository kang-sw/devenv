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

- The audience is the model re-reading under attention pressure, not a human reading fresh.
- One-liners survive pressure; paragraphs dissolve. Every rule fits one line.
- Preserve full grammar when compression could change order, ownership, or safety.

### Layout

- Directives at top, rationale (if any) as a single Doctrine paragraph at bottom. Never interleave.
- Mechanical rules and soft judgments do not mix. Soft decision points must be separated and stated explicitly.
- Use Markdown hierarchy to route attention before adding prose.
- Prefer command-shaped fragments over explanatory paragraphs.
- For dense routing or rule lists, prefer short sections, named groups, fixed lookup tables, and command-shaped lists over long flat lists.
- Do not invent a pseudo-code DSL when Markdown structure can express the route.

### Content rules

- Self-contained. Skills: no references to tickets, sessions, or sibling skills except plugin skill invocations such as `ws:` and host-specific slash forms. Agents: no references to session state or conversation history.
- Use examples only when they prevent repeated wrong execution.
- For user shorthand, name the general intent first and list shorthand only as trigger examples.

### Iteration

- Repeatedly violated rule -> mechanize (structured output block at entry point), do not repeat louder.
- Compress before adding: delete filler, merge duplicates, keep exact technical nouns.
- At every authoring turn's end, re-read additions and cut.
- After restructuring, request an authorized fresh audit: contradictions, duplication, orphan references, closure gaps.

### Skill semantics

- Skill-to-skill handoffs share the active conversation; write `Continue through <skill>` without a carry block.
- User-approval gates fire only when the user invokes the skill directly; chained invocations pass through.
- A lead skill cannot also be its own executor; if routing names a sibling but the lead remains the acting agent, absorb the sibling.
- Reserve "arguments" for MCP tools, CLI commands, and structured templates.

### Invariant / Constraint checklist

Check every invariant or constraint after drafting. Every answer is yes/no.

- **Falsifiable?** - Can you describe a concrete violation? If not, it is a wish, not a rule.
- **Actionable?** - Does it say what to *do*, not just what to *avoid*?
- **One line?** - If it needs a paragraph to state, it is not yet distilled.
- **Context-free?** - Understandable without reading the surrounding file?
- **Non-redundant?** - Does it say something no other line already covers?
- **Universal?** - Is it a constraint that holds in all situations, not a step at a specific point?
- **Derivable?** - Can it be regenerated from the Doctrine paragraph?

Grouped invariant lists are allowed when a skill has many hard rules:

```text
Group Name
- <invariant>
- <invariant>
```

Group names classify invariants only; they are not rules. Do not nest groups or
bullets. Do not put handler steps, branch policy, or rationale in invariant
groups.

### Doctrine format

Doctrine has two jobs: name the finite resource, then add the generator clause.
Use measurable nouns such as "context window", not fuzzy nouns such as "quality".
Test: invariants should re-derive from the named resource.

## Skill Layout

Use this order. Omit sections the skill does not need.
Hierarchy clarifies responsibility; handlers preserve required order.

1. **Invariants** - unambiguous imperatives, zero interpretation cost, skimmable.
2. **Event handlers** (`On: X`) - numbered step lists per entry point. Consistent sub-structure across siblings.
3. **Judgments** - soft decision points extracted from handlers. Name them (`judge: <name>`) and centralize criteria here; handlers reference by name. A fixed lookup table with unambiguous triggers is a routing rule in the handler, not a judgment.
4. **Templates** - structured output formats: brief formats, spawn signatures, addenda. Procedures belong in handlers.
5. **Doctrine** - one paragraph, the generator.

Adapt section names to the document's reading pattern; keep the principles.

## Agent Layout

Top-to-bottom order. Simpler agents use the subset they need.

1. **Identity** - one sentence: what you are and what you do. Not a persona essay.
2. **Constraints** - scope boundaries, hard rules, what you never do. Same checklist as skill invariants.
3. **Process** - how you work, step by step. Equivalent to skill handlers but typically a single linear flow rather than multiple event-driven entry points.
4. **Heuristics** - decision tables, escalation criteria. Equivalent to skill judgments. Omit if the agent's decisions are purely mechanical.
5. **Output** - structured return format. Every agent must define what it sends back to the caller.
6. **Doctrine** - one paragraph, the generator.

Agents start with no session context. Keep them self-contained. Inject team
communication rules from the calling skill, not the agent definition.

## Doctrine

Skill and agent files are reread under attention pressure. Every choice
optimizes for **executability under pressure**: skimmable imperatives first,
mechanical structure where judgment fails, preserved judgment where mechanism
would lose signal, rationale collapsed into one generator. When ambiguous,
choose what the pressured model will execute reliably.
