TASK: resume-hooks-silently-lost-8-48 — "The Stand-Down Reason Vocabulary Is Closed By Naming Convention, Not By The Type System" (tick-934b0e, phase 8, severity low, source: bank)

Make the stand-down reason vocabulary a named type so an off-convention or literal reason cannot compile, re-type the shipped outcome and its consumers, and narrow/retire the name-prefix source guard where the type now holds the property.

ACCEPTANCE CRITERIA:
1. A raw string literal cannot be passed as a decline reason — it is a compile error.
2. An off-convention const of the typed kind still fails the enumerable-set and phrase-map guards.
3. Rendered lines and log attrs are byte-identical to today's.
4. The guard that remains states what it still adds over the type.

STATUS: complete

SPEC CONTEXT:
Specification §5.4 (line 310) fixes the observable this task must not disturb: a stood-down sweep emits one line shape — `op=clean-stale-skipped`, `via=internal`, and a `reason` attr naming one of six slugs (`restoring`, `marker-read-failed`, `lock-timeout`, `empty-pane-read`, `store-read-failed`, `pane-read-failed`). Line 387 pins `clean-stale-skipped` as one of exactly three new `op` values with **no new attr key** — `reason` is an existing key newly carried by the `hooks` component. The vocabulary is spec-governed, so the type change had to be purely internal: the slugs on the wire, and the user-facing copy the two doctor surfaces render from them, must be unchanged. This task adds no spec surface; it changes how the slugs are held in Go.

IMPLEMENTATION:
- Status: Implemented, then carried by later tasks (9-9, 9-12) which renamed `skipReason` → `hooksweep.Reason` and moved the whole cycle out of `cmd` into `internal/hooksweep`. The property this task delivered survives the move intact.
- Delivering commit: `bb738fb4` ("the type closes the reason vocabulary, the guard keys on it"), touching `cmd/run_hook_stale_cleanup.go`, `cmd/doctor.go` and five test files.
- Location (current tree, post-move):
  - `internal/hooksweep/reason.go:12` — `type Reason string`, with the residual named honestly at `:10-11` ("no string-typed value can be reported as one; an untyped constant still converts implicitly").
  - `internal/hooksweep/reason.go:19-25` — the six consts, each declared `Reason`.
  - `internal/hooksweep/reason.go:35-42` — `Reasons []Reason`, the enumerable set.
  - `internal/hooksweep/standdown.go:14-16` — `standDownAttrs(reason Reason, …)`, converting at the log-attr boundary only (`"reason", string(reason)`).
  - `internal/hooksweep/standdown.go:22-26` — `StandDown.reason Reason`; `:30` `Reason() Reason`; `:41` `declineDebug(reason Reason, …)`; `:47` `declineWarn(reason Reason, …)`; `:59` `string(e.reason)` at the error-text boundary.
  - `internal/hooksweep/sweep.go:57-60` — the shipped outcome, `Outcome.DeclineReason Reason`.
  - `cmd/doctor.go:236` / `:247` — both phrase maps keyed `map[hooksweep.Reason]string`; `:260` `phraseFor(m map[hooksweep.Reason]string, reason hooksweep.Reason)`; `:266` `reportSkippedPrune(w io.Writer, reason hooksweep.Reason)`; `:352` `staleHooksNotEvaluable(name string, reason hooksweep.Reason)`.
  - Every production decline site passes a declared const: `internal/hooksweep/sweep.go:75`, `:77`, `:88`, `:99`, `:185`, `:190` — six sites, counted, no literals.
  - No `skipReason` identifier survives anywhere in the tree, and `cmd/run_hook_stale_cleanup.go` no longer exists (folded into `internal/hooksweep` by T9-12) — no half-migrated leftovers.
- Criterion 1 — divergence, judged sound, recorded rather than reported. `type Reason string` cannot make a raw literal a compile error: an untyped string constant converts implicitly to a defined string type, so `declineWarn("anything")` still compiles. What the type does deliver is the strictly weaker "no string-**typed** value can be reported as one", which is exactly what the code claims at `internal/hooksweep/reason.go:10-11` — the implementation does not overstate what it bought. The criterion as worded was reachable in Go only by abandoning a string underlying type (a struct wrapper, which cannot be `const`, or an int enum needing a slug table between the vocabulary and the `reason` attr the spec pins). The hole the task's Problem statement actually named — "a const named without the prefix, absent from the enumerable slice and from both phrase maps, passes both guards" — *is* closed, because the guard now keys on the type rather than the name (see TESTS). The residual is one inline literal at a call site, in a package with six call sites all using declared consts. Intent delivered; the wording is not.
- Criterion 3 — byte-identical. The delivering diff changed signatures and added three `string(…)` conversions, at the log attr (`standdown.go:15`), the error text (`standdown.go:59`) and (at the time) `phraseFor`'s fall-through. No phrase, slug, level or attr key moved. The `reason` attr still carries the plain slug the spec names, so §5.4's line shape is untouched.
- Criterion 4 — the surviving guard states its residual. `internal/hooksweep/reason_enumeration_guard_test.go:15-21` names it directly ("Membership is what the type cannot hold, and it is what this guard still adds over it"), and `:66-71` explains why the name-prefix arm was narrowed rather than retired ("an untyped const in the block is a plain string to the compiler, and passing it as a reason still compiles"). The name arm is not redundant: an untyped `ReasonX = "x"` in the block is caught by the prefix and not by the type.

TESTS:
- Status: Adequate.
- Coverage:
  - Criterion 2, enumerable-set arm — `internal/hooksweep/reason_enumeration_guard_test.go:22-57`. `declaredReasonConsts` (`:72-87`) matches on the declared type (`isReasonType`, `:127-130`) *or* the name prefix, which is the change this task made: the old rule matched on name alone. The subtest the plan named, "it fails a reason absent from the enumerable set" (`:45-56`), drives the rule over synthetic source declaring `const offConvention Reason = "off-convention"` and asserts the rule reports exactly `[offConvention]` — the off-convention typed const the old name-keyed guard let through. The guard also fails on an empty scan (`:27-29`, `:110`), so it cannot pass by having stopped looking.
  - Criterion 2, phrase-map arm — `cmd/doctor_stand_down_phrase_guard_test.go:47-94`, ranging over `hooksweep.Reasons` for both vocabularies, with its own failure mode exercised at `:73-93` against a typed unmapped const. The two guards compose: an off-convention typed const fails the enumeration guard until it joins `Reasons`, and joining `Reasons` without phrases fails this one.
  - Criterion 3 — `cmd/doctor_stand_down_copy_test.go` is the byte-level pin. Every reason's whole rendered output is a table row (`:56-123`): the exact `Skipped stale hook prune: …` line, the exact `  · stale hooks: …` line, the log level, and the attrs. `:342-358` proves the table covers `hooksweep.Reasons` whole with no subtraction list; `:364-379` proves the six render distinct phrases on both surfaces; `:411-420` proves no surface prints the raw slug; `:289-309` drives the real cycle and asserts the emitted stand-down record's level and attrs. A type change that leaked into rendering or into the `reason` attr would fail here.
- Notes: No over-testing. The two source guards police different failure modes (set membership vs. map coverage) and each carries exactly one self-check against synthetic source rather than a family of near-duplicates. Criterion 1 carries no test, correctly — the compiler is the only thing that could assert it, and what it asserts is the weaker property the code documents.

CODE QUALITY:
- Project conventions: Followed. The `reason` attr value stays a plain string at the emission boundary (`standdown.go:15`), so the closed attr vocabulary in CLAUDE.md is unchanged; the guard is unit-lane and stdlib+`sourceguardtest` only, matching the ~20 sibling source guards; CLAUDE.md's `hooksweep` row already describes `Reason` as "the closed stand-down vocabulary (`Reasons` makes it enumerable, and a source guard fails a declared reason left out of it)" — the docs and the tree agree.
- SOLID principles: Good. The type is declared in the package that owns the cycle and exported for the one consumer that renders it; `cmd` holds the copy, `hooksweep` holds the vocabulary.
- Complexity: Low. The change is a re-typing; the only new logic is in the guard, where `forEachValueSpec` (`:133-147`) replaced a four-level nested switch with one traversal and two named rules.
- Modern idioms: Yes. `slices.Sort`/`slices.Contains`/`slices.Equal`, a defined string type as a map key, conversion pushed to the boundaries.
- Readability: Good. Both the type and the guard state their own limits in prose that matches the code.
- Comment accuracy: Verified line by line against the current tree. `internal/hooksweep/reason.go:10-11`, `:14-17`, `:30-34`, `internal/hooksweep/reason_enumeration_guard_test.go:15-21`, `:59-71` and `:89-91` all hold. No process-artifact references (no task ids, phases or spec section numbers) in any of the changed code.
- Issues: None reported.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
