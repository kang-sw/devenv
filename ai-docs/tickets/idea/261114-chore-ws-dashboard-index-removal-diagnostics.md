---
title: "Record time and reason when the dashboard session index removes a session"
related:
  261114-bug-ws-shared-daemon-restarts-without-a-swap: "separate incident investigation; index churn did not prove a daemon restart"
---

# Record time and reason when the dashboard session index removes a session

## Background

MAIN requested this short idea capture on 2026-11-14 after the live dashboard
showed `js.banner.gone` while daemon PID 94122 remained unchanged. Retained
`dashboard.log` removal/readdition pairs at physical lines 206–207, 228–229,
233–234, 241–242, 253–254, 256–257 and 259–260 contain neither occurrence times
nor failure reasons. They cannot identify which session read failed, correlate
it with a writer, or distinguish a transient unreadable record from a genuine
registration/root/role/parent mismatch.

Current source boundaries are `internal/wsdashboard/index.go` session removal
and `internal/mcp/dashboard_sessions.go` `lookupRecord`/`ReadSession`. The
latter reduces missing/unreadable parts and identity mismatches to false;
`internal/mcp/session_layout.go` `loadRecordWith` retains more specific read
errors before that reduction. The latest bounded registration/key snapshot
was valid, so no historical failing file or writer has been established.

## Decisions

- Capture diagnostic improvement only: include a timestamp and removal reason
  when the dashboard session index removes a session. MAIN explicitly requested
  this idea; no runtime fix, daemon operation or ready promotion is authorized.
- Compatibility target T0: internal diagnostic output only. Do not change
  session-validity, heartbeat, registration, stream or removal semantics.

## Constraints

- Do not log session contents, credentials, tokens, mailbox bodies or private
  reasoning. Diagnostic context must identify the failure without exposing them.
- Keep missing registration, missing project, unreadable record part and identity
  mismatch distinguishable where the existing source can establish the reason.
  Do not invent a reason when evidence is unavailable.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before future tooling edits.
- This capture performs no source change, build, test, process signal or daemon
  restart. Implementation details remain for later fact population/review.

## Phases

### Phase 1: Make session-index removal logs actionable

Add occurrence time and an evidence-backed reason to removal diagnostics,
retaining enough safe context to correlate a failed session read with the
responsible record part or identity predicate. Preserve existing index and
stream behavior. Use isolated tests for representative missing, malformed,
mismatched and transient-read cases; confirm diagnostics omit secrets and
that unrelated session updates do not become noisy removal logs.
