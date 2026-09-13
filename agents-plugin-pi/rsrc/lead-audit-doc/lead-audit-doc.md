---
kind: print
delegates: true
---

# Audit Doc

Draft or revise the requested project document under its existing instructions.
Preserve its meaning. Remove repeated exclusions that a positive owner or
default can state once, qualifications that change no reader action, scope, or
stop, and rationale written to defend the author rather than guide the reader.

The document keeps its own structure and style.

After writing, if the user did not already request an independent audit, ask
whether to spawn one. On acceptance, render `fresh-read-doc-auditor` through
`{{.McpNamespace}}/playbook.render` with the target path or excerpts and spawn
it with the returned bindings. Give it no conversation context.

Apply meaning-preserving findings. Return any finding that would change policy
or intent to the user.

Report the changed path and whether the independent audit ran.
