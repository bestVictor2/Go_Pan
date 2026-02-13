package utils

import "strings"

// SanitizeHeaderFilename removes characters that can break headers.
func SanitizeHeaderFilename(name string) string {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return "download"
	}
	clean = strings.ReplaceAll(clean, "\r", "")
	clean = strings.ReplaceAll(clean, "\n", "")
	clean = strings.ReplaceAll(clean, "\"", "")
	return clean
}



