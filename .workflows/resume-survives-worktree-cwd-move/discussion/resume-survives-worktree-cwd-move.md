# Discussion: Resume Survives Worktree CWD Move

## Context

Portal restores tmux panes after a reboot. Each pane can carry an on-resume
hook (`hooks.json`, keyed by structural pane key). For Claude sessions, a bash
hook (`~/.dotfiles/home/.claude/hooks/portal-resume-hook.sh`) fires at Claude
`SessionStart` and writes `portal hooks set --on-resume "claude --resume <UUID>"`.
At restore, the hydrate helper (`cmd/state_hydrate.go`) runs that command via
`sh -c '<HOOK>; exec $SHELL'` in the pane's **restored** working directory.

The bug: Claude binds session storage to the `(cwd-at-creation, UUID)` tuple.
Session JSONL files live at `~/.claude/projects/<cwd-encoded-as-dashes>/<UUID>.jsonl`.
`claude --resume <UUID>` looks up by the CWD it is launched from. If the pane's
CWD has moved since session creation — e.g. the working setup moved into a git
worktree under the project root — the encoded lookup path differs, the lookup
misses, and Claude surfaces "session UID not recognised" even though the file
exists on disk under the original encoded directory.

First seen 2026-06-05 after a clean restart: one `kb-decay` worktree session
failed to resume (session created in `~/Code/agentic-workflows`, pane CWD now
pointed at a worktree underneath). Every other session in the same cycle
resumed fine. This is an edge case, not a regression.

Discovery settled the work type as a **feature** (root cause understood, no
investigation needed) and confirmed it should be fixed properly rather than
documented as a limitation — trust in resume was just restored after the
test-isolation fix, so this edge case is expected to bite more often now.

### Current mechanics worth noting

- The bash hook **already** globs `~/.claude/projects/*/<uuid>.jsonl` across all
  encoded project dirs (in its validate-and-prune loop) — so "find the session
  file regardless of CWD encoding" already exists in one place.
- The hook command is an opaque single string in `hooks.json`. Portal does no
  interpolation; `internal/hooks/store.go` stores `map[key]map[event]command`.
- `portal hooks set --on-resume` resolves the structural key from `$TMUX_PANE`.

### References

- Seed: `seeds/2026-06-05-resume-survives-worktree-cwd-move.md`
- Discovery: `discovery/session-001.md`
- `~/.dotfiles/home/.claude/hooks/portal-resume-hook.sh`
- `cmd/state_hydrate.go`, `internal/hooks/store.go`

## Discussion Map

### States

- **pending** (`○`) — identified but not yet explored
- **exploring** (`◐`) — actively being discussed
- **converging** (`→`) — narrowing toward a decision
- **decided** (`✓`) — decision reached with rationale documented

### Map

  Discussion Map — Resume Survives Worktree CWD Move (6 subtopics, all pending)

  ┌─ ○ Resume target: original CWD vs pane's current CWD [pending]
  ├─ ○ Fix locus: bash hook vs Portal (Go) vs worktree-creation flow [pending]
  ├─ ○ Where original CWD / session-file location is captured [pending]
  ├─ ○ Hook entry on-disk shape: opaque string vs structured metadata [pending]
  ├─ ○ Graceful fallback when a resume still misses [pending]
  └─ ○ Adjacent: `portal hooks doctor` diagnostic [pending]

---

*Subtopics documented below as they reach `decided`.*

---

## Summary

*(to be filled as subtopics resolve)*
