---
title: "이름 붙은 커스텀 티어: 하네스|모델|effort 묶음"
parent: 261114-epic-ws-common-session-foundation
---

# Named custom tiers: harness | model | effort bundles

## Scope & Owner Decision

- **Idea (owner 2026-11-19 17:25-17:29):** Support user-defined named tiers
  that bundle `harness | model | effort` (for example a `"discussion"` tier,
  or Claude Opus through Antigravity such as `claude-opus-4-6-thinking`, which
  `agy`'s model list offers alongside Gemini models, see
  `agents-plugin-tool/internal/wsagent/testdata/agy-1.5.9-subagent-baseline.jsonl:4`
  and `261118-feat-ws-hosted-model-list-and-agy-bypass`). Selectable by the
  same name from both the dashboard new-session dialog (`POST /agents/leads`,
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard-leads.js:249`) and
  `agent.spawn` `tier` (`agents-plugin-tool/internal/mcp/agent_tools.go:344`).
- **Owner decision — no authority in tiers:** `authority`/auth is NOT part of
  a tier; it stays on `agent.spawn`'s required `authority` argument and the
  new-session permission choice, so tier and `agent.spawn` roles do not blur.

## Background / Related

- **Current state (arbitrary custom tiers not possible today):**
  - `agents.tier` (`agents-plugin-tool/internal/wsconfig/config.go:640-667`,
    `agents-plugin-tool/internal/mcp/config_registry.go:514-548`) maps four
    fixed tier names (`small`, `medium`, `large`, `xlarge`; synonyms
    `light`/`core`/`deep`/`haiku`/`sonnet`/`opus` collapse via `NormalizedTier`
    at `config.go:892-916`) to `{backend, model, effort}` per harness bucket
    (`default`, `codex`, `claude`, `pi`, `agy`).
  - `agent.tier_aliases` (`config.go:541-560`,
    `agents-plugin-tool/internal/wsagent/agent_tools.go:320-369`) lets users
    define custom names, but `ParseTierAliases` forces every target to
    `<harness>.<tier>` where `<tier>` must normalize to `small..xlarge`. It
    cannot define a new `(harness, model, effort)` tuple independent of those
    four slots.
- **Related tickets:**
  - `261117-feat-ws-agent-tools-mailbox-routing` Decision 17 (`(53)=a`):
    rejected `agent.spawn` `preset?` (`{tier, authority, system_prompt_path}`)
    so spawn stays playbook-unaware with explicit arguments. Custom tiers
    bundle only `harness | model | effort` (no `authority`, no prompt).
  - `261115-feat-ws-session-hire-approval` decision `(952)` (`no tier`):
    rejected a tier word on `session.request_hire` in favor of optional
    `harness`, `model`, `effort` falling back to new-session popover defaults.
  - `261116-feat-orchestration-profile-foundation` (`track/playbook`): adds
    `workflow.profile` (`direct` / `orchestrate`) for playbook/include
    selection; check overlap with role/mode-like tier names such as
    `"discussion"`.

## Open Questions

1. **Definition site & config key shape:** Where are named tiers defined (e.g.,
   generalizing `agent.tier_aliases` vs. a dedicated config key vs. extending
   `agents.model_aliases`), and at which scopes (`global` / `project`)?
2. **Built-in tiers:** Do `small..xlarge` become instances of the same
   mechanism, or stay distinct per-harness fallback slots?
3. **New-session dialog UX:** How does the dashboard new-session dialog list
   and apply named tiers alongside harness/model/effort controls (and should
   `session.request_hire` stay out or follow)?
4. **Compatibility tier:** Expected **T0** for an additive config key and
   optional UI/spawn selector; confirm against
   `ai-docs/manuals/release-compatibility.md` (note
   `agents-plugin-tool/internal/wsconfig/agy_tiers_test.go:82-110` for
   unknown-field preservation in `agents.model_aliases`).

## Phases

Rough outline pending owner discussion (do not populate or promote to `ready/`
until the open questions above are settled):

### Phase 1: Config schema, `agent.tiers`, and `agent.spawn` resolution

TBD after owner discussion.

### Phase 2: Dashboard new-session dialog integration

TBD after owner discussion.
