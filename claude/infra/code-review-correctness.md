# Code Review — Correctness Partition

Review whether the implementation does what it is supposed to do.
Restrict findings to this partition's scope. Do not report issues
that belong to the Fit or Test partitions.

## Checklist

1. **Logic errors** — off-by-one, incorrect conditionals,
   null/undefined dereference, integer overflow, wrong operator precedence.
2. **Error paths** — are all failure modes handled? Are errors propagated
   or swallowed silently? Are resources released on error paths?
3. **Contract compliance** — do the changed functions satisfy invariants
   in mental-model docs? Are coupling rules respected?
4. **Security surface** — injection (SQL, shell, path), XSS, authentication
   bypass, insecure deserialization, exposed secrets — OWASP top 10.
5. **Edge cases** — boundary inputs (empty, zero, max), concurrent access,
   unexpected input shapes the implementation does not handle.
6. **Spec drift** — if a spec entry claims behavior absent from both the diff
   and the existing codebase, report it as a potentially stale spec entry.

## Out of scope

Conventions, naming, reuse, patterns → Fit partition.
Assertion validity, test coverage, mock integrity → Test partition.
