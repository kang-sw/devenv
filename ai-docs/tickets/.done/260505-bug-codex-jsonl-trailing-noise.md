---
title: Codex JSONL trailing noise breaks agent results
spec:
  - 260505-codex-jsonl-trailing-noise-tolerance
related-mental-model:
  - named-agent-runtime
completed: 2026-05-10
---

# Codex JSONL trailing noise breaks agent results

## Background

A Windows persistent-agent smoke test produced a valid Codex `agent_message` in
stdout, then appended localized non-JSONL process-control output to the same
stdout stream. The named-agent parser treated that trailing line as fatal:

```text
parse codex jsonl: invalid character '¼' looking for beginning of value
```

The agent response was visible through `agents.tail`, but `agents.result`
reported the call as failed because the parser discarded the completed result.

## Phases

### Phase 1: Tolerate trailing stdout noise after completion

Update the Codex JSONL parser so trailing non-JSONL stdout after both
`thread.started` and final `agent_message` have been observed does not fail the
call. Keep non-JSONL output before completion as a parse failure.

Success criteria:

- Valid `agent_message` followed by non-JSONL process output returns the message.
- Non-JSONL output before the session/message pair still fails parsing.
- Existing large JSONL line support remains intact.

### Result (implementation) - 2026-05-05

Implemented parser tolerance for trailing non-JSONL stdout after the completed
Codex result is already available.

Verification:

- `cd agents-plugin-tool && go test ./internal/wsagent`
