package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// emitInitScript runs `init <shell>` in-process and returns what it emitted.
func emitInitScript(t *testing.T, shell string, args ...string) string {
	t.Helper()

	buf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(buf)
	rootCmd.SetArgs(append([]string{"init", shell}, args...))

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init %s: unexpected error: %v", shell, err)
	}
	return buf.String()
}

func TestInitZsh(t *testing.T) {
	tests := []struct {
		name      string
		wantInOut string
	}{
		{
			name:      "outputs x function routing to portal open",
			wantInOut: "function x() { portal open \"$@\" }",
		},
		{
			name:      "outputs xctl function routing to portal",
			wantInOut: "function xctl() { portal \"$@\" }",
		},
		{
			name:      "outputs zsh completion setup",
			wantInOut: "compdef _portal portal",
		},
		{
			name:      "wires completions to x name through the open shim",
			wantInOut: "compdef _portal_open x",
		},
		{
			name:      "defines the zsh open completion shim",
			wantInOut: "_portal_open() {\n    words=(portal open \"${(@)words[2,-1]}\")\n    (( CURRENT += 1 ))\n    _portal \"$@\"\n}",
		},
		{
			name:      "wires completions to xctl name",
			wantInOut: "compdef _portal xctl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			resetRootCmd()
			rootCmd.SetOut(buf)
			rootCmd.SetArgs([]string{"init", "zsh"})

			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			output := buf.String()
			if !strings.Contains(output, tt.wantInOut) {
				t.Errorf("output does not contain %q\ngot:\n%s", tt.wantInOut, output)
			}
		})
	}
}

func TestInitZsh_OutputContainsCompletionFunction(t *testing.T) {
	buf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"init", "zsh"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "_portal") {
		t.Errorf("output does not contain Cobra-generated completion function _portal\ngot:\n%s", output)
	}
}

func TestInitZsh_UnsupportedShell(t *testing.T) {
	resetRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "powershell"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unsupported shell, got nil")
	}

	want := "unsupported shell: powershell (supported: bash, zsh, fish)"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestInitZsh_RequiresShellArgument(t *testing.T) {
	resetRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing shell argument, got nil")
	}
}

func TestInitBash(t *testing.T) {
	tests := []struct {
		name      string
		wantInOut string
	}{
		{
			name:      "outputs x function routing to portal open",
			wantInOut: `x() { portal open "$@"; }`,
		},
		{
			name:      "outputs xctl function routing to portal",
			wantInOut: `xctl() { portal "$@"; }`,
		},
		{
			name:      "outputs bash completion registration for portal",
			wantInOut: "complete -o default -F __start_portal portal",
		},
		{
			name:      "wires completions to x name through the open shim",
			wantInOut: "complete -o default -F __start_portal_open x",
		},
		{
			name:      "defines the bash open completion shim",
			wantInOut: "__start_portal_open() {\n    local typed=${COMP_WORDS[0]} expansion=\"portal open\"\n    COMP_WORDS=(portal open \"${COMP_WORDS[@]:1}\")\n    (( COMP_CWORD += 1 ))\n    COMP_LINE=${COMP_LINE/\"$typed\"/$expansion}\n    (( COMP_POINT += ${#expansion} - ${#typed} ))\n    __start_portal \"$@\"\n}",
		},
		{
			name:      "wires completions to xctl name",
			wantInOut: "complete -o default -F __start_portal xctl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			resetRootCmd()
			rootCmd.SetOut(buf)
			rootCmd.SetArgs([]string{"init", "bash"})

			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			output := buf.String()
			if !strings.Contains(output, tt.wantInOut) {
				t.Errorf("output does not contain %q\ngot:\n%s", tt.wantInOut, output)
			}
		})
	}
}

func TestInitBash_CmdFlag(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantInOut []string
	}{
		{
			name: "cmd flag changes launcher function name",
			args: []string{"init", "bash", "--cmd", "p"},
			wantInOut: []string{
				`p() { portal open "$@"; }`,
			},
		},
		{
			name: "cmd flag appends ctl suffix for control function",
			args: []string{"init", "bash", "--cmd", "p"},
			wantInOut: []string{
				`pctl() { portal "$@"; }`,
			},
		},
		{
			name: "cmd flag wires completions to custom names",
			args: []string{"init", "bash", "--cmd", "p"},
			wantInOut: []string{
				"complete -o default -F __start_portal_open p\n",
				"complete -o default -F __start_portal pctl\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			resetRootCmd()
			rootCmd.SetOut(buf)
			rootCmd.SetArgs(tt.args)

			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			output := buf.String()
			for _, want := range tt.wantInOut {
				if !strings.Contains(output, want) {
					t.Errorf("output does not contain %q\ngot:\n%s", want, output)
				}
			}
		})
	}
}

func TestInitFish(t *testing.T) {
	tests := []struct {
		name      string
		wantInOut string
	}{
		{
			name:      "outputs x function routing to portal open",
			wantInOut: "function x\n    portal open $argv\nend",
		},
		{
			name:      "outputs xctl function routing to portal",
			wantInOut: "function xctl\n    portal $argv\nend",
		},
		{
			name:      "outputs fish completion for portal",
			wantInOut: "complete -c portal",
		},
		{
			name:      "wires completions to x name through the open wrap",
			wantInOut: "complete -c x -f\ncomplete -c x -w 'portal open'\n",
		},
		{
			name:      "wires completions to xctl name",
			wantInOut: "complete -c xctl -w portal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			resetRootCmd()
			rootCmd.SetOut(buf)
			rootCmd.SetArgs([]string{"init", "fish"})

			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			output := buf.String()
			if !strings.Contains(output, tt.wantInOut) {
				t.Errorf("output does not contain %q\ngot:\n%s", tt.wantInOut, output)
			}
		})
	}
}

func TestInitFish_CmdFlag(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantInOut []string
	}{
		{
			name: "cmd flag changes launcher function name",
			args: []string{"init", "fish", "--cmd", "p"},
			wantInOut: []string{
				"function p\n    portal open $argv\nend",
			},
		},
		{
			name: "cmd flag appends ctl suffix for control function",
			args: []string{"init", "fish", "--cmd", "p"},
			wantInOut: []string{
				"function pctl\n    portal $argv\nend",
			},
		},
		{
			name: "cmd flag wires completions to custom names",
			args: []string{"init", "fish", "--cmd", "p"},
			wantInOut: []string{
				"complete -c p -f\ncomplete -c p -w 'portal open'\n",
				"complete -c pctl -w portal\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			resetRootCmd()
			rootCmd.SetOut(buf)
			rootCmd.SetArgs(tt.args)

			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			output := buf.String()
			for _, want := range tt.wantInOut {
				if !strings.Contains(output, want) {
					t.Errorf("output does not contain %q\ngot:\n%s", want, output)
				}
			}
		})
	}
}

func TestInitUnsupportedShell_ErrorMessage(t *testing.T) {
	resetRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "nushell"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unsupported shell, got nil")
	}

	want := "unsupported shell: nushell (supported: bash, zsh, fish)"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestInitUnsupportedShell_IsUsageError(t *testing.T) {
	resetRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "nushell"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unsupported shell, got nil")
	}

	if _, ok := errors.AsType[*UsageError](err); !ok {
		t.Errorf("expected UsageError, got %T: %v", err, err)
	}
}

func TestInitZsh_CmdFlag(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantInOut []string
	}{
		{
			name: "cmd flag changes launcher function name",
			args: []string{"init", "zsh", "--cmd", "p"},
			wantInOut: []string{
				`function p() { portal open "$@" }`,
			},
		},
		{
			name: "cmd flag appends ctl suffix for control function",
			args: []string{"init", "zsh", "--cmd", "p"},
			wantInOut: []string{
				`function pctl() { portal "$@" }`,
			},
		},
		{
			name: "cmd flag wires completions to custom names",
			args: []string{"init", "zsh", "--cmd", "p"},
			wantInOut: []string{
				"compdef _portal_open p\n",
				"compdef _portal pctl\n",
			},
		},
		{
			name: "default without cmd flag uses x and xctl",
			args: []string{"init", "zsh"},
			wantInOut: []string{
				`function x() { portal open "$@" }`,
				`function xctl() { portal "$@" }`,
				"compdef _portal_open x\n",
				"compdef _portal xctl\n",
			},
		},
		{
			name: "cmd flag with a name shadowing a real command",
			args: []string{"init", "zsh", "--cmd", "portal"},
			wantInOut: []string{
				`function portal() { portal open "$@" }`,
				`function portalctl() { portal "$@" }`,
				"compdef _portal_open portal\n",
				"compdef _portal portalctl\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			resetRootCmd()
			rootCmd.SetOut(buf)
			rootCmd.SetArgs(tt.args)

			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			output := buf.String()
			for _, want := range tt.wantInOut {
				if !strings.Contains(output, want) {
					t.Errorf("output does not contain %q\ngot:\n%s", want, output)
				}
			}
		})
	}
}

func TestInitCmdFlag_RegistrationsCarryNoDefaultName(t *testing.T) {
	tests := []struct {
		name     string
		shell    string
		notInOut []string
	}{
		{
			name:     "bash registrations name only the configured function",
			shell:    "bash",
			notInOut: []string{"__start_portal_open x", "__start_portal xctl"},
		},
		{
			name:     "zsh registrations name only the configured function",
			shell:    "zsh",
			notInOut: []string{"compdef _portal_open x", "compdef _portal xctl"},
		},
		{
			name:     "fish registrations name only the configured function",
			shell:    "fish",
			notInOut: []string{"complete -c x ", "complete -c xctl "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := emitInitScript(t, tt.shell, "--cmd", "p")

			for _, unwanted := range tt.notInOut {
				if strings.Contains(output, unwanted) {
					t.Errorf("output contains %q\ngot:\n%s", unwanted, output)
				}
			}
		})
	}
}

func TestInitBash_EmitsNoPinnedCompletionCursor(t *testing.T) {
	buf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"init", "bash"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pinned := "COMP_POINT=${#COMP_LINE}"; strings.Contains(buf.String(), pinned) {
		t.Errorf("output pins the completion cursor with %q\ngot:\n%s", pinned, buf.String())
	}
}
