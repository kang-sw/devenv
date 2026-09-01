---
title: wsdoc ticket scanner silently skips non-canonical status directories (e.g. non-hidden done/)
related:
  260728-research-duplicate-ticket-stem-silent-resolve: adjacent scanner robustness/observability topic in the same wsdoc ticket-resolution path
---

# wsdoc ticket scanner silently skips non-canonical status directories (e.g. non-hidden done/)

## Background

Surfaced in the same downstream `wsflow` v0.44.2 dogfood that produced the
`enter.proceed` report. The downstream repo had a **non-hidden**
`ai-docs/tickets/done/` directory, while the canonical archive name is the
**dotted** `.done/`. Observed consequences:

- `tickets_list` (default listing) missed a ticket that lived in `done/`; it
  surfaced only via an explicit `statuses:["done"]` query.
- `git.commit` twice emitted `related: 260818-... resolves to no ticket stem and
  no spec anchor` for a ticket whose file existed at `ai-docs/tickets/done/...`.

Root cause: the scanner's status-directory set is the canonical five dotted/bare
names only. `statusDirs` (`wsdoc/tickets_mutate.go:73-79`) maps
`idea/todo/ready/.done/.dropped`; `EffectiveTicketStatuses`
(`wsdoc/tickets.go:364-392`) builds the walk list from those; `scanTickets`
(`wsdoc/tickets.go:229-247`) joins each canonical name under the tickets root.
`normalizeTicketStatus` (`tickets.go:394-405`) maps a *requested* token
`done→.done`, but that normalizes the caller's status argument, not directory
names found on disk. So a non-hidden `done/` is never joined, never walked, and
its tickets are invisible to the scan — with **no warning**. `tickets_verify`
(`tickets_verify.go:134-139`) knows the canonical five-directory set and can flag
other directory names as invalid, but that check is not surfaced in the normal
list/commit/resolve path.

Note: this is separate from — and was NOT the cause of — the companion
`enter.proceed status=unknown` blocker (see
`260901-bug-enter-proceed-misplaced-facts-silent-unknown-status`), whose target
was in `ready/`, a name present in the canonical set. This ticket is the
independent directory-scan robustness/observability gap.

The underlying trigger is downstream repo drift (a repo whose archive dir is
`done/` instead of `.done/`), so the fix is about **surfacing** the drift rather
than silently tolerating a non-canonical layout.

## Decisions

Open (settle at triage/promotion): whether the right response is
(a) **warn-only** — emit a "non-canonical status directory `done/` detected;
canonical is `.done/`" diagnostic from the list/commit/resolve path when an
unexpected sibling directory exists under `ai-docs/tickets/`, leaving on-disk
data untouched; or (b) additionally **tolerate-and-map** the common
`done/`→`.done/` (and `dropped/`→`.dropped/`) drift in the scan. Preference
leans warn-only (keep one canonical layout; make drift loud, not silently
absorbed), but this needs confirmation before implementation.

## Phases

### Phase 1: Surface non-canonical status directories instead of silently skipping

Goal: a ticket sitting in a non-canonical status directory under
`ai-docs/tickets/` (e.g. non-hidden `done/`) must not silently disappear from
default listing, stem resolution, and commit-time related-stem resolution.

Suggested (pending the Decisions resolution):

- Detect sibling directories under the tickets root that are not in the
  canonical set and emit a single diagnostic naming the offending directory and
  the canonical name, from the paths where the miss is currently silent
  (`tickets_list` default listing; `git.commit` stem/anchor resolution).
- Reuse or lift `tickets_verify`'s canonical-set knowledge
  (`tickets_verify.go:134-139`) rather than re-encoding the directory list.
- Do not change the canonical layout or auto-migrate on-disk directories in this
  phase; keep the scanner authoritative on `.done/`/`.dropped/`.

Constraints:

- Do not degrade performance of the common (clean-layout) scan path; the
  detection should cost only when an unexpected directory is actually present.
- Keep the diagnostic advisory (non-fatal) so a drifted downstream repo is not
  hard-blocked from listing/committing.

## Spec Impact

Likely touches the ticket-scan / status-resolution behavior contract (MCP
tools spec for tickets listing and git.commit stem resolution). Exact spec area
to be confirmed at promotion; `idea/` landing does not require spec addressing.
