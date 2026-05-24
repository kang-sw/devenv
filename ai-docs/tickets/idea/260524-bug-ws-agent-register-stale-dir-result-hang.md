---
title: Investigate ws agent stale registration and post-test result hang
related:
  260517-bug-ws-agent-empty-result-after-tool-use: adjacent no-result behavior after long tool-use agent runs
related-mental-model:
  - named-agent-runtime
---

# Investigate ws agent stale registration and post-test result hang

## Background

During the dashboard icon chrome implementation dogfood, `agents.register` for
`implementer` first failed while trying to reset a stale agent directory:

```text
reset agent directory: unlinkat .../agents/implementer: directory not empty
```

After `agents.erase`, a batched register/call attempt also raced so the call
observed a missing `agent.json`. A later separate `agents.call` did start and
successfully ran build plus browser acceptance, but `agents.result` timed out
after ten minutes with the agent still marked `running` and no final result
available. The lead cancelled the agent and continued from the produced
worktree diff.

This likely overlaps with the existing no-result-after-tool-use investigation,
but the stale-directory reset and register/call ordering behavior may need
separate runtime handling.

## Observed Questions

- Should `agents.register` be able to atomically reset a non-empty stale agent
  directory, or should it return a recovery instruction that does not require
  manual erase?
- Should a same-stdio batched `agents.register` followed by `agents.call` be
  ordered strongly enough for the call to see the newly written `agent.json`?
- Why can a Codex-backed worker finish substantial tool work, including passing
  verification commands, but remain `running` without a final result?
- Should `agents.result` surface recent successful tool evidence when the final
  assistant result is missing, or should this remain a caller recovery concern?

## Evidence

- First failure: `agents.register(name: "implementer")` returned
  `reset agent directory ... directory not empty`.
- Recovery: `agents.erase(name: "implementer")` returned `erased`.
- Batched retry: `agents.call` returned `read agent ... agent.json: no such
  file or directory` while `agents.register` returned `implementer`.
- Later separate call: agent ran, edited dashboard frontend, ran
  `npm run build` and `npm run test:browser` successfully, then never emitted a
  final result before the ten-minute `agents.result` timeout.
