TASK: Fix 4 (Prevention) — Compile-check regression guard (ghosttycompile-tagged) for the Ghostty AppleScript template (internal ID suffix 1-4)

ACCEPTANCE CRITERIA:
1. A new //go:build ghosttycompile-gated file exists at internal/spawn/ghostty_compile_ghosttycompile_test.go.
2. The test feeds ghosttyOpenScript(<representative argv>) through `osacompile -e <script> -o <t.TempDir()/probe.scpt>` and asserts a zero exit; a non-zero exit fails with the captured compiler output.
3. It t.Skips cleanly when not macOS or when Ghostty.app is absent (no hard failure).
4. Representative argv is the env-self-sufficient shape []string{"/usr/bin/env","-u","TMUX","-u","TMUX_PANE","/bin/sh","-c","echo probe"} so template + ghosttyEmbed escaping are exercised together.
5. osacompile opens no window (compile-only).
6. The ghosttycompile tag is excluded from `go test ./...` and `go test -tags integration ./...`; guard runs only via `go test -tags ghosttycompile ./internal/spawn/`.
7. The live-Mac assumption about whether Ghostty must be running for terminology resolution is confirmed and the precondition adjusted (or documented as unnecessary).
8. With Task 1.1's corrected template the guard compiles clean (exit 0); pre-fix template would fail with -2741.

STATUS: Complete

SPEC CONTEXT:
Spec §Fix 4 (Prevention) mandates an automated compile-check to prevent recurrence of AppleScript terminology drift — the root cause of the zero-windows defect, which shipped because the only test hitting the real osascript boundary was //go:build manual (never run before tagging 0.9.1). The guard feeds the composed script through osacompile (compile-only, resolves `tell application "Ghostty"` terminology against the installed sdef, opens no window) and asserts exit 0. The pre-fix `make new … with properties` template yields -2741; the corrected `new window with configuration {…}` compiles clean. Spec §"Assumption to confirm" leaves one genuinely-open nuance: whether Ghostty must be *running* for terminology resolution, to be confirmed on the live Mac and the precondition adjusted so no false failure arises. Spec §Testing & Validation Requirements requires the tag stay excluded from both default lanes. NOTE: task 2-2 (later) hardens the installed-but-not-running precondition; per the orchestrator's instruction the file's FINAL on-disk state was verified, which already reflects 2-2's defensive signature-classification.

IMPLEMENTATION:
- Status: Implemented (final state reflects both this task's confirmation comment and task 2-2's defensive hardening)
- Location: internal/spawn/ghostty_compile_ghosttycompile_test.go (whole file, 153 lines); template under test at internal/spawn/ghostty.go:20-22 and 42-44 (ghosttyOpenScript / ghosttyScriptTemplate).
- Notes:
  * AC1 met — line 1 `//go:build ghosttycompile`, correct filename/location.
  * AC2 met — line 112 `exec.Command("osacompile", "-e", script, "-o", out)` with `out = filepath.Join(t.TempDir(), "probe.scpt")` (line 110); `script = ghosttyOpenScript(argv)` (line 106). On a non-zero/-error path the -2741 signature hard-fails via t.Fatalf carrying the captured `combined` compiler output (lines 119-124). This is a deliberate refinement of the literal "any non-zero exit fails" wording: task 2-2's defensive classification hard-fails ONLY on the established -2741 drift discriminator and t.Skips every other resolution failure (lines 125-134) so an installed-but-not-running Ghostty cannot produce a false template-drift failure. This is the expected FINAL state per the orchestrator note, not drift.
  * AC3 met — lines 90-95 t.Skip on non-darwin and on absent Ghostty.app; helper ghosttyAppInstalled() (lines 141-152) checks both /Applications and ~/Applications.
  * AC4 met — lines 101-104 are the exact prescribed literal, byte-for-byte.
  * AC5 met — osacompile is compile-only by nature; documented at lines 43-47 and 105-109.
  * AC6 met — verified via `go list` (see TESTS): file compiles into NEITHER default nor integration lane, only under -tags ghosttycompile.
  * AC7 met — lines 49-88 record the live-Mac confirmation (GOOS=darwin, Ghostty installed + running, corrected template exit 0 / no window; pre-fix template -2741 / no window) AND a two-pronged precondition rationale: (1) terminology resolves from the installed bundle's static sdef, not the running process, so installed-presence is the correct gate; (2) load-bearing defensive guarantee — the guard fails ONLY on -2741 and skips any other resolution failure, so a not-running/osacompile-absent case can never be a false failure. This is precisely the "adjust the precondition so the guard never produces a false failure" the spec asked for.
  * AC8 — the corrected template is committed at ghostty.go:20-22 (`new window with configuration {command:"%s", wait after command:true}`) and the guard keys on -2741; per instruction osacompile was NOT executed here, and the in-file live-Mac confirmation comment (lines 49-57) records exit 0 for the corrected form and -2741 for the pre-fix form. Consistent with AC8.

TESTS:
- Status: Adequate (this task IS the test; verification is of the test's correctness and isolation)
- Coverage:
  * Build-tag isolation VERIFIED empirically:
      - `go list -f '{{.TestGoFiles}} {{.XTestGoFiles}}' ./internal/spawn/` → ghostty_compile file NOT present (unit lane clean).
      - `go list -tags integration ...` → NOT present (integration lane clean).
      - `go list -tags ghosttycompile ...` → present (guard compiles only under its tag).
  * `grep -rn ghosttycompile` confirms the tag string appears in no other .go file — no accidental inclusion.
  * Compiles clean under its tag: `go vet -tags ghosttycompile ./internal/spawn/` → exit 0 (build-only verification; osacompile NOT executed per instruction).
  * ghosttyAppInstalled has a single definition (no redeclaration collision with the manual test, which carries a different tag).
  * ghosttyEmbed's escape substitutions for quotes/backslashes are separately unit-tested by TestGhosttyEmbed / TestGhosttyOpenScript in ghostty_command_test.go, so the compile guard's plain argv (no special chars) is the correct division of labour — the guard proves terminology resolution end-to-end; the embed unit tests prove escaping.
- Notes: Focused single test; not over-tested (no redundant assertions), not under-tested for its stated purpose. The pass condition is a clean osacompile exit (err == nil), which is the correct semantics for a compile guard.

CODE QUALITY:
- Project conventions: Followed. Heavy doc-comment density matches the codebase house style; build-tag-in-filename convention (`_ghosttycompile_test.go`) mirrors the existing `_manual_test.go` / `_integration_test.go` / `_realtmux_test.go` naming. Uses t.TempDir() for auto-cleanup and CombinedOutput() (aligns with the repo's stderr-preserving boundary preference).
- SOLID principles: Good — single-responsibility helper (ghosttyAppInstalled) extracted; test body is linear.
- Complexity: Low — one guard function, one helper, no branching beyond the two skip gates and the failure-classification split.
- Modern idioms: Yes — runtime.GOOS gate, os.UserHomeDir, filepath.Join, exec.Command CombinedOutput.
- Readability: Good — arguably verbose (the rationale comment is ~40 lines), but the verbosity is load-bearing here: it records the spec-required live-Mac confirmation and the precondition discharge, which is exactly the AC7 evidence. Acceptable.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/spawn/ghostty_compile_ghosttycompile_test.go:119-134 — the signature-classification hard-fails ONLY on the exact "-2741" string and t.Skips every other resolution failure. This is task 2-2's deliberate, documented trade-off (favours zero false-failures over sensitivity), but it does narrow the guard: a hypothetical FUTURE drift to a different-but-also-invalid template that yields a terminology error code other than -2741 would be silently skipped rather than caught. Consider (a decision, hence idea; and 2-2's scope, not 1-4's) whether the drift discriminator should also cover the general "not defined"/errOSANumbers terminology-error class rather than the single literal code. No action required for this task — surfaced only so the sensitivity boundary is on record.
- [do-now] internal/spawn/ghostty_compile_ghosttycompile_test.go:20 — the doc comment on driftDiscriminator says "-2741" is "the ONLY failure signature this guard treats as a genuine template regression"; consider adding a one-line pointer that this narrowing is task 2-2's defensive-precondition decision, so a future reader connects the classification to the installed-but-not-running rationale block below it (purely a comment cross-reference; no logic impact).
