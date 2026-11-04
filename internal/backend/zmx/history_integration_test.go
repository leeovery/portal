//go:build integration

package zmx_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/zmxtest"
)

// TestHistoryClosingState holds HistoryContent's reading of zmx to the build:
// each closing state it cuts is the one zmx answers with. A zmx that closes a
// History another way fails here, rather than leaving a restored prompt among
// the rows it replays.
func TestHistoryClosingState(t *testing.T) {
	b := zmxtest.New(t)
	for i, tc := range closingStates {
		mustCreate(t, b, "state-"+strconv.Itoa(i), t.TempDir(), "sh", "-c", tc.script+"; exec sleep 1000")
	}
	for i, tc := range closingStates {
		t.Run("it closes a History with "+tc.name+" as HistoryContent reads it", func(t *testing.T) {
			want := tc.rows + tc.state
			var got []byte
			if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
				var err error
				got, err = b.History(t.Context(), "state-"+strconv.Itoa(i), 0)
				return err == nil && string(got) == want
			}) {
				t.Errorf("History = %q, want %q", got, want)
			}
		})
	}
}
