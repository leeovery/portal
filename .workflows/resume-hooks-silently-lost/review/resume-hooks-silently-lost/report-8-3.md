TASK: resume-hooks-silently-lost-8-3 — "Two Disagreeing Definitions Of An Unwritable Session Name, And A $-Leading Name Is Still Silently Lost" (tick-c41253)

ACCEPTANCE CRITERIA:
- `ValidateSessionName("$foo")` returns an error wrapping `ErrUnaddressableSessionName`; `ValidateSessionName("a$b")` returns nil.
- `ValidateSessionName` still accepts a name containing `.`, and `SanitiseProjectName` still replaces it.
- No project directory name makes `GenerateSessionName` produce a name `ValidateSessionName` refuses.
- The capture loop's discriminator reports a live `$`-leading session as unaddressable rather than vanished.

STATUS: complete

SPEC CONTEXT:
The specification body does not govern this task — it is a phase-8 implementation-analysis finding, so its own body is the authority (per the shared verifier context). The work unit's governing concern is the silent loss of live sessions: `internal/state/capture.go` discriminates a per-session tmux failure into "vanished" (natural churn, dropped quietly) versus "anomalous" (WARN, not counted as churn), and that discrimination is driven by `internal/tmux`'s `wrapSessionTargetErr`, which must classify the *name* before it reads tmux's stderr, because tmux answers an unaddressable name with the same "no such session" words a vanished one produces. The task extends that classification from the measured `:` case to the measured leading-`$` case, and closes the generation/recognition drift between `session.SanitiseProjectName` and `tmux.ValidateSessionName`. Spec Corrigenda hold nothing bearing on this task.

IMPLEMENTATION:
- Status: Implemented (and legitimately moved past the task text by later phase-8 work — see Notes)
- Location:
  - `internal/tmux/errors.go:74` — `sessionIDPrefix = "$"`, with the measurement recorded in the comment ("bare and exact forms were measured and both fail"); `:89` — `ErrSessionNameIDPrefix` carrying the clause that names the offending character; `:99-110` — `ValidateSessionName` runs separator → ID prefix → flag prefix, each wrapping `ErrUnaddressableSessionName` plus its own rule sentinel. The `$` rule is positional (`strings.HasPrefix`, `:103`), as prescribed.
  - `internal/tmux/errors.go:118-126` — `wrapSessionTargetErr` still checks the name first, so the widened rule reaches every per-session operation without a further edit.
  - `internal/session/naming.go:25` — `unwritableLeadingChars = "$-"`; `:31-35` — `SanitiseProjectName` replaces `.`/`:` with `-` and then `strings.TrimLeft`s the unwritable leading characters, so one unwritable character never escapes into another (a leading `.` becomes `-` and is then dropped rather than shipped as a flag-leading name).
  - `internal/session/naming.go:40-61` — `GenerateSessionName` contributes no separator for an empty fragment, so `"$"` mints the nanoid alone rather than `-abc123`.
  - `internal/tui/model.go:2667` and `internal/tui/sessions_flash.go:58-74` — the picker's `r` modal now selects its band wording from the rule sentinel (`renameRefusalFlash`), so a `$`-leading rename is refused in its own words rather than under the separator copy.
  - `README.md:197` mirrors all three refusal strings verbatim, honouring CLAUDE.md's "a change to either must move both" rule for this user-visible copy.
- Notes:
  - The two rules are kept deliberately distinct, as Do item 4 required: `internal/tmux` refuses exactly what was measured unaddressable (`.` is accepted — `errors.go` has no period rule), while `internal/session` strips a superset (`.` stays sanitised for tidiness). The drift is closed by the generated-name guard rather than by collapsing either rule into the other.
  - Divergence from the task's Do item 2, and it is a gain rather than a loss: the task as written (and commit 0fd96a03 as first landed) *substituted* a leading `$` with `-`, which minted `-foo-abc123`. A later phase-8 task added the flag-prefix rule to `ValidateSessionName` and changed the sanitiser to drop rather than substitute. Judged against intent — "no name `GenerateSessionName` can produce is one `ValidateSessionName` rejects" — the current code is strictly better than the task's literal instruction, and the guard the task asked for is what proves it.
  - `internal/session`'s guard lives in the external `session_test` package and imports `internal/tmux`, so it introduces no production edge and no cycle.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/tmux/session_name_test.go:204-230` — `TestValidateSessionName` holds all three named subtests verbatim: "it refuses a session name beginning with $" (asserts the `ErrUnaddressableSessionName` wrap *and* that the message names `"$"`), "it accepts a $ that is not leading", "it accepts a name containing a period". Criteria 1 and 2 (recognition half).
  - `internal/session/naming_test.go:14-70` — `TestSanitiseProjectName` covers the leading-`$` drop, the leading-period case (`.dotfiles` → `dotfiles`, which is what proves the drop is applied *after* the replacement), the non-leading `$`, and that `.`/`:` are still replaced. Criterion 2 (generation half).
  - `internal/session/naming_test.go:219-242` — `TestGenerateSessionNameProducesAddressableNames` / "it generates only names ValidateSessionName accepts", over `{"$foo", "$", "a:b", "a.b", "$a:b", "", "-lead", ".dotfiles"}` — the prescribed table plus three the implementation added. Criterion 3.
  - `internal/state/capture_colon_session_test.go:14-55` — the discriminator test now loops over `{colonSession, dollarSession}` and asserts both halves of the criterion: the anomalous WARN is present *and* the word "vanished" is absent, with the colon-free session still captured so a run that surfaced nothing cannot pass. `internal/state/capture_colon_session_realtmux_test.go:18-63` carries the same pair against a real tmux server on a per-test socket, with an honest "a tmux that can address the name captures it" early return. Criterion 4.
  - `internal/tui/rename_colon_refusal_test.go:38-63` — the `$` path through the picker: no renamer reached, modal closed, the ID-prefix flash text selected, the character named, no `⚠` glyph embedded.
- Notes:
  - Each test would fail if the behaviour broke: dropping the `$` branch from `ValidateSessionName` fails both the unit assertion and the capture discriminator test; dropping the `TrimLeft` fails the sanitiser test and the addressability guard.
  - Not over-tested. The three surfaces asserted (validator, generator, capture discriminator) are three distinct behaviours, and no assertion is restated across files — `ValidateSessionName` is exercised directly in exactly one suite and consumed as an oracle in the generator guard.
  - The guard is a table rather than a property, so it cannot literally prove "no project directory name". That is exactly what Do item 3 prescribed, and the implementation widened the prescribed table rather than narrowing it.

CODE QUALITY:
- Project conventions: Followed. The rule sentinels follow the package's existing wrap-and-discriminate idiom (`errors.Is` on a leaf sentinel, multi-`%w` so both the sentinel and tmux's own words stay reachable); the measurement behind each constant is recorded in its comment, matching the file's established practice; the user-visible copy is mirrored into README as CLAUDE.md requires. No new log component or attr key was invented.
- SOLID principles: Good. Recognition (`internal/tmux`) and generation (`internal/session`) stay separate rules with separate reasons, joined only by a test-level guard — which is the right seam, since collapsing them would force a positional rule onto a fragment where it has no meaning.
- Complexity: Low. Three linear prefix/contains checks and one `TrimLeft`.
- Modern idioms: Yes. `strings.HasPrefix`/`TrimLeft`, `fmt.Errorf` with multiple `%w`, sentinel errors as package vars.
- Readability: Good. `renameRefusalFlash` reads as a table of rule → copy; `unwritableLeadingChars` names the concept rather than listing it at the call site.
- Comment accuracy: The comments hold against the code. `unwritableLeadingChars`' doc names both characters and both reasons; `SanitiseProjectName`'s doc states the empty result is possible, which `GenerateSessionName` then handles explicitly; `ValidateSessionName`'s doc correctly describes both the exact-target and bare-positional grounds now that the flag rule exists. No process artefacts (task ids, phase numbers, spec sections) appear in any of the changed source.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
