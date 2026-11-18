# Plan: hosted-lead-key

Downstream hotfix (owner 2026-11-18 13:05/13:07): on downstream machines a
"secure Claude Code"-style group policy blocks some hooks of the plugin's
claude-mod, so a hosted Claude lead never learns its daemon-minted session key
(and never receives the workflow manual the mod delivers). MCP calls work
there. Fix: the daemon injects the hosted lead's existing key into the Claude
lead's system prompt. Rides v0.50.7.

## Facts (develop 965b7a9f1; paths under agents-plugin-tool/internal/wsagent/)

- Lead StartSpec is built in runner_process.go:237-288 with no Instructions;
  the key is only in env `WS_HOSTED_SESSION_KEY` (runner_process.go:282-286),
  which the plugin's claude-mod adopts (agents-plugin/hooks/claude-mod/manual.ts:555-568).
- `claudeLeadArgs` (claude.go:102-132) has no system-prompt flag; children get
  `--append-system-prompt-file spec.Instructions` (claude.go:~176-193) via
  `composeInstructions` (runner_process.go:614-636 / ~717-742).
- Pi leads already inject the key (pi_lead_bridge.mjs:50-56, pi_lead.go:135-152).
- Tests currently ban the flag for leads: lead_claude_test.go:1182-1183,
  claude_lead_plugins_test.go:90.
- The key persists in `rec.WSKey` and StartSpec is rebuilt on every launch,
  including `--resume`, so an injected prompt is reapplied each launch.

## Required shape (agreed with the track/agent-v2 lead to keep their merge clean)

- Set the lead StartSpec.Instructions to the key text, written to a file the
  same way children's instructions are (composeInstructions-style), and have
  `claudeLeadArgs` pass `--append-system-prompt-file <path>` when Instructions
  is non-empty, mirroring the child path. Do NOT append a raw
  `--append-system-prompt` argv string. The track's Phase 1
  (ticket 261117-feat-ws-hosted-prompt-assembly) later swaps the content for
  the full keyed manual through a daemon-supplied render func.
- No `--mcp-config` change (MCP already works downstream).
- When the plugin's mod also runs, it delivers a duplicate key line; harmless.

## Content of the injected text

Host-neutral, short, modeled on the Pi bridge sentence: the session's ws
session key is `<key>`; pass it as `session_key` in every ws tool call; this
hosted session already has a key, do not mint a replacement (no ferrule, no
fresh-bootstrap sentinel); at session start and after any compaction, call the
workflow_manual tool with this key and follow it. Do not name a plugin
namespace (ws vs wsflow) in the text.

## Tests

- Update the banned-flag assertions so a lead passes
  `--append-system-prompt-file` (and still bans `--mcp-config`,
  `--strict-mcp-config`, `--system-prompt`, etc.).
- New test: a hosted Claude lead StartSpec/argv carries the file flag, the file
  contains the key and the workflow_manual instruction, and a resume launch
  carries it again.

## Constraints

- Compatibility tier: state it in the commit (expected T0: daemon-internal argv
  to the Claude CLI; no MCP/store/protocol surface change). Read
  ai-docs/manuals/release-compatibility.md and shipped-surface-boundary.md
  first (Implementation Conventions rows for agents-plugin-tool/internal/).
- Antivirus rule: no local go test / Go tools / gofmt / ws executables /
  launchers / Electron. Go runs only on w5090 via D:/sys_lock.sh remote.
- Temp only under D:/Temp/hosted-lead-key/.

## Done when

Commit(s) on impl/develop/hosted-lead-key; `go vet ./internal/wsagent/...` and
`go test ./internal/wsagent/...` pass on w5090 (accepted pre-existing reds only:
TestTopicStateBadgeResearchingMetadata, TestTopicStateBadgeClockAndReconciliation).
