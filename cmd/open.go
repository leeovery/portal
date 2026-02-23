package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

// openDeps holds injectable dependencies for the open command.
// When nil, real implementations are used.
var openDeps *OpenDeps

// OpenDeps allows injecting dependencies for testing.
type OpenDeps struct {
	AliasLookup  resolver.AliasLookup
	Zoxide       resolver.ZoxideQuerier
	DirValidator resolver.DirValidator
}

var openCmd = &cobra.Command{
	Use:   "open [destination]",
	Short: "Open the interactive session picker or start a session at a path",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return openTUI("")
		}

		query := args[0]

		qr, err := buildQueryResolver()
		if err != nil {
			return err
		}

		result, err := qr.Resolve(query)
		if err != nil {
			return err
		}

		switch r := result.(type) {
		case *resolver.PathResult:
			return openPath(r.Path)
		case *resolver.FallbackResult:
			return openTUI(r.Query)
		default:
			return fmt.Errorf("unexpected resolution result: %T", result)
		}
	},
}

// openPath creates a new tmux session at the given resolved directory path and execs into it.
func openPath(resolvedPath string) error {
	client := tmux.NewClient(&tmux.RealCommander{})
	gitResolver := &resolverAdapter{}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("failed to determine config directory: %w", err)
	}
	store := project.NewStore(filepath.Join(configDir, "portal", "projects.json"))
	gen := session.NewNanoIDGenerator()

	qs := session.NewQuickStart(gitResolver, store, client, gen)
	result, err := qs.Run(resolvedPath)
	if err != nil {
		return err
	}

	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	return syscall.Exec(tmuxPath, result.ExecArgs, os.Environ())
}

// resolverAdapter adapts resolver.ResolveGitRoot to the session.GitResolver interface.
type resolverAdapter struct{}

// Resolve resolves a directory to its git repository root.
func (r *resolverAdapter) Resolve(dir string) (string, error) {
	return resolver.ResolveGitRoot(dir, &resolver.RealCommandRunner{})
}

// openTUI launches the interactive session picker with an optional initial filter.
func openTUI(initialFilter string) error {
	client := tmux.NewClient(&tmux.RealCommander{})
	m := tui.New(client)
	if initialFilter != "" {
		m = m.WithInitialFilter(initialFilter)
	}
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	model, ok := finalModel.(tui.Model)
	if !ok {
		return fmt.Errorf("unexpected model type: %T", finalModel)
	}

	selected := model.Selected()
	if selected == "" {
		return nil
	}

	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	return syscall.Exec(tmuxPath, []string{"tmux", "attach-session", "-t", selected}, os.Environ())
}

// buildQueryResolver creates a QueryResolver with appropriate dependencies.
func buildQueryResolver() (*resolver.QueryResolver, error) {
	if openDeps != nil {
		return resolver.NewQueryResolver(openDeps.AliasLookup, openDeps.Zoxide, openDeps.DirValidator), nil
	}

	store, err := loadAliasStore()
	if err != nil {
		return nil, err
	}

	zoxide := resolver.NewZoxideResolver(&resolver.RealCommandRunner{}, exec.LookPath)
	dirValidator := &resolver.OSDirValidator{}

	return resolver.NewQueryResolver(store, zoxide, dirValidator), nil
}

func init() {
	rootCmd.AddCommand(openCmd)
}
