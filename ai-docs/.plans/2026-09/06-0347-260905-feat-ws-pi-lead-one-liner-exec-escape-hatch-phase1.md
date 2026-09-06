# Plan: 260905-feat-ws-pi-lead-one-liner-exec-escape-hatch — Phase 1: One-liner exec hatch

## Relevant Ticket Contract

- Add a lead-only tool `do-i-really-have-to-run-this-myself`, beside the read
  hatch, same soft-discouraged-by-name posture. Description states in one
  breath: single short command, output needed inline, changes nothing that
  needs review; anything multi-step/long-running/mutating goes through
  `ws-execute`.
- Bounded by construction: one `command` string, run as `pi.exec("sh", ["-c",
  command], { cwd, timeout })` in the session cwd — the same call shape the
  execute-worker path uses, returning `{stdout, stderr, code, killed}`. Fixed
  timeout 30s and fixed output cap 4 KB (head-truncated to the last complete
  line, with a trailing drop-hint pointing at `ws-execute`), both module
  constants — never caller-supplied. Exceeding either is not an error: the
  lead gets what fit plus the hint, and a timeout line when the timeout fired.
- `why` (one sentence) is required and echoed first in the tool result.
- No approval gate — same rationale as `GATED_EXEC_TOOL_NAME`'s exclusion from
  the lead's own active set: nothing observes the lead's own tool calls, so a
  gated lead tool would hang forever.
- Surface: added through the same `LEAD_ADDED_TOOL_NAMES` reshaping step as
  the read hatch, present for lead + fork, absent from workers/execute-workers
  /explore, survives `/reload` the same way.
- Constraints: stdout+stderr merged and capped before reaching the model; cap
  applied to bytes, then trimmed to the last complete line inside the cap;
  when no newline falls inside the cap (one long line), the byte-trimmed head
  is kept at a character boundary instead of being dropped to nothing. No
  `cwd`/`env` override param — always the session's own cwd/env. Result
  carries only: reason line, exit code, capped output, drop hint, timeout line
  — nothing else (no working context, no advice).
- Golden rule holds: all code in `agents-plugin-pi/`; no `agents-plugin-tool/`
  or `agents-plugin/skills/` change.
- Spec impact (this phase): amend `pi-adapter-runtime.md`'s
  `{#260905-pi-lead-tool-surface-execute-gateway}` anchor — add the one-liner
  hatch beside the read hatch (name its limits), and reword that anchor's
  sentence "the structural \"no raw exec for the lead\" guarantee holds by
  construction" to say "no unbounded exec for the lead" instead. The
  approval-gateway anchor needs no change (it never claims "no raw exec").
- Doc impact (this phase): add one verb-table row to `pi-lead-guide.md`.
- Phase 1's own listed tests (verbatim scope, carry all of it): tool appears
  in `computeLeadActiveTools` output and not in any child tool group;
  `capOutput` at the boundary, one byte over, with a multibyte character
  straddling the cut, and with a single line longer than the cap (head kept,
  not emptied); timeout produces the partial output plus a timeout line
  rather than a thrown error; `why` is echoed first.
- Phase 1's live check is explicitly owner-run (TUI-only), not part of this
  phase's automated verification.

## Out of Scope

- The gateway's approval/`ws-execute`/`ws-approve` machinery itself — already
  landed (`260904-feat-ws-pi-execute-approval-gateway`); this phase only adds
  a sibling lead tool beside it, does not touch `ws-worker-exec`,
  `ws-execute`, or `ws-approve` bodies.
- `260905-feat-ws-pi-harness-config-layer` — sibling ticket, no dependency,
  not touched here.
- Any relaxation of the "no approval gate" decision, any `cwd`/`env`
  parameter, any caller-tunable timeout/cap — all explicitly rejected by the
  ticket's Decisions section; not a survey judgment call.
- The live TUI check (`git rev-parse --abbrev-ref HEAD` through the hatch,
  then `cat` on a large file) is owner-run per the ticket text, not part of
  this plan's automated verification.

## Codebase Findings

### Registration site and existing sibling pattern

- `agents-plugin-pi/src/execute-gateway.ts#L122` — `UGLY_READ_TOOL_NAME =
  "do-i-really-have-to-read-this-myself"`: sibling constant to mirror for the
  new `ONE_LINER_EXEC_TOOL_NAME = "do-i-really-have-to-run-this-myself"`.
- `agents-plugin-pi/src/execute-gateway.ts#L201-L202` —
  `LEAD_REMOVED_TOOL_NAMES`/`LEAD_ADDED_TOOL_NAMES`. Only `LEAD_ADDED_TOOL_NAMES`
  needs the new name appended (`[EXECUTE_TOOL_NAME, APPROVE_TOOL_NAME,
  UGLY_READ_TOOL_NAME, ONE_LINER_EXEC_TOOL_NAME]`); `computeLeadActiveTools`
  itself (`#L223-L232`) needs no logic change — it already generically adds
  every name in that list.
- `agents-plugin-pi/src/execute-gateway.ts#L633-L653` — `UGLY_READ_TOOL_NAME`'s
  `pi.registerTool({...})` block is the direct structural template: same
  `label`, plain `description` stating the fallback posture, `parameters` cast
  `as never`, `async execute(_toolCallId, params) { ... }` reading
  `sessionCtx.cwd`. The new tool's `execute()` follows the same shape but
  calls `pi.exec` instead of `readFileSync`.
- `agents-plugin-pi/src/execute-gateway.ts#L497` (`registerExecuteGateway`)
  and its doc comment `#L483-L496` — register the new tool inside this same
  function, alongside the other four; update the doc comment's "four tools"
  list to five (minor, keeps the doc from drifting on contact).
- `agents-plugin-pi/src/execute-gateway.ts#L1-L90` (module header) —
  documents every registered tool; the `UGLY_READ_TOOL_NAME` bullet
  (`#L72-L76`) is the template for a new bullet describing the one-liner
  hatch (name, cap, timeout, no-gate rationale) — optional but keeps the
  header from going stale, same "surgical, not gold-plated" bar as the rest
  of this phase.

### `pi.exec` call shape and result (confirms the ticket's own decision text)

- `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/core/extensions/types.d.ts#L993`
  — `exec(command: string, args: string[], options?: ExecOptions):
  Promise<ExecResult>` on `ExtensionAPI`.
- `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/core/exec.d.ts#L7-L23`
  — `ExecOptions { signal?, timeout?, cwd? }`; `ExecResult { stdout, stderr,
  code, killed }`. `timeout` is a first-class option — no manual timer is
  needed; pass `timeout: ONE_LINER_TIMEOUT_MS` directly.
- `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/core/exec.js`
  (underlying `execCommand`) — on timeout it `SIGTERM`s (then `SIGKILL`s
  after 5s) and resolves (never rejects) with `killed: true` and whatever
  `stdout`/`stderr` had accumulated by then, `code` from the process's actual
  exit. Confirms "exceeding the timeout is not an error" needs no try/catch —
  `pi.exec` never throws for a timeout or a non-zero exit.
- `agents-plugin-pi/src/execute-gateway.ts#L529-L531` and `#L564-L566` — the
  two existing `pi.exec("sh", ["-c", ...], { cwd, ... })` call sites
  (`ws-worker-exec`'s and `ws-execute`'s own verbatim-`command` pre-run).
  `#L566`'s `output = \`${execResult.stdout}${execResult.stderr}\`;` is the
  exact merge convention to reuse for the new tool's stdout+stderr merge
  (same concatenation order, no separator).

### `capOutput` — no existing byte-cap-to-last-line helper; nearest reusable pattern

- `agents-plugin-pi/src/spawner.ts#L1066-L1072`
  (`truncatePromptForStorage`) — byte-safe head-truncation precedent: `const
  buf = Buffer.from(text, "utf8"); ... const decoder = new
  StringDecoder("utf8"); const head = decoder.write(buf.subarray(0,
  capBytes));`. This is the multibyte-safe cutting technique to reuse
  (`node:string_decoder` import, already used at `spawner.ts#L79`) — but it
  only guarantees a character-boundary-safe cut, NOT "trim to the last
  complete line inside the cap," which this ticket additionally requires.
  `capOutput` is new logic, not a call-through to `truncatePromptForStorage`
  (different hint text, different line-trim requirement, and reuse across
  files here would be a cross-cutting refactor the ticket does not ask for).
- Suggested `capOutput(raw: string, limitBytes: number): string` behavior:
  byte-decode-safe cut to `limitBytes` (as above) when `Buffer.byteLength(raw,
  "utf8") > limitBytes`; if the decoded head contains a `\n`, cut to the last
  one (drop the trailing partial line) and append a drop-hint line naming
  `ws-execute` for bulk output; if it contains no `\n` at all (one long line
  longer than the cap), keep the whole character-boundary-safe head as-is
  (per the ticket's own explicit "kept... instead of dropping to nothing")
  and still append the same drop-hint. Return `raw` unchanged, no hint, when
  it already fits.
- Test boundary cases to hit directly (ticket's own list): exactly at
  `limitBytes` (no truncation, no hint); one byte over (truncated, hint
  appended); a multibyte character (e.g. a 4-byte emoji) whose byte offset
  straddles the cap (no corruption/replacement-character, per the same
  assertion style as `spawner.test.ts#L2498-L2507`'s
  `truncatePromptForStorage` multibyte test); a single line far longer than
  the cap with no newline inside it (head kept, not emptied — the case the
  ticket explicitly calls out).

### `computeLeadActiveTools` test to extend, and confirming no child ever sees the new tool

- `agents-plugin-pi/test/execute-gateway.test.ts#L129-L160` — the existing
  `describe("computeLeadActiveTools", ...)` block. Extend the first test's
  assertions (`#L130-L141`) with `assert.ok(result.includes(ONE_LINER_EXEC_TOOL_NAME))`,
  and the "empty list" test (`#L157-L159`) to include the new name in its
  expected 4-member (not 3) sorted array. No new `describe` block is required
  — same fixture, same list.
- `agents-plugin-pi/src/spawner.ts#L183-L188` — `TOOL_GROUPS` is the sole
  source of what a spawned child's `--tools` CLI flag contains
  (`read-only`/`recon`/`full-worker`/`execute-worker`); none of the four
  arrays name the new tool, and nothing in this phase adds it to any of
  them. This is the mechanism (not `computeLeadActiveTools`, which only
  reshapes the LEAD's own live session) that keeps the hatch unreachable from
  workers/execute-workers/explore — a worker process never sees a tool
  outside its spawn-time `--tools` list, regardless of what is registered
  globally. A direct assertion of "not in any child tool group" is a plain
  loop over `Object.values(TOOL_GROUPS)` asserting none contains
  `ONE_LINER_EXEC_TOOL_NAME`.

### `pi-lead-guide.md` verb table

- `agents-plugin-pi/pi-lead-guide.md#L31-L50` — the verb-routing table.
  Row `#L50` (the read hatch) is the template row shape and tone
  ("`do-i-really-have-to-read-this-myself` (`path`, optional
  `offset`/`limit`)... the name is the point... Prefer X for anything wider").
  Add one new row directly after it (new last row) for the exec hatch, naming
  `command`/`why`, the fixed 30s/4KB limits, and pointing at `ws-execute` for
  anything wider.

### Spec anchor to amend

- `ai-docs/spec/pi-adapter-runtime.md#L724-L751` — the
  `{#260905-pi-lead-tool-surface-execute-gateway}` section.
  `#L726` is the exact sentence to reword: `So the structural "no raw exec for
  the lead" guarantee holds by construction and not merely by prompt
  convention, the adapter reshapes...` -> replace `"no raw exec for the
  lead"` with `"no unbounded exec for the lead"` (keep the rest of the
  sentence structure; this phase now DOES give the lead a bounded direct exec
  path, so "no raw exec" is no longer literally true). `#L729-L732`'s
  sentence listing what gets added (`ws-execute`, `ws-approve`, the ugly-read
  tool) needs the one-liner hatch added to that same list, with its fixed
  30s timeout / 4KB cap named inline (mirroring how the read hatch's name is
  already spelled out there). The `> [!note] Implementation Gap` block
  (`#L741-L751`, about `/reload` durability) is about the existing reshaping
  mechanism in general and needs no ticket-specific edit — the new tool rides
  the same `LEAD_ADDED_TOOL_NAMES` mechanism it already covers.

## Implementation Plan

1. `agents-plugin-pi/src/execute-gateway.ts`:
   - Add `export const ONE_LINER_EXEC_TOOL_NAME =
     "do-i-really-have-to-run-this-myself";` near `UGLY_READ_TOOL_NAME`
     (`#L122`).
   - Add two module constants near the top of the pure-helpers section:
     timeout (`ONE_LINER_TIMEOUT_MS = 30_000`) and output cap
     (`ONE_LINER_OUTPUT_CAP_BYTES = 4096`), both plain numbers, never wired to
     any tool parameter.
   - Implement pure `export function capOutput(raw: string, limitBytes:
     number): string` per the behavior in Codebase Findings above, using
     `Buffer.from(raw, "utf8")` + `node:string_decoder`'s `StringDecoder`
     (already imported in `spawner.ts`; add the same import to
     `execute-gateway.ts`) for the multibyte-safe cut, then a
     `lastIndexOf("\n")` search on the decoded head for the last-complete-line
     trim, falling back to the full decoded head when no newline is found.
     Append a single trailing hint line naming `ws-execute` for bulk output
     whenever truncation happened (either branch).
   - Append `ONE_LINER_EXEC_TOOL_NAME` to `LEAD_ADDED_TOOL_NAMES` (`#L202`).
   - Inside `registerExecuteGateway` (`#L497`), add a fifth
     `pi.registerTool({...})` block (after the `UGLY_READ_TOOL_NAME` block,
     `#L633-L653`): `name: ONE_LINER_EXEC_TOOL_NAME`, `parameters: { command:
     string, why: string }` both required, description stating the ticket's
     one-breath rule ("single short command whose output you need inline and
     that changes nothing you would need reviewed; anything multi-step,
     long-running, or mutating goes through ws-execute") and naming the fixed
     30s/4KB limits so the model doesn't have to guess them. `execute()`
     calls `await pi.exec("sh", ["-c", p.command], { cwd: sessionCtx.cwd,
     timeout: ONE_LINER_TIMEOUT_MS, signal })`, merges
     `\`${execResult.stdout}${execResult.stderr}\`` (same order as `#L566`),
     runs it through `capOutput(merged, ONE_LINER_OUTPUT_CAP_BYTES)`, and
     assembles the result text in the constraint's own stated order: `why:
     ${p.why}`, `exit code: ${execResult.code}`, the capped output (which
     already carries its own drop-hint line when truncated), then one
     trailing timeout line (`(timed out after 30s)` or similar) appended only
     when `execResult.killed` is true. No `cwd`/`env` param accepted.
   - Update the `registerExecuteGateway` doc comment (`#L483-L496`) and the
     module header's tool inventory (`#L72-L76` area) to mention the fifth
     tool — small doc-drift fix, not a design change.
2. `agents-plugin-pi/pi-lead-guide.md`: add one verb-table row after line 50
   naming `do-i-really-have-to-run-this-myself` (`command`, `why`), the fixed
   30s timeout and 4KB output cap, and "anything wider goes through
   `ws-execute`."
3. `ai-docs/spec/pi-adapter-runtime.md`: in the
   `{#260905-pi-lead-tool-surface-execute-gateway}` section, reword line 726's
   `"no raw exec for the lead"` to `"no unbounded exec for the lead"`, and
   extend the `#L729-L732` sentence listing added tools to also name the new
   one-liner hatch and its fixed limits.
4. `agents-plugin-pi/test/execute-gateway.test.ts`:
   - Import `ONE_LINER_EXEC_TOOL_NAME`, `capOutput` (and
     `ONE_LINER_TIMEOUT_MS`/`ONE_LINER_OUTPUT_CAP_BYTES` if exported) from
     `../src/execute-gateway.ts`.
   - Extend the existing `computeLeadActiveTools` tests (`#L129-L160`) to
     assert the new name is added and never duplicated, and update the
     empty-list expectation to the 4-member set.
   - Add a plain assertion that no `TOOL_GROUPS` entry
     (`agents-plugin-pi/src/spawner.ts#L183-L188`) contains
     `ONE_LINER_EXEC_TOOL_NAME` — satisfies "not in any child tool group"
     directly against the mechanism that actually enforces it.
   - Add a new `describe("capOutput", ...)` block covering: under/at the cap
     (unchanged, no hint); one byte over (truncated, hint present); a
     multibyte character straddling the cut (no corruption, same assertion
     style as `spawner.test.ts#L2498-L2507`); a single line longer than the
     cap with no newline at all (non-empty head kept, hint present).
   - Add a `describe` for the new tool's `execute()` body itself, following
     the `fakePi()` convention already established at
     `execute-gateway.test.ts#L364-L378` (a plain object stubbing only the
     methods used, cast `as unknown as ExtensionAPI`) — here stubbing
     `pi.exec` instead of `pi.sendMessage`. This tool's `execute()` has no
     `RpcClient`/registry/filesystem-approval dependency (unlike
     `ws-worker-exec`/`ws-execute`/`ws-approve`), so, unlike those, it is
     directly unit-testable with a stub `pi.exec`:
     - A stub resolving `{stdout: "...", stderr: "", code: 0, killed:
       false}` to assert `why` appears first in the result text, and the
       exit code/output follow.
     - A stub resolving `{stdout: "partial", stderr: "", code: 143, killed:
       true}` to assert the result includes the partial output plus a
       timeout line, and that `execute()` resolves normally (never throws)
       — covering the ticket's "timeout produces the partial output plus a
       timeout line rather than a thrown error" test directly, without
       needing a live 30-second wait.
   - This keeps the suite's documented pure/IO split intact:
     `capOutput` is pure (no stub needed), while the tool's `execute()` is
     IO-shaped but — unlike the gateway's other three tool bodies — cheaply
     stubbable, so it does not need to be deferred to the live-gate-only
     bucket the file's header comment already documents for
     `ws-worker-exec`/`ws-execute`/`ws-approve`/the read hatch.

## Verification Plan

- `cd agents-plugin-pi && npm test` (baseline: 745 tests / 146 suites passing
  before this change; expect only additions, none of the existing 745
  touched).
- `node --check` (or the project's existing type-check step, if any) on
  `agents-plugin-pi/src/execute-gateway.ts` and
  `agents-plugin-pi/test/execute-gateway.test.ts`.
- Manual/live gate (owner-run, per the ticket's own Phase 1 text — not part
  of this plan's automated verification): from a TUI lead, run `git
  rev-parse --abbrev-ref HEAD` through the hatch and confirm a one-line
  result; then run `cat` on a large file through the hatch and confirm the
  4KB cap and the `ws-execute` drop-hint appear.

## Escalations

- None.
