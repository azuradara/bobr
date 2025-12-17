package transform

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAnimated(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "Empty data",
			data:     []byte{},
			expected: false,
		},
		{
			name:     "Too short data",
			data:     []byte{0x00, 0x01},
			expected: false,
		},
		{
			name:     "GIF header",
			data:     []byte("GIF89a................"),
			expected: true,
		},
		{
			name:     "Normal WebP (VP8)",
			data:     append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 20)...),
			expected: false,
		},
		{
			name: "Animated WebP (VP8X with Animation Bit)",
			data: func() []byte {
				buf := make([]byte, 30)
				copy(buf[0:], "RIFF")
				copy(buf[8:], "WEBP")
				copy(buf[12:], "VP8X")
				buf[20] = 0x02
				return buf
			}(),
			expected: true,
		},
		{
			name: "Static WebP (VP8X without Animation Bit)",
			data: func() []byte {
				buf := make([]byte, 30)
				copy(buf[0:], "RIFF")
				copy(buf[8:], "WEBP")
				copy(buf[12:], "VP8X")
				buf[20] = 0x00
				return buf
			}(),
			expected: false,
		},
		{
			name: "Normal PNG",
			data: func() []byte {
				buf := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
				ihdrLen := make([]byte, 4)
				binary.BigEndian.PutUint32(ihdrLen, 13)
				buf = append(buf, ihdrLen...)
				buf = append(buf, []byte("IHDR")...)
				buf = append(buf, make([]byte, 13)...)
				buf = append(buf, make([]byte, 4)...)

				buf = append(buf, 0, 0, 0, 0)
				buf = append(buf, []byte("IEND")...)
				buf = append(buf, 0, 0, 0, 0)
				return buf
			}(),
			expected: false,
		},
		{
			name: "APNG (PNG with acTL)",
			data: func() []byte {
				buf := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

				ihdrLen := make([]byte, 4)
				binary.BigEndian.PutUint32(ihdrLen, 13)
				buf = append(buf, ihdrLen...)
				buf = append(buf, []byte("IHDR")...)
				buf = append(buf, make([]byte, 13+4)...)

				actlLen := make([]byte, 4)
				binary.BigEndian.PutUint32(actlLen, 8)
				buf = append(buf, actlLen...)
				buf = append(buf, []byte("acTL")...)
				buf = append(buf, make([]byte, 8+4)...)

				buf = append(buf, 0, 0, 0, 0)
				buf = append(buf, []byte("IEND")...)
				buf = append(buf, 0, 0, 0, 0)
				return buf
			}(),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsAnimated(tt.data))
		})
	}
}
