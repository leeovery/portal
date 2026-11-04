//go:build !darwin && !linux

package bootid

import "errors"

func read() (string, error) { return "", errors.ErrUnsupported }
