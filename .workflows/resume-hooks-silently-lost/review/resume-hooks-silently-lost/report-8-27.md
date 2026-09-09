TASK: resume-hooks-silently-lost-8-27 (tick-46f73b) — "There Are Four hooks.json Staging Routes Where The Code Claims Two, And The Path Composition Is Hand-Rolled Fifty Times"

ACCEPTANCE CRITERIA:
- Two routes: the stager and the path-only sibling. `seedHookStore` and `writeHooksJSON` are gone.
- Every hydrate fixture asks for the sidecar's absence explicitly and still takes the degraded read (its `load-unlocked` breadcrumbs are unchanged).
- No inline `filepath.Join(dir, "hooks.json")` remains in the two `internal/hooks` suites.
- The ENOENT-tolerant read has one implementation, and the doc comment's route count matches the tree.

STATUS: issues_found (non-blocking; both findings are contained and neither is a delivery blocker)

SPEC CONTEXT:
Phase 8 is an implementation-analysis consolidation cycle, so this task's authority is its own body rather than the specification. The relevant spec background is §6.2/§6.5: `hooks.json` is protected by a `hooks.json.lock` sidecar created only by a mutation acquire, and a read that cannot take the shared hold falls through to an unlocked read with one DEBUG `op=load-unlocked` breadcrumb. That is what makes `Staging.SidecarAbsent` load-bearing: a fixture that gains a sidecar it never had moves off the degraded read the hydrate suites model, and one that loses a sidecar starts emitting a breadcrumb its sink never meant to hold.

IMPLEMENTATION:
- Status: Implemented (commit a01ef932; 21 files).
- Location:
  - `internal/hookstest/staging.go:34-51` — new `SidecarAbsent` axis with its derivation stated in the doc comment; `:80-81` the `Body` seed arm; `:99-104` the new `HooksPath` path-only sibling; `:70` `StageStore` composes its own path through it, so stager and sibling cannot disagree.
  - `internal/hookstest/hooks_lock.go:15-20` — new `SidecarPath`, with `CreateHooksSidecar` (`:28`) and `openSidecar` (`:89`) routed through it.
  - `cmd/testhelpers_test.go:130-132` — `readFileBytes` now delegates to `hookstest.HooksFileBytes`; `:151-159` `hooksFileInTempDir` takes a body and delegates staging to `StageStore`, adding only the `PORTAL_HOOKS_FILE` pointing.
  - `cmd/state_hydrate_test.go` — `seedHookStore` deleted; all ~19 hydrate call sites across `state_hydrate_test.go`, `state_hydrate_exec_log_test.go` and `state_hydrate_empty_hookkey_test.go` now pass `Staging{SidecarAbsent: true, Body: …}`.
  - `internal/hooks/store_test.go`, `internal/hooks/lock_write_test.go` — inline joins folded onto `HooksPath`; the eight seed pairs in `store_test.go` folded onto `Staging{Seed}`.
- Notes: all four criteria are met on the tree as it stands.
  1. `seedHookStore` and `writeHooksJSON` are gone (repo-wide grep returns no hits), and `seedHooksFile` went with them.
  2. Every `StageStore` call in the three hydrate suites carries `SidecarAbsent: true` (a grep for hydrate-suite `StageStore` lines without it returns nothing), so those fixtures still take the unlocked read; the assertions in those suites select records by message (`execLogLine`, `strings.Count`), so an unchanged breadcrumb set stays unasserted-on either way.
  3. Neither named `internal/hooks` suite composes `filepath.Join(dir, "hooks.json")` any more.
  4. `readFileBytes` holds no read rule of its own.
  Behaviour parity at the folded call sites holds where it matters: `hooksFileInTempDir(t, nil)` stages no file and still stages the sidecar, matching the old two-step exactly, and `Body: map[…]{}` stages `{}` as `writeHooksJSON` did. The one silent difference is formatting — `writeHooksJSON` used `json.MarshalIndent`, `StageStore` uses compact `json.Marshal` — which no surviving assertion observes (every consumer either parses the file or compares before/after bytes read off the same staged file), so the "staged bytes unchanged" instruction is technically missed with no consequence I can name; not reported as a finding.

TESTS:
- Status: Adequate.
- Coverage: all three tests the task named exist and assert their subject — `internal/hookstest/staging_test.go:127` ("it stages a multi-event hooks body", which checks the second event survives on disk *and* loads back), `:38` ("it stages no sidecar when the fixture asks for the absence", which pairs the stat with `AssertDegradedRead`), `:163` ("it returns the hooks path without staging a file"). The default-sidecar arm at `:18` is the complement and asserts the *absence* of `load-unlocked` records, so the two halves of the new axis are pinned against each other rather than each in isolation. `TestSidecarPath` (`:190`) anchors the suffix to a literal.
- Notes: `internal/hookstest/staging_test.go:168` is a dead assertion (finding below). No over-testing: the added cases are one per new axis, and the folded call sites gained no assertions.

CODE QUALITY:
- Project conventions: Followed. `hookstest` stays test-only (`*testing.T`-first), the new helpers keep the `hookstest.<Name>: <what failed>` fatal-message convention, and CLAUDE.md's `hookstest` row was updated in the same commit with a carefully scoped claim ("neither of the two `internal/hooks` suites this consolidation reached composes a `filepath.Join(dir, "hooks.json")` of its own"), which is true.
- SOLID principles: Good. One stager describing a file, one sibling naming it; `seedWays`, `writeSeed`, `shapeEntries` and `marshalBody` are each one job.
- Complexity: Low. `StageStore` is a flat switch over mutually exclusive seeds with two validation fatals up front.
- Modern idioms: Yes.
- Readability: Good. The `SidecarAbsent` doc comment states why the default is what it is, which is the property the hydrate fixtures depend on.
- Issues: the package doc's route claim overreaches (finding below).

BLOCKING ISSUES:
- None.

FINDINGS:
- [in-scope] [contained] internal/hookstest/doc.go:8 — the sentence "A test reaches a hooks.json by path two ways and no others" is an exclusivity claim the tree falsifies: `internal/hooks/lock_test.go` composes `filepath.Join(t.TempDir(), "hooks.json")` at 12 sites (`:36`, `:65`, `:90`, `:135`, `:168`, `:201`, `:218`, `:246`, `:264`, `:279` and two nested-path variants at `:117`/`:297`), and three fixtures in the very suites this task edited hand-roll the `Staging{Unreadable: true}` axis instead — `cmd/state_hydrate_test.go:1499` and `:1543` (`filepath.Join(dir, "hooks.json")` + `os.Mkdir` + `hooks.NewStore`) and `cmd/state_hydrate_exec_log_test.go:75` (`dir + "/hooks.json"`, same three steps). Scope the sentence to what the package offers, as the CLAUDE.md row does ("hookstest offers two path-based routes …"), or fold those three fixtures onto `Staging{Unreadable: true}` and scope the claim to the suites actually reached — FAILS: the acceptance criterion asked for a doc comment whose route count matches the tree, and this one repeats the failure mode the task was chartered against — the previous "second of the two staging routes" comment was false in exactly this way, and the task's own problem statement names that as "how the drift stayed invisible". Nothing enforces the claim (there is no source guard over hooks.json path composition), so the next reader who trusts it will not go looking for the compositions that remain.
- [in-scope] [contained] internal/hookstest/staging_test.go:168 — `if want := hookstest.HooksPath(t, dir); path != want` compares the result of `HooksPath(t, dir)` (taken at `:166`) with a second call to the same function, so it holds for every possible implementation, including one that returned `dir` unchanged. Delete it — the two `os.Stat` checks at `:170` and `:174` carry the subtest's named behaviour ("without staging a file") — or anchor it to `filepath.Join(dir, "hooks.json")` the way `TestSidecarPath` anchors the suffix at `:194`. Note that `TestHooksPath`'s second subtest ("it names the same path the stager stages", `:181`) is not the same defect: `StageStore` composes through `HooksPath` at `staging.go:70`, so that one is a real delegation guard that will fail the day the stager stops delegating — FAILS: an assertion that cannot fail reads as a pin and is not one; with it in place nothing in `hookstest` ties `HooksPath` to the `<dir>/hooks.json` shape (the only thing that catches a wrong filename today is `internal/hooks/store_test.go:160`, which enumerates the staged directory expecting exactly `hooks.json` and `hooks.json.lock` — a coincidence of another suite, not a pin here).
