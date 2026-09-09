TASK: resume-hooks-silently-lost-2-4 — A Registered Hook Asks For A Capture (`hook set` touches the `save.requested` dirty flag after a successful write, best-effort, one WARN under its own `op`, exit 0 regardless)

ACCEPTANCE CRITERIA:
- A successful `hook set` creates `save.requested` in the resolved state directory
- With the state directory unresolvable or uncreatable, `hook set` exits 0 with the entry written and emits exactly one WARN under `op=touch-save-requested`
- With the state directory present but the touch failing, the outcome is identical — one WARN, exit 0, entry written
- The WARN is under the `hooks` component and carries `op`, `hook_key`, `via=cli` and `error`; no `op=set` WARN is emitted alongside it
- Exactly one WARN is emitted per failing `hook set`, never two
- The touch runs only after the write returns without error: a failed `hook set` touches nothing
- `hook rm` touches nothing on either of its paths
- `hook` starts no tmux server on this path — bootstrap-exemption is unchanged
- CLAUDE.md's Resume-hook-command bullet no longer claims `hook` is config-file only

STATUS: complete

SPEC CONTEXT:
§2.2 ("Stamping is lazy, at `hook set`") requires that after a successful write `hook set` resolve the state directory with `state.EnsureDir()` and call `state.TouchSaveRequested(dir)`, so the next daemon tick captures the newly stamped token rather than waiting up to `MaxGap` (30s) for the gap branch. The touch is explicitly best-effort and never affects the exit status: by the time it runs the entry is durably written, so failing the command would report a loss that did not happen. A failure at either step logs one WARN under the `hooks` component with message and `op` both `touch-save-requested`, plus `hook_key`, `via=cli` and `error`, filed under its own `op` rather than `set`. §6.5 lists `touch-save-requested` as one of three spec-governed additions to the closed `hooks` `op` vocabulary, adding no attr key. §9.2's requirement table pins the unresolvable-directory case as a unit test. The Corrigenda section holds nothing that touches this task.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/hooks.go:222` — `touchSaveRequestedForHook(hookKey)` is the last statement of `hooksSetCmd`'s `RunE`, reached only after `store.Set` returns nil (`cmd/hooks.go:218-220` returns on error), and `RunE` returns nil unconditionally after it.
  - `cmd/hooks.go:235-244` — the helper: `state.EnsureDir()` then, only on success, `state.TouchSaveRequested(dir)`, with the shared `err` variable funnelling both failure modes into one `hooksLogger.Warn` call. Message `touch-save-requested`, `op` `touch-save-requested`, `hook_key` the token just written, `via` `hooks.ViaCLI.String()` (= `"cli"`, `internal/hooks/via.go:23`), `error` the failure. No value is returned, so no caller can turn it into an exit status.
  - `cmd/state_common.go:11` — the emission uses the pre-existing `hooksLogger = log.For("hooks")` binding; no new component or attr key is introduced.
  - `CLAUDE.md:45` — the bullet now reads "Bootstrap-exempt: it starts no tmux server. Its one write outside the config directory holding `hooks.json` is `hook set`'s best-effort touch of `save.requested` in the state directory…". The verb, the permanent `hooks` alias, the pointer to "Resume hooks" and the `hook list` location-column sentence are all intact; no other CLAUDE.md passage was touched by this commit (`da05b809`, CLAUDE.md +1/-1).
- Notes:
  - The stated outcome is real, not nominal: `cmd/state_daemon.go:183-185` gates the tick on `dirty := fileExists(state.SaveRequested(deps.Dir))`, so a touched flag drives a full `captureAndCommit` on the next 1s tick instead of the `MaxGap` gap branch (`cmd/state_daemon.go:425` sets `MaxGap: 30 * time.Second`). Both sides resolve the directory through `state.Dir()` → `xdg.ConfigDirPath(xdg.OSEnv, xdg.StateDir)` (`internal/state/paths.go:26-28`), so writer and reader cannot disagree on the path.
  - Bootstrap-exemption is unaffected: `hook` remains in `skipTmuxCheck` (`cmd/root.go:28`), and both `state.EnsureDir` (`internal/state/paths.go:33-49`) and `state.TouchSaveRequested` (`internal/state/paths.go:58-70`) are pure filesystem calls with no tmux contact.
  - `touchSaveRequestedForHook` has exactly one call site in the repository (grep over `cmd/`: `cmd/hooks.go:222` plus its own declaration and doc comment), so `hooksRmCmd` is structurally incapable of touching the flag on either the `$TMUX_PANE` or the `--pane-key` path.
  - No drift from the plan: `state.EnsureDir` does create the state directory and its `scrollback` subdirectory when absent, which is exactly the resolution `portal state notify` performs and what the task prescribed.

TESTS:
- Status: Adequate
- Coverage: `cmd/hooks_test.go:756-910` (`TestHooksSetTouchesSaveRequested`) carries all eight subtests the task named, each mapping to a criterion:
  - `:757` happy path — `PORTAL_STATE_DIR` at a temp dir, asserts `save.requested` exists afterwards.
  - `:771` unresolvable directory — `PORTAL_STATE_DIR` under a regular file so `MkdirAll` fails with ENOTDIR; asserts exit 0, the entry present in `hooks.json`, and the WARN.
  - `:793` failing touch — state dir and `scrollback` created, then `chmod 0500`, so `EnsureDir` succeeds and `OpenFile` fails; asserts exit 0, the entry present, and the WARN. This is a genuinely distinct branch from `:771`, not a duplicate fixture.
  - `:819` no `set` WARN; `:836` exactly one WARN; `:851` no touch on a failed write (a directory at the `hooks.json` path), asserting both the absent file and the absent record; `:877` no-op re-registration still touches (removes the flag between the two runs, so the second run's touch is what is observed); `:898` `hook rm` touches nothing.
  - `assertTouchWarn` (`cmd/hooks_test.go:736-751`) is the shared assertion and is strict where it matters: `sink.Records().AtOrAboveLevel(slog.LevelWarn).Only(t, …)` pins the count at one, `assertHooksRecord` → `logtest.AssertRecord` (`internal/logtest/assert.go:24-41`) pins the **exact** level, message, `component=hooks`, `op` and `via`, and the helper then pins `hook_key` to the expected token and a non-empty `error`. Level, op and hook_key are therefore all mutation-sensitive.
  - Bootstrap-exemption is covered outside this file: `cmd/root_test.go:303-362` drives `hook set` (and the `hooks` alias form) through `PersistentPreRunE` with a recording orchestrator, asserting zero orchestrator runs and a nil error — and it runs under the package-wide `PORTAL_STATE_DIR` poison (`cmd/testmain_isolation_test.go:66`, `/nonexistent/portal-test-must-isolate-state`), so it doubles as proof that the failing-touch branch still exits 0 for every pre-existing `hook set` test.
- Notes:
  - `:819` and `:836` are strictly subsumed by `assertTouchWarn` as used at `:771` and `:793` (which already prove "exactly one record at or above WARN, and it is the touch record"). They were named explicitly in the task's Tests list and each documents a criterion by name, so they are plan-mandated rather than accidental bloat — not something to remove.
  - `:819`'s name ("only the dirty-flag touch fails") describes the composite two-step operation rather than the inner `TouchSaveRequested` call, since its fixture fails at `EnsureDir`. The assertion it makes (no `op=set` WARN) is correct for either branch, so the name is loose but not false.
  - The `hook rm` subtest exercises only the `$TMUX_PANE` path, not `--pane-key`. Since `TouchSaveRequested` has exactly one call site and it is inside `hooksSetCmd`, the second path is structurally covered.

CODE QUALITY:
- Project conventions: Followed. One emission through the existing `hooks` component binding (`cmd/state_common.go:11`), no new component and no new attr key; `via` is rendered through `hooks.Via.String()` exactly as every other `hooks` emission does (`internal/hooks/store.go:122,144,178,207`; `internal/hooksweep/standdown.go:15`). Hyphenated `op` value matches the component's existing vocabulary (`load-unlocked`, `set-noop`, `clean-stale-skipped`). Unit-lane test, no tmux server, no daemon, no built binary — correct lane. The test drives the command through the injected `HooksDeps` seam via `withHooksDeps` rather than assigning the package var directly, so `cmd/seam_guard_test.go` stays satisfied.
- SOLID principles: Good. The helper does one thing and returns nothing, which is what makes "never affects the exit status" a property of the signature rather than of caller discipline.
- Complexity: Low — nine lines, one branch, one emission site.
- Modern idioms: Yes. The `if err == nil { err = … }` chaining is the plainest way to reach "both failure modes share the one emission" without a second `if` or a sentinel.
- Readability: Good. The doc comment states the conclusion (best-effort, its own op, no exit-status effect) and the reason, and every claim in it holds against the code and against the daemon gate it names.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
