# Close the two structural-only verification gaps with end-to-end tests

Two acceptance criteria in the hydrate/restore observability work are currently guaranteed *structurally* (by upstream guards / call-site placement) rather than asserted by a dedicated end-to-end test. Adding these is low-risk (test-only, additive) and locks the contracts against a future refactor that moves the guard:

1. **Mid-stream io.Copy routing (task 6-3):** the generic-I/O and mid-stream-copy `scrollback missing` tests invoke `handleHydrateFileMissing` directly; the routing of an `io.Copy` mid-stream failure *into* the handler is verified only by reading `runHydrate`. A single end-to-end test (e.g. scrollback removed/truncated under a failing reader) would assert the `io.Copy` call site reaches the handler.

2. **Destructive-contention regression (task 11-2):** the scenario `restoretest.OpenTestLogger` fundamentally guards against (a real binary's sink unlinking the test writer's `portal.log` when sharing a stateDir) is only *structurally* prevented by the symlink-shape fix, not asserted. A test that opens `OpenTestLogger` against a stateDir, triggers a production `reopen`/`migrationGuard` against the same dir, and asserts both writers' records survive on the day file would lock the property.

Note (from review): the *consolidation* sub-items from the same finding (folding near-duplicate test pairs) were deliberately NOT recommended — removing tests during/after a review trades coverage for tidiness. Only the two additive tests above are worth doing.

Source: review of portal-observability-layer/portal-observability-layer
