// Package cmd defines the CLI commands for Portal.
package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// supportedShells lists the shells that portal init supports.
var supportedShells = map[string]bool{
	"bash": true,
	"zsh":  true,
	"fish": true,
}

var initCmd = &cobra.Command{
	Use:       "init [shell]",
	Short:     "Output shell integration script",
	Long:      "Output shell functions and tab completions for eval. Usage: eval \"$(portal init zsh)\"",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish"},
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := args[0]
		if !supportedShells[shell] {
			return fmt.Errorf("unsupported shell: %s (supported: bash, zsh, fish)", shell)
		}

		w := cmd.OutOrStdout()

		switch shell {
		case "zsh":
			return emitZshInit(w)
		default:
			return fmt.Errorf("unsupported shell: %s (supported: bash, zsh, fish)", shell)
		}
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

// emitZshInit writes the zsh shell integration script to w.
// It emits shell functions, Cobra-generated completions, and compdef wiring.
func emitZshInit(w io.Writer) error {
	// Shell functions
	if _, err := fmt.Fprintln(w, `function x() { portal open "$@" }`); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, `function xctl() { portal "$@" }`); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	// Cobra-generated zsh completions for the portal binary
	if err := rootCmd.GenZshCompletion(w); err != nil {
		return fmt.Errorf("generating zsh completions: %w", err)
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	// Wire completions to shell function names
	if _, err := fmt.Fprintln(w, "compdef _portal x"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "compdef _portal xctl"); err != nil {
		return err
	}

	return nil
}
