---
kind: print
delegates: true
---
# Backfill Docs

Target: user request

## Invariants

Scope
- Reconcile only the groups discovery returned; never widen a group to cover commits it judged already documented.
- Reconcile documentation only; source edits belong to an implementation flow.

Execution
- Resolve the audit floor once before the first dispatch; never re-resolve it after a group commits.
- Process groups oldest-first, one at a time.
- Run `lead-update-spec` inline; never delegate spec authoring.
- Dispatch `mental-model-updater` once over the whole audit window, after every group's spec pass; never once per group.
- Commit one `docs(spec): ...` per group; do not batch groups into one commit.
- Use `{{.McpNamespace}}/git.*` for range discovery, log audit, and diff inspection.
- All written content is English regardless of conversation language.

Reporting
- Report every group's outcome including `none`, so an unreconciled group stays visible.
- Report residual coverage flags that no pass resolved; do not drop them.

## On: invoke

### 1. Resolve the audit window

1. If `user request` contains a `..` range, use it as the window and continue to step 2 of the next section.
2. Find the newest commit whose body contains `(mental-model-updated)`.
3. Find the newest commit touching `ai-docs/spec/`.
4. The audit base is the older of the two; the window is `<base>..HEAD`.
5. When neither exists, apply **judge: absent-floor**.

Both markers are high-water marks, so a marker-derived window finds only drift
newer than the last documentation pass. Report that bound with the result, and
tell the caller that gaps below it need an explicit `..` range.

### 2. Discover groups

1. Render `{{.McpNamespace}}/playbook.render(name: "doc-gap-discovery")`; capture prompt path and `recommended-tier` as dispatch metadata.
2. Spawn one fresh subagent with **Discovery dispatch**; choose the worker tier from dispatch metadata.
3. On `No groups in window.`, report `Backfill: no gaps.` and stop.

### 3. Reconcile spec, group by group

Oldest first, one group at a time:

1. Print and execute `{{.McpNamespace}}/playbook.read(name: "lead-update-spec")` against `<group-range>`.
2. Record the resulting spec commit hash, or `none` when it reported no changes.

### 4. Sweep mental model once

1. Spawn one `mental-model-updater` with **Mental-model dispatch** over `<base>..HEAD`, which now contains every spec commit step 3 produced.
2. Collect its `## Spec Coverage Gaps` block.

### 5. Reconcile residual flags

1. Collect every `## Spec Coverage Gaps` entry naming a stem no group's spec pass touched.
2. Run one further inline `lead-update-spec` pass over the union of those groups' ranges.
3. Report what remains unresolved after that pass.

### 6. Report

Emit **Completion report**.

## Judgments

### judge: absent-floor

No `(mental-model-updated)` commit and no `ai-docs/spec/` commit exist. The
honest window is the whole history, which is rarely what the caller wants.

| Observation | Window |
|-------------|--------|
| HEAD differs from `main` | `{{.McpNamespace}}/git.merge_base(base: "main", head: "HEAD")`, then `<merge-base>..HEAD` |
| HEAD is on `main` | Ask the user for a starting point before spawning anything |

## Templates

### Discovery dispatch

```text
Rendered doc-gap-discovery prompt: <prompt-path>

Read that prompt file and execute it. Report groups for this window only.

Audit window: <base>..HEAD
```

### Mental-model dispatch

```text
Rendered mental-model-updater prompt: <prompt-path>

Read that prompt file and execute it.

Commit range: <base>..HEAD
Output path: <path>
```

### Completion report

```text
Backfill: <N groups reconciled> | no gaps
Window: <base>..HEAD (<marker-derived|caller-supplied>)
- <group-range>: spec <hash|none>
Mental model: <hash|none>

Unresolved:
- <group-range or stem>: <reason>
```

Omit `Unresolved` when nothing remains.

## Doctrine

Backfill optimizes for **judgment placed once**: discovery is delegated because
it is wide and mechanical, spec authoring stays with the lead because it is
narrow and contested, and groups are sized so one group is one decision. Spec
entries are per-behavior and so are grouped; mental-model domains are corpus-wide
and so are swept once. When ambiguous, reconcile fewer commits correctly rather
than more approximately.
