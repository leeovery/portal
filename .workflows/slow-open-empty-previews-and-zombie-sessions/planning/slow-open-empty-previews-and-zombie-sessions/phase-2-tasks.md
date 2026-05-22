---
phase: 2
phase_name: Capture Pipeline Hardening (Component E)
total: 5
---

## slow-open-empty-previews-and-zombie-sessions-2-1 | approved

### Task 2.1: Introduce `tmux.ErrNoSuchSession` sentinel and wrap `ShowEnvironment` at the tmux boundary

**Problem**: The daemon needs to discriminate "session no longer exists" (natural churn) from any other per-session failure (anomalous) when iterating sessions. Today the only signal is the raw `"no such session"` substring in the tmux stderr; classifying via substring match inside the daemon couples the daemon to tmux's exact wording (not a stable contract) and scatters that coupling across every callsite. The spec mandates a typed sentinel exported once from `internal/tmux/` and consumed via `errors.Is` in the daemon layer.

**Solution**: Add an exported sentinel error `tmux.ErrNoSuchSession` in `internal/tmux/`. Update `(*tmux.Client).ShowEnvironment` so that when the underlying exec error wraps a `*CommandError` whose `Stderr` contains the case-sensitive substring `"no such session"`, the returned error joins / wraps `ErrNoSuchSession` such that `errors.Is(err, tmux.ErrNoSuchSession)` returns true. Wrap once at the package boundary; do not change any other tmux method in this task (other per-session calls can be wrapped lazily as they become discriminators in their own right — out of scope here unless required to compile).

**Outcome**: Callers of `ShowEnvironment` can write `if errors.Is(err, tmux.ErrNoSuchSession)` to recognise vanished sessions without substring matching, and any other non-zero exit continues to surface as a `*CommandError` for general fail-fatal handling. The sentinel is defined in exactly one place, exported, and documented.

**Do**:
- In `internal/tmux/tmux.go` (or a new sibling file `internal/tmux/errors.go` for cohesion with `command_error.go` / `ErrOptionNotFound`), declare `var ErrNoSuchSession = errors.New("no such session")`. Add godoc explaining the contract: returned by per-session tmux calls when stderr indicates the session is gone; discriminator via `errors.Is`.
- Define a small unexported helper, e.g. `func wrapNoSuchSession(err error) error`, that inspects `err` via `errors.As` for `*CommandError`, checks `strings.Contains(cmdErr.Stderr, "no such session")` (case-sensitive — match tmux's lowercase emission; document the rationale in a code comment), and on match returns `fmt.Errorf("%w: %w", ErrNoSuchSession, err)` (Go 1.20+ multi-`%w`) so both the sentinel and original `*CommandError` remain reachable. Non-match returns the input unchanged.
- Modify `(*Client).ShowEnvironment` at `internal/tmux/tmux.go:670` to call `wrapNoSuchSession` on the non-nil error path before returning. Preserve the existing `fmt.Errorf("failed to show environment for session %q: %w", session, err)` wrap — wrap the sentinel inside so the outer message remains.
- Do not change the happy-path return.

**Acceptance Criteria**:
- [ ] `tmux.ErrNoSuchSession` is exported, has a godoc, and is the single source of truth for the discriminator.
- [ ] `(*Client).ShowEnvironment("missing")` against a `Commander` whose `Run` returns `&CommandError{Stderr: "no such session: missing", Err: errors.New("exit status 1")}` yields an error for which `errors.Is(err, tmux.ErrNoSuchSession) == true`.
- [ ] `(*Client).ShowEnvironment("missing")` against a `Commander` returning a generic `errors.New("exec: \"tmux\": not found")` (no `*CommandError`) yields an error for which `errors.Is(err, tmux.ErrNoSuchSession) == false`.
- [ ] `(*Client).ShowEnvironment("missing")` against a `Commander` returning `&CommandError{Stderr: "", Err: errors.New("exit status 1")}` yields an error for which `errors.Is(err, tmux.ErrNoSuchSession) == false`.
- [ ] The original error remains reachable: `var ce *CommandError; errors.As(err, &ce)` still succeeds when the underlying cause was a `*CommandError`.
- [ ] No callers outside `internal/tmux/` perform substring matching on the `"no such session"` literal as part of this change (audit limited to call sites of `ShowEnvironment`).

**Tests**:
- `"it returns an error matching ErrNoSuchSession when stderr contains 'no such session'"`
- `"it does not match ErrNoSuchSession when stderr is empty"`
- `"it does not match ErrNoSuchSession for a non-CommandError exec failure"`
- `"it preserves *CommandError recoverability via errors.As"`
- `"it does not match ErrNoSuchSession for mixed-case 'No such session'"` — documents the case-sensitive contract; if tmux ever emits mixed case, the substring constant is the one place to revisit.
- `"it does not match ErrNoSuchSession for unrelated non-zero exits (e.g. 'duplicate session')"`

**Edge Cases**:
- Case sensitivity: tmux emits lowercase `"no such session"`; case-sensitive `strings.Contains` is intentional. Document in code comment; mixed-case is treated as anomalous.
- Already-wrapped errors: `ShowEnvironment` wraps with `fmt.Errorf("failed to show environment ... %w", err)`; the sentinel must remain reachable through that outer wrap (Go's `errors.Is` walks the chain).
- Empty stderr (EOF / exec didn't capture): no match — sentinel not added.
- Non-`*CommandError` underlying error (e.g. PATH lookup failure via `*exec.Error`): no match — sentinel not added.

**Context**:
> From specification (Component E):
> "Classification uses a typed sentinel `tmux.ErrNoSuchSession` introduced in `internal/tmux/` and returned by `ShowEnvironment` (and any other per-session tmux call) when stderr contains `"no such session"`. The wrapping happens once at the `internal/tmux/` boundary; daemon-layer callers classify via `errors.Is(err, tmux.ErrNoSuchSession)`. Substring matching in the daemon layer is rejected — it couples the daemon to tmux's exact error-string surface, which is not a stable contract."
>
> `*CommandError` already exists (`internal/tmux/command_error.go`) with a `Stderr` field — the helper for matching is straightforward. Pattern parallels `optionAbsentStderrPatterns` at `internal/tmux/tmux.go:23` (the established convention for tmux-stderr discriminators).

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` (Component E, lines 281–342)

## slow-open-empty-previews-and-zombie-sessions-2-2 | approved

### Task 2.2: Thread `*state.Logger` parameter into `CaptureStructure` (no behaviour change)

**Problem**: `state.CaptureStructure` currently has signature `CaptureStructure(c CaptureClient, skipSet map[string]struct{}, prev *Index) (Index, error)` and has no way to emit a WARN line for a per-session skip. The spec requires per-session errors to be logged under `ComponentDaemon` once log-and-continue lands in Task 2.3; the logger has to be available before the behavioural change can be made, so a no-behaviour-change signature plumb is sequenced first to keep each TDD cycle small and isolate "signature churn" from "behaviour change" diffs.

**Solution**: Add a `logger *Logger` parameter to `CaptureStructure` (the spec-preferred option, "symmetry with `Commit`'s existing logger argument"). Treat a `nil` logger as a no-op (existing `*Logger` contract already supports this — see `internal/state/logger.go:59`). Update every call site (production daemon and tests) to pass a logger explicitly — production passes `deps.Logger`; tests pass `nil` or `state.NopLogger()` (whichever already exists in the codebase). No behavioural change in this task — the new parameter is accepted and stored locally but no `logger.Warn` call is added yet (that arrives in Task 2.3).

**Outcome**: `CaptureStructure` accepts a `*state.Logger`; all call sites compile and existing tests pass unchanged in behaviour; no new log lines emitted yet.

**Do**:
- Change the signature in `internal/state/capture.go:62` to `func CaptureStructure(c CaptureClient, skipSet map[string]struct{}, prev *Index, logger *Logger) (Index, error)`. Update the godoc immediately above to document the logger param (nil → no-op; reserved for per-session WARN entries added in a follow-up).
- Inside the function body, do not add any `logger.Warn` calls in this task — the parameter is accepted but unused beyond a `_ = logger` line (or simply held for the next task). Keep the diff minimal.
- Update the production call site in `cmd/state_daemon.go:149` (`state.CaptureStructure(deps.Client, skipSet, deps.PrevIndex)`) to pass `deps.Logger` as the fourth argument.
- Update all call sites in `internal/state/capture_test.go` (eight or so invocations — `state.CaptureStructure(client, nil, nil)`) to pass `nil` as the fourth argument. Verify by `go build ./...` succeeding.
- Grep for any additional call sites: `grep -rn 'state.CaptureStructure(' .` — fix any other production or test consumer.

**Acceptance Criteria**:
- [ ] `CaptureStructure` signature accepts `logger *state.Logger` as the trailing parameter.
- [ ] `go build ./...` passes after the signature change and all call site updates.
- [ ] `go test ./internal/state/... ./cmd/...` passes — no behavioural regressions (the parameter is currently inert).
- [ ] The godoc on `CaptureStructure` documents the new parameter, its nil-tolerance, and the forthcoming WARN-on-per-session-error usage.
- [ ] The daemon callsite at `cmd/state_daemon.go:149` passes `deps.Logger` (not a freshly constructed logger).

**Tests**:

This is a deliberate refactor cycle (signature plumbing) with no behavioural change and therefore no own-test addition. The existing CaptureStructure unit suite at `internal/state/capture_test.go` serves as the regression backstop: it must continue to pass after the signature update, with every test invocation passing `nil` as the trailing `*state.Logger`. `go build ./...` covers the type-level assertion that `CaptureStructure` accepts `*state.Logger` as its new trailing parameter. The behavioural assertion (per-session WARN on error) is owned by Task 2.3.

- `"existing CaptureStructure unit tests continue to pass when invoked with a nil logger"` — regression backstop via the existing test suite.
- `"CaptureStructure compiles with a *state.Logger argument"` — type-level assertion via `go build`.

**Edge Cases**:
- Nil logger guard: the existing `*Logger` methods early-return on `nil` receiver (`internal/state/logger.go:59`: "A nil *Logger is a valid no-op"). No new nil-check is added inside `CaptureStructure`.
- Test fixtures: `internal/state/capture_test.go` calls `state.CaptureStructure(client, nil, nil)` in ~8 places; each must be updated to `state.CaptureStructure(client, nil, nil, nil)`.
- Out-of-tree consumers: there are no production callers outside `cmd/state_daemon.go` (verified by grep). If a missed callsite surfaces, fix it as a mechanical edit; do not expand scope.
- The `restoretest` / integration helpers should not call `CaptureStructure` directly. Confirm via grep; if any do, update them.

**Context**:
> From specification (Component E):
> "`CaptureStructure` does not currently take a logger argument. To preserve the existing call-site signature without intrusive changes, the spec accepts either of the following implementation choices (planning phase decides): Add an optional `logger *Logger` parameter ... The first option is preferred for symmetry with `Commit`'s existing logger argument."
>
> The planning phase has selected option 1 (parameter). `Commit` already takes a `*state.Logger` (`internal/state/commit.go`); the new shape matches that convention. See also `cmd/state_daemon.go:25` where `daemonDeps.Logger` is the canonical handle.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` (Component E, lines 326–331)

## slow-open-empty-previews-and-zombie-sessions-2-3 | approved

### Task 2.3: Replace abort-on-error with per-session log-and-continue plus natural-churn discriminator

**Problem**: The per-session loop in `CaptureStructure` (`internal/state/capture.go:86-96`) aborts the entire capture on the first `ShowEnvironment` error, dropping every alphabetically-later session's capture for that tick. The spec's reporter scenario shows this amplifies the GC race — one transient per-session failure poisons the whole tick and produces empty previews for surviving sessions. The fix is per-session log-and-continue, paired with a post-loop discriminator that distinguishes "user killed every session mid-tick" (proceed with empty index) from "tmux broke for every session" (return error so `captureAndCommit` skips Commit + GC and preserves scrollback).

**Solution**: Rewrite the per-session loop to skip-and-log on error (mirroring the per-pane defensive pattern at `cmd/state_daemon.go:185-192`). Classify each error using `errors.Is(err, tmux.ErrNoSuchSession)` from Task 2.1. After the loop, if `len(keep) > 0 && len(sessions) == 0`, apply the discriminator: all-natural-churn → return the empty index with `nil` error; any-anomalous → return the empty index with a wrapped error carrying the count and reachable sentinel info, so `captureAndCommit` propagates the error and skips Commit.

**Outcome**: A single failing session no longer poisons the tick — surviving sessions are captured and committed. Total enumeration failure caused by genuine churn (user killed last sessions) writes an empty `sessions.json` and reclaims orphan scrollback. Total enumeration failure caused by anomalous errors leaves the prior `sessions.json` and scrollback intact.

**Do**:
- In `internal/state/capture.go`, replace lines 85–96 with a loop that:
  - Tracks `var anomalousErrs []error` and `naturalChurnCount int` alongside the existing `sessions` slice.
  - For each `name` in `sortedKeys(keep)`: call `envRaw, err := c.ShowEnvironment(name)`. On non-nil `err`:
    - If `errors.Is(err, tmux.ErrNoSuchSession)`: increment `naturalChurnCount`, log via `logger.Warn(ComponentDaemon, "show environment: session vanished mid-tick", "session", name, "err", err)` (use the existing `Logger.Warn(component, format, args...)` signature — adapt the call to whatever printf-style or key-value style the existing logger uses; match the convention used at `internal/state/commit.go:53`), and `continue`.
    - Otherwise: append to `anomalousErrs`, log `logger.Warn(ComponentDaemon, "show environment: anomalous error for session %q: %v", name, err)`, and `continue`.
  - On success: build and append the `Session` exactly as today.
- After the loop, add the discriminator: `if len(keep) > 0 && len(sessions) == 0 { if len(anomalousErrs) == 0 { /* all natural churn — proceed */ } else { return empty, fmt.Errorf("capture structure: all %d sessions failed enumeration (%d anomalous, %d natural churn): %w", len(keep), len(anomalousErrs), naturalChurnCount, errors.Join(anomalousErrs...)) } }`. The mixed case (some sessions succeeded, some failed) does NOT trigger the discriminator — the partial-success result is returned as-is, with the per-session WARN entries serving as the diagnostic trail.
- Verify `parseShowEnvironment` is unchanged (already returns non-nil empty map on empty input — see `internal/state/capture.go:411`).
- Verify the existing `skipSet`/`prev` merge logic at line 100 is unchanged and continues to run on the partial `idx`.

**Acceptance Criteria**:
- [ ] Single-session failure does not abort the tick: with `ShowEnvironment` failing for "A" and succeeding for "B"/"C", the returned index contains exactly Sessions for B and C in canonical order, and the returned error is nil.
- [ ] All-natural-churn proceeds: with `ShowEnvironment` returning a `tmux.ErrNoSuchSession`-matching error for every session in a non-empty `keep`, the returned index has `len(Sessions) == 0` and error is nil; downstream `captureAndCommit` will Commit an empty `sessions.json`.
- [ ] All-anomalous aborts: with `ShowEnvironment` returning a non-sentinel error for every session in a non-empty `keep`, the returned error is non-nil and wraps the underlying errors via `errors.Join`; `captureAndCommit` will skip Commit + GC.
- [ ] Mixed natural-churn + anomalous in a tick where at least one session succeeded: the partial index is returned with nil error (the discriminator gates only on `len(sessions) == 0`).
- [ ] Mixed natural-churn + anomalous in a tick where no session succeeded: the discriminator treats this as anomalous (any anomalous in the failure set → abort tick).
- [ ] Empty `keep` short-circuit: when `len(keep) == 0`, the function returns the empty index with nil error and never enters the per-session loop (existing behaviour preserved).
- [ ] Every per-session skip emits exactly one WARN log line under `ComponentDaemon` carrying the session name and underlying error.
- [ ] Pre-loop calls (`ListSessionNames`, `ListAllPanesWithFormat`, `parsePaneRows`) remain unchanged — see Task 2.4 for the explicit regression test.

**Tests**:
- `"it skips a failing session and captures the survivors"` — A fails, B/C succeed; assert idx has B and C, error is nil.
- `"it proceeds with empty index when every session is natural churn"` — all `ShowEnvironment` return `tmux.ErrNoSuchSession`; assert idx.Sessions is empty, error is nil.
- `"it returns an error when every session fails with anomalous errors"` — all return non-sentinel errors; assert error is non-nil and `errors.Join`-style — at least one underlying error is reachable via `errors.Is` against a sentinel chosen for the test, OR the error message names the count.
- `"it returns an error when the failure set is mixed natural-churn + anomalous and no session succeeded"` — assert error is non-nil.
- `"it returns nil error and partial index when some sessions succeed despite mixed failures"` — assert non-empty idx.Sessions and nil error.
- `"it emits a WARN log entry per failing session naming the session and error"` — drive a real `*state.Logger` against a temp file; grep the log for `session=A`, `session=D`, etc.
- `"it does not invoke the per-session loop when keep is empty"` — set list-sessions to return only internal-prefixed names; assert no `show-environment` calls were made on the mock.
- `"it preserves canonical ordering of surviving sessions"` — sessions returned successfully are sorted ascending by name.

**Edge Cases**:
- Mixed errors: one anomalous + many natural-churn in a tick where no session succeeded → discriminator treats as anomalous (errs on the side of preserving scrollback, per spec line 324).
- All sessions succeed: discriminator condition `len(sessions) == 0` is false; the post-loop branch is never entered.
- `parseShowEnvironment("")` already returns a non-nil empty map; on the success path with an empty environment, the resulting `Session.Environment` is a non-nil empty map — preserve.
- Logger is nil: existing `*Logger` methods early-return; per-session skips produce no log line but the loop still continues correctly.
- A session whose name contains spaces or non-ASCII bytes: the log entry must quote / format the name safely (use `%q` when interpolating into format strings).

**Context**:
> From specification (Component E, lines 297–324):
> "Mirror the per-pane defensive pattern already used in `captureAndCommit` (`cmd/state_daemon.go:185-192`). For each session, attempt `ShowEnvironment`; on per-session error, log WARN and skip that session; continue to the next."
>
> "**Per-session error classification.** During the loop, classify each `ShowEnvironment` error as either `natural-churn` (the session no longer exists) or `anomalous` (any other failure)."
>
> "**Post-loop discriminator.** If `len(keep) > 0 && len(sessions) == 0`: **If all per-session errors were `natural-churn`:** ... proceed with the empty index ... **If any per-session error was `anomalous`:** ... Return an error wrapping the count and types, causing `captureAndCommit` to skip Commit + GC for this tick (the existing error path) — refuse to wipe scrollback on evidence of a broken capture."
>
> "The natural-churn predicate must be conservative: any error that isn't unambiguously 'session no longer exists' is treated as anomalous."

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` (Component E, lines 281–342)

## slow-open-empty-previews-and-zombie-sessions-2-4 | approved

### Task 2.4: Lock in fail-fatal pre-loop regression coverage

**Problem**: The spec is explicit that pre-loop calls (`ListSessionNames`, `ListAllPanesWithFormat`, `parsePaneRows`) remain fail-fatal — only the per-session loop becomes log-and-continue. A future refactor of `CaptureStructure` could accidentally apply the same defensive pattern to the pre-loop calls, silently masking a tmux outage and producing destructive empty commits. Without explicit regression tests the invariant is undefended.

**Solution**: Add focused unit tests that exercise each pre-loop failure mode and assert `CaptureStructure` returns a non-nil error with an empty `Sessions` slice, so the call chain in `captureAndCommit` (`cmd/state_daemon.go:149-152`) propagates the error and skips Commit. These tests sit alongside the new per-session tests from Task 2.3 in `internal/state/capture_test.go` and complement (do not duplicate) any existing pre-loop coverage already present in that file.

**Outcome**: The pre-loop fail-fatal invariant is locked in by tests. Any future change that turns these branches into log-and-continue trips a red test.

**Do**:
- In `internal/state/capture_test.go`, add three tests under the existing `TestCaptureStructure` table (or as a new sibling `TestCaptureStructure_PreLoopFailFatal`):
  - **`ListSessionNames` error**: `captureMock.listSessionsE = errors.New("exec: tmux broken")`. Call `state.CaptureStructure(client, nil, nil, nil)`. Assert err != nil, `idx.Sessions == nil || len(idx.Sessions) == 0`, and that no `show-environment` or `list-panes` calls reached the mock.
  - **`ListAllPanesWithFormat` error** with non-empty `keep`: `listSessions` returns one or two non-internal names, `listPanesE = errors.New("list-panes failed")`. Assert err != nil, no Sessions, no `show-environment` calls.
  - **`parsePaneRows` failure (malformed row)** with non-empty `keep`: `listSessions` returns "work"; `listPanes` returns a malformed row missing fields (e.g. `"work|||0|||main"` — wrong field count). Assert err != nil (the existing `parsePaneRow` returns `fmt.Errorf("unexpected pane row field count ...")`), and no `show-environment` calls.
- Add a fourth test demonstrating the benign empty-keep path: `listSessions` returns only internal-prefix names (`_portal-saver|1|0`); assert err == nil, `len(idx.Sessions) == 0`, and crucially no `list-panes` or `show-environment` calls were made (the empty-keep short-circuit at line 74 is preserved).
- For each test, configure the `captureMock` to fatal-fail (`m.t.Fatalf`) on any unexpected command — the existing dispatcher already does this for the `default` case; the new tests use the same hook to catch a regression where, e.g., `show-environment` is called despite a pre-loop error.
- If the existing test suite already covers some of these cases (audit by reading `capture_test.go`), do not duplicate — extend or rename. The goal is one explicit assertion per pre-loop failure mode.

**Acceptance Criteria**:
- [ ] A test fails if `CaptureStructure` is modified to log-and-continue on a `ListSessionNames` failure.
- [ ] A test fails if `CaptureStructure` is modified to log-and-continue on a `ListAllPanesWithFormat` failure.
- [ ] A test fails if `CaptureStructure` is modified to log-and-continue on a `parsePaneRows` (malformed row) failure.
- [ ] A test asserts that with empty `keep`, no pane-list call is made and no error is returned.
- [ ] All new tests run against `state.CaptureStructure(client, nil, nil, nil)` (matches the Task 2.2 signature).
- [ ] No new test depends on the per-session log-and-continue change from Task 2.3 — these tests are intentionally orthogonal and pass even if 2.3 is reverted.

**Tests**:
- `"it returns an error when ListSessionNames fails and does not call show-environment"`
- `"it returns an error when ListAllPanesWithFormat fails with non-empty keep"`
- `"it returns an error when parsePaneRows hits a malformed row"`
- `"it returns an empty index with nil error when keep is empty after filtering"`

**Edge Cases**:
- Malformed pane row vs. tmux exec failure: both are pre-loop fail-fatal but exercise different code paths (parse-error inside `parsePaneRows` vs. exec-error from `ListAllPanesWithFormat`). Cover both.
- Partial pane output (one valid row, one malformed): `parsePaneRows` returns an error on the malformed row before grouping completes — confirm the test setup produces this shape (one good line + one bad line in `listPanes`).
- Non-empty `keep` but pane list returns empty string: existing behaviour returns nil error with `grouped` populated as an empty map; verify this is the existing happy path, NOT one of the fail-fatal tests.
- Empty `keep` skipping pre-loop pane fetch: must not invoke `list-panes` on the mock — assert via the dispatch table's fatal-on-unexpected.

**Context**:
> From specification (Component E, line 315):
> "Pre-loop calls remain fail-fatal. `ListSessionNames`, `ListAllPanesWithFormat`, and `parsePaneRows` (lines 66-83) are NOT changed — those failures indicate tmux itself is broken or returning malformed output, and continuing with partial state would produce destructive commits. The per-session loop is the only path where partial-success is meaningful."
>
> Also from acceptance criteria (line 339):
> "No regression in fail-fatal pre-loop paths. A `ListAllPanesWithFormat` failure still causes `CaptureStructure` to return an error; `captureAndCommit` does not Commit. Verified by existing or new unit test."

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` (Component E, lines 315, 339)

## slow-open-empty-previews-and-zombie-sessions-2-5 | approved

### Task 2.5: Wire daemon call site to pass real `ComponentDaemon` logger and assert log delivery

**Problem**: Task 2.2 plumbed the logger parameter through `CaptureStructure` and Task 2.3 added the WARN calls, but production-wiring correctness (the daemon passes its real `*state.Logger`, not nil; entries flow to the daemon's `portal.log`) is only weakly verified by the existing tests. The reporter symptom analysis in the spec depends on these log entries being available to operators diagnosing future incidents — if the daemon were ever to pass `nil` here (e.g. during the first tick before logger init, or via a refactor), the per-session diagnostic trail would silently disappear.

**Solution**: Confirm the daemon call site at `cmd/state_daemon.go:149` passes `deps.Logger`, then add a daemon-layer test that drives one full tick with a stub `*tmux.Client` whose `ShowEnvironment` is set up to fail for one session — assert the daemon's logger (a real `*state.Logger` writing to a temp file) records the WARN line under the component `daemon` with the session name. This locks in end-to-end log delivery from the daemon down through `CaptureStructure`.

**Outcome**: `cmd/state_daemon_test.go` (or a sibling) contains a test that, given a daemon-shaped scenario with one failing session, asserts the produced `portal.log` file contains a line like `... | WARN | daemon | ... session=A ...`. Any regression that disconnects the logger from `CaptureStructure` (e.g. passing `nil` at the call site) trips this test.

**Do**:
- Verify (and adjust if Task 2.2 missed it) that `cmd/state_daemon.go:149` reads `state.CaptureStructure(deps.Client, skipSet, deps.PrevIndex, deps.Logger)` — not `nil`, not a freshly-opened logger.
- Add a test to `cmd/state_daemon_test.go` (matching the existing test style — no `t.Parallel()`, mocks via package-level `*Deps` if needed):
  - Build a temp dir, open a real `*state.Logger` via `state.OpenLogger(filepath.Join(tempDir, "portal.log"), state.LevelWarn, false)` (or whatever the existing constructor signature is — match Task 2.2's discovery).
  - Build a `daemonDeps` whose `Client` is wired through a `tmux.NewClient(mock)` where the mock returns one valid session ("A") and one failing session ("B" returns an anomalous (non-sentinel) `ShowEnvironment` error).
  - Either drive one `tick` invocation directly (preferred — `tick` is package-private and accessible from `cmd/state_daemon_test.go`) or call `captureAndCommit` directly with `deps.Logger` set.
  - Read the log file after the tick. Assert it contains one line matching the regex / substring expectations: contains ` | WARN | `, contains `daemon`, contains the failing session's name (e.g. `B` or `session=B`), and contains some portion of the underlying error string.
- Add a second test asserting the all-natural-churn case still produces WARN entries (one per session) — verifies that natural-churn skips still log, not just anomalous ones.
- Verify (via direct inspection or a dedicated assertion) that during an all-natural-churn tick, `Commit` is still invoked with an empty index — i.e. log emission does not gate Commit.

**Acceptance Criteria**:
- [ ] `cmd/state_daemon.go:149` passes `deps.Logger` to `state.CaptureStructure`.
- [ ] A new test in `cmd/state_daemon_test.go` drives a tick with one failing session and asserts the daemon's `portal.log` contains a WARN line under `daemon` naming the failing session.
- [ ] A second test drives an all-natural-churn tick and asserts (a) one WARN per session, (b) `Commit` invoked exactly once writing an empty `sessions.json`.
- [ ] Both tests use `portaltest.NewIsolatedStateEnv` (Phase 1 deliverable) or otherwise scope all state writes to a per-test temp dir — no mutation of the developer's `~/.config/portal/state/`.
- [ ] No `t.Parallel()` in any new test (cmd package convention — see CLAUDE.md "Tests must not use `t.Parallel()`").
- [ ] If the daemon initialisation creates the logger after the first tick somehow (verify, do not assume), document the ordering with a code comment so subsequent edits don't introduce a tick-zero nil logger.

**Tests**:
- `"daemon tick logs WARN naming the session when one ShowEnvironment fails anomalously"`
- `"daemon tick logs WARN per session when every ShowEnvironment fails natural-churn and still commits empty"`
- `"daemon tick passes deps.Logger to CaptureStructure (not nil)"` — implicit via the above; if the daemon ever passed `nil`, the first two tests' log assertions would fail.
- `"daemon tick logs nothing extra when CaptureStructure succeeds with no failures"` — ensures no spurious WARN noise on the happy path.

**Edge Cases**:
- Logger not yet initialised at first tick: verify by reading the daemon's startup ordering in `cmd/state_daemon.go` — `daemonDeps.Logger` must be populated before `daemonRunFunc` runs. If there's any ordering hazard, lock it in with an assertion or panic at daemon startup; otherwise document.
- Empty session name in the log entry: should never occur (`keep` is built from non-empty names) but if it does, the WARN must still render unambiguously — `%q` formatting handles empty strings safely.
- Log level filtering: production opens the logger at `LevelWarn` by default; verify the test logger is opened at a level that admits WARN (the default).
- Log file rotation during the tick: not realistic in unit-test scale (well under 1 MiB threshold per `internal/state/logger.go:44`), but worth a single-line comment in the test asserting the log path is the one the assertion reads.
- All-natural-churn tick: the empty Commit happens; verify the log assertion does not inadvertently rely on Commit being skipped.

**Context**:
> From specification (Component E, acceptance criteria, line 338):
> "Every per-session skip emits a WARN log entry with the session name and the underlying error. The log uses the existing `ComponentDaemon` constant from `internal/state/logger.go` (matching the convention used by `gcOrphanScrollback` in `internal/state/commit.go:53` for capture-pipeline failures). A new component constant is NOT introduced. Verified by unit test that asserts on the logger output."
>
> The daemon's logger is held on `daemonDeps.Logger` (`cmd/state_daemon.go:25`); the call site to update is at line 149. Existing daemon tests in `cmd/state_daemon_test.go` already use temp-dir patterns and mockable `Client` plumbing — match that style.

**Spec Reference**: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md` (Component E, line 338)
