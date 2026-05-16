---
title: write-code public UI contract loss
related:
  260513-epic-workflow-question-loop-hygiene: parent workflow hygiene backlog
  260516-feat-ws-web-workbench-substrate: dogfood case that exposed the issue
related-mental-model:
  - workflow-skills
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
- The lead merged the phase because technical and structural gates passed,
  even though the screenshots should have triggered a product-intent review.

## Research Questions

- Should `lead-write-code` require a dedicated "Public UI Contract" section in
  implementation briefs whenever visible frontend behavior is in scope?
- Should reviewer prompts for visible UI include an explicit product-intent gate
  in addition to structural and visual-regression checks?
- Should visual verification distinguish "page is not broken" from "UI matches
  the agreed information architecture, density, and interaction metaphor"?
- Should internal model vocabulary be treated as suspect when it appears as
  large user-facing labels unless the brief explicitly allows it?

## Candidate Direction

For frontend/product UI tasks, the brief should carry:

- intended interaction metaphor and density target;
- public labels that are allowed or forbidden;
- internal model terms that must remain implementation-only;
- examples of UI shapes to avoid;
- screenshot/review criteria that compare against the product contract, not only
  against blank/overflow/regression checks.

The reviewer should treat failure to preserve those public UI constraints as a
blocking finding even when tests and structural checks pass.
