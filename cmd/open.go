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

// SessionConnector connects the user to a tmux session.
// The implementation differs based on whether Portal is inside or outside tmux.
type SessionConnector interface {
	Connect(name string) error
}

// SwitchClienter defines the interface for switching tmux clients.
type SwitchClienter interface {
	SwitchClient(name string) error
}

// SwitchConnector connects to a session by issuing tmux switch-client.
// Used when Portal is running inside an existing tmux session.
type SwitchConnector struct {
	client SwitchClienter
}

// Connect switches the current tmux client to the named session.
func (sc *SwitchConnector) Connect(name string) error {
	return sc.client.SwitchClient(name)
}

// AttachConnector connects to a session by exec-ing tmux attach-session.
// Used when Portal is running outside tmux (bare shell).
type AttachConnector struct{}

// Connect replaces the current process with tmux attach-session.
func (ac *AttachConnector) Connect(name string) error {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}
	return syscall.Exec(tmuxPath, []string{"tmux", "attach-session", "-t", name}, os.Environ())
}

// buildSessionConnector returns the appropriate SessionConnector based on
// whether Portal is running inside an existing tmux session.
func buildSessionConnector() SessionConnector {
	if tmux.InsideTmux() {
		client := tmux.NewClient(&tmux.RealCommander{})
		return &SwitchConnector{client: client}
	}
	return &AttachConnector{}
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

// sessionCreatorIface creates a tmux session from a directory and returns the session name.
type sessionCreatorIface interface {
	CreateFromDir(dir string) (string, error)
}

// quickStartResult contains the result of a quick-start session creation.
type quickStartResult struct {
	SessionName string
	Dir         string
	ExecArgs    []string
}

// quickStarter runs the quick-start pipeline and returns exec args.
type quickStarter interface {
	Run(path string) (*quickStartResult, error)
}

// execer abstracts process replacement for testability.
type execer interface {
	Exec(argv0 string, argv []string, envv []string) error
}

// realExecer replaces the current process via syscall.Exec.
type realExecer struct{}

// Exec replaces the current process.
func (r *realExecer) Exec(argv0 string, argv []string, envv []string) error {
	return syscall.Exec(argv0, argv, envv)
}

// quickStartAdapter adapts session.QuickStart to the quickStarter interface.
type quickStartAdapter struct {
	qs *session.QuickStart
}

// Run runs the quick-start pipeline and converts the result.
func (a *quickStartAdapter) Run(path string) (*quickStartResult, error) {
	result, err := a.qs.Run(path)
	if err != nil {
		return nil, err
	}
	return &quickStartResult{
		SessionName: result.SessionName,
		Dir:         result.Dir,
		ExecArgs:    result.ExecArgs,
	}, nil
}

// PathOpener handles creating a new tmux session from a resolved path.
// It branches on insideTmux: inside tmux creates detached then switches;
// outside tmux uses exec handoff with -A flag.
type PathOpener struct {
	insideTmux bool
	creator    sessionCreatorIface
	switcher   SwitchClienter
	qs         quickStarter
	execer     execer
	tmuxPath   string
}

// Open creates a session at the given path and connects to it.
func (po *PathOpener) Open(resolvedPath string) error {
	if po.insideTmux {
		sessionName, err := po.creator.CreateFromDir(resolvedPath)
		if err != nil {
			return err
		}
		return po.switcher.SwitchClient(sessionName)
	}

	result, err := po.qs.Run(resolvedPath)
	if err != nil {
		return err
	}

	return po.execer.Exec(po.tmuxPath, result.ExecArgs, os.Environ())
}

// openPath creates a new tmux session at the given resolved directory path.
// When inside tmux, it creates the session detached and switches to it.
// When outside tmux, it execs into tmux with the -A flag for atomic create-or-attach.
func openPath(resolvedPath string) error {
	client := tmux.NewClient(&tmux.RealCommander{})
	gitResolver := &resolverAdapter{}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("failed to determine config directory: %w", err)
	}
	store := project.NewStore(filepath.Join(configDir, "portal", "projects.json"))
	gen := session.NewNanoIDGenerator()

	insideTmux := tmux.InsideTmux()

	opener := &PathOpener{
		insideTmux: insideTmux,
		creator:    session.NewSessionCreator(gitResolver, store, client, gen),
		switcher:   client,
		qs:         &quickStartAdapter{qs: session.NewQuickStart(gitResolver, store, client, gen)},
		execer:     &realExecer{},
	}

	if !insideTmux {
		tmuxPath, err := exec.LookPath("tmux")
		if err != nil {
			return fmt.Errorf("tmux not found: %w", err)
		}
		opener.tmuxPath = tmuxPath
	}

	return opener.Open(resolvedPath)
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
	m := tui.NewWithKiller(client, client)
	if initialFilter != "" {
		m = m.WithInitialFilter(initialFilter)
	}
	if tmux.InsideTmux() {
		sessionName, err := client.CurrentSessionName()
		if err == nil && sessionName != "" {
			m = m.WithInsideTmux(sessionName)
		}
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

	connector := buildSessionConnector()
	return connector.Connect(selected)
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
