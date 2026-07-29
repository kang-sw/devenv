---
title: A vestigial loop in the attention SSE stream keeps cargo clippy permanently red
related:
  260726-bug-dashboard-terminal-notify-silent-failure-no-expiry: surfaced while running clippy as a verification step for that phase
completed: 2026-07-29
---

# A vestigial loop in the attention SSE stream keeps cargo clippy permanently red

## Background

`cargo clippy -p ws-dashboard-daemon --all-targets` fails with an **error**, not
a warning, at `ws-dashboard/crates/daemon/src/agent_attention.rs:178`:
`clippy::never_loop`. It is unrelated to whatever is being worked on, so every
phase that runs clippy has to first establish that the pre-existing error is the
only one — which means clippy cannot currently serve as a pass/fail gate for
anything.

Three separate agents hit it during
`260726-bug-dashboard-terminal-notify-silent-failure-no-expiry` Phase 1 and each
had to be told in advance that it was pre-existing.

## The Cause

The lint is correct. Inside `stream::unfold`'s closure:

```rust
loop {
    match rx.recv().await {
        Ok(event) => { ...; return Some((..., rx)); }
        Err(RecvError::Lagged(_)) => return None,
        Err(RecvError::Closed) => return None,
    }
}
```

Every arm returns, so the `loop` body can never reach a second iteration.

The `loop` is a leftover. It was load-bearing while the `Lagged` arm was a
`continue` — the shape `document_events` still uses. When the attention stream
deliberately diverged to end on lag instead of skipping forward, the `continue`
became `return None` and the loop lost its only reason to exist. The CONTRACT
comment immediately above the `Lagged` arm documents that divergence and is
still accurate; only the surrounding `loop` is stale.

## First Step

Delete the `loop` wrapper and keep the `match` as the closure body, preserving
the CONTRACT comment verbatim. Then confirm
`cargo clippy -p ws-dashboard-daemon --all-targets` reports no errors, and that
the attention coverage still passes — in particular whatever asserts the
end-on-lag behavior, since that is the semantics the removed loop used to
surround.

Do not "fix" it with an `#[allow]`. The lint is pointing at real dead structure,
and suppressing it would preserve the misleading suggestion that the closure can
iterate.

## Notes

- Filed as `idea/` because it is a one-line cleanup with a verification step,
  not because it is uncertain. Promote it directly if a session wants a green
  clippy gate.
- Worth checking in the same pass whether clippy is wired into any CI or
  pre-commit path. If it is, it has presumably been red there too; if it is not,
  that is the more interesting finding, and a follow-up could make it a gate
  once this error is gone.
- Note the run also reports 16 ordinary clippy warnings in the daemon lib. They
  are out of scope here; this ticket is only about the one error that makes the
  command fail.


## Resolution (2026-07-29)

Fixed during the `47314c17` merge reconciliation, as a prerequisite rather than a
goal: that merge mandated a green `cargo clippy --all-targets`, and this
deny-by-default `clippy::never_loop` in the attention SSE stream
(`agent_attention.rs`) was what kept the gate permanently red.

The vestigial `loop` had every arm terminal, so removing it is
semantics-preserving.

Verified on the reconciled tree: `cargo clippy --all-targets` from
`ws-dashboard/crates/daemon` finishes with 0 errors and 0 `never_loop`
occurrences. Remaining output is warnings only, all pre-existing — checked
against a baseline that emits the same count.

The lint gate can now actually gate a phase, which is what this ticket asked
for.
