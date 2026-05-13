STATUS: clean
FINDINGS_COUNT: 0
SUMMARY: Phase 2 cleanup conforms to specification and project conventions.

---

AGENT: standards
FINDINGS: none

SUMMARY: Phase 2 cleanup conforms to specification and project conventions. The WrapCommandError extraction (T2-1), absence-pattern fake commander (T2-2), and transportErrCommandError helper (T2-3) are sanctioned refactors that consolidate the spec-mandated wrap shape into a single source of truth and tighten test-fault-injection parity with production.

Details:

- T2-1 (WrapCommandError extract): internal/tmux/command_error.go:73-83 holds the canonical wrap shape; runCommand and socketCommander.Run/RunRaw both call through it. Pre-existing duplication eliminated. cmd.Stderr-nil invariant documented in one place and referenced by callers. Full godoc satisfies golang-pro's "document all exported functions" rule.
- T2-1 testing: TestWrapCommandError covers all three contract branches with subtests; the exec-exit branch drives a real sh child so (*exec.ExitError).Stderr auto-population is exercised honestly.
- T2-2 (daemonFakeCommander absence pattern): cmd/state_daemon_run_test.go now returns *tmux.CommandError{Stderr: "unknown option: " + name, ...}. The in-source comment explains why a bare ErrOptionNotFound return would bypass the discriminator. Aligns with spec's distinguishability contract under fault injection.
- T2-3 (transportErrCommandError helper): cmd/state_daemon_run_test.go:205-210 provides the canonical "non-absent transport failure" shape (Stderr: "lost server"). Stderr value does not match any optionAbsentStderrPatterns entry — exactly the shape the spec calls for.
- Spec scope check: All three cleanup tasks are refactors or test-fidelity improvements over code already in scope. No new features. No production behaviour change.
- t.Parallel discipline: No t.Parallel() in cmd/state_daemon_run_test.go. daemonFakeCommander's sync.Mutex preserved.
- Error-wrap convention: golang-pro's fmt.Errorf("%w", err) requirement is satisfied via *CommandError's Unwrap() — the spec explicitly evaluated and rejected fmt.Errorf wrap in favour of a typed error.
- Code-quality.md DRY: Three duplications flagged in cycle 1 were addressed appropriately — wrap-logic via T2-1, repeated fault-injection literal via T2-3, structurally-forced Commander-mock duplication left alone (correct — Go test-package-boundary artefact).
- Build/test verification: go build ./... clean; go test ./internal/tmux/... ./internal/state/... passes.
