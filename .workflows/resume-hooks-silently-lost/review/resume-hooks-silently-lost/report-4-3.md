TASK: resume-hooks-silently-lost-4-3 — Hook List Shows Where Each Token Lives (tick-2d146f)

ACCEPTANCE CRITERIA:
- `hook list` emits a fourth tab-separated field carrying the resolved `<session>:<window>.<pane>` location for a token a live pane carries
- The `key` / `event` / `command` field positions are byte-identical to today for every line
- The column is always emitted: an unresolved token gives an empty fourth field, never a dropped one, so every line carries exactly three separators
- The token → location map is built from non-empty tokens only: an entry keyed by `""` never picks up an unstamped pane's location
- A token no live pane carries renders empty; an old-format key renders empty
- A failed enumeration — including no tmux server running — renders every fourth field empty and still exits 0; `hook` starts no tmux server
- Exactly one enumeration read is taken per invocation regardless of entry count, and zero reads when there are no entries
- A token carried by two rows renders one line whose location is the first row's
- A location whose session name carries `|` renders verbatim
- The existing sort by key then event is unchanged
- `go test ./...` and `go test -tags integration -p 1 ./...` both pass

STATUS: complete

SPEC CONTEXT:
§4.4 fixes the user-visible contract: `hook list` gains an appended fourth tab-separated column carrying the token's resolved location, built from one `list-panes -a` read over the §3.3 enumeration, mapped from non-empty tokens only, always emitted (empty rather than dropped), and never able to fail the command — including on a machine with no tmux server, which `hook` is bootstrap-exempt from starting. §3.3 fixes the enumeration's shape (one row per live pane, `Token` empty for an unstamped pane, `Location` display-only and never a key). §5.3 is the boundary the task's Context restates: this column serves entries that still exist; recoverability of a reaped hook is the reaper's own INFO line (Phase 1), not this. §9.2 lists the unit-level verification for the column. Spec is silent on logging a failed enumeration; the task recorded the decision to emit nothing, on the grounds that the work unit's `hooks` `op` vocabulary is closed at `load-unlocked` / `touch-save-requested` / `clean-stale-skipped` and inventing an `op` at a call site is forbidden — consistent with CLAUDE.md's closed-taxonomy rule.

IMPLEMENTATION:
- Status: Implemented (later phases re-plumbed the seam defaulting; the delivered behaviour is intact)
- Location:
  - `cmd/hooks.go:21-27` — the narrow one-method `PaneHookLister` seam (`ListAllPaneHookKeys() ([]tmux.PaneHookRow, error)`), deliberately not the sweep's marker-carrying lister, which now lives in `internal/hooksweep/sweep.go:31` as a separate interface — the two never merged.
  - `cmd/hooks.go:36` — `var _ PaneHookLister = (*tmux.Client)(nil)` compile-time assertion, beside its two siblings.
  - `cmd/hooks.go:44`, `cmd/hooks.go:93-95` — the `HooksDeps.PaneLister` field and its production default through `buildHooksTmuxClient()`, filled in by `hookSeams()` exactly as `KeyResolver` and `PaneStamper` are. (Task 4-3 shipped a per-seam `buildPaneHookLister()`; the later `hookSeams()` consolidation, commit c9345df3/3a8e761f, folded all four defaults into one merge point. Same contract, one route.)
  - `cmd/hooks.go:146-149` — the empty-list early return: zero entries means the enumeration is never reached.
  - `cmd/hooks.go:151` — exactly one enumeration per invocation, hoisted out of the print loop.
  - `cmd/hooks.go:154` — `"%s\t%s\t%s\t%s\n"` with `locations[h.Key]`, so the map's zero value renders an empty fourth field and the first three positions are untouched.
  - `cmd/hooks.go:163-188` — `paneLocationsByToken`: error → nil map (safe to index, every location empty, exit 0, no log line); empty `Token` rows skipped (`:177`); first row wins for a duplicate token (`:182`).
- Notes:
  - Bootstrap exemption verified end to end: `cmd/root.go:88-93` walks to the root parent and `cmd/root.go:28` holds `"hook"`, so `hook list` never enters the orchestrator; the read itself is `list-panes -a -F` (`internal/tmux/tmux.go:604`), which fails against a down server rather than starting one.
  - `hooks.Event` is a string kind with a `String()` (`internal/hooks/event.go:11-19`), so the `%s` verb renders the persisted key — the event field is byte-identical to before.
  - Sort is untouched: `internal/hooks/store.go:229-234` still orders by key then event, and `hooksListCmd` only reads what `List` returns.
  - A `|` in a session name survives the row parse upstream (`internal/tmux/tmux.go:666-681` cuts at the first separator only, and the token half can never carry one — `nanoid.Alphabet` has no `|`), and the print path does no parsing, so the location renders verbatim.
  - `README.md:203` already documents the fourth column; `CLAUDE.md:45` documents the one-enumeration-per-invocation / none-when-empty contract. Neither was this task's to write, and neither contradicts the code.

TESTS:
- Status: Adequate
- Coverage: `cmd/hooks_test.go:119-270` (`TestHooksListLocationColumn`) carries all ten named subtests the task specified, each observing a distinct criterion:
  - `:120` resolved location as the fourth column (exact line);
  - `:136` a `""`-keyed entry beside an unstamped row — asserts the empty fourth field, which is what fails if the map ever admits an empty token;
  - `:152` duplicate token across two rows — one line, first row's location;
  - `:169` enumeration error with rows returned alongside it (a fake that hands back both, so the code is pinned to judging the error rather than the row slice) — full expected output, exit 0 via `runHookList`'s fatal-on-error;
  - `:189` zero entries against `loudPaneHookLister` (`cmd/hooks_test.go:272-280`), which fails the test the moment it is read — the early return is observed, not assumed;
  - `:200` a token no row carries; `:217` an old-format key beside a token-keyed one; `:235` a `|`-bearing location; `:251` call-count assertion (`lister.calls != 1`) over four entries.
  - The re-pointed exact-line subtests in `TestHooksListCommand` (`:34`, `:98-100`) pin the key/event/command positions and the key-then-event ordering at four fields.
  - Every `hook list` driver in the package injects the seam (`cmd/hooks_read_lock_test.go:75,101`, `cmd/root_test.go:344`, all of `cmd/hooks_test.go`), so no test reaches a real server — consistent with cmd's package-wide `TMUX` poison.
- Notes:
  - Non-blocking redundancy: `TestHooksListCommand`'s "outputs hooks in tab-separated format" (`cmd/hooks_test.go:20-38`) is now assertion-identical to `TestHooksListLocationColumn`'s "it appends the resolved location as a fourth column" (`cmd/hooks_test.go:120-134`) — same seed, same row, byte-identical `want`. The two were distinct when 4-3 landed and converged under later fixture-vocabulary edits (c7f7bd50a). Harmless; the first also carries a `withBootstrapDeps` stub (`:27`) that a bootstrap-exempt verb does not need, inherited from a4bc7bd5. Not worth an edit on its own.
  - The `|`-verbatim subtest exercises the render layer with a pre-built row rather than the enumeration's parse; the parse itself is covered in `internal/tmux` (`pane_hook_rows_test.go`), which is the right home for it.
  - Test execution was not attempted (reader role). The suite reads as consistent: every seed name used (`hookstest.SubjectSeedA`–`D`, `UnjudgeableSeedA`) exists (`internal/hookstest/hooks.go:171-198`), and the ordering the sort subtests expect holds under `nanoid.Alphabet` (`SubjectSeedA/B/C` differ only in a trailing `h`/`i`/`j`, and `"seed…"` sorts before `"unjudgeable-…"`).

CODE QUALITY:
- Project conventions: Followed. Seam is a 1-method interface on the package `*Deps` struct with a compile-time assertion and a production default (CLAUDE.md DI pattern); no `slog` construction; no new log component or `op` invented at the call site; unit-lane only, no `t.Parallel()`, no real tmux touched.
- SOLID principles: Good. The seam is segregated to the one method the listing needs, and does not borrow the sweep's marker-carrying lister — the two consumers stay independent (`internal/hooksweep/sweep.go:31` declares its own).
- Complexity: Low. One helper, two guard clauses, one loop; the command body gained four lines.
- Modern idioms: Yes. Pre-sized map, `for range` over rows, nil-map read as the zero-value lookup rather than a branch at the print site.
- Readability: Good. Each of the three non-obvious decisions (silent error, empty-token skip, first-row-wins) carries a comment stating why rather than what.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
