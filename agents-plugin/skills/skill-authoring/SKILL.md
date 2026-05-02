---
name: skill-authoring
description: Author or audit ws workflow skills and agent prompts using the repository skill-authoring rules. Use when creating, porting, normalizing, or reviewing SKILL.md files, agent prompt files, workflow instructions, invariants, constraints, handlers, judgments, templates, or doctrine text.
---

# Skill Authoring

## Invariants

- Put directives before rationale; put rationale only in Doctrine.
- Keep each invariant or constraint short enough to audit as one standalone rule.
- Keep skills self-contained; do not require tickets, session history, or sibling skills to understand the rule text.
- Keep agent prompts self-contained; do not require conversation history or caller session state.
- Mechanize repeatedly violated behavior with handlers, templates, or structured output blocks.
- Separate soft judgment from mechanical procedure; handlers reference named judgments instead of embedding criteria.
- Re-read added text at the end of every authoring pass and cut redundant or non-executable prose.

## On: Author Skill

1. Write frontmatter with only `name` and `description`.
2. Put all trigger conditions in `description`, because the body loads only after selection.
3. Start the body with `## Invariants` when the skill has hard universal rules.
4. Add `## On: <event>` handlers for entry points that require ordered execution.
5. Add `## Judgments` only for soft decisions that handlers must reference by name.
6. Add `## Templates` only for reusable output formats or invocation shapes.
7. End with `## Doctrine`, naming the finite resource and the ambiguity rule.

## On: Author Agent

1. Start with `## Identity`, one sentence that states what the agent is and does.
2. Add `## Constraints` for hard scope boundaries and universal rules.
3. Add `## Process` for the agent's linear work sequence.
4. Add `## Heuristics` only when the agent has non-mechanical decisions.
5. Add `## Output` defining the exact response contract.
6. End with `## Doctrine`, naming the finite resource and the ambiguity rule.

## On: Audit

1. Classify the document as skill or agent.
2. Check top-to-bottom section order against the matching layout.
3. Run `judge: invariant-quality` on every invariant or constraint line.
4. Run `judge: doctrine-quality` on the Doctrine paragraph.
5. Verify handlers contain procedure, judgments contain criteria, and Doctrine contains only the generator rationale.
6. When independent validation is available and authorized, request a fresh audit for contradictions, duplication, orphan references, and closure gaps.
7. Report contradictions, duplicate rules, orphan references, missing output contracts, and closure gaps.

## Judgments

### judge: invariant-quality

Accept a line only when every answer is yes:

- Can a concrete violation be described?
- Does it say what to do?
- Does it fit on one line?
- Does it make sense without surrounding prose?
- Does it add a rule not already present?
- Does it hold in every situation for this document?
- Can it be regenerated from the Doctrine paragraph?

### judge: doctrine-quality

Accept Doctrine only when it names one finite resource and includes this generator: when a rule is ambiguous, apply whichever interpretation better preserves that resource.

Concrete finite resources include context window, attention budget, execution steps, review time, and user turns. Reject fuzzy resources such as quality, focus, clarity, and robustness unless the sentence anchors them to a measurable constraint.

## Templates

### Skill Skeleton

```markdown
---
name: <skill-name>
description: <what the skill does and all trigger conditions>
---

# <Title>

## Invariants

- <hard universal rule>

## On: <event>

1. <ordered step>

## Judgments

### judge: <name>

<criteria>

## Templates

### <template name>

<format>

## Doctrine

<one paragraph naming the finite resource and ambiguity rule>
```

### Agent Skeleton

```markdown
# <Title>

## Identity

<one sentence>

## Constraints

- <hard universal rule>

## Process

1. <ordered step>

## Heuristics

### judge: <name>

<criteria>

## Output

<exact return format>

## Doctrine

<one paragraph naming the finite resource and ambiguity rule>
```

## Doctrine

Skill and agent documents optimize for the model's limited attention budget while executing under context pressure: directives stay skimmable, procedures stay mechanical, judgments stay named, and rationale stays isolated. When a rule is ambiguous, apply whichever interpretation better preserves the model's limited attention budget while executing under context pressure.
