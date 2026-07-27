---
title: the ws tree has no guard on SKILL.md shim shape, so a dropped repair pointer
  fails silently there while wsflow fails loudly
related:
  260726-chore-mcp-repair-pointer-mid-procedure-skills: its Phase 1 completed the
    pointer sweep and surfaced this asymmetry as a residual minor
  260624-epic-pre-release-cleanup: item 8 is the repair-pointer coverage line this
    belongs under
---

# The ws tree has no SKILL.md shim-shape guard

## Topic

`260726-chore-mcp-repair-pointer-mid-procedure-skills` Phase 1 brought the
`mcp-server-repair` pointer to every SKILL.md in both trees that calls
`playbook.print`. In the wsflow tree that state is enforced:
`agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` asserts each shim's tail
shape and was mutation-verified during that phase — reverting a shim to the
un-pointed tail, deleting the tail, dropping the pointer line, or using the wrong
namespace all fail the suite.

The ws tree has no equivalent. `agents-plugin/skills/manifest.json` carries a
sha256 per SKILL.md, so an edit is *visible*, but nothing asserts what the file
must contain. A future edit can drop the pointer and the only signal is a manifest
hash change that regen silently absorbs.

## Why it matters

The asymmetry is the problem, not the absence. The two trees now hold the same
invariant, and exactly one of them enforces it — so the tree with the weaker guard
is the one that will drift, and the drift will be invisible until someone hits a
transport failure mid-procedure and gets a dead end again. That is the precise
failure the sweep ticket existed to remove.

It is also pre-existing rather than introduced: the gap dates to `adbf5ec3`, which
established the pointer wording and added it to four front-door skills without a
guard. The sweep only made it matter more, because there are now twenty-plus files
holding the invariant instead of four.

## Direction

The wsflow test is Python because wsflow ships a Python package test harness; the
ws tree has none, so a literal port is not available. Plausible hosts, in rough
order of fit:

- `agents-plugin-tool/internal/wsrsrc` — already owns skills-manifest generation
  and already reads every SKILL.md, so a shim-shape assertion costs one more pass
  over data it has loaded. This is probably the answer.
- A shared assertion driven off a single declared table of shim shapes that both
  trees consult, so the two guards cannot disagree about what the invariant is.
  More work, but it removes the class rather than the instance.

Open question worth settling before building: what exactly is the invariant? "Ends
with the tree-namespaced repair pointer" is checkable but narrow. The wsflow test
also pins the `# Title` and the single-`playbook.print`-call shape, which is closer
to "this is a thin shim" — the property actually worth defending. Porting only the
pointer check would leave the ws tree guarded against one regression and blind to
the rest.

Also in scope to decide: the three inline-body skills (`lead-goal-step`,
`lead-prefer-subagent`, `lead-verify-discussion`) are legitimately pointer-free in
both trees because they make no `playbook.print` call. Any guard needs that
exemption to be derived from the file's content rather than from a hand-maintained
name list, or the list becomes the next thing that drifts.

## Prior art

- `adbf5ec3` — established the pointer wording; the un-guarded origin.
- `260726-chore-mcp-repair-pointer-mid-procedure-skills` — the sweep; its Phase 1
  `### Result` records this as deferred with the reasoning.
