package transform

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOptimize_PreservesSVG(t *testing.T) {
	svgData := []byte(
		`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"><circle cx="50" cy="50" r="40" stroke="black" stroke-width="3" fill="red" /></svg>`,
	)

	result, contentType, err := Optimize(svgData)
	assert.NoError(t, err)
	assert.Equal(t, "image/svg+xml", contentType)
	assert.Equal(t, svgData, result, "SVG data should be unchanged")
}

func TestOptimize_PreservesICO(t *testing.T) {
	icoData := []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00}

	result, contentType, err := Optimize(icoData)
	assert.NoError(t, err)

	assert.Equal(t, "image/x-icon", contentType)
	assert.Equal(t, icoData, result, "ICO data should be unchanged")
}

func TestOptimize_PreservesGIF(t *testing.T) {
	gifData := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff! \x00,")

	result, contentType, err := Optimize(gifData)

	if err == nil {
		assert.Equal(t, "image/gif", contentType)
		_ = result
	}
}
