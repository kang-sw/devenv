---
name: update-spec
description: >
  Lead-driven spec audit for a commit range. Loads conventions, scans commits
  for caller-visible behavior changes, adds missing entries, strips 🚧 markers,
  and handles removals. Invoked by implement, edit, and sprint wrap-up.
argument-hint: "[<start-commit>..<end-commit>]"
---

# Update Spec

Target: $ARGUMENTS

## Invariants

- Lead-driven — no subagent delegation.
- Load spec-conventions before any write or read.
- Only add entries for confirmed-implemented features — no 🚧 entries unless explicitly directed.
- Run `ws-spec-build-index` after any file modification.
- Commit all spec changes in a single `docs(spec): ...` commit.
- All written content must be in English regardless of conversation language.

## On: invoke

### 1. Load conventions

Run `ws-print-infra spec-conventions.md` (Bash). Read `claude-plugin/skills/write-spec/SKILL.md`.

### 2. Resolve commit range

- If `$ARGUMENTS` contains a `..` range: use it as-is.
- If the calling skill recorded a `<start-commit>`: use `<start-commit>..HEAD`.
- Otherwise: run `git merge-base HEAD main` and use `<merge-base>..HEAD`.

### 3. Scan commits

Run `git log <range> --oneline`. Apply **judge: spec-impact** to each commit.

### 4. Add new entries

For each commit with spec-impact:
1. Identify the affected spec domain. Read the relevant file(s) from `ai-docs/spec/`.
2. Check whether an entry already covers the new or changed behavior.
3. If missing: run `ws-generate-spec-stem <slug>` and insert an entry following the `spec-format` template from `write-spec/SKILL.md`.

### 5. Strip 🚧

For each `🚧` entry in any spec file:
1. Extract the stem.
2. Run `git log <range> --oneline | grep <stem>`. If matching commits exist and the feature is confirmed implemented: strip `🚧 ` from the heading and remove any `> [!note] Planned 🚧` callout block beneath it.

### 6. Handle removals

Run `git log <range> --format="%B" | grep "^removed:"`. For each `removed: <stem>` found: remove the corresponding spec entry from its file.

### 7. Finalize

If any spec file was modified:
1. Run `ws-spec-build-index`.
2. Commit: `git add ai-docs/spec/ && git commit -m "docs(spec): ..."`.

If no changes: output `Spec: no changes.`

## Judgments

### judge: spec-impact

**Qualifies (add or update an entry):**
- New CLI flag, subcommand, option, or environment variable
- Changed output format or return value
- New convention or contract a caller must follow
- Changed behavior in an existing documented feature

**Does not qualify:**
- Internal refactors that preserve all caller-visible behavior
- Bug fixes that restore documented expected behavior (not introducing new observable behavior)
- Doc-only or infra-only changes
- Platform portability fixes that don't expose a new interface

When borderline: err toward adding an entry. A false-positive entry is easier to remove than a missing one is to discover later.

## Templates

### Completion report

```
Spec: <N entries added, M 🚧 stripped, K removed> | no changes
```

## Doctrine

Update-spec optimizes for **spec coverage at commit boundaries** — every caller-visible
behavior change that lands in source should land in spec within the same sprint or
implement run. The lead's inline judgment on spec-impact is the gate; no delegation,
no suggestion mode. When a rule is ambiguous, apply whichever interpretation produces
spec entries a caller could verify without reading source code.
