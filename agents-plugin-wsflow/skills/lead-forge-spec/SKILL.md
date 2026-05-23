---
name: lead-forge-spec
description: Reconstruct specs from scratch by surveying the project, confirming behavioral domains, and writing verified anchor-keyed specs under ai-docs/spec/.
---

# Forge Spec

Target: repository spec reconstruction

## Invariants

- Call `wsflow/convention.read(name: "spec-conventions")` before any spec write.
- All surveys are read-only and self-contained.
- Use direct exploration or subagent investigation for bounded read-only surveys.
- Lead owns domain selection, anchor generation, document integration, final judgment, and commits.
- Call `wsflow/spec_stem.generate(slug: "<descriptive-slug>")` before every anchor insertion.
- Call `wsflow/spec_index.verify()` after every spec file write or update.
- Domain task names use the prefix `forge-spec-<domain>` only as local progress labels.
- Stop for user confirmation before writing or replacing domain specs.

## On: invoke

### 1. Resume Check

1. Inspect current repo state and existing `ai-docs/spec/` files.
2. If partial forge work exists, identify completed domains, open domains, and pending user confirmations.
3. Continue from the first unfinished step instead of restarting.

### 2. Repository Survey

Run the following surveys through direct project exploration or subagent
workers; each worker prompt must include the exact
question, read-only boundary, expected bullet output, and permission to use
wsflow read tools such as `wsflow/project_tree`, `wsflow/tickets.*`,
`wsflow/specs.*`, `wsflow/mental_models.*`, and `wsflow/git.*`.

Survey:

1. **Behavioral surface** - user-visible commands, files, plugins, tools,
   workflow skills, conventions, and configuration.
2. **Documentation inventory** - existing specs, tickets, mental models,
   references, and stale or duplicate behavior descriptions.
3. **Implementation map** - source directories and modules that implement
   observable behavior.
4. **History hints** - recent commits and ticket links that reveal behavior
   not obvious from current docs.

Synthesize a proposed domain list. For each domain include:
- domain name;
- caller-visible behavior it owns;
- likely spec file path;
- evidence paths;
- unresolved questions.

Ask the user to confirm, split, merge, drop, or reorder domains before writing.

### 3. Domain Survey

For each confirmed domain, run focused read-only investigation:

1. Identify implemented behavior that callers can observe.
2. Identify contract-first planned implementation behavior backed by non-epic/non-research ready tickets, plus epic/research planned decomposition or investigation text and other planned behavior for survey evidence.
3. Identify behavior that looks documented but unimplemented.
4. Identify tickets in `ready/` whose phases belong to the domain.
5. Identify spec anchors that should be reused, renamed, added, or removed.

Use direct exploration or subagent workers; keep prompts
self-contained and ask for file paths, stems, and short evidence notes.

### 4. Write Domain Spec

1. Call `wsflow/convention.read(name: "spec-conventions")` and read the output.
2. Draft the spec from caller-visible behavior only.
3. For each new implemented or planned behavior anchor, call
   `wsflow/spec_stem.generate(slug: "<descriptive-slug>")`.
4. Mark unimplemented ready-ticket behavior with `🚧` only when it is
   contract-first planned behavior following spec conventions.
5. Keep implementation details out unless they are the public contract.
6. Call `wsflow/spec_index.verify()`.
7. Ask the user to confirm the domain spec before committing if the forge pass
   is replacing broad existing coverage.

### 5. Ticket Association Check

1. From the domain survey, collect relevant `ready/` tickets.
2. Use direct inspection or one subagent audit to check whether
   each ticket phase maps to a spec stem.
3. Resolve open ticket/spec association questions with the user before commit.

### 6. Commit

Commit each completed domain or small batch through
`wsflow/git.commit(paths: ["ai-docs/spec/...", "ai-docs/_index.md"], title: "docs(spec): forge <domain> coverage", ai_context: ["<summary>"])`.

### 7. Wrap Up

1. Call `wsflow/spec_index.verify()` as a final safety pass.
2. Report completed domains, skipped domains, open questions, and suggested
   follow-up tickets or spec edits.
3. Suggest `wsflow:lead-forge-mental-model` when mental-model baselines are
   absent or obviously stale.

## Judgments

### judge: directory-vs-flat

Use a directory (`<area>/index.md` plus child files) when the domain naturally
has multiple independently maintained sub-surfaces. Use a flat file for a
single cohesive behavior surface. When uncertain, start flat and note split
candidates.

### judge: implemented-vs-planned

Write implemented behavior without `🚧` only when source or committed history
confirms it exists. Write planned implementation behavior with `🚧` only when it
is contract-first and backed by a non-`epic`, non-`research` ready ticket.
Epic or research tickets may back `🚧` text only for planned decomposition or
investigation outputs. Otherwise record the gap in the survey report, not the
spec body.

## Templates

### Survey Prompt Shape

```text
Read-only survey.
Question: <exact domain or repository question>
Use wsflow read tools where useful: wsflow/project_tree, wsflow/specs.*, wsflow/tickets.*, wsflow/mental_models.*, wsflow/git.*.
Return:
- findings: concise bullets with file paths or stems
- likely spec domains or anchors
- unresolved questions
Do not edit files.
```

### wsflow/spec_index.verify call

```text
wsflow/spec_index.verify()
```

## Doctrine

Forge-spec optimizes for **recovering behavioral truth from incomplete memory**.
Surveys gather evidence, but the lead owns domain boundaries, spec integration,
and traceability. When ambiguous, preserve caller-visible behavior and ask the
user before replacing broad coverage.
