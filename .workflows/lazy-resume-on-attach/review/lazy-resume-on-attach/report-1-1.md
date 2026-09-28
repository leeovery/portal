TASK: Resume Mode Vocabulary and Resolution (lazy-resume-on-attach-1-1, tick-b9944e)

ACCEPTANCE CRITERIA:
- `resumemode.Parse` recognises `"eager"` and `"lazy"` and nothing else; `"Lazy"`, `"LAZY"`, `" lazy"`, `"lazy "`, `"laz"`, `"lazyy"` and `""` all answer `(Unset, false)`.
- `Mode.String()` round-trips `Eager` and `Lazy` through `Parse`, and returns `""` for `Unset` and for an out-of-vocabulary `Mode` value.
- `Resolve` returns the registration's mode whenever the registration names one, whatever the install says — including a registration pinned eager under a lazy install and the reverse.
- `Resolve` returns the install's mode when the registration names none, and returns `Lazy` when neither names one.
- The package's transitive dependency set is the standard library alone, in both the default and the integration lane, and the guard fails on any added edge.

STATUS: complete

SPEC CONTEXT: Spec section 2 sets every restored registration to resolve to eager or lazy from an install-wide default plus a three-state per-registration override (eager, lazy, or nothing = inherit). Section 2.1 fixes the shipped default as lazy. Section 3.1 puts the install-wide value in prefs.json `resume_mode` (tolerant decode: missing/empty/corrupt/unrecognised/unreadable -> shipped default). Section 3.2 stores the per-registration `resume` attribute in hooks.json, where an unrecognised value carries no mode and inherits. Section 3.3 has the CLI flag refuse any value but the two words. This task supplies the shared leaf vocabulary both guarded leaves (hooks, prefs) reach without an edge between them.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/resumemode/resumemode.go:12 — `type Mode int`
  - internal/resumemode/resumemode.go:14-22 — `Unset Mode = iota`, `Eager`, `Lazy`
  - internal/resumemode/resumemode.go:24-27 — unexported spellings `eagerSpelling`/`lazySpelling`
  - internal/resumemode/resumemode.go:31 — `const Default = Lazy`
  - internal/resumemode/resumemode.go:36-45 — `String()`: spelling for Eager/Lazy, "" for everything else
  - internal/resumemode/resumemode.go:51-60 — `Parse`: exact switch on the two spellings, `(Unset, false)` otherwise; no trimming, folding or prefix matching
  - internal/resumemode/resumemode.go:64-72 — `Resolve`: registration if not Unset, else install if not Unset, else Default; never answers Unset
  - CLAUDE.md:82 — `resumemode` package-table row, placed among the other stdlib-only leaves (after `nanoid`, `shellquote`, `resumekeys`), covering every point the task asked for
  - internal/tui/pagepreview_surface_audit_test.go:204 — new package added to the surface-audit allowlist (needed so that guard keeps passing)
- Notes: The file has no imports at all, so the stdlib-only property holds trivially by reading. Downstream consumers use it as designed: cmd/hooks.go:249 (the flag refuses on `!ok`), internal/hooks/registration.go:46 (the stored value tolerates `!ok` and carries Unset), internal/prefs/store.go:218 (unrecognised -> Default), cmd/state_hydrate.go:198 (`Resolve(...) == resumemode.Lazy`). No drift from the plan.

TESTS:
- Status: Adequate
- Coverage:
  - internal/resumemode/resumemode_test.go:10-43 — Parse table over exactly the ten planned inputs (eager, lazy, Lazy, LAZY, " lazy", "lazy ", laz, lazyy, "", "1"). Checks both the bool and the returned Mode, and forces Unset for every refused row, so a Parse that answered `(Lazy, false)` would fail.
  - internal/resumemode/resumemode_test.go:47-55 — String of Unset and of Mode(99) both "".
  - internal/resumemode/resumemode_test.go:57-71 — round-trip of Eager and Lazy through String then Parse, with a non-empty guard so two empty renderings cannot pass.
  - internal/resumemode/resumemode_test.go:75-94 — registration wins in all four named-registration cases, including eager-under-lazy and lazy-under-eager.
  - internal/resumemode/resumemode_test.go:96-102 — install fallback for both named install modes.
  - internal/resumemode/resumemode_test.go:104-112 — Unset/Unset answers Lazy and equals Default, which pins the shipped default to lazy (the Unset-install edge case).
  - internal/resumemode/leaf_guard_test.go:14-20 — `AssertDepsWithin(t, pkg, nil, ForbiddingThirdParty(), lane)` over `sourceguardtest.Lanes()` (default + integration), matching internal/nanoid/leaf_guard_test.go.
- Notes: Every planned test name is present. Every acceptance criterion and edge case is covered, and each test would fail if the behaviour it names broke. The tests are not over-built.

CODE QUALITY:
- Project conventions: Followed (leaf package with its own dependency guard in the nanoid/shellquote/resumekeys shape; external test package; no t.Parallel)
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good — the doc comments state the three-state model, the refusal policy split and the totality of Resolve. None references process artifacts.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
