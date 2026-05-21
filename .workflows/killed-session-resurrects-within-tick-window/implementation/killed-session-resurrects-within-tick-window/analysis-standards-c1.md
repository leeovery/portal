AGENT: standards
STATUS: clean
FINDINGS_COUNT: 0
FINDINGS: none

SUMMARY: Implementation conforms to specification and project conventions.

Detailed verification:

- **commitNowCommand literal** — Byte-exact match at `internal/tmux/hooks_register.go:61`: `run-shell "command -v portal >/dev/null 2>&1 && portal state commit-now"`. Test mirrors live in `hooks_register_test.go:38` and the bootstrap phase5 integration test (`cmd/bootstrap/phase5_integration_test.go:106`) asserts the substring on `session-closed`.
- **Seven acceptance criteria coverage**: all covered by tests (resurrection elimination, bootstrap reconstruction suppression, TUI correctness via sessions.json, restoration-window safety, _portal-saver self-kill steady-state and upgrade, hook idempotency).
- **save.requested discipline** — Correctly inverted: silent on success, touched on short-circuit, touched on failure. Tests 8, 12, 18, 21 verify.
- **Re-entrancy gate before symptom test** — Both run on default lane (no `//go:build integration`). Re-entrancy test is spec-mandated gate.
- **@portal-restoring short-circuit honours marker semantics** — `state_commit_now.go:199` calls `isRestoring()`; on `(true, nil)` logs INFO, touches save.requested, returns nil.
- **ENOENT and decode errors both log WARN** — `loadPrevIndex:258-269` handles both paths; tests 6 and 7 verify.
- **No `t.Parallel()`** — Verified across all 9 implementation files.
- **Logger component constants** — `ComponentDaemon` for capture/commit diagnostics; `ComponentBootstrap` in `migrateSessionClosedHook`.
- **`IsRestoringSet` query-failure deferral** — Appropriate defer. On `isRestoring` error, falls through to happy path; if tmux genuinely unreachable, subsequent `captureStructure` call fails, hitting `failCommitNow` (ERROR + touch save.requested + non-zero exit). Spec does not mandate explicit query-failure semantics.
