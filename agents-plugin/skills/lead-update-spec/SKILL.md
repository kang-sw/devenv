---
name: lead-update-spec
description: Audit recent commits for caller-visible behavior changes and update ai-docs/spec accordingly. Use after implementation, edit, sprint, or release work when specs may need implemented entries, completed planned markers, or removal handling.
---

# Update Spec

Target: user request

## Invariants

- Lead-driven - no subagent delegation.
- Call `ws/convention.read(name: "spec-conventions")` before any write or read.
- Only add entries for confirmed-implemented features - no [planned] entries unless explicitly directed.
- Call `ws/spec_index.verify()` after any file modification.
- Commit all spec changes in a single `docs(spec): ...` commit.
- All written content must be in English regardless of conversation language.

## On: invoke

### 1. Load conventions

Call `ws/convention.read(name: "spec-conventions")`. Read `agents-plugin/skills/lead-write-spec/SKILL.md`.

### 2. Resolve commit range

- If `user request` contains a `..` range: use it as-is.
- If the calling skill recorded a `<start-commit>`: use `<start-commit>..HEAD`.
- Otherwise: run `git merge-base HEAD main` and use `<merge-base>..HEAD`.

### 3. Scan commits

Run `git log <range> --oneline`. Apply **judge: spec-impact** to each commit.

### 4. Add new entries

For each spec-impact commit:
1. Identify the affected spec domain. Read the relevant file(s) from `ai-docs/spec/`.
2. Check whether an entry already covers the new or changed behavior.
3. If missing: call `ws/spec_stem.generate(slug: "<slug>")` and insert an entry following the `spec-format` template from `write-spec/SKILL.md`.

### 5. Strip [planned]

For each `[planned]` entry:
1. Extract the stem.
2. Run `git log <range> --oneline | grep <stem>`. If matching commits exist and the feature is confirmed implemented: strip `[planned] ` from the heading and remove any `> [!note] Planned [planned]` callout block beneath it.

### 6. Handle removals

Run `git log <range> --format="%B" | grep "^removed:"`. For each `removed: <stem>` found: remove the corresponding spec entry from its file.

### 7. Finalize

If any spec file was modified:
1. Call `ws/spec_index.verify()`.
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
Spec: <N entries added, M [planned] stripped, K removed> | no changes
```

## Doctrine

Update-spec optimizes for **spec coverage at commit boundaries**. Caller-visible
source behavior should land in spec within the same sprint or implement run.
The lead's inline spec-impact judgment is the gate: no delegation, no suggestion
mode. When ambiguous, produce entries callers can verify without source.
