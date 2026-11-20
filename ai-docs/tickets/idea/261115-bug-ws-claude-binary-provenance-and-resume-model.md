---
title: "dashboard Claude 바이너리 출처 확인 및 재개 모델 선택 강화"
related:
  261115-feat-ws-dashboard-observed-session-model: complementary visibility of actual observed model and changes
  261114-feat-ws-whiteboard-topic-state-and-lead-nudges: implementation ordering after Phase 2, not a technical prerequisite
---

# Harden dashboard Claude binary provenance and resume model selection

## Background

MAIN reported a mid-session switch to `claude-opus-4-6` despite the expected
current `opus` mapping. Its investigation attributed the fresh mailbox-wake
launch to an ad-hoc daemon swap script placing `/opt/homebrew/bin` first in
PATH, selecting stale Homebrew Claude Code 2.1.84. A later 2.1.325 resume
reportedly restored the transcript model when no explicit model was passed.
The dashboard did not expose the selected executable/version or actual model.

These incident details and CLI alias/restore semantics are MAIN-reported, not
independently reproduced by this capture. In particular, an empty `rec.Model`
in the incident remains inferred. The source below establishes the launch
mechanisms, not the complete historical process-inheritance chain.

Owner (832) approved this idea capture. MAIN relayed that owner (830)=a removed
the stale Homebrew 2.1.84 installation and only `~/.local/bin/claude` 2.1.325
remains. This is reported remediation, not a fresh machine probe or a portable
binary-path default. Hardening remains useful when PATH changes again.

## Source Evidence

Read-only inspection of the current source found:

- `scripts/dev.py:222-235`, `run_daemon_start`: copies `os.environ`, adds
  resource/interpreter variables and passes that environment to `daemon-start`
  without replacing PATH. The path is root `scripts/dev.py`, not a script
  under `agents-plugin-tool/`.
- `agents-plugin-tool/internal/wsagent/claude.go:26-43,321-325`:
  `ClaudeBackend.Command` can select an explicit executable; when empty,
  launch calls `harnessCommand` with the fallback `claude`.
- `agents-plugin-tool/internal/wsagent/backend.go:430-456`:
  empty Command is resolved with `exec.LookPath`; on non-Windows an explicit
  Command passes through. An explicit override is existing prior art, not a
  new requirement to discover every possible installation.
- `agents-plugin-tool/internal/wsagent/runner.go:609-625,2040-2049,2166-2175`:
  record creation takes the profile model, with a nonempty lead-option override;
  worker and lead launch paths pass the record's Model and SessionID onward.
- `agents-plugin-tool/internal/wsagent/claude.go:119-126,176-183`:
  both argument builders add `--model` only for a nonempty Model, independently
  of `--resume`. No explicit empty-model resume fallback appears in this chain.
- No mandatory absolute-path policy, Claude resolved-path/version diagnostic,
  or Claude minimum-version gate was found in those scoped files.
  `scripts/dev.py:161-185` diagnoses ws-mcp version/hash, not Claude.
- `agents-plugin-tool/internal/wsagent/claude.go:335-337` distinguishes holder
  rejoin from a fresh launch: rejoin ignores the launch command. Do not describe
  changing fresh-launch arguments as replacing an already running holder.

CLI restore/alias semantics need exact-version evidence before implementation;
source comments alone do not establish the installed CLI's behavior. Ticket
queries for `claude binary` and `resume model`, plus the scoped native search,
found no direct duplicate; this is bounded discovery, not exhaustive absence.

## Decisions

- Owner-approved scope (832): resolve and record the Claude executable as an
  absolute path, include version logging, introduce a minimum-version
  refusal/warning policy, and surface the resolved binary/version in diagnostics.
  Relying silently on whichever `claude` a later PATH lookup finds is not the
  intended result. No minimum version or enforcement mode was selected here.
- Every fresh resume launch must explicitly pass `--model`, using the recorded
  value or a documented default when the record is empty. The exact empty-record
  default remains unsettled; do not infer one from this incident.
- Owner (831)=b rejects model pinning through `ANTHROPIC_*` environment variables.
  Alias resolution stays with the installed CLI; this ticket does not freeze
  `opus` to a concrete model ID or use environment pins as the remedy.
- Compatibility target: T0, with additive provenance/diagnostics and no required
  removal, protocol bump or non-additive store migration. Verify the actual
  cross-version surface before ready promotion; stop for owner approval if the
  design requires T1/T2 instead of silently treating it as T0.
- Implementation follows topic-state Phase 2 by MAIN's scheduling instruction.
  This idea capture authorizes no product change, activation or ready promotion.
- Observed-model visibility is complementary, not a replacement: link this issue
  from the observed-session-model ticket because that feature would expose a
  recurrence even when binary hardening does not prevent it.

## Constraints

- Readiness remains blocked on settling the minimum supported version, refusal
  versus warning behavior, and the documented empty-record model default through
  MAIN. No new decision IDs are assigned by this executor.
- Any downstream-facing text reads
  `ai-docs/manuals/shipped-surface-boundary.md`; tooling internals also read
  `ai-docs/manuals/release-compatibility.md` before implementation. Changes to
  prompts or conventions additionally read `ai-docs/manuals/skill-authoring.md`.
- Preserve existing explicit executable overrides and holder-rejoin semantics
  unless a separately approved decision changes them. Do not hard-code this
  machine's installation path as a shipped default.
- Verification uses isolated fixtures/fake executables and offline checks.
  No owner CLI launch, provider call, model/session mutation, installation,
  shared-daemon rebuild/restart/swap/smoke or port 29417 action is authorized.

## Phases

### Phase 1: Make binary provenance and fresh resume model selection explicit

After the readiness choices settle, implement the approved absolute executable,
version diagnostics, minimum-version policy and explicit resume `--model`
behavior. Retain installed-CLI alias resolution and the no-environment-pin
constraint. Reuse the existing explicit Command path where suitable rather than
introducing a competing launch mechanism.

Before execution, establish exact-version CLI evidence for the reported restore
behavior without assuming that settings, aliases and recorded transcript models
have identical precedence. Keep source-proven launch behavior separate from
historical incident inference.

Verify PATH collisions with distinct fake CLI paths/versions, an explicit
executable override, the selected old-version disposition, recorded and empty
resume-model cases, both Claude argument builders, and diagnostic provenance.
Cover existing-holder rejoin separately from a fresh resume; do not use a rejoin
as proof that changed argv was executed. Ensure no test relies on `ANTHROPIC_*`
pins or changes serialized/MCP shapes beyond an explicitly reviewed additive
contract. Readiness fact population must identify the concrete affected tests
and settle all remaining caller-visible behavior before dispatch.
