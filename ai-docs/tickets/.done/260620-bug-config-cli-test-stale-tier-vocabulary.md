---
title: config CLI test still expects legacy light/core/deep tier keys
related:
  260620-bug-ws-tier-vocabulary-split-undocumented: dogfood follow-up from the merged tier vocabulary collapse
completed: 2026-06-20
---

# config CLI test still expects legacy light/core/deep tier keys

## Background

Codex dogfooding after merge `2601bc8a` found `go test ./...` failing in
`TestConfigCLICommandsReturnConfigView`. The runtime config path now returns
canonical capability tier keys (`small`, `medium`, `large`, `xlarge`) and keeps
`light`/`core`/`deep` as input read-compat synonyms, but the CLI test still
asserts the old three-key output shape and reads `Tiers["light"]`,
`Tiers["core"]`, and `Tiers["deep"]`.

The test should keep exercising legacy synonym inputs while asserting canonical
capability-keyed output.

## Result

Updated the CLI config test to assert canonical `small`/`medium`/`large`/`xlarge`
output while continuing to exercise `light` and `core` as read-compat inputs.
Focused and full test runs pass after the change.
