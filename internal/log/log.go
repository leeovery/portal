package log

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
)

// swapHandler is a slog.Handler that forwards every call to a replaceable inner
// handler. The inner handler is guarded by an atomic.Pointer so a handler swap
// (via setHandler) is observed by every logger already holding a reference to
// the swapHandler — including loggers cached at package init, before Init runs.
// Each forwarded call performs exactly one synchronized read of the inner
// handler.
//
// Crucially, WithAttrs/WithGroup do NOT pre-bind their attrs onto the current
// inner handler (which would freeze the cached logger to a stale delegate).
// Instead they record the requested attrs/groups and re-apply them against the
// live inner handler at Enabled/Handle time, so a logger derived via For
// (root.With(...)) still routes through the indirection after a swap.
type swapHandler struct {
	// inner is a pointer to the shared atomic cell holding the current inner
	// handler. Derived handlers (from WithAttrs/WithGroup) reference the SAME
	// cell so a swap via setHandler is observed by all of them. It is a pointer
	// because atomic.Pointer must not be copied by value.
	inner *atomic.Pointer[slog.Handler]
	// mods is the chain of WithAttrs/WithGroup operations to replay against the
	// inner handler on each Enabled/Handle. nil for the root indirection.
	mods []handlerMod
}

// handlerMod is one deferred WithAttrs or WithGroup operation. Exactly one of
// attrs / group is set per mod.
type handlerMod struct {
	attrs []slog.Attr
	group string
}

// applyMods replays the recorded WithAttrs/WithGroup chain against h.
func (s *swapHandler) applyMods(h slog.Handler) slog.Handler {
	for _, m := range s.mods {
		if m.group != "" {
			h = h.WithGroup(m.group)
			continue
		}
		h = h.WithAttrs(m.attrs)
	}
	return h
}

// load returns the live inner handler with this swapHandler's recorded
// modification chain applied, under one atomic read of the inner handler.
func (s *swapHandler) load() slog.Handler {
	return s.applyMods(*s.inner.Load())
}

// store atomically replaces the inner handler.
func (s *swapHandler) store(h slog.Handler) {
	s.inner.Store(&h)
}

func (s *swapHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return s.load().Enabled(ctx, level)
}

func (s *swapHandler) Handle(ctx context.Context, r slog.Record) error {
	return s.load().Handle(ctx, r)
}

// WithAttrs returns a swapHandler sharing the same inner-handler pointer but
// carrying an extended modification chain, so the derived logger continues to
// observe handler swaps.
func (s *swapHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return s.derive(handlerMod{attrs: attrs})
}

// WithGroup mirrors WithAttrs for group nesting.
func (s *swapHandler) WithGroup(name string) slog.Handler {
	return s.derive(handlerMod{group: name})
}

// derive returns a swapHandler that shares s's inner-handler indirection but
// appends m to the modification chain. The shared atomic.Pointer is the same
// underlying value, so a swap via setHandler is observed by the derived
// handler too.
func (s *swapHandler) derive(m handlerMod) *swapHandler {
	mods := make([]handlerMod, len(s.mods)+1)
	copy(mods, s.mods)
	mods[len(s.mods)] = m
	return &swapHandler{inner: s.inner, mods: mods}
}

// swap is the process-wide handler indirection. It is constructed in init with
// a safe default inner handler so root is usable before Init runs.
var swap = newSwapHandler()

// root is the single *slog.Logger in the codebase, built over swap in init.
// Every For-created logger derives from root and therefore shares swap, so a
// later handler swap is observed by all of them.
var root = slog.New(swap)

// newSwapHandler returns a swapHandler whose inner delegate is the pre-Init
// default handler: a text handler writing INFO-and-above to stderr. Any
// unexpected log emitted before Init surfaces on stderr rather than silently
// vanishing; DEBUG is dropped at this level.
func newSwapHandler() *swapHandler {
	s := &swapHandler{inner: &atomic.Pointer[slog.Handler]{}}
	s.store(defaultHandler())
	return s
}

// defaultHandler builds the pre-Init safe default: INFO-and-above as text to
// stderr.
func defaultHandler() slog.Handler {
	return slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
}

// setHandler atomically swaps the inner handler behind the shared indirection.
// It is the unexported seam used by Init and SetTestHandler (downstream tasks)
// to re-point logging. Because the swap lives inside swap, every previously
// returned logger routes its records to h after this call.
func setHandler(h slog.Handler) {
	swap.store(h)
}

// For returns a component-bound child logger (root.With("component", name)).
// It is safe to call before Init — root is constructed in this package's init,
// so For always returns a valid, non-nil *slog.Logger. An empty component is
// accepted without special-casing; the closed component taxonomy is convention,
// not a runtime guard.
func For(component string) *slog.Logger {
	return root.With("component", component)
}
