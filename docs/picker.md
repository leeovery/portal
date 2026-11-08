# The picker

This page covers the TUI that `x` opens: its keys, the session rows, grouping and tags, the scrollback preview, and multi-select with its new windows.

- [Keys](#keys)
- [Mouse and touch](#mouse-and-touch)
- [Session rows](#session-rows)
- [Grouping and tags](#grouping-and-tags)
- [Scrollback preview](#scrollback-preview)
- [Multi-select and new windows](#multi-select-and-new-windows)
- [Themes](#themes)

The TUI has three views: the session list, the project picker, and the scrollback preview. It paints its own canvas in the colours of the active theme.

## Keys

The lists move with the **arrows only**, with no vim or page-jump aliases; the [preview](#scrollback-preview) scrolls its history with `j` / `k`. Press **`?`** on any page for a help modal listing that page's complete keymap.

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
| `r` | Rename session (sets its [label](sessions.md#labels)) |
| `k` | Kill session, after a confirmation |
| `e` | Edit the highlighted project: name, aliases, tags (Projects only) |
| `d` | Delete the highlighted project, after a confirmation (Projects only) |
| `n` | New session in the current directory |
| `?` | Show the full keymap for the current page |
| `q` / `Esc` | Quit (`Esc` clears an active filter first) |

A kill from the picker is final, as [`xctl kill`](commands.md#xctl-kill) is.

## Mouse and touch

The picker reads the mouse, so a wheel, a trackpad, or a swipe on a touch terminal moves the highlight through the sessions, projects and theme lists and scrolls the preview. A scroll stays with what it began on: momentum still running as you close the preview or a panel doesn't carry into the list behind it.

Tapping a session or project row opens it, as `Enter` would; in multi-select a tap marks the row instead, as `m` would. Selecting text while the picker is open takes your terminal's mouse-override modifier, such as `Shift`-drag in Ghostty.

## Session rows

A session row ends in up to two status dots, packed to the right:

- the **attached** dot, when a client is attached to the session;
- a warm-coloured **pending** dot, when the session is waiting on its [resume panel](resume.md#the-resume-panel).

Under `NO_COLOR` they render as `A` and `P` in the same cells. The `?` help on the sessions list carries the legend.

## Grouping and tags

By default the session list is flat and alphabetical. Press **`s`** on the sessions list to cycle the view through three modes:

- **Flat**: a single alphabetical list.
- **By Project**: a heading per directory, with each session listed once under its project name. Useful with no setup at all.
- **By Tag**: a heading per tag, with a session appearing under *each* tag its directory carries. Untagged sessions collect under a pinned **Untagged** group.

Portal remembers the last-used mode across launches in [`prefs.json`](configuration.md#prefsjson). Group headers are dimmed, non-selectable, and show a count, and the cursor only ever lands on sessions. While the `/` filter is active the list flattens to matching sessions and the headers step aside, returning when the filter clears.

### Tags live on directories

Tags belong to directories (projects), not to individual sessions. Every session opened in a directory inherits that directory's tags, so there is nothing to tag per session.

- Tags are freeform and trimmed, but **case-sensitive**: `Work` and `work` are different tags, and each is stored exactly as typed.
- Applying a tag to a second directory adds it to that group, and removing a tag's last use makes the group disappear.
- Only directories already opened in Portal are taggable, so open a directory once before tagging it.

### Managing tags

Open the projects picker (run `x`, then press `x` to switch from sessions to projects), highlight a project, and press `e` to edit it. The edit modal has **Name**, **Aliases**, and **Tags** fields.

- `Tab` (or `↑`/`↓`) moves between fields, and `←`/`→` moves between chips and the trailing `+ add` slot.
- To add a tag, land on `+ add`, press `Enter` (or `+`), type the tag, and press `Enter` to save.
- Press `x` on a chip to remove it.
- Every edit saves immediately, with no separate confirm step, and `Esc` never discards saved work: it backs out the current edit or closes the modal.

Projects are remembered as you create sessions; see [Configuration](configuration.md#projectsjson).

## Scrollback preview

`Space` on the highlighted session opens a Quick Look-style preview of that session's screen and scrollback, read live from the session, so you can tell similarly-named sessions apart without attaching. While it is open it reads the session again about once a second, so it follows the session as it changes, holding the place you scrolled to. A session waiting on its [resume panel](resume.md#the-resume-panel) shows the transcript it was restored from instead, since its own screen holds only the panel. The preview is read-only: opening and closing it changes nothing about the session.

| Key | Action |
|---|---|
| `↑` / `↓` | Previous / next session in the list (wraps) |
| `j` / `k`, `Ctrl+↑` / `Ctrl+↓`, `Home` / `End` | Scroll the history |
| `Enter` | Attach to this session |
| `Space` / `Esc` | Return to the sessions list |

The preview shows the session's screen with up to 10,000 lines of history above it, drawn as the terminal would draw it. The frame shows the session name and a footer of key hints, styled in a cyan "peek mode" so a preview never looks like the session itself.

- A session that doesn't answer in time renders `(session not answering)`.
- One whose history can't be read renders `(history unavailable)`.

Each stays only until the session gives a read: the preview keeps reading, so a session that comes back to life shows in it, and a read that fails once the preview shows the session leaves what it shows as it was.

## Multi-select and new windows

Press **`m`** on the sessions list to enter multi-select mode, which marks the highlighted session as your first selection. Press `m` again on any row to mark or unmark it. You can also sit in the mode with nothing selected: toggle the auto-marked row off, or enter while the cursor is on a group header.

Press **`Enter`** to open every marked session at once, each attached in its own host-terminal window. The result is **N windows for N sessions**: the picker reuses its own window for one of them and spawns the rest as new host windows, so there is never a leftover empty picker window. `Esc` cancels and clears the selection.

| Key | Action |
|---|---|
| `m` | Enter mode marking the highlighted session / toggle a row's mark |
| `↑` / `↓` | Move between sessions (marks persist) |
| `Space` | Preview the highlighted session's scrollback |
| `/` | Filter (marks persist underneath) |
| `Enter` | Open every marked session (one marked → a plain attach in place) |
| `Esc` | Cancel and clear the selection |

Marks are sticky across filtering, paging, regrouping, and the `Space`-preview round-trip; a session killed elsewhere while you were in the mode drops out of the selection. If a burst only partly succeeds, Portal leaves the windows that did open in place and keeps the failed sessions marked, so pressing `Enter` again retries only those. [`x` with several targets](sessions.md#several-sessions-at-once) does the same from the command line.

### Which terminals

Opening host windows needs a supported terminal. **Ghostty** works out of the box; other terminals are configured through [`terminals.json`](custom-terminals.md).

- On an unsupported terminal, Portal shows a banner naming the detected terminal and its bundle id, the key to copy into `terminals.json`.
- A remote or mosh client has no local window to name, and nor does a session when Portal can't tell which terminal it is shown in (several raw `zmx attach` windows on it, say), so the header stays as normal, with no banner.

On any of these, pressing **`m`** does **not** open multi-select. Portal shows a brief note that it isn't available (`over a remote connection`, `can't tell which terminal Portal is running in`, or `on this terminal`) rather than letting you mark sessions for a burst that can never run, and `m` drops out of the `?` help there. Opening a single session with `Enter` still works: it attaches in the current window and needs no host-window support. Run `xctl doctor` to see what Portal detects.

## Themes

Press `t` for the theme picker, or name a theme in [`prefs.json`](configuration.md#prefsjson). Six themes ship, and by default Portal picks a light or dark one by asking your terminal what colour its background is. `NO_COLOR` gives a colourless render, with `t` unavailable. See [Themes](theming.md) and [Theme](configuration.md#theme) in Configuration.
