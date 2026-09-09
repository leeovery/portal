TASK: resume-hooks-silently-lost-8-25 (tick-0fe11b) — "The Failed-Write Audit-Trail Assertion Is Hand-Rolled At Six Sites Whose Own File-Siblings Already Use The Shared Helper"

ACCEPTANCE CRITERIA:
- No store-logging suite spells the five shared record properties inline.
- Every failed-write case pins `via`.
- One assertion carries the `error_class`-plus-sentinel tail for hooks, projects and aliases.
- All three suites pass with no coverage lost.

STATUS: complete

SPEC CONTEXT:
The specification (`.workflows/resume-hooks-silently-lost/specification/resume-hooks-silently-lost/specification.md`,
572 lines) contains no mention of `error_class`, the audit trail, or the store-logging suites — this is a phase-8
implementation-analysis task, so per the shared verifier context its authority is its own body rather than the spec.
The surrounding project convention it serves is CLAUDE.md's `logtest` row: `Sink` is the single capture handler, and
`RecordWant`/`AssertRecord` pin in one call the five properties every audit-trail line shares. The task extends that
rule to the failed-write branch of the same lines, whose emitters are `internal/hooks/store.go`,
`internal/project/store.go`, `internal/alias/store.go` and the shared `internal/storelog/clean_stale.go:18`.

IMPLEMENTATION:
- Status: Implemented (delivered at commit af845ee6, with one fix round applied — the hooks clean-stale failed-save
  summary, flagged in the fix-tracking file as the tenth site left without a `via` pin, is now routed through the
  shared assertions at internal/hooks/store_test.go:1025-1032).
- Location:
  - New helper: internal/logtest/assert.go:50 (`AssertWriteFailure(t, rec, wantClass, sentinel)`), sitting beside
    `AssertRecord` at internal/logtest/assert.go:25.
  - hooks: internal/hooks/store_test.go:1025+1032, :1082+1092, :1244+1251, :1385+1392.
  - projects: internal/project/store_logging_test.go:103+116, :209+222, :302+312, :465+472.
  - aliases: internal/alias/store_logging_test.go:143+150, :221+228.
  - Doc: CLAUDE.md:84 (the `logtest` architecture row) records the helper and states why the sentinel is a parameter.
- Notes:
  - The sentinel is taken as a parameter rather than resolved inside, so `logtest` gains no `internal/fileutil` edge.
    `internal/logtest/leaf_guard_test.go:26` pins the transitive dependency set to `harnesstest` + `log` across both
    lanes, and the new file adds only stdlib `errors` — the guard still holds.
  - Delivery is a superset of the six named blocks: the two clean-stale WARN blocks
    (internal/hooks/store_test.go:1081, internal/project/store_logging_test.go:464) additionally replaced a
    last-match-wins `for` loop with a chained query terminating in `Only(...)`, and the alias `SetAndSave` case
    (internal/alias/store_logging_test.go:150) had its `HasAttr("error")` presence check upgraded to an
    `errors.Is` against `fileutil.ErrWriteWrite`. Both are tightenings that serve the stated outcome, not drift.
  - The `via` pins are correct against production: `internal/project/store.go:106`, `:199` and `:233` each emit
    `"via", via` on the failed-write WARN branch, and `internal/storelog/clean_stale.go:20` emits
    `"via", "internal"` on the shared clean-stale WARN — so the three newly-pinned project cases and the two
    clean-stale cases assert an attr the emitter really carries.
  - Class/sentinel pairings are internally consistent: `fileutil.ClassifyWriteError`
    (internal/fileutil/atomic.go:29-37) maps `ErrWriteTempCreate` → "write-failed-temp-create" and `ErrWriteWrite` →
    "write-failed-write", matching every call site. The alias case is not the classifier's fallback branch —
    internal/alias/store.go:93 wraps `fileutil.ErrWriteWrite` explicitly.
  - Acceptance criteria check, against the current tree: a grep of the three suites for `error_class` returns only
    subtest names (internal/hooks/store_test.go:1069, :1231, :1369; internal/project/store_logging_test.go:89, :185,
    :288, :441; internal/alias/store_logging_test.go:132, :206) — no inline assertion of the attr survives. All ten
    `AssertWriteFailure` calls in the three suites have a paired `AssertRecord` carrying a non-empty `Via`.

TESTS:
- Status: Adequate.
- Coverage: The helper carries its own coverage at internal/logtest/assert_test.go:49 (passes on a matching record),
  :56 (fails when the `error_class` attr differs) and :69 (fails when the carried error does not wrap the sentinel) —
  exactly the two failure cases the task's Tests section names, plus the happy path. Both failure tests drive a
  `harnesstest.Recorder` spy and assert the reported-failure count is 1, so a helper that silently passed, or that
  reported both checks on a single-property mismatch, would fail. The refactor itself is covered by the ten migrated
  sites, all of which still exercise the same production paths (a 0500 parent directory forcing an `AtomicWrite`
  failure) as before.
- Notes:
  - No coverage lost at any site, judged by reading each before/after pair in the commit diff. Every migrated block
    gains properties rather than losing them: the four hooks/projects sites gain `component` and/or `via`, the two
    alias sites gain `component`, `via` and the `errors.Is` upgrade, and the two clean-stale blocks gain an
    exactly-one terminal in place of a loop that silently took the last match.
  - Not over-tested: the new helper's three tests are one per branch plus the pass case, and the migrated call sites
    add no redundant assertions beyond the one already-noted overlap (`op`/`component` are re-checked for one record
    at internal/hooks/store_test.go:1025 after `partitionCleanStaleRecords` (:878-883) checks them across the whole
    set) — which is what makes that block read identically to its sibling at :1082.
  - The spy-driven tests call the helper directly rather than through `Recorder.Run`; that is safe here because
    neither fixture omits an attr, so neither `AttrString` nor `ErrorAttr` reaches its fatal path.

CODE QUALITY:
- Project conventions: Followed. `logtest` remains the single home of the shared audit-trail assertions; the helper
  reports through `harnesstest.TestingT` like its siblings, calls `t.Helper()`, and the leaf-guard dependency shape is
  preserved. The CLAUDE.md `logtest` row was updated in the same commit, so the architecture table matches the tree.
- SOLID principles: Good. The helper does one thing (the failed-write tail) and composes with `AssertRecord` rather
  than absorbing it, which keeps the two contracts — the shared five properties and the write-failure tail —
  independently reusable; `cmd/config_migrate_logging_test.go:179` and :214 already consume it for a fourth emitter.
- Complexity: Low — two guarded comparisons.
- Modern idioms: Yes. `errors.Is` for the sentinel check; no `%w`/`%v` confusion in the diagnostics.
- Readability: Good. The doc comment at internal/logtest/assert.go:44-49 states both what is asserted and why the
  sentinel is a parameter, and it holds true against the code beneath it.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
