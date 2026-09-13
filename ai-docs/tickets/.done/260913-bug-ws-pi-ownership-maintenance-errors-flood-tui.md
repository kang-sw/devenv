---
title: "Pi ownership maintenance errors flood the terminal UI"
related:
  260913-bug-ws-pi-ownership-observer-lock-contention-spams-tui: prior narrow fix for verified live-holder EEXIST contention
  260908-feat-ws-pi-agent-session-disk-retention: owns fail-closed agent-home protection and retention behavior
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1532dde6eb1d895a
sage-review-completeness-reviewed: 1532dde6eb1d895a
completed: 2026-09-13
---

# Pi ownership maintenance errors flood the terminal UI

## Background

Owner-live use reproduced repeated raw ownership-maintenance errors in the Pi terminal UI across multiple downstream projects after the narrow live-holder contention fix landed. One path repeatedly emitted `could not observe owned session write` with an ownership-lock `EEXIST` for multiple agents. Another emitted dozens of identical `could not update owned agent home` lines because ownership metadata was missing or unreadable.

The immediate caller-visible defect is the output transport: extension maintenance paths write raw failures through stdout, stderr, or `console.*`, contaminating Pi's terminal rendering stream and making the UI unusable. Lock cleanup and missing-metadata causes still require diagnosis, but root-cause repair must not be a prerequisite for containing repeated output.

## Decisions

- No ownership observer or maintenance path may write directly to stdout, stderr, or `console.*` in normal mode.
- Observer-only failures are best-effort and silent in normal mode. They retry only through their existing event lifecycle; this ticket does not add polling.
- Authoritative ownership mutation, protection, retention, and deletion remain fail-closed. Suppressing raw output must not convert their failures into success or weaken safety checks.
- When an authoritative failure needs owner visibility, surface at most one concise Pi-native notification or status per stable fingerprint during one adapter session. Do not include repeated raw stack traces or filesystem paths in the normal UI.
- Centralize ownership diagnostics behind one reporter so individual observer, agent-home update, retention, and cleanup call sites cannot regress to direct stream writes.
- Detailed repeated diagnostics may use an existing opt-in bounded file/debug sink if one is already available. If none exists, omit detailed normal-mode logging rather than adding a new public debug configuration in this hotfix. The sink must not write to stdout or stderr.
- Treat lock/metadata root-cause repair as separate diagnostic scope. This phase may apply an obvious narrow lifecycle correction found on contact, but presentation containment does not depend on proving one shared cause for both reproductions.

## Constraints

- Pi-extension local only; do not change shared ws-mcp or shared playbooks.
- Preserve the existing observer-only versus authoritative-operation classification.
- Do not add timers, polling, recursive discovery, or a new unbounded in-memory error ledger.
- Fingerprint retention must be bounded by the adapter/session lifecycle.
- Preserve the unrelated zero-byte untracked plan and exclude it from commits.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-storage.ts, agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/index.ts |
| scope.surface | internal | no exported tool or plugin interface change is named |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | no type or signature is named |
| scope.test_surface | existing | agents-plugin-pi/test/ownership-contention.test.ts and agents-plugin-pi/test/agent-storage.test.ts |
| complexity.reuse_points | confirmed | existing ctx.ui.notify and per-session dedup in agents-plugin-pi/src/bridge.ts |
| complexity.side_effect_risk | high | diagnostic routing shares ownership maintenance paths with retention and deletion |
| risk.correctness | high | authoritative failures must remain fail-closed while observer failures are silent |
| risk.fit | high | notifications must be session-bounded and omit raw filesystem details |
| risk.test | high | repeated observer and authoritative failures need stream and notification assertions |
| risk.security_or_contract | high | ownership metadata and locks protect retention and deletion boundaries |

## Phases

### Phase 1: Contain ownership diagnostics outside the terminal stream

Trace every ownership observer and agent-home maintenance error emission, classify it as observer-only or authoritative, and route it through one bounded reporter. Remove direct stdout/stderr/console writes from normal operation. Silence observer-only failures; coalesce authoritative owner notification by stable fingerprint while leaving the underlying operation fail-closed.

Add regressions for repeated live-holder `EEXIST`, stale or unverifiable ownership locks, missing/unreadable ownership metadata, and ordinary non-ownership I/O failures. Assert repeated events do not call stdout, stderr, or console; observer failures produce no normal notification; authoritative failures remain rejected and produce at most one concise Pi-native signal. Verify reporter state is session-bounded, no polling is introduced, existing retention/deletion safety tests remain green, and the full Pi suite passes.

### Result (36dd34f) - 2026-09-13

Ownership observation, metadata update, removal, and retention failures now route through one closed-fingerprint reporter over the existing session-scoped `ownerNotifyRef` lifecycle. Observer-only failures are silent in normal mode, while authoritative operations preserve their rejection or safe-retention outcomes and emit at most one concise TUI-lead warning per failure class. Capacity-eviction errors no longer expose agent identities, raw filesystem details, or stack text.

Regression coverage exercises live and unverifiable ownership locks, missing and unreadable metadata, ordinary observer I/O errors, removal and retention failures, and repeated authoritative contention. It behaviorally intercepts stdout, stderr, and common `console.*` methods, verifies session-bound dedup reset, and preserves retention/deletion safety. No timers, polling, public debug configuration, or root-cause lock/metadata repair was added.

Verification: `npm --prefix agents-plugin-pi test` passed 1,812 tests with 2 skipped. Focused storage, contention, retention, and spawner coverage passed 330 tests. Round-one correctness review was clean; its Fit and Test Important findings were fixed in `36dd34f`, and both round-two fix verifications were clean with no unresolved observations.


## Resolution (2026-09-13)

Contained ownership maintenance diagnostics within the Pi-native session notification lifecycle. Observer failures are silent, authoritative failures remain fail-closed with bounded concise owner warnings, raw capacity-eviction details are redacted, and full Pi verification plus two review rounds passed.
