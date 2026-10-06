package tmuxerr

import "errors"

// ErrNoSuchSession is wrapped by per-session tmux operations when tmux reports
// the addressed session does not exist. internal/tmux re-exports it as
// tmux.ErrNoSuchSession; the two symbols are identity-equal.
var ErrNoSuchSession = errors.New("no such session")

// ErrUnaddressableSessionName is wrapped by per-session tmux operations given a
// name tmux cannot be handed back — whether as an exact target or as the bare
// positional a rename carries. tmux.ValidateSessionName enumerates the rules
// that refuse one. It is deliberately distinct from ErrNoSuchSession — tmux
// answers such a target with the very same "no such session" stderr, and
// reading that as a vanished session drops a live session from the capture with
// nothing to show for it.
var ErrUnaddressableSessionName = errors.New("session name not addressable by exact target")

// ErrSessionListUnparseable is wrapped by the session-list readers when tmux
// answered list-sessions with a line Portal cannot parse. It marks a listing
// tmux did answer, so a caller can tell it from a failed list-sessions read.
var ErrSessionListUnparseable = errors.New("session list unparseable")
