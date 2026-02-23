package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

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
			name:      "wires completions to x name",
			wantInOut: "compdef _portal x",
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

	// Cobra's zsh completion generates a completion function named _portal.
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
			name:      "wires completions to x name",
			wantInOut: "complete -o default -F __start_portal x",
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
				"complete -o default -F __start_portal p",
				"complete -o default -F __start_portal pctl",
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
			name:      "wires completions to x name",
			wantInOut: "complete -c x -w portal",
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
				"complete -c p -w portal",
				"complete -c pctl -w portal",
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

	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
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
				"compdef _portal p",
				"compdef _portal pctl",
			},
		},
		{
			name: "default without cmd flag uses x and xctl",
			args: []string{"init", "zsh"},
			wantInOut: []string{
				`function x() { portal open "$@" }`,
				`function xctl() { portal "$@" }`,
				"compdef _portal x",
				"compdef _portal xctl",
			},
		},
		{
			name: "cmd flag with different name",
			args: []string{"init", "zsh", "--cmd", "portal"},
			wantInOut: []string{
				`function portal() { portal open "$@" }`,
				`function portalctl() { portal "$@" }`,
				"compdef _portal portal",
				"compdef _portal portalctl",
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
