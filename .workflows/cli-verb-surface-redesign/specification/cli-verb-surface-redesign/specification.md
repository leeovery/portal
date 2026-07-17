# Specification: CLI Verb Surface Redesign

## Specification

## Overview

Portal's CLI is redesigned in one intentional pass. Today's surface grew by accretion into overlapping, blurry session verbs (`open`, `attach`, `spawn`) with illegible input domains — the trigger being that even the author can't cleanly recall the difference between `open` and `attach`. This redesign audits the **full** command list (session verbs, utilities, and internal plumbing) against a single governing principle and two axioms.

### Governing principle: split the public surface by outcome, not by input shape

The public surface names *what happens*, not *what the argument looks like*. Input domains (session name, path, alias, zoxide query) are unified inside `open`'s resolution rather than made legible by choosing a different verb. Exactness (no-guessing) is demoted from a public verb to documented flags and hidden plumbing.

Concretely, this collapses today's three public session verbs into a single public verb, `open`. `open` keeps its name on semantic grounds — the portal metaphor ("you are opening a portal to a session") is the tool's founding play on words; the argument changes only how the destination is derived, not what the verb does. The name was kept explicitly **not** on migration-cost grounds.

### The two axioms

**Axiom 1 — absorb / net-N.** `open` opens N portals to N targets; the invoking terminal is one of those N surfaces. This is continuous in N: at N=1 the terminal is the only surface (open-here); at N>1 the terminal becomes one surface and N−1 host-terminal windows are spawned. There is no behavior cliff between single-target and multi-target — the "stay put while multi-opening" behavior is a deferred future flag, not the default.

**Axiom 2 — attach-vs-mint dichotomy.** A resolved target is one of two kinds:
- **Session-domain hit** (exact session name, session glob) → **attach** to that existing session.
- **Directory-domain hit** (path, alias, zoxide query) → **mint a brand-new session** at that directory, always.

There is no find-or-create. Directory targets always create a fresh `{project}-{nanoid}` session even when sessions already exist for that project (multiple sessions per project is the designed workflow). The precedence chain is therefore semantic — "surface an existing session, or open a new portal to a place" — not mere disambiguation.

### Porcelain / plumbing split

Truly-internal entry points stay invocable but hidden rather than public: the `--ack` receipt flag on `open`, and the entire `state` namespace (argv-invoked by tmux hooks and the saver pane). Everything a human is meant to type is public and documented.

### Scope of the redesign

In scope: the public verb surface and tiering (public / hidden), command names, shapes, and the back-compat posture. Out of scope: internal package/component/marker names (`internal/spawn`, the `spawn` log component, `@portal-spawn-*` markers) — these are unaffected by the redesign.

---

## `portal open` — Grammar & Target Resolution

`portal open` is the single public session verb. `x` (the shell function emitted by `portal init`, `x() { portal open "$@" }`) maps to it unchanged.

### Invocation grammar

| Invocation | Behavior |
|---|---|
| `portal open` (no args) | Launch the TUI picker — this is how you choose a destination |
| `portal open <target>` | Resolve the single target and connect this terminal to it |
| `portal open <t1> <t2> … <tN>` | Open N surfaces (absorb/net-N); this terminal becomes one, N−1 host windows spawn |

### Target resolution precedence

A bare positional target is resolved in two steps:

1. **Glob pre-check.** If the target contains glob metacharacters (`*`, `?`, `[…]`), it is **session-domain by construction**: expand it against live session names and skip the chain below entirely (see Glob Targets). Zero matches ⇒ unresolvable ⇒ hard fail.
2. **Otherwise, the precedence chain**, first match wins: **exact session name → path → alias → zoxide query**.

Each domain maps to an outcome per Axiom 2:
- **exact session name** → attach existing session
- **path** (existing directory) → mint new session there
- **alias** (known alias key) → mint at aliased dir
- **zoxide query** → mint at zoxide's best-match dir

Session-name vs directory-name collisions are rare (`{project}-{nanoid}` names don't look like paths) and resolved by precedence.

### Bare project shorthand does not reattach (accepted consequence)

Because directory hits always mint (Axiom 2, no find-or-create), a bare project name like `open api` never exactly-matches a running `api-x7Kd9a` session — it falls through to zoxide/path and mints a **new** session, even while an `api-*` session runs. Reaching an existing session is done via the picker, a session glob (`'api-*'`), or the `-s` pin. Project-prefix session matching (`api` → the sole `api-*` session) is explicitly **rejected** — it reintroduces attach-vs-create guessing with an ambiguity cliff the moment a second `api-*` session exists.

### Miss handling — total miss is a hard fail

**A target that resolves to nothing is a hard failure, at every arity and every form.** Today's terminal step of the resolution chain — a TUI-picker-with-filter fallback — is **removed**. The error message points at the escape hatch, e.g.:

```
nothing resolved for 'blog' — try -f blog
```

The `-f/--filter` flag (see the flags topic) is what makes the filtered-picker mechanic reliable and explicit, replacing the removed implicit fallback.

### Wrong-guess feedback — tmux is the receipt

There is **no dedicated confirmation surface** when resolution guesses wrong (e.g. a wrong zoxide guess silently mints a session). A receipt line has nowhere reliable to live: outside tmux, `open` exec-replaces itself and pre-exec output is swallowed by the alternate screen; inside tmux it lands in the pane you switched away from. What the user reliably sees is tmux itself — the status bar shows the `{project}-{nanoid}` session name (which encodes the resolver's choice) plus the pane cwd. A wrong guess is self-announcing at the destination; recovery is `kill` + retry with a domain-pinning flag.

One observability addition is locked: **`open` logs its resolution decision**, e.g. `resolve: 'blog' → zoxide → ~/Code/blog`, so a confusing guess is reconstructable from `portal.log`. The line is emitted from the `open` command body (`cmd/open.go`), where resolution is driven — `internal/resolver` stays a pure, log-free library.

This requires a **governed amendment to Portal's closed log taxonomy**: this feature adds **one new component, `resolve`**, to the closed component set. `open` owns no log component today (it logs only exec markers under `process` and the spawn burst under `spawn`, neither of which fits a resolution decision), so resolution has no existing home. The `resolve` component carries the decision line with attr keys `target` (raw input), `domain` (session / path / alias / zoxide), and `resolved_path` (resolved directory, or resolved session name for a session hit). This is a spec-recorded amendment, **not** a call-site invention (which the log spec prohibits); planning wires the single `log.For("resolve")` binding in `cmd/open.go`.

---

## `portal open` — Flags & Command Passthrough

### Domain-pinning flags

The domain-pinning flags name a target's domain explicitly, skipping the guessing chain. They exist primarily for scriptability and for reaching a domain shadowed by a higher-precedence match; humans typically use bare targets.

| Flag | Pins to | Semantics on that domain |
|---|---|---|
| `-s/--session <name-or-glob>` | exact session / session glob | attach; hard fail on miss; never mints |
| `-p/--path <dir>` | directory path | mint new session; dir must exist |
| `-z/--zoxide <query>` | zoxide best match | mint at matched dir; hard fail on no match; **explicit error if zoxide not installed** |
| `-a/--alias <key-or-glob>` | alias key / key glob | mint at aliased dir; hard fail on unknown key |
| `-f/--filter <text>` | (none — picker redirect) | opens the picker pre-filled with `<text>`; skips resolution entirely |

Notes:
- `-z` differs from the guessing chain on zoxide-absence: pinned `-z` **errors** when zoxide is not installed (`ErrZoxideNotInstalled`), whereas the bare-target chain treats any zoxide error as "continue to next domain" (falls through silently).
- `-a` is the only way to reach an alias key shadowed by a same-named session, and rounds out the four resolution domains.
- `-e` is already `open`'s run-command flag (see Command Passthrough) — it is not available as a pin letter.

### Pinned-domain contract — never falls back to the picker

**Every domain pin (`-s`, `-p`, `-z`, `-a`) hard-fails on unresolvable and never falls back to the TUI picker** — a spawned window or script must never pop a TUI. `--session` never mints (a bare name has no directory to mint from); `--path` / `--zoxide` / `--alias` mint per Axiom 2 on a hit and hard-fail on a miss. Only bare positionals run the guessing chain; only `-f` opens the picker.

### `-f/--filter` is the sole non-composing flag

`-f` is not a target — it is a "skip resolution, open the picker pre-filtered" redirect. It is **mutually exclusive** with positional targets and with every other pin flag (usage error otherwise).

### Command passthrough (`-e` / `--`) — mint-scoped

`open -e <cmd>` and `open <target> -- <cmd> args…` run a command in newly-created sessions (the "open this project with claude running" mechanism), fed to `CreateFromDir` / `QuickStart` as the pane's initial process.

- **The command targets mint surfaces only.** A freshly-minted session has a clean pane to *be* the command's process. An existing (attach) session has no safe injection channel (see the safety note), so a command can never run in an attach target.
- **Mixed sets are allowed; the command is scoped to the mint targets.** Mint-vs-attach is known per-target at resolve time, so the command is baked only into mint targets' invocations; attach targets get their `--session` with no command. `open api ~/new -e claude` → attach `api` as-is **and** mint `~/new` running claude, in two surfaces.
- **Zero mint targets + a command ⇒ usage error.** `open api web -e claude` (all existing sessions) → error: the command has no new session to run in. (Erroring beats silently dropping the command.)
- **The command runs in every minted target.** `x ~/Code/skill* -- claude` (shell-expanded to N paths) = N new sessions each running claude, in N windows.
- A command with **no target** is not this case — it opens the picker in mint-only (Projects) mode; see the multi-target/picker topic.

**Command-injection-safety note (why attach targets can never take a command).** There is no tmux primitive for "run a command in an existing session only if safe":
1. **mint** — the command *is* the pane's initial process; clean.
2. **existing session at a shell prompt** — only `send-keys` (type the text in); works only if the pane is genuinely idle, which Portal can't guarantee (half-typed input would get the command appended) — a fragile heuristic.
3. **existing session with a process running** (`npm run dev`, `claude`) — no safe option: `send-keys` injects keystrokes into the running process's stdin (garbage), and `respawn-pane -k` *kills* the running process and replaces it (destroys work).

This absence is the deeper reason commands are mint-only — a safety floor, not a chosen restriction. Detecting case 2 via `pane_current_command` + conditional `send-keys` is **rejected** (fragile; makes `open` mutate live sessions — a surprising new power; thin payoff).

### Hidden `--ack` flag

`open --ack <batch>:<token>` is an internal receipt flag used by spawned host windows, **marked hidden via Cobra `MarkHidden`** (gone from `--help` and completion). It remains visible in `ps` when a spawned window runs — acceptable (internal, not secret). Its behavior: the spawned Portal process, as its last act before exec'ing into tmux, writes `@portal-spawn-<batch>-<token>` as a tmux server option — a delivery receipt the parent burst polls for. Full burst mechanics are in the multi-target topic. Rejected spellings: `--on-open` (reads as a hook trigger, collides with `--on-resume` vocabulary), `--open-ack` (redundant on `open`), `--receipt` (unusual CLI vocabulary). Today's equivalent flag `--spawn-ack` is only *labelled* "internal:" in help text, not actually hidden — the redesign hides it properly and renames it `--ack`.

---

## `portal open` — Multi-Target Burst Mechanics

### Target-set composition

**The target set is the union of (all positionals + every `-s`/`-p`/`-z`/`-a` occurrence).** Each element resolves by its own rule — bare positionals run the precedence chain, pins skip the chain and pin their domain — then the whole union goes through atomic pre-flight + absorb/net-N.

- Pins **repeat freely** (`open -s a -s b` = two attach targets).
- Pins **mix across domains and with positionals** (`open -s api -p ~/Code/new blog` = attach `api` + mint at `~/Code/new` + resolve `blog` = three surfaces).
- Pins are the explicit-domain way to name a target, fully interchangeable with positionals in a burst.
- `-f` is the sole non-composing flag (picker redirect; exclusive with all targets and pins).

### Glob targets

- **A bare target containing glob metacharacters (`*`, `?`, `[…]`) is session-domain by construction** — patterns match against the finite set of live session names and skip path/alias/zoxide entirely. Expansion produces K targets that join the target list (`open 'agentic-workflows-*' blog` → K+1 surfaces; absorb rule unchanged). Glob, not regex.
- **Zero matches ⇒ unresolvable ⇒ atomic hard fail** — no special case.
- **Shell-quoting caveat (accepted, documented):** unquoted `*` is expanded by the shell against cwd files first, so session globs are typed quoted (`x 'api-*'`). Same wart as git/docker pattern args.
- **Path globs are already free via the shell:** *unquoted* `x ~/Code/skill*` is expanded by the shell into N path args before Portal sees them → N minted sessions in N windows, zero Portal code. The quote is the domain switch.
- **`-a` accepts key globs** (alias keys are a finite Portal-owned namespace: `-a 'workflow-*'`).
- **Zoxide has no glob support** (subsequence/frecency scoring). Multi-match zoxide (mint sessions for everything frecency-matching a term) is **deferred** — shotgun risk; not designed now.

### The trigger absorbs the first target, unconditionally; no dedup

**The trigger (invoking terminal) takes the first target in command-line order** (left-to-right as typed — positionals and pins interleaved; the implementation reads `os.Args` rather than cobra's split positional/flag buckets to preserve true order), and every remaining target opens a window.

- If the current session happens to be the first target → a no-op switch (you stay put).
- If the current session is **elsewhere in the set** (a non-first target) → the terminal moves to the first target, and the current session gets its own window *because it is a target*, like any other.
- If the current session is **absent from the set** (not requested) → the terminal moves to the first target, and the current session is simply left as a detached session with no surface. It is **not** given a window (it was never a target).
- **No current-session detection, no special-casing** — the current session is never treated specially; it gets a window only when it appears in the target set. The trigger's landing spot is immaterial: "it doesn't matter where the terminal ends up, as long as they all open." All requested surfaces open.
- The inside/outside-tmux split only selects the connector for the first-target surface (`switch-client` inside, `exec attach` outside); the rest run the spawned `portal open …`.

**No dedup — duplicates are honored as intent.** The target set is taken literally; repeated targets are *not* collapsed.
- **Duplicate attach targets** → tmux natively supports multiple clients attached to one session (they mirror), so `open api api api` = three host windows all showing `api` (same session across three Spaces/monitors).
- **Duplicate mint targets** → each mints a *distinct* new session anyway (fresh `{project}-{nanoid}`), so `open ~/a ~/a` = two new sessions at `~/a`.
- **Accepted consequence:** overlapping globs (`open 'api-*' 'api-1'`) can produce a duplicate surface; honored, not deduped (low-harm, killable).

### Argv parsing contract (target ordering)

Cobra remains the source of truth for flag validation, value binding, `-f` mutual exclusion, and rejecting unknown flags. Target *ordering* is recovered by a raw `os.Args` scan layered on top, under a fixed contract:

- Both value forms are recognized for each pin — `-s api` (space) and `-s=api` / `--session=api` (equals) — and the value token is attributed to that pin, never counted as a positional target.
- `-e <cmd>` and its value are not targets and are excluded from the ordered target list (`open -e claude ~/new` → sole target `~/new`; `claude` is the command).
- `--` terminates flag/target parsing; every token after `--` is command-passthrough args, never a target.
- Value-taking pins are written separately, each with its own value — no bundled `-sf`-style combining for value pins.
- The ordered target list is the sequence of positionals and pin-values in the exact left-to-right order they appear in `os.Args`; the trigger absorbs the first element of that list.

The raw scan only recovers order — it classifies each token by the same flag set cobra knows, so the two never disagree.

### Burst exec-argv & mint responsibility

Each spawned window runs the **same `open` grammar a human would** — one pinned target + the hidden `--ack` — no bespoke burst-only path.

1. **Window argv, per surface:**
   - Attach target (session / glob / `-s`) → `portal open --session <name> --ack <batch>:<token>`.
   - Mint target (path / alias / zoxide / `-p` / `-z` / `-a`) → the parent **reduces it to a literal existing directory at resolve time**, then bakes `portal open --path <literal-dir> --ack <batch>:<token>`. Alias/zoxide queries never travel to the window (they could re-resolve differently mid-burst); only the resolved literal dir does, and `--path` cannot diverge. This is why "resolution must not re-run inside the window" holds without a session existing yet.
2. **Minting happens in each window, not the parent — no pre-minting.** The atomic guarantee is precisely the **read-only resolve**: any target unresolvable ⇒ nothing opens, nothing created. Once resolve passes, each surface opens/mints itself at exec time under **leave-what-opened**; a window that never comes up never mints, so there are no orphaned detached sessions.
3. **Command passthrough rides mint windows only.** When a command is present (`-e`/`--`), it is appended to each **mint** window's argv in the multi-token passthrough form (which subsumes the single-string `-e` form), after `--ack`: `portal open --path <literal-dir> --ack <batch>:<token> -- <cmd> args…`. Attach windows never carry the command. When the **trigger** surface is itself a mint target carrying the command, the trigger mints locally (no spawned window) and feeds the command to `CreateFromDir` / `QuickStart` as the pane's initial process — the same path a spawned mint window takes.
4. **No dedup** — duplicate targets each get their own window (mirrored attach, or distinct mint); the burst never collapses them.

### Atomic pre-flight & partial failure

- **Pre-flight is a read-only resolve of the whole target set.** Any target unresolvable ⇒ atomic abort: nothing opens, nothing is created.
- **Past the resolve, per-window failure is leave-what-opened.** Opened windows stay (Portal doesn't own/tear-down host windows), the trigger's self-attach is skipped on failure, and failed/un-acked surfaces don't retry automatically.
- **Per-window ack timeout (~8s).** The parent polls for each window's `@portal-spawn-<batch>-<token>` receipt with a per-window timeout of ~8s, the timer starting at *that window's own spawn* so cumulative sequential delay never eats a later window's budget. A window whose receipt has not appeared by its timeout is the "un-acked / failed" case above.

### Mint-only command with no target → picker in Projects mode

**`open -e <cmd>` / `open -- <cmd>` with no target opens the picker restricted to Projects (mint-only) mode**, with a `Pick a project to run <cmd>` banner. This is preserved exactly from today's behavior — **not** a usage error.

- A pending command switches the picker into Projects mode, and Projects only ever mint a fresh session — so the command always lands in a clean session. No incoherence.
- The command doesn't suppress the picker; it **specializes** it to exactly the surfaces where a command is meaningful (mint), and the banner tells the user what's pending.
- `-f <text> -e <cmd>` likewise coheres (filtered Projects picker running the command). The command's only *error* case is zero mint targets (all-attach explicit set, e.g. `open api web -e cmd`).

---

## Tab Completion

Principle: **complete every Portal-owned enumerable namespace; leave the rest to the shell.** Session names and alias keys are finite sets only Portal knows; zoxide has its own `cd`-style completion, and path completion is the shell's job. This keeps completion pointed at Portal's own namespaces without cramming multiple into one noisy list.

| Slot | Completes |
|---|---|
| `open` bare positional | session names |
| `open -s` | session names |
| `open -a` | alias keys |
| `open -p` | (shell — paths) |
| `open -z` | (shell / zoxide's own) |
| `kill` positional | session names |

Rejected: sessions+directories merged into one slot (noisy); nothing at all (loses the genuinely useful session-name / alias-key completion).

---

## `attach` — Retired

`portal attach` is **deleted outright** — not aliased, not deprecated-with-warning. Every current `attach` invocation has an `open` equivalent (`open` accepts session names; the exact/no-guessing path is `open --session`).

- `attach`'s two former jobs are absorbed: (1) exact/no-guessing attach for scripts → `open --session <name>`; (2) the exec target of every spawned host window → `portal open --session <name> --ack <batch>:<token>`.
- **Both `open` and the former `attach` already call the same internal Go functions in-process** (`connect()` = exec `tmux attach-session` outside tmux / `switch-client` inside); the command form existed only for cross-process callers. Nothing is lost by deleting the public command.
- **The bootstrap fast-path is command-agnostic** — `BootstrappedLatchSatisfied` is consulted once in `PersistentPreRunE` for any bootstrap-needing command (`open` included), gated on the `@portal-bootstrapped` version-stamped latch. So `open` takes the same abridged fast-path `attach` did; there is no bootstrap reason to keep `attach`.

### Spawned-window contract (pinned `open`)

- Spawned host windows exec `portal open --session <name> --ack <batch>:<token>`.
- **Pinned-domain hard-fail:** `--session`/`--path` never fall back to the TUI picker (a spawned window or script must not pop a TUI). `--session` never mints; `--path` mints per Axiom 2.
- **Burst determinism preserved:** a session that vanished mid-burst ⇒ pinned `open` hard-fails ⇒ no ack written ⇒ the burst classifies that window failed, exactly as today.

---

## `kill` — Single + Exact (unchanged)

`portal kill <name>` stays **single + exact** — no globs, no resolution chain, unchanged from today. Instant kill of one named session. Destruction is kept maximally explicit.

- **Universal resolution does not apply to `kill`** — it takes session names only (its natural domain). A guessing chain on a destructive verb is backwards.
- Rejected: session globs on `kill` (`kill 'agentic-workflows-*'`); a terminal `[y/N]` confirm guard.
- **The CLI has no interactive-prompt machinery** — verified: no stdin reads anywhere (`bufio`/`Scanln`/`ReadString`/`[y/N]`/`confirm` are absent outside the TUI). Every CLI command is do-or-error, non-interactive. A `[y/N]` glob-kill guard would mean building a brand-new interaction pattern the CLI does not have; not worth it for a marginal feature.
- Bulk kill's natural future home, if ever wanted, is the picker's multi-select with the existing destructive-confirm modal — not the CLI. Noted as a possibility, not committed.

---

## `uninstall` — Runtime-Only Teardown (replaces `state cleanup`)

`portal state cleanup` is replaced by a public **`portal uninstall`** that is **runtime-only and fully recoverable**. The command *is* the teardown — nothing hidden behind a flag — and it touches **no files at all**.

- **Removes only Portal's tmux-server footprint:** kills the `_portal-saver` daemon and unregisters the global tmux hooks. This is precisely the part that is hard to do by hand (locating the daemon, unregistering the exact hook entries) — the reason the command earns its place.
- **Touches no filesystem** — the state dir (`sessions.json`, logs) *and* config (`projects.json`, `aliases`, `hooks.json`, `prefs.json`, `terminals.json`) are both left untouched. Nothing irreversible happens.
- **Prints the completion path**, e.g.:
  ```
  Portal's tmux runtime removed. Your saved sessions and config are untouched at ~/.config/portal/.
  To remove Portal completely, uninstall the binary and delete that directory.
  ```
  Because `state/` lives *inside* `~/.config/portal/`, one `rm -rf ~/.config/portal` wipes both — a single deliberate act by the user. Portal never silently deletes data.
- **Fully recoverable:** the self-heal is the feature — `portal open` re-bootstraps from the retained state (daemon + hooks return, sessions restore). `uninstall` means "deactivate Portal's machinery now," not "destroy my data."
- **Idempotent / nothing-to-remove.** If there is no running tmux server, no `_portal-saver` daemon, or no registered hooks, `uninstall` is a graceful no-op — it removes whatever is present and still prints the completion message; it never errors on already-clean state.
- **Leaves all sessions in place.** `uninstall` touches no sessions: user sessions **and** the load-bearing `_portal-bootstrap` anchor session are left running. "Removes Portal's tmux-server footprint" means the daemon + global hooks only — not sessions.

### Why runtime-only (context)

- The old `state cleanup` hid its meaningful action (`--purge`, which deleted the state dir) behind a flag — the exact inconsistency this redesign removes.
- The non-purge teardown already **self-heals**: bootstrap re-registers hooks and respawns the daemon on the next tmux-touching command. Even the old `--purge` was transient while the tmux server ran (the daemon recaptures every live session into a fresh `sessions.json` on its next tick). Purge only permanently lost data when the server was *also* gone (post-reboot / `kill-server`).
- Because `uninstall` deletes nothing, there is **no `--yes` gate, no prompt, and no confrontation with the "CLI never prompts" observation** — the earlier destructive-delete design (which needed `--yes` + symlink-safe removal) is dropped entirely. Leaving files behind is standard uninstaller behavior, made honest by the printed message.

Name kept (`uninstall`).

---

## `doctor` — Diagnostics & Repair (replaces `clean` and `state status`)

`portal clean` is **deleted** and `state status` is subsumed. A new public **`portal doctor`** consolidates diagnosis and low-stakes repair.

- **`portal doctor`** — a read-only health report across all of Portal. The authoritative check catalog (the set `doctor` inspects — planning implements the concrete probe per check):
  - daemon alive;
  - global tmux hooks registered without duplicates (exactly one Portal entry per managed event);
  - `_portal-saver` session up;
  - state dir sane;
  - `sessions.json` valid;
  - no stale entries (dead-pane hooks, gone-dir projects);
  - host terminal detected + supported (see "Host-terminal detection folded in" below).

  **Subsumes `state status`.**
- **`portal doctor --fix`** — performs the low-stakes, reversible-by-reconstruction repairs it diagnoses: prune stale hooks, prune stale projects, sweep logs. One coherent surface (diagnose, optionally repair the diagnosis) instead of a grab-bag verb plus scattered prune commands.
  - `--fix` is an action-behind-a-flag but is explicitly *not* the hidden-destructive pattern rejected on `uninstall`: it is the obvious paired verb to a diagnosis, and everything it does is low-stakes and reconstructable.

### Host-terminal detection folded in (`--detect` retired)

`spawn --detect` (a dry-run that printed the detected host terminal's identity, e.g. `Ghostty · com.mitchellh.ghostty`) is retired with `spawn`. Its job folds into `doctor`: the picker keeps calling `Detect()` in-process; `doctor` calls the same function and prints a line such as `host terminal: Ghostty (supported)` / `unsupported (remote session)`.

### `clean` deleted

- `portal clean` and its `--logs` flag are **removed**. Logs auto-rotate and retention-sweep in the log handler; `rm` covers the rest.
- No `logs`/`hooks` maintenance namespaces are created — those actions don't earn standing commands.
- **Stale-project pruning folds into the daemon's automation** on a slow cadence (hourly-ish; hooks already prune on the idle tick). Mechanism/cadence is an implementation detail. Net effect: `doctor` reads *healthy* almost always because the automation keeps it that way; `--fix` is the manual trigger of the same repairs.

### Rationale (context)

`clean` bundled three unrelated jobs (prune stale projects, prune stale hooks, force log sweep) behind one verb + a flag — a grab-bag. Value audit: stale-hook prune is redundant (daemon does it), the log sweep is redundant (handler retention-sweeps per day), stale-project prune was the only unique action (harmless cruft). The reorg separates *diagnosis* ("is Portal healthy?" — recurring, valuable) from *action* ("clean X" — mostly automated), which dissolves `clean`. `doctor`/`--fix` follows the `brew doctor` / `flutter doctor` idiom (a doctor diagnoses **and** treats). **Nothing internal calls `clean` or `state cleanup`** — both were purely manual backstops to already-automated work.

---

## `state` Namespace — Fully Hidden

The `state` namespace becomes **fully hidden** but cannot stop being a command. Every remaining `state` subcommand is a **separate-process entry point** invoked by an argv, not an in-process Go call, so each must stay invocable:

- `state daemon` — the process the `_portal-saver` pane runs.
- `state hydrate` — exec'd into each restored pane via `respawn-pane -k`.
- `state signal-hydrate` / `state notify` / `state commit-now` / `state migrate-rename` — all fired by tmux hooks as `run-shell "portal state …"`.

A separate process can only be handed a command line, never a Go function (the same constraint that made `open --session` the spawn exec target). Once `status` → `doctor` and `cleanup` → `uninstall`, `state` has **zero user-facing children**, so the whole namespace is marked **hidden** (gone from `--help` and completion). To the user `state` disappears; to tmux it remains plumbing.

- **Keep the `state` prefix** — the hook definitions match those command strings by substring for idempotency (`notifyCommand`, `commitNowSubstring`, `migrateRenameSubstring`, `PortalDaemonArgvPattern`, …); renaming would churn internal matching for zero user benefit.
- `state` **cannot be removed entirely** (it is real plumbing), only hidden.

---

## Remaining Verbs — Keep As-Is, except `hooks` → `hook`

`list`, `alias`, `init`, `version`, `completion` **keep as-is** (right name, shape, and tier). One grammar change:

- **`hooks` → `hook`** (canonical), following the dominant modern convention of a **singular** namespace noun for a collection (`docker container`, `gh pr`, `git remote`). `alias` was already singular and stays; `hooks` was the odd one out.
- **`hooks` is retained as a cobra alias of `hook`** — the one deliberate exception to the no-back-compat rule (see Back-Compat). `portal hook …` is canonical/documented; `portal hooks …` keeps working.

---

## Bootstrap Exemption — `doctor` & `uninstall`

`PersistentPreRunE` runs the full bootstrap (EnsureServer → RegisterHooks → EnsureSaver → Restore → …) before most commands, but `skipTmuxCheck` (`cmd/root.go`) exempts a set (including `state`). As the renamed successors to `state status`/`state cleanup`, **`doctor` and `uninstall` join `skipTmuxCheck` (bootstrap-exempt).**

- **`doctor` must be exempt** — otherwise bootstrap re-registers hooks and respawns the daemon one step *before* `doctor` reads health, so a read-only check would heal its own subject and always report green (self-defeating). Exempt, it observes raw state, starts nothing (a down server is reported honestly, not silently started), and heals nothing.
- **`uninstall` must be exempt** — otherwise it would EnsureServer / RegisterHooks / EnsureSaver / Restore and then immediately tear all of it down (circular, wasteful, racy).
- `clean` **leaves** the exempt set (deleted); `state` **stays**; the `hooks` → `hook` rename keeps the exemption (`skipTmuxCheck` keys on `c.Name()`, cobra's canonical name, so the `hooks` alias is covered).

This applies the existing, code-documented exemption to the renamed successors — no new pattern.

---

## Bare `portal` (no subcommand)

**Bare `portal` prints help/usage — it does NOT launch the picker.** The picker already has two doors (`portal open`, `x`); bare `portal` is the control-plane root and lists commands.

- Making bare `portal` open the picker would also make bare `xctl` open the picker (since `xctl() { portal "$@" }`), muddying the two-tier split that is deliberately kept: **`x` = launcher (picker / open), `xctl` / `portal` = management plane (help when bare).**

---

## Back-Compat & Deprecation Story

**There is no back-compat story — deliberately.** This is a deliberate reversal of the seed's assumption (which called for compatibility aliases), recorded so specification/planning does not reintroduce aliases.

- `attach` and `spawn` are **removed** — not aliased, not deprecated-with-warning.
- Broken scripts are the owner's to fix (single-digit user base; the author owns the known scripts).
- The `x` / `xctl` shell functions re-emit from `portal init` and keep working untouched (`x` already maps to `portal open`).
- No alias lifecycle exists because no compat aliases exist.

**One deliberate exception: `hooks` → `hook` keeps `hooks` as a permanent, silent cobra alias.** Not a softening of the rule — a targeted carve-out: `portal hooks set …` is auto-generated by the user's external Claude SessionStart skill (machine-written, not author muscle memory), so breaking that specific string has real operational cost that the removed `attach`/`spawn` don't. No deprecation timer. Every *other* renamed/removed verb takes no alias.

---

## Deferred Scope (explicitly out of this design)

These are deferred future scope, not unresolved decisions — recorded so planning does not build them:

- **Stay-put multi-open flag** — an explicit future flag on `open` (open windows for N targets but leave the trigger terminal where it is). The absorb/net-N default takes the trigger to the first target; the exceptional stay-put behavior gets the flag when designed. Not designed here.
- **Multi-match zoxide** — a `-z`/query variant that mints sessions for *every* frecency-match of a term (via `zoxide query --list`). Shotgun risk (mints N sessions for possibly-stale dirs). Not designed here.
- **Bulk kill via the picker's multi-select** — the natural future home for killing many sessions at once (reusing the multi-select mode + the existing destructive-confirm modal). Not built here; `kill` stays single + exact.

---

## Command Surface Summary (final shape)

### Public commands

| Command | Shape | Change from today |
|---|---|---|
| `portal open [targets…]` | single public session verb; no-args → picker; flags `-s/-p/-z/-a/-f`, `-e`/`--`; absorb/net-N; hidden `--ack` | **absorbs `attach` + `spawn`**; gains session-name targets, domain pins, multi-target burst; loses TUI-fallback-on-miss |
| `portal kill <name>` | single + exact | unchanged |
| `portal list` | list running sessions | unchanged |
| `portal alias {set,rm,list}` | path aliases | unchanged |
| `portal hook {set,rm,list}` | resume hooks | **renamed from `hooks`** (`hooks` kept as a silent alias) |
| `portal doctor [--fix]` | health report; `--fix` repairs | **new** — subsumes `state status`, replaces `clean`, folds in `spawn --detect` |
| `portal uninstall` | runtime-only teardown | **new** — replaces `state cleanup` |
| `portal init [shell] [--cmd name]` | shell integration | unchanged |
| `portal version` | version | unchanged |
| `portal completion` | cobra built-in | unchanged |
| bare `portal` | help / usage | unchanged (does not open picker) |

### Removed public commands

| Removed | Replacement |
|---|---|
| `portal attach <session>` | `portal open --session <name>` (or bare `open <name>`) |
| `portal spawn [sessions…]` | `portal open <t1> <t2> …` (multi-target) |
| `portal spawn --detect` | `portal doctor` (host-terminal line) |
| `portal clean [--logs]` | `portal doctor --fix` (repairs) + automatic daemon pruning |
| `portal state status` | `portal doctor` |
| `portal state cleanup [--purge]` | `portal uninstall` |

### Hidden (invocable plumbing, absent from `--help` / completion)

| Hidden | Invoked by |
|---|---|
| `portal open --ack <batch>:<token>` | spawned host windows (burst receipt) |
| `portal state daemon` | the `_portal-saver` pane |
| `portal state hydrate` | `respawn-pane -k` per restored pane |
| `portal state signal-hydrate` / `notify` / `commit-now` / `migrate-rename` | tmux hooks (`run-shell "portal state …"`) |

---

## Working Notes
