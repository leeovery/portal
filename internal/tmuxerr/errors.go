package tmuxerr

import "errors"

// ErrNoSuchSession is the typed sentinel returned (wrapped) by per-session
// tmux operations when the underlying tmux invocation reports that the
// addressed session does not exist.
//
// The wrap site lives in internal/tmux (wrapNoSuchSession). Callers — both
// inside internal/tmux and outside (e.g. internal/state's per-session
// CaptureStructure loop) — discriminate via errors.Is against this single
// value. internal/tmux re-exports it as tmux.ErrNoSuchSession so existing
// callers of tmux.ErrNoSuchSession continue to resolve to the same sentinel
// (the two symbols are identity-equal).
var ErrNoSuchSession = errors.New("no such session")
