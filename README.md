<div align="center">

# 🌀 Portal

**Interactive session picker for zmx**

Fast, fuzzy session management from a bare shell, with project memory,
<br>path aliases, reboot-safe restoration, and a keyboard-driven TUI.

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8.svg)](https://go.dev)
[![Platform: macOS](https://img.shields.io/badge/platform-macOS-lightgrey.svg)](#install)

[Install](#install) · [Quick Start](#quick-start) · [How It Works](#how-it-works) · [Commands](#commands) · [Shell Integration](#shell-integration) · [Configuration](#configuration) · [Upgrading from tmux](#upgrading-from-the-tmux-version)

<br>

<img src="art/portal-cold.webp" alt="Portal's first launch after a reboot: the loading page restores twelve sessions, the picker lands on them, and a restored session's scrollback is previewed" width="840">

</div>

---

Portal runs at a bare shell and gives you an interactive TUI for picking, creating, and managing [zmx](https://github.com/neurosnap/zmx) sessions. It remembers your projects, resolves paths via aliases and zoxide, auto-detects git roots, and restores your saved sessions after a reboot.

After [shell setup](#shell-integration) you drive it through two functions: **`x`** (the picker and opener) and **`xctl`** (subcommands like `list`, `kill`, `alias`). Both names are configurable with `--cmd`.

## Why Portal?

zmx keeps sessions alive; getting back into the right one is the chore. Finding it means `zmx list` and a name you half remember. Starting one in the right place means a `cd`, a `zmx attach` and a name to invent. A reboot takes the lot, and bringing sessions back by hand brings every command back with them, all at once, whether you still need it or not.

Portal folds that into two shell functions and a picker:

- **One verb to get anywhere.** Jump to a project by path, alias, or zoxide (`x work`), attach an existing session by name or glob (`x api`, `x 'api-*'`), or search your live sessions by name or directory (`x /api`), with git-root resolution and project memory built in.
- **Reboot-safe sessions.** Portal brings your sessions back after a reboot, each in its last working directory with its ANSI scrollback.
- **Commands come back when you ask.** Per-session commands return via resume hooks: each session comes back holding a panel showing its command, which runs when you answer it, so a reboot doesn't start every dev server you ever left open.
- **A keyboard-driven picker.** A colourful picker that owns its own canvas, with an in-app `?` keymap on every page. Six themes ship — Tokyo Night, Tokyo Night Day, Nord, Nord Wave, GitLab Dark Grey, and Horizon — switched with `t` and matched to your terminal's background unless you pin one; drop-in `.theme` files restyle every screen, and `NO_COLOR` is honoured.
- **Session grouping and tags.** Flip the list between flat, by project, and by tag with one key. Tags live on directories, so every session opened there inherits them.
- **Scrollback preview.** Hit `Space` for a read-only peek at any session's screen and scrollback, kept live while it's open, without attaching; a session waiting on its resume panel shows the transcript it was restored from.
- **Multi-window open.** Name several targets (`x work api db`) or mark them with `m` in the picker and press `Enter` to open each in its own host-terminal window — rebuild your post-reboot window layout in one action instead of by hand. Ghostty works out of the box; other terminals via a `terminals.json` recipe.

## Install

Coming from the tmux version of Portal? Install this one, then run `portal migrate` before anything else: see [Upgrading from the tmux version](#upgrading-from-the-tmux-version).

### Requirements

- **[zmx](https://github.com/neurosnap/zmx)** 1.0.1 or later, which speaks zmx's socket protocol 1, the way Portal talks to zmx. (1.0.0 was the first release to speak it, but its macOS downloads don't resize a session.) Portal refuses zmx 0.8.1, and lists a session started by it as not answering.
- **macOS**, and **Go** 1.26+ to build from source.

**Homebrew**

```bash
brew install leeovery/tools/portal
```

Homebrew installs zmx with it, as a dependency.

**Go**

```bash
go install github.com/leeovery/portal@latest
```

`go install` builds Portal alone: install zmx separately, with `brew install neurosnap/tap/zmx` or from [zmx's getting started guide](https://zmx.sh/getting-started).

## Quick Start

```bash
eval "$(portal init zsh)"              # in ~/.zshrc — defines x and xctl (bash and fish below)

x                                      # the picker: ↑/↓ move, / filter, Space preview, Enter attach
x ~/Code/api                           # new session at the repo's git root
x ~/Code/api -e "make dev"             # ...with a command running in it
x /api                                 # later: search live sessions by name or directory
xctl alias set work ~/Code/work        # alias a path...
x work                                 # ...then open it by name
xctl hook set --on-resume "make dev"   # in a session: bring its command back after a reboot
xctl doctor                            # save daemon running? state sane? anything stale?
```

Your sessions are saved as you work. After a reboot the first `x` rebuilds them — see [How It Works](#how-it-works).

## Screenshots

<div align="center">
<img src="art/portal-tour.webp" alt="Portal feature tour: grouping, fuzzy filter, scrollback preview, projects" width="840">
</div>

The full tour: grouping, fuzzy filter, scrollback preview, projects. Stills below.

|  |  |
|:---:|:---:|
| <img src="art/sessions-by-tag.webp" alt="Sessions grouped by tag" width="430"><br>**Grouped by tag** | <img src="art/preview.webp" alt="Scrollback preview" width="430"><br>**Peek-mode scrollback preview** |
| <img src="art/loading.webp" alt="Honest loading screen" width="430"><br>**Concurrent cold-boot loading** | <img src="art/help-modal.webp" alt="Keybindings help modal" width="430"><br>**Per-page `?` keymap** |
| <img src="art/projects.webp" alt="Projects page" width="430"><br>**Projects** | <img src="art/edit-project.webp" alt="Edit project modal" width="430"><br>**Edit project: name, aliases, tags** |
| <img src="art/filtering.webp" alt="Live filter" width="430"><br>**Live fuzzy filter** | <img src="art/kill-confirm.webp" alt="Kill confirmation modal" width="430"><br>**Destructive confirm** |
| <img src="art/resume-panel.webp" alt="Resume panel on a restored session" width="430"><br>**Resume panel after a reboot** | <img src="art/theme-panel.webp" alt="Theme picker over the sessions" width="430"><br>**Theme picker** |

The same screens render in light mode and under `NO_COLOR` (see [Configuration](#configuration)).

## How It Works

```
  OPEN    x                       ──▶  the picker
          x /term                 ──▶  search: one match attaches, more open the picker
          x <session | glob>      ──▶  attach, or switch inside a session
          x <path | alias | zox>  ──▶  new session at the git root

  SAVE    _portal-saver ──▶ save daemon ──▶ state/sessions.json + scrollback/*.bin
                            every 5 seconds, whatever changed

  REBOOT  first x ──▶ start the saver ──▶ rebuild sessions ─┬─▶ cwd · scrollback
                                                            └─▶ resume hooks wait on a panel
```

- **Opening.** Portal runs `zmx attach` itself and stays running while you're attached, watching for [its own keys](#keys-in-a-session); inside a session it switches the terminal you typed in, so sessions never nest. Opening a session [takes it from any other terminal](#one-terminal-at-a-time) showing it, so it is drawn for yours from the first frame. See [`x` (open)](#x-open).
- **Saving.** A daemon in a hidden `_portal-saver` session reads zmx's sessions every 5 seconds and writes what changed — the sessions there are, each one's working directory and, at most once a minute, its ANSI scrollback — under `state/` (see [Configuration](#configuration)).
- **Restoring.** After a reboot, the first command that needs your sessions brings the saver back and rebuilds every saved session that isn't already live. See [Automatic Bootstrap & Restoration](#automatic-bootstrap--restoration).
- **Resuming.** A session with a resume hook comes back holding the [resume panel](#resume-panel); `Enter` runs its command, `d` discards it, `Esc` sends the terminal back to the picker and leaves it waiting.
- **A session you didn't make.** `_portal-saver` hosts the daemon. It shows in a raw `zmx list`; the picker and `xctl list` hide it.

## Shell Integration

Portal generates shell functions via `portal init`. Add to your shell profile:

```bash
# zsh
eval "$(portal init zsh)"

# bash
eval "$(portal init bash)"

# fish
portal init fish | source
```

This creates two functions:

- **`x()`**: launches Portal (interactive picker or path-based session creation)
- **`xctl()`**: direct access to Portal subcommands (`list`, `kill`, `alias`, etc.)

Customize the function name with `--cmd`:

```bash
eval "$(portal init zsh --cmd p)"   # creates p() and pctl()
```

## Commands

> Examples below use the default `x` / `xctl` function names. If you used `--cmd p`, substitute `p` and `pctl`. You can also call the `portal` binary directly.

### `x` (open)

The single session verb — the picker, opening a target, and multi-window bursts all live here. `x` maps to `portal open`.

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

**Resolution.** A bare target runs the precedence chain, first match wins:

**exact session name → kept saved session → path → alias → zoxide query.**

An exact session name **attaches** that existing session. The name of a saved session that isn't running, one the next restore would bring back, **brings it back** with its scrollback and resume hook, then attaches it (`-s` reaches it too, and so does a name with glob characters in it, read as that name rather than a glob; a glob never matches one). `xctl doctor` lists such sessions. If it can't be brought back, one line says why and the session stays saved for the next restore. While another Portal is starting, it waits for it as a start does, and a wait that runs out fails with the same error a start gives. A path, alias, or zoxide match **mints a brand-new session** there (directory targets always create — there is no find-or-create, so `x api` mints even while an `api-*` session runs; reach the existing one with the name, a glob, or `-s`). A target that resolves to nothing is a hard failure — there is no TUI-fallback-on-miss; the error points you at `-f`.

**Session search.** A positional beginning with `/` and containing no further `/` is a search across your live sessions rather than a target to resolve — `x /port`. The rule tests the shape of the argument alone and never asks the filesystem anything, so the same command line means the same thing on every machine. Everything else is untouched: `x ./port`, `x ~/port`, `x /Users/me/Code/portal` and `x /tmp/` are the paths they always were (the trailing slash is the second one).

The term is matched **case-folded, as a contiguous run**, against each live session's label, name and directory — home-abbreviated (`~/Code/portal`), exactly as the picker's row displays it. Three outcomes, by how many sessions match:

- **exactly one** — that session is attached outright, no picker;
- **several** — the picker opens pre-filtered by the term, cursor on the first match;
- **none** — the picker opens on the term with nothing surviving it. That is a filter result, not an error: press `Esc` and your sessions are there.

In a picker a search opened, every row shows its recorded directory beside the session name, so a row that matched on its directory says why it is there; the column stays for that picker's life, however you edit the filter afterwards. A session list zmx cannot give is not a zero match: the search reports the error and exits non-zero rather than opening an empty picker that would suggest your sessions are gone.

`x /` — the slash with no term — opens the picker with the filter empty, focused and ready to type. It is not an error, and it does not mean "mint at root".

The picker's own hand-typed `/` filter stays **fuzzy** (scattered letters anywhere). The search form is deliberately stricter, because it can attach a lone match without ever showing it to you: a fuzzy `/port` could land you in `~/Projects/rust-tools` sight unseen.

**The cost**: a single-segment absolute directory typed without a trailing slash — `x /tmp`, `x /opt` — now searches instead of minting. Mint there with the pin that exists for it: `x -p /tmp`.

A search composes with nothing — a second target, a trailing command, `-f`, or any domain pin on the same line is a usage error. Recognition stops at a `--` separator, so the words after it stay the trailing command's own (`x ~/Code/api -- ls /tmp` is unaffected).

**`-f` or `/term`?** They differ by outcome: `-f` always opens the picker, which makes it the form for a script or a keybinding that must land in the same place every time; `/term` takes you straight to the session when only one matches, which makes it the interactive form.

**Domain pins** skip the chain and force one domain — a pin that misses hard-fails without popping the picker. `-f`, `/<term>` and `-e`/`--` are not pins:

| Flag / form | Pins to | Behaviour |
|---|---|---|
| `-s, --session <name/glob>` | session | attach; never mints |
| `-p, --path <dir>` | path | mint at the directory (must exist) |
| `-a, --alias <key/glob>` | alias | mint at the aliased directory |
| `-z, --zoxide <query>` | zoxide | mint at zoxide's best match (errors if zoxide isn't installed) |
| `-f, --filter <text>` | — | skip resolution, open the picker pre-filtered (mutually exclusive with any target or pin) |
| `/<term>` *(positional, not a flag)* | — | open the picker pre-filtered by the term — or attach outright when exactly one live session matches |
| `-e, --exec <cmd>` / `-- <cmd>` | — | command to run in a **freshly minted** session (never an attach target) |

**Multi-window bursts.** Two or more targets (or one glob expanding to several sessions) open a portal to each: this terminal becomes the first surface and the remaining **N−1** open in host-terminal windows — **N windows for N targets**. Pins and bare targets mix freely (`x -s api -p ~/Code/new blog`), the command rides only the minted surfaces, and a supported terminal is required for the extra windows (Ghostty natively, others via [`terminals.json`](#configuration)). This is the command-line form of the picker's [multi-select mode](#multi-select-mode).

**Tab completion.** Tab after the session-opening function asks Portal for `open`'s completions: `x <TAB>` offers your live session names, and `x /po<TAB>` completes the term after the slash against the sessions a search can reach — every live session but the one you are attached to, since a search never lands you back where you already are — leaving the slash in place. **A shell has that behaviour only once it has evaluated the current output of `portal init`** — start a new shell, or re-run the `eval` in your profile; the `/term` form itself is `portal open`'s own parsing, independent of the shell integration. The deliberate trade beside it: a path argument after the function does not fall through to filename completion, which is the contract `portal open` has always had. (Switching that fallback off needs a shell that supports it — bash 4+ with `bash-completion`, zsh, or fish. macOS's stock `/bin/bash` 3.2 cannot, so filenames still complete there.)

New sessions auto-resolve to the git repository root when applicable.

### `xctl list`

List running sessions. Portal's own are hidden; a session that does not answer is listed by its name, marked `not answering` in the long form.

```bash
xctl list                            # auto-detect format
xctl list --long                     # full details
xctl list --short                    # names only
```

| Flag | Description |
|---|---|
| `--long` | Full session details (name, label, attached or detached, directory) |
| `--short` | Session names only, one per line |

### `xctl kill`

Kill a session by name.

```bash
xctl kill myproject
```

A kill is final: the session's saved state, scrollback and resume hooks are removed, and it does not come back after a reboot. A saved session that isn't running, which [`xctl doctor`](#xctl-doctor) lists as kept, is discarded the same way, and the kill says so. While another Portal is starting or bringing a session back, that kill waits for it first, and kills the session outright if it came back meanwhile. Portal's own `_`-prefixed sessions, such as `_portal-saver`, are no session found to `kill`, as they are hidden everywhere else.

### `xctl alias`

Manage path aliases for quick session access.

```bash
xctl alias set work ~/Code/work      # create alias
xctl alias rm work                   # remove alias
xctl alias list                      # list all aliases
```

### `xctl hook`

Register per-session commands to bring back after a reboot. Under the shipped default a restored session comes back holding the [resume panel](#resume-panel), showing the registered command with `⏎ resume`, `d discard` and `esc back`, and the command runs when you answer it; a registration pinned `eager` runs its command as the session is restored, with no panel. `hook set` must be run from inside a session; `hook rm` defaults to the current session's name but accepts `--pane-key` to remove the entry under any hook key, taken verbatim (including sessions that no longer exist). `hook rm` exits non-zero when it removes nothing — a missing entry or a session with no hook of its own — so a scripted caller can tell a real removal from a no-op.

The verb is **`hook`** (singular); **`hooks`** is kept as a permanent silent alias, so existing `xctl hooks …` scripts keep working unchanged.

Hooks are keyed by the session's name, which never changes. Renaming a session sets its label, from the picker's `r` modal or `xctl rename` inside the session, and leaves the name alone, so a relabelled session still brings its command back after the next reboot — as the panel under the shipped default, and as a fired command under `eager`.

A hook goes with its session: killing the session, or the session ending for good, removes it. A hook left behind is stale once its key names neither a session zmx lists, answering or not, nor one Portal has saved in `sessions.json`. The save daemon removes stale hooks once an hour, and `xctl doctor --fix` removes them at once. Both leave every hook alone while a restore runs, while no session is listed and none is saved (so a state directory pointed somewhere fresh never costs the hooks in `hooks.json`), while the session list, `sessions.json` or the restoring flag can't be read, and when one sweep would remove three hooks or more that are at least half of `hooks.json`. A hook goes with its session, so that only happens when `hooks.json` is judged against another state or zmx directory, such as a scratch `PORTAL_STATE_DIR` or `ZMX_DIR` beside the real file, and `portal.log` says so with a warning. Each removed command is written to `portal.log` (`grep 'op=clean-stale' portal.log`), so it can be copied back out.

A label takes any printable text. Only a label holding a control or formatting character, such as a newline, tab or escape, is refused, by the picker's `r` modal and `xctl rename` alike: `label refused: it holds a control or formatting character, such as a newline, tab or escape`. The picker says exactly that and the session keeps its label; a hook registered against it is untouched. Empty text, or the session's own name, resets the label to the name.

```bash
xctl hook set --on-resume "npm start"            # register a resume hook
xctl hook set --on-resume "npm start" --resume-mode lazy   # register it pinned to a resume mode
xctl hook rm --on-resume                         # remove the current session's hook
xctl hook rm --on-resume --pane-key 'api-Rf4TmZ' # remove the entry under any hook key, verbatim (works outside a session)
xctl hook list                                   # list hooks: key, event, command, the session's label (4th column, empty if no live session carries the key), and the registration's resume mode (5th column: eager, lazy, or empty when it follows the install)
```

`--resume-mode` takes `eager` or `lazy` and nothing else; any other value is refused and nothing is written. A registration carries eager, lazy or nothing, and carrying nothing means it follows the install-wide setting in `prefs.json`. The flag's zero value is "names no mode" rather than "keep what was there", so a `hook set` that does not pass it writes a registration carrying no mode whatever its predecessor carried — re-pass the mode alongside the command to keep a pin.

**When hooks fire:** a resume hook is only ever offered — as the panel, or fired outright
under `eager` — when Portal recreates a session from saved state: after a reboot, or once
a logout or crash has ended your sessions along with the saver. Nothing happens on an
ordinary detach and reattach, because the session and its process are still alive;
bringing the command back then would launch a second copy of a long-running command such
as a dev server.

### `xctl doctor`

A read-only health report across Portal's resurrection machinery — zmx present and every session answering, `_portal-saver` up, its save daemon alive, state dir sane, `sessions.json` valid, no stale entries, sessions waiting to resume, the [key config](#keys-in-a-session), and the detected host terminal. It starts nothing and signals nothing (a saver that isn't running is reported, not started), and exits `0` only when every check passes, non-zero otherwise — a scriptable health gate. The host-terminal line is informational and never affects the exit code, nor does the key-config line, which names any binding that falls back to its default.

```bash
xctl doctor              # health report
xctl doctor --fix        # apply low-stakes repairs, then re-diagnose
```

`--fix` performs the reversible-by-reconstruction repairs: prune stale hooks, prune stale projects, and sweep old logs. It re-runs the diagnosis afterwards and the exit code reflects the post-repair state. A hook is stale when its key names neither a session zmx lists nor one saved in `sessions.json` (see [`xctl hook`](#xctl-hook)); the stale-hooks line counts them, and fails when `hooks.json` can't be read or doesn't parse. Hooks the tmux version wrote are held back until its state is migrated: see [Upgrading from the tmux version](#upgrading-from-the-tmux-version). The daemon already runs these prunes automatically on a slow cadence, so `doctor` usually reads healthy without you doing anything — `--fix` is the manual trigger. Beside another state directory's `_portal-saver`, `--fix` repairs nothing and says so: `Skipped every repair: _portal-saver saves the state in <other>, not this one`. The host-terminal check prints the detected terminal's name and whether Portal supports it; on an unsupported terminal the picker's banner names its bundle id, the key to copy into [`terminals.json`](#configuration). The report also carries informational lines that state rather than judge — the pending-resumes line reports how many live sessions are waiting on their [resume panel](#resume-panel), the kept-sessions line, shown only when there are any, names the saved sessions that aren't running, which `x <name>` brings back (with no save daemon running, every saved session that isn't running, since the next start brings them all back), and the resume-mode line reports the install's resume mode (`eager` or `lazy`, the shipped default `lazy` whenever `prefs.json` sets none) — and, like the host-terminal line, they never affect the exit code.

A missing `_portal-saver` fails the saver and daemon lines while there is anything to save: a live session, or an entry in `sessions.json`. On an install with nothing to save yet, such as a fresh one, both lines state it instead (`nothing to save yet`) and the report stays healthy; if doctor can't list the sessions or read `sessions.json`, it can't show there is nothing to save, so a missing saver still fails rather than reading not evaluable.

The daemon passes when `daemon.pid` names the process `_portal-saver` runs and that process is Portal's save daemon, recorded at this Portal's version. A daemon still starting, not yet recorded in `daemon.pid`, is given the time Portal gives it to come up before it is judged. A daemon recording another version still saves; its line reports it rather than failing, and the next `x` after an upgrade, or any other command that runs Portal's bootstrap, replaces it. Any line doctor can't judge — a saver, `daemon.pid` or daemon process it can't read, sessions it can't list — reads not evaluable, never failing, and says why. A state directory it can't resolve is different: nothing can be saved without one, so the state dir, state's zmx dir and `sessions.json` lines fail (`unresolvable`), and the lines that read the state directory read not evaluable.

While the stale hooks can't be counted, the stale-hooks line reads not evaluable, never failing, and says why: `restore in progress`, `could not read the restoring flag`, `could not resolve the state directory`, `could not list sessions`, `could not read sessions.json` (unreadable, corrupt, or written by an older or newer Portal), `no sessions listed or saved` or `half or more of hooks.json's keys name no session listed or saved`. `--fix` then removes nothing and prints `Skipped stale hook prune:` with the same reason, or with `hooks.json is locked` when another writer held it too long, `could not read hooks.json`, `hooks.json holds malformed JSON` or `the sweep could not complete`. Otherwise it names each hook it removed, one to a line:

```
Pruned 2 stale hooks:
  notes-x7Kd9a
  blog-drafts
```

### `xctl theme export`

Print a theme's file to stdout, comments and all. This is the way to start your own theme from a built-in — copy one, edit it, drop it in your themes directory. See [`docs/theming.md`](docs/theming.md).

```bash
xctl theme export nord                                    # print it
mkdir -p ~/.config/portal/themes
xctl theme export nord > ~/.config/portal/themes/mine.theme
```

### `portal uninstall`

Stop Portal's save daemon **without touching your saved sessions or config**. It asks the daemon to stop with SIGTERM, so the daemon saves once more first, and waits up to 8 seconds; a daemon still running then gets SIGKILL. Once the daemon has exited, its `_portal-saver` session ends. Saved sessions and all config are left in place, and the next `x`/`portal open` brings the daemon back, so it means "stop Portal's machinery now," not "destroy my data." With no daemon running it says so and succeeds. If another Portal command is starting the daemon at that moment, it waits for that to finish, then stops the daemon just started.

Before each signal it checks that the process is Portal's save daemon. If that check fails before the SIGTERM, nothing is sent; if it fails before the SIGKILL, the daemon has had the SIGTERM but not the SIGKILL. Either way the session is left and `portal uninstall` exits non-zero, saying which. See [Uninstall](#uninstall).

```bash
portal uninstall
```

### `portal migrate`

Move the tmux version of Portal's saved sessions and resume hooks to this version. See [Upgrading from the tmux version](#upgrading-from-the-tmux-version).

### `xctl version`

Print the Portal version.

```bash
xctl version
```

### `portal init`

Output shell integration script for eval. See [Shell Integration](#shell-integration). Like `portal migrate` and `portal uninstall`, you call it through the `portal` binary directly. It reads no state, so it works before the tmux version's state is migrated too, and a new shell's `eval` keeps defining `x` and `xctl`.

```bash
portal init zsh
portal init bash --cmd p
```

## TUI Keybindings

The lists move with the **arrows only** (no vim or page-jump aliases); the [preview](#scrollback-preview) scrolls its history with `j` / `k`. Press **`?`** on any page for an in-app help modal listing that page's complete keymap.

| Key | Action |
|---|---|
| `↑` / `↓` | Move up / down |
| `Ctrl+↑` / `Ctrl+↓` | Page up / down |
| `Enter` | Attach to / open the highlighted session |
| `Space` | Preview scrollback of highlighted session (sessions list only) |
| `/` | Filter mode (fuzzy search) |
| `s` | Switch view: cycle Flat → By Project → By Tag (sessions list only) |
| `m` | Multi-select mode: enter marks the highlighted session, then toggle any row's mark (sessions list only) |
| `x` | Toggle between Sessions and Projects |
| `t` | Open the theme picker |
| `r` | Rename session |
| `k` | Kill session |
| `e` | Edit the highlighted project: name, aliases, tags (Projects only) |
| `d` | Delete the highlighted project, after a confirmation (Projects only) |
| `n` | New session in the current directory |
| `?` | Show the full keymap for the current page |
| `q` / `Esc` | Quit (`Esc` clears an active filter first) |

**Mouse and touch.** The picker reads the mouse, so a wheel, a trackpad, or a swipe on a touch terminal moves the highlight through the sessions, projects and theme lists and scrolls the preview. Tapping a session or project row opens it, as `Enter` would; in multi-select a tap marks the row instead, as `m` would. Selecting text while the picker is open takes your terminal's mouse-override modifier, such as `Shift`-drag in Ghostty.

The TUI has three views: session list, project picker, and scrollback preview. It paints its own canvas in the colours of the active theme — press `t` for the theme picker, or name a theme in `prefs.json` (see [Configuration](#configuration) and [docs/theming.md](docs/theming.md)); `NO_COLOR` gives a colourless render.

A session row ends in up to two status dots, packed to the right: the attached dot when a client is attached to the session, and a warm-coloured pending dot when the session is waiting on its [resume panel](#resume-panel). Under `NO_COLOR` they render as `A` and `P` in the same cells. The `?` help on the sessions list carries the legend.

### Scrollback Preview

`Space` on the highlighted session opens a Quick Look-style preview of that
session's screen and scrollback, read live from the session, so you can tell
similarly-named sessions apart without attaching. While it is open it reads the
session again about once a second, so it follows the session as it changes,
holding the place you scrolled to. A session waiting on its
[resume panel](#resume-panel) shows the transcript it was restored from instead,
since its own screen holds only the panel. The preview is read-only: opening and
closing it changes nothing about the session.

| Key | Action |
|---|---|
| `↑` / `↓` | Previous / next session in the list (wraps) |
| `j` / `k`, `Ctrl+↑` / `Ctrl+↓`, `Home` / `End` | Scroll the history |
| `Enter` | Attach to this session |
| `Space` / `Esc` | Return to the sessions list |

The preview shows the session's screen with up to 10,000 lines of history above it,
drawn as the terminal would draw it. The frame shows the session name and a footer of
key hints, styled in a cyan "peek mode" so a preview never looks like the session
itself. A session that doesn't answer in time renders `(session not answering)`; one
whose history can't be read renders `(history unavailable)`. Each stays only until the
session gives a read: the preview keeps reading, so a session that comes back to life shows
in it, and a read that fails once the preview shows the session leaves what it shows as it
was.

### Multi-Select Mode

Press **`m`** on the sessions list to enter multi-select mode, which marks the
currently-highlighted session as your first selection. Press `m` again on any row to mark or
unmark it — you can also sit in the mode with nothing selected (toggle the auto-marked row off,
or enter while the cursor is on a group header). Press **`Enter`**
to open every marked session at once — each springs open attached in its own host-terminal
window. The result is **N windows for N sessions**: the picker reuses its own window for one
of them and spawns the rest as fresh host windows, so there is never a leftover empty picker
window. `Esc` cancels and clears the selection.

| Key | Action |
|---|---|
| `m` | Enter mode marking the highlighted session / toggle a row's mark |
| `↑` / `↓` | Move between sessions (marks persist) |
| `Space` | Preview the highlighted session's scrollback |
| `/` | Filter (marks persist underneath) |
| `Enter` | Open every marked session (one marked → a plain attach in place) |
| `Esc` | Cancel and clear the selection |

Marks are sticky across filtering, paging, regrouping, and the `Space`-preview round-trip;
a session killed elsewhere while you were in the mode drops out of the selection.

Spawning host windows needs a supported terminal. **Ghostty** works out of the box; other
terminals are configured via [`terminals.json`](#configuration). On an unsupported terminal
Portal shows a banner naming the detected terminal and its bundle id (the key to copy into
`terminals.json`); a remote/mosh client has no local window to name, and nor does a session
when Portal can't tell which terminal it is shown in (several raw `zmx attach` windows on it,
say), so the header stays as normal with no banner. On any of these, pressing **`m`** does
**not** open multi-select — Portal shows a brief note that it isn't available (`over a remote
connection`, `can't tell which terminal Portal is running in`, or `on this terminal`)
rather than letting you mark sessions for a burst that can never run, and `m` drops out of the
`?` help there. Opening a single session the normal way (`Enter` on a highlighted row) still
works — it attaches in the current window and needs no host-window support. Run `xctl doctor`
to see what Portal detects. If a burst only partially succeeds, Portal leaves the windows that
did open in place and keeps the failed sessions marked, so pressing `Enter` again retries just
those.

## Keys in a Session

Portal stays running while you're attached, watching for three keys of its own. Everything else
reaches the session untouched.

| Key | Action |
|---|---|
| `Alt+D` | Detach, back to the picker |
| `Alt+Shift+D` | Detach, straight to the shell |
| `Alt+Q` | Ask to kill the session: the picker opens on its kill prompt |

On that prompt `y` kills the session and stays in the picker, `d` keeps it and stays in the
picker, and `Esc` keeps it and goes back into it, so a stray `Alt+Q` costs a key, never a
session. When Portal can't be sure the terminal still shows the session (a `zmx attach` typed
inside it switched it), the picker opens without the prompt and says why.

zmx's own `Ctrl+\` detaches too, and always to the shell. Portal detaches zmx through that key, so
under Portal it can't be turned off: `ZMX_NO_DETACH_KEY` isn't honoured. When the session itself
ends, Portal exits to the shell as plain `zmx attach` would.

**Rebinding.** Set `keys` in [`prefs.json`](#configuration). Each action takes one key or a list,
an empty list turns it off, and an action you leave out keeps its default:

```json
"keys": {
  "detach": ["alt+d", "ctrl+o"],
  "detach_to_shell": "alt+shift+d",
  "kill": []
}
```

A key is `ctrl`, `alt` or `shift` plus a letter, digit, punctuation mark or `f1`–`f12`, and needs
`ctrl` or `alt`, so a binding never swallows plain typing. Refused: `ctrl+shift+<letter>`, which
sends the same bytes as `ctrl+<letter>`; `ctrl+\`, zmx's; shift on a digit or punctuation mark, or
a mark typed with Shift such as `!`, whose bytes depend on the keyboard layout; `f3` with any
modifiers, which reads as a cursor-position report; `ctrl+g` (BEL) and `alt+\` (ST), which end a
terminal's replies; and the few keys that send another key's bytes or begin an escape sequence
(`ctrl+i` is Tab, `alt+[` begins one).
A refused or unparseable binding, or a key bound to two actions, keeps that action's default, and
the log and `xctl doctor` say why. Changes apply from the next attach.

**macOS.** The terminal must send Option as Alt, or `Option+D` types `∂`: Ghostty
`macos-option-as-alt`, kitty `macos_option_as_alt`, iTerm2's Esc+ for the Option key, Terminal's
"Use Option as Meta key", Blink's Option-as-Meta.

**Two quirks** on terminals that send keys the older way, without the newer keyboard encoding:

- With Caps Lock on, `Alt+D` arrives as `Alt+Shift+D` and detaches to the shell, and `Alt+Q`
  arrives as `Alt+Shift+Q` and reaches the session.
- `Alt+<key>` is sent as Escape then the key. Over a slow link, an Escape typed just before a bound
  letter can arrive the same way and count as the Alt key: Escape then `q` opens the kill prompt.

**Portal over ssh from a Portal session.** These keys act on the local session: the Portal you
attached through catches them before ssh sees them, so a Portal on the remote machine gets none.
Nothing is lost, since a detach only detaches and `Alt+Q` only asks. To drive the remote Portal's
keys, bind other ones in the remote machine's `prefs.json`; the local Portal passes them through.

### One terminal at a time

Opening a session takes it from every other terminal showing it, before Portal attaches, so
the session is sized and drawn for the terminal you opened it in from its first frame. Every
attach Portal makes does it: `x <name>`, a pick in the picker, a switch typed inside a Portal
attach, and going back in from the kill prompt or the screen below. Nothing is killed and
nothing is typed into the session; Portal's own `_` sessions are never taken. Only terminals
are taken: what reads a session without one, such as Portal's saver or a preview, never makes
Portal take it, though a take does close a read under way at that moment, which then reads
again (a preview keeps what it shows meanwhile). Two terminals opening one session at once end
with the later one attached.

A plain `zmx attach` on the session is detached to its shell. A Portal attached there lands on
a screen saying where the session went:

```
⇄ Open on another device

release checklist

Taken by SSH from 192.168.1.20.

⏎ take it back   space watch   esc picker
```

The last line of the message reads `SSH from <address>` when the session was taken over ssh,
`<App> on this Mac` when the taking terminal names its app, such as `Ghostty on this Mac`, and
`another terminal on this Mac` otherwise. `Enter` takes the session back, `Space` watches it in
the [preview](#scrollback-preview), live and read-only, with `Esc` back to this screen, and `Esc`
goes to the picker, its cursor on the session. A detach from anywhere else lands on the same
screen, without the line saying where, only while another terminal has the session; one that
leaves no terminal on it, such as `zmx detach` typed in the session, exits to the shell.

## Session Grouping & Tags

By default the session list is flat and alphabetical. Press **`s`** on the sessions
list to cycle the view through three modes:

- **Flat**: a single alphabetical list.
- **By Project**: a heading per directory, with each session listed once under its
  project name. Useful with no setup at all.
- **By Tag**: a heading per tag, with a session appearing under *each* tag its
  directory carries. Untagged sessions collect under a pinned **Untagged** group.

Portal remembers the last-used mode across launches in `prefs.json`. Group headers are
dimmed, non-selectable, and show a count, and the cursor only ever lands on sessions.
While the `/` filter is active the list flattens to matching sessions and the headers
step aside, returning when the filter clears.

**Tags live on directories (projects), not individual sessions.** Every session opened
in a directory inherits that directory's tags, so there is nothing to tag per session.
Tags are freeform and trimmed, but **case-sensitive**: `Work` and `work` are different
tags, and each is stored exactly as typed. Applying a tag to a second directory adds it
to that group, and removing a tag's last use makes the group disappear.

**Managing tags:** open the projects picker (press `x`, then `x` again to switch from
sessions to projects), highlight a project, and edit it. The edit modal has **Name**,
**Aliases**, and **Tags** fields. `Tab` (or `↑`/`↓`) moves between fields, and `←`/`→`
moves between chips and the trailing `+ add` slot. To add a tag, land on `+ add`, press
`Enter` (or `+`), type the tag, and press `Enter` to save; press `x` on a chip to remove
it. Every edit saves immediately, with no separate confirm step, and `Esc` never
discards saved work: it just backs out the current edit or closes the modal. Only
directories already opened in Portal are taggable, so open a directory once before
tagging it.

## Automatic Bootstrap & Restoration

Whenever you run a command that needs your sessions, Portal checks that its save daemon is
running and brings it back if it is not. In the same step it re-creates the saved sessions
that are not live: after a reboot every one, and after a logout or crash that ended your
sessions along with the saver, those too. Each comes back under its own name, in its last
working directory, with its scrollback (including ANSI colour) replayed, and each carrying
a resume hook comes back holding the resume panel, which runs the command when you answer
it. While that runs, the picker shows its loading page.

Only a kill is final. Killing a session in Portal — `k` in the picker or `xctl kill` —
removes its saved state, scrollback and resume hook for good, and `xctl kill` on a saved
session that isn't running discards it the same way. A session ended outside Portal, by
`zmx kill` or by exiting its program, goes the same way once the save daemon sees it gone,
about ten seconds later, with four exceptions that err towards keeping it: when several
sessions end at once, the daemon keeps them all, as it would after a crash, and they come
back after a reboot or the next time the save daemon has to be started; a session that
ends while the save daemon isn't running comes back with the next command; a restored
session closed before Portal finished restoring it comes back within the same start, or
failing that at the next restore; and a
restored session whose saved scrollback Portal couldn't replay (it says where the file is
kept) is kept even once it ends, and comes back at every restore for as long as that file
can't be replayed.
Detaching, logging out and shutting down or rebooting the machine all leave your sessions
restorable, with nothing to run first.

Pair restoration with [resume hooks](#xctl-hook) to bring commands such as dev
servers and editors back after a reboot — on a panel you answer per session, or fired
outright for a registration pinned `eager`.

### Resume panel

A restored session whose resume hook resolves **lazy** — the shipped default — comes back
with its scrollback replayed underneath and a panel over it: `Resume session` with a
`● PAUSED` badge, the registered command under `ON RESUME`, and `⏎ resume` /
`d discard` / `esc back`. Nothing runs until you answer, so a reboot no longer starts every
registered command at once — a waiting session holds a small Portal process in its place,
and the command's own process starts only for the work you go back to.

| Key | On the panel | On the discard confirmation |
|---|---|---|
| `Enter` | Resume: the panel goes, the transcript is revealed, and the command runs in the session | — |
| `d` | Open the `▲ Discard resume?` confirmation | — |
| `y` | — | Discard the resume command |
| `Esc` | Back to the session picker; the session keeps waiting | Back to the panel |

- **Enter runs what is registered when you press it**, not what was registered when
  the panel was drawn. A hook removed or rewritten while the session waited is honoured;
  one that has gone drops the session to a plain shell.
- **A discard is permanent.** It removes that session's resume command from `hooks.json`,
  so the panel never returns — not on this boot, not after the next reboot. The session
  and its scrollback are untouched; the session drops to a plain shell. Only the
  command the confirmation showed is removed: if the hook was re-registered with a
  different command while the session waited, that one is left in place. The removed
  command is written to `portal.log` (`grep 'op=discard' portal.log`), so it can be
  copied back out.
- **Esc backs out without answering.** Every terminal showing the session through Portal
  goes back to the session picker. A terminal attached with a plain `zmx attach` is
  detached to its shell only when no Portal terminal shows the session; beside one, it
  stays attached, still showing the panel. The session keeps waiting — nothing about
  its resume changes, and the panel is there the next time you open it.
- **Every other key is swallowed**, including `Ctrl-C`, `Ctrl-Z` and `Ctrl-D`.
  `Ctrl-\` never reaches the panel: zmx takes it as its detach key, so it detaches, as
  it does in any session. A key counts only when it arrives on its own, so a stray
  paste can't answer the panel, and input already in flight when the confirmation
  opens can't confirm it.
- **Ignoring it is fine.** Detaching, closing the window and rebooting are not answers:
  the panel stays until you press a key that means something, and a reboot draws it
  again. Your other sessions stay fully live, and while it waits Portal holds the
  session's saved scrollback at what it was when the session was restored.
- **It repaints when you arrive.** Opening a waiting session, or switching to it, redraws
  its panel, so a panel drawn days ago at restore shows the theme — and the light or dark
  half of a pair — that is in force now.
- **A failed answer says so.** If an answer can't be carried out — `hooks.json`
  unreadable or locked on a discard, say — the panel states the reason on its own row
  and the session keeps waiting, so the key can be pressed again.
- A terminal too small for the card still shows the title, command and key hints, stacked
  plainly; the keys act at every size.

Waiting sessions show as a pending dot on their row in the picker, and
`xctl doctor` counts them. To go back to commands firing as their sessions are restored,
set `"resume_mode": "eager"` in `prefs.json` (there is no UI for this setting yet), or
pin a single registration with [`xctl hook set --resume-mode`](#xctl-hook).

## Configuration

Portal resolves its config directory using XDG: `$XDG_CONFIG_HOME/portal/` if set, otherwise `~/.config/portal/`. Each entry also has an env var override that takes full precedence. A relative path in any of these variables, `XDG_CONFIG_HOME` included, is read against the directory Portal was run from.

| File / directory | Purpose | Env override |
|---|---|---|
| `aliases` | Path aliases (key=value, one per line) | `PORTAL_ALIASES_FILE` |
| `projects.json` | Remembered project directories | `PORTAL_PROJECTS_FILE` |
| `hooks.json` | Per-session resume hooks (session name → event → registration). An event's value is either the command as a string or an object carrying the command alongside its settings; both shapes are permanently valid, neither deprecates the other, and there is no migration between them — the writer picks by whether there is anything to carry, so an entry that is only a command stays a plain string and a hand-edited file goes on looking as it does today. Portal creates an empty `hooks.json.lock` beside it on the first change, which it locks to keep concurrent writers from overwriting each other, and never removes. | `PORTAL_HOOKS_FILE` |
| `prefs.json` | UI preferences: last-used session-list grouping mode; the theme setting — `theme` for a pinned theme, or the `theme_light` / `theme_dark` pair; `resume_mode`, holding `eager` or `lazy` — the install-wide default a resume hook that names no mode of its own inherits; and `keys`, the [keys in a session](#keys-in-a-session) | `PORTAL_PREFS_FILE` |
| `terminals.json` | Host-terminal window recipes for [multi-select](#multi-select-mode) / multi-target `x` on custom terminals (Ghostty is built in). User-authored, read-only. | `PORTAL_TERMINALS_FILE` |
| `themes/` | Drop-in theme files (`<slug>.theme`); see [docs/theming.md](docs/theming.md). Portal never creates it — no directory simply means no drop-ins. | `PORTAL_THEMES_DIR` |
| `state/` | Saved sessions + scrollback for automatic restoration on reboot. Contains: `sessions.json` (the session index: each session's label, directories and creation), `scrollback/*.bin` (each session's saved scrollback, a file per creation), `daemon.pid` + `daemon.version` (liveness markers), `bootstrapped` (the record that the last bootstrap finished), `zmx-dir` (the zmx directory this state belongs to; see [The zmx directory](#the-zmx-directory)), `restoring`, `hydrating/` and `resume-pending/` (flags), the `*.lock` files, `runners/` (an entry and a control socket per attached Portal, for switching and new windows, and `takes/`, each session's last take's record and the lock its takes hold; see [One terminal at a time](#one-terminal-at-a-time)), `spawn-acks/` (each new window's receipt, cleaned once its burst counts it), `portal.log` (structured, rotating diagnostics; see [Logging](#logging)). See [Privacy Considerations](#privacy-considerations). | `PORTAL_STATE_DIR` |

Two of those overrides end in `_DIR` rather than `_FILE`: `themes/` and `state/` resolve a directory, not a single file.

Any of the files may be a symlink, such as one into a dotfiles repository. Portal writes through the link to the file it names, so the link survives Portal's own writes.

Projects are auto-populated when you create new sessions, pruned automatically by the daemon, and cleanable on demand with `xctl doctor --fix`.

**Theme.** Every colour Portal draws comes from the active theme, painted onto its own canvas so the palette always sits on the surface it was tuned for. Six themes ship — `tokyo-night`, `tokyo-night-day`, `nord`, `nord-wave`, `gitlab-dark-grey`, and `horizon` — and by default Portal picks a light or dark one by asking your terminal what colour its background is, falling back to dark if the terminal doesn't answer. Press `t` in the picker to choose a theme, or set `"theme": "<slug>"` in `prefs.json` to pin one whatever the terminal looks like; your own themes are `.theme` files dropped into the themes directory. Setting `NO_COLOR` to any non-empty value disables the canvas and renders on your terminal's native colours — with no colour to theme, `t` is unavailable there. See [docs/theming.md](docs/theming.md) for the colour roles, the file format, the naming rules, and how the light/dark pair is set.

<div align="center">
<img src="art/themes.webp" alt="The theme picker previewing each built-in theme over the sessions" width="840">
</div>

**Custom terminals (`terminals.json`).** Portal opens host windows natively on Ghostty. For any other terminal, add a recipe — see [docs/custom-terminals.md](docs/custom-terminals.md) for the full setup guide.

### The zmx directory

zmx keeps its sessions in the directory `ZMX_DIR` names (unset, `$XDG_RUNTIME_DIR/zmx`, else `zmx-<uid>` under `$TMPDIR`), and Portal uses the same one. A state directory belongs to one zmx directory: the first time Portal starts against it, it records the zmx directory in use in `state/zmx-dir`. A Portal run with another `ZMX_DIR` against that state stops before it records, saves or restores anything, naming both directories: run it with the recorded `ZMX_DIR`, or give it a state of its own with `PORTAL_STATE_DIR`. A state directory whose zmx directory is already served by another state directory's `_portal-saver` is refused too, sending that saver nothing and changing nothing in its state, and Portal stops with:

```
The _portal-saver running in this zmx directory saves Portal's state in <other>, not this Portal's state in <this>. Run it with PORTAL_STATE_DIR=<other>, or give it a zmx directory of its own with ZMX_DIR. If the state in <other> is gone, end its saver with zmx kill _portal-saver.
```

## Logging

Portal writes a structured diagnostic log to `state/portal.log` (under `PORTAL_STATE_DIR`). It is human-readable text with a `subsystem:` prefix on every line, so `grep "daemon:" portal.log` (or `restore:`, `saver:`, `hydrate:`, `spawn:`, `resolve:`, …) reconstructs what any subsystem did. `portal.log` is a symlink to a calendar-daily file (`portal.log.<date>`), so `tail -f portal.log` always follows today's log.

- **Rotation:** a new file each local day; older files are kept read-only. A size-cap safety valve rolls over to `portal.log.<date>.N` if a single day ever grows huge.
- **Retention:** rotated files older than 30 days are deleted automatically (one breadcrumb logged per deletion). `xctl doctor --fix` forces a sweep on demand.
- **Level:** defaults to `info` (a few lines per meaningful event). Set `PORTAL_LOG_LEVEL=debug` to capture full reconstruction detail when investigating an issue.
- **After a reboot:** `grep -E 'session dropped|drops-held' portal.log` shows what the saver did to your saved state. `session dropped session=<name>` names each session removed from it (a relabel is not a drop); `drops-held` means many sessions went missing at once, as in a crash or a zmx upgrade, and the saver kept every one of them for the next restore.

| Env var | Purpose | Default |
|---|---|---|
| `PORTAL_LOG_LEVEL` | Verbosity: `debug` / `info` / `warn` / `error` | `info` |
| `PORTAL_LOG_ROTATE_SIZE` | Per-day size cap before overflow (`K`/`M`/`G` suffix, e.g. `500M`, `1G`) | `500M` |
| `PORTAL_LOG_RETENTION_DAYS` | Days of rotated logs to keep | `30` |

## Privacy Considerations

Portal persists session scrollback to `~/.config/portal/state/` (override via
`PORTAL_STATE_DIR`) so it can rehydrate sessions after a reboot. Files are written
mode `0600`, directories `0700`.

- Same local-filesystem trust model as your shell history: anything visible in
  your terminal can end up in the saved state.
- **No encryption at rest.** If a session displays secrets (tokens, credentials,
  diffs of sensitive files), they will be captured.
- **`portal.log` records config changes verbatim.** It does not contain session
  scrollback, but config-mutation breadcrumbs and exec handoffs are logged as-is:
  a `xctl hook set --on-resume "<cmd>"` command string, a resume command
  discarded from the resume panel, alias values, and project paths appear in
  the log. Redact manually if you share it in a bug report.
- There is no per-session opt-out: a session's scrollback is saved whenever it
  changes, at most once a minute.

## Upgrading from the tmux version

Portal used to run on tmux. This version runs on zmx, saves its sessions in a new shape, and keys a resume hook by its session's name rather than by a tmux pane. `portal migrate` moves the tmux version's saved sessions and resume hooks across. Run it once, after installing this version and before anything else: until you do, every command but `version`, `init`, `doctor` and `current` stops on the tmux version's `sessions.json` with one line pointing at `portal migrate` (`doctor` reports it instead, and `current` only shows the session's name).

```bash
portal migrate
```

Until it has run, the tmux version's hooks and resume panels go on running in your tmux server, against this binary: its hooks do nothing, and a pane waiting on its resume panel stops answering and stays parked, its hook unrun. A new shell's `eval "$(portal init zsh)"` still works, and `x` tells you to run `portal migrate`. Go back to the tmux version and that pane's panel comes back at its next resize.

It runs in this order, and changes nothing it doesn't name:

1. Before it touches tmux, it makes every check that can be made up front: it reads `hooks.json`, the tmux version's `sessions.json` and zmx's sessions. If one of those checks stops it, nothing has changed, in tmux or on disk. A later step can still stop it once tmux is touched.
2. **The tmux side.** If a tmux server is running, it removes Portal's tmux hooks (and only Portal's) and ends `_portal-saver`. tmux and your tmux sessions keep running; end them when you're ready.
3. It waits, up to 30 seconds, for the tmux version's save daemon to save one last time and exit. It signals nothing itself. A daemon still running then stops the migration with nothing migrated, but Portal's tmux hooks and `_portal-saver` are already gone by then; running `portal migrate` again waits again.
4. It copies `hooks.json` to `hooks.json.tmux` in the state directory, unless that copy is already there; it never overwrites it.
5. It builds this version's state beside the old one, in `state-migrating/`: one session for each tmux pane, each keeping its tmux session's name, changed only where zmx refuses something, with the tmux name kept as its label where it changed; a second pane's session is numbered `-2`, and so on. A name zmx can't hold is made to fit rather than replaced: a character it refuses, such as `/`, becomes `-`, a leading `-` is dropped, and a name too long is cut. Only a name with nothing left, or a reserved one such as `logs`, gets a generated `{project}-{id}`. A name already taken keeps its stem and takes a fresh id, so `kiln` becomes `kiln-<id>`, and `atelier-m4TzpD` becomes `atelier-<new id>`. Each session keeps its directory and its saved scrollback.
6. It rewrites `hooks.json` from the copy: each resume hook keyed by a pane token moves to the name of that pane's session. Hooks keyed the older ways, by pane position or tmux pane id, haven't fired since pane tokens arrived, so they're removed; the copy keeps them.
7. It moves the tmux version's `state/` to `state-tmux/`, beside it, and the new state into `state/`.
8. It restores the sessions, then prints a report.

Each migrated session with a resume hook opens on its [resume panel](#resume-panel) the first time, whatever its hook's mode: your tmux sessions are still running their programs, and resuming the same command twice at once would collide. After that first restore, the hook's own mode applies. If Portal can't put a session on its panel, it opens at your shell, its hook not run.

The report leads with a summary and the restore: how many sessions it restored, and any it couldn't, by name: `x <name>` brings one back now, and otherwise they come back at the next restore after a reboot. When the restore stopped on an error, that holds once the error is fixed. It then lists every session, old name on the left and new on the right (`lumen-dev (window 1, pane 2) → lumen-dev-2`), each name it changed with why in plain words (`relay-v1/42 → relay-v1-42: '/' isn't allowed in a session name, so it became '-'`), dropped labels, skipped sessions and missing scrollback with the reason for each, and the hooks moved and removed, and ends with where `state-tmux/` and the `hooks.json` copy are.

Running it again is safe. Stopped part way, before the move, it starts over. Stopped between the two moves, it finishes them; until then every other command but `version`, `init`, `doctor` and `current` stops with one line pointing at `portal migrate`, and `doctor --fix` repairs nothing. Already migrated, it changes nothing, restores only what any Portal command would, and reports which sessions are still left to restore and where the tmux version's state is kept. With nothing of the tmux version's to migrate, it says so and exits `0`. It refuses, changing nothing, when both a tmux version's `state/` and an earlier migration's `state-tmux/` are there: move `state/` aside and keep the earlier migration's `state-tmux/`. It refuses too when `state-tmux/` is there but the state directory holds nothing it migrated. And a run that finds the `hooks.json.tmux` copy an earlier run made refuses, changing nothing, unless `hooks.json` is still that copy or what an earlier run rewrote it to: a hook registered since, say after going back to the tmux version, would be lost to the rewrite. Keep the hooks you want in `hooks.json`, move `hooks.json.tmux` aside, and run it again.

**The tmux version's hook keys.** The tmux version keyed a hook by its pane: a pane token, or in older installs `<session>:<window>.<pane>`. `xctl hook rm --pane-key` takes those keys verbatim too. A hook under one is stale, as any other, once its key names neither a session zmx lists nor one saved in `sessions.json`. But until migration has moved the tmux version's state to `state-tmux/`, beside the state directory, nothing prunes them. The state directory still holds the tmux version's `sessions.json`, which this version can't read, so [`xctl doctor`](#xctl-doctor)'s stale-hooks line reads not evaluable: `could not read sessions.json`. A state directory of this version beside a `hooks.json` that still holds the tmux version's keys, such as one `PORTAL_STATE_DIR` names, is held back too: the line counts nothing while any stale key has the shape of the tmux version's, and reads `hooks.json holds keys in the tmux version's shape`. Neither the daemon nor `--fix` prunes a hook while any of those would be pruned with it. Once `state-tmux/` is there, every hook is judged alike.

## Uninstall

Two paths depending on whether you want to keep your saved state:

- **Just remove the binary:** `brew uninstall portal` or `rm $(which portal)`.
  `brew uninstall portal` leaves zmx installed, as Homebrew leaves a
  dependency. Either way your sessions keep running in zmx, and a save daemon
  already running keeps running until its `_portal-saver` session ends. Your
  saved state is preserved, and reinstalling picks up where it left off.
- **Explicit teardown:** run `portal uninstall`, which stops the save daemon
  once it has saved. It touches **none of your files** — saved sessions and
  all config are left in place, and running `x` again brings the daemon back. To remove
  Portal completely, run `portal uninstall`, then uninstall the binary and
  delete `~/.config/portal/` (which holds both state and config) yourself.

## License

MIT
