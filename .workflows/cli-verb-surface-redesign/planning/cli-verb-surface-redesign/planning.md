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

#### Tasks
status: approved
approved_at: 2026-07-18

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| cli-verb-surface-redesign-2-1 | `-s/--session` pin — session-domain attach, never mints, hard-fails on miss | `-s` matching only a `_`-prefixed internal session (`_portal-saver`) is a miss (user-visible `ListSessions` view) and hard-fails; `-s` never falls back to the picker on miss; `-s` bypasses the guessing chain (never tries path/alias/zoxide even when a same-named dir/alias exists); session glob under `-s` expanding to >1 session → burst deferred to Phase 3 (single-match attaches); inside-tmux switch-client vs outside exec attach; empty session set → hard-fail |
| cli-verb-surface-redesign-2-2 | `-p/--path` pin — path-domain mint, dir must exist | a dir whose name contains glob metacharacters (`~/tmp/foo[1]`) is reachable via `-p` (pin bypasses the Phase-1 glob pre-check); non-existent dir hard-fails (DirNotFound), never pops the picker; tilde/relative-path expansion reused from `ResolvePath`; `-p` never runs session/alias/zoxide matching (path-domain only) |
| cli-verb-surface-redesign-2-3 | `-a/--alias` pin — alias-domain mint, key globs, shadow bypass, hard-fails on unknown key | alias key shadowed by a same-named session — `-a` mints at the aliased dir (bypasses session→path→alias precedence); unknown key hard-fails, never pops the picker; key glob (`-a 'workflow-*'`) single-match mints, multi-match → burst deferred to Phase 3; glob matches over the finite Portal-owned key namespace (enumerate via alias `List`); aliased dir no longer on disk → error |
| cli-verb-surface-redesign-2-4 | `-z/--zoxide` pin — zoxide-domain mint, explicit not-installed error, hard-fails on no match | zoxide not installed → explicit `ErrZoxideNotInstalled` (distinct from the bare chain's silent fall-through); no match → hard-fail, never pops the picker; resolved dir validated to exist before mint; `-z` never runs session/path/alias matching (zoxide-domain only) |
| cli-verb-surface-redesign-2-5 | `-f/--filter` mutual exclusivity extended to all pin flags | `-f` + `-s`/`-p`/`-z`/`-a` (each) → usage error; `-f` + positional already rejected in Phase 1 (regression); `-f` + `-e`/`--` command NOT an exclusivity violation (allowed — routed to the command-picker task); `-f` alone still opens the picker (regression); multiple pins alongside `-f` still error |
| cli-verb-surface-redesign-2-6 | Command (`-e`/`--`) is mint-scoped — reject on attach targets | command + a session-resolving target (attach) → usage error, for a bare session name or a `-s` pin; single-target command + zero mint targets → usage error (multi-target zero-mint deferred to Phase 3); both `-e` and `--` → usage error (preserved); empty command value (`-e ""` / `--` with no args) → usage error (preserved); command + mint target threads into the mint (Phase-1 wiring, regression) |
| cli-verb-surface-redesign-2-7 | Command with no target → Projects (mint-only) picker with banner | `open -e <cmd>` / `open -- <cmd>` (no target) → picker in Projects mode with `Pick a project to run <cmd>` banner (preserved, not a usage error); `-f <text> -e <cmd>` → filtered Projects picker (distinct from `-f` alone → Sessions page); banner wording exactly `Pick a project to run <cmd>`; Projects-only mode always mints a fresh session |

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

#### Tasks
status: approved
approved_at: 2026-07-18

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| cli-verb-surface-redesign-3-1 | `--ack` receiver flag on `open` (hidden) + best-effort marker write before handoff | malformed `<batch>:<token>` → usage error (exit 2) before touching tmux; write failure still connects/execs (false negative, no orphan); rides both `--session` attach and `--path` mint receivers, written as the last act before the handoff; hidden from `--help`/completion (MarkHidden) but visible in `ps`; still-present `attach --spawn-ack` left untouched until Phase 5 |
| cli-verb-surface-redesign-3-2 | Raw `os.Args` scan → ordered domain-tagged target-set union (no dedup) | `-s api` / `-s=api` / `--session=api` value attributed to the pin never a positional; `-e <cmd>` value and everything after `--` excluded; pins repeat and interleave with positionals, order preserved, no dedup; value pins never bundled; cobra stays the flag-validation source of truth; single-target arity yields a one-element list |
| cli-verb-surface-redesign-3-3 | Read-only resolve/classify engine → ordered attach/mint surfaces + glob/alias-glob K-expansion + literal-dir reduction | bare runs the Phase-1 chain, pins skip to their Phase-2 domain; session glob (bare or `-s`) expands to K user-visible session surfaces joining in place, zero-match = miss; `-a` key glob expands to K alias mints; overlapping globs may duplicate surfaces (honored, never deduped); mint targets reduced to a literal existing dir so the window never re-resolves; glob-metacharacter dir path unreachable as bare, reachable only via `-p`; strictly read-only (no mint, no tmux mutation) |
| cli-verb-surface-redesign-3-4 | Atomic aggregated pre-flight abort (report every miss; single-target `-f` carve-out) | multiple misses all reported (one re-run fixes all); any single miss in a mixed set aborts atomically (nothing opens/mints); single-target miss keeps `-f` suggestion (Phase 1), multi-target miss omits `-f`; zero-match glob counts as a miss; N=1 all-hit falls through to the single-target connect not the burst |
| cli-verb-surface-redesign-3-5 | Spawned-window `open`-grammar argv composition (attach `--session` / mint `--path <literal-dir>`) + Burster surface-spec input | attach → `env -u TMUX -u TMUX_PANE PATH=… <exe> open --session <name> --ack …`; mint → `… open --path <literal-dir> --ack …` (reduced literal dir, never alias/zoxide); name/dir with spaces one argv element, `--ack` value two discrete elements; TMUX/TMUX_PANE stripped, `os.Executable()` keeps the warm-latch satisfied; minting at window exec time not the parent (no pre-mint → no orphan); legacy `spawn` CLI + picker burst stay green (all-attach windows converge onto `open --session --ack`) |
| cli-verb-surface-redesign-3-6 | Net-N dispatch — trigger absorbs first, spawns N−1 external, connects last | trigger = first in command-line order, N−1 spawned first, trigger self-connects LAST (load-bearing outside tmux); first-element split distinct from legacy trailing-trigger `SplitNetN`; current session never special-cased (window only if in set; first → no-op switch; elsewhere → moves + own window; absent → left detached); duplicates honored never deduped; inside/outside split only selects the trigger's connector; N=1 degenerates to a plain single connect |
| cli-verb-surface-redesign-3-7 | Command rides mint windows only, byte-identical (+ trigger local-mint parity + multi-target zero-mint usage error) | appended after `--ack` on MINT windows only, attach windows never carry it; mixed set attaches bare + mints running the command; byte-identical, no word-splitting (`-e "npm run dev"` one unit) so local + spawned mints run identical commands; trigger that is a command-carrying mint mints locally via `CreateFromDir`/`QuickStart`; multi-target zero-mint + command (all-attach) → usage error; `-e`/`--` exclusivity + empty-command usage errors preserved from Phase 2 |
| cli-verb-surface-redesign-3-8 | Leave-what-opened partial failure + per-window ~8s ack timeout + `portal.log` outcomes | opened windows stay (no teardown), failed/un-acked surfaces don't auto-retry; per-window ~8s timeout timed from each window's own spawn; trigger connects independent of other windows' failures, skipped only if its own target fails at connect (outside tmux returns to the shell); each outcome in `portal.log`, stderr summary swallowed on attach (log is the durable surface); permission-required stops the burst surfaced once; batch markers cleaned on every terminal path before self-connect |

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

#### Tasks
status: approved
approved_at: 2026-07-18

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| cli-verb-surface-redesign-4-1 | `doctor` command + read-only diagnosis framework (state-package checks) | fresh install / no state dir yet reported honestly (not a crash); missing or corrupt `sessions.json` reads as invalid via `ReadIndex` skip/err (`HasLastSave=false`); dead `daemon.pid` → daemon-alive fails → non-zero; strictly read-only (no tmux or file mutation); bootstrap-exempt so it observes raw state and heals nothing; report always carries at least one check |
| cli-verb-surface-redesign-4-2 | Runtime tmux checks (saver up, hooks no duplicates) + distinct down-server report | server down → saver/hooks/daemon fail with the distinct "Portal runtime not running — run `portal open` to start" message not corruption; ≥2 Portal-fingerprint entries on any managed event → duplicate → fail (per-event `ShowGlobalHooksForEvent` + `managedEvents` fingerprints, not the tmux-3.6b-blind no-arg `show-hooks`); `_portal-bootstrap` present but saver absent still fails the saver check; transient tmux read reported honestly not as a crash |
| cli-verb-surface-redesign-4-3 | Read-only stale-entry checks — dead-pane hooks + gone-dir projects | server down → dead-pane-hook staleness reported not-evaluable (never "all stale", never a false failure); zero live panes with hooks present is exactly the not-evaluable case (mirrors `runHookStaleCleanup` hazard guard); genuine stale hook/project → non-zero; gone-dir detection `os.Stat`-based (permission-denied path retained, not counted stale); strictly read-only (no pruning here — that is `--fix`) |
| cli-verb-surface-redesign-4-4 | Host-terminal informational line (`Detect()` + resolver) | NULL/remote/mosh identity → "unsupported (remote session)"; recognised-but-undriven terminal → "unsupported"; transient detect failure folds to informational; supported = resolver `Resolution != Unsupported`; the line is outside the pass/fail set and never drives the exit code; reuses the same `Detect()`/`Resolve` seams the burst uses |
| cli-verb-surface-redesign-4-5 | `doctor --fix` — repairs + unconditional log-sweep side-action + re-diagnose | server down → NO hook pruning (reuse `runHookStaleCleanup` hazard guard; protects user-authored on-resume commands); filesystem-only stale-project prune still runs on a down server; log-sweep outside the diagnose→repair loop, never touches the exit code (no "logs" catalog check); re-diagnosis exits non-zero if anything remains unhealthy/unfixable; repairs reuse `runHookStaleCleanup` + `project` CleanStale + `log.SweepLogsForClean`; stays bootstrap-exempt (starts nothing) |
| cli-verb-surface-redesign-4-6 | `portal uninstall` — runtime-only teardown (+ delete `state cleanup`) | server down → graceful no-op (skip kill/unregister) still prints the completion message; saver already absent / no hooks registered → idempotent success; leaves all user sessions AND the load-bearing `_portal-bootstrap` anchor running; touches no state-dir or config files (fully recoverable — `open` re-bootstraps); no `--yes` gate or prompt; kill-before-unregister ordering preserved for the daemon SIGHUP flush; relocate `killSaver`/`isSessionAbsentError` out of the deleted `state_cleanup.go` |
| cli-verb-surface-redesign-4-7 | Delete `clean` + `state status`, relocate housed helpers, drop `clean` from exempt set | relocate `loadProjectStore`/`projectsFilePath`/`loadPrefsStore`/`prefsFilePath` (still used by open/TUI/doctor) and `AllPaneLister` (still consumed by `runHookStaleCleanup` + the daemon) rather than delete them; `internal/state.CollectStatus`/`StatusReport` survive (doctor reuses them); remove clean-only `cleanStaleHooks`/`cleanRotatedLogs`/`CleanDeps`/`buildCleanPaneLister` and status-only `ErrStatusUnhealthy`/render helpers; no dangling references to `cleanCmd`/`stateStatusCmd`; the daemon's throttled hook cleanup (`runHookStaleCleanup`'s sole remaining caller) still compiles + green; update `run_hook_stale_cleanup.go` doc comment naming `clean.go` |
| cli-verb-surface-redesign-4-8 | Fold stale-project pruning into the daemon's throttled automation | filesystem-only classification (gone-dir stale, permission-denied retained) mirroring `project.Store.CleanStale`; slow (hourly-ish) throttled cadence like the existing hook cleanup, not per-tick; best-effort / non-fatal to the capture loop; runs only inside the live `_portal-saver` pane so the down-server false-orphan hazard does not apply; no new log component (closed taxonomy) |

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

#### Tasks
status: approved
approved_at: 2026-07-18

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| cli-verb-surface-redesign-5-1 | Retire `attach` — delete the command and migrate its behaviours to `open --session` | `SessionValidator` relocated (still consumed by `kill`), not deleted; `mockSessionValidator` relocated to a surviving `_test.go` (used by kill/version_guard/abridged/reattach); abridged latch-satisfied fast-path stays command-agnostic — `abridged_route_test` migrates attach→`open --session` (AC #5 regression, no bootstrap behaviour lost); inside-tmux switch-client vs outside exec-attach reattach cases migrate to `open --session`; session-not-found hard-fail (never mints, never picker) preserved; `--spawn-ack` removed, its role now `open --ack` (Phase 3); `internal/spawn` `AckWriter`/`NewServerOptionAckChannel`/`ParseSpawnAckFlag` retained; drop the stale attach argv→role case in `internal/log` `ResolveProcessRole` + its test; no compat alias / no deprecation warning; version_guard/root/root_integration attach cases migrated or removed + `attachCmd` flag-reset removed; picker keymap "attach" action label is UI copy, untouched |
| cli-verb-surface-redesign-5-2 | Retire `spawn` — delete the command, `--detect`, and burst body; relocate the shared host-terminal seams | `internal/spawn` service + the `spawn` log component + `@portal-spawn-*` markers retained (out of scope), reached only via `open`'s burst; relocate `TerminalDetector`/`productionSpawnSeams`/`buildProductionSpawnSeams`/`buildResolver` to a surviving non-command home (consumed by `openTUI` + `doctor`); delete spawn-only `spawnDetector`/`SpawnDeps`/`buildSpawnDeps`/`runSpawn`/`unsupportedSpawnMessage`/`logSpawn*`/cmd `spawnLogger` var; `--detect`'s replacement is `doctor`'s host-terminal line (Phase 4); picker burst already execs `open --session --ack` (Phase 3), never `spawn`; `spawn_seams_test` split (keep `TestBuildProductionSpawnSeams`, delete `TestBuildSpawnDeps_*`); relocate `fakeTerminalDetector` out of the deleted `spawn_test.go` (still used by open_spawn_detect + doctor tests); delete `spawn_test.go` (burst/pre-flight/permission/logging coverage now proven via the Phase-3 open burst + Phase-4 doctor); `productionSpawnSeams.Logger = log.For("spawn")` retained; `SplitNetN` retained (picker consumes it); root/root_integration spawn cases migrated or removed + `spawnCmd` flag-reset removed; `internal/spawn` otherwise untouched; no compat alias / no deprecation warning |
| cli-verb-surface-redesign-5-3 | Retired-surface & reachability guard | `rootCmd` exposes no attach/spawn child and no cobra alias resolving to either; neither appears in `--help`, generated completion, or bare `portal` help; no deprecation warning and no back-compat alias for either (deliberate no-back-compat posture — contrast the `hooks`→`hook` alias carve-out, which is Phase 6, not here); reachability: exact/no-guess attach via `open --session <name>`, spawned-window exec target `portal open --session <name> --ack <batch>:<token>`, multi-window via multi-target `open`; state-hide / hook rename / tab-completion additions are Phase 6 (this guard asserts only the two verbs' absence, not the finalized completion surface); x/xctl shell functions (map to `portal open`) untouched and still work |

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

#### Tasks
status: approved
approved_at: 2026-07-18

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| cli-verb-surface-redesign-6-1 | Rename `hooks` → `hook` (canonical) + permanent silent `hooks` cobra alias + skipTmuxCheck repoint | `hooks` alias silent (no deprecation warning; cobra lists it only on the Aliases line); machine-generated `portal hooks set …` (external Claude SessionStart skill) still works via the alias; `skipTmuxCheck` keys on canonical `c.Name()`="hook" so the `hooks` alias is bootstrap-exempt too; set/rm/list reachable under both names; update the `skipTmuxCheck` doc comment that names hooks/clean |
| cli-verb-surface-redesign-6-2 | Fully hide the `state` namespace (parent hidden, children argv-invocable) | parent `stateCmd` gains `Hidden:true` — the six children (daemon/hydrate/signal-hydrate/notify/commit-now/migrate-rename) are already `Hidden:true` (assert each); all six stay fully invocable by argv (`Hidden != disabled`) so hook firing / hydrate still work; `state` prefix preserved (no rename — `notifyCommand`/`commitNowSubstring`/`migrateRenameSubstring`/`PortalDaemonArgvPattern` substring matching unchanged); state stays in `skipTmuxCheck`; gone from `--help` top-level listing and generated completion; precondition: status/cleanup deleted in Phase 4 so zero user-facing children remain |
| cli-verb-surface-redesign-6-3 | Session-name completion (shared helper) for `open` positional, `open -s`, `kill` + exempt completion from bootstrap | completion (`__complete`) must NOT trigger bootstrap — cobra runs the root `PersistentPreRunE` for the `__complete` command, which would start the server + restore sessions on a TAB press — add `__complete` to `skipTmuxCheck`; the completer builds its OWN client via `tmux.DefaultClient` (`tmuxClient(cmd)` panics without `PersistentPreRunE`); server-down / no sessions → empty completions gracefully (`ListSessions` collapses no-server to empty); user-visible set only (leading-`_` filtered — `_portal-saver`/`_portal-bootstrap` never suggested); `NoFileComp` so sessions aren't merged with file/dir completion (rejected noisy merge); hidden `--ack`/`state` never appear in completion |
| cli-verb-surface-redesign-6-4 | Alias-key completion for `open -a`; `open -p` / `open -z` delegated to the shell | alias keys via `alias.Store.Keys()` (Phase 2) loaded from the config path (no tmux client needed); empty/missing aliases file → no suggestions gracefully; `NoFileComp` (finite Portal-owned namespace, no file merge); `-p` and `-z` register NO Portal completion func so they fall to shell defaults (`-p` → file completion, `-z` → shell/zoxide's own); inherits the completion bootstrap-exemption from task 6-3 |
| cli-verb-surface-redesign-6-5 | Bare `portal` prints help/usage and does not launch the picker (control-plane root guard) | `rootCmd` has no `Run`/`RunE` so cobra returns `ErrHelp` before `PersistentPreRunE` — no bootstrap, no picker (verify); bare `portal` exits without launching the TUI; `x`/`portal open` remain the only picker doors while `xctl`/bare `portal` are the management plane; verification/guard-only — no production change expected (flag if the guard test already passes as-is) |

---

### Phase 7: Analysis (Cycle 1)

Address findings from Analysis (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| cli-verb-surface-redesign-7-1 | Repoint bootstrap warnings from the deleted `portal state status` to `portal doctor` | Both `CorruptSessionsJSONWarning` and `SaverDownWarning` rendered text asserted (contains `portal doctor`, not `portal state status`); comment-only references are out of scope (Task 7 covers those); no live user-facing string may instruct running a removed command |
| cli-verb-surface-redesign-7-2 | Extract a single domain-pin dispatch helper for the four copy-paste arms in openCmd.RunE | Byte-identical behaviour per pin (-s/-p/-a/-z): same resolution, error propagation, and `openResolved` handoff; short-circuit on first changed pin flag preserved; adding a future pin must touch one place (the table), not four |
| cli-verb-surface-redesign-7-3 | Collapse the dual source of truth for open's value-taking flags in the ordered-target argv scan | Two acceptable paths — thread `*cobra.Command`/`*pflag.FlagSet` to derive arity, or add a `VisitAll` drift-guard test; a future value-taking flag's value must not be misclassified as a bare positional target (covered by test); existing left-to-right multi-target ordering unchanged |
| cli-verb-surface-redesign-7-4 | Single-source the single-target "nothing resolved" miss error string | U+2014 em-dash and `-f %s` suffix preserved byte-identical; both the bare-positional (`open.go:344`) and N=1 glob-to-zero burst (`open_burst.go:125`) sites call the one `singleMissError` helper; multi-target `aggregatedMissError` left as-is |
| cli-verb-surface-redesign-7-5 | Extract expandSessionGlobAll to collapse the duplicated session-glob expansion block | Zero-match returns a single `MissResult{Target: pattern}`; non-zero returns K `SessionResult{Domain: "glob"}` — behaviour unchanged; `ResolveAliasPinAll` (validated-path body) left untouched; consumed by both `ResolveBareAll` and `ResolveSessionPinAll` |
| cli-verb-surface-redesign-7-6 | Update CLAUDE.md command-surface prose to the redesigned surface | Docs-only, no automated test; remove all removed-surface refs (`portal spawn`/`clean`, `attach --spawn-ack`, `cmd/spawn.go`, `cmd/clean.go` as `loadPrefsStore` home); add `doctor [--fix]`/`uninstall`/open multi-target burst/hidden `--ack`/hooks→hook; fix bootstrap-exempt list (drop `clean`) |
| cli-verb-surface-redesign-7-7 | Sweep stale removed-surface references in code comments and the process-role doc | Comments/docs only — not the bootstrap warning strings (Task 1) or CLAUDE.md (Task 6); repoint `internal/spawn` "cannot drift" anchors to the two surviving bursts (picker + open); annotate now-single-caller split helpers with their sole consumer; note dead `case "clean"` process-role arm, keep the closed-space value in place |
| cli-verb-surface-redesign-7-8 | Single-source the two governed two-site emissions (resolve-decision log line + exec-handoff marker) | Two governed contracts (`resolve` INFO line + `process:exec` marker) each emitted from exactly one helper; `!HasGlobMeta` gate and attr keys single-sourced; emitted log output byte-identical to current; both call sites route through the helper |

---

### Phase 8: Analysis (Cycle 2)

Address findings from Analysis (Cycle 2).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| cli-verb-surface-redesign-8-1 | Narrow `portal doctor`'s daemon-liveness probe off the over-scoped `CollectStatus` and share one pane counter | Routine `portal doctor` performs no state-dir tree walk and no full portal.log scan; sessions/panes derive from a single `ReadIndex` per invocation; exactly one pane counter (no `doctorPaneCount` duplicate of `state.countPanes`); dead `daemon.pid` → `DaemonRunning=false`; `CollectStatus` trimmed to consumed fields or deleted if doctor was its last production caller; daemon/sessions/panes output behaviourally identical to pre-change |
| cli-verb-surface-redesign-8-2 | Remove the production-dead, divergent session-glob branch from `QueryResolver.Resolve` | Multi-match session glob never collapses to `matches[0]`; glob reaching the resolver expands to all matches or returns an explicit error (never silent first-match); single-target and burst glob expansion share one primitive; confirm callers (cmd/open.go:262, `ResolveBareAll`) never receive glob in the prod routing path; an `os.Args`-assumption break can no longer silently fork glob semantics |
| cli-verb-surface-redesign-8-3 | Refresh stale post-redesign documentation and comments | Comment/doc-only change; process_role.go `roleTUI` mapping and `process_role_test.go` unchanged (comment-only in that file); bare `portal` described as prints help/usage, not TUI picker; CLAUDE.md "Incident of record #2" period-marked or re-anchored so removed `state cleanup` / deleted `TestStateUserFacingSubcommandsExitZero` don't read as current; underlying lesson preserved; no new code test, existing `process_role` tests stay green |

---

### Phase 10: Analysis (Cycle 4)

Address findings from Analysis (Cycle 4).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| cli-verb-surface-redesign-10-1 | Refresh the stale `open` command help text to describe the redesigned verb | Help-metadata only (`Use`/`Short`/`Long`) — do NOT touch `RunE`/`Args`/flag registration/dispatch/resolution; `portal open --help` and bare `portal --help` both name session-name attach, the `-s`/`-p`/`-z`/`-a` pins, `-f`/`--filter`, `-e`/`--` command scoping, and multi-target opening — no single-path `destination` implication; copy stays consistent with the accurate per-flag strings (open.go:976-979) and the spec §405 row; spec dictates no golden string (match intent, not exact copy); guard test asserts `openCmd.Short`/`Long` mentions the redesigned capabilities, and any test asserting the literal `Use`/`Short` strings is updated; full unit+integration suite stays green |

---

### Phase 11: Analysis (Cycle 5)

Address findings from Analysis (Cycle 5).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| cli-verb-surface-redesign-11-1 | Correct doctor's host-terminal seam provenance comments (they claim shared-bundle single-sourcing that does not exist) | Comment-only change; `resolveDoctorDeps`' `Detector`/`Resolve` construction is byte-for-byte unchanged (no behavioral/code change); no comment in cmd/doctor.go implies doctor reads from the shared `buildProductionSpawnSeams` bundle via "the SAME" seam/resolver; comments explicitly state the detector+resolve pair is independently re-constructed in `resolveDoctorDeps` and name the deliberate deferred terminals.json read (behind the lazy `Resolve` closure) as the reason doctor does not adopt the eager bundle, kept in sync by hand; the optional route-through-`buildProductionSpawnSeams` refactor is explicitly NOT required; `go build ./...` succeeds and `golangci-lint run` clean on cmd/doctor.go; no new tests, existing doctor host-terminal suite (cmd/doctor_test.go) stays green |

---

### Phase 12: Analysis (Cycle 6)

Address findings from Analysis (Cycle 6).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| cli-verb-surface-redesign-12-1 | Retarget stale source comments that cite redesign-deleted files (cmd/state_cleanup.go, attach.go) | Comment-only change (no code/signature/behaviour edit in the three files); four sites retargeted — `internal/tmux/hooks_unregister.go:14-15` & `:95-96` and `internal/tmux/tmux.go:382` point at `cmd/uninstall.go` (`buildUninstallDeps`/`killSaver`), `internal/resolver/query.go:307-308` rewords to a house-style/byte-compat justification naming no deleted file; `internal/tmux/portal_saver.go` kept as-is in the `KillSession` caller list; the `//nolint:staticcheck` directive and the `"No session found: %s"` string are byte-for-byte unchanged; `grep -rn "state_cleanup.go\|attach.go" internal/` returns nothing from these sites; no new tests, existing UnregisterPortalHooks (bootstrap/uninstall) / KillSession exact-target / resolver "No session found" miss suites stay green; `go build ./...` succeeds and `golangci-lint run` clean on the three touched files |

---

### Phase 13: Review Remediation (Cycle 1)

Address findings from Review Remediation (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| cli-verb-surface-redesign-13-1 | Sweep the surviving divergent silent-first-match glob branches out of the single-pin resolve paths | severity high; the `os.Args`-assumption-break path (`openOwnArgs()` returns nil) that silently forks a glob-bearing `-s`/`-a` to first-match is the hazard; fix `ResolveSessionPin` (:295-298) and `ResolveAliasPin` (:347-355) symmetrically via the shared all-match primitive OR an explicit loud error; retarget query_test.go:593/:896 which currently enshrine first-match; update the `ResolveSessionPin` doc comment; do NOT reopen the already-correct `QueryResolver.Resolve` change |
| cli-verb-surface-redesign-13-2 | Single-source the domain-pin set so the exclusivity guard cannot miss a future pin | one canonical pin-name list consumed by both `anyOpenDomainPin` (root.go:314-317) and the `pinDispatch` table (open.go:259-272); replace the inline `anyPin` at open.go:208-209 with `anyOpenDomainPin(cmd)`; four-pin exclusivity behaviour unchanged; drift-guard test that every `pinDispatch` key is in the shared list; a future fifth pin added in exactly one place |
| cli-verb-surface-redesign-13-3 | Fix two paths that report success while an operation silently failed | two independent paths — `killSaver` (uninstall.go:136) swap `HasSession`→`HasSessionProbe` so a transient tmux fault joins the error not a false "removed"; open_burst_run.go:222-231 defer `triggerAttached=true` or emit a corrective `spawn` WARN on `connectTrigger` failure so `portal.log` never inflates `opened N/N`; add tests injecting the transient probe fault and forcing trigger-connect failure |
| cli-verb-surface-redesign-13-4 | Doctor — correct count copy and close the test-coverage gaps | pluralise count copy singular when count == 1 (doctor.go:642,438,488) + update asserting test strings; add the two uncovered `checkStateDirSane` fail branches (existing-but-non-directory; unreadable stat, :612-618); add one Execute-level seeded-stale-entry → `ErrDoctorUnhealthy` test |
| cli-verb-surface-redesign-13-5 | Close the test-coverage parity gaps across the redesigned surface | test-only, no production change; seven symmetric sibling gaps — `--` spellings alongside `-e` (open_test.go:2853,:325), `TestOpenCommand_PathPin_Miss_HardFailsNoPicker`, project-prune idle-tick test, engine-level `Domain:"session"` (`-s`) surface, argv-scan reverse-mapping (:82) + mid-list value between positionals (:147), completion excludes `--ack`/`state` |
| cli-verb-surface-redesign-13-6 | Small DRY / legibility cleanups (redundant and misleading code) | mechanical `MatchSessions`→`MatchGlob` rename across glob.go:27 + call sites (independent of Task 13-1 — adapt to whichever call sites remain); single-source `spawnLogger` in `buildProductionSpawnSeams` (spawn_seams.go:59); drop the redundant `store.Load()` in `completionAliasKeys` (completion.go:64-67), return `store.Keys()` directly; behaviour-preserving |

---
