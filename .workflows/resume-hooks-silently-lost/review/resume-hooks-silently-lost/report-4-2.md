TASK: resume-hooks-silently-lost-4-2 — "Removing Nothing Is Never A Success" (`portal hook rm` exits 0 iff it removed an entry)

ACCEPTANCE CRITERIA (from the plan task):
- `hook rm --on-resume` exits 0 iff `Store.Remove` reported a removal, the answer coming from that removal and never from a read taken before it
- A `$TMUX_PANE` naming no live pane exits non-zero carrying tmux's own words; no production code inspects that text
- A live pane carrying no token exits non-zero with `no resume hook registered for this pane`, decided before the store is consulted
- A live stamped pane whose token has no entry exits non-zero with `no resume hook registered for <token>`
- `--pane-key` naming no entry exits non-zero with the literal key in the message and issues no tmux call
- `--pane-key <seeded key>` still removes and exits 0, no tmux call, no validation
- Every non-zero route leaves `hooks.json` byte-identical; an absent file stays absent
- No empty key reaches `Store.Remove` from either path
- `hook rm` mints no token and issues no pane-option write on any path
- `hook rm` writes no `save.requested` on any path
- The non-zero exit comes from `RunE` returning an error — no new exit-code plumbing, no usage dump
- CLAUDE.md's Resume-hooks paragraph states the exit-0-only-on-removal rule
- `--pane-key`'s flag help describes the pane token and no longer says `Structural key`
- README's `xctl hook` section carries no positional `--pane-key` example and states the non-zero-on-nothing-removed rule
- No other README passage is edited
- `go test ./...` and `go test -tags integration -p 1 ./...` both pass

STATUS: complete

SPEC CONTEXT:
§4.2 fixes the rule as one line — `hook rm` exits 0 **iff** it removed an entry — and fixes the three no-removal wordings: tmux's own words for a gone pane (via the §4.1 existence probe), `no resume hook registered for this pane` for a live pane carrying no token, `no resume hook registered for <key>` for any key naming no entry. It rejects the idempotent reading explicitly, and requires the answer to come from the locked removal itself rather than a read taken before it, since a pre-read decides the exit from a snapshot the mutation never saw. §4.3 keeps `--pane-key` a literal pass-through — no validation, no tmux call — while carrying the same exit rule with the literal key. §4.2 also records that this failure now fires routinely (61 of 63 degenerate log lines were `op=rm` from a SessionEnd firing because the pane had closed), so no retry or soft-success arm may soften it. The Corrigenda hold nothing bearing on this task.

IMPLEMENTATION:
- Status: Implemented, matching the spec and the task's Do list
- Location:
  - `cmd/hooks.go:259-302` — `hooksRmCmd`'s `RunE`. `:264-267` reads `--pane-key`; `:269-272` uses a non-empty value verbatim with no validation and no tmux touch (`hookSeams()`/`buildHooksTmuxClient()` are reached only from `resolveCurrentPaneKey`, so the pass-through constructs no client); `:273-276` otherwise resolves through `resolveCurrentPaneKey`, whose existence probe ends the command with the resolver's error unaltered; `:280-282` returns `no resume hook registered for this pane` for an empty resolved key **before** `loadHookStore`, so no empty key reaches a mutation; `:292-298` drives the exit from `removed, err := store.Remove(...)` — `err` returned unchanged, `!removed` returning `no resume hook registered for <key>`, `nil` only on a true removal. No `Get`/`Load` precedes the removal on either path, and no `touchSaveRequestedForHook` call exists in this body (it is `hook set`-only, `cmd/hooks.go:222`).
  - `internal/hooks/store.go:173-209` — the `(bool, error)` removal this exit status reads from; the boolean is derived from the map loaded and mutated under the exclusive hold (`:188-199`), and a failed save reports `false` (`:201-205`).
  - `cmd/hooks.go:310` — `--pane-key` help now reads "Hook key of the entry to remove, taken verbatim — any key, including an old-format one (defaults to the current pane's token)"; the retired "Structural key" wording is gone and the defaults clause is kept.
  - `CLAUDE.md:184` — the Resume-hooks paragraph states "**`hook rm` exits 0 iff it removed an entry**", the three wordings, the no-pre-read rule, and that removal neither mints nor unstamps and touches no dirty flag. The key-scheme passages at `:186-194` are untouched by this wording.
  - `README.md:191` — the `xctl hook` prose now states `hook rm` exits non-zero when it removes nothing, naming the three cases; `README.md:202` — the example is `--pane-key 'k3Xp7Q'`, which is token-shaped by `nanoid.IsTokenShaped` (6 bytes, all in `nanoid.Alphabet`, `internal/nanoid/nanoid.go:17,58,65`). The rename guarantee (`README.md:195-197`) and the "When hooks fire" paragraph (`:206-210`) read as before.
- Notes: no retry, no soft-success arm, no exit-code plumbing was added; the error travels out through `RunE` by the route every other `hook` failure takes.

TESTS:
- Status: Adequate
- Coverage: `cmd/hooks_rm_exit_test.go:158-458` covers all eleven tests the task listed, one subtest per route:
  - `:159-177` gone pane — the resolver's `*tmux.CommandError` reaches the user with its stderr unaltered (`CommandError.Error()` renders the trimmed stderr alone for an argv-less value, `internal/tmux/command_error.go:24-39`) and stays recoverable by `errors.As`.
  - `:179-192` / `:194-206` live pane carrying no token — exact message, and the store is not consulted (absent `hooks.json` stays absent).
  - `:208-225` resolved token naming no entry, `:227-244` `--pane-key` naming no entry (driven against the poisoned resolver/stamper pair from `cmd/hookkey_vocabulary_test.go:183-185`, so any pane read would surface the seam's error in place of the asserted message).
  - `:246-272` / `:274-295` the two success paths, the second asserting zero pane calls through `assertNoPaneTmuxCalls`.
  - `:297-352` byte-identity over the four failing routes plus the unset-`TMUX_PANE` route. The "live pane carrying no token" row (`:320-328`) seeds a `""` entry alongside a live one — this is the row that discriminates the guard's *position*: hoisting it below `store.Remove` would have `Remove("")` delete that entry and change the file, so the criterion "no empty key reaches `Store.Remove`" is genuinely mutation-covered.
  - `:354-393` mint/stamp count zero across a successful removal, a no-token pane and `--pane-key`; `:395-433` no `save.requested` on success or failure; `:435-457` the failure is a plain error — not a `*UsageError`, not silent-exit, no usage dump.
  - `cmd/hooks_test.go:623-633` pins the flag help string verbatim; `cmd/hooks_seams_test.go:91-135` pins the gone-pane rendering for `hook set` and `hook rm` together against a scripted commander, so the probe itself (not a fake's invented shape) is what is measured.
  - The `rmCase`/`runRmCase` harness is reused by the lock-timeout suite (`cmd/hooks_write_lock_test.go:191-223`), which is what `rmCase.holdLock` serves — no dead field.
- Notes: `TestRmCaseRows` (`:110-156`) tests the harness's own refusal path through `harnesstest.Recorder` rather than the command — in-keeping with this repo's convention for fatal-on-failure helpers, not redundancy. One naming observation, deliberately **not** raised as a finding: the subtest name at `:194` ("it consults the store for nothing …") promises more than its two assertions establish — an empty key that did reach `Store.Remove` would also leave the absent file absent — but the property it names is covered by the byte-identity row at `:320-328`, and the plan prescribed exactly these assertions under this name.

CODE QUALITY:
- Project conventions: Followed. The removal path stays inside the `hooksDeps` seam discipline (no direct tmux construction on the `--pane-key` branch); tests stage seams through `withHooksDeps` (`cmd/testhelpers_test.go:26-30`) and never assign `hooksDeps` directly, so `cmd/seam_guard_test.go` stays satisfied; no `t.Parallel()`; the whole suite is unit-lane and touches no real tmux server; fixtures route through `hookstest`'s staging surface rather than hand-rolled marshal-and-write.
- SOLID principles: Good. The exit decision reads one boolean from the store's own mutation; the store keeps the file rules and `cmd` keeps the wording.
- Complexity: Low — one branch on the flag, one on the empty key, two on the removal result.
- Modern idioms: Yes. (`fmt.Errorf` with no verbs at `cmd/hooks.go:281,297` would read as `errors.New`; neither the standard linter set nor `modernize` flags it and it matches the file's neighbours — noted, not a finding.)
- Readability: Good. Both comments (`:277-279`, `:290-291`) state *why* the guard sits before the store and why the answer comes from the mutation; both hold true against the code beneath them.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
