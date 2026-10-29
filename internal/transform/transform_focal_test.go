package transform

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func halves(t *testing.T, width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			c := color.RGBA{R: 255, A: 255}
			if x >= width/2 {
				c = color.RGBA{B: 255, A: 255}
			}
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))

	return buf.Bytes()
}

func TestParseParams_Focal(t *testing.T) {
	q, _ := url.ParseQuery("width=100&height=100&crop=center&focal=12.5,40")
	assert.Equal(t, &Point{12.5, 40}, ParseParams(q).Focal)
	assert.Equal(t, "_tw100h100c1f12.5,40", ParseParams(q).CacheKey())

	for _, v := range []string{"", "50", "a,b", "101,50", "-1,50"} {
		assert.Nil(t, ParseParams(url.Values{"focal": {v}}).Focal, v)
	}

	q, _ = url.ParseQuery("width=100&height=100&crop=top&focal=12.5,40")
	assert.Equal(t, "_tw100h100c2", ParseParams(q).CacheKey())
}

func TestApply_CropsAroundTheFocalPoint(t *testing.T) {
	src := halves(t, 400, 100)

	for focal, want := range map[string]color.RGBA{"10,50": {R: 255, A: 255}, "90,50": {B: 255, A: 255}} {
		q, _ := url.ParseQuery("width=50&height=50&crop=center&focal=" + focal)
		out, _, err := Apply(src, ParseParams(q), false, false)
		require.NoError(t, err)

		img, _, err := image.Decode(bytes.NewReader(out))
		require.NoError(t, err)
		assert.Equal(t, image.Rect(0, 0, 50, 50), img.Bounds(), focal)

		r, g, b, a := img.At(25, 25).RGBA()
		assert.Equal(
			t,
			want,
			color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)},
			focal,
		)
	}
}

func TestApply_FocalCropNeverEnlarges(t *testing.T) {
	q, _ := url.ParseQuery("width=800&height=400&crop=center&focal=90,50")
	out, _, err := Apply(halves(t, 400, 100), ParseParams(q), false, false)
	require.NoError(t, err)

	img, _, err := image.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, 200, 100), img.Bounds())
}
