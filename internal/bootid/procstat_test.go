package bootid

import (
	"testing"
	"time"
)

func TestBtime(t *testing.T) {
	t.Run("it reads the btime line", func(t *testing.T) {
		stat := "cpu  1 2 3\nintr 9\nbtime 1793183873\nprocesses 42\n"

		got, err := btime([]byte(stat))

		if want := time.Unix(1793183873, 0); err != nil || !got.Equal(want) {
			t.Errorf("btime = %v, %v; want %v", got, err, want)
		}
	})

	for name, stat := range map[string]string{
		"no btime line":             "cpu  1 2 3\nprocesses 42\n",
		"a btime that is no number": "btime soon\n",
	} {
		t.Run("a stat with "+name+" is an error", func(t *testing.T) {
			if got, err := btime([]byte(stat)); err == nil {
				t.Errorf("btime = %v, nil; want an error", got)
			}
		})
	}
}
