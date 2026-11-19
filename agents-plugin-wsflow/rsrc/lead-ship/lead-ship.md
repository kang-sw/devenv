---
kind: print
---

# Ship

Release a configured project. The gate and the publish decision stay with
you and the user; a `ship-handler` worker runs the mechanical steps.

## Config

- `ai-docs/ship/<proj>.md` (committed, public targets) and
  `ai-docs/ship/<proj>.local.md` (gitignored, private targets); the local
  file wins when both exist.
- Named project → load that pair, or stop with an error.
- No argument → a single config loads; several → ask which; none → follow
  `{{.McpNamespace}}/playbook.read(name: "ship-config")` first.
- The config is the only source of steps. The one addition is the release
  gate below, which runs per `AGENTS.md` whatever the config lists.
- A `## Release Branch` section → every gated release is a cut-point
  release: it ships the review frontier head (the cut point) through a
  release branch while the review track keeps taking merges. Gate not
  applied → the cut point is the review-track tip. No section → the release
  promotes the review-track tip.
- The config's steps say where the release branch is checked out; a step
  that names no place runs in the current checkout.

## Release gate

Runs when the project's `AGENTS.md` `### Review Policy` declares
`release-boundary: present`; otherwise go to **Execute**.

1. Follow-ups, before any review:
   `{{.McpNamespace}}/git.followups(category: "review", min_level: "minor",
   range: <start>..HEAD)`.
   - `<start>`: no later than just before the previous release's own gate,
     so the follow-ups that gate accepted are inside. No identifiable
     previous release → omit `range`.
   - Show the result to the user; it never blocks.
   - Each `important` or `critical` item, re-checked against HEAD: still
     valid → stays open; fixed or its target gone → resolve it by its id in
     `resolves`: on the commit this ship makes before publishing (hand the
     ids to the handler), or, when the ship makes none, on a `chore(review)`
     commit made now so the review below covers it.
   - `minor` items → listed only.
2. `{{.McpNamespace}}/review.marker(format: json)` → read `found` first.
   Never infer emptiness from a rev-list count: an empty head in
   `git rev-list --count <head>..HEAD` resolves to `HEAD` and reports `0`.
3. `found: false` → all history is unreviewed; stop for the user's choice:
   - bootstrap → `review.marker(bootstrap: true)` accepts it as unreviewed;
     proceed.
   - review → ask for an explicit base, then
     `{{.SkillNamespace}}:lead-review` over `range: <base>..HEAD`; its
     outcome proceeds or stops as in step 4.
   - neither → stop.
4. `found: true` → `git rev-list --count <frontier-head>..HEAD`:
   - `0` → proceed.
   - otherwise → `{{.SkillNamespace}}:lead-review` over `range:
     <frontier-head>..HEAD`. Outcome `clear` → proceed; anything else → stop
     for the user's explicit override, recommending against it.
5. Cut-point release → this step replaces the outcome of steps 3 and 4:
   - The gate ran a review → only a `pass` or `concern` stamp that review
     wrote clears a cut point (its empty-ledger `bootstrap` seed does not).
     The cut point is that stamp's head, or the cleared-prefix stamp's head
     when the review wrote one. No such stamp → stop for the user's override
     as in step 4.
   - Step-4 count `0` → no review; the cut point is the frontier head.
   - Cut point not an ancestor of the release target → proceed, even with a
     blocked remainder above it. Otherwise → stop; nothing is newly cleared.
   - A user override → the cut point is the review-track tip at override
     time; the step-3 bootstrap counts as an override.
6. The gate calls no `review.stamp`. The marker moves only through the step-3
   bootstrap and lead-review; any other override leaves it where it was.

## Execute

1. `{{.McpNamespace}}/playbook.render(name: "ship-handler", session_key:
   <your key>)`; spawn it at the returned tier with:

   ```text
   Read <rendered-path> as your system prompt.
   Config: <path>. Cut point: <sha> | none. Resolves: <follow-up ids> | none.
   ```

   It runs the config's steps in the config's order and returns version,
   tag, and each command's full output. `blocked` on a step that needs a
   capability only you hold, such as a lead-only tool → run that step, send
   its output to the same agent, and it continues from there.
2. Confirm with the user: version, tag, publish targets; for a cut-point
   release, the cut point and the release branch. Wait for explicit
   approval.
3. Publish per the config:
   - A step that promotes one branch into another → pin the head the gate
     cleared and re-check the source branch tip right before the merge;
     moved → abort and re-run the gate over the delta.
   - The cleared head is the tip after the gate's own lead-review returns,
     its ledger commit included.
   - Gate not applied → nothing to re-check.
   - Cut-point release → no re-check; run the config's Promote step.
4. Push the tag when the config's `Tag` section asks; the step-2
   confirmation covers that push. Run the post-ship steps; a cut-point
   release runs the config's Merge back.

## Stops

The release-gate decision; the publish confirmation, which also covers the
tag push; any other push of a tag or branch.

## Output

What shipped: version, tag, targets, a cut-point release's cut point,
deviations. Config and version files in English.
