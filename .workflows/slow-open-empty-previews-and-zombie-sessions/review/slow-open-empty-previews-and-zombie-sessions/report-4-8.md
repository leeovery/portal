TASK: 4-8 — AST-walking test asserts WritePIDFile immediately follows acquireDaemonLock in defaultDaemonRun

STATUS: Issues Found (non-blocking gaps; production ordering correct, tests work)

SPEC CONTEXT: Component C step 4 — window between `AcquireDaemonLock` and `WritePIDFile` must be a single call. Spec requires AST-walking unit test pinning adjacency plus single-call-site guarantee.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Test: `cmd/state_daemon_lock_pid_ordering_test.go` (340 lines, no build tag)
  - Production adjacency: `cmd/state_daemon.go:204` (acquire) → `:205-216` (err-guard) → `:217-219` (WritePIDFile if-stmt); no intervening statements
  - Sole call site: `cmd/state_daemon.go:204` (seam at `:58`)
- Anchors on `*ast.AssignStmt` with `*ast.CallExpr` RHS named `acquireDaemonLock`
- `i+1` validated by `ifStmtIsErrGuard`; `i+2` validated via `ifStmtContainsCallTo`
- Companion test reads non-`_test.go` `.go` files in `cmd/` (non-recursive), counts both bare and `state.AcquireDaemonLock`

TESTS:
- Status: Adequate with one gap
- Coverage: Test 1 — happy path, function-not-found, missing-acquire, insufficient-statements, wrong-AST-type at i+1/i+2, err-guard-shape mismatch, missing WritePIDFile call
- Test 2 — single-call-site count with offending-location output
- GAP: Plan-listed meta-test "AST ordering: clear diagnostic naming intruding statement when synthetic mutation injects unrelated statement between calls" NOT implemented. Diagnostic-quality guarantee asserted in code but never fired by a test

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel`; lives in `cmd` package
- SOLID: Helpers small, single-purpose
- Complexity: Low
- Modern idioms: `parser.SkipObjectResolution`; `ast.Inspect`
- Readability: Excellent; docstrings on every helper with AST-shape diagrams
- Issue: `TestAcquireDaemonLock_SingleProductionCallSite` scopes scan to `cmd/` non-recursively; future production call in `cmd/bootstrap/` or `internal/...` would silently pass
- Issue: `ifStmtIsErrGuard` accepts any non-empty left-ident name, not strictly `"err"`
- Minor: `positionString` hand-rolls position via `strings.TrimPrefix` — `fmt.Sprintf("%d:%d", p.Line, p.Column)` simpler

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Plan's meta-test (synthetic mutation injection) not implemented — add `t.Run` parsing inline source with intruder statement, asserting diagnostic substring
- [idea] `SingleProductionCallSite` scans only `cmd/` non-recursively; broaden via `filepath.WalkDir` from repo root (skipping `*_test.go`) or document scope limitation
- [idea] `ifStmtIsErrGuard` does not enforce `leftIdent.Name == "err"`; tightening would make guard contract more explicit
- [quickfix] Replace `positionString` with `fmt.Sprintf("%d:%d", p.Line, p.Column)`
