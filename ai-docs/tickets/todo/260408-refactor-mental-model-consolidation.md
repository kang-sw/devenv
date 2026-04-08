---
title: Consolidate mental-model management and remove spec layer
plans:
  phase-1: null
  phase-2: null
---

# Consolidate mental-model management and remove spec layer

## Background

Mental-model documents are the primary "project map" for AI agents modifying
a codebase — implicit contracts, coupling, extension points, silent-failure
risks. Despite this importance, the management infrastructure around them is
fragmented:

- **Inclusion test** defined independently in both `rebuild-mental-model` skill
  and `mental-model-updater` agent (same semantics, different wording).
- **Document format** duplicated across the same two files.
- **Trigger gaps**: `delegated-implement` has no doc pipeline at all;
  `discuss` mentions mental-model updates as a bullet point with no defined
  process.
- **No automated trigger** for full rebuild — only incremental updates via
  `mental-model-updater` are wired into `impl-process`.

In parallel, the spec layer (`write-spec` skill, `spec-updater` agent,
`ai-docs/spec/` documents) maintains a separate document set with its own
management pipeline. Audit of the workflow revealed that **no skill or agent
reads spec as input for decision-making** — spec is written and maintained
but has zero consumers in the current workflow.

## Key decisions

### Drop the spec layer entirely

Assessed whether spec provides unique value not covered by mental-model,
tickets, or source code. Three candidates emerged from a real-project
evaluation (libhbs):

1. **Domain-oriented reference view** — mental-model already organizes by
   cross-cutting concern, which serves the same purpose.
2. **Architectural cross-domain relationships** — these pass the mental-model
   inclusion test (ignorance causes wrong outcomes, not derivable from source
   in 30s). They belong in mental-model, not a separate system.
3. **Clean reference vs noisy ticket history** — mental-model is already a
   clean reference. This distinguishes spec from tickets, not from
   mental-model.

Conclusion: spec's valuable content (architectural narrative, design
rationale) naturally fits in mental-model. The remainder (API signatures,
struct layouts, status tracking) is either derivable from source or already
in tickets.

**Rejected alternative — merge spec into mental-model as an "Observable
Behavior" section.** When placed side by side, spec content and mental-model
content have different inclusion criteria (spec caches derivable information;
mental-model records non-derivable information). Forcing them into one
document creates a split-identity problem where some sections pass the
inclusion test and others are exempt from it.

### Broaden mental-model inclusion test

Current: "ignorance causes **silent failure** AND not derivable in 30s."

This is too narrow — it excludes architectural narrative and design rationale
that prevent wrong design decisions (not just silent runtime failures).

New: "ignorance causes **wrong outcome** AND not derivable in 30s."

"Wrong outcome" subsumes silent failure, wrong implementation, and wrong
redesign. This absorbs spec's genuinely valuable content without changing
the document format or creating heterogeneous sections.

### Promote rebuild-mental-model to write-mental-model

Following the `write-ticket` pattern: a single skill that serves as the
authoritative definition point for the document type — format, inclusion
test, doctrine. The companion agent (`mental-model-updater`) references the
skill's definitions rather than maintaining its own copies.

## Phases

### Phase 1: Remove spec layer

Remove `write-spec` skill, `spec-updater` agent, and all spec-related
references from `impl-process`, `write-ticket`, and other skills. Migrate
any architectural narrative from downstream `ai-docs/spec/` docs into
mental-model docs (downstream projects handle their own migration).

### Phase 2: Consolidate mental-model management

Rename `rebuild-mental-model` → `write-mental-model`. Establish as single
authority for format, inclusion test, and doctrine. Refactor
`mental-model-updater` agent to reference the skill's definitions. Wire
missing triggers (`delegated-implement` doc pipeline, `discuss` mental-model
path). Update inclusion test wording.
