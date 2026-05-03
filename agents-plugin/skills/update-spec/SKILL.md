---
name: update-spec
description: Audit recent commits for caller-visible behavior changes and update ai-docs/spec accordingly. Use after implementation, edit, sprint, or release work when specs may need implemented entries, completed planned markers, or removal handling.
---

# Update Spec

## Invariants

- Lead the audit directly unless the user explicitly authorizes delegation.
- Call `ws/convention.read` for `spec-conventions` before changing specs.
- Add entries only for confirmed implemented behavior unless the user explicitly asks for planned spec text.
- Strip `🚧` only after confirming the feature is implemented.
- Keep all AI-authored spec content in English.
- Call `ws/spec_index.verify` after any spec edit.
- Commit all spec changes in one `docs(spec): ...` commit.
- Do not read convention files from host-local plugin source paths.

## On: Update Spec

1. Resolve the commit range with `judge: commit-range`.
2. Call MCP tool `ws/convention.read` with `{"name":"spec-conventions"}`.
3. Run `git log <range> --oneline` and inspect commit bodies when needed.
4. Apply `judge: spec-impact` to each commit.
5. For each impacting commit, identify the affected spec domain and read the relevant files.
6. Add a missing implemented entry using `Templates / Implemented Entry` and `ws/spec_stem.generate`.
7. Scan existing `🚧` entries and Planned callouts for stems completed by the range.
8. Strip completed `🚧` markers only after confirming implementation.
9. Handle `removed: <stem>` markers with `judge: removal-handling`.
10. Call MCP tool `ws/spec_index.verify` if any spec changed.
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

Call `ws/spec_stem.generate` with `{"slug":"<descriptive-slug>"}` and use the returned `YYMMDD-slug` exactly.

### judge: removal-handling

For each `removed: <stem>` commit marker, find the matching spec entry, confirm the behavior was removed, and remove or mark the entry according to spec conventions.

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
