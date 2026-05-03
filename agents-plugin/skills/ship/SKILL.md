---
name: ship
description: Release a project by following its ai-docs/ship configuration. Use when the user asks to ship, release, publish, tag, or deploy a configured project.
---

# Ship

## Invariants

- Never infer a version number without an explicit version strategy in the ship config.
- Never publish artifacts or push tags without explicit user approval at the final gate.
- Treat the ship config as the single source of truth.
- Do not improvise steps that are absent from the config.
- Keep all AI-authored ship config content in English.

## On: Ship

1. Resolve the ship config with `judge: config-resolution`.
2. If no config exists, use `On: No Config`.
3. Read the selected config fully before executing any release step.
4. Run listed pre-flight checks.
5. Derive or bump the version exactly as specified by the config.
6. Create the configured tag locally; do not push it yet.
7. Run listed build or package commands.
8. Present version, tag, publish targets, and commands that will push or publish.
9. Wait for explicit user approval.
10. Run listed publish commands.
11. Push the configured tag only when the config says to push.
12. Run listed post-ship steps.
13. Report version, tag, publish targets, and any deviations.

## On: No Config

1. Ask which project or component is being shipped.
2. Ask whether the deploy target is public or private/sensitive.
3. Ask for the version strategy.
4. Ask for pre-flight, build, publish, tag, and post-ship steps.
5. Write `ai-docs/ship/<proj>.md` for public targets or `ai-docs/ship/<proj>.local.md` for private targets.
6. Present the config and wait for user confirmation before executing it.

## Judgments

### judge: config-resolution

If the user names a project, prefer `ai-docs/ship/<proj>.local.md` then `ai-docs/ship/<proj>.md`; without a project, use the only config when exactly one exists and ask when multiple exist.

## Templates

### Ship Config

```markdown
# Ship: <proj>

## Version Strategy
<how the version is derived or bumped>

## Pre-flight
- <check command>

## Build
- <build or package command>

## Publish
- <publish command and target>

## Tag
Format: `<prefix><version>`
Push: yes

## Post-ship
- <optional post-ship steps>
```

## Doctrine

Ship optimizes for zero-surprise releases: every irreversible step is either prescribed in the config or confirmed by the user before execution. When a rule is ambiguous, apply whichever interpretation makes the next ship invocation require less judgment at release time.
