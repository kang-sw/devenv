---
kind: print
delegates: true
---

# Forge Mental Model

Target: user request

## Invariants

- Call `{{.McpNamespace}}/convention.read(name: "mental-model-conventions")` before any document write - conventions are canonical there.
- All survey and verifier queries spawn host-native broad-scope exploration workers directly with the listed query blocks as task prompts; require cited evidence, gaps, and follow-up needs.
- No domain file is written without completing the survey for that domain first.
- Domain list must be explicitly confirmed by the user before any file is written.
- Domain todo items use the key and title prefix `forge-mental-model-<domain>` (e.g., `forge-mental-model-auth`); keep the prefix stable for resume scanning and derive the key from the domain name for status mutation.
- All survey exploration workers for a phase are spawned in a single response turn when the host can issue parallel spawns; collect all results before synthesizing.
- Every commit touching `ai-docs/mental-model/` or `ai-docs/mental-model.md` must include `(mental-model-updated)` in the message body.

## On: invoke

1. Call `{{.McpNamespace}}/todo.list(session_key: <lead key>, mode: "full")` and scan rendered item titles for the `forge-mental-model-` prefix.
2. If matching todo items exist -> skip to **On: per-domain** with the first whose status marker is not done (`- [x]`).
3. If no matching todo items exist -> proceed to **On: cold-start**.

## On: cold-start

### 1. Spec gate (soft)

Check whether `ai-docs/spec/` exists and contains at least one file:

```text
ls ai-docs/spec/ 2>/dev/null | head -1
```

If absent or empty: surface the warning below and proceed - do not block.

> No spec found - mental-model will be built without spec stem cross-references.
> Run `{{.SkillNamespace}}:lead-forge-spec` first for full cross-reference support.

Record whether spec is available (drives step 4 per domain).

### 2. Parallel codebase survey

Spawn all three host-native exploration workers with broad-tracing scope in a single response turn, using the query blocks below as task prompts. Require cited evidence, gaps, and follow-up needs. Collect all results before synthesizing.

Query 1 — directory and module structure:

```text
Survey the project's directory and module structure.

Enumerate top-level modules/packages/service boundaries. For each, identify
responsibility, outward-facing interfaces, and file count as a size signal.
Return markdown bullets by module/area.
```

Query 2 — entry points and cross-module contracts:

```text
Survey the project for entry points and cross-module contracts.

Find main entry points and cross-module contracts: trait impls, protocols,
interfaces, plugin registries, configuration schemas. Return entry points and
contracts with coupling direction.
```

Query 3 — coupling hotspots and implicit contracts:

```text
Survey the project for coupling hotspots and implicit contracts - areas that cause wrong outcomes for a developer who modifies them without knowing the contract.

Look for: shared mutable state, ordering dependencies, sync points, extension registries, global config reads, event buses, or any code that must be called in a specific order.

For each hotspot, return modules, contract, and failure mode.
```

Collect each subagent result before synthesizing.

### 3. Synthesize domain candidates

1. Cross-reference module boundaries, entry points, and coupling hotspots.
2. Produce one candidate domain per coherent operational area.
3. For each candidate, list source paths, owned coupling, and existing mental-model coverage.

### 4. User domain confirmation

Present the candidate domains to the user in a numbered list. Tell the user they may reorder, merge, split, rename, or drop entries before proceeding.

Wait for user response. Apply any adjustments. Do not proceed until the user explicitly confirms the final list.

### 5. Lock the todo list

Call `{{.McpNamespace}}/todo.clear(session_key: <lead key>)`, then one `{{.McpNamespace}}/todo.append` per confirmed domain, in confirmed order:

```text
{{.McpNamespace}}/todo.append(session_key: <lead key>, key: "forge-mental-model-<domain>", title: "forge-mental-model-<domain> - Source paths: <paths>; spec available: <yes | no>")
```

Proceed immediately to **On: per-domain** with the first domain.

## On: per-domain

For each domain task in order, skipping tasks with status `completed`:

### 1. Mark in-progress

Call `{{.McpNamespace}}/todo.check(session_key: <lead key>, key: "forge-mental-model-<domain>", status: "wip")`.

### 2. Domain survey

Spawn a host-native broad-scope exploration worker with the task prompt below; collect the result before drafting:

```text
Analyze domain: <domain>
Source paths: <paths from the todo item title>

Analyze this domain for a developer who needs to modify it.
Focus on what would cause wrong outcomes if unknown:
1. Implicit contracts between modules (ordering, data flow, sync)
2. Coupling (changes here -> must also change there)
3. Extension points (registries, enums, plugin interfaces, config)
4. Fragile areas (invariants that break silently or cause wrong results, known debt)
5. Common mistakes (forgetting required steps, wrong outcomes)
6. Distinguish existing patterns from scaffolded/planned features.

Be concrete: cite paths, functions, and types. Do not list fields or paraphrase functions.
```

### 3. Draft domain file

1. Call `{{.McpNamespace}}/convention.read(name: "mental-model-conventions")`. Read the output; apply the inclusion test to every claim before writing it.
2. Draft the domain file content for `ai-docs/mental-model/<domain>.md` following the document format in `mental-model-conventions.md`.
3. Set frontmatter: `domain` (filename stem), `description` (one-line scope summary), `sources` (directory patterns from the todo item title), `related` (other domains with coupling to this one).

### 4. Embed spec stems (conditional)

If spec is available (recorded in cold-start step 1):

1. Inspect `ai-docs/spec/**/*.md` directly to collect all `{#YYMMDD-slug}` anchors in the repo.
2. For each section in the domain draft: identify spec stems whose behavior corresponds to the section's topic. Embed the stem inline in the relevant body text (e.g., `{#260421-feature-name}`).

Skip if no spec exists.

### 5. Verify

Spawn a host-native broad-scope exploration worker with the task prompt below; collect the result before applying corrections:

```text
Verify the following mental-model domain document against the codebase.

Domain file draft:
<full draft content>

Source paths to check: <paths from the todo item title>

For each claim, assign a severity:
- [HIGH] Factually wrong - misnames a function, inverts a dependency, states a constraint that is not enforced.
- [LOW] Incomplete - a relevant contract or coupling is missing.
- [STALE] References removed code or an old API.
- [BLOAT] Fails the inclusion test - type/field listing, paraphrase of what a function does, or content derivable without cost.

Return a finding list. Each finding: severity tag, location in draft, correction or suggested removal.
```

Process verifier output:
- **[HIGH]**: Apply corrections to the draft directly.
- **[LOW]**: Add to draft if clearly relevant; otherwise collect for user summary.
- **[STALE]**: Rewrite or remove the section.
- **[BLOAT]**: Remove - content fails inclusion test.

### 6. Write file

Write the verified draft to `ai-docs/mental-model/<domain>.md`. Commit with `(mental-model-updated)` in the message body.

### 7. Complete domain

1. Call `{{.McpNamespace}}/todo.check(session_key: <lead key>, key: "forge-mental-model-<domain>", status: "done")`.
2. If more domain todo items remain, continue with the next incomplete one.
3. When every `forge-mental-model-` todo item is done, proceed to **On: wrap-up**.

## On: wrap-up

### 1. Update mental-model index

Update `ai-docs/mental-model.md`:
- Add a row to the domains table for each newly created domain file.
- Update shared conventions if new cross-domain patterns emerged.

Commit with `(mental-model-updated)` in the message body.

### 2. Summary report

```
## Forge Mental Model - Complete

Domains covered: <N>
Domain files created: <list of paths>
Spec stems embedded: <count, or 'none (no spec found)'>
Verifier corrections applied: <count>
Items for user review: <LOW findings list, or 'none'>
```

### 3. Suggested next steps

- Run `{{.SkillNamespace}}:lead-forge-spec` if spec was absent - mental-model was built without stem cross-references.
- Run `mental-model-updater` agent after future code changes to keep domain files current.

## Judgments

### judge: spec-gate (soft)

Check `ai-docs/spec/` on cold-start. If absent or empty: warn and proceed. Do not block.

## Doctrine

Forge-mental-model optimizes for **confirmed operational knowledge per domain**:
each file follows a survey-and-verify cycle before writing. Spec stems are
opportunistic. When ambiguous, complete survey and verification before any write.
