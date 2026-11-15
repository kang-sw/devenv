# Re-grade compatibility guard tickets

## Authorized outcome

MAIN requested one low-priority docs head, after Search Phase 2 re-head, re-grading existing compatibility guard ticket labels under the landed directional T0–T4 policy. Search re-head d26e78fcff193f4bdd45a658c4f7ac3f5bee0666 has been handed off. The governing policy is AGENTS.md and ai-docs/manuals/release-compatibility.md, landed as c0ec803cb.

## Bounded scope

Start with ai-docs/tickets/todo/261112-feat-ws-mixed-version-compat-guards.md and its directly relevant active compatibility-guard neighbors. Known discovery hits include idea/261021-bug-launcher-local-devenv-minor-bump-breaks-installed-plugin.md and idea/261113-bug-ws-installed-plugin-locked-out-by-newer-minor-dev-daemon.md. Resolve any other directly related ticket through tickets.query before reading. Re-grade stale tier/approval wording, distinguish implementation targets from verified behavior, and cite writer/reader/reopen source evidence. Do not sweep unrelated feature tickets or edit the live-summary 675 ticket, owned by its current worker. Never edit terminal tickets.

## Constraints and verification

Read ticket conventions and the landed compatibility manual first. Preserve confirmed feature scope, frozen Results/plans, statuses, and recorded historical decisions; append an Edition/addendum where required. New policy governs current labels but does not authorize architecture, runtime or new feature changes. Report a semantic conflict rather than widening scope. Use ticket guardrails and diff checks only, with bounded read-only source inspection as evidence. No full suite, independent audit, real agents, installation, network/provider activity, shared daemon, merge or push. One docs commit with AI Context and this plan; lead reviews and hands the head to MAIN.
