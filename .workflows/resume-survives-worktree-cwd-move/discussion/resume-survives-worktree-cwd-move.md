# Discussion: Resume Survives Worktree CWD Move

## Context

Portal restores tmux panes after a reboot. Each pane can carry an on-resume
hook (`hooks.json`, keyed by structural pane key). For Claude sessions, a bash
hook (`~/.dotfiles/home/.claude/hooks/portal-resume-hook.sh`) fires at Claude
`SessionStart` and writes `portal hooks set --on-resume "claude --resume <UUID>"`.
At restore, the hydrate helper (`cmd/state_hydrate.go`) runs that command via
`sh -c '<HOOK>; exec $SHELL'` in the pane's **restored** working directory.

The bug: Claude binds session storage to the `(launch-cwd, UUID)` tuple.
Session JSONL files live at `~/.claude/projects/<launch-cwd-encoded-as-dashes>/<UUID>.jsonl`.
`claude --resume <UUID>` looks up by the CWD it is launched from. If the pane's
CWD has drifted away from the launch CWD since session creation, the encoded
lookup path differs, the lookup misses, and Claude surfaces "session UID not
recognised" even though the file exists on disk under the original encoded dir.

**Generalised framing (session 1):** this is NOT worktree-specific. The root
cause is *any* CWD drift after the resume hook was registered. Worktrees are one
trigger; the more common one is Claude itself `cd`-ing into a nested subdir
mid-session and not returning, or the user doing so. tmux captures the pane's
*current* path at save time, Portal restores the pane there, and the resume
command then runs from the drifted dir instead of the launch dir. The **launch
CWD is the stable key** — it's where the JSONL lives regardless of any later
`cd`, because Claude's storage location is fixed at launch and never follows a
mid-session `cd`.

First seen 2026-06-05 after a clean restart: one `kb-decay` worktree session
failed to resume (session created in `~/Code/agentic-workflows`, pane CWD now
pointed at a worktree underneath). Every other session in the same cycle
resumed fine. This is an edge case, not a regression.

**Proposed mechanism (session 1, user):** capture the launch CWD at hook-
creation time (SessionStart, where `$PWD` == launch CWD) and have resume
`cd` to it before running `claude --resume`, then restore the drifted CWD for
the post-Claude shell. Open question: does the `cd`-wrap live in the external
bash hook (opaque command string) or become a first-class Portal concept?

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

  Discussion Map — Resume Survives Worktree CWD Move (6 subtopics — 3 decided · 1 converging · 1 exploring · 1 pending)

  ┌─ ✓ Resume target: launch CWD for resume, drifted CWD for post-shell [decided]
  ├─ ✓ Scope: failure-handling & Claude-specifics are not Portal's concern [decided]
  ├─ ◐ Portal hook-creation API: CWD anchoring (flag / auto-capture / nothing) [exploring]
  ├─ → cd-wrap mechanism: subshell vs Portal-native chdir [converging]
  ├─ ✓ Graceful fallback on resume miss — out of scope [decided]
  └─ ✓ `portal hooks doctor` diagnostic — out of scope [decided]

---

*Subtopics documented below as they reach `decided`.*

---

## Resume target: launch CWD for resume, drifted CWD for post-shell

### Decision
The resume command must be **launched from the launch CWD** (where the Claude
session was created) — that is the only CWD whose encoding matches the on-disk
JSONL location, and it is stable across any later `cd`. After Claude exits, the
pane's interactive shell should land back in the **drifted/restored CWD** (the
path tmux captured and Portal restored the pane to), so the post-Claude shell
matches where the pane "is". This is faithful-to-the-session for the resume and
faithful-to-the-pane for the shell — no conflict, because they happen at
different times.

Rationale: Claude fixes its storage path at launch and never moves it on a
mid-session `cd`. So the launch CWD is the stable key. There is no version of
this where resuming from the drifted dir works without physically relocating the
JSONL (rejected — mutates Claude's storage, risks divergent copies).

---

## Scope: failure-handling & Claude-specifics are not Portal's concern

### Decision
Portal's hook system runs an **opaque command** on resume — it could restart a
dev process, reopen Vim, anything. Portal has no concept of Claude, sessions, or
what "success" means for an arbitrary command. Therefore:

- **Detecting/recovering a failed `claude --resume` is categorically out of
  scope.** Portal cannot judge failure for a generic command, and shouldn't
  special-case Claude. A missed resume already degrades acceptably: the command
  exits and the helper falls through to `exec $SHELL` with scrollback intact
  (the user can identify the session in the picker). Good enough.
- **`portal hooks doctor` (checking whether a UUID's session file exists) is out
  of scope** for the same reason — it's Claude-specific knowledge. The bash hook
  already owns this via its validate-and-prune loop (globs
  `~/.claude/projects/*/<uuid>.jsonl`, prunes via `portal hooks rm --pane-key`).
  Duplicating that inside Portal would pull Claude-awareness into a generic tool.

### Journey
Started by asking whether Portal should own a graceful fallback on resume miss.
The user cut it cleanly: the hooks are generic, so Claude-resume-failure simply
isn't Portal's problem. This collapsed two seed subtopics (graceful fallback,
hooks doctor) to "out of scope" and refocused the discussion on the *only* place
Portal could legitimately help: the hook-**creation** API.

---

## Summary

*(to be filled as subtopics resolve)*
