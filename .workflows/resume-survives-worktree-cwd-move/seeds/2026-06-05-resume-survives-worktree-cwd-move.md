# Make Claude-resume hooks survive a session whose working directory has moved

When the hydrate helper runs `claude --resume <UUID>` on a restored pane, the resume can fail if the pane's current working directory has moved away from where the Claude session was originally created. Claude binds session storage to the CWD-at-creation-time: session JSONL files live at `~/.claude/projects/<cwd-encoded-as-dashes>/<UUID>.jsonl`, and `claude --resume <UUID>` looks up by the `(cwd, UUID)` tuple. If the pane's CWD at resume time encodes to a different path than at session-creation time, the lookup fails with a "session UID not recognised" message even though the session file exists on disk under a different encoded directory.

This first bit on 2026-06-05 after a clean post-test-isolation-fix restart. One `kb-decay` worktree session failed to resume: the original Claude session had been created in the project root (`~/Code/agentic-workflows`, encoded as `-Users-leeovery-Code-agentic-workflows`), but the working setup had since moved into a git worktree underneath, which the tmux pane's restored CWD pointed at. On resume, Claude looked under the worktree's encoded path and found nothing. Every other resumed Claude session in the same restart cycle worked correctly — this is an edge case, not a regression.

Directions the discussion phase should explore (not prescriptive, just to seed the conversation):

- Whether the bash hook script should capture the original CWD (or the actual session-file location) at SessionStart and bake it into the hook command, so resume always finds the session regardless of where the pane is later attached.
- Whether the hydrate helper should detect resume failures and fall back to something more graceful than a flashed error (a banner + bare `$SHELL`, an in-pane prompt offering to start a fresh session, etc.).
- Whether the hook entry's on-disk shape should grow metadata (original CWD, encoded path, session-file path) or stay a single command string.
- Whether the worktree-creation flow itself (outside Portal, in the agentic-workflows tool) should know about Claude session files and physically move them so the path encoding matches the new worktree location — a coupling decision rather than a Portal-side fix.
- Whether the right behaviour is "resume into the original CWD" (faithful to the session) or "resume into the pane's current CWD" (faithful to where the user is now working) — these have different implications for active worktree development and may not be the same choice.
- Adjacent value: a `portal hooks doctor` command that scans `hooks.json` and reports entries whose UUIDs reference session files that have moved or vanished.

The bigger framing question: is this worth solving at all, or is "if you move a worktree after Claude session creation, your resume will fail" an acceptable documented limitation? The frequency of worktree creation in the user's workflow is the load-bearing input here.

Relevant files: `~/.dotfiles/home/.claude/hooks/portal-resume-hook.sh` (the bash hook that captures UUIDs at SessionStart), `cmd/state_hydrate.go` (the hydrate helper's exec chain), `internal/hooks/store.go` (would need a schema extension if hook entries grow metadata).
