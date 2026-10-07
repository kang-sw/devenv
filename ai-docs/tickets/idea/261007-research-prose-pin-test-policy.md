---
title: Limit prose pin tests to contracts they actually protect
---

# Limit prose pin tests to contracts they actually protect

## Background

Shipped playbooks are guarded by substring pins: tests render a playbook and
assert that exact sentences appear (`agents-plugin-tool/internal/mcp/playbook_tools_test.go`
alone holds 141 `strings.Contains` checks, plus the wsflow bundle test). In
the review sweep of `bf3216981..c43ebc3cd`, the range rule "every behavior
change carries a test" made the test reviewer flag an unpinned `lead-run`
sentence as Important; the user judged most such pins ceremony
(2026-10-07): they confirm the edit was made, not that an agent behaves
accordingly, and every rewording forces a mechanical pin update.

Pins that do protect something:

- **Render skeleton**: section headings and substitution output, which check
  templating, product-namespace substitution, and harness branches.
- **Cross-surface contracts**: one rule stated in more than one place that
  must agree, for example the ODQ settle point in `lead-ticket.md`, the
  `tickets.move` refusal in `internal/wsdoc/tickets_mutate.go`, and the
  `sage_gate` next_instruction in `internal/mcp/server.go`.
- **Forbidden retired phrases**: a removed rule with a known regression risk.

## Question

Should pins be limited to those three classes, with existing sentence pins
outside them pruned? Where should the rule live (`ai-docs/manuals/skill-authoring.md`
and the code-review-test partition's reading of "a test alongside every
behavior change" for prose surfaces), and does that reading change shipped
reviewer text (an observable workflow behavior change that needs approval)?
