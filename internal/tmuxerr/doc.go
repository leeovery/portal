// Package tmuxerr is a dependency-free leaf package that holds typed error
// sentinels shared across the internal/tmux <-> internal/state boundary.
//
// It exists to break what would otherwise be an import cycle: internal/tmux
// already imports internal/state (for daemon-state plumbing in
// internal/tmux/hooks_register.go), so internal/state cannot import
// internal/tmux directly. Sentinels that both packages need to reference —
// notably ErrNoSuchSession, which is wrapped at the internal/tmux boundary
// and classified by errors.Is at the internal/state boundary — therefore
// live here instead.
//
// This package MUST NOT import any other internal/* package; doing so would
// reintroduce the cycle. Only standard-library imports are permitted.
package tmuxerr
