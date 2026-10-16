# Claude mailbox push probe mod

Probe artifact for `261016-research-claude-mod-mailbox-active-push`, not
shipped code. A Claude Code plugin mod (plugin of function hooks) that gives the
ws cross-session mailbox pi-style active push: the agent binds its session key
through the mod-registered `mailbox_watch` tool, the mod loops the existing
one-shot `ws-mcp mailbox wait` as a peek-only signal, drains through the ws MCP
`mailbox_recv` tool via `$.tool.call`, and submits the drained batch as one
prompt with `$.prompt.submit`.

Written against Claude Code 2.1.293 mod types; validated, tested, and
live-probed on 2.1.300.

## Run it

- Copy this folder somewhere outside the repo and start a session with
  `claude --plugin-dir <folder>` (or drop it into a session's dev-mods folder
  after loading the `plugin-authoring` skill, which asks to enable hot reload).
- `hooks/register.ts` hardcodes `LAUNCHER` to the installed ws plugin's
  `bin/ws-mcp-launcher.py` on the probe machine; adjust it (and the matching
  expectation in `tests/watcher.test.ts`) for another machine.
- `tsconfig.json` extends `.claude-plugin/types/tsconfig.json`, which the
  engine writes into the mod folder at load time; the generated types are not
  copied here.
- Checks: `claude plugin validate <folder>`, `claude plugin test <folder>`,
  and `npx tsc -p <folder>` after the engine has laid the types.
