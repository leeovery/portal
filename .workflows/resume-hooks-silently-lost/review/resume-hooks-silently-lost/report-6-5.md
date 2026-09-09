TASK: resume-hooks-silently-lost-6-5 — Tighten The Hooks Store's Widened Public Surface (tick-0592da)

ACCEPTANCE CRITERIA:
- `via` is a named type; no call site passes a string literal.
- `Store.Get` no longer exists and nothing references it.
- Whichever bound decision is taken, no bound exists without an in-source justification naming the stall it prevents.
- Neither new doc comment states a count a reader cannot verify locally.

STATUS: complete

SPEC CONTEXT:
The specification's §6.5 declares the acquisition bound and its emission paragraph has the clean's advisory
pre-read "degrade by the same rule" as every other read, naming `cli` / `internal` / `hydrate` / `doctor` as the
closed `via` vocabulary carried on the new `op=load-unlocked` DEBUG breadcrumb. §6.3 sequences the clean's two
reads (snapshot → enumeration → exclusive deletion) and relies on the shared hold being released before
`CleanStale` takes the exclusive one. The task's Do-item 1 required an explicit decision on the second bound the
implementation had introduced, plus a flagged spec amendment; the specification now carries that amendment as
"Corrigendum 2026-09-01" (specification.md:554-556), which states there are two bounds, that the pre-read bound is
a hundredth of the mutation bound floored at one poll interval, and why the second bound exists. The typed `Via`
vocabulary itself is this task's own authority, not the spec's.

IMPLEMENTATION:
- Status: Implemented (delivered at 3a8e761f; the bound was subsequently re-shaped from a constant to a derivation
  by later phase work, which the code-as-source-of-truth reading accepts — it still satisfies this task's criterion).
- Location:
  - internal/hooks/via.go:8 — `type Via int` with the four constants at via.go:12-19 (`iota + 1`), the wire-value
    table at via.go:22-27, and `String()` at via.go:32-34 returning "" for a value outside the vocabulary.
  - internal/hooks/store.go:45, :115, :173, :212 — `Load` / `Set` / `Remove` / `List` take `Via`;
    internal/hooks/lookup.go:14 — `LookupOnResume` takes `Via`.
  - Production call sites all pass constants: cmd/hooks.go:141, :218, :242, :292 (`hooks.ViaCLI`),
    cmd/state_hydrate.go:179 (`hooks.ViaHydrate`), cmd/doctor.go:370 (`hooks.ViaDoctor`),
    internal/hooksweep/standdown.go:15 and internal/hooks/store.go:54, :352 (`hooks.ViaInternal`).
  - `Store.Get` is gone: `grep -rn --include="*.go" "\.Get(" .` returns no hooks-store hit, and the store's method
    set (store.go / lookup.go / lock.go) holds no `Get`. Its one non-test consumer-shaped usage, the restore test
    helper `verifyHookKeyed`, was re-pointed to `Load` at internal/restore/rename_reboot_shared_test.go:45-50
    (a nil map from a missing key still answers the `_, ok := events["on-resume"]` probe correctly).
  - The second bound survives, justified: internal/hooks/lock.go:25-39 (`snapshotLockFraction` +
    `snapshotLockBound()`), whose comment names the stall — a clean takes the sidecar twice, so a stuck writer
    would cost the daemon's tick two mutation bounds rather than one — and explains the poll-interval floor.
    internal/hooks/lock.go:17-21 carries the same shape for `lockTimeout`. The former test seam
    `SetSnapshotLockTimeoutForTest` no longer exists; internal/hooks/locktest.go exposes a read-only
    `SnapshotLockBoundForTest` instead.
- Notes:
  - The task's Do list asked for `type Via string`. The delivery is `type Via int` with a `String()` wire-value
    map. That is a deliberate strengthening rather than a drift: with a string kind, an untyped constant
    (`store.Load("clii")`) still converts implicitly and compiles, which is exactly the silent-grep failure the
    task exists to close; with an int kind it cannot. The criterion ("`via` is a named type; no call site passes a
    string literal") is met more strongly than the Do-item wording.
  - Two review-round issues recorded against this task both landed: the two `cmd`-side raw `"via"` literals are now
    `hooks.ViaCLI.String()` (cmd/hooks.go:242) and `hooks.ViaInternal.String()` (internal/hooksweep/standdown.go:15,
    where `run_hook_stale_cleanup.go`'s stand-down attrs were later re-homed), and the zero-value row was added to
    the wire-value table.
  - `internal/alias` and `internal/project` still take a bare `via string` (e.g. internal/project/tags.go:94). Out
    of this task's scope, which names `internal/hooks` only.

TESTS:
- Status: Adequate
- Coverage:
  - internal/hooks/via_test.go:13-33 (`TestViaWireValues`) pins each constant's logged string plus `hooks.Via(0) → ""`,
    which is the one property the `iota + 1` offset buys and which nothing else would catch.
  - The read-degradation table survives the type change with its behaviour intact: internal/hooks/read_lock_test.go
    `TestReadSharedLock` (shared-grant, release-before-return, concurrent reads, degrade-and-still-read,
    one-DEBUG-per-degraded-read, absent sidecar, creates-nothing, no record from inside a mutation),
    `TestReadSharedLockBoundSelection` (bound taken from the parameter, not from `via` — the discriminator that
    keeps the pre-read bound off the ordinary reads), and `TestReadSharedLockVia`, which drives all four surfaces
    through their real entry points and asserts the emitted attr equals that surface's own wire value. The write
    side (internal/hooks/lock_test.go, lock_write_test.go) is mechanically re-pointed with no assertion changed.
  - The deleted `TestGet` was removed rather than re-pointed. Correct: its subject was the deleted method, and its
    two cases (events for a key, empty for an unknown key) are the same data `TestLoad` already covers via
    `store.Load` — re-pointing would have produced a duplicate.
  - The AC "no call site passes a string literal" is compiler-enforced by the int kind, which is a stronger guard
    than any test; no test is needed or written for it, and none should be.
- Notes: no over-testing observed — the new suite is one table of five rows for a property with no other observer.

CODE QUALITY:
- Project conventions: Followed. The `via` wire values are unchanged, so the closed logging vocabulary is untouched
  (the spec's `cli`/`internal`/`hydrate`/`doctor` set); the `hooks` component binding is unchanged; no new lane
  violations (via_test.go is unit-lane and stdlib-only).
- SOLID principles: Good. The vocabulary is one type in one file; the store methods carry it rather than restating
  a string contract per call site.
- Complexity: Low. `String()` is a map read; `snapshotLockBound()` is a `max` of two figures.
- Modern idioms: Yes — `iota + 1` for a vocabulary whose zero must not alias a member, `max` builtin in the bound.
- Readability: Good. Every constant carries a one-line doc naming its surface.
- Issues: None rising above preference. Two observations deliberately not raised as findings: `viaNames` is a map,
  so a fifth constant added without an entry would render "" (nothing is wrong today — the vocabulary is
  spec-closed and all four entries are pinned), and `Via` implements `fmt.Stringer` while every site calls
  `.String()` explicitly (correct at every existing site).

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
