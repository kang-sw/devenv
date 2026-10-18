---
title: A hot reload onto the auto-bind mod keeps the old mailbox_watch tool advertised
related:
  261018-feat-claude-mod-lead-key-adoption-mailbox-auto-bind: made the mailbox_watch lever unregistered where the manual feature owns the key
---

# A hot reload onto the auto-bind mod keeps the old mailbox_watch tool advertised

## Observation

Dogfood 2026-10-18 (Claude Code 2.1.302, macOS): a session that started on the
pre-auto-bind ws mod (which registered `mailbox_watch` through
`$.tool.register`) and then ran `/reload-plugins` onto the merged auto-bind mod
(832fae784) still advertises `mcp__ws__mailbox_watch` on every API request,
including the first request after a lever compaction (transcript request
shapes before and after compaction). The new module takes the gate-open path
in `register.ts` (`if (await gate) ... return` before `$.tool.register`), so
it never registers the tool and its `tool.call` hook's `toolName` stays unset.
A fresh headless session in the same project lists only
`mcp__ws__request_compaction` and `mcp__ws__request_mcp_reconnect`.

Inferred: a tool registration made by the previous module generation
survives a module hot reload; nothing unregisters it.

## Impact

Only sessions upgraded in place by a reload. The model may call the stale
tool; with `toolName` unset the mod's hook passes the call to `next(e)`,
whose behavior for an orphaned plugin tool is unprobed.

## Open questions

- Does the mod API offer an unregister / dispose for `$.tool.register`, or a
  module-unload hook where the old generation could drop its tools?
- Otherwise, should the gate-open path keep a registration that denies with a
  pointer ("push is bound automatically; no call needed")?
