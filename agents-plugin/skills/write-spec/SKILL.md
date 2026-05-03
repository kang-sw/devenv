---
name: write-spec
description: Create or update behavioral spec documents for caller-visible workflow behavior. Use when the user asks to write, create, update, or audit a spec, or when a design discussion settles behavior that should be captured in ai-docs/spec.
---

# Write Spec

## Invariants

- Read `claude-plugin/infra/spec-conventions.md` before creating or changing spec files.
- Write specs from the external caller perspective, not from implementation structure.
- Keep all AI-authored spec content in English.
- Add `🚧` only when the feature has or will receive a `todo/`-or-higher ticket in the same session.
- Never remove `🚧` without confirming the feature is implemented.
- Give every named feature a stable `{#YYMMDD-slug}` anchor.
- Run spec index verification after every spec edit when the repository fallback is available.
- Do not depend on implicit `ws-*` PATH injection, shell interpolation, or Claude slash-command chaining.

## On: Write Spec

1. Identify the target behavior, area name, or existing spec file from the user request.
2. Apply `judge: spec-impact`; if there is no caller-visible behavior, report `No public behavior affected.` and stop.
3. Read `claude-plugin/infra/spec-conventions.md` until a host-neutral convention resource exists.
4. Inspect `ai-docs/spec/` and any likely target spec before choosing create versus update.
5. If creating a spec, choose the file shape with `judge: directory-vs-flat`.
6. Generate anchors with `judge: anchor-generation` before inserting new named features.
7. Write or update the spec using `Templates / Spec Entry`.
8. Apply `judge: planned-marker` before adding any `🚧` heading or Planned callout.
9. Apply `judge: split-trigger` after writing; split sections only when the criteria are met.
10. Verify implemented entries with `judge: accuracy-check`.
11. Run spec index verification with `judge: spec-index-verification`.
12. Commit only the spec files and directly required index changes.
13. Report the changed spec paths and any deferred verification.

## Judgments

### judge: spec-impact

Spec-impact exists when the work creates or changes behavior a caller can observe: commands, options, outputs, files, documented conventions, plugin surfaces, MCP tools, or workflow contracts.

### judge: directory-vs-flat

Use `ai-docs/spec/<area>/index.md` only when the area already has or clearly needs multiple child files; otherwise use `ai-docs/spec/<area>.md`.

### judge: anchor-generation

Prefer the MCP surface once `ws.spec_stem.generate` exists; until then use the repository fallback `ws-generate-spec-stem <slug>` or manually choose a collision-free `YYMMDD-slug` after searching all specs.

### judge: planned-marker

Use a `🚧` heading for a new unimplemented feature and a Planned callout for planned change to existing behavior; ensure a `todo/`-or-higher ticket exists before session end.

### judge: split-trigger

Split a section into its own file when it has an independent ticket lifecycle, repeated constraints callouts, or a distinct reader audience.

### judge: accuracy-check

For every heading without `🚧`, confirm the behavior exists through code, tests, docs, or an available focused delegate; if confirmation is missing, keep or add the planned/gap marker instead.

### judge: spec-index-verification

Prefer the MCP surface once spec index verification exists; until then run `ws-spec-build-index` when available and report when the fallback is unavailable.

## Templates

### Spec Entry

```markdown
## <Feature Name> {#YYMMDD-feature-name}

Behavioral description of what users, callers, hosts, or tools observe.

> [!note] Implementation Gap · YYYY-MM-DD
> Known-but-unscheduled incomplete behavior. No ticket yet.

> [!note] Planned 🚧
> Planned behavior. Current behavior remains unchanged until implemented.
```

### New Spec

```markdown
---
title: <Area / Feature Name>
summary: <One-line external-perspective summary>
---

# <Area / Feature Name>

<One-two sentence summary of what this provides to users or callers.>
```

## Doctrine

Spec writing optimizes for behavioral drift resistance: a future reader should learn what callers can rely on without reverse-engineering source code. When a rule is ambiguous, apply whichever interpretation makes the documented behavior easier to verify from outside the implementation.
