//go:build !darwin && !linux

package bootid

import (
	"errors"
	"time"
)

func read() (string, error) { return "", errors.ErrUnsupported }

func bootTime() (time.Time, error) { return time.Time{}, errors.ErrUnsupported }
