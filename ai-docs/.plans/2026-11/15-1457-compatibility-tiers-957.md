# Compatibility tiers 957

## Authorized outcome

Rewrite compatibility guidance to the owner-confirmed directional T0–T4 scheme. T0: old binaries ignore unknown additions and operate normally. T1: old binaries show unknown/default values without changing data. T2: old binaries may overwrite new data, but reopening with the new version restores it from retained raw/append-only data. T0–T2 permit patches. T3 blocks old binaries from running or connecting, requires a minor and owner approval. T4 cannot read old data and never ships; migration must lower it to T3 or below.

Whole-file rewriting that loses unknown data is not T2 without demonstrable reopen recovery. Preserve unknown data or classify the boundary as T3. Verdicts identify writer, reader, and reopen evidence with source citations. Document tool alias/removal/list-refresh mappings on the same scale.

## Scope

Update ai-docs/manuals/release-compatibility.md (definitions, scenario sheet, release rules), AGENTS.md compatibility and dependent approval wording, and every tier-defining/naming surface found in agents-plugin/rsrc, agents-plugin/skills, agents-plugin-wsflow mirrors, agents-plugin-pi, and agents-plugin-tool/internal/wsdoc/conventions. Follow matching manuals before edits, especially skill-authoring, wsflow-mirroring, and shipped-surface-boundary. Shipped guidance stays downstream-generic. Do not edit the live-summary 675 ticket; its existing worker owns its re-grade.

## Boundaries and verification

This is an approved convention/docs change, not authorization to alter runtime enforcement. Report semantic conflicts before expanding scope. Preserve unrelated artifacts. Use targeted existing documentation/mirror checks and diff checks, never full suites, real agents, shared daemon, installation or provider/network activity. No additional independent audit without approval. Commit one logical docs head with AI Context and this plan. Return changed paths, precise mappings, checks and known conflicts; lead owns integration and MAIN owns develop merges/swaps.
