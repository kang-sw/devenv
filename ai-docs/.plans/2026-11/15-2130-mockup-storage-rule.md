# Mockup storage rule

## Authority and scope

MAIN assigned one owner-approved documentation change (1145=ok) for v0.50.0. The complete approved brief is `/tmp/mockup-path-brief.md`; its approved-rule section is the task authority. Work on `docs/mockup-storage-rule`, based on local develop, in this pooled full worktree. Target head handoff by 22:05 KST on 2026-11-15. The lead holds the lease until MAIN reports the merge.

## Intended changes

- Add one concise rule near the whiteboard panel payload guidance in `agents-plugin/rsrc/whiteboard/whiteboard.md`: save text mockups (HTML/SVG) at `ai-docs/assets/mockups/YYMMDD/<topic name>/`, with stable creation date and the board topic name before a ticket exists; multi-state HTML exposes reproducible state selection; the saving lead includes the source in its next commit. PNG renders are reproduced from HTML, not stored there; panel screenshots retain their existing location.
- Update `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md` and its bundled copies: approved text mockups are linked from that mockup location, not copied; binary references retain `<ticket root>/assets/<stem>/`; existing asset folders are not migrated.
- Mirror the shared shipped changes into wsflow under the mirroring manual. Update resource hashes only when required by existing packaging contracts.

## Boundaries and verification

Read the complete applicable manuals before editing: skill-authoring, wsflow-mirroring, shipped-surface-boundary, and release-compatibility for internal tooling paths. Apply the authoring invariant checklist and positive concise wording. Preserve all other semantics and do not migrate existing assets. Compatibility is T0, documentation only; record that in commit AI Context. No protocol, runtime, MCP, store-shape, skill or agent changes.

Discover and run focused document/resource/manifest drift tests covering the changed paths, offline with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. Do not run the whole Go suite or the entire dashboard package. No network, providers, credentials, downloads, installations, shared daemon build/swap/reload, push, branch integration or unrelated process actions.

Commit one logical documentation unit including this plan. Report exact head, paths, verification commands/results, compatibility, and any gaps. Return at a clean checkpoint; MAIN alone merges. Do not release the worktree. Independent fresh-read audit is optional and has not been requested or run.
