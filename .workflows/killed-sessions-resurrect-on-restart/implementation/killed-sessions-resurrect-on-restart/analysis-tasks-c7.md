# Analysis Tasks: killed-sessions-resurrect-on-restart (Cycle 7)

- topic: killed-sessions-resurrect-on-restart
- cycle: 7
- total_proposed: 2

## Discarded Findings
- Extract `bootstrapEagerHydrateScenario` helper to consolidate the ~25-line cold-start integration preamble across AC1/AC2/AC4 — discarded. Rationale: introduces premature abstraction beyond what the work unit requires; the AC2 site diverges (extra `PORTAL_HOOKS_FILE` Setenv + `hooks.NewStore(...).Set(...)` interleaved between bin-PATH and seed-sessions steps) and would force either a `preOrchestratorSetup` callback parameter or AC2 opt-out, weakening the consolidation case; the cumulative ~60-line deletion is offset by ~30-40 lines of helper boilerplate plus a new shared-state surface across three test files; the duplication is mechanical scaffolding, not load-bearing logic, and does not block correctness.

---

## Task 1: Fix gofmt drift on internal/restore/session.go docstring
status: approved
severity: low
sources: standards

**Problem**: `gofmt -l internal/restore/session.go` reports the file dirty. The docstring above `buildHydrateCommand` (line 425) contains the literal token `` `'\''` `` (the canonical POSIX close-escape-reopen shell idiom), which Go 1.26's gofmt smart-punctuation rule rewrites to a form using a typographic right-double-quotation-mark (U+201D). The file was clean against the work unit's pre-baseline — the drift was introduced by this work unit's edits (T3-1 buildHydrateCommand bare form, T8-1 shellQuoteSingle deletion). `gofmt -l .` is one of three declared linters in the work unit's manifest, so this fails the declared lint gate.

**Solution**: Rewrite the docstring line so the literal `` `'\''` `` token no longer appears in the form gofmt's smart-punctuation rule rewrites. The token is illustrative, not load-bearing — the surrounding text already describes the idiom semantically. Preferred approach: drop the literal `'\''` token entirely and let the prose carry the meaning. Alternative: split the three-quote run across separate backtick spans or escape individual apostrophes so the trigger sequence is broken.

**Outcome**: `gofmt -l internal/restore/session.go` returns empty. `gofmt -l .` from repo root returns the same five pre-existing out-of-scope files and no longer flags `internal/restore/session.go`. The docstring still communicates why the bare `buildHydrateCommand` form is safe under Portal's sanitization.

**Do**:
1. Open `/Users/leeovery/Code/portal/internal/restore/session.go` at lines 423-429.
2. Reword the paragraph containing the literal `'\''` token. Drop the bare `` `'\''` `` reference; replace it with prose such as: "the canonical POSIX close-escape-reopen idiom for embedding a single quote inside a single-quoted string". Keep the rest of the docstring (sanitization rationale, reference to `sanitizeSessionName` in `internal/state/panekey.go`, and the conclusion that the bare form is safe) intact.
3. Run `gofmt -l internal/restore/session.go` and confirm empty output.
4. Run `gofmt -l .` from repo root and confirm the result set matches the pre-fix list minus `internal/restore/session.go`.
5. Run `go build ./...` and `go test ./internal/restore/...` to confirm no regression.

**Acceptance Criteria**:
- `gofmt -l internal/restore/session.go` returns empty.
- `gofmt -l .` from repo root no longer lists `internal/restore/session.go`; the remaining flagged files are exactly the five pre-existing out-of-scope files.
- The docstring above `buildHydrateCommand` still explains why the bare form is safe (sanitization rationale preserved).
- `go build ./...` succeeds.
- `go test ./internal/restore/...` passes.

**Tests**:
- No new tests required — docstring-only edit verified by the `gofmt -l` lint gate.

---

## Task 2: Migrate the last inline OpenLogger preamble in exit_closes_pane_integration_test.go to restoretest.OpenTestLogger
status: approved
severity: low
sources: duplication

**Problem**: `setupExitClosesPane` in `/Users/leeovery/Code/portal/internal/restore/exit_closes_pane_integration_test.go` (lines 375-379) still open-codes the five-line `state.OpenLogger` + `t.Fatalf` + `t.Cleanup(Close)` preamble. This is the only remaining site after cycle 6's `restoretest.OpenTestLogger` rollout (which migrated 13 sibling sites). The file is in scope for this work unit — T2-3 and T2-4 both touched it.

**Solution**: Replace the five-line preamble with a single call to the shared helper `restoretest.OpenTestLogger(t, stateDir)`. The package is `restore_test` and the `github.com/leeovery/portal/internal/restoretest` import is already present in the file. The `path/filepath` and `internal/state` imports remain needed for other call sites.

**Outcome**: `setupExitClosesPane` constructs its `*state.Logger` via the shared helper, matching the pattern used by the 13 sites migrated in cycle 6. The 5-line inline preamble collapses to a single line. Cycle 6's migration is now complete across all in-scope sites.

**Do**:
1. Open `/Users/leeovery/Code/portal/internal/restore/exit_closes_pane_integration_test.go` at lines 375-379.
2. Replace the five-line block with: `logger := restoretest.OpenTestLogger(t, stateDir)`.
3. Confirm the `github.com/leeovery/portal/internal/restoretest` import is already present. Leave `path/filepath` and `internal/state` imports alone.
4. Run `go build ./...` to confirm no compile error.
5. Run `go test ./internal/restore/...` to confirm `setupExitClosesPane` still wires the logger correctly and the cleanup still fires.

**Acceptance Criteria**:
- Lines 375-379 in `internal/restore/exit_closes_pane_integration_test.go` are replaced by a single `logger := restoretest.OpenTestLogger(t, stateDir)` line.
- No inline `state.OpenLogger(filepath.Join(stateDir, "portal.log"), false)` call remains in `setupExitClosesPane`.
- The file's other imports (`path/filepath`, `internal/state`) are preserved for the remaining call sites.
- `go build ./...` succeeds.
- `go test ./internal/restore/...` passes (in particular tests exercising `setupExitClosesPane`).

**Tests**:
- No new tests required — mechanical refactor verified by the existing test suite.
