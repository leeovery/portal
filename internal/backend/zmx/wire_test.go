package zmx

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// capturedFrame is the pinned build's Info answer, header and record, as
// TestInfoFixture captured it.
func capturedFrame(t testing.TB) []byte {
	t.Helper()
	f, err := os.ReadFile(filepath.Join("testdata", "info.frame"))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestHeader(t *testing.T) {
	t.Run("it reads the pinned build's Info header", func(t *testing.T) {
		h, err := decodeHeader(capturedFrame(t)[:headerSize])

		if err != nil || h.tag != tagInfo || h.version != ProtocolVersion || h.len != infoSize {
			t.Errorf("decodeHeader = %+v, %v; want Info, version 1, %d bytes", h, err, infoSize)
		}
	})

	t.Run("it writes requests as zmx frames them", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			got  []byte
			want []byte
		}{
			{"kill", encodeHeader(tagKill, 0), []byte{5, 1, 0, 0, 0, 0, 0, 0}},
			{"info", encodeHeader(tagInfo, 0), []byte{6, 1, 0, 0, 0, 0, 0, 0}},
			{"history", encodeHeader(tagHistory, 1), []byte{8, 1, 0, 0, 1, 0, 0, 0}},
			{"capture", encodeHeader(tagCapture, captureSize), []byte{22, 1, 0, 0, 8, 0, 0, 0}},
			{"capture request", encodeCapture(historyVT, 0x01020304), []byte{1, 0, 0, 0, 4, 3, 2, 1}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				if !bytes.Equal(tt.got, tt.want) {
					t.Errorf("encoded %s = %v, want %v", tt.name, tt.got, tt.want)
				}
			})
		}
	})

	t.Run("it refuses a header of another protocol version, reporting it", func(t *testing.T) {
		// A packed header of a tag and a u32 length puts the length's low
		// byte where this protocol keeps the version: 552 is 0x228.
		packed := []byte{6, 0x28, 2, 0, 0, 0, 0, 0}

		h, err := decodeHeader(packed)

		if !errors.Is(err, errProtocolVersion) || h.version != 0x28 {
			t.Errorf("decodeHeader(a packed Info header) = %+v, %v; want version 0x28 refused", h, err)
		}
	})

	t.Run("it refuses a frame longer than zmx's own limit", func(t *testing.T) {
		b := encodeHeader(tagHistory, 0)
		binary.NativeEndian.PutUint32(b[4:], maxPayload+1)

		if _, err := decodeHeader(b); !errors.Is(err, errFrame) {
			t.Errorf("decodeHeader of a %d-byte payload = %v, want errFrame", maxPayload+1, err)
		}
	})
}

func TestDecodeInfo(t *testing.T) {
	t.Run("it reads the pinned build's Info record", func(t *testing.T) {
		i, err := decodeInfo(capturedFrame(t)[headerSize:])
		if err != nil {
			t.Fatalf("decodeInfo: %v", err)
		}
		const command = `sh -c 'printf '\''\033]7;file://example-host/srv/demo%%20dir\033\\'\''; sleep 1000'`
		if i.clients != 0 || i.pid <= 0 || i.cmd != command || i.cwd != "file://example-host/srv/demo%20dir" {
			t.Errorf("decodeInfo = %+v", i)
		}
		if frozen := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC); i.createdAt().Before(frozen) {
			t.Errorf("created %v, before the protocol froze", i.createdAt())
		}
	})

	t.Run("it refuses a record of any other size", func(t *testing.T) {
		record := capturedFrame(t)[headerSize:]
		for _, size := range []int{0, infoSize - 1, infoSize + 1, 545} {
			p := make([]byte, size)
			copy(p, record)
			if _, err := decodeInfo(p); !errors.Is(err, errInfoSize) {
				t.Errorf("decodeInfo of %d bytes = %v, want errInfoSize", size, err)
			}
		}
	})

	t.Run("it refuses a record of another protocol version", func(t *testing.T) {
		p := bytes.Clone(capturedFrame(t)[headerSize:])
		p[offVersion] = 2

		if _, err := decodeInfo(p); !errors.Is(err, errProtocolVersion) {
			t.Errorf("decodeInfo of a version 2 record = %v, want errProtocolVersion", err)
		}
	})

	t.Run("it clamps lengths past their arrays, as zmx does", func(t *testing.T) {
		p := bytes.Clone(capturedFrame(t)[headerSize:])
		binary.NativeEndian.PutUint16(p[offCmdLen:], 0xffff)
		binary.NativeEndian.PutUint16(p[offCwdLen:], maxCwd+1)

		i, err := decodeInfo(p)

		if err != nil || len(i.cmd) != maxCmd || len(i.cwd) != maxCwd || !i.cwdTruncated() {
			t.Errorf("decodeInfo with overlong lengths = %d-byte cmd, %d-byte cwd, %v; want both %d and the cwd cut", len(i.cmd), len(i.cwd), err, maxCmd)
		}
	})

	t.Run("it reads a record with no creation time as zero", func(t *testing.T) {
		p := bytes.Clone(capturedFrame(t)[headerSize:])
		clear(p[offCreated : offCreated+8])

		if i, err := decodeInfo(p); err != nil || !i.createdAt().IsZero() {
			t.Errorf("decodeInfo with created_at 0 = %v, %v; want a zero time", i.createdAt(), err)
		}
	})
}

func FuzzDecodeFrame(f *testing.F) {
	f.Add(capturedFrame(f))
	f.Add([]byte{6, 0x28, 2, 0, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) < headerSize {
			return
		}
		_, _ = decodeHeader(b[:headerSize])
		i, err := decodeInfo(b[headerSize:])
		if err != nil {
			return
		}
		if len(b)-headerSize != infoSize || len(i.cmd) > maxCmd || len(i.cwd) > maxCwd {
			t.Errorf("decodeInfo accepted %d bytes as %d-byte cmd, %d-byte cwd", len(b)-headerSize, len(i.cmd), len(i.cwd))
		}
	})
}
