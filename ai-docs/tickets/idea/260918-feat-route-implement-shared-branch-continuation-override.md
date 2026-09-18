---
title: route.resolve_implement has no override for sanctioned shared-branch continuation
related:
  260918-feat-claude-tickets-workflow-retarget-bootstrap: surfaced-by
---

# route.resolve_implement has no override for sanctioned shared-branch continuation

## Background

Surfaced during dogfooding while running
`260918-feat-claude-tickets-workflow-retarget-bootstrap`, a follow-up ticket
whose design (reviewed and approved) deliberately continued the predecessor
ticket's retained impl branch `impl/develop/flock-fade-snide` so the two
sequential tickets' work would merge to `develop` as one set.

`ws/route.resolve_implement`'s branch-continuation guard stopped with
"target scope differs from suspected prior work ... not overridable by
allow_rename". The worker found no input-level override for the legitimate
"continue a shared branch across two sequential tickets, merge once" pattern:
`target.scope_slug` is ignored for ticket targets (confirmed by re-invoking
with it set), and `allow_rename` does not cover this case.

The worker proceeded only after verifying via
`git log develop..impl/develop/flock-fade-snide` that the branch held solely the
predecessor's (now `.done/`) commits plus this ticket's own ready commit — no
unrelated work mixed in — and on the ticket's own design-reviewed authorization.
It judged this safe and below a stop, but flagged that the router itself has no
sanctioned way to express the pattern.

## Problem

The guard protects against accidental branch mixing, which is correct default
behavior. But there is no way for a caller with a legitimate, reviewed plan to
continue a shared branch to signal that intent to the router, so the only path
today is to override by human/worker judgment outside the tool. That pushes a
safety decision out of the tool and into ad-hoc reasoning.

## Open Questions

- Is the right fix an explicit, auditable override input (e.g. an
  acknowledged-continuation flag bound to the inspected branch head, mirroring
  how `git.merge` binds a release override to source/target OIDs), or a
  first-class notion of a multi-ticket branch that the router recognizes?
- How often does this pattern actually arise? If rare, a documented manual
  path may be enough; if it recurs (sequential tickets that must land
  together), it argues for a real input.
- Should the guard instead key on the git-verified commit set (only prior
  `.done/` tickets' commits present) rather than on scope-slug divergence?

## Notes

This is an idea-stage capture from a single dogfood instance; it is not a
settled decision to change the router. Triage before promoting.
