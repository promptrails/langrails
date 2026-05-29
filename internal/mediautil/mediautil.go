// Package mediautil provides small helpers for handling image references in
// multimodal messages, shared by the providers. Stdlib only.
package mediautil

import "strings"

// ParseImageURL classifies an image reference from a Message ContentPart.
//
// For a base64 data URI ("data:image/png;base64,...."), it returns the media
// type (e.g. "image/png"), the base64 payload, and isBase64=true. Otherwise it
// treats the input as a plain URL and returns it via the url result.
func ParseImageURL(s string) (mediaType, data, url string, isBase64 bool) {
	if !strings.HasPrefix(s, "data:") {
		return "", "", s, false
	}
	// data:<mediatype>;base64,<data>
	rest := strings.TrimPrefix(s, "data:")
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return "", "", s, false
	}
	meta, payload := rest[:comma], rest[comma+1:]
	if !strings.Contains(meta, "base64") {
		// Non-base64 data URIs aren't supported; treat as opaque URL.
		return "", "", s, false
	}
	mediaType = strings.TrimSuffix(meta, ";base64")
	return mediaType, payload, "", true
}

// ImageFormat returns the short image format from a media type, e.g.
// "image/png" -> "png". Returns "" when the input isn't an image media type.
func ImageFormat(mediaType string) string {
	if !strings.HasPrefix(mediaType, "image/") {
		return ""
	}
	return strings.TrimPrefix(mediaType, "image/")
}
