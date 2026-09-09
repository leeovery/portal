TASK: resume-hooks-silently-lost-8-6 (tick-a4589d) — "An Unspecified Session-Name Refusal With New User-Facing Copy Shipped Inside This Work Unit, Documented Nowhere". Documentation + visual-gate task: the refusal stays, and is described where the project describes behaviour (README, CLAUDE.md's `tmux` row) with the rename-refusal flash copy put through a capture gate.

ACCEPTANCE CRITERIA:
- README's rename section names the refusal and exactly what it refuses.
- CLAUDE.md's `tmux` row names the refusal and the four exact-target helpers.
- The rename-refusal flash has been rendered and reviewed at a gate, with the fixture command recorded.
- README's wording and the flash string agree; no user-facing string ships un-gated.

STATUS: complete

SPEC CONTEXT: The specification does not govern this task. It scopes the positional/structural addressing siblings as checked against the change rather than changed by it, and mentions no session-name refusal (`grep -n "ValidateSessionName\|unaddressable" specification.md` returns nothing on either term) — which is precisely the gap the task was raised for. Per the shared verifier context, a phase 6-10 task's authority is its own body, so this task was judged against what it says it does. The surrounding spec context that does bear on it: §7.1 records that a token-only hook key makes renames irrelevant by construction, and the retained rename-durability suites (`cmd/rename_restore_cleanup_survival_integration_test.go`, `internal/restore/rename_reboot_hook_integration_test.go`) keep the user-visible "a rename cannot orphan a hook" guarantee under test — which is the promise README:195 makes and which README:197 now qualifies.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `README.md:197` — the refusal paragraph, in the `hook` section directly under the "hooks survive a rename" promise at `:195`. It now names three refusals (the third arrived with task 9-4's flag-prefix rule), each quoted verbatim, plus the outcome ("leaves the session under its old name; nothing is renamed, and a hook already registered against the pane is untouched").
  - `CLAUDE.md:60` (the `tmux` row) — records `ValidateSessionName` / `ErrUnaddressableSessionName` + the three rule sentinels, `RenameSession`'s pre-argv refusal, the picker's matching check with the wording's home named (`internal/tui/sessions_flash.go`, "wording mirrored in README's `hook` section"), and all four exact-target helpers. The helper names have since been rewritten by a later task (`CoordTargetExact` / `SessionTargetExact` rather than the `ExactCoordTarget` / `ExactSessionTarget` this task's Do list named) and the row tracks the code as it stands.
  - `internal/capture/fixtures.go:339` / `:348` — `sessionsRenameRefusedSeparatorFixture` / `sessionsRenameRefusedIDPrefixFixture`, registered in `fixtureBuilders()` at `:141-142`. Each derives from a freshly-built `sessionsFlatFixture()` and declares `captureKeys` that type into the real rename modal, so the band carries production's own string rather than a seeded copy.
  - `internal/capture/harness.go:21` / `:26` — `keyEnter` / `keyLineStart` (`ctrl+a`, the textinput line-start binding) added for those sequences.
  - `testdata/vhs/sessions-rename-refused-separator.{tape,png}` and `testdata/vhs/sessions-rename-refused-id-prefix.{tape,png}` — the gate artefacts; each tape records the live command (`go run ./cmd/capturetool --fixture sessions-rename-refused-separator` / `… --fixture sessions-rename-refused-id-prefix`) and a "WHAT TO LOOK AT" note.
- Notes:
  - Documentation claims verified against the code rather than taken on trust: `internal/tui/model.go:2667` runs `tmux.ValidateSessionName` before the renamer is reached and closes the modal so the band is readable; `internal/tmux/tmux.go:285` runs it before composing the `rename-session` argv; `internal/tmux/errors.go:99-108` implements exactly the three rules the docs list; `SessionTargetExact` has one occurrence in non-test sources — its own declaration at `internal/tmux/tmux.go:448` — which is what CLAUDE.md's "taken by no production call site" asserts.
  - Both captures were opened and judged as part of this review. The separator frame shows the single-line band (`▌` bar, `⚠`, copy) under the title separator with the modal gone and the session still under its old name; the id-prefix frame shows the same band wrapping to a second, indented line at the tape's ~93-column geometry. At the harness's 120 columns (`internal/capture/swap_harness_test.go:19`) neither wraps, which is why the frame assertions can match the whole string.
  - The `capturetool` binary replays no keys of its own (`cmd/capturetool/main.go:174-190` builds the model and runs it), which is why the tapes type the sequence the fixture declares — tape and offline driver agree rather than drift.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/capture/capture_test.go:919` `TestSessionsRenameRefusedFixtures` — for each fixture asserts the seeded flash is empty (the refusal must be typed, not seeded) and that the rendered frame contains the production string. This is the assertion that would fail if the key sequence stopped reaching the refusal — notably if `ctrl+a` stopped being line-start, since a trailing `$` is a legal name and no band would render.
  - `internal/capture/capture_test.go:953` `TestFixtureNamesIncludesRenameRefusals` — the task's named "it lists the rename-refusal fixture" test.
  - `internal/capture/swap_harness_test.go:44-45` — the two new `capturedStates` entries, so the swap-and-diff completeness guard covers the new frames rather than silently skipping them.
  - The behavioural half is where the task says it is and was left alone: `internal/tui/rename_colon_refusal_test.go` pins that a refused rename reaches no renamer, closes the modal, sets the matching flash and that the copy names the offending character and embeds no `⚠` glyph; `internal/tmux/session_name_test.go` covers the validator.
- Notes: `TestFixtureNamesIncludesRenameRefusals` is close to implied by `FixtureByName` resolving in the test above it (both derive from the one `fixtureBuilders()` list), but it is the test the task named and it mirrors the existing `sessions-inline-flash` pattern at `capture_test.go:413`. Not over-tested on any reading that matters.

CODE QUALITY:
- Project conventions: Followed. The fixtures are registered in the single `fixtureBuilders()` list (no second registry), the tapes carry the `Set FontFamily`/`FontSize`/`Set Shell "bash"` shape and write their gif to `.gifcache/`, quoted-path gotcha respected, and both tapes state the scaffolding/cleared-at-sign-off rule that `testdata/vhs/README.md` sets out. The Go fixture definitions are permanent, as required. No production code was touched, so no lane, isolation or logging rule is engaged.
- SOLID principles: Good — the fixtures compose the existing flat fixture and change only name and key script.
- Complexity: Low.
- Modern idioms: Yes — table-driven subtests, `slices.Contains`.
- Readability: Good.
- Issues: None. Comment accuracy checked at each site: "the modal opens with the cursor at the end of the current name" holds (`internal/tui/model.go:2649` sets the value through `textinput.SetValue`, which lands the cursor at the end), "the refusal is typed, never seeded" holds and is itself asserted, and `keyLineStart`'s claim to be "the text input's own line-start binding" is borne out by the captured frame, which could not show the `$` refusal if the key had appended instead.

BLOCKING ISSUES:
- None.

FINDINGS:
- [out-of-scope] [contained] internal/capture/fixtures.go:348 — the third refusal string, `renameFlagPrefixRefusedFlash` (`internal/tui/sessions_flash.go:61`), has no capture fixture: `fixtureBuilders()` registers the separator and id-prefix frames only, and `grep -rn "rename-refused" testdata/vhs` lists two tapes and two PNGs. Adding `sessionsRenameRefusedFlagPrefixFixture` (a copy of the id-prefix builder with `keyRune('-')` and a `sessions-rename-refused-flag-prefix` name), a `capturedStates` entry beside `swap_harness_test.go:45` and a third case in `TestSessionsRenameRefusedFixtures` closes it. FAILS: this task's fourth criterion — "no user-facing string ships un-gated" — is untrue at the delivered state for the hyphen refusal, which reaches users unrendered and unlooked-at; its sibling `$` string, of near-identical length, wraps to two lines at the tape geometry, so band framing at that copy length is a real question nobody has answered for the third string. The string was introduced by task 9-4 (`21749af5`), not by this task's change-set, so the gap belongs to that task's delivery — recorded here because this is the criterion it contradicts, and it is the user's to take or leave.
