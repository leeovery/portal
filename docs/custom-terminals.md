# Custom terminals (`terminals.json`)

Portal opens a fresh host-terminal window per target when you open more than one
session at once — the multi-target `x` burst (`x work api db`) and the picker's
[multi-select mode](../README.md#multi-select-mode). It drives **Ghostty** out of
the box. For any other terminal you teach Portal how to open a window with a
`terminals.json` recipe.

Single-target `portal open` (attach or switch in place) never needs this — only
the multi-window burst spawns host windows.

## When you need it

Every terminal other than Ghostty is "unsupported" until you add a recipe. You'll
see this two ways:

- The sessions picker shows a banner: `⚠ unsupported terminal — <name> · <bundleID>`.
- A multi-window burst on that terminal opens nothing.

A `terminals.json` recipe is the escape hatch: it tells Portal the exact command
that opens a new window in your terminal running a given command. Remote/mosh
sessions have no local window and are always unsupported — no recipe can change
that.

## Finding your terminal's identity key

Each recipe is keyed by your terminal's identity. Two ways to read it:

- **The picker banner.** On an unsupported terminal the sessions picker shows
  `⚠ unsupported terminal — <name> · <bundleID>`. The right-hand value is the raw
  macOS bundle id (e.g. `dev.warp.Warp-Stable`); the left is the friendly `.app`
  name (e.g. `Warp`).
- **`portal doctor`.** The report ends with a `host terminal` line naming the
  detected terminal and whether Portal can drive it — e.g. `Warp (unsupported)`,
  `Ghostty (supported)`, or `unsupported (remote session)`.

You can key a recipe by any of three forms:

| Form | Example | Notes |
|---|---|---|
| Raw bundle id | `dev.warp.Warp-Stable` | Exact match; the most specific form. |
| `.app` name / alias | `Warp`, `ghostty` | The friendly display name, or a built-in Portal alias (`ghostty`, `warp`). |
| Family glob | `dev.warp.Warp-*`, `*` | `*` matches any run of characters; `*` alone is a catch-all. |

### Key-matching precedence

Several keys can match the same terminal (say both `dev.warp.Warp-Stable` and
`dev.warp.Warp-*`). Portal always picks the single **most specific** match, in
this order:

1. Exact raw bundle id
2. `.app` name / friendly alias
3. Longer glob (more literal, non-`*` characters wins between two globs)
4. Bare `*`

A remote/mosh session resolves to unsupported *before* the config is consulted, so
a `*` catch-all never hijacks it onto the wrong machine.

## Recipe shape

Each entry declares an `open` capability as **exactly one** of two forms — `argv`
or `script`:

```json
// ~/.config/portal/terminals.json
{
  "<identity key>": {
    "commands": {
      "open": { /* argv OR script — never both, never neither */ }
    }
  }
}
```

### `argv` — an argument template

An array of arguments Portal execs directly. **At least one element must contain
the `{command}` placeholder**; Portal replaces it with the composed command as one
literal string. The element count never changes — Portal substitutes in place and
never shell-splits, so a multi-word command stays intact inside its element.

```json
{
  "dev.warp.Warp-*": {
    "commands": {
      "open": {
        "argv": ["osascript", "-e", "tell app \"Warp\" to create window with command \"{command}\""]
      }
    }
  }
}
```

### `script` — an executable file

A path to a script Portal execs directly, passing the composed command as the
single positional argument `$1`. There is no `{command}` token in script mode —
the command arrives structurally as `$1`. The script:

- carries its own shebang and executable bit (Portal runs it directly, never via
  `sh <path>`), and
- may start with `~`, which Portal expands.

```json
{
  "com.example.MyTerm": {
    "commands": {
      "open": { "script": "~/.config/portal/terminals/myterm.sh" }
    }
  }
}
```

```sh
#!/usr/bin/env bash
# $1 is the full command Portal wants the new window to run.
exec myterm --new-window --command "$1"
```

Make it executable:

```sh
chmod +x ~/.config/portal/terminals/myterm.sh
```

Declaring both `argv` and `script`, or neither, is a config typo — the entry is
skipped (see [Troubleshooting](#tolerant-decoding--troubleshooting)).

## `{command}` is self-sufficient

The command Portal hands your recipe already carries everything the spawned window
needs to reconnect on its own:

- It runs `portal open` on a pinned target with an explicit `PATH` injected, so the
  new window finds `portal` and `tmux` even in a bare host environment.
- `TMUX` and `TMUX_PANE` are stripped, so the spawned window runs **outside** tmux
  (a clean attach, not a nested switch).

You never need to set environment variables, source a profile, or plumb `PATH` in a
recipe — just open a window that runs the command Portal gives you.

## Tolerant decoding & troubleshooting

`terminals.json` never crashes the picker. Every failure degrades to "no custom
terminals" and Portal falls back to the built-in (native Ghostty) adapter, or to
unsupported when no native adapter matches:

| Situation | Behaviour |
|---|---|
| File absent | Normal unconfigured state — no warning. |
| File unreadable | File ignored; one `spawn:` WARN logged. |
| Malformed JSON | Whole file ignored; one `spawn:` WARN logged. |
| One invalid entry | That entry skipped; one `spawn:` WARN; other entries still load. |
| JSON `null` | Treated as an empty config. |

An entry is invalid when it declares both `argv` and `script` (or neither), an
`argv` with no `{command}` element, or a `script` path that doesn't exist or isn't
executable. A skipped entry falls through to the native adapter — never to a
less-specific config key.

Every rejection is a `spawn:` breadcrumb in the log (see
[Logging](../README.md#logging)) naming the entry and the reason:

```sh
grep "spawn:" ~/.config/portal/state/portal.log
```

`portal doctor`'s `host terminal` line confirms the outcome: once your recipe is
valid it flips from `<name> (unsupported)` to `<name> (supported)`.

## File location

- Default: `~/.config/portal/terminals.json` (or
  `$XDG_CONFIG_HOME/portal/terminals.json` when `XDG_CONFIG_HOME` is set).
- Override: set `PORTAL_TERMINALS_FILE` to an absolute path.

The file is user-authored and read-only to Portal — it is loaded once per picker
session and never written.
