---
title: native Explore dispatch should bypass the generic explore render brief
related:
  260605-research-ws-native-subagent-pivot: parent direction for total spawn removal and harness-native exploration
  260609-refactor-ws-skill-text-playbook-conversion: introduced the current explore render-playbook delegation wording
  260612-bug-subagent-rules-stale-subquery-guidance: removed stale ws/subquery guidance but left the render-brief pattern intact
spec:
  - 260610-subquery-explore-delegation-shift
plans:
  phase-1: 2026-06/15-260615-refactor-native-explore-dispatch-skill-guidance.brief
related-mental-model:
  - workflow-skills
  - prompt-bundle
  - mcp-runtime
---

# native Explore dispatch should bypass the generic explore render brief

## Background

The subquery-to-Explore migration currently routes scoped fact-finding through a
generic `explore` render playbook:

```text
lead skill -> ws/playbook.render(name: "explore") -> rendered worker brief -> native subagent + question
```

That shape made sense while replacing the retired `ws/subquery` tool, but it
duplicates responsibility once the harness already offers a host-native
exploration worker. The rendered `explore` body is itself the prompt handed
to the worker; adding a Codex-specific `explore.codex.md` would only specialize
that worker prompt. It would not change the higher-level skill instruction that
decides whether to render `explore` first.

The desired direction is to move the dispatch decision up into the lead
workflow guidance:

```text
lead skill -> host-native exploration worker directly -> scoped question or purpose-specific query block
```

This keeps native exploration native, removes the redundant generic brief from
the common path, and leaves purpose-specific query blocks responsible for their
own task framing.

## Decisions

- **Top-level skill guidance owns Explore dispatch.** Shared lead playbooks
  should instruct the caller to spawn a host-native exploration worker
  directly, not to render `explore` first.
- **Do not add an empty Claude overlay.** An empty `explore.claude.md` would be
  selected over the base playbook and would produce an empty or incomplete
  worker prompt. Leave the Claude overlay absent until a Claude-side task can
  author the real Claude-native wording.
- **Codex work starts from the visible native subagent surface.** Because this
  ticket is authored from Codex, Codex-facing wording can name the native
  exploration-worker shape at the shared guidance level. Claude-specific
  details are deferred.
- **Purpose-specific query blocks stay purpose-specific.** Forge/spec/model
  workflows that already provide survey or verifier prompt blocks should pass
  those blocks directly to the host-native exploration worker. They should not
  rely on the generic `explore` prompt to supply the task semantics.
- **The generic `explore` playbook is not deleted in this slice.** Keep it as a
  compatibility/fallback artifact until a later cleanup can reconcile specs,
  tests, wsflow behavior, and any remaining runtime assumptions.

## Spec Impact

Target spec area: `ai-docs/spec/workflow-skills.md`

Expected caller-visible change: shipped workflow skill text no longer presents
`ws/playbook.render(name: "explore")` or `ws/playbook.print(name: "explore")` as
the normal scoped exploration path. The normal path becomes direct dispatch to a
host-native exploration worker with the scoped question or existing
purpose-specific query block. The old `explore` render playbook remains present
only as fallback/compatibility until explicitly retired.

Contract-first spec: no. The intended behavior is recoverable from this ticket,
and exact wording should be refined while editing the affected skill guidance.

## Phases

### Phase 1: Move shipped skill guidance to direct host-native exploration dispatch

Update the shared workflow guidance so host-native exploration workers are
called directly from the lead skill context.

Required scope:

- Update `lead-workflow-manual` so the Scoped Exploration primitive no longer
  tells callers to render `explore` before spawning a native subagent.
- Update generic call sites (`lead-discuss`, `lead-sprint`,
  `lead-write-ticket`, `lead-skill-authoring`, `lead-verify-discussion`,
  `lead-salvage`, and `subagent-rules`) to describe direct host-native
  exploration-worker dispatch with read-only, cited, gap-reporting output.
- Update purpose-specific call sites (`lead-forge-spec`,
  `lead-forge-mental-model`, and `lead-write-spec`) so their existing query
  blocks are passed directly as host-native exploration-worker task prompts.
- Update wsflow rsrc mirror if the touched files are mirrored there.
- Keep `explore.md` present; do not add an empty `explore.claude.md`; do not
  delete `explore` tests in this phase unless they are explicitly reframed as
  fallback/compatibility tests.

Verification boundary:

- No shipped skill guidance uses `playbook.render(name: "explore")` or
  `playbook.print(name: "explore")` as the normal scoped exploration path.
- Search confirms remaining `explore` playbook references are either
  compatibility/fallback notes, tests, or explicit follow-up documentation.
- `agents-plugin/rsrc/manifest.json` is regenerated, and the
  `agents-plugin-wsflow/rsrc/` mirror is updated when canonical rsrc changes.
- Relevant `agents-plugin-tool` rsrc/playbook tests pass; wsflow package tests
  are run or any pre-existing failure is explicitly identified.
- `workflow-skills` spec and related mental models are reconciled so they no
  longer claim the `explore` render brief is the canonical scoped exploration
  path.

### Result (da25b381) - 2026-06-15

Phase 1 moved shipped scoped-exploration guidance to direct host-native
exploration-worker dispatch. `lead-workflow-manual`, generic call sites,
purpose-specific survey/check call sites, and `subagent-rules` now instruct
callers to pass scoped English prompts or existing query blocks directly to
host-native exploration workers and require cited evidence, gaps, and follow-up
needs. The canonical rsrc manifest and byte-identical wsflow rsrc mirror were
regenerated. `workflow-skills` spec and the `workflow-skills` / `prompt-bundle`
mental models now describe `explore` as fallback/compatibility rather than the
normal scoped fact-finding path. The `explore` playbook and golden rendering
tests remain in place for Phase 2.

### Phase 2: Decide the long-term status of the generic explore playbook

After Phase 1 lands and callers no longer rely on the generic `explore` brief,
decide whether `explore.md` remains as a fallback/compatibility playbook,
becomes a host-neutral documentation helper, or is retired.

This phase must check:

- current runtime tests that assert `explore` render behavior;
- wsflow compatibility and `prompt.render` expectations;
- specs and mental models that mention `explore` as an rsrc delegate playbook;
- whether purpose-specific survey playbooks should replace any remaining
  generic exploration use.
