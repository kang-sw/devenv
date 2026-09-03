---
kind: render
delegates: true
variables:
  - ExploreAgent
  - SpawnIdiom
  - ContinueIdiom
---
# Delegate Sample Playbook

This fixture exercises harness-aware terminology substitution and the delegation tip.

Use {{.ExploreAgent}} for discovery tasks.

To delegate work, use: {{.SpawnIdiom}}.

To continue with the same agent after it returns, use: {{.ContinueIdiom}}.
