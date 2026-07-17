TASK: Close the Fix 4 compile-guard's installed-but-not-running precondition gap (ghostty-spawn-zero-windows-2-2, tick-b461d6). Adopt a spec §Fix 4 remedy so the ghosttycompile guard cannot emit a false template-drift failure when Ghostty is installed but not running.

ACCEPTANCE CRITERIA:
1. The guard's rationale no longer rests solely on the "spawn feature is only ever invoked from a running Ghostty" analogy; discharged by recorded installed-but-not-running evidence OR by a defensive precondition that a not-running-caused resolution failure cannot be reported as template drift.
2. Invoking the guard with Ghostty installed but not running cannot produce a false t.Fatalf template-drift failure (passes on clean resolution, or t.Skips / is precondition-gated).
3. A genuine drift to the pre-fix `make new surface configuration` form still fails the guard (the -2741 discriminator preserved).
4. The corrected committed template still passes the guard.

STATUS: Complete

SPEC CONTEXT: Spec §Fix 4 (Prevention — Compile-check regression guard) designates exactly one "Assumption to confirm": whether osacompile terminology resolution *requires Ghostty to be running*. The investigation reproduced the -2741 terminology error via osacompile against installed Ghostty 1.3.1, but likely with Ghostty already running; the t.Skip-on-installed gate does not cover the not-running case. The spec prescribes two remedies: either (a) confirm on a live Mac and document the observed behaviour, or (b) adjust the precondition — "either ensure/require Ghostty is running as part of the gate, or t.Skip when terminology cannot be resolved" — so the guard never produces a false failure unrelated to the template.

IMPLEMENTATION:
- Status: Implemented (spec remedy path (b) — defensive code change, no live-Mac dependency)
- Location: internal/spawn/ghostty_compile_ghosttycompile_test.go:14-20 (new `driftDiscriminator = "-2741"` const), :59-88 (rewritten PRECONDITION RATIONALE comment), :112-134 (rewritten `if err != nil` classification block). Commit 3c7fc84b.
- Notes:
  - The pre-fix code hard-failed (t.Fatalf) on ANY osacompile error: a non-zero exit (`*exec.ExitError`) and a non-exit run error both fataled. The removed rationale explicitly leaned on the "spawn feature is only ever invoked from a running Ghostty" analogy (diff old lines 101-104).
  - The new code classifies the failure by SIGNATURE: only when `combined` output contains `-2741` does it t.Fatalf (template drift); every other error path — a non-`-2741` exit or a non-exit run error (e.g. osacompile absent from PATH, or a not-running resolution failure) — is now t.Skipf'd as environmental.
  - The `errors`/`errors.As` import was dropped and `strings` added; imports (os, os/exec, path/filepath, runtime, strings, testing) are all still used, so the file compiles under the tag.
  - The corrected template in ghostty.go:20-22 is the sdef-correct `new window with configuration {command:"%s", wait after command:true}` form, so osacompile exits 0 and the `if err != nil` block never triggers → guard passes today.
  - Path (a) was NOT taken: the LIVE-MAC CONFIRMATION comment still records only "Ghostty running" and is honest that the confirmation "happened to run with Ghostty up" — no false claim of not-running evidence. The rationale is discharged by the defensive precondition (reason #2) plus a supporting sdef-static-resource argument (reason #1).

Acceptance-criteria assessment:
- Criterion 1 — MET. The analogy comment is removed and replaced by a two-reason rationale (reason #1: sdef is a static bundle resource read at compile time, not the running process; reason #2, load-bearing: the guard fails ONLY on the -2741 discriminator and skips any other resolution failure). Reason #2 is a genuine defensive precondition, satisfying the "OR" branch of the criterion.
- Criterion 2 — MET. If not-running compiles clean (reason #1), err==nil and no failure fires. If not-running instead causes a resolution failure, that failure does not carry the -2741 grammar signature (see discriminator soundness below) → t.Skipf, not t.Fatalf. Either way a false template-drift failure is structurally impossible in the installed-but-not-running case.
- Criterion 3 — MET. The pre-fix `make new surface configuration with properties {…}` template yields the reproduced -2741 code against Ghostty's dictionary; `strings.Contains(combined, "-2741")` still hard-fails it. Discriminator preserved.
- Criterion 4 — MET. Corrected committed template compiles clean → exit 0 → err==nil → block skipped → pass.

Discriminator soundness (the core question):
- -2741 is an AppleScript compile/grammar error that arises only when the app's terminology (dictionary) HAS been resolved and the statement grammar is invalid against it (`make new surface configuration` is not a valid form for Ghostty's trimmed Standard Suite). A resolution failure caused by "Ghostty not running" — if it occurred at all — would surface as a terminology-not-available error with a DIFFERENT code, not -2741. So -2741 reliably means "dictionary loaded + template grammatically wrong" = genuine drift, and cannot collide with a not-running resolution failure. The discriminator correctly distinguishes the two.
- Could a real drift be mis-classified as "not running" and skipped? For the SPECIFIC pre-fix `make new surface configuration` form: no — it produces -2741 and is caught. This is exactly what criterion 3 scopes.
- The fix genuinely closes the gap (does not merely relabel it): for the not-running case the false-Fatalf is now impossible whether or not reason #1 holds. The one deliberate tradeoff is a narrowing of sensitivity — see NON-BLOCKING NOTES.

TESTS:
- Status: Adequate (the guard IS the test; this is a self-verifying tripwire, no separate test needed).
- Coverage: The single test TestGhosttyOpenScript_CompilesAgainstInstalledDictionary compiles ghosttyOpenScript(<representative env-self-sufficient argv>) via `osacompile -e <script> -o <t.TempDir()/probe.scpt>` and asserts exit 0, with -2741-signature drift detection retained and non-drift failures skipped. The representative argv (/usr/bin/env -u TMUX -u TMUX_PANE /bin/sh -c "echo probe") exercises the template AND ghosttyEmbed escaping together, matching the spec's prescribed shape.
- Notes: Not over-tested (one focused test). Would fail if the template regressed to the -2741 pre-fix form. Skips cleanly on non-darwin / Ghostty absent / non-drift resolution failure. No default-lane or integration-lane assertions are touched — the file is `//go:build ghosttycompile` and compiles into neither lane. I did not (and per instructions cannot) execute the guard; adequacy assessed by reading.

CODE QUALITY:
- Project conventions: Followed. Correct build-tag isolation (`//go:build ghosttycompile` + `_ghosttycompile_test.go` suffix), t.Skip for unmet preconditions, t.TempDir auto-cleanup, single-sourced `driftDiscriminator` const reused across both the fatal and skip messages. Consistent with the codebase's dedicated-tag pattern (manual / integration).
- SOLID principles: Good — single-responsibility guard; ghosttyAppInstalled factored out.
- Complexity: Low. One if/else classification branch.
- Modern idioms: Yes — signature-based classification via strings.Contains is simpler and more robust than the prior errors.As(*exec.ExitError) split.
- Readability: Good, arguably heavily commented, but appropriate for a subtle terminology-drift guard whose rationale must survive future edits.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/spawn/ghostty_compile_ghosttycompile_test.go:112-134 — The guard now hard-fails ONLY on the exact -2741 signature and t.Skips every other osacompile error. This is correct for the not-running case and is within this task's acceptance criteria (criterion 3 is scoped to the pre-fix `make new surface configuration` form), but it narrows the guard relative to the pre-fix "any non-zero exit fails" behaviour: a NOVEL future template drift that produced a different AppleScript compile code (not -2741) would be silently t.Skip'd rather than caught. Decide whether that residual sensitivity loss is acceptable, or whether a broader "template drift vs environmental" discriminator is warranted (which would require either live not-running evidence per spec path (a), or a positive compile-success signal to distinguish clean resolution from a skipped resolution). Requires a design decision, hence idea rather than quickfix.
