//go:build !integration

package state

import "time"

// Every non-integration build judges by the kernel's boot time: the boot time
// sandbox is the integration build's alone.
func sandboxBootTime(read func() (time.Time, error)) func() (time.Time, error) { return read }
