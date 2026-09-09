TASK: resume-hooks-silently-lost-7-1 — A Colon-Bearing Session Name Is Now Silently Dropped From sessions.json (tick-932552)

ACCEPTANCE CRITERIA:
- `Client.RenameSession` returns an error naming `:` and issues no `rename-session` argv when `newName` contains one.
- The TUI rename path refuses the same name and surfaces the reason to the user rather than renaming or failing silently.
- A `ShowEnvironment` failure against a colon-bearing session name does not satisfy `errors.Is(err, tmuxerr.ErrNoSuchSession)`.
- `CaptureStructure` routes that session to `anomalousErrs` and its WARN does not describe a vanished session.
- A live colon-named session is present in the `Index` a real-tmux capture returns, or the capture reports the error — it is never absent from a successful capture.
- Names with no colon rename and capture exactly as before, on both the client and TUI paths.

STATUS: complete

SPEC CONTEXT:
This is a phase-7 implementation-analysis task, so its authority is its own body rather than the specification (the spec's Corrigenda carry nothing on this subject). Its premise is a regression the work unit itself introduced: pinning every per-session `-t` to the exact-match form (`=name:`) made a live session literally named `a:b` unresolvable, and tmux answers that target with the same "no such session" stderr a vanished session produces. `internal/state/capture.go` read that stderr as natural churn and dropped the session from `sessions.json` with a WARN calling it vanished — silent loss, which is the exact class this work unit exists to remove. The task's remedy is two-ended: refuse the name at Portal's write boundary, and stop the capture loop from counting an unaddressable name as churn.

IMPLEMENTATION:
- Status: Implemented (and legitimately extended by later tasks 8-3 and 9-4, which added the `$`-prefix and `-`-prefix rules to the same validator; the colon behaviour this task delivered is intact underneath).
- Location:
  - `internal/tmuxerr/errors.go:16` — `ErrUnaddressableSessionName`, the leaf sentinel `internal/state` can classify against with no import cycle.
  - `internal/tmux/errors.go:62` re-exports it identity-equal; `internal/tmux/errors.go:99-110` is `ValidateSessionName` (separator first, then ID prefix, then flag prefix); `internal/tmux/errors.go:118-126` is `wrapSessionTargetErr`, which checks the name **before** `wrapNoSuchSession` so an unaddressable name never reaches the churn sentinel.
  - `internal/tmux/tmux.go:285-287` — `RenameSession` validates `newName` before composing the argv and returns without calling `c.cmd.Run`.
  - `internal/tmux/tmux.go:616` — `ShowEnvironment` classifies through `wrapSessionTargetErr`.
  - `internal/tui/model.go:2667-2672` — the `r` modal validates on Enter, closes the modal, raises a warning flash and schedules its auto-clear; the renamer is never reached.
  - `internal/tui/sessions_flash.go:58-74` — the three refusal strings and `renameRefusalFlash`, which selects wording from the rule sentinel.
  - `internal/state/capture.go:68-79` — untouched, as the task's Do list required: the natural-churn branch still keys on `errors.Is(err, tmuxerr.ErrNoSuchSession)` (line 71) and the new sentinel falls through to `anomalousErrs` (line 76) with the "capture anomalous session error" WARN.
- Notes:
  - Every acceptance criterion holds in the current tree. The fifth ("present in the Index, or the capture reports the error") is met in the shape the Do list prescribed: the session is reported through `anomalousErrs` + the anomalous WARN, and escalates to a returned error only when every session failed. That is the intended design, not a shortfall — and it is strictly better than before, where a single colon-named session could drive an empty-index commit that wiped saved state.
  - The classification is fail-closed in the right direction: `wrapSessionTargetErr` only ever runs on an already-failed call, so a name tmux *can* resolve is never reclassified, and a `-`-leading or `$`-leading session that answers normally still captures.
  - `internal/tui/model.go:2691` is the only production caller of `RenameSession`, so the two write boundaries the task names are the complete set.
  - The user-visible copy is mirrored byte-for-byte in `README.md:197` and the behaviour is described in `CLAUDE.md`'s `tmux` architecture row; `testdata/vhs/sessions-rename-refused-separator.png` shows the delivered band (⚠ glyph prepended by the band, modal closed, list intact) matching `internal/tui/sessions_flash.go:59`.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/tmux/session_name_test.go:15-68` covers all four client-side named tests — refusal, the named character, zero argv issued (the `commandertest.New(t)` fake is loud on any unmatched argv, so a leaked call fails twice over), and the colon-free rename asserted down to the exact argv `rename-session -t =old-name: new-name`.
  - `internal/tmux/session_name_test.go:83-115` pins both directions of the classification: a colon-bearing name must NOT satisfy `errors.Is(ErrNoSuchSession)` and MUST satisfy `errors.Is(ErrUnaddressableSessionName)`; a vanished `gone` must satisfy the former and not the latter. Both drive the *same* fake stderr, which is what makes the discrimination meaningful rather than incidental.
  - `internal/state/capture_colon_session_test.go:19-55` pins the capture-loop routing (only `plain` survives, the anomalous WARN is present, the word "vanished" is absent from the log body).
  - `internal/state/capture_colon_session_realtmux_test.go:18-63` is the real-tmux regression the task's Do item 4 asked for: an isolated `-S` socket with a live `a:b` beside a live `plain`, `CaptureStructure` run against it, `plain` asserted as a precondition so a capture that surfaced nothing cannot pass.
  - `internal/tui/rename_colon_refusal_test.go:11-36, 95-112` covers the TUI refusal, the flash text, the absence of a duplicated glyph, and the unchanged colon-free rename (asserted by driving the returned `tea.Cmd` and reading the recorded renamer call).
- Notes:
  - Each test would fail if its subject broke: drop the `ValidateSessionName` call from `RenameSession`, from `updateRenameModal`, or revert `ShowEnvironment` to `wrapNoSuchSession`, and a named test fails in each case.
  - Not over-tested. The unit and real-tmux capture tests are not duplicates — one pins Portal's classification given tmux's stderr, the other pins that real tmux actually produces it.
  - The real-tmux test's early `return` when the session *is* captured is a deliberate, commented escape hatch for a tmux that can address the name; it does not weaken the test on the tmux this project targets, where the anomalous branch is the one exercised.
  - Lane placement is sound: no portal binary is built, no daemon is spawned, and the socket is a per-test `-S` path with a cleanup that kills the server — so the unit lane is correct, and `internal/spawn/ack_realtmux_test.go` (pre-dating this work unit) already set that precedent outside `internal/tmux`.

CODE QUALITY:
- Project conventions: Followed. The sentinel lives in the `tmuxerr` leaf exactly as the architecture table prescribes, so `internal/state` classifies with no import cycle; no layer above `internal/tmux` substring-matches tmux stderr; the flash is raised through `setFlash` with the post-bump generation handed to `flashTickCmd`, matching the established `(&m).setFlash(...)` pattern at `internal/tui/model.go:2500`; comments carry no task ids or spec-section references.
- SOLID principles: Good. `ValidateSessionName` is one rule with one home, consumed by the write boundary, the read classifier and the TUI alike; `wrapSessionTargetErr` is the single classification chokepoint.
- Complexity: Low.
- Modern idioms: Yes. Multi-`%w` wrapping keeps both the sentinel and the original `*CommandError` reachable on one value, which is what lets `errors.Is` discriminate at two layers without re-parsing text.
- Readability: Good. The ordering constraint that makes the whole fix work — name check strictly before stderr classification — is stated at `internal/tmux/errors.go:112-117` where a future edit would break it.
- Issues: One stale doc comment, below.

BLOCKING ISSUES:
- None.

FINDINGS:
- [in-scope] [contained] internal/tmuxerr/errors.go:10 — the sentinel's doc comment says it is wrapped by operations "whose target Portal's exact-match form cannot express: tmux reserves ':' as a target separator", but `ValidateSessionName` (`internal/tmux/errors.go:99-110`) now wraps it for three rules, and the flag-prefix rule is not an exact-target failure at all — the same file's `flagPrefix` comment (`internal/tmux/errors.go:76-79`) states that a `-`-leading name is refused because a rename passes the new name as a bare positional, and `ValidateSessionName`'s own comment (`internal/tmux/errors.go:93-98`) states the two-part rule correctly. Widen both the leaf comment and its re-export comment (`internal/tmux/errors.go:58-61`, which repeats the same single-rule claim) to say the sentinel covers any name tmux cannot be handed back — as an exact target or as the bare positional a rename carries — and point at `ValidateSessionName` as the enumeration of the rules rather than naming one. — FAILS: a maintainer discriminating on `ErrUnaddressableSessionName`, or deciding whether a new refusal rule belongs under it, is told by the sentinel's own declaration that it means a colon in the name; three rules wrap it today and one of them contradicts the stated reason, so the leaf's account of when the sentinel is returned is false as it stands.
