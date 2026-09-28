// Package resumekeys declares the keys that act on the resume panel's screens:
// the one byte each footer names and the waiter dispatches on, so the key a
// screen offers and the key that answers it cannot drift apart. It is a leaf so
// the renderers in internal/tui and the waiter, which must not import the
// rendering path, both read the same declaration. Enter and Escape are not here:
// each has a display form distinct from the bytes that carry it.
package resumekeys

const (
	// Discard opens the discard confirmation from the waiting panel.
	Discard byte = 'd'
	// Confirm discards the pane's resume command from the confirmation.
	Confirm byte = 'y'
)
