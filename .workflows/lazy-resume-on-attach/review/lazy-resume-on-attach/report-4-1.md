TASK: lazy-resume-on-attach-4-1 — The Panel-Drawing Process (`portal state resume-draw`)

ACCEPTANCE CRITERIA:
- `portal state resume-draw --command "<cmd>"` writes, in order and to its configured stdout: the alternate-screen entry, a cursor-home sequence, and exactly the bytes `tui.RenderResumePanel` returns for the same command, report, size, theme and colourless value.
- The rendered bytes are byte-identical to calling `tui.RenderResumePanel` directly with the same `ResumeScreen`, so the pane draws the production renderer and no second layout exists.
- The process execs `<os.Executable()> state resume-wait` carrying `--command`, `--hook-key`, `--pane`, `--pane-key`, the `--report` it was given and `--width`/`--height` set to the size it drew at; `--report` is absent from the argv when the report is empty.
- `resumeChainArgv` for `resume-recover` carries `--pane` and `--pane-key` and nothing else, for every payload — including one holding a command, a report and a size.
- A size read that fails, and one returning a zero or negative dimension, still paints — the value reaches the renderer unchanged and the renderer's bounded fallback applies; nothing panics and nothing writes an empty screen.
- Under `NO_COLOR` the theme resolver is called with `colourless` true, no appearance query is written, and the painted bytes carry no SGR background parameter.
- A prefs store that cannot be resolved, a `LoadThemeKeys` that errors, and a `themeResolution` that errors each paint from the shipped light/dark pair rather than failing the command.
- The theme read is `loadPrefsStoreNoMigrate` + `LoadThemeKeys` — no call reaches `loadPrefsStore`, so no draw dispatches the one-shot `appearance` translation or writes `prefs.json`.
- An `ExecSelf` that returns (the exec failed) leaves the command exiting non-zero after one WARN, with the alternate screen still entered and the panel still painted.
- The appearance probe is the only stdin read the draw performs, and it runs after any input drop and before the alternate-screen entry — so no byte it consumes can have arrived after a screen was painted.
- `log.ResolveProcessRole` answers `hydrate` for `state resume-draw`, `state resume-wait` and `state resume-recover`, and the closed role space gains no member.

STATUS: issues_found

SPEC CONTEXT: §4.2 — the process that draws must hand off (exec) to a fresh waiter before waiting, so the theme/render pages are not resident for the wait; every screen takes that same handover. §5.1/§5.2 — the panel is painted into the pane's alternate screen as a full-pane canvas with a centred card, degrading to a plain stack below the card size (a pane that drew nothing reads as restored with a dead keyboard); a named theme paints with no gate, a light/dark pair runs the detect-or-timeout probe in the drawing process; NO_COLOR paints no canvas and runs no detection. Corrigendum 2026-09-21 fixes the ordering as probe first, input drop after ("a drop taken first cannot drop the query's own reply") and names this task's "runs after any input drop" wording as reversed. §3.1 + Corrigendum 2026-09-19 — an unreadable prefs file resolves to the shipped default; the theme read takes the non-migrating route.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_chain.go:17-27 (flag constants), :46-60 (`resumeChainPayload`), :62-90 (`resumeChainArgv` — recover arm at :64-66 emits `--pane`/`--pane-key` only; report omitted when empty :69-71; size omitted when non-positive :77-82), :101-110 (`resumeChainExe`, empty path is an error), :117-127 (`resumeHandOff` — exec INFO is the statement immediately before `execSelf`)
  - cmd/state_hydrate.go:29 (`hydrateAltScreenEnter` declared beside `hydrateResetPreamble`, one owned pair)
  - cmd/state_resume_draw.go:18-31 (`resumeDrawConfig`), :37-73 (`runResumeDraw`: size → theme → alt-screen enter → cursor home → render → exec `resume-wait` carrying the drawn size; the leave sequence is never written), :86-89 (`paneDrawTheme` over `loadPrefsStoreNoMigrate`), :94-108 (`paneDrawNomination` — every failure degrades to `shippedPaneThemePair`), :128-130 (`term.GetSize` over stdin), :132 (`resumeDrawRunFunc` seam), :136-187 (hidden command; `--command` required; `--width`/`--height` registered but unread so the waiter's hand-back parses)
  - internal/log/process_role.go:27 (`resume-draw`/`resume-wait`/`resume-recover` → `roleHydrate`; no new role)
  - CLAUDE.md "Resume hooks" section carries the three-subcommand / hydrate-role / hydrate-component sentence
- Notes:
  - The code has moved past the task's text in ways later tasks required: `ResolveTheme` now takes a `dropInput` and returns the drop's error, and the payload gained `Screen`/`DropInput`. These are extensions and nothing this task asked for was lost.
  - The appearance-probe-vs-drop criterion is deliberately reversed from the task's wording: `tui.ResolvePaneTheme` (internal/tui/pane_appearance.go:69-75) runs the probe first and the drop after it. Specification Corrigendum 2026-09-21 names this task's wording as wrong and fixes probe-then-drop as the only order that clears a late OSC 11 reply. The criterion's actual guarantee still holds: both the probe and the drop run before the alternate-screen entry (cmd/state_resume_draw.go:49 before :60). The probe reads from a fresh `/dev/tty` open; the size read is an ioctl and the drop is a tcflush, so the probe is the only read. This change is sound and is not a finding.
  - On exec failure, the production `defaultExecShell` (cmd/state_hydrate.go:388-393) emits the WARN and then calls `log.Close(1)` and `osExit(1)`. A `resumeChainExe` failure returns an error after the paint, so the command exits non-zero and the parked chain's tail recovers the pane.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/state_resume_draw_test.go covers every named test: the alternate-screen entry before the panel, with no leave written; byte-identity with `RenderResumePanel` with and without a report; the waiter argv including the drawn size; `--report` omitted when empty; the size fallback table (error, 0x0, negative) with a non-empty paint; NO_COLOR (`colourless` true, no `48;2;`); the shipped pair when prefs fail (path, read and resolution errors); an `appearance` prefs file byte-unchanged with no translation dispatched; exec failure giving exactly one `osExit(1)` and one WARN with the alternate screen entered. The flag parse is exercised by driving `rootCmd` with the composed argv.
  - The file also has "it resolves the theme before it paints" (`paintedAtResolve == 0`), which pins the probe-before-paint half of the stdin-read criterion.
  - cmd/state_resume_chain_test.go:52-60 covers the recover argv for both a full payload and a minimal one.
  - internal/log/process_role_test.go:16-18 covers the three hydrate rows.
  - internal/tui/pane_appearance_test.go:216-245 and :275-299 cover the "no query written under NO_COLOR" half and the probe-then-drop order.
  - The migrating-route criterion is also held structurally: `TestLoadPrefsStore_SingleProductionCaller` (cmd/prefs_translation_test.go:289-307) fails if anything but `openTUI` calls `loadPrefsStore`.
- Notes: The production wiring of the NO_COLOR carve-out is not observed by any test (see FINDINGS). The tests are not bloated: there is a small overlap between `TestResumeHandOff`'s empty-path subtest and `TestResumeChainExeResolutionFailure`, and nothing worth acting on.

CODE QUALITY:
- Project conventions: Followed. The seam is staged through `withFuncSeam`, there is no `t.Parallel`, the command is hidden and registered on `stateCmd`, it emits under the existing `hydrate` logger, and the silent loader is used for the fallback seed.
- SOLID principles: Good. Chain-shared argv and hand-off live in `state_resume_chain.go`, and the draw-specific resolution lives in `state_resume_draw.go`.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The comments hold against the code. The only stale wording is in the task text, not the code.
- Issues: None beyond FINDINGS.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/state_resume_draw_test.go:186 — The NO_COLOR test sets `cfg.Colourless = noColorEnabled()` itself, repeating the production wiring instead of exercising it. `TestStateResumeDrawCommand` (cmd/state_resume_draw_test.go:351-385) already captures the config the real command builds, but it compares only the payload (:382), so the `Colourless: noColorEnabled()` line at cmd/state_resume_draw.go:162 is observed by nothing. Fix: add a subtest to `TestStateResumeDrawCommand`. It should `t.Setenv("NO_COLOR", "1")`, execute the composed argv through the `resumeDrawRunFunc` capture, and assert `got.Colourless` is true (and false with NO_COLOR empty). — FAILS: if that wiring line is dropped or mis-set, the code compiles and the whole suite still passes. Under NO_COLOR every waiting pane would then write an OSC 11 query into its tty and paint a coloured canvas, breaking the carve-out this criterion names.

UNSETTLED:
- None
