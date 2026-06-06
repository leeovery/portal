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

**Validated (session 1, review F3):** confirmed empirically against live data —
the kb-decay JSONL lives at `~/.claude/projects/-Users-leeovery-Code-agentic-workflows/`,
exactly what raw `$PWD=/Users/leeovery/Code/agentic-workflows` encodes to, and
`~/Code` is not a symlink. So for this environment `$PWD` at SessionStart ==
Claude's encoded dir; the launch-CWD mechanism's foundation holds.
**Known limitation:** `$PWD` is only a *proxy*. If a launch path ever sits under
a symlink, Claude may encode the resolved path while `$PWD` is the symlink path,
so anchoring to `$PWD` would still miss. The authoritative source is the JSONL's
real parent dir (the prune loop already globs for it), but that file is written
async and may not exist when SessionStart fires — so `$PWD` is the only signal
available at registration time. Accepted as a documented edge, not solved now
(user's paths are symlink-free).

**Resolved (session 1, review F5):** the "post-Claude shell lands in drifted CWD"
half needs no extra plumbing — the **subshell** form `(cd <launch> && claude
--resume)` leaves the outer `sh -c` process in the drifted dir, so the helper's
trailing `exec $SHELL` lands there automatically.

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

  Discussion Map — Resume Survives Worktree CWD Move (6 subtopics — 6 decided)

  ┌─ ✓ Resume target: launch CWD; Claude operates in root, re-navigates naturally [decided]
  ├─ ✓ Scope: failure-handling & Claude-specifics are not Portal's concern [decided]
  ├─ ✓ Fix locus / hook API: handled in the bash hook; Portal needs no change [decided]
  ├─ ✓ cd-wrap mechanism: `cd <launch> && claude --resume <UUID>` (no subshell) [decided]
  ├─ ✓ Graceful fallback on resume miss — out of scope [decided]
  └─ ✓ `portal hooks doctor` diagnostic — out of scope [decided]

---

*Subtopics documented below as they reach `decided`.*

---

## Resume target: launch CWD; Claude operates in root, re-navigates naturally

### Context
A subtle constraint surfaced mid-discussion: a Claude session's **launch CWD is
simultaneously (a) where the session is looked up AND (b) where Claude operates
for the entire session.** They are the same directory and cannot be split with a
`cd` — a `cd` placed *after* `claude --resume` in the hook string only runs when
Claude exits (potentially hours later), so it cannot move Claude's working dir
mid-session. So the real question was: while a resumed session runs, where should
Claude operate — the launch/root dir (resolvable) or the drifted dir (not
resolvable without relocating the JSONL)?

### Decision
Resume **launches Claude from the launch CWD** (root). Claude operates in root
for the session. This is not a compromise: it restores the session's *original
starting point*.

The deciding insight (user): Claude can change its own working dir mid-session
(via the `!cd` bash escape) and routinely does — that is exactly how the pane
drifted in the first place. So resuming in root and letting Claude (or the user)
re-navigate is just replaying the same self-navigation that created the drift.
The "wrong" directory is **self-correcting** — no need to engineer "operate in
the worktree", because the session re-derives its working location the same way
it did originally.

Rejected: "resume in root but operate in the worktree." Unachievable with a
`cd`-wrap (lookup-dir and operate-dir are inseparable), and the alternatives —
a Claude flag to decouple them (unverified, likely absent) or relocating/
symlinking the JSONL into the worktree encoding (mutates Claude storage, risks
divergent copies) — were both set aside.

### Mechanism of the bug (clarified)
Session starts at root → Claude/user `!cd`s into a subdir → the change persists
at the shell level → tmux records the subdir as `pane_current_path` → on restore
the pane is in the subdir → `claude --resume` from the subdir misses the
root-anchored JSONL. Worktrees are just one instance; Claude wandering into a
nested subdir is the more common trigger.

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

## Fix locus / hook API: handled in the bash hook; Portal needs no change

### Options Considered
**A — Portal does nothing; the bash hook bakes the `cd` into the command string.**
- Pros: zero Portal change, no release, no schema migration; respects Portal's
  design (it runs an *opaque* command — CWD-correctness is the command author's
  job); keeps Portal the sole writer of `hooks.json` (the hook still writes via
  `portal hooks set`); the existing prune loop's UUID-grep still works.
- Cons: the caller owns shell-quoting the path; the CWD is invisible to Portal
  (buried in the command string).

**B — Portal adds an opt-in `--resume-cwd` flag (structured metadata).**
- Pros: generic and reusable beyond Claude; Portal controls path quoting (could
  `os.Chdir` natively); CWD becomes introspectable in `hooks list`.
- Cons: real machinery (flag + storage change + helper logic + tests) for a
  single consumer; raises schema back-compat, audit-visibility, and
  auto-capture-opt-out questions (review F1/F6/F7); auto-capture variant would
  silently change execution dir for existing non-Claude hooks.

### Decision
**Option A.** The fix is a one-line edit to `portal-resume-hook.sh` (outside this
repo): change `portal hooks set --on-resume "claude --resume $SESSION_ID"` to
`portal hooks set --on-resume "cd $PWD && claude --resume $SESSION_ID"`. **Portal
itself needs no change.**

Deciding factor: Portal's hook abstraction is "run this opaque string in the
restored pane." CWD-anchoring a command is the command author's responsibility,
and the author (the bash hook) already holds the only thing Portal can't recover
on its own — the launch CWD (`$PWD` at SessionStart). Portal structurally cannot
know the launch dir (it only ever sees the drifted `pane_current_path`), so even
a Portal-owned design would still depend on the hook feeding it the value. Given
that, B is machinery for one consumer with no real payoff. The user reached the
same conclusion independently ("this is out of scope for Portal").

Confidence: high. B remains a clean future option if a *second*, non-Claude
consumer ever needs CWD-anchored resume — at which point genericity would start
to earn its keep.

---

## cd-wrap mechanism: `cd <launch> && claude --resume <UUID>` (no subshell)

### Journey
Explored a subshell form `(cd <launch> && claude --resume)` whose only effect is
to leave the post-Claude interactive shell in the *drifted* dir. Once the user
deprioritised the post-exit shell CWD ("that could be hours from now"), the
subshell solved a non-problem and was dropped.

### Decision
The hook contributes `cd <launch> && claude --resume <UUID>`; the hydrate helper
appends its existing `; exec $SHELL`, giving the full executed line
`cd <launch> && claude --resume <UUID>; exec $SHELL`.

- **`&&` (not `;`) between `cd` and `claude`:** if the launch dir was deleted
  (worktree removed — review F8), `cd` fails, the resume is skipped, and the
  trailing `exec $SHELL` still lands the pane in a usable shell. Graceful
  degradation for free, no special-casing.
- **No subshell / no `cd -`:** post-Claude shell CWD doesn't matter to the user,
  so the simplest form wins. (Subshell semantics confirmed harmless either way:
  Claude runs as a normal foreground child; `exec $SHELL` *replaces* the `sh`,
  so there is never a nested-shell stack.)

---

## Review findings — disposition

The set-001 review raised 9 items. After choosing Option A:

- **F3** (`$PWD` == encoded dir) — validated empirically; see Context (held as a
  symlink-path known limitation).
- **F5** (post-Claude drifted shell) — moot; user deprioritised post-exit CWD.
- **F2** (prune-loop interaction) — verified fine: the UUID-grep still matches
  inside `cd … && claude --resume <UUID>`.
- **F4** (resume re-fires SessionStart) — verified benign: the helper resumes
  *from* root, so the re-fired hook re-captures `$PWD` == root and re-writes the
  same correct anchor; self-correcting, not drifting.
- **F8** (cd to a deleted launch dir) — handled by `&&` (graceful skip → shell).
- **F1 / F6 / F7 / F9** (schema back-compat, non-Claude opt-out, audit
  visibility, hand-set-hook ownership) — all B-specific; retired by choosing A
  (no Portal schema, no Portal-applied wrap, so non-Claude/hand-set hooks are
  untouched).

---

## Summary

### Key Insights
1. A Claude session's **launch CWD is the single, inseparable anchor** — it is
   both where the session is found and where Claude operates. You cannot split
   them with a `cd`.
2. **Resuming in root is self-correcting, not a compromise.** Claude can `!cd`
   itself (that's what causes the drift), so re-navigation on resume just replays
   how the session drifted originally.
3. **Portal's hook system is an opaque-command runner by design.** CWD-anchoring
   is the command author's job; Portal can't even recover the launch dir on its
   own. So the fix belongs in the caller, not Portal.
4. The bug is **not worktree-specific** — any post-launch CWD drift triggers it;
   worktrees are one instance, Claude wandering into a nested subdir is the
   commoner one.

### Outcome
- **Fix:** one-line change in `~/.dotfiles/home/.claude/hooks/portal-resume-hook.sh`
  — register `cd $PWD && claude --resume $SESSION_ID` instead of the bare resume.
- **Portal code change: none.** This work unit resolves to a dotfiles fix; Portal's
  current hook API is already sufficient.

### Open Threads
- **Symlinked launch paths** (known limitation): `$PWD` is a proxy for Claude's
  encoded dir and would diverge if a launch path sits under a symlink. Not solved
  (user's paths are symlink-free); the authoritative source (JSONL parent dir)
  isn't available at SessionStart.
- **Option B (`--resume-cwd`)** remains a clean future option if a second,
  non-Claude consumer ever needs CWD-anchored resume.

### Current State
All six subtopics decided. Path forward is clear and small. Note for downstream
phases: since the fix is entirely outside this repo, the in-repo deliverable may
be limited to documentation (e.g. a note that Portal's opaque-command hooks are
CWD-sensitive and the caller owns anchoring).
