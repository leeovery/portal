package zmx

import (
	"time"

	"golang.org/x/sys/unix"
)

// bootTime counts back from now by CLOCK_BOOTTIME, which runs through
// suspend as the wall clock does.
func bootTime() (time.Time, error) {
	var up unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &up); err != nil {
		return time.Time{}, err
	}
	return time.Now().Add(-time.Duration(up.Nano())), nil
}
