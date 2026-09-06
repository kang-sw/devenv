---
title: Pi lead regains a soft-discouraged one-liner exec tool next to the ugly-named read
related:
  260904-feat-ws-pi-execute-approval-gateway: relaxes its no-direct-exec posture for a bounded one-liner path
  260905-feat-ws-pi-harness-config-layer: sibling Pi-track ticket; no dependency
parent: 260605-epic-ws-playbook-factory-pivot
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 447daa1b31119316
sage-review-completeness-reviewed: 447daa1b31119316
completed: 2026-09-06
---

# Pi lead regains a soft-discouraged one-liner exec tool next to the ugly-named read

## Background

The execute approval gateway (`260904-feat-ws-pi-execute-approval-gateway`)
removed native `bash` from the Pi lead's active tool set and left the lead no
raw exec path at all: every command goes through `ws-execute`, which spawns
an execute-worker, gates each command through `ws-approve`, and returns only
the worker's final report. That is right for anything that mutates the tree or
produces bulk output. It is wrong for the residual case the owner hit while
dogfooding: a genuine one-liner whose short output the lead needs inline right
now (`git rev-parse --abbrev-ref HEAD`, `ls` of one directory, `wc -l` on one
file, a version probe). Routing such a command through `ws-execute` costs a
worker spawn and a pushed report for a result that fits in one line (its
optional `command?` already runs ungated in the lead's process, but the
output goes to the worker, not to the lead), and the lead's guide currently
tells it there is no other way. The read side already solves the same
tension with a soft-discouraged escape hatch (`do-i-really-have-to-read-this-myself`); the exec side has no
equivalent. The gateway ticket's Decision 9 explicitly left "whether the Pi
lead regains a direct exec path" open for a later decision. This ticket is
that decision, scoped as narrowly as the read hatch.

## Decisions

- **One tool, ugly name, same posture as the read hatch.** Add a lead-only
  tool `do-i-really-have-to-run-this-myself` beside the read hatch. Its
  description states the rule in one breath: for a single short command whose
  output you need inline and that changes nothing you would need reviewed;
  anything multi-step, long-running, or mutating goes through `ws-execute`.
  Naming-as-deterrent is the same soft signal the read hatch uses; the
  structural guarantee for mutation stays with the gateway.
- **Bounded by construction, not by convention.** The tool takes one
  `command` string and runs it as `pi.exec("sh", ["-c", command], { cwd,
  timeout })` in the session cwd (the same call the execute-worker path
  uses; it returns `{stdout, stderr, code, killed}`), with a fixed timeout
  (30 s) and a fixed output cap (4 KB, head-truncated, with a trailing line
  saying the rest was dropped and to use `ws-execute` for bulk output). Both
  limits are constants, not parameters, so a lead cannot widen them at the
  call site. Exceeding either is not an error; the lead gets what fit plus
  the hint (and a timeout line when the timeout fired).
- **A stated reason is part of the call.** A required `why` parameter (one
  sentence) is echoed back at the top of the tool result. It costs the lead
  a moment of justification per call, which is the friction the name alone
  does not supply, and it leaves a trace in the transcript for later review
  of whether the hatch is being abused.
- **No approval gate.** The gateway's approval relay works because a parent
  observes a child's tool calls; nothing observes the lead's own calls, so a
  gated lead tool would block forever (the same reason `ws-worker-exec` is
  excluded from the lead's set). The hatch is therefore ungated, and the
  limits above plus the name and `why` are the whole safety story. This is a
  deliberate softening of the gateway ticket's "exec gets hard removal
  because it needs reliability", accepted because the residual case is real
  and the bounded tool cannot carry the bulk-output or multi-step work the
  hard removal was protecting against.
- **Surface.** Added through the same lead tool-surface reshaping step as the
  read hatch (`LEAD_ADDED_TOOL_NAMES` in `execute-gateway.ts`), so it is
  present for the lead and a lateral fork and absent from workers,
  execute-workers, and explore leaves, and it survives `/reload` exactly as
  the read hatch does.
- **Rejected: giving the lead native `bash` back with a prompt-only rule.**
  That is the "convention, not construction" mode the gateway ticket rejected
  and would reopen unbounded output into the lead's context.
- **Rejected: a `quick:true` flag on `ws-execute`.** It would keep the spawn
  and only trim the report; the cost the owner objects to is the detour
  itself.
- **Rejected: returning `ws-execute`'s already-ungated `command?` output to
  the lead inline.** That path exists to seed the worker, and giving one
  tool two result shapes (inline output sometimes, a worker report
  otherwise) hides the hatch behind a flag instead of behind a name and a
  `why`.

## Constraints

- The tool's stdout and stderr are merged and capped before they reach the
  model; the cap is applied to bytes, then trimmed to the last complete line
  inside the cap. When no newline falls inside the cap (one long line), the
  byte-trimmed head is kept at a character boundary instead of dropping to
  nothing.
- The command runs with the session's environment and cwd; the tool does not
  accept a `cwd` or `env` override.
- Beyond the reason line, the exit code, the capped output, the drop hint,
  and the timeout line, the tool result carries nothing (no working context,
  no advice).
- The tool is not exposed by the Pi adapter to any child; the golden rule
  (no ws-mcp change, no `agents-plugin/` change) holds throughout.

## Spec Impact

`pi-adapter-runtime`: amend "Lead native tool-surface reshaping"
(`{#260905-pi-lead-tool-surface-execute-gateway}`) so the reshaped set adds
the one-liner hatch beside the read hatch and names its limits, and rewrite
that same anchor's sentence "the structural 'no raw exec for the lead'
guarantee holds by construction" to "no unbounded exec for the lead"; the
approval-gateway anchor needs no wording change (it never claims "no raw
exec"). Add the verb-table row to `pi-lead-guide.md`.

## Phases

### Phase 1: One-liner exec hatch

In `agents-plugin-pi/src/execute-gateway.ts`: add the tool constant, register
the tool next to the read hatch (`command` and `why` required; `timeout`
and output cap as module constants), append it to `LEAD_ADDED_TOOL_NAMES`,
and implement a pure `capOutput(raw, limitBytes)` helper for the
head-truncate-to-last-line plus hint behavior. Update `pi-lead-guide.md`'s
verb table with one row and update the spec anchor named above. Tests:
the tool appears in `computeLeadActiveTools` output and not in any child tool
group; `capOutput` at the boundary, one byte over, with a multibyte
character straddling the cut, and with a single line longer than the cap
(head kept, not emptied); timeout produces the partial output plus a
timeout line rather than a thrown error; `why` is echoed first. Live check
(owner-run): from a TUI lead, run `git rev-parse --abbrev-ref HEAD` through
the hatch and confirm the one-line result, then run `cat` on a large file and
confirm the cap and the `ws-execute` hint.

### Result (45fc7948) - 2026-09-06

Landed as `4a9eb2cb` (survey plan), `45fc7948` (tool, `capOutput`, tests),
`5f698aa9` (guide row and spec anchor), `7a89bde5` (review relay #1) on the
implementation branch under the goal branch. Adapter-only change.

- `do-i-really-have-to-run-this-myself` is registered beside the read hatch
  in `execute-gateway.ts` and appended to `LEAD_ADDED_TOOL_NAMES`, so it is
  present for the lead and a lateral fork and absent from every child tool
  group. Required `command` and `why`; the `why` line comes first in the
  result. Runs `pi.exec("sh", ["-c", command], { cwd, timeout, signal })`
  with a 30 s timeout and a 4 KB cap as module constants; the pure
  `capOutput` trims to the last complete line inside the byte cap and keeps
  a character-boundary head for a single long line; the drop hint names
  `ws-execute`.
- Relay #1: Pi's `execCommand` sets `killed` for both the timeout and the
  caller's abort signal and coerces a signal-killed exit code to 0, so the
  result now distinguishes "interrupted" from "timed out" via
  `signal?.aborted` and annotates the exit-code line as killed. stdout and
  stderr are merged through `mergeExecOutput` with a newline separator when
  stdout lacks one. Tool description, guide row, and spec now say the
  timeout bounds the direct `sh` child only, not a backgrounded descendant.
- Tests: the ticket's cases (lead surface and child-group absence,
  `capOutput` at the boundary, one over, multibyte straddle, single long
  line, timeout without throwing, `why` first) plus interrupt-wording,
  merge-separator, and constant-value cases. Adapter suite 761 pass, 0 fail.
- Spec: `{#260905-pi-lead-tool-surface-execute-gateway}` lists the hatch
  and its limits and now reads "no unbounded exec for the lead"; id
  unchanged. Guide verb table gained one row.

Review (partitioned correctness/test): test clean with one Minor (comment
off-by-one, fixed); correctness one Important (killed-path misreport) and
five Minor, four fixed in relay #1 and one recorded only (the drop hint is
appended after the cap, so the returned text exceeds the cap by the hint's
length; accepted as intended).

Owner-run live check outstanding: from a TUI lead, run
`git rev-parse --abbrev-ref HEAD` through the hatch, then `cat` a large
file and confirm the cap and the `ws-execute` hint.

## Blocked (2026-09-06) — owner sign-off pending, not a work item

Phase 1 carries a Result; no autonomous work remains. Closing waits on the
owner-run live check above. Once confirmed, close the ticket to `.done/`.


## Resolution (2026-09-06)

Owner-run live check on 2026-09-06 passed: one-line branch output through the hatch, the 4 KB cap with the `use ws-execute for bulk output` hint, the `(timed out after 30s)` line, the `(interrupted before completion)` wording with `exit code: 0 (killed — not a clean exit)`, and both hatches absent from the child tool inventory.
