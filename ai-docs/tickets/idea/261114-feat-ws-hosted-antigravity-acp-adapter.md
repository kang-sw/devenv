---
title: "Deferred Antigravity hosted-agent integration: assess the official ACP path and integrate through the common contract"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261114-refactor-ws-hosted-common-contract-pi-claude-parity: "first implementation preserves Pi/Claude; intended foundation, not a newly approved hard dispatch dependency"
---

# Deferred Antigravity hosted-agent integration

## Background

The owner wants Antigravity hosted-agent support but has no Antigravity account. On 2026-11-14, whiteboard (665)=b and its explicit note prioritized Pi/Claude behavior preservation and requested that Antigravity remain a substantial later task. This idea ticket preserves that future outcome and already-collected integration questions; it is not permission to install, authenticate, probe or implement an Antigravity backend now.

## Decisions

1. **Explicitly deferred.** Antigravity is outside the first common-hosted-contract implementation. Record a full follow-up rather than a token reminder. Authority: owner (665)=b and note.
2. **Reuse the common hosted execution boundary.** A future adapter must coexist with Pi/Claude without reducing their supported controls or native integrations. MCP attachment alone does not establish sustained agent-process control.
3. **Keep current authority and lifecycle boundaries.** This ticket does not approve root-key mint restrictions, a new daemon workflow interpreter, existing API removal, a universal runtime replacement, voice, or an IDE-private transport integration.
4. **Compatibility is not yet settled for an unchosen public surface.** The future design must prefer an additive T0 adapter. Any actual protocol/store/config/MCP change is classified and settled before promotion; T2 is not authorized by this deferred idea.

## Existing Investigation Inputs

The prior session's official-documentation investigation identified these public paths. They are starting evidence for later version-specific assessment, not a current installed-binary/runtime equivalence claim:

- ACP CLI: https://antigravity.google/docs/cli/acp/
- ACP extension documentation: https://antigravity.google/docs/ide/extensions/acp/
- Headless CLI: https://antigravity.google/docs/cli/headless/
- Public Python SDK: https://antigravity.google/docs/sdk/overview/
- MCP integration: https://antigravity.google/docs/mcp/

The investigation reported `agy acp` using ACP v1 JSON-RPC over stdio, with documented session loading/resume and control/permission paths subject to capability negotiation. It also distinguished headless NDJSON from an interactive control channel: headless mode or MCP configuration is not evidence of all send/steer/cancel/resume/approval operations. The SDK and desktop/IDE app have distinct version and control surfaces. Rolling documentation and a download-page version do not prove that an installed version supports those contracts.

## Future Contract Coverage

Before an executable adapter ticket is promoted, recover a version-specific, evidence-backed answer for each item:

- Distribution/CLI version and stable public entry point; authentication requirements and where credentials remain; whether one process owns one session.
- Session create/load/resume/list/close, active-turn steer and cancel, and process EOF/exit versus turn completion or cancellation.
- Permission request pause/answer/rejection and correlation to tool call/result; no silent approval substitution.
- Ordered content/tool/usage/lifecycle/error events and mapping into existing runner events, including unsupported capabilities and unknown protocol messages.
- Hosted ws MCP attachment, key adoption, physical host identity and parent lineage, one owner for input/mail, holder/replay/recovery and reconnect behavior.
- Model/effort configuration and unsupported control behavior; credential-less fixtures versus later account-backed acceptance.
- Pi/Claude regression expectations and separation between protocol fixtures, actual CLI smoke and live delivery.

These are coverage questions, not a declaration that ACP satisfies every requirement or that the SDK alternative has been rejected.

## Constraints

- Initial state is `idea/`; no ready promotion, implementation, install, account creation, login or provider call is part of the current approval.
- Future capture must consult the exact official library/CLI version and retain cited protocol fixtures. Existing research is reused but not treated as account-backed acceptance.
- Keep machine-local credentials and account details out of tracked tickets/manuals.
- Matching implementation manuals later include `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md`; MCP, authored-surface and mirroring obligations apply only if those paths are actually touched.

## Phases

### Phase 1: Settle the exact-version integration contract when the owner resumes this task

After the owner resumes Antigravity work and required account/access is available, assess the documented public ACP path against the common hosted contract, distinguish it from headless/SDK/IDE alternatives and return missing product/API decisions to the lead. Retain a finite capability matrix and fixture plan, then settle the actual adapter scope before ready promotion.

### Phase 2: Implement and verify the selected adapter under the settled contract

This phase remains deferred and depends on the settled Phase 1 result. Implement the chosen public control path through the existing hosted runner boundary, with capability/error/permission mapping, identity/input ownership and holder recovery. Verify protocol fixtures, Pi/Claude regression behavior and separately authorized account-backed acceptance. Record source integration and live delivery independently. No exact API, transport choice or runtime grant is inferred from this placeholder phase.
