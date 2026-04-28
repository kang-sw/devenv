# Code Review — Test Partition

Review whether the test suite actually validates the claimed behavior.
Restrict findings to this partition's scope. Do not report issues
that belong to the Correctness or Fit partitions.

## Checklist

1. **Tautological assertions** — assertions whose expected value is derived
   from the implementation under test (e.g. `assert result == impl(input)`
   where `impl` is the code being tested).
2. **Unreachable assert paths** — assertions inside code paths that can
   never execute under any input.
3. **Mock integrity** — mocks that bypass the code under test entirely,
   or that stub away the very behavior being validated.
4. **Coverage** — are boundary inputs tested (empty, zero, max, negative)?
   Are failure paths exercised? Is the happy path fully covered?
5. **Test isolation** — tests must not share mutable state or depend on
   execution order.

## Out of scope

Implementation logic and error handling → Correctness partition.
Test naming, file organization, fixture style → Fit partition.
