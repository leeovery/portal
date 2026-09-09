TASK: resume-hooks-silently-lost-8-7 — `internal/hooks` Spells Its One On-Disk Shape Three Different Ways Across Its Exported API (tick-1a24b7, phase 8 implementation-analysis task, severity: duplication)

ACCEPTANCE CRITERIA:
- `internal/hooks` declares one name for the on-disk shape; `hooksFile` is gone.
- `Load` returns an exported type.
- No call site introduces a conversion to satisfy the new signatures.
- `go build ./...` and both lanes pass.

STATUS: complete

SPEC CONTEXT:
This is a phase-8 task, so its authority is its own body rather than the specification (per the
shared verifier context: phases 6–9 are implementation-analysis cycles the implementation phase
generated). The governing behavioural contract it must not disturb is the one CLAUDE.md and the
spec both hold: `internal/hooks`'s exported `StaleKeys` is the single home of the shape-aware
staleness rule, every reader of staleness routes through it, and a key the rule cannot judge is
retained forever (deleting one is data loss). The change is a type-naming refactor — no
predicate, no lock sequencing and no deletion path moves — so no spec constraint is engaged
beyond "behaviour is unchanged", which the code confirms.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hooks/store.go:29-32` — the single declaration: doc comment naming the on-disk
    shape first ("Snapshot is the on-disk shape: map[hook_key]map[event]command") and the clean's
    older-view role second, over `type Snapshot map[string]map[string]string`. No `hooksFile`
    alias survives in the file.
  - `internal/hooks/store.go:45` — `Load(via Via) (Snapshot, error)`: the exported signature now
    names an identifier a godoc reader can follow.
  - Every helper the Do list names carries it: `loadSnapshot` (:53), `loadShared` (:58),
    `loadSharedBounded` (:70), `load` (:84), `save` (:103), `classifySet` (:153),
    `narrowToSnapshot` (:266), `CleanStale`'s callback (:298), `deleteStale` (:318).
  - `internal/hooks/store.go:248` — `StaleKeys(persisted Snapshot, live []string) []string`: the
    raw map is gone from the parameter.
  - `CleanStale`'s callback signature is unchanged at `func(Snapshot) ([]string, error)`, as the
    task instructed.
  - The remaining non-test sources of the package carry no reference to the deleted alias:
    `lock.go`, `locktest.go`, `lookup.go`, `event.go`, `via.go` (read in full).
  - Callers re-pointed with no conversion: `cmd/doctor.go:370` (`persisted, err :=
    store.Load(hooks.ViaDoctor)`) flows straight into `cmd/doctor.go:387`
    (`hooks.StaleKeys(persisted, view.LiveTokens)`); `internal/hooksweep/sweep.go:126-127`
    declares its enumeration closure as `func(hooks.Snapshot) ([]string, error)`.
- Notes: The Do list cited `cmd/doctor.go:305`/`:313`; the two sites are now at `:370`/`:387`
  (later phase-8/9/10 tasks moved the file). Both are the sites the task meant, and both
  type-check without a conversion. The unexported `staleKeys` twin the task's Do list mentions no
  longer exists and is guarded against — `internal/hooks/cleanstale_staleness_guard_test.go:14-27`
  fails on a `staleKeys` FuncDecl and asserts both readers (`deleteStale`, `cmd`'s
  `checkStaleHooks`) reach the exported name.

TESTS:
- Status: Adequate
- Coverage: The task is a pure refactor and correctly ships no new test. The behaviour the renamed
  signatures carry stays covered by the existing suites, which exercise every touched entry point:
  `internal/hooks/store_test.go` (`TestLoad`, `TestStaleKeys`, `TestCleanStale`,
  `TestCleanStaleRemovesExactlyStaleKeys`, `TestCleanStaleLogging`),
  `internal/hooks/store_shape_test.go` (the retain/reap shape rule, driving `Load` → `StaleKeys`
  in the same flow doctor uses), `internal/hooks/cleanstale_snapshot_test.go` (snapshot narrowing,
  enumeration ordering, lock-free enumeration) and `internal/hooks/read_lock_test.go` (the shared
  read lock and the derived pre-read bound). Nothing about their semantics changed: they pass
  composite literals of the unnamed map type into `StaleKeys` (`store_test.go:781`, `:800`,
  `:818`), which remains assignable to the defined type, so no test acquired a conversion either.
- Notes: I did not execute either lane — test execution is outside this review's remit, so the
  fourth criterion ("`go build ./...` and both lanes pass") rests on the implementation record
  rather than on a run of my own. What I could check by reading, I did: every site I located that
  carries the shape type-checks under the new signatures, and no site needs a conversion.

CODE QUALITY:
- Project conventions: Followed. `internal/hooks` stays a persistence-only package; the log
  component, attr vocabulary and lane rules are untouched by a type rename.
- SOLID principles: Good. Naming the on-disk body once on the public face is exactly the
  interface-clarity the task set out to buy, and it is bought without widening the surface: no new
  exported identifier appears, one is removed.
- Complexity: Low — no control flow changed.
- Modern idioms: Yes. A defined type rather than an alias is the right call here: it gives the
  shape a name godoc can render while staying assignable both ways with the raw map, which is what
  keeps every call site conversion-free.
- Readability: Good. The `Snapshot` doc comment leads with the on-disk shape and demotes the
  clean's pre-read role to a second sentence, which is what the task asked for and what makes
  `Load` returning a `Snapshot` read naturally rather than as a leaked internal.
- Issues: None. For completeness, the raw `map[string]map[string]string` still appears in two
  test-side helper signatures — `internal/hooks/cleanstale_snapshot_test.go:192` (`keysOf`) and
  `internal/hookstest/staging.go:31` (`Staging.Body`). Neither is a declaration of a name for the
  shape and neither sits on `internal/hooks`'s public face, which is the boundary the task's
  outcome statement draws, so both are within the accepted criteria; recording them here only so a
  later reader does not mistake them for a missed site.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
