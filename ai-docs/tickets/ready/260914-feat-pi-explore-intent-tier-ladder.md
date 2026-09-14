---
title: "Make Pi Explore intent modes express investigation cost"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: cd9cff3313fa6853
sage-review-completeness-reviewed: cd9cff3313fa6853
---

# Make Pi Explore intent modes express investigation cost

## Background

Pi Explore currently mixes evidence-source labels (`code-search`, `history-search`, `docs-search`, `web-search`) with reasoning-shape labels (`diagnosis`, `comparison`, `synthesis`). The mode selects a model tier, appears in the child task prompt, and persists in ownership and sidecar metadata, but does not change tools, delegation depth, timeout, or execution budget. Because reasoning shape is independent of difficulty, the current names make expensive selection unpredictable: a casual `synthesis` currently selects `large` even when bounded investigation is sufficient.

Replace the public schema with a cost-and-rigor ladder whose names let the lead select a tier from the intended investigation burden. Preserve old metadata compatibility without continuing to advertise the legacy labels.

## Decisions

The public Explore intent-mode schema is exactly:

```text
lookup                  -> small
search                  -> small (default when mode is omitted)
investigation           -> medium
deep-research            -> large
high-assurance-research  -> xlarge
```

The modes mean:

- `lookup`: retrieve one fact whose likely location or identity is already known.
- `search`: perform bounded evidence discovery or tracing without requiring cross-source judgment.
- `investigation`: gather and connect bounded evidence; diagnosis, comparison, and synthesis are expressed in the query rather than as separate public modes.
- `deep-research`: pursue an ambiguous, difficult question through multiple sources and iterative hypothesis testing.
- `high-assurance-research`: apply raised evidentiary standards to a high-stakes conclusion through cross-source corroboration, adversarial challenge, repeated hypothesis testing, and explicit coverage and gap accounting.

`high-assurance-research` does not promise completeness, conformance to a formal assurance or certification standard, or an answer free of unresolved gaps.

Former public modes remain accepted by internal persisted-record, ownership, sidecar, and resume paths but are hidden from the public tool schema:

```text
code-search    -> small
history-search -> small
docs-search    -> medium
web-search     -> medium
diagnosis      -> medium
comparison     -> medium
synthesis      -> medium
```

Preserve the existing persistence-only, read-boundary migration aliases exactly:

```text
simple -> code-search
deep   -> synthesis
```

`simple` and `deep` are neither fresh-call API aliases nor valid serialized output. Reading historical persistence canonicalizes them once, and subsequent serialization must not write them again. Fresh callers use the canonical public modes. Existing records using former public modes retain those stored labels and remain resumable.

Rejected alternatives:

- Keep `diagnosis`, `comparison`, and `synthesis` public: these describe task shape rather than investigation cost and overlap each other.
- Use `systematic-review` for xlarge: it invites ordinary review use and carries methodology/completeness expectations that do not match the runtime contract.
- Use `high-assurance-deep-research`: `deep` is redundant, and embedding the full large-tier name inside the xlarge name makes fuzzy or substring selection less safe for the most expensive tier.
- Use `triangulation`: it accurately names one method but does not signal xlarge cost or the full evidentiary contract.

## Constraints

- This is a public schema and observable workflow behavior change; preserve compatibility for previously persisted Explore records while narrowing what new callers are offered.
- Keep one shared read-only Explore capability profile. The mode ladder changes intent, tier resolution, prompt text, metadata, and default selection only; it does not introduce mode-specific tools, network authority, depth, timeout, or budget branches.
- Keep task-shape instructions in the user query. Do not add separate public diagnosis/comparison/synthesis selectors elsewhere.
- Read `ai-docs/manuals/skill-authoring.md` before changing the Explore guide or any other agent-facing prompt text, and apply its invariant checklist to changed constraints.

## Verification

- Assert the exact public schema enum and `search` omission default.
- Assert every canonical mode resolves to its specified Pi tier, including `xlarge`.
- Assert former public labels are absent from the public schema yet accepted by persisted sidecar, ownership, and resume normalization.
- Assert historical persistence-only inputs normalize `simple → code-search` and `deep → synthesis`, and neither value is serialized again.
- Assert a new canonical mode is propagated into task-prompt text and persisted metadata.
- Assert all modes retain the same read-only Explore tool/network profile and delegation limits.
- Run the focused Pi Explore and sidecar tests plus the package test suite required by the affected modules.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/process-role.ts, agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/agent-storage.ts, agents-plugin-pi/src/agent-sidecar.ts |
| scope.surface | public-interface | agents-plugin-pi/src/spawner.ts exposes the public explore tool schema |
| scope.new_public_symbol | no | existing explore tool and mode parameter change their enum values only |
| scope.new_type_contract | yes | agents-plugin-pi/src/process-role.ts ExploreMode and the public mode enum change |
| scope.test_surface | existing | agents-plugin-pi/test/persistent-explore.test.ts, agents-plugin-pi/test/agent-storage.test.ts, agents-plugin-pi/test/agent-sidecar.test.ts, agents-plugin-pi/test/web-capability.test.ts |
| complexity.reuse_points | confirmed | agents-plugin-pi/src/process-role.ts shared mode mapping and normalization are consumed by schema, ownership, and sidecar paths |
| complexity.side_effect_risk | high | mode selects the resolved Pi tier and persists across researcher continuation |
| risk.correctness | high | public selection, child environment, ownership, sidecar parsing, and resume must agree |
| risk.fit | high | the confirmed public cost ladder replaces the current evidence-shape enum |
| risk.test | high | exact schema, tier mapping, compatibility, prompt/metadata propagation, and identical capability coverage require regression tests |
| risk.security_or_contract | high | public mode compatibility and resumability of stored researcher records are explicit contracts |

## Phases

### Phase 1: Implement the compatibility-aware intent tier ladder

Separate the canonical schema-visible modes from the internal accepted legacy-mode set. Update mode typing, tier resolution, omission default, schema description, persistence validation, and focused tests as needed to implement the decisions above. Keep the change local to the Pi Explore contract and avoid unrelated worker or runtime behavior changes.
