package capture

import (
	"os"
	"testing"
)

// The frames are drawn with narrow East Asian widths wherever the tests run.
func TestMain(m *testing.M) {
	os.Exit(RunNarrow(m))
}
