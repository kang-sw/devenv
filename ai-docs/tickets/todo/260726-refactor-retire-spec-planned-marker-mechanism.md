---
title: Retire the 🚧 planned-marker mechanism; pending spec contracts live in the ticket
related:
  260726-research-spec-planned-marker-management-cost: the research that decided this; carries the measurement and the function-relocation table
  260726-bug-spec-planned-marker-ready-ticket-cycle: dropped by this decision; its surviving finding is extracted to the ticket below
  260726-bug-inline-playbook-invocation-commit-ownership: the extracted finding — a real defect independent of this retirement
  260723-feat-ready-spec-address-hard-gate: owns the strength of the ready spec-address gate, which becomes the sole spec-addressing path
sage-review-design: required
---

# Retire the 🚧 planned-marker mechanism; pending spec contracts live in the ticket

## Background

Decided in `260726-research-spec-planned-marker-management-cost` after measuring
the corpus. The short version:

- **Real usage corpus-wide: 1 marker**, and it is stale — its backing ticket
  `260524-feat-ws-dashboard-workspace-root-prune-policy` is in `.done/` and
  `ws-web-dashboard/index.md:231` still carries the callout.
- **Contract-first declarations: 1 yes / 8 no** across live tickets.
- The mechanism's footprint is 2 Go call sites, an embedded convention section,
  4 rsrc playbooks, a judge, and a whole `lead-update-spec` step — for that one
  stale instance.

The decisive argument is **ownership lifetime, and it survives the adoption
confound**. Adoption was measured while the mechanism was unusable (the ordering
cycle in `260726-bug-spec-planned-marker-ready-ticket-cycle`), so "nobody used it"
is weak evidence. But this is not:

> `## Spec Impact` is ticket-side, so pending contract text **dies with its
> ticket** — drop or close it and the pending text goes too. Staleness is
> structurally impossible. `🚧` is spec-side, so it **outlives its ticket by
> default** and stays correct only if a separate reconciliation ritual is run.

That holds at any adoption rate. The one live marker is the proof: `lead-update-spec`
§5 "Strip `🚧`" exists and does exactly the right thing — extract the stem, check
`git.log`, strip when implemented — and was simply never run.

Two of the three functions `🚧` bundled were relocated to the design reviewer in
commit `2d1a731c` (reads the `## Spec Impact` target; scans `ready/` for
spec-territory conflicts). The third — forcing the author to write the contract in
the spec's own vocabulary — is **consciously given up**, not overlooked. No
instance of its value exists in the corpus; if one is demonstrated later, this
decision should be revisited rather than worked around.

## Decisions

- **`## Spec Impact` becomes the sole path for pending spec contracts.** Nothing
  planned is written into a spec document.
- **The compat note ships before or with the removal, never after.** Removing the
  mechanism deletes `lead-update-spec` §5, which is downstream's only cleanup
  path. Without a replacement, every existing downstream marker leaks silently.
  This is the ordering constraint that shapes the phases.
- **No new parser.** Both halves of the detection already exist in Go:
  `specMarkerContexts` (`spec_discovery.go:252`) already populates
  `SpecInfo.MarkerContexts` and `server.go:2526` already prints `marker:` lines
  to every `specs.find`/`specs.list` caller; `specStats`
  (`project_tree.go:174`) already counts WIP entries and extracts referencing
  ticket stems via `ticketRefRE`. The work is attaching prose to detection that
  already runs, not building detection.
- **No ticket-body reads.** The marker's anchor/feature-ref already yields a
  ticket stem, and ticket status is directory-based, so the note can say "its
  ticket is in `.done/` — strip it" versus "its ticket is still in `ready/` —
  move this text into that ticket's `## Spec Impact`" without opening any ticket
  file. This is cheaper than the originally proposed "ticket body mentions the
  stem" condition and avoids the body-read scope that
  `260726-feat-verify-ticket-graph-advisories` deliberately excludes.
- **Advisory, never blocking.** Per the reversibility principle already adopted
  in `260726-feat-verify-ticket-graph-advisories`: legacy markers are a migration
  state, not an error. The note routes; it does not fail a commit.
- **`260726-bug-spec-planned-marker-ready-ticket-cycle` is dropped by this
  ticket**, since it exists to make `🚧`'s ordering satisfiable. Its one finding
  that survives — `lead-write-spec` step 7 committing unconditionally while the
  contract-first branch invokes it inline — is extracted to
  `260726-bug-inline-playbook-invocation-commit-ownership` and must not be lost
  with the drop.

## Constraints

- Do not delete the one existing marker's *content*. Its backing ticket is
  `.done/`, so the behavior it describes is implemented; the entry becomes an
  ordinary implemented spec entry with the marker stripped, not a deletion.
- `markerContext` (`spec_discovery.go:266`) matches `🚧`, `planned`, and `wip`
  case-insensitively, so it fires on prose containing the word "planned". Do not
  tighten it to the emoji alone without checking what the looser match is
  currently carrying — and do not let the compat note inherit that false-positive
  rate.
- Embedded convention text (`agents-plugin-tool/internal/wsdoc/conventions/`) is
  `go:embed`'d, so the removal only reaches installed plugins through a version
  bump.

## Spec Impact

- Target spec area: `ai-docs/spec/documentation-system.md` — the contract-first
  `🚧` paragraph (lines ~98-102) and the spec-index reconciliation description
  (~236-240).
- Expected caller-visible change: specs no longer carry planned entries; pending
  contracts are read from `ready/` tickets' `## Spec Impact`. `specs.find`,
  `specs.list`, and `project_tree` gain an advisory note when a legacy marker is
  present, naming the backing ticket's current status and the resolution.
  `lead-update-spec` loses its Strip step; `judge: contract-first-spec` is
  removed.
- Contract-first spec: no — and deliberately so. Writing a `🚧` entry to describe
  the retirement of `🚧` is exactly the mechanism this ticket removes.

## Phases

### Phase 1: Legacy-marker compat note

Ships first and is independently valuable — this repository has a stale marker
right now, so the note is correct behavior even if Phase 2 never lands.

- Resolve each detected marker to its backing ticket stem (anchor `{#YYMMDD-slug}`
  or the `ticketRefRE` match already extracted by `specStats`), then to that
  ticket's status directory.
- Emit an advisory note on the surfaces that already compute markers —
  `specs.find` / `specs.list` (via the existing `marker:` output) and
  `project_tree` — stating that planned markers are a retired mechanism, naming
  the backing ticket and its status, and giving the resolution: strip when the
  ticket is `.done/`, move the text into `## Spec Impact` when the ticket is
  live, strip when the ticket is `.dropped/` or absent.
- Handle the unresolvable case explicitly: a marker whose stem matches no ticket
  gets a note saying so rather than being silently skipped.

Rejected alternatives: a new markdown parser (detection already exists); hosting
the check in `tickets.verify` (wrong subject — this is spec-side, and verify's
mechanical-floor role excludes it); blocking a commit on a legacy marker
(migration state, not an error).

Verification boundary: with `ws-web-dashboard/index.md`'s existing marker in
place, `specs.find` on that spec returns the note naming
`260524-feat-ws-dashboard-workspace-root-prune-policy` and its `.done/` status
with a strip instruction; a synthetic marker whose stem matches a `ready/` ticket
returns the move-to-`## Spec Impact` instruction instead; a synthetic marker with
an unmatched stem returns the unresolvable note.

### Phase 2: Remove the mechanism and ratchet downstream

Depends on Phase 1 shipping, per the ordering decision above.

- **Go:** remove marker handling at `spec_discovery.go:266` and
  `project_tree.go:174` (retaining whatever Phase 1's note needs), plus the two
  test call sites in `spec_discovery_test.go` and `project_tree_test.go`.
- **Embedded conventions:** remove the `## 🚧 Markers` section and its examples
  from `spec-conventions.md`; check `ticket-conventions.md:30` (dropping a ticket
  with linked spec entries) for dependent text.
- **rsrc playbooks:** `lead-write-spec` (the contract-first branch, the
  `> [!note] Planned 🚧` template, the `Planned marker:` output line, and the
  unsatisfiable "Session reminder"), `lead-update-spec` (§5 Strip `🚧` entirely),
  `lead-forge-spec`, `fresh-reader-audit`.
- **Judge:** remove `judge: contract-first-spec` from `lead-write-ticket` and
  `lead-write-spec`, and the Spec-address Check branch that invokes it.
- **Local data:** strip the marker from `ws-web-dashboard/index.md:231`, keeping
  the entry as implemented.
- **Bootstrap:** add a migration checklist item at a new template version so
  downstream projects on an older `<!-- Template Version: vNNNN -->` pick up the
  retirement. Note that AGENTS.md classifies migration-checklist semantics as
  always-ask; the owner approved this specific item, not a general license.
- **Regenerate and bump:** both rsrc regens (`WSRSRC_REGEN=1`,
  `WS_REGEN_WSFLOW_RSRC=1`) and a plugin version bump through
  `bump-ws-version.sh`, without which the embedded convention change does not
  reach installed plugins.
- **Drop `260726-bug-spec-planned-marker-ready-ticket-cycle`** with a
  `## Resolution` recording that its premise was retired, and confirm the
  extracted commit-ownership ticket exists before dropping it.

Rejected alternatives: leaving the convention text in place as documentation of a
retired mechanism (invites re-adoption); keeping `judge: contract-first-spec` as a
no-op (a judge that always answers the same way is dead prose).

Verification boundary: no `🚧` remains in the spec corpus, the conventions, or the
playbooks; `go test ./...` and the wsflow bundle tests pass; both regens are
idempotent; a fresh `lead-write-ticket` run on a spec-touching ticket reaches
`ready/` through `## Spec Impact` with no contract-first branch offered; the
bootstrap migration applies cleanly to a project pinned at the prior template
version.
