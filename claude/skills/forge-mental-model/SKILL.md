---
name: forge-mental-model
description: >
  From-scratch mental-model construction for a project. Surveys the codebase,
  identifies operational domains, and guides an author loop that produces
  complete domain files under ai-docs/mental-model/.
disable-model-invocation: true
argument-hint: "[target domain or 'all']"
---

# Forge Mental Model

Target: $ARGUMENTS

## Invariants

- Run `ws-print-infra mental-model-conventions.md` (Bash) before any document write — conventions are canonical there.
- Every `Agent()` dispatch carries explicit `model = "sonnet"` — never inherited.
- No domain file is written without completing the survey for that domain first.
- Domain list must be explicitly confirmed by the user before any file is written.
- Domain task names use the prefix `forge-mental-model-<domain>` (e.g., `forge-mental-model-auth`). Renaming tasks breaks cross-compact resume detection.
- All survey subagents for a phase are dispatched in a single response turn (parallel).
- Every commit touching `ai-docs/mental-model/` or `ai-docs/mental-model/index.md` must include `(mental-model-updated)` in the message body.

## On: invoke

1. Call `TaskList` and scan for tasks whose name begins with `forge-mental-model-`.
2. If matching tasks exist → skip to **On: per-domain** with the first task whose status is not `completed`.
3. If no matching tasks exist → proceed to **On: cold-start**.

## On: cold-start

### 1. Spec gate (soft)

Check whether `ai-docs/spec/` exists and contains at least one file:

```bash
ls ai-docs/spec/ 2>/dev/null | head -1
```

If absent or empty: surface the warning below and proceed — do not block.

> No spec found — mental-model will be built without spec stem cross-references.
> Run `/forge-spec` first for full cross-reference support.

Record whether spec is available (drives step 4 per domain).

### 2. Parallel codebase survey

Dispatch all three survey subagents in a single response turn:

```
Agent(
  name = "survey-structure",
  description = "Survey directory and module structure",
  subagent_type = "Explore",
  model = "sonnet",
  prompt = """
    Survey the project's directory and module structure.

    Steps:
    1. Enumerate the source tree: find top-level modules, packages, or service
       boundaries. Use glob/find as needed.
    2. For each boundary: identify its apparent responsibility and whether it
       has outward-facing interfaces (APIs, CLI commands, config options).

    Return: a bullet list of module/area names with a one-line responsibility
    description each. Include file count per area as a rough size signal.
  """
)

Agent(
  name = "survey-entry-points",
  description = "Survey entry points and cross-module contracts",
  subagent_type = "Explore",
  model = "sonnet",
  prompt = """
    Survey the project for entry points and cross-module contracts.

    Steps:
    1. Find main entry points (e.g., main.rs, __main__.py, index.ts, bin/).
    2. For each entry point: identify what it orchestrates, what modules it
       depends on, and what outputs it produces.
    3. Identify shared contracts: trait impls, protocols, interface files,
       plugin registries, or configuration schemas that cross module boundaries.

    Return: a bullet list of entry points and contracts with coupling direction
    (who depends on whom).
  """
)

Agent(
  name = "survey-coupling",
  description = "Survey coupling hotspots and implicit contracts",
  subagent_type = "Explore",
  model = "sonnet",
  prompt = """
    Survey the project for coupling hotspots and implicit contracts — areas
    that cause wrong outcomes for a developer who modifies them without
    knowing the contract.

    Look for: shared mutable state, ordering dependencies, sync points,
    extension registries, global config reads, event buses, or any code
    that must be called in a specific order.

    For each hotspot: note the modules involved, the contract, and the
    failure mode if violated.

    Return: a bullet list of hotspots with modules, contract, and failure mode.
  """
)
```

Wait for all three subagents to return.

### 3. Synthesize domain candidates

Combine the three survey returns:

1. Cross-reference module boundaries, entry points, and coupling hotspots.
2. Produce a candidate domain list — one domain per coherent operational area.
3. For each candidate: note the source paths, the coupling it owns, and any existing mental-model file that covers it.

### 4. User domain confirmation

Present the candidate domains to the user in a numbered list. Tell the user they may reorder, merge, split, rename, or drop entries before proceeding.

Wait for user response. Apply any adjustments. Do not proceed until the user explicitly confirms the final list.

### 5. Lock the task list

Call `TaskCreate` once per confirmed domain, in confirmed order:

```
TaskCreate(
  name = "forge-mental-model-<domain>",
  description = """
    Mental-model authoring for domain: <domain>
    Source paths: <inferred module paths for this domain>
    Spec available: <yes | no>
  """
)
```

Proceed immediately to **On: per-domain** with the first domain.

## On: per-domain

For each domain task in order, skipping tasks with status `completed`:

### 1. Mark in-progress

Call `TaskUpdate` to set the domain task status to `in_progress`.

### 2. Domain survey

Dispatch one Explore subagent:

```
Agent(
  name = "domain-survey-<domain>",
  description = "Survey internals of domain: <domain>",
  subagent_type = "Explore",
  model = "sonnet",
  prompt = """
    Analyze domain: <domain>
    Source paths: <paths from task description>

    Analyze this domain for a developer who needs to modify it.
    Focus on what would cause wrong outcomes if unknown:
    1. Implicit contracts between modules (ordering, data flow, sync)
    2. Coupling (changes here → must also change there)
    3. Extension points (registries, enums, plugin interfaces, config)
    4. Fragile areas (invariants that break silently or cause wrong results, known debt)
    5. Common mistakes (forgetting required steps, wrong outcomes)
    6. Distinguish existing patterns from scaffolded/planned features.

    Be concrete: cite file paths, function names, specific types.
    Do NOT produce type/field listings or paraphrase what functions do.
  """
)
```

Wait for the subagent to return.

### 3. Draft domain file

1. Run `ws-print-infra mental-model-conventions.md` (Bash). Read the output; apply the inclusion test to every claim before writing it.
2. Draft the domain file content for `ai-docs/mental-model/<domain>.md` following the document format in `mental-model-conventions.md`.
3. Set frontmatter: `domain` (filename stem), `description` (one-line scope summary), `sources` (directory patterns from task description), `related` (other domains with coupling to this one).

### 4. Embed spec stems (conditional)

If spec is available (recorded in cold-start step 1):

1. Run `ws-list-spec-stems` (no args) to get all spec stems in the repo.
2. For each section in the domain draft: identify spec stems whose behavior corresponds to the section's topic. Embed the stem inline in the relevant body text (e.g., `{#260421-feature-name}`).

Skip if no spec exists.

### 5. Verify

Dispatch one verifier subagent:

```
Agent(
  name = "verifier-<domain>",
  description = "Verify mental-model draft for domain: <domain>",
  subagent_type = "general-purpose",
  model = "sonnet",
  prompt = """
    Verify the following mental-model domain document against the codebase.

    Domain file draft:
    <full draft content>

    Source paths to check: <paths from task description>

    For each claim in the draft, assign a severity:
    - [HIGH] Factually wrong — misnames a function, inverts a dependency,
      states a constraint that is not enforced.
    - [LOW] Incomplete — a relevant contract or coupling is missing.
    - [STALE] References removed code or an old API.
    - [BLOAT] Fails the inclusion test — type/field listing, paraphrase of
      what a function does, or content derivable without cost.

    Return a finding list. Each finding: severity tag, location in draft,
    correction or suggested removal.
  """
)
```

Process verifier output:
- **[HIGH]**: Apply corrections to the draft directly.
- **[LOW]**: Add to draft if clearly relevant; otherwise collect for user summary.
- **[STALE]**: Rewrite or remove the section.
- **[BLOAT]**: Remove — content fails inclusion test.

### 6. Write file

Write the verified draft to `ai-docs/mental-model/<domain>.md`. Commit with `(mental-model-updated)` in the message body.

### 7. Complete domain

1. Call `TaskUpdate` to set the domain task status to `completed`.
2. If more domain tasks remain, continue with the next incomplete task.
3. When all domain tasks are `completed`, proceed to **On: wrap-up**.

## On: wrap-up

### 1. Update mental-model index

Update `ai-docs/mental-model/index.md`:
- Add a row to the domains table for each newly created domain file.
- Update shared conventions if new cross-domain patterns emerged.

Commit with `(mental-model-updated)` in the message body.

### 2. Summary report

```
## Forge Mental Model — Complete

Domains covered: <N>
Domain files created: <list of paths>
Spec stems embedded: <count, or 'none (no spec found)'>
Verifier corrections applied: <count>
Items for user review: <LOW findings list, or 'none'>
```

### 3. Suggested next steps

- Run `/forge-spec` if spec was absent — mental-model was built without stem cross-references.
- Run `mental-model-updater` agent after future code changes to keep domain files current.

## Judgments

### judge: spec-gate (soft)

Check `ai-docs/spec/` on cold-start. If absent or empty: warn and proceed. Do not block.

## Doctrine

Forge-mental-model optimizes for **confirmed operational knowledge per domain** —
every domain file reflects a completed survey-and-verify cycle before being written.
Spec stems are embedded opportunistically when available; their absence does not
block authoring. When a rule is ambiguous, apply whichever interpretation ensures
the domain survey and verifier steps complete before any file write begins.
