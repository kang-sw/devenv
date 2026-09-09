---
name: lead-ship
description: Ship, release, publish, tag, or deploy a configured project from its `ai-docs/ship` config. The release gate and the publish confirmation stay with the lead and the user; the mechanical steps go to a delegate.
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

## Release gate

Applies when the project's `AGENTS.md` `### Review Policy` declares
`release-boundary: present`; otherwise skip to **Execute**.

1. `{{.McpNamespace}}/review.marker(format: json)`. Read its `found` field
   first; never infer emptiness from a rev-list count, because an empty head
   in `git rev-list --count <head>..HEAD` resolves to `HEAD` and reports `0`.
2. `found: false`: all prior history is unreviewed. Stop for the user's
   choice: **bootstrap** (`review.marker(bootstrap: true)` accepts history as
   unreviewed and proceeds), or **review** (ask for an explicit base, then
   `{{.SkillNamespace}}:lead-review` over `range: <base>..HEAD`). Declining
   both stops here.
3. `found: true`: `git rev-list --count <frontier-head>..HEAD`. `0` proceeds.
   Otherwise `{{.SkillNamespace}}:lead-review` over `range:
   <frontier-head>..HEAD`; a clearing verdict proceeds, anything else stops
   for the user's explicit override with a recommendation against it.
4. This gate never calls `review.stamp`; the marker moves only through the
   review skill. An override leaves it where it was.

## Execute

1. Delegate pre-flight, version derivation, tag creation (not pushed), and
   build/package to a subagent with the config path; it returns version,
   tag, and the full output of each command.
2. Confirm with the user: version, tag, publish targets. Wait for explicit
   approval.
3. Publish per the config. When a publish step promotes one branch into
   another, pin the gate's reviewed through-SHA and re-assert it immediately
   before the merge; if the branch moved, abort and re-run the gate over the
   delta.
4. Push the tag; run post-ship steps.

Report version, tag, targets, and any deviation.

## No config

Ask for the sub-project, public or private target, version strategy (manual
semver, auto-increment patch, date-based, `git describe`, or another explicit
rule), build and publish commands, and post-ship steps; write the config to
the matching path and confirm it before executing.

[design-review: carry the existing `## Ship Config Format` block unchanged.]

## Stops

The release-gate decision; the publish confirmation; any push of a tag or
branch.

## Output

What shipped: version, tag, targets, deviations. Config and version files in
English.
