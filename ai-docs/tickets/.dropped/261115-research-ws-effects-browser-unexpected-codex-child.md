---
title: Investigate an unexpected Codex child in the isolated effects browser fixture
related:
  261115-feat-ws-whiteboard-committed-effects-in-conversation: pinned candidate verification context
---

# Investigate an unexpected Codex child in the isolated effects browser fixture

## Background

An isolated browser acceptance run unexpectedly observed an installed `codex app-server` process directly beneath its Go test binary. The accepted recipe prohibited provider actions and shared-daemon interaction. Capture the fixture-isolation question before any retry; process existence alone neither proves provider activity nor establishes its absence.

## Outcome Ledger

### Verified Findings

- The clean candidate was `e124d76e932a015470a20e6f6f5339459beca700`. The single command was `go test -p=1 -parallel=1 ./internal/wsdashboard -run '^TestWhiteboardEffectsBrowser$' -count=1 -timeout=4m`, with explicit sanitized environment, fresh ws cache/config roots, offline Go settings, and existing cached Playwright/Chromium.
- The observed ancestry contained Go command PID/PGID 78544, its `wsdashboard.test` child 79744, and a direct child 79785 whose full command was `/Users/kang-sw/.local/bin/codex app-server`, started on 2026-11-14 at 23:05:19 UTC. Chromium used a separate process group 79774. These are recorded historical identities, not current cleanup targets.
- Browser stdout reported a `frame.waitForFunction` timeout at `testdata/whiteboard-effects-browser.cjs:190`; the command failed rather than skipped. Test duration was 13.10 seconds, package duration 13.947 seconds and monotonic wall time 21.076840292 seconds. Only 1280-width artifacts were reported; 360-width acceptance was not reached.
- The parent read the complete stdout, metadata and descendant trace. Evidence is retained externally under `ws-777-json-timing.pEidiy`, including `03-c-browser.metadata.json`, `03-c-browser.observed-descendants.json`, stdout/stderr and browser artifacts. Parent session `useable-curvature-immovably` records the exact local paths.
- The worker stopped further launches and was instructed to complete proof-owned identity comparison and cleanup only. The stage metadata reported no surviving Go test-group rows; final cross-group disposition was still pending at capture time.

### Confirmed Decisions

No retry, source repair, provider invocation or daemon action is authorized by this capture. Preserve evidence and report unknown processes without signaling them.

### Proposals

None selected. Trace the fixture and session/host discovery path statically before choosing an isolation change or attributing the browser failure.

### Open Questions

- Which fixture operation caused the installed Codex app server to start, despite a sanitized environment and isolated stores?
- Did the process merely initialize, or perform any provider/network operation? The ancestry trace does not answer this.
- What deterministic fixture boundary would prevent unintended installed-harness startup while preserving realistic effects rendering?
- Is the browser wait failure related to that startup, fixture behavior, or candidate display logic? No cause is established.

### Rejected Alternatives

No implementation alternative is settled. Process presence is not evidence of a model request, and disappearance is not proof that no external action occurred. Do not suppress the observation or weaken acceptance to label the candidate merge-ready.

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - 테스트 중 codex 자식 프로세스 관찰 메모. 원인 티켓과 중복; 261115-bug-ws-dashboard-fixture-real-home-plan-usage-probe.
