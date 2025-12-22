package transform

import "github.com/h2non/bimg"

func getContentType(data []byte, imageType bimg.ImageType) string {
	typeName := bimg.ImageTypeName(imageType)

	if typeName == "svg" {
		return "image/svg+xml"
	}

	if typeName == "pdf" {
		return "application/pdf"
	}

	if typeName != "unknown" && typeName != "" {
		return "image/" + typeName
	}

	if len(data) > 4 && data[0] == 0x00 && data[1] == 0x00 && data[2] == 0x01 && data[3] == 0x00 {
		return "image/x-icon"
	}

	return "application/octet-stream"
}
