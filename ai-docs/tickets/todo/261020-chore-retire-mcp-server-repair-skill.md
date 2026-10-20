---
title: Retire the mcp-server-repair skill in favor of the reconnect tool
related:
  261018-research-claude-mod-ws-mcp-reconnect: origin of the reconnect tool that replaces the skill on Claude
  261020-feat-claude-mod-lever-continue-flag: sibling; also edits reconnect.ts (continuation becomes a flag)
---

# Retire the mcp-server-repair skill in favor of the reconnect tool

## Background

`mcp-server-repair` is the skill an agent invokes when the ws MCP server is
down. Every lead skill ends with the pointer "If this call fails to connect,
run `/ws:mcp-server-repair`" (`/wsflow:...` in the wsflow tree), across the
ws, wsflow and Pi skill trees. The skill body routes to the Claude mod's
`request_mcp_reconnect` tool when listed, otherwise to a `ws-cli` /
`ws-mcp-launcher.py` CLI fallback and manual `/mcp` reconnect steps for the
user.

The reconnect tool (`agents-plugin/hooks/claude-mod/reconnect.ts`, registered
in `register.ts` `reconnectSetup`) now restores the server on Claude reliably.
The repair need has only ever arisen on Claude: Pi and other harnesses do not
lose the server this way. The user decided to retire the skill and let the
reconnect tool's own description carry the trigger.

## Decisions

- **Retire the skill.** Delete the `mcp-server-repair` skill and every pointer
  to it. The reconnect tool's description becomes the only trigger text, so it
  is strengthened.
- **Keep the CLI call surface.** `ws-cli` and `ws-mcp-launcher.py`'s
  `tools` / `call` subcommands stay as they are; only the skill prose that
  taught them goes. The user expects a later use for them.
- **Scope across trees.** Remove the skill from `agents-plugin/skills/`,
  `agents-plugin-wsflow/skills/` and `agents-plugin-pi/skills/`, with their
  `manifest.json` entries, the pointer tail in every lead SKILL.md, and the
  tests that list the skill or assert the pointer
  (`agents-plugin/tests/test_skill_dispatch_contracts.py`,
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`,
  `agents-plugin-pi/test/skills-dir.test.ts`,
  `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go`).
- **Docs.** Update `ai-docs/manuals/wsflow-mirroring.md`, which names the
  skill as the inline-body exception. `CHANGELOG.md` history and `.done/`
  tickets stay untouched.
- **Failure text inlines manual recovery.** `failureText` tells the agent to
  ask the user to re-enable the server from `/mcp`, run `/reload-plugins`, or
  restart the session. Rejected: pointing at another document (after
  retirement nothing else carries those steps).
- **Trigger-first description.** `reconnectDescription` opens with its
  trigger ("Call this first when the <plugin> MCP server's tools have left
  your tool list or a call reports it not connected"). Rejected: today's
  mechanism-first order (the description is now the only trigger text an
  agent sees).
- **No replacement on Codex or Pi.** Harnesses other than Claude never need
  repair (user decision).
- **Drop the spec ticket.** `260728-chore-spec-mcp-server-repair-unspecified`
  is dropped in this ticket's closing commit; its subject is retired.
- **wsflow mirror.** Every `agents-plugin/hooks/claude-mod/` edit is mirrored
  into the byte-identical copy under `agents-plugin-wsflow/hooks/claude-mod/`
  (`ai-docs/manuals/wsflow-mirroring.md`).

## Phases

### Phase 1: Remove the skill and strengthen the reconnect tool

Delete the skill and its pointers per Decisions, rewrite `failureText` and
`reconnectDescription` in `reconnect.ts`, and update the tests that assert
them (`reconnect.test.ts`, `reconnect-engine.test.ts`) in both claude-mod
copies.

Verification: no live reference to `mcp-server-repair` remains outside
`CHANGELOG.md`, `ai-docs/tickets/.done/`, `ai-docs/tickets/.dropped/` and
`ai-docs/.old/` (`grep -rn mcp-server-repair`); the Python plugin tests, the
wsflow bundle tests, `npm test` in `agents-plugin-pi`, `go test ./...` in
`agents-plugin-tool`, and the claude-mod tests pass.
