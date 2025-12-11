package transform

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"testing"
)

func generateRandomImage(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(rand.Intn(256)),
				G: uint8(rand.Intn(256)),
				B: uint8(rand.Intn(256)),
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)

	return buf.Bytes()
}

func BenchmarkApply(b *testing.B) {
	srcData := generateRandomImage(1024, 1024)

	benchmarks := []struct {
		name   string
		params Params
	}{
		{"Resize_Small", Params{Width: 100, Height: 100}},
		{"Resize_Medium", Params{Width: 500, Height: 500}},
		{"Crop_Center", Params{Width: 500, Height: 500, Crop: CropCenter}},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := Apply(srcData, bm.params, false)
				if err != nil {
					b.Fatalf("Apply failed: %v", err)
				}
			}
		})
	}
}

func BenchmarkApplyParallel(b *testing.B) {
	srcData := generateRandomImage(1024, 1024)

	benchmarks := []struct {
		name   string
		params Params
	}{
		{"Resize_Small", Params{Width: 100, Height: 100}},
		{"Resize_Medium", Params{Width: 500, Height: 500}},
		{"Crop_Center", Params{Width: 500, Height: 500, Crop: CropCenter}},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, _, err := Apply(srcData, bm.params, false)
					if err != nil {
						b.Fatalf("Apply failed: %v", err)
					}
				}
			})
		})
	}
}

func BenchmarkOptimize(b *testing.B) {
	srcData := generateRandomImage(1024, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Optimize(srcData)
		if err != nil {
			b.Fatalf("Optimize failed: %v", err)
		}
	}
}

func BenchmarkOptimizeParallel(b *testing.B) {
	srcData := generateRandomImage(1024, 1024)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, err := Optimize(srcData)
			if err != nil {
				b.Fatalf("Optimize failed: %v", err)
			}
		}
	})
}
