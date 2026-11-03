package proctable

import (
	"encoding/binary"
	"errors"
	"slices"
	"testing"
)

// procArgsOf builds a kern.procargs2 buffer as the kernel lays one out.
func procArgsOf(path string, argv, env []string) []byte {
	b := binary.NativeEndian.AppendUint32(nil, uint32(len(argv)))
	b = append(b, path...)
	b = append(b, 0, 0, 0, 0)
	for _, s := range append(slices.Clone(argv), env...) {
		b = append(append(b, s...), 0)
	}
	return append(b, 0, 'p', 't', 'r', '_', 'm', 'u', 'n', 'g', 'e', '=', 0)
}

func TestParseArgs(t *testing.T) {
	t.Run("it reads argv and the environment, and stops at the end of the environment", func(t *testing.T) {
		argv := []string{"/opt/zmx", "attach", "has space", ""}
		env := []string{"ZMX_DIR=/tmp/zx-1", "HOME=/h", "EMPTY="}

		got, err := parseArgs(procArgsOf("/opt/zmx", argv, env))

		if err != nil || !slices.Equal(got.Argv, argv) || !slices.Equal(got.Env, env) {
			t.Errorf("parseArgs = %+v, %v; want argv %q, env %q", got, err, argv, env)
		}
	})

	t.Run("it refuses a buffer cut short or with a bad count", func(t *testing.T) {
		whole := procArgsOf("/opt/zmx", []string{"zmx", "attach"}, nil)
		negative := binary.NativeEndian.AppendUint32(nil, 0xffffffff)
		for name, raw := range map[string][]byte{
			"empty":                  nil,
			"no executable path end": []byte{1, 0, 0, 0, '/', 'z'},
			"arguments cut short":    whole[:len(whole)-20],
			"negative count":         append(negative, "/x\x00a\x00"...),
		} {
			t.Run(name, func(t *testing.T) {
				if got, err := parseArgs(raw); !errors.Is(err, errMalformed) {
					t.Errorf("parseArgs(%q) = %+v, %v; want errMalformed", raw, got, err)
				}
			})
		}
	})
}

func FuzzParseArgs(f *testing.F) {
	f.Add(procArgsOf("/opt/zmx", []string{"zmx", "attach", "x"}, []string{"ZMX_DIR=/tmp/zx-1"}))
	f.Fuzz(func(t *testing.T, raw []byte) {
		got, err := parseArgs(raw)
		if err == nil && slices.ContainsFunc(got.Env, func(kv string) bool { return kv == "" }) {
			t.Errorf("parseArgs(%q) returned an empty environment entry", raw)
		}
	})
}
