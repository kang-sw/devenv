---
title: Pi bridge's workflow_manual static-body cut never matches a real render, so every lead session degrades to workflow_state with a false "renderer drift" warning
related:
  260904-feat-ws-pi-lead-bootstrap-system-prompt: introduced the cut and deferred its live check
spec:
  - pi-adapter-runtime
---

# Pi bridge's workflow_manual static-body cut never matches a real render, so every lead session degrades to workflow_state with a false "renderer drift" warning

## Background

Owner dogfooding a Pi lead session, 2026-09-06, sees this warning "often":

```text
Warning: ws-pi-bridge: workflow_manual's static manual body no longer matches
the session-start snapshot (renderer drift) — falling back to workflow_state;
per-call advisories are unavailable for the rest of this session
```

It fires from `dispatchMappedWorkflowManual` (`agents-plugin-pi/src/bridge.ts`)
when `cutStaticBody` cannot find the session-start static-body snapshot as an
exact substring of a live `workflow_manual` response. The snapshot is the
`playbook.print("lead-workflow-manual")` render taken right after the
`ferrule` bootstrap; the mapping assumes `workflow_manual` embeds that render
byte-identically and only wraps dynamic material around it.

That assumption has never held. ws-mcp's `handleWorkflowManual`
(`agents-plugin-tool/internal/mcp/workflow_manual.go`, CONTINUE branch)
renders the same playbook and then edits it in place before appending the
dynamic sections:

1. `stripModeGatedRegion(body, false)` removes the `<!-- ws:fresh-only:start
   -->` … `<!-- ws:fresh-only:end -->` region (the `ferrule` instruction),
   marker lines included. `playbook.print` returns the region intact.
2. `injectSessionKeyLine(body, key)` inserts
   `> **Session key: \`<key>\`** — preserve verbatim in any compaction summary.`
   immediately after the `**Session invariant:**` line, in the middle of the
   body's opening blockquote.

Both edits land inside the static body, so `response.indexOf(snapshot)` is
`-1` for every lead session, and the first model-invoked `ws__workflow_manual`
call in a session (typically an entry skill such as `lead-discuss` or
`lead-revive`) takes the fallback: the warning is notified once, the call is
answered by `workflow_state`, and the per-call advisories (`# Manuals`, repo
notes, watermark and doc-coverage warnings, skeptical posture, the unset-tier
advisory's carrier) are lost for the session. Nothing drifted mid-session; the
cut was wrong from its first run.

Why it was not caught: the session-key injection landed 2026-06-30
(`82d399e4`) and the fresh-only strip with the original `workflow_manual`
tool, both long before the mapping (`55f36adb`, 2026-09-04). The mapping's
tests use synthetic fixtures (`"STATIC-BODY\n"`), and the ticket that shipped
it records the "live static-body cut" gate as deferred for lack of provider
credentials. Confirmed in this repository by calling `playbook.print` and
`workflow_manual` under one key and comparing: the print carries the
fresh-only block and no key line; the manual carries the key line and no
fresh-only block.

## Proposed direction

Adapter-only. Replace the exact-substring cut with an **anchor cut** that does
not depend on the body being byte-identical:

- Start anchor: the first non-empty line of the session-start snapshot
  (today `# Workflow Manual`), located in the response. Keeping the anchor
  derived from the `playbook.print` render keeps ws-mcp's render the single
  source of the heading text; the adapter does not hardcode it.
- End anchor: the `## Session Key` heading that ws-mcp appends immediately
  after the body in every CONTINUE response (spec `{#260905-pi-workflow-manual-state-mapping}`
  already names it as the first per-call section). The cut removes
  `[start, end)` and keeps everything before the body (advisory blocks that
  ws-mcp prepends) and everything from `## Session Key` on.
- Fallback stays: when either anchor is missing, or the end anchor precedes
  the start anchor, the bridge dispatches `workflow_state` as today. The
  warning text drops "renderer drift" as the stated cause and says which
  anchor was missing, since the fallback is now the genuine drift signal.
- `cutStaticBody(response, snapshot)` keeps its signature and purity; only its
  matching rule changes. `notifyMappingDegraded` gains the missing-anchor
  reason as an argument.
- Rejected: mirroring ws-mcp's two edits onto the snapshot before matching.
  That couples the adapter to Go internals (the key-line wording, the marker
  names, the insertion point) and breaks again on the next such edit; the
  anchor cut survives any change confined to the body.
- Rejected: dropping the `playbook.print` fetch and hardcoding the heading.
  The fetch is one round-trip per session and keeps the heading single-sourced.

## Spec Impact

`pi-adapter-runtime` `{#260905-pi-workflow-manual-state-mapping}`: the
Primary bullet's "exact substring match against a static-body snapshot" becomes
the anchor cut (snapshot first line to `## Session Key`); the Fallback bullet's
trigger becomes a missing anchor rather than "the snapshot body is not found".

## Constraints

- Adapter-only change in `agents-plugin-pi/`; no ws-mcp change.
- The system-prompt ws block (`{#260905-pi-lead-bootstrap-system-prompt}`) is
  unaffected: it carries the full `workflow_manual` response, not the snapshot.
- Worker/explore forwarding and the no-snapshot degraded path stay as they are.

## Phases

### Phase 1: Anchor cut and real-shaped fixtures

Change `cutStaticBody` to the anchor rule, thread the missing-anchor reason
into the warning, and replace the synthetic fixtures with a real-shaped pair: a
snapshot containing the fresh-only region and the `**Session invariant:**`
blockquote, and a response with that region stripped, the session-key line
injected, advisory blocks prepended, and `## Session Key` plus state appended.
Tests: the real-shaped pair cuts to exactly the prepended advisories plus the
sections from `## Session Key` on; a response lacking `## Session Key` falls
back with the end-anchor reason; a response lacking the heading line falls back
with the start-anchor reason; the existing fallback-dispatch and advisory-keying
tests keep passing. Amend the spec passage under Spec Impact. Live check
(owner-run): in a Pi lead session, call an entry skill that invokes
`workflow_manual` and confirm no warning, a response opening with the fixed
mapping line followed by the advisories and `## Session Key`, and no manual
body.
