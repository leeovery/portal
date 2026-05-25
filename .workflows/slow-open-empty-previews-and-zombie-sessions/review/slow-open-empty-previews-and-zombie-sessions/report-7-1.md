TASK: 7-1 — Consolidate fingerprint diff/format/sort helpers into internal/portaltest

STATUS: Complete

SPEC CONTEXT: Analysis-cycle refactor (not spec requirement). Driver: three integration-test files plus package-internal `emitFieldDeltas` were re-implementing same five-helper fingerprint-diff suite (~400 LOC combined). Any change to `Fingerprint`'s shape required four coordinated edits.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/portaltest/fingerprint.go:154-244` — `FingerprintDelta`, canonical Field constants, `DiffFingerprints`, private `fieldDeltas`
  - `internal/portaltest/fingerprint.go:246-290` — `FormatFingerprint`, `FormatDelta`
  - `internal/portaltest/fingerprint.go:292-331` — `reportStateDirDelta` delegates with `backstopFieldLabels` adapter preserving exact error strings backstop meta-tests assert against
  - `internal/portaltest/fingerprint.go:333-348` — shared `unionPaths`
- Call sites all using `portaltest.DiffFingerprints` + `portaltest.FormatDelta`:
  - `internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go:284-297`
  - `cmd/bootstrap/composition_abc_integration_test.go:272-285`
  - `cmd/state_daemon_self_supervision_integration_test.go:844-859`
  - `cmd/bootstrap/composition_e2e_self_eject_integration_test.go:349-352` (bonus from later phase)
- `emitFieldDeltas` removed entirely; logic absorbed into private `fieldDeltas`
- Legacy backstop-label preservation via `backstopFieldLabels` map — thoughtful migration preserves meta-test assertions
- Type-swap short-circuit (became-symlink as sole signal) preserved

TESTS:
- Status: Adequate
- `internal/portaltest/fingerprint_diff_test.go` (307 LOC) covers every branch: empty/nil, identical, additions/removals, size/mtime/ctime/content mutations, hashed-flag flip, symlink-target, became-symlink type swap, symlink-to-symlink negative case, mixed deltas, stable sort
- `FormatDelta`: path+field substring, single-line invariant
- `FormatFingerprint`: regular-file, symlink, unhashed
- Existing integration tests provide integration-level acceptance — message format regenerates identically

CODE QUALITY:
- Project conventions: Followed
- SOLID: Good; orchestrate / compare / set-union / render separation
- Complexity: Low; linear pass + sort
- Modern idioms: `sort.SliceStable`; call sites use `slices.Sorted(maps.Keys(m))`
- Readability: Good; doc comments explain per-path semantics, type-swap short-circuit, hashed-vs-content distinction, sort contract, legacy-label rationale

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] `unionPaths` and call-site `slices.Sorted(maps.Keys(m))` both compute "sorted keys"; expose `SortedKeys(map[string]Fingerprint) []string` if pattern proliferates beyond 3 sites
- [idea] Legacy backstop labels preserved for meta-test compatibility; if loosened to substring-contain canonical field name, adapter can be dropped
- [quickfix] Comment at `fingerprint.go:156` references "see DiffFingerprints" for change classes; constants block at 167-177 is actual source of truth
