---
kind: print
variables:
  - ExploreAgent
---

# Co-author Paged Document

Topic: co-author a document people read — a slide deck or an A4 report — with
the user, page by page, and output a PDF. Code, `AGENTS.md`, manuals, and
other working documents an AI rereads belong to
`{{.SkillNamespace}}:lead-audit-doc`.

## Invariants

Authorship
- You write the document yourself: the draft → review → correct loop with the
  user is the point, and handing pages to a worker breaks it. You may spawn
  {{.ExploreAgent}} to gather source material.
- The user decides at each confirmation point in `On: invoke`; wait for the
  answer before passing one.

Plan file
- `plan.md` in the working folder is the authority for the outline, the
  confirmed decisions with their `(nn)` IDs, the page-number map, and the
  review-pending list. Record every user correction there as a decision.
  After compaction, reread it before resuming, because the conversation no
  longer holds that state.

Tracking
- `plan.md` and `document.html` share one fate, and the folder's own
  `.gitignore` alone decides it: leave the project root `.gitignore` and the
  file names unchanged and never force-add, so the plan cannot end up
  tracked differently from the document.
- Commit the working folder only when the user chose "commit" at step 3 or
  asks, following the project's commit rules. Commit a baked PDF only when the
  user asks, wherever it is written, including inside a committed folder: the
  material may be confidential.

Style
- The visual look is your own first draft: choose it on the first page and
  let the user correct it.

## On: invoke

1. Discuss the topic: audience, purpose, and length or talk time.
2. Settle the form: a slide deck (16:9 landscape) or an A4 document
   (portrait); the structural elements (cover, part dividers, appendix, and
   so on); text density; and any reference document the user supplies.
   Confirm the form with the user before step 3.
3. Propose the working folder `ai-docs/paged/<stem>/`, where `<stem>` is
   `YYMMDD-<slug>` with today's date, and ask the user whether the folder
   should be committed; the default is not committed. Then create the folder
   and, unless the answer is "commit", write a `.gitignore` inside it whose
   content is `*`. A later change of mind is the user asking you to delete
   that `.gitignore`.
4. Create `plan.md` and `document.html` in the folder.
5. Discuss the overall order and direction of the document; once the user
   confirms it, record it in `plan.md`.
6. After the user confirms, build the document from the front, one or two
   sections per round: write the pages, render-check each new page,
   regenerate the progress section, and stop for the user's review. Apply the
   corrections, record each in `plan.md`, and repeat.
7. When the document is complete, ask whether to bake a PDF, and where and
   under what file name.

## Mechanics

Page
- Each page is one fixed-size block: 1280×720 px for slides, 210×297 mm for
  A4. A print `@page` rule uses the same size, so the PDF matches the screen.
- Page numbers are assigned automatically in document order; reviews refer to
  pages by the on-screen number.
- CSS classes for one page's own layout carry a per-page prefix, so they do
  not collide with shared classes.
- A `?s=N` query renders only page N, for render checks.
- Slides only: a presentation mode — F or Enter starts it, the arrow keys
  move between pages, Esc leaves it.

Progress section
- The document's last part, not formatted as a page and allowed to be long:
  the outline with a per-item status (done / under review / not started), the
  page-number map, the review-pending list, and open questions. Regenerate it
  from `plan.md` after each round. CSS hides it in print and in presentation
  mode, so the PDF never carries it.

Render check
- Find an available headless browser (Chrome, Edge, Chromium, or Playwright),
  screenshot each new page through `?s=N`, and inspect the image for overflow
  and collisions. With none available, tell the user the page is unverified
  and ask them to look at it.

PDF
- PDF is the only output format. Print through the headless browser; with
  none available, tell the user to open the document and use the browser's
  Print → Save as PDF, which the `@page` rule keeps faithful.
