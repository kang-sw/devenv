# Plan: 260725-feat-dashboard-pty-agent-attention-notification — Phase 3 step 1 (turn-start hook verification spike)

## Relevant Ticket Contract

- `## Decisions` > "The turn-START signal is not yet verified and gates the
  payload vocabulary" (`260725-feat-dashboard-pty-agent-attention-notification.md:175-190`):
  only `Stop` is measured; `UserPromptSubmit` is the candidate turn-start
  signal and is unverified. "Phase 3 must verify a turn-start hook by the same
  method used for `Stop` (drive the real binary under a PTY, observe the
  artifact) BEFORE the stream payload is fixed. If no turn-start hook fires,
  the first slice ships a two-state vocabulary (`ready`/`idle`) and `working`
  moves to `## Deferred scope`... Do not infer `working` from output-idle
  timing."
- `## Phases` preamble (`:366-377`): Phase 3 step 1 is a "SEPARABLE GATE" that
  "touches no daemon code," may be "completed and its result recorded on its
  own," and should "run out of order, ahead of Phase 1."
- `### Phase 3` step 1 (`:430-433`): "FIRST, verify a turn-start hook by the
  method that verified `Stop`: drive the real vendor binary under a PTY and
  observe the artifact. If none fires, drop `working` from the vocabulary...
  and record that here."
- Verification line for Phase 3 (`:440-442`): "the spike result recorded
  explicitly (including a negative result)."
- Method precedent, cited by the ticket as the bar to match
  (`260725-research-ws-dashboard-pty-agent-pivot.md:277-303`, "Signal source:
  vendor hooks (VERIFIED, not assumed)"): "A `Stop` hook supplied via
  `--settings` FIRES IN A REAL INTERACTIVE PTY SESSION. Verified 2026-07-25 on
  macOS by driving `claude` under `pty.fork()`, sending a prompt, and
  observing the hook artifact written BEFORE the session was terminated (so it
  cannot be confused with a session-teardown write)." Also flags a real
  precedent to worry about: `Notification`/`idle_prompt` "did NOT fire in the
  PTY run above, but that run ended at the 60s mark and never idled long
  enough to be a real test" — i.e. a negative result must be resistant to
  "the run just wasn't long/careful enough," not only to "the harness was
  broken."
- A negative result is an accepted, complete, terminal outcome for this step —
  not a state to retry until positive.

## Out of Scope

- Phase 1 (argv/env passthrough) and Phase 2 (agent spawn path from the
  browser) — the spike drives `claude` directly, not through the dashboard
  daemon/helper/spawn seam.
- Phase 3 steps 2-3 (materializing the hook config under
  `agent-profiles/<terminal_id>/` at `0600`, and the hidden
  `ws-dashboard terminal-notify` subcommand) — those depend on this step's
  answer and are not touched here.
- Any daemon or frontend source change. This step is a standalone, throwaway
  verification script; nothing here ships.
- The `Notification`/`idle_prompt` matcher spike (`research ticket:296-303`)
  — a distinct, still-unverified candidate the ticket explicitly scopes as "a
  second spike only if `Stop` proves too coarse," not part of this gate.
- Codex's hook-trust gating (`--dangerously-bypass-hook-trust`) — Claude-only
  profile per ticket `## Deferred scope`.
- Retrying the experiment hoping for a positive result. One clean run that
  produces an unambiguous fire/non-fire/inconclusive verdict closes this step.

## Codebase Findings

- `ai-docs/tickets/idea/260725-research-ws-dashboard-pty-agent-pivot.md:277-312`
  — the exact prior method and its result for `Stop`, and the one recorded
  near-miss (an under-tested negative for `Notification`) that motivates
  building the non-fire/inconclusive distinction into this spike rather than
  trusting a single quiet run.
- `ws-dashboard/crates/daemon/src/claude_cli.rs:462-498`
  (`default_deny_hook_settings`) — the only in-repo precedent for the
  `--settings` hook-injection JSON shape actually accepted by this `claude`
  binary: `{"hooks": {"<EventName>": [{"matcher": "*", "hooks": [{"type":
  "command", "command": "<shell string>"}]}]}}`, passed as `--settings
  <json-or-path>` (`claude_cli.rs:752-764` shows it passed as inline JSON via
  `.arg(&settings)`, which the research ticket already confirmed the CLI
  accepts either as a file path or inline JSON string). Reuse this shape for
  both `Stop` and `UserPromptSubmit` hook entries rather than inventing a new
  one; `matcher` is not meaningful for non-tool-scoped events but including
  `"*"` mirrors the one shape already proven to parse in this exact CLI
  version and costs nothing.
- No repo file implements the original `pty.fork()` spike — confirmed by
  `grep -rn "pty.fork\|node-pty" .` (only the two ticket prose references
  above hit) and by the frontend's `package.json` having no `node-pty`
  dependency. The original spike was an ephemeral, uncommitted script; there
  is no harness to reuse verbatim, only the method description to replicate.
  Python's standard-library `pty` module (`pty.fork()`) is present and
  importable in this environment (`python3 -c "import pty"` succeeds) and is
  the more likely original tool given the phrase "via `pty.fork()`" with no
  package named — reuse it rather than reaching for `node-pty` or a new
  dependency.
- `claude` CLI is on `PATH` at `/Users/kang-sw/.local/bin/claude`, version
  `2.1.220`. The installed binary's embedded strings (`strings -a
  ~/.local/share/claude/versions/2.1.220`) contain the literal substring
  "`Stop`/`UserPromptSubmit`/`SessionStart`: most commands don't read stdin,
  so `echo '{}' | <cmd>` suffices" — independent, source-level confirmation
  (beyond the docs-lookup the research ticket flagged as unverified) that
  `UserPromptSubmit` is a real, correctly-spelled hook event name in the
  installed CLI version, and that hook commands may safely ignore stdin.
- Risk signal: the research ticket explicitly records a near-miss where an
  unverified hook (`Notification`) looked like a non-fire only because the run
  ended too early (60s, no real idle window). The turn-start case does not
  have that specific failure mode (a prompt SEND is instantaneous, unlike
  waiting for idle), but the general lesson — a quiet log file is ambiguous
  between "did not fire" and "test ended before it could fire" — applies
  directly and is why the Verification Plan below requires a positive-control
  signal (`Stop`) in the same run rather than trusting an empty log alone.

## Implementation Plan

1. Write a throwaway Python script under the session scratchpad directory
   (e.g. `/private/tmp/claude-501/.../scratchpad/turn_start_spike.py`) — not
   under the shipped `ws-dashboard/` tree, matching the ticket's "touches no
   daemon code" framing for this step.
2. Script setup:
   - Create a fresh temp dir; inside it, one artifact log file (not two) that
     both hooks append to, e.g. `events.log`. A single shared file with
     per-line event names + high-resolution timestamps gives a directly
     readable ordering (`UserPromptSubmit` line before `Stop` line) instead of
     requiring two files' mtimes to be compared.
   - Build the `--settings` JSON (inline string, per the `claude_cli.rs`
     precedent) registering BOTH hooks in one settings block:
     - `UserPromptSubmit`: `command` = `echo "USER_PROMPT_SUBMIT $(date
       +%s.%N)" >> <events.log path>`
     - `Stop`: `command` = `echo "STOP $(date +%s.%N)" >> <events.log path>`
     Including `Stop` is deliberate: it is the ALREADY-VERIFIED signal, so its
     presence or absence in this exact run is the positive control that
     distinguishes "the turn-start hook genuinely does not fire" from "this
     run's harness/settings injection was broken" (see Verification Plan).
   - Write the settings JSON to a temp file (avoids shell-quoting the whole
     JSON blob into the argv the way `claude_cli.rs` does inline, which is
     fine for a non-interactive spawn but easier to get wrong when composing
     the argv by hand for an interactive PTY session).
3. Drive the CLI:
   - `pty.fork()`; in the child, `os.execvp("claude", ["claude", "--settings",
     <settings-file-path>])` (interactive mode — no `-p`/headless flags — to
     match "driving `claude` under a real PTY" and put a human-shaped
     `UserPromptSubmit` event on the wire, not the headless stream-json path
     already covered by `260620` Phase 4).
   - In the parent, write a short, single-turn prompt to the master fd
     followed by the terminal's submit sequence (Enter), e.g. a prompt whose
     expected reply is short and unambiguous ("Reply with exactly the word
     done and stop.").
   - Poll-read the master fd in a bounded loop (e.g. up to 90s, 1s ticks) only
     to decide when to stop waiting for the CLI to look done — this bounded
     wait is scaffolding for the experiment's timing budget, not the
     turn-boundary signal itself (do not treat output-idle as the thing being
     measured, per the ticket's explicit anti-pattern warning).
   - Before closing the master fd or killing the child, `open()` and read
     `events.log` from the parent process and capture its full contents. This
     order is load-bearing: it is exactly what made the original `Stop`
     result trustworthy ("observed before session teardown, so it cannot be
     confused with a session-teardown write").
   - Then terminate cleanly (best-effort `SIGTERM` the child, close the master
     fd, `os.waitpid`).
4. CLI invocation budget: exactly ONE `claude` process, one interactive
   session, one short single-turn prompt/reply exchange. That single turn
   necessarily crosses both boundaries under test (`UserPromptSubmit` at
   prompt send, `Stop` at turn end), so no second invocation is needed to
   observe both. This keeps owner credential/credit consumption to the
   minimum the ticket allows ("one short turn is enough").
5. Record the raw `events.log` contents (verbatim lines + timestamps) and the
   verdict (fire / non-fire / inconclusive, see Verification Plan) directly
   into the ticket's Phase 3 step 1 result — per `:440-442`, a negative result
   must be recorded explicitly, not silently dropped. If the verdict is
   non-fire, also append the "drop `working` from the vocabulary" consequence
   named in `## Decisions:187-190` as a forward note on the ticket, since that
   changes what Phases 4-7 and 7's spec impact commit to — but do not edit
   those phases' text in this step; recording the finding is this step's
   whole scope.

## Verification Plan

- **Proof of fire (turn-start hook works):** `events.log`, read from the
  parent BEFORE PTY teardown, contains a `USER_PROMPT_SUBMIT <ts>` line whose
  timestamp is earlier than the `STOP <ts>` line from the same run. Both
  lines present, in that order, is the positive result.
- **Proof of non-fire, distinguished from a broken observation.** This is the
  crux the delegating task called out, and the reason `Stop` is included in
  this run instead of testing `UserPromptSubmit` alone:
  - **Genuine non-fire:** `STOP <ts>` IS present (proving, in this exact run,
    that the PTY driving, `--settings` injection, hook-command execution, and
    log-file artifact path all work end to end — the same mechanism already
    trusted for `Stop`) but no `USER_PROMPT_SUBMIT` line ever appears. Because
    the mechanism is proven live by `Stop`'s own presence, the absence of the
    other line is attributable to the CLI, not the harness. This is the only
    condition under which "drop `working` from the vocabulary" is justified.
  - **Inconclusive (harness failure, not a spike result):** NEITHER line
    appears, or the `claude` process exits with an error, or stderr contains a
    settings-parse/validation complaint (the existing `PreToolUse` precedent
    in `claude_cli.rs` shows malformed hook JSON can silently no-op rather
    than error loudly, so stderr must be captured and checked even on an
    apparently-clean exit). This case must be reported as "harness did not
    reproduce the already-established `Stop` result" and re-run/debugged
    before any claim is made about `UserPromptSubmit` — it must NOT be
    recorded as a negative result for turn-start, since that would silently
    misattribute a broken test as a settled vendor fact (the exact mistake the
    research ticket flagged for the under-tested `Notification` result).
  - Additional corroboration for a genuine non-fire: re-check the bounded wait
    was long enough for the CLI to have visibly produced its reply in the PTY
    output stream before concluding `UserPromptSubmit` truly never fired —
    mirrors the research ticket's own caution that its `Notification` negative
    was weak because the run "never idled long enough to be a real test."
    `UserPromptSubmit` should fire at prompt SEND time (not after a wait), so
    this check is a sanity floor, not the primary evidence.
- **Execution:** run the script manually once (per the one-CLI-invocation
  budget above); this is a spike, not an automated/CI-integrated test — no
  `cargo test`/`npm test`/Playwright wiring, and nothing added to the
  acceptance suite (Phase 2's and Phase 6's explicit rule that the acceptance
  suite must not acquire a real-vendor-CLI dependency applies in spirit here
  even though this step predates those phases).
- Record the verdict and raw log excerpt on the ticket's Phase 3 step 1 result
  before treating this gate as closed.

## Escalations

- None.
