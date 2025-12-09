package transform

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseParams(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		expect Params
	}{
		{"empty", "", Params{}},
		{"width only", "width=100", Params{Width: 100}},
		{"height only", "height=200", Params{Height: 200}},
		{"both", "width=100&height=200", Params{Width: 100, Height: 200}},
		{"crop center", "width=100&height=100&crop=center", Params{Width: 100, Height: 100, Crop: CropCenter}},
		{"crop top", "crop=top", Params{Crop: CropTop}},
		{"crop bottom", "crop=BOTTOM", Params{Crop: CropBottom}},
		{"crop left", "crop=left", Params{Crop: CropLeft}},
		{"crop right", "crop=right", Params{Crop: CropRight}},
		{"invalid crop", "crop=invalid", Params{Crop: CropNone}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, _ := url.ParseQuery(tt.query)
			p := ParseParams(q)
			assert.Equal(t, tt.expect, p)
		})
	}
}

func TestParams_Empty(t *testing.T) {
	assert.True(t, Params{}.Empty())
	assert.False(t, Params{Width: 100}.Empty())
	assert.False(t, Params{Height: 100}.Empty())
	assert.True(t, Params{Crop: CropCenter}.Empty())
}

func TestParams_CacheKey(t *testing.T) {
	tests := []struct {
		params Params
		expect string
	}{
		{Params{}, ""},
		{Params{Width: 100}, "_tw100"},
		{Params{Height: 200}, "_th200"},
		{Params{Width: 100, Height: 200}, "_tw100h200"},
		{Params{Width: 100, Height: 100, Crop: CropCenter}, "_tw100h100c1"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expect, tt.params.CacheKey())
	}
}

func TestIsImage(t *testing.T) {
	assert.True(t, IsImage("image/jpeg"))
	assert.True(t, IsImage("image/png"))
	assert.True(t, IsImage("image/webp"))
	assert.True(t, IsImage("image/heic"))
	assert.False(t, IsImage("text/html"))
	assert.False(t, IsImage("application/json"))
}

func TestFitDimensions(t *testing.T) {
	tests := []struct {
		origW, origH, targetW, targetH int
		expectW, expectH               int
	}{
		{1000, 500, 0, 0, 1000, 500},
		{1000, 500, 500, 0, 500, 250},
		{1000, 500, 0, 250, 500, 250},
		{1000, 500, 500, 500, 500, 250},
		{500, 1000, 500, 500, 250, 500},
	}

	for _, tt := range tests {
		w, h := fitDimensions(tt.origW, tt.origH, tt.targetW, tt.targetH)
		assert.Equal(t, tt.expectW, w, "width mismatch for %+v", tt)
		assert.Equal(t, tt.expectH, h, "height mismatch for %+v", tt)
	}
}
