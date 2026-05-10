---
title: agent backend failure diagnostics
spec:
  - 260505-agent-backend-failure-diagnostics
related-mental-model:
  - named-agent-runtime
completed: 2026-05-10
---

# agent backend failure diagnostics

## Background

Named-agent calls currently surface backend invocation failures as raw process
errors such as missing binaries, unsupported backends, login failures, or CLI
exit failures. That preserves the original failure but gives the caller little
guidance when a machine has a different usable backend available.

The workflow should not run separate model probes during registration or config
inspection. Backend invocation must remain tied to actual agent calls so users
are not surprised by background model execution. However, PATH lookup for known
backend binaries is mechanical and non-invasive, so failed calls can include a
bounded hint with local backend availability and explicit recovery actions.

## Decisions

- Do not infer login state or classify vendor-specific auth failures.
- Do not run model prompts, login probes, or separate backend health checks.
- Preserve the raw backend error as the primary diagnostic.
- Add hint text only after a backend invocation failure.
- PATH detection may check known backend binary locations for `codex`, `claude`,
  and `gemini`.
- Existing registered agents keep their stored backend/model; changing tier
  defaults affects future registrations only.

## Phases

### Phase 1: Invocation failure hints

When a named-agent backend invocation fails, include:

- agent name, tier, backend, and model
- the raw backend error text, bounded for context safety
- PATH-detected backend binaries for `codex`, `claude`, and `gemini`
- guidance that existing agents should be re-registered with explicit
  backend/model when switching
- guidance that future defaults can be changed with `config.agents_tier`

Success criteria:

- Missing `codex` binary failures mention detected alternatives without probing
  their login state.
- Backend CLI exit failures preserve the original exit code/output text.
- Unsupported backend failures receive the same recovery hint.
- Async call status/result/tail surfaces include the improved diagnostic text.

### Result (implementation) - 2026-05-05

Implemented backend invocation diagnostics in `wsagent`.

Named-agent backend call failures now return a bounded diagnostic that includes
the agent name, tier, backend, model, raw backend error, PATH-detected `codex`,
`claude`, and `gemini` binaries, and recovery guidance for re-registering
existing agents or changing future tier defaults.

`agents.call` now queues non-Codex backend agents so the worker records the
unsupported-backend failure in current-call state instead of failing before any
async diagnostic state exists. Actual non-Codex execution remains unsupported
until a backend runner is implemented.

Verification:

- `cd agents-plugin-tool && go test ./internal/wsagent`
