---
title: Commit-anchored follow-ups for accepted review findings
related:
  260924-research-rationale-incremental-history-cache: independent sibling; if that cache lands, it should store every `## ` commit-body section by name so follow-up queries can read from it
sage-review-design: skipped
sage-review-completeness: completed
sage-review-completeness-reviewed: a87d300ecfbba638
---

# Commit-anchored follow-ups for accepted review findings

## Background

A review that clears with accepted non-blocking findings leaves them nowhere
tracked. Reviewer output goes to the untracked per-worktree cache
(`path.generate(kind: "review")`), the review ledger records only
`<base>..<head>: <verdict>`, and the only trace is whatever prose the lead
happens to put in a commit's `## AI Context`. The v0.46.26 release gate
accepted five Minors this way (one AI Context line in 733ecf47d); they were
closed only because the user asked, in 9176800ad. Without a tool, an accepted
finding is effectively lost.

This ticket adds a generic, filesystem-free mechanism: a root lead attaches
follow-ups to a commit body, any later commit resolves them by ID, and a
query tool computes which are still open over a commit range. lead-review
records accepted findings automatically; lead-ship surfaces open ones at the
release gate.

## Decisions

- **Storage: commit bodies.** Follow-ups live in commit message bodies, not
  in `ai-docs/.review-ledger.md`; the ledger format and parser are untouched.
  Rejected: findings embedded in the ledger (append-only file grows with
  findings that are easy to never clean up, and grows merge-conflict surface);
  `git notes` (`refs/notes/*` is not pushed or fetched by default, merges
  poorly, and is invisible on common hosts).
- **Tool surface.** `git.commit` gains two arguments; a new read-only tool
  `git.followups` queries them.

  ```text
  ws/git.commit(..., followups: [{level, category, content}, ...])  -> returns the minted id per follow-up
  ws/git.commit(..., resolves: ["<id>", ...])
  ws/git.followups(range?, category?, min_level?)
  ```

  `git.followups` defaults to the full history and returns open (unresolved)
  follow-ups only, each with its id, level, category, content, and the SHA
  and date of the commit that carries it. It walks commit bodies only (no
  `--name-only`; a full-history body-only walk measured 0.13s at 6.9k
  commits).
- **Ids.** Each follow-up id is a three-word codename minted by the same
  generator as session keys (`wskey.Generate`, EFF large wordlist, ~39 bits),
  independent of the commit SHA, so squash, rebase, and cherry-pick keep
  follow-ups and their resolutions matched; `resolves` matches by id alone.
  Word codenames cost about as many tokens as a short random string and
  agents copy them without character errors. Rejected: `<sha>/<n>` (breaks on
  rewrite); 8-character random base64 (more tokens per character, copy-error
  prone, case and `+/` awkward in commit text).
- **Body format.** `git.commit` writes the sections through the existing
  section writer, following the `## Ticket Updates` convention; content is a
  single line:

  ```text
  ## Follow-ups
  - minor/review amber-latch-dwelled: agents-plugin-pi/src/bridge.ts:485 resolvedSessionKeyArg re-implements key resolution

  ## Resolves
  - amber-latch-dwelled
  ```

- **Parsing.** `## Follow-ups` and `## Resolves` are not added to
  `wsrationale` `commitSections`, so `rationale.query` never returns
  follow-ups as rationale records; `git.followups` parses them through the
  shared `collectSections`.
- **Follow-up vs `## AI Context`.** AI Context is past-facing rationale with
  no state, queried by relevance (`rationale.query`). A follow-up is
  forward-looking work with an open -> resolved lifecycle, queried by state.
  Non-actionable context therefore stays in AI Context and there is no
  non-actionable level. Boundary with `## Ticket Updates` `> Forward:`: work
  owned by a ticket goes to Forward; ownerless work is a follow-up.
- **Levels.** Closed, ordered set `minor < important < critical`, reusing the
  reviewer severity vocabulary (`agents-plugin/rsrc/code-reviewer.md`) so a
  finding maps without translation. Rejected: `note | info | warn` (`note`
  duplicates AI Context); log levels `trace..error` (verbosity levels, not
  action levels; `error` means something already failed); adding `major`
  (overlaps `important`); P0-P3 priorities (new vocabulary).
- **Category.** Closed enum; v1 holds `review` only. Adding a value is a
  deliberate code change, admitted only when (a) the items have no existing
  home (ticket, Forward note, ws note, code comment) and (b) an item left
  unattended for one release cycle may acceptably drop out of view.
  Rejected: free-form slugs (agents invent categories and scatter queries).
- **Decay by range.** Consumers bound visibility by the range they pass; an
  open follow-up nobody handles within a cycle falls out of the next cycle's
  range. A full sweep is the same query without `range`. Rejected: always
  showing every open follow-up regardless of age (recreates the unbounded
  accumulation this design avoids).
- **Write guard.** The `followups` argument is accepted only from a root lead
  session (session scope `lead`, no parent). This is an argument-level gate,
  since `git.commit` is shared with workers and existing lead-only gating is
  tool-level. A `followups` argument with no session key, or with a key that
  is not a root lead, rejects the commit. There is no count or length cap;
  only the single-line content rule applies. Rejected: a per-commit count cap
  and content length cap that reject with a nudge toward a ticket (the root
  lead gate and closed category already bound misuse). The `resolves` argument is accepted from any caller (worker,
  delegate, or lead), because the fixing commit is usually not the root
  lead's; rejected: gating `resolves` to the root lead too (forces a separate
  lead commit after every fix). Resolving an unknown or already-resolved id
  is a warning, not a refusal.
- **lead-review integration.** At stamp time, lead-review automatically
  records each accepted Minor as a `minor`/`review` follow-up and each
  overridden Important as an `important`/`review` follow-up, carried on the
  stamp (ledger) commit. Committing the ledger after `review.stamp` becomes a
  playbook-mandated `ws/git.commit` step (today it is only a habit), since
  the follow-ups need a commit to ride. This applies to every lead-review
  stamp, release gate and merge-time alike.
- **lead-ship integration.** The release gate queries
  `git.followups(category: review, min_level: minor, range: <start>..HEAD)` (start per **Release-gate query range** below)
  and displays the result without blocking. The lead re-checks `important`
  and `critical` items individually against HEAD: one still valid stays open;
  one incidentally fixed or whose target is gone is resolved. Those
  resolutions ride the commit the ship already makes before publishing (in
  this repository, the version-bump commit), or a dedicated `chore(review)`
  commit when the ship makes none. `minor` items
  are listed only and left to decay. Rejected: re-checking every item
  (agent cost scales with item count).
- **Release-gate query range.** The range starts no later than just before
  the previous release's own release gate, so that gate's stamp commit and
  the follow-ups it accepted are inside the range; the lead judges that
  point from the project's release history and may start earlier, since the
  range exists for query cost, not correctness (resolved follow-ups are
  filtered out regardless). With no identifiable previous release the query
  runs without a range (full sweep). The query runs before the current
  gate's lead-review step, so follow-ups this gate accepts are first shown
  at the next gate; every follow-up thus gets at least one look at the next
  release gate after it was recorded. Rejected: `<previous release>..HEAD`
  (follow-ups accepted at a release gate sit before that release's tag and
  are never surfaced again); a fixed two-release window (a rule where lead
  judgment suffices); a ship-config declaration of the point or a rule
  deriving it from the ship config's `## Tag` section (both specify what a
  frontier-model lead already judges; the first also changes the downstream
  ship config template).

## Constraints

- Shipped playbook text stays downstream-neutral: it must not name this
  repository's tags, branches, or ship config
  (`ai-docs/manuals/shipped-surface-boundary.md`, AGENTS.md Architecture
  Rule 4).
- Commit-body section parsing is centralized in
  `agents-plugin-tool/internal/wsrationale` (`commitSections`,
  `collectSections`); the section writer is `wsgit` `CommitMessage` /
  `writeCommitSection`. Extend these rather than adding a second parser or
  formatter.
- Matching manuals from AGENTS.md `### Implementation Conventions`:
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`,
  `ai-docs/manuals/wsflow-mirroring.md`, `ai-docs/manuals/ws-mcp.md`.
- Out of scope: the incremental history cache
  (260924-research-rationale-incremental-history-cache), ledger compaction,
  and any change to `## Ticket Updates` / `> Forward:`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/`)

## Prior Decisions

- 733ecf47d (2026-10-02, commit): "Remaining Minors accepted as non-blocking: resolvedSessionKeyArg re-implements key-resolution rules (fail-safe), duplicated '## Session Key' anchor literal, and three untested edges" — bearing: supports
- 9463de13 (2026-09-30, commit): "Follow-up review of 2295a7bae..233168e88 confirmed the repair (mutation-checked) and found nothing new; stamped pass. Minors 2-7 accepted as non-blocking." — bearing: supports
- b0681ea6 (2026-09-21, commit): "Three non-blocking follow-ups surfaced by the 0.46.15 ship-gate review sweep (59140d29..10150d41), captured as idea/ so they do not block the release" — bearing: supports
- 41760df6 (2026-09-04, commit): "Release gate per lead-ship; verdict LGTM -> ledger token pass. Minor findings are recorded-only and not fixed inline to avoid moving the tip past the reviewed head before shipping." — bearing: constrains
- d5ca30fe (2026-09-27, commit): "This commit is the release gate's own record; the pre-flight R4 pin includes it as the final develop tip, matching the project's established release-ledger pattern." — bearing: supports
- 247fdfe8 (2026-09-10, 260909-bug-git-commit-emits-updated-tickets-heading): "Changed the emitter rather than the conventions: one emitter versus the template, its v0009 migration item, WORKFLOW.md, and AGENTS.md - the emitter was the single outlier" — bearing: constrains
- 484fab03 (2026-09-16, commit): "Read-side only, no cache in v1: every source (commit bodies, ticket files) is ground truth that does not rot; a full HEAD scan of this repo (~6600 commits) measured ~1.2-2.7s per call" — bearing: supports
- ce5f30ef (2026-09-09, 260909-refactor-lead-surface-collapse-worker-stop-protocol): "delegates: is left at each playbook's existing value rather than raised for lead-review and lead-ship, whose new bodies do spawn." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsgit/git.go, agents-plugin-tool/internal/wsrationale/commits.go, agents-plugin-tool/internal/wsrationale/extract.go, agents-plugin-tool/internal/mcp/server.go, agents-plugin/runtime.json, agents-plugin-wsflow/runtime.json, agents-plugin-pi/src/delegation-policy.ts, agents-plugin/rsrc/lead-review/lead-review.md, agents-plugin/rsrc/lead-ship/lead-ship.md, agents-plugin-wsflow/rsrc/lead-review, agents-plugin-wsflow/rsrc/lead-ship |
| scope.surface | public-interface | MCP tool git.commit gains followups and resolves arguments; new MCP tool git.followups; shipped lead-review and lead-ship playbooks |
| scope.new_public_symbol | yes | MCP tool git.followups |
| scope.new_type_contract | yes | git.commit followups [{level, category, content}] and resolves [id] arguments with minted-id result; git.followups(range, category, min_level) result rows |
| scope.test_surface | existing | agents-plugin-tool/internal/wsgit/git_test.go, agents-plugin-tool/internal/mcp/session_auth_test.go, agents-plugin-tool/internal/mcp/playbook_render_surface_test.go, agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py, agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py |
| complexity.reuse_points | confirmed | wskey.Generate at agents-plugin-tool/internal/wskey/wskey.go#L59, wsgit writeCommitSection at agents-plugin-tool/internal/wsgit/git.go#L862, wsrationale collectSections at agents-plugin-tool/internal/wsrationale/extract.go#L28, root-lead parent and scope check pattern at agents-plugin-tool/internal/mcp/mailbox_runtime.go#L489 |
| complexity.side_effect_risk | moderate | git.commit is shared by workers, delegates, and leads, and lead-review gains a mandated ledger commit after review.stamp |
| risk.correctness | moderate | open-versus-resolved computation over a range and argument-level session gating must hold under rewritten SHAs |
| risk.fit | moderate | shipped playbooks must stay downstream-neutral and the new tool must be registered in every runtime manifest and the Pi adapter policy |
| risk.test | moderate | Phase 1 lists ten required test behaviors including a rewritten-commit id match |
| risk.security_or_contract | high | new argument-level authorization gate on git.commit plus a new public MCP tool and commit-body format contract |

## Phases

### Phase 1: Follow-up tool surface

Implement the tool half of `## Decisions` in `agents-plugin-tool`:

- `git.commit` accepts `followups: [{level, category, content}]` and
  `resolves: [id]`, validates `level` against the closed ordered set
  (`minor`, `important`, `critical`), `category` against the closed enum
  (`review`), and single-line content, enforces the root-lead gate on
  `followups`, mints one id per follow-up, writes the `## Follow-ups` and
  `## Resolves` sections, and returns the minted ids in its result in the
  input order of `followups`.
- Resolving an id that is unknown or already resolved in the full history
  from HEAD produces a warning in the result; the commit still lands.
- New read-only tool `git.followups(range?, category?, min_level?)`: walks
  commit bodies (no `--name-only`) over `range` or, absent it, full history
  from HEAD; parses both sections with the shared `wsrationale` section
  parser; returns open follow-ups only, each with id, level, category,
  content, carrying commit SHA, and date. An omitted `category` or
  `min_level` applies no filter. Openness is evaluated as of the range's end:
  for an explicit `a..b`, a follow-up resolved only after `b` (or on a branch
  not reachable from `b`) is reported open.
- Register the new tool wherever the tool inventory is declared (MCP
  schema, runtime capabilities, and any adapter that maps ws tools), as the
  existing `git.*` tools are.

Done when the `go test ./...` suite in `agents-plugin-tool`, the wsflow
runtime-contract tests (`agents-plugin-wsflow/tests/`), and the
`agents-plugin-pi` test suite pass with `git.followups` registered in every
runtime manifest and the Pi delegation policy, and with new Go tests
covering: section write and parse round trip; enum, level, and
single-line validation; rejection of `followups` without a session key and
with a non-root-lead key; acceptance of `resolves` from a non-lead key; id
minting and return; open filtering after a resolve; `category` and
`min_level` filtering; range-bounded queries; the unknown or already-resolved
resolve warning; and an id still matching after the follow-up and its
resolve sit in rewritten (different-SHA) commits.

### Phase 2: lead-review and lead-ship integration

Depends on Phase 1. Wire the playbooks to the tool per `## Decisions`:

- lead-review: after `review.stamp`, commit the ledger through
  `ws/git.commit`, passing accepted Minors as `minor`/`review` and overridden
  Importants as `important`/`review` follow-ups, each content naming
  `<path>:<line>` and a one-line summary.
- lead-ship release gate: before the gate's lead-review step, call
  `git.followups(category: review, min_level: minor, range: <start>..HEAD)`
  with the start judged by the lead per `## Decisions` (no range when no
  previous release is identifiable), show the result without blocking,
  re-check `important`+ items against HEAD, and carry resolutions in the
  ship's pre-publish commit or a `chore(review)` commit.
- Mirror every changed shared playbook per
  `ai-docs/manuals/wsflow-mirroring.md`.

Done when the existing playbook render, surface, wsflow-mirror, and
downstream-neutrality test suites pass with the changed playbooks, and a
playbook test asserts lead-review names the follow-up step and lead-ship
names the `git.followups` release-gate query ahead of the gate's
lead-review step.

## Sage Review Round 1 (2026-10-02)

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | Release-gate follow-ups never reach a later gate | important |
| 2 | History walked by the resolve warning is not specified | minor |
| 3 | Phase 1 done-check does not cover tool registration | minor |
| 4 | Default filters and result mapping left implicit | minor |
| 5 | Resolve-ordering claim holds only for ranges ending at HEAD | minor |
