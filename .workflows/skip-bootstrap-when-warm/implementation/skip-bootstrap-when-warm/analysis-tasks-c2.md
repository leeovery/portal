---
topic: skip-bootstrap-when-warm
cycle: 2
total_proposed: 2
---
# Analysis Tasks: Skip Bootstrap When Warm (Cycle 2)

## Task 1: Simplify runHookStaleCleanup to its post-step-11 usage — drop the dead swallowListError axis and fix the stale contract doc
status: pending
severity: medium
sources: architecture

**Problem**: This feature removed bootstrap step 11 (the CleanStale hooks step and its `cleanStaleAdapter`) and re-homed hooks cleanup on the daemon. That removal deleted the only caller of `runHookStaleCleanup` (cmd/run_hook_stale_cleanup.go) that passed `swallowListError=false`. Both surviving production callers — the daemon's `maybeRunHookCleanup` (cmd/state_daemon.go:426) and portal-clean (cmd/clean.go:155) — pass `true`. The boolean policy axis therefore no longer varies in production: its `false` short-circuit (`return err`) is unreachable from any production path and is exercised only by run_hook_stale_cleanup_test.go passing itself `false`. This leaves a boolean parameter (a flagged anti-pattern) plus a dead error-return branch as vestigial complexity. Compounding this, the helper's contract doc (lines 3-53 and 61-77) still describes its callers as "bootstrap step 11 (cleanStaleAdapter.CleanStale)" and the portal-clean RunE and frames the `swallowListError` axis around "the bootstrap adapter passes false ... escalates the err up through the StaleCleaner interface" — a caller and interface that no longer exist. The daemon, now one of the two live consumers of this load-bearing shared seam, is not mentioned anywhere in the contract, so a maintainer is pointed at removed code and left unaware of the daemon dependency.

**Solution**: Collapse the seam to match its single remaining policy variant: remove the `swallowListError bool` parameter, delete the now-dead `return err` branch (the ListAllPanes-error path always logs the Warn and returns nil), update the two live callers, correct the contract documentation to name the current callers (daemon `maybeRunHookCleanup` + portal-clean `cleanCmd.RunE`) and record the daemon as a consumer, and retune the unit tests that pinned the `false`/`true` axis.

**Outcome**: `runHookStaleCleanup` has no boolean policy parameter and no production-dead branch; both production callers compile against the simplified signature; the contract doc accurately describes its two current callers (including the daemon) and no longer references the removed step-11 adapter or `StaleCleaner` interface; the full suite is green.

**Do**:
1. In cmd/run_hook_stale_cleanup.go, remove the `swallowListError bool` parameter from the `runHookStaleCleanup` signature. In the ListAllPanes-error branch (currently lines 90-96), keep the `logger.Warn("stale-hook cleanup: list-panes failed", "error", err)` breadcrumb and replace the `if swallowListError { return nil } return err` with an unconditional `return nil`. Leave the `store.Load` error branch (`return err`) and the `store.CleanStale` error branch (`return err`) untouched — the finding scopes the dead axis to ListAllPanes errors only; the destructive-branch error propagation is unchanged.
2. Update cmd/state_daemon.go:426 — `maybeRunHookCleanup` now calls `runHookStaleCleanup(deps.Client, deps.HookStore, deps.Logger, nil)` (drop the `true`). Keep the surrounding `if err := ...; err != nil { deps.Logger.Warn("hooks stale-cleanup failed", "error", err) }` guard; the helper still returns non-nil on Load/CleanStale errors.
3. Update cmd/clean.go:155 — remove the `true` argument from the `runHookStaleCleanup(buildCleanPaneLister(), hookStore, logger, func(paneID string){...})` call; the trailing `onRemoved` stdout writer and the `_ =` discard stay as-is.
4. Rewrite the contract documentation in cmd/run_hook_stale_cleanup.go (the package-doc-style block lines 3-53 and the function doc lines 61-77). Remove the `swallowListError` policy-axis paragraph and every reference to "bootstrap step 11", "cleanStaleAdapter.CleanStale", the `StaleCleaner` interface, and "escalates the err up". State that the helper is the shared implementation of the daemon's throttled hook cleanup (`maybeRunHookCleanup`) and the portal-clean hook-cleanup tail (`cleanCmd.RunE`), that both callers treat a ListAllPanes error as Warn-and-continue (return nil), and that Load/CleanStale errors still propagate. Keep the `onRemoved` (nil-tolerant callback) and nil-logger paragraphs.
5. In cmd/run_hook_stale_cleanup_test.go, drop the `false`/`true` argument from every call and update the case that pinned the dead `false → return err` branch (around lines 111-117, the "want error, got nil" assertion on a ListAllPanes error): it must now assert the helper returns nil after emitting the list-panes Warn. Fold or remove the separate `swallowListError=true` case (around lines 146-152) since there is now a single behaviour. Preserve the Load-error and CleanStale-error cases (they still expect a non-nil return) and the nil-logger case (line 304).
6. Confirm no other caller passes `false` (grep verified: only the test file did). Do not touch the `hooks_cleanstale_single_caller_guard_test.go` invariant — the single-caller guard is unrelated to the parameter change.

**Acceptance Criteria**:
- `runHookStaleCleanup` no longer accepts a `swallowListError` parameter and contains no `return err` on the ListAllPanes-error path.
- Both production callers (cmd/state_daemon.go, cmd/clean.go) compile and behave identically to before (ListAllPanes errors logged-and-swallowed; Load/CleanStale errors still surfaced to the caller's own Warn/return).
- The contract doc names the daemon `maybeRunHookCleanup` and portal-clean `cleanCmd.RunE` as the two callers and contains no reference to step 11, `cleanStaleAdapter`, or `StaleCleaner`.
- `go build ./...` succeeds and `go test ./cmd/...` is green.

**Tests**:
- Retuned unit test: a ListAllPanes error now results in a nil return plus the "stale-hook cleanup: list-panes failed" Warn (assert the log breadcrumb, assert nil error).
- Retained unit tests: Load error → non-nil return; CleanStale error → non-nil return; mass-deletion hazard guard (zero live panes + non-empty persisted) → nil return with hazard Warn; both-empty → nil; happy path with `onRemoved` invoked once per removed key; nil-logger tolerated.
- `go test ./cmd/...` (the daemon hook-cleanup and portal-clean integration/log-fingerprint tests must still pass unchanged).

## Task 2: Log the underlying error on abridged saver revive failure to restore diagnosability parity
status: pending
severity: low
sources: standards

**Problem**: On the abridged (warm) path, when `tmux.BootstrapPortalSaver` fails to revive an absent `_portal-saver`, `ensureSaverLiveness` (cmd/abridged_saver.go:48-50) drops the returned error entirely and surfaces only the canned `bootstrap.SaverDownWarning()` via the warning sink. The helper emits no WARN and takes no logger. The full-bootstrap sibling step logs the same class of failure with the actual cause (cmd/bootstrap/bootstrap.go, `o.Logger.Warn("step failed", "step", stepEnsureSaver, "error", err)`), and the project's logging discipline (CLAUDE.md: saver/daemon lifecycle "have closed event catalogs"; every bootstrap failure path is an observable WARN carrying its cause) treats such failures as observable. Net effect: on a warm command an operator sees only the generic "Portal save daemon failed to start" notice with zero detail in portal.log about why the revive failed — a diagnosability regression relative to every other bootstrap failure path and relative to the full-bootstrap EnsureSaver this helper is derived from. The spec's "Abridged EnsureSaver hard-failure" section is silent on logging, so the current behaviour is literally spec-conformant but diverges from established convention.

**Solution**: Emit one WARN carrying the underlying error on the `BootstrapPortalSaver` error branch before adding the `SaverDownWarning`, mirroring the full-bootstrap step-5 WARN, using the existing package-level bootstrap component logger. Keep the warning-sink funnel and the no-error-return failure posture exactly as-is.

**Outcome**: When the abridged saver revive fails, portal.log carries a bootstrap-component WARN with the underlying error (matching the full-bootstrap sibling), while the user-facing `SaverDownWarning` and the proceed-anyway control flow are unchanged.

**Do**:
1. In cmd/abridged_saver.go, on the `if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil` branch (lines 48-50), before `bootstrapWarnings.Add(bootstrap.SaverDownWarning())`, emit a WARN via the package-level `bootstrapLogger` (`log.For("bootstrap")`, declared in cmd/state_common.go:22) carrying the underlying error — e.g. `bootstrapLogger.Warn("abridged EnsureSaver: saver revive failed", "error", err)`. Match the attr shape of the full-bootstrap sibling (an `"error"` attr keyed to the closed vocabulary). Use `bootstrapLogger` directly (it is package-level and already used by run_hook_stale_cleanup.go and clean.go); do not add a logger parameter unless a unit test requires injection.
2. Update the "Failure posture" doc paragraph in the `ensureSaverLiveness` doc comment (lines 28-33) to note that a revive failure now also emits a bootstrap-component WARN with the cause, in addition to funneling the `SaverDownWarning` into the sink.
3. Do not change the presence-probe early-return, the warning-sink funnel, or the no-error-return posture — only add the WARN breadcrumb.

**Acceptance Criteria**:
- A failed `BootstrapPortalSaver` on the abridged path emits exactly one bootstrap-component WARN carrying the underlying error before adding the `SaverDownWarning`.
- The `SaverDownWarning` sink behaviour and the command-proceeds-anyway control flow are unchanged (still no error return).
- The successful-presence early return emits no WARN.
- `go build ./...` succeeds and `go test ./cmd/...` is green.

**Tests**:
- Unit test: with a commander/tmux double where `BootstrapPortalSaver` fails, assert the bootstrap-component WARN with the "error" attr is captured (via the log capture sink) AND the `SaverDownWarning` is added to `bootstrapWarnings` AND `ensureSaverLiveness` returns without surfacing an error.
- Unit test: with a live/present `_portal-saver`, assert no WARN is emitted and no warning is added (early return path).
