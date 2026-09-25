# Ship: ws

Ships the `ws` / `wsflow` plugin packages by promoting `develop` to `main` and
pushing a `v<version>` tag, which triggers the `ws-mcp release` GitHub Actions
workflow (`.github/workflows/ws-mcp-release.yml`) to build cross-platform
`ws-mcp` assets and publish a GitHub release.

## Version Strategy

Each ship owns exactly one patch-version bump, and a version counts as claimed
only once its `v<version>` tag exists on origin. After confirming that local
`develop` contains `origin/develop`, read the current version from
`agents-plugin/runtime.json` `.plugin_version` and run
`git ls-remote --tags origin v<current>`:

- Non-empty (the normal case: the previous ship tagged it): increment the patch
  component and run `agents-plugin-tool/scripts/bump-ws-version.sh
  <next-version>`. Commit the script-generated version edits on `develop`
  before testing or tagging.
- Empty (an earlier ship bumped to this version but stopped before pushing the
  tag, e.g. at Publish's CI gate): reuse `<current>` as this ship's version. No
  bump script run, no new version commit.

Ordinary merges into `develop` do not bump the version. The tag is `.release_tag`
(`v<plugin_version>`). Ship refuses if `.release_tag != "v" + .plugin_version`
for either `agents-plugin/runtime.json` or `agents-plugin-wsflow/runtime.json`
(the workflow enforces the same contract).

## Pre-flight

**Sync develop against origin FIRST — never ship from stale local refs.** This
is the recurring failure mode: a parallel session ships a patch, `origin/develop`
and the `v<version>` tag advance, and a local ship that skipped `fetch` promotes
stale `develop` (colliding on the version/tag). Every check below runs against
freshly fetched state.

- **Release gate** (this project declares `release-boundary: present` in
  `AGENTS.md` `### Review Policy` — see `lead-ship`'s Release gate section):
  call `review.marker(format: json)` to resolve the review-watermark
  frontier. Check `found` **first, before any rev-list call** — an empty
  `head` substituted into `git rev-list --count <frontier-head>..develop`
  resolves the empty side to `develop`'s own tip and silently reports `0`,
  which would wrongly read as clear.
  - `found: false` (no ledger entry yet — this project's first ship): treat
    all prior history as review-skipped, **not clear**, and **stop for an
    explicit decision** between (i) `review.marker(bootstrap: true)` to
    accept prior history as unreviewed (seeds `<HEAD>..<HEAD>`, equivalent to
    an override, nothing reviewed — this is itself the explicit accept, so it
    proceeds directly) or (ii) an explicit `lead-review` over
    `range: <chosen-base>..develop` for a human-supplied base, which stamps
    and advances the marker. These do not compose. Option (ii) is a real
    review, not an accept-as-is: apply the same clears/not-clear handling as
    the `found: true` branch below to its outcome — a `block`/non-clearing
    result must stop for another explicit decision, not auto-proceed.
  - `found: true`: run `git rev-list --count <frontier-head>..develop`.
    Empty — proceed. Non-empty — trigger `lead-review` over
    `range: <frontier-head>..develop`; if it still doesn't clear, surface a
    strong recommendation and stop for an explicit decision.
  - An override at any of these stops proceeds without stamping the marker
    (only `lead-review`'s own `review.stamp` step ever advances it).
- `git fetch origin --tags` — refresh remote-tracking refs and tags before any
  other check.
- Develop is up to date: `git merge-base --is-ancestor origin/develop develop` —
  local `develop` must contain all of `origin/develop`. If it fails,
  `origin/develop` advanced (a parallel ship or merge): **stop and reconcile
  first** — `git merge origin/develop` into `develop`, then restart Pre-flight.
  Never promote a `develop` that is behind `origin/develop`.
- Resolve this ship's version per Version Strategy: bump through
  `bump-ws-version.sh` and commit the generated version edits on `develop`
  when the current version's tag is claimed; reuse the current version when it
  is not.
- Version tag unclaimed: `git ls-remote --tags origin v<version>` is empty for the
  bumped or reused version. A non-empty result means a concurrent release claimed the
  version; increment the patch again, commit the regenerated version edits,
  re-run tests, and restart the tag check.
- `git merge-base --is-ancestor main develop` — develop must be a linear
  descendant of main (fast-forwardable); abort otherwise.
- `cd agents-plugin-tool && go test ./...` — all packages green.
- Release contract: for `agents-plugin/runtime.json` and
  `agents-plugin-wsflow/runtime.json`, assert `.release_tag == "v" + .plugin_version`.
- Marketplace sanity: `.plugins[].name` in both
  `.agents/plugins/marketplace.json` and `.claude-plugin/marketplace.json` is
  exactly `ws wsflow`.
- **R4 pin**: once every check above passes (including the version-bump
  commit, when this ship made one), record `git rev-parse develop` as
  `<reviewed-through-sha>` — the tip this ship run has actually vetted,
  version bump included. The pin-and-re-assert in Publish (below) re-checks
  against this value immediately before the develop push, not against the
  pre-bump gate-time tip, so the ship's own version-bump commit never trips
  it. The CI gate and the main promotion in Publish also target this exact
  SHA.

## Build

- No local build. The tagged push drives `ws-mcp release` on GitHub Actions,
  which runs tests, validates the plugin release contract, builds release
  assets (`agents-plugin-tool/scripts/build-release-assets.sh`), and runs the
  Windows smoke.

## Tag

Format: `v<version>` (e.g. `v0.42.1`), read from `agents-plugin/runtime.json`
`.release_tag`. Placed on the `main` tip after Publish's CI gate passes and
main is promoted. Push: yes (covered by Publish's single final-gate approval).

## Publish

Order is deliberate: publish the synced `develop` first, let `ws-mcp CI`
(`.github/workflows/ws-mcp-ci.yml`, ubuntu + windows matrix) pass on the exact
pinned commit, and only then promote `main` and push the tag. `origin/main`
never advances ahead of `origin/develop`, and neither `main` nor the tag is
published before the release commit is green on Linux and Windows, which the
local Pre-flight `go test` cannot vouch for. The final gate precedes every push
— all pushes below are reversible-only-before-they-run.

- Final gate: show version, tag, `<reviewed-through-sha>`, and the publish
  sequence below; wait for explicit approval. This single approval covers the
  whole sequence (develop push, CI wait, main + tag push): a green CI gate
  proceeds to the main and tag pushes without another prompt, and a failed one
  stops without further pushes.
- **R4 re-assert**: re-run `git rev-parse develop` and compare it to the
  `<reviewed-through-sha>` recorded by Pre-flight's R4 pin. A match means no
  new commit landed on `develop` since Pre-flight finished — proceed. A
  mismatch means the tip moved (a parallel merge landed during the Confirm
  wait-for-approval gap or later): **abort this Publish step** and re-run
  Pre-flight's Release gate over just the delta
  (`<reviewed-through-sha>..develop`) before retrying.
- `git push origin develop` — publish the synced develop first. This claims no
  version and publishes no release.
- **CI gate** on `<reviewed-through-sha>`:
  1. Find its run: `gh run list --workflow ws-mcp-ci.yml --commit
     <reviewed-through-sha> --json databaseId,headSha,event,status,conclusion`,
     polling for up to 60 seconds after the push; take the newest entry (a run
     from an earlier attempt on the same SHA counts).
  2. No run: the push changed nothing under the workflow's path filter
     (`agents-plugin-tool/**` and the workflow file itself), e.g. a docs-only
     tip or an already-pushed develop. Dispatch one with
     `gh workflow run ws-mcp-ci.yml --ref develop`, then take the newest run
     from `gh run list --workflow ws-mcp-ci.yml --event workflow_dispatch
     --json databaseId,headSha,status,conclusion` and confirm its `headSha`
     equals `<reviewed-through-sha>`. A different `headSha` means
     `origin/develop` moved after the push: abort as in the R4 re-assert.
  3. `gh run watch <run-id> --exit-status` (the Windows leg takes ~30 min),
     then `gh run view <run-id> --json jobs --jq '.jobs[] | [.name,
     .conclusion]'`: both `go test (ubuntu-latest)` and
     `go test (windows-latest)` must conclude `success`.
- **CI red or cancelled** (any leg not `success`, including a timeout): stop.
  `main` and the tag stay unpushed; the pushed `develop` stays as is. Fix
  forward on `develop`, then restart Pre-flight from the top. The version's
  tag was never pushed, so Version Strategy reuses the same version instead of
  bumping again.
- `git checkout main && git merge --ff-only <reviewed-through-sha>` — promote
  the CI-vetted commit to main (matches the historical release shape: main's
  tip is develop's tip, no extra merge commit). The target is the pinned SHA,
  not `develop`, because local `develop` may move during the CI wait. Local
  only.
- `git push origin main` — promote the release branch.
- `git push origin v<version>` — triggers the GitHub release workflow.

## Post-ship

- **Automatically** watch the `ws-mcp release` workflow run for `v<version>` to
  green (build + publish GitHub release assets, windows-smoke). This is an
  unprompted post-ship step: attach to the run without asking the user for
  approval, and report only the terminal result — green, or the failure with
  its failing job/step. Monitoring is read-only and pushes nothing, so it never
  needs a confirmation gate. (`gh run watch <run-id> --exit-status` attaches and
  exits non-zero on failure.)
- Return to `develop` (`git checkout develop`). The next develop merge resumes
  normal development without a version bump; the next ship owns the next patch
  bump.
