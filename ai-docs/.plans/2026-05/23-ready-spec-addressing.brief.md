# Brief: ready spec addressing

## Intent

Redefine ready-ticket spec gating so `ready/` means spec-addressed implementation work, not mandatory pre-implementation planned spec text.

## Scope Boundary

Update workflow skill and convention text that currently routes ready promotion through mandatory `lead-write-spec`/planned marker creation. Keep existing active `ready/` tickets and existing `🚧` spec entries grandfathered. Do not create a ticket for this direct edit.

## Caller-Visible Contract

`lead-proceed` routes `todo/` tickets and ticket-needed inline targets through `lead-write-ticket` before implementation. `lead-write-ticket` owns ready promotion and passes ready work when the ticket identifies a spec surface through existing `spec:`/`spec-remove:` stems or a ticket-local `## Spec Impact` section. `lead-write-spec` runs only for contract-first planned spec cases or explicit spec authoring.

## Contract Instructions

Update full ws skills and matching wsflow mirrored skills. Preserve wsflow notation and avoid full-ws references in wsflow distributed skill text. Update bundled `spec-conventions` and `ticket-conventions`; runtime `ws/convention.read` uses `agents-plugin-tool/internal/wsdoc/conventions/`.

## Integration Test Instructions

Run spec index verification, wsflow package tests, and targeted Go tests for wsdoc/MCP convention embedding. Run a fresh-reader audit on changed skill text and a downstream consistency sweep across affected skill/spec/mental-model surfaces.

## Implementation Strategy Decisions

- Keep the ready gate as spec-addressing, not no-gate.
- Do not make `🚧` entries the normal ready promotion output.
- Define `🚧` as a contract-first planned spec path: use it only when planned behavior must be visible and stable before implementation begins.
- Existing `ready/` tickets and existing `🚧` entries remain valid without migration.

## Rejected Alternatives

- Removing ready-level spec discipline entirely: would make post-implementation doc closeout infer too much from source and conversation.
- Keeping mandatory planned markers for every ready promotion: makes compact behavior specs carry imagined future behavior and turns `write-spec` into ceremony.
- Requiring downstream bootstrap migration: unnecessary when bundled skills and conventions can reinterpret the gate for new promotions.

## Approach

- Change `lead-proceed` routing from `write-spec -> write-ticket` to `write-ticket` for todo and ticket-needed inline targets.
- Change `lead-write-ticket` spec checks to accept `## Spec Impact` as ready addressing and invoke `lead-write-spec` only for contract-first cases.
- Change `lead-write-spec` planned marker judgment to require contract-first planned behavior.
- Mirror the same workflow intent in wsflow skills.
- Update specs and mental models to document the new interpretation.

## Constraints

- Keep skill rules short, falsifiable, and separated from soft judgments.
- Do not remove `🚧` marker support.
- Do not edit active tickets or existing spec markers as part of this change.
- Keep downstream `spec-gated` wording compatible by defining it as spec-addressed readiness.

## Out of scope

- Ticket migrations.
- Existing `🚧` cleanup.
- Runtime parser changes for a structured `spec-impact` frontmatter field.
- New MCP tools or schema changes.

## Details

`## Spec Impact` is a ticket body section for ready addressing when no current stem exists or when implementation should determine exact spec wording. It names the target spec area, expected caller-visible change, and whether a contract-first planned spec is required.

## Verification Contract

- `ws/spec_index.verify()` passes.
- `go test ./internal/wsdoc ./internal/mcp` passes under `agents-plugin-tool`.
- `python3 -m unittest discover agents-plugin-wsflow/tests` passes.
- Fresh-reader audit finds no material execution ambiguity in changed skill text.

## References

- `ai-docs/mental-model/documentation-system.md` - ready/spec/ticket convention model.
- `ai-docs/mental-model/workflow-skills.md` - proceed/write-ticket/write-spec routing model.
- `agents-plugin/skills/lead-skill-authoring/SKILL.md` - skill wording and audit rules.
- `ai-docs/ref/wsflow-mirroring.md` - mirrored skill requirements.
