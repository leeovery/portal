TASK: resume-hooks-silently-lost-9-11 — internal/hooks's Read API Is Inconsistent Across Its Three Read Entry Points (tick-2dbfeb)

ACCEPTANCE CRITERIA:
- [x] `LookupOnResume` is a method on `*Store` taking a `Via`, and no read entry point resolves a `Via` internally.
- [x] The hydrate helper's lookup still records `via=hydrate` on its degradation breadcrumb.
- [x] `internal/hooks` exports exactly one staleness function, and the unexported twin is gone.
- [x] Every existing behaviour is unchanged: empty key refused before the read, missing or malformed file degrades to "no hook", empty command degrades to "no hook", genuine I/O error returned.

STATUS: complete

SPEC CONTEXT:
This is a phase-9 implementation-analysis task, so its authority is its own body, but the specification
corroborates both halves of it. §272 already reads "The staleness test lives in a single exported function in
`internal/hooks` — `StaleKeys` — … Both callers reach it directly: `checkStaleHooks` on the read-only diagnosis
path, and `CleanStale`'s deletion pass", and the Corrigendum dated 2026-09-06 records exactly this collapse
("The exported/unexported pair the sentence described was a wrapper whose whole body called its twin with an
identical signature; it has been collapsed"), together with the observation that the re-entrancy argument never
applied — `StaleKeys` takes an already-loaded snapshot and acquires nothing. §171 pins the behaviour the task
must not disturb ("`LookupOnResume` must not honour an empty key. An empty `hookKey` argument returns 'no hook'
before the map is consulted, regardless of what the file holds"), and §387 pins the breadcrumb contract the
second criterion protects: the degraded read is "DEBUG, `op=load-unlocked`, the lock error in `error`, and `via`
naming the caller — `hydrate` for `LookupOnResume`, `doctor` for `checkStaleHooks` …".

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hooks/lookup.go:14` — `func (s *Store) LookupOnResume(hookKey string, via Via) (string, bool, error)`.
    The receiver is `*Store`, the `Via` is a parameter passed through to `s.loadShared(via)` (`:19`), and the
    body is otherwise byte-identical to the free function it replaced: the empty-key refusal at `:15-17` still
    precedes the read, the `fmt.Errorf("load hooks: %w", err)` wrap at `:21` still surfaces a genuine I/O error,
    and the absent-key / absent-event / empty-command arms at `:23-30` still degrade to `("", false, nil)`.
    Parameter order puts `via` last, matching `Set(key, event, command, via)` and `Remove(key, event, via)`.
  - `internal/hooks/store.go:248` — the implementation is now exported under the `StaleKeys` name and is the
    package's only staleness function; a repo-wide search for `staleKeys` finds no declaration and no caller,
    only the guard that forbids one. `:332` re-points `deleteStale` onto it
    (`narrowToSnapshot(StaleKeys(h, live), snapshot)`), and `cmd/doctor.go:387`
    (`stale := len(hooks.StaleKeys(persisted, view.LiveTokens))`, inside `checkStaleHooks` at `cmd/doctor.go:363`)
    was already on the exported name and needed no change, as the task predicted.
  - `internal/hooks/store.go:239-247` — both doc comments survive on the one surviving declaration: the
    staleness rule's statement ("stale iff it is absent from live and its shape is one the rule can judge —
    token-shaped, or empty … A key of any other shape … is retained") and the wrapper's rationale paragraph
    ("It carries no mass-deletion guard: an empty live set makes every judgeable persisted key stale, and
    deferring on that is the caller's repair-safety policy"). Both claims hold against the body at `:248-263`.
  - `cmd/state_hydrate.go:179` — the one production call site:
    `command, found, err := cfg.HookStore.LookupOnResume(cfg.HookKey, hooks.ViaHydrate)`. Passing `ViaHydrate`
    explicitly preserves the exact value the deleted hardcode supplied.
  - Every test caller was carried across in the same commit — a search for the old free-function form
    (`hooks.LookupOnResume(`) returns nothing anywhere in the tree.
  - `CLAUDE.md`'s `hooks` architecture row now reads "`store.go`'s exported `StaleKeys` is the single home of the
    shape-aware staleness rule — one name, no unexported twin", and the "Resume hooks" retention paragraph names
    `StaleKeys` rather than `staleKeys`. The row never named `LookupOnResume`, so the Do-list item about it was
    vacuous rather than skipped — no `.md` outside the archived `.workflows/` planning records names the old form.
- Notes:
  - AC1's second clause reads true against the three entry points the task's own problem statement enumerates:
    `Load(via)`, `List(via)` and `LookupOnResume(hookKey, via)` all take the caller's `Via` and none resolves one.
    `loadSnapshot` (`internal/hooks/store.go:53`) still supplies `ViaInternal` itself, but it is unexported
    machinery inside `CleanStale`, which takes no `Via` at all and is a mutation rather than a read entry point —
    outside the shape this task set out to unify, and the spec (§387) names `internal` as that pre-read's
    intended value.
  - The guard rewrite is the load-bearing part of the third criterion:
    `internal/hooks/cleanstale_staleness_guard_test.go:16-23` fails on any surviving `staleKeys` FuncDecl in the
    package's non-test sources (it matches on `FuncDecl.Name`, so a method-shaped twin is caught too), and
    `:25-26` + `:32-47` fail unless both `deleteStale` and `cmd`'s `checkStaleHooks` reach the rule by calling
    `StaleKeys`. The cross-package arm scans `../../cmd` by relative path, which is stable — `go test` runs the
    binary with the package directory as cwd — and `PackageGoFiles` is fatal on an empty match, so a moved or
    renamed target fails the guard rather than passing it vacuously.
  - No drift from the task body: nothing was added, renamed or restructured beyond the two changes it prescribed.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/hooks/lookup_test.go:150-165` — "it records the caller's via on a degraded lookup rather than a
    hardcoded one", a subtest per `Via` over `{ViaHydrate, ViaDoctor}`, each holding the sidecar and asserting
    `hookstest.AssertDegradedRead(t, sink, via.String())`. This is the discriminating test for the change: the
    `ViaDoctor` case fails outright against the old hardcoded `ViaHydrate`, so the parameter is proven to be read
    rather than ignored.
  - `internal/hooks/lookup_test.go:167-177` — "it refuses an empty hook key before reading the file", carried
    over intact from the deleted "reads no file for an empty key": it stages a *directory* at the hooks path, so
    a clean miss can only mean the file was never opened (a read would surface EISDIR). AC4's first clause.
  - `internal/hooks/lookup_test.go:31-64, 98-148` — the pre-existing degradation and error arms all survive
    against the method form: missing file, malformed JSON, absent key, absent event, empty command, empty-key
    entry in the file, whitespace-key non-collapse, and the wrapped EISDIR error asserting `"load hooks"`. AC4's
    remaining clauses.
  - `internal/hooks/lookup_test.go:66-80` — "it returns the registered on-resume command through the store
    method", the renamed happy path.
  - `cmd/hooks_read_lock_test.go:113-135` — "it names the hydrate helper on a degraded lookup". This one drives
    the real production function `execShellOrHookAndExit` through `hydrateCfg`, with the sidecar held, and
    asserts both that the helper still reached its exec ("a busy lock must not drop the pane") and that the
    breadcrumb carries `via=hydrate`. It is the only test that covers AC2 as written — the library tests above
    prove the parameter is honoured, but not that the call site passes the right value.
  - `internal/hooks/cleanstale_staleness_guard_test.go:14-47` — "it applies the staleness rule through the
    single exported function from both callers", the fourth named test, covering AC3 structurally.
  - Behaviour of the rule itself is unmoved and still covered: `internal/hooks/store_test.go:771-822`
    (`TestStaleKeys`) and `internal/hooks/store_shape_test.go:13-103` exercise the retention/reap arms through
    the exported name.
- Notes:
  - Mild over-coverage, not worth changing: `internal/hooks/read_lock_test.go:284-287`'s `LookupOnResume` row in
    `TestReadSharedLockVia` passes `ViaHydrate` and asserts `via=hydrate`, which is now a strict subset of the
    `ViaHydrate` case in `lookup_test.go:150`. The row earns its place as the per-read consistency table's fourth
    entry — dropping it would leave the table non-exhaustive over the reads — and its comment was correctly
    reworded from "the in-package hydrate read names itself without a caller-supplied value" to "from the
    caller's own Via", so it no longer asserts the deleted behaviour.
  - `AssertDegradedRead` (`internal/hookstest/hooks_lock.go:135-150`) narrows by message before its
    exactly-one terminal, so the `hydrate` assertion is not perturbed by the hydrate helper's own emissions, and
    it also pins level, `op` and a non-empty `error` — the whole breadcrumb, not just the `via`.
  - No test executed as part of this review; adequacy judged by reading.

CODE QUALITY:
- Project conventions: Followed. The lane rule holds — every test touched here is unit-lane and none builds,
  spawns or execs a portal binary (the one integration-tagged caller updated,
  `cmd/bootstrap/transient_listpanes_helpers_integration_test.go:121`, was already tagged and stays so). The
  `hooks` log component and the `load-unlocked` / `via` vocabulary are unchanged — no new component, op or attr
  key. `internal/hooks`'s leaf guard is unaffected: the cross-package arm of the staleness guard *reads*
  `cmd`'s sources rather than importing them, and the guard lives in `package hooks_test`, so the production
  dependency set pinned by `leaf_guard_test.go` is untouched.
- SOLID principles: Good. The change is squarely interface-segregation/consistency work: the read that was a
  free function reaching an unexported method on a receiver handed to it is now that method, and the parameter
  the caller means is supplied by the caller.
- Complexity: Low. Two mechanical transformations, no new branches; `LookupOnResume`'s cyclomatic complexity is
  unchanged.
- Modern idioms: Yes. Method-with-pointer-receiver over a package function taking the receiver as its first
  argument is the Go form; the closed `Via` int type keeps an invented literal from compiling.
- Readability: Good. The merged `StaleKeys` doc comment reads as one statement of the rule plus its
  repair-safety caveat rather than two half-statements split across a wrapper and its twin, and the guard's new
  name (`TestStalenessRuleHasOneExportedFunction`) states the property it now enforces rather than the inverted
  one it used to.
- Comment accuracy: Verified against the code. `lookup.go:5-13` ("via names the calling surface for the
  degradation breadcrumb, as it does on the store's other reads") holds — `Load` and `List` do the same;
  `store.go:239-247`'s two paragraphs both hold against `:248-263`; `store.go:62-69`'s "The bound is a parameter
  and is never derived from via, which is a log attr" still holds; `read_lock_test.go:269-270` was updated in
  the same commit and no longer claims the hydrate read names itself. No comment references a task id, phase or
  spec section.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
