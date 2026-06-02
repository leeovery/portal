---
topic: portal-observability-layer
cycle: 4
total_proposed: 1
---
# Analysis Tasks: Portal Observability Layer (Cycle 4)

## Task 1: Extend internal/logtest.Sink to expose attr values and collapse the 11 structured-attr-map capture handlers onto it
status: approved
severity: medium
sources: duplication

**Problem**: The structured-attr-map flavor of the in-process capturing `slog.Handler` — a record of `{level slog.Level; msg string; attrs map[string]slog.Value}` plus a byte-for-byte-identical ~70-line handler core — is independently re-authored across eleven test files under at least eight type names (`captureSink` ×4 in alias/hooks/project/storelog, plus `cleanSummarySink`, `captureSummarySink`, `migrateCaptureSink`, `durationCaptureSink`, `fifoSummarySink`, `saverEventSink`). The duplicated core is the struct `{ mu; records; shared; bound []slog.Attr }`, `owner()`, `Enabled` (unconditional true), `WithAttrs` (append bound+attrs into a fresh derived sink sharing the owner), `WithGroup` (passthrough preserving bound + owner), and a `Handle` that builds `attrs := make(map[string]slog.Value, len(bound)+r.NumAttrs())`, copies bound attrs, ranges `r.Attrs` into the map, then appends `{level, msg, attrs}` under the owner's mutex. `all()` (mutex-guarded snapshot copy) recurs in nearly all eleven; `installCapture(t)` / `onlyRecord(t)` / `attrString(t, key)` recur verbatim across the four store-package copies (which carry comments openly admitting the copy: "It mirrors the hooks store test sink", "mirrors the hooks/project store test sinks"). This is well past Rule-of-Three (eleven copies). The c3 task 9-1 extraction already consolidated the sibling *text-rendering/keys* flavor into `internal/logtest.Sink`, but these eleven could not adopt it because `logtest.Sink` exposes ordered attr *keys* (`Record{Level, Msg, Keys []string}`) and not the attr-*value* map these consumers assert on (level/int/duration/string value checks). The load-bearing risk is the same one c3 named, at higher cardinality: the WithAttrs/Handle attr-binding contract that every consumer's assertions key on is hand-maintained in eleven places.

**Solution**: Extend the existing `internal/logtest.Sink` (do NOT create a parallel helper package) to also expose the attr *values*, then route all eleven callers through it, leaving each test's genuinely-divergent convenience accessor as a thin wrapper. Concretely: add an `Attrs map[string]slog.Value` field to `logtest.Record`, populated inside `Sink.Handle` in the same loop that already iterates `bound + r.Attrs` to build `Keys` (zero extra traversal — capture `a.Value` into the map alongside appending `a.Key`). Add the small value-typed accessors that recur across the copies as methods on `logtest.Record` / helpers in `logtest` (e.g. `AttrString`, `IntAttr`, `DurationAttr`, `Only`/`OnlyRecord`, `WithMessage`). Each of the eleven files then deletes its ~70-line handler core (struct, record type, `owner`, `Enabled`, `WithAttrs`, `WithGroup`, `Handle`, `all`) and keeps only its genuinely-divergent convenience accessor as a thin free function or wrapper over `logtest.Sink.Records()` — mirroring how `internal/restore/logging_capture_test.go` already embeds `*logtest.Sink` and layers `recordsWithMessage` on top. The four store packages additionally collapse their identical `installCapture` / `onlyRecord` / `attrString` to `logtest` helpers. This is pure test-scaffolding consolidation of an existing identical contract: no production change and no behavior change.

**Outcome**: The structured-attr-map capture-handler core exists in exactly one place — `internal/logtest.Sink` — so the WithAttrs/Handle attr-binding contract changes in one location instead of eleven. The eight redundant sink type declarations and their byte-identical ~70-line cores are gone; each of the eleven consuming test files retains only its genuinely-divergent convenience accessor as a thin wrapper over `logtest.Sink.Records()`. `logtest.Record` exposes both `Keys` (existing) and `Attrs` (new) so both the keys-asserting (c3) and the value-asserting (these eleven) consumers share one declaration. All existing tests still pass with identical assertions.

**Do**:
1. In `internal/logtest/capture.go`, add an `Attrs map[string]slog.Value` field to the `Record` struct (keep the existing `Level`, `Msg`, `Keys` fields — do not break the c3 keys-asserting consumers).
2. In `Sink.Handle`, allocate `attrs := make(map[string]slog.Value, len(s.bound)+r.NumAttrs())` and, in the existing single loop that already appends to `keys` for both `s.bound` and the `r.Attrs` range, also set `attrs[a.Key] = a.Value`. Set `Attrs: attrs` on the `Record` appended to `owner.records`. (Last-write-wins on duplicate keys matches the existing copies, which range bound-then-call.) Update the package/struct doc comment to note Sink now retains the attr value map in addition to ordered keys.
3. Add the recurring value-typed accessors to `logtest` as methods on `Record` and/or package helpers, covering the union of what the eleven copies use: `AttrString(key string) (string, bool)` or a `t`-failing `AttrString(t, key)` variant matching the store copies' `attrString`, `IntAttr`, `DurationAttr`, an `OnlyRecord(t)` (the single-record-or-fail helper), and a `WithMessage(msg)` / records-filter-by-message helper (mirroring restore's `recordsWithMessage`). Match the existing `*testing.T`-first signatures used by the store copies so callers translate directly.
4. Route each of the eleven files onto `logtest.Sink`, deleting the local handler core in each:
   - `internal/alias/store_logging_test.go`
   - `internal/hooks/store_test.go`
   - `internal/project/store_logging_test.go`
   - `internal/storelog/clean_stale_test.go`
   - `cmd/bootstrap/clean_sweep_summary_test.go`
   - `cmd/state_daemon_cycle_summary_test.go`
   - `cmd/config_migrate_logging_test.go`
   - `cmd/state_hydrate_timeout_log_test.go`
   - `cmd/state_hydrate_replayed_log_test.go`
   - `internal/state/fifo_sweep_summary_test.go`
   - `internal/tmux/portal_saver_lifecycle_events_test.go`
   For each: delete the duplicated struct, record type, `owner`, `Enabled`, `WithAttrs`, `WithGroup`, `Handle`, and `all`; replace the local `installCapture`/sink-construction with `log.SetTestHandler(t, &logtest.Sink{})` (or a shared `logtest` install helper if one already serves this), and rewrite each genuinely-divergent accessor (`summaries()`/`onlySummary()`, `signalTimeoutRecord()`, `intAttr()`/value-typed getters, `saverEvent...`, etc.) as a thin wrapper over `logtest.Sink.Records()` / the new `logtest.Record` accessors. Add the `internal/logtest` import where missing and drop now-unused imports (`sync`, `context`, possibly `log/slog`) flagged by the compiler.
5. For the four store packages (alias/hooks/project/storelog), additionally collapse their verbatim `installCapture` / `onlyRecord` / `attrString` onto the new `logtest` helpers rather than re-declaring them locally; remove the "mirrors the hooks store test sink" copy-acknowledgment comments.
6. Scope the change strictly to the shared core: leave each per-test convenience accessor in its owning file as a thin wrapper. Do not touch any production (non-`_test.go`) file. Do not import `internal/logtest` from any non-test file.

**Acceptance Criteria**:
- `logtest.Record` carries a new `Attrs map[string]slog.Value` field populated in `Sink.Handle` within the existing bound+call attr loop (no second traversal of attrs), and the existing `Keys` field is unchanged so c3 keys-asserting consumers still compile and pass.
- All eleven listed test files no longer declare a local structured-attr-map capturing `slog.Handler` (struct + record type + `owner`/`Enabled`/`WithAttrs`/`WithGroup`/`Handle`/`all`); each routes through `logtest.Sink`, retaining only its genuinely-divergent convenience accessor as a thin wrapper.
- The four store packages no longer carry their own `installCapture`/`onlyRecord`/`attrString` copies; they use shared `logtest` helpers, and the "mirrors the hooks/project store test sinks" copy-acknowledgment comments are removed.
- No production (non-`_test.go`) file is modified; `internal/logtest` remains imported only from `_test.go` files.
- `go build ./...` succeeds and `go test ./...` passes with the same assertions the eleven files made before the change (no test was weakened or deleted to accommodate the move).

**Tests**:
- Run the existing suites for each consuming package after the rewrite — they are the regression net for this scaffolding move (`go test ./internal/alias/... ./internal/hooks/... ./internal/project/... ./internal/storelog/... ./internal/state/... ./internal/tmux/... ./cmd/... ./cmd/bootstrap/...`); every previously-passing assertion must still pass against the consolidated sink.
- Add or extend a `logtest` package-level unit test asserting `Sink.Handle` populates `Record.Attrs` with the correct `slog.Value` for (a) attrs bound via `WithAttrs` (e.g. `component`) and (b) per-call attrs, including the last-write-wins behavior on a duplicate key, and that `Keys` and `Attrs` agree on the captured key set.
- Add `logtest` unit coverage for the new accessors (`AttrString`/`IntAttr`/`DurationAttr`/`OnlyRecord`/message-filter): correct value extraction by `slog.Value` kind, and the `*testing.T`-failing paths (missing key, not-exactly-one record) fail as the original store/timeout copies did.
- Run the full `go test ./...` to confirm no cross-package breakage from removed local types or changed imports.
