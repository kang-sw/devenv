---
title: Retarget CLAUDE_TICKETS_WORKFLOW doc to Claude-only + add one-time bootstrap companion
related:
  260918-feat-downstream-tickets-workflow-doc: predecessor
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 62604d51fd93570e
sage-review-completeness-reviewed: 62604d51fd93570e
---

# Retarget CLAUDE_TICKETS_WORKFLOW doc to Claude-only + add one-time bootstrap companion

## Background

The predecessor ticket `260918-feat-downstream-tickets-workflow-doc` (now
`.done/`) authored `ai-docs/ref/TICKETS_WORKFLOW.md` as a host-neutral,
lightweight ticket-workflow doc for a downstream project with no ws/MCP tooling.
The user is now narrowing and splitting it:

- **Audience is Claude-only.** The real recipients are internal employees using
  Claude Code, distributed inside the company. The doc should stop hedging for
  non-Claude hosts and assume the Claude Code harness throughout.
- **The steady-state doc must stay lean.** One-time setup complexity (creating
  the ticket tree, wiring the embed, reconciling with a project's existing doc
  system) does not belong in a file that is `@`-embedded and paid for on every
  session. It moves into a separate bootstrap companion that is read once and
  then deleted.

This ticket builds on the retained impl branch `impl/develop/flock-fade-snide`
so the finished set (both docs, renamed) merges to `develop` once.

## Decisions

All decisions are user-confirmed this session.

1. **Claude-only retarget.** Fix the embed host as `CLAUDE.md` and assume the
   Claude Code harness: the native Explore agent, `@`-embed into `CLAUDE.md`,
   native Read/Write/Edit/Bash + git. Remove the steady-state doc's host-neutral
   hedging — the "`AGENTS.md` or `CLAUDE.md`" pairing, "or paste it in directly
   if the host has no embed mechanism", the generic "a native search/explore
   agent", and the Language section's "`AGENTS.md` or `CLAUDE.md` declares a
   different working language" — and make each Claude-specific.
   - *Why this is allowed:* the artifact lives in `ai-docs/ref/` as a bespoke
     internal deliverable, not shared plugin skill text, so Architecture Rule 3
     (host-neutral-first) does not bind it; the user explicitly chose a
     Claude-only target. `shipped-surface-boundary.md` still applies (no
     devenv-internal names), but `CLAUDE.md` / Claude Code / Explore are
     audience-standard terms, not devenv-internal, so they are permitted.

2. **Rename.** `git mv ai-docs/ref/TICKETS_WORKFLOW.md
   ai-docs/ref/CLAUDE_TICKETS_WORKFLOW.md`; the embed directive the doc shows
   itself becomes `@CLAUDE_TICKETS_WORKFLOW.md`. Update the exact-file entry in
   `agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py` `TEXT_TREES`
   from the old path to the new one.

3. **Add a one-time bootstrap companion doc**
   `ai-docs/ref/CLAUDE_TICKETS_WORKFLOW.bootstrap.md` (keep the `CLAUDE_` prefix
   so the two files sort together; refine the exact spelling only if a clearly
   better sibling name exists). It is NOT `@`-embedded — it is copied in
   alongside the steady-state doc, and the user tells the agent to read it once
   at setup. It is the home for the one-off setup tasks:
   - Create `ai-docs/tickets/{todo,.done}/` (and `.dropped/` when first needed)
     if absent — a fresh project will not have them.
   - Wire `@CLAUDE_TICKETS_WORKFLOW.md` into the project's existing `CLAUDE.md`
     (create `CLAUDE.md` if none).
   - Reconcile with the project's existing documentation / tracking system: give
     the agent a procedure and judgment (not a fixed script) to absorb or
     coexist with any pre-existing ad-hoc TODO / decision-notes convention
     without duplicating it.
   - Optionally seed one first ticket to validate the loop.
   - **Lifecycle (option A, user-chosen):** once setup is complete, the doc
     instructs the agent to delete the bootstrap file itself — it is not
     steady-state and should not linger in the downstream tree.
   - Register this bootstrap file in `TEXT_TREES` as well for banned-token
     regression coverage.

4. **Steady-state content otherwise unchanged.** The 3 pillars, decision ledger,
   commit `## AI Context`, naming + six-value enum, and autonomous
   capture/transition/search already authored stay as-is; only the host-neutral
   phrasing from decision 1 is de-neutralized.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/skill-authoring.md` before editing — both docs are
  behavioral, downstream-shipped text.
- Both produced docs stay free of devenv-internal names (`agents-plugin*`,
  `wsflow`, `ws/`, `install.sh`, real ticket stems/hashes). Claude-audience
  terms (`CLAUDE.md`, Claude Code, Explore) are allowed.
- Build on branch `impl/develop/flock-fade-snide`; the finished set (both docs,
  renamed) merges once.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)

## Prior Decisions

- 260918-feat-downstream-tickets-workflow-doc (2026-09-18, commit b8d36b3d): "added an exact-file TEXT_TREES entry for ai-docs/ref/TICKETS_WORKFLOW.md, following the same narrow-scope-not-whole-directory precedent the file already uses" — bearing: supports
- 260908-bug-shipped-surfaces-carry-devenv-only-content (2026-09-08, commit 7f7a3784): "User directive: a much stronger AGENTS.md downstream-extension rule that explicitly forbids naming concrete tickets inside shipped resources." — bearing: constrains
- 260913-bug-plugin-json-migration-vocab-uncovered (2026-09-13, Confirmed Decisions): "widen TEXT_TREES back to the full agents-plugin/.codex-plugin directory; narrowed to hooks.json only was to avoid re-litigating under an unrelated ticket" — bearing: constrains
- 260909-feat-bootstrap-refoundation-template-migration (2026-09-10, Result 81c40cf1): "The | manual | paths | column order Phase 1 forwarded was flipped to the shipped | paths | manual | — no parser reads the table positionally, so this is consistency only." — bearing: constrains
- 45c9cbaf (2026-05-04, commit): "Downstream projects migrate from Claude-centered CLAUDE.md context to Agents/Codex-oriented AGENTS.md context; company environments still expect CLAUDE.md." — bearing: supports
- 260501-research-agents-bootstrap-root-context (2026-05-01, Open Questions): "Which rules remain in CLAUDE.md as Claude compatibility behavior after AGENTS.md becomes the root context?" — bearing: constrains
- 260503-feat-agents-plugin-runtime-boundary (2026-05-05, commit 0acb8ee2): "Replaced CLAUDE.md with the @AGENTS.md shim required by the bootstrap contract." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | ai-docs/ref/TICKETS_WORKFLOW.md (renamed), ai-docs/ref/CLAUDE_TICKETS_WORKFLOW.bootstrap.md (new), agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py |
| scope.surface | public-interface | copy-once artifact embedded by a downstream CLAUDE.md via an at-mention embed directive (ticket Background/Decision 1); shipped-surface-boundary.md governs it as downstream-facing text |
| scope.new_public_symbol | no | none — prose documents only, no code symbol |
| scope.new_type_contract | no | none — no type or signature introduced |
| scope.test_surface | existing | agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py TEXT_TREES (existing exact-file entry for ai-docs/ref/TICKETS_WORKFLOW.md at line 59, to be updated/added, not a new test file) |
| complexity.reuse_points | confirmed | ai-docs/ref/TICKETS_WORKFLOW.md (169-line steady-state content to rename and de-neutralize, confirmed present); TEXT_TREES exact-file-entry pattern (confirmed precedent at agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py:59) |
| complexity.side_effect_risk | low | one renamed markdown file, one new markdown file, and a TEXT_TREES config-line change under ai-docs/ref/ and agents-plugin/tests/; no runtime or plugin code path touched |
| risk.correctness | moderate | steady-state content must stay unchanged per Decision 4 while precisely de-neutralizing only the phrases Decision 1 lists, and the bootstrap doc must correctly sequence zero-state setup (missing ticket dirs, missing/existing CLAUDE.md, pre-existing ad-hoc tracking) with no automated check enforcing either |
| risk.fit | moderate | must hold the predecessor doc's register while splitting lean steady-state content from one-time bootstrap ceremony without duplicating already-authored material |
| risk.test | moderate | the only automated coverage is TEXT_TREES banned-token regression scanning; no test validates the bootstrap doc's zero-to-loop-then-self-delete semantics, which Phase 2's own verification leaves to manual fresh-reader re-read |
| risk.security_or_contract | low | advisory documentation only; no code parses or enforces the doc's shape, so drift degrades gracefully |

## Phases

### Phase 1: Retarget and rename the steady-state doc

`git mv` `TICKETS_WORKFLOW.md` to `CLAUDE_TICKETS_WORKFLOW.md`, apply the
Claude-specific rewording from decision 1 (embed host, Explore agent, embed
directive `@CLAUDE_TICKETS_WORKFLOW.md`, Language section), and update the file's
entry in `test_shipped_surfaces_downstream_neutral.py` `TEXT_TREES`. No content
change beyond de-neutralizing.

Verification: `python3 -m unittest
agents-plugin.tests.test_shipped_surfaces_downstream_neutral` passes with the
renamed path; banned-token / real-stem / hash grep clean on the renamed doc.

### Phase 2: Author the bootstrap companion doc

Write `ai-docs/ref/CLAUDE_TICKETS_WORKFLOW.bootstrap.md` realizing decision 3,
and register it in `TEXT_TREES`. It must take an internal Claude Code user from
zero — no `ai-docs/tickets/` dir, an existing (or missing) `CLAUDE.md`, and
possibly a pre-existing ad-hoc tracking convention — to a working loop, then
remove itself.

Verification: test suite passes with both files registered; banned-token grep
clean on the bootstrap doc; fresh-reader re-read as an internal Claude Code user
setting up a brand-new project confirms the zero-to-loop-then-self-delete path
works and the steady-state doc stands alone afterward.

## Implementation Notes

Advisory (from design review; keep the split clean during authoring):

- The steady-state doc *describes* its own placement and embed directive; the
  bootstrap doc *performs* the one-time wiring into `CLAUDE.md`. Do not restate
  the wiring procedure in both — the steady doc mentions it is embedded, the
  bootstrap doc owns the actual setup step.
- Inside either produced doc, do not write the repo-side paths
  `ai-docs/ref/CLAUDE_TICKETS_WORKFLOW.md` or the bootstrap doc's own
  `ai-docs/ref/...` path verbatim: the banned-token guard (rule 2) flags
  git-tracked `ai-docs/<file>` paths outside the bootstrap-installed set.
  Reference downstream placement instead (project root, a bare
  `@CLAUDE_TICKETS_WORKFLOW.md`, `ai-docs/tickets/<status>/`). The banned-token
  grep plus the test suite catch any slip.
