---
title: "표준 데몬 hosted 커넥터로서의 Pi와 그 위의 프로바이더·로그인 레이어"
parent: 261114-epic-ws-common-session-foundation
related:
  261112-research-ws-ade-desktop-shell-and-rename: "(494)=a zero-install and (495)=a built-in agent = bundled Pi standalone binary with our extension in isolated mode; this ticket carries that decision's remaining checks (login flows) into stage 3"
  261118-research-ws-agy-custom-agent-prompt: "native agy lead quality work continues in parallel as first-class support"
---

# Pi as the canonical daemon-hosted connector, with provider and login layers on top

## Background

The harness common interface (epic `261114-epic-ws-common-session-foundation`)
is being built on the native Claude, Codex and Antigravity (agy) harnesses.
Pi can host many API providers through one harness, and the owner noted
community Pi extensions that reuse provider OAuth (for example an
Antigravity one; unverified here). The owner raised a provider hub: bundle Pi with selected
extensions and Codex and Antigravity OAuth, and map tiers such as
`pi.codex.small` and `pi.agy.small`. A login UI is a separate topic.

`261112-research-ws-ade-desktop-shell-and-rename` already settled (494)=a
(zero-install) and (495)=a (the built-in agent is a bundled Pi standalone
binary with our extension in isolated mode, conditional on verifying
distribution, size, license, redistribution and supported login flows).

## Direction

Owner answer to (128), 2026-11-18, sets three stages:

1. Dogfood-verify the harness common interface on Claude, Codex and agy first;
   their native support continues as planned in the epic.
2. Rewrite the Pi ws extension as a daemon-hosted connector whose goal is full
   support of the common interface. Pi becomes the canonical implementation of
   that interface, and it opens API providers such as OpenRouter.
3. Layer OAuth and login (provider tier mapping such as `pi.codex.*` and
   `pi.agy.*`, a login UI) on top of stage 2 later.

Stage 2 starts after stage 1's verification; no single ticket blocks it.

## Research items

- Stage 2: the gap between the current Pi extension (`agents-plugin-pi/`) and
  a daemon-hosted connector that covers the full common interface.
- Stage 3: Pi's OAuth provider support (Codex, Antigravity) and how it reaches
  a bundled, isolated Pi; terms-of-service and account risk of reusing those
  OAuth flows; a quality A/B of the same model on its native harness versus
  Pi; the `pi.<provider>.<tier>` mapping shape.

## Outcome Ledger

### Verified Findings

### Confirmed Decisions

- (128), 2026-11-18: the three-stage direction in `## Direction`. Native
  Claude, Codex and agy support stays first-class; Pi is added later as the
  canonical daemon-hosted connector; OAuth and login sit on top of it.
- Inherited: (494)=a and (495)=a from
  `261112-research-ws-ade-desktop-shell-and-rename`.

### Proposals

### Open Questions

- Stage 2 scope and its compatibility tier against the current Pi extension
  and the `pi.*` MCP tools.
- Stage 3 provider list, ToS posture, and tier mapping.

### Rejected Alternatives
