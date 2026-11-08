# How it works

This page explains how Portal sits on zmx: how it opens sessions, how it saves them, how it brings them back after a reboot, which sessions it keeps and which it lets go, and how a state directory is tied to one zmx directory.

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

- [zmx](#zmx)
- [Opening](#opening)
- [Saving](#saving)
- [Bootstrap and restoration](#bootstrap-and-restoration)
- [What comes back, and what doesn't](#what-comes-back-and-what-doesnt)
- [The zmx directory](#the-zmx-directory)

[The design](design/zmx-backend.md) has how it is built, and why.

## zmx

[zmx](https://github.com/neurosnap/zmx) keeps the sessions; Portal finds, opens, saves and restores them. It talks to zmx through zmx's socket protocol 1, and needs zmx 1.0.1 or later. 1.0.0 was the first release to speak protocol 1, but its macOS downloads don't resize a session.

- Portal refuses a zmx older than 1.0.0 before it makes anything.
- It lists a session whose daemon speaks another protocol as not answering.

## Opening

- **Portal runs `zmx attach` itself** and stays running while you're attached, watching for [its own keys](sessions.md#keys-in-a-session).
- **Inside a session it switches** the terminal you typed in, so sessions never nest.
- **Opening a session takes it** from any other terminal showing it, so it is drawn for yours from the first frame. See [One terminal at a time](sessions.md#one-terminal-at-a-time).

How `x` decides what to open is in [Opening a session](sessions.md#opening-a-session).

## Saving

A daemon in a hidden `_portal-saver` session reads zmx's sessions every 5 seconds and writes what changed under `state/`: the sessions there are, each one's working directory and, at most once a minute, its ANSI scrollback. What `state/` holds is in [Configuration](configuration.md#state).

`_portal-saver` is a session you didn't make. It hosts the daemon, and shows in a raw `zmx list`; the picker and `xctl list` hide it.

## Bootstrap and restoration

Whenever you run a command that needs your sessions, Portal checks that its save daemon is running and brings it back if it is not. In the same step it re-creates the saved sessions that are not live: after a reboot every one, and after a logout or crash that ended your sessions along with the saver, those too.

- Each comes back under its own name, in its last working directory, with its scrollback (including ANSI colour) replayed.
- Each carrying a [resume hook](resume.md) comes back holding the resume panel, which runs the command when you answer it, or, for a registration pinned `eager`, fires the command outright.
- While that runs, the picker shows its loading page.

## What comes back, and what doesn't

Only a kill is final. Killing a session in Portal, with `k` in the picker or `xctl kill`, removes its saved state, scrollback and resume hook for good, and `xctl kill` on a saved session that isn't running discards it the same way.

A session ended outside Portal, by `zmx kill` or by exiting its program, goes the same way once the save daemon sees it gone, about ten seconds later, with four exceptions that err towards keeping it:

- **Several sessions ending at once.** The daemon keeps them all, as it would after a crash, and they come back after a reboot or the next time the save daemon has to be started.
- **No save daemon running.** A session that ends then comes back with the next command.
- **Closed mid-restore.** A restored session closed before Portal finished restoring it comes back within the same start, or failing that at the next restore.
- **Scrollback that can't be replayed.** A restored session whose saved scrollback Portal couldn't replay (it says where the file is kept) is kept even once it ends, and comes back at every restore for as long as that file can't be replayed.

Detaching, logging out and shutting down or rebooting the machine all leave your sessions restorable, with nothing to run first.

## The zmx directory

zmx keeps its sessions in the directory `ZMX_DIR` names (unset, `$XDG_RUNTIME_DIR/zmx`, else `zmx-<uid>` under `$TMPDIR`), and Portal uses the same one.

A state directory belongs to one zmx directory: the first time Portal starts against it, it records the zmx directory in use in `state/zmx-dir`. A Portal run with another `ZMX_DIR` against that state stops before it records, saves or restores anything, naming both directories: run it with the recorded `ZMX_DIR`, or give it a state of its own with `PORTAL_STATE_DIR`.

A state directory whose zmx directory is already served by another state directory's `_portal-saver` is refused too, sending that saver nothing and changing nothing in its state, and Portal stops with:

```
The _portal-saver running in this zmx directory saves Portal's state in <other>, not this Portal's state in <this>. Run it with PORTAL_STATE_DIR=<other>, or give it a zmx directory of its own with ZMX_DIR. If the state in <other> is gone, end its saver with zmx kill _portal-saver.
```
