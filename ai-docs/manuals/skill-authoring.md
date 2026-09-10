---
summary: Authoring standard for skills, playbooks, agent prompts, and conventions written for current-generation models — reader model, rule lifecycle, rule tests, layouts, audits
---

# Skill Authoring

Standard for every skill, playbook, agent prompt, and convention this
repository ships. Read it before editing any of them. It optimizes one thing:
**fewest lead turns and fewest stops per ticket at unchanged review quality.**
The finite resources are the user's attention at each stop, wall-clock latency,
and the worker's context window; the lead's context is not the constraint.

## Reader Model

- The reader reads the whole file and treats every imperative as binding. Its
  failure mode is over-compliance: applying a rule outside the scope its author
  had in mind, and taking the heavier path whenever a rule leaves it unsure. It
  does not skip, and it does not need repetition.
- Therefore every rule says when it applies and what the default is when it
  does not. A one-clause rationale is content, not overhead: the reader uses it
  to decide non-application.
- Reader tiers differ. A **worker** (current-mainstream or previous-generation
  flagship class, holding a whole ticket) gets outcome, constraints, and a
  closed stop list. A **cheap-tier delegate** (fact population, survey, review
  scouting) gets a bounded task, pointer inputs, and a fixed output shape.
  Text written for one tier is wrong for the other.

## Layer Model

Every sentence of skill content belongs to exactly one layer. Layers 1 and 2
are owned by the MCP tool; delete their content from prose unconditionally.

| Layer | Owns | Model-accessible? | In prose? |
|-------|------|------|------|
| 1 — MCP schema | Field names, types, enums, call format, response fields | Yes, via tool discovery and the response | No — restatement drifts |
| 2 — MCP internal | Deterministic computation and all post-call output text (`Next:`, todo instructions) | No; it arrives in the response | No — prose that says what to do with post-call output is restatement |
| 3 — Prose | Pre-call only: what to observe, the judgment the tool cannot compute, non-obvious edge cases, the stop list | Yes | Only what passes the test below |

Test every section, burden of proof on keeping it:
- Would a reader with only Layer 3 plus the tool schemas reach the same outcome?
  Yes → delete.
- Does it say what to do with something a tool returns post-call? Yes → delete,
  after confirming against the tool's actual output text.
- Uncertain → delete. A missing Layer 3 line costs one wrong execution; a stale
  Layer 1/2 copy drifts silently.

Exempt: text rendered verbatim into a delegate prompt (`playbook.render`
payloads, template blocks). Its reader has no session context; apply the
layouts below to it, not this test.

Move logic into a tool only when it is deterministic (a git range, a directory
move, a stamp). Judgment never moves into a tool input the model must
pre-populate: that converts one judgment into a fact-gathering turn plus a
judgment, and is how routing surfaces grow.

## Rule Lifecycle

- A rule exists to prevent a named failure, and cites it in one clause. A
  rule with no citable failure is deleted.
- Add a rule only for a failure observed on the current worker tier. When the
  tier changes, reproduce each rule's failure or delete the rule; rules are not
  kept as insurance.
- Repeated friction is not a reason to add a gate. A gate whose default is the
  heavier path is a defect: state the cheap default and the condition that
  escalates.
- Unspecified cases are intentional judgment gaps. Leave them empty.
- Compress before adding: delete filler and duplicates, keep exact technical
  nouns, and keep full grammar where compression could change order, ownership,
  or safety.

## Rule Tests

Every rule passes all seven: **Falsifiable** (a concrete violation can be
described) · **Actionable** (says what to do) · **Scoped** (says when it does
not apply, or names its cheap default) · **Non-derivable** (a reader with the
code, the tests, and the tool schemas could not infer it) · **Failure-cited**
(names the failure it prevents, reproducible on the current tier) ·
**Non-redundant** (no other line covers it) · **Resolvable downstream**
(resolves in a project holding only what bootstrap installs; see
`ai-docs/manuals/shipped-surface-boundary.md` for what shipped text may not
name).

Grouped rules are allowed: `Group Name` / `- <rule>`. Group names classify;
they are not rules.

## Style

- State the outcome, the constraints, and the stop conditions. Write steps only
  where order is a safety property.
- Write prohibitions as prohibitions and preferences as preferences with their
  default. Do not soften a prohibition into a preferred alternative.
- At most one example, and only of an output shape.
- No doctrine sections. The optimization target is the file's first sentence;
  rationale sits on the rule it justifies. A separate doctrine paragraph was a
  device for readers that needed to re-derive dropped rules; this reader keeps
  them.
- Pointers, not summaries. Inputs are paths, stems, and ranges; another agent's
  summary is never a reader's sole input; any structured output carries an
  explicit omitted/deferred field.
- Skills reference other skills only as invocation targets
  (`{{.SkillNamespace}}:<skill>`); handoffs share the conversation and need no
  carry block; name the target and its entry, never its internal judgments.
- User-approval gates apply on direct user invocation only. Chained invocations
  re-ask only for the Approval Protocol's always-ask category.

## Layouts

Directives at the top, no doctrine at the bottom.

**Lead skill** (thin): Identity → what the lead does here (converse, manage the
ticket inventory, spawn, handle first-line escalation) → the stops it surfaces
to the user → Output. It carries no procedure a worker executes.

**Worker playbook** (`kind: render`): Identity and addressee (a worker holding
a lead-capability key, reporting to the lead, never waiting for a human) →
Inputs as pointers (ticket path and stem, the project's declared conventions
hook, the manuals the ticket cites) → Constraints → the closed stop list →
Report shape. Self-contained through pointers; communication rules are injected
by the caller, not written into the playbook.

**Cheap-tier delegate**: Identity → one bounded task → pointer inputs → fixed
output shape with an omitted/deferred field. No judgment beyond the task.

## Audits

**Fresh-reader audit** — once per skill or playbook before it lands, not after
every edit.
1. A separate reviewer reads only the target file: no conversation, project
   docs, or metadata. Bar: a good-faith reader who applies the stated purpose
   without hunting for loopholes.
2. It flags: awkward, surprising, context-dependent, underspecified,
   contradictory, duplicated, orphaned, missing end-state wording, Layer 1/2
   restatement, **a rule that sends a careful reader down the heavier path when
   unsure**, and **a rule that prevents a failure the current tier does not
   commit**.
3. Per finding: quote, issue, severity, suggested rewrite or deletion.
4. Classify `fix` · `risk accepted` (record cost) · `intentional difference` ·
   `out of scope`. Edit `fix` only. One cycle; a second only if a fix produced a
   new finding.

**Mirror sweep** — when a change touches a surface mirrored into
`agents-plugin-wsflow`, read `ai-docs/manuals/wsflow-mirroring.md` first and
regenerate with its documented command; never hand-edit the mirror.

The former routing-coverage audit is retired with the routing surface it
audited; a resolver's verdict set is covered by that tool's tests.
