TASK: resume-hooks-silently-lost-8-12 (tick-f1ed50) — "The Fingerprint Backstop Is Narrower Than Its Own Doc And CLAUDE.md Claim"

ACCEPTANCE CRITERIA:
- [ ] `resolveDevStateDir`'s comment describes the post-scrub resolution and the reason for it.
- [ ] CLAUDE.md no longer claims the backstop walks the developer's real install.
- [ ] The snapshot ordering is asserted by a test.
- [ ] No behaviour changes — the backstop keeps its current reach.

STATUS: complete

SPEC CONTEXT:
Per the shared verifier context, this is a phase-8 implementation-analysis task: "A phase 6–10 task's authority is
its own body, not the specification — judge it against what it says it does." The task is a documentation-accuracy
correction plus an ordering pin over `internal/portaltest`'s fingerprint backstop, which is the filesystem arm of
CLAUDE.md's ABSOLUTE INVARIANT (a test must never mutate the real system). The relevant standing constraint is
CLAUDE.md's own framing of the backstop as "defence-in-depth, not a substitute for the env override" — which the
task's derivation cites as what settles which side (code or prose) should move.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/portaltest/fingerprint.go:279-285` — rewritten `resolveDevStateDir` doc comment.
  - `internal/portaltest/isolated_env.go:30-33` — the retained in-source argument for the ordering.
  - `internal/portaltest/isolated_env.go:136-143` — the `installBackstop` var + `installBackstopCleanup` default.
  - `internal/portaltest/isolated_env.go:106-108` — the call site now routed through the var.
  - `internal/portaltest/backstop_ordering_test.go:1-65` — the two ordering tests.
  - `CLAUDE.md:81` (portaltest row) and `CLAUDE.md:114` (test-isolation section) — both claims corrected.
- Notes:
  - Criterion 1 holds. `fingerprint.go:279-285` now states the resolution "deliberately runs after the HOME
    scrub, so the path it returns is the per-test temp HOME's state dir rather than the developer's install",
    gives the reason ("resolving the install instead would let a live host daemon's own tick writes false-trip
    the backstop mid-test") and states the resulting reach ("catches a process that took the scrub and still
    resolved the default path, not one that escaped the scrub entirely"). Every claim checks out against
    `isolated_env.go`: `t.Setenv("HOME", homeDir)` / `t.Setenv("XDG_CONFIG_HOME", "")` at lines 34-35 precede
    `resolveDevStateDir()` at line 56, and with `XDG_CONFIG_HOME` empty the function falls to its HOME branch
    (`fingerprint.go:290-292`), yielding `<tempHOME>/.config/portal/state`.
  - Criterion 2 holds. `CLAUDE.md:81` now reads "over the state dir under the scrubbed `HOME`, not the
    developer's install"; `CLAUDE.md:114` carries the full narrower guarantee including the false-trip reason and
    keeps "defence-in-depth, not a substitute". `CLAUDE.md:122` ("the fingerprint backstop backs it up") makes no
    install claim and needed no edit. No remaining CLAUDE.md sentence claims the developer's real install.
  - Criterion 4 holds. The only production change is the indirection `var installBackstop = installBackstopCleanup`
    plus the call through it; the default value preserves the prior call exactly. `internal/portaltest` is
    test-only (production code must not import it), so no shipped behaviour is reachable from this change.
  - The seam is consistent with the package's existing pattern — `teardown_guard.go:68-69` already declares
    `var registerHomeQuiescenceGuard = registerDirQuiescenceGuard` with the same "a var so a test can observe"
    rationale. `cmd/seam_guard_test.go` is scoped to the `cmd` package (`sourceguardtest.PackageGoFiles(".", true)`
    at line 40), so it does not govern this var and the direct assignment in the new test is not a guard violation.

TESTS:
- Status: Adequate
- Coverage:
  - `TestIsolateStateForTest_ResolvesDevStateDirUnderScrubbedHome` (`internal/portaltest/backstop_ordering_test.go:27-43`)
    pins criterion 3's ordering. It sets a distinct host `XDG_CONFIG_HOME` before the call, so a resolution that ran
    *before* the scrub would produce `<hostConfig>/portal/state` while the expectation is computed from the
    post-call `HOME` — both assertions fire on a reorder. The second assertion is a named-diagnostic duplicate of
    the first rather than an independent case, which is deliberate and reads well in a failure message.
  - `TestIsolateStateForTest_RegistersBackstopOverResolvedDir` (`backstop_ordering_test.go:45-65`) pins that the
    (dir, pre-snapshot) pair handed to the installer is over the resolved dir: the pre-snapshot is empty because
    that dir does not exist at call time, and driving the real `installBackstopCleanup` over it after creating
    `leaked.json` there yields exactly the `created` delta. `hasDelta` (`fingerprint_test.go:32-35`) reconstructs
    the exact `deltaFmt` string `reportStateDirDelta` emits (`fingerprint.go:239, 243`), and `created` passes
    through `backstopFieldLabel` unmapped, so the match is real rather than coincidental.
  - Both tests fail if the registration is removed entirely: test 1 compares against a non-empty expectation, and
    test 2's `os.MkdirAll("")` on an unset `*gotDir` errors into `t.Fatalf`.
  - `captureBackstop` (`backstop_ordering_test.go:11-25`) restores the prior installer via `t.Cleanup`, so the
    substitution cannot leak into a later test in the binary. The project forbids `t.Parallel()`, so the shared
    package-level var carries no race.
  - The test file is correctly unit-lane (no `//go:build integration`): it builds no `portal` binary, spawns no
    daemon and execs nothing — matching the existing unit-lane `internal/portaltest/isolated_env_test.go`.
  - The binary-wide `TestMain` in `isolated_env_test.go:14-16` sandboxes `HOME`/`XDG_CONFIG_HOME` for the whole
    test binary, so these internal-package tests inherit the hermetic sandbox and cannot touch the host install.
- Notes: Test 2 overlaps `TestBackstopCleanupFiresOnExternalMutation` (`fingerprint_test.go:477-497`) on the
  delta-reporting mechanics; its distinct contribution is that the dir and pre-snapshot come from
  `IsolateStateForTest` rather than from the test. That is the property criterion 3 asks for, so the overlap is
  justified rather than redundant.

CODE QUALITY:
- Project conventions: Followed. Test-only package, no `t.Parallel()`, unit lane correct, seam-as-var pattern
  matches the sibling in `teardown_guard.go`, no new imports (so `internal/portaltest`'s dependency shape is
  unchanged), no logging machinery introduced.
- SOLID principles: Good. `backstopT` (`isolated_env.go:130-133`) narrows `*testing.T` to the two methods the
  installer needs — interface segregation at the seam.
- Complexity: Low. One added indirection and two linear tests.
- Modern idioms: Yes.
- Readability: Good. Each comment on the changed lines states a reason rather than restating the code.
- Issues: None reaching the reporting bar.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
