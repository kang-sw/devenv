---
title: Windows code signing for ws-mcp release binaries (SignPath Foundation)
---

# Windows code signing for ws-mcp release binaries (SignPath Foundation)

## Background

On a company-managed Windows machine, AhnLab V3 Internet Security 9.0 blocks
the ws-mcp runtime on every release. The alert reads "detected the execution
of a file based on cloud reputation and behavior" for
`C:\Users\...\ws-mcp-0.48.11-163a2172ec96.exe` (the runtime the launcher
downloads and runs), and the alerts pile up, one per `ws-mcp serve` start
(seen as "1 / 7"). If the user accepts the default "block once", that
session's MCP server does not start. Downstream Windows users with
reputation-based AV will see the same thing.

Why it recurs: each release ships a new unsigned binary with a new hash, so
reputation starts from zero every time. Behavior heuristics add to this: the
Python launcher downloads an exe and runs it; the daemon copies itself into a
cache directory and starts detached (breakaway from job); it listens on
loopback and runs many child processes. A company-managed V3 may not let the
user add path exceptions, so a client-side workaround is not reliable.

## SignPath Foundation fit

Source: <https://signpath.org/terms>. The page is marked draft; it was read
on 2026-11-09.

Already met:
- OSI-approved license: the repository is public under Apache-2.0, with no
  proprietary components.
- The binaries are already released, are built from our own source, and are
  built by GitHub Actions (`.github/workflows/ws-mcp-release.yml`). SignPath
  calls these "verifiable builds".
- Every release must be approved by hand. The `lead-ship` publish
  confirmation already does this.

Work needed before applying:
- A "Code signing policy" section on the project home page (README). It names
  the team roles (Authors, Reviewers, Approvers) and gives privacy
  information.
- A Windows VERSIONINFO resource in the exe: product name = project name, and
  product version = the release version, enforced on every build. The Go build
  does not set these today; a tool such as goversioninfo/rsrc would add them.
- A SignPath signing step in `ws-mcp-release.yml`. It must sign the exe before
  assets are uploaded, while the windows-smoke job still runs the signed
  binary.
- MFA for every team member on both GitHub and SignPath.
- Possibly install/uninstall instructions and a notice of system changes:
  `install.sh` patches Claude settings, and the daemon stages copies under the
  cache root.

Risk: for executables, SignPath requires "a certain verifiable reputation", and
acceptance is at its discretion. The repository currently has 1 star, 0 forks
and a single maintainer, so rejection is plausible. Every prep item above also
applies to a paid certificate, so the work is not wasted if SignPath
declines.

One upside if accepted: the signer shown would be SignPath Foundation, whose
certificate already carries reputation from many OSS projects. That may help
with reputation-based AV more than a fresh personal certificate would. This is
unverified for V3 in particular.

## Outcome Ledger

### Verified Findings
- V3 blocks `ws-mcp-0.48.11-163a2172ec96.exe` by "cloud reputation and
  behavior", once per `ws-mcp serve` start, on a company-managed Windows
  machine. Evidence: the user's screenshot in the 2026-11-09 session.
- The repository is public and licensed Apache-2.0 (`gh repo view`).
- SignPath Foundation's conditions are as listed above (signpath.org/terms,
  draft, read 2026-11-09).

### Confirmed Decisions
- Pursue Windows code signing for the ws-mcp release binaries, trying the
  SignPath Foundation free OSS program first. The user confirmed this on
  2026-11-09, to start after the 0.48.12 hotfix ship.

### Proposals
- Order the work so it can be reused: the README code signing policy,
  VERSIONINFO embedding, and a signing step in the release workflow with an
  abstract signer. Then apply to SignPath.
- Fallback if SignPath declines: a paid certificate, either Azure Trusted
  Signing (low monthly cost, though identity-validation eligibility for a
  Korean individual is unverified) or an OV certificate held in a cloud HSM
  (DigiCert KeyLocker, SSL.com eSigner). Since 2023, private keys must live in
  a hardware or cloud HSM.

### Open Questions
- Does V3's ASD reputation actually trust a binary signed by SignPath
  Foundation, or by any OV certificate, enough to stop blocking it? This can
  only be checked empirically, after a signed release.
- Do the staged daemon copies keep the signature? They are byte copies, so
  they should, but this is not checked.
- Which binaries need signing: only the Windows runtime exe, or also the
  launcher-related artifacts?
- Is the plugin install path's mention of Python launcher downloads "data
  transfer to unspecified systems"? It downloads only from this repository's
  GitHub releases.
- Who fills the Reviewer and Approver roles in a single-maintainer project?

### Rejected Alternatives
- Relying on per-machine V3 path exceptions: a company-managed V3 may not
  allow them, and downstream users would each have to configure them.
