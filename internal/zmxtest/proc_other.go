//go:build !darwin

package zmxtest

import "errors"

var errNoProcessReader = errors.New("zmxtest reads process environments only on darwin")

func processes() ([]int, error) { return nil, errNoProcessReader }

func processArgs(int) (procArgs, error) { return procArgs{}, errNoProcessReader }

func parentOf(int) (int, error) { return 0, errNoProcessReader }
