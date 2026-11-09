# Plan: ws-mcp CI runs only on manual dispatch (and PRs)

## Decision (user, whiteboard 2026-11-10)
- (544) a: remove every push trigger from `ws-mcp CI`; a `[ci]` tag in a commit
  title no longer starts anything. The ship flow dispatches CI on the pinned
  develop commit and pushes `main` and the tag only when that run is green.
- (545) ok: keep the `pull_request` trigger (and leave `ws-mcp-release.yml`'s
  own triggers alone).

## Why
The `[ci]` convention silently failed under the workflow's `paths:` filter: the
0.48.13 pin `864827548` touched only `ai-docs/.review-ledger.md`, so its `[ci]`
push started no run. `ws-mcp release` already accepts any successful
`ws-mcp CI` run on the tagged SHA regardless of event
(`ws-mcp-release.yml:70-75`), so dispatch-only CI needs no release change
beyond wording.

## Changes
1. `.github/workflows/ws-mcp-ci.yml`: delete the `push:` block and the job-level
   `if:` that tests for `[ci]`; rewrite the header and job comments to say CI
   runs on pull requests and manual dispatch, and that ship dispatches it on
   the commit to tag.
2. `ai-docs/ship/ws.md` Publish CI gate: always dispatch
   `gh workflow run ws-mcp-ci.yml --ref develop` after the develop push, unless
   a successful run with non-skipped jobs already exists for
   `<reviewed-through-sha>` (reuse it); state that a develop push starts no CI.
   Keep the headSha check, watch, and every-leg-success rule. Update the
   Build section wording if it mentions `[ci]`.
3. Remove or rewrite remaining `[ci]`-convention mentions in tracked docs and
   comments (e.g. `ws-mcp-release.yml` error text, `ai-docs/manuals/ws-mcp.md`,
   `AGENTS.md` if present). Leave historical records alone (`.review-ledger.md`,
   `.plans/`, `.done/` tickets, commit history).

## Verification
- `grep -rn '\[ci\]'` over tracked live docs/workflows shows no remaining
  instruction to rely on `[ci]`.
- YAML parses (`python3 -c 'import yaml,sys; yaml.safe_load(open(...))'` or
  `ruby -ryaml`), and `on:` holds only `pull_request` and `workflow_dispatch`.
- No Go tests needed (no Go change).
