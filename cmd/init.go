package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

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
			return NewUsageError(fmt.Sprintf("unsupported shell: %s (supported: bash, zsh, fish)", shell))
		}

		cmdName, _ := cmd.Flags().GetString("cmd")

		w := cmd.OutOrStdout()

		switch shell {
		case "bash":
			return emitBashInit(w, cmdName)
		case "zsh":
			return emitZshInit(w, cmdName)
		case "fish":
			return emitFishInit(w, cmdName)
		default:
			// unreachable: the supportedShells check above rejects these.
			return NewUsageError(fmt.Sprintf("unsupported shell: %s (supported: bash, zsh, fish)", shell))
		}
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().String("cmd", "x", "Custom name for shell functions (e.g., --cmd p creates p() and pctl())")
}

// openFunctionExpansion is what the session-opening function runs and what its
// completion shim asks Portal to complete for. Both roles read it from here so a
// change to one cannot leave the other asking for a command line nothing runs.
const openFunctionExpansion = "portal open"

// The generated script asks the typed word for its completions, and the session-opening
// function expands to openFunctionExpansion — so its registration must name a shim that
// rewrites the word rather than __start_portal itself, which sends its request through the
// typed word and so would ask the expansion, not portal, for the completions. The line must
// match the rewritten words or the cursor walk reads the wrong current word from it, so it
// is substituted into rather than rebuilt: re-joining the array would normalise the user's
// own spacing, and pinning the offset to the end would move a cursor they left mid-line. The
// replacement is unanchored so it also lands on a line that begins with whitespace. The shim
// is a format string, so any % added to it must be escaped.
const bashOpenCompletionShim = `__start_portal_open() {
    local typed=${COMP_WORDS[0]} expansion="%[1]s"
    COMP_WORDS=(%[1]s "${COMP_WORDS[@]:1}")
    (( COMP_CWORD += 1 ))
    COMP_LINE=${COMP_LINE/"$typed"/$expansion}
    (( COMP_POINT += ${#expansion} - ${#typed} ))
    __start_portal "$@"
}
`

const zshOpenCompletionShim = `_portal_open() {
    words=(%s "${(@)words[2,-1]}")
    (( CURRENT += 1 ))
    _portal "$@"
}
`

func emitBashInit(w io.Writer, cmdName string) error {
	ctlName := cmdName + "ctl"

	if _, err := fmt.Fprintf(w, "%s() { %s \"$@\"; }\n", cmdName, openFunctionExpansion); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s() { portal \"$@\"; }\n", ctlName); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if err := rootCmd.GenBashCompletionV2(w, true); err != nil {
		return fmt.Errorf("generating bash completions: %w", err)
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, bashOpenCompletionShim, openFunctionExpansion); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "complete -o default -F __start_portal_open %s\n", cmdName); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "complete -o default -F __start_portal %s\n", ctlName); err != nil {
		return err
	}

	return nil
}

func emitFishInit(w io.Writer, cmdName string) error {
	ctlName := cmdName + "ctl"

	if _, err := fmt.Fprintf(w, "function %s\n    %s $argv\nend\n", cmdName, openFunctionExpansion); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "function %s\n    portal $argv\nend\n", ctlName); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if err := rootCmd.GenFishCompletion(w, true); err != nil {
		return fmt.Errorf("generating fish completions: %w", err)
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, "complete -c %s -f\n", cmdName); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "complete -c %s -w '%s'\n", cmdName, openFunctionExpansion); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "complete -c %s -w portal\n", ctlName); err != nil {
		return err
	}

	return nil
}

func emitZshInit(w io.Writer, cmdName string) error {
	ctlName := cmdName + "ctl"

	if _, err := fmt.Fprintf(w, "function %s() { %s \"$@\" }\n", cmdName, openFunctionExpansion); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "function %s() { portal \"$@\" }\n", ctlName); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if err := rootCmd.GenZshCompletion(w); err != nil {
		return fmt.Errorf("generating zsh completions: %w", err)
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, zshOpenCompletionShim, openFunctionExpansion); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "compdef _portal_open %s\n", cmdName); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "compdef _portal %s\n", ctlName); err != nil {
		return err
	}

	return nil
}
