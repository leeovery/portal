# Consolidation Findings: Killing All Sessions Wipes Restore State (Phase 4)

## Findings

None survived the bar.

## Comment Corrections

- internal/state/commit.go:62 — the doc names only an absent file, but any read failure (permission, I/O) also returns nil, as both callers' docs ("could not be read", "cannot be read") already say
  OLD: // readPriorIndex returns nil when sessions.json is absent or not decodable.
  NEW: // readPriorIndex returns nil when sessions.json cannot be read or decoded.

- internal/tmuxerr/errors.go:19-22 — the clause is a worked example, and it is true only some of the time: a name whose text after the `|` is numeric (`api|2`) splits into fields that parse, so the line is read as a session named `api` and no unparseable error is raised
  OLD: // ErrSessionListUnparseable is wrapped by the session-list readers when tmux
// answered list-sessions with a line Portal cannot parse — a session name
// carrying the field separator is one. It marks a listing tmux did answer, so a
// caller can tell it from a failed list-sessions read.
  NEW: // ErrSessionListUnparseable is wrapped by the session-list readers when tmux
// answered list-sessions with a line Portal cannot parse. It marks a listing
// tmux did answer, so a caller can tell it from a failed list-sessions read.
