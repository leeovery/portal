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
  - **Alt+Q:** kill the session, back to the picker.

  zmx's own `ctrl+\` detaches the way plain zmx does, straight to the shell, never the picker. No own zmx protocol client, no floating menu, no pinned status bar.
- **Picker rows.** No window count. The session label, plus the attached and waiting dots.
- **Preview (Space).** The session's live screen and history from zmx, up to zmx's 10,000 lines, instead of the saved file. Window and pane cycling go.
- **Opening several sessions in new windows (`m`).** Works as today: Ghostty on the Mac opens them, and Blink and rootshell get today's refusal. See Attach runner.
- **Esc on the resume panel.** Detaches to the picker, as today.
- **Version.** Lee's call at release.
- **Out of scope:** the workflows tool.
- **zmx's socket protocol, from a pinned build.** The driver speaks zmx's socket protocol (frozen on main on 16 Oct 2026, protocol version 1) for listing, kill, detach and history, so nothing parses text meant for people; zmx declined machine-readable CLI output (#220). Creating and attaching still run the `zmx` binary, since creation isn't a protocol message. Until a release carries the protocol, Portal builds zmx from one pinned main commit (MIT licence). Development and tests use that build; Lee's installed zmx stays untouched until switch-over, when a formula in `leeovery/tools` builds the same commit (or the release, once one ships) and Portal's formula depends on it. Main still reports its version as 0.8.1, so support is judged by the protocol version, never the version string. A fork stays available if upstream won't take something Portal needs; nothing above the driver would change.
- **Leave Portal better than found.** The move is also the refactor: simpler structure, lower complexity, cleaner code. Each phase simplifies what it touches, and nothing tmux-shaped survives above the driver.
- **Output quality.** Rendered output is tested properly, and the README gets a demo recording to the standard Switchboard's has. Switchboard grew its capture and VHS tooling out of Portal's harness; Portal borrows it back (see Output and visuals).

## Names

- **Session name:** the zmx session's name, such as `dotfiles-PpaTER`. Portal sets it once, at creation, and it never changes, since zmx has no rename. Portal-made names carry a nanoid, so they're unique. It keys everything Portal stores about the session: the index, `hooks.json` and the scrollback file. Restore recreates a session under the same name. zmx itself and rootshell's own zmx picker show it.
- **Session label:** what Portal shows: the picker, the preview and `portal current`, which footers and Claude's `--name` read. It defaults to the session name and takes any text. `r` in the picker sets it, and so does `portal rename <text>` from inside the session. It lives in Portal's index.
- **Telling a recreated session apart.** A session that ends and is recreated under the same name outside Portal is a new session. Portal tells it apart by zmx's created time, which the index records and nothing can clear. A session whose created time differs from its entry, and that Portal didn't create or restore, starts clean, without the old session's label, hook or scrollback.

## Backend interface

`internal/backend` defines the interface and the session model. `internal/backend/zmx` implements it by running the `zmx` binary. A fake implementation backs unit tests.

- `List()` returns every session: name, cwd, attached client count, pid, created time and command. A failed read is an error, never an empty list. Sessions whose names start with `_` are Portal's own and are hidden from listings.
- `Exists(name)`.
- `Create(name, dir, command)` creates a detached session: `zmx attach <name> <cmd…>` with no terminal attached, succeeding only when zmx reports it created the session. It starts 120×24 and reflows on attach.
- `AttachCommand(name)` returns the argv the attach runner starts.
- `Kill(name)`.
- `Detach(name)` detaches every client of a session (`zmx detach`).
- `History(name, rows)` returns the screen plus up to `rows` of scrollback (all of it when `rows` is 0), with colours, through the protocol's bounded capture. Never empty: a daemon that doesn't answer is an error.
- `Current()` returns the session this process runs in, from `$ZMX_SESSION`.

Names follow zmx's limits. A socket path is capped at 104 bytes, which leaves about 70 bytes of name with `ZMX_DIR=$HOME/.local/state/zmx`. Generated names stay `{project}-{nanoid}`.

## State

- The zmx version owns `<config>/portal/state/`. `portal migrate` moves the tmux version's `state/` to `state-tmux/` before anything writes to `state/`.
- While `state/` still holds a version 1 `sessions.json`, every command except `migrate` and `version` stops with one line pointing at `portal migrate`.
- `sessions.json` version 2: `{version, saved_at, sessions: [{name, label, dir, created_at, scrollback_file}]}`. `label` is omitted while it's the default. Rename writes through the same locked commit as the saver.
- `scrollback/<name>.bin`. All positional re-filing goes.
- Per-session flags, as files named after the session:
  - `hydrating/<name>`: written before restore creates the session, removed when the scrollback replay finishes.
  - `resume-pending/<name>`: present while the resume panel waits.

  A session's flags are removed when it's dropped from the index, and bootstrap removes any for sessions that aren't live.
- Flags that were tmux server options become files:
  - `restoring`
  - `bootstrapped`: holds the boot id and the version, replacing the per-server latch.
  - `spawn-acks/`
  - `runners/` (see Attach runner)
- `hooks.json` stays where it is, keyed by session name. `prefs.json`, projects and aliases are unchanged.
- Before switch-over, a dev build runs beside the installed tmux Portal with `PORTAL_STATE_DIR` and `PORTAL_HOOKS_FILE` pointed at scratch paths.

## Saver

- **Host.** `portal state daemon` runs inside a `_portal-saver` zmx session, which keeps it alive the way tmux does today. The singleton lock, pid and version files and the orphan sweep stay. Self-supervision compares the saver session's pid from `List()`.
- **Index.** Polled every 5 s; committed when it changes. Portal records the created time and pid of every session it creates, restore included; zmx keeps created time in whole seconds, so the pair tells a recreated session apart (see Names).
- **Scrollback.** Captured at most once a minute per session, and only when its content has changed. Sessions flagged hydrating or resume-pending are skipped.
- **Drops.**
  - A session missing from two polls at least 10 s apart is dropped. A reboot stops the saver before that, so nothing is dropped by mistake.
  - If at least half the sessions, and at least three, vanish in one poll, nothing is dropped and a WARN is logged. That covers a zmx upgrade or a mass crash.
  - A kill from inside Portal commits at once.
- **Triggers.** Portal's own create, kill and rename request an immediate save. Changes made outside Portal are picked up by the poll.
- **Removed, because they only existed for tmux:**
  - the "own server answers" check and the empty-capture check,
  - positional pane keys, pane tokens and the re-filing of moved transcripts,
  - rename detection,
  - the environment capture.

## Restore and resume

- **Restore** writes the hydrating flag, then creates each saved session that isn't live with `Create(name, dir, portal state hydrate --file S [--hook-key N])`.
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
  - **kill:** kills the current session through the backend; the picker follows.
- The picker follows only when Portal asked for it: one of its keys, or Esc on the resume panel through the registry. Any other end of the child (`ctrl+\`, a `zmx detach` from elsewhere, the session ending) exits to the shell, as plain `zmx attach` does.

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
- A key is `ctrl`, `alt` or `shift` modifiers plus a letter, digit, punctuation mark or `f1`–`f12`. It needs `ctrl` or `alt`, so a binding never swallows plain typing. `ctrl+shift+<letter>` is refused: without the newer encoding it sends the same bytes as `ctrl+<letter>`. `ctrl+\` is refused: it's zmx's.
- A binding that doesn't parse, or a key bound to two actions, falls back to that action's default with a WARN; doctor reports it.
- Read at each attach.

### Runner registry

zmx doesn't list its clients, so the runners list themselves, taking the part tmux's client list plays today. Each runner registers in `state/runners/` with its pid, session and control socket, and records when it last forwarded input. Entries whose pid is dead are ignored and removed.
- **Switching.** `portal open <x>` inside a session asks this session's runner with the most recent input, which is the one whose terminal the command was typed in, to switch. The runner ends its child and attaches `<x>`.
- **New windows (`m`).** Portal walks that same runner's process tree up to the terminal app, as it walks the tmux client's today.
- **Detach to the picker.** The resume panel's Esc asks every runner on the session.
- **No runner** (a raw `zmx attach`, or rootshell's own zmx picker). Switching falls back to `zmx attach <x>`, which zmx routes to the client that typed it. `m` looks for the session's `zmx attach` process in the process table, and refuses unless there is exactly one.

## Commands

- **open:** the resolution chain is unchanged. Exact-name matches come from `List()`. Mints use `Create`, then the runner. Inside a session, it switches through the registry.
- **picker:** rows from `List()` joined with the index. Grouping uses the index's dir, falling back to the session cwd. Kill, rename and preview as settled above.
- **list, kill, completion, uninstall:** re-pointed to the backend. `uninstall` kills `_portal-saver`.
- **hook set/rm/list:** the current session comes from `$ZMX_SESSION`.
- **doctor:** zmx present and version supported, saver up, daemon alive, state dir, stale hooks and projects, pending resumes, key config.
- **current (new):** prints the current session's label. Works however the session was attached.
- **rename (new):** `portal rename <text>` sets the current session's label; empty text resets it to the session name.
- **spawn:** spawned windows run with `ZMX_SESSION` removed. Detection inside a session uses the registry.
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
- Unit tests use the fake backend. The integration lane runs real zmx.

## Phases

1. Golden frames for today's screens. Backend interface, zmx driver and `zmxtest`. open, picker, list, kill, hook, current, rename and doctor on zmx. No save or restore yet.
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
  - The protocol freeze isn't in a release yet, so an upgrade before it ships ends running sessions. Doctor reports the zmx version. Upgrade zmx deliberately, after a save.
- **What `zmx history --vt` returns for a session showing the alternate screen** (the resume panel). Covered by skipping waiting sessions; to confirm in phase 3.
