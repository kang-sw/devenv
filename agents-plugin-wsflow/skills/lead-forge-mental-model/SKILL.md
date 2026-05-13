---
name: lead-forge-mental-model
description: Reconstruct mental-model documents from scratch by surveying operational domains and writing verified modification-focused domain files under ai-docs/mental-model/.
---

# Forge Mental Model

Target: repository mental-model reconstruction

## Invariants

- Call `wsflow/convention.read(name: "mental-model-conventions")` before any document write.
- All surveys and verification passes are read-only and self-contained.
- Use direct exploration or subagent investigation for bounded read-only surveys.
- Lead owns domain selection, document integration, final judgment, and commits.
- Mental-model content must pass the inclusion test: wrong-outcome risk and not quickly derivable from source.
- Preserve user-authored Domain Rules exactly; move rules only when conventions require ancestor promotion.
- Stop for user confirmation before replacing broad domain coverage.

## On: invoke

### 1. Resume Check

1. Inspect current repo state and existing `ai-docs/mental-model/` files.
2. If partial forge work exists, identify completed domains, open domains, and pending user confirmations.
3. Continue from the first unfinished step instead of restarting.

### 2. Repository Survey

Run the following surveys through direct project exploration or subagent
workers; each worker prompt must include the exact
question, read-only boundary, expected bullet output, and permission to use
wsflow read tools such as `wsflow/project_tree`, `wsflow/specs.*`,
`wsflow/tickets.*`, `wsflow/mental_models.*`, and `wsflow/git.*`.

Survey:

1. **Operational domains** - modules or document groups where wrong edits are
   likely without implicit context.
2. **Coupling map** - cross-domain dependencies, generated artifacts, runtime
   contracts, release paths, and workflow boundaries.
3. **Existing knowledge** - current mental models, specs, tickets, references,
   and stale or duplicated operational rules.

Synthesize a proposed mental-model domain list. For each domain include:
- domain name and file path;
- source directories or document roots;
- related specs or tickets;
- modification risks that justify a mental model;
- unresolved questions.

Ask the user to confirm, split, merge, drop, or reorder domains before writing.

### 3. Domain Survey

For each confirmed domain, run focused read-only investigation:

1. Entry points a future editor should read first.
2. Module contracts that are easy to violate.
3. Coupling with other domains.
4. Extension points and change recipes.
5. Common mistakes with wrong outcomes.
6. Technical debt that changes implementation decisions.
7. Related spec stems worth referencing inline.

Use direct exploration or subagent workers; keep prompts
self-contained and ask for file paths, stems, and short evidence notes.

### 4. Write Domain Document

1. Call `wsflow/convention.read(name: "mental-model-conventions")` and read the output.
2. Draft only modification-relevant knowledge that passes the inclusion test.
3. Use frontmatter with `domain`, `description`, `sources`, and optional `related`.
4. Reference related spec stems inline when useful.
5. Preserve existing Domain Rules exactly; ask the user before changing rule text.
6. If a domain needs sub-domains, create an `index.md` parent and child files per convention.
7. Ask the user to confirm the domain document before committing if the forge pass replaces broad existing coverage.

### 5. Verification

Verify each drafted domain with direct reread or one subagent audit:

```text
Read-only mental-model audit.
Question: Does this draft include only modification-relevant knowledge, preserve Domain Rules, and avoid source paraphrase?
Use wsflow read tools where useful: wsflow/specs.*, wsflow/tickets.*, wsflow/mental_models.*, wsflow/git.*.
Return:
- contradictions or unsupported claims
- missing wrong-outcome risks
- source-paraphrase sections to cut
- stale rule concerns for the user to resolve
Do not edit files.
```

Resolve findings before commit or record open questions for the user.

### 6. Commit

Commit each completed domain or small batch through
`wsflow/git.commit(paths: ["ai-docs/mental-model/...", "ai-docs/mental-model.md", "ai-docs/_index.md"], title: "docs(mental-model): forge <domain> model", ai_context: ["<summary>"])`.
Include `(mental-model-updated)` in the commit body.

### 7. Wrap Up

Report completed domains, skipped domains, stale rule concerns, and follow-up
spec or ticket work. Suggest `wsflow:lead-forge-spec` when specs are absent and
mental-model cross-references could not be anchored.

## Judgments

### judge: include-claim

Include a claim only when ignorance can cause a wrong edit and the claim is not
derivable from entry-point files in under 30 seconds. Otherwise omit it.

### judge: split-domain

Split a flat domain into a directory when multiple child domains have distinct
entry points, coupling, and change recipes. Keep a parent `index.md` for shared
rules and cross-cutting context.

## Templates

### Survey Prompt Shape

```text
Read-only survey.
Question: <exact mental-model domain question>
Use wsflow read tools where useful: wsflow/project_tree, wsflow/specs.*, wsflow/tickets.*, wsflow/mental_models.*, wsflow/git.*.
Return:
- modification risks
- file paths or stems as evidence
- candidate domain rules or stale rule concerns
- unresolved questions
Do not edit files.
```

## Doctrine

Forge-mental-model optimizes for **recovering modification judgment from
incomplete memory**. Surveys gather evidence, but the lead owns domain
boundaries, document integration, inclusion-test discipline, and rule
preservation. When ambiguous, exclude derivable facts and ask the user before
changing durable rules.
