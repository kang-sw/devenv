---
name: update-spec
description: Audit recent commits for caller-visible behavior changes and update ai-docs/spec accordingly. Use after implementation, edit, sprint, or release work when specs may need implemented entries, completed planned markers, or removal handling.
---

# Update Spec

## Invariants

- Lead the audit directly unless the user explicitly authorizes delegation.
- Read `claude-plugin/infra/spec-conventions.md` before changing specs.
- Add entries only for confirmed implemented behavior unless the user explicitly asks for planned spec text.
- Strip `🚧` only after confirming the feature is implemented.
- Keep all AI-authored spec content in English.
- Run spec index verification after any spec edit when the repository fallback is available.
- Commit all spec changes in one `docs(spec): ...` commit.
- Do not depend on implicit `ws-*` PATH injection, shell interpolation, or Claude named-agent orchestration.

## On: Update Spec

1. Resolve the commit range with `judge: commit-range`.
2. Read `claude-plugin/infra/spec-conventions.md` until a host-neutral convention resource exists.
3. Run `git log <range> --oneline` and inspect commit bodies when needed.
4. Apply `judge: spec-impact` to each commit.
5. For each impacting commit, identify the affected spec domain and read the relevant files.
6. Add a missing implemented entry using `Templates / Implemented Entry` and `judge: anchor-generation`.
7. Scan existing `🚧` entries and Planned callouts for stems completed by the range.
8. Strip completed `🚧` markers only after confirming implementation.
9. Handle `removed: <stem>` markers with `judge: removal-handling`.
10. Run spec index verification with `judge: spec-index-verification` if any spec changed.
11. Commit only the changed spec files and directly required index changes.
12. Report `Spec: <N entries added, M markers stripped, K removals handled>` or `Spec: no changes.`

## Judgments

### judge: commit-range

Use an explicit `A..B` range from the user when provided; otherwise use the caller-supplied start commit when available; otherwise use `git merge-base HEAD main` through `HEAD`.

### judge: spec-impact

Qualifying changes alter caller-visible commands, options, outputs, files, plugin surfaces, MCP tools, documented conventions, or workflow contracts.

### judge: non-impact

Do not add spec entries for internal refactors, documentation-only edits, test-only changes, or bug fixes that merely restore already documented behavior.

### judge: anchor-generation

Prefer the MCP surface once `ws.spec_stem.generate` exists; until then use `ws-generate-spec-stem <slug>` when available or manually choose a collision-free `YYMMDD-slug` after searching all specs.

### judge: removal-handling

For each `removed: <stem>` commit marker, find the matching spec entry, confirm the behavior was removed, and remove or mark the entry according to spec conventions.

### judge: spec-index-verification

Prefer the MCP surface once spec index verification exists; until then run `ws-spec-build-index` when available and report when the fallback is unavailable.

## Templates

### Completion Report

```text
Spec: <N entries added, M markers stripped, K removals handled>
```

```text
Spec: no changes.
```

### Implemented Entry

```markdown
## <Feature Name> {#YYMMDD-feature-name}

Behavioral description of what users, callers, hosts, or tools observe.
```

## Doctrine

Update-spec optimizes for spec coverage at commit boundaries: every caller-visible behavior change that lands in source should be represented in specs before the work is considered wrapped. When a rule is ambiguous, apply whichever interpretation leaves future readers less dependent on commit archaeology.
