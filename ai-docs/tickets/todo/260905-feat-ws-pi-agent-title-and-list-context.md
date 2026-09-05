---
title: "Pi adapter: spawn-time agent title and context-bearing ws-agent-list"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260905-feat-ws-pi-push-only-child-reports: sibling — its status line now carries only a running count; this ticket gives the lead the way to see which children those are
  260905-feat-ws-pi-live-agent-widget: consumer — the always-visible widget should show the title, not the uuid
  260903-feat-ws-pi-subagent-rpc-ux: origin of `ws-agent-spawn` / `ws-agent-list` and the RPC agent registry this ticket extends
related-mental-model:
  - plugin-runtime
spec:
  - pi-adapter-runtime
sage-review-design: recommended
---

# Pi adapter: spawn-time agent title and context-bearing ws-agent-list

## Background

Owner request (2026-09-05, after the second live run of the push-only report
channel): agents are identified only by uuid. The fan-in status line on every
pushed message used to list running ids, which the owner judged noise and which
duplicated `ws-agent-list`; the id suffix was dropped in the sibling ticket's
Edition. That leaves two gaps:

- **Nothing human-readable names a child.** `ws-agent-spawn` takes
  `system_prompt_path`, `prompt`, `model_name`, `model_effort`; the owner sees
  `[ws-agent-report] agent 5e134551-…` and the lead reads the same. The
  `260905-feat-ws-pi-live-agent-widget` panel would render uuids too.
- **`ws-agent-list` cannot restore context.** It returns
  `{agent_id, status, last_report_at}` only. A lead that lost its transcript
  (compaction, `/reload` re-registration of dormant children, `/resume`) has
  no way to recall what each child was asked to do without
  `ws-agent-transcript` on each one.

## Decisions

- **`title` is an optional `ws-agent-spawn` parameter, persisted on the
  record and in the shutdown sidecar.** One short line, model-authored. The
  adapter never derives one from the prompt; an untitled agent stays untitled.
  Rationale: a spawn-time field is the only place the lead has the intent in
  hand, and the sidecar makes it survive `/reload`.
- **`ws-agent-list` grows `title` and an opt-in `include_prompt`.** Default
  rows add `title` (absent when untitled). `include_prompt: true` adds the
  initial `prompt` text so a recovering lead can re-read what each child was
  asked; prompts are not returned by default because they are long and the
  common call is a liveness check. The initial prompt is persisted alongside
  `title` so the option works for dormant/revived entries too.
- **Titles show wherever a child is named to a human.** The pushed-message
  head becomes `[ws-agent-report] agent <title> (<id>)` when a title exists,
  and the TUI renderer, the orphan roll-call, and the future widget use the
  same form. The uuid stays present because it is what `ws-agent-send` /
  `ws-agent-stop` take.
- **Rejected: `include_prompt` on the pushed messages.** Pushes are already
  the model's main context load; recovery is a pull, not a push.
- **Rejected: making `title` required.** Would break every existing playbook
  and skill prompt that spawns without one; the guide recommends it instead.

## Constraints

- No ws-mcp changes: `agents-plugin-tool/` and `agents-plugin/skills/` stay
  untouched. The adapter alone owns the field.
- Sidecar compatibility: entries without `title`/`prompt` (written by the
  current build) must still revive.
- The status line contract from `260905-feat-ws-pi-push-only-child-reports`
  (`N delegated agents still running`, omitted when nothing is delegated) is
  unchanged.

## Spec Impact

`pi-adapter-runtime`: `ws-agent-spawn` parameter list, `ws-agent-list` row
shape and `include_prompt`, pushed-message head form, and the sidecar entry
shape gain the title/prompt fields.

## Phases

### Phase 1: Title at spawn, title and prompt in list

- `ws-agent-spawn`: add optional `title`; store it and the initial `prompt` on
  `RpcAgentRecord`; write both to `PersistedOrphan`; `reviveOrphans` restores
  them.
- `ws-agent-list`: add `title` to each row; add optional `include_prompt`
  parameter returning `prompt` per row.
- Naming surfaces: pushed-message head (`buildPushContent`), the TUI renderer
  in `src/push-render.ts`, and `buildOrphanSummary` use `<title> (<id>)` when
  a title exists.
- `pi-lead-guide.md`: recommend a title on every spawn and describe
  `include_prompt` as the recovery path.
- Tests: spawn with/without title; list row shapes with and without
  `include_prompt`; sidecar round-trip for old and new entry shapes; head
  rendering with and without a title.
- Verification: `cd agents-plugin-pi && npm test`; one live run with two
  titled workers checking the push head, `ws-agent-list`, and a `/reload`
  roll-call.
