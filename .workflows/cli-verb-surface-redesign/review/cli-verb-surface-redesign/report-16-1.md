TASK: Extract shared down-server result helper in doctor.go (cli-verb-surface-redesign-16-1)

ACCEPTANCE CRITERIA:
- All three checks return via `runtimeDownResult` on the `!serverUp` path.
- The produced `checkResult` (name/status/detail) is byte-identical to the current behaviour for each of the three checks.
- No behaviour change: doctor's per-line output and exit-code contract (0 iff all pass; down server → fail via daemon/saver/hooks) are unchanged.

STATUS: Complete

SPEC CONTEXT: doctor is bootstrap-exempt (starts nothing), so a down tmux server is an honest "Portal runtime not running" state — distinct from corruption/dead-daemon. Per the spec's Exit-code contract, that state is unhealthy → non-zero. The three runtime checks (daemon/saver/hooks) share the front `serverUp` gate read once in runDoctorDiagnosis (doctor.go:363). This is a low-severity duplication-only consolidation task with no spec-behaviour surface of its own.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Helper: cmd/doctor.go:40-42 `runtimeDownResult(name string) checkResult` → `checkResult{name: name, status: checkFail, detail: doctorRuntimeNotRunning}`
  - checkDaemonAlive: cmd/doctor.go:494-496 (`const name = "daemon"`, `if !serverUp { return runtimeDownResult(name) }`)
  - checkSaverUp: cmd/doctor.go:522-524 (`const name = "saver"`, `if !serverUp { return runtimeDownResult(name) }`)
  - checkHooksRegistered: cmd/doctor.go:553-555 (`const name = "hooks"`, `if !serverUp { return runtimeDownResult(name) }`)
- Notes: Byte-identity confirmed. The helper returns the identical struct literal the three checks previously inlined, sourced from the same `doctorRuntimeNotRunning` constant (doctor.go:33). status is `checkFail` and detail is the shared constant in all cases — the only per-check variable is `name`, which is threaded through the parameter. No other return path in any of the three checks was touched (the server-up probe arms are unchanged). The helper carries a clear doc comment justifying the extraction (Rule-of-Three, one-line future edit). This is a faithful, behaviour-preserving consolidation matching the plan exactly — no drift.

TESTS:
- Status: Adequate
- Coverage:
  - Existing down-server assertion retained and passes through the new path: TestDoctorServerDownReportsRuntimeNotRunning (cmd/doctor_test.go:496-538) drives runDoctorDiagnosis with `ServerRunning: false` and asserts all three of daemon/saver/hooks return `checkFail` + the byte-exact `doctorRuntimeNotRunningDetail`, plus `doctorUnhealthy == true` and that server-independent checks (state dir, sessions.json) still pass. This covers the integration path through the helper and the exit-code contract.
  - New focused helper unit test added: TestRuntimeDownResult (cmd/doctor_test.go:544-557) calls `runtimeDownResult(name)` directly for each of "daemon"/"saver"/"hooks" and asserts name echoes the argument, status is `checkFail`, and detail equals the byte-exact expected string. Directly satisfies the "add/extend a focused unit test" acceptance line.
  - The test independently re-declares the expected string as `doctorRuntimeNotRunningDetail` (doctor_test.go:494) rather than importing the production constant, deliberately pinning the contract byte-for-byte instead of trusting the source of truth. I verified the two strings are character-identical (including the em-dash).
- Notes: Test balance is correct — not under-tested (both the direct helper contract and the end-to-end down-server routing are covered) and not over-tested (no redundant per-check duplication; the table-driven loop over the three names is the minimal expression). Tests assert behaviour (status/detail/name), not implementation shape. A test would fail if the helper returned a wrong status/detail or if any check stopped routing through it.

CODE QUALITY:
- Project conventions: Followed. Consistent with the file's existing `const name` idiom and small-helper style; the "single source of the result shape" comment matches the codebase's heavy doc-comment convention.
- SOLID principles: Good. Single-responsibility helper; DRY win consolidating three identical literals.
- Complexity: Low. Trivial pure function; no new branches.
- Modern idioms: Yes. Idiomatic Go.
- Readability: Good. Intent is self-evident and documented.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
