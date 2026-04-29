---
title: Agent System
summary: Spawnable agent roles — output contracts, refusals, and spawn contexts within ws workflow skills.
---

# Agent System

Agent roles are defined as prompt documents in `claude-plugin/infra/prompts/` (with frontmatter declaring model and tools). All agents are invoked via `ws-oneshot-agent -p <stem>` or `ws-named-agent new -p <stem>` — the native Claude Code Agent tool is not used. All agents are stateless at spawn — they carry no memory of prior invocations. Skills pass all necessary context in the spawn prompt.

## `clerk` {#260421-clerk-agent}

One-shot ticket operations agent. Accepts a spawn prompt listing all pending ticket operations; the caller must supply exact technical values (data formats, field names, enum values, API shapes) — clerk asks rather than infers when values are missing.

**Output:** a structured `## Clerk report` block:
```
## Clerk report
Operations: <N completed, M skipped>

### <ticket-name>
Action: <created | edited | moved | dropped>
Changed: <field or section names>
Path: <ai-docs/tickets/<status>/<stem>.md>
Convention issues: <none | description>

### Open questions
- <anything clerk could not resolve without caller input>
```

**Refusals:**
- No source code reads, mental-model edits, or `CLAUDE.md` touches.
- No ticket stem renames — flags and suggests new-ticket-plus-drop instead.
- No commits — caller handles all git operations.

> [!note] Constraints
> - Caller override to `sonnet` model is expected when the spawn prompt requires synthesis rather than mechanical application.

## Review Agents

### `code-reviewer` {#260421-code-reviewer-agent}

Read-only diff review agent. Produces a two-phase output:

**Phase 1 — Findings report** (one per review round):
```
## Review findings: <scope>

### Critical
- <file>:<line> — <description>

### Important
- <file>:<line> — <description>

### Minor
- <file>:<line> — <description>
```

**Phase 2 — Final report** (emitted only after all Critical and Important issues are resolved):
```
## Review: <scope>
Rounds: <N>
Summary: <1-2 sentences>
Remaining: <none | Minor items list>
```

A clean first pass produces `No issues found.` in Phase 1 and skips directly to Phase 2.

Severity:
- **Critical** — incorrect behavior, security issue, or broken contract. Blocks merge.
- **Important** — non-trivial quality or correctness concern. Blocks final report.
- **Minor** — style, naming, or low-impact suggestion. Does not block.

Re-review is restricted to previously reported issues only — no expansion to unchanged code on subsequent rounds.

**Refusals:**
- Read-only — no code edits, no commits.
- No suggestions outside the diff scope.

### `code-reviewer` partition mode {#260421-code-reviewer-partition-mode}

When spawned with a partition doc via `-p`, the reviewer restricts findings exclusively to that partition's checklist. Three partition docs are available in `infra/`:

| Partition | `-p` stem | Covers |
|---|---|---|
| Correctness | `code-review-correctness` | Logic errors, error paths, contract compliance, security surface, edge cases |
| Fit | `code-review-fit` | Conventions, code reuse, established patterns, test style |
| Test | `code-review-test` | Assertion validity, unreachable paths, mock integrity, coverage, test isolation |

Each partition doc explicitly names what it excludes. When a partition is active, findings outside that partition are not reported.

`/implement` spawns Correctness and Fit reviewers in parallel and consolidates findings before sending to the implementer.

## Maintenance Agents

### `mental-model-updater` {#260421-mental-model-updater-agent}

Post-implementation doc updater. Locates its own base commit by running `git log --grep="mental-model-updated" -1`; falls back to a caller-provided base commit when no checkpoint is found.

Applies surgical edits to `ai-docs/mental-model/`: adds new contracts and coupling discovered since the base commit, fixes stale content, removes content that fails the inclusion test or appears in disallowed sections (Overview, Relevant Source Files). Updates `sources` and `related` frontmatter fields.

**Output:** `## Mental-Model Updates` bullet list, one entry per domain file:
```
## Mental-Model Updates
- ai-docs/mental-model/<domain>.md: <description of change, or "no changes needed">
- ai-docs/mental-model/<new-domain>.md (new): <description>
```

Produces git commits with `(mental-model-updated)` in the commit body.

**Refusals:**
- No expansion beyond domains affected by the commits since the base commit.
- No content that fails the inclusion test (defined in `claude-plugin/infra/mental-model-conventions.md`).

### `spec-updater` {#260421-spec-updater-agent}

Strips `🚧` markers from spec entries whose implementations have landed in commit history. Conservative: defers to caller on ambiguous matches, never strips speculatively.

Full behavioral spec: see [`spec-system.md` → `spec-updater` Agent](spec-system.md#spec-updater-agent).

## Mental-Model Document System {#260424-mental-model-directory-hierarchy}

Structural conventions for `ai-docs/mental-model/` that agents observe when reading and writing mental-model docs.

### Domain Rules Section {#260424-domain-rules-section}

Each mental-model domain doc may carry a `## Domain Rules` section containing user-authored prescriptions for AI agents working in that domain. These rules describe patterns the agent must follow when implementing code in this domain — analogous to `## Architecture Rules` in `CLAUDE.md` but scoped to a specific domain.

Rules in this section are authored via `/add-rule` or manual edit. No agent modifies their content autonomously.

### Ancestor Loading Contract {#260424-executor-ancestor-load}

When a skill loads a sub-domain mental-model doc (`mental-model/<domain>/<sub>.md`), it also loads all ancestor `index.md` files (`mental-model/<domain>/index.md`). Ancestor docs are loaded before the sub-domain doc so that inherited `## Domain Rules` are visible to the agent before work begins.
