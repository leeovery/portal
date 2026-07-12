package spawn

// ghosttyAdapter is the native window-spawning driver for the Ghostty terminal.
//
// Placeholder — real osascript driver lands in Tasks 2.4/2.5.
type ghosttyAdapter struct{}

// newGhosttyAdapter builds the native Ghostty adapter.
//
// Placeholder — real osascript driver lands in Tasks 2.4/2.5.
func newGhosttyAdapter() *ghosttyAdapter {
	return &ghosttyAdapter{}
}

// OpenWindow satisfies the Adapter interface.
//
// Placeholder — real osascript driver lands in Tasks 2.4/2.5. It performs no
// osascript call and builds no argv; it exists only so the resolver can
// construct and type-assert the native Ghostty adapter.
func (g *ghosttyAdapter) OpenWindow(command []string) Result {
	return Unsupported("ghostty driver not yet implemented (Task 2.5)")
}

// Compile-time assertion that *ghosttyAdapter satisfies the Adapter contract.
var _ Adapter = (*ghosttyAdapter)(nil)
