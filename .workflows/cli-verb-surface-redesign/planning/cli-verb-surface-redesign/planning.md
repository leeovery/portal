# Plan: CLI Verb Surface Redesign

## Overview

Portal's CLI is redesigned in one intentional pass: the three public session verbs (`open`, `attach`, `spawn`) collapse into a single public `open`, with unified outcome-based target resolution (attach-vs-mint), a multi-target absorb/net-N burst, and a reshaped maintenance surface (`doctor`/`uninstall` replacing `clean`/`state status`/`state cleanup`). The `state` namespace is hidden, `hooks` is renamed to `hook`, and tab completion is added for Portal-owned namespaces.

**Overall sequencing rationale.** The redesign's centre of gravity is collapsing three session verbs into one `open`, so the plan builds `open` outward from its core: single-target resolution grammar (Phase 1) → domain-pinning flags and command semantics (Phase 2) → multi-target absorb/net-N burst (Phase 3). Only once `open` fully covers what `attach` and `spawn` did do we reshape the maintenance surface into `doctor`/`uninstall` (Phase 4) — deliberately *before* retiring `spawn`, so `doctor`'s host-terminal line is the ready replacement for `spawn --detect` when `spawn` is deleted (Phase 5). Retirement is the completion of the `open` arc and depends on both the burst (Phase 3) and `doctor` (Phase 4). Final surface presentation — the `hook` rename, `state` hiding, and tab completion — lands last (Phase 6) because it depends on the finalised command and flag shapes. Each phase leaves the CLI in a working, testable state; the old verbs remain as a safety net until Phase 5 proves the new surface.

---

## Phase 1: `open` single-target resolution grammar
status: approved
approved_at: 2026-07-18

**Goal**: Replace `open`'s directory-only resolution chain with the new outcome-based grammar for a single bare target — glob pre-check, the precedence chain (exact session name → path → alias → zoxide), the attach-vs-mint dichotomy (Axiom 2), hard-fail on total miss (implicit TUI-fallback removed, `-f` escape hatch added), and the new `resolve` log component. This is the foundational new behaviour every later phase builds on.

**Acceptance Criteria**:
- [ ] A bare target containing glob metacharacters (`*`, `?`, `[…]`) is treated as session-domain by construction: it expands against the user-visible session set and never runs the path/alias/zoxide chain; zero matches is a hard fail.
- [ ] A non-glob bare target resolves first-match-wins through exact session name → path → alias → zoxide; a session-name hit **attaches** the existing session, a path/alias/zoxide hit **mints** a fresh `{project}-{nanoid}` session (no find-or-create).
- [ ] All session-domain matching (exact name and glob) matches only the leading-underscore-filtered `ListSessions` view; `_portal-saver` / `_portal-bootstrap` are never matchable and fall through/hard-fail as if absent.
- [ ] A bare project name (`open api`) never reattaches an existing `api-*` session — it falls through to path/zoxide and mints, per the accepted "bare shorthand does not reattach" consequence.
- [ ] A target that resolves to nothing hard-fails at single-target arity with a message pointing at the escape hatch, e.g. `nothing resolved for 'blog' — try -f blog`; the old implicit picker-with-filter fallback is gone.
- [ ] `-f/--filter <text>` opens the picker on the Sessions page pre-filled with `<text>`, skipping resolution; it is mutually exclusive with a positional target (usage error otherwise).
- [ ] `open` with no args still launches the TUI picker unchanged.
- [ ] The `resolve` log component is added to the closed taxonomy and bound once via `log.For("resolve")` in `cmd/open.go`; a bare positional resolved through the chain emits one INFO line with attrs `target`, `domain` (session/path/alias/zoxide, or `miss`), and `resolved_path` (resolved dir, or session name for a session hit, empty on miss); `internal/resolver` stays log-free.

**Rationale**: The feature's Phase 1 must deliver the most fundamental new capability integrated with existing patterns. The resolution grammar is that capability — attach-vs-mint classification is the contract that pins, the burst, and command-scoping all consume. Removing the TUI fallback and adding `-f` together keeps the miss-handling story complete (the error message's suggested flag actually works), leaving a coherent, testable single-target `open`.

#### Tasks
status: approved
approved_at: 2026-07-18

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| cli-verb-surface-redesign-1-1 | Exact session-name match → attach outcome | internal `_`-prefixed sessions never matchable (filtered ListSessions view, not tmux HasSession), empty session set, inside-tmux attach via switch-client vs outside via exec attach, a name matching no session falls through to the directory chain |
| cli-verb-surface-redesign-1-2 | Directory chain (path → alias → zoxide) mint outcomes + total-miss hard-fail | bare project name (`api`) mints and never reattaches an `api-*` session, alias resolving to a non-existent dir errors, zoxide-not-installed / no-match falls through silently to miss, miss message names the raw target (`nothing resolved for 'blog' — try -f blog`), existing command→mint threading preserved (attach+command formalization is Phase 2) |
| cli-verb-surface-redesign-1-3 | Glob pre-check → session-domain expansion + zero-match hard-fail | glob matching only `_`-prefixed internal sessions counts as zero, a path whose name contains glob metacharacters (`foo[1]`) is unreachable as a bare positional (zero-match hard-fail), glob skips the chain even when a same-named alias/dir exists, multi-match expansion into a burst deferred to Phase 3 |
| cli-verb-surface-redesign-1-4 | `resolve` log component — INFO decision line | session hit logs resolved_path = session name, miss logs domain=miss with empty resolved_path, glob targets emit no line (deterministic — gate on the resolver's glob predicate), INFO level so guesses are reconstructable after the fact |
| cli-verb-surface-redesign-1-5 | `-f/--filter` picker redirect + mutual exclusivity | `-f` + positional target → usage error, empty `-f` value, `-f` alone opens the filtered Sessions picker, no-arg `open` still launches the picker unchanged (regression) |

---

## Phase 2: Domain-pinning flags & mint-scoped command passthrough
status: approved
approved_at: 2026-07-18

**Goal**: Add the four explicit domain-pinning flags (`-s`, `-p`, `-z`, `-a`) that skip the guessing chain, enforce the pinned-domain hard-fail contract (never fall back to the picker), and finalise the single-target `-e`/`--` command passthrough as mint-scoped, building on Phase 1's attach-vs-mint outcomes.

**Acceptance Criteria**:
- [ ] `-s/--session <name-or-glob>` attaches (against the user-visible set), never mints, and hard-fails on miss; `-p/--path <dir>` mints and requires the dir to exist; `-a/--alias <key-or-glob>` mints at the aliased dir (accepts key globs), hard-fails on unknown key; `-z/--zoxide <query>` mints at the best match and hard-fails on no match.
- [ ] Pinned `-z` errors explicitly (`ErrZoxideNotInstalled`) when zoxide is absent, distinct from the bare-target chain which silently falls through on any zoxide error.
- [ ] Every pin (`-s`/`-p`/`-z`/`-a`) hard-fails on unresolvable and never pops the TUI picker; only bare positionals run the guessing chain and only `-f` opens the picker.
- [ ] `-a` reaches an alias key shadowed by a same-named session (the pin bypasses precedence).
- [ ] `-e <cmd>` and `<target> -- <cmd>` are two spellings of one command; specifying both is a usage error, and an empty command value is a usage error.
- [ ] A command targeting an attach (existing-session) target is rejected; a command with zero mint targets is a usage error; a command with no target opens the picker restricted to Projects (mint-only) mode with a `Pick a project to run <cmd>` banner (preserved from today), and `-f <text> -e <cmd>` opens a filtered Projects picker.
- [ ] `-f` is mutually exclusive with every pin flag as well as positionals (usage error otherwise).

**Rationale**: Pins are the "extended capability" layer on Phase 1's resolver — they name a target's domain without re-proving resolution, and they are the deterministic, scriptable path a spawned window will later exec. Grouping the single-target command semantics here keeps all of `open`'s flag surface cohesive and independently testable before the multi-target burst introduces per-target command baking.

---

## Phase 3: Multi-target burst (absorb / net-N)
status: approved
approved_at: 2026-07-18

**Goal**: Make `open <t1> … <tN>` open N surfaces continuously in N — the invoking terminal becomes one surface and N−1 host windows spawn — by composing the union of positionals and pins, recovering true target order from `os.Args`, running an atomic read-only pre-flight, and dispatching the leave-what-opened burst (reusing `internal/spawn`) with the hidden `--ack` receipt flag.

**Acceptance Criteria**:
- [ ] The target set is the union of all positionals plus every `-s`/`-p`/`-z`/`-a` occurrence; pins repeat freely and mix with positionals; a raw `os.Args` scan recovers left-to-right order (handling `-s api`, `-s=api`, `--session=api`, excluding `-e <cmd>` and everything after `--`), while cobra remains the source of truth for flag validation.
- [ ] A session-glob target expands to K session targets that join the set; zero matches is an atomic hard fail; a directory path whose name contains glob metacharacters is unreachable as a bare positional and reachable only via `-p`.
- [ ] Pre-flight is a read-only resolve of the whole set: any unresolvable target aborts atomically (nothing opens, nothing minted) and the abort reports *every* unresolvable target; the `-f` suggestion appears only in the single-target miss message.
- [ ] The trigger absorbs the first target in command-line order and connects **last** (after all N−1 spawns are issued); the current session is never special-cased and gets a window only if it appears in the set; duplicates are honoured, never deduped.
- [ ] Each spawned window execs the same `open` grammar a human would: attach targets → `portal open --session <name> --ack <batch>:<token>`; mint targets → `portal open --path <literal-dir> --ack <batch>:<token>` (parent reduces alias/zoxide to a literal dir at resolve time so resolution never re-runs in the window); minting happens in each window, not the parent.
- [ ] A command (`-e`/`--`) rides mint windows only, appended after `--ack` and carried byte-identically (no word-splitting) so the trigger's local mint and every spawned mint window run the same command; attach windows never carry it.
- [ ] The hidden `--ack <batch>:<token>` flag is added to `open` and `MarkHidden` (absent from `--help` and completion); the spawned process writes `@portal-spawn-<batch>-<token>` best-effort as its last act before exec (still execs on write failure).
- [ ] Past pre-flight, per-window failure is leave-what-opened with a per-window ~8s ack timeout (timer starting at each window's own spawn); the trigger connects to its own first-target surface independent of other windows' failures and is skipped only if its own target fails at connect; each window's outcome is recorded in `portal.log`.

**Rationale**: The burst is the second axiom (absorb/net-N) made real and the mechanism that makes `spawn` redundant. It depends on Phase 1 (read-only resolve underpins the atomic pre-flight) and Phase 2 (spawned windows exec the `--session`/`--path` pins). Keeping it a distinct phase lets the burst be validated end-to-end while `attach`/`spawn` still exist as a safety net, deferring their removal to a clean checkpoint.

---

## Phase 4: `doctor` & `uninstall` — maintenance surface reshuffle
status: approved
approved_at: 2026-07-18

**Goal**: Introduce the two public maintenance verbs that replace the hidden/grab-bag surface: `portal doctor [--fix]` (read-only health report plus low-stakes repair, subsuming `state status`, replacing `clean`, folding in `spawn --detect`) and `portal uninstall` (runtime-only, file-touching-none teardown replacing `state cleanup`), both wired bootstrap-exempt; delete `clean`.

**Acceptance Criteria**:
- [ ] `portal doctor` produces a read-only health report over the catalog: daemon alive; global hooks registered without duplicates; `_portal-saver` up; state dir sane; `sessions.json` valid; no stale entries (dead-pane hooks, gone-dir projects); host-terminal detected/supported.
- [ ] `portal doctor` exits 0 iff every check passes and non-zero (1) if any check reports a problem; a down server is reported honestly as unhealthy → non-zero and distinctly from corruption; the host-terminal check is informational only and never drives the exit code.
- [ ] `portal doctor --fix` performs the reversible-by-reconstruction repairs (prune stale hooks, prune stale projects) and the unconditional log-sweep side-action, then re-runs the diagnosis and exits 0 iff healthy post-repair; the log-sweep is outside the diagnose→repair loop and never affects the exit code.
- [ ] With the server down, `--fix` performs **no** hook pruning (dead-pane enumeration is not evaluable and would falsely orphan every hook), protecting user-authored on-resume commands; the filesystem-only stale-project prune may still run.
- [ ] `doctor` prints a host-terminal line (e.g. `host terminal: Ghostty (supported)` / `unsupported (remote session)`) by calling the same `Detect()` the picker uses, replacing the retired `spawn --detect`.
- [ ] `portal uninstall` removes only Portal's tmux-server footprint (kills `_portal-saver`, unregisters global hooks), touches no state-dir or config files, leaves all sessions running (including `_portal-bootstrap`), is an idempotent graceful no-op on already-clean state, and prints the completion/recovery path message; no `--yes` gate or prompt exists.
- [ ] `doctor` and `uninstall` are added to `skipTmuxCheck` (bootstrap-exempt) so `doctor` observes raw state without healing its own subject and `uninstall` does not bootstrap-then-teardown; `clean` is removed from the codebase and the exempt set.

**Rationale**: This reshuffle is independent of the `open` burst but is sequenced before retiring `spawn` so that `doctor`'s host-terminal line is a ready replacement the moment `spawn --detect` is deleted — no user-facing capability is ever absent. `doctor`'s bootstrap-exemption is intrinsic to its correctness (a non-exempt read-only check would heal itself and always read green), so the exemption belongs in the same phase that creates it.

---

## Phase 5: Retire `attach` & `spawn`
status: approved
approved_at: 2026-07-18

**Goal**: Delete the two public session verbs `open` now fully absorbs — `attach` (covered by `open --session` and the burst's `open --session --ack`) and `spawn` (covered by the multi-target burst; `--detect` now lives in `doctor`) — with no back-compat aliases, realising the redesign's headline collapse to a single public session verb.

**Acceptance Criteria**:
- [ ] `portal attach` is deleted outright (not aliased, not deprecated): the command and its `--spawn-ack` flag are removed, its tests are removed or migrated to `open --session`, and no code references the deleted command.
- [ ] `portal spawn` (including `--detect` and its burst body) is deleted outright; the shared `internal/spawn` service is retained and reached only through `open`'s burst; its still-relevant tests are migrated to the `open` burst.
- [ ] Neither verb appears in `--help` or completion; bare `portal` help lists neither; no compatibility alias or deprecation warning exists for either (the deliberate no-back-compat posture).
- [ ] Every former `attach`/`spawn` behaviour is reachable via `open`: exact/no-guess attach via `open --session <name>`, the spawned-window exec target `portal open --session <name> --ack <batch>:<token>`, and multi-window opening via multi-target `open`.
- [ ] The bootstrap fast-path remains command-agnostic — `open` takes the same abridged latch-satisfied path `attach` did — verified by a regression check that no bootstrap behaviour was lost with `attach`'s removal.

**Rationale**: Retirement is the completion of the `open` arc and depends on both the burst (Phase 3, so spawned windows already exec `open --session --ack`) and `doctor` (Phase 4, so `--detect`'s replacement exists). Kept as its own checkpoint so the new surface is proven end-to-end before the old safety-net verbs are removed, and so the deletion's acceptance (surface shrinks, behaviours preserved, no aliases) is verified in isolation.

---

## Phase 6: Surface presentation — `hook` rename, `state` hiding, tab completion
status: approved
approved_at: 2026-07-18

**Goal**: Finalise the public surface's presentation: rename `hooks` → `hook` (with a permanent silent `hooks` alias), fully hide the `state` namespace while keeping it argv-invocable, add tab completion for Portal-owned enumerable namespaces, and confirm bare `portal` is the help/management root.

**Acceptance Criteria**:
- [ ] `portal hook {set,rm,list}` is canonical and documented; `portal hooks …` keeps working as a silent cobra alias (the one deliberate back-compat carve-out); `skipTmuxCheck` covers the renamed command (it keys on cobra's canonical name, so the alias is covered).
- [ ] The `state` namespace and every remaining child (`daemon`, `hydrate`, `signal-hydrate`, `notify`, `commit-now`, `migrate-rename`) are marked hidden (gone from `--help` and completion) but remain fully invocable by argv; the `state` prefix is preserved so hook-definition substring matching is unchanged, and hook firing / hydrate still work.
- [ ] Tab completion completes session names (from the user-visible set) for the `open` bare positional, `open -s`, and `kill`; alias keys for `open -a`; and delegates `open -p` / `open -z` to the shell (no Portal completion crammed there).
- [ ] Bare `portal` prints help/usage and does **not** launch the picker (control-plane root), leaving `x` / `portal open` as the only picker doors.

**Rationale**: These items depend on the finalised command and flag shapes from Phases 1–5 (completion targets `open`'s pins and the retired-verb-free surface; the `state`-hide and `hook`-rename operate on the post-reshuffle command tree), so they land last as the feature's refinement stage. They share a single theme — how the surface names, hides, and completes itself — making a cohesive closing phase rather than scattered one-off changes.
