TASK: resume-hooks-silently-lost-1-8 — A Shared Test Vocabulary For Hook-Key Seeds (consolidation finding F3, severity: duplication)

ACCEPTANCE CRITERIA:
- Both constructors live in one package reachable from `cmd`, `cmd/bootstrap` and `internal/hooks`
- `ReapableHookKey` fails loudly if its output is not token-shaped
- No hand-rolled token-shaped hook-key literal survives at a seed site
- Live-set-coupled positional keys are unchanged
- Every test keeps its current name, cases and expected counts
- `go test ./...` and `go test -tags integration -p 1 ./...` both pass

STATUS: complete

SPEC CONTEXT:
This task is a phase-1 consolidation finding (`consolidation-findings-p1.md` F3), so its authority is its own body rather than the specification. It exists to protect the specification's staleness contract: spec:121-129 makes a key "token-shaped" iff it is exactly the pane-token width drawn from `nanoid.Alphabet`, spec:268-270 makes shape the sole discriminator between a reaped and a retained entry, and spec:466 records that shape is what moves an entry into or out of the protected class. Because shape alone decides reap-vs-retain, a hand-rolled seed literal that drifted by one byte would silently convert a reap test into a retention test and stay green — which is exactly the failure the vocabulary is built to make impossible. The 2026-08-30 / 2026-09-01 corrigenda re-home the predicate in `internal/nanoid`, which is the package the delivered vocabulary asserts against.

IMPLEMENTATION:
- Status: Implemented (delivered, then relocated by later phases — the relocation is sound)
- Location:
  - `internal/hookstest/hooks.go:19-35` — `seedKeyPrefix`, `disambiguatorWidth`, and `paneTokenWidth` taken from the mint (`nanoid.NewPaneTokenGenerator`) rather than restated
  - `internal/hookstest/hooks.go:129-142` — `tokenShapedHookKey(n)`, which panics both on an out-of-range `n` and when its own output fails `nanoid.IsTokenShaped`
  - `internal/hookstest/hooks.go:146-154` — `fitPrefix`, which panics when the token width leaves no room for a legible prefix
  - `internal/hookstest/hooks.go:159-161` — `unjudgeableHookKey(n)`, the legacy `<name>:window.pane` shape
  - `internal/hookstest/hooks.go:167-199` — the named seed vars: `ReapableSeedA-D`, `LiveSeedA-C`, `UnjudgeableSeedA-C`, `SubjectSeedA-D`
  - `internal/hookstest/hooks.go:206-209` — `StaleHookSeed`, the stale-beside-live body keyed on `ReapableSeedA` / `LiveSeedA`
  - Original delivery: commit `c5452064`, which sited `ReapableHookKey` / `UnjudgeableHookKey` in `internal/transienttest` exactly as the task's Do list wrote it.
- Notes:
  - **Drift from the task's wording, and it is sound.** The task named `internal/transienttest` and two exported constructors. The delivered end state puts the vocabulary in `internal/hookstest` with the constructors *unexported* behind named vars. Nothing is lost: the shape assertion still runs (now at package-var init, so a mis-shaped seed panics before any test in any importing package runs), the one-declaration property is strengthened (a fixture can no longer mint an unnamed key of its own), and the roles gained two halves the original lacked — `LiveSeed*` and `SubjectSeed*` — which separate "survives because its pane is live" from "survives because the rule cannot judge its shape". `internal/transienttest/doc.go:1-4` now describes only the `list-panes -a` scaffolding, matching what the package holds. CLAUDE.md's `hookstest` architecture row documents the delivered shape. No leftover references to `transienttest.ReapableHookKey` / `UnjudgeableHookKey` exist anywhere in the tree.
  - AC1 holds in substance: `internal/hookstest` is imported by `cmd` (e.g. `cmd/doctor_test.go`, `cmd/hooks_test.go`), `cmd/bootstrap` (`cmd/bootstrap/transient_listpanes_helpers_integration_test.go:103`) and `internal/hooks` (`internal/hooks/store_test.go:18`, package `hooks_test`, so no cycle). `internal/hooksweep` — the package the sweep moved to in phase 9 — reaches it too.
  - AC2 holds and is stronger than asked: `internal/hookstest/hooks.go:138-140` panics naming the drift ("the seed vocabulary has drifted from nanoid.IsTokenShaped"), and because the seeds are package vars the panic fires at init rather than at a call site a suite might not reach.
  - AC3 holds. Every literal F3 enumerated (`sessA1`, `keyA00`–`keyD00`, `gone01`, `stalA0`, `alpha1`, `smoke1` and the rest) is gone from the tree. Every fixture that actually exercises the staleness rule now names a role: `cmd/doctor_test.go:1189-1193`, `internal/hooks/store_shape_test.go:15,40`, `internal/hooks/cleanstale_snapshot_test.go:26`, `internal/hooksweep/sweep_test.go:200,224-247`. 481 named-seed references across 40 files.
  - AC4 is moot in the delivered end state rather than violated: phases 2+ moved the live enumeration off positional coordinates onto pane tokens, so the positional keys the criterion protected no longer exist as live-set members. The `LiveSeed*` half is their successor and is used as the live set throughout.
  - AC5 holds for the commit itself — `git show c5452064` adds `TestHookKeySeedVocabulary` and changes no existing `func Test` or `t.Run` name; the re-points swap literals for names inside unchanged assertions and counts.
  - AC6 (`go test` / integration lane) is not verifiable here — running tests is outside this review's remit. Nothing read suggests a compile break: every referenced seed name resolves to a declaration, and the `hookstest` package's only new dependency (`internal/nanoid`) is a stdlib-only leaf.

TESTS:
- Status: Adequate
- Coverage: `internal/hookstest/hooks_test.go:37-75` (`TestHookKeySeedVocabulary`) covers the guarantee the task asked for and more: every named token-shaped seed is token-shaped (`:38-44`), every one is authored at the width the live mint returns rather than a hardcoded 6 (`:46-56`), an unjudgeable seed is *not* token-shaped (`:58-64`), and every named seed across both halves is distinct (`:66-74`). The two maps at `:16-34` enumerate the whole vocabulary by name, so a seed added to `hooks.go` without being enrolled is visible as an absence rather than covered by a prefix of the set. The re-pointed hook-cleanup, retention and doctor suites are the second half of the proof, exactly as the task's Tests section stated.
- Notes:
  - The width assertion at `:46-56` is the one that carries the task's real payoff — it is what makes a `nanoid.paneTokenWidth` move a loud failure in one place instead of eighteen silently-reclassified fixtures.
  - Not over-tested: four subtests over one small vocabulary, none redundant with another (shape, width, negative shape, distinctness are four different failure modes).
  - The guarantees are additionally enforced outside the test: `tokenShapedHookKey`'s own panic fires at package init, so a drift fails every importing package's binary rather than only this suite.

CODE QUALITY:
- Project conventions: Followed. The package is test-only, lives outside `_test.go` so other packages' tests can import it, and says so (`internal/hookstest/doc.go:17-18`). It takes no `internal/log` edge and reaches `nanoid` — the leaf the corrigenda designate as the shape's home — rather than restating a width or a charset. CLAUDE.md's `hookstest` architecture row matches what the package holds.
- SOLID principles: Good. One reason to change per declaration: `tokenShapedHookKey` mints and asserts, `fitPrefix` fits the prefix to the width, `unjudgeableHookKey` renders the legacy shape. The seed vars are the only exported surface, which is what makes "no package re-derives a key of its own" structural rather than advisory.
- Complexity: Low. No branching beyond three range/width guards, each with a message that names the drift it caught.
- Modern idioms: Yes. `sync.OnceValue` for the one-shot width read (`:29-35`) is the right primitive, and taking the width from a minted token is the only route available given `nanoid.paneTokenWidth` is deliberately unexported — the comment at `:27-28` says so.
- Readability: Good. The var block's comments (`:167-198`) explain why the four halves are kept apart rather than restating what the code does, and each panic message names the invariant it is defending.
- Issues: None that clear the bar. Two observations recorded and deliberately not raised as findings: (a) `internal/hooks/lock_write_test.go:34,52,107,123,165` and `internal/hooks/store_test.go:465` still spell token-shaped literals (`tok123`, `tok999`, `abc123`), but their subjects are the mutation lock and a no-op `Remove` — no staleness rule runs on those paths, so the shape decides nothing and the failure AC3 guards against cannot occur; (b) `internal/hooks/store_test.go:834` and `internal/hooksweep/sweep_test.go:409` place a `ReapableSeed*` in a live set rather than a `LiveSeed*`. Both are correct — the two halves are the same shape, so which set holds a key is what decides the outcome, not which name it carries — and the remedy in each case is a rename with no consequence attached.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
