---
ticket: 260506-feat-ready-ticket-status
---

# Ready Ticket Status Implementation Brief

## Goal

Split ticket lifecycle semantics so `todo/` means accepted backlog and `ready/`
means spec-gated implementation queue.

Implement the complete behavior across docs/conventions, bootstrap migration,
runtime ticket discovery, Git ticket move handling, workflow skills/prompts, and
this repository's own ticket layout.

## Required Semantics

- Active statuses are `idea/`, `todo/`, and `ready/`.
- `idea/` is rough capture before triage.
- `todo/` is accepted backlog with recoverable ticket intent; it is not an
  implementation queue.
- `ready/` is the spec-gated implementation queue.
- `.done/` and `.dropped/` remain archived statuses and are omitted unless a
  tool explicitly opts into them.
- `todo/` tickets may include `spec:` links when known in advance; this is a
  recoverability hint and promotion candidate.
- Non-`epic`, non-`research` tickets entering `ready/` require spec linkage.
- `ready/` only implies spec gate completion. Plans and skeletons remain
  downstream `lead-proceed` decisions.
- `## Ticket Queue` lists `ready/` work only after migration.
- Avoid vague phrases such as `todo-or-higher`; use exact status semantics.

## References

- Ticket: `ai-docs/tickets/todo/260506-feat-ready-ticket-status.md`
- Specs:
  - `ai-docs/spec/documentation-system.md`
  - `ai-docs/spec/workflow-skills.md`
  - `ai-docs/spec/mcp-tools.md`
  - `ai-docs/spec/claude-compatibility.md`
- Mental models:
  - `ai-docs/mental-model/documentation-system.md`
  - `ai-docs/mental-model/workflow-skills.md`
  - `ai-docs/mental-model/git-workflow-tools.md`
  - `ai-docs/mental-model/claude-compatibility.md`
- Runtime areas:
  - `agents-plugin-tool/internal/wsdoc/`
  - `agents-plugin-tool/internal/wsgit/`
  - `agents-plugin-tool/internal/mcp/`
  - `agents-plugin-tool/cmd/ws-mcp/`
- Workflow text:
  - `agents-plugin/skills/lead-*`
  - `agents-plugin/skills/lead-bootstrap/AGENTS.template.md`
  - `agents-plugin-tool/internal/wsprompt/prompts/`
  - Claude compatibility text under `claude-plugin/`

## Implementation Scope

### Docs And Conventions

- Update bundled and compatibility convention docs:
  - ticket lifecycle is `idea/` -> `todo/` -> `ready/` -> `.done/` or
    `.dropped/`;
  - `idea/` tickets may omit `spec:`;
  - `todo/` tickets may include optional `spec:` links;
  - non-`epic`, non-`research` `ready/` tickets require spec linkage;
  - `ready/` is the implementation queue.
- Convert planned spec callouts to implemented body prose after code and skill
  behavior land.
- Update mental models for status parsing, queue semantics, and workflow gates.

### Bootstrap Migration

- Add a new `AGENTS.template.md` version after v0034.
- Include `ready/` in the initial ticket directory sketch.
- Add an idempotent migration rule:
  - create `ai-docs/tickets/ready/` if absent;
  - move existing non-`epic`, non-`research` implementation-ready tickets from
    `todo/` to `ready/` with `git mv`;
  - keep `epic`, `research`, missing-spec, and uncertain tickets in `todo/`;
  - recreate/keep an empty `todo/` directory when needed;
  - treat `ready/` as the implementation queue;
  - promote scoped `idea/` tickets to `todo/` through `ws:lead-discuss`.

### Runtime Discovery And Git

- Add `ready` to ticket status normalization/ranking.
- Default active ticket scans must include `idea`, `todo`, and `ready`.
- `tickets.list`, `tickets.find`, `tickets.status`, CLI help, MCP schemas, and
  reference docs must accept/report `ready`.
- `project_tree` should render `ready` tickets before `todo` and `idea`.
- `git.commit` ticket move expansion must recognize `ready` paths.

### Workflow Skills And Prompts

- `lead-write-ticket`:
  - accepted actionable backlog starts in `todo/`;
  - queue entry is added only for `ready/`;
  - spec gate fires only when a non-`epic`, non-`research` action creates or
    moves a ticket into `ready/`;
  - `todo/` `spec:` links are optional recovery hints.
- `lead-discuss`:
  - `idea/` -> `todo/` is triage and does not require spec creation;
  - `todo/` -> `ready/` is spec-gated promotion and adds queue entry.
- `lead-proceed`:
  - `ready/` tickets are direct implementation targets;
  - `todo/` tickets route through ready promotion before implementation.
- Update `lead-write-spec`, forge/reconstruction flows, survey prompts, and
  Claude compatibility skill text so `ready/` is the planned-entry ticket
  status, not `todo/`.

### Repository Self-Migration

- Add `ai-docs/tickets/ready/`.
- Move only clearly implementation-ready current `todo/` tickets to `ready/`.
- Keep epic/research/ambiguous backlog in `todo/`.
- Update `_index.md` so:
  - status directories mention `idea/`, `todo/`, `ready/`, `.done/`,
    `.dropped/`;
  - active ticket table includes `ready`;
  - `## Ticket Queue` lists `ready/` work only.
- The current ticket may move to `ready/` when the new rules are in place.

## Verification

Run at minimum:

- `cd agents-plugin-tool && go test ./...`
- `cd agents-plugin-tool && scripts/smoke-ws-mcp.sh ..`
- `python3 -m py_compile agents-plugin/**/*.py` only if Python files change
- `ws/spec_index.verify`
- `git diff --check`

Add or update targeted Go tests for ticket discovery, project tree rendering,
and Git ticket move expansion.
