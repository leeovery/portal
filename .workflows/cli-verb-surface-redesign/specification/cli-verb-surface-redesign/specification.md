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

A bare positional target is resolved through a fixed precedence chain, first match wins:

**exact session name → path → alias → zoxide query**

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

One observability addition is locked: **the resolver logs its decision** under the existing log taxonomy, e.g. `resolve: 'blog' → zoxide → ~/Code/blog`, so a confusing guess is reconstructable from `portal.log`.

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

`--session` and `--path` invocations **hard-fail on unresolvable and never fall back to the TUI picker** — a spawned window or script must never pop a TUI. `--session` never mints (a bare name has no directory to mint from); `--path` mints per Axiom 2 (the directory must exist).

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

## Working Notes
