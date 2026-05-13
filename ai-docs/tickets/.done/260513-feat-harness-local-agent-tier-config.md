---
title: Harness-local agent tier configuration
spec:
  - 260513-harness-local-agent-tier-config
related-mental-model:
  - named-agent-runtime
completed: 2026-05-13
---

# Harness-local agent tier configuration

## Background

`config.agents_tier` currently updates the alias default plus a backend-keyed
alias entry. That makes `backend` do double duty: it is both the execution
backend and the alias-table key to update. In a detected Claude MCP session,
the prefilled `model_aliases.<tier>.claude` entry wins before `default`, so a
call that sets `core` to a Codex model can update `default` and `codex` while
leaving Claude registrations on `claude/sonnet`.

The intended caller model is host-local: when a user configures `core` from a
Claude session, that call should change what `core` means in that Claude
session unless the caller explicitly targets another harness.

## Decisions

- Add an optional `harness` selector to `config.agents_tier` and the CLI.
- When `harness` is omitted in MCP, target the detected MCP session harness when
  available.
- When neither explicit nor detected harness is available, target `default`.
- Keep `backend` as the execution backend value stored in the alias mapping.
- Do not change alias resolution precedence; configured harness keys should keep
  winning over `default`.

## Phases

### Phase 1: Harness-local alias updates

Update config persistence, MCP schema/handler, CLI flags, runtime/reference
documentation, and tests so `config.agents_tier` writes
`model_aliases.<tier>.<target-harness>` for explicit or detected harnesses.
Unknown/no-harness callers continue to update `default` for compatibility.

Acceptance checks:

- A Claude-harness MCP call without `harness` can set `core` to `codex/gpt-5.4`
  and a later `agents.register(model: "core")` in that harness resolves to
  `codex/gpt-5.4`.
- A CLI call with `--harness claude --backend codex --model gpt-5.4` writes the
  Claude alias entry.
- A CLI call without `--harness` still updates the default alias mapping.
- Existing explicit-backend registration protections remain intact.

### Result (08f44bb) - 2026-05-13

Implemented `config.agents_tier` harness targeting across wsconfig, MCP, and
CLI surfaces. MCP calls now use explicit `harness`, then detected session
harness, then `default`; CLI calls can pass `--harness` and otherwise update
`default`. Added coverage for Claude-harness `core` resolving to
`codex/gpt-5.4`, CLI harness writes, default fallback behavior, and
explicit-backend mismatch protection.
