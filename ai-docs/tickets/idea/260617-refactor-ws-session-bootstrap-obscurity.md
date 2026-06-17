---
title: reduce subagent discovery of the session-bootstrap tool
related:
  260617-refactor-mcp-stateless-subagent-context: same lead/delegate session model; storage substrate there vs bootstrap-tool discovery obscurity here are independent levers
  260605-research-ws-native-subagent-pivot: recursion containment is a soft guard by design; this lowers accidental discovery of the bootstrap tool
related-mental-model:
  - mcp-runtime
---

# reduce subagent discovery of the session-bootstrap tool

## Background

`ws.lead.login` is the session-key bootstrap tool. The keyed tools/call gate
already blocks a non-lead key from calling `ws.lead.*`, but a keyless caller can
re-login and re-escalate — this is an accepted soft guard, not a hard barrier
(spec `mcp-tools.md`; recursion containment in the native-subagent pivot is soft
by design). There is no per-agent MCP tool visibility for native subagents
because they share the lead's single MCP connection, so hiding the tool from
`tools/list` is not viable — it would hide it from the lead too.

The realistic lever is to make the bootstrap tool *less tempting to discover and
call* for a subagent, without changing the hard authority boundary. Removing an
obviously-named escalation option is expected to cut accidental and curious
invocation substantially even though it cannot stop a determined or name-aware
caller. The value claim is risk reduction by a large factor at low cost, not a
new security guarantee.

## Decisions

- **Soft guard only.** This does not prevent escalation (a name-aware caller can
  still keyless-re-login). It reduces accidental and curious discovery. The hard
  authority boundary remains the keyed tools/call gate.
- **Rename the bootstrap tool to a flat, non-attention-grabbing plumbing name.**
  Avoid `internal`/`admin`/`auth`/`login`/`session` tokens — those attract
  attention. The lead learns the name from `ws:workflow-manual`. The exact name
  is an open decision (still under discussion).
- **The rename only works if the name is scrubbed from every subagent-reachable
  surface (3-surface scrub):**
  1. `tools/list` advertised name — the rename itself.
  2. Error-guidance strings: `unknown_session` / `mandatory_session_key`
     currently embed the literal recovery call. Change to a role-agnostic,
     name-free hint (e.g., "if you are the lead, re-bootstrap per
     workflow-manual").
  3. Delegate prompt negative instruction: the rendered prompt currently says
     "Do not call `ws.lead.login` ...", which itself signposts the name. Drop it
     or rephrase to a capability-level instruction with no literal tool name.
- **Lower-priority surface.** Lead-facing docs (spec, mental-model, skill text)
  also name the tool; they are not subagent runtime-reachable, but full
  consistency means the rename touches them too.

## Phases

### Phase 1: rename the bootstrap tool and scrub discovery surfaces

Rename `ws.lead.login` to the chosen plumbing name and apply the 3-surface
scrub. Update `ws:workflow-manual` so the lead bootstrap path still works, and
update wsflow product-mode bootstrap to the new name. Verification: the lead
bootstrap path works end to end under the new name; the old name no longer
appears in `tools/list`, error-guidance strings, or rendered delegate prompts; a
delegate that hits `unknown_session` receives a name-free recovery hint.
