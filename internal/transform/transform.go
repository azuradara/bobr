package transform

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/h2non/bimg"
)

type Crop int

const (
	CropNone Crop = iota
	CropCenter
	CropTop
	CropBottom
	CropLeft
	CropRight
)

type Params struct {
	Width  int
	Height int
	Crop   Crop
}

func ParseParams(query url.Values) Params {
	var p Params

	if w := query.Get("width"); w != "" {
		p.Width, _ = strconv.Atoi(w)
	}

	if h := query.Get("height"); h != "" {
		p.Height, _ = strconv.Atoi(h)
	}

	switch strings.ToLower(query.Get("crop")) {
	case "center":
		p.Crop = CropCenter
	case "top":
		p.Crop = CropTop
	case "bottom":
		p.Crop = CropBottom
	case "left":
		p.Crop = CropLeft
	case "right":
		p.Crop = CropRight
	default:
		p.Crop = CropNone
	}

	return p
}

func (p Params) Empty() bool {
	return p.Width == 0 && p.Height == 0
}

func (p Params) CacheKey() string {
	if p.Empty() {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("_t")

	if p.Width > 0 {
		sb.WriteString("w")
		sb.WriteString(strconv.Itoa(p.Width))
	}

	if p.Height > 0 {
		sb.WriteString("h")
		sb.WriteString(strconv.Itoa(p.Height))
	}

	if p.Crop != CropNone {
		sb.WriteString("c")
		sb.WriteString(strconv.Itoa(int(p.Crop)))
	}

	return sb.String()
}

func IsImage(contentType string) bool {
	return strings.HasPrefix(contentType, "image/")
}

func Apply(data []byte, p Params, optimize bool, lossless bool) ([]byte, string, error) {
	imageType := bimg.DetermineImageType(data)

	switch imageType {
	case bimg.JPEG, bimg.HEIF, bimg.AVIF:
	case bimg.PNG:
		if IsAnimated(data) {
			return data, "image/png", nil
		}
	default:
		return data, getContentType(data, imageType), nil
	}

	img := bimg.NewImage(data)

	size, err := img.Size()
	if err != nil {
		return nil, "", err
	}

	opts := bimg.Options{
		Quality:       84,
		StripMetadata: true,
		Interlace:     true,
		NoAutoRotate:  false,
		Interpolator:  bimg.Bicubic,
	}

	if optimize {
		opts.Quality = 84
		opts.Lossless = lossless
		if imageType != bimg.GIF {
			opts.Type = bimg.WEBP
		}
	}

	if !p.Empty() {
		if p.Crop != CropNone {
			opts.Width = p.Width
			opts.Height = p.Height
			opts.Crop = true
			opts.Gravity = cropToGravity(p.Crop)
		} else {
			newW, newH := fitDimensions(size.Width, size.Height, p.Width, p.Height)
			opts.Width = newW
			opts.Height = newH
		}
	}

	result, err := img.Process(opts)
	if err != nil {
		return nil, "", err
	}

	var contentType string
	if optimize && imageType != bimg.GIF {
		contentType = "image/webp"
	} else {
		contentType = getContentType(result, bimg.DetermineImageType(result))
	}

	return result, contentType, nil
}

func IsAnimated(data []byte) bool {
	if len(data) < 12 {
		return false
	}

	if string(data[:3]) == "GIF" {
		return true
	}

	if string(data[8:12]) == "WEBP" {
		if len(data) >= 21 && string(data[12:16]) == "VP8X" {
			flags := data[20]
			if flags&0x02 != 0 {
				return true
			}
		}
	}

	if string(data[1:4]) == "PNG" {
		offset := 8
		for offset+8 < len(data) {
			chunkDataLen := int(
				uint32(
					data[offset],
				)<<24 | uint32(
					data[offset+1],
				)<<16 | uint32(
					data[offset+2],
				)<<8 | uint32(
					data[offset+3],
				),
			)
			chunkType := string(data[offset+4 : offset+8])

			if chunkType == "acTL" {
				return true
			}

			if chunkType == "IEND" {
				break
			}

			offset += chunkDataLen + 12
		}
	}

	return false
}

func Optimize(data []byte) ([]byte, string, error) {
	return Apply(data, Params{}, true, false)
}

func cropToGravity(c Crop) bimg.Gravity {
	switch c {
	case CropCenter:
		return bimg.GravityCentre
	case CropTop:
		return bimg.GravityNorth
	case CropBottom:
		return bimg.GravitySouth
	case CropLeft:
		return bimg.GravityWest
	case CropRight:
		return bimg.GravityEast
	default:
		return bimg.GravityCentre
	}
}

func fitDimensions(origW, origH, targetW, targetH int) (int, int) {
	if targetW == 0 && targetH == 0 {
		return origW, origH
	}

	aspectRatio := float64(origW) / float64(origH)

	if targetW > 0 && targetH > 0 {
		targetAspect := float64(targetW) / float64(targetH)
		if aspectRatio > targetAspect {
			return targetW, int(float64(targetW) / aspectRatio)
		}

		return int(float64(targetH) * aspectRatio), targetH
	}

	if targetW > 0 {
		return targetW, int(float64(targetW) / aspectRatio)
	}

	return int(float64(targetH) * aspectRatio), targetH
}
