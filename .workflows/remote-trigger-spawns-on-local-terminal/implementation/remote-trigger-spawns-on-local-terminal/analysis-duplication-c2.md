AGENT: duplication
FINDINGS:
- FINDING: Seven near-duplicate happy-path detection subtests in detect_inside_test.go
  SEVERITY: medium
  FILES: internal/spawn/detect_inside_test.go:46-63, internal/spawn/detect_inside_test.go:65-81, internal/spawn/detect_inside_test.go:83-99, internal/spawn/detect_inside_test.go:101-115, internal/spawn/detect_inside_test.go:117-131, internal/spawn/detect_inside_test.go:133-154, internal/spawn/detect_inside_test.go:156-183
  DESCRIPTION: Seven subtests inside TestDetectInsideTmux share an identical
    arrange/act/assert skeleton and differ only in the input client slice and the
    expected outcome. Each one: builds `&fakeClientLister{clients: []ClientActivity{...}}`,
    calls `walker, reader := localWalkSeams()`, invokes
    `detectInsideTmux("dev", lister, walker, reader)`, asserts `err == nil` with the
    same `t.Fatalf` shape, then asserts either `got.IsNull()` or
    `got.BundleID`/`got.Name`. That is roughly 100 lines of repeated structure
    covering one behaviour (winner-select-then-locality-gate) across a matrix of
    client sets: all-remote -> NULL, sole local -> Ghostty, two locals (higher
    listed second, higher listed first, exact tie), remote-most-active-with-local
    -> NULL, local-most-active-with-remote -> Ghostty. This is exactly the
    multiple-scenarios-of-one-behaviour case the project's golang-testing skill
    calls out ("Table-driven tests are the idiomatic Go way to test multiple
    scenarios"), and the same package already uses that form for equivalent matrices
    (TestAppBundlePath, TestParsePSProcessInfo in walk_test.go; the phase-1
    scenario table in detect_test.go). Left as parallel copy-paste subtests, adding
    a new selection scenario means re-stating the whole skeleton, and the assertion
    shapes can drift row-to-row.
  RECOMMENDATION: Collapse the seven happy-path / clean-NULL subtests into one
    table-driven test whose rows carry `{name string, clients []ClientActivity,
    wantNull bool, wantBundleID, wantName string}`, with the shared body doing the
    single `localWalkSeams()` + `detectInsideTmux("dev", ...)` call and the err-nil /
    IsNull-or-bundle assertion once. Keep the three error-path subtests
    (list-clients failure at :185, walk failure at :205, winner-walk transient at
    :230) separate, since they wire bespoke walker/reader fakes and assert on
    `ErrDetectTransient`/underlying-cause chains rather than a resolved identity.
    This removes the ~100 lines of repeated skeleton, aligns with the package's
    existing table-driven style, and makes a new selection case a one-row change.
SUMMARY: The production change (detect_inside.go) is clean and well-factored — it
  reuses the shared walkToBundle, transient, and DI-seam helpers, returns walkToBundle
  directly (the cycle-1 re-branching item is resolved), and its ClientActivity mirror
  of tmux.ClientInfo is a deliberate, documented DI decoupling, not accidental
  duplication. The one meaningful item is test-side: seven happy-path detection
  subtests are near-duplicate arrange/act/assert blocks that should collapse into a
  single table-driven test per the project's testing conventions.
