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

var openCmd = &cobra.Command{
	Use:   "open [path]",
	Short: "Open the interactive session picker or start a session at a path",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 && resolver.IsPathArgument(args[0]) {
			return openPath(args[0])
		}

		return openTUI()
	},
}

// openPath resolves a path argument, creates a new tmux session, and execs into it.
func openPath(arg string) error {
	resolvedPath, err := resolver.ResolvePath(arg)
	if err != nil {
		return err
	}

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

// openTUI launches the interactive session picker.
func openTUI() error {
	client := tmux.NewClient(&tmux.RealCommander{})
	m := tui.New(client)
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

func init() {
	rootCmd.AddCommand(openCmd)
}
