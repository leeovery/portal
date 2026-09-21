//go:build integration

package bootstrap_test

import (
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

type daemonTickOpts struct {
	skipGuard       bool
	emptyScrollback bool
}

type daemonTickOption func(*daemonTickOpts)

func withoutSkipGuard() daemonTickOption {
	return func(o *daemonTickOpts) { o.skipGuard = false }
}

func withEmptyScrollback() daemonTickOption {
	return func(o *daemonTickOpts) { o.emptyScrollback = true }
}

func runDaemonTick(
	t *testing.T,
	client *tmux.Client,
	stateDir string,
	options ...daemonTickOption,
) state.Index {
	t.Helper()

	opts := daemonTickOpts{skipGuard: true}
	for _, apply := range options {
		apply(&opts)
	}

	skipSet, err := state.ListSkeletonMarkers(client)
	if err != nil {
		t.Fatalf("ListSkeletonMarkers: %v", err)
	}

	idx, _, err := state.CaptureStructure(client, skipSet, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}

	hm := state.HashMap{}
	anyChanged := false
	for _, sess := range idx.Sessions {
		for _, win := range sess.Windows {
			for _, pane := range win.Panes {
				key := state.SanitizePaneKey(sess.Name, win.Index, pane.Index)
				if opts.skipGuard {
					if _, skipped := skipSet[key]; skipped {
						continue
					}
				}

				var (
					data []byte
					hash uint64
				)
				if !opts.emptyScrollback {
					target := tmux.PaneTargetExact(sess.Name, win.Index, pane.Index)
					data, hash, err = state.CaptureAndHashPane(client, target)
					if err != nil {
						t.Fatalf("CaptureAndHashPane %s: %v", target, err)
					}
				}

				written, err := state.WriteScrollbackIfChanged(stateDir, key, data, hash, hm)
				if err != nil {
					t.Fatalf("WriteScrollbackIfChanged %s: %v", key, err)
				}
				if written {
					anyChanged = true
				}
			}
		}
	}

	if err := state.Commit(stateDir, idx, anyChanged, nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	return idx
}
