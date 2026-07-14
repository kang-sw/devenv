---
title: "lead-drain-ready-queue goal-staging branch slug collides across concurrent goal runs"
completed: 2026-07-13
---

# lead-drain-ready-queue goal-staging branch slug collides across concurrent goal runs

## Background

`agents-plugin/skills/lead-drain-ready-queue/SKILL.md` instructs the lead to,
when an active `/goal` context is detected, "derive a short branch-safe slug
from the goal text and create and check out the staging branch directly —
`git checkout -b goal/<slug>`". For this skill the goal text is typically just
the bare invocation `/lead-drain-ready-queue`, with no per-run distinguishing
content, so the derived slug is deterministic and identical across every
concurrent or sequential run of this skill: `goal/drain-ready-queue`.

Dogfood surprise (2026-07-13): two independent worktrees of the same repo
(`/home/swkang/devenv` and `.worktree/ws-dashboard-dev`) each ran
`/lead-drain-ready-queue` as their own goal around the same time. Git branches
are shared across worktrees of the same repository, so the second run's
`git checkout -b goal/drain-ready-queue` collided head-on with the first run's
already-existing, actively-in-progress branch of the same name (which by then
had dozens of `merge(dashboard): ... into goal/drain-ready-queue` commits).
The second lead session's attempt to create the branch failed loudly (`fatal:
a branch named 'goal/drain-ready-queue' already exists`), which is the lucky
non-destructive outcome — a slightly different sequencing (e.g. the second
session using `-B` instead of `-b`, or merging into what it assumed was its
own fresh staging branch) could have silently corrupted the first session's
in-progress goal-integration branch with unrelated changes.

## Phases

### Phase 1: Make the goal-staging branch slug unique per goal run

Adjust `lead-drain-ready-queue`'s branch-derivation guidance so the resulting
branch name (`goal/<slug>`) cannot collide across independent goal runs, even
when the goal text itself is identical (e.g. two runs both invoked as bare
`/lead-drain-ready-queue` with no distinguishing argument). Candidate
directions to evaluate: incorporate the lead's session key, a worktree-scoped
discriminator, or another per-run-unique token into the slug; or detect an
existing `goal/<slug>` branch before creating one and disambiguate
automatically (e.g. `goal/<slug>-2`) rather than failing or silently reusing
it. Read `agents-plugin/skills/lead-skill-authoring/SKILL.md` before editing
this skill text, and check `ai-docs/ref/wsflow-mirroring.md` for whether
`agents-plugin-wsflow/skills/lead-drain-ready-queue/SKILL.md` needs a mirrored
update.

### Result (8e694e4a) - 2026-07-13

Hotfixed per direct user request: `lead-drain-ready-queue` now generates an
arbitrary random word-word-word slug (e.g. `canny-hello-stride`) for the
goal-staging branch instead of deriving it from goal text, in both
`agents-plugin/skills/lead-drain-ready-queue/SKILL.md` and the curated
`agents-plugin-wsflow` mirror. This removes the deterministic-collision
property; it does not add auto-disambiguation (detecting and renaming around
an existing `goal/<slug>` branch) or a worktree-scoped naming scheme — those
remain open candidate directions if a random slug ever collides in practice
(astronomically unlikely at this token space, so not pursued further now).

## Spec Impact

None yet — this is workflow-skill text, not an MCP tool contract; no
`ai-docs/spec/` entry currently documents goal-staging branch naming.

## Related

- `agents-plugin/skills/lead-drain-ready-queue/SKILL.md` — skill text with the
  collision-prone slug derivation.

