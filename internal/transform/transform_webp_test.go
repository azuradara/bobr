package transform

import (
	"testing"

	"github.com/h2non/bimg"
	"github.com/stretchr/testify/assert"
)

func staticWebp(t *testing.T, width, height int) []byte {
	t.Helper()

	data, err := bimg.NewImage(generateRandomImage(width, height)).Convert(bimg.WEBP)
	assert.NoError(t, err)
	assert.Equal(t, bimg.WEBP, bimg.DetermineImageType(data))
	assert.False(t, IsAnimated(data))

	return data
}

func TestApply_ResizesStaticWebp(t *testing.T) {
	src := staticWebp(t, 800, 600)

	out, contentType, err := Apply(src, Params{Width: 200}, false, false)
	assert.NoError(t, err)
	assert.Equal(t, "image/webp", contentType)

	size, err := bimg.NewImage(out).Size()
	assert.NoError(t, err)
	assert.Equal(t, 200, size.Width)
}

func TestApply_LeavesAnimatedGifAlone(t *testing.T) {
	gif := []byte("GIF89a")
	gif = append(gif, make([]byte, 32)...)

	out, _, err := Apply(gif, Params{Width: 10}, false, false)
	assert.NoError(t, err)
	assert.Equal(t, gif, out)
}
