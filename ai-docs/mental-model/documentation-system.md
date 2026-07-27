---
domain: documentation-system
description: "Project memory, conventions, specs, tickets, mental models, and wsdoc discovery contracts."
sources:
  - ai-docs/
  - agents-plugin-tool/internal/wsdoc/
related:
  workflow-skills: "planning skills enforce spec and ticket conventions before implementation."
  mcp-runtime: "MCP and CLI adapters render wsdoc discovery results; wsdoc owns matching and metadata."
  git-workflow-tools: "verifyAdapter/formatGitCommit render VerifyResult.Advisories on the commit path; wsdoc owns the graph pass and the Kind carrier."
---

# Documentation System

## Entry Points

- `agents-plugin-tool/internal/wsdoc/` powers read-only MCP discovery for infra docs, specs, tickets, mental models, references, conventions, and project tree.
- `agents-plugin-tool/internal/wsdoc/conventions/*.md` is the bundled source for `ws/convention.read`. {#260505-documentation-convention-access}

## Module Contracts

- Spec identity is body-anchor based. Tools scan `{#YYMMDD-slug}` anchors under `ai-docs/spec/`, not frontmatter. {#260505-spec-document-system}
- `ai-docs/.old/` is the Git-tracked project archive for inactive reference material; dot-prefix keeps it out of default listings. {#260511-project-old-archive}
- Reconstructed old spec inputs live under `ai-docs/.old/spec/`, not `ai-docs/ref/old-spec/`. {#260511-project-old-archive}
- `ws/spec_index.verify` currently verifies duplicate anchors; it does not rebuild indexes or validate stale behavior.
- Ticket status is directory state: `ready/`, `todo/`, and `idea/` are active; `.done` and `.dropped` are invisible unless discovery calls opt in. {#260505-ticket-document-system}
- Ticket Result hashes identify the commit that made the completed phase reviewable on its current branch; merge commits are only required when the phase was already merged before the ticket update. {#260505-ticket-document-system}
- Ticket phase plan text freezes after the first Result, but later implementation tweaks append `#### Edition` entries under the Result area; existing Result and Edition text remains frozen. {#260513-ticket-result-editions}
- Result and Edition prose is delta-only: record behavioral changes, deviations, verification evidence, unresolved findings, and deferred follow-up findings without repeating the phase plan or linked spec. {#260513-ticket-result-editions}
- Ready-ticket convention keeps spec addressing mandatory through `spec:`, `spec-remove:`, or `## Spec Impact`; `lead-write-ticket` never invokes `lead-write-spec` — those three paths are the whole surface. {#260505-documentation-authoring-workflows}
- Epic tickets are lightweight milestone boards for scope, child-ticket decomposition, cross-child decisions, and completion criteria; child tickets carry implementation detail and complete phase units. `lead-proceed` executes one selected phase without changing ticket decomposition. {#260508-lightweight-epic-ticket-conventions} {#260505-proceed-routing-pipeline}
- Workset tickets are non-hierarchical operating-context boards for a session, goal, sprint, or temporary focus area; included tickets are listed by stem/path with status and role, planned entries stay under `## Planned References`, workset inclusion never changes `parent:`, implementation detail belongs in included actionable tickets, and worksets normally stay out of `ready/`. {#260524-workset-ticket-conventions}
- Mental-model hierarchy is path-derived; subdomain callers must load parent `index.md` before child docs. {#260505-mental-model-document-system}
- The root `ai-docs/mental-model.md` may carry a compact project reading map for task/topic routing; it does not own behavior, status, focus, or source-derived claims. {#260505-mental-model-document-system}
- `### Mental Model Notes` is an optional workflow-internal commit subsection under `## AI Context`; `mental-model-updater` treats those notes as primary intent and uses diffs for verification and fallback. {#260518-mental-model-update-context-annotation}
- `ws/infra.read` serves infra docs from the rsrc tree (`agents-plugin/rsrc/`, 260611 Phase 6b); `ws/convention.read` serves convention docs from a `go:embed` bundle in `wsdoc`. Retired legacy copies do not affect either.
- Broad `specs.find` and `mental_models.find` query matching is token-scored in shared wsdoc helpers, not per-discovery substring checks; exact selectors (`spec_stem`, `ticket_stem`, `domain`) still filter before query scoring. {#260519-tolerant-documentation-lookup-query-evidence}
- Query evidence is body-line-only: metadata can raise a document score, but it must not create synthetic line evidence. This prevents text output from pointing callers at non-existent line zero hits. {#260519-tolerant-documentation-lookup-query-evidence}
- `ai-docs/WORKFLOW.md` is bootstrap-installed explanatory documentation for plugin-less maintenance; wsdoc parsers and MCP tools do not treat it as convention, spec, ticket, or runtime input. {#260506-bootstrap-workflow-guide}
- `ai-docs/ref/` documents are operational runbooks or stable references; caller-visible behavior belongs in specs, modification coupling belongs in mental models, and code-derived inventories belong in source or runtime discovery. {#260524-reference-document-ownership}
- `TicketVerify`'s cross-file ticket-graph pass (`internal/wsdoc/tickets_graph.go`) loads the whole board once — `scanTickets` with `IncludeDone`/`IncludeDropped` plus `scanSpecs` anchors — and reuses that one load for both the `parent:` ancestor walk and four non-blocking integrity checks (unresolvable `parent:`, unresolvable `related:`, `parent:` cycles, non-epic `parent:`). A load failure degrades to silence: advisories are dropped and the ordinary verdict returns, because `TicketVerify`'s error return stays reserved for caller-input shape errors, never for a whole-board scan failure unrelated to the commit being verified. {#260727-tickets-verify-graph-advisories}
- The graph pass keeps two distinct lookups on purpose: `ticketGraph.byStem` (most-open copy wins when a stem is duplicated across status directories) drives the board's child-count/closure logic, while each verified ticket's own integrity-check subject resolves through `byPath`/`verifiedInfo` instead. Resolving the subject through `byStem` would, on a duplicate-stem board, check a different file's frontmatter than the one actually verified — this collapsed once already and is the most likely reintroduced regression if the two lookups get merged for "simplicity."
- The board block's "No further ancestors." claim is derived per-ancestor from that ancestor's own frontmatter (`chainEndsAt`: does *this* ancestor's `parent:` resolve or is it absent), not from a call-scoped truncation flag. Ancestors are deduplicated across a multi-ticket verify call, so a call-scoped flag would let one ticket's truncated `parent:` chain suppress the claim on another ticket's genuinely complete chain.
- Integrity advisories cap at 5 per **verified ticket** (`graphIntegrityCap`), not per call: `VerifyAdvisory` carries no subject attribution, so a per-call cap could drop one ticket's advisories entirely on a multi-ticket commit with nothing naming which ticket lost them.
- Advisory output whose two branches differ in destructiveness must land every uncertain or degraded input on the non-destructive branch. The legacy planned-marker note (`internal/wsdoc/legacy_marker.go`) resolves to either "live tickets own this marker" or "orphaned; strip it", and three separate defects in one phase — an unbalanced code fence hiding a ticket's `## Spec Impact`, an absent `ai-docs/tickets/` read as a failed scan, and a nil resolver — each silently flipped a live contract into the strip-it branch, i.e. emitted a delete instruction off an error path. Hence: a scan that could not complete says "ownership could not be determined", a nil resolver defaults to incomplete, and a document whose fences do not balance is rescanned fence-blind rather than guessed at. Same shape as `TicketVerify`'s degrade-to-silence rule above; treat it as the default for any new advisory.
- `## Spec Impact` detection lives in two places that must agree. `readyGateWarning` (`tickets_mutate.go`) decides whether a ticket counts as spec-addressed for `ready/`; `collectSpecImpact` (`legacy_marker.go`, constant `specImpactHeading`) harvests the same section to decide whether a spec marker has a live owner. Both open on the loose `## Spec Impact` prefix. Tightening only one produced a ticket that passed the ready gate as spec-addressed and then had its markers reported as orphaned — loose is the safe direction for both, and neither site changes without the other in the same commit.
- `relatedEntries` (tickets.go) normalises `related:` frontmatter from three legal shapes — nested map, list, bare string — into the `map[string]string` `TicketInfo.Related` advertises; list items run through `cleanScalar` to strip trailing comments. Before this the list form silently resolved to `nil` on a bare type assertion, which would have made the graph pass's dangling-`related:` check silently blind to that frontmatter shape.

## Coupling

- `references.trace` composes ticket, spec, and mental-model discovery. Changes to any parser change trace completeness. {#260505-documentation-reference-tracing}
- A project reading map points to specs, mental-model docs, references, or lookup guidance; those target documents remain the owners for behavioral and implementation facts.
- `VerifyResult.Advisories` (`VerifyAdvisory{Kind, Text}`) crosses into `git-workflow-tools`: `verifyAdapter`/`formatGitCommit` render the identical advisory set the standalone `tickets.verify` tool renders, appending the amend-recipe sentence only to `Kind == AdvisoryKindFix` entries. `Kind`, not the rendered text, drives that decision. {#260723-tickets-verify-tool}
- Ticket/spec linking has two directions: ticket `spec:` frontmatter and spec body/frontmatter ticket refs. Both matter to trace output.
- Mental-model/spec linking is one-way from mental-model text to spec stems; when a spec stem is renamed, mental-model references must change in the same commit.
- `ProjectTree` still has compatibility behavior around old `features:` frontmatter; active spec truth is body anchors and markers.

## Extension Points & Change Recipes

- **Add a convention**: add Markdown under `internal/wsdoc/conventions/`, then update any compatibility copy that remains authoritative for Claude fallbacks.
- **Add a ticket status**: update status normalization, rank, scan defaults, project tree rendering, conventions, Git move detection, MCP schemas, prompts, and skills.
- **Add doc discovery tools**: update MCP dispatch, tool schema, parameter validation, tests, and docs; reuse shared query matching/evidence helpers when the tool accepts broad human `query` text. {#260505-documentation-authoring-workflows}
- **Scan markdown structure**: reuse the block-structure helpers in `internal/wsdoc/legacy_marker.go` — `fenceTracker` (backtick and tilde fences, info strings, closing-run length), `htmlCommentTracker`, `splitMarkdownIndent` with `maxMarkdownBlockIndent` (CommonMark's ≤3-column rule), and `yamlFrontmatterEnd` — instead of a bare `strings.TrimSpace` prefix test. The docs in this repo document their own markup, so a trim-only scan matches the fenced, indented, and commented examples that exist precisely to be inert. The block rules apply to the document body only: YAML frontmatter is not CommonMark and nests by indentation, so applying the indent rule there turns a real hit into a silent miss.

## Common Mistakes

- Adding spec anchors manually without checking for duplicates.
- Promoting non-epic, non-research, non-workset work into `ready/` without a confirmed spec stem, `spec-remove:`, or `## Spec Impact`.
- Using full YAML features in frontmatter; the parser is deliberately minimal.
- Changing workflow semantics in the downstream workflow guide instead of the canonical plugin/runtime, bundled conventions, or bootstrap templates.
- Loading mental-model child docs without ancestors and missing inherited Domain Rules.
- Moving current feature inventory or implementation status from `_index.md` into the project reading map instead of specs, tickets, source, or tests.
- Duplicating MCP tool schemas or current tool inventory in `ai-docs/ref/`; use `tools/list`, `runtime capabilities`, source registries, or specs for stable behavior instead. {#260524-reference-document-ownership}
- Re-glossing ticket-system concept meaning (status dirs, type prefixes, sage review, phase model, epic/workset) in convention or playbook docs; concept meaning lives once in the `lead-workflow-manual` "Ticket System Concepts" section, guardrails stay in `ticket.verify`, and mechanical content stays in the Go template/checklist/sage constants. Convention/playbook docs keep only rules + a pointer. {#260723-ticket-system-concept-grounding}
- Replacing tolerant broad-query scoring with exact phrase matching; multi-word user questions should find candidates by shared terms while exact selectors remain exact filters.

## Technical Debt

- `markerContext` treats `planned` and legacy `wip` prose broadly, which can surface false-positive marker metadata. It cannot simply be tightened: its output also feeds `specs.find` match scoring, so narrowing it changes discovery ranking. Code that needs a precise marker predicate reimplements detection (`legacyMarkerLines`) rather than reusing it.
- Project-tree spec feature stats are compatibility output, not the active spec index. `specStats` reads `features:` frontmatter, which no spec file in this corpus declares, so it is structurally dead here and is not a usable hook for anything that must detect spec *body* content.
