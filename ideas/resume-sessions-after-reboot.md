# Resume sessions after reboot

## Problem

After a system reboot, tmux-resurrect restores sessions (layout, CWD, pane structure) but the processes that were running inside them are dead. You open Portal, select a restored session, and find a bare shell prompt where Claude Code (or any long-running process) used to be.

With 5-10 Claude sessions running across separate tmux windows, a reboot means manually resuming each one — finding the right conversation UUID, running `claude --resume <uuid>` in the right pane. This is painful.

## Background: how Zellij handled this

Zellij has built-in session resurrection that serializes session state every second, including the running command per pane. On reattach after reboot, Zellij re-executes the saved command (with a "Press ENTER to run..." safety banner by default). Claude Code stores conversations on disk, so re-launching `claude` in the right project directory allowed it to load persisted state — making it appear seamless.

tmux-resurrect has a similar capability via `@resurrect-processes`, which controls which programs get re-launched on restore. By default it only restores a small whitelist (vi, vim, man, less, top, etc.). Adding `"~claude"` to this list would make it re-run the saved Claude command. However, this alone isn't sufficient — see below.

## Why simple re-execution isn't enough

tmux-resurrect saves the command that was running (e.g., `claude --name portal-HApPAF`). But re-running the original command doesn't reliably resume the conversation:

- `claude` by itself starts a fresh conversation
- `claude --resume` (no ID) opens an interactive picker for that project's sessions
- `claude --continue` continues the most recent session in the CWD, but with multiple sessions per project, "most recent" is ambiguous — it's unclear if it's scoped to the specific pane/window or just picks the last globally
- `claude --resume <uuid>` is the only deterministic way to resume a specific conversation

Additionally, the original launch command isn't always what you want to resume. Users don't always start Claude via Portal's `-e` flag or `--` args. Common patterns include:
- `x .` to open a session, then manually running `claude` inside it
- Starting a tmux session directly, then launching Claude later
- Switching between different Claude conversations within the same session

The command to resume is not the command that was used to create the session — it's whatever was running at the time of shutdown.

## Claude Code session storage

Claude Code stores sessions on disk:
- Location: `~/.claude/projects/<encoded-cwd>/<uuid>.jsonl`
- `<encoded-cwd>` replaces `/` with `-`, e.g., `-Users-leeovery-Code-portal`
- Each session is a JSONL file with `sessionId` in the first line
- Sessions are project-scoped (scoped to working directory)

Relevant CLI flags:
- `claude --resume <uuid>` or `claude -r <uuid>` — resume a specific session
- `claude --continue` or `claude -c` — continue the most recent session in CWD
- `claude --session-id <uuid>` — force a specific UUID for a new session

The `SessionStart` hook in Claude Code fires on every session start (startup, resume, clear, compact) and provides the `session_id` in stdin JSON. The `CLAUDE_ENV_FILE` mechanism allows hooks to export environment variables into the session.

## Design constraint: Portal must stay generic

Portal is a tmux session manager, not a Claude launcher. Any solution must work generically — Claude is one use case, but the same mechanism should support any long-running process (a dev server, a database REPL, etc.) that a user wants to resume after reboot. Portal must have zero knowledge of Claude Code.

## Decision: restart command registry

### Portal side (generic)

Portal exposes a `register-restart` subcommand. This is a generic facility — Portal stores and replays opaque command strings without knowing what they do.

**Registration**: `portal register-restart "claude --resume abc-123"`

When called from within a tmux session, Portal auto-detects the current tmux session name (via `tmux display-message -p "#{session_name}"`). No need for the caller to specify which session — Portal figures out the context itself.

Users call this via the shell integration as `xctl register-restart "..."` (or whatever their configured ctl name is). In contexts outside the shell integration (e.g., Claude Code hooks), `portal register-restart` is used directly since hooks don't go through the shell functions.

**Persistent storage**: restart commands are written to disk, e.g., `~/.config/portal/restart/<tmux-session-name>`. Survives reboot.

**Dead session detection**: after reboot + tmux restore, Portal detects sessions where the pane's active process is just the shell (the original process died). Checked via `tmux list-panes -t {name} -F "#{pane_current_command}"`.

**TUI integration**: dead sessions with a registered restart command are shown with a visual indicator (e.g., "suspended" or "resumable"). Selecting one sends the registered command to the pane via `tmux send-keys`.

**Cleanup**: when a session is killed normally (not via reboot), the restart registration is cleaned up.

### Claude Code side (user-configured, outside Portal)

A single user-level Claude Code hook, configured once in `~/.claude/settings.json`, fires across all projects automatically. No per-project setup needed.

**Hook configuration** (`~/.claude/settings.json`):
```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume",
        "command": "portal register-restart \"claude --resume $(cat | grep -o '\"session_id\":\"[^\"]*\"' | head -1 | sed 's/.*\"session_id\":\"//;s/\"//')\""
      }
    ]
  }
}
```

This is a one-liner that extracts the session UUID from stdin JSON and calls `portal register-restart` directly. No external script files needed.

The hook fires on every `startup` and `resume` event, so the registered restart command always reflects the current conversation UUID — even if the user switches to a different Claude conversation in the same tmux window.

Portal has zero knowledge of Claude. The hook is the bridge. It's user-configured, lives in the user's Claude Code settings, and just happens to call a Portal CLI command.

### Full flow

**Before reboot (registration):**

1. User runs `x portal` — Portal creates tmux session `portal-HApPAF`
2. User runs `claude` inside that session (however they want — manually, via `-e`, etc.)
3. Claude starts, fires `SessionStart` hook
4. Hook runs: `portal register-restart "claude --resume abc-123"`
5. Portal detects it's inside tmux session `portal-HApPAF`
6. Portal writes `claude --resume abc-123` to `~/.config/portal/restart/portal-HApPAF`

**After reboot (restoration):**

1. User opens terminal, hits `x`
2. Portal boots tmux server (see: auto-start-tmux-server.md), resurrect restores sessions
3. Portal lists sessions for the TUI. For each session:
   - Checks if `~/.config/portal/restart/<session-name>` exists (has restart command)
   - Checks if pane's current process is just `zsh`/`bash` (original process is dead)
   - If both: marks as "resumable"
4. TUI shows `portal-HApPAF` with a resumable indicator
5. User selects it
6. Portal switches to the session and sends `claude --resume abc-123` to the pane via `tmux send-keys`

**Ongoing:**

- If the user starts a different Claude conversation in the same window, the hook fires again and overwrites the registration with the new UUID. Always current.
- If the user kills the session normally via Portal, the restart registration is cleaned up.

## Open questions

1. **Registration scope**: should it be per tmux session or per pane? A session with multiple panes could have different processes in each.

2. **Safety**: should there be a confirmation step before auto-sending commands to restored panes? (Zellij has "Press ENTER to run..." by default.) Or is selecting the session in the TUI sufficient confirmation?

3. **tmux-resurrect `@resurrect-processes`**: should users be advised NOT to add Claude to `@resurrect-processes` if using this feature? Otherwise both tmux-resurrect and Portal would try to restart Claude, causing a conflict.

4. **Multiple restart commands**: could a session need more than one restart command? (e.g., a pane running Claude and another pane running a dev server.) This ties into the per-session vs per-pane scoping question.

5. **Hook command robustness**: the inline grep/sed for extracting session_id from JSON is fragile. Might want to recommend `jq` if available, or Portal could accept stdin JSON directly and extract the field itself (though that would mean Portal understanding the Claude hook input format, which conflicts with the "no Claude knowledge" constraint).
