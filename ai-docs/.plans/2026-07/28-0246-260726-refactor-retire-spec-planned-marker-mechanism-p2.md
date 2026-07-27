# Plan: 260726-refactor-retire-spec-planned-marker-mechanism — Phase 2: Remove the mechanism and ratchet downstream

## Relevant Ticket Contract

- Run 2.1-2.9 in the stated order. 2.7 changes downstream-visible distribution and
  must not run before the in-tree removal is green; 2.8 is the delivery vehicle.
- **Retain everything Phase 1's advisory is built on.** Phase 1's `### Result`
  "Retained detection surface" list may be extended, never shrunk. Phase 2 retains
  the compat note; its deletion belongs to a later ticket.
- **`SpecInfo.TicketRefs` is out of scope** — a separate mechanism backing
  `references.trace` and the `tickets=` render flag.
- **`fresh-reader-audit:58` is out of scope** — a generic `TODO`/`🚧` audit
  exclusion with no dependence on the spec convention.
- Delete the `🚧` split condition in both sites; **do not author a replacement**
  (that would be a convention change, out of scope).
- Do not delete the one live marker's *content* (`## Constraints` line 1).
- Advisory, never blocking. Ticket→spec scan direction only.
- Owner approved the 2.7 checklist item and the `WORKFLOW.md` prose edit
  specifically (2026-07-26), not a general license to edit template prose.
- v0024 is stripped at **clause granularity** — only its third clause goes.

## Out of Scope

- Phase 1 (landed: `3b4afa52`, `ed79d3c7`, `3514973a`, `073b6325`, merged
  `1c889258`). Its retained surface is a hard boundary, not a work item.
- `SpecInfo.TicketRefs`, `specTicketRefs`, `ticketsFromSpecRefs`,
  `references.trace`, the `tickets=` flag, and `mcp-tools.md`'s ticket-reference
  prose.
- `fresh-reader-audit.md:58` in **both** trees.
- `markerContext` / `specMarkerContexts` themselves (see F5 — they do not fall
  dangling).
- Authoring a replacement spec-split condition.
- `260723-feat-ready-spec-address-hard-gate` (unscheduled; accepted weakening).
- Removing the compat note (later ticket).
- `lead-forge-spec` step 6 "Associate stems with tickets"
  (`agents-plugin/rsrc/lead-forge-spec/lead-forge-spec.md:200-225`) — not
  marker-dependent; see F12.
- Worktree copies under `.worktree/` and `.claude/worktrees/` (separate checkouts).

## Codebase Findings

Baseline: `go build ./...` + `go test ./... -count=1` green across all 12 packages.
Current plugin version `0.36.25`. Branch `impl/retire-planned-marker`, tree clean.

### Ticket premises that FAILED — corrected locations

- **F1 — `agents-plugin-wsflow/rsrc/**` is GENERATED, not hand-maintained.**
  `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L54-L71`
  (`TestWsflowRsrcMirrorUpToDate`) asserts the wsflow rsrc tree is a
  **byte-identical** copy of `agents-plugin/rsrc/`; `#L88-L95`
  (`TestRegenerateWsflowRsrcMirror`, gated on `WS_REGEN_WSFLOW_RSRC=1`) wipes and
  rewrites it. So 2.3 is **4 file edits under `agents-plugin/rsrc/` + a regen**,
  not 8 hand edits. The ticket's "both trees / 8 files" phrasing invites
  hand-editing a generated tree. (Its separate two-lineage decision about the
  *templates* is correct — those are genuinely hand-maintained; see the held table.)
- **F2 — `test_wsflow_skill_bundle.py:196` is wrong by +31.** The real test is
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L225-L230`
  (`test_bootstrap_template_uses_wsflow_local_version_lineage`):
  - `:227` `self.assertIn("<!-- Template Version: v0005 -->", text)` ← update to v0006
  - `:228` `self.assertIn("This template has package-local version history", text)` ← unchanged
  - `:229-230` `assertNotIn("<!-- Template Version: v0038 -->")` / `assertNotIn("- v0038:")`
    ← unchanged guard rails the ticket never mentions.
- **F3 — `workflow-skills.md:818-819` → real `:857`** ("adds or updates spec
  entries, strips planned markers when implementation lands"). Off by 38.
- **F4 — `workflow-skills.md:997` → real `:1036-1037`** ("writes anchor-keyed spec
  entries, verifies the index, and associates planned stems with active tickets
  when required"), under anchor `{#260707-forge-spec-autoproceed-classification-2}`
  at `:1038`. Off by 39.
- **F5 — 2.1's "drop the struct fields that fall dangling" over-reaches.**
  `SpecInfo.MarkerContexts` (`internal/wsdoc/spec_discovery.go#L31`) and
  `SpecAnchorInfo.MarkerContext` (`#L54`) are **still read** by `specs.find`'s match
  scoring at `#L105` and `#L107` — the exact consumer the ticket's own
  `## Constraints` requires retaining. Only the *render* lines go.
  What genuinely falls dangling and is invisible to the compiler:
  `specStats`'s wip/refs half (`internal/wsdoc/project_tree.go#L221-L233`) and the
  package-level var `ticketRefRE` (`#L15`, sole other use `#L228`) — Go reports
  unused **locals** and imports, never unused package-level vars, struct fields, or
  unexported funcs.
- **F6 — `staticcheck` is NOT available in this tree** (`which staticcheck` → not
  found; no Makefile, no tool dependency in `go.mod`, no vendored binary). The
  dangling-symbol method must be **grep**, not staticcheck. See step 1c.
- **F7 — `server.go` offsets drifted again since Phase 1's correction.** Real now:
  `formatSpecs` at `internal/mcp/server.go#L2508`, its marker render at **`#L2536`**
  (Phase 1's correction still holds); `tickets=` at **`#L2528-L2529`** (retain);
  `formatSpecFind` at `#L2551`; `formatSpecStatus` at `#L2653`, its
  `loc.MarkerContext` render at **`#L2666-L2667`** (Phase 1 recorded 2637-2638).
- **F8 — `workflow-skills.md`'s `## Planned References` exclusion is at `:365`,
  not 354-355.** (`documentation-system.md:147` is correct within the claimed
  146-147.) The exclusion premise itself HOLDS: both are the workset
  planned-but-not-created reference mechanism, unrelated to `🚧`.
- **F9 — `mcp-tools.md`'s specs.* contract line is `:832`, not ~829.** It reads
  "They expose spec file metadata, anchors, ticket references, **marker context**,
  query matches, and exact-stem status". `project_tree` at `:807-813` — the `~807`
  claim holds. `mcp-tools.md` does **not** document project_tree's WIP/feature
  counts anywhere (grep for `WIP` finds only unrelated todo-tool text), so 2.1
  removes an *undocumented* output there. It also contains no literal `tickets=`
  string; clause 4's "`tickets=` / spec ticket references" is satisfied by `:832`'s
  "ticket references", which must survive the same-line edit.
- **F10 — `lead-write-spec` has 11 `🚧` sites, not the 7 enumerated**, and
  `lead-update-spec`'s markers are **not confined to §5**. See the sweep table.
- **F11 — the `[obsoleted by vNNNN]` claim is partly wrong**:
  `agents-plugin/skills/lead-bootstrap/AGENTS.template.md:131` `- v0002: [obsoleted]`
  and `:136` `- v0007: [obsoleted]` are bare; only `:137` v0008 and `:141` v0012 use
  `[obsoleted by v0014]`. Immaterial — v0024 is only partly nullified, so no
  placeholder form applies.
- **F12 — the "associates planned stems with active tickets" prose describes a
  surviving behavior.** `lead-forge-spec.md:200-225` step 6 merges spec stems into
  ticket `spec:` frontmatter and is not marker-dependent. So
  `workflow-skills.md:1036-1037` and `documentation-system.md:258` must be
  **reworded** ("planned stems" → "spec stems"), never deleted.
- **F13 — 2.4 conflicts with a stated landing-order decision that is unsatisfied.**
  `ai-docs/tickets/ready/260726-bug-inline-playbook-invocation-commit-ownership.md#L53-L61`
  decides "This ticket lands before 260726-refactor-retire-spec-planned-marker-mechanism",
  because 2.4 deletes `lead-write-ticket.md:106`, its *only known inline caller*. It
  is still in `ready/`, unlanded. The same bullet supplies the fallback: "If the
  retirement does land first anyway, this ticket must **re-run the survey**." Not
  blocking, but must be recorded (step 4b).

### 2.6 — the shipped-or-not finding: **SHIPPED**

Evidence from code, which the ticket instructs to trust over both doc sources:

- `ws-dashboard/crates/daemon/src/discovery.rs#L107-L113` — automatic prune when
  `workspace.active_work_root_count == 0`.
- `#L169-L170`, `#L186-L203` — root-anchor vs. discovered child workRoot,
  `active_work_root_count`, `root_unavailable_with_active_child`.
- `#L242-L262` — `recovery_needed` → `state.status = "recoveryNeeded"`, `stale: true`.
- `ws-dashboard/crates/daemon/src/resources.rs#L30-L38`, `#L60-L66` — prune side
  effects (terminal sessions removed, `opened.unregister` per pruned id).
- Tests: `discovery.rs#L616`, `#L638`, `#L664`;
  `ws-dashboard/crates/daemon/tests/routes.rs#L881`, `#L1197`.
- Implementing commit `0d53a727` "feat(dashboard): discover and prune workRoots"
  (2026-05-24); its `## Updated Tickets` moves the ticket to done.
- `ai-docs/tickets/.done/260524-feat-ws-dashboard-workspace-root-prune-policy.md`
  `### Result (pending) - 2026-05-24` reads "Implemented the first lifecycle policy
  slice…". `(pending)` is an unbackfilled commit-hash placeholder (9 other `.done/`
  tickets carry the same artifact), **not** a not-done status.

**Wrinkle the ticket does not anticipate.** The callout at
`ai-docs/spec/ws-web-dashboard/index.md#L231-L241` defines active as "activation
permits targeting **and** availability is currently usable" (`:237-238`). The
shipped rule counts availability **only** (`discovery.rs#L198-L200`), a deliberate
decision asserted by `routes.rs#L909-L913`. `spec-conventions.md:28` says "No `🚧`
means implemented — verify each such feature actually exists before committing", so
converting the callout to a marker-free implemented entry **without** fixing
`:237-238` publishes a false claim under the very convention 2.2 keeps.

`ai-docs/mental-model/ws-web-dashboard.md:64` ("The **planned** workspace-root
policy…") is now factually wrong; `:147` is phrased `**Implement** workspace root
pruning`. Both cross-reference the anchor, which must survive (clause 5).

### Confirmed-held premises (locations re-verified)

| Premise | Verdict |
|---|---|
| `SpecInfo.TicketRefs` is separate and backs `references.trace` | HELD — `spec_discovery.go#L231,L281`; `references.go#L72,L81-L85`; renders at `server.go#L2528-L2529` |
| `fresh-reader-audit:58`, under `## What not to flag` (`:55`) | HELD exactly, both trees |
| `workflow-skills.md` contains **zero** `🚧` | HELD (`grep -c` = 0) — an emoji sweep passes clause 5 trivially, as warned |
| nine `🚧` sites in `lead-forge-spec` | HELD exactly: 74, 144, 155, 183, 194, 250, 268, 269, 279 |
| `lead-write-spec` `judge: split-trigger` condition at `:61` | HELD |
| `spec-conventions.md:43-44` gap resolution path | HELD |
| `ticket-conventions.md:29-30` | HELD — both lines need work (`:30` also carries a `🚧`) |
| `documentation-system.md` 98-102, 104-107, 236-240, 247, 258 | HELD ("marker context" is at `:107`) |
| `AGENTS.template.md` v0024 at `:153`, tag at `:191`, current v0044 | HELD (file is exactly 191 lines) |
| wsflow `AGENTS.template.md` tag at `:163`, v0005, zero `🚧` | HELD; "package-local version history" at `:128` |
| v0022 `features:` item at `:151` | HELD |
| three `WORKFLOW.md` at `:60` / `:60` / `:63` | HELD; each has exactly one `🚧` and one marker paragraph (lines 60-63 / 60-63 / 63-66) |
| three regen flags exist and do what the ticket says | HELD — see step 8 |
| `manifest.json` hashes both bootstrap files; `TestSkillsManifestDriftIsVisible` | HELD — `agents-plugin/skills/manifest.json#L5,L7`; test at `internal/wsrsrc/skills_manifest_test.go#L29` |
| 2.9 precondition | HELD — `ai-docs/tickets/ready/260726-bug-inline-playbook-invocation-commit-ownership.md#L19-L36,L55` carries the extracted `lead-write-spec` step-7 finding; the cycle ticket is in `todo/` |
| `project_tree_test.go:16` `features:` fixture | HELD |
| bootstrap templates + `WORKFLOW.md` are un-generated | HELD — `substitutionMirroredSkills` (`internal/wsrsrc/skills_mirror_test.go#L15-L20`) covers only 4 skills × `SKILL.md`; `lead-bootstrap` absent; no test of any kind reads either `WORKFLOW.md`; `test_wsflow_skill_bundle.py#L229-L230` *requires* the two templates to differ |

### 2.3 sweep — real hit counts (canonical tree only, per F1)

`grep -rn 🚧 agents-plugin/rsrc/`:

| File | `🚧` lines | vs. the ticket |
|---|---|---|
| `lead-write-spec/lead-write-spec.md` | **11**: 14, 26, 33, 36, 38, 51, 52, 61, 100, 103, 112 | named 7; omits `:103` (`## 🚧 New Feature` in the `spec-format` template), `:112` (no-ticket-refs rule), and `:120` `Planned marker: <added\|none\|removed>` (emoji-free, so a `🚧` sweep misses it entirely) |
| `lead-update-spec/lead-update-spec.md` | **5**: 12, 41, 43, 45, 82 | said "§5 entirely"; `:12` (Invariants) and `:82` (completion report) are **outside** §5. Removing §5 forces renumbering §6→§5, §7→§6 (`grep -n '^#'` → 41, 47, 51) |
| `lead-forge-spec/lead-forge-spec.md` | **9**: 74, 144, 155, 183, 194, 250, 268, 269, 279 | count HELD |
| `fresh-reader-audit/fresh-reader-audit.md` | 1: `:58` | **OUT OF SCOPE** |

2.2 targets in `agents-plugin-tool/internal/wsdoc/conventions/`: `spec-conventions.md`
22, 24, 25, 28, 29, 30, 31 (the `## 🚧 Markers` section, `:22-31`), 43, 44 (gap
resolution), 95, 98 (the `spec-format` example block); `ticket-conventions.md:30`.

### 2.5 sweep — predicate is `🚧` OR planned-marker prose, `## Planned References` excluded

`grep -rniE "planned marker|planned entr|planned stem|strips planned|planned spec|🚧|contract-first" ai-docs/spec/`
→ **14 hits across 4 files**:

- `documentation-system.md`: 98, 100, 101, 102 (contract-first paragraph),
  **107** ("marker context" in the specs.* sentence — a 2.1 output change), 236, 240
  (authoring-workflow prose), 247, 258.
- `workflow-skills.md`: **310** ("writes planned or implemented entries"), 322, 323,
  **857**, **1036**. Zero `🚧`.
- `mcp-tools.md`: **811** and **837-855** are Phase 1's own advisory — **RETAIN**.
  Only `:832` needs editing.
- `ws-web-dashboard/index.md:231` — 2.6's job.

Excluded (live workset mechanism): `documentation-system.md:146-147`,
`workflow-skills.md:364-365`.

## Implementation Plan

One commit per sub-step group is fine; 2.7 and 2.8 must land together.

**1. (2.1) Go removal.**
   a. `internal/mcp/server.go`: delete `writeIndentedLines(&b, "  marker: ", spec.MarkerContexts)`
      at `#L2536` (`formatSpecs`) and the `if loc.MarkerContext != ""` block at
      `#L2666-L2667` (`formatSpecStatus`). Keep the adjacent `"  legacy-marker: "` /
      `"legacy-marker: "` appends and `formatSpecFind`'s own advisory loop — Phase 1's
      retained surface. Keep `tickets=` at `#L2528-L2529`.
   b. `internal/wsdoc/project_tree.go`: remove the WIP/planned half — the `wip`
      counter and `refs` from `specStats` (`#L221-L233`) and the `wipText` block in
      `renderSpecDir` (`#L182-L189`). Removing `refs` orphans `ticketRefRE` (`#L15`);
      delete it. `specStats`'s surviving `total` (`Nf`) reads the same dead
      `features:` frontmatter and is structurally 0 on any real corpus — prefer
      deleting `specStats` outright and dropping the `[…]` stats segment; whichever
      way, state the choice in the commit body. Do **not** touch
      `legacyMarkerAdvisoryFor` (`#L206-L217`), `scanLegacyMarkersUnderSpecRoot`, or
      the repo-root threading.
   c. **Dangling-symbol pass — grep, not staticcheck (F6).** From `agents-plugin-tool/`:
      `for s in specStats ticketRefRE MarkerContexts MarkerContext markerContext specMarkerContexts; do echo "== $s"; grep -rn "$s" --include="*.go" .; done`
      Expected after the edits: `MarkerContexts` still at
      `spec_discovery.go#L31,L105,L232` and `MarkerContext` at `#L54,L107,L270`
      (specs.find scoring — **retain**, F5); zero non-declaration hits for
      `specStats` / `ticketRefRE` if deleted.
   d. Update `internal/wsdoc/project_tree_test.go#L28` — the assertion
      `"  demo.md  - Demo  [2f, WIP 1 -> 260503-feat-demo/p1]"` must match the new
      output. Keep the `features:` fixture at `#L16`; it is also the legacy-marker
      list-shape fixture.
   e. `go build ./... && go vet ./... && go test ./... -count=1`.

**2. (2.2) Embedded conventions** (`agents-plugin-tool/internal/wsdoc/conventions/`).
   a. `spec-conventions.md`: delete the `## 🚧 Markers` section (`:22-31`) and the two
      `🚧` blocks in the `spec-format` example (`:95-96` callout, `:98-100` heading
      entry).
   b. Rewrite `:43-44`. `:43`'s "distinguishes it from `🚧` entries…" clause loses its
      referent — reduce it to "No ticket required." `:44`'s resolution path becomes:
      create the qualifying ticket and carry the gap's contract text in that ticket's
      `## Spec Impact`, removing the callout at implementation closeout. The callout
      itself survives; only its exit path changes.
   c. `ticket-conventions.md:29` — the answer is **never**: `lead-write-ticket` does
      not invoke `lead-write-spec`; spec addressing runs through `spec:`,
      `spec-remove:`, or `## Spec Impact`. `:30` — rewrite the drop path so it no
      longer routes through `lead-write-spec` to remove orphaned `🚧` entries.
   d. State the `:29` answer once and reuse it verbatim at `documentation-system.md:247`
      and `workflow-skills.md:322-323` (step 5).

**3. (2.3) rsrc playbooks — edit `agents-plugin/rsrc/` ONLY** (F1; the wsflow mirror
   is regenerated in step 8).
   a. `lead-write-spec.md`: `:14` and `:36` → "for every heading" (drop the exception
      clause, and `:36`'s "Never remove `🚧` without confirmation"); `:26` and `:33` →
      drop the `judge: contract-first-spec` / `🚧` clauses; `:38` → drop "whether any
      `🚧` marker was added"; `:46-52` → delete the `judge: contract-first-spec` block
      (shared with step 4); `:61` → delete the split condition, **no replacement**
      (leaves 2 conditions; "Any one condition is sufficient" at `:65` still reads);
      `:100-101` and `:103-105` → remove from the `spec-format` template; `:112` →
      drop the `🚧` half of the no-ticket-refs rule; `:120` → remove the
      `Planned marker:` line from the Output-handoff template (**emoji-free — F10**).
   b. `lead-update-spec.md`: delete §5 (`:41-45`), renumber §6→§5 and §7→§6 plus any
      cross-references; `:12` → drop the `🚧` clause from Invariants; `:82` → drop
      `M 🚧 stripped` from the completion report.
   c. `lead-forge-spec.md`: all nine sites — `:74`, `:144`, `:155` (survey prompts),
      `:183` (the "Implemented or planned?" branch), `:194` (heading-marker
      placement), `:250` (`🚧 Planned: <count>`), `:268` and `:269` (the closeout
      strip/review bullets — its forge-time analogue of lead-update-spec §5), `:279`
      (the split-condition table row, condition (1); renumber (2)/(3)).
   d. **Do not touch `fresh-reader-audit.md:58`.** It is prose with no compiler and no
      test behind it; deleting it would start flagging issues inside text its author
      explicitly marked unfinished.
   e. Verify `grep -rn 🚧 agents-plugin/rsrc/` returns exactly one line —
      `fresh-reader-audit/fresh-reader-audit.md:58`.

**4. (2.4) Judge.**
   a. Delete `judge: contract-first-spec` from `lead-write-ticket.md:164-168` and
      `lead-write-spec.md:46-52`, and rewrite the invoking branch at
      `lead-write-ticket.md:106` so step 3 of **On: Spec-address Check** ends at
      writing/updating `## Spec Impact`, with no inline `lead-write-spec` invocation.
      Leave `judge: spec-address-gate` (`:150-154`) and `judge: missing-spec-address`
      (`:188-193`) intact.
   b. **Record the landing-order inversion (F13).**
      `260726-bug-inline-playbook-invocation-commit-ownership` is still in `ready/` and
      its `## Decisions` requires it to land first. Note in Phase 2's `### Result` that
      the retirement landed first, and add an `#### Edition` note (or a
      `## Ticket Updates` forward line) to that ticket so its Phase 1 **re-runs the
      survey** rather than assuming its instance survives. Do not silently delete its
      motivating case.

**5. (2.5) Repo spec corpus.**
   - `documentation-system.md`: delete the contract-first paragraph `:98-102`; `:107`
     drop "marker context" from the specs.* capability list; `:236` "implemented
     entries or contract-first `🚧` entries" → implemented entries; `:240` drop
     "strips `🚧` markers when implementation has landed"; `:247` apply step 2d's
     answer; `:258` "associates planned stems" → "associates spec stems"
     (**F12 — reword, do not delete**).
   - `workflow-skills.md`: `:310` "writes planned or implemented entries" →
     implemented entries; `:322-324` apply step 2d's answer; `:857` drop "strips
     planned markers when implementation lands"; `:1036-1037` "planned stems" →
     "spec stems" — an anchored-entry amendment under
     `{#260707-forge-spec-autoproceed-classification-2}` (`:1038`); keep the anchor.
   - `mcp-tools.md`: `:832` only — drop "marker context", **keep "ticket references"**.
     Retain `:811-813` and `:837-855` (Phase 1's advisory).
   - Do not touch `documentation-system.md:146-147` or `workflow-skills.md:364-365`.

**6. (2.6) The one live marker** — `ai-docs/spec/ws-web-dashboard/index.md:231-241`.
   Finding: **shipped**. Convert to an ordinary implemented entry: drop the
   `> [!note] Planned 🚧` callout wrapper and fold the body into the parent section
   `## Durable WorkRoot Registry And Activation` (`:224`) as present-tense prose,
   **preserving `{#260524-dashboard-workspace-root-prune-policy}`** on the retained
   text (two mental-model cross-references depend on it).
   - Correct `:237-238`'s definition of "active" to availability-only, matching
     `discovery.rs#L198-L200`; otherwise the now-marker-free entry asserts behavior
     that does not exist, against `spec-conventions.md:28`.
   - Update `ai-docs/mental-model/ws-web-dashboard.md:64` (drop "planned") and `:147`
     (`**Implement** …` → `**Change** …`), keeping the anchor reference.
   - Record the shipped verdict and its code evidence in Phase 2's `### Result`
     (clause 5 requires it).

**7. (2.7) Bootstrap ratchet, both lineages** (all four files hand-maintained).
   a. `agents-plugin/skills/lead-bootstrap/AGENTS.template.md`: add `- v0045: …` after
      `:188`; move `<!-- Template Version: v0044 -->` at `:191` to v0045; strip
      **only** the third clause of `:153` (`planned ticketed features ->
      \`### 🚧 <Feature Name>\``), leaving the `[!note] Constraints` and Implementation
      Gap clauses. Match house style (single line, `- vNNNN: `, imperative verb,
      backticked paths, ASCII `->`; see `:186-188`).
   b. v0045's text must cover **three** marker forms: `## 🚧` headings,
      `> [!note] Planned 🚧` body callouts, **and** `- 🚧 <name> [stem/pN]` entries in
      spec `features:` frontmatter (`:151` v0022 still tells downstream to rebuild
      that frontmatter, and 2.1 deletes the only tool that surfaced it). Resolution
      per marker: move the pending text into the owning ticket's `## Spec Impact` when
      a live ticket references that spec; otherwise keep it as an implemented entry if
      it shipped, or an Implementation Gap callout if it did not.
   c. `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md`: add the
      equivalent as `- v0006:` after `:160`; move the tag at `:163` to v0006. No clause
      strip (zero `🚧` in this lineage).
   d. `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py:227` → assert
      `"<!-- Template Version: v0006 -->"`. Leave `:228-230` unchanged.
      (**Not `:196` — F2.**)
   e. Delete the marker paragraph from all three `WORKFLOW.md` files:
      `agents-plugin/skills/lead-bootstrap/WORKFLOW.md:60-63`,
      `agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md:60-63`,
      `ai-docs/WORKFLOW.md:63-66`. Each file's marker mention is confined to that one
      bullet; keep the surviving "planned work stays in ticket `## Spec Impact`" sense
      as the sole rule.

**8. (2.8) Regenerate and bump.** From `agents-plugin-tool/`:
   - `WSRSRC_REGEN=1 go test ./internal/wsrsrc/ -run TestGenerateRealManifest -count=1`
     → `agents-plugin/rsrc/manifest.json` (flag read at `internal/wsrsrc/wsrsrc_test.go#L959`).
   - `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc/ -run TestRegenerateWsflowRsrcMirror -count=1`
     → rewrites `agents-plugin-wsflow/rsrc/` (flag at `wsflow_mirror_test.go#L89`).
     **This is what propagates step 3 to the wsflow tree.**
   - `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/ -run TestGenerateRealSkillsManifest -count=1`
     → `agents-plugin/skills/manifest.json` (flag at `skills_manifest_test.go#L56`);
     required because step 7 edits `lead-bootstrap/AGENTS.template.md` and
     `WORKFLOW.md`, both SHA-256'd at `manifest.json#L5,L7`.
   - Stale-artifact detectors: `TestWsflowRsrcMirrorUpToDate` (`wsflow_mirror_test.go#L54`)
     and `TestSkillsManifestDriftIsVisible` (`skills_manifest_test.go#L29`). Note there
     is **no** manifest under `agents-plugin-wsflow/skills/`, so step 7c/7e's wsflow
     edits have no hash gate — review them by hand.
   - **Version bump — ONE bump, at dev-merge.** Current `0.36.25`. AGENTS.md requires
     one bump per dev-merge through `agents-plugin-tool/scripts/bump-ws-version.sh <X.Y.Z>`,
     and 2.8 requires a bump so the embedded convention change reaches installed
     plugins. **These are the same bump.** Run `bump-ws-version.sh 0.36.26` once, at
     merge time, not twice. Never hand-edit the edition points.

**9. (2.9) Ticket drop.** Precondition verified now: the commit-ownership ticket exists
   at `ai-docs/tickets/ready/260726-bug-inline-playbook-invocation-commit-ownership.md`
   and carries the extracted finding (`#L19-L36`, `#L55`).
   `git mv ai-docs/tickets/todo/260726-bug-spec-planned-marker-ready-ticket-cycle.md ai-docs/tickets/.dropped/`
   and append a `## Resolution` recording that its premise — making `🚧`'s ordering
   satisfiable — was retired by this ticket, and that its surviving finding lives in
   the commit-ownership ticket.

## Verification Plan

Mapped to the ticket's numbered clauses.

1. **Clause 1 (2.1)** — `go build ./... && go vet ./... && go test ./... -count=1`
   green (12 packages). Run `ws/project_tree`, `ws/specs.list`, `ws/specs.status`
   against this repo: no `WIP`/`Nf` stats segment, no `marker:` line and no
   `# <marker context>` suffix, and Phase 1's `legacy-marker:` advisory still present
   on `ai-docs/spec/ws-web-dashboard/index.md` **until step 6 lands** (afterwards it
   correctly reports nothing). `ws/references.trace(spec_stem: …)` still resolves
   spec→ticket; `ws/specs.list` still renders `tickets=`. Plus the step-1c grep pass.
2. **Clause 2 (2.2/2.3)** —
   `grep -rn 🚧 agents-plugin/rsrc/ agents-plugin-wsflow/rsrc/ agents-plugin-tool/internal/wsdoc/conventions/`
   returns **exactly two** lines, both `fresh-reader-audit.md:58`, both still reading
   "Issues already present in text marked explicitly as `TODO` or `🚧`."
   `ws/convention.read(name: "spec-conventions")` shows the Implementation Gap Callout
   carrying the `## Spec Impact` resolution path. Both split-condition sites
   (`lead-write-spec.md:61`, `lead-forge-spec.md:279`) gone with no replacement
   condition added. wsflow bundle suite green.
   Mirror gate: `go test ./internal/wsrsrc/ -run TestWsflowRsrcMirrorUpToDate -count=1`
   green **only after** step 8's regen — a failure before it is the expected signal.
3. **Clause 3 (2.4)** — a fresh `lead-write-ticket` run
   (`ws/playbook.print(name: "lead-write-ticket")`) on a spec-touching ticket reaches
   `ready/` through `## Spec Impact` with no contract-first branch offered;
   `grep -rn "contract-first-spec" agents-plugin/rsrc/` returns nothing. Phase 2
   `### Result` records the F13 landing-order inversion and the note added to the
   commit-ownership ticket.
4. **Clause 4 (2.5)** — re-run the 2.5 sweep
   (`grep -rniE "planned marker|planned entr|planned stem|strips planned|planned spec|🚧|contract-first" ai-docs/spec/`):
   the only surviving hits are `mcp-tools.md:811` and `:837-855` (Phase 1's advisory,
   retained) plus `documentation-system.md:146-147` / `workflow-skills.md:364-365`
   (`## Planned References`, excluded). `mcp-tools.md:832` still says "ticket
   references".
5. **Clause 5 (2.6)** — `grep -rn 🚧 ai-docs/spec/` returns zero.
   `ws/specs.status(spec_stem: "260524-dashboard-workspace-root-prune-policy")`
   resolves; `ws/spec_index.verify()` clean; both
   `ai-docs/mental-model/ws-web-dashboard.md` cross-references still land
   (`ws/references.trace`). The entry's form is an implemented entry, and `### Result`
   records the shipped verdict with the `discovery.rs` evidence.
6. **Clause 6 (2.7)** — `agents-plugin/skills/lead-bootstrap/AGENTS.template.md`
   carries v0045 with `<!-- Template Version: v0045 -->` as its last line and v0024
   reduced to two clauses; wsflow's carries v0006 with its tag at v0006;
   `test_wsflow_skill_bundle.py` green; `grep -n 🚧` on all three `WORKFLOW.md` → zero.
   Read v0045 as a project pinned at v0044 would: it applies cleanly, gives an
   instruction for a `features:`-frontmatter marker (not only body forms), and a
   project pinned below v0024 still receives v0024's two surviving clauses.
7. **Clause 7 (2.8)** — re-run each of the three regens a second time:
   `git status --porcelain` shows no new diff (idempotent).
   `go test ./internal/wsrsrc/ -count=1` green including `TestSkillsManifestDriftIsVisible`
   and `TestWsflowRsrcMirrorUpToDate`. After the merge-time bump, `git show --stat`
   touches only the script's edition points (`.github/workflows/ws-mcp-release.yml`,
   `cmd/ws-mcp/main.go`, `scripts/build-release-assets.sh`, both `plugin.json` pairs,
   both `runtime.json`, `ai-docs/_index.md`).
8. **Clause 8 (2.9)** —
   `ls ai-docs/tickets/.dropped/260726-bug-spec-planned-marker-ready-ticket-cycle.md`
   present with a `## Resolution` section;
   `ls ai-docs/tickets/ready/260726-bug-inline-playbook-invocation-commit-ownership.md`
   still present and still carrying the extracted finding.

Pre-merge gate: `go build ./... && go vet ./... && go test ./... -count=1` plus the
wsflow python bundle suite.

## Escalations

- Confidence: **high**. Every location was re-verified against the tree; the two
  judgment calls the ticket flagged (2.6 shipped-or-not, and whether `TicketRefs` is a
  separate mechanism) both resolved decisively from code.
- Two items want a lead decision but do **not** need research:
  1. **F13 (2.4 landing order).** `260726-bug-inline-playbook-invocation-commit-ownership`
     explicitly decides it lands before this ticket and is still unlanded. Step 4b's
     fallback (proceed + record + require its survey re-run) is written into that
     ticket itself, so proceeding is sanctioned — but the lead should confirm rather
     than have the executor pick.
  2. **2.6's `:237-238` accuracy fix.** Strictly it corrects a pre-existing spec
     inaccuracy rather than removing a marker. It is in scope because
     `spec-conventions.md:28` makes "no `🚧`" an assertion of implemented behavior, so
     shipping the conversion without it publishes a false claim. Flagged because it
     widens 2.6 beyond "strip the marker".
- **F1 changes the shape of 2.3** from 8 hand edits to 4 edits plus a regen. That is a
  premise correction, not a strategy question — but it is the one place where
  following the ticket's wording literally would hand-maintain a generated tree.
