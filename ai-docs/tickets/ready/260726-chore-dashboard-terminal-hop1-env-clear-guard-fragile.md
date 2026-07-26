---
title: hop-1 default-spawn env regression guard is fragile and platform-partial
related:
  260725-feat-dashboard-pty-agent-attention-notification: found-during
sage-review-design: completed
sage-review-completeness: completed
---

# hop-1 default-spawn env regression guard is fragile and platform-partial

## Background

**The guard.** `helper_spawn_default_no_command_matches_existing_arg_shape`
is a lib unit test in `ws-dashboard/crates/daemon/src/terminal.rs`
(`terminal.rs:2691`; the original capture's "around :2064" was wrong). It
builds the default (no explicit command) helper command through
`build_helper_command` and asserts two things:

1. `command.get_envs().next().is_none()` — platform-neutral, catches a stray
   `.env()` addition (`terminal.rs:2710`).
2. Under `#[cfg(unix)]` only: `!format!("{command:?}").starts_with("env ")` —
   the actual `env_clear()` detector (`terminal.rs:2727`).

**What it protects.** Hop 1 is the daemon's spawn of the per-terminal helper
process. `build_helper_command` calls `.env_clear().envs(...)` only inside
its `command.is_some()` branch (branch at `terminal.rs:1306`, clear at
`terminal.rs:1328`). An ordinary shell terminal reaches that function with
`command: None`, because `resolve_create_command` returns
`(None, vec![], None, None)` the moment `profile_id` is absent
(`terminal.rs:1392`) — a literal no-branch-taken path. So the env scrub is
**profile-gated at the branch**, and ordinary shell terminals are untouched
by gating, not merely by an empty marker list. (The empty-marker
`agent_env_profile::NONE` no-op — `agent_env_profile.rs:55`, used by the
test-only `dummy-echo` profiles at `agent_profile_registry.rs:120,150` — is a
different mechanism: it makes `scrub_env_os` inert for a *resolved* profile
that has no vendor markers. It never applies to a profile-less shell spawn,
which never reaches the scrub call at all.)

The regression the guard exists to catch is therefore an unconditional
`env_clear()` at hop 1: it would hand every terminal helper an empty
environment, and because hop 2 (the helper's own shell spawn) inherits hop
1's env wholesale (`terminal_helper_process.rs:397`-`402`), every ordinary
shell terminal — the most-used path in the product — would come up with no
`PATH` and no `HOME`.

**Why the guard is weak.** Four findings, all verified against this
toolchain (rustc 1.95.0):

1. It couples a test to `std::process::Command`'s `Debug` format, which is
   undocumented and unstable.
2. **It can go vacuous with no std change at all.** The unix `Debug` impl
   writes `cd {cwd:?} && ` *before* the `env -i ` marker when a cwd is set
   (std `sys/process/unix/common.rs:533`-`538`). `build_helper_command` sets
   no `current_dir` today (it passes the cwd as the `--cwd` argv flag
   instead, `terminal.rs:1293`), so `starts_with("env ")` works right now —
   but the day an ordinary local refactor adds a `current_dir`, the
   assertion becomes permanently false-negative and never fails. This is the
   ticket's defect in its purest form, and it is reachable from this repo,
   not only from an upstream std change.
3. It is `#[cfg(unix)]`-gated because the Windows `Debug` impl prints only
   program + args, with no env content whatsoever (std
   `sys/process/windows.rs:438`-`450`). Windows is a supported target here
   (`crates/daemon/tests/terminal_windows_reaper_acceptance.rs`), so hop 1
   has **zero** coverage for this regression on Windows.
4. Root cause: `std::process::Command` exposes no public clear-flag
   introspection. `get_envs()` yields the same empty iterator for "no env
   method ever called" and for "`env_clear()` called, nothing re-added". The
   `Debug` string is the only public signal, and only on unix.

**Prior art in this codebase.** Hop 2's mirror guard,
`spawn_shell_default_no_command_matches_existing_behaviour`, has no such
problem: `portable_pty::CommandBuilder` seeds a *real* base env, so
`command.get_env("PATH").is_some()` is a direct, platform-neutral proof that
nothing cleared the base env (`terminal_helper_process.rs:1156`). Hop 1's
`std::process::Command` has no equivalent observable — which is why the fix
has to create one rather than look harder for one.

Also relevant: the parent ticket's Phase 1 already paid for a silently
vacuous guard once (the env-overlay pairing guard, cited in the
`resolve_create_command` CONTRACT at `terminal.rs:1374`), which is why this
ticket treats mutation proof as a deliverable rather than a nicety.

This limitation was scoped honestly by the Phase 1 implementer of
`260725-feat-dashboard-pty-agent-attention-notification` rather than hidden;
this ticket records it so it is not later read as an oversight.

## Decisions

**Settled: create the observable — a different seam, not a stronger string
assertion.** Extract the env decision from `build_helper_command` into a
pure function returning an explicit value, and assert that value directly.
Agreed shape (literal):

```rust
#[derive(Debug, Eq, PartialEq)]
enum HelperEnvPlan {
    /// Inherit the daemon's environment untouched — no `env_clear()`, no
    /// `env()`. The default (no explicit command) spawn path.
    InheritHost,
    /// Clear the inherited env and set exactly these pairs.
    ClearAndSet(Vec<(std::ffi::OsString, std::ffi::OsString)>),
}

fn helper_env_plan(
    command: Option<&(String, Vec<String>)>,
    scrub: Option<&crate::agent_env_profile::EnvScrubProfile>,
    host_env: impl IntoIterator<Item = (std::ffi::OsString, std::ffi::OsString)>,
) -> HelperEnvPlan;
```

`build_helper_command` then has exactly one env-touching site:

```rust
match helper_env_plan(command, scrub, host_env) {
    HelperEnvPlan::InheritHost => {}
    HelperEnvPlan::ClearAndSet(env) => {
        helper_command.env_clear().envs(env);
    }
}
```

Why this and not a stronger assertion: "less fragile" here means **the
assertion no longer reads a rendering of the decision, it reads the decision
itself**. `assert_eq!(helper_env_plan(None, None, fixture), HelperEnvPlan::InheritHost)`
compiles and runs identically on every target, depends on no std formatting,
and cannot be silently defeated by an unrelated refactor of the built
`Command`. It closes the fragility defect on every platform, which is what
the capture asked for. **It does not close the Windows defect outright**: it
gives Windows the same primary-guard coverage as unix for every
plan-routed decision (Windows now runs the identical `assert_eq!` unix
runs), but Windows gains no coverage for an `env_clear()` written directly
into `build_helper_command` outside the plan-application site - that residual
is platform-universal, not Windows-specific, and is named explicitly in
Constraints below and in the surviving CONTRACT comment.

**Resolved under goal-run posture (owner away, reversible local call):**
keep a *hardened, self-validating* unix `Debug` check as a secondary
detector rather than deleting it. Hardened means `!debug.contains("env -i")`
instead of `starts_with`, plus a positive control in the same test that
builds a deliberately `env_clear()`ed `Command` and asserts its `Debug`
*does* contain `env -i`. Rationale: the primary plan assertion cannot see an
`env_clear()` written directly into `build_helper_command` outside the plan
application site (see Constraints), and the secondary detector can — while
the positive control converts any future std format drift from a silent pass
into a loud failure. Cost is three assertions; the fragility that motivated
this ticket is removed because the detector is now non-load-bearing and
self-checking.

**Rejected alternatives.**

- *Real spawn-and-inspect integration test.* Rejected. `build_helper_command`
  is a private fn in the lib crate (`terminal.rs:1268`); an integration test
  cannot reach it without widening visibility purely for a test, and
  `CARGO_BIN_EXE_*` is not defined for lib unit tests (documented in-tree at
  `terminal.rs:2634`-`2639`). The built command's program is the helper
  binary with a fixed argv, so it cannot be repointed at an env-dumping
  program without also faking the argv contract the same test verifies. A
  genuine end-to-end version (create a default terminal, echo `PATH` through
  the real shell) is slow, flaky, and introduces a *second* platform matrix
  (`echo $PATH` vs `echo %PATH%`) — the exact class of problem this ticket
  removes.
- *Std-version canary test as the whole answer.* Rejected as the primary
  fix: it converts silent breakage into loud breakage on unix but leaves
  Windows at zero coverage, addressing half the stated defect. Its useful
  half is absorbed as the in-test positive control above.
- *Snapshot/assert the exact `Debug` string.* Rejected — doubles down on the
  unstable format and breaks on every unrelated std formatting tweak.
- *Accept the status quo and document it.* Rejected — the guarded regression
  silently breaks the product's most-used path, and finding 2 above shows
  the current guard can stop discriminating without anyone noticing.

## Constraints

- **No caller-visible behavior change.** The default path must stay
  byte-for-byte identical, including the existing "no `.env()` and no
  `.env_clear()` at all" property. This is a test-and-seam chore.
- **Preserve the command/scrub pairing invariant.** `resolve_create_command`
  returns `command` and `scrub` paired, never independently defaulted
  (`terminal.rs:1376`-`1379` CONTRACT). `helper_env_plan` keys `ClearAndSet`
  off `command.is_some()`, exactly as the current branch does, and keeps the
  defensive `scrub.unwrap_or(&agent_env_profile::CLAUDE)` fallback
  (`terminal.rs:1326`) rather than panicking or skipping the scrub.
- **One fallback resolution, shared by both consumers.** Today the single
  `let scrub = scrub.unwrap_or(&CLAUDE);` binding at `terminal.rs:1326`
  feeds BOTH the env scrub (`terminal.rs:1329`) and the `--scrub-marker`
  argv loop that threads the same list to hop 2 (`terminal.rs:1337`,
  CONTRACT C1). Moving the fallback entirely inside `helper_env_plan`
  removes that shared binding from `build_helper_command` - the argv loop
  would then have only the unresolved `Option<&EnvScrubProfile>` to read
  `.markers` from, which does not compile on `None` and, if patched
  independently (e.g. `if let Some(scrub) = scrub { ... }`), would silently
  make hop 1 clear with the `CLAUDE` fallback while hop 2 receives zero
  markers on the defensive `command=Some`/`scrub=None` path. The
  implementation must resolve the fallback exactly once - either by keeping
  the resolution in `build_helper_command` and passing the resolved
  `&'static EnvScrubProfile` into `helper_env_plan`, or by having
  `helper_env_plan` return the resolved profile alongside the plan - so the
  argv loop and the env plan are provably reading the same resolved profile,
  not two independently-defaulted ones.
- **Known residual, must be named in code, not silently dropped.** An
  `env_clear()` added directly inside `build_helper_command` outside the
  single plan-application site is invisible to std's public API on every
  platform; the plan assertion would still pass. The mitigation is
  structural (one application site) plus the retained unix secondary
  detector, and the surviving CONTRACT comment must say so explicitly. Do
  not let the rewrite quietly delete the "std has no clear-flag
  introspection" knowledge recorded at `terminal.rs:2714`-`2726`.
- **No new dependency**, and no test-only widening of `build_helper_command`'s
  visibility.
- Hop 2's guard is already correct and platform-neutral; this ticket does not
  touch `terminal_helper_process.rs`.

## Spec Impact

**This ticket changes no caller-visible behavior**, and that is the
deliberate, stated way it satisfies the spec-address gate — not an omission.

The behavior in question is already specified, and stays exactly as written:
`260725-ws-web-dashboard-terminal-spawn-profile`
(`ai-docs/spec/ws-web-dashboard/index.md:2124`, under *Terminal Registry And
PTY Spawn*) states that "an absent profile id keeps the default
interactive-shell spawn unchanged, byte for byte, from a request that names
no profile at all." That sentence is precisely the invariant this guard
defends. Phase 1 changes only *how the repository proves* that sentence
(an asserted plan value instead of a `Debug`-string sniff) and how a private
builder is factored internally. No HTTP contract, no request/response field,
no PTY-visible env, and no spec text changes.

Consequently there is no `spec:` frontmatter entry: the ticket implements no
spec entry, it hardens the regression guard for one that already exists. If
implementation finds itself needing to change what a caller observes, that is
a signal the scope drifted and the phase should stop, not a signal to edit
the spec.

Contract-first spec: no.

## Phases

Single phase: the seam extraction and the guard rewrite are one change — the
new assertion has nothing to assert until the plan value exists, and the plan
value has no purpose until the assertion uses it. There is no sequential
dependency worth splitting.

### Phase 1: Replace the Debug-string env guard with an asserted env plan value

**Completed behavior.**

- `helper_env_plan` exists as a pure fn in `terminal.rs` with the literal
  signature and `HelperEnvPlan` shape given in Decisions.
- `build_helper_command` computes the plan once and applies it at a single
  site; that site carries a CONTRACT comment recording (a) std's missing
  clear-flag introspection, (b) that the plan value — not the built
  `Command` — is the guarded surface, (c) the named residual from
  Constraints, and (d) that `build_helper_command` still evaluates
  `command.is_some()` twice - once in its own `if let Some((program, args))
  = command` argv branch (unchanged, owns `--command`/`--command-arg`/
  `--env-overlay`/`--scrub-marker`) and once inside the `match
  helper_env_plan(command, scrub, host_env)` env site - and that both
  branches are given the same `command` reference so they cannot diverge;
  the comment must say this explicitly since no test asserts the two
  branches agree with each other.
- `helper_spawn_default_no_command_matches_existing_arg_shape` keeps its name
  (it still owns the default-path arg-shape assertions) and keeps its
  platform-neutral `get_envs()` assertion, but its `#[cfg(unix)]`
  `starts_with("env ")` assertion is replaced by:
  - a `#[cfg]`-free `assert_eq!(helper_env_plan(None, None, fixture), HelperEnvPlan::InheritHost)`
    as the primary guard, and
  - the hardened `!debug.contains("env -i")` secondary detector plus its
    positive control, still `#[cfg(unix)]`.
- A positive-control test asserts `helper_env_plan(Some(&("agent-cli".into(), vec![])), Some(&agent_env_profile::CLAUDE), fixture)`
  is `ClearAndSet` with every `CLAUDE` marker removed and `PATH`/`HOME`
  preserved — so the plan is proven to discriminate in both directions, not
  just to return `InheritHost` unconditionally.
- Windows now runs the same primary guard as unix, because it is no longer
  `#[cfg]`-gated.

**Deferred scope (explicitly not this phase).**

- No Windows CI job is added. The point of the refactor is that the primary
  assertion needs no platform-specific rendering, so a darwin/linux run
  exercises the identical assertion Windows would; adding CI infrastructure
  is a separate concern with its own cost.
- No real-process spawn test, no end-to-end shell env check (rejected above).
- `terminal_helper_process.rs` (hop 2) is untouched.
- No change to marker lists, profile registry contents, or the
  `scrub.unwrap_or(&CLAUDE)` defensive fallback.

**Non-vacuity proof — mandatory, this is why the ticket exists.** A guard
that cannot fail is the defect being removed, so the new guard must be shown
to discriminate before the phase is considered done. Apply each mutation to
production source, run the suite, record the failing test name and failure
site, then revert:

- **M1 (primary guard).** In `helper_env_plan`, make the `command.is_none()`
  case return `HelperEnvPlan::ClearAndSet(host_env.into_iter().collect())`
  instead of `InheritHost` — i.e. simulate an accidental unconditional
  clear. The default-path plan assertion MUST fail. This is the mutation the
  whole ticket is about; if it passes, stop and rethink the seam.
- **M2 (scrub-side positive control).** Make `scrub_env_os`
  (`agent_env_profile.rs:69`) return its input unfiltered. The
  `ClearAndSet` marker-removal assertion MUST fail, proving the positive
  control is not merely asserting "some `ClearAndSet` came back".
- **M3 (secondary detector's own control).** Remove `.env_clear()` from the
  positive control's locally-built cleared `Command` inside the test. The
  `env -i` positive-control assertion MUST fail — this proves the format
  canary itself is live rather than passing on a string that would match
  anything.
- **M4 (revert check).** Revert all mutations, re-run, and confirm the
  failure set returns to exactly the two known sites below and the tree is
  clean.

Record the mutation evidence (mutation → failing test name → failure site) in
this phase's Result, in the same form the parent ticket used.

**Verification boundary.**

- Daemon-side: `cargo test -p ws-dashboard-daemon`, run from `ws-dashboard/`.
- **Two failures are KNOWN and pre-existing on this branch** — verified
  2026-07-26 *before* any change from this ticket:
  - `dashboard_resources_refresh_prunes_workspace_without_available_work_roots`
    at `crates/daemon/tests/routes.rs:1066`
  - `online_missing_work_root_returns_bounded_unavailable_without_path_leak`
    at `crates/daemon/tests/routes.rs:1383`

  The baseline run was `174 passed; 2 failed` in the `tests/routes.rs`
  target, with the lib unit-test target (which owns this ticket's guard)
  reporting `204 passed; 0 failed; 2 ignored`. **Exit status is 101 even on
  a clean result — judge by failure SITE, not exit code.**
- Exit-status capture discipline: run `cmd > file 2>&1` on one line and
  `echo $?` on the NEXT line. Never `cmd | tee`, `cmd | tail`, or
  `cmd; echo $?` — the pipeline's exit status is the pager's, not the
  command's.
- No frontend or Playwright verification is in scope: nothing this phase
  touches is reachable from the browser.
- Phase is done when: the suite's failure set is exactly the two sites above,
  every mutation M1-M4 was observed failing at a named site and reverted, and
  the tree is clean.
