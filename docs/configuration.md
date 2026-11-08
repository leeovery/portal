# Configuration

This page covers where Portal keeps its files and what each holds, `prefs.json`, the keys Portal watches while you're attached, the theme setting, logging, and what Portal's saved state means for your privacy.

- [Where Portal keeps its files](#where-portal-keeps-its-files)
- [The files](#the-files)
- [Keys in a session](#keys-in-a-session)
- [Theme](#theme)
- [Logging](#logging)
- [Privacy](#privacy)

## Where Portal keeps its files

Portal resolves its config directory using XDG: `$XDG_CONFIG_HOME/portal/` if set, otherwise `~/.config/portal/`. Each entry also has an environment variable that takes full precedence.

| File / directory | Purpose | Env override |
|---|---|---|
| [`aliases`](#aliases) | Path aliases | `PORTAL_ALIASES_FILE` |
| [`projects.json`](#projectsjson) | Remembered project directories | `PORTAL_PROJECTS_FILE` |
| [`hooks.json`](#hooksjson) | Per-session resume hooks | `PORTAL_HOOKS_FILE` |
| [`prefs.json`](#prefsjson) | UI preferences, the resume mode and the attach keys | `PORTAL_PREFS_FILE` |
| [`terminals.json`](#terminalsjson) | Host-terminal window recipes | `PORTAL_TERMINALS_FILE` |
| [`themes/`](#themes) | Drop-in theme files | `PORTAL_THEMES_DIR` |
| [`state/`](#state) | Saved sessions and scrollback, and Portal's log | `PORTAL_STATE_DIR` |

- Two of those overrides end in `_DIR` rather than `_FILE`: `themes/` and `state/` resolve a directory, not a single file.
- A relative path in any of these variables, `XDG_CONFIG_HOME` included, is read against the directory Portal was run from.
- Any of the files may be a symlink, such as one into a dotfiles repository. Portal writes through the link to the file it names, so the link survives Portal's own writes.

## The files

### `aliases`

Path aliases, `key=value`, one per line. `xctl alias` writes it; see [Aliases](sessions.md#aliases).

### `projects.json`

Remembered project directories, with their names and tags. Projects are added automatically when you create new sessions, pruned automatically by the daemon, and cleanable on demand with `xctl doctor --fix`. Edit a project in the picker; see [Managing tags](picker.md#managing-tags).

### `hooks.json`

Per-session [resume hooks](resume.md): session name → event → registration.

- An event's value is either the command as a string, or an object carrying the command alongside its settings. Both shapes are permanently valid, neither deprecates the other, and there is no migration between them.
- The writer picks by whether there is anything to carry, so an entry that is only a command stays a plain string, and a hand-edited file keeps the shape you gave it.
- Portal creates an empty `hooks.json.lock` beside it on the first change, which it locks to keep concurrent writers from overwriting each other, and never removes.

### `prefs.json`

UI preferences:

| Key | What it holds |
|---|---|
| `session_list_mode` | the last-used session-list [grouping mode](picker.md#grouping-and-tags) |
| `theme`, or `theme_light` / `theme_dark` | a pinned theme, or the light/dark pair; see [Theme](#theme) |
| `resume_mode` | `eager` or `lazy`: the install-wide default a resume hook that names no mode of its own inherits; see [Lazy or eager](resume.md#lazy-or-eager) |
| `keys` | the [keys in a session](#keys-in-a-session) |

### `terminals.json`

Host-terminal window recipes for [multi-select](picker.md#multi-select-and-new-windows) and multi-target `x` on custom terminals (Ghostty is built in). User-authored, read-only to Portal. Portal opens host windows natively on Ghostty; for any other terminal, add a recipe. [Custom terminals](custom-terminals.md) is the full setup guide.

### `themes/`

Drop-in theme files (`<slug>.theme`); see [Themes](theming.md). Portal never creates it: no directory means no drop-ins.

### `state/`

Saved sessions and scrollback for automatic restoration on reboot. It holds:

| Entry | What it is |
|---|---|
| `sessions.json` | the session index: each session's label, directories and creation |
| `scrollback/*.bin` | each session's saved scrollback, a file per creation |
| `daemon.pid`, `daemon.version` | liveness markers |
| `bootstrapped` | the record that the last bootstrap finished |
| `zmx-dir` | the zmx directory this state belongs to; see [The zmx directory](how-it-works.md#the-zmx-directory) |
| `restoring`, `hydrating/`, `resume-pending/` | flags |
| `*.lock` | lock files |
| `runners/` | an entry and a control socket per attached Portal, for switching and new windows, and `takes/`, each session's last take's record and the lock its takes hold; see [One terminal at a time](sessions.md#one-terminal-at-a-time) |
| `spawn-acks/` | each new window's receipt, cleaned once its burst counts it |
| `portal.log` | structured, rotating diagnostics; see [Logging](#logging) |

See [Privacy](#privacy) for what the saved state can hold.

## Keys in a session

Set `keys` in `prefs.json` to rebind the keys Portal watches while you're attached ([Keys in a session](sessions.md#keys-in-a-session) has the defaults). Each action takes one key or a list, an empty list turns it off, and an action you leave out keeps its default:

```json
"keys": {
  "detach": ["alt+d", "ctrl+o"],
  "detach_to_shell": "alt+shift+d",
  "kill": []
}
```

A key is `ctrl`, `alt` or `shift` plus a letter, digit, punctuation mark or `f1`–`f12`, and needs `ctrl` or `alt`, so a binding never swallows plain typing. Refused:

- `ctrl+shift+<letter>`, which sends the same bytes as `ctrl+<letter>`;
- `ctrl+\`, zmx's;
- shift on a digit or punctuation mark, or a mark typed with Shift such as `!`, whose bytes depend on the keyboard layout;
- `f3` with any modifiers, which reads as a cursor-position report;
- `ctrl+g` (BEL) and `alt+\` (ST), which end a terminal's replies;
- the few keys that send another key's bytes or begin an escape sequence (`ctrl+i` is Tab, `alt+[` begins one).

A refused or unparseable binding, or a key bound to two actions, keeps that action's default, and the log and `xctl doctor` say why. Changes apply from the next attach.

## Theme

Every colour Portal draws comes from the active theme, painted onto its own canvas so the palette always sits on the surface it was tuned for.

- Six themes ship: `tokyo-night`, `tokyo-night-day`, `nord`, `nord-wave`, `gitlab-dark-grey`, and `horizon`.
- By default Portal picks a light or dark one by asking your terminal what colour its background is, falling back to dark if the terminal doesn't answer.
- Press `t` in the picker to choose a theme, or set `"theme": "<slug>"` in `prefs.json` to pin one whatever the terminal looks like.
- Your own themes are `.theme` files dropped into the themes directory.
- Setting `NO_COLOR` to any non-empty value disables the canvas and renders on your terminal's native colours. With no colour to theme, `t` is unavailable there.

[Themes](theming.md) covers the colour roles, the file format, the naming rules, and how the light/dark pair is set.

## Logging

Portal writes a structured diagnostic log to `state/portal.log` (under `PORTAL_STATE_DIR`). It is human-readable text with a `subsystem:` prefix on every line. `portal.log` is a symlink to a calendar-daily file (`portal.log.<date>`), so `tail -f portal.log` always follows today's log. [Troubleshooting](troubleshooting.md#the-logs) says how to read it.

- **Rotation:** a new file each local day; older files are kept read-only. A size-cap safety valve rolls over to `portal.log.<date>.N` if a single day ever grows huge.
- **Retention:** rotated files older than 30 days are deleted automatically (one breadcrumb logged per deletion). `xctl doctor --fix` forces a sweep on demand.
- **Level:** defaults to `info` (a few lines per meaningful event). Set `PORTAL_LOG_LEVEL=debug` to capture full reconstruction detail when investigating an issue.

| Env var | Purpose | Default |
|---|---|---|
| `PORTAL_LOG_LEVEL` | Verbosity: `debug` / `info` / `warn` / `error` | `info` |
| `PORTAL_LOG_ROTATE_SIZE` | Per-day size cap before overflow (`K`/`M`/`G` suffix, e.g. `500M`, `1G`) | `500M` |
| `PORTAL_LOG_RETENTION_DAYS` | Days of rotated logs to keep | `30` |

## Privacy

Portal persists session scrollback to `~/.config/portal/state/` (override with `PORTAL_STATE_DIR`) so it can rehydrate sessions after a reboot. Files are written mode `0600`, directories `0700`.

- Same local-filesystem trust model as your shell history: anything visible in your terminal can end up in the saved state.
- **No encryption at rest.** If a session displays secrets (tokens, credentials, diffs of sensitive files), they will be captured.
- **`portal.log` records config changes verbatim.** It does not contain session scrollback, but config-mutation breadcrumbs and exec handoffs are logged as-is: a `xctl hook set --on-resume "<cmd>"` command string, a resume command discarded from the resume panel, alias values, and project paths appear in the log. Redact manually if you share it in a bug report.
- There is no per-session opt-out: a session's scrollback is saved whenever it changes, at most once a minute.
