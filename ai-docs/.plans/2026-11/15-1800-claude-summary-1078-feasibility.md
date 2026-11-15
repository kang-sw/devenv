# Claude Code summary producer feasibility — owner 1078

## Authorized outcome

Read-only feasibility investigation, with a short tracked reference note at `ai-docs/ref/claude-code-reasoning-summary-feasibility-261115.md`. No producer or runtime implementation. Answer MAIN's four questions: live exposure and timing; usable thinking versus redacted/empty/summarized/signature-only content on the current model; required Go source/admission changes and directional compatibility; recommendation and evidence gaps.

## Evidence and privacy

Inspect current repository Claude mod/hooks, hosted Claude mapping, Pi summary contract and Go receiver. MAIN explicitly authorized read-only examination of this machine's real `~/.claude/projects` transcripts for the current model. Use bounded structural samples and aggregate presence/shape/length/timestamp facts. Do not copy raw thinking, prompts, signatures, secrets, private paths or transcript contents into the note or reports. Distinguish explicit public summary from raw/private thinking; a nonempty thinking block is not automatically the approved summary source.

Use locally installed/cached official API or hook documentation and source where available. No network/install/download, credentials, live provider/Claude/Pi launch, owner-session mutation, shared daemon/29417, screenshots, monitors or timers. Existing transcript timestamps alone do not prove write timing: mark unsupported inferences and gaps. Do not inspect broad unrelated histories.

## Existing verified limits

675 currently supports Pi 1.5.2 built-in OpenAI Responses/openai-codex Responses only. Native Codex and external/hosted Claude are not producers. `agents-plugin-pi/src/reasoning-summary.ts:93–107,208–218`; `agents-plugin-tool/internal/mcp/summary_owner.go:16–18`; `internal/mcp/live_snapshot.go:495–496`; `internal/wsagent/runner_summary.go:33–42`. Claude mod `agents-plugin/hooks/claude-mod/dashboard.ts:374–395,1579–1590` tracks a thinking phase and ordinary reply text. `internal/wsagent/claude.go:1226–1277` has no summary producer. Restart/re-registration cannot enable Claude coverage. Do not re-derive this whole diagnosis; trace the candidate Claude input and missing admission only.

## Deliverable and bounds

Read applicable project documentation and authoring guidance before writing the reference. Use concise English, VERIFIED/INFERRED/UNVERIFIED labels, source file:line citations, inspected version/model identifiers and a narrow sample summary. Avoid tracking machine-local paths or raw evidence. Keep the note short, with four answers and a recommendation conditional on unresolved source semantics/timing. Read the directional release manual before grading; a new source is T0 only if legacy callers/writers/reopen remain unchanged and new callers never drop intent. Explain why admitting a source is not merely adding a validation string.

Run documentation whitespace checks, read the note back, and commit only the reference plus this plan on `research/claude-reasoning-summary-1078`, with AI Context. Do not edit tickets, code, manifests or shared workflow behavior. No independent audit/review or additional child is requested; return concise findings, gaps, commit and quiescence. EXEC2 will read the note and mail MAIN; MAIN alone decides whether the owner approves implementation.
