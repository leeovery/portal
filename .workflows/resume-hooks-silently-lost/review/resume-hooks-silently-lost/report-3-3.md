TASK: resume-hooks-silently-lost-3-3 — "An Empty Key Fires On Nothing" (tick-7987c1, done)

ACCEPTANCE CRITERIA:
- `buildHydrateCommand` emits no `--hook-key` token anywhere in its output for an empty key, and the rest of the command line is unchanged
- `buildHydrateCommand` emits a single-quoted `--hook-key '<token>'` for a non-empty key, unchanged from today
- `portal state hydrate --fifo <f> --file <s>` with no `--hook-key` parses and runs; `--fifo` and `--file` remain required
- An absent `--hook-key` and an empty `--hook-key ''` produce identical behaviour on every hydrate path
- `LookupOnResume` with an empty key returns `("", false, nil)` even when `hooks.json` holds a `""` entry whose `on-resume` is set
- `LookupOnResume` with an empty key reads no file: an unreadable or absent `hooks.json` still returns a clean miss with no error
- A non-empty key behaves exactly as it does today, including the load-error and miss paths
- A whitespace-only key is not special-cased — it is looked up literally
- The `--hook-key` help text describes the pane token
- Restoring a saved pane with no token still creates its FIFO, arms it and hydrates it; only the hook is absent

STATUS: complete

SPEC CONTEXT:
Spec §3.4 ("An empty key is rejected at every boundary") requires **two independent guards**: `collectArmInfos`/`buildHydrateCommand` must bake no empty key (concretely: omit the `--hook-key` flag rather than pass an empty value, and treat an absent flag and an empty one alike as "no hook", "so the flag cannot be required" — under lazy stamping an unstamped pane is the ordinary case), and `LookupOnResume` must return "no hook" for an empty key **before the map is consulted**, whatever the file holds. §3.3 keeps the flag and its meaning, corrects its help text away from `Saved structural identifier (<session>:<window>.<pane>)`, and retains the `shellquote.Single` quoting on the interpolated values. §9.2's acceptance row ("An empty key fires on nothing") is unit-lane.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/restore/session.go:343-358` — `buildHydrateCommand(exe, fifo, file, hookKey)` builds `<exe> state hydrate --fifo <q> --file <q>` and returns early for `hookKey == ""`; the non-empty path appends `" --hook-key " + shellquote.Single(hookKey)`. All three retained values still go through `shellquote.Single` (`session.go:351-352`).
  - `internal/restore/session.go:127-155` — `armPanes` creates the FIFO (`state.CreateFIFO`, :141) and respawns unconditionally; `restampPaneToken` (:162-169) skips an empty token, so an untokened pane is armed and hydrated with only the hook absent.
  - `cmd/state_hydrate.go:292-302` — `MarkFlagRequired("hook-key")` is gone; `--fifo`/`--file` remain required (:298-299); the flag's help is now `Saved pane token identifying the pane's resume hook` (:297) with a comment stating the absent/empty equivalence (:295-296). An absent flag yields cobra's `""` default at `:269`, which is the same value an explicit `--hook-key ''` produces, so every downstream path (normal replay `:149`, timeout `:89`, file-missing `:119`/`:134`) converges on the same `execShellOrHookAndExit` miss.
  - `internal/hooks/lookup.go:14-17` — `LookupOnResume` returns `("", false, nil)` for an empty key before `s.loadShared(via)` at `:18`, so the file is neither read nor indexed. The comparison is `hookKey == ""` only; no trimming anywhere.
- Notes: The task was authored against the pre-consolidation signatures (`LookupOnResume(store, hookKey)`, `buildHydrateCommand(fifo, file, hookKey)`, `shellQuoteSingle`); later phases moved the lookup onto `*Store` with a `Via`, added the `exe` parameter and re-homed the quoting in `internal/shellquote`. The guards survived both moves intact — no drift in substance. `cmd/state_hydrate.go:179` is the only production caller of `LookupOnResume` (verified by a repo-wide grep of non-test Go sources), and `internal/restore/session.go:351` is the only production site composing a `state hydrate` argv, so both guards sit on the single path each protects. CLAUDE.md:186 already states the matching contract ("an empty saved token bakes no key and the pane restores with no hook").

TESTS:
- Status: Adequate
- Coverage:
  - `internal/restore/session_build_hydrate_test.go:13-30` — the two rendering criteria: exact non-empty rendering, and for the empty key both an exact-string match and an explicit "contains no `--hook-key`" assertion.
  - `internal/restore/session_test.go:268-312` — the same two properties driven through a real `Restore` (baked token vs. untokened pane); `Restore` returning nil is what proves the FIFO was created, since `CreateFIFO` failure aborts arming (`session.go:141-143`).
  - `internal/restore/session_test.go:409-423` — `extractHookKey` now returns a `found` bool and distinguishes "no flag at all" from "flag with an empty value", exactly as the task required; `respawnPaneHookKeys` (:392-407) fatals on a missing flag, and every caller of it seeds tokens.
  - `cmd/state_test.go:136` — cobra parses `state hydrate --fifo … --file …` with no `--hook-key`; `cmd/state_test.go:182-199` keeps the missing-`--fifo` / missing-`--file` / no-flags cases, so the two surviving required flags are still enforced (and those cases no longer pass by accident on the dropped `hook-key` requirement).
  - `cmd/state_hydrate_empty_hookkey_test.go:43-89` — absent-vs-empty equivalence at both levels: cobra parse (both yield `""`) and a full `runHydrate` against a `hooks.json` seeded `{"": {"on-resume": "rm -rf /"}}`, asserting the exec target is `$SHELL` and the two runs' exec target+argv are identical.
  - `cmd/state_hydrate_empty_hookkey_test.go:92-137` — the unstamped-pane end state: scrollback replayed to stdout, bare `$SHELL` exec, `hook_present=false` on the exec INFO.
  - `cmd/state_hydrate_empty_hookkey_test.go:139-157` — the timeout WARN renders an empty `hook_key`, pinning that no branch was added for it.
  - `internal/hooks/lookup_test.go:98-103` (empty key with a seeded `""` entry misses), `:105-124` (whitespace key misses when unseeded, hits when literally seeded under `" "`), `:167-177` (empty key against a *directory* at the hooks.json path still returns a clean miss — a genuine proof the file was never read, since the same fixture at `:126-148` surfaces EISDIR for a non-empty key). The load-error case correctly uses a non-empty key, as the task required.
- Notes: The equivalence test's second half compares two values that are both `""` by construction, so only its cobra half can distinguish absent from empty; the "" entry does not fire is however genuinely asserted there and again in the unstamped-pane test. Mild overlap between those two `cmd` tests (both seed the same `""` entry and assert a `$SHELL` exec), but each has its own subject — equivalence versus the whole replay-and-exec end state — so neither is redundant enough to remove. No implementation-detail testing, no excess mocking; the fixtures use `hookstest.StageStore` with `SidecarAbsent: true`, which is the documented convention for hydrate fixtures.

CODE QUALITY:
- Project conventions: Followed. Both tests are unit-lane (no portal binary, no daemon, no real tmux server); `cmd` tests stage the `hydrateRunFunc` seam through `withFuncSeam` rather than assigning it directly, satisfying `cmd/seam_guard_test.go`; `PORTAL_STATE_DIR` is pointed at a temp dir on every hydrate-parse path. No new log component or attr key was introduced.
- SOLID principles: Good. Each guard sits at the boundary that owns it — the renderer decides whether to emit the flag, the store decides whether a key is lookupable — and neither depends on the other.
- Complexity: Low. One early return in each of two functions.
- Modern idioms: Yes.
- Readability: Good. Both doc comments state the *why* (the map index is exact, so a stray `""` entry would fire on every unkeyed pane) rather than restating the branch.
- Issues: None. The rewritten `LookupOnResume` comment no longer claims the old raw-identifier/colon round-trip contract, and the `--hook-key` help text now names the pane token, so no comment or help string in the changed code is falsified by it.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
