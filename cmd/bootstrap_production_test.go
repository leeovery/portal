package cmd

import (
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hooksweep"
	"github.com/leeovery/portal/internal/tmux"
)

var _ bootstrap.LatchWriter = (*tmux.Client)(nil)

var _ hooksweep.Reader = (*tmux.Client)(nil)

func keysOf(m hooks.Snapshot) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
