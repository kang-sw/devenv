# Plan: ws-dashboard drop sweep — archive tag, git-surface teardown, doc and board removal — Phase 3: Remove dashboard documentation and reconcile the board

## Relevant Ticket Contract
- Delete the entire `ai-docs/spec/ws-web-dashboard/` domain and `ai-docs/mental-model/ws-web-dashboard.md`; do not archive them under `ai-docs/.old/` because Git plus `archive/ws-dashboard` is the recovery path.
- Move exactly 13 dashboard tickets to `ai-docs/tickets/.dropped/` and remove their `_index.md` rows; keep and re-home `260523-bug-worktree-local-index-missing` as the unchanged ws-core defect.
- Correct dashboard-survival premises in the pivot/research record without changing the unrelated ws/wsflow plugin surface or recording the unsettled markdown-board alternative.
- `ws/spec_index.verify` must pass, no active ticket may retain a dangling `parent:`, and residual-reference checks must use the ticket's stated exemption set.

## Out of Scope
- Dashboard code, branches, PRs, and archive-tag safety work completed in Phases 1–2.
- The retained `260729-feat-workflow-manual-submodule-detection` and `260523-bug-implement-merge-target-discovery` tickets.
- Replacing the dashboard with an Orca plugin or deciding the Markdown-board alternative.
- Rewriting historical dashboard release entries in `CHANGELOG.md`.

## Codebase Findings
- `ai-docs/mental-model.md#L20-L20` and `ai-docs/mental-model.md#L36-L36` are the two domain-index rows that point only to the deleted dashboard spec and mental model; `developer-environment-tools` at `#L19` is a separate dashboard-sounding but retained row.
- `ai-docs/mental-model/named-agent-runtime.md#L11-L11`, `#L53-L53`, and `#L80-L80` respectively retain the dashboard relation, Activity Console coupling, and a Windows sharing-violation example; preserve the `replaceFile` retry contract while removing only that retired example.
- `README.md#L7-L8` and `README.md#L23-L31` name the dashboard scaffold in prose and the root-tree listing.
- `ai-docs/_index.md#L18-L19`, `#L161-L161`, and `#L171-L226` contain the scaffold, spec, and active-ticket projections; the retained local-context ticket is the row at `#L213` and needs rewording, not deletion.
- Nine current tickets carry `parent: 260514-epic-ws-web-dashboard-mvp`; `ai-docs/tickets/idea/260523-bug-worktree-local-index-missing.md#L1-L44` is the sole retained child, so the other eight children plus the epic and four named unparented tickets make the required 13 moves.
- `ai-docs/tickets/idea/260523-bug-worktree-local-index-missing.md#L22-L44` couples a still-live ignored-local-context defect to the retired dashboard; re-home it by removing the parent/mental-model relation, retitling it, reframing its third Background paragraph as a broad-API constraint, and replacing Direction with an undecided-mechanism statement.
- `ai-docs/tickets/todo/260605-epic-ws-playbook-factory-pivot.md#L6-L6`, `#L28-L32`, and `#L38-L40` still state dashboard-retention/M3-compilation premises; `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md#L603-L616` and `#L884-L893` establish the existing `### Supersede:` style and the obsolete port-vs-remove decision.
- `ai-docs/tickets/idea/260726-research-spec-planned-marker-management-cost.md#L87-L99` cites the soon-deleted dashboard marker; its dated measurement must be corrected without claiming that this sweep retired the mechanism.
- `ai-docs/tickets/idea/260730-research-ws-dashboard-drop-for-orca.md#L63-L85` and `#L106-L114` still describe a dangling branch and open cleanup decisions, while `ai-docs/tickets/idea/260730-research-orca-plugin-ws-workflow-surface.md#L54-L79` and `#L88-L98` contain the four already-verified Orca findings to record.
- `ai-docs/ref/ws-dashboard-playwright.local.md` and `ai-docs/_index.local.md` are ignored by `.gitignore#L16`; both are absent from this Phase 3 worktree, so their workstation-local cleanup cannot be proved or committed from this tree.

## Implementation Plan
1. Delete `ai-docs/spec/ws-web-dashboard/` and `ai-docs/mental-model/ws-web-dashboard.md`; remove their map/table references from `ai-docs/mental-model.md`, remove only dashboard-specific relation/coupling/example text from `ai-docs/mental-model/named-agent-runtime.md`, and remove the dashboard prose/tree entry from `README.md`.
2. Reconcile `ai-docs/_index.md`: remove the scaffold inventory and spec-table entries, delete the 13 dropped-ticket rows, and reword only the retained `260523-bug-worktree-local-index-missing` row to surface-neutral worktree-local-context propagation.
3. Move the epic, the eight parent-linked dashboard children other than `260523-bug-worktree-local-index-missing`, and the four ticket-specified unparented dashboard tickets to `ai-docs/tickets/.dropped/`; in the retained local-context ticket, perform the exact frontmatter/title/Background/Direction re-home described above so no active `parent:` targets the dropped epic.
4. Update `ai-docs/tickets/todo/260605-epic-ws-playbook-factory-pivot.md` to remove dashboard retention/compilation and deferred port-vs-remove statements; append a terminal `### Supersede:` section to `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` recording the archive-tagged removal decision.
5. Make the ticket-scoped historical reconciliations: correct only the stale dashboard-spec citation in `ai-docs/tickets/idea/260726-research-spec-planned-marker-management-cost.md`; convert the origin ticket's open decisions to outcomes and correct its branch/tag/revive history; add the four verified, bounded capability findings to `ai-docs/tickets/idea/260730-research-orca-plugin-ws-workflow-surface.md` without adding an unsettled design.
6. In the canonical workstation worktree, remove `ai-docs/ref/ws-dashboard-playwright.local.md` if present and remove dashboard dogfood/Playwright references from `ai-docs/_index.local.md` if present; these ignored-file changes remain outside the commit.

## Verification Plan
- Run `ws/spec_index.verify` after removing the dashboard spec domain.
- Run `ws/tickets.verify` on the retained, edited ticket and verify the active ticket graph has no `parent: 260514-epic-ws-web-dashboard-mvp` or other dangling parent after the moves.
- Confirm `_index.md` has no dashboard artifact or dropped-ticket row and retains only the reworded local-context ticket row.
- Run the ticket-defined residual searches: under `ai-docs/`, exempt `.plans/`, `.done/`, `.dropped/`, and the nine specified live/sweep-ticket stems; outside `ai-docs/`, search `ws-dashboard` while exempting historical `CHANGELOG.md`. Require no other residual hits.
- Verify the canonical worktree's ignored local docs are absent or contain no dashboard/Playwright guidance, recording that result separately because Git cannot validate ignored-file deletion.

## Escalations
- None.
