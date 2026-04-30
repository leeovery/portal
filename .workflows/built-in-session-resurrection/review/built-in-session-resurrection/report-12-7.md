# Review Report: built-in-session-resurrection-12-7

**TASK**: Defer `logger.Close()` in `state_signal_hydrate` and `state_hydrate` RunE

**ACCEPTANCE CRITERIA**:
- Add `defer logger.Close()` to RunE bodies in `cmd/state_signal_hydrate.go` and `cmd/state_hydrate.go`.
- Edge: production exec'd-away path — OS closes duplicated fd on process replacement.
- Edge: `Close()` idempotent on already-exec'd logger and nil receiver.

**STATUS**: Complete

**SPEC CONTEXT**:
Both helpers open a non-rotating `*state.Logger` via `openNoRotateLogger()` per spec § Log Rotation → Concurrent-writer discipline (only the daemon rotates). `state hydrate` exec's $SHELL on success; `state signal-hydrate` is run-shell-invoked and returns normally. On the exec path the defer is irrelevant (OS reclaims fds); on non-exec error paths and test seams the defer prevents fd leak — the original review finding.

**IMPLEMENTATION**:
- Status: Implemented
- Locations:
  - `/Users/leeovery/Code/portal/cmd/state_signal_hydrate.go:150-151`
  - `/Users/leeovery/Code/portal/cmd/state_hydrate.go:366-367`
- Both use closure-wrapped form `defer func() { _ = logger.Close() }()`, consistent with project pattern (`state_notify.go:48`, `state_migrate_rename.go:34`, `state_cleanup.go:78`, `state_daemon.go:216`).
- `state_hydrate.go:360-365` carries an explicit doc-comment justifying defer presence on the exec'd-away path, preventing future "dead code" simplification.
- Defer placed BEFORE cfg construction so all short-circuit return paths are covered.

**TESTS**:
- Status: Adequate
- `Logger.Close` idempotency is structurally guaranteed by `internal/state/logger.go:213-218`: nil-receiver and nil-file both return nil. `openNoRotateLogger` failure returns `(nil, err)` — Close on nil is a no-op.
- Existing tests in `cmd/state_hydrate_test.go` and `cmd/state_signal_hydrate_test.go` exercise `runHydrate`/`runSignalHydrate` directly; argv-validation tests traverse RunE and exercise the defer in non-exec paths.

**CODE QUALITY**:
- Project conventions: Followed (idiomatic closure-wrapped defer).
- SOLID: Good. RunE owns logger lifetime.
- Complexity: Low.
- Modern idioms: Canonical Go closure-wrapped defer with explicit error discard.
- Readability: Good. state_hydrate.go inline rationale prevents future-maintainer regression.
- Issues: None.

**BLOCKING ISSUES**:
- None

**NON-BLOCKING NOTES**:
- [idea] signal-hydrate's existing comment is accurate (it does not exec away); no symmetry change with state_hydrate's production-exec rationale needed.
