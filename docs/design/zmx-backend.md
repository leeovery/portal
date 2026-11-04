# Portal on zmx

Portal moves from tmux to [zmx](https://github.com/neurosnap/zmx). Each session becomes one zmx session: a background process that holds the program and a copy of its screen, with bytes passed straight to the terminal. The terminal (Ghostty, rootshell, Blink) renders natively and keeps its own scrollback. tmux is removed at the end; nothing runs both.

Board: sitrep `sr-fpq124` (Portal on zmx). Branch: `zmx-backend`. Base: v0.12.5.

## Settled

- **zmx only.** No dual backend. Rollback means reinstalling v0.12.5 and putting back `state-tmux/` and the saved `hooks.json` (see Migration).
- **One interface, at Portal's level.** Portal code talks to a `Backend` that speaks in Portal's terms: a session is a persistent terminal with a name, a directory and a command. Only a zmx implementation is built. Windows, panes, layouts, hooks, servers and options exist nowhere above the driver. A later tmux driver would map one Portal session to one single-pane tmux session.
- **Portal stores nothing on zmx sessions.** Everything Portal knows about a session is in Portal's own state, keyed by the session name. zmx's key=value tags (`zmx set`, `zmx clear`) are never used.
- **Scrollback is still saved and restored.** zmx keeps scrollback in memory only, so a reboot still loses it.
- **Kills.** A session you didn't kill comes back after a reboot. A killed session normally stays gone, but nothing heavy is built to guarantee it.
- **Keys.** Portal runs `zmx attach` itself and adds these keys, the same from every terminal, each configurable (see Key config):
  - **Alt+D:** detach, back to the picker.
  - **Alt+Shift+D:** detach, straight to the shell.
  - **Alt+Q:** ask to kill the session. It detaches to the picker, which opens on the kill prompt for that session: `y` kills it, Esc keeps it and stays in the picker, Enter keeps it and attaches it again. A mistaken Alt+Q costs a key, never a session.

  zmx's own `ctrl+\` detaches the way plain zmx does, straight to the shell, never the picker. The runner wraps `zmx attach` rather than speaking zmx's attach protocol itself; no floating menu, no pinned status bar.
- **Picker rows.** No window count. The session label, plus the attached and waiting dots.
- **Preview (Space).** The session's live screen and history from zmx, up to zmx's 10,000 lines, instead of the saved file. Window and pane cycling go.
- **Opening several sessions in new windows (`m`).** Works as today: Ghostty on the Mac opens them, and Blink and rootshell get today's refusal. See Attach runner.
- **Esc on the resume panel.** Detaches to the picker, as today.
- **Version.** Lee's call at release.
- **Out of scope:** the workflows tool.
- **zmx's socket protocol, on official zmx.** The driver speaks zmx's socket protocol (frozen on main on 16 Oct 2026, protocol version 1) for listing, kill, detach and history, so nothing parses text meant for people; zmx declined machine-readable CLI output (#220). Creating and attaching still run the `zmx` binary, since creation isn't a protocol message. Portal runs on official zmx, installed as a Homebrew dependency of Portal's formula (`neurosnap/tap/zmx`); it bundles, pins and forks none. Homebrew can't hold a dependency to a version range, so zmx upgrades freely: Portal names the sessions whose daemons speak a protocol it doesn't, and a zmx release that changes the protocol is caught up with in a Portal release, as a tmux change would have been. The README states the zmx release Portal supports. A weekly GitHub Action builds zmx's latest release and runs the backend contract suite against it, so a breaking release shows up before users meet it. Development and tests use a build of the main commit pinned in `scripts/zmx-commit`, so Portal on zmx releases once a zmx release carries protocol 1. Main still reports its version as 0.8.1, so support is judged by the protocol version, never the version string.
- **Leave Portal better than found.** The move is also the refactor: simpler structure, lower complexity, cleaner code. Each phase simplifies what it touches, and nothing tmux-shaped survives above the driver.
- **Output quality.** Rendered output is tested properly, and the README gets a demo recording to the standard Switchboard's has. Switchboard grew its capture and VHS tooling out of Portal's harness; Portal borrows it back (see Output and visuals).

## Names

- **Session name:** the zmx session's name, such as `dotfiles-PpaTER`. Portal sets it once, at creation, and it never changes, since zmx has no rename. Portal-made names carry a nanoid, so they're unique. It keys everything Portal stores about the session: the index, `hooks.json` and the scrollback file. Restore recreates a session under the same name. zmx itself and rootshell's own zmx picker show it.
- **Session label:** what Portal shows: the picker, the preview and `portal current`, which footers and Claude's `--name` read. It defaults to the session name and takes any text. `r` in the picker sets it, and so does `portal rename <text>` from inside the session. It lives in Portal's index.
- **Telling a recreated session apart.** A session that ends and is recreated under the same name outside Portal is a new session. Portal tells it apart by zmx's created time, which the index records and nothing can clear. A session whose created time differs from its entry, and that Portal didn't create or restore, starts clean, without the old session's label, hook or scrollback. Every claim of an entry (a mint, a rename, `hook set`, the saver) judges by one rule, `state.ClaimRule`: an entry is replaced, and what it kept forgotten, only when both creations are known, both are from this boot, and they differ. An entry recording no creation, one saved before this boot (with a few seconds' leeway, as the boot time is read against the wall clock), and one whose session has its hydrating or resume-pending flag raised is taken over, keeping its label, dirs, scrollback and hook. A claimer that knows no creation leaves the entry as it is, and a rename or `hook set` refuses a session that reports none. What a replaced or removed entry kept is forgotten under the commit lock its write held, so no claim lands between the write and the forgetting. `hook set` writes its hook under its claim's lock as well, once that forgetting is done, so no kill lands between the claim and the hook, and the forgetting of an earlier creation's entry never takes the hook just set.

## Backend interface

`internal/backend` defines the interface and the session model. `internal/backend/zmx` implements it over zmx's socket protocol, running the `zmx` binary only to create and attach. A fake implementation backs unit tests. Every method that does I/O takes a context, with per-operation ceilings beneath it.

- `List(ctx)` returns every session in zmx's dir, found from its sockets and probed concurrently for name, cwd, attached client count, pid, created time and command. A session that doesn't answer is kept with only its name (`Answered` false), never dropped from the list. A socket that refuses is stale only when it predates this boot, and is then left out; one from this boot is kept as not answering. A session that doesn't answer, whether its socket refuses or its daemon stays silent, fails every per-session call with a timeout, and `Create` refuses its name. So a crashed daemon holds its name until the next reboot, unless a plain `zmx list` or `zmx attach` (which delete any refused socket) clears it. Portal never deletes zmx's files. Only an unreadable dir is an error; a missing one means no sessions. Sessions whose names start with `_` are Portal's own and are hidden from listings.
- `Exists(ctx, name)`.
- `Create(ctx, name, dir, command)` creates a detached session: `zmx attach <name> <cmd…>` with no terminal attached, succeeding only when zmx reports it created the session. A name already held, under any spelling the filesystem folds together, is refused with the held name. It starts 120×24 and reflows on attach.
- `AttachCommand(ctx, name)` returns the argv the attach runner starts.
- `Kill(ctx, name)`: done only when the session's socket is gone; a timeout says the kill may still land.
- `Detach(ctx, name)` detaches every client of a session.
- `History(ctx, name, rows)` returns the screen plus up to `rows` of scrollback (all of it when `rows` is 0), with colours, through the protocol's bounded capture. Never empty: a daemon that doesn't answer is an error.
- `Current()` returns the session this process runs in, from `$ZMX_SESSION`.

Names follow zmx's limits. A socket path is capped at 104 bytes, which leaves about 70 bytes of name with `ZMX_DIR=$HOME/.local/state/zmx`. Generated names stay `{project}-{nanoid}`.

## State

- The zmx version owns `<config>/portal/state/`. `portal migrate` moves the tmux version's `state/` to `state-tmux/` before anything writes to `state/`.
- While `state/` still holds a version 1 `sessions.json`, every command except `migrate` and `version` stops with one line pointing at `portal migrate`. The hidden `state` helpers a restored session runs (hydrate and the resume chain) are exempt too: a refused helper exits, and the session it runs in ends with it. `doctor` is exempt as well, so it can report a version 1, corrupt or newer `sessions.json` itself.
- `sessions.json` version 2: `{version, saved_at, sessions: [{name, label, dir, cwd, created_at, pid, scrollback_file}]}`. `label` is omitted while it's the default. Members a version doesn't model, on the root or an entry, are kept through every rewrite. Rename writes through the same locked commit as the saver.
- A session's files are named by its stem, `<name>-<hash>`, the hash the first 12 hex digits of the SHA-256 of the exact name, so names a filesystem folds together (`Foo`, `foo`) never share a file.
- `scrollback/<stem>-<creation>.bin`, the creation its UTC created time and pid, so each creation of a name writes its own and forgetting one removes no other's. The entry names its file; forgetting an entry removes only that one. All positional re-filing goes.
- Per-session flags, as files named by the session's stem:
  - `hydrating/<stem>`: written before restore creates the session, removed when the scrollback replay finishes.
  - `resume-pending/<stem>`: present while the resume panel waits.

  A session's flags are removed when it's dropped from the index, and bootstrap removes any for sessions that aren't live.
- Flags that were tmux server options become files:
  - `restoring`
  - `bootstrapped`: holds the boot id and the version, replacing the per-server latch.
  - `spawn-acks/`: one empty file, `<batch>-<token>`, per window a burst opened, written by that window once its attach has begun. The burst removes its own, and any an hour old, as a burst that died leaves them. The directory is 0700 and each file 0600.
  - `runners/` (see Attach runner)
- `hooks.json` stays where it is, keyed by session name. `prefs.json`, projects and aliases are unchanged.
- Every file Portal replaces is replaced in order: the temp file is synced before its rename and the directory after it, so after a power loss each file is its old version or its new one, never empty, and no later write persists ahead of a rename. On darwin both syncs are `F_BARRIERFSYNC`, which orders without flushing the drive's cache (fsync where a filesystem has no barriers); on linux, fsync. That costs about 0.5 ms a write on an M1 Max's internal SSD, where `F_FULLFSYNC` cost 10 ms. A filesystem that makes no such sync is written to without it. A write has landed once its rename has: a directory sync that fails after it is a warning, never an error, so no caller undoes or repeats a change already in place.
- Before switch-over, a dev build runs beside the installed tmux Portal with `PORTAL_STATE_DIR` and `PORTAL_HOOKS_FILE` pointed at scratch paths.

## Saver

- **Host.** `portal state daemon` runs inside a `_portal-saver` zmx session, which keeps it alive the way tmux does today. The singleton lock, pid and version files and the orphan sweep stay. Self-supervision compares the saver session's pid from `List()`.
- **Index.** Polled every 5 s; committed when it changes. Portal records the created time and pid of every session it creates, restore included; zmx keeps created time in whole seconds, so the pair tells a recreated session apart (see Names).
- **Scrollback.** Captured at most once a minute per session, and only when its content has changed. Sessions flagged hydrating or resume-pending are skipped. A poll starts no capture once about 3 s of its captures have passed, and never cuts one short; a session it doesn't reach is captured at the next poll.
- **Drops.**
  - A session missing from two polls at least 10 s apart is dropped. An entry created before this boot, or recording no creation, is never dropped, so a session whose restore failed comes back at the next bootstrap.
  - If at least three of the entries the saver may drop, and at least half of them, are missing at once, whether they went in one poll or over several, it drops none of them while it runs, unless one is listed again, and logs a WARN. That covers a zmx upgrade or a mass crash. The entries it holds count no further, so a session ending on its own later is still dropped.
  - A kill from inside Portal commits at once.
- **Removed, because they only existed for tmux:**
  - the "own server answers" check and the empty-capture check,
  - positional pane keys, pane tokens and the re-filing of moved transcripts,
  - rename detection,
  - the environment capture.

## Restore and resume

- **Restore** writes the hydrating flag, then creates each saved session that isn't live with `Create(name, dir, portal state hydrate --session N [--file S])`.
- **The helper** replays the transcript straight away; there's no FIFO, because nothing else races it. It then removes the hydrating flag and runs the eager hook, the resume chain or the shell, exactly as today.
- **The resume chain** keeps its draw, wait and recover steps, its keys and its discard. Its tmux calls become flag-file reads and writes. The alternate-screen pin goes. Esc asks the session's runners to detach to the picker; with no runner attached, it detaches through the backend.
- **Repaint.** Before attaching, the attach runner sends SIGWINCH to a waiting session's process group, so the panel redraws in the current theme. This replaces the attach hook.

## Bootstrap

1. Check `zmx` is present and its version is supported. Fatal otherwise.
2. Write the `restoring` file.
3. Sweep orphaned daemons and stale flags.
4. Ensure `_portal-saver`.
5. Restore.
6. Remove `restoring`.
7. Write the `bootstrapped` latch.

The concurrent cold-path picker and its progress page stay, with the shorter step list. The tmux server anchor, hook registration, eager FIFO signalling and stale-marker sweeps go.

## Attach runner

Portal starts `zmx attach <name>` as a child in a pseudo-terminal it owns:
- It puts the real terminal in raw mode and copies bytes both ways unchanged.
- It forwards size changes to the child.
- It watches input for the bound keys, each recognised in both its legacy escape form and its newer encoded (CSI-u) form:
  - **detach:** writes `ctrl+\` to the child, which detaches; the picker follows.
  - **detach to shell:** the same, then Portal exits to the shell.
  - **kill:** detaches as detach does; the picker follows, open on the kill prompt for the session (see Keys). Unsure which session the terminal shows (see Runner registry), the runner first confirms through zmx; unconfirmed, the picker opens without the prompt and flashes why.
- The picker follows only when Portal asked for it: one of its keys, or Esc on the resume panel through the registry. Any other end of the child (`ctrl+\`, a `zmx detach` from elsewhere, the session ending) exits to the shell, as plain `zmx attach` does.
- A switch asked for through the registry ends the attach as detach does, and the session it names attaches next, with no picker between.
- When the attach ends, Portal resets the terminal unless zmx's own reset is known to have reached it. Restoring the terminal (`TCSAFLUSH`) waits for the terminal to take its output, reset or not, and ends early only if the terminal hangs up.
- The picker that follows is the plain one: no filter, no search, no pending command, no bootstrap, even when the first picker carried a pending command or a search, which is spent by then. Picking attaches again, and quitting exits 0. A pick, a switch or the kill prompt's Enter whose session can't be attached for a fault of its own (gone, held under another spelling, refusing connections or not answering) reopens it, with why, worded as `portal kill` words it; that holds for every pick, the first picker's included. A kill key refused because the runner can't confirm its session reopens it on that refusal, with no kill prompt. A failed kill at the prompt's `y` flashes there, as `k`'s does.
- **Portal inside Portal over ssh.** From a session Portal attached, ssh to another machine and run Portal there, and every key passes through the local runner first: the bound keys act on the local session, never reaching the remote Portal. Nothing is lost, since a detach only detaches and the kill key asks. Documented, not built around: the remote machine's `prefs.json` can bind other keys, which the local runner passes through.
- Every other end exits with the runner's code, silently: the child's, or 128 plus a signal's. zmx attach exits 0 however the session's program ended, so a session's own end exits 0.
- With no terminal to own (input or output isn't one), Portal execs the attach command instead, as before the runner.

### Key config

In `prefs.json`; a missing entry keeps its default:

```json
"keys": {
  "detach": "alt+d",
  "detach_to_shell": "alt+shift+d",
  "kill": "alt+q"
}
```

- A value is one key or a list of keys; an empty list turns the action off.
- A key is `ctrl`, `alt` or `shift` modifiers plus a letter, digit, punctuation mark or `f1`–`f12`. It needs `ctrl` or `alt`, so a binding never swallows plain typing. `ctrl+shift+<letter>` is refused: without the newer encoding it sends the same bytes as `ctrl+<letter>`. `ctrl+\` is refused: it's zmx's. Under Portal it always detaches, since the runner detaches zmx through it, so `ZMX_NO_DETACH_KEY` isn't honoured.
- A binding that doesn't parse, or a key bound to two actions, falls back to that action's default with a WARN; doctor reports it. A `prefs.json` that can't be read keeps every default, with a WARN.
- Read at each attach.
- On macOS the terminal must send Option as Alt for these keys (Ghostty `macos-option-as-alt`, kitty `macos_option_as_alt`, iTerm2's Esc+, Terminal's "Use Option as Meta key", Blink's Option-as-Meta); otherwise Option+D types `∂`, which no binding matches, as it depends on the layout. The README says so.

### Runner registry

zmx doesn't list its clients, so the runners list themselves, taking the part tmux's client list plays today (`internal/runnerreg`). While it is attached, each runner has an entry in `state/runners/`: `<token>.json` holding its pid and session, and beside it its control socket, `<token>.sock`. It registers once its attach has begun, moves the entry to the next session when a switch attaches it, and removes it when an attach ends any other way. The directory is 0700 and the socket 0600.
- **Which terminal.** zmx already knows which of a session's clients the user is typing in: its leader, the client that last sent keyboard input (terminal replies, mouse and focus events don't count, and a client attaching to a session with no leader becomes it). It routes an in-session `zmx attach <x>` to that client. Portal asks zmx rather than guessing: each runner starts its `zmx attach` with `PORTAL_ATTACH=<token>` among the variables zmx tracks (`ZMX_TRACK_ENV`: the inherited value, else zmx's own default list, plus `PORTAL_ATTACH`), and zmx's EnvGet answers the leader's tracked variables. A leader carrying a token is that runner's terminal; one without is a raw zmx client. A contract test pins Portal's copy of zmx's default list against what zmx tracks for a client that sets none. `PORTAL_ATTACH` and the extended `ZMX_TRACK_ENV` also reach the shells of a session zmx creates through a runner's client (a `zmx attach <new>` typed in a Portal attach), and show in `zmx print-env`. Portal can't strip them, since zmx reads the tracked variables from the client's own environment. They're harmless: Portal never reads its own `PORTAL_ATTACH`, only the leader's, and asks a runner only when its entry names the session typed in (see Switching).
- **Liveness.** An entry is live only when its pid is running and its socket answers as that pid. It is removed with its socket only on proof its runner is gone: its pid not running, or its entry from another boot. Each entry records the boot id its runner registered in, since the state dir outlives a reboot and a recycled pid would keep a dead runner's entry looking alive; an entry naming another boot or none is stale, and a reader that can't read the current boot id judges by the pid alone. Everything else leaves it: a socket that refuses (on macOS a live runner's full listen backlog refuses too) or is missing, an answer from another pid, no answer in time, an entry that can't be read, or a refusal, as the runner may be busy or another version's. Within one boot, a reused pid can so keep a dead runner's entry until the process reusing it exits; that is accepted. A runner sweeps the registry as it registers: stale entries, on any session, and sockets with no entry that refuse a connection. Reading the registry for a session's runners sweeps every session the same way.
- **Socket paths.** A socket's path must fit a unix socket's address, 103 bytes on macOS. A state dir too deep for that holds no runner: the registration fails with a WARN and the attach runs unlisted. A path is never cut short.
- **Control protocol.** A connection carries one request, a JSON line such as `{"version":1,"request":"switch","session":"<x>","deadline":…}`, and one answer, `{"version":1,"ok":true,"pid":…}` or a refusal saying why. Keys match exactly and nothing may follow the line: the caller half-closes after it. A runner refuses another version, an unknown request or field, and a session given to a request that takes none or missing from one that needs it, so a Portal upgraded while attached never acts on a request it doesn't understand. A runner takes a request only while its attach waits to end, and refuses one sent between attaches, once the attach is ending, or past the caller's deadline, so it never takes a request its caller has given up on.
  - `switch`: answered once the runner has taken it, not once the new attach is up. The runner ends its attach as the detach key does, by `ctrl+\` to its child, and attaches `<x>`, as the picker's pick is attached. A target that can't be attached for a fault of its own (gone by then, held under another spelling, refusing connections or not answering) brings the picker, with why, worded as `portal kill` words it. Any other failure ends Portal, as a first attach's does.
  - `detach-to-picker`: ends the attach as the detach key does; the picker follows.
  - `confirm-session`: accepted when the runner's attach under way is on the session named and the runner is sure of it: it has seen no switch since the attach began, or, having seen one, it confirms through zmx as it is asked (see Switched outside Portal). It ends nothing. New windows ask it (see below).
- **Switching.** `portal open <x>` inside a session, a mint there (created first) and a pick in a picker running there ask zmx for the session's leader. A runner leading is asked to switch, if its entry names the session typed in, and Portal exits 0 once it accepts. Otherwise (a raw client leading, no leader, a token naming a runner on another session, or a runner that can't be reached or refuses) Portal execs `zmx attach <x>`, which zmx routes to the leader. Opening the session typed in does nothing and exits 0: a switch would detach and re-attach it.
- **Switched outside Portal.** zmx can switch a runner's child under it: a `zmx attach <x>` typed by hand in a Portal attach, or the fallback above when the leading runner didn't answer. zmx's client re-attaches in the same process, so the runner sees only its output: zmx's terminal reset followed by its screen clear. A zmx inside the session switching its own client writes the same bytes, as one reached over ssh does, since zmx sends a session's output to every client. So on that signature the runner becomes unsure for the rest of the attach, and stays listed.
  - Unsure, the runner confirms through zmx each time it is asked to act on the session it shows: the kill key as it is pressed, a `detach-to-picker` or a `confirm-session` as it comes, never when the signature was seen, so zmx has long since seen a client that left. It acts only if the session's leader carries the runner's token, and refuses otherwise: the kill key opens the picker without its kill prompt and flashes why, a request is answered with why. A confirmation leaves the runner unsure; the next asks again. So the runner never offers to kill, detaches or vouches for a session the user has left. This is sound because a client that has left a session can't lead it: zmx clears the leader as its client disconnects, and zmx's client closes the old socket before it writes the reset. When another terminal on the session has typed more recently, a real attach that is unsure is refused: narrow, and failing safe.
  - A switch request is taken without confirming: it reaches a runner only when zmx says its token leads the session typed in and its entry names that session, which proves it still shows it. The detach keys still detach.
- **New windows (`m`).** Portal asks zmx for the session's leader. A leader carrying a token is that runner's terminal, once its entry names the session and the runner accepts `confirm-session`: Portal walks the runner's process tree up to the terminal app. A token the registry doesn't hold or naming a runner on another session, a runner that refuses (between attaches, or unsure and unconfirmed), no leader, or any read that fails opens nothing, saying Portal can't tell which terminal it is running in; a leader whose terminal is on another machine opens nothing as remote. Doctor detects the same way but removes no registry entry, even a dead runner's.
  - Each window runs `portal open` with what its opener resolved by: its `PATH`, its zmx directory and state directory as resolved, and `XDG_CONFIG_HOME`, Portal's path variables and its log settings as the opener has them, a path made absolute against the opener's directory and one the opener lacks unset. A window's shell starts from the terminal's environment, which need not match. `ZMX_SESSION_PREFIX` needs no carrying: Portal removes it from every zmx it runs. `ZMX_SESSION`, `TMUX` and `TMUX_PANE` are removed from the window and from the command that asks the terminal for it.
  - A session counts as the one Portal runs in only when the environment's zmx directory is the backend's; inside a session of another directory, detection takes the route from outside one.
- **Detach to the picker.** The resume panel's Esc asks every runner on the session, all at once. Each request has its own deadline: 2 s after it is sent, or the asker's deadline less the 0.5 s answer grace, whichever comes first. So a runner confirming through zmx (see Switched outside Portal) costs the others nothing. With no runner registered, Esc detaches through the backend, and a raw client goes to its shell; a runner whose pid is found not running as it is asked counts as none. With runners registered and none accepting, nothing is detached, and the panel says why: each runner's refusal or failure.
- **Read-only.** Doctor's read-only registry refuses to ask a runner to switch or to detach, since either moves the user's terminal. Its refusal means do nothing: a caller must not fall back to moving the terminal itself. It still asks `confirm-session`, which ends nothing.
- Known limits:
  - The backend's detach takes every client at once, so it is used only when no runner is registered. A raw client attached to the same session as a runner stays attached.
  - An entry the registry can't read is left out of the session's runners. A session whose only runner wrote such an entry, as a newer binary's during an upgrade, falls back to the backend's detach.
- **No runner** (a raw `zmx attach`, or rootshell's own zmx picker). When the leader carries no token, `m` looks for the session's `zmx attach` processes in the process table: this user's, on a terminal, the kernel naming their executable `zmx`, whose argv names the session as zmx's `parseAttachArgs` reads it (`attach` or `a`; `--labels` with its value or `=`, any number of times, only before the name; no name at all meaning the prefix alone), after `ZMX_SESSION_PREFIX`, with `ZMX_DIR` resolving to the backend's dir. zmx's daemon keeps the argv it forked from but has no terminal, so it is never one. Of those it passes over any carrying `PORTAL_ATTACH`, a runner's own client, and any whose variables zmx tracks for the leader differ from the leader's (a variable the leader lacks must be absent too), so a client over ssh, with `SSH_CONNECTION`, never stands in for a local one. It refuses unless exactly one is left, and when a `zmx` process it would read can't be read, or shows no environment.
  - Known limits: zmx switches a raw client in-process, Portal's own exec fallback included, so the process table still shows it under the session it first attached.
    - When no other raw client names the session it now leads, `m` refuses in that terminal until it attaches afresh. That fails safe, and an attach through Portal's runner is unaffected.
    - When another raw client whose tracked variables equal its own still names that session, the leader is mistaken for that other client. With zmx's default tracked list, local raw clients on one Mac usually share their tracked values, so the windows open on that Mac, though maybe in the other terminal app; they could land on another machine only if `ZMX_TRACK_ENV` drops the SSH variables.

## Commands

- **open:** the resolution chain is unchanged and matches session names, not labels. Exact-name matches come from `List()`. Mints use `Create` and record the session in the index (name, dir, created time), then the runner. A generated name always carries a project part, never a bare nanoid. Inside a session, it switches through the registry.
- **picker:** rows from `List()` joined with the index. The filter and the search form match labels as well as names. Grouping uses the index's dir, falling back to the git root of the session's cwd. Kill, rename and preview as settled above; a kill also removes the session's index entry. A session that doesn't answer is listed dimmed and marked as not answering; opening or killing it flashes why. A failed kill or rename flashes and the picker stays open.
- **list, kill, completion, uninstall:** re-pointed to the backend. `list` prints names; `--long` adds label, attached and dir. Completion offers names. `uninstall` kills `_portal-saver`.
- **hook set/rm/list:** the current session comes from `$ZMX_SESSION`. `hook list` keeps its columns, with the session's label as the location. `--pane-key` keeps its name.
- **doctor:** zmx present, the sessions whose daemons speak a protocol other than 1 named, saver up, daemon alive, state dir, stale hooks and projects, pending resumes, key config.
- **current (new):** prints the current session's label. Works however the session was attached. Outside a session it prints nothing and exits 1.
- **rename (new):** `portal rename <text>` sets the current session's label, its arguments joined with spaces; empty text resets it to the session name.
- **spawn:** spawned windows run with `ZMX_SESSION` removed, so each attaches in its own terminal, and with what the opener resolved its sessions, state and config by. Detection inside a session uses the registry. Each window acks in `spawn-acks/`.
- **migrate (new):** see Migration.

## Migration

`portal migrate` is the only code that knows tmux after phase 5.
1. Stop the tmux version's saver and remove Portal's tmux hooks. tmux and its sessions keep running, untouched otherwise.
2. Move `state/` to `state-tmux/`, and copy `hooks.json` into it.
3. For each session in `state-tmux/sessions.json`, create a zmx session with its directory, scrollback and the resume panel, so Enter resumes Claude.
   - The session name is the part of the tmux name before ` · ` when it fits zmx's limits, otherwise a generated name.
   - The session label is the full tmux name, so `dotfiles-PpaTER · user-search` reads as before.
   - The two-pane session becomes `<name>` and `<name>-2`.
4. Rewrite `hooks.json` from pane tokens to session names.
5. Report what was created. The tmux sessions stay until Lee ends them.

Kit follow-ups at switch-over: `session-label` moves to `portal current` and `portal rename`, and the `xc`, `xcc` and `xcr` aliases take `--name "$(portal current)"`.

## Output and visuals

Borrowed from Switchboard, which grew these out of Portal's harness.

- **Golden frames.** `capturetool` gains `--print`, `--ansi`, `--size WxH` and `--frames DIR [names…]`, behind a testable `run(args, stdout, stderr, getenv)`. Every fixture, the standalone surfaces included, has a committed frame: a `.txt` and `.ansi` pair in `testdata/vhs/frames/`, the size in the stem, drawn in one theme (tokyo-night). Tests compare each fixture against its frame cell by cell (glyph, colours, bold), list the first differing cells on failure, and check each fixture draws the same twice. Frames are regenerated by command in the same change that alters a screen, never edited by hand; an unintended frame change is a regression. They land before the picker moves, so every screen change in this work shows up as a frame diff.
- **Size sweeps.** Each fixture is drawn across a grid of sizes, with a list of known breaks that fails once a break is fixed.
- **Demo data.** Nothing personal: fixture and demo names are invented.
- **Demos.** Clips and stills play in-process through `capturetool --scenario`: a fixture world plus timed cues, with the last keys pressed drawn as keycaps. No container, no real binary, no shell prompt on screen. Clips end at selection or preview, since attaching is the terminal's own rendering. A test replays each tape's keys at its times on a virtual clock and checks the screen at checkpoints.
- **Recording.** Switchboard's tape header (JetBrains Mono, exact cell grid, matching `Set Theme`, no cursor blink, `Wait+Screen`, `TZ=UTC`), its `record.sh` and its `finalize.sh`: vhs raw frames to lossless 30 fps WebP at twice the display width, a canvas-coloured margin, no colour grading. Every still comes from a fixture or scenario, so all art can be made again with one command.
- **Docs.** `demo/README.md` and `testdata/vhs/README.md` rewritten in Switchboard's shape.

## Testing

- A `zmxtest` package gives each test its own short `ZMX_DIR` under the system temp dir, kept short for the socket limit, and kills every daemon in that directory at cleanup.
- `TestMain` blanks `ZMX_DIR` and `ZMX_SESSION`, so a missed injection fails loudly instead of reaching the real sessions.
- The test-safety rules in `CLAUDE.md` carry over with zmx in place of tmux: never the real `ZMX_DIR`, never a daemon the test didn't start.
- Unit tests use the fake backend, held to the driver by one contract suite (`backendtest.Contract`) that runs against the fake, the driver over fake daemons, and the driver over real zmx. The integration lane runs real zmx.

## Phases

1. Golden frames for today's screens. Backend interface, zmx driver and `zmxtest`. open, picker, list, kill, completion, hook, current, rename and doctor on zmx. No save or restore yet. From this phase no command runs the tmux bootstrap; the version 1 `sessions.json` stop lands here as the dev build's safety net, and the concurrent bootstrap and loading page stay dormant until phase 3. Attaching execs `AttachCommand` directly until the runner. The multi-target burst and `m` refuse until phase 2. Doctor checks zmx, the protocol, the state dir, projects, the terminal and resume mode; its saver, daemon, hook and pending-resume checks return in phase 3, once hooks are keyed by session name.
2. The attach runner and its registry: keys and key config, detach to picker, switching from inside a session, `m`.
3. State v2, the saver, restore, resume and bootstrap.
4. `portal migrate`.
5. Remove `internal/tmux` and every tmux-only path; a structure pass over what remains.
6. Size sweeps, demo scenarios, recording, new README art and docs. Release.

Phases 1–2 make it usable day to day. Phase 3 is needed before trusting it through a reboot. Phase 4 is switch-over day.

## Risks to watch

- **zmx bugs:**
  - #294: crash on session exit on macOS.
  - #295: part of the scrollback lost on reattach.
- **Sessions started by zmx 0.8.1** speak the older protocol and don't answer Portal: they list as not answering until killed. A 0.8.1 daemon waits on each of Portal's frames for payload that never comes, so a connection carries one frame, enforced in the driver: a second would complete the first and be read as frames of its own.
- **What zmx's history returns for a session showing the alternate screen** (the resume panel). Covered by skipping waiting sessions; to confirm in phase 3.
- **zmx's protocol not yet released.** The last release is v0.8.1 (5 Sep 2026); the protocol sits under "Staged" in zmx's changelog, headed by the breaking IPC change, so it looks batched for a breaking release. Portal on zmx releases once a zmx release carries protocol 1; check upstream before then.
