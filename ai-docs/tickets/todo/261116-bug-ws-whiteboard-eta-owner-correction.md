---
title: "Whiteboard implementing topics cannot shorten ETA or correct owner in place"
related:
  261114-feat-ws-whiteboard-topic-state-and-lead-nudges: "preserve landed run identity, elapsed-time and cumulative-ETA semantics until owner decisions change them"
---

# Whiteboard implementing topics cannot shorten ETA or correct owner in place

## Background

Dogfood finding #13, 2026-11-16, minor UX friction. On an implementing topic
whose total ETA is already 60 minutes, `wb:eta` refuses any total that does not
extend beyond 60. Repeating `wb:state value="implementing"` with a changed owner
or ETA also refuses unless the agent explicitly exits and re-enters the state.
An ordinary estimate/assignment correction therefore requires a synthetic
lifecycle transition. This is existing deliberate behavior, not proof that the
extension-only implementation violates its original contract.

Source evidence:

- `agents-plugin-tool/internal/wswhiteboard/topic_eta.go:49-50` requires a total
  strictly greater than the existing run total; `:62-64` records an `extend`.
- `agents-plugin-tool/internal/wswhiteboard/topic_state.go:317-318` rejects an
  implementing owner/ETA change and directs callers to ETA extension or exit/re-enter.
- `topic_eta.go:17` and `topic_state.go:163-166` validate monotonic extension
  history. A new correction event cannot be assumed safe for old readers.

## Reproduction

Create an isolated topic, enter implementing with an owner and `eta_minutes="60"`,
then submit `wb:eta minutes="30"` (with a reason), or repeat implementing with
another owner/ETA. Observe refusals. Compare a valid extension to 61 and an
explicit exit/re-enter, retaining run IDs and started-at values.

## Candidate fix directions

- Add a deliberate correction/reassignment operation distinct from extension.
- Permit a documented in-place correction if run history and elapsed-time
  authority can be preserved compatibly.
- If extension-only behavior remains policy, make the limitation and safe
  correction workflow clear without implying that it already supports shortening.

## Decisions

Compatibility tier: **unsettled; target T0** only if old readers/writers preserve
run history and still operate normally. The existing writer emits `extend` and
the readers validate monotonic totals at the cited lines. New shorter totals or
unknown correction events may fail that check; specify retained raw history and
new-version reopen behavior before claiming T2. A required old-writer/reader
boundary is T3 and needs explicit owner approval and a minor bump. This capture
approves neither a new event nor a lifecycle-policy change.

## Open questions

- Should shortening correct the same run or start a new run, and should its
  initial ETA, elapsed time and extension count be retained?
- Is owner correction the same operation as executor replacement?
- If corrections are supported, what reason/history and old-version behavior
  are required?

## Phases

### Phase 1: Settle and implement a compatibility-safe estimate/owner correction path

Ground the current state/event readers and writers, settle the correction
questions, then implement the approved path or clarify the existing workflow.
Future fixtures cover shortening, no-op, extension, owner correction, real
executor replacement, history replay, old writes and new reopen. Do not reset
elapsed time or rewrite historical events incidentally. No whiteboard mutation
or test is performed during this capture.
