# Delegate plan: dashboard session-name copy text becomes a `<mailbox>` tag

## Outcome

Clicking the session name in the dashboard session head (`.snm[data-copy]`)
copies a populated tag instead of `(mailbox, label "...") id:<rid>`:

```text
<mailbox id="id:<rid>" dir="<session cwd>" root="<git toplevel>" branch="<branch>" harness="<harness>">label</mailbox>
<mailbox id="id:<rid>" dir="<session cwd>" branch="<branch>" harness="<harness>"/>     (no label)
```

User request (dashboard UI feedback, 2026-11-13), verbatim shape:
`<mailbox id="id:abcdefg" dir="cur-dir" [root="git-root-dir"] branch="..." harness="...">[label]</mailbox>`,
single self-closing tag when no label is set.

## Settled decisions (lead)

1. Attribute order: `id`, `dir`, `root`, `branch`, `harness`.
2. `dir` = the session's working directory as the registry stores it
   (use a cwd field if the registration has one, else `Registration.Project`,
   the session root path). Paths are emitted as stored, no slash rewriting.
3. `root` = the git toplevel of the session's directory; emitted only when it
   differs from `dir` (compare cleaned paths, case-insensitively on Windows).
   The existing git badge (`gitbadge.go`, `d.git.badge(view.Project)`) is the
   expected source; do not add a new git probe if the badge already has it.
4. `branch` = current branch; the attribute is omitted when detached or
   unknown. `harness` (claude/codex/pi) is omitted when the registration has
   none. `id` and `dir` are always present.
5. Escaping: attribute values escape `&`, `<`, `>`, `"`; the label body
   escapes `&`, `<`, `>`. The old `labelQuoter` goes away with the old format.
6. One owner of the format: the Go side (`pageview.go` `mailboxRef` /
   `SessionView.MailboxRef`, rendered into `dashboard.tmpl` `sname`
   `data-copy`). The JS rename path (`dashboard.js` `mailboxRef` /
   `labelSave`, ~2296-2349) must not re-implement the whole format. Pick the
   simpler of: (a) the label route's response carries the refreshed copy text
   and JS sets it; (b) the server renders the open-tag attribute string in a
   data attribute and JS only appends the escaped label and closing tag (or
   self-closes when the label is reset). Record the choice in `## AI Context`.
7. Freshness: the copy text is computed at render time; if the head already
   re-renders over SSE it follows along. Add no new live-update machinery.

## Files (expected)

- `agents-plugin-tool/internal/wsdashboard/pageview.go` (SessionView fields,
  `mailboxRef`, `buildSessionView`)
- `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl` (`sname`)
- `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js` (rename path)
- possibly `label_route.go` (only for option 6a)
- `agents-plugin-tool/internal/wsdashboard/label_route_test.go` (asserts at
  ~49, 76, 238/258/281/319, 331-334, 360, 408-418)

## Out of scope

- `wsmailbox.ParseAddress` and the `lead-use-mailbox` playbooks: nothing
  parses the copied string today; leave them unchanged.
- Any other dashboard asset change. The main track is merging
  `search-palette` and `lead-wake-events` into develop (both touch
  `dashboard.js`); keep the diff minimal to ease that merge.

## Verification

- Update the existing assertions; add cases: label + differing root, no
  label (self-closing), detached (no `branch`), no harness, escaping of `"`,
  `&`, `<` in label and path.
- `go test ./internal/wsdashboard/` must pass. Build/test on remote w5090 per
  the clone note (Linux, Go at ~/sdk/go/bin, no Node). Tests that need Node
  (the JS harness, e.g. `TestHeadRenameScript`) skip or fail there: run those
  locally under `bash D:/sys_lock.sh run <who> <min> -- <cmd>`.
- One passing full check of that package closes the work.

## Return boundary

Commit on `impl/develop/mailbox-copy-tag` in this worktree (plan file rides
the commit). Do not merge. Stop and report if the change needs anything
beyond the files above (e.g. a registry/store shape change).
