// Package bootid reads what the kernel says of the machine's current boot:
// the id it draws afresh at each boot and fixes for the boot's life, and when
// the boot began.
package bootid

import (
	"errors"
	"fmt"
	"time"
)

// Read returns the current boot's id. An OS whose kernel keeps none fails
// with errors.ErrUnsupported.
func Read() (string, error) {
	id, err := read()
	if err != nil {
		return "", fmt.Errorf("read boot id: %w", err)
	}
	if id == "" {
		return "", errors.New("read boot id: the kernel reported an empty one")
	}
	return id, nil
}

// BootTime returns when the current boot began. Unlike the id it is worked
// out against the wall clock, to the second on linux, so it moves when the
// clock is stepped: compare a time against it with room to spare. An OS that
// keeps none fails with errors.ErrUnsupported.
func BootTime() (time.Time, error) {
	at, err := bootTime()
	if err != nil {
		return time.Time{}, fmt.Errorf("read boot time: %w", err)
	}
	return at, nil
}
