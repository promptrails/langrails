package mediautil

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/promptrails/langrails"
)

// Support describes which non-text, non-image content parts a provider can
// send.
type Support struct {
	// Audio reports whether audio parts are accepted at all.
	Audio bool
	// AudioFormats, when non-nil, limits audio to these media types.
	AudioFormats []string
	// DocumentData reports whether base64 document parts are accepted.
	DocumentData bool
	// DocumentURL reports whether URL document parts are accepted.
	DocumentURL bool
}

// CheckParts returns an error wrapping langrails.ErrUnsupportedContent for
// the first content part in req the provider cannot send, and for
// malformed audio/document parts (no data, no media type).
func CheckParts(provider string, req *langrails.CompletionRequest, s Support) error {
	for i, m := range req.Messages {
		for j, p := range m.ContentParts {
			where := fmt.Sprintf("%s: message %d part %d", provider, i, j)
			switch p.Type {
			case langrails.ContentAudio:
				if !s.Audio {
					return fmt.Errorf("%s: audio input: %w", where, langrails.ErrUnsupportedContent)
				}
				if p.Data == "" || p.MediaType == "" {
					return fmt.Errorf("%s: audio part needs Data and MediaType", where)
				}
				if s.AudioFormats != nil && !contains(s.AudioFormats, p.MediaType) {
					return fmt.Errorf("%s: audio format %q (supported: %s): %w",
						where, p.MediaType, strings.Join(s.AudioFormats, ", "), langrails.ErrUnsupportedContent)
				}
			case langrails.ContentDocument:
				switch {
				case p.Data != "":
					if !s.DocumentData {
						return fmt.Errorf("%s: document input: %w", where, langrails.ErrUnsupportedContent)
					}
					if p.MediaType == "" {
						return fmt.Errorf("%s: document part needs a MediaType", where)
					}
				case p.URL != "":
					if !s.DocumentURL {
						return fmt.Errorf("%s: document by URL (send the data inline instead): %w", where, langrails.ErrUnsupportedContent)
					}
				default:
					return fmt.Errorf("%s: document part needs Data or URL", where)
				}
			}
		}
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// DataURI builds a base64 data URI from a media type and payload.
func DataURI(mediaType, data string) string {
	return "data:" + mediaType + ";base64," + data
}

// DecodeText decodes a base64 payload holding text. It returns the input
// unchanged if it is not valid base64.
func DecodeText(data string) string {
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return data
	}
	return string(b)
}

// Filename returns name, or a generated "document-<n>" when it is empty.
func Filename(name string, n int) string {
	if name != "" {
		return name
	}
	return fmt.Sprintf("document-%d", n)
}
