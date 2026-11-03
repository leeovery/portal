//go:build !darwin

package proctable

func List() ([]Process, error) { return nil, ErrUnsupported }

func ReadArgs(int) (Args, error) { return Args{}, ErrUnsupported }
