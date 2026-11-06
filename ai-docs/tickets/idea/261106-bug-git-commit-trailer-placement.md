---
title: git.commit cannot place a commit trailer such as Co-Authored-By
---

# git.commit cannot place a commit trailer such as Co-Authored-By

## Background

Dogfood surprise (2026-11-06, devenv lead session): delegates committing
through `ws/git.commit` could not end the message with a `Co-Authored-By:`
trailer, which the host's attribution guidance requires.

`wsgit.CommitMessage` (`agents-plugin-tool/internal/wsgit/git.go`) writes the
title, the description, `## AI Context`, then the optional `## Ticket
Updates`, follow-ups, and resolves sections. It has no trailer input. A
caller's only places for the line are the description, which lands before
`## AI Context` (commit 8f2d513d1), or an `ai_context` bullet. Neither is the
final paragraph, so git does not parse it as a trailer. One delegate
amended with native git to move the line (45f722292); others left commits
without a trailer (b1771b9df, 8a926fd83).

Open for design: a `trailers` argument that `CommitMessage` writes as the last
paragraph (and the same for `git.merge`), or a configured attribution line
the tool appends.
