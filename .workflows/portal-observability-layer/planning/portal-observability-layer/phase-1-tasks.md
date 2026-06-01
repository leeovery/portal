---
phase: 1
phase_name: Logging foundation and call-site migration
total: 10
---

## portal-observability-layer-1-1 | approved

### Task 1-1: Create `internal/log` package skeleton with swappable-handler indirection and `For()`

**Problem**: Portal has no single owner of logging machinery. The spec mandates a new `internal/log` package that constructs the only `*slog.Logger` in the codebase, behind a swappable inner handler so loggers cached at package-init (before `Init` runs) still route to the configured handler once it lands. Without this foundation, nothing else in the feature can be built.

**Solution**: Create package `internal/log` with a package-`init`-constructed root `*slog.Logger` over a small custom handler whose inner delegate is replaceable via an `atomic.Pointer`-guarded (or mutex-guarded) indirection. Expose `For(component string) *slog.Logger` returning `root.With("component", name)`. The default inner handler (pre-`Init`) writes INFO-and-above as text to stderr.

**Outcome**: `log.For("daemon")` returns a valid non-nil `*slog.Logger` at any time — before or after `Init` — and a later handler swap (Task 1-4) is observed by every previously-returned logger because the indirection is shared.

**Do**:
- Create `internal/log/log.go` with `package log` and the swappable-handler indirection. Define an unexported handler type (e.g. `swapHandler`) that holds an `atomic.Pointer[slog.Handler]` (or a `sync.RWMutex` + `slog.Handler` field) as its inner delegate, and forwards `Enabled`, `Handle`, `WithAttrs`, `WithGroup` to the currently-pinned inner handler under a single synchronized read per call.
- In the package `init` function, construct the swap handler with a safe default inner handler that writes INFO-and-above as text to stderr (`slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})` is acceptable for the pre-`Init` default; the configured handler installed in Task 1-3/1-4 replaces it). Build the package-private `root = slog.New(swap)`.
- Expose an unexported setter the later `Init` (Task 1-4) uses to swap the inner handler atomically — e.g. `setHandler(h slog.Handler)` storing into the indirection. Keep it unexported; only `Init`/`SetTestHandler` call it.
- Implement `func For(component string) *slog.Logger { return root.With("component", component) }`. Do NOT special-case an empty component string — `For("")` must still return a valid non-nil logger (it binds `component=""`); the closed taxonomy is enforced by convention, not a runtime guard.
- Add `internal/log/doc.go` (or a package comment) stating the single-owner invariant: no `*slog.Logger` is constructed anywhere outside this package.

**Acceptance Criteria**:
- [ ] `log.For("daemon")` returns a non-nil `*slog.Logger` when called before any `Init`.
- [ ] The package `init` runs before any consumer's `For` call (guaranteed by Go's import-ordering — assert by a test that calls `For` from its own `init`/`TestMain` path and sees non-nil).
- [ ] A logger obtained via `For` before a handler swap routes its records to the swapped-in handler after the swap (verified once `SetTestHandler` from Task 1-5 exists; for this task, verify via the unexported `setHandler` seam in an in-package `_test.go`).
- [ ] The pre-`Init` default handler writes INFO-and-above to stderr as text (DEBUG is dropped at the default level).
- [ ] Concurrent `For` calls and a concurrent handler swap do not data-race (`go test -race`).

**Tests**:
- `"it returns a non-nil logger from For before Init"`
- `"it routes a pre-swap cached logger to the handler installed after the swap"`
- `"it returns a valid logger for an empty component string"`
- `"it is race-free under concurrent For and handler swap"` (run with `-race`)
- `"it drops DEBUG and emits INFO to stderr under the pre-Init default handler"`

**Edge Cases**:
- For-before-Init must return a valid, non-nil logger (never nil, never panic).
- Concurrent `For` / handler-swap: the indirection's read/write must be synchronized (atomic load/store or RW-lock); verify under `-race`.
- Empty component string: `For("")` is valid; no runtime rejection (closed taxonomy is a convention/review gate, not enforced here).

**Context**:
> "The root `*slog.Logger` is constructed in `internal/log`'s own package `init`, over a small custom handler whose inner delegate is **replaceable** (mutex- or `atomic.Pointer`-guarded). Because every consumer imports `internal/log`, Go runs its `init` first, so `root` exists before any `For` call." … "Before `Init` runs, the indirection holds a **safe default handler that writes INFO-and-above as text to stderr**." … "Cost: one synchronized read (atomic load / RLock) per `Handle`." (spec § The `internal/log` package → Init/For contract)
>
> `For returns a component-bound child logger (root.With("component", name)). Safe to call before Init — always returns a valid, non-nil *slog.Logger.` (spec § Public API)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § The `internal/log` package (Public API, Init/For contract — swappable-handler indirection)

---

## portal-observability-layer-1-2 | approved

### Task 1-2: Resolve log level from `PORTAL_LOG_LEVEL` with INFO default and invalid-value fallback

**Problem**: The legacy `internal/state.parseLevel` defaults to WARN (the posture that lost evidence on 2026-05-28) and accepts a legacy `"warning"` alias. The spec locks the production default to INFO, defines a closed valid set (`debug`/`info`/`warn`/`error`), and requires invalid values to fall back to INFO with a startup WARN — and the resolver must report not just the level but *how* it was resolved (`env`/`default`/`fallback`) and the raw observed value, so Phase 2's `log-level resolved` line can be emitted.

**Solution**: Add a level-resolution function in `internal/log` that reads `PORTAL_LOG_LEVEL`, trims and lowercases it, maps it to an `slog.Level`, and returns a small result carrying the resolved level, a `source` enum (`env`/`default`/`fallback`), and the raw env string. The function performs the resolution only; the WARN emission and the `log-level resolved` line are wired in later tasks/phases — this task delivers the pure resolution logic and its data shape.

**Outcome**: Given any `PORTAL_LOG_LEVEL` value (or unset), the resolver returns the correct `(level, source, raw)` triple: unset → `(info, default, "")`; valid (any case/whitespace) → `(<level>, env, <raw>)`; invalid → `(info, fallback, <raw>)`.

**Do**:
- In `internal/log` (e.g. `internal/log/level.go`), define an unexported resolver, e.g. `func resolveLevel(raw string) (lvl slog.Level, source string, normRaw string)`. Read the raw value at the call site (`os.Getenv("PORTAL_LOG_LEVEL")`) and pass it in, so the resolver is pure and unit-testable without env mutation; provide a thin wrapper that reads the env if convenient.
- Map after `strings.ToLower(strings.TrimSpace(raw))`: `"debug"→slog.LevelDebug`, `"info"→slog.LevelInfo`, `"warn"→slog.LevelWarn`, `"error"→slog.LevelError`. Do NOT accept `"warning"` (the legacy alias is dropped per the migration sweep).
- `source` values: empty/unset raw → `"default"` with `slog.LevelInfo`; a value matching the set → `"env"`; any other non-empty value → `"fallback"` with `slog.LevelInfo`.
- Return the `raw` value as observed (verbatim, NOT trimmed/lowercased) so the eventual `raw=` attr and the invalid-value WARN render the exact user input. (Normalize only for the match; preserve verbatim for reporting.)
- Provide a helper that maps the resolved `slog.Level` back to the lowercase string (`debug`/`info`/`warn`/`error`) for the `resolved=` attr Phase 2 needs.
- Do NOT emit the WARN or the `log-level resolved` line here — those are Phase 2 (lifecycle markers / level-filter bypass). This task only computes and returns the resolution.

**Acceptance Criteria**:
- [ ] Unset `PORTAL_LOG_LEVEL` resolves to `(slog.LevelInfo, "default", "")`.
- [ ] `"debug"`, `"info"`, `"warn"`, `"error"` (exact) resolve to their levels with `source="env"`.
- [ ] Mixed-case (`"DEBUG"`, `"Warn"`) and surrounding whitespace (`"  info  "`) resolve correctly with `source="env"`, while `raw` preserves the verbatim input.
- [ ] `"warning"` (legacy alias) is NOT accepted → resolves to `(info, fallback, "warning")`.
- [ ] Any other invalid value (`"trace"`, `"verbose"`, `"5"`) resolves to `(info, fallback, <verbatim>)`.

**Tests**:
- `"it defaults to info when PORTAL_LOG_LEVEL is unset"`
- `"it resolves each valid level with source=env"`
- `"it normalises mixed case and surrounding whitespace"`
- `"it rejects the legacy warning alias and falls back to info"`
- `"it falls back to info with source=fallback for an invalid value, preserving raw verbatim"`

**Edge Cases**:
- Unset → `default` (not `fallback`); the two sources are distinct.
- Mixed-case + whitespace must still match `env`.
- `"warning"` must NOT map to WARN (regression guard against the legacy behaviour).
- `raw` is preserved verbatim for reporting even though matching is done on the normalised form.

**Context**:
> "Default `PORTAL_LOG_LEVEL = info`. Invalid env value (any value that is not exactly `debug` / `info` / `warn` / `error` after lowercasing) → fall back to `info` and emit one WARN at process start." (spec § Log-level discipline → Default and invalid-value handling)
>
> `source` is one of: `env` (set to a valid value), `default` (unset → info), `fallback` (set to an invalid value → fell back to info). `raw` is the raw env var value as observed (empty string if unset, the verbatim string if set — including invalid values). (spec § Log-level propagation verification → Mechanical rule)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Log-level discipline (Default and invalid-value handling), § Log-level propagation verification (source/raw definitions)

---

## portal-observability-layer-1-3 | approved

### Task 1-3: Inject per-record baseline attrs and render component-prefix text in the foundation handler

**Problem**: Every log line must be self-describing (`component`, `pid`, `version`, `process_role`) and greppable by `grep "hydrate:"`. Baselines must be injected **per-record by the handler**, not via `root.With(...)` — otherwise package-init children created before `Init` would miss them. The text-mode line must render `component:` as a literal prefix (not also in the attr list), with a specific attr ordering and value-formatting rules.

**Solution**: Implement the configured text-mode handler in `internal/log` (the one swapped into the indirection by `Init`). It wraps an `io.Writer`, holds the three injectable baselines (`pid`, `version`, `process_role`) given at construction, and on each `Handle` injects them per-record, reads the `component` attr to render the `component:` prefix, and emits the remaining attrs as space-separated `key=value` pairs per the spec's formatting rules. This task delivers the SIMPLE (non-rotating, single-writer) text handler behind the swap indirection; rotation/retention internals are Phase 2.

**Outcome**: A record logged through the configured handler renders exactly `<RFC3339Nano> <LEVEL> <component>: <msg> <contextual attrs> pid=… version=… process_role=…`, baselines present even on a logger cached before `Init`, with multi-word values quoted, durations via `String()`, and groups flattened to dotted keys.

**Do**:
- Create `internal/log/handler.go` with a constructor, e.g. `newTextHandler(w io.Writer, level slog.Leveler, pid int, version, processRole string) slog.Handler`. Store the three baselines on the handler struct.
- Implement `Handle(ctx, record)`:
  1. Read the `component` attr from the record (it is set by `For` via `root.With("component", ...)`). Render it as the literal prefix immediately before `:` and do NOT also emit it in the `key=value` attr list.
  2. Render the line: `<record.Time RFC3339Nano> <LEVEL upper> <component>: <record.Message> <attrs>`.
  3. Emit contextual attrs in `slog.Record` iteration order, then append the three remaining baselines `pid=<pid> version=<version> process_role=<processRole>` (injected here, NOT via `With`).
  4. Quote multi-word string values with `"` (a value containing whitespace gets surrounding double quotes); single-token strings unquoted.
  5. Render `time.Duration` attr values with Go's default `String()` (e.g. `1.234s`).
  6. Flatten `slog.Group` attrs to dotted keys (`group.key=value`), mirroring JSON nesting.
- Honour `WithAttrs`/`WithGroup` so `For`'s `With("component", ...)` and any sticky `.With(...)` context compose correctly (the `component` attr must be findable regardless of whether it arrives via record attrs or accumulated `WithAttrs`).
- Implement `Enabled(ctx, level)` against the handler's `slog.Leveler` so level filtering works. (The lifecycle-marker level-filter *bypass* is Phase 2 — do NOT implement it here; this handler is an ordinary level-gated handler.)
- The writer is a plain `io.Writer` write per record (no `bufio`) — keep it unbuffered, consistent with the locked constraint, though the rotating `*os.File` wiring is Phase 2.
- Provide a JSON-mode path only if trivially free via `slog.NewJSONHandler`; the spec says JSON is standard slog with no special handling. If a mode switch is not needed for Phase 1 wiring, keep the handler text-only and leave JSON for when a consumer needs it — note this in a code comment. `[needs-info]`: the spec describes both text and JSON rendering but does not pin a Phase-1 mechanism for selecting between them (no env var named). Implement text-mode (the tail/grep default) and leave a clearly-commented seam for JSON; do not invent a selection env var.

**Acceptance Criteria**:
- [ ] A record carrying `component=hydrate`, msg `ok`, and `pane_key=foo:0.0`, `took=1.2s` renders as `… INFO hydrate: ok pane_key=foo:0.0 took=1.2s pid=<pid> version=<v> process_role=<role>` with baselines last and `component` NOT duplicated in the attr list.
- [ ] Baseline attrs (`pid`/`version`/`process_role`) appear on a record emitted through a logger obtained from `For` BEFORE the handler was constructed/swapped (per-record injection, not construction-time `With`).
- [ ] A multi-word string attr value is wrapped in double quotes; a single-token value is not.
- [ ] A `time.Duration` attr renders via `String()` (e.g. `3s`, `1.2s`), not as an integer nanosecond count.
- [ ] A `slog.Group("g", slog.String("k","v"))` attr renders as `g.k=v`.
- [ ] The handler level-filters: at INFO level a DEBUG record is dropped.

**Tests**:
- `"it renders component as a literal prefix and omits it from the attr list"`
- `"it appends pid/version/process_role baselines per-record in trailing order"`
- `"it injects baselines on a logger cached before the handler existed"`
- `"it quotes multi-word string values and leaves single tokens unquoted"`
- `"it renders time.Duration via String()"`
- `"it flattens slog.Group attrs to dotted keys"`
- `"it drops a DEBUG record when the configured level is INFO"`

**Edge Cases**:
- Package-init child created before the handler existed still carries baselines (the per-record-injection guarantee).
- Multi-word values quoted; durations via default `String()`; groups flattened to dotted keys.
- `component` rendered as prefix only, never duplicated in the `key=value` list.

**Context**:
> Example text line: `2026-05-29T08:38:00Z INFO hydrate: ok pane_key=foo:0.0 took=1.2s pid=12345 version=0.5.0 process_role=hydrate` (spec § Subsystem prefix taxonomy → Rendering mechanism)
>
> Text-mode rendering rule: `<RFC3339Nano timestamp> <LEVEL> <component>: <msg> <attrs as key=value pairs>`; `<component>` emitted as literal prefix and NOT in the attr list; attrs in `slog.Record` order then the three remaining baselines (`pid`, `version`, `process_role`); multi-word values quoted; `time.Duration` via `String()`; `slog.Group` flattened to dotted keys. (spec § Subsystem prefix taxonomy → Custom `slog.Handler` text-mode rendering rule)
>
> "Baseline-attr injection: baseline attrs (`pid`, `version`, `process_role`) are injected by the configured handler **per-record**, NOT via `root.With(...)` at construction." (spec § Init/For contract)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Subsystem prefix taxonomy (Rendering mechanism, Mandatory baseline attrs, text-mode rendering rule), § Init/For contract (per-record baseline injection)

---

## portal-observability-layer-1-4 | approved

### Task 1-4: Implement `Init`/`Close` public API with stateDir/version/processRole wiring and startTime capture

**Problem**: `main` needs an idempotent `Init` that builds the configured handler (with resolved level + injected baselines) and swaps it into the indirection, plus a `Close` marker-emitter that owns no control flow. The handler must be wired so loggers cached before `Init` route to it afterwards, and `startTime` must be captured at `Init` for the eventual `process: exit` `took` computation.

**Solution**: Implement `Init(stateDir, version, processRole string) error` and `Close(exitCode int)` in `internal/log`. `Init` resolves the level (Task 1-2), captures `os.Getpid()` and a package-private `startTime`, constructs the configured text handler (Task 1-3) bound to those baselines and the resolved level, and atomically swaps it into the indirection (Task 1-1's `setHandler`). `Init` is idempotent and re-entrant. `Close` computes `took` from `startTime` but does NOT call `os.Exit`. The lifecycle-marker *emission* bodies (`process: start`, `process: exit`) are Phase 2 — Phase 1 delivers the wiring/signatures and the non-control-flow contract; leave a clearly-marked seam where Phase 2 adds the marker emission.

**Outcome**: `main` can call `Init("/state", version, "tui")` and every `For`-created logger (including pre-`Init` cached ones) then writes through the configured handler at the resolved level with baselines; a second `Init` re-points the handler without panicking; `Close(0)` returns normally without exiting the process.

**Do**:
- In `internal/log` add `func Init(stateDir, version, processRole string) error`. Steps: resolve the level via Task 1-2's resolver (reading `PORTAL_LOG_LEVEL`); capture `pid := os.Getpid()`; set the package-private `startTime = time.Now()`; construct the configured handler (Task 1-3) bound to `(writer, level, pid, version, processRole)`; swap it in via the unexported `setHandler`.
- For Phase 1, the writer target is the simple (non-rotating) sink: open/append `${stateDir}/portal.log` (reuse `state.PortalLog`/`state.EnsureDir`-equivalent path resolution, or accept `stateDir` and join `portal.log`) with `O_APPEND|O_CREATE|O_WRONLY`, mode `0600`, unbuffered. `[needs-info]`: the date-aware rotating handler, `O_CREAT|O_EXCL` first-of-day open, and symlink swing are explicitly Phase 2 — for Phase 1 use a plain append open so `Init` is end-to-end functional, and leave a code comment marking the open site as the Phase-2 rotating-handler insertion point. On open failure, fall back to a stderr-text handler (best-effort; logging never fails the caller) and return the error so `main` can decide — but `main` per the spec calls `Init` first-thing and does not abort on logging failure; document that the returned error is advisory.
- Make `Init` idempotent/re-entrant: a second call re-resolves and re-swaps the handler; it must not panic and must not leak/close in a way that breaks a concurrent `Handle`. Reset `startTime` on each `Init` (the most recent `Init` defines the process's logical start for `took`).
- Add `func Close(exitCode int)`. Phase 1 body: compute `took := time.Since(startTime)`. Do NOT call `os.Exit`. Leave the actual `process: exit` INFO emission as a Phase-2 seam (a clearly-commented TODO referencing Phase 2 / Defensive invariants), but the signature, the `took` computation from `startTime`, and the no-control-flow guarantee land now so `main` (Task 1-7) can call it.
- `Close` must be safe to call before `Init` (startTime would be the zero value → `took` is large but harmless; guard so it does not panic).
- Add the package-private `startTime` var and `pid`/baseline storage as needed.

**Acceptance Criteria**:
- [ ] After `Init("/dir", "0.5.0", "tui")`, a logger from `For("daemon")` (obtained before `Init`) writes through the configured handler with `pid`/`version=0.5.0`/`process_role=tui` baselines at the resolved level.
- [ ] A second `Init` call re-points the handler (e.g. different `processRole`) without panicking; subsequent records carry the new baselines.
- [ ] `Close(0)` returns normally and does NOT terminate the test process (assert the function returns).
- [ ] `Close` called before any `Init` does not panic.
- [ ] `Init` captures `startTime`; `Close` computes a non-negative `took` from it.

**Tests**:
- `"it routes a pre-Init cached logger to the configured handler after Init"`
- `"it re-points the handler on a second Init without panicking"`
- `"it captures startTime at Init and Close computes took from it"`
- `"it returns from Close without calling os.Exit"`
- `"it is safe to call Close before Init"`

**Edge Cases**:
- Second `Init` re-points the handler without panic (idempotent/re-entrant).
- Pre-`Init` cached loggers route to the configured handler after `Init`.
- `Close` owns no control flow (never exits).
- `Close` before `Init` is safe.

**Context**:
> `Init configures the process-wide logger … and atomically swaps it in behind the stable root logger. Called from main.go before any other portal code runs. IDEMPOTENT and re-entrant — a second call re-points the handler, it does NOT panic.` (spec § Public API)
>
> `Close emits the "process: exit" marker, computing took from the package-private startTime captured at Init. Does NOT call os.Exit — the logger owns no control flow.` (spec § Public API). Per the planning scope boundary, the marker *body* lands in Phase 2; Phase 1 delivers the signature, startTime capture, took computation, and the no-control-flow contract.
>
> Per the Phase 1 scope boundary: "Init/Close exist and are called by main, but their lifecycle-marker bodies land in Phase 2." The rotating handler internals, `O_CREAT|O_EXCL` first-of-day open, and `process: start/exit` emission are all Phase 2.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § The `internal/log` package (Public API, Init/For contract); planning Phase 1/Phase 2 scope boundary

---

## portal-observability-layer-1-5 | approved

### Task 1-5: Add `SetTestHandler` test-only seam restoring prior handler via `t.Cleanup`

**Problem**: Tests need to capture or silence log output in-process without spawning a subprocess, and without leaking a swapped handler into sibling tests. The "configured once in prod" invariant is preserved by convention plus this test-only seam — not by panicking.

**Solution**: Add `func SetTestHandler(t *testing.T, h slog.Handler)` to `internal/log` that swaps `h` into the indirection for the duration of the test and registers a `t.Cleanup` restoring the previously-pinned inner handler.

**Outcome**: A test can call `log.SetTestHandler(t, captureHandler)`, exercise code that logs via `For`, assert on the captured records, and have the prior handler automatically restored when the test ends — including correct LIFO restoration under nested swaps.

**Do**:
- In `internal/log` add `func SetTestHandler(t *testing.T, h slog.Handler)`. The `*testing.T`-first parameter structurally marks it test-only (consistent with `portaltest.IsolateStateForTest`).
- Read the current inner handler from the indirection, swap in `h` via the unexported `setHandler`, and register `t.Cleanup(func() { setHandler(prev) })` to restore.
- Nested swaps must restore in LIFO order: because each `SetTestHandler` captures the handler present at call time and `t.Cleanup` runs in reverse registration order, the original is restored after inner swaps unwind. Verify this explicitly.
- Restoring on a test that never logged must be a no-op-safe operation (the cleanup runs regardless; the restore must not panic if no record was ever emitted).
- Document that this is the only sanctioned way to replace the handler outside `Init`, and that production code must never call it (the `*testing.T` parameter prevents importing it from non-test code).

**Acceptance Criteria**:
- [ ] `SetTestHandler(t, h)` causes subsequent `For(...)` records to route to `h`.
- [ ] After the test ends (cleanup), the previously-pinned handler is restored.
- [ ] Two nested `SetTestHandler` calls restore in correct LIFO order (inner restored first, then outer, leaving the original).
- [ ] A test that calls `SetTestHandler` but never logs still restores cleanly (no panic).

**Tests**:
- `"it routes records to the test handler after SetTestHandler"`
- `"it restores the prior handler via t.Cleanup"`
- `"it restores nested swaps in LIFO order"`
- `"it restores cleanly when the test never logged"`

**Edge Cases**:
- Nested swaps restore in the correct (LIFO) order.
- Restore on a test that never logged is safe.

**Context**:
> `SetTestHandler swaps in h for the duration of the test and restores the previous handler via t.Cleanup. Test-only seam for capturing or silencing log output in-process — no subprocess required.` (spec § Public API)
>
> "The 'configured once in prod' invariant is preserved by **convention** (only `main` calls `Init`) plus the test-only `SetTestHandler` seam — **not** by panicking. In-process tests swap a capture / `io.Discard` handler; no subprocess required." (spec § Init/For contract)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § The `internal/log` package (Public API, Init/For contract)

---

## portal-observability-layer-1-6 | approved

### Task 1-6: Resolve `process_role` from `os.Args` longest-prefix match against the closed table

**Problem**: `Init` is called from `main` before Cobra parses argv, so `process_role` must be resolved from a lightweight `os.Args` inspection — a longest-prefix match of the leading subcommand path tokens (flags ignored) against a small static table covering the closed 6-value space, defaulting to `bootstrap`.

**Solution**: Add a resolver in `internal/log` (or a tiny helper `main` calls) that takes `os.Args[1:]`, strips flag tokens, and longest-prefix-matches the leading subcommand-path tokens against the static table, returning one of `daemon`/`hydrate`/`hooks_cli`/`clean`/`tui`/`bootstrap`.

**Outcome**: `portal state daemon` → `daemon`; `portal state hydrate`/`state signal-hydrate` → `hydrate`; `portal hooks …` → `hooks_cli`; `portal clean` → `clean`; `portal open …`/`x …`/`attach …`/bare `portal` → `tui`; anything else → `bootstrap`.

**Do**:
- Add `func ResolveProcessRole(args []string) string` in `internal/log` (exported so `main` can call it with `os.Args[1:]`). Keep it pure (takes args, returns string) for testability.
- Filter out flag tokens (any token starting with `-`) before matching, so flags interleaved among subcommand tokens are ignored. Match on the leading **subcommand path tokens only**.
- Implement the table with first-match-wins, longest-prefix semantics:
  - leading tokens `state daemon` → `daemon`
  - leading tokens `state hydrate` OR `state signal-hydrate` → `hydrate`
  - leading token `hooks` → `hooks_cli`
  - leading token `clean` → `clean`
  - leading token `open` OR `x` OR `attach`, OR no subcommand token at all (bare `portal`) → `tui`
  - anything else → `bootstrap` (explicit default/fallback)
- The closed result space is exactly the 6 values; ensure no invocation returns anything else.
- `main` (Task 1-7) calls `log.ResolveProcessRole(os.Args[1:])` and passes the result to `log.Init`.

**Acceptance Criteria**:
- [ ] `["state","daemon"]` → `daemon`; `["state","hydrate"]` and `["state","signal-hydrate"]` → `hydrate`.
- [ ] `["hooks","set","--on-resume","x"]` → `hooks_cli`; `["clean"]` and `["clean","--logs"]` → `clean`.
- [ ] `["open","."]`, `["x"]`, `["attach","foo"]`, and `[]` (bare portal) → `tui`.
- [ ] An unknown subcommand (`["version"]`, `["init"]`, `["alias","add"]`) → `bootstrap`.
- [ ] Interleaved flags are ignored: `["--verbose","state","daemon"]` and `["state","--foo","daemon"]` both → `daemon`.

**Tests**:
- `"it resolves state daemon to daemon"`
- `"it resolves state hydrate and state signal-hydrate to hydrate"`
- `"it resolves hooks to hooks_cli and clean to clean"`
- `"it resolves open/x/attach/bare-portal to tui"`
- `"it resolves an unknown subcommand to bootstrap"`
- `"it ignores interleaved flag tokens when matching"`

**Edge Cases**:
- Bare `portal` (no subcommand) → `tui`.
- Unknown subcommand → `bootstrap` default.
- Flags interleaved/ignored (match on path tokens only).
- `state hydrate` vs `state daemon` disambiguation under the shared `state` prefix.
- `x`/`attach` aliases → `tui`.

**Context**:
> Table (spec § The `internal/log` package → `process_role` resolution):
> `state daemon` → `daemon`; `state hydrate` / `state signal-hydrate` → `hydrate`; `hooks …` → `hooks_cli`; `clean` → `clean`; `open …` / `x …` / `attach …` / no subcommand → `tui`; anything else → `bootstrap`.
>
> "main resolves `process_role` from a lightweight `os.Args` inspection — a longest-prefix match of the leading subcommand tokens against a small static table, matching on subcommand path tokens only (flags ignored) so it needs no full parse." … "First match wins. `bootstrap` is the explicit default for any invocation not matched above, so the closed 6-value space is fully covered."

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § The `internal/log` package (`process_role` resolution)

---

## portal-observability-layer-1-7 | approved

### Task 1-7: Adopt the `main` exit shape — single `os.Exit`, panic recovery, `Close` on non-panic path

**Problem**: The current `main.go` calls `os.Exit` in several branches and never initialises logging or marks process termination. The spec requires `main` to be the single owner of `os.Exit`, to `log.Init` before any other portal code, to recover panics into a code, and to call `log.Close` exactly once on the non-panic path — while preserving the existing `FatalError`, `UsageError`, and `IsSilentExitError` exit-code/stderr semantics.

**Solution**: Rewrite `main.go` to the idiomatic single-exit shape: call `log.Init(stateDir, version, role)` first; run `cmd.Execute()` inside a `func(){ defer recover() … }()` that maps a recovered panic to code 2 and an `Execute` error to the existing code mapping; on the non-panic path call `log.Close(code)`; then `os.Exit(code)` exactly once at the end of `main`.

**Outcome**: A clean run exits 0 after `Close(0)`; an `Execute` error exits 1 (or 2 for `UsageError`/`FatalError`) after `Close(code)`; a recovered panic exits 2 with `Close` skipped; silent-exit and fatal-error stderr behaviour is byte-identical to today.

**Do**:
- In `main.go`, before anything else: resolve `stateDir` (use the same resolution the rest of the code uses, e.g. `state.Dir()`; tolerate error — pass whatever path resolution yields, logging must not block startup), resolve `processRole := log.ResolveProcessRole(os.Args[1:])` (Task 1-6), and `version` from the `cmd.version` build var (expose an accessor if `cmd.version` is unexported — e.g. a `cmd.Version()` getter, or read the existing exported symbol). Call `log.Init(stateDir, version, processRole)` and tolerate its error (logging never blocks startup; the returned error is advisory).
- Restructure termination to the canonical shape:
  ```go
  code := 0
  panicked := false
  func() {
      defer func() {
          if r := recover(); r != nil {
              // process: panic emission is Phase 2; Phase 1 maps the code.
              code = 2
              panicked = true
          }
      }()
      if err := cmd.Execute(); err != nil {
          // preserve existing classification:
          //   *bootstrap.FatalError → code already-printed-to-stderr by Execute; UsageError-style exit
          //   IsSilentExitError → do not print
          //   UsageError → code 2
          //   default → code 1
          code = classify(err)  // inline the existing main.go logic
      }
  }()
  if !panicked {
      log.Close(code)
  }
  os.Exit(code)
  ```
- Port the existing classification verbatim: `errors.As(err, &fatal)` → code 1 (stderr already written by `Execute`; do not duplicate); `!cmd.IsSilentExitError(err)` → `fmt.Fprintln(os.Stderr, err)`; `errors.As(err, &usageErr)` → code 2; otherwise code 1. The `FatalError` path currently exits 1 — preserve that exact mapping. Do NOT change the exit codes or the stderr-suppression contract.
- `os.Exit` must appear exactly once, as the final statement of `main`. No other `os.Exit` in `main`.
- Do NOT emit `process: start`/`exit`/`panic` markers here — those bodies are Phase 2. The recover block sets `code=2`/`panicked=true` and is the *seam* where Phase 2 adds `log.For("process").Error("panic", "reason", r)`; leave a clearly-marked comment. Phase 1 proves the exit-shape and code mapping only.
- Confirm no production code outside `main` introduces a new `os.Exit` in this task; the daemon self-eject's existing `osExit` seam is untouched (its marker pairing is Phase 5).

**Acceptance Criteria**:
- [ ] A clean `Execute()` (nil error) results in `code=0`, `log.Close(0)` called once, `os.Exit(0)`.
- [ ] An `Execute()` error that is a `*UsageError` → `code=2`; an ordinary error → `code=1`; a `*bootstrap.FatalError` → `code=1` with no duplicated stderr; an `IsSilentExitError` → `code=1` with nothing printed to stderr.
- [ ] A panic during `Execute()` is recovered → `code=2`, `panicked=true`, `log.Close` is NOT called, `os.Exit(2)`.
- [ ] `log.Init` is called before `cmd.Execute()`.
- [ ] `os.Exit` appears exactly once in `main` (the final statement).
- [ ] Existing stderr output for `FatalError`/silent-exit/ordinary-error paths is unchanged.

**Tests**:
- Because `main` calls `os.Exit`, drive the classification via an extracted, testable `run() (code int, panicked bool)` helper (or assert through the existing `cmd.Execute` test seams + an in-`main`-package test that mocks `cmd.Execute`). Name tests:
- `"it returns code 0 and calls Close on a clean Execute"`
- `"it returns code 1 on an ordinary Execute error and prints it to stderr"`
- `"it returns code 2 on a UsageError"`
- `"it returns code 1 on a FatalError without duplicating stderr"`
- `"it suppresses stderr for an IsSilentExitError"`
- `"it recovers a panic to code 2 and skips Close"`

**Edge Cases**:
- `Execute` error → code 1; `UsageError` → code 2; `FatalError` → code 1 (no duplicate stderr).
- Recovered panic → code 2 with `Close` skipped (the four-way classification stays mutually exclusive: panic path's terminal marker is Phase 2's `process: panic`, not `Close`).
- `IsSilentExitError` suppression preserved.
- Exactly one `os.Exit`, in `main` only.

**Context**:
> The idiomatic shape (exit only in `main`, everything else returns a code/error) — see spec § Defensive invariants → Mechanical rule — `process: exit` and the `main` exit shape, including the `panicked` flag, the recover block, and "On the panic path `process: panic` is the **sole** terminal marker — `Close` is skipped."
>
> "`os.Exit` skips deferred functions, so a Close-defer would miss Cobra's `Execute()`-error path — the most operationally-interesting termination class." … "Bare `os.Exit` is prohibited outside `main`."
>
> Per the Phase 1 scope boundary, the `process: start/exit/panic` marker *emission* is Phase 2; this task delivers the exit shape and code mapping, leaving the marker emission as a commented seam.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Defensive invariants against log destruction (Mechanical rule — `process: exit` and the `main` exit shape); current `main.go` exit shape

---

## portal-observability-layer-1-8 | approved

### Task 1-8: Migrate intermediate logging seams off `*state.Logger` to `*slog.Logger`

**Problem**: Several packages define their own logging seams typed around `*state.Logger` or a `Debug(component, format, args...)` interface that mirrors it — `bootstrap.Logger`/`NoopLogger`, the bootstrap `*Core`/`*Sweeper` types, `tmux.BarrierLogger`/`MigrationLogger`/`SetBarrierLogger`/`SetVersionWriterLogger`/`SaverVersionSeams.WriterLogger`, `restore.Orchestrator.Logger`, the `bootstrapadapter` adapters, and the `tui.previewAttachPipeline`. These seams sit between call sites and the logger; they must be retyped to `*slog.Logger` so the big-bang call-site rewrite (Task 1-9) has a coherent target. The legacy seams rely on a nil-receiver no-op contract that `*slog.Logger` does NOT provide.

**Solution**: Retype every intermediate logging seam to hold a real `*slog.Logger` (never nil — callers pass a real logger, obtained from `log.For`). Replace the `(component, format, args...)` method-arg interface shape with the slog `(msg, attrs...)` shape, binding the component at the seam's owning package via `log.For`. Remove the nil-receiver-no-op assumption; where a "silent" logger is needed, callers pass `slog.New(slog.NewTextHandler(io.Discard, nil))`.

**Outcome**: All intermediate seams compile against `*slog.Logger`; the `bootstrap.Logger` interface is either removed (callers hold `*slog.Logger` directly) or redefined to the slog method shape; `tmux` and `restore` seams hold `*slog.Logger`; `export_test.go`'s `VersionWriterLoggerSeam` returns `**slog.Logger`; no intermediate seam references `state.Logger` or `state.Component*`.

**Do**:
- **`cmd/bootstrap/bootstrap.go`**: Replace the `bootstrap.Logger` interface (`Debug/Info/Warn/Error(component, format, args...)`) and `NoopLogger`. Two viable shapes — pick the one minimising churn against Task 1-9:
  - Preferred: drop the custom interface and change `Orchestrator.Logger` to `*slog.Logger`, binding `component=bootstrap` at the package level via `var logger = log.For("bootstrap")` — but note the orchestrator's nil-substitution (`if o.Logger == nil { o.Logger = NoopLogger{} }`) must become "callers always inject a real logger" (a `log.For("bootstrap")` result is never nil). If a no-op default is still desired for tests, default to `slog.New(slog.NewTextHandler(io.Discard, nil))`.
  - The `Orchestrator.Run` step-entry calls (`o.Logger.Debug(state.ComponentBootstrap, "step N…")`) become slog calls — but their *message/attr rewrite* is Task 1-9; this task only retypes the field/interface so it compiles. Coordinate: if the field becomes `*slog.Logger`, the call-site bodies must be rewritten in the same PR (Tasks 1-8/1-9 land together as one big-bang). Treat 1-8 as "retype the seams"; 1-9 as "rewrite the call expressions."
- **`cmd/bootstrap/{orphan_sweep,stale_marker_cleanup,eager_signal_hydrate}.go`**: their `Logger` fields are the `bootstrap.Logger` interface; retype to `*slog.Logger` (or the redefined interface). Remove the `if logger == nil { logger = NoopLogger{} }` local substitution in favour of an always-real injected logger (or an `io.Discard` slog default).
- **`internal/tmux/portal_saver.go`**: replace `BarrierLogger`/`noopBarrierLogger` and `SaverVersionSeams.WriterLogger *state.Logger` with `*slog.Logger`. `SetBarrierLogger`/`SetVersionWriterLogger` take `*slog.Logger`. Bind `var logger = log.For("saver")` (and `log.For("bootstrap")` where the WARN sites render under bootstrap — confirm against Phase 5 catalog; for Phase 1, preserve the current component literal these emit under, i.e. `bootstrap`, by binding the appropriate `log.For`). The nil-ignore guards in the setters stay (ignore a nil `*slog.Logger`), but the default sink becomes an `io.Discard` slog logger rather than `noopBarrierLogger{}`.
- **`internal/tmux/hooks_register.go`**: replace `MigrationLogger` interface and its `(*state.Logger)(nil)` fallback. `migrateHydrationHooks`/`migrateSessionClosedHook`/`RegisterPortalHooks` take `*slog.Logger`; the `if log == nil { log = (*state.Logger)(nil) }` fallback becomes `if log == nil { log = <discard slog logger> }`.
- **`internal/tmux/export_test.go`**: `VersionWriterLoggerSeam()` returns `**state.Logger` → change to `**slog.Logger`. Update `SaverIdentifyDaemonSeam` etc. only if they reference logger types (they don't). Remove the `state` import if it becomes unused.
- **`internal/restore/restore.go`**: `Orchestrator.Logger *state.Logger` → `*slog.Logger`; the `Logger: o.Logger` propagation into the session restorer stays but retyped. Bind `var logger = log.For("restore")` for the call sites (rewrites in 1-9).
- **`internal/bootstrapadapter/adapters.go`** and **`orphan_sweep.go`**: `HookRegistrar.Logger`, `FIFOSweeper.Logger`, `NewRestoreAdapter(logger *state.Logger)`, `NewOrphanSweeper(logger *state.Logger)` → `*slog.Logger`.
- **`internal/tui/preview_attach.go`**: `previewAttachPipeline.logger *state.Logger` and `NewPreviewAttachPipeline(t, logger *state.Logger)` → `*slog.Logger`. Bind `var logger = log.For("preview")` for the WARN sites (rewritten in 1-9). The nil-tolerance comment must change: a `*slog.Logger` is not nil-receiver-safe, so production passes a real logger (`log.For("preview")`), and the open-failure-passes-nil pattern is removed (see Task 1-9 for the open-site change).
- **`internal/state/{commit,scrollback,daemon_state,capture,fifo_sweep}.go` function signatures** that take `logger *state.Logger` (e.g. `Commit`, `gcOrphanScrollback`, `SeedHashMap`, `WriteVersionFile`, `CaptureStructure`, `SweepOrphanFIFOs`): retype the parameter to `*slog.Logger`. NOTE: these live in `internal/state`, which will no longer own a Logger after Task 1-10 — importing `internal/log` from `internal/state` is the new dependency; confirm no import cycle (`internal/log` must not import `internal/state`; if `Init`'s Phase-1 path-join needs `state.PortalLog`, accept a `stateDir string` instead to avoid the cycle — see Task 1-4). The call-site message rewrites are Task 1-9; this task retypes the signatures.
- Across all the above, remove `noopBarrierLogger` and any custom no-op logger types that existed only to satisfy the nil-receiver contract; the canonical "silent logger" is now `slog.New(slog.NewTextHandler(io.Discard, nil))`.

**Acceptance Criteria**:
- [ ] No intermediate seam type (struct field, interface, function parameter, or `export_test` accessor) references `*state.Logger`, `state.Logger`, or `state.Component*`.
- [ ] `bootstrap.Logger`/`NoopLogger` are either removed or redefined to the slog `(msg, attrs...)` method shape; the orchestrator holds a real `*slog.Logger` (or the redefined interface) and never relies on a nil-receiver no-op.
- [ ] `tmux.SetBarrierLogger`/`SetVersionWriterLogger`/`SaverVersionSeams.WriterLogger` and the `MigrationLogger` seam are typed `*slog.Logger`; `export_test.go`'s `VersionWriterLoggerSeam` returns `**slog.Logger`.
- [ ] `restore.Orchestrator.Logger`, the `bootstrapadapter` adapters, and `tui.previewAttachPipeline.logger` are typed `*slog.Logger`.
- [ ] The canonical silent logger is `slog.New(slog.NewTextHandler(io.Discard, nil))` (or a shared `internal/log` helper); no `noopBarrierLogger`-style nil-receiver shim remains.
- [ ] `go build ./...` succeeds for these packages once Task 1-9 lands (the two tasks land together — see note).

**Tests**:
- `"it constructs an Orchestrator with a real *slog.Logger and Run does not panic"`
- `"it accepts a *slog.Logger via SetBarrierLogger and SetVersionWriterLogger"`
- `"it threads a *slog.Logger through RegisterPortalHooks migration helpers"`
- `"it constructs NewRestoreAdapter/NewOrphanSweeper/NewPreviewAttachPipeline with a *slog.Logger"`
- `"VersionWriterLoggerSeam returns a **slog.Logger"`

**Edge Cases**:
- The nil-receiver no-op contract is removed — callers hold a real logger; "silent" is `io.Discard` slog, not a nil pointer.
- `component` becomes a bound attr (via `log.For`) not a method argument — interface method shape changes from `(component, format, args...)` to `(msg, attrs...)`.
- `export_test.go` `VersionWriterLoggerSeam` type change to `**slog.Logger`.
- Import-cycle guard: `internal/log` must not import `internal/state`; `internal/state` may import `internal/log` for the retyped signatures.

**Context**:
> "All test mock surfaces in `bootstrapDeps` and friends that previously held `*state.Logger` are updated to hold `*slog.Logger`." (spec § Migration sweep)
>
> "`state.NopLogger()` is **deleted**; tests requiring a silent logger use `slog.New(slog.NewTextHandler(io.Discard, nil))` directly." (spec § Migration sweep)
>
> Planning Phase 1 task row 1-8 enumerates: `bootstrap.Logger`/`NoopLogger`/`BarrierLogger`/`MigrationLogger`/`SetBarrierLogger`/`SetVersionWriterLogger`/`SaverVersionSeams.WriterLogger`/`restore.Orchestrator.Logger`, and the `export_test` `VersionWriterLoggerSeam` type change.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § The `internal/log` package (Migration sweep, Consumer usage); planning Phase 1 task 1-8

---

## portal-observability-layer-1-9 | approved

### Task 1-9: Big-bang rewrite of all production `state.Logger` call sites to `log.For` + slog attrs

**Problem**: Every production call site still calls `state.Logger.{Debug,Info,Warn,Error}(component, fmt, args...)` with `fmt.Sprintf`-style format strings and `state.Component*` constants. These must be rewritten to `logger.{Debug,Info,Warn,Error}(msg, attrs...)` where `logger` is bound once per package via `log.For("<taxonomy-name>")`, `component` is the bound attr (not an arg), and the formatted values become slog attrs from the closed vocabulary. The logger-open sites (`state_common.openNoRotateLogger`, `state_daemon` rotate=true, `open.go` preview) and the `*state.Logger` `*Deps`/config fields must also be replaced with `*slog.Logger` from `log.For`/`log.Init`.

**Solution**: Rewrite every production call expression to the slog form, binding each package's component at package-init via `var logger = log.For("<component>")`, mapping the `state.Component*` constant to the literal taxonomy name, converting `fmt.Sprintf` interpolations into attrs from the closed 49-key vocabulary, and replacing the logger-open helpers / `*Deps` `*state.Logger` fields with `*slog.Logger`. This is the big-bang sweep — it lands together with Task 1-8's seam retyping in one PR.

**Outcome**: No production code calls `state.Logger` methods or references `state.Component*`; every package that logs binds `var logger = log.For("<component>")` once and calls `logger.{Level}(msg, attrs...)`; `grep "<component>:" portal.log` works for migrated subsystems at INFO.

**Do**:
- For each production package that logs, add `var logger = log.For("<component>")` at package scope. Map components from the existing `state.Component*` constants to the taxonomy literals: `ComponentDaemon→daemon`, `ComponentRestore→restore`, `ComponentHydrate→hydrate`, `ComponentNotify→notify`, `ComponentHooks→hooks`, `ComponentBootstrap→bootstrap`, `ComponentPreview→preview`. (`capture`/`saver`/`signal`/etc. are introduced where Phase 5/6 promote them; in Phase 1, keep the existing component for each migrated line — e.g. the daemon capture-loop lines stay under `daemon` until Phase 5 re-homes them to `capture`. Do NOT pre-introduce Phase 5/6 components.)
- Rewrite each call. Convert format-string interpolation into attrs from the closed vocabulary. Examples (mechanical, not exhaustive):
  - `deps.Logger.Warn(state.ComponentDaemon, "capture pane %s: %v", target, err)` → `logger.Warn("capture pane failed", "pane_key", target, "error", err)` (or keep `target` as the existing identifier; use `error` attr per the boundary-preservation convention — pass the wrapped `err` directly, not `err.Error()`).
  - `logger.Info(state.ComponentDaemon, "starting, version=%s, pid=%d", version, os.Getpid())` → this is the daemon's startup line; under the new model the OS-process boundary is owned by `process: start` (Phase 2). For Phase 1, keep an equivalent `daemon`-level line only if it carries info `process: start` won't (it does not — `version`/`pid` are baselines). Per the spec's Process/subsystem boundary, drop this redundant startup line; the `daemon: lock acquired` event (Phase 5) carries the unique `tmux_pane`. `[needs-info]`: Phase 5 owns `daemon: lock acquired`; in Phase 1 simply convert this line to a terse `daemon`-level INFO without re-deriving baselines (e.g. `logger.Info("starting")`) OR drop it — prefer converting to a minimal `logger.Info("starting")` so no log line silently disappears mid-migration, and leave a comment noting Phase 5 re-homes daemon lifecycle.
  - `logger.Warn(ComponentDaemon, "read @portal-restoring: %v", err)` → `logger.Warn("read @portal-restoring", "error", err)`.
  - `cfg.Logger.Warn(state.ComponentPreview, "select-window %q:%d failed: %v", session, window, err)` → `logger.Warn("select-window failed", "session", session, "error", err)` (drop the `%d` window into an existing attr only if one applies; `window` has no closed key in Phase 1 — render it inside the message phrase or omit. `[needs-info]`: there is no closed attr key for a bare window index in the contextual set; keep the window in the terse message or use `pane_key` if a structural key is available. Do NOT invent a `window` contextual key — note `windows` exists only as a cycle-summary count.)
- Convert `fmt.Sprintf`-in-message anti-patterns wholesale: never `logger.Info(fmt.Sprintf(...))`; always terse msg + attrs.
- **Logger-open sites**:
  - `cmd/state_common.go openNoRotateLogger()`: this returned a non-rotating `*state.Logger`. Under the new model there is no per-call logger open — the process-wide logger is configured once by `main`→`log.Init`. Replace `openNoRotateLogger` usages with `log.For("<component>")` package vars. Remove `openNoRotateLogger` (or reduce it to returning `log.For(...)` if a few call sites are simpler that way — but prefer package-level `var logger = log.For(...)`). Remove the `defer logger.Close()` calls at these sites (the process-wide logger is closed by `main` via `log.Close`, and `For` loggers are not closeable resources).
  - `cmd/state_daemon.go` `state.OpenLogger(..., true)` (rotate=true) and `defer logger.Close()`: remove the open; the daemon uses `log.For("daemon")`. (Rotation is now handler-owned — Phase 2. The daemon no longer opens its own rotating file.) The `daemonDeps.Logger *state.Logger` field → `*slog.Logger` set to `log.For("daemon")`.
  - `cmd/open.go` preview: `state.OpenLogger(...)`/`previewLogger=nil`/`defer previewLogger.Close()` → pass `log.For("preview")` into `NewPreviewAttachPipeline`; remove the open + nil-fallback + Close.
- **`*Deps`/config `*state.Logger` fields** → `*slog.Logger`: `daemonDeps.Logger`, `StateCleanupDeps.Logger`, the `state_hydrate.go cfg.Logger`, `state_signal_hydrate.go cfg.Logger`, the `state_commit_now.go` function-typed deps that take `logger *state.Logger` (e.g. `CaptureStructure`/`Commit` deps func types — already retyped at the source in Task 1-8; update the dep field/func-type signatures here), and `runMigrateRename(... logger *state.Logger)`. Each production construction site sets the field to `log.For("<component>")`.
- Update `internal/state` production call bodies (`capture.go`, `commit.go`, `scrollback.go`, `daemon_state.go`, `fifo_sweep.go`) to the slog form against the retyped `*slog.Logger` params from Task 1-8 — e.g. `logger.Warn(ComponentDaemon, "gc remove %s: %v", paneKey, err)` → `logger.Warn("gc remove failed", "pane_key", paneKey, "error", err)`.
- Do NOT introduce any new instrumentation beyond a faithful conversion of existing lines (no new cycle summaries, no lifecycle markers, no new components/attrs) — those are Phases 2-6. The level of each converted line stays as-is unless the existing level is plainly wrong under the level table; if so, leave a code comment but keep behaviour (level re-classification is the later phases' concern).
- `error` attr convention: pass the wrapped `err` directly (`"error", err`), never `err.Error()`.

**Acceptance Criteria**:
- [ ] No production (non-`_test.go`) file calls `state.Logger.{Debug,Info,Warn,Error}` or references `state.Component*`, `state.OpenLogger`, `state.NopLogger`, or `openNoRotateLogger`.
- [ ] Each migrated package binds `var logger = log.For("<component>")` once; call sites use `logger.{Level}(msg, attrs...)` with attr keys from the closed vocabulary.
- [ ] No `fmt.Sprintf` (or `%s`/`%v` format string) remains inside a log message argument; interpolated values are attrs.
- [ ] The daemon no longer opens a rotating `*state.Logger`; `daemonDeps.Logger` is `*slog.Logger = log.For("daemon")`; no `logger.Close()` is deferred at per-command logger-open sites.
- [ ] `open.go` passes `log.For("preview")` to `NewPreviewAttachPipeline`; the preview open + nil-fallback + Close are gone.
- [ ] All `*Deps`/config `*state.Logger` fields are `*slog.Logger`.
- [ ] `go build ./...` and `go test ./...` are green (with Task 1-8 and 1-10).

**Tests**:
- `"the daemon emits its capture-loop WARN under component=daemon with a pane_key attr"` (via `log.SetTestHandler` capture)
- `"the preview pipeline emits select-window WARN under component=preview"`
- `"the bootstrap orchestrator emits step-entry lines under component=bootstrap"`
- `"hooks-register migration emits its eviction lines under component=bootstrap"`
- `"no production source references state.Component* or state.OpenLogger"` (a grep-style guard test or a build-level assertion)
- `"the error attr carries the wrapped error chain, not the .Error() string"`

**Edge Cases**:
- `fmt.Sprintf`-in-message converted to attrs (prohibited pattern removed).
- `state.Component*` constants resolved to literal taxonomy names (Phase-1 components only: `daemon`/`restore`/`hydrate`/`notify`/`hooks`/`bootstrap`/`preview`; do NOT pre-introduce `capture`/`saver`/`signal`).
- Attr keys mapped to the closed vocabulary; window-index-style values with no closed key stay in the message phrase (do not invent keys).
- Logger-open sites (`state_common`/`state_daemon` rotate=true/`open.go` preview) replaced with package-level `log.For`; per-command `Close` defers removed.
- `error` attr passes the wrapped error directly.

**Context**:
> "All call sites of `state.Logger.{Debug,Info,Warn,Error}(component, fmt, args...)` are rewritten to `logger.{Debug,Info,Warn,Error}(msg, attrs...)`, component bound at package-init via `log.For`." (spec § Migration sweep)
>
> "Message string is a terse phrase; data lives in attrs: `logger.Info("ok", "pane_key", k, "took", d)` — never `logger.Info(fmt.Sprintf(...))`." (spec § Conventions)
>
> "The `"error"` attr value MUST be the wrapped error directly (`err`, not `err.Error()`); slog handles serialization." (spec § Diagnostic context preservation → slog attr usage)
>
> Closed component value space (15) and closed attr-key vocabulary (49) — spec § Subsystem prefix taxonomy. Phase 1 migrates existing lines under existing components only; new components/attrs are introduced by the later phases that own them.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § The `internal/log` package (Migration sweep, Consumer usage), § Subsystem prefix taxonomy (closed component + attr vocabularies), § Conventions; planning Phase 1 task 1-9

---

## portal-observability-layer-1-10 | approved

### Task 1-10: Delete the legacy `internal/state` logger and migrate its tests off `OpenLogger`/`NopLogger`

**Problem**: With all production call sites and intermediate seams migrated (Tasks 1-8/1-9), the legacy `internal/state/logger.go` — `Logger` type, `Level`/`LevelDebug..LevelError` constants, `Component*` constants, `OpenLogger`/`NopLogger`/`parseLevel`/`rotateIfOversized`/`maybeRotate`, `LogRotateThreshold`, and the pipe-delimited line format — is dead code, and its tests plus test helpers (`restoretest.OpenTestLogger`, `portaltest` log readers, tests asserting the old format or the WARN default) must be removed or retargeted at the new package.

**Solution**: Delete `internal/state/logger.go` wholesale and migrate every test that depended on it: tests asserting the pipe-delimited format are rewritten to assert the new slog text format (or deleted if redundant with `internal/log` tests), `NopLogger` usages become `slog.New(slog.NewTextHandler(io.Discard, nil))`, `restoretest.OpenTestLogger` is retargeted to return a `*slog.Logger` (or removed), `portaltest` log-read helpers are kept (they read the file) but updated for the new content, and tests depending on the LevelWarn default are updated to the INFO default.

**Outcome**: `internal/state/logger.go` no longer exists; `internal/state` exposes no logging type; `go build ./...` and `go test ./...` are green; no test references `state.Logger`, `state.OpenLogger`, `state.NopLogger`, `state.LevelWarn`, or the pipe format.

**Do**:
- Delete `internal/state/logger.go` in full (including `Level`, `LevelDebug`/`LevelInfo`/`LevelWarn`/`LevelError`, the `Component*` constants, `OpenLogger`, `NopLogger`, `parseLevel`, `openAppendLog`, `rotateIfOversized`, `maybeRotate`, `LogRotateThreshold`, and the `Logger` type with its methods). Confirm nothing in `internal/state` still references these after Tasks 1-8/1-9 (the retyped function params now take `*slog.Logger`).
- Delete or rewrite `internal/state/logger_test.go`: tests for `parseLevel`, the pipe format, rotation-at-1MiB, `NopLogger`, and the LevelWarn default are obsolete. Behaviour that still matters (level resolution, text rendering) is now tested in `internal/log` (Tasks 1-2/1-3). Delete the obsolete tests; do NOT port rotation tests (Phase 2 re-tests rotation against the new handler).
- `internal/restoretest/logger.go` (`OpenTestLogger`): retarget to return a `*slog.Logger`. Two options — pick the one matching how integration tests now obtain a logger:
  - If integration adapters (`FIFOSweeper`, `HookRegistrar`, `RestoreAdapter`) now take `*slog.Logger` (Task 1-8), `OpenTestLogger(t, stateDir)` should return a `*slog.Logger` that writes to `<stateDir>/portal.log` so the file-content assertions still work — e.g. construct an `slog.NewTextHandler` over an appended `*os.File` at that path, register `t.Cleanup` to close the file. Keep the same signature shape but change the return type.
  - Update `internal/restoretest/logger_test.go` accordingly.
- `internal/restoretest/doc.go`: update any reference to `*state.Logger`/`OpenLogger` in the package doc.
- `internal/portaltest/portal_log.go` (`ReadPortalLogSafe`): keep — it reads the file path; it does not depend on the logger type. Verify it still resolves the right path (now `portal.log`, written by the new handler). No change unless it referenced the legacy type.
- Sweep remaining test files that reference the legacy symbols (from the grep surface): `cmd/state_daemon_*_test.go`, `cmd/state_commit_now_test.go`, `cmd/state_cleanup_test.go`, `cmd/state_hydrate_test.go`, `cmd/state_signal_hydrate_test.go`, `cmd/state_notify_test.go`, `cmd/state_migrate_rename_test.go`, `cmd/run_hook_stale_cleanup_test.go`, `cmd/bootstrap_production_test.go`, `cmd/bootstrap/*_test.go`, `internal/restore/restore_test.go`, `internal/tui/preview_attach_test.go`, `internal/tmux/portal_saver_test.go`, `internal/tmux/hooks_register_test.go`, `internal/bootstrapadapter/adapters_test.go`, `internal/state/{capture,scrollback,fifo_sweep,commit,daemon_version_breadcrumb}_test.go`. For each:
  - Replace `state.NopLogger()` → `slog.New(slog.NewTextHandler(io.Discard, nil))` (or a shared test helper).
  - Replace any recording `MigrationLogger`/`BarrierLogger`/`bootstrap.Logger` fakes with a capture `slog.Handler` (e.g. set via `log.SetTestHandler`) or a buffer-backed `slog.NewTextHandler`, asserting on rendered content / attrs rather than `(component, format, args)` method calls.
  - Replace pipe-format assertions (`strings.Contains(line, " | WARN | daemon | ")`) with slog-text assertions (`WARN daemon:` prefix + attr substring) or attr-based assertions on captured records.
  - Replace `parseLevel`/`LevelWarn`-default assertions with the INFO-default contract.
- Ensure `internal/log` is NOT imported by `internal/state` in a way that creates a cycle (per Task 1-8's guard); if a test helper needs `log.For`, it lives in a `_test.go` or in `restoretest`/`portaltest`, not in `internal/state` production code.

**Acceptance Criteria**:
- [ ] `internal/state/logger.go` is deleted; `internal/state` exposes no `Logger`/`Level`/`Component*`/`OpenLogger`/`NopLogger`/`LogRotateThreshold` symbol.
- [ ] No `_test.go` or production file references `state.Logger`, `state.OpenLogger`, `state.NopLogger`, `state.Level*`, `state.Component*`, or `state.LogRotateThreshold`.
- [ ] `restoretest.OpenTestLogger` returns a `*slog.Logger` writing to `<stateDir>/portal.log`; its test passes.
- [ ] Tests that asserted the pipe-delimited format now assert the slog text format (or are deleted as redundant).
- [ ] Tests that depended on the LevelWarn default are updated to the INFO default.
- [ ] `go build ./...` and `go test ./...` are green.

**Tests**:
- `"restoretest.OpenTestLogger returns a *slog.Logger that writes to stateDir/portal.log"`
- `"no source file references the deleted state.Logger surface"` (grep-guard or compile-level)
- (Retargeted existing tests must pass — they are the regression surface for the migration.)

**Edge Cases**:
- Tests asserting on the old pipe format → rewritten to slog text format.
- `NopLogger` sentinel usages → `io.Discard` slog logger.
- `restoretest.OpenTestLogger` / `portaltest` log-read helpers retargeted (`OpenTestLogger` returns `*slog.Logger`; `ReadPortalLogSafe` unchanged).
- Tests depending on the LevelWarn default → updated to INFO default.

**Context**:
> "The `internal/state.Logger` type is **deleted**." … "The `internal/state.Component*` constants (`internal/state/logger.go:30-38`) are **deleted**." … "The pipe-delimited line format (`timestamp | level | component | message`) is **deleted**." … "`state.NopLogger()` is **deleted**; tests requiring a silent logger use `slog.New(slog.NewTextHandler(io.Discard, nil))` directly." (spec § Migration sweep)
>
> Production default INFO (changed from the historical WARN). (spec § Log-level discipline → Decision)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § The `internal/log` package (Migration sweep), § Log-level discipline (Decision); planning Phase 1 task 1-10
