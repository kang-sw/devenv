---
kind: print
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
- For dense `On:` handlers, use H3 sub-blocks when a flat list hides responsibility boundaries.
- Use Markdown structure before introducing custom notation.

### Content rules

- Skills stand alone. Refer to other skills only as explicit invocation targets, such as `{{.SkillNamespace}}:<skill>` or host-specific slash commands.
- Agents stand alone. Do not reference session state or conversation history.
- Use examples only when they prevent repeated wrong execution.
- Prefer `Do X through Y` over `Do not do X` when a positive action exists.
- For user shorthand, name the general intent first and list shorthand only as trigger examples.

### Iteration

- Repeatedly violated rule -> mechanize with structure instead of restating it.
- Compress before adding: delete filler, merge duplicates, keep exact technical nouns.
- After each authoring pass, re-read additions and cut.
- After skill, agent, or prompt edits, run the audit in **On: Fresh-Reader Audit**.
- After doctrine, terminology, routing, layout, or audit-gate edits, run a downstream consistency sweep across affected skill surfaces; conservative reviewers report findings only, then the lead classifies fixes.

### Skill semantics

- Skill-to-skill handoffs share the active conversation; write `Continue through <skill>` without a carry block.
- Skill-level user-approval gates apply only on direct user invocation; chained invocations re-ask only for safety, deletion, or explicit consent rules.
- If a route references another skill without instructing invocation or delegation, perform the referenced steps locally.
- The calling lead skill/session owns active-conversation judgments; use direct host-native exploration-worker dispatch (see `lead-workflow-manual`) only for self-contained artifact, source, spec, or ticket evidence.
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

### Handler structure

Use H3 sub-blocks when an `On:` handler exceeds four steps and mixes
responsibilities. Name each sub-block by the responsibility it performs.
Do not split single-purpose checklists only because they are long.
Use domain-specific labels when they make the execution path clearer.
Compact handlers do not need sub-blocks.

### Doctrine format

Doctrine has two jobs: name the finite resource, then add the guiding principle.
Use measurable nouns such as "context window", not fuzzy nouns such as "quality".
Test: invariants should re-derive from the named resource.

## On: Fresh-Reader Audit

Audit targets: `agents-plugin/rsrc/lead-*/lead-*.md` (migrated procedure playbooks) and `agents-plugin/skills/*/SKILL.md` (entry skills).

The audit bar is a good-faith reader: one who applies the text's stated purpose and does not hunt for loopholes. The text passes when such a reader reaches the intended end-state. A finding reachable only by reading the text in a way a good-faith reader would not is `out of scope`, not `fix`.

1. Run a separate fresh reviewer (agent or subagent) for the audit.
2. Give the reviewer only the target file or excerpt; do not include prior conversation, project docs, skill docs, specs, rationale, or host-generated metadata.
3. Tell the reviewer to read only the provided target and not to read any other files, skills, docs, or context.
4. Ask the reviewer to flag awkward, surprising, context-dependent, underspecified, contradictory, duplicated, orphaned, or missing end-state/output wording.
5. Require each finding to include a quote, the issue, severity (`low`/`medium`/`high`), and either a suggested rewrite or a suggested deletion.
6. Classify each reviewer finding as `fix`, `risk accepted`, `intentional difference`, or `out of scope` before editing. `risk accepted`: a valid finding whose fix's concrete cost exceeds its residual risk under the bar — record that cost and risk, not a bare assertion. `intentional difference`: wording that diverges from a sibling file on purpose. `out of scope`: the finding targets text outside the audit target, or is reachable only outside the good-faith bar.
7. Edit only findings classified as `fix`. Record the cost and risk for `risk accepted`; record or ignore `intentional difference` and `out of scope`.
8. Run at most three audit/revision cycles; stop once no finding is still classified `fix` — all have been edited or are `risk accepted`, `intentional difference`, or `out of scope`. If `fix` findings remain after the third cycle, stop and report them as unresolved rather than starting a fourth cycle.

## On: Downstream Consistency Sweep

1. Select affected rsrc playbook sources (`agents-plugin/rsrc/lead-*/lead-*.md`), entry skill files (`agents-plugin/skills/*/SKILL.md`), agent prompts, specs, mental-models, tests, and mirrored-package surfaces from the edited doctrine, terminology, route, layout, or audit gate.
2. Use a conservative finding-only first pass for broad skill-surface scans.
3. Classify each finding as `fix`, `risk accepted`, `intentional difference`, or `out of scope`.
4. Edit only accepted `fix` findings; record intentional differences when drift would otherwise look stale.

## Skill Layout

Use this order. Omit sections the skill does not need.
Hierarchy clarifies responsibility; handlers preserve required order.

1. **Invariants** - unambiguous imperatives, zero interpretation cost, skimmable.
2. **Event handlers** (`On: X`) - numbered step lists per entry point; use H3 sub-blocks for dense mixed-responsibility handlers.
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
