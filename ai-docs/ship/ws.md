# Ship: ws

Ships the `ws` / `wsflow` plugin packages as a cut-point release: a
`release/<version>` branch cut at the review frontier head carries the version
bump, `main` fast-forwards to it, and a `v<version>` tag on it triggers the
`ws-mcp release` GitHub Actions workflow (`.github/workflows/ws-mcp-release.yml`)
to build cross-platform `ws-mcp` assets plus macOS/Windows desktop archives and
publish them to the same GitHub release. `develop` keeps taking merges for the whole release; the release branch is merged back
into it afterwards, which keeps `main` an ancestor of `develop`.

## Version Strategy

Each ship owns exactly one patch-version bump, and a version counts as claimed
only once its `v<version>` tag exists on origin. Read the version on the
release branch after Release Branch's Cut and Merge release target steps, so a
merged `main` brings in the previous release's version: read
`agents-plugin/runtime.json` `.plugin_version` and run
`git ls-remote --tags origin v<current>`:

- Non-empty (the normal case: the previous ship tagged it): increment the patch
  component and run `agents-plugin-tool/scripts/bump-ws-version.sh
  <next-version>`. Commit the script-generated version edits on the release
  branch before testing or tagging.
- Empty (an earlier bump reached the cut point but its tag was never pushed):
  reuse `<current>` as this ship's version. No bump script run, no new version
  commit.

Ordinary merges into `develop` do not bump the version. The tag is `.release_tag`
(`v<plugin_version>`). Ship refuses if `.release_tag != "v" + .plugin_version`
for either `agents-plugin/runtime.json` or `agents-plugin-wsflow/runtime.json`
(the workflow enforces the same contract).

## Release Branch

All release-branch work runs in a dedicated worktree, never in the main
checkout: parallel lead sessions share that checkout, and moving its HEAD would
land their commits on the release branch. Steps marked **(lead)** call
lead-only tools (`ws/worktree.*`, `ws/git.merge`) or hand work to
`ws:lead-delegate`, so they run in the lead session; the Execute delegate runs
the rest of Pre-flight in the worktree the lead hands it.

- Cut **(lead)**: first clear what an earlier attempt left, because a leftover
  lease or branch refuses the acquire and the later rename: release each
  `ws/worktree.list` worktree whose branch is `release/*`
  (`ws/worktree.release(path: <worktree>)`), then delete each local
  `release/*` branch (`git branch -D`), `release/pending-<short cut sha>`
  included. Then `ws/worktree.acquire(base: <cut>, target_branch:
  release/pending-<short cut sha>, profile: "full")`; run every
  release-branch command below in the returned worktree. Once Version
  Strategy has the version, rename the branch there with
  `git branch -m release/<version>`.
- Merge release target: when `git merge-base --is-ancestor main <cut>` fails
  (the cut predates the previous release's merge back), run `git merge --no-ff
  main` on the release branch before Version Strategy reads the version,
  resolving any conflict per Conflicts below.
- Promote (Publish): `git checkout main && git merge --ff-only
  <release-head>` in the release worktree.
- Merge back (Post-ship), when `git merge-base --is-ancestor <release-head>
  develop` fails (the release branch carries a bump commit or the merged
  `main`). Without it, `main` would stop being an ancestor of `develop` and
  the next release would need the Merge release target step.
  1. In the release worktree, `git checkout release/<version> && git merge
     --no-ff develop`, resolving any conflict per Conflicts below.
  2. **(lead)** When `git merge-base --is-ancestor develop release/<version>`
     fails (`develop` moved after step 1), repeat step 1, so this merge
     cannot conflict and leave an in-progress merge in the shared main
     checkout. Then, from the main checkout, `ws/git.merge(branch:
     "release/<version>", target: "develop")` and `git push origin develop`.
- Conflicts (Merge release target and Merge back) resolve in the release
  worktree. On a line `bump-ws-version.sh` writes, take the higher version;
  hand any other conflict **(lead)** to `ws:lead-delegate` with the worktree
  path. Commit the merge, then rerun Pre-flight's release-contract check on
  the release branch head. A conflict is never a stop: a restart cuts at the
  same point and meets the same conflict.
- Stops **(lead)**: every stop after Cut releases the release worktree
  (`ws/worktree.release(path: <worktree>)`), which detaches its HEAD and frees
  the release branch for the restart's Cut. That covers a red Pre-flight
  test, a failed release-contract or marketplace check, a conflict
  `ws:lead-delegate` could not resolve, a declined final gate, a rejected
  push, and a red CI gate. A rejected `main` push also restores local `main`
  with `git branch -f main origin/main` once the worktree is released, and
  deletes the unpushed local tag (`git tag -d v<version>`), so the restart's
  ref check passes. A stop before the tag push restarts Pre-flight from the
  top, after a fix forward on `develop` through ordinary review when a check
  was red; the version's tag was never pushed, so Version Strategy derives
  the same version again. A Merge back stop keeps `release/<version>` for a
  later Merge back.

## Pre-flight

**Sync against origin FIRST — never ship from stale local refs.** This is the
recurring failure mode: a parallel session ships a patch, `origin/main` and the
`v<version>` tag advance, and a local ship that skipped `fetch` cuts from stale
refs (colliding on the version/tag). Every check below runs against freshly
fetched state.

- `git fetch origin --tags` — refresh remote-tracking refs and tags before any
  other check.
- Local refs are up to date: `git merge-base --is-ancestor origin/develop
  develop` (local `develop` contains all of `origin/develop`), and
  `git rev-parse main` equals `git rev-parse origin/main`. If either fails,
  **stop and reconcile first** (`git merge origin/develop` into `develop`;
  fast-forward local `main` to `origin/main`), then restart Pre-flight.
- **Release gate** **(lead)** (this project declares `release-boundary:
  present` in `AGENTS.md` `### Review Policy`): run `lead-ship`'s Release
  gate over `develop`. Check `review.marker(format: json)` `found` **first,
  before any rev-list call** — an empty `head` substituted into `git rev-list
  --count <frontier-head>..develop` resolves the empty side to `develop`'s own
  tip and silently reports `0`, which would wrongly read as clear. An override at any
  gate stop proceeds without stamping the marker (only `lead-review`'s own
  `review.stamp` step ever advances it).
- Resolve the cut point **(lead)**: `<cut>` is the head of the `pass` or
  `concern` stamp the gate's review wrote (the clean-prefix stamp's when it
  wrote one), or the frontier head when the gate ran no review. The
  `bootstrap` entry the review seeds on an empty ledger clears nothing: a
  review that wrote no `pass` or `concern` stamp stops for an explicit
  override. For an override, or the gate's own bootstrap choice, `<cut>` is
  `develop`'s tip at that decision. Proceed only when `git merge-base
  --is-ancestor <cut> main` fails; otherwise nothing is newly cleared and the
  ship stops.
- Long tests **(lead)**: ask once, before handing off Pre-flight (and thus
  before Publish's final gate), via a whiteboard ask: "long test 돌릴까요?"
  Record the choice for this ship and pass it to the delegate. Default **no**
  selects the regular group; explicit **yes** adds the long group locally and
  in CI. This choice covers only the long group; all other gates still apply.
- Release Branch: Cut **(lead)** at `<cut>` and hand the worktree to the
  delegate, which runs Merge release target when its guard fails and the
  steps below.
- Resolve this ship's version per Version Strategy on the release branch, bump
  and commit when the current version's tag is claimed, then rename the branch
  to `release/<version>`. A `release/<version>` an earlier attempt left is
  gone locally (Cut deleted it) and is replaced on origin by Publish's
  force-push: its bump never reached `develop`, so Version Strategy derives
  the same version again.
- Version tag unclaimed: `git ls-remote --tags origin v<version>` is empty for the
  bumped or reused version. A non-empty result means a concurrent release claimed the
  version; increment the patch again, commit the regenerated version edits,
  rename the branch, re-run tests, and restart the tag check.
- `cd agents-plugin-tool && bash scripts/ci-test-shard.sh all` on the release
  branch head — the regular group must pass. When the owner chose **yes**,
  also run `bash scripts/ci-test-shard.sh long` from `agents-plugin-tool` and
  require it to pass. The script owns long-group membership; `all` excludes it.
- Release contract on the release branch head: for
  `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json`, assert
  `.release_tag == "v" + .plugin_version`.
- Marketplace sanity on the release branch head: `.plugins[].name` in both
  `.agents/plugins/marketplace.json` and `.claude-plugin/marketplace.json` is
  exactly `ws wsflow`.
- Record `git rev-parse HEAD` in the release worktree as `<release-head>`. The
  CI gate, the main promotion, and the tag in Publish all target this exact
  SHA. `develop` moving afterwards changes nothing that ships, so there is no
  tip re-check.

## Build

- No local build. The tagged push drives `ws-mcp release` on GitHub Actions,
  which requires a green `ws-mcp CI` run on the tagged commit (it does not
  re-run `go test`; Publish's CI gate is the release's only test run),
  validates the plugin release contract, builds release assets
  (`agents-plugin-tool/scripts/build-release-assets.sh`), and runs the Windows
  executable smoke.
- Desktop assets now come from that same CI run (owner (1224)=a), not a manual
  package build. The macOS (`darwin-arm64`) and Windows (`win32-x64`) jobs
  download the existing `ws-mcp-assets` workflow artifact, so the daemon binary
  and adjacent metadata are exactly the native release build's bytes. Each
  installs the lockfile-pinned desktop dependencies and pinned Electron zip,
  assembles a `wsflow` daemon input with
  `agents-plugin-tool/desktop-electron/scripts/assemble-daemon-input.cjs`, then
  runs `desktop-electron/scripts/build.cjs` without launching Electron. The
  packaged app version comes from the validated plugin runtime contract.
- A final job waits for both desktop platforms, verifies each archive against
  its platform checksum, and writes one combined
  `Werkschaft-desktop-SHA256SUMS` (SHA256, two spaces, asset filename per line).
  It uploads exactly `Werkschaft-darwin-arm64.zip`,
  `Werkschaft-win32-x64.zip`, and that combined SUMS file to the native assets'
  tag release. These are the updater's existing asset names. The same three
  files are retained as the `werkschaft-desktop-assets` workflow artifact.
- **Unsigned / unnotarized:** no signing secrets are configured. Gatekeeper may
  block the downloaded macOS app until the user explicitly approves it;
  Windows may show SmartScreen warnings. A green packaging job is not proof of
  installed-app launch, updater swap, daemon lifetime, or platform acceptance.
- The native release can appear before desktop packaging finishes. Treat the
  release workflow as incomplete until **all** jobs, including
  `desktop-release`, succeed; do not infer desktop availability from the native
  assets alone.

### Desktop dry run before shipping

After the workflow changes are on `develop`, the lead can build both platforms
without creating or uploading to any GitHub release:

```sh
gh workflow run ws-mcp-release.yml --ref develop -f desktop_dry_run=true
gh run list --workflow ws-mcp-release.yml --branch develop --event workflow_dispatch
gh run watch <run-id> --exit-status
gh run download <run-id> -n werkschaft-desktop-assets -D desktop-dry-run
(cd desktop-dry-run && shasum -a 256 -c Werkschaft-desktop-SHA256SUMS)
```

Confirm the selected run's head SHA is the intended `develop` commit before
using its result. The existing native build and Windows executable smoke also
run; desktop packaging never launches the app. `desktop_dry_run=true` vetoes
**both** native and desktop release-publishing steps, even if dispatched on a
tag. Ordinary branch dispatches without this flag retain native-only behavior.
The optional `version` input defaults to the bump-maintained development
version; an override (`-f version=X.Y.Z-dev`) must match the plugin/runtime
numeric core or desktop assembly fails closed. Keep this verification before
the release tag push; it is not a substitute for Publish's native CI gate or
real packaged-app acceptance.

## Tag

Format: `v<version>` (e.g. `v0.42.1`), read from `agents-plugin/runtime.json`
`.release_tag`. Placed on `<release-head>` after Publish's CI gate passes and
main is promoted to it. Push: yes (covered by Publish's single final-gate
approval).

## Publish

Order is deliberate: publish the release branch first, let `ws-mcp CI`
(`.github/workflows/ws-mcp-ci.yml`, ubuntu + sharded windows matrix, plus the
long legs when requested) pass on `<release-head>`, and only then promote
`main` and push the tag. Neither `main`
nor the tag is published before the release commit is green on Linux and
Windows, which the local Pre-flight `go test` cannot vouch for. The final gate
precedes every push — all pushes below are reversible-only-before-they-run.

- Final gate: show version, tag, `<cut>`, `release/<version>` at
  `<release-head>`, and the publish sequence below; wait for explicit approval.
  This single approval covers the whole sequence (release branch push, CI
  wait, main + tag push): a green CI gate proceeds to the main and tag pushes
  without another prompt, and a failed one stops without further pushes.

1. `git push --force origin release/<version>`. This claims no version and
   publishes no release; `--force` replaces a `release/<version>` an earlier
   stopped attempt pushed.
2. **CI gate** on `<release-head>`. A branch push starts no `ws-mcp CI` run
   (the workflow runs only on pull requests and manual dispatch), so the gate
   dispatches one on the release branch.
   1. Reuse a run only when it already passed: `gh run list --workflow
      ws-mcp-ci.yml --commit <release-head> --json
      databaseId,headSha,event,status,conclusion`, and take the newest entry
      whose `conclusion` is `success` and whose jobs (`gh run view <run-id>
      --json jobs --jq '.jobs[] | [.name, .conclusion]'`) meet step 3 for this
      ship's long-test choice. Then continue with step 3 (`gh run watch`
      returns at once for a finished run).
   2. No such run: dispatch one with
      `gh workflow run ws-mcp-ci.yml --ref release/<version>` (append
      `-f long_tests=true` when the owner chose **yes**), then take the
      newest run from `gh run list --workflow ws-mcp-ci.yml --event
      workflow_dispatch --json databaseId,headSha,status,conclusion` and
      confirm its `headSha` equals `<release-head>`, the commit that gets
      tagged (`ws-mcp-release.yml` requires a green ws-mcp CI run on the tag's
      commit). A different `headSha` means `origin/release/<version>` moved
      after the push: stop.
   3. `gh run watch <run-id> --exit-status` (the Windows shards run in
      parallel), then `gh run view <run-id> --json jobs --jq '.jobs[] |
      [.name, .conclusion]'`: the ubuntu `all` leg and every Windows shard
      must conclude `success`. With **yes**, both `go test (ubuntu-latest,
      long)` and `go test (windows-latest, long)` must also be present and
      conclude `success`. With **no**, only those two named long legs may
      conclude `skipped` (a successful long leg is also acceptable). Every
      other job must conclude `success`.

   **CI gate failure** (a failed, cancelled, timed-out, or otherwise
   nonconforming job): stop per Release Branch's Stops; `main` and the tag
   stay unpushed.
3. Once the CI gate passes: Release Branch's Promote (`main` `--ff-only` to
   `<release-head>`), `git tag v<version> <release-head>`, then
   `git push origin main` and `git push origin v<version>` — the tag push
   triggers the GitHub release workflow.

There is no "push develop first" step: develop pushes are ordinary and
independent of the release.

## Post-ship

- **Automatically** watch the `ws-mcp release` workflow run for `v<version>` to
  green (native build/publish, windows-smoke, both desktop builds, and combined
  desktop asset verification/publish). This is an
  unprompted post-ship step: attach to the run without asking the user for
  approval, and report only the terminal result — green, or the failure with
  its failing job/step. Monitoring is read-only and pushes nothing, so it never
  needs a confirmation gate. (`gh run watch <run-id> --exit-status` attaches and
  exits non-zero on failure.)
- Release Branch's Merge back, in the release worktree, then from the main
  checkout.
- **(lead)** Release the release worktree
  (`ws/worktree.release(path: <worktree>)`).
- Delete the release branch: `git push origin --delete release/<version>` and
  `git branch -D release/<version>`. The next develop merge resumes normal
  development without a version bump; the next ship owns the next patch bump.
