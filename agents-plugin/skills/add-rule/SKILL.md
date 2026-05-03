---
name: add-rule
description: Classify a natural-language rule as cross-cutting or domain-scoped and write it to the correct document. Use when the user states a persistent rule with words such as always, never, must, or should.
---

# Add Rule

## Invariants

- Never modify existing rule content; only append a new rule.
- Write to exactly one target file per invocation.
- Never write the same rule to both `CLAUDE.md` and a mental-model document.
- Keep all AI-authored rule content in English.
- Use `ws.convention.read` for mental-model conventions.
- Use `ws.mental_models.list` for the current domain catalog.
- Commit the single target-file change with `## AI Context` recording the classification decision.

## On: Add Rule

1. Parse the rule from the user request; if missing, ask for the rule text and wait.
2. Call MCP tool `ws.convention.read` with `{"name":"mental-model-conventions"}`.
3. Call MCP tool `ws.mental_models.list`.
4. Read `CLAUDE.md` to inspect current `## Architecture Rules`.
5. Read candidate mental-model documents before selecting a domain target.
6. Apply `judge: classification`.
7. Route cross-cutting rules to `CLAUDE.md` `## Architecture Rules`.
8. Route domain-scoped rules with `judge: domain-match`.
9. Stop and ask the user when classification or domain target is ambiguous.
10. State the resolved target and rule text before writing.
11. Append the rule without reordering, rewrapping, or editing existing bullets.
12. Commit only the target file.
13. Report the rule text, final target path, section, and commit hash.

## Judgments

### judge: classification

Cross-cutting rules apply regardless of domain; domain-scoped rules apply only when working in a named module, subsystem, file family, or workflow.

### judge: domain-match

Choose a single domain only when one mental-model document clearly matches the rule's subject by domain, description, or sources; otherwise ask the user to select or approve creating a new domain document.

### judge: section-placement

In mental-model documents, `## Domain Rules` belongs immediately after the frontmatter body; if it exists elsewhere, report the misplaced section before appending.

## Templates

### Rule Append

```markdown
## Domain Rules

- <new rule ending in a period>.
```

Use the same bullet form for `## Architecture Rules` in `CLAUDE.md`.

### Commit Message

```text
docs(<scope>): add <cross-cutting|domain-scoped> rule

<target path> ## <section>: <rule text>

## AI Context
- Classification: <cross-cutting|domain-scoped>.
- Chose <target> because <rationale>; rejected <alternative> because <rationale>.
```

## Doctrine

Add-rule optimizes for classification accuracy when a rule is captured: a misrouted rule either dilutes global guidance or hides a global invariant in a domain document. When a rule is ambiguous, apply whichever interpretation better preserves classification accuracy, and ask the user instead of guessing.
