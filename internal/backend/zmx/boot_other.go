//go:build !darwin && !linux

package zmx

import (
	"errors"
	"time"
)

func bootTime() (time.Time, error) { return time.Time{}, errors.ErrUnsupported }
