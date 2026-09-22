## Attempt 1

ISSUES:
- `cmd/state_resume_draw.go:126-149` — nothing exercises the cobra command. The RunE flag→payload mapping (five `GetString` calls), the `--command` required marking, and the fact that the argv `resumeChainArgv` composes for `resume-draw` parses against the flags it registers are all unverified; `resumeDrawRunFunc` (`:117`) exists for exactly this and no test uses it. A transposition (`pane` ↔ `pane-key`) or a payload field whose flag is never registered compiles, passes every current test, and in production yields a chain whose records name the wrong pane — or, on the tail's path, an argv that fails to parse and closes the pane the tail exists to keep open. The package already has this pattern: `cmd/state_test.go:124-166` drives `state hydrate` through `rootCmd.Execute()` with `withFuncSeam(t, &hydrateRunFunc, …)`.
  FIX: Add two subtests driving the real command. (1) Stage the seam — `withFuncSeam(t, &resumeDrawRunFunc, func(cfg resumeDrawConfig) error { got = cfg; return nil })` — then `resetRootCmd()`, `rootCmd.SetArgs(resumeChainArgv("portal", resumeDrawSubcommand, payload)[1:])`, `Execute()`, and assert the captured `cfg.resumeChainPayload`'s five string fields equal the payload's (leave Width/Height out of the comparison — the draw measures the pane itself); parsing that argv at all is what proves `--width`/`--height` are registered. (2) A subtest asserting `Execute()` returns an error for `state resume-draw` with no `--command`, mirroring `TestStateHydrateRequiresFIFOAndFile`. While in `cmd/state_test.go`, finish the registration update the task began: add `"resume-draw"` to the `names` list at `:261` and the `hidden` list at `:287` (both are complete-set enumerations with no catch-all), and correct the subtest name at `:236` — it reads "it registers exactly five hidden state children" while asserting 6.
  CONFIDENCE: high
- `cmd/state_resume_draw.go:41-46` — the ordering the task calls load-bearing is only half pinned. `"it writes the alternate-screen entry before the panel"` pins enter-before-panel, but nothing pins that `ResolveTheme` (and with it the appearance query, and any input drop the chain later hangs off it) completes before a byte of screen is written. A later edit moving the resolve below the writes passes every test, and the corrigendum's failure follows: the terminal's late OSC 11 reply then lands after a screen was painted, where its leading `ESC` and any hex `d` can open and answer the discard confirmation on a pane nobody touched, destroying the only copy of a user-authored command.
  FIX: In `newResumeDrawConfig`'s `ResolveTheme` fake (`cmd/state_resume_draw_test.go:40-43`), record `p.stdout.Len()` at call time on the probe, and add a subtest — e.g. `"it resolves the theme before it paints"` — asserting that recorded length is 0.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- cmd/state_resume_chain.go:73-74 — `os.Executable` answers a platform it cannot serve with an error, not with an empty path, so the stated cause is false; the consequence is the part worth keeping.
  OLD: // An empty path is an error: os.Executable reports one on platforms that cannot
// answer, and exec'ing it would replace the process image with nothing.
  NEW: // An empty path is an error: exec'ing it would replace the process image with
// nothing.
- cmd/open_theme_nomination_test.go:18-21 — the blanket claim no longer holds for all three files: `state_resume_draw.go` resolves through `newThemeLoader()` (loud), so the runtime half would catch a call from the exec path there, unlike `theme.go` and `doctor_theme.go`, which both take `theme.NewSilentLoader()`. The edit also left line 19 unwrapped.
  OLD: // Three files are exempt in full because each is a separate verb no `portal
// open` invocation reaches. The runtime half does not back that exemption up: doctor
// hands its loader log.Discard(), so a doctor-side helper called from the exec
// path would read the poisoned directory and still write no record.
  NEW: // Three files are exempt in full because each is a separate verb no `portal
// open` invocation reaches. For the two whose loader is silent, the runtime half
// does not back that exemption up: doctor hands its loader log.Discard(), so a
// doctor-side helper called from the exec path would read the poisoned directory
// and still write no record.

NOTES:
- The NO_COLOR background assertion (`cmd/state_resume_draw_test.go:173`) tests for `"\x1b[48;"`, which misses a combined run (`\x1b[38;2;…;48;2;…m`); the repo's other guards use `"48;2;"`. It is not vacuous as written — a coloured pane canvas does emit standalone `\x1b[48;2;…m` runs — but the sibling subtest's exact comparison against `RenderResumePanel` would be strictly stronger here too, with `Colourless: true` in the expected `ResumeScreen`.
- `TestPaneDrawNomination/"it never takes the migrating prefs route"` sets `cfg.ResolveTheme = paneDrawTheme`, so it runs the real `paneAppearanceProbe` against the process's own stdin/stdout. Verified empirically that `go test` hands the test binary `/dev/null` (not a tty), so `detect()` short-circuits to dark and writes nothing — safe under the project's documented invocation. Run from a tty-attached test binary it would `MakeRaw` the developer's terminal and write an OSC 11 query. Asserting on `paneDrawNomination` directly (which is where the prefs route actually lives) would remove the exposure entirely.
- `withOsExitFake`'s doc (`cmd/state_daemon_self_supervision_test.go:21-23`, untouched by this diff and out of scope) says the replacement "is expected to panic"; the new exec-failure test's fake returns instead. Harmless here — `runResumeDraw` only returns nil afterwards — but the doc now generalises past its callers.
- The executor's two self-reported caveats are accurate and the reviewer agrees with both calls: the path/read rows of `TestPaneDrawNomination` cannot distinguish the degrade from the ordinary empty-keys route (only the resolution-error row can), and the "no appearance query under NO_COLOR" half is properly owned by `internal/tui/pane_appearance_test.go`, which pins it at `resolvePaneTheme` with an explicit `assertWroteNothing`.
- The three `internal/log` table rows are correct rather than a differently-named subtest; `subcommandPath` keeps flag *values* in the path, but they never occupy positions 0-1, so every composed chain argv resolves to `hydrate`.
