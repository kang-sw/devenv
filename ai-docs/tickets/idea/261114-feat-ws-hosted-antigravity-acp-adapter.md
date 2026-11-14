---
title: "Deferred Antigravity hosted-agent integration: assess the official ACP path and integrate through the common contract"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261114-refactor-ws-hosted-common-contract-pi-claude-parity: "first implementation preserves Pi/Claude; intended foundation, not a newly approved hard dispatch dependency"
  261114-refactor-ws-common-session-interface: "landed Phase 2 is the extension implementation prerequisite; existing owner-resume/account gates remain"
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

## Documentation-Only Capability and Gap Checklist

MAIN's 2026-11-14 assignment permits organizing the existing ACP research
against the current hosted interface. MAIN described (799)=a as assumed while
the owner was asleep; this does not lift owner (665)=b's deferral, establish an
account or authorize an executable/credential-less probe. No external ACP
contract or installed binary was reverified for this checklist. The target
version/build/platform remains **unselected and unverified**.

Use the official links above as the starting evidence, not an assertion that
all ACP implementations expose the same capabilities:

| Contract area | Existing research / boundary | Gap to resolve only when separately authorized |
|---|---|---|
| Public distribution and framing | Prior research reports `agy acp`, ACP v1 JSON-RPC over stdio; headless NDJSON and SDK/IDE paths are distinct. | Exact CLI/SDK build, public entry point, framing/initialization and capability negotiation; no IDE-private transport equivalence. |
| Process/session ownership | The common substrate is `Backend.Start` / `Process` / Runner, with a holder owning the process. | Session create/load/resume/close and one-process ownership; attach/reconnect without duplicate session/process launch. |
| Input and turn controls | Headless output or MCP availability alone does not prove bidirectional session control. | Send, cancel and any active-turn steer; distinguish queued next-turn input, interruption, completion and EOF/exit. |
| Permissions | Existing research mentions negotiated permission requests and answers. | Exact options, pause/reject semantics and call/result correlation; prove safe mapping or return richer-choice policy to the owner, never silently approve. |
| Events and errors | A public interactive stream is the candidate, not an implemented mapper. | Ordered content/tool/usage/lifecycle/error events, unknown messages, replay/gaps and waiting/busy/recovery evidence. |
| Holder and recovery | Current shared hosted contract retains ordered raw output, reattach and single delivery ownership. | Required transport fit, session-load/replay boundaries and no premature queued mail/input release; do not rewrite holder protocol by assumption. |
| ws identity and context | MCP attachment is required integration plumbing, not process control. | Daemon key adoption, physical host identity, lineage, common context/state access and one input/mail owner; no new root authority. |
| Authentication and config | Owner had no Antigravity account when deferring this work. | Exact auth/config-home/refresh behavior and authorized account/access; no install, account creation, login or credential handling now. |
| Models and optional controls | Existing Pi/Claude controls and backend metadata are not universal agy support. | Supported model/effort choices, explicit unavailable/unsupported operations and safe capability exposure. |
| Subagents and idle safety | No agy native-subagent producer/count/tree contract is established. | Stable IDs/parents/status and actual running counts or explicit unknown/unsupported telemetry; missing evidence is not known zero. |
| Evidence and compatibility | Recorded public-document research exists, but no exact-version fixtures or account-backed acceptance are established here. | Finite version-pinned fixture plan, separately authorized execution/account acceptance and actual T0/T1/T2 classification. |

MAIN's supporting `shared/docs/harness-interface-readiness.md`, sections 4–6,
inspected source at `951af006a`; it distinguishes useful existing ACP research
from the missing exact-version executable contract. This local report is not a
downstream dependency or approval of all suggested APIs. Documentation-only
checklist completion does not complete Phase 1 or make this ticket ready.

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

- Initial state is `idea/`; no ready promotion, implementation, install, account creation, login or provider call is part of the current approval. The added documentation checklist does not authorize local binary/daemon/protocol probes or new executable fixture collection, including credential-less probes.
- Future capture must consult the exact official library/CLI version and retain cited protocol fixtures. Existing research is reused but not treated as account-backed acceptance.
- Keep machine-local credentials and account details out of tracked tickets/manuals.
- Matching implementation manuals later include `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md`; MCP, authored-surface and mirroring obligations apply only if those paths are actually touched.

## Phases

### Phase 1: Settle the exact-version integration contract when the owner resumes this task

After the owner resumes Antigravity work and required account/access is available, assess the documented public ACP path against the common hosted contract, distinguish it from headless/SDK/IDE alternatives and return missing product/API decisions to the lead. Retain a finite capability matrix and fixture plan, then settle the actual adapter scope before ready promotion.

### Phase 2: Implement and verify the selected adapter under the settled contract

This phase remains deferred and depends on the settled Phase 1 result, landed `261114-refactor-ws-common-session-interface#phase2` under the existing foundation extension map, and separately authorized implementation/account access. Documentation preparation does not relax any of those gates. Implement the chosen public control path through the existing hosted runner boundary, with capability/error/permission mapping, identity/input ownership and holder recovery. Verify protocol fixtures, Pi/Claude regression behavior and separately authorized account-backed acceptance. Record source integration and live delivery independently. No exact API, transport choice or runtime grant is inferred from this placeholder phase.
