---
name: lead-verify-discussion
description: Run a compact discussion verification checkpoint with premise, evidence, and over-alignment checks.
---

# Verify Discussion

Topic: user request

## Checks

- Treat user preference as input, not evidence.
- Verify assumptions against project evidence before endorsing the direction.
- Look for over-alignment: unsupported agreement, skipped trade-offs, ignored
  constraints, or confirmation-only search.
- Seek disconfirming evidence and cheaper or reusable alternatives.
- Check maintainability hygiene for proposed structure or implementation shape.
- Build the strongest concise countercase; do not force false balance against strong evidence.
- Keep the checkpoint lightweight and do not edit files.

## Process

1. Re-objectify the discussion as claims, assumptions, and desired outcome.
2. Check the highest-risk assumptions against project evidence and existing mechanisms.
3. Spawn one or more host-native exploration workers directly with scoped task prompts when investigation is useful; collect results before synthesizing.
4. Name any over-alignment risk in the current direction.
5. Test the best countercase against the evidence.
6. Recommend keep, revise, reject, or ask the user to choose.

## Output

Return corrected premises, concrete evidence, reuse opportunities, constraints,
hygiene findings, countercase, and the best-supported recommendation.

---
**Continuity tip:** This playbook delegates to a subagent. When the subagent returns an agent id, continue by resuming the agent using its returned id to send follow-up messages to the same agent rather than spawning a new one. The playbook surface keeps no agent registry; record the agent id in your workflow state if you need it across turns.

**Mercenary path (always available):** A ws-managed external subprocess agent (mercenary) is always reachable on request via `ws.mercenary.call`, even without `config.workflow_prefer_mercenary`. Pass the session_key received with this prompt and a self-contained prompt from `ws/playbook.render`; the returned handle is an agent id you can resume with the same continuation idiom.
