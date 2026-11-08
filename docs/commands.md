# Commands

This page describes every Portal command: what it does, its flags, and what it prints.

The examples use the default `x` / `xctl` function names that [`portal init`](#portal-init) defines. `x` runs `portal open`, and `xctl <command>` runs `portal <command>`. If you used `--cmd p`, substitute `p` and `pctl`. You can also call the `portal` binary directly.

| Group | Command | What it does |
|---|---|---|
| [Opening](#opening) | [`x`](#x-open) | Opens the picker, or a session, path, alias or zoxide match; several targets open several windows. |
| [Sessions](#sessions) | [`xctl list`](#xctl-list) | Lists running sessions. |
| | [`xctl kill`](#xctl-kill) | Kills a session, and everything Portal saved for it. |
| | [`xctl rename`](#xctl-rename) | Sets the current session's label. |
| | [`xctl current`](#xctl-current) | Prints the current session's label. |
| [Paths](#paths) | [`xctl alias`](#xctl-alias) | Sets, removes and lists path aliases. |
| [Resume hooks](#resume-hooks) | [`xctl hook`](#xctl-hook) | Registers, removes and lists resume hooks. |
| [Health](#health) | [`xctl doctor`](#xctl-doctor) | Reports Portal's health, and makes the low-stakes repairs. |
| | [`xctl version`](#xctl-version) | Prints Portal's version. |
| [Themes](#themes) | [`xctl theme export`](#xctl-theme-export) | Prints a theme's file. |
| [Setting up and taking down](#setting-up-and-taking-down) | [`portal init`](#portal-init) | Prints the shell integration. |
| | [`portal uninstall`](#portal-uninstall) | Stops Portal's save daemon, leaving your sessions and config. |

## Opening

### `x` (open)

The single session verb: the picker, opening a target, and multi-window bursts all live here. `x` maps to `portal open`.

```bash
x                                    # interactive TUI picker
x api                                # attach the existing session named "api"
x 'api-*'                            # attach every session matching the glob
x /port                              # search live sessions for "port"
x /                                  # open the picker with the filter ready to type
x ~/Code/myproject                   # mint a new session at a path
x myalias                            # resolve alias → mint a new session there
x ~/Code/app -e "make dev"           # mint a session and run a command
x ~/Code/app -- npm start            # alternative command syntax
x work api ~/Code/new                # open three surfaces at once (see below)
```

**Resolution.** A bare target runs the precedence chain, first match wins: **exact session name → kept saved session → path → alias → zoxide query.** A session name attaches; a kept saved session is brought back and attached; a path, alias or zoxide match mints a new session there, at the git root when there is one. A target that resolves to nothing fails, pointing you at `-f`. [Opening a session](sessions.md#opening-a-session) has each step in full.

**Session search.** A positional beginning with `/` and containing no further `/`, such as `x /port`, searches your live sessions rather than resolving a target: one match attaches, several or none open the picker filtered by the term, and `x /` opens the picker with the filter ready to type. A single-segment absolute directory typed without a trailing slash, such as `x /tmp`, searches too; mint there with `x -p /tmp`. A search composes with nothing else on its line. See [Searching your sessions](sessions.md#searching-your-sessions).

**Domain pins** skip the chain and force one domain. A pin that misses hard-fails without popping the picker. `-f`, `/<term>` and `-e`/`--` are not pins:

| Flag / form | Pins to | Behaviour |
|---|---|---|
| `-s, --session <name/glob>` | session | attach; never mints |
| `-p, --path <dir>` | path | mint at the directory (must exist) |
| `-a, --alias <key/glob>` | alias | mint at the aliased directory |
| `-z, --zoxide <query>` | zoxide | mint at zoxide's best match (errors if zoxide isn't installed) |
| `-f, --filter <text>` | — | skip resolution, open the picker pre-filtered (mutually exclusive with any target or pin) |
| `/<term>` *(positional, not a flag)* | — | open the picker pre-filtered by the term, or attach outright when exactly one live session matches |
| `-e, --exec <cmd>` / `-- <cmd>` | — | command to run in a **freshly minted** session (never an attach target) |

**Multi-window bursts.** Two or more targets, or one glob expanding to several sessions, open each in its own window, this terminal among them; see [Several sessions at once](sessions.md#several-sessions-at-once).

**Tab completion.** Tab after the session-opening function asks Portal for `open`'s completions: `x <TAB>` offers your live session names, and `x /po<TAB>` completes the term after the slash against the sessions a search can reach (every live session but the one you are attached to, since a search never lands you back where you already are), leaving the slash in place. A shell has this only once it has evaluated the current output of `portal init`: start a new shell, or run the `eval` in your profile again. The `/term` form itself is `portal open`'s own parsing, and doesn't depend on the shell integration. A path argument after the function does not fall through to filename completion. Switching that fallback off needs a shell that supports it: bash 4+ with `bash-completion`, zsh, or fish. macOS's stock `/bin/bash` 3.2 can't, so filenames still complete there.

## Sessions

### `xctl list`

Lists running sessions. Portal's own are hidden. A session that does not answer is listed by its name, marked `not answering` in the long form.

```bash
xctl list                            # auto-detect format
xctl list --long                     # full details
xctl list --short                    # names only
```

| Flag | Description |
|---|---|
| `--long` | Full session details (name, label, attached or detached, directory) |
| `--short` | Session names only, one per line |

With neither flag, the long form goes to a terminal and names alone go to a pipe. `--long` and `--short` can't be given together.

### `xctl kill`

Kills a session by name.

```bash
xctl kill myproject
```

A kill is final: the session's saved state, scrollback and resume hooks are removed, and it does not come back after a reboot. A saved session that isn't running, which [`xctl doctor`](#xctl-doctor) lists as kept, is discarded the same way, and the kill says so. While another Portal is starting or bringing a session back, the kill waits for it first, and kills the session outright if it came back meanwhile. Portal's own `_`-prefixed sessions, such as `_portal-saver`, are no session found to `kill`, as they are hidden everywhere else.

### `xctl rename`

Sets the label of the session it runs in. Run it inside a session. The label is what the picker shows for the session; the session's name never changes.

```bash
xctl rename api work                 # label the current session "api work"
xctl rename -- -wip                  # a label starting with - goes after --
xctl rename                          # reset the label to the session's name
```

Empty text, or the session's own name, resets the label to the name. A label holding a control or formatting character is refused; see [Labels](sessions.md#labels).

### `xctl current`

Prints the label of the session it runs in, or its name when it has no label. Outside a session it prints nothing and exits 1, so a status line or prompt reading it shows nothing there.

```bash
xctl current
```

## Paths

### `xctl alias`

Manages path aliases for quick session access. See [Aliases](sessions.md#aliases).

```bash
xctl alias set work ~/Code/work      # create alias
xctl alias rm work                   # remove alias
xctl alias list                      # list all aliases
```

## Resume hooks

### `xctl hook`

Registers per-session commands to bring back after a reboot. [Resume hooks](resume.md) says what they do and when they fire.

```bash
xctl hook set --on-resume "npm start"                      # register a resume hook
xctl hook set --on-resume "npm start" --resume-mode lazy   # register it pinned to a resume mode
xctl hook rm --on-resume                                   # remove the current session's hook
xctl hook rm --on-resume --key 'api-Rf4TmZ'                # remove the entry under any hook key, verbatim (works outside a session)
xctl hook list                                             # list hooks
```

| Subcommand | Flag | Description |
|---|---|---|
| `hook set` | `--on-resume <cmd>` | The command to bring back when the session is restored |
| | `--resume-mode <mode>` | `eager` or `lazy`: the resume mode this registration is pinned to |
| `hook rm` | `--on-resume` | Remove the on-resume hook |
| | `--key <key>` | The hook key of the entry to remove, taken verbatim; defaults to the current session's name |

- **`hook set`** must be run from inside a session. `--resume-mode` takes `eager` or `lazy` and nothing else; any other value is refused and nothing is written. A registration carries eager, lazy or nothing, and one carrying nothing follows the install-wide `resume_mode` in [`prefs.json`](configuration.md#prefsjson). Leaving `--resume-mode` out writes a registration that names no mode, whatever its predecessor carried, so pass the mode again alongside the command to keep a pin.
- **`hook rm`** defaults to the current session's name, but `--key` removes the entry under any hook key, taken verbatim, including sessions that no longer exist. It exits non-zero when it removes nothing (a missing entry, or a session with no hook of its own), so a scripted caller can tell a real removal from a no-op.
- **`hook list`** prints one hook to a line, tab-separated: the key, the event, the command, the session's label (empty if no live session carries the key), and the registration's resume mode (`eager`, `lazy`, or empty when it follows the install).

The verb is **`hook`** (singular); **`hooks`** is kept as a permanent silent alias, so `xctl hooks …` scripts keep working.

## Health

### `xctl doctor`

A read-only health report across Portal's resurrection machinery, from zmx and the saver to the state directory, hooks and projects. It starts nothing and signals nothing. [Troubleshooting](troubleshooting.md#doctor) explains each line it prints.

```bash
xctl doctor              # health report
xctl doctor --fix        # apply low-stakes repairs, then re-diagnose
```

| Flag | Description |
|---|---|
| `--fix` | Make the low-stakes repairs, then run the diagnosis again; see [`doctor --fix`](troubleshooting.md#doctor---fix) |

It exits `0` only when every check passes, non-zero otherwise, so it serves as a scriptable health gate. Lines that only inform, and theme advisories, never affect the exit code. With `--fix`, the exit code reflects the state after the repairs, and a `--fix` that refuses every repair exits non-zero.

### `xctl version`

Prints the Portal version.

```bash
xctl version
```

## Themes

### `xctl theme export`

Prints a theme's file to stdout, comments and all. This is the way to start your own theme from a built-in: copy one, edit it, drop it in your themes directory. See [Themes](theming.md).

```bash
xctl theme export nord                                    # print it
mkdir -p ~/.config/portal/themes
xctl theme export nord > ~/.config/portal/themes/mine.theme
```

## Setting up and taking down

### `portal init`

Prints the shell integration script, for `eval`: the `x` and `xctl` functions and their tab completions. It takes the shell, `zsh`, `bash` or `fish`. You call it through the `portal` binary directly. It reads no state, so a new shell's `eval` keeps defining `x` and `xctl` whatever the state directory holds.

```bash
portal init zsh
portal init bash --cmd p
```

| Flag | Description |
|---|---|
| `--cmd <name>` | Name the functions `<name>` and `<name>ctl` rather than `x` and `xctl` |

### `portal uninstall`

Stops Portal's save daemon **without touching your saved sessions or config**. It asks the daemon to stop with SIGTERM, so the daemon saves once more first, and waits up to 8 seconds; a daemon still running then gets SIGKILL. Once the daemon has exited, its `_portal-saver` session ends. Saved sessions and all config are left in place, and the next `x`/`portal open` brings the daemon back, so it means "stop Portal's machinery now," not "destroy my data." With no daemon running it says so and succeeds. If another Portal command is starting the daemon at that moment, it waits for that to finish, then stops the daemon that command started.

Before each signal it checks that the process is Portal's save daemon. If that check fails before the SIGTERM, nothing is sent; if it fails before the SIGKILL, the daemon has had the SIGTERM but not the SIGKILL. Either way the session is left and `portal uninstall` exits non-zero, saying which.

```bash
portal uninstall
```

#### Removing Portal

Two paths, depending on whether you want to keep your saved state:

- **Remove the binary:** `brew uninstall portal` or `rm $(which portal)`. `brew uninstall portal` leaves zmx installed, as Homebrew leaves a dependency. Either way your sessions keep running in zmx, and a save daemon already running keeps running until its `_portal-saver` session ends. Your saved state is preserved, and reinstalling picks up where it left off.
- **Explicit teardown:** run `portal uninstall`, which stops the save daemon once it has saved. It touches **none of your files**: saved sessions and all config are left in place, and running `x` again brings the daemon back. To remove Portal completely, run `portal uninstall`, then uninstall the binary and delete `~/.config/portal/` (which holds both state and config) yourself.
