---
title: Deep exploration researcher lacks its blocking collection tool in live Pi
related:
  260907-feat-ws-pi-persistent-explore-deep-research: prerequisite; implementation present, live acceptance still unfinished
spec:
  - 260903-pi-explore-recon-leaf
sage-review-design: skipped
sage-review-completeness: skipped
---

# Deep exploration researcher lacks its blocking collection tool in live Pi

## Background

Live dogfood after the owner fully exited Pi and reopened the same session reproduced a contract failure: `explore({ query, deep_research: true })` created a researcher that reported only `read`, `grep`, `find`, `ls` and the parallel wrapper, with no blocking `explore`. It could inspect files directly but could not delegate collection to small. The owner requested a ready bug ticket and explicitly skipped sage review. Merge was subsequently postponed; ticket capture only is authorized now.

Observed deep researcher: `18d88dd1-48ee-4f76-9603-717e63199dd0` / `explore-2`. Its answer explicitly reported that blocking exploration was unavailable; no collection leaf ran. The simple researcher `07f8574e-a616-4df4-b1bc-49c2c53b9cbe` / `explore-1` returned an initial answer and a follow-up on the same identity with the intended read-only surface. These are live observations, not model/effort verification: neither researcher could directly observe its model or thinking level.

The implementation and correction commits are `048a9cb2` and `6998a5b0`; the feature checkpoint is `d796a30`. Automated tests and a generic dynamic allowlist probe passed, but did not prevent this actual deep-child registration failure.

## Decisions

- Restore the already-approved contract: default researchers have no delegation tool; deep researchers have read-only inspection plus blocking `explore`; their small collection leaves have read-only inspection without bash or recursive delegation.
- Preserve the existing public boolean-only interface and persistent lifecycle. Do not expose arbitrary tool-list overrides, widen researchers to full workers, or change ordinary worker/execute-worker recon permissions as a workaround.
- Keep spawn-time model/effort inheritance and fail-closed small selection unchanged.
- Root cause is unconfirmed. Source inspection found the expected deep group, mode environment propagation and role/mode registration gate in `agents-plugin-pi/src/spawner.ts` and `src/process-role.ts`; that does not prove which source, argv or environment the failing live child used.
- A prior investigator inferred a stale parent from the durable session creation timestamp. That inference was withdrawn: the owner confirms full process exit and reopening the same session. A durable session timestamp is not process uptime. Do not prescribe repeated restarts without actual process/loaded-source evidence.
- Adapter-only correction; no ws-mcp or shared rsrc semantic changes on this track. If diagnosis requires a new public contract, stop for owner approval instead of inventing one.

## Spec Impact

Restore `260903-pi-explore-recon-leaf` in `ai-docs/spec/pi-adapter-runtime.md`, including mode-specific tool availability and bounded collection. Update affected documentation only if investigation establishes an inaccurate implemented description; do not weaken the spec to accept missing deep delegation.

## Phases

### Phase 1: Restore actual deep-child blocking collection and prove the launch path

Reproduce with a newly created deep researcher after a full Pi process restart while resuming the same session. Establish actual parent/child executable and extension source paths, process start times, launch allowlist, deep role/mode environment and registration timing. Inspect only relevant environment keys; do not dump credentials or whole environments. Distinguish a source/runtime registration bug from a loaded-copy mismatch with concrete evidence. A parked child may require a fresh controlled probe rather than inferring its lost launch environment from source.

Correct the demonstrated failure at its owning adapter/launch boundary, retaining all approved permission, lifecycle and model-selection constraints. If the issue is deployment/source resolution rather than implementation, document and verify the concrete correction rather than making speculative code changes.

Verification:

- Exercise the actual extension launch/registration path used by deep researchers, not merely a generic custom-tool allowlist or static group string assertion. A regression fixture must fail for the reproduced cause and pass after the correction.
- Fresh simple researcher: only read/grep/find/ls; no bash or explore. Fresh deep researcher: those reads plus functional query-only blocking explore.
- Invoke deep blocking collection against a small read-only repository question. Observe the collection's actual tool surface, small model and effort; no bash or recursive explore. Confirm its result reaches the researcher for synthesis.
- Repeat after stop/send and full process exit/reopen of the same session; retain identity, mode, permissions and frozen model/effort. Verify ordinary worker recon leaves remain unchanged.
- Run the adapter suite with clean and inherited role/mode environments. Record full output and actual live evidence separately. Never claim a passed provider/restart gate from mocked tests or child self-report of unobservable model state.
- Record findings back on the prerequisite feature ticket. Its mandatory live acceptance remains incomplete until supported by evidence; this bug's capture does not mark it done.
