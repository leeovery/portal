package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tui"
)

var errNoTranscript = errors.New("no saved transcript")

// indexTranscripts reads a session's saved transcript from the state
// directory dir: the scrollback file its sessions.json entry names, found as
// restore finds the file it hands the hydrate helper, so a file the entry
// names that is no transcript of its session is never read.
type indexTranscripts struct{ dir string }

func (t indexTranscripts) Transcript(session string) ([]byte, error) {
	idx, err := sessionindex.Read(t.dir)
	if err != nil {
		return nil, err
	}
	e, ok := idx.Lookup(session)
	if !ok || e.ScrollbackFile == "" {
		return nil, fmt.Errorf("%w: %q", errNoTranscript, session)
	}
	path, err := state.ScrollbackPath(t.dir, e)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// pickerTranscripts is the preview's read of a waiting session's transcript.
// A state directory that can't be resolved leaves it unwired, and a waiting
// session previews its History.
func pickerTranscripts() tui.TranscriptReader {
	dir, err := state.Dir()
	if err != nil {
		return nil
	}
	return indexTranscripts{dir: dir}
}
