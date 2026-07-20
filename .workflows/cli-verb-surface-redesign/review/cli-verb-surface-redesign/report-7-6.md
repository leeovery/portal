TASK: cli-verb-surface-redesign-7-6 — Update CLAUDE.md command-surface prose to the redesigned surface

ACCEPTANCE CRITERIA (from plan row 7-6; docs-only, no automated test):
- Remove all removed-surface refs: `portal spawn` / `portal clean` CLIs, `attach --spawn-ack`, `cmd/spawn.go`, `cmd/clean.go` as `loadPrefsStore` home.
- Add: `doctor [--fix]`, `uninstall`, open multi-target burst, hidden `--ack`, `hooks` → `hook`.
- Fix bootstrap-exempt list (drop `clean`; list `doctor`/`uninstall`).

STATUS: Complete

SPEC CONTEXT:
The spec's "Command Surface Summary (final shape)" is the target end-state: `open` absorbs `attach`+`spawn` (multi-target burst, domain pins, hidden `--ack`); `doctor [--fix]` is new (subsumes `state status`, replaces `clean`, folds in `spawn --detect`); `uninstall` is new (replaces `state cleanup`); `hooks` → `hook` (canonical) with `hooks` kept as a permanent silent cobra alias; `doctor` & `uninstall` join `skipTmuxCheck` (bootstrap-exempt) while `clean` leaves the exempt set. `attach`/`spawn` are deleted outright (no aliases).

IMPLEMENTATION:
- Status: Implemented
- Location: CLAUDE.md (repo root) — key lines:
  - L35 open key-command prose: resolution grammar, domain pins, `-f/--filter`, no-args picker, multi-target routing; "former `portal attach` command is retired".
  - L37 Multi-target burst prose: `cmd/open_burst.go`/`cmd/open_burst_run.go`, hidden `--ack`, "There is no `portal spawn` CLI."
  - L39 Diagnostics: `cmd/doctor.go` / `portal doctor [--fix]`, bootstrap-exempt, subsumes `state status`.
  - L41 Teardown: `cmd/uninstall.go` / `portal uninstall`, bootstrap-exempt.
  - L43 Resume-hook command: `portal hook set/rm/list`; `hook` canonical, `hooks` permanent silent alias.
  - L86 `loadPrefsStore` correctly homed at `cmd/config.go` (verified: `cmd/config.go:149`), NOT `cmd/clean.go`.
  - L121 bootstrap-exempt `skipTmuxCheck` set lists `doctor`, `uninstall`; does NOT list `clean`.
- Notes:
  - Grep for removed strings (`--spawn-ack`, `cmd/spawn.go`, `cmd/clean.go`, `portal clean`, `spawn --detect`) returns ZERO live references. The only hits for `portal spawn` / `portal attach` / `state status` / `state cleanup` are retirement/absence/historical framings ("is retired", "There is no `portal spawn` CLI", "subsumes the retired `state status`", and the L114 pre-redesign incident-of-record which itself notes the fold into `portal uninstall`). None assert the removed surfaces still exist.
  - Prose matches on-disk reality: `cmd/doctor.go`, `cmd/uninstall.go`, `cmd/hooks.go` present; `cmd/spawn.go`, `cmd/clean.go`, `cmd/attach.go` absent.
  - Internal-name survivals are correct and spec-sanctioned (out of redesign scope): `internal/spawn`, the `spawn` log component (L64/L187), `SweepLogsForClean` (L60), the `clean:` sweep cycle-summaries, and the former step-11 `CleanStale` note (L134) — all refer to internal machinery, not the deleted `portal clean` verb.

TESTS:
- Status: N/A (docs-only chore — no automated test per the plan row; correctly so)
- Coverage: Verification is by grep of the removed/added strings against CLAUDE.md plus cross-check against on-disk cmd files.
- Notes: None.

CODE QUALITY:
- Project conventions: N/A (prose)
- SOLID principles: N/A
- Complexity: N/A
- Modern idioms: N/A
- Readability: Good — the four new command-surface paragraphs (L35–43) are consistent in voice with the surrounding architecture prose and cross-link ("see ... below").
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] CLAUDE.md:39 — The doctor entry states it "subsumes the retired `state status`" but omits the spec's other two doctor roles (replaces `clean` for repairs, and folds in the retired `spawn --detect` host-terminal line). Optional: append a short clause noting doctor also replaces `clean` and reports the host-terminal check, to fully mirror the spec's Command Surface Summary. Zero logic risk; purely additive prose.
