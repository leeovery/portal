---
phase: 1
phase_name: Alive-check gating in EnsurePortalSaverVersion + daemon.version breadcrumb
total: 6
---

## saver-kill-respawn-loop-leaks-daemons-1-1 | approved

### Task saver-kill-respawn-loop-leaks-daemons-1-1: Reframe `portalSaverVersionMismatch` table tests to cover all six matrix rows

**Problem**: The existing unit test for `portalSaverVersionMismatch` (in `internal/tmux/portal_saver_test.go`) documents "absent counts as version mismatch" as a load-bearing contract. The specification inverts that framing — `absent → true` remains a valid predicate-layer verdict, but it is no longer the authoritative gate for the kill decision (the alive-check in `EnsurePortalSaverVersion` is). The test framing must be reworked, not deleted, so future readers do not interpret the predicate's `absent → true` row as the load-bearing rule.

**Solution**: Rename and reframe the existing table test, expand its case list to cover all six rows of the specification's predicate matrix, and update the test/case names plus any leading documentation comment so the framing reads "the predicate alone — the alive-check happens in the caller" rather than "absent counts as mismatch."

**Outcome**: A reframed table test in `internal/tmux/portal_saver_test.go` whose six cases pin `match`, `real mismatch`, `absent + neither dev`, `non-absent I/O read error`, `stored=dev`, and `current=dev`. Test documentation no longer claims absence is load-bearing mismatch contract; instead it documents that the predicate's behaviour is consumed by the alive-check gate in `EnsurePortalSaverVersion`.

**Do**:
- Locate the existing table test for `portalSaverVersionMismatch` in `internal/tmux/portal_saver_test.go`.
- Rename the test (e.g. from `TestPortalSaverVersionMismatch` to a name that does not claim "absent is mismatch", such as `TestPortalSaverVersionMismatch_PredicateMatrix`) and update its leading comment block to document: "the predicate's verdict is one input — `EnsurePortalSaverVersion` consults `BootstrapAliveCheck` first; this predicate is only consulted on the alive-with-readable-version branch."
- Replace the case slice so it covers exactly the six rows from spec §Testing Requirements:
  1. stored=`0.5.0`, current=`0.5.0`, readErr=nil → expected `false`
  2. stored=`0.5.0`, current=`0.5.1`, readErr=nil → expected `true` (real mismatch)
  3. stored=`""`, current=`0.5.0`, readErr=`ErrVersionFileAbsent` → expected `true` (predicate alone — alive-check happens in caller)
  4. stored=`""`, current=`0.5.0`, readErr=arbitrary non-absent I/O error (e.g. `fs.ErrPermission` or a synthesized `errors.New("io: read failed")`) → expected `true`
  5. stored=`dev`, current=`0.5.0`, readErr=nil → expected `true` (dev preserved)
  6. stored=`0.5.0`, current=`dev`, readErr=nil → expected `true` (dev preserved)
- Each case must have a descriptive name (e.g. `match`, `real_mismatch_neither_dev`, `absent_neither_dev_predicate_layer_only`, `non_absent_io_read_error`, `stored_dev_short_circuit`, `current_dev_short_circuit`).
- Drive each case via the existing predicate seam — the predicate currently consults a `readVersionFile`-style function or returned (stored, err) pair; reuse the existing injection pattern. Do not change the predicate's signature in this task.
- Keep all assertion logic identical in shape to the existing test (compare boolean result; failure message names the case).

**Acceptance Criteria**:
- [ ] The renamed test exists in `internal/tmux/portal_saver_test.go` and runs under `go test ./internal/tmux/...`.
- [ ] All six cases above are present, each with a distinct case name, and each pins the expected boolean.
- [ ] The leading comment block does not assert "absent counts as version mismatch" as a load-bearing contract; instead it documents the predicate-as-one-input framing.
- [ ] `go test ./internal/tmux/...` is green.
- [ ] No production code in `internal/tmux/portal_saver.go` is modified by this task.

**Tests**:
- `"match — stored and current equal, neither dev, returns false"`
- `"real mismatch — stored and current differ, neither dev, returns true"`
- `"absent neither dev — read returns ErrVersionFileAbsent, predicate returns true (alive-check is caller's responsibility)"`
- `"non-absent I/O read error — read returns a generic I/O error, predicate returns true"`
- `"stored dev short-circuit — stored=dev forces true regardless of current"`
- `"current dev short-circuit — current=dev forces true regardless of stored"`

**Edge Cases**:
- `stored=""` paired with `ErrVersionFileAbsent` vs `stored=""` paired with a non-absent I/O error must be distinguishable cases — do not collapse them.
- Dev cases must assert `true` even though they superficially "match" the absent-case behaviour; the reason differs (dev short-circuit vs. unreadable file), and the case names should reflect that.
- The reframed comment must not introduce ambiguity: if a reader's first question is "why does absent return true if it's no longer load-bearing?", the comment should answer it (predicate's verdict is consumed by an alive-check gate in `EnsurePortalSaverVersion`).

**Context**:
> Spec §Testing Requirements > Unit tests: "the existing test's assertions are preserved (the predicate still returns true for absent), but its framing must be reworked, not deleted: rename the test and update its documentation so it no longer claims 'absent counts as version mismatch' as a load-bearing contract."

**Spec Reference**: `.workflows/saver-kill-respawn-loop-leaks-daemons/specification/saver-kill-respawn-loop-leaks-daemons/specification.md` §Testing Requirements

---

## saver-kill-respawn-loop-leaks-daemons-1-2 | approved

### Task saver-kill-respawn-loop-leaks-daemons-1-2: Add DEBUG breadcrumb to `state.WriteVersionFile` under `ComponentDaemon`

**Problem**: Defect 3 (`daemon.version` disappearance) was not pinned to any production code path during the investigation. Without instrumentation, the next recurrence would launch another full investigation from scratch. A stable, greppable log line at every write site provides a paper trail in `portal.log` correlating writes to daemon lifecycle events.

**Solution**: Inside `state.WriteVersionFile` (`internal/state/`), emit a single DEBUG log line under `state.ComponentDaemon` containing the version, caller pid (`os.Getpid()`), and destination path. The line MUST begin with the literal prefix `daemon.version write:` so future grep on `portal.log` is stable.

**Outcome**: Every invocation of `state.WriteVersionFile` produces exactly one DEBUG log line of the form `daemon.version write: version=<v> pid=<p> path=<absolute path>` under component `state.ComponentDaemon`. No behavioural change, no new error paths, no new file I/O. Existing tests for `WriteVersionFile` remain green.

**Do**:
- Open `internal/state/` and locate `WriteVersionFile` (search for the function definition; it is the sole writer of `daemon.version`).
- Identify the package's existing logger access pattern — `internal/state/logger.go` exposes `state.ComponentDaemon` and the structured-logger entry points used elsewhere in the package. Match that pattern; do not introduce a new logger seam.
- At the head of `WriteVersionFile`, after the function's argument list is validated (or after the destination path is computed if path computation happens inside the function), emit one DEBUG log line:
  - Prefix exactly: `daemon.version write:`
  - Fields: `version=<value>`, `pid=<os.Getpid()>`, `path=<destination absolute path>`
  - Component tag: `state.ComponentDaemon`
- The exact `fmt`-template choice is implementation choice; the `daemon.version write:` prefix is contract.
- Place the log call **before** the atomic-write side effect so a write that subsequently fails still leaves a breadcrumb.
- Do not wrap the call in error handling — the logger never returns to the caller in a way that influences `WriteVersionFile`'s return value.
- Run `go test ./internal/state/...` and confirm existing `WriteVersionFile` tests are unaffected.

**Acceptance Criteria**:
- [ ] `state.WriteVersionFile` emits exactly one DEBUG log line per invocation, prefixed with the literal string `daemon.version write:` and tagged with `state.ComponentDaemon`.
- [ ] The log line includes the version value, `os.Getpid()` of the caller, and the destination path.
- [ ] The log call sits before the atomic-write side effect.
- [ ] `WriteVersionFile`'s signature and return shape are unchanged.
- [ ] `go test ./internal/state/...` is green; existing `WriteVersionFile` tests pass without modification.
- [ ] `go build -o portal .` succeeds.

**Tests**:
- `"it logs a DEBUG breadcrumb with prefix 'daemon.version write:' on every WriteVersionFile call"` — drive `WriteVersionFile` against a temp dir with a captured logger and assert the line is present, prefixed correctly, and contains version + pid + path tokens.
- `"it logs the breadcrumb even when the underlying atomic write fails"` — inject a write failure (read-only directory) and assert the breadcrumb was emitted before the error returned.
- `"it does not emit duplicate breadcrumbs on a single call"` — assert exactly one matching line per call.

**Edge Cases**:
- The atomic-write target may fail (read-only filesystem, ENOSPC). The breadcrumb must still appear in the log so the caller's intent is recorded even when the write side effect did not land.
- `os.Getpid()` must be evaluated at call time, not cached at package init — different callers (daemon vs. bootstrap-side defensive write from Task 1-4) will have different pids.
- The log line must not include any sensitive data beyond version/pid/path. The version is build-time; pid is process-local; path is already known to anyone reading `portal.log`.

**Context**:
> Spec §Change 3: "Add a single DEBUG-level log entry inside `state.WriteVersionFile` capturing: the version string being written, the caller's pid (`os.Getpid()`), the destination path. Component: `state.ComponentDaemon`. Format anchor: the log line MUST begin with `daemon.version write:` so future Defect 3 investigations can grep on a stable prefix. Example: `daemon.version write: version=0.5.0 pid=12345 path=/Users/x/.config/portal/state/daemon.version`. The exact fmt template is implementation choice, but the prefix is contract. No behavioural change. No additional file I/O, no new error paths, no return-shape changes. Pure instrumentation."

**Spec Reference**: `.workflows/saver-kill-respawn-loop-leaks-daemons/specification/saver-kill-respawn-loop-leaks-daemons/specification.md` §Change 3

---

## saver-kill-respawn-loop-leaks-daemons-1-3 | approved

### Task saver-kill-respawn-loop-leaks-daemons-1-3: Gate kill decision on `BootstrapAliveCheck` in `EnsurePortalSaverVersion` before mismatch predicate

**Problem**: `EnsurePortalSaverVersion` (`internal/tmux/portal_saver.go:249` approx) currently consults `portalSaverVersionMismatch` as the lone gate for `killSaverAndWaitForDaemon`. Because the predicate returns `true` for `ErrVersionFileAbsent`, any bootstrap where `daemon.version` is missing — but the daemon is alive and healthy — fires an unnecessary kill-respawn cycle, leaking orphan daemons and pausing saves silently. The kill decision must be gated on daemon aliveness first.

**Solution**: Rework the decision in `EnsurePortalSaverVersion` to consult `BootstrapAliveCheck(stateDir)` before the mismatch predicate. The new matrix is documented in spec §Change 1: dev-build short-circuit kills when alive; alive+absent → no kill (defensive write handled in Task 1-4); alive+readable+match → no kill; alive+readable+mismatch (neither dev) → kill; alive+read-error → kill (conservative); not-alive → no kill regardless of version state.

**Outcome**: `EnsurePortalSaverVersion` consults `BootstrapAliveCheck(stateDir)` first. The kill barrier (`killSaverAndWaitForDaemon`) fires only on the four "kill" rows of the new matrix; the two "no kill" rows proceed straight to `BootstrapPortalSaver`. Function signature unchanged. Six new unit tests pin the matrix end-to-end.

**Do**:
- Open `internal/tmux/portal_saver.go` and locate `EnsurePortalSaverVersion(c *Client, stateDir, currentVersion string) error` (approx line 249).
- Confirm `BootstrapAliveCheck` is the existing package-level variable wired to `state.DaemonAlive` (search the package for `BootstrapAliveCheck` to confirm; do not rewire it).
- Rework the decision flow to evaluate, in order:
  1. **Read the stored version + err once up front** (so the same read result is reused across branches). Capture `stored, readErr := portalSaverReadVersionFile(stateDir)` (or whatever helper the predicate currently uses — preserve the helper).
  2. **Alive check first:** `alive := BootstrapAliveCheck(stateDir)`.
  3. **If `!alive`:** skip the kill entirely; fall through to `BootstrapPortalSaver`. No version write here (Task 1-4 owns the defensive write only on the alive+absent branch).
  4. **If `alive`:**
     - **Dev short-circuit:** if `stored == "dev"` or `stored == ""` paired with no readErr, OR `currentVersion == "dev"` or `currentVersion == ""`, treat as dev and **kill** via `killSaverAndWaitForDaemon`, then proceed to `BootstrapPortalSaver`. (Match the existing dev predicate exactly — do not redefine "dev"; reuse the same condition the current predicate uses.)
     - **Absent version file:** `errors.Is(readErr, ErrVersionFileAbsent)` → **no kill**. Task 1-4 layers the defensive `WriteVersionFile(currentVersion)` onto this branch — leave a clear comment marking the integration point.
     - **Read error (non-absent):** `readErr != nil && !errors.Is(readErr, ErrVersionFileAbsent)` → **kill** (conservative).
     - **Versions match (neither dev, no read error):** `stored == currentVersion` → **no kill**.
     - **Versions mismatch (neither dev, no read error):** `stored != currentVersion` → **kill**.
- Keep `portalSaverVersionMismatch`'s external shape unchanged (Task 1-1 reframes its test but does not change its signature). The predicate may now be unused inside `EnsurePortalSaverVersion` — that is acceptable; the predicate stays exported/visible for any external callers and for the reframed table test. Do **not** delete it.
- Preserve the dev-build short-circuit exactly as today's predicate evaluates it — copy the condition; do not redefine "dev".
- Preserve all existing log messages (WARN/INFO/DEBUG); add no new ones in this task beyond what is needed for the kill decision branches (one DEBUG line per branch is acceptable; not required).
- The signature `EnsurePortalSaverVersion(c *Client, stateDir, currentVersion string) error` is unchanged — no caller-side changes propagate.
- Add the six unit tests below in `internal/tmux/portal_saver_test.go` (alongside the reframed table test from Task 1-1). Drive `BootstrapAliveCheck` and the version-file reader via existing package-level seams; install fakes via `t.Cleanup()` (no `t.Parallel()` — see CLAUDE.md).

**Acceptance Criteria**:
- [ ] `EnsurePortalSaverVersion` consults `BootstrapAliveCheck(stateDir)` exactly once, before any decision branch.
- [ ] The six matrix rows from spec §Change 1 are implemented in the order specified: not-alive → no kill; alive+dev (either side) → kill; alive+absent → no kill (defensive-write hook for Task 1-4); alive+read-error → kill; alive+match → no kill; alive+mismatch (neither dev) → kill.
- [ ] The function signature `EnsurePortalSaverVersion(c *Client, stateDir, currentVersion string) error` is unchanged.
- [ ] `portalSaverVersionMismatch` is not deleted (Task 1-1 still references it from the reframed table test).
- [ ] Six new unit tests in `internal/tmux/portal_saver_test.go` (listed below) all pass.
- [ ] `go test ./internal/tmux/...` is green.
- [ ] The dev short-circuit rule is byte-equivalent in semantics to the existing predicate (verify against the existing dev cases).

**Tests**:
- `"alive=false, version=absent → no kill, proceeds to BootstrapPortalSaver"`
- `"alive=false, versions mismatch (neither dev) → no kill, proceeds to BootstrapPortalSaver"`
- `"alive=true, stored=dev → kill barrier runs, then BootstrapPortalSaver"`
- `"alive=true, current=dev → kill barrier runs, then BootstrapPortalSaver"`
- `"alive=true, version absent (neither dev) → no kill, proceeds to BootstrapPortalSaver"` (Task 1-4 asserts the defensive write on this same branch)
- `"alive=true, read returns non-absent I/O error → kill barrier runs"`
- `"alive=true, versions match (neither dev) → no kill, proceeds to BootstrapPortalSaver"`
- `"alive=true, versions mismatch (neither dev) → kill barrier runs"`
- `"BootstrapAliveCheck is consulted before portalSaverVersionMismatch"` (assert via call-ordering recorder on a fake)

**Edge Cases**:
- `currentVersion` is empty string vs. `currentVersion == "dev"` — both are treated as dev. Reuse the existing predicate's notion of "dev" exactly; do not redefine.
- The not-alive branch is unaffected by version state — assert this for at least one combo (e.g. not-alive + mismatch returns no-kill).
- The alive+read-error branch is conservative: any `readErr != nil && !errors.Is(readErr, ErrVersionFileAbsent)` triggers kill. Distinguish this from the alive+absent branch via `errors.Is`, not by string matching.
- A fake `BootstrapAliveCheck` and fake version-file reader installed via package-level vars + `t.Cleanup()` is the testing seam — confirm with the existing test idiom in `portal_saver_test.go` before introducing a new pattern.

**Context**:
> Spec §Change 1, decision matrix (verbatim):
>
> | Daemon alive? | Version file state | Versions match? | Action |
> |---|---|---|---|
> | Yes | (any) | Either side is `dev` or `""` | **Kill.** Dev-build short-circuit. |
> | Yes | Absent | (unknowable, neither dev) | **No kill.** Write daemon.version defensively from bootstrap, then proceed. |
> | Yes | Present, reads cleanly | Match (neither dev) | **No kill.** Proceed. |
> | Yes | Present, reads cleanly | Mismatch, neither dev (real upgrade) | **Kill.** Run `killSaverAndWaitForDaemon`, then BootstrapPortalSaver. |
> | Yes | Read error (non-absent I/O failure) | (unknowable) | **Kill.** Conservative. |
> | No | (any) | (any) | **No kill needed.** Proceed. |
>
> Spec §Change 1: "No new function signature. `EnsurePortalSaverVersion(c *Client, stateDir, currentVersion string) error` already takes `stateDir` as a parameter. `BootstrapAliveCheck` is the existing package-level variable (already wired to `state.DaemonAlive`) and is invoked with that same `stateDir`. No caller-side signature changes propagate from this fix."

**Spec Reference**: `.workflows/saver-kill-respawn-loop-leaks-daemons/specification/saver-kill-respawn-loop-leaks-daemons/specification.md` §Change 1

---

## saver-kill-respawn-loop-leaks-daemons-1-4 | approved

### Task saver-kill-respawn-loop-leaks-daemons-1-4: Defensive `WriteVersionFile(currentVersion)` on alive+absent branch before `BootstrapPortalSaver`

**Problem**: Lock-loser daemons exit cleanly before writing `daemon.version`, leaving the file observably absent until the next bootstrap repairs it. Without a defensive write on the alive+absent branch, the next bootstrap re-enters the same "alive + absent" state and we lose the audit trail (and a re-introduced bug elsewhere would silently re-trigger the kill cascade). The bootstrap-side write closes this lifecycle hole.

**Solution**: On the alive+absent branch newly introduced by Task 1-3, call `state.WriteVersionFile(stateDir, currentVersion)` before falling through to `BootstrapPortalSaver`. The value written is `currentVersion` — the same string `EnsurePortalSaverVersion` received as a parameter. No comparison against the actual running-daemon binary is performed; the alive-check has already determined health, and the user is responsible for an explicit recycle on intentional upgrades.

**Outcome**: After `EnsurePortalSaverVersion` returns on the alive+absent path, `daemon.version` exists synchronously and contains `currentVersion`. A failure of `WriteVersionFile` surfaces as an error returned from `EnsurePortalSaverVersion` (so the bootstrap step 4 warning machinery can record it). The defensive write does not race with the daemon's own write because, on the survived-daemon path, the daemon has already passed its own version-write point during its startup.

**Do**:
- In `internal/tmux/portal_saver.go`, modify the alive+absent branch added by Task 1-3 to call `state.WriteVersionFile(stateDir, currentVersion)` before proceeding to `BootstrapPortalSaver`.
- The call must occur **inside** `EnsurePortalSaverVersion`, **before** the function continues into `BootstrapPortalSaver`. Specifically, after the alive-check returns `true` and after `errors.Is(readErr, ErrVersionFileAbsent)` is observed.
- If `state.WriteVersionFile` returns an error, return that error from `EnsurePortalSaverVersion` wrapped with a descriptive message (e.g. `fmt.Errorf("defensive daemon.version write failed: %w", err)`). The bootstrap orchestrator step 4 will surface this as a `SaverDownWarning`-class warning — do not swallow it.
- The import of `internal/state.WriteVersionFile` is the same import path Task 1-2 modified; reuse it. If `internal/tmux/portal_saver.go` does not already import `internal/state`, add the import (the package boundary is established).
- Add or extend the unit test "alive=true, version absent (neither dev) → no kill, proceeds to BootstrapPortalSaver" from Task 1-3 to additionally assert:
  - `state.WriteVersionFile` was called with arguments `(stateDir, currentVersion)`.
  - The call happened **before** `BootstrapPortalSaver` was invoked (use a call-order recorder on injected fakes).
  - If `WriteVersionFile` returns an error, the error propagates out of `EnsurePortalSaverVersion` and `BootstrapPortalSaver` is **not** invoked.
- Use the package's existing seam pattern to inject a fake `WriteVersionFile` — search `internal/tmux/portal_saver.go` for a package-level `WriteVersionFile`-style variable; if one does not exist, introduce one (`var portalSaverWriteVersionFile = state.WriteVersionFile`) consistent with how `BootstrapAliveCheck` is already wired.

**Acceptance Criteria**:
- [ ] On the alive+absent branch of `EnsurePortalSaverVersion`, `state.WriteVersionFile(stateDir, currentVersion)` is invoked before `BootstrapPortalSaver`.
- [ ] A `WriteVersionFile` error propagates out of `EnsurePortalSaverVersion` and prevents `BootstrapPortalSaver` from being called on the same invocation.
- [ ] On a successful defensive write, `daemon.version` exists synchronously after `EnsurePortalSaverVersion` returns (the file's presence is observable from the immediately-following code path).
- [ ] No other branch (alive+dev, alive+match, alive+mismatch, alive+read-error, not-alive) calls the defensive write — only alive+absent.
- [ ] Unit tests assert the call ordering (WriteVersionFile → BootstrapPortalSaver) and the error-propagation path.
- [ ] `go test ./internal/tmux/...` is green.

**Tests**:
- `"alive=true + absent → defensive WriteVersionFile is called with (stateDir, currentVersion) before BootstrapPortalSaver"`
- `"alive=true + absent → WriteVersionFile error propagates and BootstrapPortalSaver is not invoked"`
- `"alive=true + match → defensive WriteVersionFile is NOT called"` (regression guard)
- `"alive=true + mismatch (neither dev) → defensive WriteVersionFile is NOT called; killSaverAndWaitForDaemon runs instead"` (regression guard)
- `"alive=true + dev (either side) → defensive WriteVersionFile is NOT called; kill barrier runs instead"`
- `"not-alive → defensive WriteVersionFile is NOT called"`

**Edge Cases**:
- **Pathological older-binary alive case**: when the alive daemon is running an older binary AND `daemon.version` was absent, the defensive write asserts the "going forward" version, not the daemon's actual version. Spec accepts this: "any disagreement is resolved by the next legitimate recycle." No special handling required — document this in a code comment at the call site.
- **Race with daemon's own write**: on the survived-daemon path the daemon has already passed its own startup-time write before bootstrap observes alive=true. The two writes do not overlap. Confirmed by the integration test in Task 1-6; this task does not introduce additional synchronization.
- **Write failure on read-only filesystem or ENOSPC**: must propagate as a wrapped error, not be swallowed. The bootstrap orchestrator's step 4 already classifies errors here as `SaverDownWarning`.

**Context**:
> Spec §Change 1: "Defensive complement: when bootstrap observes 'alive daemon + absent `daemon.version`' on the survived path, write `daemon.version` from the bootstrap side before proceeding. The string written is `currentVersion` — the same value `EnsurePortalSaverVersion` received as a parameter (injected at build time via `-ldflags -X github.com/leeovery/portal/cmd.version`). No comparison against the running daemon's actual binary is performed — the alive-check has already determined the daemon is healthy, and the user is responsible for an explicit recycle on intentional upgrades. In pathological cases where the alive daemon is running an older binary AND `daemon.version` was absent, the defensive write effectively asserts 'the version going forward' rather than the daemon's actual version; any disagreement is resolved by the next legitimate recycle. This closes the lock-loser lifecycle hole."
>
> Spec §Risk & Rollout: "The defensive `WriteVersionFile` from bootstrap on the 'alive + absent' path must not race with the daemon's own `WriteVersionFile`. The bootstrap write happens before `BootstrapPortalSaver` proceeds; the daemon's own write happens after lock acquisition. They don't overlap on the survived-daemon path because the daemon's already past that point. Confirm via the integration test."

**Spec Reference**: `.workflows/saver-kill-respawn-loop-leaks-daemons/specification/saver-kill-respawn-loop-leaks-daemons/specification.md` §Change 1 (Defensive complement)

---

## saver-kill-respawn-loop-leaks-daemons-1-5 | approved

### Task saver-kill-respawn-loop-leaks-daemons-1-5: Revise function comment at `internal/tmux/portal_saver.go:232-241` to match new contract

**Problem**: The function comment currently at `internal/tmux/portal_saver.go:232-241` explicitly encodes "`ErrVersionFileAbsent` counts as mismatch — for first-ever bootstrap or user-initiated state-dir cleanup" as intentional design. Tasks 1-3 and 1-4 invert that invariant: absence no longer drives the kill decision in isolation — the alive-check ordering is the authoritative gate. Leaving the stale comment in place is a future trap for the next reader.

**Solution**: Rewrite the comment block at `internal/tmux/portal_saver.go:232-241` to reflect the new contract: the predicate's `absent → true` verdict is still valid at the predicate layer, but `EnsurePortalSaverVersion` consults `BootstrapAliveCheck` first; the predicate is only consulted on the alive-with-readable-version branch.

**Outcome**: The comment block at `internal/tmux/portal_saver.go:232-241` (or wherever the function declaration sits after Task 1-3's refactor) reads as a faithful description of the post-fix contract. No code is changed in this task — comment edit only.

**Do**:
- Open `internal/tmux/portal_saver.go` and locate the comment block at lines 232-241 (or, if Task 1-3's edits have shifted the line range slightly, the comment block immediately preceding `portalSaverVersionMismatch` or `EnsurePortalSaverVersion` — whichever the existing comment describes).
- Read the existing comment to confirm which function it documents. (If both functions have comment blocks in the 232-241 range, identify the one that asserts "absent counts as mismatch" and target that one.)
- Rewrite the comment so it documents:
  - **What the predicate returns**: `true` if stored != current (real mismatch), OR if either side is `dev`/empty (dev short-circuit), OR if the read failed for any reason (including `ErrVersionFileAbsent`). `false` otherwise.
  - **Why absent returns true at the predicate layer**: the predicate's job is to answer "should the current daemon, if we kill it, be replaced by a fresh one whose version we trust?" — an absent file does not let us answer "no", so it answers "yes" defensively at the predicate layer.
  - **Why this is not load-bearing for the kill decision anymore**: `EnsurePortalSaverVersion` now gates the kill on `BootstrapAliveCheck(stateDir)` first. The predicate is only consulted when the daemon is alive AND the version file read returned without error. The alive-check ordering captures the broader invariant that a healthy daemon should never be killed for a missing version marker.
  - **Cross-reference**: point the reader at `EnsurePortalSaverVersion` for the authoritative kill-decision matrix.
- Keep the comment block godoc-style (`// ` prefix lines, attached directly to the function declaration with no blank line between).
- Do not change any code in this task. Comment-only edit.
- Run `go build -o portal .` and `go test ./internal/tmux/...` to confirm nothing accidentally regressed.

**Acceptance Criteria**:
- [ ] The comment block at the targeted location no longer asserts "absent counts as mismatch" as a load-bearing rule.
- [ ] The new comment documents the predicate's three-condition behaviour (real mismatch, dev short-circuit, read error including absence) AND explicitly states that `EnsurePortalSaverVersion` gates the kill on `BootstrapAliveCheck` first.
- [ ] The comment cross-references `EnsurePortalSaverVersion` so a future reader who lands on the predicate is directed to the authoritative matrix.
- [ ] `go build -o portal .` succeeds.
- [ ] `go test ./internal/tmux/...` is green.
- [ ] No code outside the comment block is modified.

**Tests**: None — this is a comment-only edit. Verification is by code review against the new contract.

**Edge Cases**: None.

**Context**:
> Spec §Change 1: "Update the existing function comment. The comment currently at `internal/tmux/portal_saver.go:232-241` explicitly encodes 'ErrVersionFileAbsent counts as mismatch — for first-ever bootstrap or user-initiated state-dir cleanup' as intentional design. That invariant is being inverted by Change 1 (alive-check ordering is what captures the broader invariant; the predicate no longer treats absence as mismatch in isolation). The comment must be revised to reflect the new contract, otherwise it becomes a future trap for the next reader."

**Spec Reference**: `.workflows/saver-kill-respawn-loop-leaks-daemons/specification/saver-kill-respawn-loop-leaks-daemons/specification.md` §Change 1

---

## saver-kill-respawn-loop-leaks-daemons-1-6 | approved

### Task saver-kill-respawn-loop-leaks-daemons-1-6: Integration test — alive daemon + absent `daemon.version` survives bootstrap (real-tmux fixture)

**Problem**: The unit tests added in Tasks 1-1, 1-3, and 1-4 pin the predicate matrix and the call ordering of `EnsurePortalSaverVersion`, but they do not pin the user-visible end-to-end contract: "on a real tmux server, with a real alive daemon and an absent `daemon.version`, bootstrap completes without firing the kill barrier and `_portal-saver` survives." Without this integration test, the user-visible Defect 1 contract is unprotected — a future refactor could re-introduce the cascade and unit tests would not catch it.

**Solution**: Add a real-tmux integration test (using the existing `tmuxtest` socket fixture) that: (a) brings up a clean `_portal-saver` with a live daemon, (b) deletes `daemon.version` from the state dir, (c) runs the bootstrap path (or directly invokes `EnsurePortalSaverVersion` followed by `BootstrapPortalSaver`), (d) asserts the three "kill cascade" WARN lines are absent from `portal.log`, `_portal-saver` is alive, `daemon.version` is present and equal to `currentVersion`, and exactly one daemon PID is running and matches the `daemon.lock` holder.

**Outcome**: A new integration test in the existing real-tmux integration test file (likely `internal/tmux/portal_saver_integration_test.go` — confirm the exact filename by grep) that exercises the alive+absent path against a real tmux socket and pins the post-fix user-visible behaviour. The existing "kill-respawn under explicit version mismatch" test in the same file remains green.

**Do**:
- Locate the existing real-tmux integration test file for `portal_saver` (search `internal/tmux/` for `*_integration_test.go` referencing `portal_saver` or `BootstrapPortalSaver`). Use the `tmuxtest` package's real-tmux socket fixture as the harness.
- Add a new test function (suggested name: `TestEnsurePortalSaverVersion_AliveAndVersionAbsent_NoKill`) that follows the same setup/teardown pattern as the existing tests in that file. Do **not** use `t.Parallel()`.
- Test setup:
  1. Allocate a fresh state dir under `t.TempDir()`.
  2. Bootstrap `_portal-saver` via `BootstrapPortalSaver` (or the existing test helper that does so) so a real `portal state daemon` is running and holds `daemon.lock`.
  3. Poll until `state.DaemonAlive(stateDir)` returns true (with a sensible ceiling, e.g. 5s).
  4. Confirm `daemon.version` is present (the daemon's own startup write), then **delete it** via `os.Remove`.
  5. Capture the current daemon PID via `state.DaemonAlive` companion (or `lsof` on `daemon.lock`).
- Test action:
  - Invoke `EnsurePortalSaverVersion(client, stateDir, currentVersion)` where `currentVersion` is a non-dev, non-empty string (e.g. `"0.5.0-test"`).
- Test assertions:
  1. The call returns nil.
  2. `tmux has-session -t _portal-saver` returns success against the test fixture's socket (use `client.HasSession` or equivalent).
  3. `daemon.version` exists on disk and equals `currentVersion` exactly.
  4. The daemon PID after the call equals the PID captured before — same process, not a respawn.
  5. `state.DaemonAlive(stateDir)` returns true.
  6. The three WARN lines are absent from `portal.log`:
     - `prior daemon (pid=` substring
     - `another daemon holds the lock; exiting` substring
     - `step 4 (EnsureSaver) failed:` substring
     (Capture `portal.log` output via the package's existing log-capture seam, or scan the file directly under `stateDir`.)
  7. `pgrep -f "portal state daemon"` (or the platform-agnostic equivalent — count processes whose argv0 matches `portal state daemon` against the test fixture's process group) returns exactly one PID, equal to the PID from assertion 4.
- Teardown:
  - `tmux kill-server` on the test socket via `t.Cleanup()`.
  - Verify the daemon process exits cleanly (best-effort — do not fail teardown on exit-latency outliers, but log the latency).
- Confirm the existing `portal_saver_integration_test.go` "kill-respawn under explicit version mismatch" test still passes. If the file's existing helpers need extension (e.g. to delete `daemon.version` between steps), extend rather than duplicate.

**Acceptance Criteria**:
- [ ] A new integration test function exists in `internal/tmux/portal_saver_integration_test.go` (or the equivalent existing real-tmux integration test file) named clearly to indicate the alive+absent scenario.
- [ ] The test uses the existing `tmuxtest` real-tmux socket fixture, not a mock client.
- [ ] The test does not use `t.Parallel()`.
- [ ] All assertions listed in the "Do" section pass on a clean run.
- [ ] The existing "kill-respawn under explicit version mismatch" test in the same file remains green.
- [ ] `go test ./internal/tmux/...` is green end-to-end.
- [ ] The test cleans up its `_portal-saver` session and daemon process via `t.Cleanup()`.

**Tests**:
- `"alive daemon + daemon.version absent → bootstrap completes without firing kill barrier"` — happy path, primary assertion set.
- `"alive daemon + daemon.version absent → daemon PID is unchanged after EnsurePortalSaverVersion"` — regression guard against silent respawn.
- `"alive daemon + daemon.version absent → daemon.version is repaired and contains currentVersion"` — pins the defensive write from Task 1-4.
- `"alive daemon + daemon.version absent → three WARN lines are absent from portal.log"` — pins the user-visible Defect 1 contract.
- `"pgrep returns exactly one daemon PID matching the daemon.lock holder"` — pins acceptance criterion 3 from the spec's steady-state section.

**Edge Cases**:
- **Single live daemon PID matches `daemon.lock` holder**: use either `lsof daemon.lock` (if available on darwin/linux test runners) or rely on the in-process `state.AcquireDaemonLock` flock semantics — only one process can hold the flock at a time, so identifying "the daemon process whose PID is recorded in `daemon.pid`" is sufficient. Cross-check `daemon.pid` against the PID found by enumerating processes matching `portal state daemon`.
- **Three WARN lines absent from `portal.log`**: scan the file for each substring independently; assert each is absent. Do not assert the file is empty (the daemon may emit unrelated INFO/DEBUG lines).
- **Existing kill-respawn-under-explicit-mismatch integration test stays green**: run it as part of the same `go test ./internal/tmux/...` invocation and confirm. If it requires modification to coexist with the new test (e.g. shared fixture helpers), extend the helpers — do not weaken existing assertions.
- **Real-tmux fixture availability**: this test requires a working tmux binary at test-run time. If the existing real-tmux integration tests are guarded by a build tag or env var (e.g. `//go:build integration` or `TMUX_INTEGRATION=1`), apply the same guard.
- **`portal.log` location**: confirm where the test fixture writes `portal.log` — likely under `stateDir`. If the log file does not exist (no warnings ever emitted), assertion 6 trivially holds; do not fail when the file is absent.

**Context**:
> Spec §Testing Requirements > Integration tests, item 1: "'Alive daemon, daemon.version absent, versions match' → bootstrap completes without firing the kill barrier. `_portal-saver` survives; `daemon.version` is present and correct post-bootstrap. Pins Defect 1's user-visible contract."
>
> Spec §Acceptance Criteria > Steady-state bootstrap, items 1-4: "No kill-respawn on healthy bootstrap … `_portal-saver` survives bootstrap … Single live daemon, no orphans … `daemon.version` is repaired defensively on the survived-daemon path."

**Spec Reference**: `.workflows/saver-kill-respawn-loop-leaks-daemons/specification/saver-kill-respawn-loop-leaks-daemons/specification.md` §Testing Requirements > Integration tests #1, §Acceptance Criteria > Steady-state bootstrap
