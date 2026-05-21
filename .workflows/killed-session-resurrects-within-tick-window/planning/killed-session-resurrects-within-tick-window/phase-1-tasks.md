---
phase: 1
phase_name: Synchronous Commit On Kill
total: 7
---

## killed-session-resurrects-within-tick-window-1-1 | approved

### Task 1-1: Add `portal state commit-now` happy-path subcommand

**Problem**: `sessions.json` is rewritten eventually by the daemon's 1s ticker. On every kill path, the file lags by `[0, ticker.period + per-tick wall time]` (field-measured 3.9–5s on heavy scrollback profiles), so a subsequent bootstrap step 5 `Restore` resurrects the just-killed session. There is no synchronous writer available to be wired into the `session-closed` hook.

**Solution**: Introduce a new sibling subcommand `portal state commit-now` that captures the live structural index via `state.CaptureStructure` (no scrollback content, no hash work) and atomically rewrites `sessions.json` via `state.Commit(dir, idx, anyScrollbackChanged=false, logger)`. `PrevIndex` is sourced from disk via `state.ReadIndex` so scrollback-hash and per-pane content fields on live sessions are preserved verbatim; dead sessions drop out via `mergeSkippedPanes`'s live-structure rule. A missing or undecodable `sessions.json` falls back to a zero-value `PrevIndex` and logs WARN under the daemon's `Component` constant.

**Outcome**: Running `portal state commit-now` against a live tmux server produces an atomic `sessions.json` rewrite reflecting the current set of live, non-underscore-prefixed sessions, with live sessions' scrollback-hash fields preserved from the prior on-disk index. Underscore-prefixed sessions (e.g., `_portal-saver`) are omitted by the existing `keepSessionNames` filter in `state.CaptureStructure`. The subcommand exits 0 on success and writes no `.bin` files.

**Do**:
- Create `cmd/state_commit_now.go` (sibling to `cmd/state_notify.go`) defining a Cobra subcommand `commit-now` registered under the existing `state` parent command.
- Wire it into the existing `state` command tree in `cmd/state.go` (or wherever `state notify` is wired today — match that idiom exactly).
- In the `RunE` body, resolve the state directory via the existing path helper used by `state notify` / the daemon (do not duplicate path resolution).
- Read existing `sessions.json` via `state.ReadIndex(stateDir)`. On any error (ENOENT or decode failure), proceed with a zero-value `state.Index` as `PrevIndex` and emit a WARN-level structured log event via the state logger using the same `Component` constant the daemon uses for `sessions.json` captures.
- Construct a `*tmux.Client` (matching how `cmd/state_daemon.go` or other state subcommands construct one) and call `state.CaptureStructure(ctx, client, prevIndex)` to build the new `state.Index`.
- Call `state.Commit(stateDir, idx, /*anyScrollbackChanged=*/false, logger)` to atomically rewrite `sessions.json`.
- Expose a small package-level `commitNowDeps` struct (mirroring `bootstrapDeps`/`openDeps` pattern) so unit tests can inject mock `ReadIndex`, `CaptureStructure`, `Commit`, and tmux client implementations.
- Do NOT touch `save.requested` on this success path (per `save.requested` Discipline — successful sync commits are silent to the daemon).
- Do NOT write any `.bin` files; do NOT call any hash routines; do NOT touch markers.
- The `@portal-restoring` short-circuit and failure-path `save.requested` touch are out of scope here — they land in tasks 1-2 and 1-3 respectively. This task implements the bare happy path and the `PrevIndex` fallback only.

**Acceptance Criteria**:
- [ ] `portal state commit-now` is a registered Cobra subcommand under `portal state`, parallel to `portal state notify`.
- [ ] With one or more live tmux sessions and a fresh state directory (no pre-existing `sessions.json`), invoking `commit-now` writes `sessions.json` containing exactly the live, non-underscore-prefixed sessions; exit code 0.
- [ ] With a pre-existing `sessions.json` containing scrollback-hash and per-pane content fields for live sessions, the post-`commit-now` file preserves those fields verbatim on the still-live sessions.
- [ ] When `state.ReadIndex` returns an error (file missing or corrupt), `commit-now` proceeds with a zero-value `PrevIndex`, emits a WARN log event, and still produces a valid `sessions.json` containing the live sessions; exit code 0.
- [ ] Sessions whose names begin with `_` (e.g., `_portal-saver`) are absent from the resulting `sessions.json` via the existing `keepSessionNames` filter — `commit-now` does not re-implement or bypass this filter.
- [ ] No `.bin` files are written; no `gcOrphanScrollback` runs; no markers are touched.
- [ ] `save.requested` is not touched on the successful-commit exit path.
- [ ] Package-level `commitNowDeps` exists for test injection following the existing `cmd` package DI idiom; tests can swap `ReadIndex`, `CaptureStructure`, `Commit`, and the tmux client constructor.

**Tests** (`cmd/state_commit_now_test.go`, no `t.Parallel()`):
- `"it writes sessions.json with the expected sessions when tmux reports zero live sessions"`
- `"it writes sessions.json with a single session including windows and panes"`
- `"it writes sessions.json for a session with multiple windows and multiple panes"`
- `"it preserves scrollback-hash and per-pane content fields on live sessions when PrevIndex is supplied"`
- `"it omits underscore-prefixed sessions via the keepSessionNames filter"`
- `"it falls back to a zero-value PrevIndex and logs WARN when ReadIndex returns ENOENT"`
- `"it falls back to a zero-value PrevIndex and logs WARN when ReadIndex returns a decode error"`
- `"it does not touch save.requested on a successful commit"`
- `"it exits 0 on success"`

**Edge Cases**:
- Zero live sessions: `sessions.json` is rewritten with an empty session set (this is correct — the daemon does the same).
- Underscore-prefixed sessions present: filtered by `keepSessionNames` inside `state.CaptureStructure`. No new filter logic in `commit-now`.
- Missing `sessions.json` (first-ever invocation): zero-value `PrevIndex` fallback + WARN log; subsequent daemon tick repopulates scrollback-hash fields.
- Corrupt `sessions.json` (decode error from a partial write or external mutation): same fallback as ENOENT.
- Multi-window, multi-pane session: `CaptureStructure` enumerates all panes; `commit-now` does no per-pane work beyond what `CaptureStructure` already performs.

**Context**:
> Per spec § Mechanism: "This path uses only existing primitives. `state.CaptureStructure` already takes a `CaptureClient` interface satisfied by `*tmux.Client` — it is not daemon-exclusive. `state.Commit` is already the atomic-write primitive used by the daemon and is structurally available to any caller."
>
> Per spec § Mechanism step 1: "if `sessions.json` does not exist yet … **or** if the file exists but cannot be decoded …, `commit-now` proceeds with a zero-value `PrevIndex`, logs a WARN-level event via the state logger …, and completes the structural commit normally. A `ReadIndex` failure is **not** treated as a `commit-now` failure exit."
>
> Per spec § Logging Discipline: "`commit-now` uses **the same component constant the daemon uses for `sessions.json` captures**. … if it is named `ComponentDaemon` today, `commit-now` adopts it as-is."
>
> Per spec § `save.requested` Discipline: "Successful sync commit (exit 0): `sessions.json` is already current; no daemon work is needed. `save.requested` is **not** touched (avoids a redundant daemon tick)."

**Spec Reference**: `.workflows/killed-session-resurrects-within-tick-window/specification/killed-session-resurrects-within-tick-window/specification.md` § Fix Approach / Mechanism, § Entry-Point Design Decision, § Logging Discipline, § `save.requested` Discipline.

---

## killed-session-resurrects-within-tick-window-1-2 | approved

### Task 1-2: Add `@portal-restoring` short-circuit to `commit-now`

**Problem**: Bootstrap step 5 `Restore` deliberately sets `@portal-restoring` on the tmux server to suppress writers from racing reconstruction. During bootstrap step 4, `_portal-saver` version-upgrade kills can fire `session-closed` while restoration is still in progress. A synchronous commit at that moment would capture a partial skeleton view and corrupt the in-flight restore. The daemon's `tick()` already honours this marker; `commit-now` must adopt identical discipline.

**Solution**: Before doing any structural work in `commit-now`, query the `@portal-restoring` server option. If set, log an INFO-level skip event and return without invoking `CaptureStructure` or `Commit`. As mandated by the `save.requested` Discipline, the short-circuit also touches `save.requested` on the way out (best-effort) so the daemon's first post-restoration tick is guaranteed to commit rather than waiting up to 30s for the `gap` rule. Exit code remains 0 — the short-circuit is a deliberate skip, not an error.

**Outcome**: With `@portal-restoring` set, invoking `commit-now` leaves `sessions.json` byte-identical to its pre-invocation state, touches `save.requested`, emits an INFO log, and exits 0. With the marker absent, `commit-now` runs the happy path from task 1-1 unchanged.

**Do**:
- In `cmd/state_commit_now.go`, before the `ReadIndex` / `CaptureStructure` / `Commit` sequence, query the `@portal-restoring` server option via the existing primitive used by `state.IsRestoringSet` (or call `IsRestoringSet` directly).
- If set: log an INFO-level event via the state logger ("commit-now skipped: `@portal-restoring` set") under the daemon's `Component` constant, then call the existing save-requested touch helper from the `state` package (whatever `state notify` uses today — reuse, do not duplicate).
- Treat the `save.requested` touch as best-effort: if it errors, log the touch failure at WARN via the state logger and continue to exit 0. Do not nest error handling; do not propagate.
- Return exit code 0.
- Extend `commitNowDeps` with the `IsRestoringSet` query and the `save.requested` touch primitive so tests can inject failures on either.

**Acceptance Criteria**:
- [ ] When `@portal-restoring` is set, `commit-now` does not call `ReadIndex`, `CaptureStructure`, or `Commit`.
- [ ] When `@portal-restoring` is set, `sessions.json` is byte-identical before and after the invocation.
- [ ] When `@portal-restoring` is set, `save.requested` is touched (created or modified timestamp updated).
- [ ] When `@portal-restoring` is set, the subcommand emits an INFO-level structured log event noting the skip.
- [ ] When `@portal-restoring` is set, exit code is 0.
- [ ] When `@portal-restoring` is set and the `save.requested` touch itself fails, the subcommand still exits 0 (deliberate skip dominates) and logs the touch failure at WARN.
- [ ] When `@portal-restoring` is clear, `commit-now` proceeds to the task 1-1 happy path unchanged.

**Tests** (extend `cmd/state_commit_now_test.go`, no `t.Parallel()`):
- `"it short-circuits without writing sessions.json when @portal-restoring is set"`
- `"it touches save.requested during the @portal-restoring short-circuit"`
- `"it logs an INFO skip event when @portal-restoring is set"`
- `"it exits 0 on the @portal-restoring short-circuit"`
- `"it still exits 0 when save.requested touch fails during the short-circuit"`
- `"it proceeds normally when @portal-restoring is clear"`

**Edge Cases**:
- Marker set: skip everything, touch `save.requested`, exit 0.
- Marker clear: proceed to happy path.
- `save.requested` touch failure during skip: best-effort, WARN log, exit 0 nonetheless (the deliberate skip dominates per § `save.requested` Touch Failure Handling).
- Query of `@portal-restoring` itself fails (tmux unreachable): out of scope for this task — falls through to task 1-3 failure-path handling. For this task assume the query primitive returns `(false, nil)` and `(true, nil)` deterministically; the failure mode is exercised in 1-3.

**Context**:
> Per spec § `@portal-restoring` Defence: "`portal state commit-now` **must short-circuit as a no-op** when the `@portal-restoring` server option is set on the tmux server. … The daemon already honours this marker in its tick loop (`tick()` returns early if `restoring`). `commit-now` adopts the identical discipline: read `@portal-restoring`, return immediately if set, log the skip at INFO via the existing `state` package structured logger."
>
> Per spec § `save.requested` Discipline: "**`@portal-restoring` short-circuit (exit 0):** `save.requested` is touched. The daemon honours the marker too, so the flag queues a commit for the daemon's first post-restoration tick — without it, the daemon could skip ticks (via the `dirty || gap` rule) until the 30s gap is reached, briefly leaving `sessions.json` stale after restoration completes."
>
> Per spec § `save.requested` Touch Failure Handling: "Short-circuit path with `save.requested` touch error → still exit 0 (the deliberate skip dominates; the daemon's `gap` rule provides a 30-second worst-case fallback)."

**Spec Reference**: `.workflows/killed-session-resurrects-within-tick-window/specification/killed-session-resurrects-within-tick-window/specification.md` § `@portal-restoring` Defence, § `save.requested` Discipline, § `save.requested` Touch Failure Handling, § Exit Code Summary.

---

## killed-session-resurrects-within-tick-window-1-3 | approved

### Task 1-3: Add `commit-now` failure-path discipline

**Problem**: `commit-now` runs as a tmux hook subprocess with no attached terminal and no in-process consumer. Failures (tmux unreachable, `list-panes` error, disk full during the atomic rename, permission denied on the state directory) must not block or revert the kill, must not silently disappear, and must guarantee bounded recovery via the daemon's dirty-flag mechanism. Without an explicit fallback, a failed `commit-now` leaves the resurrection window open until the daemon's 30-second `gap` rule fires (or indefinitely if the daemon is down).

**Solution**: Wrap the `CaptureStructure` and `Commit` calls in error handling that, on any failure, (a) emits an ERROR-level structured log event with the underlying error, (b) touches `save.requested` as a best-effort daemon-fallback handoff, and (c) exits non-zero. The `save.requested` touch is itself best-effort — if it also fails, log the touch failure at WARN and still exit non-zero (the original `commit-now` failure dominates per § `save.requested` Touch Failure Handling). The kill is unaffected; tmux has already removed the session before the hook runs.

**Outcome**: A `commit-now` invocation that fails at any point during structural capture or atomic commit exits non-zero, logs the failure via the state logger, and touches `save.requested` so the daemon's next tick (within 1s of its scheduler) will commit the correct state. If `save.requested` touch also fails, the WARN is logged and the non-zero exit is unchanged. The kill itself is never propagated as failed back to tmux.

**Do**:
- In `cmd/state_commit_now.go`, wrap the `CaptureStructure` call: on error, log ERROR via the state logger under the daemon `Component` constant, touch `save.requested` (best-effort), and exit non-zero.
- Wrap the `Commit` call: on error, same treatment — ERROR log, best-effort `save.requested` touch, non-zero exit.
- The `IsRestoringSet` query (added in task 1-2) is also wrapped: on error, treat as "marker indeterminate" → safer default is to **proceed with the commit attempt** (we cannot prove restoration is in progress; suppressing the commit would re-open the resurrection window). If the subsequent capture/commit fails it lands in the same failure path. Document this choice in code comments. (Alternative — fail-closed and skip — would re-open the original bug on transient tmux query glitches; the spec's framing favours kill-side correctness over restoration-window safety on this narrow edge.)
- Treat the `save.requested` touch on every failure exit as best-effort: on touch error, log WARN with the touch failure, do not propagate, do not panic; the original failure's non-zero exit is preserved.
- Never `panic`; never block; never return an error that could cause Cobra to print a stack trace to tmux's hook subprocess stderr. All exits are via explicit exit-code paths.

**Acceptance Criteria**:
- [ ] When `CaptureStructure` returns an error (e.g., tmux unreachable), `commit-now` exits non-zero, touches `save.requested`, and emits an ERROR log event.
- [ ] When `Commit` returns an error (e.g., disk full, permission denied), `commit-now` exits non-zero, touches `save.requested`, and emits an ERROR log event. No partial or torn `sessions.json` is observable on disk (atomic rename guarantees this — verify the test does not see a partial file).
- [ ] When both the structural commit and the `save.requested` touch fail, `commit-now` still exits non-zero (original failure dominates) and logs the touch failure at WARN in addition to the ERROR log for the primary failure.
- [ ] `commit-now` failure does not cause `panic` and does not emit a Go stack trace; failures are reported via the state logger only.
- [ ] When `IsRestoringSet` returns an error, `commit-now` proceeds with the commit attempt (does not silently short-circuit). If the subsequent commit fails it falls through the standard failure path.
- [ ] The kill path (tmux's session removal) is not affected by `commit-now`'s exit code — verified by the integration test in task 1-6, but the unit-level acceptance here is that `commit-now` only sets its own exit code and writes no signal back to the caller beyond that.

**Tests** (extend `cmd/state_commit_now_test.go`, no `t.Parallel()`):
- `"it exits non-zero when CaptureStructure returns an error"`
- `"it touches save.requested when CaptureStructure returns an error"`
- `"it logs ERROR when CaptureStructure returns an error"`
- `"it exits non-zero when Commit returns an error"`
- `"it touches save.requested when Commit returns an error"`
- `"it leaves sessions.json byte-identical when Commit's atomic rename fails before the rename step"`
- `"it still exits non-zero when both Commit and save.requested touch fail"`
- `"it logs WARN for save.requested touch failure in addition to the ERROR for the primary failure"`
- `"it does not panic on any failure path"`
- `"it proceeds to commit when IsRestoringSet returns an error"`

**Edge Cases**:
- tmux unreachable (server socket gone mid-hook): `CaptureStructure` fails → failure path.
- Disk full during atomic rename: `Commit` fails → failure path; atomic semantics mean no torn write.
- Permission denied on state directory: `Commit` fails → failure path; `save.requested` touch likely also fails → both logged, non-zero exit.
- `save.requested` touch fails on every failure exit: log WARN, do not propagate, original failure dominates.
- `IsRestoringSet` query errors: proceed (fail-open) rather than skipping; documented in code comment.
- Goroutine panic inside `CaptureStructure`: out of scope — `state.CaptureStructure` is trusted by the daemon today and `commit-now` adopts the same trust level.

**Context**:
> Per spec § `commit-now` Failure Behaviour: "1. **Touch `save.requested` before exiting.** This is the explicit fallback that hands recovery to the daemon. Without this touch, the daemon's tick body short-circuits unless either `save.requested` is set or the 30-second `gap` has elapsed — leaving the resurrection window open for up to 30 seconds on failure. … 2. Exit non-zero so the failure is logged in tmux's hook subprocess context. 3. **Not** propagate the failure back to the kill — tmux has already removed the session; the kill is authoritative regardless of Portal's persistence success."
>
> Per spec § `save.requested` Touch Failure Handling: "Failure path with `save.requested` touch error → still exit non-zero (the original failure dominates). … This is a recovery-of-last-resort layer and the spec accepts that if both the synchronous commit and the dirty-flag fallback fail, the next Portal bootstrap will capture fresh state on its own."
>
> Per spec § Exit Code Summary: failure (tmux unreachable, disk error, etc.) → non-zero.

**Spec Reference**: `.workflows/killed-session-resurrects-within-tick-window/specification/killed-session-resurrects-within-tick-window/specification.md` § `commit-now` Failure Behaviour, § `save.requested` Discipline, § `save.requested` Touch Failure Handling, § Exit Code Summary.

---

## killed-session-resurrects-within-tick-window-1-4 | approved

### Task 1-4: Migrate `session-closed` hook registration to `commitNowCommand`

**Problem**: `RegisterPortalHooks` (bootstrap step 2) currently appends a single shared `notifyCommand` to all seven save-trigger events including `session-closed`. The bugfix requires that `session-closed` fire `commit-now` instead, and only `commit-now` — running both would do redundant work. Bootstrap-upgrade installs may have a pre-fix `notifyCommand` already registered on `session-closed`; the migration must remove that stale entry and register `commitNowCommand` in its place, idempotently and without disturbing user-customised hand-rolled hooks. The other six events stay on `notifyCommand` unchanged.

**Solution**: In `internal/tmux/hooks_register.go`, split the registration logic for `session-closed` from the other six events. Define `commitNowCommand` as a new const. For `session-closed`: enumerate currently-registered hooks via `ShowGlobalHooks`, exact-string match against the historical `notifyCommand` literal to identify stale entries, call `UnsetGlobalHookAt` highest-index-first to remove them, then append `commitNowCommand` if not already present (exact-string match against the `commitNowCommand` literal). For the six other events: retain the existing append-if-absent discipline unchanged.

**Outcome**: After `RegisterPortalHooks` runs against any of {empty, pre-fix, post-fix} starting states, the `session-closed` hook event has exactly one Portal-managed entry — `commitNowCommand` — and zero entries matching the historical `notifyCommand` literal. The six other save-trigger events have exactly one entry each — `notifyCommand` — unchanged from today. User-customised hooks (textually different from the Portal-emitted literals) are left untouched. Repeated bootstrap runs append no duplicates.

**Do**:
- In `internal/tmux/hooks_register.go`, add `const commitNowCommand = ``run-shell "command -v portal >/dev/null 2>&1 && portal state commit-now"```.
- Refactor `RegisterPortalHooks` (or add a helper called by it) so the seven events are no longer processed by a single shared loop. The six non-`session-closed` events retain today's append-if-absent behaviour. The `session-closed` event uses the scan-and-remove + append-if-absent migration algorithm below.
- For `session-closed`:
  1. Call `ShowGlobalHooks` to enumerate all currently-registered global hooks; filter in-process to entries whose event is `session-closed`.
  2. Build a slice of indices whose hook body **exact-string matches** the historical `notifyCommand` literal (`run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`). Sort descending.
  3. For each such index, call `UnsetGlobalHookAt("session-closed", idx)`. Per-index `UnsetGlobalHookAt` failures are best-effort: log WARN and continue to the next index. Do not abort the migration on a single index failure.
  4. After removal, re-enumerate `session-closed` entries (or track the in-process list, accounting for the removals). If no remaining entry exact-string matches `commitNowCommand`, call `AppendGlobalHook("session-closed", commitNowCommand)`.
- If `ShowGlobalHooks` itself fails, log WARN under the bootstrap component, return the error to the orchestrator's accumulated warnings (consistent with how step 2 surfaces failures today), and skip the `session-closed` migration. The six-other-events path may still run if structurally independent — match existing step-2 error-handling shape.
- Exact-string match only — no regex, no substring, no quoting tolerance. The match-set today contains exactly one literal (the current Portal-emitted `notifyCommand`); if a historical Portal version emitted a textually different literal, that literal is added to the match set in a subsequent change, not speculated about now.
- Update unit tests for `RegisterPortalHooks` in `internal/tmux/` to cover the three starting states.

**Acceptance Criteria**:
- [ ] `commitNowCommand` const is defined exactly as `run-shell "command -v portal >/dev/null 2>&1 && portal state commit-now"`.
- [ ] From an empty hook state, `RegisterPortalHooks` produces: `commitNowCommand` on `session-closed` (exactly one entry); `notifyCommand` on each of the six other save-trigger events (exactly one entry each).
- [ ] From a pre-fix state (`notifyCommand` registered on `session-closed`), `RegisterPortalHooks` removes the stale `notifyCommand` from `session-closed` and registers `commitNowCommand` in its place — net result: exactly one `commitNowCommand` and zero `notifyCommand` entries on `session-closed`. The six other events are unchanged.
- [ ] From a post-fix state (`commitNowCommand` already on `session-closed`), `RegisterPortalHooks` is a no-op on `session-closed` — no duplicate `commitNowCommand` appended.
- [ ] A user-customised hook on `session-closed` whose body differs textually from both Portal literals (e.g., `run-shell "portal state notify --debug"` or `run-shell "/path/to/my-wrapper.sh"`) is preserved verbatim across the migration.
- [ ] If `ShowGlobalHooks` returns an error, the migration logs WARN and skips the `session-closed` migration (surfaced consistent with existing step-2 warning handling) without panicking and without affecting the six other events' registration.
- [ ] If `UnsetGlobalHookAt` fails for a specific index, the migration logs WARN for that index and continues to the next.
- [ ] Repeated bootstrap runs against a fully-migrated state produce no changes (true idempotency).

**Tests** (extend `internal/tmux/hooks_register_test.go` or similar, no `t.Parallel()`):
- `"it registers commitNowCommand on session-closed from an empty hook state"`
- `"it registers notifyCommand on each of the six other save-trigger events from an empty hook state"`
- `"it removes a stale notifyCommand from session-closed during a pre-fix upgrade"`
- `"it registers commitNowCommand after removing the stale notifyCommand on session-closed"`
- `"it does not duplicate commitNowCommand when run against a post-fix state"`
- `"it preserves a user-customised hook on session-closed that does not exact-match the Portal literals"`
- `"it skips the session-closed migration and logs WARN when ShowGlobalHooks fails, leaving the six other events to be processed"`
- `"it logs WARN and continues when UnsetGlobalHookAt fails for a specific index"`
- `"it is idempotent across repeated invocations"`
- `"it processes removal indices highest-first so earlier removals do not shift later indices"`

**Edge Cases**:
- Empty hook state: append-only path for all seven events.
- Pre-fix install: scan finds one stale `notifyCommand` on `session-closed` → remove → append `commitNowCommand`.
- Post-fix install: scan finds zero stale entries; appendable entry already present → no-op.
- Mixed state (both `notifyCommand` and `commitNowCommand` on `session-closed` — e.g., from a partially-applied earlier migration): scan removes `notifyCommand`; `commitNowCommand` already present → no append.
- Multiple stale `notifyCommand` entries (theoretical duplicate from a prior bug): all removed; one `commitNowCommand` appended.
- User-customised hook on `session-closed`: exact-string mismatch against both Portal literals → preserved.
- `ShowGlobalHooks` failure: WARN log, skip `session-closed` migration, do not abort the bootstrap step.
- Per-index `UnsetGlobalHookAt` failure: WARN log, continue to next index; the migration may leave a residual stale entry but does not panic — daemon's next tick still produces a correct `sessions.json` via task 1-1's path.

**Context**:
> Per spec § Hook Registration Migration > Registration Strategy: "`session-closed` is registered with **only** `commitNowCommand`, not both `commitNowCommand` and `notifyCommand`."
>
> Per spec § Migration Algorithm: "Using the existing tmux-client primitives (`ShowGlobalHooks`, `AppendGlobalHook`, `UnsetGlobalHookAt`), the algorithm for `session-closed` is: 1. Call `ShowGlobalHooks` to enumerate the currently-registered hook entries across all events, filter in-process to entries for `session-closed`. 2. Scan the entries. For each entry whose body **exact-string matches** the historical pre-fix `notifyCommand` literal …, record its index and call `UnsetGlobalHookAt(event, index)`. Indices must be processed highest-first so removal does not shift the remaining indices. 3. After removal, scan the resulting entries again. If none **exact-string match** the `commitNowCommand` literal …, call `AppendGlobalHook(event, commitNowCommand)`."
>
> Per spec § Migration Algorithm: "**Why exact-string match (not substring/regex):** an exact-string comparison against the historical Portal-emitted literal is robust against accidentally removing user-customised hand-rolled hooks. … If a historical Portal version emitted a textually different `notifyCommand` literal, that exact string is added to the match set (the spec assumes a single historical literal until proven otherwise)."

**Spec Reference**: `.workflows/killed-session-resurrects-within-tick-window/specification/killed-session-resurrects-within-tick-window/specification.md` § Hook Registration Migration (Today, After Fix, Registration Strategy, Idempotency Requirements, Migration Algorithm, Why `session-closed` Is The Right Hook).

---

## killed-session-resurrects-within-tick-window-1-5 | approved

### Task 1-5: Real-tmux re-entrancy integration gate

**Problem**: `commit-now` makes tmux client calls (`ListSessionNames`, `list-panes -a -F …`, per-session `ShowEnvironment`) back into the same tmux server from within the `session-closed` hook subprocess. `pane-focus-out` and `session-renamed` are known to have historical re-entrancy quirks in the tmux server; `session-closed` is less suspect but not pre-validated for this specific call pattern. The spec explicitly mandates a real-tmux integration fixture confirming no deadlock or hang occurs **before** the rest of the implementation is taken as complete. On failure, the work returns to specification phase, not implementation — the mechanism is structurally dependent on tmux tolerating this re-entrancy.

**Solution**: Add a real-tmux integration test (built on the existing `tmuxtest` package) that wires `commitNowCommand` onto `session-closed`, starts a tmux server with a few sessions, kills one session, and asserts the hook subprocess completes within a bounded timeout (e.g., 1.5s — generous over the spec's ~50–200ms estimate) and writes a valid `sessions.json`. The test must fail visibly with a clear diagnostic if the subprocess hangs or deadlocks — not time out silently.

**Outcome**: A passing real-tmux integration test in `cmd/state_commit_now_integration_test.go` (or `internal/tmux/.../commit_now_reentrancy_test.go`, whichever matches the project's test-organisation idiom) that demonstrates `commit-now` invoked from inside the `session-closed` hook context completes without deadlock, produces a correct `sessions.json`, and finishes within 1.5s. If the test fails (hang, deadlock, or panic), the failure halts the rest of the phase and surfaces a spec-level pivot signal.

**Do**:
- Add an integration test file (e.g., `cmd/state_commit_now_reentrancy_integration_test.go`) gated by the integration build tag the project uses (match `restoretest` / `tmuxtest` idiom).
- Use `tmuxtest` to spawn a real tmux server on an isolated socket; use `portalbintest.BuildPortalBinary` (or `StagePortalBinary`) to build the `portal` binary and put it on PATH for the test process.
- In the test, register `commitNowCommand` on `session-closed` via the same `RegisterPortalHooks` code path used in production (do not bypass — the registration migration from task 1-4 is part of what's being validated under real tmux).
- Create two sessions (e.g., `A` and `B`) via the test's tmux client.
- Kill session `B` via `tmux kill-session -t B` against the test socket.
- Wait up to a bounded `1.5s` for the hook subprocess to complete. Implementation hint: poll `sessions.json` (via the test's `stateDir`) for the absence of `B`, with a `context.WithTimeout` of 1.5s. On timeout, fail loudly with a diagnostic that distinguishes hang/deadlock from a slow but progressing subprocess (e.g., capture and log the tmux server's pane state and the contents of the state directory).
- Assert: `sessions.json` exists, contains session `A`, omits session `B`, and the test completed in <1.5s.
- The test must not use `t.Parallel()` (per CLAUDE.md — `cmd` package mocks are package-level mutable state, and integration tests share build artefacts).
- Place the test under the project's integration build tag so it runs on the integration CI lane, not on every `go test ./...`.

**Acceptance Criteria**:
- [ ] Test file exists under the project's integration build tag conventions and is wired into the integration test lane.
- [ ] Test uses real tmux (via `tmuxtest` socket fixture) and a real `portal` binary (via `portalbintest`).
- [ ] Test exercises the production `RegisterPortalHooks` path to install `commitNowCommand` on `session-closed`.
- [ ] After `tmux kill-session -t B`, the test observes `sessions.json` reflecting the kill (B absent) within 1.5s, without hanging.
- [ ] On hang/deadlock, the test fails with a diagnostic that distinguishes deadlock from slow progress (captures pane state and state-dir contents in the failure message).
- [ ] Test does not use `t.Parallel()`.
- [ ] Test failure is treated as a spec-level pivot signal — the task description and PR description note that hangs here block the phase and return work to specification.

**Tests** (the test itself is the deliverable; the named cases are the assertions inside it):
- `"it does not hang when commit-now is invoked from inside the session-closed hook"` — the primary assertion (1.5s timeout)
- `"it writes a sessions.json omitting the killed session after the hook completes"`
- `"it preserves sessions other than the killed one in sessions.json"`
- `"it fails with a deadlock-diagnostic message rather than a silent timeout"` — the failure-mode assertion (forced by introducing a known-failing scenario in a sub-test or by code inspection of the timeout branch)

**Edge Cases**:
- Hang/deadlock: test must fail visibly with diagnostic, not time out silently. Use `context.WithTimeout` and an explicit `t.Fatalf` with structured failure context.
- tmux server unreachable during test setup: fail the test setup before the assertion phase with a clear setup-failure message (do not conflate test setup failures with re-entrancy failures).
- Slow but progressing subprocess (e.g., test runner under heavy load): 1.5s budget is generous over the 50–200ms estimate; if persistently flaky on CI, the budget can be raised in a follow-up — do not pre-emptively raise it here.
- Race between hook subprocess exit and the test's `sessions.json` poll: poll with a short interval (e.g., 25ms) and require two consecutive consistent reads before asserting absence, to avoid mistaking an in-flight rename for completion.

**Context**:
> Per spec § Hook Re-entrancy: "`commit-now` makes tmux client calls (`ListSessionNames`, `list-panes -a -F …`, per-session `ShowEnvironment`) back into the same tmux server from within the `session-closed` hook context. `pane-focus-out` and `session-renamed` have historical re-entrancy quirks in the tmux server; `session-closed` is less suspect but not pre-validated for this specific call pattern."
>
> Per spec § Hook Re-entrancy: "**Requirement on plan/implementation phase:** a real-tmux integration test fixture must confirm no deadlock or hang occurs when `commit-now` runs from inside the `session-closed` hook. The test must be written and passing **before** the rest of the implementation work is taken as complete."
>
> Per spec § Hook Re-entrancy: "**On re-entrancy test failure:** the work unit returns to the specification phase, not the implementation phase. The chosen mechanism (synchronous tmux calls from within the hook subprocess) is structurally dependent on tmux tolerating this re-entrancy pattern …"

**Spec Reference**: `.workflows/killed-session-resurrects-within-tick-window/specification/killed-session-resurrects-within-tick-window/specification.md` § Hook Re-entrancy, § Testing Requirements > Integration Tests (Real Tmux Fixture, Required) bullet "Hook re-entrancy validation".

---

## killed-session-resurrects-within-tick-window-1-6 | approved

### Task 1-6: Real-tmux kill→bootstrap canonical symptom integration test

**Problem**: The phase's headline acceptance criterion is that, immediately after any kill path, a fresh `portal` bootstrap does not reconstruct the killed session and the TUI does not list it. Unit tests cover `commit-now`'s mechanics; the re-entrancy gate (1-5) covers the hook→subprocess wiring. Neither directly demonstrates the end-to-end symptom is gone: kill → `sessions.json` updated → next bootstrap's step 5 `Restore` correctly skips reconstruction. The spec calls out this exact integration test as required, plus the two `_portal-saver` self-kill scenarios.

**Solution**: Add a real-tmux integration test that drives the canonical symptom timeline end to end: bootstrap into a stable two-session state, kill one session via `tmux kill-session` (the external path — which fires through the same `session-closed` seam as every cmd-internal kill), immediately read `sessions.json` and assert the killed session is absent, then run a second bootstrap and assert step 5 `Restore` does not reconstruct it. Add two `_portal-saver` scenarios: (a) marker clear → underscore filter omits `_portal-saver` while preserving user sessions; (b) marker set → `sessions.json` is byte-identical before and after the hook fires.

**Outcome**: A passing real-tmux integration test demonstrates the kill→bootstrap symptom is gone end-to-end via one external kill path (which subsumes all kill paths since they converge through `session-closed`), plus both `_portal-saver` self-kill scenarios behave as the spec mandates.

**Do**:
- Add an integration test file (e.g., `cmd/state_commit_now_symptom_integration_test.go`) gated by the integration build tag.
- Sub-test 1 — canonical symptom:
  1. Spawn real tmux via `tmuxtest`, build/stage the `portal` binary via `portalbintest`, run a full bootstrap (e.g., `portal open` against an isolated state dir, or invoke the bootstrap orchestrator directly via the existing integration entry point) to reach a stable state with two sessions `A` and `B`.
  2. Verify `sessions.json` contains both A and B.
  3. Run `tmux kill-session -t B` against the test socket.
  4. Immediately (no `time.Sleep`; use a tight poll with two-consecutive-consistent-reads as in 1-5) read `sessions.json` and assert B is absent and A is present.
  5. Run a second bootstrap. Assert step 5 `Restore` does not create a skeleton pane for B — verify by listing live tmux sessions/panes after the second bootstrap and confirming no resurrected B-session pane exists.
- Sub-test 2 — `_portal-saver` self-kill, marker clear:
  1. Bootstrap to stable state with user sessions A, B, and the auto-spawned `_portal-saver`.
  2. Kill `_portal-saver` via `tmux kill-session -t _portal-saver` from outside any bootstrap (marker is clear).
  3. Poll `sessions.json`; assert it contains A and B intact and omits `_portal-saver` (via existing `keepSessionNames` underscore filter).
- Sub-test 3 — `@portal-restoring` defence end-to-end (covers spec § Testing Requirements > "`@portal-restoring` defence under real tmux"):
  1. Bootstrap to stable state with user sessions A and B and `_portal-saver` running.
  2. Manually set `@portal-restoring` on the tmux server via the test's tmux client (simulating the bootstrap-step-4 timeline window).
  3. Snapshot `sessions.json` bytes.
  4. Kill `_portal-saver` via `tmux kill-session -t _portal-saver`.
  5. Wait briefly (e.g., 250ms) for the hook subprocess to run and exit.
  6. Re-read `sessions.json` bytes; assert byte-identical to the snapshot (the marker-set short-circuit fired).
  7. Clear `@portal-restoring`.
  8. Kill session `B` via `tmux kill-session -t B` (a normal user kill now that the marker is clear).
  9. Poll `sessions.json` (two-consecutive-consistent-reads, ≤1.5s budget) and assert B is absent and A is present — verifies the marker-clear path resumes the synchronous commit on the same fixture.
- Do not enumerate all five kill paths as separate sub-tests. The spec is explicit that all kill paths converge through `session-closed`; one external kill demonstrates the seam works under real tmux. Manual verification (per spec § Testing Requirements > Manual Verification) covers the path-by-path matrix outside automated tests.
- Test does not use `t.Parallel()`.

**Acceptance Criteria**:
- [ ] Sub-test 1 demonstrates: after `tmux kill-session -t B`, `sessions.json` omits B without any `time.Sleep`/retry from the test (tight poll with consistency check).
- [ ] Sub-test 1 demonstrates: a second bootstrap completes without reconstructing B as a skeleton pane (verified by enumerating live tmux state post-bootstrap).
- [ ] Sub-test 2 demonstrates: `_portal-saver` self-kill in marker-clear state leaves `sessions.json` containing all user sessions intact and omitting `_portal-saver`.
- [ ] Sub-test 3 demonstrates: `_portal-saver` self-kill in marker-set state leaves `sessions.json` byte-identical before and after the hook subprocess runs.
- [ ] Sub-test 3 demonstrates: after clearing `@portal-restoring` on the same fixture, a subsequent `tmux kill-session -t B` results in `sessions.json` omitting B within the bounded poll window — verifies the short-circuit gate is per-invocation, not a static state captured at boot.
- [ ] Test does not use `t.Parallel()`.
- [ ] Test runs on the integration build tag, not on every `go test ./...`.
- [ ] Each sub-test's diagnostic message on failure surfaces the actual file contents and tmux session list to aid post-mortem.

**Tests** (the test is the deliverable; the named cases are the assertions):
- `"it removes the killed session from sessions.json before the hook subprocess exits"` (canonical symptom)
- `"a fresh bootstrap does not reconstruct the killed session"` (canonical symptom)
- `"_portal-saver self-kill with marker clear leaves user sessions intact and omits _portal-saver"`
- `"_portal-saver self-kill with marker set leaves sessions.json byte-identical"`
- `"after clearing @portal-restoring on the same fixture, a subsequent kill updates sessions.json correctly"` (sub-test 3 continuation)

**Edge Cases**:
- `_portal-saver` not present at the moment of the marker-clear sub-test (bootstrap step 4 failed or was disabled in the test scaffold): skip the sub-test with a `t.Skip` and a clear message, or ensure the test scaffold guarantees `_portal-saver` presence before the kill — prefer the latter.
- Second bootstrap in sub-test 1 races with the post-kill daemon tick: not a problem — the daemon's merge filter (`mergeSkippedPanes`) preserves the absence invariant, and the assertion is on the absence of a reconstructed pane, not on byte-equivalence of `sessions.json`.
- Marker-set sub-test interferes with state-dir cleanup: ensure `@portal-restoring` is cleared in test teardown so cross-test pollution is avoided.
- Polling for "B is absent": require two consecutive consistent reads (matching 1-5) to avoid race with the atomic rename's `[temp, rename]` window — atomic rename is instantaneous on POSIX but defensive reads cost nothing.

**Context**:
> Per spec § Testing Requirements > Integration Tests > "Kill → bootstrap timeline (the canonical symptom test)": "1. Bootstrap into a stable state with two sessions A and B. 2. Kill session B via `tmux kill-session -t B` (drives the hook in the real way). 3. Immediately — no sleep, no retry — read `sessions.json` and assert B is absent. 4. Run another bootstrap; assert B is not reconstructed."
>
> Per spec § Testing Requirements > Integration Tests > "`_portal-saver` self-kill.": "During the version-upgrade kill of `_portal-saver` in bootstrap step 4, assert `sessions.json` remains valid and contains all user sessions intact (no corruption from the underscore-session firing through the synchronous path)."
>
> Per spec § Why `session-closed` Is The Right Hook: all kill paths converge through `session-closed`, so a single external `tmux kill-session` test exercises the seam used by TUI `K`, `portal kill`, `Option-Q`, and `M-q`.

**Spec Reference**: `.workflows/killed-session-resurrects-within-tick-window/specification/killed-session-resurrects-within-tick-window/specification.md` § Testing Requirements > Integration Tests, § Why `session-closed` Is The Right Hook, § `_portal-saver` Self-Kill, § Acceptance Criteria items 1, 2, 5, 6.

---

## killed-session-resurrects-within-tick-window-1-7 | approved

### Task 1-7: Non-regression tests for daemon merge and six-event eventual consistency

**Problem**: The fix touches two seams (the new `commit-now` subprocess and the `session-closed` hook registration) but explicitly does not change the daemon's tick body, the merge filter, the atomic-commit primitive, or the six other save-trigger events' behaviour. The spec calls out two specific non-regression risks: (a) `daemon-merge-reintroduces-dead-sessions` could regress if the daemon's `PrevIndex`-driven merge re-introduces a killed session that `commit-now` already removed; (b) the six other events could silently regress to a structural-capture cost path if a future refactor incorrectly generalises the migration in task 1-4. Both need explicit regression tests so future changes that break these invariants fail loudly.

**Solution**: Add two regression tests. The first asserts that after a `commit-now` write removes a killed session, the daemon's next tick produces a `sessions.json` whose session-name set still omits that session (byte-equivalence is not asserted — the daemon legitimately repopulates scrollback-hash fields). The second asserts that each of the six non-`session-closed` save-trigger events fires `portal state notify` (touching `save.requested`) and produces no `sessions.json` write inside the same tick window — i.e., they remain on the cheap path.

**Outcome**: Two regression tests in place that fail loudly if (a) the daemon's next-tick output re-introduces a killed session after `commit-now`, or (b) any of the six events accidentally starts writing `sessions.json` synchronously. These tests guard the invariants documented in the spec's Non-Regression section.

**Do**:
- Regression test 1 — daemon merge stability after `commit-now` (integration, real tmux):
  1. Spawn real tmux, bootstrap to a stable state with sessions A and B, `_portal-saver` running.
  2. `tmux kill-session -t B`; wait for `sessions.json` to reflect the kill (poll with consistency check).
  3. Force a daemon tick to fire — either touch `save.requested` directly, or wait up to 2s for the daemon's next scheduled tick.
  4. After the daemon tick completes, read `sessions.json` and assert the **set of session names** still omits B and still contains A. Do not assert byte-equivalence to the pre-tick file (daemon legitimately repopulates fields).
  5. Document in the test body (comment) that this guards `daemon-merge-reintroduces-dead-sessions`'s invariant under the new synchronous-commit timeline.
- Regression test 2 — six-event eventual consistency (unit, with mock tmux + observable `notify` invocation):
  1. For each of the six non-`session-closed` events (`session-created`, `session-renamed`, `window-linked`, `window-unlinked`, `window-layout-changed`, `pane-focus-out`), assert via `RegisterPortalHooks` unit tests that the registered hook body is exactly `notifyCommand` and not `commitNowCommand`.
  2. Add a hook-fire simulation (driven by `tmuxtest` real-tmux fixture, or a unit-level fake that records the executed command) that fires each event and asserts a `save.requested` touch was observed and no `sessions.json` write occurred within a 500ms window after the fire.
  3. If a real-tmux event simulator is impractical for every event (e.g., `pane-focus-out` is hard to drive deterministically in a test), assert the registration-level invariant (each event maps to `notifyCommand`) plus a unit-level invariant that `notifyCommand` itself does no `sessions.json` work — that combination is sufficient to guard the eventual-consistency contract.
- Place test 1 under the integration build tag; test 2 may be a unit test (in `internal/tmux/`) supplemented by an integration sub-test for the events that are easy to drive.
- Tests do not use `t.Parallel()`.

**Acceptance Criteria**:
- [ ] Regression test 1 asserts on the set of session names in `sessions.json` after the daemon's post-`commit-now` tick (semantic invariant, not byte-equivalence).
- [ ] Regression test 1 fails clearly if the killed session reappears in the daemon's next-tick output.
- [ ] Regression test 2 asserts each of the six non-`session-closed` events is registered with `notifyCommand` and not `commitNowCommand`.
- [ ] Regression test 2 asserts (via real or fake hook-fire simulation) that firing each of the six events touches `save.requested` and writes nothing to `sessions.json` within a bounded window.
- [ ] Both tests are gated to the appropriate test lane (integration vs unit) per the project's conventions.
- [ ] Tests do not use `t.Parallel()`.
- [ ] Both tests' failure messages surface enough context (file contents, registered hook bodies, observed command invocations) to diagnose a regression without re-running.

**Tests** (the tests are the deliverables; named assertions inside):
- `"daemon's next tick after commit-now does not re-introduce the killed session by name"` (regression test 1)
- `"daemon's next tick after commit-now retains all live sessions by name"` (regression test 1)
- `"session-created is registered with notifyCommand and not commitNowCommand"` (regression test 2)
- `"session-renamed is registered with notifyCommand and not commitNowCommand"` (regression test 2)
- `"window-linked is registered with notifyCommand and not commitNowCommand"` (regression test 2)
- `"window-unlinked is registered with notifyCommand and not commitNowCommand"` (regression test 2)
- `"window-layout-changed is registered with notifyCommand and not commitNowCommand"` (regression test 2)
- `"pane-focus-out is registered with notifyCommand and not commitNowCommand"` (regression test 2)
- `"firing notifyCommand touches save.requested and writes nothing to sessions.json"` (regression test 2, unit-level)

**Edge Cases**:
- `PrevIndex` staleness in the daemon's tick: the test 1 must run the daemon's tick **after** `commit-now`'s write so the daemon's `PrevIndex` may be stale relative to disk. This is the exact condition spec § Daemon Merge Interaction calls "verified safe" — the test materialises that verification.
- `pane-focus-out` high-frequency fire path: difficult to drive deterministically; rely on registration-level assertion + `notifyCommand` semantics assertion as the combined invariant.
- `session-renamed` event: drive via `tmux rename-session` against the test socket; assert `save.requested` touch and no `sessions.json` write in the window.
- Daemon down during test 1: ensure `_portal-saver` is running (or directly invoke a single daemon tick via the existing integration helper) so the "next tick" actually fires within the test's bound.
- Bounded window for "no `sessions.json` write": 500ms is comfortably under the daemon's 1s tick period; tests should not stretch to where the daemon's natural tick could legitimately fire and create a false positive.

**Context**:
> Per spec § Daemon Merge Interaction (Verified Safe): "The daemon's next tick after a `commit-now` invocation will run `captureAndCommit` with a `PrevIndex` (the in-memory previous-tick view) that may be staler than the just-written `sessions.json`. `mergeSkippedPanes` (`internal/state/capture.go`) filters by **live structure**, not by `PrevIndex` — it only retains prev panes that are also present in the fresh capture. The killed session won't be in fresh, so it won't be re-introduced regardless of `PrevIndex` staleness. **Verdict:** safe by inspection. No code change required. Documented here so future readers don't re-litigate."
>
> Per spec § Testing Requirements > Regression Tests: "**Daemon merge stability after `commit-now`.** After a `commit-now` write that omits a killed session, the daemon's next tick must produce a `sessions.json` whose session-name set still omits that session. The test asserts on the set of session names, not on byte-equivalence …"
>
> Per spec § Testing Requirements > Regression Tests: "**Six-event eventual consistency.** Fire each of the six non-`session-closed` save-trigger events; assert each one results in a `save.requested` touch and **no** `sessions.json` write within the same tick window. Verifies they remained on the cheap path."
>
> Per spec § Acceptance Criteria item 11: "byte-equivalence between `commit-now`'s output and the daemon's next-tick output is not required — the daemon legitimately enriches the schema with fields that `commit-now` preserves from prev (scrollback hashes, per-pane content references). The invariant is **semantic**: dead sessions stay out, live sessions stay in."

**Spec Reference**: `.workflows/killed-session-resurrects-within-tick-window/specification/killed-session-resurrects-within-tick-window/specification.md` § Daemon Merge Interaction, § Testing Requirements > Regression Tests, § Acceptance Criteria items 9, 10, 11, 13.
