---
title: playbook.render serves pre-edit prompt text, so subagent runs are not reproducible against the tree
related:
  260726-refactor-retire-spec-planned-marker-mechanism: the review that exposed this; its reviewer had to be pointed at the repo file by hand
  260625-bug-wsflow-rsrc-mirror-regen-missed-after-shipped-edit: adjacent staleness class — that one is a missed regen, this one is a render path that ignores the tree
  260611-bug-rsrc-manifest-regen-missed-after-shipped-edit: same class
sage-review-design: required
---

# playbook.render serves pre-edit prompt text, so subagent runs are not reproducible against the tree

## Background

Found while dogfooding a sage review. Commit `2d1a731c` edited
`agents-plugin/rsrc/ticket-reviewer-design/ticket-reviewer-design.md`, adding a
`## Spec Impact` read, a `ready/` conflict scan step, and a **Spec territory
conflict** checklist item. Its commit message asserted the change was live in-tree
immediately, reasoning that `wsrsrc` "reads plain-text playbook files from a
filesystem root (NOT go:embed)" (`wsrsrc.go:3`) and therefore needs no version
bump.

**That reasoning is wrong, or at least incomplete.** A `playbook.render` issued
*after* that commit produced a prompt without any of the three additions:

| | bytes | contains `Spec territory conflict` |
|---|---|---|
| `agents-plugin/rsrc/ticket-reviewer-design/ticket-reviewer-design.md` | 4182 | yes |
| render output `.../prompt-paths/21359c70-01-ticket-reviewer-design.md` | 3912 | **no** |

The rendered file's Constraints still read "any spec files in `spec:`
frontmatter", its Process was the old 7 steps with no `ready/` scan, and its
Checklist had 4 items.

The reviewer subagent only applied the new instruction because the spawning
prompt pointed it at the in-repo path by hand. Left to the render, it would have
run the old lens and the run would have looked entirely normal.

Not a write-order race: the cache file's mtime was *newer* than the repo file's.
The render resolved its source somewhere other than the working tree — the
reviewer's reading of `wsagent/agent.go:284` suggests an installed-plugin path,
which needs confirming since no `.codex/plugins/cache/` directory exists in this
checkout.

## Why this matters beyond one stale prompt

- **Silent.** No error, no warning, no version mismatch surfaced. The render
  returns a valid path and a plausible prompt.
- **Unreproducible reviews.** Every delegated review, reviewer, or implementer
  prompt may be running text that does not match the tree under review. A sage
  verdict then describes a procedure nobody can find in git.
- **It falsifies a shipped commit's stated reasoning.** `2d1a731c`'s AI Context
  tells a future reader that rsrc edits are live without a bump. Anyone trusting
  that will ship prompt changes that never take effect.

## Decisions

- **Diagnose before choosing a fix.** The root cause is not established: it could
  be source-root resolution, a cache keyed on something that did not change, or a
  deliberate installed-plugin precedence. The fix differs completely per cause, so
  Phase 1 is diagnosis and must not pre-commit to a remedy.
- **Silence is the defect, whatever the cause.** Even if serving installed-plugin
  text turns out to be intended precedence, doing it without saying so is not.
  Any outcome must leave the caller able to tell which source a render came from.

## Constraints

- Do not "fix" this by making callers hand-point at repo paths, as the session
  that found it did. That is the workaround that hid the bug.
- Do not assume the working tree should always win. In a plugin-installed
  deployment, preferring the installed tree may be correct; this ticket is about
  the divergence being invisible, not about which side should be preferred.

## Spec Impact

- Target spec area: `ai-docs/spec/plugin-runtime.md` (playbook source resolution)
  and `ai-docs/spec/mcp-tools.md` (`playbook.render` / `playbook.print` output
  contract).
- Expected caller-visible change: a render reports which source root it resolved
  from, so a stale or shadowed playbook is detectable at call time rather than by
  byte-diffing the output.
- Contract-first spec: no. The remedy depends on Phase 1's diagnosis.

## Phases

### Phase 1: Diagnose the source-resolution path

- Reproduce: edit a playbook in the working tree, call `playbook.render` on it,
  and diff the output against the source. Confirm the divergence is deterministic
  rather than a one-off cache artifact.
- Trace how the rsrc root is resolved for a render — `wsrsrc`'s filesystem root,
  the value the MCP server passes, and any installed-plugin precedence in
  `wsagent`. Establish which source actually won and why.
- Record whether the same divergence affects `playbook.print`, which is what the
  shipped skills call, or only `playbook.render`. This determines blast radius:
  `print` affects every skill invocation, `render` only delegate spawns.
- State the root cause in `### Result` and stop. The remedy is chosen after.

Rejected alternatives: patching a cache-invalidation key before knowing the cause;
forcing the working tree to win unconditionally (may be wrong in an installed
deployment).

Verification boundary: the diagnosis names the resolved source root, the reason it
won over the working tree, and whether `playbook.print` shares the behavior —
reproduced at least twice, with the second run after touching the source file so
mtime-based caching is excluded as the explanation.
