# Discovery Session 001

Date: 2026-06-05
Work unit: resume-survives-worktree-cwd-move

## Description (as of session)

Make Portal's Claude-resume hooks survive a session whose working directory
has moved (e.g. into a git worktree) since the Claude session was created.

## Seed

- seeds/2026-06-05-resume-survives-worktree-cwd-move.md (inbox:idea)

## Imports

(none)

## Map State at Start

(n/a — single-topic work)

## Exploration

Originated from an inbox idea. When the hydrate helper runs
`claude --resume <UUID>` on a restored pane, the resume fails if the pane's
current working directory has moved away from where the Claude session was
originally created. Claude binds session storage to the CWD-at-creation:
session JSONL files live under `~/.claude/projects/<cwd-encoded-as-dashes>/`,
and `claude --resume <UUID>` looks up by the `(cwd, UUID)` tuple. A moved
pane CWD encodes to a different path, so the lookup misses and surfaces a
"session UID not recognised" error even though the session file exists on
disk under the original encoded directory.

The triggering incident was a single `kb-decay` worktree session that failed
to resume after a clean restart: the Claude session had been created in the
project root, but the working setup had since moved into a git worktree
underneath, which the restored tmux pane's CWD pointed at. Every other
resumed session in the same restart cycle worked — this is an edge case, not
a regression.

Shaping settled the work type as a **feature** rather than a bugfix: the root
cause is already understood (no investigation to run), and the genuinely open
work is a set of design decisions — capturing the original CWD / session-file
location in the hook vs. extending the hook-entry schema; whether resume
should target the original CWD (faithful to the session) or the pane's
current CWD (faithful to where the user now works); graceful fallback when a
resume misses rather than a flashed error; and an adjacent `portal hooks
doctor` idea to report entries whose UUIDs reference moved/vanished session
files. The user confirmed it needs fixing properly — trust in the resume
system has just been restored (after the test-isolation fix that was wiping
the live hooks.json), so this edge case is expected to bite more often now.

These directions are seed material for the discussion phase, not decisions to
make here. Relevant files noted in the seed:
`~/.dotfiles/home/.claude/hooks/portal-resume-hook.sh`, `cmd/state_hydrate.go`,
`internal/hooks/store.go`.

## Edits

(none)

## Topics Identified

(none)

## Conclusion

Routed to discussion.
