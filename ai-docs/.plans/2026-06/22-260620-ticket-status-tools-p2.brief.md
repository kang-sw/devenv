# Brief: 22-260620-ticket-status-tools-p2

## Intent

Rewire scattered ticket-transition directives so `tickets.close` and
`tickets.move` are the canonical transition path; `git mv` becomes the named
fallback. Four files are edited; the rsrc manifest and wsflow mirror are
regenerated after all edits.

## Scope Boundary

Phase 2 of `260620-feat-ws-ticket-status-transition-tools`. Scope is limited
to the six transition-directing sites in four source files and the regeneration
steps. Leave all other content in those files untouched. Do not change
`lead-forge-spec` — its `git mv` references are spec-file archiving operations,
not ticket transitions.

Do not edit ticket files, spec files, or any file not listed below.

## Caller-Visible Contract

After this change, callers reading `ticket-conventions`, `lead-workflow-manual`,
`lead-write-ticket`, or `lead-discuss` see `tickets.close`/`tickets.move` as
the canonical mechanism for ticket status transitions, with native `git mv`
named as an explicit fallback. No caller-facing tool API changes.

## Contract Instructions

### Files to edit

**1. `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md`**

Current line 16:
```
- Move tickets with `git mv`; no cross-link updates needed.
```
Replace with:
```
- Move tickets with `tickets.close(stem, status)` (to done/dropped) or
  `tickets.move(stem, to)` (idea/todo/ready) MCP tools; use native `git mv`
  as fallback when MCP tools are unavailable. No cross-link updates needed.
```
This is a flat-list rule line; apply SKILL.md invariant checklist: Falsifiable,
Actionable, One-line (two lines here is acceptable given dual tool names),
Context-free, Non-redundant, Universal, Doctrine-aligned.

---

**2. `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`**

Current line ~180:
```
For ticket status moves, use native `git mv` between status directories and commit through `{{.McpNamespace}}/git.commit`; `ready/` is implementation-ready and `todo/` is accepted backlog.
```
Replace with:
```
For ticket status moves, use `{{.McpNamespace}}/tickets.close(stem, status)` to close (done/dropped) or `{{.McpNamespace}}/tickets.move(stem, to)` to transition (idea/todo/ready); both stage atomically with convention guards. Fall back to native `git mv` when MCP tools are unavailable. Commit the staged change with `{{.McpNamespace}}/git.commit`. `ready/` is implementation-ready and `todo/` is accepted backlog.
```

---

**3. `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`**

Three sites in `## On: Edit Ticket` → `### 3. Move`:

Site A — current line ~109:
```
1. For moves, use native `git mv`.
```
Replace with:
```
1. For moves, use `{{.McpNamespace}}/tickets.close(stem, status)` for done/dropped, or `{{.McpNamespace}}/tickets.move(stem, to)` for idea/todo/ready; fall back to native `git mv` when MCP tools are unavailable.
```

Site B — current line ~110:
```
2. For `.done/` moves, add `completed:` date in frontmatter.
```
Replace with:
```
2. For `.done/` moves via native `git mv`, add `completed:` date in frontmatter; `tickets.close` writes this automatically.
```

Site C — current line ~113:
```
5. For proceed-routed `todo/` -> `ready/` promotion, defer `git mv` until **Spec-address Check** passes.
```
Replace with:
```
5. For proceed-routed `todo/` -> `ready/` promotion, defer the move until **Spec-address Check** passes.
```

And in `## On: Ready Focus`:

Site D — current line ~186:
```
5. For deferred `todo/` -> `ready/` promotion, perform native `git mv` before commit.
```
Replace with:
```
5. For deferred `todo/` -> `ready/` promotion, use `{{.McpNamespace}}/tickets.move(stem, to: "ready")` or native `git mv` as fallback; then commit.
```

---

**4. `agents-plugin/rsrc/lead-discuss/lead-discuss.md`**

In the `## On: ticket status change` handler:

Site E — current line ~99:
```
   a. Perform native `git mv ai-docs/tickets/idea/<stem>.md ai-docs/tickets/todo/<stem>.md`.
```
Replace with:
```
   a. Use `{{.McpNamespace}}/tickets.move(stem, to: "todo")`; fall back to native `git mv ai-docs/tickets/idea/<stem>.md ai-docs/tickets/todo/<stem>.md` when MCP tools are unavailable.
```

Site F — current line ~103:
```
   b. The lead-write-ticket procedure owns spec addressing, frontmatter population, the `git mv`, focus update, and commit.
```
Replace with:
```
   b. The lead-write-ticket procedure owns spec addressing, frontmatter population, the move, focus update, and commit.
```

## Integration Test Instructions

After all edits, run regeneration and the full test suite from
`agents-plugin-tool/`:

```bash
cd agents-plugin-tool

# 1. Regenerate agents-plugin/rsrc/manifest.json
WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -run TestGenerateRealManifest -v

# 2. Regenerate wsflow rsrc mirror
WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror

# 3. Full suite (incl. TestWsflowRsrcMirrorUpToDate)
go test ./...
```

Pass criteria:
- `TestGenerateRealManifest` exits with `PASS` (or skip message if env not set — must use env).
- `TestRegenerateWsflowRsrcMirror` exits with `PASS`.
- `TestWsflowRsrcMirrorUpToDate` PASS in the final `go test ./...` run.
- Full suite green.

## Implementation Strategy Decisions

- No SKILL.md invariant checklist triggered for changed lines in Invariants/Constraints sections (the changes are in handler/procedure steps). Apply general SKILL.md authoring quality: command-shaped, context-free, one-liner where possible.
- `lead-forge-spec.md` git mv references are spec-file archive operations and are intentionally excluded from scope.
- The `{{.McpNamespace}}` template var is the correct MCP namespace prefix; match existing usage in the file.

## Rejected Alternatives

- Removing all git mv references entirely: rejected — fallback is needed when MCP tools are unavailable (e.g., non-MCP playbook execution contexts).
- Updating only `ticket-conventions` and relying on convention propagation: rejected — the transition-directing playbooks have explicit `git mv` directives that remain actionable references independent of convention.

## Approach

1. Edit `ticket-conventions.md` (binary-embedded convention).
2. Edit three rsrc playbook files.
3. Run regeneration commands in sequence.
4. Run full test suite; verify `TestWsflowRsrcMirrorUpToDate` passes.
5. Commit as one logical unit.

## Constraints

- All edits are surgical: change only the listed text. Do not reformat surrounding lines.
- After editing rsrc files, always regenerate manifest and wsflow mirror before committing; `TestWsflowRsrcMirrorUpToDate` fails on stale mirrors.
- Preserve `{{.McpNamespace}}` template syntax exactly; do not expand to `ws`.

## Out of scope

- `lead-forge-spec.md` (spec archive git mv — different operation).
- Any other playbook not listed.
- Ticket file changes, spec file changes.
- SKILL.md Fresh-Reader Audit (separate authoring concern).

## Details

The six edit sites and their before/after text are fully specified in
**Contract Instructions** above. No additional interface specs required.

## Verification Contract

- All six edit sites changed per contract.
- `WSRSRC_REGEN=1` regeneration produces updated `agents-plugin/rsrc/manifest.json`.
- `WS_REGEN_WSFLOW_RSRC=1` regeneration produces updated `agents-plugin-wsflow/rsrc/` mirror.
- `TestWsflowRsrcMirrorUpToDate` PASS.
- Full `go test ./...` green.

## References

- `[Must] agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md` — invariant checklist and authoring principles for skill/playbook edits
- `[Must] agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md` — convention doc to edit (line 16)
- `[Must] agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md` — primitive catalog (line ~180)
- `[Must] agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md` — Move section (lines ~109-113, ~186)
- `[Must] agents-plugin/rsrc/lead-discuss/lead-discuss.md` — triage handler (lines ~99-103)
