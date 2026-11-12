---
title: "Tracker body merge: two minor gaps from the assignee-edit review"
related:
  - 261111-feat-ws-tracker-assignee-edit-and-body-merge
---

# Tracker body merge: two minor gaps from the assignee-edit review

Lite review of 261111-feat-ws-tracker-assignee-edit-and-body-merge (merged
c0e300deb) left three Minor findings open; two remain. Each needs the change
in both the Go domain (`internal/wstracker`) and the dashboard engine
(`engine.js`), with goldens.

1. **Playbook wording overstates conflict marking.** A change made by a write
   that is in the editor's causal past but outside its `base` closure resolves
   to the later write with no mark. The tracker playbook says "both are marked
   as a conflict", which is not true for this case. Fix either the wording or
   the behavior.
2. **Unbounded diff memory.** `diffLines` (Myers) keeps an O(D^2) trace and
   bodies have no size cap, so two very large concurrent rewrites cost a lot of
   memory on every replay. Options: a body size cap, or a linear-space diff.

The third gap (attachments dropped on a text-neutral based edit) is fixed:
attachments now merge three-way by oid from the base value instead of as a
union over the writes whose lines survive (the review-sweep fix for a merging
edit that could not remove an attachment). Refs and mentions still follow the
surviving lines.

Also unverified at merge: the two-window same-base merge driven through the
dashboard (domain tests cover it).
