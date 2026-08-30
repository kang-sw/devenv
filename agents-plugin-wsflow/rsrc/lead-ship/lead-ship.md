---
kind: print
---

# Ship

Target: user request

## Invariants

- Never infer a version number without an explicit strategy in the ship config - ask if ambiguous.
- Never publish or push tags without user confirmation at the final gate.
- The ship config is the single source of truth; do not improvise steps not listed there — except the **Release gate** below, which is un-omittable regardless of what the loaded config does or does not mention (it is still user-overridable at the gate itself, per its own stop-for-decision step).
- All written artifacts (ship config, version files) must be in English regardless of conversation language.

## On: invoke

### 1. Resolve config

Config naming:
- `ai-docs/ship/<proj>.md` - committed; for public publish targets.
- `ai-docs/ship/<proj>.local.md` - gitignored; for private or sensitive deploy targets (internal registries, SSH deploys, credentials). Takes precedence over the `.md` variant when both exist for the same `<proj>`.

1. List `ai-docs/ship/` for `*.md` and `*.local.md` files.
2. If `user request` names a project, look for `<proj>.local.md` first, then `<proj>.md`. Stop with an error if neither is found.
3. If no argument:
   - One config found (either variant) -> load it.
   - Multiple configs found -> list them (noting which are local) and ask the user which project to ship.
   - No configs found -> go to **On: no config**.

### 2. Release gate

Un-omittable, user-overridable. Applies only when the loaded project's `AGENTS.md` `### Review Policy` section declares `release-boundary: present`, read as plain prose the same way this skill and `ws.md`-style configs already read project config text — no MCP tool resolves this field. `release-boundary: absent` (or the field unset) skips this section entirely; that project's ship path is unchanged. Run this before **3. Execute** step 1 (Pre-flight), not as one of its bullets — a Pre-flight bullet is defeatable by a config that simply omits it, which is exactly what this gate must not be.

1. Call `{{.McpNamespace}}/review.marker(format: json)` to resolve the review-watermark frontier's `head` (the bare/structured output — never string-scrape the text-mode response).
2. Run `git rev-list --count <frontier-head>..HEAD`.
3. Empty (`0`) — proceed to **3. Execute**.
4. Non-empty — trigger `{{.SkillNamespace}}:lead-review` over `range: <frontier-head>..HEAD`.
   - Clears (the range now reviews clean) — proceed to **3. Execute**.
   - Still not clear — surface a strong recommendation against proceeding and **stop for an explicit user decision**.
5. On an explicit user override: proceed to **3. Execute** anyway. This gate never calls `review.stamp` — the marker stays exactly where it was found; only `{{.SkillNamespace}}:lead-review`'s own step 7 ever advances it (single-writer invariant). No audit record of the override is required or written by this mechanism.

### 3. Execute

Follow the loaded config exactly, section by section:

1. **Pre-flight** - run any listed checks (tests, lint, build).
2. **Version** - derive or bump the version per the config's version strategy.
3. **Tag** - create the git tag per the config. Do not push yet.
4. **Build / package** - run listed build or package commands.
5. **Confirm** - show the user: version string, tag, and publish targets. **Wait for explicit approval before proceeding.**
6. **Publish** - run listed publish commands (e.g. `cargo publish`, `npm publish`, `docker push`). When a publish step promotes one branch into another (e.g. `develop` -> `main`), pin the release-gate's reviewed through-SHA and re-assert it immediately before the merge, aborting and re-running the gate over the delta if the branch moved since — the project's own config supplies the concrete git incantation.
7. **Push tag** - `git push origin <tag>`.
8. **Post-ship** - run any listed post-ship steps.

Report what was done: version, tag, publish targets, any deviations.

## On: no config

The project has no ship config. Ask for:
- Sub-project/component, which determines `<proj>`.
- Public vs private/sensitive target; private targets use `<proj>.local.md`.
- Version strategy: manual semver, auto-increment patch, date-based (`YYYY.MM.DD`), `git describe`, or another explicit strategy.
- Build/package steps, publish/deploy commands, and post-ship steps.

Then:
1. Write the config to `ai-docs/ship/<proj>.md` or `ai-docs/ship/<proj>.local.md` depending on the answer above.
2. Confirm the written config with the user before proceeding to **Execute**.

## Ship Config Format

```markdown
# Ship: <proj>

## Version Strategy
<how the version is derived or bumped - be specific enough that no judgment is needed at ship time>

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

Ship optimizes for **zero-surprise releases**: every step is config-prescribed
or user-confirmed before execution. The first invocation captures judgment so
future invocations need less human input. When ambiguous, reduce future judgment.
