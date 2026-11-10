---
kind: print
---

# Ship

You are the lead releasing a project. A release is low-reversibility, so its
decisions stop with you and the user; the mechanical steps run in a delegate
with the config as its input.

## Config

`ai-docs/ship/<proj>.md` is committed, for public targets;
`ai-docs/ship/<proj>.local.md` is gitignored, for private targets, and wins
when both exist. A named project loads that pair or stops with an error; no
argument loads the single config, asks which of several, or goes to **No
config**. The config is the only source of steps: never infer a version
without an explicit strategy in it, and never add a step it does not list.
The release gate below is the one exception and cannot be omitted by a
config.

A config with a `## Release Branch` section makes every gated release a
cut-point release: it ships the review frontier head (the cut point) through a
release branch while the review track keeps taking merges, so a blocked merge
late in the range does not stall the release; where the release gate does
not apply, the cut point is the review-track tip. Without that section, the
release promotes the review-track tip. The config's steps decide where the
release branch is checked out; steps that name no place use the current
checkout.

## Release gate

Applies when the project's `AGENTS.md` `### Review Policy` declares
`release-boundary: present`; otherwise skip to **Execute**.

1. Open review follow-ups, before any review below:
   `{{.McpNamespace}}/git.followups(category: "review", min_level: "minor",
   range: <start>..HEAD)`. Judge `<start>` from the project's release history:
   no later than just before the previous release's own release gate, so the
   follow-ups that gate accepted are inside the range; earlier costs only
   query time. With no identifiable previous release, omit `range`. Show the
   result to the user; it never blocks. Re-check each `important` and
   `critical` item against HEAD: one still valid stays open; one already
   fixed, or whose target is gone, is resolved by passing its id in
   `resolves` on the commit this ship makes before publishing (hand the ids
   to the Execute delegate), or, when the ship makes none, on a dedicated
   `chore(review)` commit made now so the review below covers it. `minor`
   items are listed only and left to age out of later ranges.
2. `{{.McpNamespace}}/review.marker(format: json)`. Read its `found` field
   first; never infer emptiness from a rev-list count, because an empty head
   in `git rev-list --count <head>..HEAD` resolves to `HEAD` and reports `0`.
3. `found: false`: all prior history is unreviewed. Stop for the user's
   choice: **bootstrap** (`review.marker(bootstrap: true)` accepts history as
   unreviewed and proceeds), or **review** (ask for an explicit base, then
   `{{.SkillNamespace}}:lead-review` over `range: <base>..HEAD`). Declining
   both stops here.
4. `found: true`: `git rev-list --count <frontier-head>..HEAD`. `0` proceeds.
   Otherwise `{{.SkillNamespace}}:lead-review` over `range:
   <frontier-head>..HEAD`; a clearing verdict proceeds, anything else stops
   for the user's explicit override with a recommendation against it.
5. A cut-point release replaces the outcome of steps 3 and 4: once they
   finish, read `review.marker` again; its head is the cut point. The gate
   proceeds when the cut point is not an ancestor of the release target, even
   when the review left a blocked remainder, because only content at or below
   the cut point ships. Otherwise it stops: nothing is newly cleared. The
   user's explicit override still means shipping uncleared content: its cut
   point is the review-track tip at override time.
6. This gate never calls `review.stamp`; apart from the step-3 bootstrap the
   marker advances only through the review skill. An override leaves it where
   it was.

## Execute

1. Delegate pre-flight, version derivation, tag creation (not pushed), and
   build/package to a subagent with the config path; it returns version,
   tag, and the full output of each command. For a cut-point release, also
   pass the cut point: the delegate runs the config's Cut step there, and
   before any version bump checks that the release target is an ancestor of
   the cut point. When it is not (the cut predates the previous release's
   merge back), the delegate runs the config's merge of the release target
   into the release branch first, or stops when the config declares none. A
   version bump the config's strategy makes lands on the release branch.
2. Confirm with the user: version, tag, publish targets, and for a cut-point
   release the cut point and the release branch. Wait for explicit approval.
3. Publish per the config. When a publish step promotes one branch into
   another, pin the head the gate cleared and re-check the source branch tip
   immediately before the merge; if it moved, abort and re-run the gate over
   the delta. The cleared head is the tip after the gate's own lead-review
   returns, its ledger commit included; pinning before that commit would
   re-run the gate forever. A gate that did not apply leaves nothing to
   re-check. A cut-point release skips this re-check: what publishes is the
   pinned cut point and the release branch head on top of it, so review-track
   movement during the release changes nothing that ships.
4. Push the tag when the config's `Tag` section asks for it; the step-2
   confirmation covers that push. Run post-ship steps; a cut-point release
   runs the config's merge of the release branch back into the review track,
   which keeps the release target an ancestor of the review track for the
   next release.

## No config

Ask for the sub-project, public or private target, version strategy (manual
semver, auto-increment patch, date-based, `git describe`, or another explicit
rule), build and publish commands, tag format and whether the tag is pushed,
post-ship steps, and whether releases ship a cut point through a release
branch; write the config to the matching path and confirm it before
executing.

### Ship Config Format

```markdown
# Ship: <proj>

## Version Strategy
<how the version is derived or bumped - be specific enough that no judgment is needed at ship time>

## Pre-flight
- <check command>

## Build
- <build or package command>

## Publish
- <publish command and target>

## Release Branch                      ← optional; makes every gated release a cut-point release
- Cut: <create the release branch at the cut point, and where it is checked out>
- Merge release target: <merge the release target into the release branch>
- Promote: <publish step: fast-forward the release target to the release branch head>
- Merge back: <post-ship step: merge the release branch into the review track>

## Tag
Format: `<prefix><version>` (e.g. `v1.2.3` or `proj-2024.04.19`)
Push: yes

## Post-ship
- <optional post-ship steps>
```

Omit sections that do not apply.

## Stops

The release-gate decision; the publish confirmation; any push of a tag or
branch.

## Output

What shipped: version, tag, targets, the cut point of a cut-point release,
deviations. Config and version files in English.
