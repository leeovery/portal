# Troubleshooting

This page shows where to look when Portal doesn't do what you expect: `xctl doctor`, what `--fix` repairs, and how to read the logs.

- [Where to start](#where-to-start)
- [doctor](#doctor)
- [doctor --fix](#doctor---fix)
- [The logs](#the-logs)

## Where to start

- **How things stand:** run `xctl doctor`. It reads everything and changes nothing; see [doctor](#doctor).
- **Stale hooks or projects, or old logs:** run `xctl doctor --fix`; see [doctor --fix](#doctor---fix).
- **Sessions missing after a reboot:** run `grep -E 'session dropped|drops-held' portal.log`; see [After a reboot](#after-a-reboot). [What comes back, and what doesn't](how-it-works.md#what-comes-back-and-what-doesnt) has the rules.
- **Portal stops, naming two directories:** a state directory belongs to one zmx directory. See [The zmx directory](how-it-works.md#the-zmx-directory).
- **`Alt+D` types `∂` on macOS:** the terminal must send Option as Alt; see [macOS](sessions.md#macos).
- **A key you bound does nothing:** a refused or unparseable binding keeps its default, and `xctl doctor`'s key-config line says why. See [Keys in a session](configuration.md#keys-in-a-session).
- **`m` in the picker says it isn't available, or a banner names an unsupported terminal:** see [Which terminals](picker.md#which-terminals) and [Custom terminals](custom-terminals.md).

## doctor

`xctl doctor` is a read-only health report across Portal's resurrection machinery. It starts nothing and signals nothing: a saver that isn't running is reported, not started. It exits `0` only when every check passes, non-zero otherwise, so it serves as a scriptable health gate.

### What it checks

- zmx present, and every session answering;
- `_portal-saver` up, and its save daemon alive;
- the state directory sane, and `sessions.json` valid;
- no stale entries;
- sessions waiting to resume;
- the [key config](configuration.md#keys-in-a-session);
- the detected host terminal.

### The saver and its daemon

A missing `_portal-saver` fails the saver and daemon lines while there is anything to save: a live session, or an entry in `sessions.json`. On an install with nothing to save yet, such as a fresh one, both lines state it instead (`nothing to save yet`) and the report stays healthy. If doctor can't list the sessions or read `sessions.json`, it can't show there is nothing to save, so a missing saver still fails rather than reading not evaluable.

The daemon passes when `daemon.pid` names the process `_portal-saver` runs and that process is Portal's save daemon, recorded at this Portal's version. A daemon still starting, not yet recorded in `daemon.pid`, is given the time Portal gives it to come up before it is judged. A daemon recording another version still saves; its line reports it rather than failing, and the next `x` after an upgrade, or any other command that runs Portal's bootstrap, replaces it.

### Lines that inform

These state rather than judge, and never affect the exit code:

- **Host terminal:** the detected terminal's name and whether Portal supports it. On an unsupported terminal the picker's banner names its bundle id, the key to copy into [`terminals.json`](custom-terminals.md).
- **Key config:** names any binding that falls back to its default.
- **Pending resumes:** how many live sessions are waiting on their [resume panel](resume.md#the-resume-panel).
- **Kept sessions,** shown only when there are any: the saved sessions that aren't running, which `x <name>` brings back. With no save daemon running, that is every saved session that isn't running, since the next start brings them all back.
- **Resume mode:** the install's resume mode, `eager` or `lazy`; the shipped default `lazy` whenever `prefs.json` sets none.

### Not evaluable

Any line doctor can't judge (a saver, `daemon.pid` or daemon process it can't read, sessions it can't list) reads not evaluable, never failing, and says why.

A state directory it can't resolve is different: nothing can be saved without one, so the state dir, state's zmx dir and `sessions.json` lines fail (`unresolvable`), and the lines that read the state directory read not evaluable.

The stale-hooks line counts stale hooks, and fails when `hooks.json` can't be read or doesn't parse. A hook is stale when its key names neither a session zmx lists nor one saved in `sessions.json` (see [Hooks and sessions](resume.md#hooks-and-sessions)). While the stale hooks can't be counted, the line reads not evaluable, never failing, and says why:

- `restore in progress`
- `could not read the restoring flag`
- `could not resolve the state directory`
- `could not list sessions`
- `could not read sessions.json` (unreadable, corrupt, or written by an older or newer Portal)
- `no sessions listed or saved`
- `half or more of hooks.json's keys name no session listed or saved`

## doctor --fix

`--fix` performs the reversible-by-reconstruction repairs: prune stale hooks, prune stale projects, and sweep old logs. It runs the diagnosis again afterwards, and the exit code reflects the state after the repairs. The daemon already runs these prunes automatically on a slow cadence, so `doctor` usually reads healthy without you doing anything; `--fix` is the manual trigger.

When it prunes stale hooks, it names each hook it removed, one to a line:

```
Pruned 2 stale hooks:
  notes-x7Kd9a
  blog-drafts
```

When it can't, it removes nothing and prints `Skipped stale hook prune:` with one of the reasons above, or with:

- `hooks.json is locked`, when another writer held it too long;
- `could not read hooks.json`;
- `hooks.json holds malformed JSON`;
- `the sweep could not complete`.

Beside another state directory's `_portal-saver`, `--fix` repairs nothing and says so: `Skipped every repair: _portal-saver saves the state in <other>, not this one`.

## The logs

Portal's log is `state/portal.log`, under `PORTAL_STATE_DIR`; [Logging](configuration.md#logging) covers rotation, retention and the level. Every line carries a `subsystem:` prefix, so `grep "daemon:" portal.log` (or `restore:`, `saver:`, `hydrate:`, `spawn:`, `resolve:`, …) reconstructs what any subsystem did. Set `PORTAL_LOG_LEVEL=debug` to capture full reconstruction detail while you investigate.

### After a reboot

`grep -E 'session dropped|drops-held' portal.log` shows what the saver did to your saved state.

- `session dropped session=<name>` names each session removed from it (a relabel is not a drop).
- `drops-held` means many sessions went missing at once, as in a crash or a zmx upgrade, and the saver kept every one of them for the next restore.

### Getting a removed command back

- Each stale hook removed is written with its command: `grep 'op=clean-stale' portal.log`.
- Each resume command discarded from the resume panel is too: `grep 'op=discard' portal.log`.
