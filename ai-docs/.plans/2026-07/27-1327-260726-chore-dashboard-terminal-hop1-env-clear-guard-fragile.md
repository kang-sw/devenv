# Plan: 260726-chore-dashboard-terminal-hop1-env-clear-guard-fragile — Phase 1: Replace the Debug-string env guard with an asserted env plan value

## Relevant Ticket Contract

- Add a pure `helper_env_plan(command, scrub, host_env) -> HelperEnvPlan` in
  `terminal.rs`; `HelperEnvPlan` is `{ InheritHost, ClearAndSet(Vec<(OsString,
  OsString)>) }`, `#[derive(Debug, Eq, PartialEq)]`.
- `build_helper_command` applies the plan at exactly one site:
  `match helper_env_plan(...) { InheritHost => {}, ClearAndSet(env) => {
  helper_command.env_clear().envs(env); } }`, replacing today's inline
  `.env_clear().envs(...)` call.
- No caller-visible behavior change; default (no-command) path stays
  byte-for-byte identical (no `.env()`/`.env_clear()` call at all).
- Preserve the command/scrub pairing invariant: `ClearAndSet` is keyed off
  `command.is_some()`, and the defensive `scrub.unwrap_or(&CLAUDE)` fallback
  is kept, not removed or turned into a panic.
- **One fallback resolution, shared by both consumers** — the env-scrub path
  and the `--scrub-marker` argv loop must provably read the same resolved
  profile, not two independently-defaulted ones.
- Keep the hardened unix secondary detector (`!debug.contains("env -i")`)
  plus its own positive control (a locally-built `env_clear()`ed `Command`
  whose `Debug` must contain `"env -i"`), `#[cfg(unix)]`, non-load-bearing.
- Add a positive-control test proving `helper_env_plan` discriminates in
  both directions (not just always `InheritHost`).
- The single application site must carry a CONTRACT comment stating: (a)
  std's missing clear-flag introspection, (b) the plan value — not the built
  `Command` — is the guarded surface, (c) the named residual (an
  `env_clear()` written directly into `build_helper_command` outside the
  plan-application site is invisible to this guard on every platform), (d)
  that `build_helper_command` still evaluates `command.is_some()` twice (its
  own argv `if let` and the plan match) against the same `command`
  reference, so they cannot diverge.
- Mandatory non-vacuity proof: apply M1–M4 mutations to production source,
  observe the named failing test/site, then revert; record mutation ->
  failing test -> failure site.
- No new dependency; no widening `build_helper_command`'s visibility; do not
  touch `terminal_helper_process.rs` (hop 2).

## Out of Scope

- Any Windows CI job addition.
- Real-process spawn / end-to-end shell env tests (rejected in ticket
  Decisions).
- `terminal_helper_process.rs` (hop 2) changes.
- Changes to marker lists, profile registry contents, or the
  `scrub.unwrap_or(&CLAUDE)` constant itself.
- Any later phase (ticket states this is a single-phase ticket; none exist).

## Codebase Findings

**Current state (`build_helper_command`, `ws-dashboard/crates/daemon/src/terminal.rs`):**

- `crates/daemon/src/terminal.rs#L1310-L1388` — `fn build_helper_command(...)`
  (private, unchanged signature otherwise). `if let Some((program, args)) =
  command` argv branch starts `#L1348`; inside it, `#L1368` is `let scrub =
  scrub.unwrap_or(&crate::agent_env_profile::CLAUDE);`, `#L1369-L1371` is the
  `.env_clear().envs(scrub_env_os(host_env, scrub))` call to remove, and
  `#L1379-L1380` is the `--scrub-marker` argv loop that reads
  `scrub.markers` — this loop is the second consumer of the same resolved
  value and stays inside `build_helper_command`, not inside
  `helper_env_plan`.
- `crates/daemon/src/terminal.rs#L1423-L1447` — `fn resolve_create_command`;
  its return line for the absent-`profile_id` case is `#L1435`: `return
  Ok((None, Vec::new(), None, None));`.
- `crates/daemon/src/terminal.rs#L1410-L1422` — CONTRACT comment on
  `resolve_create_command` stating "`command` and `scrub` are always
  returned paired ... never independently defaulted" — this is the actual
  location of the pairing-invariant text the ticket describes, not the line
  range the ticket cites (see Drift table below).
- `crates/daemon/src/terminal.rs#L1635-L1650` — the real call site:
  `build_helper_command(..., command.as_ref(), &env_overlay, scrub,
  std::env::vars_os())`. `host_env` in production is `std::env::vars_os()`
  (`#L1649`), confirming design-question 3 in the delegate prompt: the real
  caller already passes the full host env; no change needed here.
- `crates/daemon/src/terminal.rs#L2789-L2841` — the existing test
  `helper_spawn_default_no_command_matches_existing_arg_shape` to be
  extended: `#L2790-L2805` builds `command` via `build_helper_command(...,
  None, &[], None, Vec::<(std::ffi::OsString, std::ffi::OsString)>::new())`
  — i.e. it already uses an **empty `Vec::<(OsString, OsString)>::new()`
  fixture** for the default-path host_env; `#L2807-L2810` is the
  platform-neutral `get_envs().next().is_none()` assertion to keep;
  `#L2811-L2823` is the CONTRACT comment on std's missing clear-flag
  introspection (this is the exact block the ticket wants preserved, not
  deleted — see Constraints); `#L2824-L2833` is the unix `starts_with("env
  ")` block to harden into `!debug.contains("env -i")` plus its own positive
  control.
- `crates/daemon/src/terminal.rs#L2843-L2920` — sibling test
  `helper_spawn_with_command_scrubs_claude_markers_and_forwards_argv`: an
  existing, directly reusable fixture pattern — builds `host_env` from every
  `CLAUDE.markers` entry plus `PATH` plus a non-marker `SOME_OTHER_VAR`
  (`#L2845-L2866`), then asserts every marker absent and `PATH`/
  `SOME_OTHER_VAR` present (`#L2894-L2902`). The new `helper_env_plan`
  positive-control test should mirror this exact fixture shape (swap
  `SOME_OTHER_VAR` for `HOME` per the ticket's literal wording, or keep
  both) and assert against the returned `HelperEnvPlan::ClearAndSet(pairs)`
  instead of pulling envs off a built `Command`.
- `crates/daemon/src/terminal.rs#L2668-L2674` and `#L2730-L2736` — the two
  (duplicated, one per boot-reconcile row test) comment blocks documenting
  "`CARGO_BIN_EXE_ws-dashboard` ... only compile-time-defined inside
  integration test/bench targets, not the lib crate's own `#[cfg(test)]`
  unit tests" — this is the actual current location of that knowledge, not
  the ticket's cited range (see Drift table).
- `crates/daemon/src/agent_env_profile.rs#L55-L58` (`NONE`), `#L69-L81`
  (`scrub_env_os`) — **exact match**, no drift. `scrub_env_os(env: impl
  IntoIterator<Item = (OsString, OsString)>, profile: &EnvScrubProfile) ->
  Vec<(OsString, OsString)>` is a plain deny-list filter; `helper_env_plan`'s
  `ClearAndSet` branch can call it directly and use its return value
  as-is (already the right shape) — no extra `.into_iter().collect()`
  needed.
- `crates/daemon/src/agent_env_profile.rs#L92-L110` —
  `scrub_env_os_removes_exactly_the_claude_markers_and_preserves_the_rest`:
  the canonical fixture pattern (all `CLAUDE.markers` + `PATH` + `HOME`) —
  matches exactly what the ticket's positive-control test wants; reuse this
  shape.
- `crates/daemon/src/agent_profile_registry.rs#L116-L122`, `#L146-L152` —
  **exact match**, no drift. `DUMMY_ECHO_PROFILE`/`DUMMY_ECHO_HOOKED_PROFILE`
  both set `scrub: &agent_env_profile::NONE` at `#L120`/`#L150` respectively.
- `crates/daemon/src/terminal_helper_process.rs#L393-L419` — CONTRACT
  comment; the "hop 2's inherited env is itself inherited wholesale from hop
  1" sentence is at `#L397-L402`, **exact match**, no drift.
- `crates/daemon/src/terminal_helper_process.rs#L1126-L1161` — sibling hop-2
  guard `spawn_shell_default_no_command_matches_existing_behaviour`; the
  `command.get_env("PATH").is_some()` assertion is at `#L1157` (ticket cites
  `:1156`, the `assert!(` opening brace — 1-line drift, negligible, no
  action needed). Untouched by this phase either way (Constraints:
  `terminal_helper_process.rs` is out of scope).

**Drift table — ticket citation vs. verified current location** (all against
this branch's actual `terminal.rs`; `agent_env_profile.rs` and
`agent_profile_registry.rs` citations had zero drift):

| Ticket cites | Actual content is at | Drift |
|---|---|---|
| `terminal.rs:2691` (test fn) | `#L2789` | +98 |
| `terminal.rs:2710` (`get_envs().next().is_none()`) | `#L2808` | +98 |
| `terminal.rs:2727` (`starts_with("env ")`) | `#L2828` | +101 |
| `terminal.rs:1306` (`command.is_some()` branch) | `#L1348` | +42 |
| `terminal.rs:1328` (`.env_clear()`) | `#L1369-L1371` | +41/+43 |
| `terminal.rs:1392` (`resolve_create_command` None-case return) | `#L1435` | +43 |
| `terminal.rs:1293` (`--cwd` argv flag) | `#L1335-L1336` | +42/+43 |
| `terminal.rs:1268` ("build_helper_command is private") | fn at `#L1310` | +42 |
| `terminal.rs:1326` (`scrub.unwrap_or(&CLAUDE)`) | `#L1368` | +42 |
| `terminal.rs:1329` (env scrub application) | `#L1369-L1371` | +40/+42 |
| `terminal.rs:1337` (`--scrub-marker` loop) | `#L1379-L1380` | +42/+43 |
| `terminal.rs:1374-1379` (pairing-invariant CONTRACT) | `#L1410-L1422` (esp. `#L1418-L1420`) | +36 to +43, and it's the `resolve_create_command` doc comment, not a `build_helper_command`-adjacent comment |
| `terminal.rs:2714-2726` (std clear-flag introspection knowledge) | `#L2811-L2823` | +97 |
| `terminal.rs:2634-2639` (`CARGO_BIN_EXE_*` note) | `#L2668-L2674` and duplicated at `#L2730-L2736` | +34, and now appears twice (once per boot-reconcile row test) — real `#2634-2639` content today is an unrelated NOTE about boot-reconcile row-test naming |
| `terminal_helper_process.rs:1156` | `#L1157` | +1 (negligible) |
| `agent_env_profile.rs:55`, `:69` | same | 0 |
| `agent_profile_registry.rs:120,150` | same | 0 |
| `terminal_helper_process.rs:397-402` | same | 0 |

The whole `build_helper_command`-adjacent block drifted by a consistent
**+42 lines**; the two later-in-file test/CONTRACT blocks drifted by
**+97/+98/+101 lines** (larger, not perfectly uniform — the intervening
`resolve_create_command` unit tests and boot-reconcile row tests grew
independently). Root cause is simply prior unrelated commits adding code
above these points (e.g. `post_terminal_turn_state`, `resolve_create_command`
tests, `260723` boot-reconcile row tests) — nothing about the cited code
itself changed; every citation still names real, relevant code, just at a
different line.

## Implementation Plan

1. **Add `HelperEnvPlan` and `helper_env_plan`** in
   `crates/daemon/src/terminal.rs`, placed immediately before
   `build_helper_command` (currently `#L1310`), replacing/extending the
   existing CONTRACT comment block at `#L1291-L1309`:
   ```rust
   #[derive(Debug, Eq, PartialEq)]
   enum HelperEnvPlan {
       InheritHost,
       ClearAndSet(Vec<(std::ffi::OsString, std::ffi::OsString)>),
   }

   fn helper_env_plan(
       command: Option<&(String, Vec<String>)>,
       scrub: Option<&crate::agent_env_profile::EnvScrubProfile>,
       host_env: impl IntoIterator<Item = (std::ffi::OsString, std::ffi::OsString)>,
   ) -> HelperEnvPlan {
       match command {
           None => HelperEnvPlan::InheritHost,
           Some(_) => {
               let scrub = scrub.unwrap_or(&crate::agent_env_profile::CLAUDE);
               HelperEnvPlan::ClearAndSet(crate::agent_env_profile::scrub_env_os(host_env, scrub))
           }
       }
   }
   ```
   The internal `scrub.unwrap_or(&CLAUDE)` here is a defensive/non-panicking
   fallback for a direct unit-test caller (mirrors the codebase's existing
   "fallback, never panic" philosophy at the current `#L1361-L1367`
   comment) — see Fallback Resolution Recommendation below for why this is
   provably dead code on the only production call path and does not violate
   "resolve exactly once".

2. **Restructure `build_helper_command`** (`#L1310-L1388`):
   - Hoist the fallback resolution to the top of the function, unconditional
     on `command`:
     ```rust
     let scrub = scrub.unwrap_or(&crate::agent_env_profile::CLAUDE);
     ```
     (shadows the `scrub: Option<&EnvScrubProfile>` parameter; cheap, no
     behavior change since it is a `&'static` reference select, and it was
     already being computed on the `command.is_some()` path today).
   - Inside the existing `if let Some((program, args)) = command { ... }`
     argv branch (currently `#L1348-L1382`): keep everything unchanged
     **except** remove the `.env_clear().envs(crate::agent_env_profile::
     scrub_env_os(host_env, scrub).into_iter())` call (currently
     `#L1369-L1371`). The `--scrub-marker` loop (`#L1379-L1380`, `for marker
     in scrub.markers { helper_command.arg("--scrub-marker").arg(*marker);
     }`) stays exactly as-is, now reading the function-top-hoisted `scrub`.
   - After the `if let Some(...)` block, before the stdio setup
     (`#L1384-L1386`), add the single application site:
     ```rust
     // CONTRACT (Phase 1, hop-1 env-plan guard): the guarded surface is
     // this plan VALUE, not the `Command` built below - `std::process::
     // Command` exposes no public API to tell "no env method ever called"
     // apart from "env_clear() called with nothing re-added" (both report
     // an empty get_envs() iterator), so a Debug-string sniff was the only
     // prior observable and it is fragile (see the unix secondary detector
     // below and its own CONTRACT). Note `command.is_some()` is evaluated
     // TWICE in this function - once by the `if let Some((program, args))`
     // argv branch above (owns --command/--command-arg/--env-overlay/
     // --scrub-marker) and once by `helper_env_plan` below - both read the
     // SAME `command` reference, so they cannot diverge. KNOWN RESIDUAL: an
     // `env_clear()` written directly into this function outside this one
     // application site is invisible to this guard (and to std's public API)
     // on every platform; the hardened unix secondary detector in the test
     // below is the only thing that can still catch that specific case.
     match helper_env_plan(command, Some(scrub), host_env) {
         HelperEnvPlan::InheritHost => {}
         HelperEnvPlan::ClearAndSet(env) => {
             helper_command.env_clear().envs(env);
         }
     }
     ```
     Passing `Some(scrub)` here — the already-hoisted, already-resolved
     value — keeps `helper_env_plan`'s parameter as the literal `Option<
     &EnvScrubProfile>` type with zero signature deviation (see Fallback
     Resolution Recommendation).

3. **Extend `helper_spawn_default_no_command_matches_existing_arg_shape`**
   (`#L2789-L2841`):
   - Keep the platform-neutral `get_envs().next().is_none()` assertion
     (`#L2807-L2810`).
   - Add, unconditional (`#[cfg]`-free):
     ```rust
     assert_eq!(
         helper_env_plan(None, None, Vec::<(std::ffi::OsString, std::ffi::OsString)>::new()),
         HelperEnvPlan::InheritHost
     );
     ```
   - Replace the `#[cfg(unix)] { let debug = ...; assert!(!debug.starts_with("env "), ...); }`
     block (`#L2824-L2833`) with the hardened detector plus its positive
     control, still `#[cfg(unix)]`:
     ```rust
     #[cfg(unix)]
     {
         let debug = format!("{command:?}");
         assert!(
             !debug.contains("env -i"),
             "default path's Debug rendering must not contain an env_clear() \
              marker (env -i): {debug:?}"
         );
         // Positive control (secondary detector's own control, M3 target):
         // proves the "env -i" substring check is actually live, not
         // passing on a string that would match anything.
         let mut cleared = std::process::Command::new("/usr/bin/true");
         cleared.env_clear();
         let cleared_debug = format!("{cleared:?}");
         assert!(
             cleared_debug.contains("env -i"),
             "positive control: a deliberately env_clear()ed Command's Debug \
              must contain env -i, or this secondary detector cannot fire: {cleared_debug:?}"
         );
     }
     ```
   - Keep `#L2811-L2823` (the CONTRACT comment on std's missing clear-flag
     introspection) — do not delete this per Constraints; relocate it
     adjacent to the new assertions if the block is reordered.

4. **Add a new positive-control test** proving `helper_env_plan`
   discriminates in both directions, e.g.
   `helper_env_plan_with_command_scrubs_claude_markers_and_preserves_others`
   (name chosen for this plan; adjust if a better name fits nearby
   convention), placed near the existing sibling tests (`#L2843-L2984`).
   Mirror the fixture-building pattern from
   `helper_spawn_with_command_scrubs_claude_markers_and_forwards_argv`
   (`#L2845-L2866`): build every `CLAUDE.markers` entry plus `PATH` plus
   `HOME`, call `helper_env_plan(Some(&("agent-cli".to_owned(), Vec::new())),
   Some(&crate::agent_env_profile::CLAUDE), host_env)`, and assert the
   returned `HelperEnvPlan::ClearAndSet(pairs)` contains no marker key while
   `PATH` and `HOME` are both present with their original values (e.g. via
   `matches!` destructure or a small helper converting `pairs` into a
   `HashMap` the same way the existing sibling test does at `#L2885-L2893`).

5. **Non-vacuity proof (mandatory)** — apply each mutation to production
   source, run `cargo test -p ws-dashboard-daemon --no-fail-fast` (see
   Verification Plan), record failing test name + failure site, then revert
   before the next mutation:
   - **M1** (primary guard): in `helper_env_plan`, change the `None =>
     HelperEnvPlan::InheritHost` arm to `None =>
     HelperEnvPlan::ClearAndSet(host_env.into_iter().collect())`. Expected
     failure: `terminal::tests::
     helper_spawn_default_no_command_matches_existing_arg_shape`, at the new
     `assert_eq!(helper_env_plan(None, None, ...), HelperEnvPlan::InheritHost)`
     line.
   - **M2** (scrub-side positive control): in `agent_env_profile.rs`
     `scrub_env_os` (`#L69-L81`), change the body to `env.into_iter().
     collect()` (drop the `.filter(...)`). Expected failure: the new test
     from step 4 (`helper_env_plan_with_command_scrubs_claude_markers_and_preserves_others`),
     at its marker-absence assertion.
   - **M3** (secondary detector's own control): in the test from step 3,
     remove `cleared.env_clear();` from the locally-built `cleared` Command.
     Expected failure: the same test
     (`helper_spawn_default_no_command_matches_existing_arg_shape`), at the
     `cleared_debug.contains("env -i")` assertion.
   - **M4** (revert check): revert M1–M3, re-run the full suite, confirm the
     failure set is exactly the two pre-existing `routes.rs` sites below and
     `git status`/`git diff` show a clean tree.

6. **Do not touch** `terminal_helper_process.rs` (hop 2) or
   `agent_profile_registry.rs` beyond the M2 mutate-then-revert step above.

## Verification Plan

- Command: from `ws-dashboard/`, run
  `cargo test -p ws-dashboard-daemon --no-fail-fast > /tmp/out.txt 2>&1`
  then `echo $?` **on the next line of the same invocation** (never `| tee`,
  `| tail`, or `cmd; echo $?`) — per ticket, `--no-fail-fast` is required
  because the `routes` integration target's two known failures otherwise
  abort the run before later integration targets execute.
- **Current true baseline** (lead re-ran on this branch at `b7f524f7`,
  supersedes the ticket's own stale `174 passed; 2 failed` / `204 passed; 0
  failed; 2 ignored` numbers):
  - lib unit-test target: `236 passed; 0 failed; 2 ignored`
  - `tests/routes.rs`: `176 passed; 2 failed` — same two known sites,
    unrelated to this phase:
    `dashboard_resources_refresh_prunes_workspace_without_available_work_roots`
    (`crates/daemon/tests/routes.rs:1066`) and
    `online_missing_work_root_returns_bounded_unavailable_without_path_leak`
    (`crates/daemon/tests/routes.rs:1383`).
  - every other target green; overall exit status `101` even on a clean
    result — judge by failure **site**, not exit code.
- Phase is done when: the failure set after this phase's changes is exactly
  those same two `routes.rs` sites (lib target still `0 failed`, now with
  additional passing tests for `helper_env_plan`), all four M1-M4 mutations
  were observed failing at the named site and reverted, and the tree is
  clean (`git status`/`git diff` — read-only check only, no commit from this
  survey or its executor unless separately authorized).
- No frontend/Playwright verification is in scope (nothing this phase
  touches is browser-reachable).

## Fallback Resolution Recommendation

**Recommend: keep the resolution in `build_helper_command`, hoisted to the
top of the function (unconditional on `command`), feeding both the
`--scrub-marker` argv loop and — wrapped as `Some(scrub)` — the
`helper_env_plan` call.** This is Constraints' first named option ("keeping
the resolution in `build_helper_command` and passing the resolved
`&'static EnvScrubProfile` into `helper_env_plan`"), but with one refinement
worth flagging explicitly: I do **not** recommend changing
`helper_env_plan`'s `scrub` parameter to a bare (non-`Option`)
`&'static EnvScrubProfile`. Reason, from the actual code:

- The ticket's own mandatory "Completed behavior" test literals call
  `helper_env_plan(None, None, fixture)` (default-path primary guard) and
  `helper_env_plan(Some(&(...)), Some(&agent_env_profile::CLAUDE), fixture)`
  (positive control). Both pass `Option`-typed values (`None` / `Some(...)`)
  at the second argument position — this only compiles if `helper_env_plan`
  keeps the literal `Option<&EnvScrubProfile>` parameter type from
  Decisions. Changing that parameter to a bare `&'static EnvScrubProfile`
  would make the first literal test call (`None` in position 2) fail to
  compile, which is a bigger deviation than any that Constraints
  authorizes.
- The genuine risk Constraints warns about — "two independently-defaulted"
  reads of `scrub` — is fully closed without any signature change: today's
  single `let scrub = scrub.unwrap_or(&crate::agent_env_profile::CLAUDE);`
  (`#L1368`) moves to the top of `build_helper_command`, executes exactly
  once per call, and its result literally is what both the `--scrub-marker`
  loop and the `helper_env_plan(..., Some(scrub), ...)` call consume. There
  is no code path where these two consumers can see different values.
- `helper_env_plan` itself still carries `scrub.unwrap_or(&crate::
  agent_env_profile::CLAUDE)` internally, for defensive non-panicking
  behavior on a direct unit-test call with `scrub = None` and
  `command = Some(...)` — mirroring the codebase's existing stated
  philosophy at `#L1361-L1367` ("panicking or silently skipping the scrub"
  is explicitly rejected). This is provably **dead code on the only
  production call path**, because `build_helper_command` never passes `None`
  once the hoist lands — it is not a second live resolution, only a
  total-function safety net for an input shape production never produces.
  No mandatory test in the ticket exercises this exact branch
  (`Some(command), None, host_env`), so nothing depends on which specific
  fallback value it uses, only that it does not panic.

I did not recommend Constraints' second option (`helper_env_plan` returning
the resolved profile alongside the plan) because it requires changing the
return type away from the literal, mandatory-test-pinned `HelperEnvPlan` —
the `assert_eq!(helper_env_plan(...), HelperEnvPlan::InheritHost)` /
`... is ClearAndSet ...` literals in "Completed behavior" only typecheck
against a bare `HelperEnvPlan` return, not a tuple.

## Escalations

- None.
