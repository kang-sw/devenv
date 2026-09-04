---
title: "Epic: MCP tool-surface affordance reduction & surface sanitize"
sage-review-design: completed
related:
  260904-refactor-enter-affordance-rename-route-opaque: child ① (implementation) — enter.* → route.resolve_* rename + full-opaque published params; the actionable layer-① ticket
  260901-research-enter-tool-direct-call-affordance-rename: input to child ① — the settled affordance direction (route.* rename, full-opaque params, collapse rejected) and rationale the impl ticket implements
  260630-epic-skill-playbook-diet: coordinate, not owned here — that epic moves contracts INTO tools (Lever B MCP-ification) and can grow the very tools this epic reshapes; sequence so a tool is not expanded immediately before it is renamed/collapsed
  260901-bug-enter-proceed-misplaced-facts-silent-unknown-status: explicitly OUT of scope — diagnosability hardening, not surface reduction
  260901-bug-ticket-scanner-silently-skips-noncanonical-status-dir: explicitly OUT of scope — scanner robustness, not surface reduction
sage-review-design-reviewed: e632ab15bc085a1a
---

# Epic: MCP tool-surface affordance reduction & surface sanitize

## Summary

The ws MCP surface exposes ~62 tools (68 dotted tools defined counting the
gated `exec.*`/`mercenary.*` families), organized as a flat CRUD-style palette.
Three coupled problems:

1. **Attractiveness (affordance).** Intent-named, richly-documented tools
   (`enter.proceed`, `enter.implement`, and other workflow-entry-shaped tools)
   invite agents to call them **directly, skipping the lead-* skill** that was
   supposed to build their inputs. Recurred across independent downstream Codex
   sessions — a systematic affordance mismatch. The tool surface is competing
   with the skills as the model's mental API.
2. **Weight.** The raw count carries mechanical over-splitting (e.g. `todo.*` is
   a 9-tool ordered-list CRUD) and proven read-surface redundancy
   (`tickets.list`/`find`/`status` return the same `TicketInfo`; likewise
   `specs.*`).
3. **Inconsistent vocabulary (sanitize).** A single "read/query" intent is
   spread across **`list` (×6), `status` (×5), `read` (×3), `find` (×2),
   `search`, `info`, `print`** — 6-7 synonyms. Worse, `status` conflates two
   different meanings: **corpus lookup** (`tickets.status`) and **live state**
   (`git.status`). This trains the wrong mental model and adds grazing noise.

This epic owns shrinking the **count**, the **attractiveness**, and the
**naming/shape consistency** of the surface — a general sanitize, not only a
diet. It does not own moving contracts into tools (that is
`260630-epic-skill-playbook-diet`) nor router diagnosability (the two bug
tickets above).

## Layers

Numeric labels are identities, **not** the execution order (see the organizing
model for the cost-ordered sequence ④ → ③ → ②).

- **① Semantic / affordance (judgment: settled).** The workflow-entry-shaped
  tools that read as user intent. Direction settled (L1 rename to
  `route.resolve_*` + L3 full-opaque published `params`, L2 collapse rejected):
  the rationale lives in research
  `260901-research-enter-tool-direct-call-affordance-rename`, and the actionable
  work is child `260904-refactor-enter-affordance-rename-route-opaque`. Highest
  value — the only layer actively blocking downstream — and design already frozen.
- **② Mechanical over-split → signature merge (JUDGMENT-heavy).** Fold
  over-split families to the operation callers actually compose. Concrete win:
  `todo.append`/`insert_before`/`insert_after` differ only by position anchor →
  one `todo.add(position: end|before|after, ref_key?)` (3→1). The `note.mute`/
  `unmute` → `note.set_visible(bool)` candidate was considered and **dropped**
  (small win; the `mute`/`unmute` pair reads more clearly than a boolean setter),
  so this layer is `todo`-only. This is the one workstream that **designs a novel
  merged signature and reconciles output/anchor semantics** — the genuine
  judgment cost of the epic.
- **③ Read-surface collapse (CLEAN collapse).** `tickets.{list,find,status}`
  and `specs.{list,find,status}` are proven redundant: all three return the
  same `TicketInfo`/spec-metadata struct via the same formatter, and
  `find(ticket_stem=X)` already equals `status(X)` (same `Resolve:true`,
  identical output — verified in `server.go` tickets.find/status handlers). So
  collapse each triple to the survivor: `find()` = list, `find(ticket_stem=X)` =
  status. **6 → 2.** No semantic reconciliation — the survivor already is the
  superset; `status(stem)→find(ticket_stem=stem)` is a fixed param remap. The
  survivor is renamed to `query` as part of ④.
- **④ Verb-vocabulary unification (DETERMINISTIC rename).** Rename the read/query
  surface to a small canonical verb set (below). Once the old→new name map is
  fixed, application is a **scripted substitution + diff/test verification**
  across Go registration/switch, `runtime.json`, specs, playbook tokens, and
  tests — no per-site judgment, only a thin prose-cleanup tail. This is the
  cheapest layer despite touching the most tools (~15-20 names).

## Canonical read-surface verbs (sanitize target)

The key line: **separate corpus-read from live-state**, and give each one verb.

| verb | meaning | targets |
|---|---|---|
| **`query`** | search / resolve / enumerate a **searchable corpus** | tickets, specs (absorb list+status via ③), `note.search→`, `mental_models.find→` |
| **`read`** | fetch **one item's body** by id/key | convention, infra, `todo.read`, `playbook.print→read`, `runtime.info→read` |
| **`list`** | enumerate a **short bounded set** (not a searchable corpus) | agenda, api, config — kept, NOT renamed to query |
| **`status`** | **live** runtime / vcs / process state only | git, `runtime.debug_events`; gated exec/mercenary |

Not every `list` becomes `query`: the line is corpus-vs-bounded-set. `status`
stops being used for corpus point-lookup (that becomes `query`).

**Corpus triples move together.** `mental_models` is a searchable-corpus family
that exposes the same `list`/`find`/`status` triple as tickets/specs, so ④ does
NOT rename `mental_models.find→query` in isolation and leave `mental_models.list`
/`status` behind (that would reproduce the very inconsistency this removes).
Instead, `mental_models` is added to ③'s audit-then-collapse: verify the triple
is a clean superset (as tickets/specs were), then collapse all three into
`mental_models.query`. If the audit finds it is NOT a clean superset, ④ still
aligns the verb (`find→query`) and the residual `list`/`status` are left as an
explicit, noted exception rather than a silent half-rename. Any other family with
a `list`/`find`/`status` corpus triple is treated the same way.

Finalized rename map (evidence in child ④): `tickets.find→tickets.query`,
`specs.find→specs.query`, `note.search→note.query`,
`mental_models.find→mental_models.query`, `playbook.print→playbook.read`,
`runtime.info→runtime.read` (static build metadata, **not** live state — corrects
the earlier `→status` guess; the real live-state member is `runtime.debug_events`,
left as-is). ③'s audit found `mental_models` is **not** a clean superset, so its
`list`/`status` are left as a noted exception (④ verb-aligns `find→query` only),
not collapsed; a genuine `mental_models.query` merge is a deferred follow-up.

## Organizing model — decision vs execution, deterministic vs judgment

The execution cost floor is ws-side playbook re-authoring (a skill-authoring
ceremony per touched skill), so layer-serial passes would re-author the same
playbooks (`lead-proceed`/`lead-implement`) up to 3× — organize on
**decision → execution** instead of layer-serial:

- **Decision phase (freeze the end-state surface).** Freeze the canonical verb
  map (④, nearly done via the table above), the ③ collapse survivors (proven),
  and the ② merged signatures (the genuinely hard design). Difficulty is
  deciding, not editing.
- **Execution phase.** The landing order below is deterministic → judgment; it
  is **not** three re-authoring passes over the same skill. Each ws skill gets at
  most **one** authoring ceremony (per the pen-holder rule); ④ is a scripted
  token substitution, not an authoring pass. Landing order:
  1. **④ rename** — a scripted name-map substitution across the whole tree
     (Go registration/switch, `runtime.json`, specs, playbook **tokens**, tests)
     + diff/test. It touches tokens inside pen-held skills too, but mechanically,
     never as authoring — so it does not count as a rewrite of those skills. Run
     it first to lock the vocabulary so every later layer acts on canonical names.
  2. **③ collapse** — clean removal of the redundant read siblings, folding into
     the now-`query` survivor (for tickets/specs, rename and collapse coincide
     on the same survivor).
  3. **② merge** — the judgment-heavy signature merges last.
  **Owner of a shared-skill rewrite:** the child owning the widest *authoring*
  reshape of a skill holds the pen; others contribute frozen deltas into that one
  pass. The overlap-heavy `lead-proceed`/`lead-implement` are ①'s targets, so ①
  owns their coordinated rewrite; ②/③ fold their deltas to those two skills into
  ①'s pass.
  **① vs ④ ordering (the one real collision):** ④'s scripted substitution edits
  the *tokens* in `lead-proceed`/`lead-implement`; ①'s authoring pass edits their
  *prose/structure*. To avoid re-authoring those two skills twice, run ④'s script
  first (or last) as a mechanical sweep and let ① do its single authoring pass
  against canonical names — ① never re-runs ④'s rename by hand. Default: ④ script
  → ① authoring (stated in the ① impl child).

## Cross-child invariants

- **Rename is scripted, not hand-edited.** ④ applies a fixed old→new name map by
  script across Go registration/switch, `runtime.json`, specs, playbook tokens,
  and tests, verified by diff + tests, with only a thin prose-cleanup tail.
  Hand-editing name-by-name is a smell.
- **Collapse only where redundancy is proven a clean superset.** ③ removes tools
  only where the survivor already returns the removed tool's output (tickets,
  specs — verified). A merely "looks similar" pair is NOT collapsed here; it is
  left, or routed to a fresh audit. Semantic reconciliation (novel merged
  signatures) belongs to ②, not ③.
- **Naming/shape hygiene only; semantics unchanged** except ②'s scoped signature
  merges. No tool changes what it *does*.
- **Exposed surface is the gate.** Gated `exec.*`/`mercenary.*` adopt the same
  canonical vocabulary but are not this epic's landing gate.
- **Authoring is ws-only, ×1.** `agents-plugin-wsflow` assets are mechanically
  mirrored from ws by script; only ws-side playbooks are authored. Hand-editing
  wsflow separately is a smell — fix the mirror.
- **Non-playbook reference surfaces are part of every freeze.** `runtime.json`,
  the MCP/workflow specs, resolver next-instruction text
  (`proceed_resolver.go:355`), and Go tool registration land atomically with the
  rename/collapse.
- **Coordinate with skill-playbook-diet.** Its Lever B pushes more decision logic
  INTO `enter.*`; this epic makes those tools less visible. A tool must not be
  expanded by diet immediately before this epic renames or reshapes it.
- **Deprecation posture — one-shot hard cut (all children).** Old tool names
  have no persisted cross-version consumer: every caller (Go dispatch,
  `runtime.json`, playbook tokens, specs, workflow manual, wsflow mirror) ships
  in-package and is swept atomically by ④'s script, so the rename is a one-shot
  atomic operation, not a compat break — no alias/transition window. Keeping an
  alias would only re-surface dead names in the workflow manual, re-widening the
  very surface this epic cuts. The two out-of-package leaks are the misuse this
  epic removes: downstream sessions' learned `enter.*` direct-call habit
  (softened by the redirect guard in the companion bug ticket, not by an alias)
  and any downstream hardcode of an old name. Children inherit this posture; none
  re-decides it.

## Child tickets

- **①** `260904-refactor-enter-affordance-rename-route-opaque` — at `todo/`
  (design review passed). Implements the affordance rename (`route.resolve_*`) +
  full-opaque params; research
  `260901-research-enter-tool-direct-call-affordance-rename` is its
  settled-direction input (stays at `idea/` as reference).
- **④** `260903-refactor-mcp-verb-vocabulary-unification` — at `todo/` (design
  review passed). Scripted rename to canonical verbs; carries the name map.
- **③** `260903-refactor-mcp-read-surface-collapse` — at `todo/` (design review
  passed). `tickets`/`specs` list+status → query, plus `mental_models` and any
  other corpus triple (audit-gated per the table note); depends on ④'s survivor
  naming.
- **②** `260903-refactor-mcp-todo-signature-merge` — at `todo/` (design review
  passed). `todo` insert-trio → `add`; note candidate dropped; the judgment-heavy
  one.

## Closure conditions

- Workflow-entry-shaped tools no longer read as user intent nor publish a
  self-inviting input schema (① landed).
- The read/query surface uses the canonical verb set; `status` no longer names a
  corpus lookup; `list` survives only for bounded sets (④ landed).
- Proven-redundant read triples are collapsed to their `query` survivor (③
  landed).
- Over-split families are merged to composed operations, or explicitly kept (②
  resolved either way).
- The mirror keeps wsflow in sync with no hand-authored wsflow drift introduced.
