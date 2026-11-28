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
		if cmdName == "portal" {
			return NewUsageError(portalNameRefusal)
		}

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

// portalNameRefusal refuses --cmd portal: a function named portal hides the
// binary from everything that runs it bare, portalctl's commands and the shells'
// completion among them, which would reach the opener instead.
const portalNameRefusal = "--cmd portal would hide the portal binary behind the opening function: choose another name"

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().String("cmd", "x", "Custom name for shell functions (e.g., --cmd p creates p() and pctl())")
}

// openFunctionExpansion is what the session-opening function runs and what its
// completion shim asks Portal to complete for. Both roles read it from here so a
// change to one cannot leave the other asking for a command line nothing runs.
const openFunctionExpansion = "portal open"

// The session-opening functions leave the shell in the directory of the last
// session Portal left, which a child process can't do for its parent: they hand
// Portal a temp file through --cwd-file, read it once Portal exits, and change to
// the directory it names if that exists, returning Portal's own status. Portal
// writes nothing for a run that attached no session, so the shell stays put. With
// no temp file to be had, they run Portal as before. Their variables are local and
// prefixed; mktemp, rm, fish's cat and Portal itself are reached through
// `command`, and bash's and zsh's cd through `builtin`, so nothing the user aliased
// or defined intercepts them. Under errexit and zsh's err_return, Portal's status
// is taken on the right of ||, a file gone before it is read reads as empty, and a
// cd that fails leaves the shell where it is, so none of them returns before the
// file is removed or ends the shell. Each is a format string: the function's
// name, then openFunctionExpansion.
const bashOpenFunction = "%[1]s() {\n" + bashZshOpenFunctionBody

// zsh's function keyword keeps an alias of the name from expanding as the
// function is defined.
const zshOpenFunction = "function %[1]s() {\n" + bashZshOpenFunctionBody

// bash and zsh read the file with $(<…), which no alias or function reaches.
const bashZshOpenFunctionBody = `    local __portal_file __portal_status=0 __portal_dir
    __portal_file=$(command mktemp "${TMPDIR:-/tmp}/portal-cwd.XXXXXX" 2>/dev/null) || { command %[2]s "$@"; return; }
    command %[2]s --cwd-file "$__portal_file" "$@" || __portal_status=$?
    { __portal_dir=$(<"$__portal_file"); } 2>/dev/null || __portal_dir=
    command rm -f -- "$__portal_file"
    if [[ -n $__portal_dir && -d $__portal_dir ]]; then
        builtin cd -- "$__portal_dir" || :
    fi
    return "$__portal_status"
}
`

// fish's own cd keeps dirprev, so it is the one to call. string collect keeps
// a directory name holding a newline whole, trimming a trailing one as bash's
// and zsh's $(<…) do.
const fishOpenFunction = `function %[1]s
    set -l __portal_tmp $TMPDIR
    test -n "$__portal_tmp"; or set __portal_tmp /tmp
    set -l __portal_file (command mktemp "$__portal_tmp/portal-cwd.XXXXXX" 2>/dev/null)
    or begin
        command %[2]s $argv
        return $status
    end
    command %[2]s --cwd-file "$__portal_file" $argv
    set -l __portal_status $status
    set -l __portal_dir (command cat -- "$__portal_file" | string collect)
    command rm -f -- "$__portal_file"
    if test -n "$__portal_dir"; and test -d "$__portal_dir"
        cd "$__portal_dir"
    end
    return $__portal_status
end
`

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

	if _, err := fmt.Fprintf(w, bashOpenFunction, cmdName, openFunctionExpansion); err != nil {
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

	if _, err := fmt.Fprintf(w, fishOpenFunction, cmdName, openFunctionExpansion); err != nil {
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

	if _, err := fmt.Fprintf(w, zshOpenFunction, cmdName, openFunctionExpansion); err != nil {
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
