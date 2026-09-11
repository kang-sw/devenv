---
title: Degrade safely when a Pi task fork cannot load a parent extension tool
related:
  260907-bug-ws-pi-fork-first-call-prompt-cache-miss: established serialized definition and order identity; this ticket intentionally adds a narrow unavailable-stub exception to its no-synthetic-registration rule
  260911-bug-ws-pi-question-queue-dogfood: live 79-versus-78 readiness reproducer that blocks fork-raised question acceptance
spec:
  - 260905-pi-side-thread-fork-task-thread
sage-review-completeness: completed
sage-review-design: completed
sage-review-design-reviewed: 22d8149774f40e6e
sage-review-completeness-reviewed: 22d8149774f40e6e
completed: 2026-09-11
---

# Degrade safely when a Pi task fork cannot load a parent extension tool

## Background

During fork-cache documentation closeout on 2026-09-08, a lateral `ws-fork` child reported only edit/write/parallel tools, with no read, shell or `ws-report-to-lead`. It could neither inspect documentation nor emit the required completion report. Repeated requests to report produced another incomplete-run advisory rather than completion. It reported no repository changes.

A replacement ordinary spawned delegate had read/shell/edit/write but reported no ws tools, preventing the explicitly requested ws commit. Its implementer prompt also required plan-local authorization before reading the ticket. Work resumed only after the lead supplied that authorization and retained ws verification/commit responsibility.

These are observed child reports, not a verified root-cause diagnosis. The installed adapter may differ from reviewed working-tree code; do not infer a regression in that code from this incident alone. Distinguish missing callable tools from prompt-level restrictions and from missing tool exposure in inherited context.

A current-worktree live probe on 2026-09-11 narrowed the task-fork failure: the parent advertised 79 callable tools while the child, launched with ambient extensions disabled and only the ws Pi adapter loaded, registered 78. Readiness rejected the fork before its first task. The missing tool is likely a parent-extension tool such as `codex_generate_image`; the current count-only error does not prove its identity.

## Decisions

- Preserve the parent fork's byte-identical system instructions, ordered tool definitions, and inherited conversation prefix. Do not filter a missing tool out of the captured surface and do not append capability notices to the system prompt.
- Before readiness comparison, register an unavailable stub for every expected task-fork tool that is absent from the child and is not completion-critical. Reuse the parent's captured name, description, and parameter schema exactly so the provider-visible ordered tool array remains cache-prefix eligible. This metadata-identical unavailable handler is an explicitly accepted narrow exception to the existing rule that every missing actual child registration fails and synthetic schema replay is forbidden.
- An unavailable stub always fails deterministically and names the tool as unavailable in this fork because its providing parent extension was not loaded. It must never pretend that the original tool executed.
- Append the deterministic missing-tool list to the first task-fork user message, after the existing fork task framing. This notice is new suffix content and must not rewrite the inherited system/tool/conversation prefix.
- Missing `ws-report-to-lead` remains fatal because a task fork cannot satisfy its completion protocol without it. Keep the completion-critical set explicit and add another tool only when the fork protocol actually requires it.
- Unexpected extra tools, changed schemas or descriptions, and registration-order drift remain fatal. The degraded path handles absence only; it does not weaken structural equality for registrations that exist.
- Preserve the existing caveat that prefix equality establishes only client-side cache eligibility, not provider retention, routing, billing, or a cache-hit guarantee.

## Phases

### Phase 1: Preserve fork prefix while degrading absent extension tools

Capture the exact missing names before readiness, install identical-definition unavailable stubs for non-critical absences, and carry the unavailable list in the parent's captured tool order into the existing first task-fork user-message builder. Keep the parent system prompt and inherited prefix untouched. Make the readiness diagnostic name missing, extra, reordered, and changed registrations rather than reporting only counts. Amend `260905-pi-side-thread-fork-task-thread` to replace its blanket no-synthetic-registration rule with this narrow unavailable-stub exception.

Verify a parent-only extension tool reproducer end to end: the task fork passes readiness with the same ordered provider-visible definitions, receives the unavailable-tools notice only in its first user task, gets the deterministic failure when it invokes the stub, and can still finish through `ws-report-to-lead`. Verify that no-mismatch forks produce no notice or stub, that `ws-report-to-lead` absence still rejects before the first model turn, and that extra/order/schema/description drift still rejects. Re-run the fork prefix/cache serialization and fork lifecycle suites. Complete the blocked fork-raised question live acceptance after the fix is loaded.

### Result (0c95be75) - 2026-09-11

Task forks now classify registration drift, install metadata-identical deterministic failure stubs only for missing non-critical tools, keep `ws-report-to-lead` absence fatal, and append the captured-order unavailable list only to the first task input. Readiness diagnostics retain fatal handling for extra, reordered, schema-changed, and description-changed registrations. Review added executable coverage proving the actual child-bootstrap completion-channel rejection occurs before any provider prompt and that a degraded fork can report and settle through `ws-report-to-lead`.

Focused fork suites passed 274/274 and the full adapter suite passed 1625/1625 after both implementation commits (`c96dcd08`, `0c95be75`). The only implementation deviation was a portable Homebrew/Linuxbrew Pi SDK path fallback required for the existing native-transition test fixture on macOS. Correctness, fit, and test reviews had no Critical findings; two test Important findings were fixed, and the duplicate spec Important findings were resolved by updating `{#260905-pi-side-thread-fork-task-thread}` in `cdf179ee`.

After plugin reload, a live fork inherited the parent-only `codex_generate_image` slot through the unavailable stub, received its notice in the initial task, raised owner thread `q4`, resumed after the owner selected `alpha`, and delivered a final report whose `Decisions:` preserved both the choice and notice observation. No unresolved implementation or acceptance blocker remains.


## Resolution (2026-09-11)

Implemented cache-prefix-preserving unavailable stubs for absent non-critical task-fork tools, retained fatal completion-channel and structural drift checks, updated the Pi task-fork spec, and passed the live fork-raised question round trip after reload.
