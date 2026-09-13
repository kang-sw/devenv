---
title: Codex hook-trust persistence mechanism (dogfood gap from the mailbox wake Codex adapter)
related:
  260913-feat-cross-session-mailbox-wake: prior-art — Phase 2's Codex Stop-hook adapter assumed a persistable hook-trust deployment step; this ticket investigates whether one currently exists
---

# Codex hook-trust persistence mechanism

## Background

While wiring `260913-feat-cross-session-mailbox-wake` Phase 2 (the Codex
`Stop`-hook adapter for cross-session mailbox wake), the ticket's own
decisions state: "Codex 0.154 gates hooks behind persisted hook trust
(`--dangerously-bypass-hook-trust` bypasses for automation), so a real
adapter must persist hook trust — a deployment step." Looking for that
deployment step (a CLI subcommand or `config.toml` key to trust a hook
source once, non-interactively) turned up nothing on Codex CLI 0.154.0:

- `codex hooks` / `codex hooks trust` are not subcommands: `error:
  unexpected argument 'hooks' found`.
- `codex --help` / `codex exec --help` document only
  `--dangerously-bypass-hook-trust` ("Run enabled hooks without requiring
  persisted hook trust for this invocation. DANGEROUS.") — no companion
  "make this persistent" flag.
- A fetched copy of the official config reference
  (`developers.openai.com/codex/config-reference`, redirects to
  `learn.chatgpt.com/docs/config-file/config-reference`) documents the
  `[hooks]` table and `features.hooks` boolean but no trust-persistence key.

This was not independently re-verified against the live docs site or an
interactive Codex session in this pass — running `codex exec` with
`--dangerously-bypass-hook-trust` for an isolated adapter smoke test was
denied by the sandbox's "Create Unsafe Agents" guard, so the mailbox wake
worker fell back to CLI-level and doc-derived evidence only. The open
question this ticket owns: does persisted hook trust exist as anything
other than an interactive first-run prompt (if that), and if so, what is
the exact mechanism (config key, keychain-style store, per-plugin manifest
flag)?

## Investigation

Needed: an interactive (non-`exec`, non-bypass) Codex session that
triggers a configured hook for the first time, observing whether Codex
prompts for trust and, if so, where it persists the answer (`config.toml`,
a separate trust-store file under `$CODEX_HOME`, or nowhere — re-prompting
every session). Re-check the current docs pages directly (not through a
redirect-following fetch) in case the reference has since been updated
with the missing key.

## Outcome Ledger

### Verified Findings

- Codex CLI 0.154.0 has no `codex hooks` subcommand.
- `--dangerously-bypass-hook-trust` is the only hook-trust-related flag
  documented in `codex exec --help` / `codex --help` on this version.
- The fetched config reference (unverified against the live page directly)
  shows no trust-persistence key alongside `[hooks]`/`features.hooks`.

### Confirmed Decisions

(none yet)

### Proposals

(none yet)

### Open Questions

- Does Codex persist hook trust anywhere today (interactive prompt +
  on-disk record), or does every non-`--dangerously-bypass-hook-trust`
  invocation with enabled hooks require a fresh trust decision?
- If a persistence mechanism exists, is it host/user-level, project-level,
  or plugin-scoped — and does the mailbox wake adapter's plugin-bundled
  `hooks/hooks.json` (`agents-plugin/hooks/hooks.json`) need any manifest
  field to become eligible for persisted trust at all, or is trust
  orthogonal to the plugin/inline distinction?

### Rejected Alternatives

(none yet)
