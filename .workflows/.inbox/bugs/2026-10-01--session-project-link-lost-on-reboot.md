# A session's project link is lost on every reboot

Portal records which project directory a session belongs to when it creates the session — the `@portal-dir` session user-option, stamped from the resolved git root in `internal/session/create.go` and `internal/session/quickstart.go`. The documented promise is that this anchors the session to its origin directory for good, so the session stays grouped under the right project even after its pane `cd`s somewhere else. That promise doesn't survive a reboot.

After a reboot and restore, restored sessions carry no `@portal-dir` at all. Every one of them falls back to the lazy resolution path, which guesses the directory from wherever the session's active pane currently sits (pane `current_path` → git root). Only sessions created since the last reboot carry the stamp.

Observed on the live install on 2026-10-01, read-only: of 38 live sessions, 34 had an empty `@portal-dir` and were grouping on the guess; the 4 carrying a stamp were all created since the last reboot. The saved session record in `sessions.json` (`internal/state/schema.go`) has no directory field, and nothing on the restore path writes `@portal-dir` back.

The impact is silent mis-grouping. A restored session whose active pane has wandered away from its origin directory is filed under whatever project that pane is in now, or lands in Unknown, in both the By Project and By Tag views. Nothing signals that anything is off; the session looks correctly grouped, just under the wrong heading. Because tags are anchored to project directories, a session's effective tags are wrong in exactly the same cases.

This was split out of the tagging and filtering work (tag suggestions, bulk tagging, filtering by tag or project) as a prerequisite to it. Everything that work builds keys off the session-to-project link, so a link that degrades to a guess after every reboot would make a tag or project filter silently drop or misplace sessions.

Out of scope here, deliberately kept with the tagging and filtering work as a design question rather than this bug: sessions that land in Unknown because their directory is legitimately not a known project. Two shapes were seen on the live install: sessions working inside a git worktree (e.g. under `agentic-workflows/.claude/worktrees/…` and a `switchboard` worktree), which resolve to the worktree folder rather than the parent repo's project, and sessions in folders never registered as projects (four in `~/Code/flowx` where the saved project is `~/Code/fabric/flowx`, and one in `~/Code/goosemates`).
