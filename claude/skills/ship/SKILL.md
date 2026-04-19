---
name: ship
description: >
  Release a project: bump version, tag, build, and publish following
  the project's ship configuration. Reads ai-docs/ship/<proj>.md for
  instructions; if none exists, consults the user and writes the file
  first.
argument-hint: "[proj-name — optional if only one config exists]"
disable-model-invocation: true
---

# Ship

Target: $ARGUMENTS

## Invariants

- Never infer a version number without an explicit strategy in the ship config — ask if ambiguous.
- Never publish or push tags without user confirmation at the final gate.
- The ship config is the single source of truth; do not improvise steps not listed there.
- All written artifacts (ship config, version files) must be in English regardless of conversation language.

## On: invoke

### 1. Resolve config

1. Glob `ai-docs/ship/` for `*.md` files.
2. If `$ARGUMENTS` names a project, load `ai-docs/ship/<proj>.md`. Stop with an error if not found.
3. If no argument:
   - One file found → load it.
   - Multiple files found → list them and ask the user which project to ship.
   - No files found → go to **On: no config**.

### 2. Execute

Follow the loaded config exactly, section by section:

1. **Pre-flight** — run any listed checks (tests, lint, build).
2. **Version** — derive or bump the version per the config's version strategy.
3. **Tag** — create the git tag per the config. Do not push yet.
4. **Build / package** — run listed build or package commands.
5. **Confirm** — show the user: version string, tag, and publish targets. **Wait for explicit approval before proceeding.**
6. **Publish** — run listed publish commands (e.g. `cargo publish`, `npm publish`, `docker push`).
7. **Push tag** — `git push origin <tag>`.
8. **Post-ship** — run any listed post-ship steps.

Report what was done: version, tag, publish targets, any deviations.

## On: no config

The project has no ship config. Consult the user to establish one.

1. Ask:
   - Which sub-project or component is being shipped (determines `<proj>` name and file path).
   - Version strategy: options include semantic versioning with manual bump, auto-increment patch, date-based (`YYYY.MM.DD`), or `git describe`.
   - Build and package steps.
   - Publish targets and commands.
   - Post-ship steps (e.g. update changelog, notify).
2. Write the config to `ai-docs/ship/<proj>.md` using the format below.
3. Confirm the written config with the user before proceeding to **Execute**.

## Ship Config Format

```markdown
# Ship: <proj>

## Version Strategy
<how the version is derived or bumped — be specific enough that no judgment is needed at ship time>

## Pre-flight
- <check command>

## Build
- <build or package command>

## Publish
- <publish command and target>

## Tag
Format: `<prefix><version>` (e.g. `v1.2.3` or `proj-2024.04.19`)
Push: yes

## Post-ship
- <optional post-ship steps>
```

Omit sections that do not apply.

## Doctrine

Ship optimizes for **zero-surprise releases** — every step is either
prescribed in the config or confirmed by the user before execution.
The config is written once and reused, so the first invocation is the
only time judgment is required. When a rule is ambiguous, apply
whichever interpretation makes the next ship invocation require less
human input, not more.
