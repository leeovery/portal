TASK: Portal Hook Set --resume-mode (lazy-resume-on-attach-1-6, tick-8da5c0)

ACCEPTANCE CRITERIA:
- `hook set --on-resume "<cmd>" --resume-mode eager` and `… lazy` each write the object form under the pane's hook key, carrying that command and that mode.
- `hook set --on-resume "<cmd>"` with no mode flag writes the string form, and does so over an object-form predecessor — the stored entry keeps no mode and no unmodelled attribute.
- `hook set --resume-mode lazy` with no `--on-resume` exits non-zero, writes nothing, and performs no tmux read.
- `--resume-mode` with an unrecognised value, and with an explicitly empty value, each exit non-zero and write nothing — with no pane-key read, no token mint, no `set-option -p` stamp and no `save.requested` touch.
- A refusal names both accepted values in its message; the wording is one line and goes to the command's error stream.
- A recognised mode leaves the rest of the command unchanged: an unstamped pane is still stamped before the entry is written, a stamped one is reused with no `set-option`, and `save.requested` is still touched after a successful write.

STATUS: complete

SPEC CONTEXT: §3.3 puts the per-registration override on `portal hook set` as `--resume-mode eager|lazy` beside `--on-resume`, because registrations are written by a script (the external SessionStart hook). A mode is never passed alone (no entry a mode could attach to), and an unrecognised value is refused on the writer's side so a typo fails where it was typed; the reader stays tolerant. §2.2: a registration is written whole — a call that passes no mode writes one carrying none, whatever its predecessor held. §3.2: the writer picks the string form when there is nothing to carry and the object form (`{"command","resume"}`) when there is.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/hooks.go:38 — `resumeModeFlagName` constant
  - cmd/hooks.go:199-202 — mode resolved as the first act of `hooksSetCmd.RunE`, ahead of the `--on-resume` read, `resolveCurrentPaneKey()`, the mint/stamp, the store write and the `save.requested` touch
  - cmd/hooks.go:227 — `hooks.Registration{Command: command, Resume: mode}` handed to `store.Set`; pane-key resolution, lazy stamp and touch unchanged
  - cmd/hooks.go:239-256 — `resumeModeFlag`: unpassed (`Changed` false) → `resumemode.Unset`; passed → strict `resumemode.Parse`, a rejection (the empty string included) returns `NewUsageError("--resume-mode must be \"eager\" or \"lazy\"")` (exit 2 via main.go's classify)
  - cmd/hooks.go:337-338 — flag registered; `MarkFlagRequired("on-resume")` left as it was, so cobra refuses a lone `--resume-mode` before RunE
  - README.md:224, README.md:230 — example-block line and the paragraph the task asked for (eager/lazy/nothing, nothing follows `prefs.json`, unpassed flag drops the predecessor's mode)
- Notes: Matches the plan's wording. Traced the error path: `rootCmd` sets SilenceErrors, so the returned error reaches stderr once through main.go's `classify` (one `Fprintln`), and `*UsageError` maps to exit 2. `%q` against the `resumemode.Mode` constants goes through `String()`, and the test pins the exact rendered message. A predecessor that differs only by its mode is rewritten as a `modify` (store.go `classifySet`/`sameRegistration`), so the README's "whatever its predecessor carried" holds. One case is a no-op: an object-form predecessor with the same command, no recognised mode and an unmodelled attribute is left untouched. The owning store task says so on purpose (phase-1 task text: "an identical object-form rewrite is still a no-op … unmodelled attributes survive"), so it is not a drift in this task.

TESTS:
- Status: Adequate
- Coverage (cmd/hooks_resume_mode_test.go, `TestHooksSetResumeMode`):
  - eager/lazy subtests assert the exact compacted raw JSON `{"command":"npm start","resume":"<mode>"}` under the resolved key, zero `set-option` calls for an already-stamped pane, and that `save.requested` was touched (criteria 1 and 6).
  - "no mode flag" asserts the raw stored value is the JSON string form (criterion 2).
  - "drops an object-form predecessor's mode" seeds `{"command":"old-cmd","resume":"lazy","note":"kept"}` and asserts the result is exactly `"new-cmd"`, which fails if a mode or unmodelled attribute were carried forward (criterion 2).
  - "refuses --resume-mode with no --on-resume" asserts cobra's required-flag error, zero resolver and stamper calls, and no hooks.json created (criterion 3).
  - "refuses an unrecognised mode" asserts resolver call count 0, stamper untouched, seeded hooks.json byte-identical, and `save.requested` absent (criterion 4). The mint needs a resolved empty key, so a zero resolver count also rules out a mint.
  - "refuses an explicitly empty mode" (`--resume-mode ""`, which pflag records as Changed with value "") asserts non-zero exit, resolver count 0, and no file (criterion 4). This fails if the `Changed` gate were dropped and "" treated as Unset.
  - "names both accepted values" pins the exact one-line message and the `*UsageError` type (criterion 5).
  - "still stamps a freshly minted token" asserts one `@portal-pane-id` stamp, that hooks.json was absent when the stamp ran, and the pinned registration under the minted token (criterion 6).
  - Helper changes (cmd/testhelpers_test.go:180 variadic `runHookSet`, cmd/testhelpers_test.go:221 `readHooksJSON` → `hooks.Snapshot`), existing callers moved to `.Command`, and `resetRootCmd` resets the new flag's value and `Changed` (cmd/root_test.go), so a pinned run cannot leak into a later unpinned one.
- Notes: Every tmux-touching seam a path can reach is injected through `withHooksDeps`. Where no stamper is injected, the resolver answers a non-empty key, so the production stamper is never reached. None of the eight tests duplicates another's assertions.

CODE QUALITY:
- Project conventions: Followed (`NewUsageError` for a malformed flag value like the other CLI refusals; seams through `hookSeams()`; `withHooksDeps`; no `t.Parallel`; vocabulary taken from `internal/resumemode` rather than restated)
- SOLID principles: Good — flag parsing is split out of the command body
- Complexity: Low
- Modern idioms: Yes
- Readability: Good; the comments at cmd/hooks.go:197-198 and 236-238 match the code
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
