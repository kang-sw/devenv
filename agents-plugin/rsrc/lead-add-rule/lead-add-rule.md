---
kind: print
---

# Add Rule

Target: user request

## Project Map

Read `AGENTS.md`'s `## Workflow` -> `### Implementation Conventions` section for
the manuals this project scopes to paths. A project that declares no such section
has no path-scoped manual, so every rule routes to `CLAUDE.md`.

## Invariants

- Never modify existing rule content - this skill only appends new rules. Correcting or editing a stale rule is the user's responsibility via manual edit.
- Never write the same rule to both `CLAUDE.md` and a path-scoped manual in one invocation.
- One invocation writes to exactly one target file.
- All written rules are in English regardless of conversation language.
- Commit the change at the end following CLAUDE.md commit rules. Include `## AI Context` recording the classification decision.
- A manual's rule section is whichever section that manual already uses for rules. When it has none, create `## Rules` at the end of the manual and report the new section to the user.

## On: invoke

### 1. Read

1. Parse the rule from `user request`. If `user request` is empty, ask the user for the rule description and wait.
2. Read `CLAUDE.md` to see current `## Architecture Rules` entries and avoid near-duplicates.
3. Use the declared conventions section for the current catalog of path-scoped manuals.
4. Read each candidate manual before proposing it; a rule it already states is a near-duplicate.

### 2. Classify

Apply `judge: classification`: **cross-cutting**, **domain-scoped**, or **ambiguous**.

### 3. Route

| Classification | Route target |
|---|---|
| cross-cutting | `CLAUDE.md` -> append to `## Architecture Rules`. |
| domain-scoped | Enumerate the manuals the conventions section declares; apply `judge: domain-match`. |
| ambiguous | Stop. Prompt the user with the two plausible classifications plus the best-match domain candidate; wait for selection. |

For domain-scoped rules, apply `judge: domain-match`:

| Match | Action |
|---|---|
| **single clear domain** | Target = the declared manual whose `paths` cover the rule's subject. Propose and write. |
| **multiple candidates** | Stop. Present the candidate list with one-line rationales. Wait for user selection. |
| **no matching doc** | Stop. Propose creating `ai-docs/manuals/<topic>.md` and declaring it as a new row of `### Implementation Conventions` with the `paths` it scopes. Wait for user confirmation before writing. |

### 4. Propose or prompt

1. If a clear target exists: state `Target: <path> section <section>; rule: "<rule>".` Proceed to section 5.
2. Otherwise: surface the ambiguity, list options, and stop. Do not write. Resume from section 5 once the user selects.

### 5. Write

1. Open the target doc.
2. Locate the target section (`## Architecture Rules` in `CLAUDE.md`, or the manual's rule section).
   - If the section is **absent**: In a manual, add a `## Rules` heading at the end of the file. In `CLAUDE.md`, add the `## Architecture Rules` heading after `## Code Standards`, or at end-of-file if that section is absent.
   - If the section is **present**: append to it as written; do not move it.
3. Append the rule as a new bullet under the section, preserving the existing formatting convention.
4. Do not reorder, rewrap, or edit any existing bullet.

### 6. Commit

1. Stage the single modified (or newly created) target file.
2. Commit per CLAUDE.md commit rules. Commit type `docs`; scope is `architecture-rule` for `CLAUDE.md` changes or the manual's topic for manual changes.
3. Include an `## AI Context` section recording the classification decision, the rejected alternative, and any user input that resolved ambiguity.

### 7. Report

Report to the user: rule text, final target path + section, commit hash.

## Judgments

### judge: classification

Assess whether the rule applies everywhere or only within a specific domain.

| Signal | Cross-cutting | Domain-scoped |
|---|---|---|
| Subject of the rule | Broad (any code, any module, any agent) | Names a module, subsystem, file family, or workflow |
| Verb / action | Structural invariants (dependencies, layering, naming, formatting) | Pattern in how a specific area solves something |
| Counterexample test | "A change in an unrelated area could still break this" | "Only matters when touching <that area>" |
| Downstream reader | Every skill or every file is affected | Only work inside one domain triggers the rule |

When both signals fire or neither dominates, return **ambiguous** and
prompt the user - do not guess. A rule that names a specific module but
whose invariant holds for all modules is still cross-cutting (the
example is just illustrative). Prefer cross-cutting when the rule names
a whole-codebase property.

### judge: domain-match

Applied only for domain-scoped rules.

| Outcome | When |
|---|---|
| Single clear domain | One declared row's `paths` and its manual's subject match the rule's subject; no other row is a plausible fit. |
| Multiple candidates | Two or more declared rows cover related paths or overlapping subject area. |
| No matching doc | No declared manual covers the rule's subject - a new manual is warranted. |

When two declared rows' `paths` nest, route to the row whose `paths` are the
narrowest set that still covers every path the rule governs.

## Templates

### Rule append

```markdown
## Rules

- <existing rule>.
- <new rule ending in a period>.
```

Use the same bullet form for `## Architecture Rules` in `CLAUDE.md`. Keep
each rule one sentence unless a second sentence is required to name a
hidden constraint.

### Commit message

```
docs(<scope>): add <cross-cutting|domain-scoped> rule - <short summary>

<target path> ## <section>: <rule text>

## AI Context
- Classification: <cross-cutting|domain-scoped>.
- Chose <target> because <rationale>; rejected <alternative> because <rationale>.
- <Any user input that resolved ambiguity>.
```

### New manual (when proposed)

When the user confirms creating a new manual, write it with a `## Rules`
section primed with the new rule, then declare it under
`AGENTS.md`'s `### Implementation Conventions` so path-scoped readers find it:

```markdown
# <Topic>

<one-line statement of what this manual governs>

## Rules

- <new rule>.
```

Add sibling sections only if the user asks - `{{.SkillNamespace}}:lead-add-rule`
is a rule authoring skill, not a manual scaffolding skill.

## Doctrine

`{{.SkillNamespace}}:lead-add-rule` optimizes for **classification accuracy at capture time**.
A mis-routed rule either dilutes `## Architecture Rules` or hides a cross-cutting
invariant in a path-scoped manual. The skill writes autonomously only when classification
is unambiguous, asks on ambiguity, never edits existing rule content, and
preserves the user's phrasing except grammar and active voice.
