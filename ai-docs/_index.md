<!-- Memory policy: prune aggressively as project advances. Completed
     work belongs in git history, not here. Keep only what an AI session
     needs to orient itself and pick up work. If it's derivable from
     code or git log, delete it from this file. -->

# devenv — Project Index

## What This Repo Is

Configuration and template repository for Claude Code workflows.
Meta-workflow project only — defines skills, agents, and workflow patterns for downstream projects. Sessions here work on the workflow system itself; domain specs, mental-models, and domain tickets belong to downstream projects. The skill system itself has a spec at `ai-docs/spec/skills.md`.

**Plugin:** `ws@0.3.0` — see `claude/.claude-plugin/plugin.json`.

**Plugin topology:**
- Skills and agents are delivered via the `ws` Claude Code plugin, sourced from `claude/` via a `directory`-type marketplace entry in `~/.claude/settings.json`.
- After any change to `claude/`, run `claude plugin update ws@ws` to propagate to the plugin cache. `./install.sh update` handles first-time install and settings patching on a new machine.
- `claude/CLAUDE.md` is symlinked to `~/.claude/CLAUDE.md` — edits to the global thinking doctrine land in this repo; `git diff` surfaces them here.
- External install: `/plugin marketplace add kang-sw/devenv` → `/plugin install ws@ws`.

## Reference Documents

Read before authoring or modifying skills, agents, or infra:

| Document | Purpose |
|----------|---------|
| `ai-docs/ref/skill-authoring.md` | Skill & agent document layout, invariant/constraint checklist, doctrine format |
| `claude/infra/impl-playbook.md` | Implementation discipline: test strategy, verify, deviation protocol. Access via `load-infra impl-playbook.md`. |
| `claude/infra/subagent-rules.md` | Subagent dispatch rules: exploration, branches, general rules. Access via `load-infra subagent-rules.md`. |

## Native Agents

Agent definitions live in `claude/agents/`. Each file defines what an
agent IS and HOW it works (identity, constraints, process, output).
Team communication rules are injected by the calling skill at spawn time.

```
claude/agents/
  code-reviewer.md        — code diff review: correctness, security, contracts (read-only)
  document-reviewer.md    — fresh-eye design/ticket review against mental-model and spec (read-only)
  worker.md               — general-purpose non-code tasks
  clerk.md                — ticket management
  mental-model-updater.md — mental-model doc updates after code changes
  spec-updater.md         — strip 🚧 markers when spec-stems appear in merged commits
```

## Infra Layout

```
claude/infra/                 — docs only; accessed via load-infra
  impl-playbook.md            — subagent-safe implementation discipline
  mental-model-conventions.md — mental-model doc format and invariants
  ticket-conventions.md       — ticket format, status directories, stem convention; optional spec: field
  spec-conventions.md         — spec doc format, 🚧 marker rules, {#slug} anchor protocol
  subagent-rules.md           — exploration, branches, general rules
  implementer.md              — code implementer role; spawn as general-purpose + read first
  code-review-correctness.md  — Correctness review partition: logic, error paths, contracts, security
  code-review-fit.md          — Fit review partition: conventions, naming, reuse, patterns
  code-review-test.md         — Test review partition: assertion validity, coverage, mock integrity

claude/bin/                   — PATH-accessible executables (added by plugin)
  subquery                    — scoped sub-query via headless claude subprocess
  spec-build-index            — rebuild features: frontmatter in spec docs; removes stale stems: blocks
  list-stems                  — list {#YYMMDD-slug} anchors from spec files; file-arg adds heading context
  merge-branch                — branch merge with strategy selection (squash or --no-ff)
  list-mental-model           — enumerate mental-model docs relevant to target paths
  load-infra                  — cat any infra doc by name (agent Bash tool context)
```

## Skill Inventory

```
claude/skills/
  enter-session/       — session bootstrap; clerk-forked context synthesis + workflow map injection
  exit-session/        — end-of-session handoff; seal continuation payload for next /enter-session
  discuss/             — explore approach/direction, capture as tickets
  write-ticket/        — create/edit tickets in ai-docs/tickets/
  write-spec/          — create/update external-perspective spec docs in ai-docs/spec/
  write-skeleton/      — public interface stubs + integration tests
  write-plan/          — deep codebase research → implementation plan
  edit/                — main-agent-direct single-scope cycle (warm sessions, owner edits)
  implement/           — delegated implementer + reviewer cycle (cold sessions or wide scope)
  parallel-implement/  — multiple implementer pairs, disjoint file sets on shared branch
  proceed/             — auto-route through the canonical pipeline
  ship/                — release: version bump, tag, build, publish per project config
  team-lead/           — team orchestration mode (TeamCreate, coordination, shutdown)
  manual-think/        — manual chain-of-thought when native thinking unavailable
  write-mental-model/  — mental-model document format, inclusion test, rebuild
  bootstrap/           — scaffold new project or upgrade existing to canonical template
  forge-spec/          — from-scratch spec reconstruction; archive-first, domain-by-domain, cross-compact via TaskCreate
```

## Canonical Flows

```
Full ceremony:  /discuss → /write-spec → /write-ticket → /proceed
                                                             ↓
                           /write-skeleton? → /write-plan? → /edit (warm)
                                                           → /implement (cold)
                                                           → /parallel-implement (disjoint)
Direct:         /edit <description>
Auto-route:     /proceed <ticket-path>    — routes via warmth + scope judges
```

Agent suggests next step at each point; user decides. `/proceed` is the explicit opt-in for auto-chaining through the pipeline.

## Tickets

Status directories: `idea/` → `todo/` → `wip/` → `done/` (or `dropped/`).
Reference by stem only (e.g., `260407-research-delegation-model-consolidation`).

| Stem | Status | Summary |
|------|--------|---------|
| `260419-chore-blueprint-plugin-extraction` | done | Package claude/ as a Claude Code plugin (now named "ws"); all phases complete and validated |
| `260420-feat-spec-driven-workflow` | wip | Spec-driven workflow infrastructure (phases 1-5 done; phase 6 pending) |
| `260421-feat-global-spec-stems` | wip | Global unique YYMMDD-slug stems (phases 1-4 done; phase 5 migration pending) |
| `260421-feat-forge-spec` | done | /forge-spec skill — from-scratch spec reconstruction; all 3 phases complete |
| `260421-feat-delegate-implement-feature-branch` | done | /implement feature-branch auto-merge mode; all phases complete |
| `260422-chore-write-ticket-workflow-drift` | done | Fix stale /write-spec suggestion in write-ticket + workflow-skills.md chain drift |
| `260422-chore-workflow-chain-drift` | done | Fix remaining chain drift in discuss/SKILL.md, write-spec/SKILL.md, write-skeleton/SKILL.md |
| `260422-feat-write-ticket-review` | done | Add mandatory document-reviewer step to write-ticket after intent review |
| `260422-chore-rename-implement-to-edit` | done | Rename /implement skill to /edit; phase 1 of two-phase skill rename |
| `260422-chore-rename-delegate-implement-to-implement` | done | Rename /delegate-implement skill to /implement; phase 2 of skill rename |

## Session Notes

<!-- Cross-session intent only, 2-5 lines max, delete when stale. -->
