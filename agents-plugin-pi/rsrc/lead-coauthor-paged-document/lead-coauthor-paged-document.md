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
- The visual look of the document's pages is your own first draft: choose it
  on the first page and let the user correct it. The one exception is the
  progress section, whose structure and theme are fixed (`## Mechanics`) so
  every document's checklist reads the same.

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
- The document's last part, not formatted as a page and allowed to be long.
  Regenerate it from `plan.md` after each round. Write its labels in the
  document's language. Its structure, in order:
  1. Eyebrow: `<checklist label> · <stage> (<done>/<total>)`, followed by a
     small muted note that the section is hidden in print and presentation.
  2. Title: one line such as "Overall structure and planned content".
  3. A two-column list with one row per planned page (per section for an A4
     document): the status marker, a bold label carrying the on-screen page
     number once the page exists (none for an unbuilt page), and one line of
     planned content. There are exactly three markers: `✓` done (green),
     `◐` under review (amber), `☐` not started (gray).
  4. Footer: a "Review pending" line naming pages by on-screen number and an
     "Open questions" line naming items by `(nn)` ID; omit a line that would
     be empty.
- Leave out a progress bar, the next action, and recent decisions; they live
  in `plan.md`.
- Use this theme verbatim. Its custom properties are scoped under
  `.progress`, so it neither inherits nor leaks the document's theme; it is
  hidden in print and while the `presenting` class is on `body` (rename that
  selector to match the presentation mode you build).

  ```html
  <style>
  .progress { --p-ink: #1f2328; --p-mute: #656d76; --p-line: #e1e4e8; --p-accent: #1f6f8b;
    --p-done: #1a7f37; --p-review: #bf8700; --p-todo: #8c959f;
    max-width: 1280px; margin: 32px auto; background: #fff; color: var(--p-ink);
    font: 15px/1.5 system-ui, -apple-system, "Segoe UI", "Malgun Gothic", sans-serif;
    padding: 28px 40px 32px; box-sizing: border-box; border-top: 4px solid var(--p-ink); }
  .p-eyebrow { font-size: 14px; font-weight: 700; color: var(--p-accent); letter-spacing: .02em; }
  .p-eyebrow span { color: var(--p-mute); font-weight: 500; margin-left: 8px; }
  .p-title { font-size: 30px; font-weight: 800; margin: 4px 0 18px; }
  .p-list { columns: 2; column-gap: 40px; margin: 0; padding: 0; list-style: none; }
  .p-list li { break-inside: avoid; display: grid; grid-template-columns: 22px 92px 1fr; gap: 0 8px;
    padding: 7px 0; border-bottom: 1px solid var(--p-line); }
  .p-list .m, .p-list .l { font-weight: 700; }
  .p-list .done .m { color: var(--p-done); }
  .p-list .review .m { color: var(--p-review); }
  .p-list .todo .m { color: var(--p-todo); }
  .p-list .todo .l { color: #3d444d; }
  .p-list .pg { color: var(--p-mute); font-size: 12px; font-weight: 400; margin-left: 4px;
    font-variant-numeric: tabular-nums; }
  .p-foot { display: grid; grid-template-columns: 92px 1fr; gap: 6px 12px; margin-top: 20px; font-size: 14px; }
  .p-foot b { color: var(--p-mute); font-weight: 600; }
  .p-foot .id { font-family: ui-monospace, Consolas, monospace; color: var(--p-mute); font-size: 13px; }
  @media print { .progress { display: none !important; } }
  body.presenting .progress { display: none; }
  </style>

  <div class="progress">
    <div class="p-eyebrow">Checklist · Drafting (2/4)<span>Hidden in print and presentation</span></div>
    <div class="p-title">Overall structure and planned content</div>
    <ul class="p-list">
      <li class="done"><span class="m">✓</span><span class="l">Cover<span class="pg">1</span></span><span>Title, audience, date</span></li>
      <li class="done"><span class="m">✓</span><span class="l">Agenda<span class="pg">2</span></span><span>Three parts and an appendix</span></li>
      <li class="review"><span class="m">◐</span><span class="l">Part 1<span class="pg">3</span></span><span>Why the change</span></li>
      <li class="todo"><span class="m">☐</span><span class="l">Part 2</span><span>How to apply it</span></li>
    </ul>
    <div class="p-foot">
      <b>Review pending</b><span>p.3 diagram density</span>
      <b>Open questions</b><span><span class="id">(01)</span> Include the appendix?</span>
    </div>
  </div>
  ```

Render check
- Find an available headless browser (Chrome, Edge, Chromium, or Playwright),
  screenshot each new page through `?s=N`, and inspect the image for overflow
  and collisions. With none available, tell the user the page is unverified
  and ask them to look at it.

PDF
- PDF is the only output format. Print through the headless browser; with
  none available, tell the user to open the document and use the browser's
  Print → Save as PDF, which the `@page` rule keeps faithful.
