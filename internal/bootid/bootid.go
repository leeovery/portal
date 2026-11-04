// Package bootid reads the identity the kernel gives the machine's current
// boot: drawn afresh at each boot and fixed for its life. It is not the boot
// time, which Linux works out from a clock and so drifts between readings.
package bootid

import (
	"errors"
	"fmt"
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
