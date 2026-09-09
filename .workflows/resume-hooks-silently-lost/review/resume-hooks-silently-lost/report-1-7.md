TASK: resume-hooks-silently-lost-1-7 — "Correct The Claims The Shape Rule Narrowed" (severity: comments). Narrow three prose claims that phase 1's own changes made false-in-part: the empty-live-set comment in `cmd/run_hook_stale_cleanup.go`, the guard comment above `checkStaleHooks` in `cmd/doctor.go`, and `CLAUDE.md`'s "What's instrumented" logging bullet. Prose only — no code, no test, no assertion.

ACCEPTANCE CRITERIA:
- [ ] Neither in-source comment claims a scope wider than the staleness rule now permits
- [ ] `CLAUDE.md`'s logging bullet is true of the hooks clean-stale sweep
- [ ] The diff contains no non-comment source change
- [ ] `go test ./...` and `go test -tags integration -p 1 ./...` both pass

STATUS: complete

SPEC CONTEXT:
Two spec sections govern the claims this task corrects.

§5.2 ("Deletion becomes shape-aware", specification.md:266-274) makes the staleness rule shape-aware: a token-shaped key absent from the live set is deleted, an **empty** key is deleted, and a key of any other shape is retained untouched on every run. It also pins the rule to one implementation — the exported `StaleKeys` in `internal/hooks` — through which every reader of staleness routes. That narrowing is exactly what made the two in-source comments overstate: after it, an empty or unreadable live set can only endanger the *judgeable* subset, never "every entry".

§5.3 (specification.md:286-290) promotes the per-removed-key line from DEBUG to INFO under the `hooks` component, carrying the removed entry's `on-resume` command in the existing `value` attr alongside `hook_key` — "promoted, not duplicated". That promotion is what made `CLAUDE.md`'s blanket "per-item detail at DEBUG" generalisation a claim the reaper contradicts, and it is what the doc bullet had to absorb.

IMPLEMENTATION:
- Status: Implemented (and still holding at HEAD after four later phases rewrote both comment regions)
- Location:
  - Commit c23e9c43 — the whole delivered change-set: `CLAUDE.md` (1 line), `cmd/doctor.go` (comment, 2 lines), `cmd/run_hook_stale_cleanup.go` (comment, 2 lines), plus `.tick/tasks.jsonl` and `.workflows/.../manifest.json` bookkeeping. No non-comment source line in the diff, and no test file touched.
  - At HEAD, claim 1 lives at `internal/hooksweep/sweep.go:93-97` — the sweep was re-homed out of `cmd/run_hook_stale_cleanup.go` by task 9-12 and the comment travelled with it, still narrowed: "it must never reach the staleness rule, which would judge every key it can parse stale."
  - At HEAD, claim 2 lives at `cmd/doctor.go:356-362`, rewritten by a later phase and still narrowed: "Past the ladder every judgeable entry is counted: an unreadable pane list, or a server with no panes at all, would otherwise report the lot stale…"
  - At HEAD, claim 3 is `CLAUDE.md:104` — the "What's instrumented" bullet now reads "…including one INFO line per key the hooks clean-stale sweep removes, carrying the removed `on-resume` command so a reaped hook is recoverable at the production default level".
- Notes: I verified each claim against the code rather than against the diff, since three of the four regions were rewritten downstream.
  - `hooks.StaleKeys` (`internal/hooks/store.go:248-263`) admits a key as stale only when it is absent from `live` **and** `key == "" || nanoid.IsTokenShaped(key)`. So "every key it can parse" (sweep.go:94) and "every judgeable entry" (doctor.go:358) are both exact, and neither overstates.
  - `checkStaleHooks` (`cmd/doctor.go:363-392`) counts through that same `hooks.StaleKeys` at line 387 — it is a judgeable-only count, which is what its comment claims.
  - The per-key INFO the doc bullet asserts exists at `internal/hooks/store.go:350-353`: `logger.Info("clean-stale", "op", "clean-stale", "hook_key", key, "value", removedValue(h[key]), …)`, emitted after the write from the store method (the chokepoint the bullet names), one line per removed key, carrying the command from the pre-delete map. `removedValue` (store.go:366) renders the command itself when the key holds one event, and `EventOnResume` (`internal/hooks/event.go:14`) is the only declared event, so the bullet's "the removed `on-resume` command" is true as written today.
  - The bullet's surviving second clause ("cycle summaries … the `clean:` sweeps … per-item detail at DEBUG") does not re-assert the corrected claim: `clean:` names the `clean` component (`cmd/bootstrap/bootstrap.go:16`, `internal/state/fifo_sweep.go:13`), while the reaper's summary renders under `hooks`. The generalisation and the new clause do not collide.
  - I swept for surviving wide-scope wording across `cmd/` and `internal/` ("delete every", "report every", "every entry", "mass-delet"): the remaining hits are all correctly narrowed — `internal/hooks/store.go:245` ("an empty live set makes every judgeable persisted key stale") and `cmd/doctor_test.go:1063` ("would report every token-shaped key on the machine as lost").

TESTS:
- Status: Adequate (none added, correctly)
- Coverage: The task is prose-only and rightly carries no test surface. The behaviour the corrected claims describe is independently pinned: `internal/hooksweep/sweep_test.go:224` (`TestUnjudgeableHookKeyRetention`) proves an unjudgeable key survives a sweep, and `cmd/doctor_test.go:1121` proves the diagnosis fails on one genuinely stale token-shaped key while retaining two unjudgeable ones — i.e. the count is judgeable-only, as doctor.go:358 says.
- Notes: No assertion was touched by the commit, so the "existing suites prove nothing changed" claim in the task's Tests section holds by construction. Judged by reading: a comment-only diff cannot alter any assertion, and no guard in the tree scans these comment bodies (the nearby guards — `cmd/doctor_stand_down_phrase_guard_test.go`, `internal/hooksweep/reason_enumeration_guard_test.go` — key on declared constants and reason values, not prose).

CODE QUALITY:
- Project conventions: Followed. `CLAUDE.md`'s logging section is the binding home of the instrumentation description, and the amendment names the component-and-level facts rather than inventing vocabulary; the sentence reads as one clause rather than a caveat, which is what the task asked for.
- SOLID principles: N/A (no code changed)
- Complexity: N/A
- Modern idioms: N/A
- Readability: Good. Both comments still state the hazard rather than re-arguing it, which was the explicit instruction ("Do not re-argue the guard; the hazard is real, only its scope moved"), and both survived two downstream rewrites with the narrowing intact.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
