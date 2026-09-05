---
title: Pi lead regains a soft-discouraged one-liner exec tool next to the ugly-named read
related:
  260904-feat-ws-pi-execute-approval-gateway: relaxes its "no raw exec at all" for a bounded one-liner path
  260905-feat-ws-pi-harness-config-layer: sibling Pi-track ticket; no dependency
parent: 260605-epic-ws-playbook-factory-pivot
spec:
  - pi-adapter-runtime
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
worker spawn, an approval round trip, and a pushed report for a result that
fits in one line, and the lead's guide currently tells it there is no other
way. The read side already solves the same tension with a soft-discouraged
escape hatch (`do-i-really-have-to-read-this-myself`); the exec side has no
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
  `command` string and runs it through the session shell in the session cwd
  with a fixed timeout (30 s) and a fixed output cap (4 KB, head-truncated,
  with a trailing line saying the rest was dropped and to use `ws-execute`
  for bulk output). Both limits are constants, not parameters, so a lead
  cannot widen them at the call site. Exceeding either is not an error; the
  lead gets what fit plus the hint.
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
  and the approval round trip and only trim the report; the cost the owner
  objects to is the round trip itself.

## Constraints

- The tool's stdout and stderr are merged and capped before they reach the
  model; the cap is applied to bytes, then trimmed to the last complete line.
- The command runs with the session's environment and cwd; the tool does not
  accept a `cwd` or `env` override.
- The tool result never carries more than the reason line, the exit code, and
  the capped output.
- The tool is not exposed by the Pi adapter to any child; the golden rule
  (no ws-mcp change, no `agents-plugin/` change) holds throughout.

## Spec Impact

`pi-adapter-runtime`: amend "Lead native tool-surface reshaping" so the
reshaped set adds the one-liner hatch beside the read hatch and names its
limits; amend the approval-gateway anchor's "no raw exec at all" sentence to
"no unbounded exec"; add the verb-table row to `pi-lead-guide.md`.

## Phases

### Phase 1: One-liner exec hatch

In `agents-plugin-pi/src/execute-gateway.ts`: add the tool constant, register
the tool next to the read hatch (`command` and `why` required; `timeout`
and output cap as module constants), append it to `LEAD_ADDED_TOOL_NAMES`,
and implement a pure `capOutput(raw, limitBytes)` helper for the
head-truncate-to-last-line plus hint behavior. Update `pi-lead-guide.md`'s
verb table with one row and update the two spec anchors named above. Tests:
the tool appears in `computeLeadActiveTools` output and not in any child tool
group; `capOutput` at the boundary, one byte over, and with a multibyte
character straddling the cut; timeout produces the partial output plus a
timeout line rather than a thrown error; `why` is echoed first. Live check
(owner-run): from a TUI lead, run `git rev-parse --abbrev-ref HEAD` through
the hatch and confirm the one-line result, then run `cat` on a large file and
confirm the cap and the `ws-execute` hint.
