---
kind: print
---

# Ship Config

Write a ship config for a project that has none.

1. Ask the user, in one response: sub-project, public or private target,
   version strategy (manual semver, auto-increment patch, date-based,
   `git describe`, or another explicit rule), build and publish commands,
   tag format and whether the tag is pushed, post-ship steps, and whether
   releases ship a cut point through a release branch.
2. Write the config from the template below to `ai-docs/ship/<proj>.md`
   (public) or `ai-docs/ship/<proj>.local.md` (private), and confirm it with
   the user before the release continues.

## Template

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
