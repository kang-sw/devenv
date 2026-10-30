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
- With a template chosen at step 2, follow its look. Without one, the look of
  the document's pages is your own first draft: choose it on the first page
  and let the user correct it. In `On: template`, the look follows the user's
  reference.
- The progress section keeps its fixed structure and theme (`## Mechanics`)
  whatever the look, so every document's checklist reads the same.

## On: invoke

A request to make a reusable template, rather than a document, takes
`On: template`; every other request follows these steps.

1. Discuss the topic: audience, purpose, and length or talk time.
2. Settle the form: a slide deck (16:9 landscape) or an A4 document
   (portrait); the structural elements (cover, part dividers, appendix, and
   so on); text density; and any reference document the user supplies.
   List the templates found (`## Templates`) and let the user pick one or
   choose a fresh draft; a chosen template fixes the page format and offers
   its page kinds. Confirm the form with the user before step 3.
3. Propose the working folder `ai-docs/paged/<stem>/`, where `<stem>` is
   `YYMMDD-<slug>` with today's date, and ask the user whether the folder
   should be committed; the default is not committed. Then create the folder
   and, unless the answer is "commit", write a `.gitignore` inside it whose
   content is `*`. A later change of mind is the user asking you to delete
   that `.gitignore`.
4. Create `plan.md` and `document.html` in the folder. With a chosen
   template, copy its CSS and page-kind skeletons from `template.html` into
   `document.html` rather than linking the file, because the document must
   stand alone for PDF baking and when moved; record the template's name and
   location in `plan.md`.
5. Discuss the overall order and direction of the document; once the user
   confirms it, record it in `plan.md`.
6. After the user confirms, build the document from the front, one or two
   sections per round: write the pages, render-check each new page,
   regenerate the progress section, and stop for the user's review. Apply the
   corrections, record each in `plan.md`, and repeat.
7. When the document is complete, ask whether to bake a PDF, and where and
   under what file name.
8. Ask whether to save the document's look as a template (`## Templates`).
   When the document used a template and its look changed — `plan.md`
   records a correction to the shared CSS or a page-kind skeleton, or an
   added page kind — ask instead whether to update that template or save the
   look under a new name.
9. Report the working-folder path, whether the folder is committed, the
   template used or saved with its location, and the PDF path when one was
   baked.

## On: template

1. Settle the template: its name, the reference material the user supplies
   (HTML, PDF, screenshots, or anything else you can read), the page format
   (16:9 slides or A4), and the page kinds it offers — at least cover, part
   divider, and body. Confirm with the user before step 2.
2. Allocate a working folder and create `plan.md` and `document.html` in it,
   as in `On: invoke` steps 3 and 4.
3. Build one sample page per page kind in the reference's look, with
   placeholder text. Render-check each page and stop for the user's review;
   apply the corrections, record each in `plan.md`, and repeat until the user
   approves the look.
4. Save the template (`## Templates`).
5. Report the template's folder path and whether it was committed.

## Templates

Location
- A template is a folder `<name>/` in the project at
  `ai-docs/paged-templates/` or in the global location: when your shell sees
  `$WS_CONFIG_HOME`, `$WS_CONFIG_HOME/paged-templates/`, otherwise
  `~/.ws/paged-templates/`. The global location serves a template reused
  across projects; the project location serves one that belongs to this
  project or is shared with the team through the repository.
- When both locations hold the same name, the project template wins, as the
  narrower scope. Every template list shown to the user marks each entry as
  project or global.

Contents
- `template.html`: the shared CSS and one skeleton per page kind (cover, part
  divider, body, table, and so on); it renders as a sample of the look.
- `template.md`: the page-kind list and when to use each, the page format,
  and the source of the look (a finished document or a reference).
- The skeletons carry placeholder text only, never a document's real
  content: a global template travels into other projects, and real text
  would carry possibly confidential material out of its project.

Save
- Write the template folder only at this save step; samples and drafts stay
  in the working folder.
- Save globally by default; save to the project location when the user says
  the template is for this project only or for team sharing. Name the
  location to the user before writing.
- Saving to the project location asks whether to commit the template folder.
  Commit it only on a yes, following the project's commit rules; otherwise
  write a `.gitignore` containing `*` in the template folder, so a later
  broad commit cannot sweep it in. Updating an already-committed project
  template asks again before committing the change. A global template is
  outside the repository and is never committed.

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
    font: 15px/1.5 system-ui, sans-serif;
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
  .p-foot .id { font-family: ui-monospace, monospace; color: var(--p-mute); font-size: 13px; }
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
