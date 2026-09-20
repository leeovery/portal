# Review Tracking: Lazy Resume On Attach - Integrity

## Findings

### 1. Registering the panel's capture surfaces breaks six guards the task says are untouched

**Severity**: Important
**Plan Reference**: Phase 3, task `lazy-resume-on-attach-3-6` (Both screens on demand for the visual gate)
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Move**: settled
**Change Type**: update-task

**Problem**:
The visual gate for the feature's whole user surface cannot be reached: the commit that registers the six panel surfaces leaves the capture harness red in five files across two packages, and the task's own acceptance criteria say that is wrong.

Six names join `capture.FixtureNames()` that `FixtureByName` deliberately does not resolve. Every place in the tree that enumerates `FixtureNames()` and resolves each name spells the exception as `name == capture.ContrastValidationFixture`, on its own, and there are six such places:

- `internal/capture/theme_swap_guard_test.go`'s `registryFixtures` (line 102) `t.Fatalf`s on the first unresolvable name, which takes `guardedFixtures` with it and therefore every swap guard reading through it — `theme_swap_guard_test.go`'s four token-diff tests and `theme_panel_fixture_render_test.go`'s coverage assertion.
- The **first** sub-test of `TestThemeSwapGuard_EnumeratesRegistry` (line 149) computes its `want` by deleting the swatch alone.
- `internal/capture/fixture_registry_test.go`'s `TestFixtureRegistry_NamesDeriveFromTheBuilders` fails outright: it asserts `FixtureNames()` equals the builders' own names plus the swatch, and six surfaces are neither.
- `TestFixtureByName_ResolvesEveryEnumeratedName` in the same file `t.Errorf`s on each of the six.
- `internal/capture/fixture_colourless_test.go` and `internal/capture/fixture_render_size_test.go` each fatal on a name they cannot resolve.
- `internal/capture/swap_harness_test.go`'s `buildBackedFixtureNames`, and `cmd/capturetool/theme_persister_test.go`, do the same.

The task's `Do` names one of those six (the second sub-test, the one that is least load-bearing), and its acceptance criteria then assert "Every existing fixture guard (`TestThemeSwapGuard_*`, the colourless and render-size suites) passes unchanged." That is not achievable — `TestThemeSwapGuard_EnumeratesRegistry` is a `TestThemeSwapGuard_*` and the task itself edits it — so an executor who lands the change meets six red suites and a criterion telling them the correct repair is a violation.

The sibling enumerations in this plan are exact — task 1.4 names `LookupOnResume`'s five call sites and misses none, task 2.2 names the five daemon fixture files and misses none, task 6.6 counts `applySessions`'s thirty-six test calls correctly. This one is the outlier.

**Proposal**:
Say where the skip lives and move every site onto it, and replace the criterion that cannot hold with the one the change actually owes: the guards keep covering exactly what they covered, with the six surfaces skipped through one shared set rather than fataling six suites. Which sites those are is read off the tree, not chosen — the six are every `FixtureNames()` consumer that resolves each name. The shared-set shape is the plan's own: the task already declares `SurfaceNames()` as the enumeration, so the skip is that plus the swatch, stated once.

**Current**:

*(a) the `Do` bullet beginning "Widen `TestThemeSwapGuard_EnumeratesRegistry`'s second sub-test")*

- Widen `TestThemeSwapGuard_EnumeratesRegistry`'s second sub-test in `internal/capture/theme_swap_guard_test.go` from "the swatch is the only skip" to a named skip set — the swatch plus `SurfaceNames()` — asserting every member is enumerated by `FixtureNames()`, that none of them resolves through `FixtureByName`, and that the guarded fixtures plus the skip set are exactly `FixtureNames()`.

*(b) the acceptance criterion about the existing guards)*

- [ ] Every existing fixture guard (`TestThemeSwapGuard_*`, the colourless and render-size suites) passes unchanged.

**Proposed Text**:

*(a) replaces the `Do` bullet quoted above with two bullets, in this order)*

- Declare the skip once and move every enumerating site onto it. The enumerated names that do not resolve through `FixtureByName` become the swatch plus `SurfaceNames()`, where each site today spells the exception as `name == capture.ContrastValidationFixture` on its own: `internal/capture/theme_swap_guard_test.go`'s `registryFixtures` — which `guardedFixtures` and every swap guard read through — and the **first** sub-test of `TestThemeSwapGuard_EnumeratesRegistry`; `internal/capture/fixture_registry_test.go`'s `TestFixtureRegistry_NamesDeriveFromTheBuilders`, whose `want` is the builders' own names plus the swatch and becomes the builders' names plus the whole skip set, and its `TestFixtureByName_ResolvesEveryEnumeratedName`; `internal/capture/fixture_colourless_test.go`; `internal/capture/fixture_render_size_test.go`; `internal/capture/swap_harness_test.go`'s `buildBackedFixtureNames`; and `cmd/capturetool/theme_persister_test.go`. A surface added later then joins the skip at one edit rather than fataling six suites in two packages.
- Widen `TestThemeSwapGuard_EnumeratesRegistry`'s second sub-test in `internal/capture/theme_swap_guard_test.go` from "the swatch is the only skip" to that same named set, asserting every member is enumerated by `FixtureNames()`, that none of them resolves through `FixtureByName`, and that the guarded fixtures plus the skip set are exactly `FixtureNames()`.

*(b) replaces the acceptance criterion quoted above)*

- [ ] Every existing fixture guard still covers exactly what it covered: `guardedFixtures` ranges over the same `*Fixture` set as before, and the registry, colourless, render-size, swap-harness and capturetool theme-persister suites each skip the six surface names through the shared skip set rather than fataling on a name they cannot resolve. No guard loses a fixture, and none is exempted.

*(c) added to the Tests list, after `"it skips exactly the named standalone surfaces"`)*

- `"it keeps every enumerating guard over its own set"` (the registry, colourless, render-size, swap-harness and theme-persister suites, run with the six surfaces registered)

*(d) added to the Edge Cases list, after the "The swap guard's single-skip assertion widens to a named set" bullet)*

- The skip is declared once rather than at each enumerating site. Six places in two packages today name the swatch as the only enumerated name that does not resolve, and each of them fatals on a name it cannot resolve — so registering a surface without moving them takes `registryFixtures` down, and with it `guardedFixtures` and every swap guard that reads through it. One shared set is what makes the next surface an edit rather than an outage.

**Resolution**: Fixed — task 3-6's Do gains the declare-once bullet naming all six enumerating sites, its second sub-test bullet points at the same shared set, the false "passes unchanged" criterion is replaced by one stating what holds, and a test and an edge case follow. Tick body re-synced and byte-verified.
**Notes**: Verified in the tree before applying — `ContrastValidationFixture` appears in twelve Go files, the guard files among them exactly as the finding names them.

---

### 2. Task 1.3's `Store.Set` call-site list misses four packages, five of the files behind the integration tag

**Severity**: Important
**Plan Reference**: Phase 1, task `lazy-resume-on-attach-1-3` (hook set writes the registration whole in the shape its content chooses)
**Category**: Task Self-Containment / Scope and Granularity
**Move**: settled
**Change Type**: update-task

**Problem**:
The commit that changes `Store.Set`'s signature leaves the repository in a state where `go test ./...` is green and `go test -tags integration -p 1 ./...` will not build — and nothing in the task tells the executor to look. With no CI, that break surfaces at a release or at whatever much later moment somebody next runs the integration lane, by which point several more commits sit on top of it.

`Set` gains a `Registration` where it took a command string. The task enumerates its call sites as `cmd/hooks.go`, `internal/hookstest/hooks.go`, "then the `internal/hooks` and `cmd` suites that call `Set` directly". The tree holds four more packages:

- `internal/hooksweep/snapshot_order_test.go` — two calls, unit lane.
- `internal/hookstest/staging_test.go` — one call; the `Do` names that package's `hooks.go` but not its own suite.
- `internal/restore/reboot_fixture_test.go` and `internal/restore/exit_closes_pane_integration_test.go` — both `//go:build integration`. The second is `setupExitClosesPane`, the fixture Phase 4's own task 4.7 and Phase 5's task 5.6 are modelled on.
- `cmd/bootstrap/reboot_roundtrip_test.go` and `cmd/bootstrap/phase2_hook_fire_integration_test.go` — both `//go:build integration`, and `cmd/bootstrap` is a different package from `cmd`.

`cmd/noncontiguous_window_reboot_integration_test.go` is inside the named `cmd` set but is integration-tagged too, so the fast lane compiles none of it either.

This plan's other signature changes get this exactly right — task 1.4 names `LookupOnResume`'s five call sites and the tree holds five, task 2.2 names the five daemon fixture files and the tree holds five, task 3.3 names both `renderHeaderWithBadge` call sites. The convention is already the plan's; this task is the one place it lapses.

**Proposal**:
Enumerate the call sites the way the sibling tasks do, and say which lane each is reachable from, so the executor knows the fast lane cannot report this one. Add the criterion that both lanes build — the only check that catches a tagged file left at the old signature.

**Current**:

*(a) the `Do` bullet beginning "Update the two non-test call sites")*

- Update the two non-test call sites: `cmd/hooks.go`'s `hooksSetCmd` (`hooks.Registration{Command: command}` — no mode passes through the CLI until the flag task) and `internal/hookstest/hooks.go`'s `SeedHooksJSON`; then the `internal/hooks` and `cmd` suites that call `Set` directly.

*(b) the last acceptance criterion)*

- [ ] The `set` / `modify` / `set-noop` breadcrumbs carry the same message, `op`, `hook_key`, `via` and `value` (the command) as before, out of either stored shape, and a failed save still carries `error` and `error_class` and no `value`-less shape change.

**Proposed Text**:

*(a) replaces the `Do` bullet quoted above with two bullets, in this order)*

- Update the two non-test call sites: `cmd/hooks.go`'s `hooksSetCmd` (`hooks.Registration{Command: command}` — no mode passes through the CLI until the flag task) and `internal/hookstest/hooks.go`'s `SeedHooksJSON`.
- Carry the signature through every suite that calls `Set` directly, across both lanes. Unit lane: `internal/hooks` (`store_test.go`, `event_test.go`, `lock_test.go`, `lock_write_test.go`, `read_lock_test.go`, `cleanstale_snapshot_test.go`), `internal/hookstest/staging_test.go`, `internal/hooksweep/snapshot_order_test.go`, `cmd/state_daemon_test.go`. Integration lane: `cmd/noncontiguous_window_reboot_integration_test.go`, `cmd/bootstrap/reboot_roundtrip_test.go`, `cmd/bootstrap/phase2_hook_fire_integration_test.go`, `internal/restore/reboot_fixture_test.go` and `internal/restore/exit_closes_pane_integration_test.go` — five `//go:build integration` files that `go test ./...` does not compile at all, so the fast lane cannot report one left behind.

*(b) added after the acceptance criterion quoted above)*

- [ ] Both lanes build: `go test ./...` and `go test -tags integration -p 1 ./...` each compile the whole tree, so no integration-tagged suite is left calling the old signature.

*(c) added to the Edge Cases list, after the "A registration built in a test literal" bullet)*

- Five of the suites calling `Set` are `//go:build integration`, including `internal/restore`'s `setupExitClosesPane` — the fixture Phase 4's and Phase 5's end-to-end suites are built from. `go test ./...` does not compile a tagged file, so a green fast lane says nothing about them; the integration lane is what closes this signature change, and there is no CI to run it later.

**Resolution**: Fixed — task 1-3's call-site bullet is split in two and enumerates every suite per lane, naming the five `//go:build integration` files the fast lane does not compile; a both-lanes-build criterion and an edge case follow. Tick body re-synced and byte-verified.
**Notes**: Verified before applying — counted the `Set(` calls in each omitted file (hooksweep 2, hookstest staging 1, restore reboot_fixture 1, restore exit_closes_pane 1, bootstrap reboot_roundtrip 12, bootstrap phase2_hook_fire 1) and read each file's build line: five carry the integration tag, two do not, as stated. One correction made during application: the (b) block is an addition, not a replacement, and was first applied over the breadcrumb criterion; that criterion was restored and the new one placed after it, both verified present exactly once.
