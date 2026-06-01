// Package log is the single owner of all logging machinery in portal.
//
// It wraps the standard library's log/slog. The root *slog.Logger is
// constructed exactly once, in this package's own init, over a small custom
// handler whose inner delegate is swappable behind a synchronized indirection.
// Because every consumer imports this package, Go runs its init first, so the
// root logger exists before any For call — For never returns nil.
//
// Single-owner invariant: no *slog.Logger is constructed anywhere outside this
// package. Consumers bind a component-scoped child logger once at package init
// via:
//
//	var logger = log.For("<component>")
//
// Loggers cached this way pick up a later handler swap automatically because
// the swap lives inside the shared handler indirection, not on the cached
// logger itself.
package log
