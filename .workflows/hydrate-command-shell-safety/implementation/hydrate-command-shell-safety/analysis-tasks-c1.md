---
topic: hydrate-command-shell-safety
cycle: 1
total_proposed: 1
---
# Analysis Tasks: hydrate-command-shell-safety (Cycle 1)

## Task 1: Extract sanitized-stem assertion helper in panekey_test.go
status: pending
severity: low
sources: duplication

**Problem**: Four new sub-tests in `TestSanitizePaneKey` ("replaces whitespace bytes...", per-byte shell-meta cases via tc loop, "replaces mixed whitespace and shell-meta...", and "collapses an all-non-allowlist input...") inline the same three-step assertion shape: `strings.TrimSuffix(got, "__0.0")`, `strings.HasPrefix(stem, "<expected>-")`, then `strings.TrimPrefix` + `len(hashPart) != 8`. The pre-existing collision sub-test (lines 56-79) additionally validates `hashPart` is lowercase hex; the four new copies silently dropped that check, so the assertion has diverged and weakened. ~20 lines of repeated structure across `internal/state/panekey_test.go:117-119, 147-149, 161-163, 174-176`.

**Solution**: Introduce a test-local helper (e.g. `assertSanitizedStem(t *testing.T, got, wantStem string, w, p int)`) in `internal/state/panekey_test.go` that strips the `__<w>.<p>` suffix, asserts `strings.HasPrefix(stem, wantStem+"-")`, and asserts the trailing 8 chars after the prefix are lowercase hex. Replace the four inline assertion blocks with one-line calls to the helper. This restores the lowercase-hex check uniformly across all collision-bearing cases.

**Outcome**: A single helper centralises the sanitized-stem-plus-collision-suffix invariant. All five collision-bearing sub-tests (the original plus the four new ones) assert the same shape including lowercase-hex on the 8-char hash. Test file shrinks by ~15-20 lines and future sub-tests can adopt the helper with one line.

**Do**:
1. Open `internal/state/panekey_test.go`.
2. Add a test-local helper near the top of the file (after imports, before `TestSanitizePaneKey`).
3. Replace the inline TrimSuffix/HasPrefix/len-8 blocks at the four sites (~lines 117-119, 147-149, 161-163, 174-176) with a single call to `assertSanitizedStem(t, got, "<expected stem>", 0, 0)` using the appropriate expected stem for each case.
4. Optionally refactor the original pre-existing collision sub-test (lines 56-79) to use the same helper for consistency, provided its inputs share the `__0.0` window/pane shape.
5. Run `go test ./internal/state/...` and confirm all sub-tests still pass.

**Acceptance Criteria**:
- A single helper function in `internal/state/panekey_test.go` encapsulates the suffix-strip + prefix-check + 8-char lowercase-hex assertion.
- The four new sub-tests added in this implementation cycle each call the helper instead of inlining the three-step assertion.
- The lowercase-hex assertion is applied in every collision-bearing sub-test (no silent divergence from the original).
- `go test ./internal/state/...` passes.
- No production code under `internal/state/` is modified.

**Tests**:
- Existing `TestSanitizePaneKey` sub-tests continue to pass unchanged in semantics.
- The helper itself is exercised indirectly by the four refactored sub-tests.
