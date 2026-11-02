package zmx

import "time"

// ShortenCeilings sets the ceiling of every socket operation to d, so tests
// of what happens past one need not wait out the real ones. Create keeps its
// own: a script's first run on macOS can take a few hundred milliseconds.
func ShortenCeilings(b *Backend, d time.Duration) {
	b.limits.probe, b.limits.kill, b.limits.detach, b.limits.history = d, d, d, d
}

// ShortenCreateCeiling sets Create's ceiling to d.
func ShortenCreateCeiling(b *Backend, d time.Duration) { b.limits.create = d }
