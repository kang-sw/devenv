---
title: "lead-run identifies the worker key as the one un-noted child, but delegate keys are never noted"
related:
  260909-refactor-drain-ready-queue-worker-spawner: shipped the un-noted-child lookup rule this ticket corrects
completed: 2026-09-10
---

# lead-run identifies the worker key as the one un-noted child, but delegate keys are never noted

## Background

`lead-run` step 2 tells the lead to read the freshly rendered worker's key off
`session.children` as "the one child of your key carrying no note", relying on
step 4 noting every dispatched worker so exactly one un-noted child remains.
But `playbook.render` mints a child key for every delegate the lead renders
(ticket-fact-populator, sage reviewers, code-reviewer), and neither
`lead-ticket` nor `lead-review` notes those keys. After a few promotions the
lead's subtree holds several un-noted `scope: delegate` children and the rule
returns nothing usable.

Observed 2026-09-10 on the installed 0.45.2 build: `session.children` for the
lead key listed five un-noted delegate-scope children before any worker was
rendered, so the "exactly one un-noted child" precondition was already false.

`session.children` already reports `scope` per child: workers render with
`scope: control`, delegates with `scope: delegate`. Filtering on scope removes
the ambiguity without asking other skills to note their delegates.

## Phases

### Phase 1: Filter the worker-key lookup by control scope

Change the `lead-run` step 2 sentence to "the one `scope: control` child of
your key carrying no note", regenerate the rsrc manifest and the wsflow
mirror, and update any test that pins the sentence. Rejected: making
`lead-ticket` and `lead-review` note every delegate they render — the note is
the run's carry-over record, not a registry, and delegate keys are disposable.

### Result (bf365ddd) - 2026-09-10

`agents-plugin/rsrc/lead-run/lead-run.md` step 2 now reads the worker key off
`session.children` as the one `scope: control` child of the lead key carrying
no note, and the adjacent clause names delegate scope as what the filter
excludes. `sessionChildScopeLabel` in `internal/mcp/server.go` confirmed the
labels the playbook now cites: lead-capability children print `control`,
delegates print `delegate`. The rsrc manifest and the byte-identical
`agents-plugin-wsflow/rsrc/` mirror were regenerated; no test pinned the old
sentence.
