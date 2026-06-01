package log

import (
	"log/slog"
	"testing"
)

func TestResolveLevel_DefaultsToInfoWhenUnset(t *testing.T) {
	lvl, source, raw := resolveLevel("")

	if lvl != slog.LevelInfo {
		t.Errorf("level = %v, want %v", lvl, slog.LevelInfo)
	}
	if source != sourceDefault {
		t.Errorf("source = %q, want %q", source, sourceDefault)
	}
	if raw != "" {
		t.Errorf("raw = %q, want %q", raw, "")
	}
}

func TestResolveLevel_ResolvesEachValidLevelWithSourceEnv(t *testing.T) {
	cases := []struct {
		raw  string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			lvl, source, raw := resolveLevel(tc.raw)

			if lvl != tc.want {
				t.Errorf("level = %v, want %v", lvl, tc.want)
			}
			if source != sourceEnv {
				t.Errorf("source = %q, want %q", source, sourceEnv)
			}
			if raw != tc.raw {
				t.Errorf("raw = %q, want verbatim %q", raw, tc.raw)
			}
		})
	}
}

func TestResolveLevel_NormalisesMixedCaseAndWhitespace(t *testing.T) {
	cases := []struct {
		raw  string
		want slog.Level
	}{
		{"DEBUG", slog.LevelDebug},
		{"Warn", slog.LevelWarn},
		{"  info  ", slog.LevelInfo},
		{"\tERROR\n", slog.LevelError},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			lvl, source, raw := resolveLevel(tc.raw)

			if lvl != tc.want {
				t.Errorf("level = %v, want %v", lvl, tc.want)
			}
			if source != sourceEnv {
				t.Errorf("source = %q, want %q", source, sourceEnv)
			}
			// raw is preserved verbatim (not trimmed/lowercased).
			if raw != tc.raw {
				t.Errorf("raw = %q, want verbatim %q", raw, tc.raw)
			}
		})
	}
}

func TestResolveLevel_RejectsLegacyWarningAlias(t *testing.T) {
	lvl, source, raw := resolveLevel("warning")

	if lvl != slog.LevelInfo {
		t.Errorf("level = %v, want %v (legacy alias must not map to WARN)", lvl, slog.LevelInfo)
	}
	if source != sourceFallback {
		t.Errorf("source = %q, want %q", source, sourceFallback)
	}
	if raw != "warning" {
		t.Errorf("raw = %q, want %q", raw, "warning")
	}
}

func TestResolveLevel_FallsBackToInfoForInvalidValuePreservingRaw(t *testing.T) {
	cases := []string{"trace", "verbose", "5", "WARNING", "  bogus  "}

	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			lvl, source, gotRaw := resolveLevel(raw)

			if lvl != slog.LevelInfo {
				t.Errorf("level = %v, want %v", lvl, slog.LevelInfo)
			}
			if source != sourceFallback {
				t.Errorf("source = %q, want %q", source, sourceFallback)
			}
			if gotRaw != raw {
				t.Errorf("raw = %q, want verbatim %q", gotRaw, raw)
			}
		})
	}
}

func TestLevelString_MapsLevelToLowercaseToken(t *testing.T) {
	cases := []struct {
		lvl  slog.Level
		want string
	}{
		{slog.LevelDebug, "debug"},
		{slog.LevelInfo, "info"},
		{slog.LevelWarn, "warn"},
		{slog.LevelError, "error"},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := levelString(tc.lvl); got != tc.want {
				t.Errorf("levelString(%v) = %q, want %q", tc.lvl, got, tc.want)
			}
		})
	}
}
