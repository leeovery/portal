# Sessions

This page covers getting into a session: how `x` resolves what you type, the search form, aliases and zoxide, labels, the keys Portal watches while you're attached, and how a session is shown in one terminal at a time. Every flag is in [Commands](commands.md#x-open).

- [Opening a session](#opening-a-session)
- [Searching your sessions](#searching-your-sessions)
- [Aliases](#aliases)
- [zoxide](#zoxide)
- [Several sessions at once](#several-sessions-at-once)
- [Inside a session](#inside-a-session)
- [Labels](#labels)
- [Keys in a session](#keys-in-a-session)
- [One terminal at a time](#one-terminal-at-a-time)

## Opening a session

`x` with no arguments opens the [picker](picker.md). Given a target, it runs the precedence chain, and the first match wins:

**exact session name → kept saved session → path → alias → zoxide query.**

- **An exact session name** attaches that existing session.
- **A kept saved session**, the name of a saved session that isn't running and that the next restore would bring back, is brought back and attached. See [Kept sessions](#kept-sessions).
- **A path, alias or zoxide match** mints a brand-new session there. New sessions resolve to the git repository root when there is one.
- **Nothing:** a target that resolves to nothing is a hard failure. There is no fallback to the picker on a miss; the error points you at `-f`.

### Kept sessions

A kept session is brought back under its own name, with its scrollback and resume hook, then attached. `xctl doctor` lists such sessions.

- `-s` reaches it too, and so does a name with glob characters in it, read as that name rather than a glob. A glob never matches one.
- If it can't be brought back, one line says why and the session stays saved for the next restore.

While another Portal is starting, bringing a kept session back waits for it as a start does, and a wait that runs out fails with the same error a start gives.

### Minting, and pinning a domain

Directory targets always create. There is no find-or-create, so `x api` mints even while an `api-*` session runs; reach the existing one with the name, a glob, or `-s`.

A domain pin (`-s`, `-p`, `-a`, `-z`) skips the chain and forces one domain, and a pin that misses hard-fails without popping the picker. A command given with `-e` or after `--` runs only in a freshly minted session, never in one attached. The table is in [Commands](commands.md#x-open).

## Searching your sessions

A positional beginning with `/` and containing no further `/` is a search across your live sessions rather than a target to resolve: `x /port`. The rule tests the shape of the argument alone and never asks the filesystem anything, so the same command line means the same thing on every machine. Any other argument holding a slash resolves as a target: `x ./port`, `x ~/port`, `x /Users/me/Code/portal` and `x /tmp/` are paths (in the last, the trailing slash is the second one).

The term is matched **case-folded, as a contiguous run**, against each live session's label, name and directory, home-abbreviated (`~/Code/portal`) exactly as the picker's row displays it. What happens depends on how many sessions match:

| Matches | Outcome |
|---|---|
| exactly one | that session is attached outright, no picker |
| several | the picker opens pre-filtered by the term, cursor on the first match |
| none | the picker opens on the term with nothing surviving it: a filter result, not an error, so `Esc` shows your sessions |

In a picker a search opened, every row shows its recorded directory beside the session name, so a row that matched on its directory says why it is there. The column stays for that picker's life, however you edit the filter afterwards. A session list zmx can't give is not a zero match: the search reports the error and exits non-zero rather than opening an empty picker that would suggest your sessions are gone.

`x /`, the slash with no term, opens the picker with the filter empty, focused and ready to type. It is not an error, and it does not mean "mint at root".

**Stricter than the picker's filter.** The picker's own hand-typed `/` filter is fuzzy: scattered letters anywhere. The search form is deliberately stricter, because it can attach a lone match without ever showing it to you: a fuzzy `/port` could land you in `~/Projects/rust-tools` sight unseen.

**The cost.** A single-segment absolute directory typed without a trailing slash, such as `x /tmp` or `x /opt`, searches rather than minting. Mint there with the pin that exists for it: `x -p /tmp`.

**It stands alone.** A search composes with nothing: a second target, a trailing command, `-f`, or any domain pin on the same line is a usage error. Recognition stops at a `--` separator, so the words after it stay the trailing command's own (`x ~/Code/api -- ls /tmp` is unaffected).

**`-f` or `/term`?** They differ by outcome. `-f` always opens the picker, which makes it the form for a script or a keybinding that must land in the same place every time. `/term` takes you straight to the session when only one matches, which makes it the interactive form.

## Aliases

An alias names a directory, so `x work` mints a session there. Aliases come after paths in the chain, and `-a <key>` pins the alias domain, taking a key or a key glob.

```bash
xctl alias set work ~/Code/work      # create alias
x work                               # mint a new session there
xctl alias rm work                   # remove alias
xctl alias list                      # list all aliases
```

They are kept in the `aliases` file, one `key=value` to a line; see [Configuration](configuration.md#where-portal-keeps-its-files). A project's aliases can also be edited in the picker's [edit modal](picker.md#managing-tags).

## zoxide

The last step of the chain asks [zoxide](https://github.com/ajeetdsouza/zoxide) for its best match, and mints a session there. `-z <query>` pins the zoxide domain, and errors if zoxide isn't installed.

## Several sessions at once

Two or more targets, or one glob expanding to several sessions, open a portal to each: this terminal becomes the first and the rest open in their own host-terminal windows, **N windows for N targets**. Pins and bare targets mix freely (`x -s api -p ~/Code/new blog`), and a command rides only the minted sessions. The extra windows need a supported terminal; see [Which terminals](picker.md#which-terminals). The picker's [multi-select](picker.md#multi-select-and-new-windows) does the same from the list.

## Inside a session

Portal runs `zmx attach` itself and stays running while you're attached, watching for [its own keys](#keys-in-a-session). Inside a session, Portal switches the terminal you typed in rather than attaching within it, so sessions never nest. When the session itself ends, Portal exits to the shell as plain `zmx attach` would.

## Labels

A session's name is zmx's and never changes. What the picker shows is its label, which starts as the name. Set it from the picker's `r` modal, or with [`xctl rename`](commands.md#xctl-rename) inside the session. A relabelled session keeps its [resume hook](resume.md), which is keyed by the name.

A label takes any printable text. Only a label holding a control or formatting character, such as a newline, tab or escape, is refused, by the picker's `r` modal and `xctl rename` alike: `label refused: it holds a control or formatting character, such as a newline, tab or escape`. The picker says exactly that and the session keeps its label; a hook registered against it is untouched. Empty text, or the session's own name, resets the label to the name.

## Keys in a session

Portal stays running while you're attached, watching for three keys of its own. Everything else reaches the session untouched.

| Key | Action |
|---|---|
| `Alt+D` | Detach, back to the picker |
| `Alt+Shift+D` | Detach, straight to the shell |
| `Alt+Q` | Ask to kill the session: the picker opens on its kill prompt |

On that prompt `y` kills the session and stays in the picker, `d` keeps it and stays in the picker, and `Esc` keeps it and goes back into it, so a stray `Alt+Q` costs a key, never a session. When Portal can't be sure the terminal still shows the session (a `zmx attach` typed inside it switched it), the picker opens without the prompt and says why.

zmx's own `Ctrl+\` detaches too, and always to the shell. Portal detaches zmx through that key, so under Portal it can't be turned off: `ZMX_NO_DETACH_KEY` isn't honoured.

To bind other keys, or turn one off, see [Rebinding the attach keys](configuration.md#rebinding-the-attach-keys).

### macOS

The terminal must send Option as Alt, or `Option+D` types `∂`:

| Terminal | Setting |
|---|---|
| Ghostty | `macos-option-as-alt` |
| kitty | `macos_option_as_alt` |
| iTerm2 | Esc+ for the Option key |
| Terminal | "Use Option as Meta key" |
| Blink | Option-as-Meta |

### Two quirks

On terminals that send keys the older way, without the newer keyboard encoding:

- With Caps Lock on, `Alt+D` arrives as `Alt+Shift+D` and detaches to the shell, and `Alt+Q` arrives as `Alt+Shift+Q` and reaches the session.
- `Alt+<key>` is sent as Escape then the key. Over a slow link, an Escape typed just before a bound letter can arrive the same way and count as the Alt key: Escape then `q` opens the kill prompt.

### Portal over ssh from a Portal session

These keys act on the local session: the Portal you attached through catches them before ssh sees them, so a Portal on the remote machine gets none. Nothing is lost, since a detach only detaches and `Alt+Q` only asks. To drive the remote Portal's keys, bind other ones in the remote machine's `prefs.json`; the local Portal passes them through.

## One terminal at a time

Opening a session takes it from every other terminal showing it, before Portal attaches, so the session is sized and drawn for the terminal you opened it in from its first frame. Every attach Portal makes does it: `x <name>`, a pick in the picker, a switch typed inside a Portal attach, and going back in from the kill prompt or the screen below.

- Nothing is killed and nothing is typed into the session.
- Portal's own `_` sessions are never taken.
- Only terminals are taken. What reads a session without one, such as Portal's saver or a preview, never makes Portal take it, though a take does close a read under way at that moment, which then reads again (a preview keeps what it shows meanwhile).
- Two terminals opening one session at once end with the later one attached.

### The taken screen

A plain `zmx attach` on the session is detached to its shell. A Portal attached there lands on a screen saying where the session went:

```
⇄ Open on another device

release checklist

Taken by SSH from 192.168.1.20.

⏎ take it back   space watch   esc picker
```

The last line of the message reads `SSH from <address>` when the session was taken over ssh, `<App> on this Mac` when the taking terminal names its app, such as `Ghostty on this Mac`, and `another terminal on this Mac` otherwise.

| Key | Action |
|---|---|
| `Enter` | Take the session back |
| `Space` | Watch it in the [preview](picker.md#scrollback-preview), live and read-only; `Esc` comes back to this screen |
| `Esc` | Go to the picker, its cursor on the session |

A detach from anywhere else lands on the same screen, without the line saying where, only while another terminal has the session. One that leaves no terminal on it, such as `zmx detach` typed in the session, exits to the shell.
