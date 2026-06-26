---
kind: print
delegates: true
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
