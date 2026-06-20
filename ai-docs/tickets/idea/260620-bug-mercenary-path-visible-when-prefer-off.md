---
title: mercenary path is too visible when prefer_mercenary is off
related:
  260605-research-ws-native-subagent-pivot: establishes native subagents as the default and mercenary as opt-in or prefer_mercenary-selected
  260611-refactor-ws-tier-taxonomy-delegate-tier-routing: records native default and mercenary registration as an alternate delegation path
  260619-feat-ws-layered-config-scope-substrate: moved prefer_mercenary into session-scoped layered config
related-mental-model:
  - mcp-runtime
  - workflow-skills
---

# mercenary path is too visible when prefer_mercenary is off

## Background

During dashboard no-auth implementation dogfood, `config.show` reported
`prefer_mercenary` as the builtin empty value, which resolves to off. The
rendered `lead-implement` procedure still exposed the full mercenary dispatch
idiom beside the native dispatch idiom. The lead then selected the visible
`ws.mercenary.*` route even though the expected default was the host-native
subagent path.

Native subagent tooling was available in the Codex session through host tool
discovery, so this was not a missing-capability fallback. The failure mode was
guidance salience: the default playbook made the mercenary route too easy to
choose while the session did not ask for mercenary and did not enable
`prefer_mercenary`.

## Observed Evidence

- `config.show(session_key=...)` reported `prefer_mercenary` with `value: ""`
  and `scope: builtin`.
- Source comments define absent, empty, and `"false"` as off for
  `prefer_mercenary`.
- `lead-implement` says "Native is the default" but still prints a concrete
  mercenary `register` + `call` dispatch recipe in the default playbook body.
- The Codex host native subagent tools were discoverable after the mistake via
  `tool_search`, so default routing could have used native subagents.

## Desired Direction

When `prefer_mercenary` is off, default implementer/reviewer playbook rendering
should not expose mercenary as a normal dispatch recipe. The primary rendered
procedure should steer to native subagents only. Mercenary may remain available
through explicit user request, a clearly secondary troubleshooting/runbook
reference, or the existing `prefer_mercenary` toggle, but it should not appear
as an equally actionable default path.

When `prefer_mercenary` is on, the rendered playbooks may surface the
mercenary-primary guidance block and concrete `ws.mercenary.*` dispatch steps.

## Open Questions

- Should the always-on mercenary continuity tip also disappear when
  `prefer_mercenary` is off, or is it enough to hide only the concrete
  `register` + `call` dispatch recipe from the default delegate-dispatch
  section?
- Should this be implemented as conditional render filtering, a separate
  advanced playbook reference, or a prompt override point around the delegation
  dispatch block?

