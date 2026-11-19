# Plan: daemon system mails show "mail from system" instead of "mail from unknown"

## Request (owner, 2026-11-19 11:11)

The hourly "Whiteboard lead nudge (hourly)." mail reaches a lead as
`mail from unknown (<time>):`, which looks suspicious. Make daemon-originated
mails read as coming from the system, e.g. `mail from system (...)`.

## Evidence (develop)

- Nudge sent in `agents-plugin-tool/internal/mcp/whiteboard_nudges.go:428`:
  `wsmailbox.PostSystem(path, ReplyID(t.key), "", message, now)` (empty label).
- `wsmailbox/replyid.go:150-164` PostSystem writes an Envelope with no From,
  no ReplyTo, and Label = the label argument.
- `mcp/lead_mail.go` formatMailEnvelope (~272-283): handle = first non-empty
  of From, ReplyTo, Label, else literal "unknown"; parse-back ~244-263 also
  special-cases "unknown".
- Other system mails with an empty label: `session_hire.go:412`;
  undelivered-mail notices `wsmailbox/undelivered.go:142` via postSystem.
- Dashboard matchers keyed on the "mail from unknown" prefix:
  `wsdashboard/conversation_mail_wake.go:196`, tests
  `conversation_board_nudge_test.go:14,119,120,134,190`.

## Outcome

- Add one exported constant in wsmailbox, `SystemLabel = "system"`, and pass
  it as the label for every daemon-originated system mail (nudge, unblocked
  notice if it goes through the same path, session hire, undelivered
  notices). Do NOT set From or ReplyTo (they are reply addresses).
- The formatter needs no change for display; update the parse-back and the
  dashboard nudge/wake matchers to recognise both `system` (new) and
  `unknown` (old mails already stored / older daemons) so nudge cards keep
  being recognised.
- Agent turn reports (`pi:<id>` labels) are unchanged.
- Compatibility: T0 for the mailbox record (Label already exists, omitempty,
  no shape change); an older dashboard that only matches "unknown" would show
  a nudge as plain mail (T1 display-only). Record this in the commit AI
  Context.

## Verification

- Update/extend the existing Go tests (lead_mail, whiteboard nudges,
  conversation_board_nudge_test, undelivered) to assert `mail from system`
  and that old `unknown` mails are still recognised.
- Run on w5090 only, redirecting output to a file under
  D:/Temp/mail-system-sender/ and reading it:
  `bash D:/sys_lock.sh remote w5090 run <who> 30 --sync '<this worktree path>' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsmailbox/... ./internal/mcp/... ./internal/wsdashboard/...'`
  (wsdashboard tests that need Node may be skipped with -skip and noted).

## Constraints

- NO local go test / Go tools / gofmt / ws exes / launcher / Electron on this
  PC. Go only on w5090.
- Do NOT start, stop, rebuild or swap the shared ws daemon.
- Temp only under D:/Temp/mail-system-sender/. Never read or print
  credentials.
- Commit on impl/develop/mail-system-sender in this worktree only, no
  Co-Authored-By trailer (Pi worker).

## Return boundary

Stop and report before changing anything beyond the system-mail label, its
constant, the two matchers/parse-back, and their tests.
