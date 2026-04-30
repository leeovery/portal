---
phase: 2
phase_name: Rename Bootstrap Session to `_portal-bootstrap`
total: 3
---

## hidden-sessions-showing-on-startup-2-1 | approved

### Task 2-1: Add `PortalBootstrapName` constant, rename bootstrap session in `StartServer`, and refresh doc-comment

**Problem**: `internal/tmux.Client.StartServer` currently invokes `tmux new-session -d` with no `-s` flag (`internal/tmux/tmux.go:175-181`), so tmux assigns the default numeric name (typically `0`). Phase 1 shipped a chokepoint `_*`-prefix filter inside `Client.ListSessions`, but the bootstrap session has no underscore prefix and therefore still leaks through to the TUI picker and `portal list`. This task closes Root Cause 2 by giving the bootstrap session a reserved underscore-prefixed name keyed off a new exported constant. The `tmux.StartServer` doc-comment (`internal/tmux/tmux.go:169-174`) also encodes the stale tmux-resurrect / tmux-continuum rationale that motivated the original unnamed session and MUST be refreshed in the same change so the doc-comment reflects post-fix reality.

**Solution**: Add a package-level exported constant `PortalBootstrapName = "_portal-bootstrap"` to `internal/tmux` (sibling to `PortalSaverName` at `internal/tmux/portal_saver.go:7-13`). Update `Client.StartServer` to invoke `tmux new-session -d -s` referenced via the constant. Update the existing `TestStartServer` in `internal/tmux/tmux_test.go` to assert the args passed to `Commander.Run` include both `-s` and the value of `PortalBootstrapName`. Rewrite the `tmux.StartServer` doc-comment per the spec's five-point checklist: drop the tmux-resurrect cleanup claim, reframe the keep-server-alive rationale around Portal's own `Restore` step (bootstrap step 5), document the reserved name, document that the session is hidden by `Client.ListSessions`'s underscore-prefix filter, and retain the `exit-empty on` justification for `new-session -d` over `start-server` (commit `bd659a3`).

**Outcome**: `Client.StartServer` creates a session named `_portal-bootstrap` (referenced exclusively via `PortalBootstrapName`), the existing `TestStartServer` asserts the new args, the doc-comment accurately describes the post-fix invariants without referencing tmux-resurrect or tmux-continuum, and `go test ./...` is green. No literal `"_portal-bootstrap"` string appears anywhere in production code or tests — every reference goes through the constant.

**Do**:
- Add `PortalBootstrapName = "_portal-bootstrap"` as an exported `const` at the top of `internal/tmux/portal_saver.go` (or a new file in `internal/tmux` if preferred — sibling placement to `PortalSaverName` is the spec's stated convention; co-locating in `portal_saver.go` keeps both reserved-name constants discoverable in one file). Include a doc-comment that mirrors `PortalSaverName`'s style: state the underscore prefix and its meaning, and reference `Client.ListSessions` as the chokepoint that hides it.
- Update `Client.StartServer` at `internal/tmux/tmux.go:175-181` to call `c.cmd.Run("new-session", "-d", "-s", PortalBootstrapName)` instead of `c.cmd.Run("new-session", "-d")`.
- Rewrite the `StartServer` doc-comment at `internal/tmux/tmux.go:169-174` per the spec's five-point checklist (see Context block below). The replacement comment MUST: (1) drop the `tmux-resurrect recognizes and cleans up` sentence entirely; (2) drop or reframe `plugins like tmux-continuum can restore saved sessions` so Portal's own `Restore` step (bootstrap step 5) is the documented beneficiary of keeping the server alive; (3) state that the session is created with the reserved name `PortalBootstrapName` (`_portal-bootstrap`); (4) state that the session is hidden from user-facing listings by the underscore-prefix filter in `Client.ListSessions`; (5) retain the `exit-empty on` rationale for `new-session -d` over `start-server`, reframed against Portal's own resurrection.
- Locate the existing `TestStartServer` in `internal/tmux/tmux_test.go` (currently at line 404 onwards; `TestListSessions` and `MockCommander` live earlier in the same file at lines 12-100+). Update it to assert that the recorded `Commander.Run` invocation contains the args `-s` and `tmux.PortalBootstrapName` in addition to the existing `new-session -d` assertion. Reference the value via the exported constant — do NOT compare against the literal `"_portal-bootstrap"` string.
- Run `go build ./...` to confirm no other production code depended on the previous unnamed-session shape, then `go test ./...` to confirm green across the whole module.

**Acceptance Criteria**:
- [ ] `PortalBootstrapName = "_portal-bootstrap"` is defined as an exported package-level `const` in `internal/tmux` (sibling to `PortalSaverName`).
- [ ] `Client.StartServer` invokes `c.cmd.Run("new-session", "-d", "-s", PortalBootstrapName)` — the constant is the sole reference to the reserved name in production code.
- [ ] No literal `"_portal-bootstrap"` string appears anywhere in the codebase (production code or tests). A repo-wide search for the literal returns only the constant definition itself.
- [ ] `TestStartServer` in `internal/tmux/tmux_test.go` asserts the args passed to `Commander.Run` include `-s` and `tmux.PortalBootstrapName` (referenced as the constant).
- [ ] `tmux.StartServer` doc-comment no longer mentions `tmux-resurrect` or `tmux-continuum`; instead references Portal's own `Restore` step (bootstrap step 5), the reserved name `PortalBootstrapName`, the chokepoint hiding via `Client.ListSessions`, and retains the `exit-empty on` rationale for `new-session -d`.
- [ ] `go test ./...` is green.
- [ ] `StartServer`'s precondition (only called when no tmux server is running, per the existing `ServerRunning` gate in `cmd/root.go`) is preserved — no new precondition handling is added; the duplicate-session failure mode against an already-running server remains explicitly out of scope.

**Tests**:
- `TestStartServer` (updated): `"it invokes new-session -d -s with the PortalBootstrapName constant value"` — drives `Client.StartServer` with a `MockCommander` and asserts `m.Calls[0]` contains exactly `["new-session", "-d", "-s", tmux.PortalBootstrapName]`.
- `TestStartServer` (updated): `"it returns a wrapped error when Commander.Run fails"` — preserve the existing error-wrapping assertion (if present) so the change is purely additive on the args side.
- Manual verification by running `go test ./...` and `go build ./...` to confirm no other call site or test depended on the old unnamed-session shape.

**Edge Cases**:
- Every reference to the reserved name MUST route through the constant (no literal string in code or tests). A future contributor copy-pasting the literal would break the spec's "single source of truth" invariant — code review is the enforcement, but the absence of any literal in the current commit is a precondition for the rule to be enforceable.
- `StartServer` is contracted to be called only when no tmux server is running (existing `ServerRunning` gate). Behaviour against a server that already hosts a `_portal-bootstrap` session is undefined and out of scope — `tmux new-session -d -s _portal-bootstrap` would fail with "duplicate session" in that case. Do NOT add a fallback or precondition check.
- `exit-empty` default-on opportunistic reaping is retained as a nice-to-have, not a correctness requirement. The doc-comment must frame the bootstrap session as the keep-server-alive anchor when `Restore` produces no user sessions; the user is free to override `exit-empty off` and the reaping silently no-ops, with Phase 1's filter still hiding the session from view.
- The constant's doc-comment should mirror `PortalSaverName`'s: state the underscore prefix and its purpose, reference `Client.ListSessions` as the chokepoint. Avoid claiming filter behaviour at any other layer (e.g. per-consumer filters in TUI / `cmd/list.go`) — Phase 1 deliberately centralised filtering at the chokepoint.

**Context**:

> **Fix B — Behaviour Contract** (specification § Fix B → Behaviour Contract):
> `internal/tmux.Client.StartServer` MUST create the bootstrap session with an explicit underscore-prefixed name. The chosen name is `_portal-bootstrap`. The implementation invokes `tmux new-session -d -s _portal-bootstrap` instead of the current `tmux new-session -d`. The reserved name MUST be exposed as an exported package-level constant in `internal/tmux` (sibling to `PortalSaverName`), e.g. `PortalBootstrapName = "_portal-bootstrap"`. **All references to the reserved name — production code (`StartServer`'s tmux args), tests, and any future diagnostic tooling — MUST go through the constant.** No literal `"_portal-bootstrap"` string elsewhere in the codebase.

> **Doc-comment cleanup — `tmux.StartServer`** (specification § Doc-Comment Cleanup → `tmux.StartServer`): After Fix B, the comment MUST: drop the tmux-resurrect cleanup claim entirely; drop or reframe the "plugins like tmux-continuum can restore saved sessions" wording — Portal's own bootstrap step 5 (`Restore`) is now the beneficiary of keeping the server alive, not external plugins; document that the session is created with the reserved name `PortalBootstrapName` (`_portal-bootstrap`); document that the session is hidden from user-facing listings by the underscore-prefix filter in `Client.ListSessions`; retain the `exit-empty on` rationale for using `new-session -d` rather than `start-server` (this is still load-bearing — commit `bd659a3`) — but framed against Portal's own `Restore` step.

> **Test mandate** (specification § Test Requirements → "Unit — `StartServer` Uses Reserved Bootstrap Name"): Update the existing `TestStartServer` in `internal/tmux/tmux_test.go` to assert the args passed to `Commander.Run` include `-s _portal-bootstrap` (referenced via the `PortalBootstrapName` constant, not the literal string). This prevents accidental regression to an unnamed session.

> **Lifecycle framing** (specification § Fix B → Lifecycle After The Rename): Portal does not explicitly set or modify tmux's `exit-empty` option. When `exit-empty` is at its tmux default (`on`), the server exits when its last session closes — Portal benefits from this naturally. When the user has overridden `exit-empty off`, `_portal-bootstrap` may persist indefinitely, but Fix A's filter still hides it from view, so the user never sees it. Reaping is therefore a nice-to-have, not a correctness requirement.

> **Sole production caller verified** (specification § Fix B → Sole Production Caller Verified): `StartServer` is the only call site in production code that invokes `tmux new-session` without `-s`. Once Fix B lands there is no remaining code path that produces an unnamed (and therefore numerically-defaulted) session.

> **Convention precedent** (specification § Doc-Comment Cleanup → Convention Precedent): Existing tests already follow the underscore-prefix convention for seeding sessions (`internal/restore/integration_test.go:280` uses `_seed`; `cmd/bootstrap/reboot_roundtrip_test.go:236, 319` uses `_seed`). Test-bench code already demonstrates the pattern works; production was the outlier.

> **Rollout** (specification § Rollout): Phase 2 ships as a single targeted commit (Fix B + `TestStartServer` update + `PortalBootstrapName` constant + doc-comment cleanup + the end-to-end test from task 2-2) per the Rollout section, reviewed together with Phase 1's commit as a pair. The end-to-end test in task 2-2 ships in this same commit (or a third commit landing after both fixes), never with Phase 1.

**Spec Reference**: `.workflows/hidden-sessions-showing-on-startup/specification/hidden-sessions-showing-on-startup/specification.md` § Fix B (Behaviour Contract, Why Rename Instead Of Kill, Lifecycle After The Rename, Naming Constraint, Sole Production Caller Verified) and § Doc-Comment Cleanup (`tmux.StartServer`, Convention Precedent) and § Test Requirements (Unit — `StartServer` Uses Reserved Bootstrap Name) and § Rollout.

## hidden-sessions-showing-on-startup-2-2 | approved

### Task 2-2: Extend `reboot_roundtrip_test.go` with raw-tmux and `ListSessions` post-bootstrap assertions

**Problem**: The bug shipped in part because the `built-in-session-resurrection` planning's review phase scored implementation against the explicit task list rather than against an end-to-end UX walk-through. There is currently no automated regression guard that asserts on the *post-bootstrap* session list — neither at the raw tmux layer (which would catch a regression of Fix B where `0` reappears) nor at the user-facing layer (which would catch a regression of Fix A where reserved names leak through `Client.ListSessions`). Without both assertions in one place, a future change that drops the `-s` arg from `StartServer` would silently regress Root Cause 2: the bootstrap session would default to `0`, which has no `_*` prefix and would therefore still be filtered into invisibility for `ListSessions` callers — masking the regression. This task ships the spec-mandated end-to-end regression guard.

**Solution**: Extend `cmd/bootstrap/reboot_roundtrip_test.go` (the real-tmux fixture path under the `//go:build integration` tag) with two post-bootstrap assertions executed after the orchestrator's `Run` completes against the real tmux server. Assertion 1 reads tmux's raw session list directly via `tmux list-sessions -F '#{session_name}'` (using the existing `ts.Run` / `ts.TryRun` test helper that shells to tmux on the fixture's socket) and asserts the set is a subset of `{PortalBootstrapName, PortalSaverName, <expected restored sessions>}` with no `0` and no other unexpected names. Assertion 2 calls `client.ListSessions()` and asserts the returned slice contains exactly the expected restored user sessions (`alpha`, `beta`) with both reserved names excluded. Both reserved names are referenced via the package constants — no literal strings.

**Outcome**: `cmd/bootstrap/reboot_roundtrip_test.go` ships two new post-bootstrap assertions that together regression-guard both root causes. Assertion 1 fails meaningfully if Fix B regresses (e.g. `-s` arg silently dropped → `0` reappears in raw tmux output). Assertion 2 fails meaningfully if Fix A regresses (e.g. underscore-prefix filter removed from `Client.ListSessions` → reserved names leak through). The integration test is green when both fixes are in place. The test ships in the same commit as task 2-1 (or a third commit landing after both fixes), never with Phase 1.

**Do**:
- Open `cmd/bootstrap/reboot_roundtrip_test.go` and locate the primary round-trip body `runRebootRoundTrip` (starts at line 174). The new assertions belong AFTER `o.Run(context.Background())` returns (line 354) and BEFORE the existing `verifyLiveStructure` / `verifyLayoutAndZoom` block (line 367) — they assert the post-bootstrap session-set invariant, which is conceptually adjacent to `verifyLiveStructure`'s topology check but operates at the session-name layer rather than the pane layer.
- Add a new helper `verifyPostBootstrapSessionSet(t *testing.T, ts *tmuxtest.Socket, client *tmux.Client, expectedRestored []string)` near the other `verify*` helpers in the same file. The helper performs two assertions:
  - **Assertion 1 (Root Cause 2 guard — raw tmux state):** call `out := ts.Run(t, "list-sessions", "-F", "#{session_name}")` to read the raw session list directly from tmux. Split the output into a set of names. Assert the set is a subset of `{tmux.PortalBootstrapName, tmux.PortalSaverName} ∪ expectedRestored`. Specifically: assert the set does NOT contain `"0"` and does NOT contain any name not in the allowed superset. The wording of the failure message MUST identify which unexpected name appeared so a regression of Fix B is diagnosable at a glance. **This assertion MUST NOT use `client.ListSessions()`** — using `ListSessions` would re-test the chokepoint filter and would NOT catch a regression of Root Cause 2 (because `0` has no `_*` prefix it would still be filtered into invisibility for `ListSessions` callers, masking the regression).
  - **Assertion 2 (Root Cause 1 guard — user-facing visibility):** call `sessions, err := client.ListSessions()`. Assert no error. Build a set of the returned session names and assert it equals exactly `expectedRestored` (no more, no less). Specifically: assert `tmux.PortalBootstrapName` and `tmux.PortalSaverName` are NOT in the returned slice. The wording of the failure message MUST identify which reserved name leaked so a regression of Fix A is diagnosable at a glance.
- Reference both reserved names via `tmux.PortalBootstrapName` and `tmux.PortalSaverName` — no literal `"_portal-bootstrap"` or `"_portal-saver"` strings.
- Call `verifyPostBootstrapSessionSet(t, ts, client, []string{"alpha", "beta"})` from `runRebootRoundTrip` after `o.Run` returns and before `verifyLiveStructure`. The two restored sessions in the round-trip body are `alpha` and `beta` (built by `createSavedTopology` at line 435 and verified at `verifyCapturedIndex` line 533).
- Decide whether to also extend `TestPhase5RebootRoundTripBothSessionsHydrateViaSignalHydrateBinary` (line 765 onwards) with the same helper call. The spec mandates the assertion lives in `reboot_roundtrip_test.go` — the primary `runRebootRoundTrip` covers both default-indices and base-index-drift sub-tests via the shared body, so wiring the helper into `runRebootRoundTrip` covers both. The `BothSessionsHydrate...` sub-test exercises the same orchestrator wiring against the same real-tmux fixture, so adding the helper call there as well costs little and doubles the regression coverage. Default to wiring it into both: `runRebootRoundTrip` (covers `TestPhase5RebootRoundTripEndToEnd` and `TestPhase5RebootRoundTripBaseIndexDrift`) and the standalone `TestPhase5RebootRoundTripBothSessionsHydrateViaSignalHydrateBinary`.
- Note: the round-trip body uses `bootstrap.NoOpSaver{}` (line 344, 833), which means `_portal-saver` is NOT created during the test. The `expectedRestored` set therefore must NOT include `_portal-saver`, and Assertion 1's allowed superset must be `{PortalBootstrapName} ∪ expectedRestored` in *this test's* execution context — but the assertion's *contract* is "subset of `{PortalBootstrapName, PortalSaverName, <expected restored sessions>}`", and the runtime allowed-set narrows by virtue of `_portal-saver` not being created. Implement the helper to take the allowed-superset as the union of `{PortalBootstrapName, PortalSaverName}` and `expectedRestored` — even though `PortalSaverName` is not present in this test run, future variants (or a real production run) may have it, and the helper's assertion is "subset of the allowed superset", which tolerates absence. The strict equality piece is that the set of UNEXPECTED names is empty.
- Note: the round-trip body uses a `_seed` bootstrap session created at line 319 (`ts.Run(t, "new-session", "-d", "-s", "_seed")`) BEFORE the orchestrator runs. `_seed` is underscore-prefixed and therefore in the allowed-superset by virtue of starting with `_` — but the helper's allowed-superset as written above does NOT include `_seed`. The fixture's `_seed` session is part of the test's bootstrap discipline (mirrors the daemon's `_portal-saver` discipline), and after the orchestrator runs `_seed` may still be present. Decision: include `_seed` in the allowed-superset by passing it as an additional reserved-name argument, OR widen the helper's allowed-superset to include any name starting with `_`. Pick the explicit-allowlist approach (pass `_seed` as an additional reserved name) so the helper's contract stays strict — a regression that introduces a new unexpected `_*` name (e.g. a typo) would still fail, where a `HasPrefix("_")` allowance would silently tolerate it. Update the helper signature to `verifyPostBootstrapSessionSet(t *testing.T, ts *tmuxtest.Socket, client *tmux.Client, allowedReserved []string, expectedRestored []string)` and pass `[]string{tmux.PortalBootstrapName, tmux.PortalSaverName, "_seed"}` for the round-trip body. **Do not** parameterise `_seed` as a constant in production code — it is a test-only seed name.
- Run `go test -tags=integration ./cmd/bootstrap/...` to verify the new assertions pass against the real-tmux fixture path. The test is gated by the `//go:build integration` tag (file line 1) and `testing.Short()` (e.g. line 122), so a default `go test ./...` does NOT exercise it — but the standard CI lane that runs the integration tag MUST be green.
- Run `go test ./...` (without the integration tag) to confirm the change does not break the default test surface.

**Acceptance Criteria**:
- [ ] `cmd/bootstrap/reboot_roundtrip_test.go` contains a new helper `verifyPostBootstrapSessionSet` (or equivalently named) that performs both Assertion 1 and Assertion 2.
- [ ] Assertion 1 reads tmux's raw session list directly via `ts.Run(t, "list-sessions", "-F", "#{session_name}")` (or equivalent test-only path that shells to tmux) — and **does NOT** call `client.ListSessions()` for this assertion. The failure message identifies which unexpected name appeared.
- [ ] Assertion 2 calls `client.ListSessions()` and asserts both `tmux.PortalBootstrapName` and `tmux.PortalSaverName` are excluded from the returned slice and the slice contains exactly the expected restored user sessions. The failure message identifies which reserved name leaked.
- [ ] Both reserved names are referenced via `tmux.PortalBootstrapName` and `tmux.PortalSaverName` — no literal `"_portal-bootstrap"` or `"_portal-saver"` strings appear anywhere in the test file.
- [ ] The helper is invoked from `runRebootRoundTrip` (covering `TestPhase5RebootRoundTripEndToEnd` and `TestPhase5RebootRoundTripBaseIndexDrift`) and from `TestPhase5RebootRoundTripBothSessionsHydrateViaSignalHydrateBinary` after each test's `o.Run` returns.
- [ ] `go test -tags=integration ./cmd/bootstrap/...` is green.
- [ ] `go test ./...` (default surface) is green.
- [ ] The end-to-end test ships in the same commit as task 2-1 (or in a third commit landing after both fixes), per the Rollout section. It is NOT shipped with Phase 1's commit.

**Tests**:
- `TestPhase5RebootRoundTripEndToEnd` (now extended): `"after orchestrator Run, raw tmux state contains no '0' session and no unexpected names beyond {_portal-bootstrap, _portal-saver, _seed, alpha, beta}"` (Assertion 1).
- `TestPhase5RebootRoundTripEndToEnd` (now extended): `"after orchestrator Run, Client.ListSessions returns exactly [alpha, beta] with both reserved names excluded"` (Assertion 2).
- `TestPhase5RebootRoundTripBaseIndexDrift` (inherits via shared `runRebootRoundTrip` body): both assertions evaluated under the base-index-drift sub-test as well.
- `TestPhase5RebootRoundTripBothSessionsHydrateViaSignalHydrateBinary` (extended): both assertions evaluated against the alpha/beta two-session fixture.
- Negative assertion (covered by failure messages, not a separate test): Assertion 1 fails with a clear "unexpected session '0' present" message if Fix B regresses; Assertion 2 fails with a clear "reserved name '_portal-bootstrap' leaked through ListSessions" message if Fix A regresses.

**Edge Cases**:
- The raw assertion MUST bypass `Client.ListSessions`. A regression that silently drops the `-s` arg from `StartServer` would re-introduce `0` — and `0` has no `_*` prefix, so it would still be filtered into invisibility for `ListSessions` callers, masking Root Cause 2. Using `tmux list-sessions -F '#{session_name}'` directly (via the test fixture's `ts.Run` helper) is the only way to surface the regression.
- Phase 1's chokepoint filter must already be in place for Assertion 2 to evaluate cleanly. This is why the spec mandates the end-to-end test ship with Phase 2 (or after both fixes), never with Phase 1: against a Phase-1-only state, `0` would still be present in raw tmux output and Assertion 1 would fail.
- The test must fail meaningfully against the intermediate state where only one of the two fixes is applied:
  - **Phase-1-only state** (Fix A applied, Fix B not applied): `0` still present in raw tmux output → Assertion 1 fails with "unexpected session '0' present".
  - **Phase-2-only state hypothetically without Phase 1** (Fix B applied, Fix A reverted): reserved names leak through `ListSessions` → Assertion 2 fails with "reserved name '_portal-bootstrap' leaked through ListSessions".
- The fixture's `_seed` session (created at line 236 for the save-side bootstrap and re-created at line 319 for the restore-side bootstrap) MUST be in Assertion 1's allowed-reserved set. Its presence is part of the test's bootstrap discipline, not a regression. Pass it explicitly as an additional reserved name to keep the allowed-set strict; do not relax the helper to `HasPrefix("_")`.
- The round-trip body uses `bootstrap.NoOpSaver{}`, so `_portal-saver` is not created in this test's runtime — but the helper's allowed-superset includes it for forward-compatibility (a future variant that wires the real saver adapter MUST not need to update the helper). The strict piece is "no name outside the allowed-superset"; absence of allowed names is tolerated.
- `cmd/bootstrap/reboot_roundtrip_test.go` is gated by `//go:build integration` (line 1) and `testing.Short()` (line 122 etc.), so the new assertions only run on the integration CI lane (`go test -tags=integration`). The default `go test ./...` lane skips them — confirm both lanes are green.

**Context**:

> **End-to-end test mandate** (specification § Test Requirements → "End-To-End — Post-Bootstrap Session State"):
> Extend `cmd/bootstrap/reboot_roundtrip_test.go` (the real-tmux fixture path) with two assertions after a full bootstrap. The test must read tmux's raw session list directly — either via a test-only helper (e.g. `ListAllSessions` if introduced) or by shelling to `tmux list-sessions -F '#{session_name}'` — **not** via `Client.ListSessions`, because asserting on `ListSessions` output would only re-test the chokepoint filter and would NOT catch a regression of Root Cause 2 (rename arg silently dropped, `0` returns — `0` has no `_*` prefix and would still be filtered into invisibility for `ListSessions` callers).

> **Assertion 1 — raw tmux state (catches Root Cause 2)**: The set of session names reported directly by tmux MUST be a subset of `{_portal-bootstrap, _portal-saver, <expected restored sessions>}`. Specifically: no `0`, no other unexpected names. A regression of Fix B (e.g. `-s` arg dropped) breaks this assertion because `0` would re-appear.

> **Assertion 2 — user-facing visibility (catches Root Cause 1)**: `Client.ListSessions` MUST return the expected user-facing slice — i.e. the restored user sessions only, with both reserved names (`_portal-bootstrap`, `_portal-saver`) excluded. A regression of Fix A breaks this assertion because the reserved names would leak through.

> **Why both assertions live together** (specification § Test Requirements → "End-To-End — Post-Bootstrap Session State"): Together these two assertions catch both root causes. This is the end-to-end regression guard for the entire bugfix and for any future `_*` session that joins the codebase.

> **Why this test ships with Phase 2 not Phase 1** (specification § Rollout): The end-to-end test MUST ship in commit 2 (or in a third commit landing after both fixes), never in commit 1. Shipping it in commit 1 would yield a misleading-green test: commit 1 alone leaves the bootstrap session named `0`, and Assertion 1 of the end-to-end test would fail against that intermediate state. Pairing the test with commit 2 keeps the test green only when both root causes are fixed.

> **Review-process gap context** (specification § Why It Wasn't Caught Earlier): The `built-in-session-resurrection` planning's "review" phase scored the implementation against the explicit task list rather than against an end-to-end UX walk-through. Manual QA of the post-bootstrap session list as a user sees it would have caught both root causes. The end-to-end test mandated in this bugfix is the regression guard for that review-process gap, not just the test-surface gap.

> **Fixture context** (`cmd/bootstrap/reboot_roundtrip_test.go`): The round-trip body uses `bootstrap.NoOpSaver{}` (line 344, 833) so `_portal-saver` is not created in this test run. A `_seed` session is created at lines 236 and 319 as part of the test's bootstrap discipline (mirrors the daemon's `_portal-saver` discipline) — it is underscore-prefixed and must be in the allowed-reserved set for Assertion 1.

**Spec Reference**: `.workflows/hidden-sessions-showing-on-startup/specification/hidden-sessions-showing-on-startup/specification.md` § Test Requirements ("End-To-End — Post-Bootstrap Session State") and § Rollout (commit-shape and review-as-a-pair guidance) and § Why It Wasn't Caught Earlier (review-process-gap rationale).

## hidden-sessions-showing-on-startup-2-3 | approved

### Task 2-3: Add release-notes line covering legacy `0` session cleanup on upgrade

**Problem**: When a user upgrades to a Portal version that includes Fix B, any tmux server already running was started by an older Portal and therefore already hosts a session named `0`. Fix B's `StartServer` does not run because the server is already running, so the rename never happens for that server's lifetime. Fix A's filter targets `_*` and does not hide the literal name `0`. The accepted resolution is to instruct affected users to restart their tmux server once after upgrading. The specification mandates this guidance MUST appear in the release notes.

**Solution**: Add a one-line release-notes entry alongside the Phase 2 commit (or in the release artefact that ships with Phase 2) instructing users to run `tmux kill-server` once after upgrading to clear any leftover `0` session created by the previous version. No code change is required.

**Outcome**: The release notes for the shipping version contain wording equivalent to the spec's suggested line: "After upgrading, restart your tmux server (`tmux kill-server`) once to clear any leftover `0` session created by the previous version." Affected users have a documented path to clean up the legacy session without Portal needing to attempt automatic cleanup (which is unsafe — a user is free to create a session named `0`).

**Do**:
- Locate the project's release-notes channel (release commit message, `CHANGELOG.md`, GitHub release body, or whichever artefact the goreleaser pipeline ships per `.goreleaser.yaml`). If no dedicated file exists, the release note belongs in the GitHub release body that goreleaser publishes for the tagged version.
- Add a short upgrade note matching the spec's suggested wording (verbatim or equivalent): "After upgrading, restart your tmux server (`tmux kill-server`) once to clear any leftover `0` session created by the previous version."
- The wording MUST reference the literal name `0` (not `_portal-bootstrap`) because the legacy session pre-dates the rename and Fix A's filter does not match `0`.
- Do NOT add automatic cleanup code. The spec is explicit: "Auto-cleanup is **not** added because Portal cannot safely distinguish 'leftover bootstrap session named `0`' from 'user-owned session named `0`' — a user is free to create one. Filtering the literal name `0` carries the same risk."
- The release-notes line ships with Phase 2 (or the release artefact accompanying Phase 2), never with Phase 1. Phase 1 alone leaves `0` still visible, so the note's framing ("leftover `0` session created by the previous version") is only coherent once Phase 2 has landed.

**Acceptance Criteria**:
- [ ] Release notes for the shipping version contain a one-line upgrade note instructing users to run `tmux kill-server` once after upgrading to clear any leftover `0` session.
- [ ] The wording references the literal name `0` (not `_portal-bootstrap`).
- [ ] No automatic-cleanup code is added — the legacy session's persistence is resolved entirely via user action documented in release notes.
- [ ] The note is delivered with Phase 2's commit / release artefact, not Phase 1.

**Tests**:
- No automated tests. Verification is editorial: confirm the release-notes artefact for the shipping tag contains the required line before the release ships.

**Edge Cases**:
- Project has no dedicated release-notes file — use the GitHub release body that goreleaser publishes (per `.goreleaser.yaml`); document the resolution in the commit message rather than blocking on creating new release infrastructure.
- A user-owned session legitimately named `0` exists at upgrade time — the release-notes wording does not commit users to running `tmux kill-server`; it offers it as the accepted cleanup path. Users who have a real `0` session can ignore the note. No automated cleanup is attempted because the ambiguity is unresolvable from Portal's side.
- Wording must use the literal `0`, not `_portal-bootstrap`. The rename only affects new server starts; the legacy session that existed before upgrade is still named `0` for the rest of that server's lifetime.

**Context**:
> From the specification's Out Of Scope / Deferred → "Cleanup Of Pre-Existing `0` Sessions On Upgrade":
>
> "When users upgrade to a Portal version that includes Fix B, tmux servers that were started by an older Portal will already host a session named `0`. The new `StartServer` does not run because the server is already running, so the rename never happens for that server's lifetime. Fix A does not filter `0` (it filters only `_*`)."
>
> "Auto-cleanup is **not** added because Portal cannot safely distinguish 'leftover bootstrap session named `0`' from 'user-owned session named `0`' — a user is free to create one. Filtering the literal name `0` carries the same risk."
>
> "The accepted resolution is: the legacy `0` session persists until the user restarts their tmux server (machine reboot, manual `tmux kill-server`, `pkill tmux`, etc.). The release notes for the shipping change MUST mention this — the suggested wording is 'After upgrading, restart your tmux server (`tmux kill-server`) once to clear any leftover `0` session created by the previous version.' No code change is required."

**Spec Reference**: `.workflows/hidden-sessions-showing-on-startup/specification/hidden-sessions-showing-on-startup/specification.md` § Out Of Scope / Deferred → "Cleanup Of Pre-Existing `0` Sessions On Upgrade".
