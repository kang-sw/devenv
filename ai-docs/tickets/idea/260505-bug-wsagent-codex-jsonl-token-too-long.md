---
title: wsagent Codex JSONL token too long
related:
  260505-bug-ws-agent-dogfood-timeout-tail-context: adjacent dogfood context-boundary issue exposed during forge-spec survey
---

# wsagent Codex JSONL token too long

## Background

During the forge-spec cold-start survey, a deep subquery over tickets failed
after the Codex backend emitted a very large JSONL event:

```text
read codex jsonl: bufio.Scanner: token too long
```

Normal `agents.tail` now truncates large diagnostic fields before returning them
to callers, so the model context did not receive the full payload. That fix does
not address the underlying runner failure: the wsagent Codex JSONL reader still
uses a scanner path that cannot tolerate a long single JSONL token.

## Phases

### Phase 1: Robust Codex JSONL Reader

Replace or harden the Codex JSONL reader so a single large backend event cannot
fail the whole agent call with `bufio.Scanner: token too long`.

The implementation should preserve existing event parsing behavior, avoid
pulling unbounded payloads into normal tool responses, and include a regression
test with a large single JSONL line. Debug/raw inspection can remain explicit,
but long backend events should not crash result collection.

