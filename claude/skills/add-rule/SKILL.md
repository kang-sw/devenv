---
name: add-rule
description: >
  Classify a natural-language rule as cross-cutting or domain-scoped and
  write it to the correct document. Use when the user states a rule
  ("always", "never", "must", or similar prescription) that should persist
  across sessions.
argument-hint: "<rule description>"
---

# Add Rule

Target: $ARGUMENTS

## Project Map

!`ws-list-mental-model`

## Invariants

- Never modify existing rule content — this skill only appends new rules. Correcting or editing a stale rule is the user's responsibility via manual edit.
- Never write the same rule to both `CLAUDE.md` and a mental-model doc in one invocation.
- One invocation writes to exactly one target file.
- All written rules are in English regardless of conversation language.
- Commit the change at the end following CLAUDE.md commit rules. Include `## AI Context` recording the classification decision.

## On: invoke

### 1. Read

1. Parse the rule from `$ARGUMENTS`. If `$ARGUMENTS` is empty, ask the user for the rule description and wait.
2. Read `CLAUDE.md` to see current `## Architecture Rules` entries and avoid near-duplicates.
3. Read the output of `ws-list-mental-model` (rendered above) for the current domain catalog and hierarchy.
4. Ancestor loading (one-level hierarchies — `<domain>/<sub>.md` only): for domain-scoped candidates, if any candidate target is a direct-child sub-domain doc (`mental-model/<domain>/<sub>.md`), read its parent `mental-model/<domain>/index.md` first — inherited `## Domain Rules` may already cover the rule.

### 2. Classify

Apply `judge: classification`. The decision is one of:

- **cross-cutting** — applies everywhere regardless of domain.
- **domain-scoped** — applies when working in a specific domain area.
- **ambiguous** — both plausible; user input required.

### 3. Route

| Classification | Route target |
|---|---|
| cross-cutting | `CLAUDE.md` → append to `## Architecture Rules`. |
| domain-scoped | Enumerate candidate domain docs; apply `judge: domain-match`. |
| ambiguous | Stop. Prompt the user with the two plausible classifications plus the best-match domain candidate; wait for selection. |

For domain-scoped rules, `judge: domain-match` yields one of:

| Match | Action |
|---|---|
| **single clear domain** | Target = that domain's `mental-model/<domain>.md` or `<domain>/index.md` when inherited across sub-domains. Propose and write. |
| **multiple candidates** | Stop. Present the candidate list with one-line rationales. Wait for user selection. |
| **no matching doc** | Stop. Propose creating a new `ai-docs/mental-model/<new-domain>.md` with the minimal frontmatter (`domain`, `description`, `sources`). Wait for user confirmation before writing. |

### 4. Propose or prompt

1. If the handler above resolved to a clear single target: state the target and the rule text verbatim in one line (`Target: <path> §<section>; rule: "<rule>".`). Proceed to §5.
2. Otherwise: surface the ambiguity, list options, and stop. Do not write. Resume from §5 once the user selects.

### 5. Write

1. Open the target doc.
2. Locate the target section (`## Architecture Rules` or `## Domain Rules`). If the section is absent:
   - In a mental-model doc: add the `## Domain Rules` heading immediately after the frontmatter body.
   - In `CLAUDE.md`: add the `## Architecture Rules` heading after `## Code Standards`, or at end-of-file if that section is absent.
3. Append the rule as a new bullet under the section, preserving the existing formatting convention.
4. Do not reorder, rewrap, or edit any existing bullet.

### 6. Commit

1. Stage the single modified (or newly created) target file.
2. Commit per CLAUDE.md commit rules. Commit type `docs`; scope is `architecture-rule` for `CLAUDE.md` changes or the domain name for mental-model changes.
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
prompt the user — do not guess. A rule that names a specific module but
whose invariant holds for all modules is still cross-cutting (the
example is just illustrative). Prefer cross-cutting when the rule names
a whole-codebase property.

### judge: domain-match

Applied only for domain-scoped rules.

| Outcome | When |
|---|---|
| Single clear domain | One domain doc's `domain`/`description`/`sources` matches the rule's subject; no other doc is a plausible fit. |
| Multiple candidates | Two or more domains cover related surface area or overlapping sources. |
| No matching doc | No existing domain doc covers the rule's subject — a new doc is warranted. |

For directory-layout domains (`<domain>/index.md` + children), route to
`index.md` when the rule applies across all sub-domains; route to the
specific child when it applies to only one sub-concern.

## Templates

### Rule append

```markdown
## Domain Rules

- <existing rule>.
- <new rule ending in a period>.
```

Use the same bullet form for `## Architecture Rules` in `CLAUDE.md`. Keep
each rule one sentence unless a second sentence is required to name a
hidden constraint.

### Commit message

```
docs(<scope>): add <cross-cutting|domain-scoped> rule — <short summary>

<target path> ## <section>: <rule text>

## AI Context
- Classification: <cross-cutting|domain-scoped>.
- Chose <target> because <rationale>; rejected <alternative> because <rationale>.
- <Any user input that resolved ambiguity>.
```

### New mental-model doc (when proposed)

When the user confirms creating a new domain doc, write it with the
minimal frontmatter and the `## Domain Rules` section primed with the
new rule:

```markdown
---
domain: <new-domain>
description: "<one-line summary of the domain's scope>"
sources:
  - <directory-pattern>/
---

# <Domain Name>

## Domain Rules

- <new rule>.
```

Add sibling sections (Entry Points, Module Contracts, etc.) only if the
user asks — `/add-rule` is a rule authoring skill, not a mental-model
scaffolding skill. `/forge-mental-model` or `/write-mental-model` owns
full-doc authorship.

## Doctrine

`/add-rule` optimizes for **classification accuracy at the moment a rule
is captured** — a mis-routed rule either dilutes `## Architecture Rules`
with domain trivia or hides a cross-cutting invariant inside a domain
doc where cross-domain work never sees it. The skill is autonomous when
the classification is unambiguous and paused on ambiguity; it never
modifies existing rule content, so every write is purely additive and
git-history recoverable. When a rule is ambiguous, apply whichever
interpretation better preserves classification accuracy — ask the user
when in doubt, never guess. Preserve the user's original phrasing — tighten only grammar and active voice.
