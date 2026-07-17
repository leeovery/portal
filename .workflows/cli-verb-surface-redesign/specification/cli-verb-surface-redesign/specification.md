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

## Working Notes
