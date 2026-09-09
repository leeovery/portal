TASK: resume-hooks-silently-lost-9-4 — "ValidateSessionName Accepts A Leading Hyphen, Which tmux Refuses As A Rename Target" (tick-466828)

ACCEPTANCE CRITERIA:
1. `ValidateSessionName("-bar")` returns an error satisfying `errors.Is(err, tmux.ErrUnaddressableSessionName)` and `errors.Is(err, tmux.ErrSessionNameFlagPrefix)`, and names the hyphen in its message.
2. A hyphen appearing anywhere but the first byte is still accepted — `my-cool-app-abc123` validates.
3. `RenameSession` refuses a `-`-leading new name before composing its argv, so tmux never sees it and no `unknown flag` error can surface.
4. The picker's `r` modal reports the refusal in its notice band naming the hyphen, distinct from the `:` and `$` wordings.
5. `SanitiseProjectName("$work")` returns `work`, and `GenerateSessionName("", …)` returns a name that is the nanoid alone with no leading hyphen.
6. No production path can produce a session name beginning with `-`.

STATUS: complete

SPEC CONTEXT:
Phase 9 is an implementation-analysis cycle; the specification (`.workflows/resume-hooks-silently-lost/specification/…/specification.md`) carries no session-name-validation material (grepped for `hyphen`, `ValidateSessionName`, `unaddressable`, `session ID prefix` — no hits), so the task body is the authority, as the shared verifier context directs. The surrounding project rule the change extends is CLAUDE.md's `tmux` architecture row: `ValidateSessionName` refuses a name tmux cannot be handed back, wrapping `ErrUnaddressableSessionName` plus a per-rule sentinel, and `RenameSession` runs it before composing its argv. This task adds a third rule on that machinery and closes the generator side so Portal stops minting names of the refused shape.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/errors.go:76-79` — `flagPrefix = "-"` const beside `targetSeparator` (`:68`) and `sessionIDPrefix` (`:74`), with the "rename-session answers one with unknown flag" rationale.
  - `internal/tmux/errors.go:87-91` — `ErrSessionNameFlagPrefix` in the same `var` block as its two siblings, carrying its own clause (`begins with "-", which tmux reads as a command flag`).
  - `internal/tmux/errors.go:106-108` — the third `strings.HasPrefix` rule in `ValidateSessionName`, wrapping `ErrUnaddressableSessionName` + the new sentinel and quoting the name, in the same shape as the two existing arms.
  - `internal/tmux/tmux.go:285-287` — `RenameSession` validates `newName` before `c.cmd.Run` at `:288`; the pre-existing early return now covers the hyphen rule for free.
  - `internal/tui/sessions_flash.go:61` — `renameFlagPrefixRefusedFlash`, and `:70-72` — the third `errors.Is` arm in `renameRefusalFlash`.
  - `internal/tui/model.go:2667-2672` — the picker's `r` modal already routes every refusal through `ValidateSessionName` → `renameRefusalFlash`, so the new wording reaches the notice band with no call-site change.
  - `internal/session/naming.go:25` — `unwritableLeadingChars = "$-"`, and `:31-35` — `SanitiseProjectName` now `strings.TrimLeft`s that set after the `.`/`:` replacement, so no fragment can lead with either character.
  - `internal/session/naming.go:49-54` — `GenerateSessionName` composes the candidate as the suffix alone when the fragment sanitises to empty.
  - `README.md:197` — the wording set now names three refusals with the flash constants' exact copy; `CLAUDE.md`'s `tmux` row records the third rule and the third sentinel.
- Notes:
  - **Deliberate, sound divergence from the Do list.** The task prescribed `strings.TrimPrefix` for the leading `$`; the implementation uses `strings.TrimLeft(…, "$-")`. That is strictly better and is what makes criterion 6 hold: `TrimPrefix("$")` alone would leave `$-work` → `-work` and (since `.` and `:` are replaced with `-` first) `.dotfiles` → `-dotfiles`, both of which are exactly the shape the task set out to eliminate. `internal/session/naming_test.go:40-44` pins the `.dotfiles` case. No loss — nothing the task asked for in substance is missing.
  - The `$`/`-` character knowledge is now stated in two packages (`internal/tmux`'s unexported `sessionIDPrefix`/`flagPrefix` and `internal/session`'s `unwritableLeadingChars`). This is the codebase's existing arrangement — the tmux constants are unexported, and CLAUDE.md declares generation is "pinned to the recogniser on the other side" — and the pinning is enforced by a test (`internal/session/naming_test.go:219-242`) rather than by prose, so drift fails a build.
  - `ValidateSessionName` is also reached by `wrapSessionTargetErr` (`internal/tmux/errors.go:118-126`), so a failed per-session operation on a `-`-leading session name now classifies as unaddressable rather than as `ErrNoSuchSession`. Unlike `:` and `$`, a `-`-leading name *is* addressable through the `-t` optarg, so this widens that classifier's reach. The consequence is conservative and unreachable in practice: such a name can only be created outside Portal, the picker now refuses to mint or rename to one, and the only observable effect (`internal/state/capture.go:71-78`) is that a genuinely-vanished session of that shape is logged as anomalous instead of natural churn — which, in the sole-session case, aborts the tick and *retains* prior state rather than committing an empty index. No defect to name; recorded here rather than as a finding.

TESTS:
- Status: Adequate
- Coverage: All six test names the task specifies exist verbatim and each observes the behaviour it names:
  - `internal/tmux/session_name_test.go:233` "it refuses a session name beginning with a hyphen" — asserts both `errors.Is` arms (criterion 1) and that the message names `"-"`.
  - `internal/tmux/session_name_test.go:250` "it accepts a hyphen that is not the first character" — `my-cool-app-abc123` (criterion 2).
  - `internal/tmux/session_name_test.go:256` "it refuses the rename before composing the tmux argv" — asserts `len(mock.Calls()) == 0` on a `commandertest.New(t)` fake, which is loud on unmatched argv, so a regression that reached tmux fails twice over (criterion 3).
  - `internal/tui/rename_colon_refusal_test.go:65` "it reports the hyphen refusal with the flag-prefix wording in the rename band" — drives the real `updateRenameModal` with `-bar`, asserting no renamer call, `modalNone`, and the exact flash constant; `:83-93` adds the character-naming, glyph-absence and *distinctness from both siblings* assertions (criterion 4).
  - `internal/session/naming_test.go:36` "drops a leading dollar from a project fragment rather than substituting a hyphen" (criterion 5, first half).
  - `internal/session/naming_test.go:163` "it generates the nanoid alone when the sanitised project fragment is empty" (criterion 5, second half).
  - Criterion 6 is carried by `internal/session/naming_test.go:219-242`, whose hostile-input table now includes `"-lead"`, `""` and `".dotfiles"` and validates every generated name through `tmux.ValidateSessionName` — the coupling that makes generation and recognition unable to drift apart. `internal/session/prepare.go:37` is the sole production caller of `GenerateSessionName` (grepped tree-wide, non-test), and `QuickStart` reaches it through `PrepareSession`, so that table covers the whole minting surface.
- Notes: Not over-tested. The additions are three subtests, two table rows and two hostile-input entries; each asserts one property, and none restates a sibling's. Each would fail if the corresponding production line were reverted: dropping the `ValidateSessionName` arm breaks three, reverting `SanitiseProjectName` breaks two plus the addressability guard, dropping the flash arm breaks one. All new tests are unit-lane (no tmux server, no binary build, no daemon), which is correct — nothing here touches the integration lane's triggers.

CODE QUALITY:
- Project conventions: Followed. The new const, sentinel and validator arm each sit beside their two existing siblings in the same block and same shape; the flash constant and its `errors.Is` arm mirror the ID-prefix pair exactly; the README/CLAUDE.md pair was moved together, as the `tmux` architecture row requires of this user-visible copy. Test naming follows the `it …` subtest convention, no `t.Parallel()`, and the tmux fake is `commandertest`, per the single-declaration rule.
- SOLID principles: Good. The rule is added at the one place that owns it (`ValidateSessionName`) and every consumer — `RenameSession`, `wrapSessionTargetErr`, the picker modal — inherits it without change; the sentinel-per-rule shape keeps wording selection in the presentation layer.
- Complexity: Low. Three straight-line `if` arms; the sanitiser stays a two-call expression.
- Modern idioms: Yes. `strings.TrimLeft` over a cutset const rather than chained `TrimPrefix` calls; `for range maxRetries` in the generator loop.
- Readability: Good. Every new const and sentinel carries the tmux behaviour that motivates it, and `internal/session/naming.go:49-51` states why an empty fragment contributes no separator.
- Issues: None. Comment accuracy checked line by line against the code: the `flagPrefix` rationale (`errors.go:76-79`), the `unwritableLeadingChars` description of what is dropped and why (`naming.go:19-25`), `SanitiseProjectName`'s docstring including "The result may be empty" (`naming.go:27-30`), `GenerateSessionName`'s "the nanoid alone when the project name sanitises to nothing" (`naming.go:37-39`), and the amended `sessions_flash.go:56` "None carries a ⚠ glyph" all hold against the code beneath them. No process-artifact references (no task ids, phases or spec sections) in any changed comment.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
