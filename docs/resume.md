# Resume hooks

This page covers bringing a session's command back after a reboot: registering a resume hook, when it fires, lazy and eager modes, how hooks are kept and cleaned up, and the resume panel a restored session holds. The flags are in [Commands](commands.md#xctl-hook).

- [What a resume hook is](#what-a-resume-hook-is)
- [When hooks fire](#when-hooks-fire)
- [Lazy or eager](#lazy-or-eager)
- [Hooks and sessions](#hooks-and-sessions)
- [The resume panel](#the-resume-panel)

## What a resume hook is

[Restoration](how-it-works.md#bootstrap-and-restoration) brings a session back in its last working directory with its scrollback, but not the command that was running in it. A resume hook is that command, registered against the session, so a dev server or an editor comes back too.

```bash
xctl hook set --on-resume "npm start"            # inside the session
xctl hook list                                   # every hook
xctl hook rm --on-resume                         # remove the current session's hook
```

Under the shipped default a restored session comes back holding the [resume panel](#the-resume-panel), showing the registered command with `⏎ resume`, `d discard` and `esc back`, and the command runs when you answer it. A registration pinned `eager` runs its command as the session is restored, with no panel.

## When hooks fire

A resume hook is only ever offered, as the panel or fired outright under `eager`, when Portal recreates a session from saved state: after a reboot, or once a logout or crash has ended your sessions along with the saver. Nothing happens on an ordinary detach and reattach, because the session and its process are still alive; bringing the command back then would launch a second copy of a long-running command such as a dev server.

## Lazy or eager

A hook resolves to one of two modes:

- **lazy**, the shipped default: the session comes back holding the resume panel, and the command runs when you answer it.
- **eager**: the command runs as the session is restored, with no panel.

The mode comes from the registration, if it names one (`xctl hook set --resume-mode eager|lazy`); otherwise from `resume_mode` in [`prefs.json`](configuration.md#prefsjson); otherwise **lazy**. To have every command fire as its session is restored, set `"resume_mode": "eager"` in `prefs.json` (there is no UI for this setting yet), or pin a single registration with `--resume-mode`. `xctl doctor` reports the install's resume mode.

## Hooks and sessions

**Hooks are keyed by the session's name**, which never changes. Renaming a session sets its [label](sessions.md#labels), from the picker's `r` modal or `xctl rename` inside the session, and leaves the name alone, so a relabelled session still brings its command back after the next reboot: as the panel under the shipped default, and as a fired command under `eager`.

**A hook goes with its session.** Killing the session, or the session ending for good, removes it. A hook left behind is stale once its key names neither a session zmx lists, answering or not, nor one Portal has saved in `sessions.json`. The save daemon removes stale hooks once an hour, and `xctl doctor --fix` removes them at once.

Both leave every hook alone:

- while a restore runs;
- while no session is listed and none is saved, so a state directory pointed somewhere fresh never costs the hooks in `hooks.json`;
- while the session list, `sessions.json` or the restoring flag can't be read;
- when one sweep would remove three hooks or more that are at least half of `hooks.json`.

Since a hook goes with its session, that last case only happens when `hooks.json` is judged against another state or zmx directory, such as a scratch `PORTAL_STATE_DIR` or `ZMX_DIR` beside the real file, and `portal.log` says so with a warning.

Each removed command is written to `portal.log` (`grep 'op=clean-stale' portal.log`), so it can be copied back out. [Troubleshooting](troubleshooting.md#doctor---fix) lists what `doctor --fix` reports when it removes nothing.

## The resume panel

A restored session whose resume hook resolves **lazy** comes back with its scrollback replayed underneath and a panel over it: `Resume session` with a `● PAUSED` badge, the registered command under `ON RESUME`, and `⏎ resume` / `d discard` / `esc back`. Nothing runs until you answer, so a reboot doesn't start every registered command at once: a waiting session holds a small Portal process in its place, and the command's own process starts only for the work you go back to.

| Key | On the panel | On the discard confirmation |
|---|---|---|
| `Enter` | Resume: the panel goes, the transcript is revealed, and the command runs in the session | — |
| `d` | Open the `▲ Discard resume?` confirmation | — |
| `y` | — | Discard the resume command |
| `Esc` | Back to the session picker; the session keeps waiting | Back to the panel |

### Answering it

- **Enter runs what is registered when you press it**, not what was registered when the panel was drawn. A hook removed or rewritten while the session waited is honoured; one that has gone drops the session to a plain shell.
- **A discard is permanent.** It removes that session's resume command from `hooks.json`, so the panel never returns, not on this boot and not after the next reboot. The session and its scrollback are untouched; the session drops to a plain shell. Only the command the confirmation showed is removed: if the hook was registered again with a different command while the session waited, that one is left in place. The removed command is written to `portal.log` (`grep 'op=discard' portal.log`), so it can be copied back out.
- **Esc backs out without answering.** Every terminal showing the session through Portal goes back to the session picker. A terminal attached with a plain `zmx attach` is detached to its shell only when no Portal terminal shows the session; beside one, it stays attached, still showing the panel. The session keeps waiting: nothing about its resume changes, and the panel is there the next time you open it.
- **A failed answer says so.** If an answer can't be carried out (`hooks.json` unreadable or locked on a discard, say), the panel states the reason on its own row and the session keeps waiting, so the key can be pressed again.

### What doesn't answer it

- **Every other key is swallowed**, including `Ctrl-C`, `Ctrl-Z` and `Ctrl-D`. `Ctrl-\` never reaches the panel: zmx takes it as its detach key, so it detaches, as it does in any session.
- **A key counts only when it arrives on its own**, so a stray paste can't answer the panel, and input already in flight when the confirmation opens can't confirm it.
- **Keys typed before it appears don't count.** Anything typed while a restored session is still replaying its scrollback, before its panel is first drawn, is dropped, so a stray `Enter` or `d` can't answer a panel you haven't seen.
- **Ignoring it is fine.** Detaching, closing the window and rebooting are not answers: the panel stays until you press a key that means something, and a reboot draws it again. Your other sessions stay fully live, and while it waits Portal holds the session's saved scrollback at what it was when the session was restored.

### How it draws

- **It repaints when you arrive.** Opening a waiting session, or switching to it, redraws its panel, so a panel drawn days ago at restore shows the theme, and the light or dark half of a pair, that is in force now. A key you press as it repaints still counts, so `Enter` the moment the session opens resumes it.
- **Over a slow link it may wait a moment.** Under a light/dark pair the panel asks your terminal its background each time it draws, and over a slow link, such as SSH from a phone, the answer can arrive after you have already pressed `Enter`. Portal then waits for it, at most a second after the panel was drawn, before the command (or, after a discard, the shell) starts, so the answer never lands there as stray text; a key typed in that moment is dropped.
- **A small terminal** too small for the card still shows the title, command and key hints, stacked plainly; the keys act at every size.

Waiting sessions show as a pending dot on their row in the [picker](picker.md#session-rows), and `xctl doctor` counts them. [Themes](theming.md) covers how the panel picks its theme.
