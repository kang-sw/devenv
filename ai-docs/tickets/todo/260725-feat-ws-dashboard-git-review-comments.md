---
title: Local code-review comments on diffs + copy-to-clipboard for agents
sage-review-design: required
parent: 260725-epic-ws-dashboard-git-panel
related:
  260725-feat-ws-dashboard-git-diff-view: the diff view this comment gutter layers onto
  260725-feat-ws-dashboard-design-guide: comment gutter/affordances built to this guide
related-mental-model:
  - ws-web-dashboard
---

# Local code-review comments on diffs + copy-to-clipboard for agents

## Background

Purpose: let the user do code review inside the dashboard — leave comments on a
commit line or line range in the diff view — and then hand the flagged segments
to an agent to analyze. There is no agent-connection spec for the dashboard yet,
so the bridge to an agent is a **clipboard copy** the user pastes into a prompt.

## Decisions

- **Comments are local, file-backed, not git-tracked.** Stored in a **gitignored
  `.ws-dashboard/reviews/` store** (a new per-repo local-data concept — today only
  `.ws-dashboard/worktrees` exists; there is no generic local-data folder,
  resolver, or gitignore handling, so this ticket creates that). Rationale:
  GitHub PR comments are permanent in GitHub's server DB, **not** in the git
  object store; tracking review notes in the repo pollutes history and creates
  conflicts. File-backed (not browser localStorage) so an agent / other tools can
  read them and they survive browser state.
- **Daemon owns the store.** Comments are host files, so the frontend cannot write
  them directly — add daemon read/write endpoints (extend the `/git/*` +
  `git_toolbar.rs` pattern) for the comment store.
- **Keying:** commit hash → file → line-range, with a separate **working-tree**
  bucket. Commit-hash-anchored comments are stable.
- **Anchoring is best-effort for working-tree targets; the clipboard copy
  specializes to compensate.** Rather than precise re-anchoring after edits, a
  working-tree comment's clipboard payload **embeds the original code segment**
  plus a **"position may differ"** note, so the payload stays correct regardless
  of line drift. (Display-side, a working-tree comment may still show stale after
  edits — accepted for v1.)
- **Copy comments to clipboard** action on the diff view:
  - For each comment, encode enough for an agent to extract the exact segment on
    its own: HEAD/working-tree target → file pointer + line range; historical
    target → commit hash + file pointer + line range (a raw
    `git show <hash>:<path>` command may be encoded when that is what an agent
    needs). Working-tree targets additionally embed the code + "position may
    differ" per above.
  - Wrap the whole payload in **`<code-comments>[content]</code-comments>`** XML
    and write to the clipboard (reuse the existing inline
    `navigator.clipboard.writeText` pattern; the codebase convention is no shared
    clipboard helper). The user pastes it into a prompt and asks for analysis.

## Constraints

- The diff view must expose a per-line / per-range seam for attaching comments
  (coordinate with `260725-feat-ws-dashboard-git-diff-view`).
- `.ws-dashboard/reviews/` must be created gitignored (add gitignore handling; it
  must not become a tracked artifact).
- Comment gutter/affordances built to `260725-feat-ws-dashboard-design-guide`.

## Prior Art

- `.ws-dashboard/worktrees` convention (`git_worktree.rs:796`; spec
  `ws-web-dashboard/index.md:623`) — the only existing `.ws-dashboard` usage; this
  extends the folder to a general local-data store.
- `navigator.clipboard.writeText` inline pattern (`documentViewer.tsx:450`,
  `agentChatBubbles.tsx:155`) — the copy mechanism to reuse.
- `git_toolbar.rs` shell-out + `/git/*` routes — pattern for the comment-store
  daemon endpoints.

## Phases

### Phase 1: Comment store + gutter + clipboard export

Create the gitignored `.ws-dashboard/reviews/` store with daemon read/write
endpoints (keyed commit hash → file → line-range + working-tree bucket). Add the
diff-view comment gutter for per-line/per-range comments. Add the "copy comments
to clipboard" action emitting `<code-comments>…</code-comments>` payloads with
per-comment segment coordinates, specializing working-tree targets to embed the
code + "position may differ" note.

Verification boundary: a comment on a commit line/range persists to
`.ws-dashboard/reviews/` (gitignored, survives reload); commit-hash comments
re-anchor correctly; "copy comments" produces a wrapped payload from which an
agent can locate each segment; working-tree comment payloads embed the code and
the drift note.

## Spec Impact

Target spec area: none in the workflow spec set — downstream ws-dashboard local
feature; the `.ws-dashboard` local-data-folder concept may warrant a short note
in the `ws-web-dashboard` spec/mental-model on landing, but it is not a
workflow-system contract.

Contract-first spec: no.
