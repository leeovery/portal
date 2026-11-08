# Troubleshooting

This page shows where to look when Portal doesn't do what you expect: what each line of `xctl doctor` means, what `--fix` repairs, and how to read the logs. The command's flag and exit code are in [Commands](commands.md#xctl-doctor).

- [Where to start](#where-to-start)
- [`doctor`](#doctor)
- [`doctor --fix`](#doctor---fix)
- [The logs](#the-logs)

## Where to start

- **How things stand:** run `xctl doctor`. It reads everything and changes nothing; see [`doctor`](#doctor).
- **Stale hooks or projects, or old logs:** run `xctl doctor --fix`; see [`doctor --fix`](#doctor---fix).
- **Sessions missing after a reboot:** run `grep -E 'session dropped|drops-held' portal.log`; see [After a reboot](#after-a-reboot). [What comes back, and what doesn't](how-it-works.md#what-comes-back-and-what-doesnt) has the rules.
- **Portal stops, naming two directories:** a state directory belongs to one zmx directory. See [The zmx directory](how-it-works.md#the-zmx-directory).
- **`Alt+D` types `∂` on macOS:** the terminal must send Option as Alt; see [macOS](sessions.md#macos).
- **A key you bound does nothing:** a refused or unparseable binding keeps its default, and doctor's `key config` line says why. See [Rebinding the attach keys](configuration.md#rebinding-the-attach-keys).
- **`m` in the picker says it isn't available, or a banner names an unsupported terminal:** see [Which terminals](picker.md#which-terminals) and [Custom terminals](custom-terminals.md).

## `doctor`

`xctl doctor` is a read-only health report across Portal's resurrection machinery. It starts nothing and signals nothing: a saver that isn't running is reported, not started.

It prints one line per check, marked `✓` when it passes, `✗` when it fails, `·` when it can't be judged, and with no mark when it only informs. Theme advisories (`⚠`) follow, then a last line counting the checks that passed and any advisories.

- [zmx](#zmx)
- [The saver and its daemon](#the-saver-and-its-daemon)
- [State](#state)
- [Stale hooks](#stale-hooks)
- [Stale projects](#stale-projects)
- [Lines that inform](#lines-that-inform)
- [Theme advisories](#theme-advisories)

### zmx

- **`zmx`:** the zmx on `PATH`, judged as Portal's start judges it: it must run, and be no release older than 1.0.0 (see [zmx](how-it-works.md#zmx)). It shows the path, or says why Portal can't use it.
- **`zmx dir`:** the directory `ZMX_DIR` names (see [The zmx directory](how-it-works.md#the-zmx-directory)). It passes when it can be read, or isn't made yet (`not created yet`: zmx makes it with its first session), and fails when it isn't an absolute path, can't be read or isn't a directory.
- **`sessions`:** every session answering, `_portal-saver` among them. It shows how many answer (or `no sessions`), and fails naming each session that doesn't, with why: `no answer`, `refuses connections`, or the protocol its daemon speaks when that isn't Portal's. It reads not evaluable when the sessions can't be listed.

### The saver and its daemon

- **`saver`:** `_portal-saver up` when the saver session runs.
- **`daemon`:** the save daemon in that session. It passes when `daemon.pid` names the process `_portal-saver` runs and that process is Portal's save daemon, recorded at this Portal's version. It fails when there is no `daemon.pid`, when `daemon.pid` names another process than the one `_portal-saver` runs, when that process has exited, or when it isn't Portal's save daemon.

A missing `_portal-saver` fails both lines while there is anything to save: a live session, or an entry in `sessions.json`. On an install with nothing to save yet, such as a fresh one, both lines state it instead (`nothing to save yet`) and the report stays healthy. If doctor can't list the sessions or read `sessions.json`, it can't show there is nothing to save, so a missing saver still fails rather than reading not evaluable.

A daemon still starting, not yet recorded in `daemon.pid`, is given the time Portal gives it to come up before it is judged. A daemon recording another version still saves; its line reports it rather than failing, and the next `x` after an upgrade, or any other command that runs Portal's bootstrap, replaces it.

A `_portal-saver` that saves another state directory fails both lines, naming that state directory; see [The zmx directory](how-it-works.md#the-zmx-directory). A saver, `daemon.pid` or daemon process doctor can't read reads not evaluable, and says why.

### State

- **`state dir`:** the [state directory](configuration.md#state). It passes when it can be read, or isn't made yet, and fails when it can't be read or isn't a directory.
- **`state's zmx dir`:** the zmx directory the state directory belongs to, read as Portal's start reads it but recording nothing. It passes showing the recorded directory, or `not recorded yet`, since the next start records the one in use. It fails when the state belongs to another zmx directory, saying how to run against it or give Portal a state of its own, and when the record can't be read, naming the file: delete it, and Portal records the directory in use at its next start.
- **`sessions.json`:** the session index. It shows how many sessions are indexed (or `no sessions indexed yet`), and fails when the file is unreadable, corrupt, or from a version of Portal this one can't read, saying which.

A state directory doctor can't resolve fails all three lines (`unresolvable`), since nothing can be saved without one, and the lines that read the state directory read not evaluable.

### Stale hooks

**`stale hooks`** counts the hooks the daemon's hourly sweep would remove, and removes none. A hook is stale when its key names neither a session zmx lists nor one saved in `sessions.json` (see [Hooks and sessions](resume.md#hooks-and-sessions)). It passes with `no hooks` or `no stale hooks`, and fails with the count of stale entries, or when `hooks.json` can't be read or doesn't parse.

While the stale hooks can't be counted, the line reads not evaluable, never failing, and says why:

- `restore in progress`
- `could not read the restoring flag`
- `could not resolve the state directory`
- `could not list sessions`
- `could not read sessions.json` (unreadable, corrupt, or written by an older or newer Portal)
- `no sessions listed or saved`
- `hooks.json is locked`, when another writer held it too long
- `half or more of hooks.json's keys name no session listed or saved`

### Stale projects

**`stale projects`** counts the remembered projects whose directory no longer exists. A directory that can't be read is not counted stale. It fails with the count, which `--fix` prunes, and reads not evaluable when `projects.json` can't be read.

### Lines that inform

These state rather than judge, and none of them fails:

- **`pending resumes`:** how many live sessions are waiting on their [resume panel](resume.md#the-resume-panel).
- **`kept sessions`,** shown only when there are any: the saved sessions that aren't running, which `x <name>` brings back. With no save daemon running, that is every saved session that isn't running, since the next start brings them all back. It is left out when the `state's zmx dir` line fails, and when the sessions or `sessions.json` can't be read.
- **`key config`:** the bindings of the [attach keys](configuration.md#rebinding-the-attach-keys). It passes when every binding holds, and otherwise names each that falls back to its default, and why, or says `prefs.json` isn't valid JSON or a JSON object, so every key keeps its default. It reads not evaluable when `prefs.json` can't be read.
- **`host terminal`:** the detected terminal's name and whether Portal supports it, or `unsupported` with why: a remote session, or a terminal Portal can't tell. On an unsupported terminal the picker's banner names its bundle id, the key to copy into [`terminals.json`](custom-terminals.md).
- **`resume mode`:** the install's resume mode, `eager` or `lazy`; the shipped default `lazy` whenever `prefs.json` sets none.

### Theme advisories

After the checks, a `⚠` line names each theme file Portal can't use and why, a themes directory it can't read, and a theme `prefs.json` names that doesn't resolve. Advisories are counted on the last line, apart from the checks. [Themes](theming.md) has the rules they name.

## `doctor --fix`

`--fix` performs the reversible-by-reconstruction repairs, then runs the diagnosis again:

- prune stale hooks;
- prune stale projects, naming each: `Pruned stale project: <name> (<path>)`;
- sweep old logs.

The daemon already runs these prunes automatically on a slow cadence, so `doctor` usually reads healthy without you doing anything; `--fix` is the manual trigger.

When it prunes stale hooks, it names each hook it removed, one to a line:

```
Pruned 2 stale hooks:
  notes-x7Kd9a
  blog-drafts
```

When it can't, it removes nothing and prints `Skipped stale hook prune:` with one of the [stale hooks](#stale-hooks) reasons, or with:

- `could not read hooks.json`;
- `hooks.json holds malformed JSON`;
- `the sweep could not complete`.

It repairs nothing at all, and says so, in two cases:

- when the `state's zmx dir` line fails: `Skipped every repair: state's zmx dir failed`;
- beside another state directory's `_portal-saver`: `Skipped every repair: _portal-saver saves the state in <other>, not this one`.

## The logs

Portal's log is `state/portal.log`; [Logging](configuration.md#logging) covers where it lives, rotation, retention and the level, `debug` among them for full detail while you investigate. Every line carries a `subsystem:` prefix, so `grep "daemon:" portal.log` (or `restore:`, `saver:`, `hydrate:`, `spawn:`, `resolve:`, …) reconstructs what any subsystem did.

### After a reboot

`grep -E 'session dropped|drops-held' portal.log` shows what the saver did to your saved state.

- `session dropped session=<name>` names each session removed from it (a relabel is not a drop).
- `drops-held` means many sessions went missing at once, as in a crash or a zmx upgrade, and the saver kept every one of them for the next restore.

### Getting a removed command back

- Each stale hook removed is written with its command: `grep 'op=clean-stale' portal.log`.
- Each resume command discarded from the resume panel is too: `grep 'op=discard' portal.log`.
