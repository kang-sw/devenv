---
title: rsrc Load reports an unknown playbook name as "manifest-listed file missing"
related:
  260609-refactor-ws-spawn-runtime-deletion-session-auth: surfaced while dogfooding the unmerged 2c build via playbook.print on the Claude plugin install
  260611-bug-rsrc-manifest-regen-missed-after-shipped-edit: sibling rsrc-tree/manifest integrity concern (different failure)
---

# rsrc Load reports an unknown playbook name as "manifest-listed file missing"

## Background

While dogfooding the ws plugin install, `playbook.print(name: "implementer")`
was called with a name that is NOT an rsrc playbook (implementer is an
agent-prompt-bundle stem, not a `<root>/<name>/<name>.md` playbook). The tool
returned:

```
rsrc manifest-listed file missing: "implementer/implementer.md"
```

## What is misleading

The `ErrFileMissing` message text — "rsrc manifest-listed file missing" — names
the *manifest-present-but-disk-absent* condition. But `loadAndVerify`
(`agents-plugin-tool/internal/wsrsrc/loader.go`) returns the SAME typed error
for the opposite condition: the derived path `<name>/<name>.md` is simply **not
listed in the manifest at all** (i.e. the playbook name does not exist). The
loader doc comment confirms the conflation: "Returns ErrHashMismatch on
mismatch, ErrFileMissing if the file is not listed in the manifest."

So a caller who mistypes or guesses a playbook name gets an error that reads as
a corrupted/torn install ("a manifest-listed file went missing") rather than the
real cause ("no such playbook").

## Why it matters

Wrong-name is a routine caller mistake (the bundle/playbook stem namespaces
overlap by intent — `implementer`, `code-reviewer` exist as prompt bundles but
not as playbooks). The current message sends the caller toward
install-integrity forensics instead of "you asked for a playbook that doesn't
exist." It also masks genuine torn-install cases, since both now read the same.

## Possible follow-ups

- Split the two conditions in `loadAndVerify`: if the resolved `<name>/<name>.md`
  is absent from BOTH the manifest and disk, return an `ErrPlaybookNotFound`
  (or similar) with a "no such playbook %q" message and, ideally, the list of
  available playbook stems. Reserve `ErrFileMissing` for the true
  manifest-lists-it-but-disk-lacks-it integrity failure.
- Consider validating the playbook directory exists before the manifest lookup
  so the not-found path is unambiguous.

## Notes

- Distinct from `260611-bug-rsrc-manifest-regen-missed-after-shipped-edit`
  (that one is about manifest regeneration drift after a shipped edit). This
  ticket is purely the unknown-name vs. torn-install message conflation.
