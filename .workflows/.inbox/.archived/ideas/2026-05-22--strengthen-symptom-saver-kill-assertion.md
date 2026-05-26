# Strengthen `TestCommitNowSymptom` sub-test 2 (saver self-kill marker-clear)

Sub-test 2 of `TestCommitNowSymptom` (`cmd/state_commit_now_symptom_integration_test.go`) kills `_portal-saver` with `@portal-restoring` clear and asserts `sessions.json` contains `A`+`B` and omits `_portal-saver`. That shape is **identical to the pre-kill baseline** — the saver was never in `sessions.json` to begin with (filtered out by `keepSessionNames`'s underscore-prefix rule), and `A`+`B` were already present. Two consecutive consistent reads after the kill can pass even if `commit-now` never executed at all.

The underscore-prefix filter is well-covered by unit tests on `keepSessionNames`, so the integration coverage gap is small — but the assertion in this sub-test is materially weaker than its siblings. Options to tighten:

1. **`sessions.json` mtime delta** — record mtime before the kill, assert it advances within the budget. Cheapest mechanical change.
2. **`portal.log` scan** — grep for a `commit-now` log entry matching the saver session name. More involved but pins the actual code path.
3. **`save.requested` mtime assertion** — per spec § `save.requested` Discipline, a successful sync commit does **not** touch `save.requested`. Asserting `save.requested` is absent (or its mtime unchanged) confirms commit-now ran and took the success path.

Option 1 is the lightest. Risk of flakiness on systems with low mtime resolution can be addressed with a 2s tolerance window (consistent with the existing `TestTouchSaveRequested` mtime bracket).

Source: review of killed-session-resurrects-within-tick-window/killed-session-resurrects-within-tick-window
