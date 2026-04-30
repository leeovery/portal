# Plan: Hidden Sessions Showing On Startup

## Phases

### Phase 1: Filter `_*` Sessions in Client.ListSessions
status: approved
approved_at: 2026-04-30

**Goal**: Apply the chokepoint underscore-prefix filter inside `internal/tmux.Client.ListSessions` so every consumer (TUI picker, `portal list`, capture path, future callers) inherits the `_*`-hiding invariant without per-consumer changes, and refresh the `tmux.PortalSaverName` doc-comment to reflect the post-fix reality.

**Why this order**: Establishes the chokepoint filter first. The capture path's existing `keepSessionNames` filter makes the new filter a strict no-op for capture (set difference is identical), so there is no intermediate regression risk on the capture caller. Phase 2's rename to `_portal-bootstrap` depends on this filter already being in place — once Phase 2 lands, the renamed bootstrap session is hidden by virtue of the work done in this phase. Shipping the end-to-end test here (rather than in Phase 2) would fail Assertion 1 against the intermediate state where `0` still exists, hence the spec mandates the end-to-end test ships with Phase 2.

**Acceptance**:
- [ ] `Client.ListSessions` excludes any session whose name begins with `_` (literal `strings.HasPrefix(name, "_")`, no trimming, no case-folding) as the final post-processing step before return.
- [ ] `ListSessions` returns an empty (non-nil) slice when all underlying sessions are filtered out — callers may rely on `len(result) == 0` and JSON-`[]`.
- [ ] `ListSessionNames` continues to delegate to `ListSessions` (no new low-level enumeration introduced); the delegation invariant documented in the spec is preserved.
- [ ] `cmd/list.go` is verified against the empty-input contract: emits no output when `ListSessions` returns an empty slice (silent exit, no "no sessions" message). Adjusted if the existing path produces output; left unchanged with explicit verification note if already compliant.
- [ ] New unit test in `internal/tmux/tmux_test.go` asserts that, given mocked `Commander` output containing a mix of `_*` and non-`_*` names, `Client.ListSessions` returns only the non-`_*` names.
- [ ] Existing `internal/state/capture_test.go` regression tests pass unchanged (capture-path regression guard from spec).
- [ ] `tmux.PortalSaverName` doc-comment is reviewed in the post-fix context — either updated to correctly reference `Client.ListSessions` as the chokepoint for TUI-picker filtering, or explicitly noted as "reviewed, no change required" in the commit message.
- [ ] `go test ./...` is green; change ships as a single targeted commit (Fix A + unit test + doc-comment) per the Rollout section.

#### Tasks
status: approved
approved_at: 2026-04-30

| ID | Task | Acceptance | Edge Cases |
|----|------|------------|------------|
| hidden-sessions-showing-on-startup-1-1 | Add underscore-prefix filter to `Client.ListSessions` with unit test | Filter applied as final post-processing step before return using `strings.HasPrefix(name, "_")`; returns empty (non-nil) slice when all sessions filtered; `ListSessionNames` continues to delegate to `ListSessions`; new unit test in `internal/tmux/tmux_test.go` drives mocked `Commander` output containing a mix of `_*` and non-`_*` names and asserts only non-`_*` names survive plus a fully-filtered case yields a non-nil empty slice; existing `internal/state/capture_test.go` regression tests pass unchanged; `go test ./...` green | all sessions filtered (empty non-nil slice, never nil), zero sessions reported by tmux, `ListSessionNames` delegation invariant preserved, capture-path `keepSessionNames` double-filter remains a no-op |
| hidden-sessions-showing-on-startup-1-2 | Verify `cmd/list.go` empty-input contract and refresh `tmux.PortalSaverName` doc-comment | `cmd/list.go` confirmed to emit no output when `ListSessions` returns empty slice (existing `len(sessions) == 0 { return nil }` guard at line 66-68 is compliant — no code change, verification recorded in commit message); `tmux.PortalSaverName` doc-comment reviewed against post-fix reality and either revised to reference `Client.ListSessions` as the chokepoint for TUI-picker filtering or explicitly noted "reviewed, no change required" in the commit message; entire phase ships as a single targeted commit per Rollout (Fix A + unit test from task 1 + this verification + doc-comment) | existing empty-input path already compliant (verification-only outcome), doc-comment already accurate after Fix A lands (commit-message note path), no per-consumer code change in `cmd/list.go` or `internal/tui` |

### Phase 2: Rename Bootstrap Session to `_portal-bootstrap`
status: approved
approved_at: 2026-04-30

**Goal**: Replace the unnamed bootstrap session created by `internal/tmux.Client.StartServer` with a named `_portal-bootstrap` session keyed off a new exported `PortalBootstrapName` constant, refresh the `tmux.StartServer` doc-comment to drop stale tmux-resurrect / tmux-continuum rationale, and ship the end-to-end regression guard that proves both root causes are fixed together.

**Why this order**: Must follow Phase 1 because the end-to-end test's Assertion 2 (user-facing visibility via `Client.ListSessions` excluding both reserved names) only passes once the chokepoint filter exists. Shipping the end-to-end test in Phase 1 would yield a misleading-green test against an intermediate state where the bootstrap session is still named `0` and Assertion 1 (raw tmux state subset of `{_portal-bootstrap, _portal-saver, <restored>}`) fails. Pairing the rename and the end-to-end test in this phase keeps the test green only when both root causes are resolved, matching the spec's explicit "Review as a pair" directive and the Rollout section's commit shape.

**Acceptance**:
- [ ] `PortalBootstrapName = "_portal-bootstrap"` is added as an exported package-level constant in `internal/tmux` (sibling to `PortalSaverName`).
- [ ] `StartServer` invokes `tmux new-session -d -s _portal-bootstrap` (referenced via the `PortalBootstrapName` constant). No literal `"_portal-bootstrap"` string appears anywhere else in production code or tests — every reference goes through the constant.
- [ ] Existing `TestStartServer` in `internal/tmux/tmux_test.go` is updated to assert the args passed to `Commander.Run` include `-s` and the value of the `PortalBootstrapName` constant.
- [ ] `tmux.StartServer` doc-comment is updated to: (1) drop the tmux-resurrect cleanup claim entirely; (2) drop or reframe the tmux-continuum wording, replacing it with Portal's own `Restore` step (bootstrap step 5) as the beneficiary of keeping the server alive; (3) document that the session is created with the reserved name `PortalBootstrapName` (`_portal-bootstrap`); (4) document that the session is hidden from user-facing listings by the underscore-prefix filter in `Client.ListSessions`; (5) retain the `exit-empty on` rationale for using `new-session -d` over `start-server` (commit `bd659a3`), reframed against Portal's own resurrection.
- [ ] `cmd/bootstrap/reboot_roundtrip_test.go` is extended with two post-bootstrap assertions:
  - **Assertion 1 (Root Cause 2 guard):** the set of session names reported directly by tmux — read via `tmux list-sessions -F '#{session_name}'` or a test-only helper, **not** via `Client.ListSessions` — is a subset of `{_portal-bootstrap, _portal-saver, <expected restored sessions>}` with no `0` and no other unexpected names present.
  - **Assertion 2 (Root Cause 1 guard):** `Client.ListSessions` returns the expected user-facing slice of restored sessions only, with both `_portal-bootstrap` and `_portal-saver` excluded.
- [ ] `go test ./...` is green, including the new end-to-end assertions against the real-tmux fixture path.
- [ ] Change ships as a single targeted commit (Fix B + `TestStartServer` update + `PortalBootstrapName` constant + doc-comment + end-to-end test) per the Rollout section, reviewed together with Phase 1's commit as a pair.

#### Tasks
status: approved
approved_at: 2026-04-30

| ID | Task | Acceptance | Edge Cases |
|----|------|------------|------------|
| hidden-sessions-showing-on-startup-2-1 | Add `PortalBootstrapName` constant, rename bootstrap session in `StartServer`, and refresh doc-comment | `PortalBootstrapName = "_portal-bootstrap"` exported as a package-level constant in `internal/tmux` (sibling to `PortalSaverName`); `StartServer` invokes `tmux new-session -d -s _portal-bootstrap` referenced via the constant with no literal `"_portal-bootstrap"` string elsewhere in production code or tests; existing `TestStartServer` updated to assert the args passed to `Commander.Run` include `-s` and the `PortalBootstrapName` constant value; `tmux.StartServer` doc-comment updated per spec's five-point checklist (drops tmux-resurrect cleanup claim entirely; drops or reframes tmux-continuum wording so Portal's own `Restore` step / bootstrap step 5 is the documented beneficiary of keeping the server alive; documents the reserved name `PortalBootstrapName` (`_portal-bootstrap`); documents that the session is hidden from user-facing listings by the underscore-prefix filter in `Client.ListSessions`; retains the `exit-empty on` rationale for `new-session -d` over `start-server` reframed against Portal's own resurrection); `go test ./...` green | every reference to the reserved name routed through the constant (no literal string in code or tests), `StartServer` precondition (only called when no tmux server is running per existing `ServerRunning` gate in `cmd/root.go`) preserved — duplicate-session failure mode against an already-running server explicitly out of scope, `exit-empty` default-on opportunistic reaping retained as a nice-to-have not a correctness requirement, doc-comment frames `_portal-bootstrap` as the keep-server-alive anchor when `Restore` produces no user sessions |
| hidden-sessions-showing-on-startup-2-2 | Extend `reboot_roundtrip_test.go` with raw-tmux and `ListSessions` post-bootstrap assertions | `cmd/bootstrap/reboot_roundtrip_test.go` extended with two assertions executed after a full bootstrap against the real-tmux fixture; Assertion 1 (Root Cause 2 guard) reads tmux's raw session list directly via `tmux list-sessions -F '#{session_name}'` or a test-only helper — **not** via `Client.ListSessions` — and asserts the set is a subset of `{PortalBootstrapName, PortalSaverName, <expected restored sessions>}` with no `0` and no other unexpected names; Assertion 2 (Root Cause 1 guard) calls `Client.ListSessions` and asserts the returned slice contains exactly the expected restored user sessions with both `PortalBootstrapName` and `PortalSaverName` excluded; both reserved names referenced via the package constants, no literal strings; `go test ./...` green including the real-tmux fixture path; entire Phase 2 ships as a single targeted commit (Fix B + `TestStartServer` update + constant + doc-comment + this end-to-end test) per Rollout, reviewed as a pair with Phase 1's commit | raw assertion MUST bypass `Client.ListSessions` (a regression that silently drops the `-s` arg would re-introduce `0` which has no `_*` prefix and would be filtered into invisibility for `ListSessions` callers, masking Root Cause 2), Phase 1's chokepoint filter must already be in place for Assertion 2 to evaluate cleanly, test fails meaningfully against the intermediate state where only one of the two fixes is applied (commit-1-only state: `0` still present → Assertion 1 fails; commit-2-only-without-Phase-1: reserved names leak through `ListSessions` → Assertion 2 fails) |
