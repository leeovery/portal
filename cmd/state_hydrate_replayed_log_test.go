package cmd

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHydrateReplayedLog(t *testing.T) {
	// NUL, non-UTF8 and escape bytes, so the count is the byte length rather
	// than a rune count.
	for name, body := range map[string]string{
		"a populated transcript": "line1\r\nline2\x00\xff\x1b[31mred\x1b[0m",
		"an empty transcript":    "",
		"a 5MB transcript":       string(bytes.Repeat([]byte("A"), 5*1024*1024)),
	} {
		t.Run("it records the bytes a replay of "+name+" wrote", func(t *testing.T) {
			w := newHydrateWorld(t)
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			cfg := w.config(&stubExecShell{})
			cfg.File, cfg.Logger = transcript(t, body), logger

			runHydrate(t.Context(), cfg)

			rec := sink.Records().Matching("hydrate", "scrollback replayed").Only(t, "the replay's record")
			if got := rec.IntAttr(t, "bytes"); got != int64(len(body)) {
				t.Errorf("bytes = %d, want %d", got, len(body))
			}
			if info := execLogLine(t, sink.Body(), "INFO", "scrollback replayed"); !strings.Contains(info, fmt.Sprintf("bytes=%d", len(body))) {
				t.Errorf("INFO %q does not render bytes=%d", info, len(body))
			}
		})
	}

	t.Run("its took spans the copy, never the settle after it", func(t *testing.T) {
		w := newHydrateWorld(t)
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		cfg := w.config(&stubExecShell{})
		cfg.File, cfg.Logger = transcript(t, "CONTENT"), logger
		cfg.Settle = func() { time.Sleep(hydrateSettleSleep) }

		runHydrate(t.Context(), cfg)

		rec := sink.Records().Matching("hydrate", "scrollback replayed").Only(t, "the replay's record")
		if took := rec.DurationAttr(t, "took"); took >= hydrateSettleSleep {
			t.Errorf("took = %v, want the copy alone, well under the %v settle", took, hydrateSettleSleep)
		}
	})

	t.Run("it precedes the exec INFO", func(t *testing.T) {
		w := newHydrateWorld(t)
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		cfg := w.config(&stubExecShell{})
		cfg.File, cfg.Logger = transcript(t, "DUMP"), logger

		runHydrate(t.Context(), cfg)

		assertLoggedBeforeExec(t, sink.Body(), "INFO scrollback replayed")
	})

	t.Run("it is not recorded for a transcript that could not be read", func(t *testing.T) {
		w := newHydrateWorld(t)
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		cfg := w.config(&stubExecShell{})
		cfg.File, cfg.Logger = filepath.Join(t.TempDir(), "absent.bin"), logger

		runHydrate(t.Context(), cfg)

		if strings.Contains(sink.Body(), "scrollback replayed") {
			t.Errorf("an unread transcript recorded a replay: %q", sink.Body())
		}
	})
}

func TestHydrateFileMissingLog(t *testing.T) {
	cases := map[string]func(t *testing.T) string{
		"missing":          func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.bin") },
		"refused":          func(t *testing.T) string { return unreadableTranscript(t, "HIDDEN") },
		"failing part way": func(t *testing.T) string { return t.TempDir() },
	}
	for name, file := range cases {
		t.Run("a transcript "+name+" records one scrollback missing INFO naming the file, before the exec INFO", func(t *testing.T) {
			w := newHydrateWorld(t)
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			cfg := w.config(&stubExecShell{})
			cfg.File, cfg.Logger = file(t), logger

			runHydrate(t.Context(), cfg)

			body := sink.Body()
			if n := countLogLines(body, "INFO", "scrollback missing"); n != 1 {
				t.Fatalf("got %d scrollback missing INFOs, want 1: %q", n, body)
			}
			info := execLogLine(t, body, "INFO", "scrollback missing")
			if !strings.Contains(info, "path="+cfg.File) || strings.Contains(info, "target=") {
				t.Errorf("INFO %q, want it to name the file by the path attr", info)
			}
			assertLoggedBeforeExec(t, body, "INFO scrollback missing")
		})
	}
}

// assertLoggedBeforeExec fails unless line comes before the exec INFO in body.
func assertLoggedBeforeExec(t *testing.T, body, line string) {
	t.Helper()
	at, execAt := strings.Index(body, line), strings.Index(body, "INFO exec")
	if at < 0 || execAt < 0 || at >= execAt {
		t.Errorf("%q at %d, the exec INFO at %d; want it first: %q", line, at, execAt, body)
	}
}
