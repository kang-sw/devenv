---
title: Native fork session keys lose their parent identity
related:
  261114-research-ws-sub-lead-session-role: "owner 705=b selects role design instead of narrow 676 repair; no implementation authorized"
---

# Native fork session keys lose their parent identity

## Background

MAIN observed unexplained dashboard rows and independently traced them to native fork identities. This capture records missing lineage; it does not authorize root-key mint restrictions, session deletion or a common-interface architecture.

## Evidence

Read-only inspection on 2026-11-14 independently found these exact session `key.json` files contain only schema_version 2, root `/Users/kang-sw/devenv`, and scope `lead`, with no parent field:

- `geology-swimmer-finalist`
- `onion-unsaid-urologist`
- `imagines-antiquity-cannon`

MAIN's native session `rebelrepugnantstonewareemcee/pi/session.jsonl:839`, 05:39:16.938Z, preserves the earlier approved deterministic SHA256/BigEndian.Uint64/mod7772 pool calculation plus hyphen removal:

| session key | dashboard reply/row ID | recorded attribution |
|---|---|---|
| geology-swimmer-finalist | frailabdomenenjoyercurly | MAIN density fork6930bf3a |
| onion-unsaid-urologist | epiccrucialasideapproach | MAIN design forka2da1c43, prior resume |
| imagines-antiquity-cannon | draperyhypnosesphosphategrimy | native owner01a1d79b, fork3216d18f; owner==EXEC not independently established |

The mapping is MAIN's recorded deterministic result, not a rerun by this capture. Current dashboard agent.json files for these three row IDs were absent. The displayed row strings are mailbox reply IDs, not ws session keys.

Current source boundary:

- `agents-plugin-pi/src/fork.ts:215-237` supplies legacy `parentSessionKey` but no `parentPolicy` in buildForkSpawnCtx.
- `spawner.ts:3679-3694,3718` defines caller policy without a key, a separate keyed leadParentPolicy helper, and the fallback to caller policy when parentPolicy is absent.
- `delegation-policy.ts:108` carries lineage from parent.sessionKey.
- `bridge.ts:1233-1239` forwards policy.parentSessionKey during bootstrap when available.

This plausibly explains missing lineage in current source. Loaded/source equivalence and each historical bootstrap invocation remain unverified; no root-cause certainty is claimed.

Full local evidence artifact: `75748f2c-01-live-lead-and-fork-parent-incident-capture.md` in generated review paths, shared with root lead and MAIN. This issue is separate from `261114-bug-ws-live-lead-state-prompt-compaction`.

## Design Disposition

Owner (705)=b, relayed by MAIN on 2026-11-14 at 12:02, rejects the narrow
(676) fix as the selected path and requests a sub-lead role design inside
`261114-epic-ws-common-session-foundation`. The design is captured in
`261114-research-ws-sub-lead-session-role`: retain parent-lead lineage for
display/ownership while supplying lead-equivalent authoring without top-level
lead identity. This is not implementation or T2 approval.

The earlier simple-wiring explanation is incomplete: `5fa2e7cb8` and
`260926-bug-pi-execute-worker-steals-mailbox-owner` explicitly exempted native
and discussion forks as lateral leads. The design must also preserve the
mailbox ownership protections in that ticket and
`260926-bug-pi-children-inherit-mailbox-identity`; merely adding a parent can
change tool gates and board/tracker visibility. Historical evidence above is
retained, not promoted into a claim that loaded runtime behavior is repaired.

## Constraints

Preserve existing sessions and keys. No retroactive rewrite, deletion, restart, root-mint policy, protocol/API restriction or T2 compatibility change is authorized. Those policy choices remain MAIN-owned. This factual idea capture does not settle them.

## Phases

### Phase 1: Establish native fork lineage handoff

Verify loaded-code identity and actual fork admission/bootstrap lineage
against the preserved key/row evidence, and hand findings to
`261114-research-ws-sub-lead-session-role` under owner (705)=b. Do not execute
the rejected narrow (676) fix. Return any product/API or compatibility choice
through MAIN before promotion or implementation; no new dependency on the
existing common-interface implementation is introduced.
