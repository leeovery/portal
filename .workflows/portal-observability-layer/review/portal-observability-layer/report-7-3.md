TASK: Emit the state-mutation op as the required op= attr rather than as the slog message (portal-observability-layer-7-3)

ACCEPTANCE CRITERIA:
- Every state-mutation INFO and its matching WARN failure line carries an op=<verb> attr.
- Rendered text includes an op= token; JSON handler emits an "op" field.
- op values confined to the closed value space (set/modify/rm/clean-stale/migrate/set-noop).
- No other attrs or message semantics regress.
- go build / go test ./internal/{hooks,alias,project}/... ./cmd/... pass.

STATUS: Complete

SPEC CONTEXT:
Spec § State-mutation audit trail (658-727): op is a required attr from a closed value space; op is in the closed 49-key vocabulary (190). Rationale: programmatic filtering by the op attr (JSON field + grep op=) fails if op lives only in the message. migrateConfigFile is the sanctioned non-store emitter (op=migrate via=migrate). Batch CleanStale: one INFO summary with op=<batch-op>, entries=N.

IMPLEMENTATION:
- Status: Implemented (uniform across all cited sites)
- Location: hooks/store.go:98,103,133,143,148,193,198,282; alias/store.go:175,187,192,215,220; project/store.go:143,148,220,258,262,303,308; storelog/clean_stale.go:52,59 (shared batch summary, op=clean-stale both branches); cmd/config.go:62,69,75 (op=migrate via=migrate).
- Notes: op verb carried BOTH as slog message (preserving <component>: <verb> catalog shape + grep "hooks:" idiom) AND as explicit op attr (documented per-store). All op values in closed space; no out-of-vocabulary verbs. value correctly absent on rm/clean-stale. Repo-wide scan for the anti-pattern (op verb as bare message with no op attr) found zero remaining sites.

TESTS:
- Status: Adequate
- Coverage: every mutation path asserts op via rec.AttrString(t,"op") on both INFO and WARN arms — hooks (set/modify/set-noop/rm/clean-stale, + TestSetEmitsOpAsJSONField parses real JSON handler asserting rec["op"]=="set"); aliases (set/modify/set-noop/rm INFO+WARN); projects (Upsert set/modify, Rename modify, rm, clean-stale per-entry+summary); migrate (INFO table per component, WARN Rename/MkdirAll with error_class, empty-component suppression, configFilePath threading); storelog (success-INFO + failure-WARN op=clean-stale).
- Notes: Both arms covered. Asserts structured attr (would fail if reverted to message-only). JSON-field test appears once (representative). Not over-tested.

CODE QUALITY:
- Project conventions: Followed (log.For per package; store-method seam; logtest.Sink + SetTestHandler; no t.Parallel).
- SOLID/DRY: Good — storelog.EmitCleanStaleSummary removes duplicated terminal batch emission (no drift); import-cycle rationale documented.
- Complexity: Low.
- Modern idioms: Yes (slog variadics, error attr, ClassifyWriteError mapping).
- Readability: Good — each store has a "Message-shape" doc explaining message-AND-attr duality + closed value space.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Closed op value space enforced only by convention + per-site literals (no shared typed const set); a future site could emit an out-of-vocabulary verb without a compile-time guard. A shared op const block (or a vocabulary-validation test) would make the invariant structurally enforced. Out of scope.
