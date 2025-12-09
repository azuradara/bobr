package transform

import (
	"path/filepath"
	"strings"
)

// Deprecated: for legacy garbage that i'm too lazy to update
func ParsePreset(
	filename string,
	presets map[string]int,
) (originalFilename string, width int, ok bool) {
	ext := filepath.Ext(filename)
	nameWithoutExt := strings.TrimSuffix(filename, ext)

	for suffix, w := range presets {
		presetSuffix := "_" + suffix
		if strings.HasSuffix(nameWithoutExt, presetSuffix) {
			originalName := strings.TrimSuffix(nameWithoutExt, presetSuffix) + ext

			return originalName, w, true
		}
	}

	return "", 0, false
}

// Deprecated: for legacy garbage that i'm too lazy to update
func IsPresetCandidate(filename string) bool {
	ext := filepath.Ext(filename)
	nameWithoutExt := strings.TrimSuffix(filename, ext)

	if len(nameWithoutExt) < 3 {
		return false
	}

	suffixToCheck := nameWithoutExt[len(nameWithoutExt)-3:]

	return strings.HasPrefix(suffixToCheck, "_")
}
