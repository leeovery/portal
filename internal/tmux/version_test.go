package tmux_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

func TestParseTmuxVersion(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantMajor int
		wantMinor int
		wantLabel string
		wantErr   bool
	}{
		{
			name:      "parses plain semver tmux 3.3",
			raw:       "tmux 3.3",
			wantMajor: 3,
			wantMinor: 3,
			wantLabel: "3.3",
		},
		{
			name:      "parses suffixed version tmux 3.3a",
			raw:       "tmux 3.3a",
			wantMajor: 3,
			wantMinor: 3,
			wantLabel: "3.3a",
		},
		{
			name:      "parses suffixed version tmux 3.0b",
			raw:       "tmux 3.0b",
			wantMajor: 3,
			wantMinor: 0,
			wantLabel: "3.0b",
		},
		{
			name:      "parses tmux 3.0",
			raw:       "tmux 3.0",
			wantMajor: 3,
			wantMinor: 0,
			wantLabel: "3.0",
		},
		{
			name:      "parses tmux 2.9",
			raw:       "tmux 2.9",
			wantMajor: 2,
			wantMinor: 9,
			wantLabel: "2.9",
		},
		{
			name:      "parses tmux-next 4.0",
			raw:       "tmux-next 4.0",
			wantMajor: 4,
			wantMinor: 0,
			wantLabel: "4.0",
		},
		{
			name:      "parses pre-release tmux 3.3-rc",
			raw:       "tmux 3.3-rc",
			wantMajor: 3,
			wantMinor: 3,
			wantLabel: "3.3-rc",
		},
		{
			name:      "parses pre-release tmux 3.0-rc1",
			raw:       "tmux 3.0-rc1",
			wantMajor: 3,
			wantMinor: 0,
			wantLabel: "3.0-rc1",
		},
		{
			name:      "tolerates trailing parenthetical tmux 3.0 (OpenBSD)",
			raw:       "  tmux 3.0 (OpenBSD)  ",
			wantMajor: 3,
			wantMinor: 0,
			wantLabel: "3.0",
		},
		{
			name:      "trims leading and trailing whitespace",
			raw:       "   tmux 3.3a   ",
			wantMajor: 3,
			wantMinor: 3,
			wantLabel: "3.3a",
		},
		{
			name:      "treats missing minor as .0",
			raw:       "tmux 3",
			wantMajor: 3,
			wantMinor: 0,
			wantLabel: "3",
		},
		{
			name:    "errors on unparseable output",
			raw:     "unintelligible",
			wantErr: true,
		},
		{
			name:    "errors on empty input",
			raw:     "",
			wantErr: true,
		},
		{
			name:    "errors when no digit token present",
			raw:     "tmux foo bar",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, minor, label, err := tmux.ParseTmuxVersion(tt.raw)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseTmuxVersion(%q) error = nil, want non-nil", tt.raw)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseTmuxVersion(%q) unexpected error: %v", tt.raw, err)
			}
			if major != tt.wantMajor {
				t.Errorf("major = %d, want %d", major, tt.wantMajor)
			}
			if minor != tt.wantMinor {
				t.Errorf("minor = %d, want %d", minor, tt.wantMinor)
			}
			if label != tt.wantLabel {
				t.Errorf("label = %q, want %q", label, tt.wantLabel)
			}
		})
	}
}

func TestCheckTmuxVersion(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		runErr      error
		wantErr     bool
		wantErrMsg  string // exact match when set
		wantContain string // substring match when set
	}{
		{
			name:   "accepts tmux 3.0 as satisfying minimum",
			output: "tmux 3.0",
		},
		{
			name:   "accepts tmux 3.3a",
			output: "tmux 3.3a",
		},
		{
			name:   "accepts tmux 4.0",
			output: "tmux 4.0",
		},
		{
			name:   "accepts tmux-next 4.0",
			output: "tmux-next 4.0",
		},
		{
			name:   "accepts tmux 3.0 with parenthetical",
			output: "tmux 3.0 (OpenBSD)",
		},
		{
			name:       "rejects tmux 2.9 with specified user-facing error",
			output:     "tmux 2.9",
			wantErr:    true,
			wantErrMsg: "Portal requires tmux \u2265 3.0 (found 2.9). Please upgrade.",
		},
		{
			name:       "rejects tmux 1.0 with specified user-facing error",
			output:     "tmux 1.0",
			wantErr:    true,
			wantErrMsg: "Portal requires tmux \u2265 3.0 (found 1.0). Please upgrade.",
		},
		{
			name:       "rejects tmux 2.9a (suffixed) with label preserved",
			output:     "tmux 2.9a",
			wantErr:    true,
			wantErrMsg: "Portal requires tmux \u2265 3.0 (found 2.9a). Please upgrade.",
		},
		{
			name:        "wraps commander error when tmux -V fails",
			output:      "",
			runErr:      errors.New("exec failure"),
			wantErr:     true,
			wantContain: "failed to detect tmux version",
		},
		{
			name:        "errors on empty output",
			output:      "",
			wantErr:     true,
			wantContain: "tmux -V returned no output",
		},
		{
			name:        "errors on unparseable output",
			output:      "unintelligible",
			wantErr:     true,
			wantContain: "unintelligible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockCommander{Output: tt.output, Err: tt.runErr}

			err := tmux.CheckTmuxVersion(mock)

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("CheckTmuxVersion() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("CheckTmuxVersion() error = nil, want non-nil")
				}
				if tt.wantErrMsg != "" && err.Error() != tt.wantErrMsg {
					t.Errorf("CheckTmuxVersion() error = %q, want exact %q", err.Error(), tt.wantErrMsg)
				}
				if tt.wantContain != "" && !strings.Contains(err.Error(), tt.wantContain) {
					t.Errorf("CheckTmuxVersion() error = %q, want substring %q", err.Error(), tt.wantContain)
				}
			}

			// Verify it called tmux -V exactly once.
			if len(mock.Calls) != 1 {
				t.Fatalf("expected 1 call, got %d", len(mock.Calls))
			}
			if len(mock.Calls[0]) != 1 || mock.Calls[0][0] != "-V" {
				t.Errorf("called with %v, want [-V]", mock.Calls[0])
			}
		})
	}
}

func TestCheckTmuxVersion_WrapsCommanderError(t *testing.T) {
	t.Run("wraps original error so errors.Is works", func(t *testing.T) {
		sentinel := errors.New("the original cause")
		mock := &MockCommander{Err: sentinel}

		err := tmux.CheckTmuxVersion(mock)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("errors.Is(err, sentinel) = false, want true (err = %v)", err)
		}
	})
}
