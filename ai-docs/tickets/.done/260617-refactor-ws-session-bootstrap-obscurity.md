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
blocks a non-lead key from calling `ws.lead.*`, but a keyless caller can re-login
and re-escalate — an accepted soft guard, not a hard barrier (spec
`mcp-tools.md`; recursion containment in the native-subagent pivot is soft by
design). Native subagents share the lead's single MCP connection, so there is no
per-agent `tools/list`; hiding the tool is not viable — it would hide it from the
lead too.

A plausible, function-descriptive rename (`attach`, `bind`, `hello`, ...) does
not help: any name that reads as a session-start action invites every agent —
including a subagent that already holds a prompt-supplied key — to call it. The
pull is semantic, so a meaningful name cannot remove it.

The lever that works is an **arbitrary, semantically-disconnected name** whose
only binding to "this is the bootstrap call" lives in `ws:workflow-manual`. This
rests on a property of the workflow: leads start through directly-runnable entry
skills (`lead-discuss`, `lead-proceed`, `lead-bootstrap`, ...), and
`workflow-manual` is the near-only source exposed lead-only (soft, via those
skills; subagents receive rendered delegate prompts instead). An arbitrary name
known only through the manual is therefore invisible-by-meaning to anyone who did
not enter through a lead skill. The value claim is risk reduction by a large
factor at low cost, not a new security guarantee.

## Decisions

- **Soft guard only.** This does not prevent escalation (a name-aware caller can
  still keyless-re-login). It removes the semantic invitation that makes
  accidental/curious invocation likely. The hard authority boundary remains the
  keyed tools/call gate.
- **Keep the bootstrap tool callable; do not remove the verb.** A callable
  bootstrap preserves the lead recovery path (re-bootstrap after key loss or
  process restart). Removing the callable mint surface entirely (delivering the
  lead key only through the manual render channel) was considered and left out of
  scope here.
- **Rename to `ws.ferrule`** — an arbitrary, semantically-disconnected name, not
  a function-descriptive one. Function-style names (`attach`/`bind`/`hello`/
  `preflight`) are rejected: they re-create the session-start pull for every
  agent. Among arbitrary names, a bland/obscure noun is preferred over a famous
  one (e.g. `james.bond`): both carry zero session-semantic hook, but a famous
  name attracts curiosity ("what is this?") — exactly the accidental invocation
  this ticket aims to reduce. The lead learns the name from `workflow-manual`, so
  memorability is not a requirement.
- **Inert tool description too.** A `tools/list` entry is name + description, so
  an arbitrary name with a function-revealing description still leaks. The
  description must be inert (e.g., "Reserved workflow primitive; see
  workflow-manual"), so the name↔function mapping exists only in the manual.
- **3-surface scrub** — the name and its meaning must not leak through any
  subagent-reachable surface:
  1. `tools/list` — arbitrary name + inert description.
  2. Error-guidance strings: `unknown_session` / `mandatory_session_key`
     currently embed the literal recovery call; change to a role-agnostic,
     name-free hint (e.g., "if you are the lead, re-bootstrap per
     workflow-manual").
  3. Delegate prompt negative instruction (currently "Do not call
     `ws.lead.login` ..."), which itself signposts the name; drop it or rephrase
     to a capability-level instruction with no literal tool name.
- **Every lead entry skill must route to `workflow-manual`** so the arbitrary
  name is consistently taught; otherwise a lead entering through a manual-less
  skill cannot bootstrap. Found during dogfood: `lead-discuss`, `lead-proceed`,
  `lead-sprint`, and `lead-salvage` route to `workflow-manual`, but
  `lead-bootstrap` does not (it routes to `ai-docs/WORKFLOW.md`, a different
  document). Closing this gap is a precondition of the rename.
- **Lower-priority surface.** Lead-facing docs (spec, mental-model, skill text)
  also name the tool; not subagent runtime-reachable, but full consistency means
  the rename touches them too.

## Phases

### Phase 1: route every lead entry skill to workflow-manual

Audit the directly-runnable lead entry skills and ensure each routes to
`ws:workflow-manual` (the lead-only teaching surface for the bootstrap name).
Close the confirmed gap: `lead-bootstrap` does not currently expose
`workflow-manual`. Verification: each lead entry skill surfaces the manual; a
lead starting from any entry skill can learn the bootstrap call.

### Result (6762aaf5) - 2026-06-19

Resolved without editing `lead-bootstrap`. On closer reading the earlier
characterization was imprecise: `lead-bootstrap` does not "route to
`WORKFLOW.md`" as a manual — it reads `WORKFLOW.md` as a *template source* to
copy into a downstream project. It is a one-time project-scaffolding skill, not
a work-session entry like `lead-discuss`/`lead-proceed`, and it loads no
orchestration primitives by design, so the absence of manual routing is correct
for its purpose. The discoverability goal is instead met by the Phase 2 scrub:
the `mandatory_session_key` / `unknown_session` error guidance now routes any
lead that attempts a root-aware call without a key to `ws:workflow-manual`. The
other four lead entry skills already route to the manual. A skill behavior
change of questionable necessity ("Ask first" surface) was therefore avoided;
the precondition's intent (a lead can learn the obscure bootstrap name) holds.

### Phase 2: rename the bootstrap tool to an arbitrary name and scrub surfaces

Depends on Phase 1. Rename `ws.lead.login` to the chosen arbitrary,
semantically-disconnected name, give it an inert description, and apply the
3-surface scrub. Update `workflow-manual` to teach the name and update wsflow
product-mode bootstrap to use it. Verification: the lead bootstrap path works end
to end under the new name; the old name and meaning no longer appear in
`tools/list` (name or description), error-guidance strings, or rendered delegate
prompts; a delegate that hits `unknown_session` receives a name-free recovery
hint.

### Result (6762aaf5) - 2026-06-19

Renamed `ws.lead.login` -> `ws.ferrule` and applied the 3-surface scrub.

- **Gate:** `bootstrapToolName` const + `isLeadOnlyTool()`. The tool left the
  `ws.lead.*` prefix, so the non-lead escalation block now matches it by
  explicit name (the prefix still covers `ws.lead.prefer_mercenary`). This was
  the load-bearing landmine — a prefix-only check would have silently stopped
  blocking the bootstrap verb.
- **Surface 1 (tools/list):** inert description — "Reserved workflow primitive.
  See ws:workflow-manual." Also scrubbed the broadly-visible `session_key`
  property description shared by every root-aware tool.
- **Surface 2 (error guidance):** `mandatory_session_key`, `unknown_session`,
  and the legacy `session.*` errors now name no tool and route the lead to
  `ws:workflow-manual`.
- **Surface 3 (delegate cred block):** capability-level instruction with no tool
  name ("already authenticated; do not attempt to mint, log in, or escalate").
- **Teaching:** `workflow-manual` (both copies) teaches `ws.ferrule` and why the
  name is obscure. `runtime.json` (both), rsrc manifests (both), and the wsflow
  rsrc mirror regenerated. spec `mcp-tools.md` + mental-model `mcp-runtime.md`
  renamed for consistency and given the obscurity/scrub invariant (renamed, not
  made name-free: lower-priority non-runtime surface per `## Decisions`).
- **Verification:** `go build ./...`, `go vet`, and the full repo test suite are
  green. The old name no longer appears in `tools/list`, error strings, or
  rendered delegate prompts; tests assert the recovery text does not leak the
  name and routes to the manual. Ticket-body references to the old name in
  decision-history records (`.done/`, `.plans/`, epic/research tickets) were
  intentionally left.
