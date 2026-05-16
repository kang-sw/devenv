---
title: write-code public UI contract loss
related:
  260513-epic-workflow-question-loop-hygiene: parent workflow hygiene backlog
  260516-feat-ws-web-workbench-substrate: dogfood case that exposed the issue
related-mental-model:
  - workflow-skills
completed: 2026-05-16
---

# write-code public UI contract loss

## Background

During the ws web dashboard workbench substrate dogfood run, the delegated
implementation successfully satisfied structural checks but lost an important
public UI intent. The user discussion asked for a constrained editor/workbench
experience, while the implementation brief and review gates emphasized
topology: left nav, split groups, pinned rows, opened rows, toolbar, and
placeholder surfaces. The resulting UI exposed internal model terms as heavy
visible chrome and looked like an explanatory dashboard mock rather than an
editor/workbench surface.

This suggests a workflow bug in `lead-write-code` or the surrounding
brief/review relay for public UI work: intent can be transformed into structural
acceptance criteria that are too weak to preserve the product-facing contract.

## Observed Failure Mode

- The implementation brief named structural requirements but did not explicitly
  forbid visible topology labels, card-heavy placeholders, or thick non-editor
  section chrome.
- The delegated visual review checked blank page, auth, dark theme, split
  presence, overflow, and screenshots, but did not evaluate whether the UI
  matched the discussed product chrome.
- The review did not test interaction-affordance honesty. It accepted visible
  pinned/opened areas even though the apparent tab/selector controls did not
  switch active panes, did not support placement, and did not clearly present
  themselves as disabled or deferred.
- The lead merged the phase because technical and structural gates passed,
  even though the screenshots should have triggered a product-intent review.

## Research Questions

- Should `lead-write-code` require a dedicated "Public UI Contract" section in
  implementation briefs whenever visible frontend behavior is in scope?
- Should reviewer prompts for visible UI include an explicit product-intent gate
  in addition to structural and visual-regression checks?
- Should visual verification distinguish "page is not broken" from "UI matches
  the agreed information architecture, density, and interaction metaphor"?
- Should visual verification require basic interaction checks for visible
  controls, such as clicking tab selectors, checking active pane changes, and
  flagging draggable-looking affordances that cannot actually drag?
- Should internal model vocabulary be treated as suspect when it appears as
  large user-facing labels unless the brief explicitly allows it?

## Candidate Direction

For frontend/product UI tasks, the brief should carry:

- intended interaction metaphor and density target;
- public labels that are allowed or forbidden;
- internal model terms that must remain implementation-only;
- examples of UI shapes to avoid;
- interaction-affordance honesty rules: visible selectors must select, visible
  drag affordances must drag or be absent/disabled, and deferred behavior must
  not masquerade as an available control;
- screenshot/review criteria that compare against the product contract, not only
  against blank/overflow/regression checks.

The reviewer should treat failure to preserve those public UI constraints as a
blocking finding even when tests and structural checks pass.

## Resolution

The hotfix generalized the issue beyond UI-specific work. `lead-write-code`
briefs now preserve selected-slice binding decisions instead of only structural
summaries: caller-visible contracts, implementation strategy decisions, rejected
alternatives, and verification expectations must appear in the brief or be
explicitly deferred or out of scope.

Ticket-driven fit review now reads the ticket and treats omitted selected-slice
binding decisions or implementation violations as blocking findings, while the
implementer still reads only the brief and optional plan.
