---
title: "Live spec/mental-model coverage warning at session-bootstrap time"
related:
  260703-chore-bootstrap-staleness-alarm: sibling session-bootstrap warning; this ticket reuses its config-item and warning-delivery-channel pattern for a different signal (doc coverage, not template version)
  260707-feat-forge-autonomy-bootstrap-chaining: complementary same-session forge-spec to forge-mental-model chaining; this ticket is the cross-session safety net for the same forgetting risk
sage-review: completed
---

# Live spec/mental-model coverage warning at session-bootstrap time

## Background

`ai-docs/tickets/ready/260703-chore-bootstrap-staleness-alarm.md` already
established a session-bootstrap warning pattern for a different signal
(`AGENTS.md` template-version staleness): inject at session-bootstrap time
(`ws/ferrule` / `ws/workflow_manual`), not per `ws/project_tree` call
(`260703-...-alarm.md:36-44`); add a dedicated `wsconfig.Item*` on/off entry
following the existing named-item registry shape (`ItemWorkflowPreferSubagent`
/ `ItemSageReview`), exposed through `config.tuning`/`ws:lead-tune`
(`:45-54`); and have the warning text itself carry the silencing instruction
(`:55-59`). That ticket explicitly considered and rejected a standalone
`config.set_flag(name, bool)` tool because it would create "a second,
unstructured config surface parallel to the disciplined named-item registry
every other config point already uses" (`:51-54`).

Separately, `lead-bootstrap` currently only suggests `lead-forge-spec` /
`lead-forge-mental-model` once, on the fresh-install path
(`agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md:46`), with no
persistent cross-session reminder if a user skips or forgets — including
the case where `lead-forge-spec` was run but `lead-forge-mental-model` was
not (see `260707-feat-forge-autonomy-bootstrap-chaining`, which only covers
the same-session chaining case).

Confirmed convention: both existing spec and mental-model files use YAML
frontmatter (`ai-docs/spec/mcp-tools.md:1-4`,
`ai-docs/mental-model/mcp-runtime.md:1-14`), so "at least one frontmatter-
bearing `.md` file present" is a convention-aligned, pragmatic proxy for "this
doc area has real content" that naturally excludes `.gitkeep` or empty
placeholder files.

## Decisions

- **No stored/settable flag.** Coverage is computed live, every time, at
  session-bootstrap time (`ws/ferrule` / `ws/workflow_manual`): check whether
  `ai-docs/spec/` and `ai-docs/mental-model/` each contain at least one `.md`
  file with a YAML frontmatter block. No bootstrap-sets / forge-completes-
  clears flag lifecycle. Rejected because a set/clear flag can desync from
  reality (partial `lead-forge-spec` failure, manual edits to `ai-docs/`,
  running outside the flag-aware path) in ways a live, stateless check
  cannot.
- **No new generic config-setter tool.** A parameterized setter (e.g. a
  `config.fatal_missing_doc(name, bool)`-shaped tool) was considered and
  rejected for the same reason `260703` already rejected
  `config.set_flag(name, bool)`: it duplicates the disciplined named-item
  config registry with a second, unstructured surface.
- **No warning text written into `AGENTS.md`/`CLAUDE.md`.** Persisting the
  warning as literal file text was considered and rejected: it would need an
  explicit remove step once the docs exist, reintroducing the same
  desync risk the live-check decision above avoids. The warning is
  delivered only through the `ferrule`/`workflow_manual` response, matching
  `260703`'s channel.
- **Single combined mute config item**, not one per doc type (spec vs.
  mental-model). A new `wsconfig.Item*` entry (name to be decided at
  implementation time, e.g. `ItemDocCoverageAlarm`), `on`/`off`, builtin
  default `on`, following the exact shape of `260703`'s
  `ItemBootstrapAlarm`, exposed through `config.tuning`/`ws:lead-tune`.
- **Warning text embeds the mute instruction as a tip**, consistent with
  `260703`'s decision for its own warning content — no separate doc lookup
  required to learn how to silence it.
- **Injection point matches `260703`**: session-bootstrap time only
  (`ws/ferrule` / `ws/workflow_manual`), not every `ws/project_tree` call.
- **Delivery-channel reuse**: this warning likely reuses whatever
  warning-delivery-channel plumbing `260703` builds in
  `agents-plugin-tool/internal/mcp/workflow_manual.go` for its own
  template-version warning. Whichever of the two tickets lands first should
  build that shared channel generically enough for the other to reuse rather
  than duplicating it; exact sequencing is a survey question at
  implementation time, not settled here.

## Phases

### Phase 1: Live doc-coverage check and mute config item

- Add a check, run at session-bootstrap time (`ws/ferrule` /
  `ws/workflow_manual`), for whether `ai-docs/spec/` and
  `ai-docs/mental-model/` each contain at least one `.md` file with a YAML
  frontmatter block.
- Add the new combined `wsconfig.Item*` mute entry (builtin default `on`),
  wire it into `config.tuning`/`ws:lead-tune`.
- Surface the warning in the `ferrule`/`workflow_manual` response payload
  when coverage is missing and the mute item is `on`; warning text includes
  the silencing instruction per Decisions.
- Survey and, where practical, reuse the warning-delivery-channel plumbing
  from `260703-chore-bootstrap-staleness-alarm` rather than duplicating it.
- Verification: add a test confirming the warning fires when a doc area has
  no frontmatter-bearing `.md` file, is silent when at least one exists, is
  suppressed when the mute item is `off`, and that `config.tuning`/
  `ws:lead-tune` lists and can set the new item.

## Spec Impact

New caller-visible session-bootstrap behavior (a warning surfaced to the
lead) and a new config item — needs spec addressing before `ready/`
promotion. Likely shares spec area with whatever `260703` addresses for its
own session-bootstrap warning. Not addressed yet; left for the
implementation-survey pass that promotes this ticket.
