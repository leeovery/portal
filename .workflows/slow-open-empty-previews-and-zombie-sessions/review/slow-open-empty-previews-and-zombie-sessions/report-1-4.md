TASK: 1-4 — Audit and migrate existing test helpers to isolated env (Component G item 2)

ACCEPTANCE CRITERIA:
- Audit file enumerating every `exec.Command`/`CommandContext` call site in `internal/portalbintest`, `internal/tmuxtest`, `internal/restoretest`
- Every (a)-tagged helper updated to take/use isolated env
- Every (b)-tagged helper has one-line justification
- Post-change `grep -rn "exec.Command.*portal\b" ...` yields only tagged sites
- `go test ./...` passes
- No helper overload omits env

STATUS: Complete

SPEC CONTEXT: Spec § Component G item 2 — helpers in three test-only packages spawning `portal` must route isolated env from `portaltest.IsolateStateForTest`. Goal: prevent leaked test daemons from inheriting `$XDG_CONFIG_HOME`. Grep-based completion criterion in audit deliverable.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Audit deliverable: `.workflows/slow-open-empty-previews-and-zombie-sessions/planning/slow-open-empty-previews-and-zombie-sessions/audit-G-test-helpers.md`
  - Migrated helper: `internal/restoretest/restoretest.go:176` — `DriveSignalHydrateBinary(t, portalBinaryDir, socketPath, stateDir, hooksFile, sessions, env)`; env is mandatory positional parameter, no env-less overload
  - Env composition at lines 199-218: `cmd.Env = append(append([]string{}, env...), <overrides>)` — copy-then-extend, per-spawn overrides for TMUX, PORTAL_STATE_DIR, PORTAL_HOOKS_FILE, PATH shadow inherited duplicates via last-write-wins
  - Helper godoc lines 161-170 explicitly mandate `portaltest.IsolateStateForTest(t)` usage with Component G citation
  - Caller migrations in `cmd/bootstrap/reboot_roundtrip_test.go`:
    - line 190 (env capture), line 400 (call) — `TestPhase5RebootRoundTripEndToEnd` family
    - line 906 (env capture), lines 995, 1019 (calls) — `TestRestoreSwitchClientHookFires`
    - line 1140 (env capture), line 1237 (call) — `TestRestoreLeadingDashSessionNamePropagatesToHook`
  - Out-of-scope (b) sites verified intact: `internal/portalbintest/build.go:108` (go build); `internal/tmuxtest/socket.go:79, 123` (tmux against isolated socket)
- Audit footer documents `$ grep -rn "exec.Command.*portal\b" ...` returns zero matches because the only portal-binary spawn uses a `binary` variable. The audit widens to all `exec.Command*` sites in the three packages

TESTS:
- Status: Adequate
- Coverage:
  - Audit grep footer documents completion
  - Helper signature mandates `env []string` at compile time (no env-less overload exists)
  - All four `DriveSignalHydrateBinary` callers pass env via `IsolateStateForTest`
  - Integration tests: `TestPhase5RebootRoundTripEndToEnd`, `TestPhase5RebootRoundTripBaseIndexDrift`, `TestPhase5RebootRoundTripBothSessionsHydrateViaSignalHydrateBinary`, `TestRebootRoundTrip_LeadingDashSessionName`
- Audit's test-suite verification candidly notes that with a live `portal state daemon` running, the Phase-1-3 backstop fires on integration tests — this is the backstop functioning as designed

CODE QUALITY:
- Project conventions: Followed; `t.Helper()`; canonical `IsolateStateForTest` consumed
- SOLID: Good; env parameter is single load-bearing input; `pathFromEnv` is focused
- Complexity: Low; migration is signature widening + caller threading
- Modern idioms: `append(append([]string{}, env...), ...)` defensive copy-then-extend
- Readability: Good; 35+ lines of godoc spell out env contract; rationale comments at each call site cite Component G locally

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [quickfix] Task 1-4's "Do" section references `portaltest.NewIsolatedStateEnv`; live symbol is `portaltest.IsolateStateForTest` per Phase 9 task 9-5. Audit file and one godoc line on `restoretest.go:163` still mention `NewIsolatedStateEnv` in narrative
- [idea] Audit's "test-suite verification notes" footer documents the backstop fires when developer's live daemon is running — a future reader may misread as test regression; one-line "backstop firings here are CORRECT" header would close the loop
- [idea] `internal/portaltest/spawn_daemon.go:60` is a portal-binary spawn site added after this audit (Phase 10 task 10-1). By-construction isolated; out of scope. A brief "see also" appendix would prevent future readers from concluding audit is stale
